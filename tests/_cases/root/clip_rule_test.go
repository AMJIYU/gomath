package gomath_test

import (
	"gomath"
	"gomath/geom"
	"gomath/path"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func ruleClip(t *testing.T, p path.Path, rule path.FillRule) *gomath.Clip {
	t.Helper()
	c, err := gomath.NewClipWithRule(p, rule)
	must(t, err)
	return c
}
func TestClipFillRuleImmutableDefaultAndTransforms(t *testing.T) {
	p := gomath.Square(2).Path
	c := ruleClip(t, p, path.EvenOdd)
	want := p.Clone()
	p.Contours[0].Start.X = 99
	returned := c.Path()
	returned.Contours[0].Curves[0].P1.X = 77
	if c.FillRule() != path.EvenOdd || !reflect.DeepEqual(c.Path(), want) {
		t.Fatal("clip is mutable")
	}
	defaultClip, err := gomath.NewClip(want)
	must(t, err)
	if defaultClip.FillRule() != path.NonZero || (*gomath.Clip)(nil).FillRule() != path.NonZero {
		t.Fatal("default changed")
	}
	for _, rule := range []path.FillRule{2, 255} {
		if c, err := gomath.NewClipWithRule(want, rule); c != nil || err == nil {
			t.Fatal("invalid rule accepted", rule)
		}
	}
	for _, p := range []path.Path{{Contours: make([]path.Contour, gomath.MaxClipContours+1)}, {Contours: []path.Contour{{Curves: make([]path.Cubic, gomath.MaxClipCurves+1)}}}} {
		if c, err := gomath.NewClipWithRule(p, path.EvenOdd); c != nil || err == nil {
			t.Fatal("evenodd bypassed outline limits")
		}
	}
	moved, err := c.Transform(geom.RotationZ(.3))
	must(t, err)
	o := gomath.Square(4).SetClip(c)
	cp := o.Copy()
	if cp.Clip != c {
		t.Fatal("Copy lost immutable sharing")
	}
	cp.Shift(geom.V(3, 2)).Rotate(.2)
	if moved.FillRule() != path.EvenOdd || cp.Clip.FillRule() != path.EvenOdd || o.Clip != c {
		t.Fatal("transform lost rule or ownership")
	}
}

func TestClipEvenOddMorphSuccessionReplayCamera(t *testing.T) {
	s := gomath.NewScene()
	child := gomath.Square(4).SetClip(ruleClip(t, gomath.Square(1).Path, path.EvenOdd))
	group := gomath.Group(child)
	must(t, s.Add(group))
	target := child.Copy().SetClip(ruleClip(t, gomath.Circle(2).Path, path.EvenOdd))
	// Square/circle alignment must preserve the common rule through subdivision.
	must(t, s.Play(1, gomath.Transform(child, target).WithRate(gomath.Linear)))
	must(t, s.Play(2, gomath.Succession(gomath.Shift(group, geom.V(2, 0)), gomath.Rotate(group, .6))))
	must(t, s.SetCamera(gomath.Camera2D{FrameWidth: 8, Center: geom.V(1, 2), Rotation: .2}))
	replay, err := gomath.CompileReplay(s, gomath.DefaultReplayOptions())
	must(t, err)
	for _, time := range []float64{0, .5, 1, 1.5, 2, 2.5, 3} {
		frame := sample(t, s, time)
		if frame.Find(child.ID).Clip.FillRule() != path.EvenOdd {
			t.Fatal("animation lost rule", time)
		}
		projected, err := frame.ProjectCamera()
		must(t, err)
		if projected.Find(child.ID).Clip.FillRule() != path.EvenOdd {
			t.Fatal("camera lost rule", time)
		}
		rr, err := replay.Sample(time)
		must(t, err)
		if rr.Find(child.ID).Clip.FillRule() != path.EvenOdd {
			t.Fatal("replay lost rule")
		}
	}
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 8 {
				f, err := replay.Sample(.5)
				if err != nil || f.Find(child.ID).Clip.FillRule() != path.EvenOdd {
					t.Error("concurrent replay rule", err)
				}
			}
		}()
	}
	wg.Wait()
	if group.Children[0] != child {
		t.Fatal("lost existing child pointer")
	}
}

func TestClipRuleMismatchMorphsAreAtomic(t *testing.T) {
	for _, kind := range []string{"transform", "fromcopy", "replacement", "succession", "child"} {
		t.Run(kind, func(t *testing.T) {
			s := gomath.NewScene()
			leaf := gomath.Square(2).SetClip(ruleClip(t, gomath.Square(1).Path, path.EvenOdd))
			o := leaf
			if kind == "child" {
				o = gomath.Group(leaf)
			}
			must(t, s.Add(o))
			target := o.Copy()
			targetLeaf := target
			if kind == "child" {
				targetLeaf = target.Children[0]
			}
			targetLeaf.SetClip(ruleClip(t, leaf.Clip.Path(), path.NonZero))
			a := gomath.Transform(o, target)
			switch kind {
			case "fromcopy":
				a = gomath.TransformFromCopy(o, target)
			case "replacement":
				a = gomath.ReplacementTransform(o, target)
			case "succession":
				a = gomath.Succession(gomath.Shift(o, geom.V(2, 0)), a)
			}
			before := leaf.Clip
			if err := s.Play(2, a); err == nil || !strings.Contains(err.Error(), "clip fill rules") {
				t.Fatal("mismatched rules accepted", err)
			}
			if s.Duration() != 0 || leaf.Clip != before || leaf.Center() != geom.V(0, 0) {
				t.Fatal("failed morph changed authored state")
			}
			f := sample(t, s, 0)
			if f.Find(leaf.ID) == nil || f.Find(leaf.ID).Clip != before {
				t.Fatal("failed morph changed timeline")
			}
			// Independent layers may intentionally switch rules by crossfading.
			must(t, s.Play(1, gomath.FadeTransform(o, target)))
		})
	}
}
