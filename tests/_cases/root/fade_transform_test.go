package gomath_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"gomath"
	"gomath/geom"
	"gomath/rasterimage"
	"gomath/render"
	"image"
	"image/color"
	"math"
	"os"
	"reflect"
	"testing"
)

func TestFadeTransformDifferentAssetsAndVectorTransition(t *testing.T) {
	makeImage := func(c color.RGBA) *gomath.Object {
		pixels := image.NewRGBA(image.Rect(0, 0, 2, 2))
		for y := range 2 {
			for x := range 2 {
				pixels.SetRGBA(x, y, c)
			}
		}
		o, err := rasterimage.New(pixels, 2)
		must(t, err)
		return o
	}
	for _, vector := range []bool{false, true} {
		s := gomath.NewScene()
		a := makeImage(color.RGBA{R: 255, A: 255})
		b := makeImage(color.RGBA{B: 255, A: 255})
		if vector {
			b = gomath.Square(2).SetFill(gomath.RGB(0, 0, 1)).SetStroke(gomath.Transparent, 0)
		}
		original := a.Copy().Path
		must(t, s.Add(a))
		must(t, s.Play(1, gomath.FadeTransform(a, b).WithRate(gomath.Linear)))
		cfg := render.DefaultConfig()
		cfg.Width, cfg.Height, cfg.FrameWidth = 64, 64, 4
		cfg.Background = gomath.Black
		r, err := render.NewRasterizer(cfg)
		must(t, err)
		img, err := r.Render(sample(t, s, .5))
		must(t, err)
		p := img.RGBAAt(32, 32)
		// Two independently composited half-opacity layers: blue over red.
		if math.Abs(float64(p.R)-64) > 2 || math.Abs(float64(p.B)-128) > 2 || p.G != 0 {
			t.Fatal("crossfade did not composite both independent assets", vector, p)
		}
		if sample(t, s, 1).Find(a.ID) != nil || sample(t, s, 1).Find(b.ID) == nil {
			t.Fatal("crossfade endpoint topology is wrong")
		}
		if !reflect.DeepEqual(a.Path, original) || a.Style.Opacity != 1 {
			t.Fatal("removed source was not restored")
		}
		must(t, s.Play(.5, gomath.Shift(b, geom.V(1, 0))))
		near(t, b.Center().X, 1)
	}
}

func TestFadeTransformAtomicFailureAndSuccession(t *testing.T) {
	s := gomath.NewScene()
	a, b := gomath.Rectangle(4, 2), gomath.Rectangle(2, 4).Shift(geom.V(3, 0))
	must(t, s.Add(a))
	before := sample(t, s, 0)
	for _, animation := range []gomath.Animation{
		gomath.FadeTransform(nil, b), gomath.FadeTransform(a, nil),
		gomath.FadeTransformWithOptions(a, b, gomath.FadeTransformOptions{Dimension: 3}),
		gomath.FadeTransform(a, a),
	} {
		if s.Play(1, animation) == nil {
			t.Fatal("invalid crossfade accepted")
		}
		if s.Duration() != 0 || !reflect.DeepEqual(before, sample(t, s, 0)) {
			t.Fatal("failed crossfade committed")
		}
	}
	must(t, s.Play(2, gomath.Succession(gomath.FadeTransform(a, b), gomath.Shift(b, geom.V(1, 0)))))
	near(t, b.Center().X, 4)
	if sample(t, s, 1).Find(a.ID) != nil {
		t.Fatal("sequence crossfade cleanup missed boundary")
	}
	near(t, a.Center().X, 0)
}

func TestPinnedManimFadeTransformLayers(t *testing.T) {
	data, err := os.ReadFile("tests/data/root/manim-fade-transform-0.22.1.json")
	must(t, err)
	var fixture struct {
		Source struct {
			Version, Commit string
			Hash            string `json:"generator_sha256"`
		}
		Cases []struct {
			Kind   string
			Alpha  float64
			Layers []struct {
				Points  [][]float64
				Opacity float64
			}
		}
	}
	must(t, json.Unmarshal(data, &fixture))
	generator, err := os.ReadFile("tests/reference/fade_transform.py")
	must(t, err)
	hash := sha256.Sum256(generator)
	if fixture.Source.Version != "0.22.1" || fixture.Source.Commit != "16ea443e2a5f1d79db0a721161411ce916d6e733" || fixture.Source.Hash != hex.EncodeToString(hash[:]) || len(fixture.Cases) != 33 {
		t.Fatal("fade transform provenance/count differs")
	}
	for _, c := range fixture.Cases {
		s := gomath.NewScene()
		a, b := gomath.Rectangle(4, 2).SetFill(gomath.White).Shift(geom.V(-2, 0)), gomath.Rectangle(2, 4).SetFill(gomath.White).Shift(geom.V(2, 1))
		must(t, s.Add(a))
		opt := gomath.DefaultFadeTransformOptions()
		opt.Stretch = c.Kind == "stretch"
		if c.Kind == "width" {
			opt.Dimension = 0
		}
		must(t, s.Play(1, gomath.FadeTransformWithOptions(a, b, opt).WithRate(gomath.Linear)))
		// At the exact endpoint the removed source has been restored. Compare
		// its ghost geometry via a nextafter sample before cleanup instead.
		time := c.Alpha
		if time == 1 {
			time = math.Nextafter(1, 0)
		}
		f := sample(t, s, time)
		for layer, id := range []uint64{a.ID, b.ID} {
			o := f.Find(id)
			if o == nil {
				t.Fatal("missing crossfade layer")
			}
			var points []geom.Vec3
			for _, curve := range o.Path.Contours[0].Curves {
				points = append(points, curve.P0, curve.P1, curve.P2, curve.P3)
			}
			if len(points) != len(c.Layers[layer].Points) {
				t.Fatal("crossfade reference topology differs")
			}
			for i, p := range points {
				for axis, v := range []float64{p.X, p.Y, p.Z} {
					if math.Abs(v-c.Layers[layer].Points[i][axis]) > 1e-10 {
						t.Fatalf("%s alpha%g layer%d point%d differs", c.Kind, c.Alpha, layer, i)
					}
				}
			}
			near(t, o.Style.Opacity, c.Layers[layer].Opacity)
		}
	}
}
