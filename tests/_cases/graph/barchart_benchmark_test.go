package graph_test

import (
	"fmt"
	"gomath/graph"
	"math"
	"testing"
)

func BenchmarkBarChartTargets(b *testing.B) {
	for _, n := range []int{64, 1024} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			values := make([]float64, n)
			for i := range values {
				values[i] = math.Sin(float64(i))
			}
			chart, err := graph.NewBarChart(values, graph.Range{Min: -1, Max: 1, Step: .5}, 10, 5, graph.DefaultBarChartOptions())
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				if _, err := chart.TargetBars(values); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkBarChartSetValues(b *testing.B) {
	values := make([]float64, 1024)
	for i := range values {
		values[i] = math.Sin(float64(i))
	}
	chart, err := graph.NewBarChart(values, graph.Range{Min: -1, Max: 1, Step: .5}, 10, 5, graph.DefaultBarChartOptions())
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := chart.SetValues(values); err != nil {
			b.Fatal(err)
		}
	}
}
