package geom

import (
	"math"
	"testing"
)

func TestMatrixComposition(t *testing.T) {
	p := V(2, 0)
	m := Translation(V(1, 2)).Mul(RotationZ(math.Pi / 2))
	q := m.Apply(p)
	if q.Sub(V(1, 4)).Norm() > 1e-10 {
		t.Fatalf("unexpected result %+v", q)
	}
	r := About(V(1, 0), RotationZ(math.Pi)).Apply(V(2, 0))
	if r.Norm() > 1e-10 {
		t.Fatal(r)
	}
	axis := Rotation(Vec3{Z: 1}, math.Pi/2).Apply(V(1, 0))
	if axis.Sub(V(0, 1)).Norm() > 1e-10 {
		t.Fatal(axis)
	}
}
func TestZeroNormalization(t *testing.T) {
	if (Vec3{}).Unit() != (Vec3{}) {
		t.Fatal("zero normalization")
	}
	if math.Abs((Vec3{1, 2, 3}).Unit().Norm()-1) > 1e-12 {
		t.Fatal("unit vector")
	}
}
func BenchmarkMatrixApply(b *testing.B) {
	m := RotationZ(.3)
	p := V(1, 2)
	b.ReportAllocs()
	for b.Loop() {
		p = m.Apply(p)
	}
	if !p.Finite() {
		b.Fatal(p)
	}
}
