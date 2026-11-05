package graph_test

import (
	"gomath/geom"
	"gomath/graph"
	"math"
	"testing"
)

func TestCheckedScalesAndLimits(t *testing.T) {
	for _, value := range []float64{-2, -.1, 0, .5, 4} {
		identity := graph.Scale{}
		if v, e := identity.Forward(value); e != nil || v != value {
			t.Fatal(v, e)
		}
		if v, e := identity.Inverse(value); e != nil || v != value {
			t.Fatal(v, e)
		}
	}
	linear, e := graph.NewLinearScale(2.5)
	if e != nil {
		t.Fatal(e)
	}
	logarithmic, e := graph.NewLogScale(10)
	if e != nil {
		t.Fatal(e)
	}
	for _, scale := range []graph.Scale{linear, logarithmic} {
		for _, value := range []float64{-2, -.1, 0, .5, 4} {
			data, e := scale.Forward(value)
			if e != nil {
				t.Fatal(e)
			}
			got, e := scale.Inverse(data)
			if e != nil || math.Abs(got-value) > 1e-14 {
				t.Fatal(value, got, e)
			}
		}
	}
	for _, base := range []float64{-1, 0, .5, 1, math.NaN(), math.Inf(1)} {
		if _, e := graph.NewLogScale(base); e == nil {
			t.Fatal("invalid log base", base)
		}
	}
	for _, factor := range []float64{-1, 0, math.NaN(), math.Inf(1)} {
		if _, e := graph.NewLinearScale(factor); e == nil {
			t.Fatal("invalid factor", factor)
		}
	}
	for _, value := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		if _, e := logarithmic.Inverse(value); e == nil {
			t.Fatal("invalid log input", value)
		}
	}
	for _, scale := range []graph.Scale{{}, linear, logarithmic} {
		for _, value := range []float64{math.Inf(1), math.NaN()} {
			if _, e := scale.Forward(value); e == nil {
				t.Fatal("nonfinite coordinate")
			}
			if _, e := scale.Inverse(value); e == nil {
				t.Fatal("nonfinite value")
			}
		}
	}
	for _, value := range []float64{400, -400} {
		if _, e := logarithmic.Forward(value); e == nil {
			t.Fatal("pow overflow/underflow", value)
		}
	}
	tiny, _ := graph.NewLinearScale(math.SmallestNonzeroFloat64)
	if _, e := tiny.Forward(.01); e == nil {
		t.Fatal("linear underflow")
	}
	if _, e := tiny.Inverse(1); e == nil {
		t.Fatal("linear inverse overflow")
	}
	huge, _ := graph.NewLinearScale(math.MaxFloat64)
	if _, e := huge.Forward(2); e == nil {
		t.Fatal("linear overflow")
	}
	if _, e := huge.Inverse(math.SmallestNonzeroFloat64); e == nil {
		t.Fatal("linear inverse underflow")
	}
}

func TestScaledNumberLineMappingAndTickLabels(t *testing.T) {
	scale, _ := graph.NewLogScale(10)
	opt := graph.DefaultNumberLineOptions()
	opt.Origin = geom.V(1, -2)
	opt.Direction = geom.V(3, 4)
	labels := graph.DefaultNumberLabelOptions()
	count := 0
	labels.Format = func(value float64) string {
		count++
		if value <= 0 {
			t.Fatal("formatter received exponent")
		}
		return "x"
	}
	opt.Labels = &labels
	line, e := graph.NewScaledNumberLine(graph.Range{Min: -2, Max: 2, Step: 1}, 8, scale, opt)
	if e != nil {
		t.Fatal(e)
	}
	if count != 5 || labels.Format == nil {
		t.Fatal("label formatter not called for every tick")
	}
	for i, tick := range line.Ticks() {
		want := math.Pow(10, float64(i-2))
		if tick.Value != want || tick.Tick == nil || tick.Label == nil {
			t.Fatal(tick)
		}
		p, e := line.Point(tick.Value)
		if e != nil {
			t.Fatal(e)
		}
		if p.Sub(tick.Tick.Center()).Norm() > 1e-12 {
			t.Fatal("tick placement")
		}
		v, e := line.Coordinate(p.Add(geom.V(-.8, .6)))
		if e != nil || math.Abs(v/want-1) > 1e-13 {
			t.Fatal("inverse/projection", v, want, e)
		}
	}
	lo, hi := line.DataBounds()
	if lo != .01 || hi != 100 {
		t.Fatal(lo, hi)
	}
	if _, e := line.Point(0); e == nil {
		t.Fatal("zero log point")
	}
	if _, e := line.Coordinate(geom.V(math.Inf(1), 0)); e == nil {
		t.Fatal("nonfinite point")
	}
	ticks := line.Ticks()
	ticks[0].Value = 7
	if line.Ticks()[0].Value != .01 {
		t.Fatal("tick slice leaked")
	}
	opt.Labels = nil
	for _, r := range []graph.Range{{Min: 400, Max: 401, Step: 1}, {Min: -400, Max: -399, Step: 1}, {Min: 1, Max: 0, Step: 1}} {
		if _, e := graph.NewScaledNumberLine(r, 8, scale, opt); e == nil {
			t.Fatal("invalid scaled range accepted")
		}
	}
	var absent *graph.ScaledNumberLine
	if _, e := absent.Point(1); e == nil {
		t.Fatal("nil mapping")
	}
	if _, e := absent.Coordinate(geom.V(0, 0)); e == nil {
		t.Fatal("nil inverse")
	}
	if absent.Ticks() != nil {
		t.Fatal("nil ticks")
	}
	if a, b := absent.DataBounds(); a != 0 || b != 0 {
		t.Fatal("nil bounds")
	}
}

func TestLogAxesPlotSamplesActualData(t *testing.T) {
	scale, _ := graph.NewLogScale(10)
	axes, e := graph.NewScaledAxes(graph.Range{Min: -2, Max: 2, Step: 1}, graph.Range{Min: -4, Max: 4, Step: 2}, 8, 6, scale, scale)
	if e != nil {
		t.Fatal(e)
	}
	for _, x := range []float64{.01, .1, 1, 10, 100} {
		p, e := axes.Point(x, x*x)
		if e != nil {
			t.Fatal(e)
		}
		xx, yy, e := axes.Coordinates(p)
		if e != nil || math.Abs(xx/x-1) > 1e-13 || math.Abs(yy/(x*x)-1) > 1e-13 {
			t.Fatal(xx, yy, e)
		}
		if math.Abs(p.X/8-p.Y/6) > 1e-13 {
			t.Fatal("power law not a straight line", p)
		}
	}
	var inputs []float64
	curve, e := axes.Plot(func(x float64) float64 { inputs = append(inputs, x); return x * x }, 5)
	if e != nil {
		t.Fatal(e)
	}
	if len(inputs) != 5 || len(curve.Path.Contours) != 1 || len(curve.Path.Contours[0].Curves) != 4 {
		t.Fatal("sampling geometry")
	}
	for i, x := range inputs {
		if x != math.Pow(10, float64(i-2)) {
			t.Fatal("function did not receive actual data", inputs)
		}
	}
	if _, e := axes.Point(-1, 1); e == nil {
		t.Fatal("negative x accepted")
	}
	if _, e := axes.Point(1, 0); e == nil {
		t.Fatal("zero y accepted")
	}
	if object, e := axes.Plot(func(float64) float64 { return -1 }, 5); e == nil || object != nil {
		t.Fatal("finite invalid log output dropped")
	}
	gap, e := axes.Plot(func(x float64) float64 {
		if x == 1 {
			return math.NaN()
		}
		return x
	}, 5)
	if e != nil || len(gap.Path.Contours) != 2 {
		t.Fatal("nonfinite gap", e)
	}
	for _, n := range []int{0, 1, 1000001} {
		if _, e := axes.Plot(func(x float64) float64 { return x }, n); e == nil {
			t.Fatal("sample budget")
		}
	}
	if _, e := axes.Plot(nil, 5); e == nil {
		t.Fatal("nil function")
	}
	if _, _, e := axes.Coordinates(geom.V(math.NaN(), 0)); e == nil {
		t.Fatal("nonfinite coordinates")
	}
	var absent *graph.ScaledAxes
	if _, e := absent.Point(1, 1); e == nil {
		t.Fatal("nil axes")
	}
	if _, _, e := absent.Coordinates(geom.V(0, 0)); e == nil {
		t.Fatal("nil inverse")
	}
	if _, e := absent.Plot(func(x float64) float64 { return x }, 5); e == nil {
		t.Fatal("nil plot")
	}
}
