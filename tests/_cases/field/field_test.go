package field_test

import (
	"bytes"
	"gomath"
	"gomath/field"
	"gomath/geom"
	"gomath/render"
	"math"
	"reflect"
	"sync"
	"testing"
)

func closeVec(t *testing.T, got, want geom.Vec3, tol float64) {
	t.Helper()
	if got.Sub(want).Norm() > tol {
		t.Fatalf("got %v, want %v within %g", got, want, tol)
	}
}

func TestVectorSamplingLengthsDirectionsAndColors(t *testing.T) {
	o := field.DefaultVectorOptions()
	o.MaxLength = .5
	var err error
	o.ColorFunc, err = field.SpeedColor(gomath.Blue, gomath.Red, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	v, err := field.NewVectorField(func(p geom.Vec3) geom.Vec3 { return p }, field.Range{-1, 1, 1}, field.Range{-1, 1, 1}, o)
	if err != nil {
		t.Fatal(err)
	}
	samples := v.Samples()
	if len(samples) != 9 || len(v.Object.Children) != 8 {
		t.Fatal("wrong grid or zero vector handling")
	}
	for i, sample := range samples {
		closeVec(t, sample.Position, geom.V(float64(i%3)-1, float64(i/3)-1), 1e-12)
		if sample.Speed == 0 {
			if sample.Arrow != nil {
				t.Fatal("zero vector has a direction")
			}
			continue
		}
		shaft, head := sample.Arrow.Children[0], sample.Arrow.Children[1]
		start, end := shaft.Path.Contours[0].Start, head.Path.Contours[0].Start
		delta := end.Sub(start)
		if math.Abs(delta.Norm()-.5) > 1e-12 || delta.Dot(sample.Vector) <= 0 || delta.Cross(sample.Vector).Norm() > 1e-12 {
			t.Fatal("wrong scaled arrow", sample)
		}
		if shaft.Style.Stroke != gomath.Red || head.Style.Fill != gomath.Red || head.Style.Stroke.A != 0 {
			t.Fatal("arrow color/stroke missing")
		}
	}
	if err := v.Object.Validate(); err != nil {
		t.Fatal(err)
	}
	samples[0].Position.X = 500
	if v.Samples()[0].Position.X == 500 {
		t.Fatal("sample records alias")
	}
}

func TestCustomScalingSubnormalAndLargeFiniteVectors(t *testing.T) {
	o := field.DefaultVectorOptions()
	o.Length = func(speed float64) float64 { return .25 }
	v, err := field.NewVectorField(func(geom.Vec3) geom.Vec3 { return geom.V(3, 4) }, field.Range{0, 0, 1}, field.Range{0, 0, 1}, o)
	if err != nil {
		t.Fatal(err)
	}
	a := v.Samples()[0].Arrow
	closeVec(t, a.Children[1].Path.Contours[0].Start, geom.V(.15, .2), 1e-12)
	for _, magnitude := range []float64{math.SmallestNonzeroFloat64, 1e-200, 1e300} {
		o := field.DefaultVectorOptions()
		o.Length = func(float64) float64 { return .5 }
		v, err := field.NewVectorField(func(geom.Vec3) geom.Vec3 { return geom.V(magnitude, 0) }, field.Range{0, 0, 1}, field.Range{0, 0, 1}, o)
		if err != nil {
			t.Fatal("finite vector rejected", magnitude, err)
		}
		closeVec(t, v.Samples()[0].Arrow.Children[1].Path.Contours[0].Start, geom.V(.5, 0), 1e-12)
	}
	o = field.DefaultVectorOptions()
	o.Scale = 1e300
	if _, err := field.NewVectorField(func(geom.Vec3) geom.Vec3 { return geom.V(1e300, 0) }, field.Range{0, 0, 1}, field.Range{0, 0, 1}, o); err != nil {
		t.Fatal("capped finite scaling overflowed", err)
	}
	o.Length = func(float64) float64 { return .5 }
	v, err = field.NewVectorField(func(geom.Vec3) geom.Vec3 {
		return geom.V(math.SmallestNonzeroFloat64, math.SmallestNonzeroFloat64)
	}, field.Range{0, 0, 1}, field.Range{0, 0, 1}, o)
	if err != nil {
		t.Fatal(err)
	}
	closeVec(t, v.Samples()[0].Arrow.Children[1].Path.Contours[0].Start, geom.V(.5/math.Sqrt2, .5/math.Sqrt2), 1e-12)
	o = field.DefaultVectorOptions()
	o.TipSize = 1e-200
	if _, err := field.NewVectorField(func(geom.Vec3) geom.Vec3 { return geom.V(1, 0) }, field.Range{0, 0, 1}, field.Range{0, 0, 1}, o); err == nil {
		t.Fatal("arrowhead precision loss accepted")
	}
}

func TestVectorFieldRejectsInvalidInputs(t *testing.T) {
	fn := func(geom.Vec3) geom.Vec3 { return geom.V(1, 0) }
	for _, mutate := range []func(*field.VectorOptions){
		func(o *field.VectorOptions) { o.Scale = math.NaN() },
		func(o *field.VectorOptions) { o.MaxLength = 0 },
		func(o *field.VectorOptions) { o.StrokeWidth = -1 },
		func(o *field.VectorOptions) { o.TipSize = math.Inf(1) },
		func(o *field.VectorOptions) { o.Color.R = 2 },
		func(o *field.VectorOptions) { o.Length = func(float64) float64 { return -1 } },
		func(o *field.VectorOptions) { o.Length = func(float64) float64 { return math.NaN() } },
		func(o *field.VectorOptions) {
			o.ColorFunc = func(_, _ geom.Vec3, _ float64) gomath.Color { return gomath.Color{A: math.NaN()} }
		},
	} {
		o := field.DefaultVectorOptions()
		mutate(&o)
		if _, err := field.NewVectorField(fn, field.Range{0, 1, 1}, field.Range{0, 1, 1}, o); err == nil {
			t.Fatal("invalid option accepted")
		}
	}
	for _, r := range []field.Range{{0, 1, 0}, {1, 0, 1}, {0, 1, math.NaN()}, {0, 1, 1e-20}, {1e20, 2e20, 1}} {
		if _, err := field.NewVectorField(fn, r, field.Range{0, 0, 1}, field.DefaultVectorOptions()); err == nil {
			t.Fatal("invalid range accepted", r)
		}
	}
	calls := 0
	counted := func(p geom.Vec3) geom.Vec3 { calls++; return fn(p) }
	if _, err := field.NewVectorField(counted, field.Range{0, 1000, 1}, field.Range{0, 1000, 1}, field.DefaultVectorOptions()); err == nil || calls != 0 {
		t.Fatal("grid budget failed before evaluation", calls, err)
	}
	for _, vector := range []geom.Vec3{geom.V(math.NaN(), 0), geom.V(0, math.Inf(1)), {X: 1, Z: 1}, geom.V(math.MaxFloat64, math.MaxFloat64)} {
		if _, err := field.NewVectorField(func(geom.Vec3) geom.Vec3 { return vector }, field.Range{0, 0, 1}, field.Range{0, 0, 1}, field.DefaultVectorOptions()); err == nil {
			t.Fatal("invalid vector accepted", vector)
		}
	}
	if _, err := field.NewVectorField(nil, field.Range{0, 1, 1}, field.Range{0, 1, 1}, field.DefaultVectorOptions()); err == nil {
		t.Fatal("nil callback accepted")
	}
	if _, err := field.SpeedColor(gomath.Blue, gomath.Red, 1, 1); err == nil {
		t.Fatal("empty color interval accepted")
	}
	if _, err := field.SpeedColor(gomath.Color{R: -1}, gomath.Red, 0, 1); err == nil {
		t.Fatal("invalid gradient color accepted")
	}
}

func TestRK4FourthOrderAndCircleInvariant(t *testing.T) {
	rotation := func(p geom.Vec3) geom.Vec3 { return geom.V(-p.Y, p.X) }
	o := field.DefaultStreamOptions()
	o.StepSize, o.MaxSteps = .2, 10
	coarse, err := field.Integrate(rotation, geom.V(1, 0), o)
	if err != nil {
		t.Fatal(err)
	}
	o.StepSize, o.MaxSteps = .1, 20
	fine, err := field.Integrate(rotation, geom.V(1, 0), o)
	if err != nil {
		t.Fatal(err)
	}
	want := geom.V(math.Cos(2), math.Sin(2))
	e1, e2 := coarse.Points[len(coarse.Points)-1].Sub(want).Norm(), fine.Points[len(fine.Points)-1].Sub(want).Norm()
	if ratio := e1 / e2; ratio < 14 || ratio > 18 {
		t.Fatalf("RK4 convergence ratio %g, want approximately 16", ratio)
	}
	if fine.Reason != field.StepLimit || fine.Steps != 20 || fine.Time != 2 {
		t.Fatal("wrong step/time bookkeeping", fine)
	}
	for _, p := range fine.Points {
		if !p.Finite() || math.Abs(p.Norm()-1) > 2e-7 {
			t.Fatal("circular flow changed energy", p)
		}
	}
	closeVec(t, fine.Points[len(fine.Points)-1], want, 2e-6)
	o.StepSize = -.1
	back, err := field.Integrate(rotation, geom.V(1, 0), o)
	if err != nil {
		t.Fatal(err)
	}
	closeVec(t, back.Points[len(back.Points)-1], geom.V(math.Cos(2), -math.Sin(2)), 2e-6)
	if back.Time != -2 {
		t.Fatal("backward integration time lost")
	}
}

func TestRK4UsesVelocityMagnitude(t *testing.T) {
	o := field.DefaultStreamOptions()
	o.StepSize, o.MaxSteps = .1, 10
	line, err := field.Integrate(func(p geom.Vec3) geom.Vec3 { return geom.V(p.X, 0) }, geom.V(1, 0), o)
	if err != nil {
		t.Fatal(err)
	}
	closeVec(t, line.Points[len(line.Points)-1], geom.V(math.E, 0), 3e-6)
	if line.Reason != field.StepLimit {
		t.Fatal("unexpected exponential termination", line.Reason)
	}
}

func TestExplicitTerminationsKeepOnlyValidAcceptedPoints(t *testing.T) {
	o := field.DefaultStreamOptions()
	o.Domain = field.Domain{Min: geom.V(-1, -1), Max: geom.V(1, 1)}
	o.StepSize, o.MaxSteps = .3, 10
	calls := 0
	line, err := field.Integrate(func(p geom.Vec3) geom.Vec3 {
		calls++
		if !o.Domain.Contains(p) {
			t.Fatal("evaluated outside domain", p)
		}
		return geom.V(1, 0)
	}, geom.V(0, 0), o)
	if err != nil {
		t.Fatal(err)
	}
	if line.Reason != field.DomainExit || line.Steps != 3 || calls > 4*o.MaxSteps {
		t.Fatal("wrong domain termination", line, calls)
	}
	closeVec(t, line.Points[len(line.Points)-1], geom.V(.9, 0), 1e-12)
	if o.Domain.Contains(line.StopPoint) {
		t.Fatal("missing failed domain stage")
	}
	for _, tc := range []struct {
		name   string
		fn     field.Function
		reason field.Termination
	}{
		{"zero", func(geom.Vec3) geom.Vec3 { return geom.V(0, 0) }, field.Stagnation},
		{"small", func(geom.Vec3) geom.Vec3 { return geom.V(1e-12, 0) }, field.Stagnation},
		{"NaN", func(geom.Vec3) geom.Vec3 { return geom.V(math.NaN(), 0) }, field.InvalidVector},
		{"Z", func(geom.Vec3) geom.Vec3 { return geom.Vec3{Z: 1} }, field.InvalidVector},
		{"stage singularity", func(p geom.Vec3) geom.Vec3 {
			if p.X > 0 {
				return geom.V(math.Inf(1), 0)
			}
			return geom.V(1, 0)
		}, field.InvalidVector},
	} {
		t.Run(tc.name, func(t *testing.T) {
			line, err := field.Integrate(tc.fn, geom.V(0, 0), o)
			if err != nil || line.Reason != tc.reason || line.Steps != 0 || len(line.Points) != 1 {
				t.Fatal(line, err)
			}
			for _, p := range line.Points {
				if !p.Finite() {
					t.Fatal("invalid point escaped into path")
				}
			}
		})
	}
	// Floating-point failure to advance has its own reason, not a false success.
	o.Domain = field.Domain{Min: geom.V(1e20, -1), Max: geom.V(2e20, 1)}
	line, err = field.Integrate(func(geom.Vec3) geom.Vec3 { return geom.V(1, 0) }, geom.V(1e20, 0), o)
	if err != nil || line.Reason != field.NoProgress {
		t.Fatal("loss of precision was not reported", line.Reason, err)
	}
	o = field.DefaultStreamOptions()
	o.StepSize, o.MaxSteps = 1e308, 1
	line, err = field.Integrate(func(geom.Vec3) geom.Vec3 { return geom.V(10, 0) }, geom.V(0, 0), o)
	if err != nil || line.Reason != field.InvalidPoint {
		t.Fatal("overflow was not reported", line.Reason, err)
	}
}

func TestStreamConfigurationAndSeedValidation(t *testing.T) {
	fn := func(geom.Vec3) geom.Vec3 { return geom.V(1, 0) }
	for _, mutate := range []func(*field.StreamOptions){
		func(o *field.StreamOptions) { o.StepSize = 0 },
		func(o *field.StreamOptions) { o.StepSize = math.NaN() },
		func(o *field.StreamOptions) { o.MaxSteps = 0 },
		func(o *field.StreamOptions) { o.MaxSteps = 100001 },
		func(o *field.StreamOptions) { o.MinSpeed = -1 },
		func(o *field.StreamOptions) { o.Domain.Min.Z = 1 },
		func(o *field.StreamOptions) { o.Domain.Max = o.Domain.Min },
		func(o *field.StreamOptions) { o.Color.A = math.Inf(1) },
	} {
		o := field.DefaultStreamOptions()
		mutate(&o)
		if _, err := field.NewStreamLines(fn, []geom.Vec3{geom.V(0, 0)}, o); err == nil {
			t.Fatal("invalid streamline options accepted")
		}
	}
	for _, seed := range []geom.Vec3{geom.V(10, 0), geom.V(math.NaN(), 0), {Z: 1}} {
		if _, err := field.NewStreamLines(fn, []geom.Vec3{seed}, field.DefaultStreamOptions()); err == nil {
			t.Fatal("invalid seed accepted", seed)
		}
	}
	o := field.DefaultStreamOptions()
	o.MaxSteps = 100_000
	calls := 0
	if _, err := field.NewStreamLines(func(geom.Vec3) geom.Vec3 { calls++; return geom.V(1, 0) }, make([]geom.Vec3, 11), o); err == nil || calls != 0 {
		t.Fatal("step budget exceeded before evaluation", calls, err)
	}
	if _, err := field.Integrate(nil, geom.V(0, 0), o); err == nil {
		t.Fatal("nil function accepted")
	}
	s, err := field.NewStreamLines(fn, nil, field.DefaultStreamOptions())
	if err != nil || len(s.Lines()) != 0 {
		t.Fatal("empty seed set failed", err)
	}
	// A singular seed has a reported termination even when using a color map.
	o = field.DefaultStreamOptions()
	o.ColorFunc = func(_, _ geom.Vec3, _ float64) gomath.Color {
		t.Fatal("invalid vector passed to color callback")
		return gomath.Red
	}
	s, err = field.NewStreamLines(func(geom.Vec3) geom.Vec3 { return geom.V(math.NaN(), 0) }, []geom.Vec3{geom.V(0, 0)}, o)
	if err != nil || s.Lines()[0].Reason != field.InvalidVector {
		t.Fatal("singular seed lost termination", err)
	}
}

func TestStreamSeedColorsAndSingularityAfterAcceptedSteps(t *testing.T) {
	o := field.DefaultStreamOptions()
	o.StepSize, o.MaxSteps = .1, 10
	var err error
	o.ColorFunc, err = field.SpeedColor(gomath.Blue, gomath.Red, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	s, err := field.NewStreamLines(func(p geom.Vec3) geom.Vec3 {
		if p.X > .36 {
			return geom.V(math.NaN(), 0)
		}
		return geom.V(1, 0)
	}, []geom.Vec3{geom.V(0, 0)}, o)
	if err != nil {
		t.Fatal(err)
	}
	line := s.Lines()[0]
	if line.Reason != field.InvalidVector || line.Steps != 3 || len(line.Points) != 4 || line.Object.Style.Stroke != gomath.Red {
		t.Fatal("accepted prefix, seed color or termination was lost", line)
	}
	if err := s.Object.Validate(); err != nil {
		t.Fatal("singularity escaped into geometry", err)
	}
	closeVec(t, line.Points[3], geom.V(.3, 0), 1e-12)
	closeVec(t, line.StopPoint, geom.V(.4, 0), 1e-12)
	o.ColorFunc = func(_, _ geom.Vec3, _ float64) gomath.Color { return gomath.Color{R: math.NaN()} }
	if _, err := field.NewStreamLines(func(geom.Vec3) geom.Vec3 { return geom.V(1, 0) }, []geom.Vec3{geom.V(0, 0)}, o); err == nil {
		t.Fatal("invalid line color accepted")
	}
}

func TestStreamObjectsAndSceneRenderAreIndependent(t *testing.T) {
	o := field.DefaultStreamOptions()
	o.MaxSteps, o.StepSize = 20, .05
	rotation := func(p geom.Vec3) geom.Vec3 { return geom.V(-p.Y, p.X) }
	s, err := field.NewStreamLines(rotation, []geom.Vec3{geom.V(1, 0), geom.V(2, 0)}, o)
	if err != nil {
		t.Fatal(err)
	}
	s2, err := field.NewStreamLines(rotation, []geom.Vec3{geom.V(1, 0), geom.V(2, 0)}, o)
	if err != nil {
		t.Fatal(err)
	}
	lines := s.Lines()
	for i, line := range lines {
		if !reflect.DeepEqual(line.Points, s2.Lines()[i].Points) {
			t.Fatal("integration is not deterministic")
		}
		if line.Object == s2.Lines()[i].Object || len(line.Object.Path.Contours) != 1 || len(line.Object.Path.Contours[0].Curves) != line.Steps {
			t.Fatal("line ownership/path topology incorrect")
		}
	}
	lines[0].Points[0].X = 500
	if s.Lines()[0].Points[0].X == 500 {
		t.Fatal("numerical records alias internal state")
	}
	v, err := field.NewVectorField(rotation, field.Range{-1, 1, 1}, field.Range{-1, 1, 1}, field.DefaultVectorOptions())
	if err != nil {
		t.Fatal(err)
	}
	scene := gomath.NewScene()
	if err := scene.Add(s.Object, s2.Object, v.Object); err != nil {
		t.Fatal("independent trees rejected", err)
	}
	if err := scene.Play(1, gomath.Shift(v.Object, geom.V(0, -.5))); err != nil {
		t.Fatal(err)
	}
	c := render.DefaultConfig()
	c.Width, c.Height, c.FrameWidth, c.Background = 128, 128, 6, gomath.Black
	r, err := render.NewRasterizer(c)
	if err != nil {
		t.Fatal(err)
	}
	f, err := scene.Sample(.5)
	if err != nil {
		t.Fatal(err)
	}
	img, err := r.Render(f)
	if err != nil {
		t.Fatal(err)
	}
	lit := 0
	for i := 0; i < len(img.Pix); i += 4 {
		if img.Pix[i]+img.Pix[i+1]+img.Pix[i+2] > 0 {
			lit++
		}
	}
	if lit < 100 {
		t.Fatal("sampled scene contains no visible field/lines", lit)
	}
	want := append([]byte(nil), img.Pix...)
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			f, err := scene.Sample(.5)
			if err != nil {
				t.Error(err)
				return
			}
			// Rasterizers have their own scratch state; only the scene is shared.
			r, err := render.NewRasterizer(c)
			if err != nil {
				t.Error(err)
				return
			}
			img, err := r.Render(f)
			if err != nil {
				t.Error(err)
				return
			}
			if !bytes.Equal(want, img.Pix) {
				t.Error("concurrent sampling changed pixels")
			}
		})
	}
	wg.Wait()
}

func BenchmarkRK4StreamLines(b *testing.B) {
	seeds := make([]geom.Vec3, 25)
	for i := range seeds {
		seeds[i] = geom.V(.1+float64(i)*.05, 0)
	}
	o := field.DefaultStreamOptions()
	o.MaxSteps, o.StepSize = 200, .02
	fn := func(p geom.Vec3) geom.Vec3 { return geom.V(-p.Y, p.X) }
	b.ReportAllocs()
	for b.Loop() {
		if _, err := field.NewStreamLines(fn, seeds, o); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkVectorField(b *testing.B) {
	fn := func(p geom.Vec3) geom.Vec3 { return geom.V(-p.Y, p.X) }
	b.ReportAllocs()
	for b.Loop() {
		if _, err := field.NewVectorField(fn, field.Range{-2, 2, .2}, field.Range{-2, 2, .2}, field.DefaultVectorOptions()); err != nil {
			b.Fatal(err)
		}
	}
}
