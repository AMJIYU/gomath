package field_test

import (
	"gomath/field"
	"testing"
)

func BenchmarkStreamlineFlow(b *testing.B) {
	lines := make([]field.FlowLine, 128)
	for i := range lines {
		times := make([]float64, 257)
		for j := range times {
			times[j] = float64(j*j) / 65536
		}
		lines[i] = flowInput(times...)
	}
	o := field.DefaultFlowOptions()
	o.TraceTimeWidth = .125
	o.Loop = true
	b.Run("build_128x256", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, e := field.NewFlow(lines, o); e != nil {
				b.Fatal(e)
			}
		}
	})
	f, e := field.NewFlow(lines, o)
	if e != nil {
		b.Fatal(e)
	}
	b.Run("sample_128x256_window0125", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, e := f.Sample(.73); e != nil {
				b.Fatal(e)
			}
		}
	})
}
