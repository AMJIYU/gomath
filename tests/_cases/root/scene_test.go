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

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func sample(t *testing.T, s *gomath.Scene, time float64) *gomath.Frame {
	t.Helper()
	f, e := s.Sample(time)
	must(t, e)
	return f
}
func near(t *testing.T, a, b float64) {
	t.Helper()
	if math.Abs(a-b) > 1e-8 {
		t.Fatalf("got %g, want %g", a, b)
	}
}
func TestSnapshotAndOrderIndependence(t *testing.T) {
	s := gomath.NewScene()
	o := gomath.Square(2)
	must(t, s.Add(o))
	must(t, s.Play(2, gomath.Shift(o, geom.V(4, 0)).WithRate(gomath.Linear)))
	mid := sample(t, s, 1)
	near(t, mid.Find(o.ID).Center().X, 2)
	o.Shift(geom.V(100, 0))
	mid.Find(o.ID).Shift(geom.V(100, 0))
	near(t, sample(t, s, 1).Find(o.ID).Center().X, 2)
	near(t, sample(t, s, 0).Find(o.ID).Center().X, 0)
	near(t, sample(t, s, 2).Find(o.ID).Center().X, 4)
}
func TestFullRotationUsesCircularMotion(t *testing.T) {
	s := gomath.NewScene()
	o := gomath.Rectangle(4, 1)
	must(t, s.Add(o))
	must(t, s.Play(1, gomath.Rotate(o, 2*math.Pi).WithRate(gomath.Linear)))
	b := sample(t, s, .25).Find(o.ID).Bounds().Size()
	near(t, b.X, 1)
	near(t, b.Y, 4)
	b = sample(t, s, 1).Find(o.ID).Bounds().Size()
	near(t, b.X, 4)
	near(t, b.Y, 1)
}
func TestGroupTransformPreservesExistingChildReferences(t *testing.T) {
	child := gomath.Square(1)
	group := gomath.Group(child)
	s := gomath.NewScene()
	must(t, s.Add(group))
	target := gomath.Group(gomath.Circle(1).MoveTo(geom.V(2, 0)), gomath.Square(.5).MoveTo(geom.V(-2, 0)))
	must(t, s.Play(1, gomath.Transform(group, target)))
	if group.Children[0] != child {
		t.Fatal("existing child reference replaced")
	}
	near(t, child.Center().X, 2)
	if len(group.Children) != 2 {
		t.Fatal("new child missing")
	}
	if group.Children[1].ID == target.Children[1].ID {
		t.Fatal("target child identity reused")
	}
	must(t, s.Wait(.1))
	if len(sample(t, s, .5).Objects[0].Children) != 2 {
		t.Fatal("aligned child missing")
	}
}
func TestFadeWindowAndBoundary(t *testing.T) {
	s := gomath.NewScene()
	o := gomath.Circle(1)
	must(t, s.Play(1, gomath.FadeIn(o).WithWindow(.5, 1).WithRate(gomath.Linear)))
	near(t, sample(t, s, .25).Find(o.ID).Style.Opacity, 0)
	near(t, sample(t, s, .75).Find(o.ID).Style.Opacity, .5)
	must(t, s.Play(1, gomath.FadeOut(o)))
	near(t, sample(t, s, 1).Find(o.ID).Style.Opacity, 1)
	if sample(t, s, 2).Find(o.ID) != nil {
		t.Fatal("FadeOut did not remove object")
	}
}
func TestConflictAndInvalidInputAreAtomic(t *testing.T) {
	s := gomath.NewScene()
	child := gomath.Square(1)
	group := gomath.Group(child)
	must(t, s.Add(group))
	before := group.Copy().Bounds()
	if e := s.Play(1, gomath.Shift(child, geom.V(2, 0)), gomath.Shift(group, geom.V(1, 0))); e == nil {
		t.Fatal("overlapping parent and child accepted")
	}
	near(t, s.Duration(), 0)
	if group.Bounds() != before {
		t.Fatal("failed Play changed geometry")
	}
	other := gomath.Circle(1)
	if e := s.Play(1, gomath.FadeIn(other).WithWindow(.8, .2)); e == nil {
		t.Fatal("invalid window accepted")
	}
	if sample(t, s, 0).Find(other.ID) != nil {
		t.Fatal("failed Play added object")
	}
	for _, v := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		if s.Wait(v) == nil {
			t.Fatalf("accepted duration %g", v)
		}
	}
}
func TestRejectCyclesSharedChildrenAndBadStyles(t *testing.T) {
	s := gomath.NewScene()
	o := gomath.Group()
	o.Children = append(o.Children, o)
	if s.Add(o) == nil {
		t.Fatal("cycle accepted")
	}
	child := gomath.Square(1)
	if s.Add(gomath.Group(child), gomath.Group(child)) == nil {
		t.Fatal("shared child accepted")
	}
	if s.Add(gomath.Square(1).SetOpacity(math.NaN())) == nil {
		t.Fatal("NaN opacity accepted")
	}
}
func TestConcurrentSampling(t *testing.T) {
	s := gomath.NewScene()
	o := gomath.Square(1)
	must(t, s.Add(o))
	must(t, s.Play(2, gomath.Shift(o, geom.V(4, 0))))
	want := sample(t, s, .71)
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 10 {
				f, e := s.Sample(.71)
				if e != nil || !reflect.DeepEqual(f, want) {
					t.Error("concurrent sample differs")
				}
			}
		}()
	}
	wg.Wait()
}
func TestUpdaterUsesPrivateFrame(t *testing.T) {
	s := gomath.NewScene()
	o := gomath.Dot(geom.V(0, 0), .1)
	must(t, s.Add(o))
	must(t, s.Wait(2))
	must(t, s.AddUpdater(func(f *gomath.Frame, time float64) error { f.Find(o.ID).MoveTo(geom.V(time*time, 0)); return nil }))
	near(t, sample(t, s, 1.5).Find(o.ID).Center().X, 2.25)
	near(t, sample(t, s, .5).Find(o.ID).Center().X, .25)
	near(t, o.Center().X, 0)
	sentinel := errors.New("updater failed")
	must(t, s.AddUpdater(func(*gomath.Frame, float64) error { return sentinel }))
	_, e := s.Sample(0)
	if !errors.Is(e, sentinel) {
		t.Fatal("updater error not propagated")
	}
}
func TestCopyAssignsUniqueIDs(t *testing.T) {
	a := gomath.Group(gomath.Circle(1), gomath.Square(1))
	b := a.Copy()
	s := gomath.NewScene()
	must(t, s.Add(a, b))
	b.Children[0].Shift(geom.V(3, 0))
	near(t, a.Children[0].Center().X, 0)
}
func TestThereAndBackFinalState(t *testing.T) {
	s := gomath.NewScene()
	o := gomath.Square(1)
	must(t, s.Add(o))
	must(t, s.Play(1, gomath.Shift(o, geom.V(3, 0)).WithRate(gomath.ThereAndBack)))
	near(t, sample(t, s, .5).Find(o.ID).Center().X, 3)
	near(t, o.Center().X, 0)
}

func TestGroupRotationKeepsChildReferences(t *testing.T) {
	child := gomath.Dot(geom.V(1, 0), .1)
	group := gomath.Group(child, gomath.Dot(geom.V(-1, 0), .1))
	s := gomath.NewScene()
	must(t, s.Add(group))
	must(t, s.Play(1, gomath.Rotate(group, math.Pi).WithRate(gomath.Linear)))
	if group.Children[0] != child {
		t.Fatal("rotation replaced child reference")
	}
	near(t, child.Center().X, -1)
}
func TestRateErrorsAreAtomicAndPropagate(t *testing.T) {
	s := gomath.NewScene()
	a, b := gomath.Square(1), gomath.Circle(1)
	must(t, s.Add(a, b))
	bad := func(float64) float64 { return math.NaN() }
	if e := s.Play(1, gomath.Shift(a, geom.V(2, 0)), gomath.Shift(b, geom.V(3, 0)).WithRate(bad)); e == nil {
		t.Fatal("non-finite rate accepted")
	}
	near(t, a.Center().X, 0)
	near(t, s.Duration(), 0)
	middleBad := func(t float64) float64 {
		if t > .1 && t < .9 {
			return math.NaN()
		}
		return t
	}
	must(t, s.Play(1, gomath.Shift(a, geom.V(2, 0)).WithRate(middleBad)))
	if _, e := s.Sample(.5); e == nil {
		t.Fatal("sample rate error lost")
	}
}

func TestFiniteRateOverflowReturnsSamplingError(t *testing.T) {
	rate := func(alpha float64) float64 {
		if alpha > 0 && alpha < 1 {
			return 4
		}
		return alpha
	}
	for _, kind := range []string{"scalar", "geometry", "rotation"} {
		s := gomath.NewScene()
		var object *gomath.Object
		var animation gomath.Animation
		switch kind {
		case "scalar":
			tracker, err := gomath.NewValueTracker(1e308)
			must(t, err)
			object, animation = tracker.Object, gomath.ValueTo(tracker, -1e308)
		case "geometry":
			object = gomath.Square(1)
			animation = gomath.Shift(object, geom.V(1e308, 0))
		case "rotation":
			object = gomath.Square(1)
			animation = gomath.Rotate(object, math.MaxFloat64)
		}
		must(t, s.Add(object))
		must(t, s.Play(1, animation.WithRate(rate)))
		before := sample(t, s, 1)
		if _, err := s.Sample(.5); err == nil {
			t.Fatal("finite rate arithmetic overflow returned an invalid frame", kind)
		}
		if !reflect.DeepEqual(before, sample(t, s, 1)) {
			t.Fatal("failed sample changed captured endpoint", kind)
		}
	}
}
func TestHex(t *testing.T) {
	c, e := gomath.Hex("#ff008080")
	must(t, e)
	near(t, c.R, 1)
	near(t, c.A, 128.0/255)
	if _, e = gomath.Hex("invalid"); e == nil {
		t.Fatal("invalid color accepted")
	}
}
