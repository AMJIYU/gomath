package gomath_test

import (
	"gomath"
	"gomath/geom"
	"math"
	"testing"
)

func TestReplacementInstallsActualRootAndNestedTarget(t *testing.T) {
	for _, nested := range []bool{false, true} {
		s := gomath.NewScene()
		source := gomath.Square(1).MoveTo(geom.V(-2, 0))
		target := gomath.Circle(.6).MoveTo(geom.V(2, 0))
		sibling := gomath.Square(.2).MoveTo(geom.V(0, 2))
		var parent *gomath.Object
		if nested {
			parent = gomath.Group(source, sibling)
			if err := s.Add(parent); err != nil {
				t.Fatal(err)
			}
		} else {
			if err := s.Add(source, sibling); err != nil {
				t.Fatal(err)
			}
		}
		if err := s.Play(1, gomath.ReplacementTransform(source, target).WithRate(gomath.Linear)); err != nil {
			t.Fatal(err)
		}
		if nested && (len(parent.Children) != 2 || parent.Children[0] != target || parent.Children[1] != sibling) {
			t.Fatal("nested replacement lost target or sibling identity")
		}
		mid, err := s.Sample(.5)
		if err != nil {
			t.Fatal(err)
		}
		if mid.Find(source.ID) == nil || mid.Find(target.ID) != nil || math.Abs(mid.Find(source.ID).Center().X) > 1e-8 {
			t.Fatal("mid-animation replacement IDs or position wrong")
		}
		end, err := s.Sample(1)
		if err != nil {
			t.Fatal(err)
		}
		if end.Find(source.ID) != nil || end.Find(target.ID) == nil {
			t.Fatal("endpoint identities not replaced")
		}
		if err := s.Play(1, gomath.Shift(target, geom.V(1, 0))); err != nil {
			t.Fatal("actual target not available for later animation", err)
		}
	}
}

func TestReplacementAndRemovalTransactionalTopology(t *testing.T) {
	s := gomath.NewScene()
	source := gomath.Square(1)
	target := gomath.Circle(1)
	if err := s.Add(source); err != nil {
		t.Fatal(err)
	}
	if err := s.Play(1, gomath.ReplacementTransform(source, target), gomath.FadeIn(target)); err == nil || s.Duration() != 0 {
		t.Fatal("duplicate target committed")
	}
	if source.Path.Contours[0].Curves[0].P3 != geom.V(.5, -.5) {
		t.Fatal("failed replacement mutated source")
	}
	if err := s.Add(target); err != nil {
		t.Fatal(err)
	}
	if err := s.Play(1, gomath.ReplacementTransform(source, target)); err == nil {
		t.Fatal("existing target accepted")
	}
	forged := source.Copy()
	forged.ID = source.ID
	if err := s.Play(1, gomath.Shift(forged, geom.V(1, 0))); err == nil {
		t.Fatal("forged scene identity accepted")
	}
	child := gomath.Square(.3)
	sibling := gomath.Circle(.2)
	parent := gomath.Group(child, sibling)
	s2 := gomath.NewScene()
	if err := s2.Add(parent); err != nil {
		t.Fatal(err)
	}
	if err := s2.Play(1, gomath.FadeOut(child)); err != nil {
		t.Fatal(err)
	}
	if len(parent.Children) != 1 || parent.Children[0] != sibling {
		t.Fatal("nested FadeOut did not remove only its target")
	}
	before, err := s2.Sample(.5)
	if err != nil || before.Find(child.ID) == nil {
		t.Fatal("historical child disappeared", err)
	}
	s2.Remove(sibling)
	if len(parent.Children) != 0 {
		t.Fatal("nested Remove failed")
	}
}
