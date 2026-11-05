package render

import (
	"bytes"
	"crypto/sha256"
	"gomath"
	"gomath/geom"
	"gomath/path"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func renderGradient(t *testing.T, radial bool, stops []gomath.GradientStop) *gomath.Gradient {
	t.Helper()
	var g *gomath.Gradient
	var e error
	if radial {
		g, e = gomath.NewRadialGradient(geom.V(0, 0), 1, stops)
	} else {
		g, e = gomath.NewLinearGradient(geom.V(-60, 0), geom.V(60, 0), stops)
	}
	if e != nil {
		t.Fatal(e)
	}
	return g
}
func gradientReferenceFrame(t *testing.T, name string) *gomath.Frame {
	t.Helper()
	red, blue := gomath.RGB(1, 0, 0), gomath.RGB(0, 0, 1)
	var o *gomath.Object
	switch name {
	case "linear-hard-hole":
		stops := []gomath.GradientStop{{Offset: 0, Color: red}, {Offset: .4, Color: gomath.RGB(0, 1, 0)}, {Offset: .4, Color: blue}, {Offset: 1, Color: gomath.White}}
		var p path.Builder
		for _, vertices := range [][]geom.Vec3{{geom.V(-72, 52), geom.V(72, 52), geom.V(72, -52), geom.V(-72, -52)}, {geom.V(-20, 20), geom.V(-20, -20), geom.V(20, -20), geom.V(20, 20)}} {
			p.MoveTo(vertices[0])
			for _, v := range vertices[1:] {
				p.LineTo(v)
			}
			p.Close()
		}
		o = gomath.NewObject(p.Path()).SetStroke(gomath.Transparent, 0).SetFillGradient(renderGradient(t, false, stops))
	case "radial-affine-clip":
		g := renderGradient(t, true, []gomath.GradientStop{{Offset: 0, Color: gomath.White}, {Offset: .5, Color: red}, {Offset: 1, Color: blue}})
		m := geom.Identity()
		m[0], m[1], m[4], m[5] = 60, 10, -15, -35
		g, e := g.Transform(m)
		if e != nil {
			t.Fatal(e)
		}
		clip, e := gomath.NewClip(gomath.Rectangle(120, 100).Path)
		if e != nil {
			t.Fatal(e)
		}
		o = gomath.Rectangle(150, 110).SetStroke(gomath.Transparent, 0).SetFillGradient(g).SetClip(clip).SetOpacity(.6)
	case "linear-alpha":
		g := renderGradient(t, false, []gomath.GradientStop{{Offset: 0, Color: gomath.Color{R: 1}}, {Offset: 1, Color: blue}})
		o = gomath.Rectangle(144, 104).SetStroke(gomath.Transparent, 0).SetFillGradient(g)
	default:
		t.Fatal("unknown reference")
	}
	return &gomath.Frame{Objects: []*gomath.Object{o}}
}
func gradientConfig() Config {
	c := DefaultConfig()
	c.Width, c.Height, c.FrameWidth, c.Workers, c.Background = 160, 120, 160, 1, gomath.Transparent
	return c
}
func TestGradientIndependentCairoSVGReferences(t *testing.T) {
	for _, name := range []string{"linear-hard-hole", "radial-affine-clip", "linear-alpha"} {
		t.Run(name, func(t *testing.T) {
			c := gradientConfig()
			r, e := NewRasterizer(c)
			if e != nil {
				t.Fatal(e)
			}
			got, e := r.Render(gradientReferenceFrame(t, name))
			if e != nil {
				t.Fatal(e)
			}
			file, e := os.Open("../tests/data/render/gradients/" + name + ".png")
			if e != nil {
				t.Fatal(e)
			}
			ref, e := png.Decode(file)
			file.Close()
			if e != nil {
				t.Fatal(e)
			}
			if got.Bounds() != ref.Bounds() {
				t.Fatal("reference dimensions")
			}
			total, maximum := 0, 0
			for y := range c.Height {
				for x := range c.Width {
					a, b := got.RGBAAt(x, y), color.RGBAModel.Convert(ref.At(x, y)).(color.RGBA)
					for _, d := range []int{int(a.R) - int(b.R), int(a.G) - int(b.G), int(a.B) - int(b.B), int(a.A) - int(b.A)} {
						v := int(math.Abs(float64(d)))
						total += v
						maximum = max(maximum, v)
					}
				}
			}
			mae := float64(total) / float64(c.Width*c.Height*4)
			t.Logf("independent CairoSVG %s: premultiplied RGBA MAE %.6g, max %d", name, mae, maximum)
			if mae > .55 || maximum > 4 {
				t.Fatal("gradient source/raster differs", mae, maximum)
			}
			if name == "linear-hard-hole" && got.RGBAAt(80, 60).A != 0 {
				t.Fatal("gradient covered nonzero winding hole")
			}
			if preview := os.Getenv("GOMATH_GRADIENT_PREVIEW_DIR"); preview != "" {
				if e := os.MkdirAll(preview, 0755); e != nil {
					t.Fatal(e)
				}
				out, e := os.OpenFile(filepath.Join(preview, name+"-native.png"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
				if e != nil {
					t.Fatal(e)
				}
				e = png.Encode(out, got)
				out.Close()
				if e != nil {
					t.Fatal(e)
				}
			}
		})
	}
}
func TestGradientAnalyticPixelOpacityCoverageAndCamera(t *testing.T) {
	g := renderGradient(t, false, []gomath.GradientStop{{Offset: 0, Color: gomath.Color{R: 1, A: .5}}, {Offset: 1, Color: gomath.Color{B: 1, A: .5}}})
	o := gomath.Circle(45).SetStroke(gomath.Transparent, 0).SetFillGradient(g).SetOpacity(.5)
	frame := &gomath.Frame{Objects: []*gomath.Object{gomath.Group(o).SetOpacity(.5)}}
	c := gradientConfig()
	r, e := NewRasterizer(c)
	if e != nil {
		t.Fatal(e)
	}
	pixels, e := r.Render(frame)
	if e != nil {
		t.Fatal(e)
	}
	if pixels.RGBAAt(5, 5).A != 0 || pixels.RGBAAt(34, 60).A != 0 {
		t.Fatal("gradient paint replaced actual curved coverage")
	}
	p := pixels.RGBAAt(80, 60)
	if p.A != 32 || gradientAbs(int(p.R)-16) > 1 || gradientAbs(int(p.B)-16) > 1 {
		t.Fatal("stop/object/ancestor opacity product", p)
	}
	// Translate and rotate both view and geometry: the affine ellipse/ramp and
	// clip coverage must return to the identical view without mutating input.
	source := gradientReferenceFrame(t, "radial-affine-clip")
	baseline, e := r.Render(source)
	if e != nil {
		t.Fatal(e)
	}
	source.Objects[0].Rotate(.6).Shift(geom.V(7, 9))
	cam := gomath.Camera2D{Center: geom.V(7, 9), Rotation: .6, FrameWidth: 160}
	source.Camera = &cam
	viewed, e := r.Render(source)
	if e != nil {
		t.Fatal(e)
	}
	for i := range baseline.Pix {
		if gradientAbs(int(baseline.Pix[i])-int(viewed.Pix[i])) > 1 {
			t.Fatal("camera/geometry/clip paint coordinates diverged")
		}
	}
	// A no-camera render after a camera render must restore the configuration.
	again, e := r.Render(gradientReferenceFrame(t, "radial-affine-clip"))
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(again.Pix, baseline.Pix) {
		t.Fatal("camera config leaked")
	}
}
func TestGradientRasterBudgetsAndInvalidContent(t *testing.T) {
	r, e := NewRasterizer(gradientConfig())
	if e != nil {
		t.Fatal(e)
	}
	g := renderGradient(t, false, []gomath.GradientStop{{Offset: 0, Color: gomath.Black}, {Offset: 1, Color: gomath.White}})
	for _, over := range []int{0, 1} {
		r.gradientWork = MaxGradientRasterWork - 16 + over
		e := r.gradientPaint(g, image.Rect(0, 0, 4, 4))
		if (e != nil) != (over == 1) {
			t.Fatal("gradient work boundary", e)
		}
	}
	r.gradientWork = 0
	if e := r.gradientPaint(g, image.Rect(0, 0, 4097, 4097)); e == nil {
		t.Fatal("gradient area budget bypass")
	}
	for _, o := range []*gomath.Object{gomath.Square(2).SetFillGradient(&gomath.Gradient{}), gomath.Square(2).SetFillGradient(g).Scale(0)} {
		if pixels, e := r.Render(&gomath.Frame{Objects: []*gomath.Object{o}}); e == nil || pixels != nil {
			t.Fatal("invalid gradient silently rendered")
		}
	}
	c := gradientConfig()
	c.FrameWidth = math.SmallestNonzeroFloat64
	r, e = NewRasterizer(c)
	if e != nil {
		t.Fatal(e)
	}
	if e := r.gradientPaint(g, image.Rect(0, 0, 4, 4)); e == nil {
		t.Fatal("overflowed raster coordinates accepted")
	}
	// Work counters reset for each frame, including after a prior rejection.
	r, e = NewRasterizer(gradientConfig())
	if e != nil {
		t.Fatal(e)
	}
	r.gradientWork = MaxGradientRasterWork
	if _, e := r.Render(gradientReferenceFrame(t, "linear-alpha")); e != nil {
		t.Fatal("gradient work did not reset", e)
	}
}
func TestGradientConcurrentImmutableRender(t *testing.T) {
	frame := gradientReferenceFrame(t, "radial-affine-clip")
	c := gradientConfig()
	r, e := NewRasterizer(c)
	if e != nil {
		t.Fatal(e)
	}
	p, e := r.Render(frame)
	if e != nil {
		t.Fatal(e)
	}
	want := sha256.Sum256(p.Pix)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := NewRasterizer(c)
			if e != nil {
				t.Error(e)
				return
			}
			for range 4 {
				got, e := r.Render(frame)
				if e != nil {
					t.Error(e)
					return
				}
				if sha256.Sum256(got.Pix) != want {
					t.Error("concurrent gradient render changed")
				}
			}
		}()
	}
	wg.Wait()
}
func BenchmarkGradientRaster(b *testing.B) {
	stops := []gomath.GradientStop{{Offset: 0, Color: gomath.White}, {Offset: .4, Color: gomath.Blue}, {Offset: 1, Color: gomath.Color{R: 1, A: .3}}}
	for _, radial := range []bool{false, true} {
		name := "linear"
		g, e := gomath.NewLinearGradient(geom.V(-3, 0), geom.V(3, 0), stops)
		if radial {
			name = "radial"
			g, e = gomath.NewRadialGradient(geom.V(0, 0), 3, stops)
		}
		if e != nil {
			b.Fatal(e)
		}
		b.Run(name, func(b *testing.B) {
			c := DefaultConfig()
			c.Width, c.Height, c.FrameWidth, c.Workers = 640, 360, 8, 1
			o := gomath.Circle(2).SetStroke(gomath.Transparent, 0).SetFillGradient(g)
			r, e := NewRasterizer(c)
			if e != nil {
				b.Fatal(e)
			}
			f := &gomath.Frame{Objects: []*gomath.Object{o}}
			if _, e = r.Render(f); e != nil {
				b.Fatal(e)
			}
			b.ReportAllocs()
			for b.Loop() {
				if _, e := r.Render(f); e != nil {
					b.Fatal(e)
				}
			}
		})
	}
}

func gradientAbs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
