package gomath_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"gomath"
	"gomath/geom"
	"gomath/path"
	"math"
	"os"
	"reflect"
	"sync"
	"testing"
)

type matchingReferencePart struct {
	Name   string
	Points [][3]float64
	Color  [3]float64
	Fill   [3]float64
	Alpha  float64
}

func matchingReferenceGroup(parts []matchingReferencePart) *gomath.Object {
	root := gomath.Group()
	for _, part := range parts {
		vec := func(p [3]float64) geom.Vec3 { return geom.Vec3{X: p[0], Y: p[1], Z: p[2]} }
		c := path.Contour{Start: vec(part.Points[0]), Closed: true}
		for i := 0; i < len(part.Points); i += 4 {
			c.Curves = append(c.Curves, path.Cubic{P0: vec(part.Points[i]), P1: vec(part.Points[i+1]), P2: vec(part.Points[i+2]), P3: vec(part.Points[i+3])})
		}
		o := gomath.NewObject(path.Path{Contours: []path.Contour{c}}).SetFill(gomath.RGB(part.Color[0], part.Color[1], part.Color[2])).SetStroke(gomath.Transparent, 0)
		o.Name = part.Name
		root.Children = append(root.Children, o)
	}
	return root
}

func matchingCapture(root *gomath.Object) []matchingReferencePart {
	var result []matchingReferencePart
	var visit func(*gomath.Object, float64)
	visit = func(o *gomath.Object, alpha float64) {
		alpha *= o.Style.Opacity
		if len(o.Path.Contours) > 0 {
			p := matchingReferencePart{Name: o.Name, Alpha: alpha * o.Style.Fill.A, Fill: [3]float64{o.Style.Fill.R, o.Style.Fill.G, o.Style.Fill.B}}
			for _, c := range o.Path.Contours {
				for _, q := range c.Curves {
					for _, v := range [4]geom.Vec3{q.P0, q.P1, q.P2, q.P3} {
						p.Points = append(p.Points, [3]float64{v.X, v.Y, v.Z})
					}
				}
			}
			result = append(result, p)
		}
		for _, child := range o.Children {
			visit(child, alpha)
		}
	}
	visit(root, 1)
	return result
}

func TestPinnedManimMatchingParts(t *testing.T) {
	var fixture struct {
		Source struct {
			Version, Commit string
			Hash            string `json:"generator_sha256"`
			Files           map[string]string
		}
		Cases []struct {
			Name           string
			Arc            float64
			SK             []string `json:"source_keys"`
			TK             []string `json:"target_keys"`
			Source, Target []matchingReferencePart
			States         []struct {
				Alpha   float64
				Objects []matchingReferencePart
			}
		}
	}
	data, e := os.ReadFile("tests/data/root/matching-parts/manim-0.22.1.json")
	must(t, e)
	must(t, json.Unmarshal(data, &fixture))
	script, e := os.ReadFile("tests/reference/matching_parts.py")
	must(t, e)
	hash := sha256.Sum256(script)
	if fixture.Source.Version != "0.22.1" || fixture.Source.Commit != "16ea443e2a5f1d79db0a721161411ce916d6e733" || fixture.Source.Hash != hex.EncodeToString(hash[:]) || len(fixture.Source.Files) != 9 || len(fixture.Cases) != 9 {
		t.Fatal("matching reference provenance mismatch")
	}
	for _, c := range fixture.Cases {
		t.Run(c.Name, func(t *testing.T) {
			a, b := matchingReferenceGroup(c.Source), matchingReferenceGroup(c.Target)
			before := a.Copy()
			children := append([]*gomath.Object(nil), a.Children...)
			animation, e := gomath.TransformMatchingParts(a, b, c.SK, c.TK)
			must(t, e)
			if c.Arc != 0 {
				animation = animation.WithPathArc(c.Arc)
			}
			s := gomath.NewScene()
			must(t, s.Add(a))
			must(t, s.Play(1, animation.WithRate(gomath.Linear)))
			for _, state := range c.States {
				f := sample(t, s, state.Alpha)
				must(t, f.Objects[0].Validate())
				got := matchingCapture(f.Objects[0])
				if len(got) != len(state.Objects) {
					t.Fatalf("alpha %g: got%d drawable nodes want%d", state.Alpha, len(got), len(state.Objects))
				}
				used := make([]bool, len(got))
				for _, want := range state.Objects {
					found := false
					for j, part := range got {
						if used[j] || part.Name != want.Name || len(part.Points) != len(want.Points) || math.Abs(part.Alpha-want.Alpha) > 1e-11 {
							continue
						}
						maximum := 0.0
						for k := range 3 {
							maximum = max(maximum, math.Abs(part.Fill[k]-want.Fill[k]))
						}
						for i, p := range part.Points {
							for k := range 3 {
								maximum = max(maximum, math.Abs(p[k]-want.Points[i][k]))
							}
						}
						if maximum < 1e-10 {
							used[j], found = true, true
							break
						}
					}
					if !found {
						t.Fatalf("alpha %g: no matching complete geometry/color state for %s", state.Alpha, want.Name)
					}
				}
			}
			if len(children) != len(a.Children) || !reflect.DeepEqual(matchingCapture(a), matchingCapture(before)) {
				t.Fatal("removed source was modified")
			}
			for i, p := range children {
				if a.Children[i] != p {
					t.Fatal("source child pointer replaced")
				}
			}
			if f := sample(t, s, 1); len(f.Objects) != 1 || f.Objects[0].ID != b.ID {
				t.Fatal("actual target missing at cleanup")
			}
			must(t, s.Play(1, gomath.Shift(b, geom.V(1, 0))))
		})
	}
}

func TestMatchingPartsOwnershipTimingAtomicity(t *testing.T) {
	makePair := func() (*gomath.Object, *gomath.Object) {
		return gomath.Group(gomath.Square(1), gomath.Square(.5).Shift(geom.V(1, 0))), gomath.Group(gomath.Square(.5).Shift(geom.V(-1, 1)), gomath.Square(1).Shift(geom.V(1, 1)))
	}
	a, b := makePair()
	keys := []string{"a", "b"}
	anim, e := gomath.TransformMatchingParts(a, b, keys, []string{"b", "a"})
	must(t, e)
	keys[0] = "modified caller key"
	container := gomath.Group(a, gomath.Dot(geom.V(-2, 0), .1))
	s := gomath.NewScene()
	must(t, s.Add(container))
	must(t, s.Play(1, gomath.AnimationGroup(anim.WithWindow(.2, .8))))
	if container.Children[0] != b {
		t.Fatal("nested replacement lost actual target identity")
	}
	for _, time := range []float64{0, .1, .5, .9, 1} {
		must(t, sample(t, s, time).Objects[0].Validate())
	}
	// Replay captures immutable matching tracks, and the removed source may be
	// re-added without contaminating the captured earlier segment.
	r, e := gomath.CompileReplay(s, gomath.DefaultReplayOptions())
	must(t, e)
	baseline, e := r.Sample(.5)
	must(t, e)
	must(t, s.Add(a))
	a.Shift(geom.V(100, 0))
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			f, err := r.Sample(.5)
			if err != nil {
				t.Error(err)
				return
			}
			if !reflect.DeepEqual(f, baseline) {
				t.Error("matching replay snapshots alias authoring/other samples")
			}
			f.Objects[0].Shift(geom.V(5, 0))
		})
	}
	wg.Wait()
	// A late failure in Succession must restore every original input.
	a, b = makePair()
	anim, e = gomath.TransformMatchingParts(a, b, []string{"a", "b"}, []string{"b", "a"})
	must(t, e)
	s = gomath.NewScene()
	must(t, s.Add(a))
	savedA, savedB := matchingCapture(a), matchingCapture(b)
	if e := s.Play(2, gomath.Succession(anim, gomath.Scale(b, math.Inf(1)))); e == nil {
		t.Fatal("invalid late step accepted")
	}
	if s.Duration() != 0 || !reflect.DeepEqual(savedA, matchingCapture(a)) || !reflect.DeepEqual(savedB, matchingCapture(b)) {
		t.Fatal("failed sequence changed inputs")
	}
	must(t, s.Play(2, gomath.Succession(anim, gomath.Shift(b, geom.V(1, 0)))))
	if sample(t, s, 2).Objects[0].ID != b.ID {
		t.Fatal("sequence did not retain matching replacement target")
	}
}

func TestMatchingPartsStrictErrorsAndIndependentModes(t *testing.T) {
	a := gomath.Group(gomath.Square(1))
	b := gomath.Group(gomath.Square(1).Shift(geom.V(2, 0)))
	for _, keys := range [][]string{nil, {"a", "b"}, {string([]byte{0xff})}, {string(make([]byte, gomath.MaxMatchingKeyBytes+1))}} {
		if _, err := gomath.TransformMatchingParts(a, b, keys, []string{"a"}); err == nil {
			t.Fatal("invalid keys accepted")
		}
	}
	if _, err := gomath.TransformMatchingParts(nil, b, nil, []string{"a"}); err == nil {
		t.Fatal("nil source accepted")
	}
	if _, err := gomath.TransformMatchingParts(a, a, []string{"a"}, []string{"a"}); err == nil {
		t.Fatal("shared source/target accepted")
	}
	for _, mutate := range []func(){
		func() { a.Style.Opacity = .5 },
		func() { a.Path = gomath.Square(1).Path },
		func() { a.Children[0].Path.Contours[0].Start.X = math.Inf(1) },
	} {
		a = gomath.Group(gomath.Square(1))
		mutate()
		if _, err := gomath.TransformMatchingParts(a, b, []string{"a"}, []string{"a"}); err == nil {
			t.Fatal("unsupported/invalid input accepted")
		}
	}
	// Captured identities reject reordered/replaced direct children at Play.
	a = gomath.Group(gomath.Square(1))
	anim, e := gomath.TransformMatchingParts(a, b, []string{"a"}, []string{"a"})
	must(t, e)
	a.Children[0] = gomath.Square(1)
	s := gomath.NewScene()
	must(t, s.Add(a))
	if s.Play(1, anim) == nil || s.Duration() != 0 {
		t.Fatal("changed captured child identity accepted")
	}
	// Empty roots are valid; unsupported clocks fail atomically before cleanup.
	a, b = gomath.Group(), gomath.Group()
	anim, e = gomath.TransformMatchingParts(a, b, nil, nil)
	must(t, e)
	s = gomath.NewScene()
	must(t, s.Add(a))
	if s.Play(1, anim.WithRate(func(float64) float64 { return 1.2 })) == nil || s.Duration() != 0 {
		t.Fatal("fade extrapolation accepted")
	}
	must(t, s.Play(1, anim.WithRate(gomath.Linear)))
}

func TestMatchingPartsBudgetsAndSuspension(t *testing.T) {
	large := gomath.Group()
	for range 100 {
		large.Children = append(large.Children, gomath.Square(.1))
	}
	source, target := gomath.Group(large), gomath.Group()
	keys := make([]string, gomath.MaxMatchingParts)
	for i := range keys {
		keys[i] = "x"
		target.Children = append(target.Children, gomath.Square(.1))
	}
	anim, e := gomath.TransformMatchingParts(source, target, []string{"x"}, keys)
	must(t, e)
	s := gomath.NewScene()
	must(t, s.Add(source))
	before := matchingCapture(source)
	if s.Play(1, anim) == nil || s.Duration() != 0 || !reflect.DeepEqual(before, matchingCapture(source)) {
		t.Fatal("repetition expansion escaped pre-copy budget or failed nonatomically")
	}
	wide := gomath.Group()
	wide.Children = make([]*gomath.Object, 8193)
	if _, e := gomath.TransformMatchingParts(gomath.Group(wide), gomath.Group(gomath.Square(1)), []string{"x"}, []string{"x"}); e == nil {
		t.Fatal("pending-child budget not enforced")
	}
	cyclic := gomath.Group()
	cyclic.Children = []*gomath.Object{cyclic}
	if _, e := gomath.TransformMatchingParts(cyclic, gomath.Group(gomath.Square(1)), []string{"x"}, []string{"x"}); e == nil {
		t.Fatal("cycle accepted")
	}
	// Managed source handles remain findable but are automatically suspended
	// during keyed animation. Target handles become active after cleanup.
	source = gomath.Group(gomath.Square(1))
	target = gomath.Group(gomath.Square(1).Shift(geom.V(2, 0)))
	s = gomath.NewScene()
	must(t, s.Add(source))
	must(t, s.Wait(.1))
	_, e = s.AddObjectUpdater(source.Children[0], func(o *gomath.Object, _ *gomath.Frame, _ float64) error {
		o.MoveTo(geom.V(50, 0))
		return nil
	}, gomath.DefaultObjectUpdaterOptions())
	must(t, e)
	anim, e = gomath.TransformMatchingParts(source, target, []string{"x"}, []string{"x"})
	must(t, e)
	must(t, s.Play(1, anim.WithRate(gomath.Linear)))
	f := sample(t, s, .6)
	if got := f.Find(source.Children[0].ID); got == nil || math.Abs(got.Center().X-1) > 1e-10 {
		t.Fatal("source child identity/suspension lost through private buckets")
	}
	// The target is actually owned now: registering on its live handle works.
	_, e = s.AddObjectUpdater(target.Children[0], func(o *gomath.Object, _ *gomath.Frame, _ float64) error {
		o.MoveTo(geom.V(3, 0))
		return nil
	}, gomath.DefaultObjectUpdaterOptions())
	must(t, e)
	must(t, s.Wait(.2))
	if sample(t, s, 1.2).Find(target.Children[0].ID).Center().X != 3 {
		t.Fatal("target live updater did not run")
	}
}

func BenchmarkMatchingFrame(b *testing.B) {
	var from, to []*gomath.Object
	var keys, reversed []string
	for i := range 32 {
		from = append(from, gomath.Square(.1).Shift(geom.V(float64(i)*.2, -.5)))
		to = append(to, gomath.Square(.1).Shift(geom.V(float64(i)*.2, .5)))
		keys = append(keys, string(rune('A'+i)))
		reversed = append(reversed, string(rune('A'+31-i)))
	}
	s := gomath.NewScene()
	root := gomath.Group(from...)
	target := gomath.Group(to...)
	a, e := gomath.TransformMatchingParts(root, target, keys, reversed)
	if e != nil {
		b.Fatal(e)
	}
	if e := s.Add(root); e != nil {
		b.Fatal(e)
	}
	if e := s.Play(1, a.WithRate(gomath.Linear)); e != nil {
		b.Fatal(e)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, e := s.Sample(.5); e != nil {
			b.Fatal(e)
		}
	}
}
