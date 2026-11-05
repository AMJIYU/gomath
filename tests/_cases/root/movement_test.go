package gomath_test

import (
	"gomath"
	"gomath/geom"
	"math"
	"testing"
)

func TestMoveAlongPathInverseLengthAndIdentity(t *testing.T) {
	// x=t^3 has a nonuniform cubic parameter, but uniform arc-length x.
	route := gomath.CubicBezier(geom.V(0, 0), geom.V(0, 0), geom.V(0, 0), geom.V(8, 0))
	child := gomath.Square(.2)
	o := gomath.Group(child)
	s := gomath.NewScene()
	if err := s.Add(o); err != nil {
		t.Fatal(err)
	}
	if err := s.Play(2, gomath.MoveAlongPath(o, route)); err != nil {
		t.Fatal(err)
	}
	route.Shift(geom.V(100, 0))
	if o.Children[0] != child {
		t.Fatal("authoring child identity lost")
	}
	for _, at := range []float64{1.75, 0, .25, 1, 2, .25} {
		f, err := s.Sample(at)
		if err != nil {
			t.Fatal(err)
		}
		got := f.Find(child.ID)
		if got == nil || math.Abs(got.Center().X-4*at) > 2e-6 || math.Abs(got.Bounds().Size().X-.2) > 1e-8 {
			t.Fatalf("time %g: wrong geometry %#v", at, got)
		}
	}
}

func TestMoveAlongClosedPathMidpoints(t *testing.T) {
	o := gomath.Square(.2)
	s := gomath.NewScene()
	if err := s.Add(o); err != nil {
		t.Fatal(err)
	}
	if err := s.Play(4, gomath.MoveAlongPath(o, gomath.Circle(2))); err != nil {
		t.Fatal(err)
	}
	for i, want := range []geom.Vec3{geom.V(2, 0), geom.V(0, 2), geom.V(-2, 0), geom.V(0, -2), geom.V(2, 0)} {
		f, err := s.Sample(float64(i))
		if err != nil {
			t.Fatal(err)
		}
		if f.Find(o.ID).Center().Sub(want).Norm() > 2e-6 {
			t.Fatalf("quarter %d center %v", i, f.Find(o.ID).Center())
		}
	}
}

func TestMoveAlongPathRejectsInvalidInputAtomically(t *testing.T) {
	for _, route := range []*gomath.Object{nil, gomath.Group(), gomath.Group(gomath.Line(geom.V(0, 0), geom.V(1, 0)))} {
		s := gomath.NewScene()
		o := gomath.Square(1)
		if err := s.Add(o); err != nil {
			t.Fatal(err)
		}
		if err := s.Play(1, gomath.MoveAlongPath(o, route)); err == nil {
			t.Fatal("invalid path accepted")
		}
		if s.Duration() != 0 || o.Center() != geom.V(0, 0) {
			t.Fatal("failure mutated scene")
		}
	}
	s := gomath.NewScene()
	o := gomath.Square(1)
	if err := s.Add(o); err != nil {
		t.Fatal(err)
	}
	if err := s.Play(1, gomath.MoveAlongPathWithTolerance(o, gomath.Circle(1), math.NaN())); err == nil {
		t.Fatal("nonfinite tolerance accepted")
	}
}
