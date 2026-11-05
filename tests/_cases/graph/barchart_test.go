package graph_test

import (
	"gomath"
	"gomath/geom"
	"gomath/graph"
	"gomath/rasterimage"
	"image"
	"image/color"
	"math"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func newTestBars(t *testing.T) *graph.BarChart {
	t.Helper()
	options := graph.DefaultBarChartOptions()
	options.Names = []string{"A", "B", "C", "D"}
	chart, err := graph.NewBarChart([]float64{-2, 0, 1, 4}, graph.Range{Min: -3, Max: 5, Step: 1}, 8, 4, options)
	if err != nil {
		t.Fatal(err)
	}
	return chart
}

func TestBarChartSignedValuesOwnershipAndAtomicUpdate(t *testing.T) {
	chart := newTestBars(t)
	if len(chart.Bars()) != 4 || len(chart.CategoryLabels()) != 4 || chart.BarsObject() == nil {
		t.Fatal("chart selections")
	}
	bars := chart.Bars()
	ids := []uint64{bars[0].ID, bars[1].ID, bars[2].ID, bars[3].ID}
	for i, value := range []float64{-2, 0, 1, 4} {
		p := bars[i].Path.Contours[0]
		base, _ := chart.Point(float64(i)+.5-.3, 0)
		top, _ := chart.Point(float64(i)+.5+.3, value)
		if len(p.Curves) != 4 || !p.Closed || p.Start.Sub(base).Norm() > 1e-13 || p.Curves[1].P3.Sub(top).Norm() > 1e-13 {
			t.Fatal("signed rectangle", i, p)
		}
	}
	before := bars[0].Path.Clone()
	for _, values := range [][]float64{{1}, {1, 2, 3, 6}, {1, 2, math.NaN(), 4}, {1, 2, 3, math.Inf(1)}} {
		if target, err := chart.TargetBars(values); target != nil || err == nil {
			t.Fatal("invalid target", target, err)
		}
		if err := chart.SetValues(values); err == nil || !reflect.DeepEqual(before, bars[0].Path) {
			t.Fatal("failed update changed source", err)
		}
	}
	target, err := chart.TargetBars([]float64{3, 2, -1, 0})
	if err != nil {
		t.Fatal(err)
	}
	if target.Children[0].ID == bars[0].ID || !reflect.DeepEqual(before, bars[0].Path) {
		t.Fatal("target retained author identity or changed source")
	}
	target.Children[0].Path.Contours[0].Curves[0].P0.X = 100
	if !reflect.DeepEqual(before, bars[0].Path) {
		t.Fatal("target aliases source")
	}
	if err = chart.SetValues([]float64{3, 2, -1, 0}); err != nil {
		t.Fatal(err)
	}
	for i, bar := range chart.Bars() {
		if bar != bars[i] || bar.ID != ids[i] {
			t.Fatal("update replaced borrowed bar")
		}
	}
	bars[0] = nil
	labels := chart.CategoryLabels()
	labels[0] = nil
	if chart.Bars()[0] == nil || chart.CategoryLabels()[0] == nil {
		t.Fatal("selection slice aliases chart")
	}
}

func TestBarChartAffineFrameAndValueAnimation(t *testing.T) {
	chart := newTestBars(t)
	initial, _ := chart.Point(1.5, 2)
	transform := geom.Translation(geom.V(1.2, -.7)).Mul(geom.RotationZ(.45)).Mul(geom.Mat4{1, .3, 0, 0, 0, -.8, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1})
	chart.Object.ApplyMatrix(transform)
	point, err := chart.Point(1.5, 2)
	if err != nil || point.Sub(transform.Apply(initial)).Norm() > 2e-12 {
		t.Fatal("frame did not follow whole-chart transform", point, err)
	}
	target, err := chart.TargetBars([]float64{2, 0, -1, -2})
	if err != nil {
		t.Fatal(err)
	}
	scene := gomath.NewScene()
	if err = scene.Add(chart.Object); err != nil {
		t.Fatal(err)
	}
	if err = scene.Play(1, gomath.Transform(chart.BarsObject(), target).WithRate(gomath.Linear)); err != nil {
		t.Fatal(err)
	}
	for _, time := range []float64{0, .125, .25, .5, .75, 1} {
		frame, err := scene.Sample(time)
		if err != nil {
			t.Fatal(err)
		}
		bar := frame.Find(chart.Bars()[0].ID)
		base, _ := chart.Point(.2, 0)
		top, _ := chart.Point(.2, -2+4*time)
		p := bar.Path.Contours[0]
		if p.Start.Sub(base).Norm() > 2e-12 || p.Curves[2].P3.Sub(top).Norm() > 2e-12 || len(p.Curves) != 4 {
			t.Fatal("bar lost baseline or signed value during morph", time)
		}
	}
	if err = chart.SetValues([]float64{0, 0, 0, 0}); err != nil {
		t.Fatal(err)
	}
	for _, bar := range chart.Bars() {
		if len(bar.Path.Contours[0].Curves) != 4 {
			t.Fatal("zero bar topology")
		}
	}
	// Subsequent authoring must not change a previously frozen animation.
	frame, err := scene.Sample(.25)
	if err != nil {
		t.Fatal(err)
	}
	top, _ := chart.Point(.2, -1)
	if frame.Find(chart.Bars()[0].ID).Path.Contours[0].Curves[2].P3.Sub(top).Norm() > 2e-12 {
		t.Fatal("authoring update leaked into snapshot")
	}
}

func TestBarChartRejectsLimitsTopologyAndCollapsedFrames(t *testing.T) {
	for _, mutate := range []func(*graph.BarChartOptions){
		func(o *graph.BarChartOptions) { o.BarWidth = 0 },
		func(o *graph.BarChartOptions) { o.BarWidth = 1.01 },
		func(o *graph.BarChartOptions) { o.Colors = nil },
		func(o *graph.BarChartOptions) { o.Colors = []gomath.Color{{R: math.NaN(), A: 1}} },
		func(o *graph.BarChartOptions) { o.FillOpacity = 2 },
		func(o *graph.BarChartOptions) { o.StrokeWidth = -1 },
		func(o *graph.BarChartOptions) { o.Names = []string{"a", "b"} },
		func(o *graph.BarChartOptions) { o.Names = []string{strings.Repeat("a", 16385)} },
		func(o *graph.BarChartOptions) { o.Names = []string{" "} },
		func(o *graph.BarChartOptions) { o.Names = []string{"a"}; o.Labels.Gap = -1 },
	} {
		opt := graph.DefaultBarChartOptions()
		mutate(&opt)
		if chart, err := graph.NewBarChart([]float64{0}, graph.Range{Min: -1, Max: 1, Step: 1}, 4, 3, opt); chart != nil || err == nil {
			t.Fatal("invalid options", chart, err)
		}
	}
	opt := graph.DefaultBarChartOptions()
	for _, values := range [][]float64{nil, make([]float64, graph.MaxBars+1), {2}} {
		if chart, err := graph.NewBarChart(values, graph.Range{Min: -1, Max: 1, Step: 1}, 4, 3, opt); chart != nil || err == nil {
			t.Fatal("invalid values/count", chart, err)
		}
	}
	chart := newTestBars(t)
	chart.Object.Scale(0)
	if _, err := chart.Point(0, 0); err == nil {
		t.Fatal("collapsed frame accepted")
	}
	chart = newTestBars(t)
	chart.BarsObject().Children[0] = gomath.Circle(1)
	if _, err := chart.TargetBars([]float64{1, 1, 1, 1}); err == nil {
		t.Fatal("replaced bar accepted")
	}
	var absent *graph.BarChart
	if _, err := absent.Point(0, 0); err == nil || absent.BarsObject() != nil || absent.Bars() != nil || absent.CategoryLabels() != nil {
		t.Fatal("nil chart")
	}
	if err := absent.SetValues(nil); err == nil {
		t.Fatal("nil update")
	}
}

func TestBarChartConcurrentTargetsDoNotMutateAuthoring(t *testing.T) {
	chart := newTestBars(t)
	before := chart.Bars()[0].Path.Clone()
	var group sync.WaitGroup
	for range 8 {
		group.Go(func() {
			for range 10 {
				target, err := chart.TargetBars([]float64{1, 2, 3, 4})
				if err != nil || target == nil {
					t.Error("concurrent target", err)
					return
				}
			}
		})
	}
	group.Wait()
	if !reflect.DeepEqual(before, chart.Bars()[0].Path) {
		t.Fatal("concurrent target changed authoring")
	}
}

func TestBarChartSetValuesPreservesChildrenAndBitmap(t *testing.T) {
	chart := newTestBars(t)
	bar := chart.Bars()[0]
	child := gomath.Circle(.1)
	bar.Children = append(bar.Children, child)
	childPath := child.Path.Clone()
	if err := chart.SetValues([]float64{1, 2, 3, 4}); err != nil {
		t.Fatal(err)
	}
	if bar.Children[0] != child || !reflect.DeepEqual(child.Path, childPath) {
		t.Fatal("value update changed child content or identity")
	}
	asset := image.NewRGBA(image.Rect(0, 0, 1, 1))
	asset.SetRGBA(0, 0, color.RGBA{R: 255, A: 255})
	bitmap, err := rasterimage.New(asset, 1)
	if err != nil {
		t.Fatal(err)
	}
	bar.Bitmap = bitmap.Bitmap
	bar.SetFill(gomath.Transparent).SetStroke(gomath.Transparent, 0)
	if err = chart.Object.Validate(); err != nil {
		t.Fatal("source image bar should be valid", err)
	}
	// A collapsed affine bitmap is intentionally valid: renderers treat its
	// zero-area plane as invisible, including during signed value animation.
	if err = chart.SetValues([]float64{0, 2, 3, 4}); err != nil {
		t.Fatal("zero-area bitmap bar", err)
	}
	if err = chart.Object.Validate(); err != nil {
		t.Fatal("updated bitmap bar", err)
	}
	before := bar.Path.Clone()
	// A later invalid value must not commit the already staged first bar.
	if err = chart.SetValues([]float64{1, 2, 3, 6}); err == nil {
		t.Fatal("out-of-range value accepted")
	}
	if !reflect.DeepEqual(before, chart.Bars()[0].Path) {
		t.Fatal("failed visual staging changed source")
	}
}
