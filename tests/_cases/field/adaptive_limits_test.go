package field_test

import (
	"context"
	"gomath/field"
	"gomath/geom"
	"math"
	"testing"
)

func TestAdaptiveMalformedPreflight(t *testing.T) {
	seed := geom.V(1, 0)
	ctx := context.Background()
	calls := 0
	fn := func(geom.Vec3) geom.Vec3 { calls++; return geom.V(1, 0) }
	for _, mutate := range []func(*field.AdaptiveOptions){
		func(o *field.AdaptiveOptions) { o.Domain.Min.Z = 1 }, func(o *field.AdaptiveOptions) { o.Domain.Max = o.Domain.Min },
		func(o *field.AdaptiveOptions) { o.StartTime = math.NaN() }, func(o *field.AdaptiveOptions) { o.EndTime = math.Inf(1) },
		func(o *field.AdaptiveOptions) { o.StartTime = -1e308; o.EndTime = 1e308 },
		func(o *field.AdaptiveOptions) { o.InitialStep = 0 }, func(o *field.AdaptiveOptions) { o.InitialStep = -1 }, func(o *field.AdaptiveOptions) { o.InitialStep = math.NaN() },
		func(o *field.AdaptiveOptions) { o.MinStep = 0 }, func(o *field.AdaptiveOptions) { o.MinStep = o.InitialStep * 2 }, func(o *field.AdaptiveOptions) { o.MaxStep = o.InitialStep / 2 }, func(o *field.AdaptiveOptions) { o.MaxStep = math.Inf(1) },
		func(o *field.AdaptiveOptions) { o.AbsoluteTolerance = -1 }, func(o *field.AdaptiveOptions) { o.RelativeTolerance = math.NaN() }, func(o *field.AdaptiveOptions) { o.RelativeTolerance = math.Inf(1) }, func(o *field.AdaptiveOptions) { o.AbsoluteTolerance = 0; o.RelativeTolerance = 0 },
		func(o *field.AdaptiveOptions) { o.MinSpeed = -1 }, func(o *field.AdaptiveOptions) { o.MinSpeed = math.NaN() },
		func(o *field.AdaptiveOptions) { o.Limits.MaxAcceptedSteps = -1 }, func(o *field.AdaptiveOptions) { o.Limits.MaxAcceptedSteps = 1000001 }, func(o *field.AdaptiveOptions) { o.Limits.MaxRejectedSteps = -1 }, func(o *field.AdaptiveOptions) { o.Limits.MaxRejectedSteps = 1000001 }, func(o *field.AdaptiveOptions) { o.Limits.MaxEvaluations = -1 }, func(o *field.AdaptiveOptions) { o.Limits.MaxEvaluations = 8000001 }, func(o *field.AdaptiveOptions) { o.Limits.MaxStorageBytes = 512<<20 + 1 },
	} {
		o := adaptiveOptions()
		mutate(&o)
		adaptiveFailure(t, ctx, fn, seed, o, field.ErrAdaptiveOptions)
	}
	for _, seed := range []geom.Vec3{geom.V(20, 0), geom.V(math.NaN(), 0), {Z: 1}} {
		adaptiveFailure(t, ctx, fn, seed, adaptiveOptions(), field.ErrAdaptiveOptions)
	}
	adaptiveFailure(t, nil, fn, seed, adaptiveOptions(), field.ErrAdaptiveOptions)
	adaptiveFailure(t, ctx, nil, seed, adaptiveOptions(), field.ErrAdaptiveOptions)
	if calls != 0 {
		t.Fatal("malformed call reached field", calls)
	}
}

func TestAdaptiveExactBudgetsAndNoPartialResult(t *testing.T) {
	ctx := context.Background()
	o := adaptiveOptions()
	o.EndTime = .125
	o.InitialStep = .125
	o.Limits = field.AdaptiveLimits{MaxAcceptedSteps: 1, MaxRejectedSteps: 0, MaxEvaluations: 7, MaxStorageBytes: 64<<10 + 64}
	calls := 0
	fn := func(geom.Vec3) geom.Vec3 { calls++; return geom.V(1, 0) }
	r, e := field.IntegrateAdaptive(ctx, fn, geom.V(0, 0), o)
	if e != nil || r.Stats() != (field.AdaptiveStats{AcceptedSteps: 1, Evaluations: 7}) || calls != 7 {
		t.Fatalf("exact limits %v %v %d", r, e, calls)
	}
	q := o
	q.Limits.MaxStorageBytes--
	calls = 0
	adaptiveFailure(t, ctx, fn, geom.V(0, 0), q, field.ErrAdaptiveLimit)
	if calls != 0 {
		t.Fatal("storage preflight called field")
	}
	q = o
	q.Limits.MaxAcceptedSteps = 0
	calls = 0
	adaptiveFailure(t, ctx, fn, geom.V(0, 0), q, field.ErrAdaptiveLimit)
	if calls != 0 {
		t.Fatal("accepted zero budget called field")
	}
	q = o
	q.Limits.MaxEvaluations = 6
	calls = 0
	adaptiveFailure(t, ctx, fn, geom.V(0, 0), q, field.ErrAdaptiveLimit)
	if calls != 6 {
		t.Fatal("evaluation cap exceeded", calls)
	}
	q = o
	q.Limits.MaxEvaluations = 0
	calls = 0
	adaptiveFailure(t, ctx, fn, geom.V(0, 0), q, field.ErrAdaptiveLimit)
	if calls != 0 {
		t.Fatal("zero evaluation budget called field")
	}
	q = o
	q.EndTime = .5
	adaptiveFailure(t, ctx, fn, geom.V(0, 0), q, field.ErrAdaptiveLimit)
	q = adaptiveOptions()
	q.InitialStep = .5
	q.Limits.MaxRejectedSteps = 0
	adaptiveFailure(t, ctx, func(p geom.Vec3) geom.Vec3 { return geom.V(-p.Y, p.X) }, geom.V(1, 0), q, field.ErrAdaptiveLimit)
}

func TestAdaptiveExplicitPhysicalTerminations(t *testing.T) {
	ctx := context.Background()
	o := adaptiveOptions()
	o.Domain = field.Domain{Min: geom.V(-1, -1), Max: geom.V(1, 1)}
	o.InitialStep = .5
	o.MaxStep = .5
	o.EndTime = 3
	fn := func(p geom.Vec3) geom.Vec3 {
		if !o.Domain.Contains(p) {
			t.Fatal("field called outside Domain", p)
		}
		return geom.V(1, 0)
	}
	r, e := field.IntegrateAdaptive(ctx, fn, geom.V(0, 0), o)
	if e != nil {
		t.Fatal(e)
	}
	checkAdaptive(t, r, geom.V(0, 0), o)
	if r.Reason() != field.DomainExit || r.Time() != 1 || r.Stats().AcceptedSteps != 2 {
		t.Fatal("boundary prefix lost", r.Reason(), r.Time(), r.Stats())
	}
	_, stop := r.Stop()
	if o.Domain.Contains(stop) {
		t.Fatal("failed stage missing")
	}
	cases := []struct {
		name   string
		fn     field.Function
		reason field.Termination
	}{
		{"zero", func(geom.Vec3) geom.Vec3 { return geom.V(0, 0) }, field.Stagnation},
		{"small", func(geom.Vec3) geom.Vec3 { return geom.V(1e-12, 0) }, field.Stagnation},
		{"NaN", func(geom.Vec3) geom.Vec3 { return geom.V(math.NaN(), 0) }, field.InvalidVector},
		{"Z", func(geom.Vec3) geom.Vec3 { return geom.Vec3{X: 1, Z: 1} }, field.InvalidVector},
		{"speed_overflow", func(geom.Vec3) geom.Vec3 { return geom.V(math.MaxFloat64, math.MaxFloat64) }, field.InvalidVector},
		{"stage_singularity", func(p geom.Vec3) geom.Vec3 {
			if p.X > 0 {
				return geom.V(math.Inf(1), 0)
			}
			return geom.V(1, 0)
		}, field.InvalidVector},
		{"stage_stagnation", func(p geom.Vec3) geom.Vec3 {
			if p.X > 0 {
				return geom.V(0, 0)
			}
			return geom.V(1, 0)
		}, field.Stagnation},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q := adaptiveOptions()
			q.MinSpeed = 1e-10
			r, e := field.IntegrateAdaptive(ctx, tc.fn, geom.V(0, 0), q)
			if e != nil || r.Reason() != tc.reason || r.Stats().AcceptedSteps != 0 || len(r.Points()) != 1 {
				t.Fatalf("%v %v", r, e)
			}
		})
	}
}

func TestAdaptiveMinStepStiffAndFloatingResolution(t *testing.T) {
	ctx := context.Background()
	o := adaptiveOptions()
	o.InitialStep = .5
	o.MinStep = .5
	o.MaxStep = .5
	o.RelativeTolerance = 1e-15
	o.AbsoluteTolerance = 1e-15
	r, e := field.IntegrateAdaptive(ctx, func(p geom.Vec3) geom.Vec3 { return geom.V(-p.Y, p.X) }, geom.V(1, 0), o)
	if e != nil || r.Reason() != field.MinimumStep || r.Stats().RejectedSteps != 1 || r.Stats().AcceptedSteps != 0 {
		t.Fatalf("minstep %v %v", r, e)
	}
	// A terminal remainder smaller than MinStep may be tried but never forced to
	// pass tolerance. It returns MinimumStep if the error is still too large.
	o.EndTime = .4
	r, e = field.IntegrateAdaptive(ctx, func(p geom.Vec3) geom.Vec3 { return geom.V(-p.Y, p.X) }, geom.V(1, 0), o)
	if e != nil || r.Reason() != field.MinimumStep {
		t.Fatalf("terminal minstep %v %v", r, e)
	}
	o = adaptiveOptions()
	o.Domain = field.Domain{Min: geom.V(-1e100, -1e100), Max: geom.V(1e100, 1e100)}
	o.InitialStep = .1
	o.MinStep = .05
	o.MaxStep = .1
	r, e = field.IntegrateAdaptive(ctx, func(p geom.Vec3) geom.Vec3 { return geom.V(-1000*p.X, -p.Y) }, geom.V(1, 1), o)
	if e != nil || r.Reason() != field.MinimumStep || r.Stats().AcceptedSteps != 0 || r.Stats().RejectedSteps < 2 {
		t.Fatalf("stiff tolerance silently bypassed %v %v", r, e)
	}
	o = adaptiveOptions()
	o.Domain = field.Domain{Min: geom.V(1e20, -1), Max: geom.V(2e20, 1)}
	r, e = field.IntegrateAdaptive(ctx, func(geom.Vec3) geom.Vec3 { return geom.V(1, 0) }, geom.V(1e20, 0), o)
	if e != nil || r.Reason() != field.NoProgress {
		t.Fatalf("coordinate precision %v %v", r, e)
	}
	o = adaptiveOptions()
	o.StartTime = 1e20
	o.EndTime = 1e20 + 65536
	r, e = field.IntegrateAdaptive(ctx, func(geom.Vec3) geom.Vec3 { return geom.V(1, 0) }, geom.V(0, 0), o)
	if e != nil || r.Reason() != field.NoProgress {
		t.Fatalf("time precision %v %v", r, e)
	}
	o = adaptiveOptions()
	o.StartTime = 1e16
	o.EndTime = 1e16 + 8
	o.InitialStep = 1.5
	o.MaxStep = 1.5
	r, e = field.IntegrateAdaptive(ctx, func(geom.Vec3) geom.Vec3 { return geom.V(1, 0) }, geom.V(0, 0), o)
	if e != nil || r.Reason() != field.NoProgress {
		t.Fatalf("time rounding exceeded max step %v %v", r, e)
	}
}

func TestAdaptiveFiniteExtremeProductsAndUnderflow(t *testing.T) {
	ctx := context.Background()
	for _, pair := range [][2]float64{{1e300, 1e-300}, {1e-300, 1e300}} {
		o := adaptiveOptions()
		o.EndTime = pair[1]
		o.InitialStep = pair[1]
		o.MaxStep = pair[1]
		o.MinStep = pair[1] / 10
		r, e := field.IntegrateAdaptive(ctx, func(geom.Vec3) geom.Vec3 { return geom.V(pair[0], 0) }, geom.V(0, 0), o)
		if e != nil {
			t.Fatal(e)
		}
		if r.Reason() != field.TimeComplete {
			t.Fatalf("finite products failed %v", r.Reason())
		}
		p := r.Points()
		closeVec(t, p[len(p)-1], geom.V(1, 0), 1e-14)
	}
	o := adaptiveOptions()
	r, e := field.IntegrateAdaptive(ctx, func(geom.Vec3) geom.Vec3 { return geom.V(math.SmallestNonzeroFloat64, 0) }, geom.V(0, 0), o)
	if e != nil || r.Reason() != field.InvalidPoint {
		t.Fatalf("nonzero coordinate underflow was discarded %v %v", r, e)
	}
	o = adaptiveOptions()
	o.Domain = field.Domain{Min: geom.V(-1e308, -1), Max: geom.V(1e307, 1)}
	o.EndTime = 1e308
	o.InitialStep = 1e308
	o.MaxStep = 1e308
	o.MinStep = 1
	r, e = field.IntegrateAdaptive(ctx, func(geom.Vec3) geom.Vec3 { return geom.V(100, 0) }, geom.V(0, 0), o)
	if e != nil || r.Reason() != field.InvalidPoint {
		t.Fatalf("stage overflow %v %v", r, e)
	}
}
