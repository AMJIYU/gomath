package field_test

import (
	"context"
	"encoding/json"
	"errors"
	"gomath/field"
	"gomath/geom"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func adaptiveOptions() field.AdaptiveOptions {
	o := field.DefaultAdaptiveOptions()
	o.Domain = field.Domain{Min: geom.V(-10, -10), Max: geom.V(10, 10)}
	o.EndTime = 2
	o.AbsoluteTolerance = 1e-11
	o.RelativeTolerance = 1e-10
	o.MinSpeed = 0
	return o
}
func checkAdaptive(t testing.TB, r *field.AdaptiveResult, seed geom.Vec3, o field.AdaptiveOptions) {
	t.Helper()
	if r == nil {
		t.Fatal("nil result")
	}
	times, points, stats := r.Times(), r.Points(), r.Stats()
	if len(times) != len(points) || len(points) != stats.AcceptedSteps+1 || points[0] != seed || times[0] != o.StartTime {
		t.Fatal("wrong shape/initial conditions")
	}
	direction := math.Copysign(1, o.EndTime-o.StartTime)
	for i, p := range points {
		if !o.Domain.Contains(p) {
			t.Fatalf("invalid accepted point %v", p)
		}
		if i > 0 {
			h := direction * (times[i] - times[i-1])
			if h <= 0 || h > o.MaxStep || h < o.MinStep && times[i] != o.EndTime {
				t.Fatalf("bad accepted time/step %v", times)
			}
		}
	}
	if r.Time() != times[len(times)-1] {
		t.Fatal("Time is not last accepted time")
	}
	if r.Reason() == field.TimeComplete && r.Time() != o.EndTime {
		t.Fatal("false complete")
	}
}

func TestAdaptiveHarmonicAccuracyAndSignedNonuniformTimes(t *testing.T) {
	fn := func(p geom.Vec3) geom.Vec3 { return geom.V(-p.Y, p.X) }
	for _, end := range []float64{6.2, -6.2} {
		o := adaptiveOptions()
		o.StartTime = 3
		o.EndTime = 3 + end
		o.InitialStep = .5
		r, e := field.IntegrateAdaptive(context.Background(), fn, geom.V(1, 0), o)
		if e != nil {
			t.Fatal(e)
		}
		checkAdaptive(t, r, geom.V(1, 0), o)
		if r.Reason() != field.TimeComplete || r.Stats().RejectedSteps == 0 {
			t.Fatalf("expected complete and actual adaptivity %v %v", r.Reason(), r.Stats())
		}
		nonuniform := false
		times := r.Times()
		for i, p := range r.Points() {
			elapsed := times[i] - o.StartTime
			closeVec(t, p, geom.V(math.Cos(elapsed), math.Sin(elapsed)), 2e-9)
			if i > 1 && math.Abs((times[i]-times[i-1])-(times[1]-times[0])) > 1e-12 {
				nonuniform = true
			}
		}
		if !nonuniform {
			t.Fatal("adaptive times became fixed-step")
		}
		if r.Stats().Evaluations != 1+6*(r.Stats().AcceptedSteps+r.Stats().RejectedSteps) {
			t.Fatal("FSAL call bookkeeping", r.Stats())
		}
	}
}

func TestAdaptiveLinearConvergenceAndConstantCancellation(t *testing.T) {
	fn := func(p geom.Vec3) geom.Vec3 { return geom.V(p.X, -.3*p.Y) }
	o := adaptiveOptions()
	o.EndTime = 1
	o.InitialStep = .5
	o.AbsoluteTolerance = 1e-5
	o.RelativeTolerance = 1e-5
	coarse, e := field.IntegrateAdaptive(context.Background(), fn, geom.V(1, 1), o)
	if e != nil {
		t.Fatal(e)
	}
	o.AbsoluteTolerance = 1e-12
	o.RelativeTolerance = 1e-12
	fine, e := field.IntegrateAdaptive(context.Background(), fn, geom.V(1, 1), o)
	if e != nil {
		t.Fatal(e)
	}
	want := geom.V(math.E, math.Exp(-.3))
	a, b := coarse.Points(), fine.Points()
	if b[len(b)-1].Sub(want).Norm() >= a[len(a)-1].Sub(want).Norm()/100 {
		t.Fatal("tighter tolerance did not improve solution")
	}
	if fine.Stats().AcceptedSteps <= coarse.Stats().AcceptedSteps {
		t.Fatal("step controller did not adapt")
	}
	// Rational tableau sums make a constant field's error exactly zero even with
	// literal tiny positive tolerances. No roundoff floor or tolerance substitution.
	o = adaptiveOptions()
	o.InitialStep = .125
	o.MaxStep = .5
	o.EndTime = 1
	o.AbsoluteTolerance = 1e-100
	o.RelativeTolerance = 0
	r, e := field.IntegrateAdaptive(context.Background(), func(geom.Vec3) geom.Vec3 { return geom.V(1, 2) }, geom.V(0, 0), o)
	if e != nil {
		t.Fatal(e)
	}
	if r.Reason() != field.TimeComplete || r.Stats().RejectedSteps != 0 {
		t.Fatal(r.Reason(), r.Stats())
	}
	equal := r.Points()
	if equal[len(equal)-1] != geom.V(1, 2) {
		t.Fatal("constant field not exact", equal)
	}
}

func TestAdaptiveIndependentSciPySampleTimes(t *testing.T) {
	// Resolve fixture without changing process cwd or depending on Python at test time.
	dir, e := os.Getwd()
	if e != nil {
		t.Fatal(e)
	}
	for {
		if _, e = os.Stat(filepath.Join(dir, "go.mod")); e == nil {
			break
		}
		next := filepath.Dir(dir)
		if next == dir {
			t.Fatal("repo not found")
		}
		dir = next
	}
	raw, e := os.ReadFile(filepath.Join(dir, "tests/data/field/adaptive_scipy.json"))
	if e != nil {
		t.Fatal(e)
	}
	var refs struct {
		Cases []struct {
			Name         string
			Matrix       [2][2]float64
			Offset, Seed [2]float64
			Times        []float64
			Points       [][2]float64
		}
	}
	if e = json.Unmarshal(raw, &refs); e != nil {
		t.Fatal(e)
	}
	if len(refs.Cases) != 6 {
		t.Fatal("missing reference cases")
	}
	for _, c := range refs.Cases {
		t.Run(c.Name, func(t *testing.T) {
			fn := func(p geom.Vec3) geom.Vec3 {
				return geom.V(c.Matrix[0][0]*p.X+c.Matrix[0][1]*p.Y+c.Offset[0], c.Matrix[1][0]*p.X+c.Matrix[1][1]*p.Y+c.Offset[1])
			}
			seed := geom.V(c.Seed[0], c.Seed[1])
			for i, target := range c.Times {
				o := adaptiveOptions()
				o.EndTime = target
				// Reference domain is deliberately wide enough for stiff trial stages.
				o.Domain = field.Domain{Min: geom.V(-1e4, -1e4), Max: geom.V(1e4, 1e4)}
				o.MinStep = 1e-13
				r, e := field.IntegrateAdaptive(context.Background(), fn, seed, o)
				if e != nil {
					t.Fatal(e)
				}
				checkAdaptive(t, r, seed, o)
				if r.Reason() != field.TimeComplete {
					t.Fatalf("target %g: %v", target, r.Reason())
				}
				p := r.Points()
				closeVec(t, p[len(p)-1], geom.V(c.Points[i][0], c.Points[i][1]), 2e-8)
			}
		})
	}
}

func TestAdaptiveOwnershipAndZeroDuration(t *testing.T) {
	o := adaptiveOptions()
	seed := geom.V(1, 0)
	r, e := field.IntegrateAdaptive(context.Background(), func(p geom.Vec3) geom.Vec3 { return geom.V(-p.Y, p.X) }, seed, o)
	if e != nil {
		t.Fatal(e)
	}
	saved := r.Points()
	p, ts := r.Points(), r.Times()
	p[0].X = 999
	ts[0] = 999
	if !reflect.DeepEqual(saved, r.Points()) || r.Times()[0] != 0 {
		t.Fatal("result slices alias")
	}
	o.EndTime = o.StartTime
	o.Limits.MaxAcceptedSteps = 0
	o.Limits.MaxEvaluations = 0
	calls := 0
	r, e = field.IntegrateAdaptive(context.Background(), func(geom.Vec3) geom.Vec3 { calls++; return geom.V(1, 0) }, seed, o)
	if e != nil || calls != 0 || r.Stats() != (field.AdaptiveStats{}) || r.Reason() != field.TimeComplete {
		t.Fatalf("zero interval %v %v %d", r, e, calls)
	}
	var n *field.AdaptiveResult
	if n.Times() != nil || n.Points() != nil || n.Time() != 0 || n.Stats() != (field.AdaptiveStats{}) || n.Reason() != "" {
		t.Fatal("nil methods")
	}
	ts0, p0 := n.Stop()
	if ts0 != 0 || p0 != (geom.Vec3{}) {
		t.Fatal("nil stop")
	}
}

func adaptiveFailure(t testing.TB, ctx context.Context, fn field.Function, seed geom.Vec3, o field.AdaptiveOptions, want error) {
	t.Helper()
	r, e := field.IntegrateAdaptive(ctx, fn, seed, o)
	if r != nil || !errors.Is(e, want) {
		t.Fatalf("want nil/%v got %v/%v", want, r, e)
	}
}

func TestAdaptiveRelativeOnlyAndHugeFiniteScale(t *testing.T) {
	o := adaptiveOptions()
	o.EndTime = 1
	o.AbsoluteTolerance = 0
	o.RelativeTolerance = 1e-9
	r, e := field.IntegrateAdaptive(context.Background(), func(p geom.Vec3) geom.Vec3 { return geom.V(p.X, 0) }, geom.V(1, 0), o)
	if e != nil {
		t.Fatal(e)
	}
	if r.Reason() != field.TimeComplete {
		t.Fatal(r.Reason())
	}
	points := r.Points()
	closeVec(t, points[len(points)-1], geom.V(math.E, 0), 1e-8)
	o = adaptiveOptions()
	o.Domain = field.Domain{Min: geom.V(1e299, -1), Max: geom.V(2e300, 1)}
	o.RelativeTolerance = 1e100
	o.AbsoluteTolerance = 1e-300
	o.EndTime = .1
	o.InitialStep = .1
	r, e = field.IntegrateAdaptive(context.Background(), func(p geom.Vec3) geom.Vec3 { return geom.V(p.X, 0) }, geom.V(1e300, 0), o)
	if e != nil || r.Reason() != field.TimeComplete || r.Stats().AcceptedSteps != 1 {
		t.Fatalf("finite tolerance scale overflow %v %v", r, e)
	}
}
