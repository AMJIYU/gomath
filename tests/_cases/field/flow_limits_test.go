package field_test

import (
	"context"
	"errors"
	"gomath"
	"gomath/field"
	"gomath/geom"
	"gomath/path"
	"math"
	"strings"
	"testing"
)

func TestFlowExactBudgetBoundariesAndSourceRejection(t *testing.T) {
	line := flowInput(0, 1)
	o := field.DefaultFlowOptions()
	// One line, one cubic: fixed reserve4096 + line1024 + cubic256.
	o.Limits.MaxStorageBytes = 5376
	o.Limits.MaxLines = 1
	o.Limits.MaxSegments = 1
	o.Limits.MaxSampleBytes = 5248
	o.Limits.MaxSampleSegments = 1
	o.Limits.MaxSampleWork = 11 // 2 visits + 4*bits.Len(2) comparisons + curve
	f, e := field.NewFlow([]field.FlowLine{line}, o)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.Sample(.5); e != nil {
		t.Fatal("exact sample limits", e)
	}
	for _, mut := range []func(*field.FlowOptions){func(o *field.FlowOptions) { o.Limits.MaxSampleBytes-- }, func(o *field.FlowOptions) { o.Limits.MaxSampleSegments-- }, func(o *field.FlowOptions) { o.Limits.MaxSampleWork-- }} {
		q := o
		mut(&q)
		f, e := field.NewFlow([]field.FlowLine{line}, q)
		if e != nil {
			t.Fatal(e)
		}
		if fr, e := f.Sample(.5); fr != nil || !errors.Is(e, field.ErrFlowLimit) {
			t.Fatal("sample limit not strict", fr, e)
		}
	}
	o.Limits.MaxStorageBytes--
	if f, e := field.NewFlow([]field.FlowLine{line}, o); f != nil || !errors.Is(e, field.ErrFlowLimit) {
		t.Fatal("storage off by one")
	}
	o = field.DefaultFlowOptions()
	line.Object.Name = strings.Repeat("x", 1024)
	o.Limits.MaxStorageBytes = 5376 + 1023
	if f, e := field.NewFlow([]field.FlowLine{line}, o); f != nil || !errors.Is(e, field.ErrFlowLimit) {
		t.Fatal("names not budgeted")
	}
	o = field.DefaultFlowOptions()
	if f, e := field.NewFlow([]field.FlowLine{{Times: []float64{0, 1}}}, o); f != nil || !errors.Is(e, field.ErrFlowOptions) {
		t.Fatal("nil source")
	}
	for _, mut := range []func(*gomath.Object){
		func(p *gomath.Object) { p.Style.StrokeWidth = math.Inf(1) },
		func(p *gomath.Object) { p.Scalar = math.NaN() },
		func(p *gomath.Object) { p.Path.Contours[0].Curves[0].P1.X = math.Inf(1) },
		func(p *gomath.Object) { p.Style.Dash, _ = gomath.NewDashPattern([]float64{1, 1}, 0) },
	} {
		line := flowInput(0, 1)
		mut(line.Object)
		if f, e := field.NewFlow([]field.FlowLine{line}, o); f != nil || e == nil {
			t.Fatal("invalid geometry/style", f, e)
		}
	}
	// Zero-span geometric segments remain timed; they must not be silently removed.
	obj := gomath.NewObject(path.Path{Contours: []path.Contour{{Start: geom.V(1, 2), Curves: []path.Cubic{path.Line(geom.V(1, 2), geom.V(1, 2))}}}})
	f, e = field.NewFlow([]field.FlowLine{{Times: []float64{0, 1}, Object: obj}}, o)
	if e != nil {
		t.Fatal(e)
	}
	if len(flowCurves(t, f, .5)) != 1 {
		t.Fatal("timed degenerate segment removed")
	}
}
func TestFlowSourceWrappersRespectGroupContentAndBounds(t *testing.T) {
	fn := func(geom.Vec3) geom.Vec3 { return geom.V(1, 0) }
	ro := field.DefaultStreamOptions()
	ro.MaxSteps = 2
	o := field.DefaultFlowOptions()
	r, e := field.NewStreamLines(fn, nil, ro)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = r.NewFlow(o); e != nil {
		t.Fatal("empty RK4", e)
	}
	ao := field.DefaultAdaptiveStreamOptions()
	ao.Integration.EndTime = .125
	ao.Integration.InitialStep = .125
	a, e := field.NewAdaptiveStreamLines(context.Background(), fn, nil, ao)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = a.NewFlow(o); e != nil {
		t.Fatal("empty adaptive", e)
	}
	for _, adaptive := range []bool{false, true} {
		var makeFlow func(field.FlowOptions) (*field.Flow, error)
		var root *gomath.Object
		if adaptive {
			a, e = field.NewAdaptiveStreamLines(context.Background(), fn, []geom.Vec3{geom.V(0, 0)}, ao)
			if e != nil {
				t.Fatal(e)
			}
			makeFlow = a.NewFlow
			root = a.Object
		} else {
			r, e = field.NewStreamLines(fn, []geom.Vec3{geom.V(0, 0)}, ro)
			if e != nil {
				t.Fatal(e)
			}
			makeFlow = r.NewFlow
			root = r.Object
		}
		q := o
		q.Limits.MaxLines = 0
		if f, e := makeFlow(q); f != nil || !errors.Is(e, field.ErrFlowLimit) {
			t.Fatal("wrapper line cap")
		}
		q = o
		q.Limits.MaxStorageBytes = 4096
		if f, e := makeFlow(q); f != nil || !errors.Is(e, field.ErrFlowLimit) {
			t.Fatal("wrapper storage cap")
		}
		q = o
		q.Limits.MaxSegments = 0
		if f, e := makeFlow(q); f != nil || !errors.Is(e, field.ErrFlowLimit) {
			t.Fatal("wrapper segment cap")
		}
		root.Name = "parent label"
		root.ZIndex = 7
		f, e := makeFlow(o)
		if e != nil {
			t.Fatal(e)
		}
		fr, e := f.Sample(.05)
		if e != nil || fr.Objects[0].Name != root.Name || fr.Objects[0].ZIndex != 7 {
			t.Fatal("root metadata not captured")
		}
		root.Path = gomath.Polyline(geom.V(0, 0), geom.V(1, 1)).Path
		if f, e := makeFlow(o); f != nil || !errors.Is(e, field.ErrFlowUnsupported) {
			t.Fatal("root visible content discarded")
		}
		root.Path = path.Path{}
		child := root.Children[0]
		root.Children[0] = gomath.Polyline(geom.V(0, 0), geom.V(1, 1))
		if f, e := makeFlow(o); f != nil || e == nil {
			t.Fatal("replaced child ignored")
		}
		root.Children = []*gomath.Object{child, gomath.Group()}
		if f, e := makeFlow(o); f != nil || !errors.Is(e, field.ErrFlowUnsupported) {
			t.Fatal("additional children discarded")
		}
	}
}

func TestFlowNumericalClockRangeIsExplicit(t *testing.T) {
	o := field.DefaultFlowOptions()
	o.Duration = .25
	o.FlowSpeed = math.SmallestNonzeroFloat64
	if f, e := field.NewFlow(nil, o); f != nil || !errors.Is(e, field.ErrFlowRange) {
		t.Fatal("duration product underflow", f, e)
	}
	o = field.DefaultFlowOptions()
	o.FlowSpeed = .5
	f, e := field.NewFlow([]field.FlowLine{flowInput(0, 1)}, o)
	if e != nil {
		t.Fatal(e)
	}
	if fr, e := f.Sample(math.SmallestNonzeroFloat64); fr != nil || !errors.Is(e, field.ErrFlowRange) {
		t.Fatal("sample product underflow", fr, e)
	}
	o = field.DefaultFlowOptions()
	o.FlowSpeed = -1
	f, e = field.NewFlow([]field.FlowLine{flowInput(0, 1)}, o)
	if e != nil {
		t.Fatal(e)
	}
	if fr, e := f.Sample(math.SmallestNonzeroFloat64); fr != nil || !errors.Is(e, field.ErrFlowRange) {
		t.Fatal("reverse clock precision loss", fr, e)
	}
	o = field.DefaultFlowOptions()
	o.TraceTimeWidth = math.SmallestNonzeroFloat64
	if f, e := field.NewFlow([]field.FlowLine{flowInput(0, 1)}, o); f != nil || !errors.Is(e, field.ErrFlowRange) {
		t.Fatal("width lost in period", f, e)
	}
	// Ratio to a very long first interval cannot be represented, even when its
	// physical endpoints and the global clock themselves are representable.
	o = field.DefaultFlowOptions()
	o.TraceTimeWidth = 1e300
	f, e = field.NewFlow([]field.FlowLine{flowInput(0, 1e300)}, o)
	if e != nil {
		t.Fatal(e)
	}
	if fr, e := f.Sample(math.SmallestNonzeroFloat64); fr != nil || !errors.Is(e, field.ErrFlowRange) {
		t.Fatal("curve fraction underflow", fr, e)
	}
}

func TestFlowZeroValueAndInvisibleClock(t *testing.T) {
	var zero field.Flow
	if fr, e := zero.Sample(0); fr != nil || !errors.Is(e, field.ErrFlowOptions) {
		t.Fatal("uninitialized Flow returned invalid frame")
	}
	o := field.DefaultFlowOptions()
	o.Loop = true
	o.TraceTimeWidth = math.SmallestNonzeroFloat64
	f, e := field.NewFlow([]field.FlowLine{flowInput(2)}, o)
	if e != nil {
		t.Fatal("invisible seed need not evaluate modulo", e)
	}
	if flowCurves(t, f, .5) != nil {
		t.Fatal("zero span became visible")
	}
}
