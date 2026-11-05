package gomath

import (
	"fmt"
	"gomath/geom"
	"image"
	"image/color"
	"image/draw"
	"math"
)

// MaxBitmapPixels bounds a single decoded asset to 64 megapixels.
const MaxBitmapPixels = 64 * 1024 * 1024

// Bitmap owns immutable premultiplied RGBA pixels. Object snapshots and Copy
// share the immutable asset, while geometry and style remain independent.
// It implements image.Image without exposing a writable pixel slice.
type Bitmap struct{ pixels *image.RGBA }

func NewBitmap(source image.Image) (*Bitmap, error) {
	if source == nil {
		return nil, fmt.Errorf("nil image")
	}
	b := source.Bounds()
	if b.Dx() <= 0 || b.Dy() <= 0 || b.Dx() > MaxBitmapPixels/b.Dy() {
		return nil, fmt.Errorf("bitmap must have 1..%d pixels", MaxBitmapPixels)
	}
	pixels := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(pixels, pixels.Bounds(), source, b.Min, draw.Src)
	return &Bitmap{pixels: pixels}, nil
}

func (b *Bitmap) ColorModel() color.Model { return color.RGBAModel }
func (b *Bitmap) Bounds() image.Rectangle {
	if b == nil || b.pixels == nil {
		return image.Rectangle{}
	}
	return b.pixels.Bounds()
}
func (b *Bitmap) RGBAAt(x, y int) color.RGBA {
	if b == nil || b.pixels == nil {
		return color.RGBA{}
	}
	return b.pixels.RGBAAt(x, y)
}
func (b *Bitmap) At(x, y int) color.Color { return b.RGBAAt(x, y) }

// NewImage places a bitmap at the origin with a positive world width and its
// original aspect ratio. Its four-edge path is the editable affine image plane.
func NewImage(bitmap *Bitmap, width float64) (*Object, error) {
	bounds := bitmap.Bounds()
	if bounds.Empty() || !finite(width) || width <= 0 {
		return nil, fmt.Errorf("image needs a nonempty bitmap and finite positive world width")
	}
	height := width * float64(bounds.Dy()) / float64(bounds.Dx())
	if !finite(height) || height <= 0 {
		return nil, fmt.Errorf("image world height overflows or underflows")
	}
	o := Rectangle(width, height).SetStroke(Transparent, 0)
	o.Bitmap = bitmap
	o.Name = "Image"
	return o, nil
}

// ImageCorners returns the top-left, top-right and bottom-left world points of
// an affine image. It rejects curved paths, perspective quads and painted image
// borders/tints rather than silently dropping unsupported content.
func (o *Object) ImageCorners() (topLeft, topRight, bottomLeft geom.Vec3, err error) {
	bad := func(message string) (geom.Vec3, geom.Vec3, geom.Vec3, error) {
		return geom.Vec3{}, geom.Vec3{}, geom.Vec3{}, fmt.Errorf("image: %s", message)
	}
	if o.Bitmap == nil || o.Bitmap.Bounds().Empty() {
		return bad("missing bitmap")
	}
	if o.Style.FillGradient != nil || o.Style.Fill.A > 0 || (o.Style.Stroke.A > 0 && o.Style.StrokeWidth > 0) || o.Style.Reveal != 1 {
		return bad("fill/tint, stroke and path reveal are unsupported; use opacity, FadeIn or FadeOut")
	}
	if len(o.Path.Contours) != 1 || !o.Path.Contours[0].Closed || len(o.Path.Contours[0].Curves) != 4 {
		return bad("expected a closed four-edge affine plane")
	}
	c := o.Path.Contours[0].Curves
	bottomLeft, topRight, topLeft = c[0].P0, c[1].P3, c[2].P3
	bottomRight := c[0].P3
	size := o.Path.Bounds().Size()
	if !size.Finite() {
		return bad("plane bounds overflow")
	}
	tolerance := 1e-8 * math.Max(1, math.Max(size.X, math.Max(size.Y, size.Z)))
	close := func(a, b geom.Vec3) bool {
		d := a.Sub(b)
		return d.Finite() && math.Abs(d.X) <= tolerance && math.Abs(d.Y) <= tolerance && math.Abs(d.Z) <= tolerance
	}
	if !close(topRight, topLeft.Add(bottomRight.Sub(bottomLeft))) || !close(c[3].P3, bottomLeft) {
		return bad("perspective or nonaffine quad")
	}
	for _, curve := range c {
		if !close(curve.P1, curve.P0.Lerp(curve.P3, 1.0/3)) || !close(curve.P2, curve.P0.Lerp(curve.P3, 2.0/3)) {
			return bad("curved image plane")
		}
	}
	return topLeft, topRight, bottomLeft, nil
}

// ValidateVisual checks one node's renderable content. Validate also checks
// ownership, identities, descendants and cycles.
func (o *Object) ValidateVisual() error {
	if o == nil {
		return fmt.Errorf("nil object")
	}
	if !finite(o.Scalar) {
		return fmt.Errorf("non-finite object scalar")
	}
	if err := o.Style.Validate(); err != nil {
		return err
	}
	if err := o.validateGradientGeometry(); err != nil {
		return err
	}
	if err := o.Path.Validate(); err != nil {
		return err
	}
	if o.Clip != nil {
		if err := o.Clip.Validate(); err != nil {
			return fmt.Errorf("object clip: %w", err)
		}
	}
	if o.Bitmap != nil {
		_, _, _, err := o.ImageCorners()
		return err
	}
	return nil
}
