package gomath_test

import (
	"gomath"
	"gomath/geom"
	"math"
	"reflect"
	"sync"
	"testing"
)

func TestCameraTimelineRandomAccessAndSnapshots(t *testing.T) {
	s := gomath.NewScene()
	initial := gomath.Camera2D{Center: geom.V(1, -2), FrameWidth: 12, Rotation: .2}
	target := gomath.Camera2D{Center: geom.V(5, 2), FrameWidth: 4, Rotation: 2*math.Pi + .2}
	must(t, s.Wait(1))
	must(t, s.SetCamera(initial))
	object := gomath.Square(1)
	must(t, s.Add(object))
	must(t, s.AnimateCameraWithRate(2, target, gomath.Linear, gomath.Shift(object, geom.V(4, 0)).WithRate(gomath.Linear)))
	must(t, s.Wait(1))
	// Authoring inputs are captured values, and full turns stay visible.
	target.FrameWidth = 100
	for _, time := range []float64{4, 2, 0, 3, 1, 2.5, 2, 4} {
		f := sample(t, s, time)
		if time < 1 {
			if f.Camera != nil {
				t.Fatal("camera applied before SetCamera")
			}
			continue
		}
		a := math.Min(1, (time-1)/2)
		near(t, f.Camera.Center.X, 1+4*a)
		near(t, f.Camera.Center.Y, -2+4*a)
		near(t, f.Camera.FrameWidth, 12-8*a)
		near(t, f.Camera.Rotation, .2+2*math.Pi*a)
		near(t, f.Find(object.ID).Center().X, 4*a)
	}
	f := sample(t, s, 2)
	*f.Camera = gomath.Camera2D{}
	f.Find(object.ID).Shift(geom.V(100, 0))
	near(t, sample(t, s, 2).Camera.FrameWidth, 8)
	near(t, sample(t, s, 2).Find(object.ID).Center().X, 2)
}

func TestCameraBoundaryStepsAndThereAndBack(t *testing.T) {
	s := gomath.NewScene()
	a := gomath.Camera2D{FrameWidth: 12}
	b := gomath.Camera2D{FrameWidth: 4, Center: geom.V(4, 0)}
	must(t, s.SetCamera(a))
	must(t, s.SetCamera(b))
	if got := sample(t, s, 0).Camera; *got != b {
		t.Fatal("last same-time set did not win", got)
	}
	must(t, s.AnimateCameraWithRate(2, a, gomath.ThereAndBack))
	near(t, sample(t, s, 1).Camera.FrameWidth, 12)
	near(t, sample(t, s, 2).Camera.FrameWidth, 4)
	must(t, s.Wait(1))
	near(t, sample(t, s, 2.5).Camera.FrameWidth, 4)
	must(t, s.SetCamera(a))
	near(t, sample(t, s, 3).Camera.FrameWidth, 12)
	near(t, sample(t, s, math.Nextafter(3, 0)).Camera.FrameWidth, 4)
	must(t, s.AnimateCameraWithRate(1, b, nil))
	near(t, sample(t, s, 3.25).Camera.FrameWidth, 10)
	near(t, sample(t, s, 3).Camera.FrameWidth, 12)
	near(t, sample(t, s, 4).Camera.FrameWidth, 4)
}

func TestCameraValidationAndRollback(t *testing.T) {
	s := gomath.NewScene()
	a := gomath.Camera2D{FrameWidth: 12}
	b := gomath.Camera2D{FrameWidth: 4, Center: geom.V(3, 4), Rotation: 1}
	if s.AnimateCamera(1, b) == nil {
		t.Fatal("animation accepted without SetCamera")
	}
	if s.Duration() != 0 {
		t.Fatal("missing camera advanced timeline")
	}
	must(t, s.SetCamera(a))
	object := gomath.Square(1)
	must(t, s.Add(object))
	must(t, s.Wait(1))
	before := sample(t, s, 1)
	badCameras := []gomath.Camera2D{
		{}, {FrameWidth: -1}, {FrameWidth: math.NaN()}, {FrameWidth: math.Inf(1)},
		{FrameWidth: 1, Center: geom.V(math.NaN(), 0)},
		{FrameWidth: 1, Center: geom.Vec3{Z: 1}},
		{FrameWidth: 1, Rotation: math.Inf(-1)},
	}
	for _, bad := range badCameras {
		if s.SetCamera(bad) == nil {
			t.Fatal("invalid SetCamera accepted", bad)
		}
		if s.AnimateCamera(1, bad, gomath.Shift(object, geom.V(3, 0))) == nil {
			t.Fatal("invalid camera animation accepted", bad)
		}
	}
	for _, d := range []float64{0, -1, math.NaN(), math.Inf(1), math.SmallestNonzeroFloat64} {
		if s.AnimateCamera(d, b) == nil {
			t.Fatal("invalid/non-advancing duration accepted", d)
		}
	}
	for _, rate := range []gomath.RateFunc{
		func(float64) float64 { return math.NaN() },
		func(x float64) float64 {
			if x == 1 {
				return math.Inf(1)
			}
			return x
		},
	} {
		if s.AnimateCameraWithRate(1, b, rate, gomath.Shift(object, geom.V(3, 0))) == nil {
			t.Fatal("unsafe endpoint rate accepted")
		}
	}
	if s.AnimateCamera(1, b, gomath.Shift(object, geom.V(3, 0)), gomath.Shift(object, geom.V(1, 0))) == nil {
		t.Fatal("conflicting object tracks accepted")
	}
	absent := gomath.Circle(1)
	if s.AnimateCamera(1, b, gomath.FadeIn(absent).WithWindow(1, 0)) == nil {
		t.Fatal("bad object track accepted")
	}
	if got := sample(t, s, 1); !reflect.DeepEqual(got, before) || s.Duration() != 1 || object.Center().X != 0 {
		t.Fatal("failed camera authoring changed state")
	}
	must(t, s.AnimateCamera(2, b))
	near(t, sample(t, s, 1.5).Camera.FrameWidth, 12-8*gomath.Smooth(.25))
}

func TestCameraRateFailuresClampingAndActualEndpoints(t *testing.T) {
	s := gomath.NewScene()
	initial := gomath.Camera2D{FrameWidth: 8}
	target := gomath.Camera2D{FrameWidth: 2}
	must(t, s.SetCamera(initial))
	unsafe := func(x float64) float64 {
		if x > .2 && x < .8 {
			return math.NaN()
		}
		return x
	}
	must(t, s.AnimateCameraWithRate(1, target, unsafe))
	if f, err := s.Sample(.5); err == nil || f != nil {
		t.Fatal("unsafe interior camera rate not propagated")
	}
	near(t, sample(t, s, 1).Camera.FrameWidth, 2)
	must(t, s.AnimateCameraWithRate(1, initial, func(x float64) float64 { return 4*x - 1 }))
	near(t, sample(t, s, 1.1).Camera.FrameWidth, 2)
	near(t, sample(t, s, 1.9).Camera.FrameWidth, 8)
	// Custom endpoints define both boundary states, not a forced target snap.
	must(t, s.AnimateCameraWithRate(1, target, func(float64) float64 { return .5 }))
	must(t, s.Wait(1))
	for _, time := range []float64{2, 2.5, 3, 4} {
		near(t, sample(t, s, time).Camera.FrameWidth, 5)
	}
}

func TestCameraVisibleBeforeUpdatersAndConcurrentSampling(t *testing.T) {
	s := gomath.NewScene()
	must(t, s.SetCamera(gomath.Camera2D{FrameWidth: 10}))
	must(t, s.AnimateCameraWithRate(2, gomath.Camera2D{FrameWidth: 2}, gomath.Linear))
	must(t, s.AddUpdater(func(f *gomath.Frame, time float64) error {
		if f.Camera == nil {
			t.Error("updater cannot see camera")
			return nil
		}
		f.Camera.Center.X = time
		return nil
	}))
	want := sample(t, s, .75)
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := range 10 {
				time := []float64{2, .75, 0, 1.1}[(i+j)%4]
				f, err := s.Sample(time)
				if err != nil {
					t.Error(err)
					return
				}
				if time == .75 && !reflect.DeepEqual(f, want) {
					t.Error("camera differs across sampling order")
				}
				if f.Camera.Center.X != time {
					t.Error("updater camera mutation leaked")
				}
				f.Camera.FrameWidth = 99
			}
		}(i)
	}
	wg.Wait()
	near(t, sample(t, s, .75).Camera.FrameWidth, 7)
}

func TestCameraExtremeInterpolation(t *testing.T) {
	s := gomath.NewScene()
	initial := gomath.Camera2D{Center: geom.V(-math.MaxFloat64, 0), FrameWidth: math.MaxFloat64, Rotation: -math.MaxFloat64}
	target := gomath.Camera2D{Center: geom.V(math.MaxFloat64, 0), FrameWidth: math.MaxFloat64, Rotation: math.MaxFloat64}
	must(t, s.SetCamera(initial))
	must(t, s.AnimateCameraWithRate(1, target, gomath.Linear))
	f := sample(t, s, .5)
	if err := f.Camera.Validate(); err != nil {
		t.Fatal(err)
	}
	if f.Camera.Center.X != 0 || f.Camera.Rotation != 0 {
		t.Fatal("finite interpolation overflowed")
	}
	// Equal positive subnormal widths must not round down to zero halfway.
	small := gomath.Camera2D{FrameWidth: math.SmallestNonzeroFloat64}
	s = gomath.NewScene()
	must(t, s.SetCamera(small))
	must(t, s.AnimateCameraWithRate(1, small, gomath.Linear))
	if sample(t, s, .5).Camera.FrameWidth != small.FrameWidth {
		t.Fatal("subnormal camera width lost during interpolation")
	}
}
