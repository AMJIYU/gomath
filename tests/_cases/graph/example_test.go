package graph_test

import (
	"fmt"
	"gomath"
	"gomath/graph"
)

func ExampleNewNumberLine() {
	options := graph.DefaultNumberLineOptions()
	labels := graph.DefaultNumberLabelOptions()
	options.Labels = &labels
	line, err := graph.NewNumberLine(graph.Range{Min: -2, Max: 2, Step: 1}, 8, options)
	if err != nil {
		panic(err)
	}
	scene := gomath.NewScene()
	if err := scene.Add(line.Object); err != nil {
		panic(err)
	}
	fmt.Println(len(line.Ticks()), line.Coordinate(line.Point(1.5)))
	// Output: 5 1.5
}

func ExampleAxes_RiemannRectangles() {
	axes, err := graph.NewAxes(graph.Range{Min: -2, Max: 2, Step: 1}, graph.Range{Min: -3, Max: 3, Step: 1}, 8, 6)
	if err != nil {
		panic(err)
	}
	options := graph.DefaultRiemannOptions()
	options.Rectangles, options.Baseline = 4, -1
	rectangles, err := axes.RiemannRectangles(func(float64) float64 { return -2 }, -1, 1, options)
	if err != nil {
		panic(err)
	}
	scene := gomath.NewScene()
	if err := scene.Add(axes.Object, rectangles.Object); err != nil {
		panic(err)
	}
	fmt.Println(rectangles.SignedArea)
	// Output: -2
}

func ExampleAxes_AreaBetween() {
	axes, err := graph.NewAxes(graph.Range{Min: -2, Max: 2, Step: 1}, graph.Range{Min: -2, Max: 2, Step: 1}, 8, 8)
	if err != nil {
		panic(err)
	}
	options := graph.DefaultAreaOptions()
	options.Samples = 2
	area, err := axes.AreaBetween(func(x float64) float64 { return x }, func(float64) float64 { return 0 }, -1, 1, options)
	if err != nil {
		panic(err)
	}
	fmt.Println(area.Area, area.SignedArea, len(area.Object.Children))
	// Output: 1 0 2
}

func ExampleNewNumberPlane() {
	plane, err := graph.NewNumberPlane(graph.Range{Min: -2, Max: 2, Step: 1}, graph.Range{Min: -1, Max: 1, Step: .5}, 8, 4, graph.DefaultNumberPlaneOptions())
	if err != nil {
		panic(err)
	}
	scene := gomath.NewScene()
	if err := scene.Add(plane.Object); err != nil {
		panic(err)
	}
	x, y := plane.Coordinates(plane.Point(1, .5))
	fmt.Println(len(plane.GridLines()), x, y)
	// Output: 10 1 0.5
}

func ExampleAxes_PlotAdaptive() {
	axes, err := graph.NewAxes(graph.Range{Min: -2, Max: 2, Step: 1}, graph.Range{Min: -2, Max: 2, Step: 1}, 8, 8)
	if err != nil {
		panic(err)
	}
	plot, err := axes.PlotAdaptive(func(x float64) float64 { return x }, -1, 1, graph.DefaultAdaptiveOptions())
	if err != nil {
		panic(err)
	}
	scene := gomath.NewScene()
	if err := scene.Add(axes.Object, plot.Object); err != nil {
		panic(err)
	}
	fmt.Println(plot.Evaluations, plot.NonFiniteSamples, len(plot.Gaps))
	// Output: 17 0 0
}
