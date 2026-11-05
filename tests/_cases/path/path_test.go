package path_test

import (
	"gomath"
	"gomath/geom"
	"gomath/path"
	"math"
	"testing"
)

func TestSubdivisionPreservesCurve(t *testing.T) {
	c := path.Cubic{P0: geom.V(0, 0), P1: geom.V(1, 3), P2: geom.V(2, -2), P3: geom.V(4, 1)}
	for _, split := range []float64{.01, .25, .5, .99} {
		a, b := c.Split(split)
		for i := range 101 {
			u := float64(i) / 100
			var got geom.Vec3
			if u <= split {
				got = a.At(u / split)
			} else {
				got = b.At((u - split) / (1 - split))
			}
			if got.Sub(c.At(u)).Norm() > 1e-10 {
				t.Fatal("subdivision changed geometry")
			}
		}
	}
}
func TestAlignAndEndpoints(t *testing.T) {
	a, b := gomath.Circle(1).Path, gomath.RegularPolygon(7, 2).Path
	x, y := path.Align(a, b)
	if len(x.Contours[0].Curves) != len(y.Contours[0].Curves) {
		t.Fatal("curve counts differ")
	}
	if len(a.Contours[0].Curves) != 4 {
		t.Fatal("input mutated")
	}
	for _, p := range []path.Path{path.InterpolateAligned(x, y, 0), path.InterpolateAligned(x, y, 1)} {
		if e := p.Validate(); e != nil {
			t.Fatal(e)
		}
	}
	got := path.InterpolateAligned(x, y, 0).Bounds()
	want := a.Bounds()
	if got.Min.Sub(want.Min).Norm() > 1e-8 || got.Max.Sub(want.Max).Norm() > 1e-8 {
		t.Fatal("aligned start changed bounds")
	}
}
func TestExactBounds(t *testing.T) {
	c := path.Cubic{P0: geom.V(0, 0), P1: geom.V(0, 4), P2: geom.V(1, 4), P3: geom.V(1, 0)}
	p := path.Path{Contours: []path.Contour{{Start: c.P0, Curves: []path.Cubic{c}}}}
	b := p.Bounds()
	if math.Abs(b.Max.Y-3) > 1e-12 {
		t.Fatalf("got max Y %g", b.Max.Y)
	}
	for i := range 1001 {
		v := c.At(float64(i) / 1000)
		if v.Y > b.Max.Y+1e-10 || v.X > b.Max.X+1e-10 {
			t.Fatal("bounds exclude curve")
		}
	}
}
func TestFlattenDoubleBackAndPartial(t *testing.T) {
	c := path.Cubic{P0: geom.V(0, 0), P1: geom.V(4, 0), P2: geom.V(-4, 0), P3: geom.V(0, 0)}
	p := path.Path{Contours: []path.Contour{{Start: c.P0, Curves: []path.Cubic{c}}}}
	if len(p.Flatten(.01)[0].Points) <= 2 {
		t.Fatal("collinear double-back was discarded")
	}
	if len(p.Partial(0).Contours) != 0 {
		t.Fatal("zero reveal not empty")
	}
	q := p.Partial(.5)
	if q.Contours[0].Curves[0].P3.Sub(c.At(.5)).Norm() > 1e-12 {
		t.Fatal("partial endpoint incorrect")
	}
}
func FuzzSubdivision(f *testing.F) {
	f.Add(1.0, 2.0, 3.0, 4.0, .3)
	f.Fuzz(func(t *testing.T, a, b, c, d, u float64) {
		for _, n := range []float64{a, b, c, d, u} {
			if math.IsNaN(n) || math.IsInf(n, 0) || math.Abs(n) > 1e6 {
				t.Skip()
			}
		}
		u = math.Max(.001, math.Min(.999, u))
		curve := path.Cubic{P0: geom.V(0, 0), P1: geom.V(a, b), P2: geom.V(c, d), P3: geom.V(1, 0)}
		left, right := curve.Split(u)
		if left.P3.Sub(right.P0).Norm() > 1e-8 || left.P3.Sub(curve.At(u)).Norm() > 1e-8 {
			t.Fatal("split discontinuity")
		}
	})
}
func BenchmarkAlign(b *testing.B) {
	a, c := gomath.Circle(1).Path, gomath.RegularPolygon(12, 1).Path
	b.ReportAllocs()
	for b.Loop() {
		path.Align(a, c)
	}
}
