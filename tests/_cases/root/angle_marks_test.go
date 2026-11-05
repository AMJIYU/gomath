package gomath_test

import (
	"gomath"
	"gomath/geom"
	"math"
	"reflect"
	"sync"
	"testing"
)

func TestAngleQuadrantsSweepRadiusAndElbow(t *testing.T) {
	a, b := gomath.Line(geom.V(-2, 0), geom.V(2, 0)), gomath.Line(geom.V(0, -1), geom.V(0, 1))
	for _, q := range [][2]int{{1, 1}, {1, -1}, {-1, 1}, {-1, -1}} {
		for _, clockwise := range []bool{false, true} {
			opts := gomath.DefaultAngleOptions()
			opts.Quadrant, opts.OtherAngle = q, clockwise
			angle, err := gomath.NewAngle(a, b, opts)
			must(t, err)
			want := math.Pi / 2
			if q[0] != q[1] {
				want = 3 * math.Pi / 2
			}
			if clockwise {
				want -= 2 * math.Pi
			}
			near(t, angle.Sweep, want)
			near(t, angle.Value(true), want*180/math.Pi)
			near(t, angle.Radius, .4)
			if angle.Vertex != (geom.Vec3{}) || angle.Parallel || len(angle.Object.Path.Contours[0].Curves) != 8 {
				t.Fatal("angle construction metadata or topology", angle)
			}
			c := angle.Object.Path.Contours[0]
			if c.Start.Sub(geom.V(float64(q[0])*.4, 0)).Norm() > 1e-12 || c.Curves[7].P3.Sub(geom.V(0, float64(q[1])*.4)).Norm() > 1e-12 || c.Closed {
				t.Fatal("angle anchors", q, clockwise)
			}
		}
		opts := gomath.AngleOptions{Radius: .3, Quadrant: q}
		elbow, err := gomath.NewRightAngle(a, b, opts)
		must(t, err)
		c := elbow.Object.Path.Contours[0]
		if len(c.Curves) != 2 || c.Curves[0].P3.Sub(geom.V(float64(q[0])*.3, float64(q[1])*.3)).Norm() > 1e-12 {
			t.Fatal("elbow corner", q)
		}
	}
	for _, distance := range []float64{.1, .59, .6, 2} {
		angle, err := gomath.NewAngleFromPoints(geom.V(distance, 0), geom.V(0, 0), geom.V(0, 2), gomath.AngleOptions{})
		must(t, err)
		want := .4
		if distance < .6 {
			want = (2.0 / 3) * distance
		}
		near(t, angle.Radius, want)
	}
}

func TestAngleDotOwnershipAndParallel(t *testing.T) {
	a := gomath.Line(geom.V(-2, 0), geom.V(2, 0))
	b := gomath.Line(geom.V(0, -2), geom.V(0, 2))
	beforeA, beforeB := a.Copy(), b.Copy()
	opts := gomath.DefaultAngleOptions()
	opts.Dot, opts.Radius = true, 1
	angle, err := gomath.NewAngle(a, b, opts)
	must(t, err)
	if !reflect.DeepEqual(beforeA.Path, a.Path) || !reflect.DeepEqual(beforeB.Path, b.Path) {
		t.Fatal("constructor mutated its lines")
	}
	if len(angle.Object.Children) != 1 {
		t.Fatal("dot missing")
	}
	dot := angle.Object.Children[0]
	near(t, dot.Center().X, .55/math.Sqrt2)
	near(t, dot.Center().Y, .55/math.Sqrt2)
	near(t, dot.Bounds().Size().X, .2)
	if dot.Style.Fill != gomath.White || dot.Style.Stroke.A != 0 {
		t.Fatal("dot style")
	}
	a.Shift(geom.V(100, 0))
	near(t, angle.Object.Path.Contours[0].Start.X, 1)
	s := gomath.NewScene()
	must(t, s.Add(angle.Object))
	must(t, s.Play(1, gomath.Shift(angle.Object, geom.V(2, 1)).WithRate(gomath.Linear)))
	if angle.Object.Children[0] != dot {
		t.Fatal("animation replaced dot identity")
	}
	near(t, angle.Vertex.X, 0) // Construction metadata is intentionally stable.
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			frame, err := s.Sample(float64(i) / 7)
			if err != nil || len(frame.Objects) != 1 || len(frame.Objects[0].Children) != 1 {
				t.Errorf("concurrent angle sample: %v", err)
			}
		}()
	}
	wg.Wait()
	for _, points := range [][4]geom.Vec3{
		{geom.V(-1, 0), geom.V(1, 0), geom.V(-1, 1), geom.V(1, 1)},
		{geom.V(-1, 0), geom.V(1, 0), geom.V(4, 0), geom.V(-2, 0)},
	} {
		p, err := gomath.NewAngle(gomath.Line(points[0], points[1]), gomath.Line(points[2], points[3]), opts)
		must(t, err)
		must(t, p.Object.Validate())
		if !p.Parallel || p.Sweep != 0 || p.Radius != 0 || len(p.Object.Path.Contours) != 0 || len(p.Object.Children) != 0 {
			t.Fatal("parallel lines must produce an empty mark", p)
		}
	}
}

func TestAngleRejectsInvalidAndIllConditionedGeometry(t *testing.T) {
	a, b := gomath.Line(geom.V(-1, 0), geom.V(1, 0)), gomath.Line(geom.V(0, -1), geom.V(0, 1))
	for _, opts := range []gomath.AngleOptions{
		{Radius: -1}, {Radius: math.NaN()}, {Radius: math.Inf(1)}, {Radius: 1e13},
		{Quadrant: [2]int{1, 0}}, {Quadrant: [2]int{2, 1}}, {DotRadius: -1},
		{DotDistance: math.NaN()}, {DotDistance: -1}, {DotColor: gomath.Color{A: 2}},
		{Dot: true, Elbow: true}, {OtherAngle: true, Elbow: true},
		{Dot: true, Radius: 1e12, DotDistance: 1e12},
	} {
		if got, err := gomath.NewAngle(a, b, opts); got != nil || err == nil {
			t.Fatal("invalid angle options accepted", opts)
		}
	}
	curved := gomath.CubicBezier(geom.V(0, 0), geom.V(0, 1), geom.V(1, 1), geom.V(1, 0))
	for _, line := range []*gomath.Object{nil, gomath.Group(a), gomath.Circle(1), curved,
		gomath.Line(geom.V(0, 0), geom.V(0, 0)), gomath.Line(geom.V(0, 0), geom.V(1e-12, 0)),
		gomath.Line(geom.Vec3{Z: 1}, geom.Vec3{X: 1, Z: 1}),
		gomath.Line(geom.V(math.Inf(1), 0), geom.V(1, 0)),
	} {
		if got, err := gomath.NewAngle(line, b, gomath.AngleOptions{}); got != nil || err == nil {
			t.Fatal("invalid line accepted", line)
		}
	}
	for _, pair := range [][2]*gomath.Object{
		{a, gomath.Line(geom.V(0, 1), geom.V(1, 1+1e-13))},
		{a, gomath.Line(geom.V(0, 10), geom.V(1, 10+2e-12))},
		// The naive product difference rounds to zero, but the exact
		// determinant of these rounded directions is -(2^-52)^2.
		{gomath.Line(geom.V(0, 0), geom.V(1, 1+0x1p-52)), gomath.Line(geom.V(0, 0), geom.V(1+0x1p-52, 1+0x1p-51))},
	} {
		if got, err := gomath.NewAngle(pair[0], pair[1], gomath.AngleOptions{Radius: .5}); got != nil || err == nil {
			t.Fatal("unstable or out-of-bounds intersection accepted")
		}
	}
	if got, err := gomath.NewAngleFromPoints(geom.V(-1, 0), geom.V(0, 0), geom.V(0, 1), gomath.AngleOptions{Quadrant: [2]int{-1, 1}}); got != nil || err == nil {
		t.Fatal("zero automatic radius should require explicit input")
	}
}

func BenchmarkAngleWithDot(b *testing.B) {
	first, second := gomath.Line(geom.V(-1, 0), geom.V(1, 0)), gomath.Line(geom.V(0, -1), geom.V(0, 1))
	opts := gomath.DefaultAngleOptions()
	opts.Dot = true
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := gomath.NewAngle(first, second, opts); err != nil {
			b.Fatal(err)
		}
	}
}
