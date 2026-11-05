package graph_test

import (
	"gomath"
	"gomath/geom"
	"gomath/graph"
	"gomath/render"
	"math"
	"testing"
)

func integralAxes(t *testing.T) *graph.Axes {
	t.Helper()
	a, err := graph.NewAxes(graph.Range{Min: -2, Max: 2, Step: 1}, graph.Range{Min: -3, Max: 3, Step: 1}, 8, 6)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func polygonArea(object *gomath.Object) float64 {
	c := object.Path.Contours[0]
	sum, previous := 0.0, c.Start
	for _, curve := range c.Curves {
		end := curve.P3
		sum += previous.X*end.Y - end.X*previous.Y
		previous = end
	}
	return math.Abs(sum) / 2
}

func TestRiemannSamplingSignedAreasAndNegativeBaseline(t *testing.T) {
	a := integralAxes(t)
	f := func(x float64) float64 { return x }
	for _, tc := range []struct {
		sample graph.RiemannSample
		want   float64
	}{{graph.LeftSample, 1.5}, {graph.MidSample, 2}, {graph.RightSample, 2.5}} {
		o := graph.DefaultRiemannOptions()
		o.Rectangles, o.Sample = 4, tc.sample
		r, err := a.RiemannRectangles(f, 0, 2, o)
		if err != nil || math.Abs(r.SignedArea-tc.want) > 1e-12 {
			t.Fatal("wrong Riemann sum", tc, r, err)
		}
		if len(r.Cells()) != 4 || len(r.Object.Children) != 4 {
			t.Fatal("wrong rectangle topology")
		}
		reverse, err := a.RiemannRectangles(f, 2, 0, o)
		if err != nil || math.Abs(reverse.SignedArea+tc.want) > 1e-12 {
			t.Fatal("reversed integral sign lost", err)
		}
	}
	o := graph.DefaultRiemannOptions()
	o.Rectangles, o.Baseline = 4, -1
	r, err := a.RiemannRectangles(func(float64) float64 { return -2 }, -1, 1, o)
	if err != nil {
		t.Fatal(err)
	}
	if r.SignedArea != -2 {
		t.Fatal("negative baseline integral incorrect", r.SignedArea)
	}
	for _, cell := range r.Cells() {
		if cell.Value != -2 || cell.Baseline != -1 || cell.SignedArea != -.5 || cell.Object.Style.Fill != o.Style.NegativeColor {
			t.Fatal("negative cell metadata/style incorrect", cell)
		}
		b := cell.Object.Bounds()
		nearPoint(t, b.Min, a.Point(cell.Left, -2))
		nearPoint(t, b.Max, a.Point(cell.Right, -1))
		// X scene scale is 2 and Y scene scale is 1.
		if math.Abs(polygonArea(cell.Object)-math.Abs(cell.SignedArea)*2) > 1e-12 {
			t.Fatal("rectangle geometric area disagrees with sum")
		}
	}
	cells := r.Cells()
	cells[0].Value = 500
	if r.Cells()[0].Value == 500 {
		t.Fatal("rectangle records alias")
	}
}

func TestAreaBetweenCrossingsAndOrientedIntegral(t *testing.T) {
	a := integralAxes(t)
	o := graph.DefaultAreaOptions()
	o.Samples = 2
	r, err := a.AreaBetween(func(x float64) float64 { return x }, func(float64) float64 { return 0 }, -1, 1, o)
	if err != nil {
		t.Fatal(err)
	}
	if r.Area != 1 || r.SignedArea != 0 || len(r.Object.Children) != 2 {
		t.Fatal("crossing area cancellation or topology failure", r)
	}
	worldArea := 0.0
	for _, strip := range r.Object.Children {
		if len(strip.Path.Contours[0].Curves) != 3 {
			t.Fatal("crossing was not split into triangles")
		}
		worldArea += polygonArea(strip)
	}
	if math.Abs(worldArea-2*r.Area) > 1e-12 {
		t.Fatal("filled lobes disagree with absolute area")
	}
	if r.Object.Children[0].Style.Fill != o.Style.NegativeColor || r.Object.Children[1].Style.Fill != o.Style.PositiveColor {
		t.Fatal("signed regions use wrong colors")
	}
	// Both functions lie below zero; no implicit zero baseline may replace g.
	r, err = a.AreaBetween(func(x float64) float64 { return x - 2 }, func(float64) float64 { return -2 }, -1, 2, o)
	if err != nil || math.Abs(r.Area-2.5) > 1e-12 || math.Abs(r.SignedArea-1.5) > 1e-12 {
		t.Fatal("negative function baseline or crossing integral failed", r, err)
	}
	reverse, err := a.AreaBetween(func(x float64) float64 { return x - 2 }, func(float64) float64 { return -2 }, 2, -1, o)
	if err != nil || math.Abs(reverse.Area-2.5) > 1e-12 || math.Abs(reverse.SignedArea+1.5) > 1e-12 {
		t.Fatal("area orientation failed", err)
	}
	flat, err := a.AreaBetween(func(x float64) float64 { return x }, func(x float64) float64 { return x }, -1, 1, o)
	if err != nil || flat.Area != 0 || len(flat.Object.Children) != 0 {
		t.Fatal("identical functions should have zero area", err)
	}
}

func TestAreaTrapezoidConvergenceAndFiniteSampling(t *testing.T) {
	a := integralAxes(t)
	f := func(x float64) float64 { return x * x }
	g := func(float64) float64 { return 0 }
	o := graph.DefaultAreaOptions()
	o.Samples = 11
	coarse, err := a.AreaBetween(f, g, 0, 1, o)
	if err != nil {
		t.Fatal(err)
	}
	o.Samples = 21
	fine, err := a.AreaBetween(f, g, 0, 1, o)
	if err != nil {
		t.Fatal(err)
	}
	if ratio := (coarse.Area - 1.0/3) / (fine.Area - 1.0/3); math.Abs(ratio-4) > 1e-9 {
		t.Fatal("trapezoid refinement should reduce error by four", ratio)
	}
	samples := fine.Samples()
	samples[0].First = 500
	if fine.Samples()[0].First == 500 {
		t.Fatal("area samples alias")
	}
	if err := fine.Object.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestIntegralObjectsRejectInvalidAndUnrepresentableInputs(t *testing.T) {
	a := integralAxes(t)
	f := func(float64) float64 { return 1 }
	for _, ends := range [][2]float64{{0, 0}, {0, math.NaN()}, {-1e308, 1e308}} {
		if _, err := a.RiemannRectangles(f, ends[0], ends[1], graph.DefaultRiemannOptions()); err == nil {
			t.Fatal("invalid interval accepted")
		}
		if _, err := a.AreaBetween(f, f, ends[0], ends[1], graph.DefaultAreaOptions()); err == nil {
			t.Fatal("invalid area interval accepted")
		}
	}
	for _, mutate := range []func(*graph.RiemannOptions){
		func(o *graph.RiemannOptions) { o.Rectangles = 0 },
		func(o *graph.RiemannOptions) { o.Rectangles = 100001 },
		func(o *graph.RiemannOptions) { o.Sample = "random" },
		func(o *graph.RiemannOptions) { o.Baseline = math.Inf(1) },
		func(o *graph.RiemannOptions) { o.Style.NegativeColor.A = math.NaN() },
	} {
		o := graph.DefaultRiemannOptions()
		mutate(&o)
		if _, err := a.RiemannRectangles(f, 0, 1, o); err == nil {
			t.Fatal("invalid Riemann options accepted")
		}
	}
	for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		bad := func(float64) float64 { return value }
		if _, err := a.AreaBetween(bad, f, 0, 1, graph.DefaultAreaOptions()); err == nil {
			t.Fatal("non-finite area sample accepted")
		}
		if _, err := a.RiemannRectangles(bad, 0, 1, graph.DefaultRiemannOptions()); err == nil {
			t.Fatal("non-finite rectangle sample accepted")
		}
	}
	if _, err := a.AreaBetween(nil, f, 0, 1, graph.DefaultAreaOptions()); err == nil {
		t.Fatal("nil area function accepted")
	}
	if _, err := a.RiemannRectangles(nil, 0, 1, graph.DefaultRiemannOptions()); err == nil {
		t.Fatal("nil Riemann function accepted")
	}
	o := graph.DefaultRiemannOptions()
	o.Rectangles = 2
	if _, err := a.RiemannRectangles(f, 1, math.Nextafter(1, 2), o); err == nil {
		t.Fatal("collapsed floating-point partitions accepted")
	}
	a.Shift(geom.V(1e20, 0))
	if _, err := a.AreaBetween(f, func(float64) float64 { return 0 }, 0, 1, graph.DefaultAreaOptions()); err == nil {
		t.Fatal("collapsed scene mapping accepted")
	}
}

func TestIntegralObjectsRenderSeparatedPositiveAndNegativeLobes(t *testing.T) {
	a := integralAxes(t)
	o := graph.DefaultAreaOptions()
	o.Samples = 2
	o.Style.Opacity, o.Style.StrokeWidth = 1, 0
	r, err := a.AreaBetween(func(x float64) float64 { return x }, func(float64) float64 { return 0 }, -1, 1, o)
	if err != nil {
		t.Fatal(err)
	}
	s := gomath.NewScene()
	if err := s.Add(r.Object); err != nil {
		t.Fatal(err)
	}
	f, err := s.Sample(0)
	if err != nil {
		t.Fatal(err)
	}
	c := render.DefaultConfig()
	c.Width, c.Height, c.FrameWidth, c.Background = 160, 120, 8, gomath.Black
	raster, err := render.NewRasterizer(c)
	if err != nil {
		t.Fatal(err)
	}
	img, err := raster.Render(f)
	if err != nil {
		t.Fatal(err)
	}
	negative, positive := img.RGBAAt(50, 65), img.RGBAAt(110, 55)
	if negative.R < 200 || negative.R <= negative.B || positive.B < 150 || positive.B <= positive.R {
		t.Fatal("positive or negative filled lobe missing", negative, positive)
	}
	if p := img.RGBAAt(50, 50); p.R != 0 || p.B != 0 {
		t.Fatal("area filled outside the functions", p)
	}
}
