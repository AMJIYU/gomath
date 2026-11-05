package rasterimage

import (
	"gomath"
	"image"
	"image/color"
	"math"
	"reflect"
	"testing"
)

type pixelBoundsProbe struct {
	bounds image.Rectangle
	reads  int
}

func (p *pixelBoundsProbe) Bounds() image.Rectangle { return p.bounds }
func (p *pixelBoundsProbe) ColorModel() color.Model { return color.RGBAModel }
func (p *pixelBoundsProbe) At(_, _ int) color.Color {
	p.reads++
	return nil
}

func TestPixelCoordinateAndResourceBounds(t *testing.T) {
	for _, b := range []image.Rectangle{
		{Min: image.Pt(math.MinInt, 0), Max: image.Pt(math.MaxInt, 1)},
		{Min: image.Pt(0, math.MinInt), Max: image.Pt(1, math.MaxInt)},
		{Max: image.Pt(MaxPixelPixels+1, 1)},
		{Max: image.Pt(1, MaxPixelPixels+1)},
		{Max: image.Pt(4096, 4096)},
		{Max: image.Pt(1, 0)},
		{Min: image.Pt(2, 2), Max: image.Pt(1, 1)},
	} {
		probe := &pixelBoundsProbe{bounds: b}
		got, err := NewAsset(probe)
		if err == nil || got != nil || probe.reads != 0 {
			t.Fatalf("bad bounds %v: result=%v err=%v reads=%d", b, got, err, probe.reads)
		}
	}
	for _, b := range []image.Rectangle{
		{Max: image.Pt(MaxPixelPixels, 1)},
		{Max: image.Pt(1, MaxPixelPixels)},
		{Min: image.Pt(math.MaxInt-2, math.MinInt), Max: image.Pt(math.MaxInt, math.MinInt+3)},
	} {
		w, h, err := pixelDimensions(b)
		if err != nil || w*h > MaxPixelPixels || w <= 0 || h <= 0 {
			t.Fatalf("valid boundary %v rejected: %v", b, err)
		}
	}
	// Coordinates close to machine limits are safe when spans are bounded.
	source := image.NewNRGBA(image.Rectangle{Min: image.Pt(math.MaxInt-1, math.MinInt), Max: image.Pt(math.MaxInt, math.MinInt+1)})
	source.SetNRGBA(math.MaxInt-1, math.MinInt, color.NRGBA{R: 255, A: 255})
	a, err := NewAsset(source)
	if err != nil || a.Bitmap().RGBAAt(0, 0) != (color.RGBA{R: 255, A: 255}) {
		t.Fatal("valid extreme origin rejected", err)
	}
	if MaxPixelPixels*9 > MaxPixelBytes || (MaxPixelPixels+1)*9 <= MaxPixelBytes || MaxPixelPixels > gomath.MaxBitmapPixels {
		t.Fatal("pixel storage budget is inconsistent")
	}
}

type invalidPixelColor struct{}

func (invalidPixelColor) RGBA() (uint32, uint32, uint32, uint32) { return 65536, 0, 0, 10 }

type pixelColorImage struct{ color.Color }

func (pixelColorImage) Bounds() image.Rectangle   { return image.Rect(0, 0, 1, 1) }
func (pixelColorImage) ColorModel() color.Model   { return color.RGBAModel }
func (p pixelColorImage) At(_, _ int) color.Color { return p.Color }

func TestPixelNilAndInvalidColors(t *testing.T) {
	var typedNil *image.NRGBA
	for _, source := range []image.Image{nil, typedNil, pixelColorImage{}, pixelColorImage{invalidPixelColor{}}, pixelColorImage{color.RGBA{R: 200, A: 10}}} {
		if got, err := NewAsset(source); err == nil || got != nil {
			t.Fatal("invalid source accepted", source)
		}
	}
	for _, a := range []*Asset{nil, {}} {
		if a.Bitmap() != nil {
			t.Fatal("empty asset exposed a bitmap")
		}
		for _, operation := range []func() (*Asset, error){
			a.Invert,
			func() (*Asset, error) { return a.SetOpacity(.5) },
			func() (*Asset, error) { return a.SetColor(gomath.Red) },
			func() (*Asset, error) { return a.Tint(gomath.Red) },
		} {
			if got, err := operation(); err == nil || got != nil {
				t.Fatal("nil/zero asset operation accepted")
			}
		}
		if got, err := a.NewImage(1); err == nil || got != nil {
			t.Fatal("nil/zero asset image accepted")
		}
		if got, err := a.WithPixels(nil); err == nil || got != nil {
			t.Fatal("nil/zero asset replacement accepted")
		}
	}
}

func TestPixelFactorsAtomicErrorsAndPrivateOwnership(t *testing.T) {
	source := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	source.SetNRGBA(0, 0, color.NRGBA{R: 9, G: 201, B: 254, A: 1})
	source.SetNRGBA(1, 0, color.NRGBA{R: 91, G: 200, B: 25, A: 128})
	a, err := NewAsset(source)
	if err != nil {
		t.Fatal(err)
	}
	before := append([]byte(nil), a.straight.Pix...)
	alphaBefore := append([]byte(nil), a.originalAlpha...)
	for _, factor := range []float64{-.001, 1.001, math.NaN(), math.Inf(1), math.Inf(-1)} {
		for _, op := range []func() (*Asset, error){
			func() (*Asset, error) { return a.SetOpacity(factor) },
			func() (*Asset, error) { return a.SetColor(gomath.White, factor) },
		} {
			if got, err := op(); err == nil || got != nil {
				t.Fatal("invalid factor accepted", factor)
			}
		}
		for i := range 4 {
			c := gomath.White
			channels := []*float64{&c.R, &c.G, &c.B, &c.A}
			*channels[i] = factor
			if got, err := a.SetColor(c); err == nil || got != nil {
				t.Fatal("invalid replacement color accepted")
			}
			if got, err := a.Tint(c); err == nil || got != nil {
				t.Fatal("invalid tint accepted")
			}
		}
	}
	if got, err := a.SetColor(gomath.White, .1, .2); err == nil || got != nil {
		t.Fatal("multiple alpha values accepted")
	}
	if !reflect.DeepEqual(a.straight.Pix, before) || !reflect.DeepEqual(a.originalAlpha, alphaBefore) || !reflect.DeepEqual(source.Pix, before) {
		t.Fatal("failed operation mutated inputs")
	}
	inverted, err := a.Invert()
	if err != nil {
		t.Fatal(err)
	}
	if &inverted.straight.Pix[0] == &a.straight.Pix[0] || &a.straight.Pix[0] == &source.Pix[0] || &a.originalAlpha[0] == &source.Pix[3] {
		t.Fatal("input or output storage aliases writable pixels")
	}
	if !reflect.DeepEqual(a.straight.Pix, before) || !reflect.DeepEqual(a.originalAlpha, alphaBefore) {
		t.Fatal("successful edit mutated its original")
	}
	// Color.A is deliberately not a second opacity input.
	c := gomath.RGB(.1, .501, 1)
	c.A = 0
	replaced, err := a.SetColor(c)
	if err != nil || replaced.straight.Pix[0] != 25 || replaced.straight.Pix[1] != 127 || replaced.straight.Pix[2] != 255 || replaced.straight.Pix[3] != 1 {
		t.Fatal("color replacement rounded or multiplied channels", err)
	}
	for _, factor := range []float64{0, math.SmallestNonzeroFloat64, 1} {
		if _, err := a.SetOpacity(factor); err != nil {
			t.Fatal("valid opacity boundary rejected", err)
		}
	}
}
