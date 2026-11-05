package gomath_test

import (
	"gomath"
	"gomath/geom"
	"gomath/path"
	"math"
	"reflect"
	"sync"
	"testing"
)

func testClip(t *testing.T, object *gomath.Object) *gomath.Clip {
	t.Helper()
	clip, err := gomath.NewClip(object.Path)
	must(t, err)
	return clip
}

func TestClipOwnershipValidationAndTransforms(t *testing.T) {
	outline := gomath.Square(2).Path
	want := outline.Clone()
	clip, err := gomath.NewClip(outline)
	must(t, err)
	outline.Contours[0].Start.X = 9
	got := clip.Path()
	got.Contours[0].Curves[0].P1.X = 19
	if !reflect.DeepEqual(clip.Path(), want) {
		t.Fatal("clip storage aliases input or exported path")
	}
	o := gomath.Square(4).SetClip(clip)
	copy := o.Copy()
	if copy.Clip != clip || copy.ID == o.ID {
		t.Fatal("copy did not share only immutable clip")
	}
	copy.Shift(geom.V(3, 2))
	near(t, copy.Clip.Bounds().Center().X, 3)
	near(t, copy.Clip.Bounds().Center().Y, 2)
	near(t, clip.Bounds().Center().X, 0)
	if copy.Clip == clip {
		t.Fatal("geometry transform retained old world-space clip")
	}
	projected, err := clip.Transform(geom.About(geom.V(1, 0), geom.RotationZ(math.Pi/2)))
	must(t, err)
	near(t, projected.Bounds().Center().X, 1)
	near(t, projected.Bounds().Center().Y, -1)
	copy.SetClip(nil)
	if copy.Clip != nil || o.Clip != clip {
		t.Fatal("removing clip changed shared source")
	}
	if (&gomath.Clip{}).Validate() == nil || (*gomath.Clip)(nil).Validate() == nil {
		t.Fatal("uninitialized clip accepted")
	}
	if err = gomath.Square(1).SetClip(&gomath.Clip{}).Validate(); err == nil {
		t.Fatal("invalid object clip accepted")
	}
	bad := want.Clone()
	bad.Contours[0].Curves[0].P1.X = math.Inf(1)
	if result, err := gomath.NewClip(bad); err == nil || result != nil {
		t.Fatal("nonfinite clip accepted")
	}
	if result, err := gomath.NewClip(path.Path{Contours: make([]path.Contour, gomath.MaxClipContours+1)}); err == nil || result != nil {
		t.Fatal("clip contour budget ignored")
	}
	if result, err := gomath.NewClip(path.Path{Contours: []path.Contour{{Curves: make([]path.Cubic, gomath.MaxClipCurves+1)}}}); err == nil || result != nil {
		t.Fatal("clip curve budget ignored")
	}
	for _, matrix := range []geom.Mat4{geom.Scaling(math.Inf(1), 1, 1), {15: 2}, geom.Translation(geom.V(math.MaxFloat64, 0))} {
		_, err := testClip(t, gomath.Square(1e308)).Transform(matrix)
		if err == nil {
			t.Fatal("invalid or overflowing clip transform accepted")
		}
	}
}

func TestClipAnimationSuccessionReplayAndCamera(t *testing.T) {
	s := gomath.NewScene()
	child := gomath.Square(4).SetClip(testClip(t, gomath.Square(2)))
	group := gomath.Group(child)
	must(t, s.Add(group))
	must(t, s.SetCamera(gomath.Camera2D{Center: geom.V(1, 2), Rotation: math.Pi / 2, FrameWidth: 8}))
	must(t, s.Play(2, gomath.Succession(gomath.Shift(group, geom.V(2, 0)).WithRate(gomath.Linear), gomath.Shift(group, geom.V(0, 4)).WithRate(gomath.Linear))))
	if group.Children[0] != child {
		t.Fatal("clip sequence lost child pointer")
	}
	near(t, child.Clip.Bounds().Center().X, 2)
	near(t, child.Clip.Bounds().Center().Y, 4)
	for _, tc := range []struct{ time, x, y float64 }{{0, 0, 0}, {.5, 1, 0}, {1, 2, 0}, {1.5, 2, 2}, {2, 2, 4}} {
		frame := sample(t, s, tc.time)
		clip := frame.Find(child.ID).Clip
		near(t, clip.Bounds().Center().X, tc.x)
		near(t, clip.Bounds().Center().Y, tc.y)
		projected, err := frame.ProjectCamera()
		must(t, err)
		p := projected.Find(child.ID)
		near(t, p.Clip.Bounds().Center().X, tc.y-2)
		near(t, p.Clip.Bounds().Center().Y, 1-tc.x)
		near(t, frame.Find(child.ID).Clip.Bounds().Center().X, tc.x)
	}
	replay, err := gomath.CompileReplay(s, gomath.DefaultReplayOptions())
	must(t, err)
	want := sample(t, s, .5)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 8 {
				got, err := replay.Sample(.5)
				if err != nil || !reflect.DeepEqual(got, want) {
					t.Error("concurrent clip sampling differs", err)
				}
			}
		}()
	}
	wg.Wait()
}

func TestClipMorphAndUnsupportedPresenceAreAtomic(t *testing.T) {
	s := gomath.NewScene()
	o := gomath.Square(4).SetClip(testClip(t, gomath.Square(1)))
	must(t, s.Add(o))
	target := o.Copy().SetClip(testClip(t, gomath.Rectangle(3, 2).Shift(geom.V(2, 0))))
	must(t, s.Play(1, gomath.Transform(o, target).WithRate(gomath.Linear)))
	clip := sample(t, s, .5).Find(o.ID).Clip
	near(t, clip.Bounds().Center().X, 1)
	near(t, clip.Bounds().Size().X, 2)
	near(t, clip.Bounds().Size().Y, 1.5)
	before := o.Clip.Path()
	if err := s.Play(1, gomath.Transform(o, o.Copy().SetClip(nil))); err == nil {
		t.Fatal("clip appearance changed through ambiguous path morph")
	}
	near(t, s.Duration(), 1)
	if !reflect.DeepEqual(o.Clip.Path(), before) {
		t.Fatal("failed clip transform mutated authoring state")
	}
	// Independent layers have independent clip presence and can crossfade.
	must(t, s.Play(1, gomath.FadeTransform(o, gomath.Circle(2))))
}

func TestClipExtrapolationOverflowIsSamplingError(t *testing.T) {
	line := func(x float64) path.Path {
		return path.Path{Contours: []path.Contour{{Start: geom.V(x, 0)}}}
	}
	clip, err := gomath.NewClip(line(-1e308))
	must(t, err)
	targetClip, err := gomath.NewClip(line(1e308))
	must(t, err)
	s := gomath.NewScene()
	o := gomath.Square(1).SetClip(clip)
	must(t, s.Add(o))
	rate := func(a float64) float64 {
		if a == 0 || a == 1 {
			return a
		}
		return 2
	}
	must(t, s.Play(1, gomath.Transform(o, o.Copy().SetClip(targetClip)).WithRate(rate)))
	if _, err = s.Sample(.5); err == nil {
		t.Fatal("finite rate allowed clip coordinate overflow")
	}
	if _, err = s.Sample(1); err != nil {
		t.Fatal(err)
	}
}

func TestClipMorphAlignmentBudgetRejectsBeforeSubdivision(t *testing.T) {
	point := geom.V(0, 0)
	outline := path.Path{Contours: []path.Contour{{Start: point, Curves: []path.Cubic{path.Line(point, point)}}}}
	clip, err := gomath.NewClip(outline)
	must(t, err)
	outline.Contours[0].Curves = make([]path.Cubic, 4097)
	for i := range outline.Contours[0].Curves {
		outline.Contours[0].Curves[i] = path.Line(point, point)
	}
	complex, err := gomath.NewClip(outline)
	must(t, err)
	s := gomath.NewScene()
	o := gomath.Square(1).SetClip(clip)
	must(t, s.Add(o))
	for _, animation := range []gomath.Animation{gomath.Transform(o, o.Copy().SetClip(complex)), gomath.TransformFromCopy(o, o.Copy().SetClip(complex))} {
		if err = s.Play(1, animation); err == nil {
			t.Fatal("expensive clip alignment exceeded work budget")
		}
		near(t, s.Duration(), 0)
		if o.Clip != clip {
			t.Fatal("alignment failure mutated source clip")
		}
	}
}
