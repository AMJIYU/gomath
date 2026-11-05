package gomath_test

import (
	"encoding/json"
	"gomath"
	"gomath/geom"
	"image"
	"math"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func gradientStops() []gomath.GradientStop {
	return []gomath.GradientStop{{Offset: 0, Color: gomath.RGB(1, 0, 0)}, {Offset: 1, Color: gomath.RGB(0, 0, 1)}}
}
func linearGradient(t *testing.T) *gomath.Gradient {
	t.Helper()
	g, err := gomath.NewLinearGradient(geom.V(-1, 0), geom.V(1, 0), gradientStops())
	must(t, err)
	return g
}
func gradientColorNear(t *testing.T, got, want gomath.Color) {
	t.Helper()
	for _, pair := range [][2]float64{{got.R, want.R}, {got.G, want.G}, {got.B, want.B}, {got.A, want.A}} {
		if math.Abs(pair[0]-pair[1]) > 1e-10 {
			t.Fatalf("color %v want %v", got, want)
		}
	}
}

func TestGradientAnalyticStopsAlphaPadAndOwnership(t *testing.T) {
	stops := gradientStops()
	g, err := gomath.NewLinearGradient(geom.V(-1, 0), geom.V(1, 0), stops)
	must(t, err)
	stops[0].Color = gomath.White
	copy := g.Stops()
	copy[0].Color = gomath.Black
	if g.Stops()[0].Color != gomath.RGB(1, 0, 0) {
		t.Fatal("stop storage aliases caller")
	}
	for _, x := range []float64{-3, -1, -.5, 0, .5, 1, 3} {
		c, err := g.ColorAt(geom.V(x, 9))
		must(t, err)
		a := math.Max(0, math.Min(1, (x+1)/2))
		gradientColorNear(t, c, gomath.Color{R: 1 - a, B: a, A: 1})
	}
	stops = []gomath.GradientStop{{Offset: 0, Color: gomath.Color{R: 1, A: 0}}, {Offset: 1, Color: gomath.Color{B: 1, A: 1}}}
	alpha, err := gomath.NewLinearGradient(geom.V(0, 0), geom.V(1, 0), stops)
	must(t, err)
	c, err := alpha.ColorAt(geom.V(.25, 0))
	must(t, err)
	gradientColorNear(t, c, gomath.Color{R: .75, B: .25, A: .25})
	hard := []gomath.GradientStop{{Offset: 0, Color: gomath.Black}, {Offset: .5, Color: gomath.White}, {Offset: .5, Color: gomath.Blue}, {Offset: 1, Color: gomath.Blue}}
	h, err := gomath.NewLinearGradient(geom.V(0, 0), geom.V(1, 0), hard)
	must(t, err)
	c, err = h.ColorAt(geom.V(.5, 0))
	must(t, err)
	if c != gomath.Blue {
		t.Fatal("duplicate offset must select last stop")
	}
	radial, err := gomath.NewRadialGradient(geom.V(3, 4), 2, gradientStops())
	must(t, err)
	for _, p := range []geom.Vec3{geom.V(4, 4), geom.V(3, 5), geom.V(2, 4), geom.V(3, 3)} {
		c, err = radial.ColorAt(p)
		must(t, err)
		gradientColorNear(t, c, gomath.Color{R: .5, B: .5, A: 1})
	}
	if radial.Kind() != gomath.RadialGradient || g.Kind() != gomath.LinearGradient {
		t.Fatal("wrong gradient kinds")
	}
}
func TestGradientDuplicateEndpointPadBoundary(t *testing.T) {
	stops := []gomath.GradientStop{{Offset: 0, Color: gomath.Red}, {Offset: 0, Color: gomath.Blue}, {Offset: 1, Color: gomath.Green}, {Offset: 1, Color: gomath.White}}
	g, err := gomath.NewLinearGradient(geom.V(0, 0), geom.V(1, 0), stops)
	must(t, err)
	// Independent SVG/Cairo evaluation confirms pad before zero uses the first
	// stop, while exact-zero and exact-one use the last coincident stop.
	for _, tc := range []struct {
		x float64
		c gomath.Color
	}{{-1, gomath.Red}, {0, gomath.Blue}, {1, gomath.White}, {2, gomath.White}} {
		c, e := g.ColorAt(geom.V(tc.x, 0))
		must(t, e)
		if c != tc.c {
			t.Fatal("duplicate endpoint pad", tc.x, c, tc.c)
		}
	}
}
func TestGradientAnimationRejectsExtrapolationAndCarriesCreate(t *testing.T) {
	s := gomath.NewScene()
	o := gomath.Square(2).SetFillGradient(linearGradient(t))
	must(t, s.Add(o))
	rate := func(a float64) float64 {
		if a > .25 && a < .75 {
			return 1.2
		}
		return a
	}
	must(t, s.Play(1, gomath.Shift(o, geom.V(1, 0)).WithRate(rate)))
	if f, e := s.Sample(.5); e == nil || f != nil {
		t.Fatal("gradient extrapolation silently mismatched paint and geometry")
	}
	s = gomath.NewScene()
	o = gomath.Square(2).SetFillGradient(linearGradient(t))
	must(t, s.Play(1, gomath.Create(o)))
	f, e := s.Sample(.5)
	must(t, e)
	if f.Find(o.ID).Style.FillGradient == nil || f.Find(o.ID).Style.Reveal != .5 {
		t.Fatal("Create lost gradient/reveal")
	}
}

func TestGradientAffineObjectCameraAndStyleSemantics(t *testing.T) {
	shear := geom.Identity()
	shear[1] = .7
	transforms := []geom.Mat4{geom.Translation(geom.V(4, 3)), geom.RotationZ(.7), geom.Scaling(-2, .5, 1), shear}
	for _, g := range []*gomath.Gradient{linearGradient(t), func() *gomath.Gradient {
		g, e := gomath.NewRadialGradient(geom.V(0, 0), 2, gradientStops())
		must(t, e)
		return g
	}()} {
		for _, m := range transforms {
			q, err := g.Transform(m)
			must(t, err)
			for _, p := range []geom.Vec3{geom.V(.25, .3), geom.V(-.3, .4)} {
				want, e := g.ColorAt(p)
				must(t, e)
				got, e := q.ColorAt(m.Apply(p))
				must(t, e)
				gradientColorNear(t, got, want)
			}
			o := gomath.Square(2).SetFillGradient(g)
			id := o.ID
			o.ApplyMatrix(m)
			must(t, o.Validate())
			if o.ID != id || o.Style.FillGradient == g {
				t.Fatal("object transform mutates identity/paint")
			}
			if o.Copy().Style.FillGradient != o.Style.FillGradient {
				t.Fatal("copy should share immutable paint")
			}
		}
	}
	g := linearGradient(t)
	o := gomath.Square(2).SetFillGradient(g).Shift(geom.V(4, 3))
	clip, e := gomath.NewClip(o.Path)
	must(t, e)
	o.SetClip(clip)
	camera := gomath.Camera2D{Center: geom.V(4, 3), FrameWidth: 8, Rotation: .7}
	f := &gomath.Frame{Objects: []*gomath.Object{o}, Camera: &camera}
	q, e := f.ProjectCamera()
	must(t, e)
	p := geom.V(4.5, 3)
	projected := geom.V(.5*math.Cos(.7), -.5*math.Sin(.7))
	want, e := o.Style.FillGradient.ColorAt(p)
	must(t, e)
	got, e := q.Objects[0].Style.FillGradient.ColorAt(projected)
	must(t, e)
	gradientColorNear(t, got, want)
	if f.Camera == nil || q.Camera != nil || o.Clip != clip || o.Style.FillGradient.Matrix()[4] != 3 {
		t.Fatal("projection mutated source or double projects")
	}
	o.SetFill(gomath.Green)
	if o.Style.FillGradient != nil || o.Style.Fill != gomath.Green {
		t.Fatal("SetFill must explicitly select solid paint")
	}
	o.SetFillGradient(g).SetColor(gomath.Yellow)
	if o.Style.FillGradient != nil || o.Style.Fill != gomath.Yellow {
		t.Fatal("SetColor must explicitly select solid paint")
	}
	data, e := json.Marshal(gomath.DefaultStyle())
	must(t, e)
	if strings.Contains(string(data), "FillGradient") {
		t.Fatal("nil paint changed old JSON/style fixtures")
	}
}
func TestGradientInvalidParametersAndImageRejection(t *testing.T) {
	for _, stops := range [][]gomath.GradientStop{nil, {{Offset: 0, Color: gomath.White}}, {{Offset: .1, Color: gomath.White}, {Offset: 1, Color: gomath.Black}}, {{Offset: 0, Color: gomath.White}, {Offset: .9, Color: gomath.Black}}, {{Offset: 0, Color: gomath.White}, {Offset: .7, Color: gomath.Black}, {Offset: .5, Color: gomath.White}, {Offset: 1, Color: gomath.Black}}, {{Offset: 0, Color: gomath.Color{R: 2}}, {Offset: 1, Color: gomath.Black}}, {{Offset: 0, Color: gomath.White}, {Offset: math.NaN(), Color: gomath.Black}}, make([]gomath.GradientStop, gomath.MaxGradientStops+1)} {
		if g, e := gomath.NewLinearGradient(geom.V(0, 0), geom.V(1, 0), stops); e == nil || g != nil {
			t.Fatal("bad stops accepted")
		}
	}
	for _, point := range []geom.Vec3{{X: math.NaN()}, {X: math.Inf(1)}, {Z: 1}, {X: 1e9 + 1}} {
		if _, e := gomath.NewLinearGradient(point, geom.V(1, 0), gradientStops()); e == nil {
			t.Fatal("invalid point accepted")
		}
	}
	for _, r := range []float64{0, -1, math.SmallestNonzeroFloat64, math.NaN(), math.Inf(1), 1e9 + 1} {
		if _, e := gomath.NewRadialGradient(geom.V(0, 0), r, gradientStops()); e == nil {
			t.Fatal("invalid radius", r)
		}
	}
	if _, e := gomath.NewLinearGradient(geom.V(0, 0), geom.V(0, 0), gradientStops()); e == nil {
		t.Fatal("collapsed line accepted")
	}
	g := linearGradient(t)
	perspective := geom.Identity()
	perspective[12] = .1
	for _, m := range []geom.Mat4{perspective, geom.Scaling(0, 1, 1), geom.Scaling(1e-14, 1, 1), geom.Translation(geom.Vec3{Z: 1}), geom.Rotation(geom.Vec3{X: 1}, .5), geom.Scaling(math.Inf(1), 1, 1)} {
		if _, e := g.Transform(m); e == nil {
			t.Fatal("invalid gradient transform accepted", m)
		}
		o := gomath.Square(2).SetFillGradient(g).ApplyMatrix(m)
		if o.Validate() == nil {
			t.Fatal("invalid deferred object transform accepted")
		}
	}
	if (&gomath.Gradient{}).Validate() == nil {
		t.Fatal("zero gradient accepted")
	}
	if _, e := g.ColorAt(geom.Vec3{Z: 1}); e == nil {
		t.Fatal("3D evaluation accepted")
	}
	bmp, e := gomath.NewBitmap(image.NewRGBA(image.Rect(0, 0, 2, 2)))
	must(t, e)
	o, e := gomath.NewImage(bmp, 2)
	must(t, e)
	o.SetFillGradient(g)
	if o.Validate() == nil {
		t.Fatal("image gradient tint silently dropped")
	}
}
func TestGradientAnimationCompatibilityAndConcurrentSampling(t *testing.T) {
	s := gomath.NewScene()
	o := gomath.Square(2).SetFillGradient(linearGradient(t))
	must(t, s.Add(o))
	must(t, s.Play(3, gomath.Succession(gomath.Shift(o, geom.V(2, 0)), gomath.Rotate(o, math.Pi), gomath.Scale(o, 2)).WithRate(gomath.Linear)))
	for _, time := range []float64{0, .5, 1, 1.5, 2, 2.5, 3} {
		f, e := s.Sample(time)
		must(t, e)
		object := f.Find(o.ID)
		c, e := object.Style.FillGradient.ColorAt(object.Center())
		must(t, e)
		gradientColorNear(t, c, gomath.Color{R: .5, B: .5, A: 1})
		must(t, object.Validate())
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 8 {
				f, e := s.Sample(1.5)
				if e != nil {
					t.Error(e)
					return
				}
				f.Find(o.ID).Shift(geom.V(10, 10))
			}
		}()
	}
	wg.Wait()
	for _, target := range []*gomath.Object{o.Copy().SetFill(gomath.Red), o.Copy().SetFillGradient(func() *gomath.Gradient {
		g, e := gomath.NewRadialGradient(geom.V(0, 0), 1, gradientStops())
		must(t, e)
		return g
	}()), o.Copy().SetFillGradient(func() *gomath.Gradient {
		stops := gradientStops()
		stops[1].Color = gomath.Green
		g, e := gomath.NewLinearGradient(geom.V(0, 0), geom.V(1, 0), stops)
		must(t, e)
		return g
	}())} {
		before := o.Copy()
		duration := s.Duration()
		if e := s.Play(1, gomath.Transform(o, target)); e == nil {
			t.Fatal("unsupported paint transition accepted")
		}
		if s.Duration() != duration || !reflect.DeepEqual(o.Path, before.Path) || o.Style != before.Style {
			t.Fatal("failed paint animation changed authoring state")
		}
	}
	// Direct matrix morphs cannot pass through a singular paint frame; rotation
	// animations have their own trigonometric path and do support a half turn.
	target := o.Copy().Rotate(math.Pi)
	if e := s.Play(1, gomath.Transform(o, target)); e == nil {
		t.Fatal("singular affine interpolation accepted")
	}
	must(t, s.Play(1, gomath.FadeOut(o)))
}
