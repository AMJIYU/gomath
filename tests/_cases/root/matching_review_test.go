package gomath_test

import (
	"encoding/json"
	"gomath"
	"gomath/geom"
	"gomath/render"
	"math"
	"reflect"
	"strings"
	"testing"
)

func matchingReviewState(t *testing.T, o *gomath.Object) string {
	t.Helper()
	b, err := json.Marshal(o)
	must(t, err)
	return string(b)
}

func matchingReviewIDs(t *testing.T, f *gomath.Frame) map[uint64]int {
	t.Helper()
	ids := map[uint64]int{}
	for _, root := range f.Objects {
		must(t, root.Validate())
		root.Walk(func(o *gomath.Object) {
			ids[o.ID]++
			if o.ID == 0 || ids[o.ID] != 1 {
				t.Errorf("time %g: duplicated/zero frame ID %d", f.Time, o.ID)
			}
		})
	}
	return ids
}

// Unlike the reference's drawable multiset comparison, check whole-frame IDs
// with two private bucket trees, a nested replacement and an ordinary sibling
// track. In particular the two-child source can have two bucket parents too.
func TestMatchingReviewParallelNestedBuckets(t *testing.T) {
	var (
		a       = gomath.Group(gomath.Square(.2), gomath.Group(gomath.Square(.3)))
		b       = gomath.Group(gomath.Group(gomath.Square(.4)), gomath.Square(.2), gomath.Square(.3))
		c       = gomath.Group(gomath.Square(.2), gomath.Square(.3), gomath.Square(.4))
		d       = gomath.Group(gomath.Square(.3))
		sibling = gomath.Square(.2).Shift(geom.V(0, 3))
		inner   = gomath.Group(c, sibling)
		parent  = gomath.Group(a, inner)
		s       = gomath.NewScene()
	)
	beforeA, beforeC := matchingReviewState(t, a), matchingReviewState(t, c)
	childrenA, childrenC := append([]*gomath.Object(nil), a.Children...), append([]*gomath.Object(nil), c.Children...)
	ab, err := gomath.TransformMatchingParts(a, b, []string{"x", "y"}, []string{"y", "x", "x"})
	must(t, err)
	cd, err := gomath.TransformMatchingParts(c, d, []string{"z", "z", "lost"}, []string{"z"})
	must(t, err)
	must(t, s.Add(parent))
	must(t, s.Play(1, gomath.AnimationGroup(
		ab.WithRate(gomath.Linear),
		gomath.AnimationGroup(cd.WithRate(gomath.Linear), gomath.Shift(sibling, geom.V(2, 0)).WithRate(gomath.Linear)),
	)))
	if parent.Children[0] != b || parent.Children[1] != inner || inner.Children[0] != d || inner.Children[1] != sibling {
		t.Fatal("parallel cleanup lost actual target/sibling pointers or tree slots")
	}
	if matchingReviewState(t, a) != beforeA || matchingReviewState(t, c) != beforeC || !reflect.DeepEqual(a.Children, childrenA) || !reflect.DeepEqual(c.Children, childrenC) {
		t.Fatal("parallel cleanup changed a removed source")
	}
	replay, err := gomath.CompileReplay(s, gomath.DefaultReplayOptions())
	must(t, err)
	baseline := sample(t, s, .5)
	for _, time := range []float64{0, .5, .99, 1, .125, .5} {
		f := sample(t, s, time)
		ids := matchingReviewIDs(t, f)
		if time < 1 {
			for _, source := range []*gomath.Object{a, c} {
				source.Walk(func(o *gomath.Object) {
					if ids[o.ID] != 1 {
						t.Errorf("time %g: source ID %d lost inside buckets", time, o.ID)
					}
				})
			}
			for _, target := range []*gomath.Object{b, d} {
				target.Walk(func(o *gomath.Object) {
					if ids[o.ID] != 0 {
						t.Errorf("time %g: target ID %d exposed before cleanup", time, o.ID)
					}
				})
			}
		} else if ids[a.ID] != 0 || ids[c.ID] != 0 || ids[b.ID] != 1 || ids[d.ID] != 1 {
			t.Fatal("wrong parallel endpoint IDs")
		}
		near(t, f.Find(sibling.ID).Center().X, 2*time)
		frozen, err := replay.Sample(time)
		must(t, err)
		if !reflect.DeepEqual(frozen, f) || time == .5 && !reflect.DeepEqual(f, baseline) {
			t.Fatal("bucket IDs/topology changed across sampling or replay")
		}
		f.Objects[0].Shift(geom.V(50, 0))
	}
}

func TestMatchingReviewSuccessionReintroducesSource(t *testing.T) {
	var (
		a = gomath.Group(gomath.Square(.2), gomath.Square(.3).Shift(geom.V(2, 0)))
		b = gomath.Group(gomath.Square(.4).Shift(geom.V(4, 0)))
		c = gomath.Group(gomath.Square(.5).Shift(geom.V(6, 0)))
		s = gomath.NewScene()
	)
	first, err := gomath.TransformMatchingParts(a, b, []string{"x", "x"}, []string{"x"})
	must(t, err)
	second, err := gomath.TransformMatchingParts(a, c, []string{"x", "x"}, []string{"x"})
	must(t, err)
	a0 := a.Children[0]
	must(t, s.Add(a))
	must(t, s.Play(4, gomath.Succession(
		first.WithRate(gomath.Linear), gomath.FadeIn(a).WithRate(gomath.Linear),
		gomath.AnimationGroup(gomath.Shift(a0, geom.V(1, 0)).WithRate(gomath.Linear), gomath.Shift(b.Children[0], geom.V(2, 0)).WithRate(gomath.Linear)),
		second.WithRate(gomath.Linear),
	)))
	if a.Children[0] != a0 || len(a.Children) != 2 {
		t.Fatal("reintroduced source lost its original direct child handles")
	}
	near(t, a0.Center().X, 1)
	near(t, a.Children[1].Center().X, 2)
	for _, time := range []float64{0, .5, 1, 1.5, 2, 2.5, 3, 3.5, 4} {
		f := sample(t, s, time)
		ids := matchingReviewIDs(t, f)
		if time < 1 && ids[b.ID] != 0 || time >= 1 && ids[b.ID] != 1 || time < 4 && ids[c.ID] != 0 || time == 4 && (ids[c.ID] != 1 || ids[a.ID] != 0) {
			t.Fatalf("time %g: wrong sequence cleanup/introduction", time)
		}
		if time >= 1 && time < 3 {
			near(t, f.Find(a0.ID).Center().X, math.Max(0, time-2))
		}
		if time < 1 {
			// The first animation must still capture the original pre-reuse state.
			near(t, f.Find(a0.ID).Center().X, 4*time)
		}
	}
}

func TestMatchingReviewManagedBucketReplay(t *testing.T) {
	for _, automatic := range []bool{true, false} {
		t.Run(map[bool]string{true: "suspended", false: "active"}[automatic], func(t *testing.T) {
			var (
				source = gomath.Group(gomath.Square(.2))
				target = gomath.Group(gomath.Square(.2).Shift(geom.V(2, 0)), gomath.Square(.2).Shift(geom.V(4, 0)))
				parent = gomath.Group(source)
				s      = gomath.NewScene()
			)
			must(t, s.Add(parent))
			must(t, s.Wait(.25))
			options := gomath.DefaultObjectUpdaterOptions()
			options.SuspendDuringAnimations = automatic
			rootHandle := managedUpdater(t, s, source, func(o *gomath.Object, _ *gomath.Frame, _ float64) error { o.Scalar = 11; return nil }, options)
			childHandle := managedUpdater(t, s, source.Children[0], func(o *gomath.Object, _ *gomath.Frame, _ float64) error { o.Scalar = 22; return nil }, options)
			parentHandle := managedUpdater(t, s, parent, func(o *gomath.Object, _ *gomath.Frame, _ float64) error { o.Scalar = 33; return nil }, options)
			animation, err := gomath.TransformMatchingParts(source, target, []string{"x"}, []string{"x", "x"})
			must(t, err)
			must(t, s.Play(1, animation.WithRate(gomath.Linear).WithWindow(.25, .75)))
			targetHandle := managedUpdater(t, s, target.Children[0], func(o *gomath.Object, _ *gomath.Frame, _ float64) error { o.Scalar = 44; return nil }, options)
			must(t, s.Wait(.25))
			must(t, s.Add(source))
			must(t, s.Wait(.5))
			replay, err := gomath.CompileReplay(s, gomath.DefaultReplayOptions())
			must(t, err)
			times := []float64{.25, .4, .75, 1.1, 1.25, 1.5, 2}
			baseline := make([]*gomath.Frame, len(times))
			for i, time := range times {
				f := sample(t, s, time)
				matchingReviewIDs(t, f)
				near(t, f.Find(parent.ID).Scalar, 33)
				if time < 1.25 || time >= 1.5 {
					wantRoot, wantChild := 11., 22.
					if automatic && time < 1.25 {
						wantRoot, wantChild = 0, 0
					}
					near(t, f.Find(source.ID).Scalar, wantRoot)
					near(t, f.Find(source.Children[0].ID).Scalar, wantChild)
				}
				if time >= 1.25 {
					near(t, f.Find(target.Children[0].ID).Scalar, 44)
				}
				baseline[i] = f
			}
			for _, h := range []*gomath.UpdaterHandle{rootHandle, childHandle, parentHandle, targetHandle} {
				must(t, s.RemoveUpdater(h))
			}
			source.Shift(geom.V(100, 0))
			target.Shift(geom.V(200, 0))
			must(t, s.Wait(.25))
			for i := len(times) - 1; i >= 0; i-- {
				f, err := replay.Sample(times[i])
				must(t, err)
				if !reflect.DeepEqual(f, baseline[i]) {
					t.Fatalf("time %g: frozen matching/updater lifecycle changed", times[i])
				}
				f.Objects[0].Shift(geom.V(500, 0))
			}
		})
	}
}

// Hand-derived compositing oracle: matched buckets paint first (including an
// interleaved repeated key), then unmatched source green, then incoming white.
// The existing reference compares sets and cannot catch swapping fade layers.
func TestMatchingReviewPaintOrder(t *testing.T) {
	part := func(c gomath.Color) *gomath.Object {
		return gomath.Square(2).SetFill(c).SetStroke(gomath.Transparent, 0)
	}
	var (
		source = gomath.Group(part(gomath.RGB(1, 0, 0)), part(gomath.RGB(0, 1, 0)), part(gomath.RGB(0, 0, 1)), part(gomath.RGB(1, 1, 0)))
		target = gomath.Group(part(gomath.RGB(0, 0, 1)), part(gomath.RGB(1, 0, 0)), part(gomath.RGB(1, 1, 0)), part(gomath.White))
		s      = gomath.NewScene()
	)
	a, err := gomath.TransformMatchingParts(source, target, []string{"a", "out", "b", "a"}, []string{"b", "a", "a", "in"})
	must(t, err)
	must(t, s.Add(source))
	must(t, s.Play(1, a.WithRate(gomath.Linear)))
	cfg := render.DefaultConfig()
	cfg.Width, cfg.Height, cfg.FrameWidth, cfg.Background = 32, 32, 4, gomath.Black
	r, err := render.NewRasterizer(cfg)
	must(t, err)
	for _, time := range []float64{.25, .5, .75} {
		img, err := r.Render(sample(t, s, time))
		must(t, err)
		pixel := img.RGBAAt(16, 16)
		// Blue under green (1-t), under white t, on an opaque background.
		want := []float64{time, (1-time)*(1-time) + time, time*(1-time) + time}
		for i, got := range []uint8{pixel.R, pixel.G, pixel.B} {
			if math.Abs(float64(got)-255*want[i]) > 2 {
				t.Fatalf("time %g: painted %v, want RGB fractions %v", time, pixel, want)
			}
		}
	}
}

func TestMatchingReviewFailureAtomicity(t *testing.T) {
	for _, mode := range []string{"window", "arc", "rate", "late-sequence-rate"} {
		t.Run(mode, func(t *testing.T) {
			var (
				source  = gomath.Group(gomath.Square(.2), gomath.Square(.3))
				target  = gomath.Group(gomath.Square(.4))
				sibling = gomath.Square(.5)
				parent  = gomath.Group(source, sibling)
				s       = gomath.NewScene()
			)
			a, err := gomath.TransformMatchingParts(source, target, []string{"x", "x"}, []string{"x"})
			must(t, err)
			must(t, s.Add(parent))
			must(t, s.Wait(.25))
			beforeParent, beforeTarget := matchingReviewState(t, parent), matchingReviewState(t, target)
			before := sample(t, s, .25)
			badRate := func(float64) float64 { return math.NaN() }
			switch mode {
			case "window":
				a = a.WithWindow(.8, .2)
			case "arc":
				a = a.WithPathArc(math.Pi).WithPathArcAxis(geom.Vec3{X: math.Inf(1)})
			case "rate":
				a = a.WithRate(badRate)
			case "late-sequence-rate":
				a = gomath.Succession(a, gomath.Shift(target, geom.V(1, 0)).WithRate(badRate))
			}
			animations := []gomath.Animation{gomath.Shift(sibling, geom.V(5, 0)), a}
			if mode == "late-sequence-rate" {
				animations = []gomath.Animation{a}
			}
			if err := s.Play(1, animations...); err == nil {
				t.Fatal("invalid matching operation accepted")
			}
			if s.Duration() != .25 || matchingReviewState(t, parent) != beforeParent || matchingReviewState(t, target) != beforeTarget || !reflect.DeepEqual(before, sample(t, s, .25)) || parent.Children[0] != source {
				t.Fatal("failure published authoring geometry/topology or a timeline segment")
			}
		})
	}
}

func TestMatchingReviewInteriorRateFailureIsIsolated(t *testing.T) {
	for _, invalid := range []float64{math.NaN(), 1.2, -.2} {
		var (
			source  = gomath.Group(gomath.Square(.2))
			target  = gomath.Group(gomath.Square(.2).Shift(geom.V(4, 0)), gomath.Square(.2).Shift(geom.V(6, 0)))
			sibling = gomath.Square(.3)
			s       = gomath.NewScene()
		)
		a, err := gomath.TransformMatchingParts(source, target, []string{"x"}, []string{"x", "x"})
		must(t, err)
		rate := func(alpha float64) float64 {
			if alpha > .25 && alpha < .75 {
				return invalid
			}
			return alpha
		}
		must(t, s.Add(source, sibling))
		must(t, s.Play(1, gomath.Shift(sibling, geom.V(3, 0)).WithRate(gomath.Linear), a.WithRate(rate).WithPathArc(.7)))
		replay, err := gomath.CompileReplay(s, gomath.DefaultReplayOptions())
		must(t, err)
		beforeSource, beforeTarget := matchingReviewState(t, source), matchingReviewState(t, target)
		before := sample(t, s, .125)
		for _, sampleFn := range []func(float64) (*gomath.Frame, error){s.Sample, replay.Sample} {
			if f, err := sampleFn(.5); err == nil || f != nil {
				t.Fatalf("rate %g: invalid matching sample published a partial frame: %v", invalid, err)
			}
			f, err := sampleFn(.125)
			must(t, err)
			if !reflect.DeepEqual(f, before) || matchingReviewState(t, source) != beforeSource || matchingReviewState(t, target) != beforeTarget {
				t.Fatal("failed matching sample contaminated later sampling or authoring")
			}
		}
	}
}

// The pinned grow/shrink fixtures cover 1-to-N and N-to-1. A 2-to-3 bucket
// distinguishes floor(j*old/total) repetition from appending a trailing copy.
func TestMatchingReviewRepeatDistribution(t *testing.T) {
	var (
		source = gomath.Group(gomath.Square(.2), gomath.Square(.2).Shift(geom.V(6, 0)))
		target = gomath.Group(gomath.Square(.2).Shift(geom.V(10, 0)), gomath.Square(.2).Shift(geom.V(20, 0)), gomath.Square(.2).Shift(geom.V(30, 0)))
		s      = gomath.NewScene()
	)
	a, err := gomath.TransformMatchingParts(source, target, []string{"x", "x"}, []string{"x", "x", "x"})
	must(t, err)
	must(t, s.Add(source))
	must(t, s.Play(1, a.WithRate(gomath.Linear)))
	f := sample(t, s, .5)
	matchingReviewIDs(t, f)
	var leaves []*gomath.Object
	f.Objects[0].Walk(func(o *gomath.Object) {
		if len(o.Path.Contours) > 0 {
			leaves = append(leaves, o)
		}
	})
	if len(leaves) != 3 || leaves[0].ID != source.Children[0].ID || leaves[2].ID != source.Children[1].ID {
		t.Fatal("2-to-3 repetition lost the originals' ordered bucket positions")
	}
	for i, want := range []float64{5, 10, 18} {
		near(t, leaves[i].Center().X, want)
		wantOpacity := 1.
		if i == 1 {
			wantOpacity = .5
		}
		near(t, leaves[i].Style.Opacity, wantOpacity)
	}
}

// Existing expansion coverage exhausts nodes first. Here only the repeated
// curve total exceeds its budget (516*128), with just 131 input tree nodes.
func TestMatchingReviewRepeatedCurveBudget(t *testing.T) {
	var (
		part   = gomath.Square(.1)
		source = gomath.Group(part)
		target = gomath.Group()
		keys   = make([]string, 128)
		s      = gomath.NewScene()
	)
	contour := part.Path.Contours[0]
	for range 128 {
		part.Path.Contours = append(part.Path.Contours, contour)
	}
	for i := range keys {
		keys[i] = "x"
		target.Children = append(target.Children, gomath.Square(.1))
	}
	a, err := gomath.TransformMatchingParts(source, target, []string{"x"}, keys)
	must(t, err)
	must(t, s.Add(source))
	beforeSource, beforeTarget := matchingReviewState(t, source), matchingReviewState(t, target)
	before := sample(t, s, 0)
	if err := s.Play(1, a); err == nil || !strings.Contains(err.Error(), "aggregate node/contour/curve budget") {
		t.Fatalf("repeated curves were not rejected by the pre-copy budget: %v", err)
	}
	if s.Duration() != 0 || matchingReviewState(t, source) != beforeSource || matchingReviewState(t, target) != beforeTarget || !reflect.DeepEqual(before, sample(t, s, 0)) {
		t.Fatal("curve-budget rejection changed authoring/timeline")
	}
}
