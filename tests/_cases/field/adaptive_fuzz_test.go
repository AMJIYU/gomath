package field_test

import (
	"context"
	"gomath/field"
	"gomath/geom"
	"math"
	"math/big"
	"testing"
)

// Analytic constant-velocity oracle over reciprocal time/velocity scales spans
// more than 1700 binary exponents. Rational arithmetic computes exact expected
// positions at accepted times independently of any RK tableau or controller.
func FuzzAdaptiveConstant(f *testing.F) {
	f.Add([]byte{1, 2, 3, 4, 5})
	f.Add([]byte{127, 255, 0, 250, 1})
	f.Add([]byte{128, 3, 17, 23, 9})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) < 5 {
			return
		}
		e := 7 * int(int8(data[0]))
		duration := math.Ldexp(1, -e)
		if data[1]&1 != 0 {
			duration = -duration
		}
		velocity := geom.V(math.Ldexp(float64(int(data[2]%9)-4)/4, e), math.Ldexp(float64(int(data[3]%9)-4)/4, e))
		if velocity == (geom.Vec3{}) {
			velocity.X = math.Ldexp(.25, e)
		}
		seed := geom.V(float64(data[4]%9)/4, float64(data[1]%9)/4)
		o := adaptiveOptions()
		o.EndTime = duration
		o.InitialStep = math.Abs(duration) / 8
		o.MinStep = math.Abs(duration) / 1024
		o.MaxStep = math.Abs(duration) / 2
		o.AbsoluteTolerance = 1e-100
		o.RelativeTolerance = 0
		o.Limits.MaxAcceptedSteps = 16
		o.Limits.MaxRejectedSteps = 0
		o.Limits.MaxEvaluations = 128
		r, err := field.IntegrateAdaptive(context.Background(), func(geom.Vec3) geom.Vec3 { return velocity }, seed, o)
		if err != nil {
			t.Fatal(err)
		}
		checkAdaptive(t, r, seed, o)
		if r.Reason() != field.TimeComplete {
			t.Fatal("constant flow terminated", r.Reason())
		}
		if r.Stats().RejectedSteps != 0 {
			t.Fatal("constant field embedded error should cancel exactly")
		}
		points, times := r.Points(), r.Times()
		for i, p := range points {
			want := geom.V(0, 0)
			for axis := 0; axis < 2; axis++ {
				s, v := seed.X, velocity.X
				if axis == 1 {
					s, v = seed.Y, velocity.Y
				}
				rational := new(big.Rat).Mul(new(big.Rat).SetFloat64(v), new(big.Rat).SetFloat64(times[i]))
				rational.Add(rational, new(big.Rat).SetFloat64(s))
				value, _ := rational.Float64()
				if axis == 0 {
					want.X = value
				} else {
					want.Y = value
				}
			}
			if p.Sub(want).Norm() > 1e-13 {
				t.Fatalf("rational trajectory mismatch at %g: got %v want %v", times[i], p, want)
			}
		}
	})
}
