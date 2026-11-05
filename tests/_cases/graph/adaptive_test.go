package graph_test

import (
	"errors"
	"gomath"
	"gomath/geom"
	"gomath/graph"
	"gomath/render"
	"math"
	"testing"
)

func TestAdaptiveQuadraticErrorAndEvaluationCache(t *testing.T) {
	a := integralAxes(t)
	o := graph.DefaultAdaptiveOptions()
	o.InitialSegments, o.Tolerance = 1, .001
	calls := map[float64]int{}
	r, err := a.PlotAdaptive(func(x float64) float64 { calls[x]++; return x * x }, 0, 1, o)
	if err != nil {
		t.Fatal(err)
	}
	if r.Evaluations != len(calls) || r.Evaluations > o.MaxEvaluations || r.NonFiniteSamples != 0 || len(r.Gaps) != 0 {
		t.Fatal("wrong adaptive diagnostics", r)
	}
	for x, count := range calls {
		if count != 1 {
			t.Fatal("repeated evaluation despite shared endpoints", x, count)
		}
	}
	if len(r.Object.Path.Contours) != 1 || len(r.Object.Path.Contours[0].Curves) <= 2 {
		t.Fatal("curved function was not refined")
	}
	for _, curve := range r.Object.Path.Contours[0].Curves {
		p := curve.At(.5)
		x, _ := a.Coordinates(p)
		expected := a.Point(x, x*x)
		if error := math.Abs(p.Y - expected.Y); error > o.Tolerance {
			t.Fatal("quadratic interpolation error exceeds requested midpoint criterion", error)
		}
	}
	o.Tolerance /= 4
	finer, err := a.PlotAdaptive(func(x float64) float64 { return x * x }, 0, 1, o)
	if err != nil || finer.Evaluations <= r.Evaluations {
		t.Fatal("tighter tolerance did not refine", err)
	}
}

func TestAdaptiveNeverBridgesSampledNonFiniteIntervals(t *testing.T) {
	a := integralAxes(t)
	o := graph.DefaultAdaptiveOptions()
	o.MaxDepth, o.InitialSegments = 12, 4
	for _, invalid := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		r, err := a.PlotAdaptive(func(x float64) float64 {
			if math.Abs(x) <= .2 {
				return invalid
			}
			return x
		}, -1, 1, o)
		if err != nil {
			t.Fatal(err)
		}
		if r.NonFiniteSamples == 0 || len(r.Gaps) != 1 || len(r.Object.Path.Contours) != 2 {
			t.Fatal("non-finite interval not exposed/split", r)
		}
		if r.Gaps[0].Min > -.2 || r.Gaps[0].Max < .2 {
			t.Fatal("gap did not conservatively enclose invalid samples", r.Gaps)
		}
		for _, contour := range r.Object.Path.Contours {
			for _, curve := range contour.Curves {
				left, _ := a.Coordinates(curve.P0)
				right, _ := a.Coordinates(curve.P3)
				if left <= .2 && right >= -.2 {
					t.Fatal("curve connects through invalid interval", left, right)
				}
			}
		}
		if err := r.Object.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	r, err := a.PlotAdaptive(func(x float64) float64 {
		if x == 0 {
			return math.Inf(1)
		}
		return x
	}, -1, 1, o)
	if err != nil || len(r.Object.Path.Contours) != 2 || len(r.Gaps) != 1 {
		t.Fatal("isolated infinity was connected", r, err)
	}
	r, err = a.PlotAdaptive(func(float64) float64 { return math.NaN() }, -1, 1, o)
	if err != nil || len(r.Object.Path.Contours) != 0 || len(r.Gaps) != 1 || r.Gaps[0].Min != -1 || r.Gaps[0].Max != 1 || r.Evaluations != 2*o.InitialSegments+1 {
		t.Fatal("fully invalid domain should be an explicit empty plot/gap", r, err)
	}
}

func TestAdaptiveBudgetsAndPrecisionAreExplicitErrors(t *testing.T) {
	a := integralAxes(t)
	o := graph.DefaultAdaptiveOptions()
	o.InitialSegments, o.MaxEvaluations = 1, 2
	calls := 0
	r, err := a.PlotAdaptive(func(x float64) float64 { calls++; return x * x }, 0, 1, o)
	if !errors.Is(err, graph.ErrAdaptiveBudget) || calls != 2 || r != nil {
		t.Fatal("evaluation cap returned an inaccurate partial plot", r, calls, err)
	}
	o = graph.DefaultAdaptiveOptions()
	o.InitialSegments, o.MaxDepth, o.Tolerance = 1, 0, 1e-9
	if r, err := a.PlotAdaptive(func(x float64) float64 { return x * x }, 0, 1, o); !errors.Is(err, graph.ErrAdaptiveDepth) || r != nil {
		t.Fatal("unresolved finite error silently accepted", r, err)
	}
	if _, err := a.PlotAdaptive(func(x float64) float64 { return x }, 1, math.Nextafter(1, 2), o); !errors.Is(err, graph.ErrAdaptivePrecision) {
		t.Fatal("unrepresentable midpoint accepted", err)
	}
	a.Shift(geom.V(1e20, 0))
	if _, err := a.PlotAdaptive(func(x float64) float64 { return x }, 0, 1, o); !errors.Is(err, graph.ErrAdaptivePrecision) {
		t.Fatal("collapsed scene-space plot accepted", err)
	}
}

func TestAdaptiveValidationAndExplicitIntervals(t *testing.T) {
	a := integralAxes(t)
	fn := func(x float64) float64 {
		if x < 0 {
			return x + 1
		}
		return x - 1
	}
	for _, ends := range [][2]float64{{-1, -.01}, {.01, 1}} {
		r, err := a.PlotAdaptive(fn, ends[0], ends[1], graph.DefaultAdaptiveOptions())
		if err != nil || len(r.Gaps) != 0 || len(r.Object.Path.Contours) != 1 {
			t.Fatal("explicitly split finite discontinuity failed", r, err)
		}
	}
	for _, mutate := range []func(*graph.AdaptiveOptions){
		func(o *graph.AdaptiveOptions) { o.Tolerance = math.NaN() },
		func(o *graph.AdaptiveOptions) { o.Tolerance = 0 },
		func(o *graph.AdaptiveOptions) { o.InitialSegments = 0 },
		func(o *graph.AdaptiveOptions) { o.MaxDepth = 25 },
		func(o *graph.AdaptiveOptions) { o.MaxEvaluations = 1_000_001 },
	} {
		o := graph.DefaultAdaptiveOptions()
		mutate(&o)
		if _, err := a.PlotAdaptive(fn, -1, 1, o); err == nil {
			t.Fatal("invalid adaptive options accepted")
		}
	}
	if _, err := a.PlotAdaptive(nil, -1, 1, graph.DefaultAdaptiveOptions()); err == nil {
		t.Fatal("nil adaptive function accepted")
	}
	if _, err := a.PlotAdaptive(fn, 1, -1, graph.DefaultAdaptiveOptions()); err == nil {
		t.Fatal("descending plot interval accepted")
	}
	var missing *graph.Axes
	if _, err := missing.PlotAdaptive(fn, -1, 1, graph.DefaultAdaptiveOptions()); err == nil {
		t.Fatal("nil axes accepted")
	}
	a.Height = math.MaxFloat64
	if _, err := a.PlotAdaptive(func(float64) float64 { return math.MaxFloat64 }, 0, 1, graph.DefaultAdaptiveOptions()); err == nil {
		t.Fatal("scene coordinate overflow accepted")
	}
}

func TestAdaptiveRenderPreservesDomainGap(t *testing.T) {
	a := integralAxes(t)
	r, err := a.PlotAdaptive(func(x float64) float64 {
		if math.Abs(x) <= .3 {
			return math.Inf(1)
		}
		return 0
	}, -1, 1, graph.DefaultAdaptiveOptions())
	if err != nil {
		t.Fatal(err)
	}
	r.Object.SetStroke(gomath.White, .1)
	s := gomath.NewScene()
	if err := s.Add(r.Object); err != nil {
		t.Fatal(err)
	}
	if err := s.Play(1, gomath.Create(r.Object)); err != nil {
		t.Fatal(err)
	}
	c := render.DefaultConfig()
	c.Width, c.Height, c.FrameWidth, c.Background = 160, 120, 8, gomath.Black
	raster, err := render.NewRasterizer(c)
	if err != nil {
		t.Fatal(err)
	}
	for _, time := range []float64{.5, 1} {
		f, err := s.Sample(time)
		if err != nil {
			t.Fatal(err)
		}
		img, err := raster.Render(f)
		if err != nil {
			t.Fatal(err)
		}
		if p := img.RGBAAt(80, 60); p.R != 0 || p.G != 0 || p.B != 0 {
			t.Fatal("Create bridged non-finite interval", time, p)
		}
		if time == 1 && (img.RGBAAt(60, 60).R < 200 || img.RGBAAt(100, 60).R < 200) {
			t.Fatal("valid branches did not render")
		}
	}
}
