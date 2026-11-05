package field_test

import (
	"bytes"
	"context"
	"errors"
	"gomath"
	"gomath/field"
	"gomath/geom"
	"gomath/path"
	"gomath/render"
	"math"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
)

func flowInput(times ...float64) field.FlowLine {
	p := make([]geom.Vec3, len(times))
	for i := range p {
		p[i] = geom.V(float64(i), 0)
	}
	return field.FlowLine{Times: times, Object: gomath.Polyline(p...).SetStroke(gomath.Red, .08)}
}
func flowCurves(t *testing.T, f *field.Flow, at float64) []path.Cubic {
	t.Helper()
	fr, e := f.Sample(at)
	if e != nil {
		t.Fatal(e)
	}
	p := fr.Objects[0].Children[0].Path
	if len(p.Contours) == 0 {
		return nil
	}
	return p.Contours[0].Curves
}
func TestFlowPhysicalWindowsDirectionDelayAndLoop(t *testing.T) {
	for _, times := range [][]float64{{4, 4.25, 5, 6}, {4, 3.75, 3, 2}} {
		line := flowInput(times...)
		o := field.DefaultFlowOptions()
		o.Duration = 8
		o.TraceTimeWidth = .5
		o.StartDelays = []float64{.25}
		for _, speed := range []float64{1, -1, 2, -2} {
			o.FlowSpeed = speed
			for _, loop := range []bool{false, true} {
				o.Loop = loop
				f, e := field.NewFlow([]field.FlowLine{line}, o)
				if e != nil {
					t.Fatal(e)
				}
				if flowCurves(t, f, 0) != nil || flowCurves(t, f, .25) != nil {
					t.Fatal("delay not empty")
				}
				at := .25 + .5/math.Abs(speed)
				c := flowCurves(t, f, at)
				a, b := 0.0, 1+(.5-.25)/.75
				if speed < 0 {
					a, b = 2.5, 3
				}
				if len(c) == 0 || math.Abs(c[0].P0.X-a) > 1e-14 || math.Abs(c[len(c)-1].P3.X-b) > 1e-14 {
					t.Fatalf("physical window speed %g: %v", speed, c)
				}
				cycle := 2.5 / math.Abs(speed)
				if flowCurves(t, f, .25+cycle) != nil {
					t.Fatal("endpoint must be empty")
				}
				later := flowCurves(t, f, .25+cycle+.5/math.Abs(speed))
				if loop && !reflect.DeepEqual(c, later) || !loop && len(later) != 0 {
					t.Fatal("loop/drain mismatch")
				}
			}
		}
	}
}
func TestFlowRK4AdaptiveActualTimesAndAuthoredSnapshot(t *testing.T) {
	var calls, colors atomic.Int64
	fn := func(geom.Vec3) geom.Vec3 { calls.Add(1); return geom.V(1, 0) }
	co := func(_, _ geom.Vec3, _ float64) gomath.Color { colors.Add(1); return gomath.Red }
	ro := field.DefaultStreamOptions()
	ro.StepSize = -.125
	ro.MaxSteps = 8
	ro.ColorFunc = co
	r, e := field.NewStreamLines(fn, []geom.Vec3{geom.V(1, 0)}, ro)
	if e != nil {
		t.Fatal(e)
	}
	ao := field.DefaultAdaptiveStreamOptions()
	ao.Integration = adaptiveOptions()
	ao.Integration.StartTime = 2
	ao.Integration.EndTime = 1
	ao.Integration.InitialStep = .03125
	ao.Integration.MaxStep = .25
	ao.ColorFunc = co
	a, e := field.NewAdaptiveStreamLines(context.Background(), fn, []geom.Vec3{geom.V(1, 0)}, ao)
	if e != nil {
		t.Fatal(e)
	}
	r.Object.Shift(geom.V(0, 1)).SetOpacity(.5)
	a.Object.Shift(geom.V(0, 1)).SetOpacity(.5)
	o := field.DefaultFlowOptions()
	o.TraceTimeWidth = .2
	rf, e := r.NewFlow(o)
	if e != nil {
		t.Fatal(e)
	}
	af, e := a.NewFlow(o)
	if e != nil {
		t.Fatal(e)
	}
	before := calls.Load()
	r.Object.Shift(geom.V(5, 5))
	a.Object.SetColor(gomath.Blue)
	for _, f := range []*field.Flow{rf, af} {
		c := flowCurves(t, f, .57)
		if len(c) == 0 {
			t.Fatalf("empty window: rk4=%+v adaptive times=%v reason=%v", r.Lines()[0], a.Lines()[0].Result.Times(), a.Lines()[0].Result.Reason())
		}
		closeVec(t, c[0].P0, geom.V(.63, 1), 1e-13)
		closeVec(t, c[len(c)-1].P3, geom.V(.43, 1), 1e-13)
		fr, _ := f.Sample(.57)
		if fr.Objects[0].Style.Opacity != .5 || fr.Objects[0].Children[0].Style.Stroke != gomath.Red {
			t.Fatal("style not snapshotted")
		}
	}
	if calls.Load() != before || colors.Load() != 2 {
		t.Fatal("sampling evaluated callbacks")
	}
	// Adaptive physical times are genuinely nonuniform.
	ts := a.Lines()[0].Result.Times()
	if len(ts) < 3 || math.Abs((ts[1]-ts[0])-(ts[2]-ts[1])) < 1e-6 {
		t.Fatal("fixture lacks unequal time spans")
	}
}
func TestFlowOwnershipConcurrentOutOfOrderAndPixels(t *testing.T) {
	in := flowInput(0, .25, 1, 2)
	o := field.DefaultFlowOptions()
	o.TraceTimeWidth = .5
	o.StartDelays = []float64{.125}
	f, e := field.NewFlow([]field.FlowLine{in}, o)
	if e != nil {
		t.Fatal(e)
	}
	in.Times[1] = 999
	in.Object.Shift(geom.V(3, 3))
	o.StartDelays[0] = 999
	want, e := f.Sample(.625)
	if e != nil {
		t.Fatal(e)
	}
	cfg := render.DefaultConfig()
	cfg.Width = 128
	cfg.Height = 96
	cfg.FrameWidth = 6
	draw := func(fr *gomath.Frame) []byte {
		rr, e := render.NewRasterizer(cfg)
		if e != nil {
			t.Error(e)
			return nil
		}
		im, e := rr.Render(fr)
		if e != nil {
			t.Error(e)
			return nil
		}
		return im.Pix
	}
	pixels := draw(want)
	expected := gomath.Polyline(geom.V(0, 0), geom.V(1, 0), geom.V(1+1.0/3, 0)).SetStroke(gomath.Red, .08)
	if !bytes.Equal(pixels, draw(&gomath.Frame{Objects: []*gomath.Object{expected}})) {
		t.Fatal("visible flow pixels differ from independent physical window")
	}
	lit := 0
	for i := 0; i < len(pixels); i += 4 {
		if pixels[i] > 0 {
			lit++
		}
	}
	if lit < 20 {
		t.Fatal("invisible flow")
	}
	var wg sync.WaitGroup
	for k := range 16 {
		wg.Go(func() {
			for j := range 10 {
				at := float64((k*7+j*3)%32) / 8
				_, e := f.Sample(at)
				if e != nil {
					t.Error(e)
				}
				fr, e := f.Sample(.625)
				if e != nil {
					t.Error(e)
					return
				}
				if !reflect.DeepEqual(want, fr) || !bytes.Equal(pixels, draw(fr)) {
					t.Error("out-of-order snapshot changed")
				}
				fr.Objects[0].Children[0].Path.Contours[0].Curves[0].P0.X = 999
				fr.Objects[0].Children[0].Style.Opacity = 0
			}
		})
	}
	wg.Wait()
	fr, _ := f.Sample(.625)
	if !reflect.DeepEqual(want, fr) {
		t.Fatal("caller frame mutation leaked")
	}
	if fr.Objects[0].Children[0].ID == in.Object.ID {
		t.Fatal("flow retained source identity")
	}
}
func TestFlowInvalidUnsupportedBudgetsAndEmpty(t *testing.T) {
	for _, mut := range []func(*field.FlowOptions){
		func(o *field.FlowOptions) { o.Duration = -1 }, func(o *field.FlowOptions) { o.Duration = math.Inf(1) }, func(o *field.FlowOptions) { o.FlowSpeed = 0 }, func(o *field.FlowOptions) { o.FlowSpeed = math.NaN() }, func(o *field.FlowOptions) { o.TraceTimeWidth = 0 }, func(o *field.FlowOptions) { o.StartDelays = []float64{-1} }, func(o *field.FlowOptions) { o.StartDelays = []float64{0, 1} }, func(o *field.FlowOptions) { o.Limits.MaxLines = -1 }, func(o *field.FlowOptions) { o.Limits.MaxStorageBytes = 0 }, func(o *field.FlowOptions) { o.Limits.MaxSampleWork = -1 },
	} {
		o := field.DefaultFlowOptions()
		mut(&o)
		f, e := field.NewFlow([]field.FlowLine{flowInput(0, 1)}, o)
		if f != nil || !errors.Is(e, field.ErrFlowOptions) {
			t.Fatalf("options: %v %v", f, e)
		}
	}
	for _, ts := range [][]float64{nil, {0}, {0, 0}, {0, math.NaN()}, {0, 1, .5}} {
		line := flowInput(0, 1)
		line.Times = ts
		f, e := field.NewFlow([]field.FlowLine{line}, field.DefaultFlowOptions())
		if f != nil || e == nil {
			t.Fatal("invalid times accepted", ts)
		}
	}
	for _, mut := range []func(*gomath.Object){func(o *gomath.Object) { o.Children = []*gomath.Object{o} }, func(o *gomath.Object) { o.Path.Contours[0].Closed = true }, func(o *gomath.Object) { o.Style.Fill = gomath.Red }, func(o *gomath.Object) { o.Style.Reveal = .5 }, func(o *gomath.Object) { o.Bitmap = &gomath.Bitmap{} }, func(o *gomath.Object) { o.Clip = &gomath.Clip{} }, func(o *gomath.Object) { o.Path.Contours[0].Curves[0].P0.X = .1 }, func(o *gomath.Object) { o.Path.Contours = append(o.Path.Contours, path.Contour{}) }} {
		line := flowInput(0, 1)
		mut(line.Object)
		f, e := field.NewFlow([]field.FlowLine{line}, field.DefaultFlowOptions())
		if f != nil || e == nil {
			t.Fatal("unsupported content accepted")
		}
	}
	for _, which := range []string{"lines", "segments", "storage"} {
		o := field.DefaultFlowOptions()
		switch which {
		case "lines":
			o.Limits.MaxLines = 0
		case "segments":
			o.Limits.MaxSegments = 0
		case "storage":
			o.Limits.MaxStorageBytes = 4096
		}
		f, e := field.NewFlow([]field.FlowLine{flowInput(0, 1)}, o)
		if f != nil || !errors.Is(e, field.ErrFlowLimit) {
			t.Fatal(which, f, e)
		}
	}
	for _, which := range []string{"segments", "work", "bytes"} {
		o := field.DefaultFlowOptions()
		switch which {
		case "segments":
			o.Limits.MaxSampleSegments = 0
		case "work":
			o.Limits.MaxSampleWork = 0
		case "bytes":
			o.Limits.MaxSampleBytes = 4096
		}
		f, e := field.NewFlow([]field.FlowLine{flowInput(0, 1)}, o)
		if e != nil {
			t.Fatal(e)
		}
		fr, e := f.Sample(.5)
		if fr != nil || !errors.Is(e, field.ErrFlowLimit) {
			t.Fatal(which, fr, e)
		}
	}
	o := field.DefaultFlowOptions()
	f, e := field.NewFlow(nil, o)
	if e != nil {
		t.Fatal(e)
	}
	fr, e := f.Sample(0)
	if e != nil || len(fr.Objects) != 1 || len(fr.Objects[0].Children) != 0 {
		t.Fatal("empty flow", e)
	}
	f, e = field.NewFlow([]field.FlowLine{flowInput(3)}, o)
	if e != nil || flowCurves(t, f, .5) != nil {
		t.Fatal("degenerate seed", e)
	}
	var nilf *field.Flow
	if nilf.Duration() != 0 {
		t.Fatal("nil duration")
	}
	if fr, e := nilf.Sample(0); fr != nil || e == nil {
		t.Fatal("nil sample")
	}
	for _, at := range []float64{-1, 6, math.NaN(), math.Inf(1)} {
		if fr, e := f.Sample(at); fr != nil || e == nil {
			t.Fatal("invalid time", at)
		}
	}
	o.Duration = math.MaxFloat64
	o.FlowSpeed = 2
	if f, e := field.NewFlow(nil, o); f != nil || !errors.Is(e, field.ErrFlowRange) {
		t.Fatal("overflow", e)
	}
	o = field.DefaultFlowOptions()
	o.Loop = true
	o.TraceTimeWidth = 1e-300
	if f, e := field.NewFlow([]field.FlowLine{flowInput(0, 1e-300)}, o); f != nil || !errors.Is(e, field.ErrFlowRange) {
		t.Fatal("cycle precision", e)
	}
	if f, e := field.NewFlow([]field.FlowLine{flowInput(-math.MaxFloat64, math.MaxFloat64)}, field.DefaultFlowOptions()); f != nil || !errors.Is(e, field.ErrFlowRange) {
		t.Fatal("time difference overflow", e)
	}
	o = field.DefaultFlowOptions()
	o.Duration = 0
	f, e = field.NewFlow([]field.FlowLine{flowInput(0, 1)}, o)
	if e != nil || flowCurves(t, f, 0) != nil {
		t.Fatal("zero duration")
	}
	var rs *field.StreamLines
	var as *field.AdaptiveStreamLines
	if f, e := rs.NewFlow(o); f != nil || e == nil {
		t.Fatal("nil RK4")
	}
	if f, e := as.NewFlow(o); f != nil || e == nil {
		t.Fatal("nil adaptive")
	}
}

func TestFlowIndependentLineClocksAndAggregateSampleBudget(t *testing.T) {
	o := field.DefaultFlowOptions()
	o.TraceTimeWidth = .5
	o.Loop = true
	o.StartDelays = []float64{.25, .5}
	inputs := []field.FlowLine{flowInput(0, .5, 1), flowInput(4, 5.5, 6)}
	f, e := field.NewFlow(inputs, o)
	if e != nil {
		t.Fatal(e)
	}
	curve := func(at float64, line int) []path.Cubic {
		fr, e := f.Sample(at)
		if e != nil {
			t.Fatal(e)
		}
		p := fr.Objects[0].Children[line].Path
		if len(p.Contours) == 0 {
			return nil
		}
		return p.Contours[0].Curves
	}
	a, b := curve(.75, 0), curve(.75, 1)
	closeVec(t, a[0].P0, geom.V(0, 0), 1e-14)
	closeVec(t, a[len(a)-1].P3, geom.V(1, 0), 1e-14)
	closeVec(t, b[0].P0, geom.V(0, 0), 1e-14)
	closeVec(t, b[len(b)-1].P3, geom.V(1.0/6, 0), 1e-14)
	if !reflect.DeepEqual(a, curve(2.25, 0)) {
		t.Fatal("line period used another line's duration")
	}
	b = curve(2.25, 1)
	closeVec(t, b[0].P0, geom.V(5.0/6, 0), 1e-14)
	closeVec(t, b[len(b)-1].P3, geom.V(1.5, 0), 1e-14)
	o.Limits.MaxSampleSegments = 2
	limited, e := field.NewFlow(inputs, o)
	if e != nil {
		t.Fatal(e)
	}
	if _, e := limited.Sample(.75); e != nil {
		t.Fatal("two segment window", e)
	}
	if fr, e := limited.Sample(2.25); fr != nil || !errors.Is(e, field.ErrFlowLimit) {
		t.Fatal("aggregate window budget failed", fr, e)
	}
}
