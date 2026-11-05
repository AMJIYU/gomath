package gomath_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"gomath"
	"gomath/geom"
	"gomath/path"
	"gomath/render"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func transformPathPair() (*gomath.Object, *gomath.Object) {
	return gomath.Rectangle(2, 1).SetFill(gomath.White).SetStroke(gomath.Transparent, 0).Shift(geom.V(-2, 0)),
		gomath.Rectangle(1, 2).SetFill(gomath.White).SetStroke(gomath.Transparent, 0).Shift(geom.V(2, .5))
}

func transformPathPoints(o *gomath.Object) []geom.Vec3 {
	var out []geom.Vec3
	for _, c := range o.Path.Contours {
		for _, q := range c.Curves {
			out = append(out, q.P0, q.P1, q.P2, q.P3)
		}
	}
	return out
}

func transformPathNear(t *testing.T, a, b *gomath.Object) {
	t.Helper()
	x, y := transformPathPoints(a), transformPathPoints(b)
	if len(x) != len(y) {
		t.Fatal("point count differs")
	}
	for i := range x {
		if x[i].Sub(y[i]).Norm() > 1e-9 {
			t.Fatalf("point %d: %v != %v", i, x[i], y[i])
		}
	}
}

func TestTransformPathPinnedGeometryAndInteriorFrames(t *testing.T) {
	data, err := os.ReadFile("tests/data/root/transform-paths/manifest.json")
	must(t, err)
	var fixture struct {
		Source struct {
			Version, Commit string
			Generator       string `json:"generator_sha256"`
			SHA256          map[string]string
		}
		Cases []struct {
			Kind       string
			Alpha, Arc float64
			Axis       [3]float64
			Points     [][3]float64
			Frame      string
			FrameSHA   string `json:"frame_sha256"`
		}
	}
	must(t, json.Unmarshal(data, &fixture))
	generator, err := os.ReadFile("tests/reference/transform_paths.py")
	must(t, err)
	hash := sha256.Sum256(generator)
	if fixture.Source.Version != "0.22.1" || fixture.Source.Commit != "16ea443e2a5f1d79db0a721161411ce916d6e733" || fixture.Source.Generator != hex.EncodeToString(hash[:]) || len(fixture.Source.SHA256) != 8 || len(fixture.Cases) != 105 {
		t.Fatal("reference provenance/count differs")
	}
	frames := 0
	for _, c := range fixture.Cases {
		t.Run(fmt.Sprintf("%s/%g", c.Kind, c.Alpha), func(t *testing.T) {
			s := gomath.NewScene()
			a, b := transformPathPair()
			must(t, s.Add(a))
			rate := gomath.RateFunc(gomath.Linear)
			if c.Kind == "copy_square_rate" {
				rate = func(t float64) float64 { return t * t }
			}
			if c.Kind == "overshoot" {
				rate = func(t float64) float64 { return 1.5 * t }
			}
			anim := gomath.Transform(a, b).WithPathArc(c.Arc)
			id := a.ID
			switch {
			case strings.HasPrefix(c.Kind, "copy"):
				anim = gomath.TransformFromCopy(a, b).WithPathArc(c.Arc)
				id = b.ID
			case c.Kind == "replacement":
				anim = gomath.ReplacementTransform(a, b).WithPathArc(c.Arc)
			case c.Kind == "clockwise":
				anim = gomath.ClockwiseTransform(a, b)
			case c.Kind == "counterclockwise":
				anim = gomath.CounterclockwiseTransform(a, b)
			}
			anim = anim.WithPathArcAxis(geom.Vec3{X: c.Axis[0], Y: c.Axis[1], Z: c.Axis[2]}).WithRate(rate)
			must(t, s.Play(1, anim))
			time := c.Alpha
			if c.Kind == "replacement" && time == 1 {
				time = math.Nextafter(1, 0)
			}
			f := sample(t, s, time)
			o := f.Find(id)
			if o == nil {
				t.Fatal("missing animated object")
			}
			points := transformPathPoints(o)
			if len(points) != len(c.Points) {
				t.Fatal("topology differs")
			}
			for i, p := range points {
				q := geom.Vec3{X: c.Points[i][0], Y: c.Points[i][1], Z: c.Points[i][2]}
				if p.Sub(q).Norm() > 1e-10 {
					t.Fatalf("point %d got %v want %v", i, p, q)
				}
			}
			if c.Frame == "" {
				return
			}
			frames++
			pngBytes, err := os.ReadFile("tests/data/root/transform-paths/" + c.Frame)
			must(t, err)
			frameHash := sha256.Sum256(pngBytes)
			if hex.EncodeToString(frameHash[:]) != c.FrameSHA {
				t.Fatal("reference frame hash differs")
			}
			ref, err := png.Decode(bytes.NewReader(pngBytes))
			must(t, err)
			cfg := render.DefaultConfig()
			cfg.Width, cfg.Height, cfg.FrameWidth = 128, 96, 10
			cfg.Background = gomath.Black
			r, err := render.NewRasterizer(cfg)
			must(t, err)
			// The copy reference isolates the animated target layer.
			f.Objects = []*gomath.Object{o}
			img, err := r.Render(f)
			must(t, err)
			union, intersect, sumDiff := 0, 0, 0.0
			for y := 0; y < 96; y++ {
				for x := 0; x < 128; x++ {
					rr, _, _, _ := ref.At(x, y).RGBA()
					value := float64(rr) / 257
					v := float64(img.RGBAAt(x, y).R)
					sumDiff += math.Abs(v - value)
					if v > 127 || value > 127 {
						union++
					}
					if v > 127 && value > 127 {
						intersect++
					}
				}
			}
			if union == 0 || float64(intersect)/float64(union) < .96 || sumDiff/(128*96) > 1.5 {
				t.Fatalf("Cairo frame mask IoU %.5f MAE %.5f", float64(intersect)/float64(union), sumDiff/(128*96))
			}
		})
	}
	if frames != 12 {
		t.Fatalf("checked %d frames", frames)
	}
}

func TestTransformPathStraightPixelsAndValueOptions(t *testing.T) {
	var previous []byte
	for _, arc := range []float64{0, .009, -.009} {
		s := gomath.NewScene()
		a, b := transformPathPair()
		must(t, s.Add(a))
		anim := gomath.Transform(a, b).WithRate(gomath.Linear)
		if arc != 0 {
			anim = anim.WithPathArc(arc)
		}
		must(t, s.Play(1, anim))
		cfg := render.DefaultConfig()
		cfg.Width, cfg.Height = 80, 60
		r, err := render.NewRasterizer(cfg)
		must(t, err)
		img, err := r.Render(sample(t, s, .375))
		must(t, err)
		if previous != nil && !bytes.Equal(previous, img.Pix) {
			t.Fatal("straight threshold changed pixels")
		}
		previous = append([]byte(nil), img.Pix...)
	}
	a, b := transformPathPair()
	base := gomath.Transform(a, b).WithPathArc(1.4)
	_ = base.WithPathArc(-1.4).WithPathArcAxis(geom.Vec3{X: 1})
	s := gomath.NewScene()
	must(t, s.Add(a))
	must(t, s.Play(1, base.WithRate(gomath.Linear)))
	if sample(t, s, .5).Find(a.ID).Center().Y >= .25 {
		t.Fatal("derived option mutated base animation")
	}
}

func TestTransformPathWindowsGroupsAndSuccession(t *testing.T) {
	makeScene := func(group bool) (*gomath.Scene, uint64) {
		s := gomath.NewScene()
		a, b := transformPathPair()
		must(t, s.Add(a))
		leaf := gomath.Transform(a, b).WithPathArc(1.4).WithRate(func(a float64) float64 { return a * a }).WithWindow(.25, .75)
		if group {
			leaf = gomath.AnimationGroup(leaf).WithRate(func(a float64) float64 { return a * a })
		}
		must(t, s.Play(1, leaf))
		return s, a.ID
	}
	leaf, id := makeScene(false)
	group, gid := makeScene(true)
	transformPathNear(t, sample(t, leaf, .25).Find(id), sample(t, group, .5).Find(gid))
	transformPathNear(t, sample(t, leaf, .5).Find(id), sample(t, group, math.Sqrt(.5)).Find(gid))
	transformPathNear(t, sample(t, leaf, 1).Find(id), sample(t, group, 1).Find(gid))
	for _, copyMode := range []bool{false, true} {
		s := gomath.NewScene()
		a, b := transformPathPair()
		parent := gomath.Group(a)
		must(t, s.Add(parent))
		anim := gomath.ReplacementTransform(a, b).WithPathArc(1.4)
		if copyMode {
			anim = gomath.TransformFromCopy(a, b).WithPathArc(1.4)
		}
		beforeA, beforeB := a.Copy(), b.Copy()
		must(t, s.Play(3, gomath.Succession(anim.WithDuration(2), gomath.Shift(b, geom.V(1, 0)).WithDuration(1))))
		if !copyMode && parent.Children[0] != b {
			t.Fatal("replacement did not install actual target pointer")
		}
		if copyMode && parent.Children[0] != a {
			t.Fatal("copy changed source child pointer")
		}
		if copyMode {
			transformPathNear(t, a, beforeA)
		}
		near(t, b.Center().X, beforeB.Center().X+1)
		boundary := sample(t, s, 2)
		if boundary.Find(b.ID) == nil || (!copyMode && boundary.Find(a.ID) != nil) {
			t.Fatal("cleanup did not precede next step")
		}
		near(t, boundary.Find(b.ID).Center().X, beforeB.Center().X)
	}
}

func TestTransformPathCopyWindowsGroupFinishAndChildIdentity(t *testing.T) {
	makeCopy := func(grouped bool) (*gomath.Scene, uint64) {
		s := gomath.NewScene()
		a, b := transformPathPair()
		must(t, s.Add(a))
		anim := gomath.TransformFromCopy(a, b).WithPathArc(1.4).WithRate(func(x float64) float64 { return x * x })
		if grouped {
			anim = gomath.AnimationGroup(anim.WithWindow(.25, .75)).WithRate(func(x float64) float64 { return x * x })
		}
		must(t, s.Play(1, anim))
		return s, b.ID
	}
	plain, pid := makeCopy(false)
	grouped, gid := makeCopy(true)
	for _, p := range []float64{0, .125, .5, .875} {
		transformPathNear(t, sample(t, plain, p).Find(pid), sample(t, grouped, math.Sqrt(.25+.5*p)).Find(gid))
	}
	// A returning parent clock still finishes each child at its own rate(1).
	a, b := transformPathPair()
	child := a
	targetChild := b
	parent, target := gomath.Group(a), gomath.Group(b)
	targetBefore := b.Copy()
	s := gomath.NewScene()
	must(t, s.Add(parent))
	must(t, s.Play(1, gomath.AnimationGroup(gomath.Transform(parent, target).WithPathArc(1.4)).WithRate(func(x float64) float64 { return 4 * x * (1 - x) })))
	if parent.Children[0] != child || target.Children[0] != targetChild {
		t.Fatal("Transform changed existing child pointers")
	}
	transformPathNear(t, child, targetBefore)
	transformPathNear(t, targetChild, targetBefore)
	// Replacement cleanup installs the actual target even with a returning
	// leaf rate. The removed source retains the leaf's authored final state.
	x, y := transformPathPair()
	before := x.Copy()
	ss := gomath.NewScene()
	must(t, ss.Add(x))
	must(t, ss.Play(1, gomath.ReplacementTransform(x, y).WithPathArc(1.4).WithRate(func(x float64) float64 { return 4 * x * (1 - x) })))
	transformPathNear(t, x, before)
	if sample(t, ss, 1).Find(x.ID) != nil || sample(t, ss, 1).Find(y.ID) == nil {
		t.Fatal("returning rate prevented replacement cleanup")
	}
}

func TestTransformPathAtomicRejectionsAndNumericalBounds(t *testing.T) {
	s := gomath.NewScene()
	a, b := transformPathPair()
	child := a
	parent := gomath.Group(a)
	must(t, s.Add(parent))
	before, oldA, oldB := sample(t, s, 0), a.Copy(), b.Copy()
	for _, anim := range []gomath.Animation{
		gomath.Transform(a, b).WithPathArc(math.NaN()), gomath.Transform(a, b).WithPathArc(math.Inf(1)),
		gomath.Transform(a, b).WithPathArc(7), gomath.Transform(a, b).WithPathArc(2 * math.Pi),
		gomath.Transform(a, b).WithPathArc(2*math.Pi - 1e-7), gomath.Transform(a, b).WithPathArc(1).WithPathArcAxis(geom.Vec3{X: math.Inf(1)}),
		gomath.FadeOut(a).WithPathArc(0), gomath.Rotate(a, 1).WithPathArc(1), gomath.AnimationGroup(gomath.Transform(a, b)).WithPathArc(1),
		gomath.Succession(gomath.Shift(a, geom.V(1, 0)), gomath.Transform(a, b).WithPathArc(7)),
		gomath.Transform(a, b).WithPathArc(1).WithRate(func(float64) float64 { return 1e6 + 1 }),
		gomath.Transform(a, b).WithPathArc(1).WithRate(func(float64) float64 { return math.NaN() }),
	} {
		if s.Play(1, anim) == nil {
			t.Fatal("invalid path accepted")
		}
		if s.Duration() != 0 || !reflect.DeepEqual(before, sample(t, s, 0)) || parent.Children[0] != child {
			t.Fatal("failure was not atomic")
		}
		transformPathNear(t, a, oldA)
		transformPathNear(t, b, oldB)
	}
	for _, axis := range []geom.Vec3{{Z: 1}, {Z: math.SmallestNonzeroFloat64}, {Z: math.MaxFloat64}, {}} {
		s := gomath.NewScene()
		a, b := transformPathPair()
		must(t, s.Add(a))
		must(t, s.Play(1, gomath.Transform(a, b).WithPathArc(2*math.Pi-1e-4).WithPathArcAxis(axis)))
		for _, time := range []float64{0, .5, 1} {
			must(t, sample(t, s, time).Find(a.ID).Validate())
		}
	}
	large := b.Copy().Shift(geom.V(2e9, 0))
	if s.Play(1, gomath.Transform(a, large).WithPathArc(1)) == nil {
		t.Fatal("coordinate budget ignored")
	}
	busy := gomath.Square(1)
	busy.Path = path.Path{Contours: []path.Contour{{Curves: make([]path.Cubic, 5000)}}}
	if s.Play(1, gomath.Transform(a, busy).WithPathArc(1)) == nil {
		t.Fatal("alignment work budget ignored")
	}
	// Sample-time failures return no partial frame, without invalidating later samples.
	ss := gomath.NewScene()
	x, y := transformPathPair()
	must(t, ss.Add(x))
	must(t, ss.Play(1, gomath.Transform(x, y).WithPathArc(1).WithRate(func(p float64) float64 {
		if p == .5 {
			return 2e6
		}
		return p
	})))
	if f, err := ss.Sample(.5); err == nil || f != nil {
		t.Fatal("invalid intermediate rate escaped")
	}
	must(t, sample(t, ss, .25).Find(x.ID).Validate())
	negative := gomath.NewScene()
	x, y = transformPathPair()
	must(t, negative.Add(x))
	must(t, negative.Play(1, gomath.Transform(x, y).WithPathArc(math.Pi).WithRate(func(p float64) float64 { return -p })))
	if sample(t, negative, .5).Find(x.ID).Center().Sub(geom.V(-.25, 2.25)).Norm() > 1e-10 {
		t.Fatal("negative rate was clamped instead of extrapolated along point circles")
	}
}

func TestTransformPathPlanarClipAndGradientFrames(t *testing.T) {
	stops := []gomath.GradientStop{{Offset: 0, Color: gomath.White}, {Offset: 1, Color: gomath.Black}}
	for _, radial := range []bool{false, true} {
		a := gomath.Rectangle(2, 2).SetStroke(gomath.Transparent, 0).Shift(geom.V(-2, 0))
		g, err := gomath.NewLinearGradient(geom.V(-3, 0), geom.V(-1, 0), stops)
		if radial {
			g, err = gomath.NewRadialGradient(geom.V(-2, 0), 1, stops)
		}
		must(t, err)
		a.SetFillGradient(g)
		clip, err := gomath.NewClip(gomath.Rectangle(1, 2).Shift(geom.V(-2, 0)).Path)
		must(t, err)
		a.SetClip(clip)
		b := a.Copy().Shift(geom.V(4, .5))
		expected := a.Copy().Shift(geom.V(2.25, -1.75))
		s := gomath.NewScene()
		must(t, s.Add(a))
		must(t, s.Play(1, gomath.Transform(a, b).WithPathArc(math.Pi).WithRate(gomath.Linear)))
		o := sample(t, s, .5).Find(a.ID)
		// Center (-2,0) -> (2,.5), positive half turn: (.25,-1.75).
		want := geom.V(.25, -1.75)
		if o.Center().Sub(want).Norm() > 1e-10 || o.Clip.Bounds().Center().Sub(want).Norm() > 1e-10 {
			t.Fatal("geometry/clip frame failed to follow arc")
		}
		matrix := o.Style.FillGradient.Matrix()
		original := g.Matrix()
		near(t, matrix[4], original[4]+2.25)
		near(t, matrix[5], original[5]-1.75)
		for i := range 4 {
			near(t, matrix[i], original[i])
		}
		cfg := render.DefaultConfig()
		cfg.Width, cfg.Height, cfg.FrameWidth = 128, 96, 10
		r, err := render.NewRasterizer(cfg)
		must(t, err)
		actualPixels, err := r.Render(sample(t, s, .5))
		must(t, err)
		expectedPixels, err := r.Render(&gomath.Frame{Objects: []*gomath.Object{expected}})
		must(t, err)
		for i, v := range actualPixels.Pix {
			if math.Abs(float64(v)-float64(expectedPixels.Pix[i])) > 1 {
				t.Fatal("translated clip/gradient rendered frame differs from independently placed paint")
			}
		}
		// Geometry is planar, but an oblique arc or changed paint basis cannot
		// be represented by this planar translation-frame contract.
		ss := gomath.NewScene()
		x := b.Copy()
		must(t, ss.Add(x))
		before := x.Copy()
		if ss.Play(1, gomath.Transform(x, b).WithPathArc(1).WithPathArcAxis(geom.Vec3{X: 1, Z: 1})) == nil {
			t.Fatal("tilted paint accepted")
		}
		y := b.Copy().Scale(2)
		if ss.Play(1, gomath.Transform(x, y).WithPathArc(1)) == nil {
			t.Fatal("gradient basis morph accepted")
		}
		transformPathNear(t, x, before)
	}
	// Clips with changing geometry follow their own corresponding point arcs.
	a, b := transformPathPair()
	ca, err := gomath.NewClip(a.Path)
	must(t, err)
	cb, err := gomath.NewClip(b.Path)
	must(t, err)
	a.SetClip(ca)
	b.SetClip(cb)
	s := gomath.NewScene()
	must(t, s.Add(a))
	must(t, s.Play(1, gomath.Transform(a, b).WithPathArc(1.4).WithRate(gomath.Linear)))
	o := sample(t, s, .375).Find(a.ID)
	if !reflect.DeepEqual(o.Path, o.Clip.Path()) {
		t.Fatal("clip control trajectories differ from object")
	}
}

func TestTransformPathBitmapAffineValidation(t *testing.T) {
	pixels := image.NewRGBA(image.Rect(0, 0, 2, 2))
	pixels.SetRGBA(0, 0, color.RGBA{R: 255, A: 255})
	asset, err := gomath.NewBitmap(pixels)
	must(t, err)
	a, err := gomath.NewImage(asset, 2)
	must(t, err)
	a.Shift(geom.V(-2, 0))
	b := a.Copy().Scale(.5).Shift(geom.V(4, 1))
	s := gomath.NewScene()
	must(t, s.Add(a))
	must(t, s.Play(1, gomath.Transform(a, b).WithPathArc(1.4)))
	for _, p := range []float64{0, .125, .5, .875, 1} {
		_, _, _, err := sample(t, s, p).Find(a.ID).ImageCorners()
		must(t, err)
	}
	x := a.Copy()
	y := b.Copy()
	y.Path.Contours[0].Curves[0].P1.Y += .25
	ss := gomath.NewScene()
	must(t, ss.Add(x))
	before := x.Copy()
	if ss.Play(1, gomath.Transform(x, y).WithPathArc(1)) == nil {
		t.Fatal("curved bitmap plane accepted")
	}
	transformPathNear(t, x, before)
}

func TestTransformPathConcurrentSnapshotsAndFrozenReplay(t *testing.T) {
	a, b := transformPathPair()
	g, err := gomath.NewLinearGradient(geom.V(-3, 0), geom.V(-1, 0), []gomath.GradientStop{{Offset: 0, Color: gomath.White}, {Offset: 1, Color: gomath.Black}})
	must(t, err)
	a.SetFillGradient(g)
	h, err := g.Transform(geom.Translation(geom.V(4, .5)))
	must(t, err)
	b.SetFillGradient(h)
	ca, err := gomath.NewClip(a.Path)
	must(t, err)
	a.SetClip(ca)
	cb, err := gomath.NewClip(b.Path)
	must(t, err)
	b.SetClip(cb)
	s := gomath.NewScene()
	must(t, s.Add(a))
	must(t, s.Play(1, gomath.Transform(a, b).WithPathArc(1.4)))
	replay, err := gomath.CompileReplay(s, gomath.DefaultReplayOptions())
	must(t, err)
	want := sample(t, s, .375)
	a.Shift(geom.V(100, 100))
	b.Shift(geom.V(-100, -100))
	var wg sync.WaitGroup
	errs := make(chan error, 32)
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				f, err := s.Sample(.375)
				if err != nil {
					errs <- err
					return
				}
				r, err := replay.Sample(.375)
				if err != nil {
					errs <- err
					return
				}
				if !reflect.DeepEqual(f, want) || !reflect.DeepEqual(r, want) {
					errs <- fmt.Errorf("snapshot changed")
					return
				}
				f.Find(a.ID).Shift(geom.V(1, 2))
				r.Find(a.ID).Path.Contours[0].Curves[0].P1.X += 99
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	transformPathNear(t, sample(t, s, .375).Find(a.ID), want.Find(a.ID))
}

func BenchmarkTransformPathSample(b *testing.B) {
	for _, arc := range []float64{0, 1.4} {
		b.Run(fmt.Sprint(arc), func(b *testing.B) {
			s := gomath.NewScene()
			a, target := transformPathPair()
			if err := s.Add(a); err != nil {
				b.Fatal(err)
			}
			if err := s.Play(1, gomath.Transform(a, target).WithPathArc(arc)); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := s.Sample(.375); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkTransformPathRender(b *testing.B) {
	for _, arc := range []float64{0, 1.4} {
		b.Run(fmt.Sprint(arc), func(b *testing.B) {
			s := gomath.NewScene()
			a, target := transformPathPair()
			if err := s.Add(a); err != nil {
				b.Fatal(err)
			}
			if err := s.Play(1, gomath.Transform(a, target).WithPathArc(arc)); err != nil {
				b.Fatal(err)
			}
			cfg := render.DefaultConfig()
			cfg.Width, cfg.Height, cfg.FrameWidth = 128, 96, 10
			r, err := render.NewRasterizer(cfg)
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				frame, err := s.Sample((float64(i%30) + .5) / 30)
				if err != nil {
					b.Fatal(err)
				}
				if _, err := r.Render(frame); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
