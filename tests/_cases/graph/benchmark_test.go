package graph_test

import (
	"gomath/graph"
	"math"
	"testing"
)

func benchmarkAxes(b *testing.B) *graph.Axes {
	b.Helper()
	a, err := graph.NewAxes(graph.Range{Min: -2, Max: 2, Step: 1}, graph.Range{Min: -3, Max: 3, Step: 1}, 8, 6)
	if err != nil {
		b.Fatal(err)
	}
	return a
}

func BenchmarkAdaptivePlot(b *testing.B) {
	a := benchmarkAxes(b)
	f := func(x float64) float64 { return math.Sin(3*x) + .1*math.Sin(17*x) }
	o := graph.DefaultAdaptiveOptions()
	o.Tolerance = .002
	probe, err := a.PlotAdaptive(f, -2, 2, o)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := a.PlotAdaptive(f, -2, 2, o); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportMetric(float64(probe.Evaluations), "evals/op")
}

func BenchmarkRiemannRectangles(b *testing.B) {
	a := benchmarkAxes(b)
	o := graph.DefaultRiemannOptions()
	o.Rectangles = 200
	f := func(x float64) float64 { return x*x - 1 }
	b.ReportAllocs()
	for b.Loop() {
		if _, err := a.RiemannRectangles(f, -2, 2, o); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkAreaBetween(b *testing.B) {
	a := benchmarkAxes(b)
	o := graph.DefaultAreaOptions()
	o.Samples = 201
	f := func(x float64) float64 { return x*x - 1 }
	g := func(float64) float64 { return 0 }
	b.ReportAllocs()
	for b.Loop() {
		if _, err := a.AreaBetween(f, g, -2, 2, o); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkNumberLineLabels(b *testing.B) {
	o := graph.DefaultNumberLineOptions()
	labels := graph.DefaultNumberLabelOptions()
	o.Labels = &labels
	b.ReportAllocs()
	for b.Loop() {
		if _, err := graph.NewNumberLine(graph.Range{Min: -10, Max: 10, Step: .5}, 10, o); err != nil {
			b.Fatal(err)
		}
	}
}
