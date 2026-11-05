package gomath_test

import (
	"errors"
	"gomath"
	"gomath/geom"
	"math"
	"reflect"
	"sync"
	"testing"
)

func managedUpdater(t *testing.T, s *gomath.Scene, o *gomath.Object, fn gomath.ObjectUpdater, options gomath.ObjectUpdaterOptions) *gomath.UpdaterHandle {
	t.Helper()
	h, e := s.AddObjectUpdater(o, fn, options)
	must(t, e)
	if h == nil {
		t.Fatal("nil registered handle")
	}
	return h
}

func TestObjectUpdaterCursorLifecycleAndLegacyOrder(t *testing.T) {
	s := gomath.NewScene()
	o := gomath.Square(1)
	must(t, s.Add(o))
	must(t, s.Wait(1))
	h := managedUpdater(t, s, o, func(o *gomath.Object, _ *gomath.Frame, time float64) error { o.MoveTo(geom.V(time, 0)); return nil }, gomath.DefaultObjectUpdaterOptions())
	must(t, s.AddUpdater(func(f *gomath.Frame, _ float64) error { f.Find(o.ID).Shift(geom.V(.25, 0)); return nil }))
	must(t, s.Wait(1))
	must(t, s.SuspendUpdater(h))
	must(t, s.Wait(1))
	must(t, s.ResumeUpdater(h))
	must(t, s.Wait(1))
	must(t, s.RemoveUpdater(h))
	must(t, s.Wait(1))
	for _, c := range []struct{ time, x float64 }{{0, .25}, {.9, .25}, {1, 1.25}, {1.5, 1.75}, {2, .25}, {2.5, .25}, {3, 3.25}, {3.8, 4.05}, {4, .25}, {5, .25}, {1.5, 1.75}} {
		near(t, sample(t, s, c.time).Find(o.ID).Center().X, c.x)
	}
	near(t, o.Center().X, 0)
	if s.ResumeUpdater(h) == nil || s.SuspendUpdater(h) == nil || s.RemoveUpdater(h) == nil {
		t.Fatal("removed handle accepted")
	}
}

func TestObjectUpdaterSameCursorControlsAndHandleOwnership(t *testing.T) {
	s := gomath.NewScene()
	o := gomath.Group()
	must(t, s.Add(o))
	fn := func(o *gomath.Object, _ *gomath.Frame, _ float64) error { o.Scalar = 7; return nil }
	h := managedUpdater(t, s, o, fn, gomath.ObjectUpdaterOptions{})
	must(t, s.SuspendUpdater(h))
	must(t, s.ResumeUpdater(h))
	must(t, s.ResumeUpdater(h))
	must(t, s.Wait(1))
	near(t, sample(t, s, 0).Find(o.ID).Scalar, 7)
	must(t, s.SuspendUpdater(h))
	must(t, s.SuspendUpdater(h))
	must(t, s.Wait(1))
	near(t, sample(t, s, 1).Find(o.ID).Scalar, 0)
	must(t, s.ResumeUpdater(h))
	must(t, s.RemoveUpdater(h))
	must(t, s.Wait(1))
	near(t, sample(t, s, 2).Find(o.ID).Scalar, 0)
	foreign := gomath.NewScene()
	for _, bad := range []*gomath.UpdaterHandle{nil, {}, h} {
		if foreign.ResumeUpdater(bad) == nil || foreign.SuspendUpdater(bad) == nil || foreign.RemoveUpdater(bad) == nil {
			t.Fatal("foreign/uninitialized handle accepted")
		}
	}
	var nilScene *gomath.Scene
	if _, e := nilScene.AddObjectUpdater(o, fn, gomath.ObjectUpdaterOptions{}); e == nil {
		t.Fatal("nil scene")
	}
	if nilScene.RemoveUpdater(h) == nil {
		t.Fatal("nil scene control")
	}
	for _, c := range []struct {
		o       *gomath.Object
		f       gomath.ObjectUpdater
		options gomath.ObjectUpdaterOptions
	}{{nil, fn, gomath.ObjectUpdaterOptions{}}, {o, nil, gomath.ObjectUpdaterOptions{}}, {gomath.Square(1), fn, gomath.ObjectUpdaterOptions{}}, {o, fn, gomath.ObjectUpdaterOptions{Limits: gomath.DynamicRedrawOptions{MaxObjects: -1}}}} {
		if hh, e := s.AddObjectUpdater(c.o, c.f, c.options); e == nil || hh != nil {
			t.Fatal("invalid registration accepted")
		}
	}
	impostor := o.Copy()
	impostor.ID = o.ID
	if _, e := s.AddObjectUpdater(impostor, fn, gomath.ObjectUpdaterOptions{}); e == nil {
		t.Fatal("forged scene pointer accepted")
	}
}

func TestObjectUpdaterAnimationFamilySuspensionAndWindows(t *testing.T) {
	for _, automatic := range []bool{true, false} {
		s := gomath.NewScene()
		child := gomath.Square(1)
		parent := gomath.Group(child)
		other := gomath.Square(1).MoveTo(geom.V(3, 0))
		must(t, s.Add(parent, other))
		options := gomath.DefaultObjectUpdaterOptions()
		options.SuspendDuringAnimations = automatic
		fn := func(o *gomath.Object, _ *gomath.Frame, _ float64) error { o.Scalar = 17; return nil }
		managedUpdater(t, s, child, fn, options)
		managedUpdater(t, s, other, fn, options)
		// The read-only morph target is owned by the scene but is not animated.
		must(t, s.Play(2, gomath.Transform(parent, gomath.Group(other.Copy())).WithWindow(.25, .75)))
		for _, time := range []float64{0, .2, .5, 1, 1.8} {
			f := sample(t, s, time)
			want := 17.
			if automatic {
				want = 0
			}
			near(t, f.Find(child.ID).Scalar, want)
			near(t, f.Find(other.ID).Scalar, 17)
		}
		near(t, sample(t, s, 2).Find(child.ID).Scalar, 17)
	}
	// Parent callbacks remain active while only a child is animated.
	s := gomath.NewScene()
	child := gomath.Square(1)
	parent := gomath.Group(child)
	must(t, s.Add(parent))
	managedUpdater(t, s, parent, func(o *gomath.Object, _ *gomath.Frame, _ float64) error { o.Scalar = 9; return nil }, gomath.DefaultObjectUpdaterOptions())
	must(t, s.Play(1, gomath.Shift(child, geom.V(2, 0))))
	near(t, sample(t, s, .5).Find(parent.ID).Scalar, 9)
	// Reading a live target is never an animation on that target.
	s = gomath.NewScene()
	a, b := gomath.Square(1), gomath.Square(1).MoveTo(geom.V(3, 0))
	must(t, s.Add(a, b))
	managedUpdater(t, s, b, func(o *gomath.Object, _ *gomath.Frame, _ float64) error { o.Scalar = 23; return nil }, gomath.DefaultObjectUpdaterOptions())
	must(t, s.Play(1, gomath.Transform(a, b)))
	near(t, sample(t, s, .5).Find(b.ID).Scalar, 23)
}

func TestObjectUpdaterSuccessionAndReplacementBinding(t *testing.T) {
	s := gomath.NewScene()
	o, other := gomath.Square(1), gomath.Square(1).MoveTo(geom.V(3, 0))
	must(t, s.Add(o, other))
	managedUpdater(t, s, o, func(o *gomath.Object, _ *gomath.Frame, _ float64) error { o.Scalar = 4; return nil }, gomath.DefaultObjectUpdaterOptions())
	must(t, s.Play(2, gomath.Succession(gomath.Shift(other, geom.V(0, 1)), gomath.Shift(o, geom.V(1, 0)))))
	near(t, sample(t, s, .5).Find(o.ID).Scalar, 4)
	near(t, sample(t, s, 1).Find(o.ID).Scalar, 0)
	near(t, sample(t, s, 1.5).Find(o.ID).Scalar, 0)
	near(t, sample(t, s, 2).Find(o.ID).Scalar, 4)
	target := gomath.Circle(.5)
	must(t, s.Play(1, gomath.ReplacementTransform(o, target)))
	must(t, s.Wait(1))
	if sample(t, s, 3).Find(o.ID) != nil {
		t.Fatal("source remains after replacement")
	}
	near(t, sample(t, s, 3.5).Find(target.ID).Scalar, 0)
	// An absent root is skipped, but binding remains if the same object is re-added.
	must(t, s.Add(o))
	must(t, s.Wait(1))
	near(t, sample(t, s, 4.5).Find(o.ID).Scalar, 4)
}

func TestObjectUpdaterRejectsBadOutputWithoutPublishing(t *testing.T) {
	sentinel := errors.New("updater sentinel")
	cases := []struct {
		name string
		f    gomath.ObjectUpdater
	}{
		{"error", func(o *gomath.Object, _ *gomath.Frame, _ float64) error { o.Shift(geom.V(7, 0)); return sentinel }},
		{"panic", func(o *gomath.Object, _ *gomath.Frame, _ float64) error { o.Scalar = 3; panic("bad") }},
		{"panic nil", func(*gomath.Object, *gomath.Frame, float64) error { panic(nil) }},
		{"id", func(o *gomath.Object, _ *gomath.Frame, _ float64) error { o.ID++; return nil }},
		{"new child", func(o *gomath.Object, _ *gomath.Frame, _ float64) error {
			o.Children = append(o.Children, gomath.Group())
			return nil
		}},
		{"replaced child", func(o *gomath.Object, _ *gomath.Frame, _ float64) error {
			o.Children[0] = o.Children[0].Copy()
			return nil
		}},
		{"cycle", func(o *gomath.Object, _ *gomath.Frame, _ float64) error { o.Children[0] = o; return nil }},
		{"invalid", func(o *gomath.Object, _ *gomath.Frame, _ float64) error { o.Style.Opacity = math.NaN(); return nil }},
		{"curves", func(o *gomath.Object, _ *gomath.Frame, _ float64) error {
			o.Children[0].Path = gomath.Circle(1).Path
			return nil
		}},
		{"remove root", func(_ *gomath.Object, f *gomath.Frame, _ float64) error { f.Objects = nil; return nil }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := gomath.NewScene()
			child := gomath.Line(geom.V(0, 0), geom.V(1, 0))
			o := gomath.Group(child)
			must(t, s.Add(o))
			snapshot := o.Copy()
			opt := gomath.ObjectUpdaterOptions{Limits: gomath.DynamicRedrawOptions{MaxCurves: 1}}
			managedUpdater(t, s, o, c.f, opt)
			must(t, s.Wait(1))
			f, e := s.Sample(.5)
			if e == nil || f != nil {
				t.Fatal("bad callback published frame", e)
			}
			if c.name == "error" && !errors.Is(e, sentinel) {
				t.Fatal("error chain lost", e)
			}
			if len(o.Children) != 1 || o.Children[0] != child || !reflect.DeepEqual(o.Path, snapshot.Path) || o.Scalar != snapshot.Scalar || o.Style != snapshot.Style {
				t.Fatal("failed callback changed author tree")
			}
			near(t, child.Center().X, .5)
		})
	}
	// Legacy callbacks are still supported; malformed earlier output is bounded
	// before a managed callback can recursively inspect its target.
	s := gomath.NewScene()
	o := gomath.Group()
	must(t, s.Add(o))
	must(t, s.AddUpdater(func(f *gomath.Frame, _ float64) error {
		f.Find(o.ID).Children = []*gomath.Object{f.Find(o.ID)}
		return nil
	}))
	called := false
	managedUpdater(t, s, o, func(*gomath.Object, *gomath.Frame, float64) error { called = true; return nil }, gomath.ObjectUpdaterOptions{})
	must(t, s.Wait(1))
	if _, e := s.Sample(.5); e == nil || called {
		t.Fatal("malformed earlier output entered managed callback")
	}
}

func TestObjectUpdaterReplayFreezeAndConcurrentSampling(t *testing.T) {
	s := gomath.NewScene()
	o := gomath.Square(1)
	must(t, s.Add(o))
	h := managedUpdater(t, s, o, func(o *gomath.Object, _ *gomath.Frame, time float64) error {
		o.MoveTo(geom.V(time, time*time))
		return nil
	}, gomath.DefaultObjectUpdaterOptions())
	must(t, s.Wait(2))
	replay, e := gomath.CompileReplay(s, gomath.DefaultReplayOptions())
	must(t, e)
	must(t, s.RemoveUpdater(h)) // same-cursor edit must not alias frozen events
	o.MoveTo(geom.V(100, 100))
	expected, e := replay.Sample(2)
	must(t, e)
	near(t, expected.Find(o.ID).Center().X, 2)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for _, time := range []float64{2, .1, 1.3, 0, 1.3, 2} {
				f, e := replay.Sample(time)
				if e != nil {
					t.Error(e)
					return
				}
				q := f.Find(o.ID)
				if math.Abs(q.Center().X-time) > 1e-12 || math.Abs(q.Center().Y-time*time) > 1e-12 {
					t.Error("shared sampler state")
					return
				}
				q.Shift(geom.V(8, 8))
			}
		})
	}
	wg.Wait()
}

func TestObjectUpdaterClearRecursionAndRegistrationLimit(t *testing.T) {
	s := gomath.NewScene()
	child := gomath.Square(1)
	parent := gomath.Group(child)
	must(t, s.Add(parent))
	fn := func(o *gomath.Object, _ *gomath.Frame, _ float64) error { o.Scalar = 8; return nil }
	a := managedUpdater(t, s, parent, fn, gomath.ObjectUpdaterOptions{})
	b := managedUpdater(t, s, child, fn, gomath.ObjectUpdaterOptions{})
	must(t, s.Wait(1))
	must(t, s.ClearObjectUpdaters(parent, false))
	must(t, s.Wait(1))
	f := sample(t, s, 1.5)
	near(t, f.Find(parent.ID).Scalar, 0)
	near(t, f.Find(child.ID).Scalar, 8)
	if s.ResumeUpdater(a) == nil {
		t.Fatal("clear did not remove parent registration")
	}
	must(t, s.ClearObjectUpdaters(parent, true))
	must(t, s.Wait(1))
	near(t, sample(t, s, 2.5).Find(child.ID).Scalar, 0)
	if s.ResumeUpdater(b) == nil {
		t.Fatal("recursive clear missed child")
	}
	if s.ClearObjectUpdaters(nil, true) == nil || s.ClearObjectUpdaters(parent.Copy(), true) == nil {
		t.Fatal("invalid clear accepted")
	}
	s = gomath.NewScene()
	o := gomath.Group()
	must(t, s.Add(o))
	fn = func(*gomath.Object, *gomath.Frame, float64) error { return nil }
	var last *gomath.UpdaterHandle
	for range gomath.MaxObjectUpdaters {
		last = managedUpdater(t, s, o, fn, gomath.ObjectUpdaterOptions{})
	}
	if h, e := s.AddObjectUpdater(o, fn, gomath.ObjectUpdaterOptions{}); e == nil || h != nil {
		t.Fatal("registration budget ignored")
	}
	must(t, s.RemoveUpdater(last))
	if _, e := s.AddObjectUpdater(o, fn, gomath.ObjectUpdaterOptions{}); e == nil {
		t.Fatal("historical registrations silently discarded/reused")
	}
}

func TestObjectUpdaterManagedRedrawLifecycleAndIndependentGeometry(t *testing.T) {
	s := gomath.NewScene()
	o := gomath.Group(gomath.Square(1))
	must(t, s.Add(o))
	childID := o.Children[0].ID
	geometry := gomath.Group(gomath.Square(.5).MoveTo(geom.V(2, 0)))
	h, e := s.AddObjectRedraw(o, func(*gomath.Frame, float64) (*gomath.Object, error) { return geometry, nil }, gomath.DefaultObjectUpdaterOptions())
	must(t, e)
	must(t, s.Wait(1))
	must(t, s.SuspendUpdater(h))
	must(t, s.Wait(1))
	must(t, s.ResumeUpdater(h))
	must(t, s.Wait(1))
	f := sample(t, s, .5)
	near(t, f.Find(childID).Center().X, 2)
	f.Find(childID).Shift(geom.V(20, 0))
	near(t, geometry.Children[0].Center().X, 2)
	near(t, sample(t, s, 1.5).Find(childID).Center().X, 0)
	near(t, sample(t, s, 2.5).Find(childID).Center().X, 2)
	for _, draw := range []gomath.RedrawFunc{nil, func(*gomath.Frame, float64) (*gomath.Object, error) { return nil, nil }, func(*gomath.Frame, float64) (*gomath.Object, error) { return gomath.Group(), nil }, func(*gomath.Frame, float64) (*gomath.Object, error) { return gomath.Group(gomath.Circle(1)), nil }} {
		q := gomath.NewScene()
		root := gomath.Group(gomath.Line(geom.V(0, 0), geom.V(1, 0)))
		must(t, q.Add(root))
		handle, e := q.AddObjectRedraw(root, draw, gomath.ObjectUpdaterOptions{Limits: gomath.DynamicRedrawOptions{MaxCurves: 1}})
		if draw == nil {
			if e == nil || handle != nil {
				t.Fatal("nil managed redraw accepted")
			}
			continue
		}
		must(t, e)
		must(t, q.Wait(1))
		if f, e := q.Sample(.5); e == nil || f != nil {
			t.Fatal("invalid managed redraw published frame")
		}
	}
}
