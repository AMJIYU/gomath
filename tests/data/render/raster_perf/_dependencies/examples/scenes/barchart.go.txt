package scenes

import (
	"gomath"
	"gomath/geom"
	"gomath/graph"
)

func BarChart() (*gomath.Scene, error) {
	options := graph.DefaultBarChartOptions()
	options.Names = []string{"A", "B", "C", "D", "E", "F"}
	chart, err := graph.NewBarChart([]float64{3, -2, 0, 4, -1, 2}, graph.Range{Min: -4, Max: 5, Step: 1}, 10, 4.6, options)
	if err != nil {
		return nil, err
	}
	heading, err := title("Signed bars / stable identities / value animation through zero")
	if err != nil {
		return nil, err
	}
	chart.Object.Shift(geom.V(0, -.15))
	s := gomath.NewScene()
	if err = s.Add(heading, chart.Object); err != nil {
		return nil, err
	}
	if err = s.Wait(.5); err != nil {
		return nil, err
	}
	for _, values := range [][]float64{{-3, 2, 3, -2, 0, 4}, {1, 0, -3, 2, 4, -2}} {
		target, err := chart.TargetBars(values)
		if err != nil {
			return nil, err
		}
		if err = s.Play(1.5, gomath.Transform(chart.BarsObject(), target)); err != nil {
			return nil, err
		}
	}
	if err = s.Wait(.5); err != nil {
		return nil, err
	}
	return s, nil
}
