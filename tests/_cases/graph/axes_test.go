package graph_test

import (
	"gomath/geom"
	"gomath/graph"
	"math"
	"testing"
)

func TestCoordinatesRoundTripAndShift(t *testing.T) {
	a, e := graph.NewAxes(graph.Range{Min: -3, Max: 5, Step: 1}, graph.Range{Min: -2, Max: 2, Step: .5}, 10, 4)
	if e != nil {
		t.Fatal(e)
	}
	a.Shift(geom.V(2, 1))
	for _, v := range []geom.Vec3{geom.V(-2, 1), geom.V(4, -1)} {
		x, y := a.Coordinates(a.Point(v.X, v.Y))
		if math.Abs(x-v.X) > 1e-12 || math.Abs(y-v.Y) > 1e-12 {
			t.Fatal("coordinate round trip")
		}
	}
}
func TestPlotBreaksNonFiniteDomain(t *testing.T) {
	a, _ := graph.NewAxes(graph.Range{Min: -1, Max: 1, Step: .5}, graph.Range{Min: -1, Max: 1, Step: .5}, 4, 4)
	o, e := a.Plot(func(x float64) float64 {
		if x == 0 {
			return math.NaN()
		}
		return x
	}, 5)
	if e != nil {
		t.Fatal(e)
	}
	if len(o.Path.Contours) != 2 {
		t.Fatal("non-finite domain not split")
	}
	if e = o.Path.Validate(); e != nil {
		t.Fatal(e)
	}
}
func TestBadRangesAndSampling(t *testing.T) {
	if _, e := graph.NewAxes(graph.Range{Min: 0, Max: 1, Step: 1e-30}, graph.Range{Min: 0, Max: 1, Step: 1}, 4, 4); e == nil {
		t.Fatal("unbounded ticks accepted")
	}
	if _, e := graph.Parametric(nil, 0, 1, 100); e == nil {
		t.Fatal("nil function accepted")
	}
}
