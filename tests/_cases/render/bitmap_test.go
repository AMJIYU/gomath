package render_test

import (
	"bytes"
	"context"
	"gomath"
	"gomath/geom"
	"gomath/rasterimage"
	"gomath/render"
	"image"
	"image/color"
	"math"
	"testing"
)

func TestBitmapParallelSamplingMatchesSerial(t *testing.T) {
	o := quadrantImage(t)
	s := gomath.NewScene()
	if err := s.Add(o); err != nil {
		t.Fatal(err)
	}
	if err := s.Play(1, gomath.Rotate(o, 2*math.Pi)); err != nil {
		t.Fatal(err)
	}
	c := config()
	c.Workers = 1
	a := &capture{}
	if _, err := render.RenderScene(context.Background(), s, c, a); err != nil {
		t.Fatal(err)
	}
	c.Workers = 3
	b := &capture{}
	if _, err := render.RenderScene(context.Background(), s, c, b); err != nil {
		t.Fatal(err)
	}
	if len(a.pixels) != len(b.pixels) {
		t.Fatal("frame count differs")
	}
	for i := range a.pixels {
		if !bytes.Equal(a.pixels[i], b.pixels[i]) {
			t.Fatalf("bitmap sampling differs at frame %d", i)
		}
	}
}

func quadrantImage(t *testing.T) *gomath.Object {
	t.Helper()
	pixels := image.NewRGBA(image.Rect(0, 0, 4, 4))
	colors := [4]color.RGBA{{R: 255, A: 255}, {G: 255, A: 255}, {B: 255, A: 255}, {R: 255, G: 255, B: 255, A: 255}}
	for y := range 4 {
		for x := range 4 {
			pixels.SetRGBA(x, y, colors[(y/2)*2+x/2])
		}
	}
	o, err := rasterimage.New(pixels, 4)
	if err != nil {
		t.Fatal(err)
	}
	return o
}

func TestBitmapAffineOrientation(t *testing.T) {
	o := quadrantImage(t)
	for _, test := range []struct {
		name   string
		object *gomath.Object
		x, y   int
	}{
		{"original", o, 40, 40},
		{"rotate", o.Copy().Rotate(math.Pi / 2), 40, 88},
		{"reflect", o.Copy().ApplyMatrix(geom.Scaling(-1, 1, 1)), 88, 40},
		{"translate", o.Copy().Shift(geom.V(1, -1)), 56, 56},
	} {
		t.Run(test.name, func(t *testing.T) {
			r, err := render.NewRasterizer(config())
			if err != nil {
				t.Fatal(err)
			}
			img, err := r.Render(&gomath.Frame{Objects: []*gomath.Object{test.object}})
			if err != nil {
				t.Fatal(err)
			}
			p := img.RGBAAt(test.x, test.y)
			if p.R < 250 || p.G > 5 || p.B > 5 {
				t.Fatal("wrong image orientation", p)
			}
		})
	}
}

func TestBitmapAlphaAndAntialiasedBoundary(t *testing.T) {
	pixels := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	pixels.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 128})
	o, err := rasterimage.New(pixels, 3)
	if err != nil {
		t.Fatal(err)
	}
	o.Rotate(math.Pi / 4).Shift(geom.V(.03, .07))
	group := gomath.Group(o).SetOpacity(.5)
	r, _ := render.NewRasterizer(config())
	img, err := r.Render(&gomath.Frame{Objects: []*gomath.Object{group}})
	if err != nil {
		t.Fatal(err)
	}
	if p := img.RGBAAt(64, 64); p.R != 64 || p.G != 0 || p.B != 0 || p.A != 255 {
		t.Fatal("premultiplied composition wrong", p)
	}
	edges := 0
	for y := range 128 {
		for x := range 128 {
			p := img.RGBAAt(x, y)
			if p.R > 64 || p.G != 0 || p.B != 0 {
				t.Fatal("alpha edge overflow", p)
			}
			if p.R > 0 && p.R < 64 {
				edges++
			}
		}
	}
	if edges < 10 {
		t.Fatal("transformed edges lack antialiasing")
	}
}

func TestBitmapBilinearAndValidation(t *testing.T) {
	pixels := image.NewRGBA(image.Rect(0, 0, 2, 1))
	pixels.SetRGBA(0, 0, color.RGBA{R: 255, A: 255})
	pixels.SetRGBA(1, 0, color.RGBA{B: 255, A: 255})
	o, err := rasterimage.New(pixels, 4)
	if err != nil {
		t.Fatal(err)
	}
	r, _ := render.NewRasterizer(config())
	img, err := r.Render(&gomath.Frame{Objects: []*gomath.Object{o}})
	if err != nil {
		t.Fatal(err)
	}
	p := img.RGBAAt(64, 64)
	if p.R < 120 || p.R > 132 || p.B < 124 || p.B > 136 {
		t.Fatal("missing bilinear filtering", p)
	}
	bad := o.Copy().SetFill(gomath.Blue)
	if _, err := r.Render(&gomath.Frame{Objects: []*gomath.Object{bad}}); err == nil {
		t.Fatal("unsupported tint accepted")
	}
	bad = o.Copy()
	bad.Path.Contours[0].Curves[0].P1.Y += 1
	if _, err := r.Render(&gomath.Frame{Objects: []*gomath.Object{bad}}); err == nil {
		t.Fatal("curved plane accepted")
	}
	if _, err := r.Render(&gomath.Frame{Objects: []*gomath.Object{o.Copy().Scale(0)}}); err != nil {
		t.Fatal("collapsed image failed", err)
	}
}
