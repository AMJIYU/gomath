package svg

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"gomath"
	"gomath/geom"
	"gomath/rasterimage"
	"image"
	"math"
	"strings"
)

// Budgets are per import. Repeated URI text counts toward input/URI limits;
// identical URIs share one immutable asset and consume decode budgets once.
const (
	maxSVGInputBytes          = 32 << 20
	maxImageURIBytes          = 12 << 20
	maxImageTotalURIBytes     = 24 << 20
	maxImageEncodedBytes      = 8 << 20
	maxImageTotalEncodedBytes = 16 << 20
	maxImagePixels            = 4 << 20
	maxImageTotalPixels       = 16 << 20
	maxImageAssets            = 128
)

type imageBudget struct{ uri, encoded, pixels, assets int }

func (b *imageBudget) consumeURI(n int) error {
	b.uri += n
	if n > maxImageURIBytes || b.uri > maxImageTotalURIBytes {
		return fmt.Errorf("SVG image URI budget exceeded (per image %d, total %d bytes)", maxImageURIBytes, maxImageTotalURIBytes)
	}
	return nil
}

func (doc *document) prepareImages() error {
	cache := make(map[string]*gomath.Bitmap)
	budget := imageBudget{}
	for _, e := range doc.elements {
		if e.name.Local != "image" {
			continue
		}
		for key := range e.attrs {
			switch key {
			case "id", "class", "href", "x", "y", "width", "height", "preserveAspectRatio", "transform", "style", "xmlns":
			default:
				if !cssProperty(key) {
					return fmt.Errorf("unsupported SVG image attribute %q", key)
				}
			}
		}
		uri := e.attrs["href"]
		if err := budget.consumeURI(len(uri)); err != nil {
			return err
		}
		if asset := cache[uri]; asset != nil {
			e.bitmap = asset
			continue
		}
		asset, err := decodeImageURI(uri, &budget)
		if err != nil {
			return fmt.Errorf("SVG image %q: %w", e.attrs["id"], err)
		}
		cache[uri], e.bitmap = asset, asset
	}
	return nil
}

func decodeImageURI(uri string, budget *imageBudget) (*gomath.Bitmap, error) {
	header, body, ok := strings.Cut(uri, ",")
	var format string
	switch strings.ToLower(header) {
	case "data:image/png;base64":
		format = "png"
	case "data:image/jpeg;base64":
		format = "jpeg"
	case "data:image/gif;base64":
		format = "gif"
	default:
		return nil, fmt.Errorf("requires a base64 PNG/JPEG/GIF data URI; external href and other encodings are unsupported")
	}
	if !ok || body == "" {
		return nil, fmt.Errorf("empty image data URI")
	}
	// Canonical padded base64 only; do not accept newline folding or URL escapes.
	if len(body)%4 != 0 || strings.ContainsAny(body, "\r\n\t %") {
		return nil, fmt.Errorf("invalid SVG image base64")
	}
	n := base64.StdEncoding.DecodedLen(len(body))
	if strings.HasSuffix(body, "==") {
		n -= 2
	} else if strings.HasSuffix(body, "=") {
		n--
	}
	if n > maxImageEncodedBytes || budget.encoded+n > maxImageTotalEncodedBytes {
		return nil, fmt.Errorf("SVG image encoded byte budget exceeded")
	}
	if budget.assets >= maxImageAssets {
		return nil, fmt.Errorf("SVG image asset count exceeds %d", maxImageAssets)
	}
	data, err := base64.StdEncoding.Strict().DecodeString(body)
	if err != nil {
		return nil, fmt.Errorf("invalid SVG image base64: %w", err)
	}
	config, detected, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("SVG image header: %w", err)
	}
	if detected != format {
		return nil, fmt.Errorf("SVG image MIME mismatch: declared %s, decoded %s", format, detected)
	}
	if config.Width <= 0 || config.Height <= 0 || config.Width > maxImagePixels/config.Height {
		return nil, fmt.Errorf("SVG image decoded pixel budget exceeds %d pixels", maxImagePixels)
	}
	pixels := config.Width * config.Height
	if budget.pixels+pixels > maxImageTotalPixels {
		return nil, fmt.Errorf("SVG image total decoded pixel budget exceeds %d pixels", maxImageTotalPixels)
	}
	object, err := rasterimage.Decode(bytes.NewReader(data), 1)
	if err != nil {
		return nil, fmt.Errorf("SVG image decode: %w", err)
	}
	// rasterimage intentionally decodes only GIF frame zero. Partial canvas
	// frames require background/disposal composition, which this importer rejects.
	bounds := object.Bitmap.Bounds()
	if bounds.Dx() != config.Width || bounds.Dy() != config.Height {
		return nil, fmt.Errorf("SVG GIF first frame must cover the full logical canvas")
	}
	budget.encoded += len(data)
	budget.pixels += pixels
	budget.assets++
	return object.Bitmap, nil
}

func imageDimensions(e *element, viewport [2]float64) ([4]float64, error) {
	if e.bitmap == nil {
		return [4]float64{}, fmt.Errorf("SVG image has no decoded asset")
	}
	// Use attributes width/height apply to symbols/svg only, not image targets.
	bounds := e.bitmap.Bounds()
	iw, ih := float64(bounds.Dx()), float64(bounds.Dy())
	x, err := viewportLength(e.attrs, "x", 0, viewport[0])
	if err != nil {
		return [4]float64{}, err
	}
	y, err := viewportLength(e.attrs, "y", 0, viewport[1])
	if err != nil {
		return [4]float64{}, err
	}
	w, err := viewportLength(e.attrs, "width", iw, viewport[0])
	if err != nil || w < 0 {
		return [4]float64{}, fmt.Errorf("invalid SVG image width")
	}
	h, err := viewportLength(e.attrs, "height", ih, viewport[1])
	if err != nil || h < 0 {
		return [4]float64{}, fmt.Errorf("invalid SVG image height")
	}
	// One auto dimension uses the specified dimension and intrinsic aspect ratio.
	autoW := e.attrs["width"] == "" || strings.TrimSpace(e.attrs["width"]) == "auto"
	autoH := e.attrs["height"] == "" || strings.TrimSpace(e.attrs["height"]) == "auto"
	if autoW && !autoH {
		w = h * iw / ih
	}
	if autoH && !autoW {
		h = w * ih / iw
	}
	if !valid(x+w) || !valid(y+h) {
		return [4]float64{}, fmt.Errorf("SVG image viewport exceeds finite coordinate range")
	}
	return [4]float64{x, y, w, h}, nil
}

func imageViewport(e *element, viewport [2]float64) (geom.Bounds, error) {
	d, err := imageDimensions(e, viewport)
	if err != nil {
		return geom.Bounds{}, err
	}
	return geom.Bounds{Min: geom.V(d[0], d[1]), Max: geom.V(d[0]+d[2], d[1]+d[3]), Valid: true}, nil
}

func imageInstance(e *element, ctx expansionContext) (*gomath.Object, error) {
	d, err := imageDimensions(e, ctx.viewport)
	if err != nil {
		return nil, err
	}
	bounds := e.bitmap.Bounds()
	iw, ih := float64(bounds.Dx()), float64(bounds.Dy())
	x, y, w, h := d[0], d[1], d[2], d[3]
	aspect := strings.Fields(e.attrs["preserveAspectRatio"])
	if len(aspect) == 0 {
		aspect = []string{"xMidYMid", "meet"}
	}
	if len(aspect) > 2 || len(aspect) == 2 && aspect[1] != "meet" {
		return nil, fmt.Errorf("SVG image preserveAspectRatio supports meet/none only; slice/clipping is unsupported")
	}
	if aspect[0] != "none" {
		align := map[string][2]float64{
			"xMinYMin": {0, 0}, "xMidYMin": {.5, 0}, "xMaxYMin": {1, 0},
			"xMinYMid": {0, .5}, "xMidYMid": {.5, .5}, "xMaxYMid": {1, .5},
			"xMinYMax": {0, 1}, "xMidYMax": {.5, 1}, "xMaxYMax": {1, 1},
		}
		a, ok := align[aspect[0]]
		if !ok {
			return nil, fmt.Errorf("invalid SVG image preserveAspectRatio alignment %q", aspect[0])
		}
		scale := math.Min(w/iw, h/ih)
		dw, dh := iw*scale, ih*scale
		x, y, w, h = x+(w-dw)*a[0], y+(h-dh)*a[1], dw, dh
	}
	// Native bitmap planes use Y up; flip locally so SVG row zero stays at top
	// after Read's document-wide SVG-to-world coordinate conversion.
	mapping := geom.Translation(geom.V(x+w/2, y+h/2)).Mul(geom.Scaling(w/iw, -h/ih, 1))
	local := ctx.transformed(mapping)
	if !finiteMatrix(local.matrix) {
		return nil, fmt.Errorf("SVG image placement exceeds finite coordinate range")
	}
	object, err := gomath.NewImage(e.bitmap, iw)
	if err != nil {
		return nil, err
	}
	for _, clip := range local.clips {
		box := object.Path.Transform(clip.matrix).Bounds()
		if w > 0 && h > 0 && box.Valid && (box.Min.X < -1e-8 || box.Min.Y < -1e-8 || box.Max.X > clip.width+1e-8 || box.Max.Y > clip.height+1e-8) {
			return nil, fmt.Errorf("SVG image requires symbol viewport clipping; clipping is unsupported")
		}
	}
	object.ApplyMatrix(local.matrix)
	// Paint properties do not apply to SVG raster pixels. Object/group opacity,
	// display and visibility are assigned by the common instantiation path.
	if err := object.ValidateVisual(); err != nil {
		return nil, fmt.Errorf("invalid SVG image plane: %w", err)
	}
	return object, nil
}
