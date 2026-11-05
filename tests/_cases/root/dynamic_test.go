package gomath_test

import (
	"errors"
	"gomath"
	"gomath/geom"
	"gomath/path"
	"math"
	"reflect"
	"sync"
	"testing"
)

func TestDynamicRedrawTopologyAndDeterministicIdentities(t *testing.T) {
	s := gomath.NewScene()
	originalChild := gomath.Square(.2)
	object := gomath.Group(originalChild)
	tracker, err := gomath.NewValueTracker(0)
	must(t, err)
	must(t, s.Add(object, tracker.Object))
	id := tracker.ID
	must(t, s.AddDynamicRedraw(object, func(f *gomath.Frame, _ float64) (*gomath.Object, error) {
		value, err := f.Value(id)
		if err != nil {
			return nil, err
		}
		group := gomath.Group()
		for i := 0; i < int(math.Round(value)); i++ {
			group.Children = append(group.Children, gomath.Square(.5).MoveTo(geom.V(float64(i), value/10)))
		}
		return group, nil
	}, gomath.DynamicRedrawOptions{MaxObjects: 9}))
	must(t, s.Play(2, gomath.ValueTo(tracker, 8).WithRate(gomath.Linear)))
	a, b, c := sample(t, s, .5), sample(t, s, 1.5), sample(t, s, .5)
	if len(a.Find(object.ID).Children) != 2 || len(b.Find(object.ID).Children) != 6 {
		t.Fatal("topology did not follow sampled tracker")
	}
	if !reflect.DeepEqual(a, c) {
		t.Fatal("sampling order changed dynamic output identities or geometry")
	}
	if a.Find(object.ID).Children[0].ID != b.Find(object.ID).Children[0].ID {
		t.Fatal("ordinal child slot is not stable")
	}
	if a.Find(originalChild.ID) != nil {
		t.Fatal("dynamic output retained author child identity")
	}
	if len(object.Children) != 1 || object.Children[0] != originalChild {
		t.Fatal("display redraw mutated authoring topology")
	}
	near(t, originalChild.Center().X, 0)
	a.Find(object.ID).Children[0].Shift(geom.V(100, 0))
	near(t, sample(t, s, .5).Find(object.ID).Children[0].Center().X, 0)
	replay, err := gomath.CompileReplay(s, gomath.DefaultReplayOptions())
	must(t, err)
	want := sample(t, s, 1.25)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 10 {
				got, err := replay.Sample(1.25)
				if err != nil || !reflect.DeepEqual(got, want) {
					t.Error("dynamic concurrent replay differs", err)
				}
			}
		}()
	}
	wg.Wait()
}

func TestDynamicRedrawCopiesResultAndRegistrationOrder(t *testing.T) {
	s := gomath.NewScene()
	o := gomath.Group()
	other := gomath.Square(.2)
	must(t, s.Add(o, other))
	generated := gomath.Group(gomath.Square(.5).MoveTo(geom.V(2, 0)))
	must(t, s.AddDynamicRedraw(o, func(*gomath.Frame, float64) (*gomath.Object, error) { return generated, nil }, gomath.DynamicRedrawOptions{}))
	must(t, s.AddUpdater(func(f *gomath.Frame, _ float64) error {
		// Later callbacks see the actual generated topology, not the author tree.
		f.Find(other.ID).MoveTo(f.Find(o.ID).Children[0].Center())
		return nil
	}))
	must(t, s.Wait(1))
	f := sample(t, s, .5)
	near(t, f.Find(other.ID).Center().X, 2)
	f.Find(o.ID).Children[0].Shift(geom.V(20, 0))
	near(t, generated.Children[0].Center().X, 2)
	if generated.Children[0].ID == sample(t, s, .5).Find(o.ID).Children[0].ID {
		t.Fatal("redraw reused callback child IDs")
	}
}

func TestDynamicRedrawBoundsAndMalformedGeometry(t *testing.T) {
	valid := func(*gomath.Frame, float64) (*gomath.Object, error) { return gomath.Group(), nil }
	s := gomath.NewScene()
	o := gomath.Group()
	must(t, s.Add(o))
	for _, opt := range []gomath.DynamicRedrawOptions{{MaxObjects: -1}, {MaxObjects: 65537}, {MaxCurves: 1048577}, {MaxContours: -1}, {MaxDepth: 257}} {
		if s.AddDynamicRedraw(o, valid, opt) == nil {
			t.Fatal("invalid dynamic budgets accepted", opt)
		}
	}
	if s.AddDynamicRedraw(nil, valid, gomath.DynamicRedrawOptions{}) == nil || s.AddDynamicRedraw(o, nil, gomath.DynamicRedrawOptions{}) == nil || s.AddDynamicRedraw(gomath.Group(), valid, gomath.DynamicRedrawOptions{}) == nil {
		t.Fatal("invalid dynamic registration accepted")
	}
	cycle := gomath.Group()
	cycle.Children = []*gomath.Object{cycle}
	shared := gomath.Square(1)
	deep := gomath.Group(gomath.Group(gomath.Group()))
	tooMany := gomath.Group(gomath.Group(), gomath.Group())
	curves := gomath.Square(1)
	contours := gomath.NewObject(path.Path{Contours: []path.Contour{{Start: geom.V(0, 0)}, {Start: geom.V(1, 0)}}})
	for _, c := range []struct {
		geometry *gomath.Object
		opt      gomath.DynamicRedrawOptions
	}{
		{nil, gomath.DynamicRedrawOptions{}}, {cycle, gomath.DynamicRedrawOptions{}},
		{gomath.Group(shared, shared), gomath.DynamicRedrawOptions{}},
		{deep, gomath.DynamicRedrawOptions{MaxDepth: 2}}, {tooMany, gomath.DynamicRedrawOptions{MaxObjects: 2}},
		{curves, gomath.DynamicRedrawOptions{MaxCurves: 2}}, {contours, gomath.DynamicRedrawOptions{MaxContours: 1}},
		{gomath.Square(1).SetOpacity(math.NaN()), gomath.DynamicRedrawOptions{}},
	} {
		s := gomath.NewScene()
		root := gomath.Group()
		must(t, s.Add(root))
		must(t, s.AddDynamicRedraw(root, func(*gomath.Frame, float64) (*gomath.Object, error) { return c.geometry, nil }, c.opt))
		must(t, s.Wait(1))
		if _, err := s.Sample(.5); err == nil {
			t.Fatal("invalid dynamic geometry accepted", c.opt)
		}
		if len(root.Children) != 0 {
			t.Fatal("failed redraw changed authoring")
		}
	}
}

func TestDynamicRedrawRemovalAndErrorPropagation(t *testing.T) {
	s := gomath.NewScene()
	o := gomath.Group()
	must(t, s.Add(o))
	sentinel := errors.New("dynamic sentinel")
	calls := 0
	must(t, s.AddDynamicRedraw(o, func(*gomath.Frame, float64) (*gomath.Object, error) { calls++; return nil, sentinel }, gomath.DynamicRedrawOptions{}))
	must(t, s.Play(1, gomath.FadeOut(o)))
	if _, err := s.Sample(.5); !errors.Is(err, sentinel) {
		t.Fatal("dynamic callback error lost", err)
	}
	if _, err := s.Sample(1); err != nil {
		t.Fatal("removed dynamic target was evaluated", err)
	}
	if calls != 1 {
		t.Fatal("callback invoked for absent target", calls)
	}
}

func TestDynamicRedrawRejectsLaterForgedSlotCollision(t *testing.T) {
	s := gomath.NewScene()
	o := gomath.Group()
	must(t, s.Add(o))
	must(t, s.AddDynamicRedraw(o, func(*gomath.Frame, float64) (*gomath.Object, error) {
		return gomath.Group(gomath.Square(1)), nil
	}, gomath.DynamicRedrawOptions{MaxObjects: 2}))
	must(t, s.Wait(1))
	childID := sample(t, s, .5).Find(o.ID).Children[0].ID
	forged := gomath.Group()
	forged.ID = childID
	// The authored family is empty, so this ID is unique in authoring state.
	// It must nevertheless be rejected when it collides with a generated slot.
	must(t, s.Add(forged))
	if _, err := s.Sample(1); err == nil {
		t.Fatal("reserved dynamic ID silently aliased an unrelated object")
	}
	if len(o.Children) != 0 {
		t.Fatal("failed generated topology mutated authoring")
	}
}
