package geom

import (
	"encoding/json"
	"math"
	"os"
	"testing"
)

func quatMatrixClose(t *testing.T, got Mat4, want []float64, tolerance float64) {
	t.Helper()
	for i := range 3 {
		for j := range 3 {
			if math.Abs(got[4*i+j]-want[3*i+j]) > tolerance {
				t.Fatalf("matrix[%d,%d] %.17g want %.17g", i, j, got[4*i+j], want[3*i+j])
			}
		}
	}
}

func TestQuaternionSciPy(t *testing.T) {
	var fixture struct {
		Cases []struct {
			Name        string
			A, B        [4]float64
			Vector      [3]float64
			NormalizedA [4]float64
			Product     []float64
			Samples     []struct {
				T       float64
				Matrix  []float64
				Rotated [3]float64
			}
		}
	}
	data, err := os.ReadFile("../tests/data/geom/quaternion_scipy.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Cases) != 20 {
		t.Fatal("missing reference cases")
	}
	for _, c := range fixture.Cases {
		t.Run(c.Name, func(t *testing.T) {
			a, b := Quat{c.A[0], c.A[1], c.A[2], c.A[3]}, Quat{c.B[0], c.B[1], c.B[2], c.B[3]}
			n, err := a.Normalize()
			if err != nil {
				t.Fatal(err)
			}
			for i, v := range []float64{n.W, n.X, n.Y, n.Z} {
				if math.Abs(v-c.NormalizedA[i]) > 3e-15 {
					t.Fatal("normalization", n, c.NormalizedA)
				}
			}
			nb, err := b.Normalize()
			if err != nil {
				t.Fatal(err)
			}
			product, err := n.Mul(nb).Matrix()
			if err != nil {
				t.Fatal(err)
			}
			quatMatrixClose(t, product, c.Product, 4e-14)
			if len(c.Samples) != 65 {
				t.Fatal("missing dense times")
			}
			for _, s := range c.Samples {
				q, err := a.Slerp(b, s.T)
				if err != nil {
					t.Fatal(err)
				}
				m, err := q.Matrix()
				if err != nil {
					t.Fatal(err)
				}
				quatMatrixClose(t, m, s.Matrix, 5e-14)
				v, err := q.Rotate(Vec3{c.Vector[0], c.Vector[1], c.Vector[2]})
				if err != nil {
					t.Fatal(err)
				}
				if v.Sub(Vec3{s.Rotated[0], s.Rotated[1], s.Rotated[2]}).Norm() > 1e-13 {
					t.Fatalf("t=%g rotated=%v want=%v", s.T, v, s.Rotated)
				}
			}
		})
	}
}

func TestQuaternionAlgebraAndConventions(t *testing.T) {
	z, err := QuatFromAxisAngle(Vec3{Z: 1}, math.Pi/2)
	if err != nil {
		t.Fatal(err)
	}
	x, err := QuatFromAxisAngle(Vec3{X: 1}, math.Pi/2)
	if err != nil {
		t.Fatal(err)
	}
	v, err := z.Rotate(Vec3{X: 1})
	if err != nil || v.Sub(Vec3{Y: 1}).Norm() > 1e-14 {
		t.Fatal(v, err)
	}
	// +X sends +Y to +Z, then +Z leaves +Z unchanged. Reverse order differs.
	v, err = z.Mul(x).Rotate(Vec3{Y: 1})
	if err != nil || v.Sub(Vec3{Z: 1}).Norm() > 1e-14 {
		t.Fatal(v, err)
	}
	w, err := x.Mul(z).Rotate(Vec3{Y: 1})
	if err != nil || w.Sub(Vec3{X: -1}).Norm() > 1e-14 {
		t.Fatal(w, err)
	}
	v, err = z.Conjugate().Rotate(v)
	if err != nil {
		t.Fatal(err)
	}
	unit, err := z.Mul(z.Conjugate()).Normalize()
	if err != nil || math.Abs(unit.W-1) > 1e-15 {
		t.Fatal(unit, err)
	}
	for _, axis := range []Vec3{{X: 1e308, Y: -1e308, Z: 1e308}, {X: math.SmallestNonzeroFloat64, Z: math.SmallestNonzeroFloat64}} {
		q, err := QuatFromAxisAngle(axis, .91)
		if err != nil {
			t.Fatal(err)
		}
		m, err := q.Matrix()
		if err != nil {
			t.Fatal(err)
		}
		v := m.Apply(Vec3{X: .3, Y: 1.1, Z: -.8})
		if math.Abs(v.Norm()-math.Sqrt(.09+1.21+.64)) > 1e-14 {
			t.Fatal(v)
		}
	}
	// Norm cancellation/overflow must not turn invalid rotations into identity.
	for _, q := range []Quat{{}, {W: math.NaN()}, {X: math.Inf(1)}, {Z: math.Inf(-1)}} {
		if _, err := q.Normalize(); err == nil {
			t.Fatal("normalize accepted", q)
		}
		if _, err := q.Matrix(); err == nil {
			t.Fatal("matrix accepted", q)
		}
		if _, err := q.Rotate(Vec3{}); err == nil {
			t.Fatal("rotate accepted", q)
		}
		if _, err := q.Slerp(z, .5); err == nil {
			t.Fatal("slerp accepted", q)
		}
		if _, err := z.Slerp(q, .5); err == nil {
			t.Fatal("slerp endpoint accepted", q)
		}
	}
	for _, u := range []float64{-1, 1.001, math.NaN(), math.Inf(1)} {
		if _, err := z.Slerp(x, u); err == nil {
			t.Fatal("parameter accepted", u)
		}
	}
	for _, axis := range []Vec3{{}, {X: math.NaN()}, {Z: math.Inf(1)}} {
		if _, err := QuatFromAxisAngle(axis, 0); err == nil {
			t.Fatal("axis accepted", axis)
		}
	}
	if _, err := QuatFromAxisAngle(Vec3{Z: 1}, math.Inf(1)); err == nil {
		t.Fatal("angle accepted")
	}
	if _, err := z.Rotate(Vec3{X: math.NaN()}); err == nil {
		t.Fatal("nonfinite vector accepted")
	}
	q, _ := QuatFromAxisAngle(Vec3{Z: 1}, math.Pi/4)
	if _, err := q.Rotate(Vec3{X: math.MaxFloat64, Y: -math.MaxFloat64}); err == nil {
		t.Fatal("rotation overflow accepted")
	}
	neg := Quat{-z.W, -z.X, -z.Y, -z.Z}
	end, err := z.Slerp(neg, 1)
	if err != nil || end != neg {
		t.Fatal("endpoint sign lost", end, neg, err)
	}
	if IdentityQuat() != (Quat{W: 1}) {
		t.Fatal("identity")
	}
}

var quaternionSink Quat
var quaternionVectorSink Vec3

func BenchmarkQuaternion(b *testing.B) {
	a, _ := QuatFromAxisAngle(Vec3{X: 1, Y: 2, Z: -3}, .73)
	c, _ := QuatFromAxisAngle(Vec3{X: -2, Y: 1, Z: 3}, 2.1)
	b.Run("Slerp", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			q, err := a.Slerp(c, float64(i%65)/64)
			if err != nil {
				b.Fatal(err)
			}
			quaternionSink = q
		}
	})
	b.Run("Rotate", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			v, err := a.Rotate(Vec3{X: .3, Y: -1.2, Z: 2.4})
			if err != nil {
				b.Fatal(err)
			}
			quaternionVectorSink = v
		}
	})
}
