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
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
)

const parityFixtureDir = "../tests/data/render/clip-evenodd"

type parityGraphic struct {
	Clips []struct {
		D, Rule string
		Matrix  [6]float64
	}
	Color            [4]float64
	Opacity          *float64
	Bitmap, Gradient bool
}

func parityCases(t testing.TB) map[string][]parityGraphic {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(parityFixtureDir, "cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cases map[string][]parityGraphic
	if err = json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	return cases
}
func parityFrame(t testing.TB, entries []parityGraphic) *gomath.Frame {
	t.Helper()
	toWorld := geom.Translation(geom.V(-64, 48)).Mul(geom.Scaling(1, -1, 1))
	frame := &gomath.Frame{}
	for _, entry := range entries {
		opacity := 1.0
		if entry.Opacity != nil {
			opacity = *entry.Opacity
		}
		col := gomath.Color{R: entry.Color[0] / 255, G: entry.Color[1] / 255, B: entry.Color[2] / 255, A: entry.Color[3] / 255}
		o := gomath.Rectangle(128, 96).SetFill(col).SetStroke(gomath.Transparent, 0).SetOpacity(opacity)
		if entry.Bitmap {
			pixels := image.NewNRGBA(image.Rect(0, 0, 16, 16))
			for y := range 16 {
				for x := range 16 {
					pixels.SetNRGBA(x, y, color.NRGBA{R: uint8(entry.Color[0]), G: uint8(entry.Color[1]), B: uint8(entry.Color[2]), A: uint8(entry.Color[3])})
				}
			}
			asset, err := gomath.NewBitmap(pixels)
			if err != nil {
				t.Fatal(err)
			}
			o, err = gomath.NewImage(asset, 128)
			if err != nil {
				t.Fatal(err)
			}
			o.ApplyMatrix(geom.Scaling(1, .75, 1)).SetOpacity(opacity)
		}
		if entry.Gradient {
			gradient, err := gomath.NewLinearGradient(geom.V(-64, 0), geom.V(64, 0), []gomath.GradientStop{{Offset: 0, Color: gomath.Color{R: 1, A: .7}}, {Offset: 1, Color: gomath.Color{B: 1, A: .7}}})
			if err != nil {
				t.Fatal(err)
			}
			o.SetFillGradient(gradient)
		}
		// Clips are all in canonical world coordinates; parents express intersections.
		for i := len(entry.Clips) - 1; i >= 0; i-- {
			spec := entry.Clips[i]
			p, err := svg.ParsePath(spec.D)
			if err != nil {
				t.Fatal(err)
			}
			m := geom.Identity()
			m[0], m[1], m[3], m[4], m[5], m[7] = spec.Matrix[0], spec.Matrix[2], spec.Matrix[4], spec.Matrix[1], spec.Matrix[3], spec.Matrix[5]
			p = p.Transform(toWorld.Mul(m))
			rule := path.NonZero
			if spec.Rule == "evenodd" {
				rule = path.EvenOdd
			}
			c, err := gomath.NewClipWithRule(p, rule)
			if err != nil {
				t.Fatal(err)
			}
			if o.Clip != nil {
				o = gomath.Group(o)
			}
			o.SetClip(c)
		}
		frame.Objects = append(frame.Objects, o)
	}
	return frame
}
func parityConfig() render.Config {
	c := render.DefaultConfig()
	c.Width, c.Height, c.FrameWidth, c.Workers, c.Background = 128, 96, 128, 1, gomath.Transparent
	return c
}
func parityPNG(t *testing.T, name string, scale int) image.Image {
	t.Helper()
	file, err := os.Open(filepath.Join(parityFixtureDir, fmt.Sprintf("%s-cairo-ss%d.png", name, scale)))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	image, err := png.Decode(file)
	if err != nil {
		t.Fatal(err)
	}
	return image
}
func parityDifference(t *testing.T, got, want image.Image) (float64, float64, int) {
	t.Helper()
	if got.Bounds() != want.Bounds() {
		t.Fatal("reference dimensions")
	}
	total, squared, maximum := 0.0, 0.0, 0
	for y := range got.Bounds().Dy() {
		for x := range got.Bounds().Dx() {
			a, b := color.RGBAModel.Convert(got.At(x, y)).(color.RGBA), color.RGBAModel.Convert(want.At(x, y)).(color.RGBA)
			for _, d := range []int{int(a.R) - int(b.R), int(a.G) - int(b.G), int(a.B) - int(b.B), int(a.A) - int(b.A)} {
				total += math.Abs(float64(d))
				squared += float64(d * d)
				maximum = max(maximum, int(math.Abs(float64(d))))
			}
		}
	}
	n := float64(got.Bounds().Dx() * got.Bounds().Dy() * 4)
	return total / n, math.Sqrt(squared / n), maximum
}
func TestEvenOddClipIndependentCairoReferences(t *testing.T) {
	for name, entries := range parityCases(t) {
		t.Run(name, func(t *testing.T) {
			frame := parityFrame(t, entries)
			before := frame.Objects[0].Copy()
			for _, scale := range []int{1, 4} {
				c := parityConfig()
				c.Supersampling = scale
				r, err := render.NewRasterizer(c)
				if err != nil {
					t.Fatal(err)
				}
				got, err := r.Render(frame)
				if err != nil {
					t.Fatal(err)
				}
				mae, rms, maximum := parityDifference(t, got, parityPNG(t, name, scale))
				t.Logf("scale=%d MAE=%.6f RMS=%.6f max=%d", scale, mae, rms, maximum)
				if mae > .4 || rms > 2 || maximum > 32 {
					t.Fatal("independent Cairo clip bounds exceeded")
				}
				if name == "duplicate" {
					for _, v := range got.Pix {
						if v != 0 {
							t.Fatal("duplicate contours must cancel exactly")
						}
					}
				}
				if name == "thin" && got.RGBAAt(20, 40).A != 8 {
					t.Fatal("thin band disappeared", got.RGBAAt(20, 40))
				}
				// The copy made before any rendering still produces the same first graphic.
				first, err := r.Render(&gomath.Frame{Objects: []*gomath.Object{frame.Objects[0]}})
				if err != nil {
					t.Fatal(err)
				}
				copied, err := r.Render(&gomath.Frame{Objects: []*gomath.Object{before}})
				if err != nil || !bytes.Equal(first.Pix, copied.Pix) {
					t.Fatal("clip/paint source ownership changed", err)
				}
			}
		})
	}
}
func TestEvenOddClipReferenceProvenance(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(parityFixtureDir, "provenance.json"))
	if err != nil {
		t.Fatal(err)
	}
	var p struct {
		Generator string
		Hash      string            `json:"generator_sha256"`
		Files     map[string]string `json:"files_sha256"`
	}
	if err = json.Unmarshal(data, &p); err != nil {
		t.Fatal(err)
	}
	for filename, expected := range p.Files {
		data, err := os.ReadFile(filepath.Join(parityFixtureDir, filename))
		if err != nil {
			t.Fatal(err)
		}
		if fmt.Sprintf("%x", sha256.Sum256(data)) != expected {
			t.Fatal("fixture hash mismatch", filename)
		}
	}
	data, err = os.ReadFile("../" + p.Generator)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprintf("%x", sha256.Sum256(data)) != p.Hash {
		t.Fatal("reference generator hash mismatch")
	}
}
func TestClipDefaultNonzeroPixelsRemainExact(t *testing.T) {
	for name, entries := range parityCases(t) {
		original := parityFrame(t, entries)
		explicit := &gomath.Frame{}
		for _, o := range original.Objects {
			cp := o.Copy()
			cp.Walk(func(node *gomath.Object) {
				if node.Clip != nil {
					c, err := gomath.NewClip(node.Clip.Path())
					if err != nil {
						t.Fatal(err)
					}
					node.SetClip(c)
				}
			})
			explicit.Objects = append(explicit.Objects, cp)
		}
		implicit := &gomath.Frame{}
		for _, o := range explicit.Objects {
			cp := o.Copy()
			cp.Walk(func(node *gomath.Object) {
				if node.Clip != nil {
					c, err := gomath.NewClipWithRule(node.Clip.Path(), path.NonZero)
					if err != nil {
						t.Fatal(err)
					}
					node.SetClip(c)
				}
			})
			implicit.Objects = append(implicit.Objects, cp)
		}
		c := parityConfig()
		r, _ := render.NewRasterizer(c)
		a, err := r.Render(explicit)
		if err != nil {
			t.Fatal(err)
		}
		b, err := r.Render(implicit)
		if err != nil || !bytes.Equal(a.Pix, b.Pix) {
			t.Fatal("nonzero default changed", name, err)
		}
	}
}
func BenchmarkEvenOddClip(b *testing.B) {
	for _, name := range []string{"compound", "curve", "crossings", "nested-bitmap"} {
		entries := parityCases(b)[name]
		frame := parityFrame(b, entries)
		b.Run(name, func(b *testing.B) {
			c := parityConfig()
			c.Width, c.Height = 640, 480
			r, err := render.NewRasterizer(c)
			if err != nil {
				b.Fatal(err)
			}
			if _, err = r.Render(frame); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if _, err = r.Render(frame); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func TestEvenOddClipCameraRuleAndPixels(t *testing.T) {
	for name, entries := range parityCases(t) {
		frame := parityFrame(t, entries)
		for _, scale := range []int{1, 4} {
			c := parityConfig()
			c.Supersampling = scale
			r, err := render.NewRasterizer(c)
			if err != nil {
				t.Fatal(err)
			}
			got, err := r.Render(frame)
			if err != nil {
				t.Fatal(err)
			}

			transformed := &gomath.Frame{Camera: &gomath.Camera2D{FrameWidth: 128, Center: geom.V(7, -3), Rotation: .3}}
			for _, o := range frame.Objects {
				transformed.Objects = append(transformed.Objects, o.Copy().ApplyMatrix(geom.Translation(geom.V(7, -3)).Mul(geom.RotationZ(.3))))
			}
			moved, err := r.Render(transformed)
			if err != nil {
				t.Fatal(err)
			}
			for i, v := range moved.Pix {
				if math.Abs(float64(v)-float64(got.Pix[i])) > 1 {
					t.Fatal("camera or transform lost clip rule", name, scale, i)
				}
			}
		}
	}
}
func TestEvenOddClipSupersamplingAnalyticPixelsAndWorkers(t *testing.T) {
	cases := parityCases(t)
	for _, scale := range []int{1, 2, 4, 8} {
		c := parityConfig()
		c.Supersampling = scale
		r, err := render.NewRasterizer(c)
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"thin", "duplicate", "compound", "css-use"} {
			got, err := r.Render(parityFrame(t, cases[name]))
			if err != nil {
				t.Fatal(err)
			}
			switch name {
			case "thin":
				if got.RGBAAt(20, 40).A != 8 {
					t.Fatal("SSAA dropped thin band", scale)
				}
			case "duplicate":
				for _, v := range got.Pix {
					if v != 0 {
						t.Fatal("SSAA duplicate cancellation", scale)
					}
				}
			case "compound":
				if got.RGBAAt(64, 48).A != 0 || got.RGBAAt(20, 20).A != 204 {
					t.Fatal("SSAA hole or alpha", scale)
				}
			case "css-use":
				if got.RGBAAt(28, 44).A != 0 || got.RGBAAt(96, 44).A != 255 {
					t.Fatal("SSAA clip rule contexts", scale)
				}
			}
		}
	}
	frame := parityFrame(t, cases["nested-bitmap"])
	scene := gomath.NewScene()
	if err := scene.Add(frame.Objects...); err != nil {
		t.Fatal(err)
	}
	if err := scene.Wait(.25); err != nil {
		t.Fatal(err)
	}
	c := parityConfig()
	c.FPS, c.Supersampling = 12, 4
	serial := &capture{}
	if _, err := render.RenderScene(context.Background(), scene, c, serial); err != nil {
		t.Fatal(err)
	}
	c.Workers = 3
	parallel := &capture{}
	if _, err := render.RenderScene(context.Background(), scene, c, parallel); err != nil {
		t.Fatal(err)
	}
	if len(serial.pixels) != 3 || len(parallel.pixels) != 3 {
		t.Fatal("frame count")
	}
	for i := range serial.pixels {
		if !bytes.Equal(serial.pixels[i], parallel.pixels[i]) {
			t.Fatal("parallel clip rendering differs", i)
		}
	}
}
