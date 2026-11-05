package gomath_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"gomath"
	"gomath/geom"
	"math"
	"os"
	"reflect"
	"sync"
	"testing"
)

func TestPinnedManimNestedGroupTiming(t *testing.T) {
	data, err := os.ReadFile("tests/data/root/manim-composition-0.22.1.json")
	must(t, err)
	var fixture struct {
		Source struct {
			Version, Commit string
			Hash            string `json:"generator_sha256"`
		}
		Cases []struct {
			Kind   string
			Alpha  float64
			Values []float64
		}
	}
	must(t, json.Unmarshal(data, &fixture))
	generator, err := os.ReadFile("tests/reference/composition.py")
	must(t, err)
	hash := sha256.Sum256(generator)
	if fixture.Source.Version != "0.22.1" || fixture.Source.Commit != "16ea443e2a5f1d79db0a721161411ce916d6e733" || fixture.Source.Hash != hex.EncodeToString(hash[:]) || len(fixture.Cases) != 44 {
		t.Fatal("unexpected composition provenance/count")
	}
	for _, c := range fixture.Cases {
		t.Run(fmt.Sprintf("%s/%g", c.Kind, c.Alpha), func(t *testing.T) {
			s := gomath.NewScene()
			objects := make([]*gomath.ValueTracker, 3)
			for i, x := range []float64{-2, 1, 4} {
				objects[i], err = gomath.NewValueTracker(x)
				must(t, err)
				must(t, s.Add(objects[i].Object))
			}
			leaf := func(i int, duration float64, rate gomath.RateFunc) gomath.Animation {
				return gomath.ValueTo(objects[i], []float64{2, 7, -2}[i]).WithDuration(duration).WithRate(rate)
			}
			var group gomath.Animation
			switch c.Kind {
			case "parallel":
				group = gomath.AnimationGroup(leaf(0, 2, gomath.Linear), leaf(1, 1, gomath.Linear), leaf(2, .5, gomath.Linear))
			case "long_first":
				group = gomath.LaggedGroup(.1, leaf(0, 10, gomath.Linear), leaf(1, 1, gomath.Linear), leaf(2, 2, gomath.Linear))
			case "nested":
				inner := gomath.LaggedGroup(.2, leaf(0, 3, gomath.Smooth), leaf(1, .5, gomath.Linear)).WithDuration(2).WithRate(gomath.Smooth)
				group = gomath.LaggedGroup(.5, inner, leaf(2, 1, gomath.Linear)).WithDuration(4).WithRate(gomath.Smooth)
			case "returning":
				group = gomath.LaggedGroup(.3, leaf(0, 1, gomath.Linear), leaf(1, 2, gomath.Linear), leaf(2, .5, gomath.Linear)).WithRate(gomath.ThereAndBack)
			default:
				t.Fatal("unexpected group", c.Kind)
			}
			must(t, s.Play(3, group))
			f := sample(t, s, c.Alpha*3)
			for i, want := range c.Values {
				got, err := f.Value(objects[i].ID)
				must(t, err)
				if math.Abs(got-want) > 1e-11 {
					t.Fatalf("tracker %d: got %.17g want %.17g", i, got, want)
				}
			}
		})
	}
}

func TestGroupWindowsIsolationAndReplay(t *testing.T) {
	s := gomath.NewScene()
	o := gomath.Square(1)
	must(t, s.Add(o))
	children := []gomath.Animation{gomath.Shift(o, geom.V(4, 0)).WithWindow(.25, .75).WithRate(gomath.Linear)}
	group := gomath.AnimationGroup(children...).WithWindow(.2, .8).WithRate(gomath.EaseInQuad)
	children[0] = gomath.Animation{}
	must(t, s.Play(2, group))
	for _, a := range []float64{0, .1, .2, .35, .5, .65, .8, .9, 1} {
		clock := math.Max(0, math.Min(1, (a-.2)/.6))
		clock *= clock
		want := 4 * math.Max(0, math.Min(1, (clock-.25)/.5))
		near(t, sample(t, s, 2*a).Find(o.ID).Center().X, want)
	}
	replay, err := gomath.CompileReplay(s, gomath.DefaultReplayOptions())
	must(t, err)
	want := sample(t, s, .91)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 10 {
				got, err := replay.Sample(.91)
				if err != nil || !reflect.DeepEqual(got, want) {
					t.Error("concurrent group replay differs", err)
				}
			}
		}()
	}
	wg.Wait()
}

func TestGroupInvalidInputIsAtomic(t *testing.T) {
	o := gomath.Square(1)
	leaf := gomath.Shift(o, geom.V(2, 0))
	deep := leaf
	for range 66 {
		deep = gomath.AnimationGroup(deep)
	}
	tooMany := make([]gomath.Animation, 10001)
	for i := range tooMany {
		tooMany[i] = leaf
	}
	invalid := []gomath.Animation{
		gomath.AnimationGroup(), gomath.LaggedGroup(-.1, leaf), gomath.LaggedGroup(math.NaN(), leaf),
		gomath.AnimationGroup(leaf.WithDuration(0)), leaf.WithDuration(math.Inf(1)),
		gomath.AnimationGroup(leaf).WithWindow(.7, .2), deep, gomath.AnimationGroup(tooMany...),
		gomath.LaggedGroup(0, leaf.WithDuration(math.MaxFloat64), leaf.WithDuration(math.SmallestNonzeroFloat64)),
		gomath.AnimationGroup(leaf, leaf), gomath.AnimationGroup(leaf).WithRate(func(float64) float64 { return math.NaN() }),
		gomath.Shift(nil, geom.V(1, 0)), gomath.MoveTo(nil, geom.V(1, 0)), gomath.Rotate(nil, 1), gomath.Scale(nil, 2),
	}
	for i, a := range invalid {
		s := gomath.NewScene()
		must(t, s.Add(o))
		if err := s.Play(1, a); err == nil {
			t.Errorf("accepted invalid group %d", i)
		}
		near(t, s.Duration(), 0)
		near(t, o.Center().X, 0)
	}
	s := gomath.NewScene()
	must(t, s.Add(o))
	unsafe := func(a float64) float64 {
		if a > .2 && a < .8 {
			return math.NaN()
		}
		return a
	}
	must(t, s.Play(1, gomath.AnimationGroup(leaf).WithRate(unsafe)))
	if _, err := s.Sample(.5); err == nil {
		t.Fatal("nonfinite group clock ignored")
	}
}
