package render_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"gomath"
	"gomath/geom"
	"gomath/path"
	"gomath/render"
	"gomath/svg"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
)

const fillFixtureDir = "../tests/data/render/fill-evenodd"

type fillGraphic struct {
	D, Rule, Gradient              string
	Matrix                         [6]float64
	Color, Stroke                  [4]float64
	Opacity, Reveal, Width, Offset float64
	Dash                           []float64
	Clips                          []struct{ D, Rule string }
}

func fillCases(t testing.TB) map[string][]fillGraphic {
	t.Helper()
	raw, e := os.ReadFile(filepath.Join(fillFixtureDir, "cases.json"))
	if e != nil {
		t.Fatal(e)
	}
	var result map[string][]fillGraphic
	if e = json.Unmarshal(raw, &result); e != nil {
		t.Fatal(e)
	}
	return result
}
func fillColor(v [4]float64) gomath.Color {
	return gomath.Color{R: v[0] / 255, G: v[1] / 255, B: v[2] / 255, A: v[3] / 255}
}
func fillFrame(t testing.TB, entries []fillGraphic) *gomath.Frame {
	t.Helper()
	toWorld := geom.Translation(geom.V(-64, 48)).Mul(geom.Scaling(1, -1, 1))
	frame := &gomath.Frame{}
	for _, entry := range entries {
		p, e := svg.ParsePath(entry.D)
		if e != nil {
			t.Fatal(e)
		}
		o := gomath.NewObject(p).SetFill(fillColor(entry.Color)).SetStroke(fillColor(entry.Stroke), entry.Width).SetOpacity(entry.Opacity)
		o.Style.Reveal = entry.Reveal
		if entry.Rule == "evenodd" {
			o.SetFillRule(path.EvenOdd)
		}
		o.Style.StrokeCap = gomath.CapButt
		if len(entry.Dash) > 0 {
			o.Style.Dash, e = gomath.NewDashPattern(entry.Dash, entry.Offset)
			if e != nil {
				t.Fatal(e)
			}
		}
		if entry.Gradient != "" {
			stops := []gomath.GradientStop{{Offset: 0, Color: gomath.Color{R: 1, A: .7}}, {Offset: 1, Color: gomath.Color{B: 1, A: .7}}}
			var g *gomath.Gradient
			if entry.Gradient == "linear" {
				g, e = gomath.NewLinearGradient(geom.V(0, 0), geom.V(128, 0), stops)
			} else {
				g, e = gomath.NewRadialGradient(geom.V(64, 48), 55, stops)
			}
			if e != nil {
				t.Fatal(e)
			}
			o.SetFillGradient(g)
		}
		m := geom.Identity()
		m[0], m[1], m[3], m[4], m[5], m[7] = entry.Matrix[0], entry.Matrix[2], entry.Matrix[4], entry.Matrix[1], entry.Matrix[3], entry.Matrix[5]
		o.ApplyMatrix(toWorld.Mul(m))
		for i := len(entry.Clips) - 1; i >= 0; i-- {
			c := entry.Clips[i]
			p, e := svg.ParsePath(c.D)
			if e != nil {
				t.Fatal(e)
			}
			rule := path.NonZero
			if c.Rule == "evenodd" {
				rule = path.EvenOdd
			}
			clip, e := gomath.NewClipWithRule(p.Transform(toWorld), rule)
			if e != nil {
				t.Fatal(e)
			}
			if o.Clip != nil {
				o = gomath.Group(o)
			}
			o.SetClip(clip)
		}
		frame.Objects = append(frame.Objects, o)
	}
	return frame
}
func fillReference(t *testing.T, name string, scale int) image.Image {
	t.Helper()
	f, e := os.Open(filepath.Join(fillFixtureDir, fmt.Sprintf("%s-cairo-ss%d.png", name, scale)))
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	p, e := png.Decode(f)
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func TestOrdinaryEvenOddIndependentCairoReferences(t *testing.T) {
	for name, entries := range fillCases(t) {
		t.Run(name, func(t *testing.T) {
			frame := fillFrame(t, entries)
			for _, scale := range []int{1, 4} {
				c := parityConfig()
				c.Supersampling = scale
				r, e := render.NewRasterizer(c)
				if e != nil {
					t.Fatal(e)
				}
				got, e := r.Render(frame)
				if e != nil {
					t.Fatal(e)
				}
				mae, rms, mx := parityDifference(t, got, fillReference(t, name, scale))
				t.Logf("ss%d MAE=%.6f RMS=%.6f max=%d", scale, mae, rms, mx)
				if mae > .4 || rms > 2 || mx > 32 {
					t.Fatal("direct Cairo ordinary fill mismatch")
				}
				if name == "duplicate" {
					for _, v := range got.Pix {
						if v != 0 {
							t.Fatal("duplicate fill did not cancel")
						}
					}
				}
				if name == "thin" && got.RGBAAt(20, 40).A != 8 {
					t.Fatal("thin parity fill disappeared", got.RGBAAt(20, 40))
				}
				if name == "css-use" && (got.RGBAAt(28, 44).A != 0 || got.RGBAAt(96, 44).A != 255) {
					t.Fatal("mixed rules conflated")
				}
			}
		})
	}
}
func TestOrdinaryEvenOddCameraSSAAAndOwnership(t *testing.T) {
	for name, entries := range fillCases(t) {
		frame := fillFrame(t, entries)
		for _, scale := range []int{1, 2, 4, 8} {
			c := parityConfig()
			c.Supersampling = scale
			r, e := render.NewRasterizer(c)
			if e != nil {
				t.Fatal(e)
			}
			got, e := r.Render(frame)
			if e != nil {
				t.Fatal(e)
			}
			moved := &gomath.Frame{Camera: &gomath.Camera2D{FrameWidth: 128, Center: geom.V(7, -3), Rotation: .3}}
			for _, o := range frame.Objects {
				moved.Objects = append(moved.Objects, o.Copy().ApplyMatrix(geom.Translation(geom.V(7, -3)).Mul(geom.RotationZ(.3))))
			}
			actual, e := r.Render(moved)
			if e != nil {
				t.Fatal(e)
			}
			for i, v := range actual.Pix {
				if math.Abs(float64(v)-float64(got.Pix[i])) > 1 {
					t.Fatal("camera changed parity/paint", name, scale, i, v, got.Pix[i])
				}
			}
			again, e := r.Render(frame)
			if e != nil || !bytes.Equal(got.Pix, again.Pix) {
				t.Fatal("rasterizer state leaked across camera/SSAA", name, e)
			}
		}
	}
	// Bitmap content ignores vector fill rules; clipping retains its own rule.
	f := parityFrame(t, parityCases(t)["nested-bitmap"])
	r, _ := render.NewRasterizer(parityConfig())
	a, e := r.Render(f)
	if e != nil {
		t.Fatal(e)
	}
	for _, o := range f.Objects {
		o.SetFillRule(path.EvenOdd)
	}
	b, e := r.Render(f)
	if e != nil || !bytes.Equal(a.Pix, b.Pix) {
		t.Fatal("vector rule tinted/clipped bitmap", e)
	}
}
func TestOrdinaryEvenOddWorkersAndValidation(t *testing.T) {
	s := gomath.NewScene()
	for _, o := range fillFrame(t, fillCases(t)["partial"]).Objects {
		if e := s.Add(o); e != nil {
			t.Fatal(e)
		}
	}
	if e := s.Wait(.2); e != nil {
		t.Fatal(e)
	}
	c := parityConfig()
	c.Width, c.Height, c.FPS, c.Supersampling = 127, 95, 10, 4
	var baseline [][]byte
	for _, workers := range []int{1, 3} {
		c.Workers = workers
		sink := &capture{}
		if _, e := render.RenderScene(context.Background(), s, c, sink); e != nil {
			t.Fatal(e)
		}
		if len(sink.pixels) != 2 {
			t.Fatal("ordered frame count")
		}
		if workers == 1 {
			baseline = sink.pixels
		} else {
			for i := range baseline {
				if !bytes.Equal(baseline[i], sink.pixels[i]) {
					t.Fatal("worker parity scratch leaked")
				}
			}
		}
		if len(sink.pixels[0]) != 127*95*4 {
			t.Fatal("odd-size resolve")
		}
	}
	r, _ := render.NewRasterizer(parityConfig())
	for _, rule := range []path.FillRule{2, 255} {
		if img, e := r.Render(&gomath.Frame{Objects: []*gomath.Object{gomath.Square(10).SetFillRule(rule)}}); e == nil || img != nil {
			t.Fatal("invalid fill rule exposed output", e)
		}
	}
}
func TestOrdinaryEvenOddReferenceProvenance(t *testing.T) {
	raw, e := os.ReadFile(filepath.Join(fillFixtureDir, "provenance.json"))
	if e != nil {
		t.Fatal(e)
	}
	var p struct {
		Generator string
		Hash      string            `json:"generator_sha256"`
		Files     map[string]string `json:"files_sha256"`
	}
	if e = json.Unmarshal(raw, &p); e != nil {
		t.Fatal(e)
	}
	for name, want := range p.Files {
		data, e := os.ReadFile(filepath.Join(fillFixtureDir, name))
		if e != nil {
			t.Fatal(e)
		}
		if fmt.Sprintf("%x", sha256.Sum256(data)) != want {
			t.Fatal("fixture hash", name)
		}
	}
	data, e := os.ReadFile("../" + p.Generator)
	if e != nil {
		t.Fatal(e)
	}
	if fmt.Sprintf("%x", sha256.Sum256(data)) != p.Hash {
		t.Fatal("generator hash")
	}
}

func TestOrdinaryEvenOddStrokeAndGradientAlphaSemantics(t *testing.T) {
	p := gomath.Square(4).Path
	p.Contours = append(p.Contours, p.Contours[0])
	c := parityConfig()
	c.Width, c.Height, c.FrameWidth = 32, 32, 8
	for _, dash := range []bool{false, true} {
		odd := gomath.NewObject(p).SetFill(gomath.White).SetStroke(gomath.Red, .5).SetFillRule(path.EvenOdd)
		odd.Style.StrokeCap = gomath.CapButt
		if dash {
			var e error
			odd.Style.Dash, e = gomath.NewDashPattern([]float64{1, .5}, .3)
			if e != nil {
				t.Fatal(e)
			}
		}
		nonzero := odd.Copy().SetFillRule(path.NonZero).SetFill(gomath.Transparent)
		for _, scale := range []int{1, 4} {
			c.Supersampling = scale
			r, _ := render.NewRasterizer(c)
			a, e := r.Render(&gomath.Frame{Objects: []*gomath.Object{odd}})
			if e != nil {
				t.Fatal(e)
			}
			b, e := r.Render(&gomath.Frame{Objects: []*gomath.Object{nonzero}})
			if e != nil || !bytes.Equal(a.Pix, b.Pix) {
				t.Fatal("parity affected nonzero stroke outlines", dash, scale, e)
			}
			if a.RGBAAt(16, 16).A != 0 {
				t.Fatal("duplicate fill remained under stroke")
			}
		}
	}
	c.Width, c.Height, c.FrameWidth, c.Supersampling = 8, 8, 8, 1
	outer := gomath.Square(8).Path
	outer.Contours = append(outer.Contours, gomath.Square(2).Path.Contours...)
	g, e := gomath.NewLinearGradient(geom.V(-4, 0), geom.V(4, 0), []gomath.GradientStop{{Offset: 0, Color: gomath.Color{R: 1, A: .2}}, {Offset: 1, Color: gomath.Color{B: 1, A: .8}}})
	if e != nil {
		t.Fatal(e)
	}
	o := gomath.NewObject(outer).SetFillGradient(g).SetFillRule(path.EvenOdd).SetStroke(gomath.Transparent, 0).SetOpacity(.6)
	r, _ := render.NewRasterizer(c)
	got, e := r.Render(&gomath.Frame{Objects: []*gomath.Object{o}})
	if e != nil {
		t.Fatal(e)
	}
	sample, e := g.ColorAt(geom.V(-2.5, -.5))
	if e != nil {
		t.Fatal(e)
	}
	want := color.RGBAModel.Convert(sample.NRGBA(.6)).(color.RGBA)
	if got.RGBAAt(1, 4) != want || got.RGBAAt(4, 4).A != 0 {
		t.Fatal("gradient alpha/opacity did not use parity coverage", got.RGBAAt(1, 4), want)
	}
}

func TestOrdinaryEvenOddAndNonzeroReuseCoverageScratch(t *testing.T) {
	c := parityConfig()
	c.Supersampling = 4
	cases := fillCases(t)
	odd := fillFrame(t, cases["linear"]).Objects[0]
	nonzero := odd.Copy().SetFillRule(path.NonZero).Shift(geom.V(15, 0)).SetOpacity(.4)
	small := gomath.Circle(5).SetFill(gomath.White).SetStroke(gomath.Transparent, 0).SetFillRule(path.EvenOdd).Shift(geom.V(-40, 20))
	r, _ := render.NewRasterizer(c)
	for _, o := range []*gomath.Object{odd, nonzero, small, nonzero, odd} {
		got, e := r.Render(&gomath.Frame{Objects: []*gomath.Object{o}})
		if e != nil {
			t.Fatal(e)
		}
		fresh, _ := render.NewRasterizer(c)
		want, e := fresh.Render(&gomath.Frame{Objects: []*gomath.Object{o}})
		if e != nil || !bytes.Equal(got.Pix, want.Pix) {
			t.Fatal("shared scratch retained another rule/region", e)
		}
	}
	// At scale 1 independently composited layers check mixed drawables inside
	// one frame as well; SSAA resolves after all compositing, not per layer.
	c.Supersampling = 1
	r, _ = render.NewRasterizer(c)
	objects := []*gomath.Object{odd, nonzero, small}
	got, e := r.Render(&gomath.Frame{Objects: objects})
	if e != nil {
		t.Fatal(e)
	}
	want := image.NewRGBA(got.Bounds())
	for _, o := range objects {
		layer, e := r.Render(&gomath.Frame{Objects: []*gomath.Object{o}})
		if e != nil {
			t.Fatal(e)
		}
		draw.Draw(want, want.Bounds(), layer, image.Point{}, draw.Over)
	}
	_, _, mx := parityDifference(t, got, want)
	if mx > 1 {
		t.Fatal("shared mask changed mixed-rule compositing", mx)
	}
}
func BenchmarkOrdinaryEvenOddFill(b *testing.B) {
	for _, name := range []string{"compound", "partial", "linear"} {
		frame := fillFrame(b, fillCases(b)[name])
		for _, scale := range []int{1, 2, 4} {
			b.Run(fmt.Sprintf("%s/ss%d", name, scale), func(b *testing.B) {
				c := parityConfig()
				c.Width, c.Height, c.Supersampling = 320, 240, scale
				r, e := render.NewRasterizer(c)
				if e != nil {
					b.Fatal(e)
				}
				if _, e = r.Render(frame); e != nil {
					b.Fatal(e)
				}
				b.ReportAllocs()
				b.ResetTimer()
				for range b.N {
					if _, e = r.Render(frame); e != nil {
						b.Fatal(e)
					}
				}
			})
		}
	}
}
