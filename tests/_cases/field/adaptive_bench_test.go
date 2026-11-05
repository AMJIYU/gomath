package field_test

import (
	"context"
	"gomath/field"
	"gomath/geom"
	"testing"
)

func BenchmarkAdaptiveRK45(b *testing.B) {
	fn := func(p geom.Vec3) geom.Vec3 { return geom.V(-.2*p.X-2*p.Y, 2*p.X-.2*p.Y) }
	o := field.DefaultAdaptiveOptions()
	o.EndTime = 6
	o.MinSpeed = 0
	b.Run("Numerical_damped_rotation", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			r, e := field.IntegrateAdaptive(context.Background(), fn, geom.V(1, 0), o)
			if e != nil {
				b.Fatal(e)
			}
			if r.Reason() != field.TimeComplete {
				b.Fatal(r.Reason())
			}
			s := r.Stats()
			b.ReportMetric(float64(s.AcceptedSteps), "accepted/op")
			b.ReportMetric(float64(s.RejectedSteps), "rejected/op")
			b.ReportMetric(float64(s.Evaluations), "evaluations/op")
		}
	})
	b.Run("Geometry_25_damped_rotations", func(b *testing.B) {
		seeds := make([]geom.Vec3, 25)
		for i := range seeds {
			seeds[i] = geom.V(.1+.05*float64(i), 0)
		}
		so := field.DefaultAdaptiveStreamOptions()
		so.Integration = o
		b.ReportAllocs()
		for b.Loop() {
			s, e := field.NewAdaptiveStreamLines(context.Background(), fn, seeds, so)
			if e != nil {
				b.Fatal(e)
			}
			for _, line := range s.Lines() {
				if line.Result.Reason() != field.TimeComplete {
					b.Fatal(line.Result.Reason())
				}
			}
			stats := s.Stats()
			b.ReportMetric(float64(stats.AcceptedSteps), "accepted/op")
			b.ReportMetric(float64(stats.RejectedSteps), "rejected/op")
			b.ReportMetric(float64(stats.Evaluations), "evaluations/op")
		}
	})
}
