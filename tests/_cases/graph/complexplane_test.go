package graph_test

import (
	"gomath"
	"gomath/geom"
	"gomath/graph"
	"math"
	"math/cmplx"
	"reflect"
	"sync"
	"testing"
)

func newTestComplexPlane(t *testing.T) *graph.ComplexPlane {
	t.Helper()
	c, err := graph.NewComplexPlane(graph.Range{Min: -3, Max: 5, Step: 1}, graph.Range{Min: -2, Max: 4, Step: 1}, 8, 3, graph.DefaultNumberPlaneOptions())
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestComplexPlaneAffineRoundTripAndProjection(t *testing.T) {
	for _, matrix := range []geom.Mat4{
		geom.Identity(),
		geom.Translation(geom.Vec3{X: 1.2, Y: -.7, Z: .9}).Mul(geom.Rotation(geom.Vec3{X: 1, Y: 2, Z: -1}, .8)),
		{2, .7, 0, 1, .3, -1, 0, -2, .4, .2, 1, .8, 0, 0, 0, 1},
	} {
		c := newTestComplexPlane(t)
		c.Object.ApplyMatrix(matrix)
		u := matrix.Apply(geom.V(1, 0)).Sub(matrix.Apply(geom.V(0, 0)))
		v := matrix.Apply(geom.V(0, 1)).Sub(matrix.Apply(geom.V(0, 0)))
		n := u.Cross(v).Unit()
		for _, z := range []complex128{0, 1, 1i, -2 + 3i, 4.5 - 1.25i, 10 - 7i} {
			p, err := c.Point(z)
			want := matrix.Apply(geom.V(real(z)-1, (imag(z)-1)*.5))
			if err != nil || p.Sub(want).Norm() > 2e-13 {
				t.Fatal("affine forward", z, p, want, err)
			}
			for _, delta := range []float64{0, 4} {
				back, err := c.Number(p.Add(n.Mul(delta)))
				if err != nil || cmplx.Abs(back-z) > 2e-12 {
					t.Fatal("affine inverse/projection", z, delta, back, err)
				}
			}
		}
	}
}

func TestComplexPlaneOwnershipAndSnapshot(t *testing.T) {
	c := newTestComplexPlane(t)
	selection := c.GridLines()
	first, before := selection[0], selection[0].Path.Clone()
	selection[0] = nil
	if c.GridLines()[0] != first {
		t.Fatal("selection aliases stored slice")
	}
	scene := gomath.NewScene()
	if err := scene.Add(c.Object); err != nil {
		t.Fatal(err)
	}
	if err := scene.Wait(1); err != nil {
		t.Fatal(err)
	}
	c.Object.Shift(geom.V(2, 1))
	frame, err := scene.Sample(.5)
	if err != nil || !reflect.DeepEqual(frame.Find(first.ID).Path, before) {
		t.Fatal("authoring changed scene snapshot", err)
	}
	var group sync.WaitGroup
	for range 8 {
		group.Go(func() {
			for i := range 100 {
				z := complex(float64(i)/10, .3)
				p, err := c.Point(z)
				if err != nil {
					t.Error(err)
					return
				}
				back, err := c.Number(p)
				if err != nil || cmplx.Abs(back-z) > 1e-12 {
					t.Error("concurrent inverse", back, err)
					return
				}
			}
		})
	}
	group.Wait()
}

func TestComplexPlanePlotGapsAndFrozenGeometry(t *testing.T) {
	c := newTestComplexPlane(t)
	calls := 0
	curve, err := c.Plot(func(x float64) complex128 {
		calls++
		if x == .5 {
			return complex(math.NaN(), 0)
		}
		return cmplx.Exp(complex(0, 2*math.Pi*x))
	}, 0, 1, 9)
	if err != nil || calls != 9 || len(curve.Path.Contours) != 2 {
		t.Fatal("complex trajectory", calls, err)
	}
	before := curve.Path.Clone()
	c.Object.Shift(geom.V(2, 1))
	if !reflect.DeepEqual(before, curve.Path) {
		t.Fatal("plot should own geometry")
	}
	for _, contour := range curve.Path.Contours {
		if len(contour.Curves) != 3 {
			t.Fatal("bridged nonfinite gap", len(contour.Curves))
		}
	}
	c.Object.ApplyMatrix(geom.Scaling(4, 4, 1))
	if object, err := c.Plot(func(float64) complex128 { return complex(math.MaxFloat64, 0) }, 0, 1, 2); err == nil || object != nil {
		t.Fatal("mapped overflow accepted")
	}
}

func TestComplexPlaneInvalidAndExtremeFrames(t *testing.T) {
	var empty *graph.ComplexPlane
	if _, err := empty.Point(0); err == nil {
		t.Fatal("nil plane")
	}
	if _, err := empty.Number(geom.V(0, 0)); err == nil || empty.GridLines() != nil {
		t.Fatal("nil inverse/selection")
	}
	if _, err := (&graph.ComplexPlane{}).Point(0); err == nil {
		t.Fatal("zero plane")
	}
	c := newTestComplexPlane(t)
	for _, z := range []complex128{complex(math.NaN(), 0), complex(0, math.Inf(1))} {
		if _, err := c.Point(z); err == nil {
			t.Fatal("nonfinite data")
		}
	}
	if _, err := c.Number(geom.V(math.Inf(1), 0)); err == nil {
		t.Fatal("nonfinite point")
	}
	for _, matrix := range []geom.Mat4{
		geom.Scaling(0, 1, 1),
		{1, 1, 0, 0, 0, 1e-14, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1},
	} {
		bad := newTestComplexPlane(t)
		bad.Object.ApplyMatrix(matrix)
		if _, err := bad.Point(0); err == nil {
			t.Fatal("singular/ill-conditioned frame")
		}
	}
	c = newTestComplexPlane(t)
	c.Object.Children = c.Object.Children[:1]
	if _, err := c.Point(0); err == nil {
		t.Fatal("detached frame")
	}
	// Axis scales differ by 600 orders without a condition-angle failure.
	c, err := graph.NewComplexPlane(graph.Range{Min: -1, Max: 1, Step: 1}, graph.Range{Min: -1, Max: 1, Step: 1}, 1e-300, 1e300, graph.DefaultNumberPlaneOptions())
	if err != nil {
		t.Fatal(err)
	}
	p, err := c.Point(.5 - .25i)
	back, inverseErr := c.Number(p)
	if err != nil || inverseErr != nil || cmplx.Abs(back-(.5-.25i)) > 1e-12 {
		t.Fatal("anisotropic frame", p, back, err, inverseErr)
	}
}

func BenchmarkComplexPlaneMapping(b *testing.B) {
	c, err := graph.NewComplexPlane(graph.Range{Min: -3, Max: 5, Step: 1}, graph.Range{Min: -2, Max: 4, Step: 1}, 8, 3, graph.DefaultNumberPlaneOptions())
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		p, err := c.Point(2 + 3i)
		if err != nil {
			b.Fatal(err)
		}
		if _, err = c.Number(p); err != nil {
			b.Fatal(err)
		}
	}
}
