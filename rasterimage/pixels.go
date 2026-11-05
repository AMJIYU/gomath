package rasterimage

import (
	"fmt"
	"gomath"
	"image"
	"image/color"
	"math"
	"reflect"
)

// MaxPixelBytes bounds the retained pixel storage of one Asset (four straight
// bytes, four premultiplied bytes, and one saved-alpha byte per pixel). Derived
// assets share only the immutable saved-alpha buffer. Callers bound the number
// of assets they retain; this is not a process-wide memory limit.
const MaxPixelBytes = 64 * 1024 * 1024

const MaxPixelPixels = MaxPixelBytes / 9

// Asset is an immutable pixel-editing value. Its Bitmap is premultiplied RGBA;
// a private straight-color buffer avoids repeated unpremultiplication and loss
// of low-alpha RGB during edits. All methods are safe for concurrent readers.
// Pixel operations follow ImageMobject's uint8 rules at Manim commit
// 16ea443e2a5f1d79db0a721161411ce916d6e733 (MIT; see THIRD_PARTY_NOTICES.md).
type Asset struct {
	bitmap        *gomath.Bitmap
	straight      *image.NRGBA
	originalAlpha []byte
}

// NewAsset copies a bounded image, normalizing its origin to (0,0). Straight
// NRGBA input retains every RGB byte with nonzero 8-bit alpha. Already
// premultiplied input can only supply its recoverable straight color. Hidden
// RGB at alpha zero is discarded on import, including 16-bit subbyte alpha
// that quantizes to zero. The caller must not mutate source during this call.
func NewAsset(source image.Image) (*Asset, error) {
	if nilPixelValue(source) {
		return nil, fmt.Errorf("pixel asset: nil image")
	}
	b := source.Bounds()
	width, height, err := pixelDimensions(b)
	if err != nil {
		return nil, err
	}
	pixels := image.NewNRGBA(image.Rect(0, 0, width, height))
	alphas := make([]byte, width*height)
	for y := range height {
		for x := range width {
			c := source.At(b.Min.X+x, b.Min.Y+y)
			if nilPixelValue(c) {
				return nil, fmt.Errorf("pixel asset: nil color at (%d,%d)", x, y)
			}
			// NRGBA conversion must precede premultiplication, especially at A=1.
			// Check generic premultiplied inputs before conversion can wrap them.
			r, g, blue, a := c.RGBA()
			if a > 65535 || r > a || g > a || blue > a {
				return nil, fmt.Errorf("pixel asset: invalid premultiplied color at (%d,%d)", x, y)
			}
			var n color.NRGBA
			switch c := c.(type) {
			case color.NRGBA:
				n = c
			case color.NRGBA64:
				n = color.NRGBA{R: uint8(c.R >> 8), G: uint8(c.G >> 8), B: uint8(c.B >> 8), A: uint8(c.A >> 8)}
			default:
				n = color.NRGBAModel.Convert(c).(color.NRGBA)
			}
			if n.A == 0 {
				n = color.NRGBA{}
			}
			pixels.SetNRGBA(x, y, n)
			alphas[y*width+x] = n.A
		}
	}
	return makePixelAsset(pixels, alphas)
}

func nilPixelValue(v any) bool {
	if v == nil {
		return true
	}
	r := reflect.ValueOf(v)
	switch r.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Interface, reflect.Func, reflect.Chan:
		return r.IsNil()
	}
	return false
}

func pixelDimensions(b image.Rectangle) (int, int, error) {
	if b.Max.X <= b.Min.X || b.Max.Y <= b.Min.Y {
		return 0, 0, fmt.Errorf("pixel asset: empty or reversed image bounds")
	}
	// Unsigned differences avoid signed coordinate-span overflow, including a
	// rectangle spanning MinInt..MaxInt. Validate before allocating or reading.
	w, h := uint(b.Max.X)-uint(b.Min.X), uint(b.Max.Y)-uint(b.Min.Y)
	if w > MaxPixelPixels || h > MaxPixelPixels || w > uint(MaxPixelPixels)/h {
		return 0, 0, fmt.Errorf("pixel asset: exceeds %d pixels (%d retained bytes)", MaxPixelPixels, MaxPixelBytes)
	}
	return int(w), int(h), nil
}

func makePixelAsset(pixels *image.NRGBA, alphas []byte) (*Asset, error) {
	bitmap, err := gomath.NewBitmap(pixels)
	if err != nil {
		return nil, err
	}
	return &Asset{bitmap: bitmap, straight: pixels, originalAlpha: alphas}, nil
}

func (a *Asset) valid() error {
	if a == nil || a.bitmap == nil || a.straight == nil || len(a.originalAlpha) == 0 {
		return fmt.Errorf("pixel asset: nil or uninitialized asset")
	}
	return nil
}

// Bitmap returns the immutable premultiplied image consumed by existing raster,
// PNG and SVG paths. A nil or zero Asset returns nil.
func (a *Asset) Bitmap() *gomath.Bitmap {
	if a == nil {
		return nil
	}
	return a.bitmap
}

func (a *Asset) edit(fn func([]byte, int)) (*Asset, error) {
	if err := a.valid(); err != nil {
		return nil, err
	}
	p := &image.NRGBA{Pix: append([]byte(nil), a.straight.Pix...), Stride: a.straight.Stride, Rect: a.straight.Rect}
	for i := 0; i < len(p.Pix); i += 4 {
		fn(p.Pix[i:i+4], i/4)
	}
	return makePixelAsset(p, a.originalAlpha)
}

// Invert replaces each straight RGB byte c with 255-c and preserves alpha.
// It never subtracts premultiplied channels from 255 or from alpha. Hidden RGB
// already discarded at import cannot be recovered.
func (a *Asset) Invert() (*Asset, error) {
	return a.edit(func(p []byte, _ int) {
		for c := range 3 {
			p[c] = 255 - p[c]
		}
	})
}

func pixelFactor(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 && v <= 1 }

// SetOpacity sets alpha to trunc(originalAlpha*factor), using the alpha saved
// by NewAsset, not the previous edit's alpha. Thus SetOpacity(0) followed by
// SetOpacity(1) restores the original alpha and straight colors. Object/group
// opacity and FadeIn/FadeOut remain separate multiplicative scene effects.
func (a *Asset) SetOpacity(factor float64) (*Asset, error) {
	if !pixelFactor(factor) {
		return nil, fmt.Errorf("pixel opacity must be finite and in [0,1]")
	}
	return a.edit(func(p []byte, i int) { p[3] = uint8(float64(a.originalAlpha[i]) * factor) })
}

func pixelColor(c gomath.Color) bool {
	return pixelFactor(c.R) && pixelFactor(c.G) && pixelFactor(c.B) && pixelFactor(c.A)
}

// SetColor replaces RGB with trunc(255*color.R/G/B), matching Manim's
// set_color rather than multiplying existing RGB. Color.A is validated but
// ignored. With no alpha argument current per-pixel alpha is preserved; one
// optional alpha replaces every alpha with trunc(255*alpha). Saved original
// alpha is unchanged, so a later SetOpacity still uses the import-time mask.
func (a *Asset) SetColor(c gomath.Color, alpha ...float64) (*Asset, error) {
	if !pixelColor(c) || len(alpha) > 1 || (len(alpha) == 1 && !pixelFactor(alpha[0])) {
		return nil, fmt.Errorf("pixel color channels and optional alpha must be finite and in [0,1]; at most one alpha")
	}
	rgb := [3]byte{uint8(255 * c.R), uint8(255 * c.G), uint8(255 * c.B)}
	return a.edit(func(p []byte, _ int) {
		copy(p[:3], rgb[:])
		if len(alpha) == 1 {
			p[3] = uint8(255 * alpha[0])
		}
	})
}

// Tint multiplies each current straight RGB byte by color.R/G/B and truncates
// to uint8. This explicit sRGB-byte operation is an extension, not Manim's
// set_color and not linear-light blending. Color.A is validated but ignored;
// both current and saved original alpha are preserved.
func (a *Asset) Tint(c gomath.Color) (*Asset, error) {
	if !pixelColor(c) {
		return nil, fmt.Errorf("pixel tint channels must be finite and in [0,1]")
	}
	factors := [3]float64{c.R, c.G, c.B}
	return a.edit(func(p []byte, _ int) {
		for i, factor := range factors {
			p[i] = uint8(float64(p[i]) * factor)
		}
	})
}

// NewImage creates a new image plane using the asset's original aspect ratio.
// For an already positioned image use WithPixels to preserve its geometry.
func (a *Asset) NewImage(worldWidth float64) (*gomath.Object, error) {
	if err := a.valid(); err != nil {
		return nil, err
	}
	return gomath.NewImage(a.bitmap, worldWidth)
}

// WithPixels returns an independent image object with new identity and this
// asset's pixels. It preserves the source's exact affine path, clip, metadata
// and style (including opacity). It requires a valid leaf image with matching
// pixel dimensions. The source object and all input assets remain unchanged.
// Pixel assets can differ in fades; Transform requires the same Bitmap pointer
// on both ends, so use a geometry-only target copy or separate FadeOut/FadeIn.
func (a *Asset) WithPixels(source *gomath.Object) (*gomath.Object, error) {
	if err := a.valid(); err != nil {
		return nil, err
	}
	if source == nil || source.Bitmap == nil || len(source.Children) != 0 {
		return nil, fmt.Errorf("pixel replacement requires a leaf image object")
	}
	if err := source.Validate(); err != nil {
		return nil, fmt.Errorf("pixel replacement: %w", err)
	}
	if source.Bitmap.Bounds().Size() != a.bitmap.Bounds().Size() {
		return nil, fmt.Errorf("pixel replacement requires matching pixel dimensions")
	}
	copy := source.Copy()
	copy.Bitmap = a.bitmap
	return copy, nil
}
