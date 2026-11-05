package path_test

import (
	"gomath/geom"
	"gomath/path"
	"testing"
)

func BenchmarkInverseLookupOptions(b *testing.B) {
	c := path.Cubic{P1: geom.V(12, 25), P2: geom.V(-18, -32), P3: geom.V(2, 1)}
	for _, entry := range []struct {
		name    string
		options path.MeasureOptions
	}{
		{"table-off", path.MeasureOptions{}},
		{"adaptive-table", path.DefaultMeasureOptions()},
	} {
		b.Run(entry.name, func(b *testing.B) {
			m, err := cubicPath(c).MeasureWithOptions(1e-8, entry.options)
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			i := 0
			for b.Loop() {
				if _, err := m.AtFraction(float64(i%997) / 996); err != nil {
					b.Fatal(err)
				}
				i++
			}
		})
	}
}

func BenchmarkMeasureLookupOptions(b *testing.B) {
	var builder path.Builder
	builder.MoveTo(geom.V(0, 0))
	for i := range 32 {
		x := float64(i)
		builder.CubicTo(geom.V(x+.2, 2), geom.V(x+.8, -2), geom.V(x+1, 0))
	}
	p := builder.Path()
	for _, entry := range []struct {
		name    string
		options path.MeasureOptions
	}{
		{"table-off", path.MeasureOptions{}},
		{"adaptive-table", path.DefaultMeasureOptions()},
	} {
		b.Run(entry.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := p.MeasureWithOptions(1e-6, entry.options); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
