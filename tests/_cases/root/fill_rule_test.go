package gomath_test

import (
	"encoding/json"
	"gomath"
	"gomath/geom"
	"gomath/path"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestFillRuleValueOwnershipValidationAndSnapshots(t *testing.T) {
	if (gomath.Style{}).FillRule != path.NonZero || gomath.DefaultStyle().FillRule != path.NonZero {
		t.Fatal("zero/default fill rule changed")
	}
	// Omitting the zero value preserves serialized old style shapes as well.
	raw, err := json.Marshal(gomath.DefaultStyle())
	must(t, err)
	if strings.Contains(string(raw), "FillRule") {
		t.Fatal("default serialization changed")
	}
	child := gomath.Square(2)
	root := gomath.Group(child).SetFillRule(path.EvenOdd)
	if root.Style.FillRule != path.EvenOdd || child.Style.FillRule != path.EvenOdd {
		t.Fatal("setter did not recurse")
	}
	cp := root.Copy()
	cp.SetFillRule(path.NonZero)
	if child.Style.FillRule != path.EvenOdd || cp.Children[0].Style.FillRule != path.NonZero {
		t.Fatal("style value ownership")
	}
	s := gomath.NewScene()
	must(t, s.Add(root))
	must(t, s.Play(1, gomath.Create(root)))
	target := root.Copy().Shift(geom.V(2, 1))
	must(t, s.Play(1, gomath.Transform(root, target)))
	must(t, s.Play(2, gomath.Succession(gomath.Shift(root, geom.V(1, 0)), gomath.Rotate(root, .2))))
	must(t, s.SetCamera(gomath.Camera2D{FrameWidth: 8, Center: geom.V(1, 0), Rotation: .1}))
	replay, err := gomath.CompileReplay(s, gomath.DefaultReplayOptions())
	must(t, err)
	for _, time := range []float64{0, .35, .75, 1, 1.5, 2, 2.5, 3, 4} {
		f := sample(t, s, time)
		g, err := f.ProjectCamera()
		must(t, err)
		r, err := replay.Sample(time)
		must(t, err)
		for _, frame := range []*gomath.Frame{f, g, r} {
			if frame.Find(child.ID).Style.FillRule != path.EvenOdd {
				t.Fatal("rule lost", time)
			}
		}
	}
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 8 {
				f, e := replay.Sample(1.5)
				if e != nil || f.Find(child.ID).Style.FillRule != path.EvenOdd {
					t.Error("concurrent replay", e)
				}
			}
		}()
	}
	wg.Wait()
	for _, rule := range []path.FillRule{2, 255} {
		bad := gomath.Square(2).SetFillRule(rule)
		if bad.Validate() == nil {
			t.Fatal("invalid authoring rule accepted")
		}
		badScene := gomath.NewScene()
		if badScene.Add(bad) == nil || badScene.Duration() != 0 {
			t.Fatal("invalid scene rule accepted")
		}
	}
}

func TestMixedObjectFillRuleMorphsAreAtomic(t *testing.T) {
	for _, kind := range []string{"transform", "replacement", "fromcopy", "child", "succession", "aligned-child"} {
		t.Run(kind, func(t *testing.T) {
			s := gomath.NewScene()
			leaf := gomath.Square(2).SetFill(gomath.Blue).SetFillRule(path.EvenOdd)
			o := leaf
			if kind == "child" || kind == "aligned-child" {
				o = gomath.Group(leaf)
			}
			must(t, s.Add(o))
			target := o.Copy()
			if kind == "child" || kind == "aligned-child" {
				target.Children[0].SetFillRule(path.NonZero)
			} else {
				target.SetFillRule(path.NonZero)
			}
			if kind == "aligned-child" {
				target.Children = append(target.Children, gomath.Circle(1))
			}
			a := gomath.Transform(o, target)
			switch kind {
			case "replacement":
				a = gomath.ReplacementTransform(o, target)
			case "fromcopy":
				a = gomath.TransformFromCopy(o, target)
			case "succession":
				a = gomath.Succession(gomath.Shift(o, geom.V(3, 0)), a)
			}
			before := leaf.Path.Clone()
			style := leaf.Style
			if e := s.Play(2, a); e == nil || !strings.Contains(e.Error(), "object fill rules") {
				t.Fatal("mixed-rule morph accepted", e)
			}
			if s.Duration() != 0 || leaf.Style != style || !reflect.DeepEqual(leaf.Path, before) || sample(t, s, 0).Find(leaf.ID).Style != style {
				t.Fatal("failed morph changed author/timeline")
			}
			must(t, s.Play(1, gomath.FadeTransform(o, target)))
			frame := sample(t, s, .5)
			rules := map[path.FillRule]bool{}
			for _, root := range frame.Objects {
				root.Walk(func(n *gomath.Object) {
					if len(n.Path.Contours) > 0 {
						rules[n.Style.FillRule] = true
					}
				})
			}
			if !rules[path.NonZero] || !rules[path.EvenOdd] {
				t.Fatal("crossfade merged discrete rules", rules)
			}
		})
	}
}
