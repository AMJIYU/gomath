package mathtex_test

import (
	"gomath"
	"gomath/geom"
	"gomath/mathtex"
	"testing"
)

func TestMatchingExplicitMathParts(t *testing.T) {
	a, err := mathtex.NewParts([]string{"x", "+", "x", "=", "y"}, mathtex.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	b, err := mathtex.NewParts([]string{"y", "=", "2", "x"}, mathtex.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	b.Object().Shift(geom.V(0, 1))
	animation, err := mathtex.TransformMatchingParts(a, b)
	if err != nil {
		t.Fatal(err)
	}
	s := gomath.NewScene()
	if err := s.Add(a.Object()); err != nil {
		t.Fatal(err)
	}
	if err := s.Play(1, animation); err != nil {
		t.Fatal(err)
	}
	for _, time := range []float64{0, .25, .5, .75, 1} {
		f, err := s.Sample(time)
		if err != nil {
			t.Fatal(err)
		}
		if err := f.Objects[0].Validate(); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := a.Snapshot(); err != nil {
		t.Fatal("source Parts ownership invalid after cleanup", err)
	}
	if _, err := b.Snapshot(); err != nil {
		t.Fatal("target Parts ownership invalid after cleanup", err)
	}
	p, _ := b.Part(3)
	if err := s.Play(1, gomath.Shift(p, geom.V(0, .5))); err != nil {
		t.Fatal("target live handle lost", err)
	}
	if _, err := mathtex.TransformMatchingParts(nil, b); err == nil {
		t.Fatal("nil source accepted")
	}
}
