package gomath_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"gomath"
	"gomath/geom"
	"math"
	"os"
	"reflect"
	"sync"
	"testing"
)

func TestSuccessionSharedTargetAndNestedDurations(t *testing.T) {
	s := gomath.NewScene()
	o := gomath.Square(1)
	must(t, s.Add(o))
	sequence := gomath.Succession(
		gomath.Shift(o, geom.V(2, 0)).WithRate(gomath.Linear),
		gomath.Succession(
			gomath.Shift(o, geom.V(4, 0)).WithRate(gomath.Linear),
			gomath.MoveTo(o, geom.V(-2, 0)).WithRate(gomath.Linear).WithDuration(3),
		).WithDuration(2),
	)
	must(t, s.Play(6, sequence))
	for _, c := range []struct{ time, x float64 }{{0, 0}, {1, 1}, {2, 2}, {2.5, 4}, {3, 6}, {4.5, 2}, {6, -2}} {
		near(t, sample(t, s, c.time).Find(o.ID).Center().X, c.x)
	}
	near(t, o.Center().X, -2)
	near(t, s.Duration(), 6)
}

func TestSuccessionBeginStateRotationScaleAndPointers(t *testing.T) {
	s := gomath.NewScene()
	left, right := gomath.Square(.5).Shift(geom.V(-1, 0)), gomath.Square(.5).Shift(geom.V(1, 0))
	group := gomath.Group(left, right)
	must(t, s.Add(group))
	must(t, s.Play(4, gomath.Succession(
		gomath.Shift(group, geom.V(3, 0)).WithRate(gomath.Linear),
		gomath.Rotate(group, math.Pi).WithRate(gomath.Linear),
		gomath.Scale(group, 2).WithRate(gomath.Linear),
		gomath.Rotate(group, math.Pi).WithRate(gomath.Linear),
	)))
	near(t, sample(t, s, 1.5).Find(left.ID).Center().X, 3)
	near(t, sample(t, s, 1.5).Find(left.ID).Center().Y, -1)
	near(t, sample(t, s, 2.5).Find(left.ID).Center().X, 4.5)
	near(t, left.Center().X, 1)
	near(t, right.Center().X, 5)
	if group.Children[0] != left || group.Children[1] != right {
		t.Fatal("Succession replaced existing child references")
	}
}

func TestSuccessionAppearanceCleanupAndReplacementBoundaries(t *testing.T) {
	s := gomath.NewScene()
	o := gomath.Square(1).SetOpacity(.4)
	other := gomath.Circle(1).Shift(geom.V(2, 0))
	must(t, s.Play(5, gomath.Succession(
		gomath.FadeIn(o).WithRate(gomath.Linear),
		gomath.FadeOut(o).WithRate(gomath.Linear),
		gomath.FadeIn(o).WithRate(gomath.Linear),
		gomath.ReplacementTransform(o, other).WithRate(gomath.Linear),
		gomath.Shift(other, geom.V(2, 0)).WithRate(gomath.Linear),
	)))
	near(t, sample(t, s, .5).Find(o.ID).Style.Opacity, .2)
	near(t, sample(t, s, 1.5).Find(o.ID).Style.Opacity, .2)
	near(t, sample(t, s, 2.5).Find(o.ID).Style.Opacity, .2)
	if sample(t, s, 3.9).Find(other.ID) != nil || sample(t, s, 4).Find(o.ID) != nil {
		t.Fatal("replacement did not happen at its step boundary")
	}
	near(t, sample(t, s, 4.5).Find(other.ID).Center().X, 3)
	near(t, other.Center().X, 4)
	// Authoring holds the actual replacement pointer, ready for a later Play.
	must(t, s.Play(1, gomath.Shift(other, geom.V(1, 0))))
	near(t, other.Center().X, 5)

	s = gomath.NewScene()
	a, b := gomath.Square(1), gomath.Circle(.5)
	parent, sibling := gomath.Group(a), gomath.Square(.25).Shift(geom.V(2, 0))
	parent.Children = append(parent.Children, sibling)
	must(t, s.Add(parent))
	must(t, s.Play(2, gomath.Succession(gomath.ReplacementTransform(a, b), gomath.Shift(b, geom.V(1, 0)))))
	if parent.Children[0] != b || parent.Children[1] != sibling {
		t.Fatal("nested replacement retained old pointers")
	}
}

func TestSuccessionDefersIntroductionAndAllowsParallelSteps(t *testing.T) {
	s := gomath.NewScene()
	a, b := gomath.Square(1), gomath.Circle(1)
	must(t, s.Play(3, gomath.Succession(gomath.FadeIn(a), gomath.FadeOut(a), gomath.FadeIn(b))))
	if sample(t, s, 1.5).Find(b.ID) != nil || sample(t, s, 2).Find(a.ID) != nil {
		t.Fatal("step appearance or cleanup timing is wrong")
	}
	// A parallel step retains the existing independent clock semantics.
	must(t, s.Play(2, gomath.Succession(
		gomath.AnimationGroup(gomath.FadeIn(a), gomath.Shift(b, geom.V(2, 0))),
		gomath.AnimationGroup(gomath.Shift(a, geom.V(1, 0)), gomath.Shift(b, geom.V(1, 0))),
	)))
	near(t, a.Center().X, 1)
	near(t, b.Center().X, 3)
}

func TestSuccessionAtomicLateFailureAndInvalidSchedules(t *testing.T) {
	a, b := gomath.Square(1), gomath.Circle(1)
	badRate := func(float64) float64 { return math.NaN() }
	invalid := []gomath.Animation{
		gomath.Succession(),
		gomath.Succession(gomath.Shift(a, geom.V(1, 0)), gomath.Shift(b, geom.V(1, 0))),
		gomath.Succession(gomath.FadeOut(a), gomath.Shift(a, geom.V(1, 0))),
		gomath.Succession(gomath.Shift(a, geom.V(1, 0)), gomath.Shift(a, geom.V(1, 0)).WithRate(badRate)),
		gomath.Succession(gomath.Shift(a, geom.V(1, 0)), gomath.Scale(a, math.Inf(1))),
		gomath.Succession(gomath.Shift(a, geom.V(1, 0))).WithRate(gomath.Smooth),
		gomath.Succession(gomath.Shift(a, geom.V(1, 0))).WithWindow(.1, 1),
		gomath.AnimationGroup(gomath.Succession(gomath.Shift(a, geom.V(1, 0)))),
		gomath.Succession(gomath.Shift(a, geom.V(1, 0)), gomath.AnimationGroup(gomath.Succession(gomath.Shift(a, geom.V(1, 0))))),
		gomath.Succession(gomath.Shift(a, geom.V(1, 0)).WithDuration(1), gomath.Shift(a, geom.V(1, 0)).WithDuration(math.SmallestNonzeroFloat64)),
	}
	for i, sequence := range invalid {
		s := gomath.NewScene()
		must(t, s.Add(a))
		before, untouched := sample(t, s, 0), b.Copy().Path
		if err := s.Play(2, sequence); err == nil {
			t.Errorf("accepted invalid sequence %d", i)
		}
		if s.Duration() != 0 || !reflect.DeepEqual(before, sample(t, s, 0)) || !reflect.DeepEqual(untouched, b.Path) {
			t.Fatalf("invalid sequence %d partially committed", i)
		}
		near(t, a.Center().X, 0)
	}
	// A different pointer with a copied ID must never be canonicalized silently.
	forged := *a
	s := gomath.NewScene()
	must(t, s.Add(a))
	if s.Play(2, gomath.Succession(gomath.Shift(a, geom.V(2, 0)), gomath.Shift(&forged, geom.V(1, 0)))) == nil {
		t.Fatal("forged input identity accepted")
	}
	near(t, a.Center().X, 0)
	// Tiny absolute steps after an existing large timestamp must fail atomically.
	s = gomath.NewScene()
	must(t, s.Add(a))
	must(t, s.Wait(1e16))
	if s.Play(2, gomath.Succession(gomath.Shift(a, geom.V(2, 0)), gomath.Shift(a, geom.V(1, 0)))) == nil {
		t.Fatal("unrepresentable absolute child boundary accepted")
	}
	near(t, a.Center().X, 0)
}

func TestSuccessionSnapshotReplayAndConcurrentSampling(t *testing.T) {
	s := gomath.NewScene()
	o := gomath.Square(1)
	must(t, s.Add(o))
	must(t, s.Play(2, gomath.Succession(gomath.Shift(o, geom.V(2, 0)), gomath.Rotate(o, math.Pi))))
	replay, err := gomath.CompileReplay(s, gomath.DefaultReplayOptions())
	must(t, err)
	want := sample(t, s, 1.4)
	o.Shift(geom.V(100, 0))
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 10 {
				got, err := replay.Sample(1.4)
				if err != nil || !reflect.DeepEqual(got, want) {
					t.Error("concurrent sequence replay differs", err)
				}
				if !reflect.DeepEqual(sample(t, s, 1.4), want) {
					t.Error("author mutation leaked into sequence")
				}
			}
		}()
	}
	wg.Wait()
}

func TestPinnedManimSuccessionSharedMotion(t *testing.T) {
	data, err := os.ReadFile("tests/data/root/manim-succession-0.22.1.json")
	must(t, err)
	var fixture struct {
		Source struct {
			Version, Commit string
			Hash            string `json:"generator_sha256"`
		}
		Cases []struct {
			Kind   string
			Alpha  float64
			Points [][]float64
		}
	}
	must(t, json.Unmarshal(data, &fixture))
	generator, err := os.ReadFile("tests/reference/succession.py")
	must(t, err)
	hash := sha256.Sum256(generator)
	if fixture.Source.Version != "0.22.1" || fixture.Source.Commit != "16ea443e2a5f1d79db0a721161411ce916d6e733" || fixture.Source.Hash != hex.EncodeToString(hash[:]) || len(fixture.Cases) != 51 {
		t.Fatal("unexpected succession reference provenance/count")
	}
	for _, c := range fixture.Cases {
		s := gomath.NewScene()
		o := gomath.Rectangle(4, 2)
		// Pin the same clockwise line-cubic topology as the independent fixture.
		o.Path.Contours[0].Start = geom.V(-2, -1)
		corners := []geom.Vec3{geom.V(-2, -1), geom.V(2, -1), geom.V(2, 1), geom.V(-2, 1)}
		for i := range 4 {
			a, b := corners[i], corners[(i+1)%4]
			o.Path.Contours[0].Curves[i].P0 = a
			o.Path.Contours[0].Curves[i].P1 = a.Lerp(b, 1.0/3)
			o.Path.Contours[0].Curves[i].P2 = a.Lerp(b, 2.0/3)
			o.Path.Contours[0].Curves[i].P3 = b
		}
		must(t, s.Add(o))
		shift := func(x float64) gomath.Animation { return gomath.Shift(o, geom.V(x, 0)).WithRate(gomath.Linear) }
		var sequence gomath.Animation
		switch c.Kind {
		case "shift":
			sequence = gomath.Succession(shift(2), shift(4).WithDuration(2), gomath.MoveTo(o, geom.V(-2, 0)).WithRate(gomath.Linear))
		case "nested":
			sequence = gomath.Succession(shift(2), gomath.Succession(shift(4), gomath.Scale(o, 2).WithRate(gomath.Linear).WithDuration(3)).WithDuration(2))
		case "rotate":
			sequence = gomath.Succession(shift(3), gomath.Rotate(o, math.Pi).WithRate(gomath.Linear), gomath.Rotate(o, math.Pi).WithRate(gomath.Linear))
		default:
			t.Fatal("unknown reference kind", c.Kind)
		}
		must(t, s.Play(3, sequence))
		got := sample(t, s, 3*c.Alpha).Find(o.ID)
		var points []geom.Vec3
		for _, b := range got.Path.Contours[0].Curves {
			points = append(points, b.P0, b.P1, b.P2, b.P3)
		}
		if len(points) != len(c.Points) {
			t.Fatal("reference topology differs")
		}
		for i, p := range points {
			for j, v := range []float64{p.X, p.Y, p.Z} {
				if math.Abs(v-c.Points[i][j]) > 1e-10 {
					t.Fatalf("%s alpha %g point %d axis %d got %g want %g", c.Kind, c.Alpha, i, j, v, c.Points[i][j])
				}
			}
		}
	}
}
