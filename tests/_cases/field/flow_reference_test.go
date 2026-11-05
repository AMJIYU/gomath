package field_test

import (
	"encoding/json"
	"gomath"
	"gomath/field"
	"gomath/geom"
	"gomath/path"
	"math"
	"os"
	"testing"
)

type flowReference struct {
	Controls [][][]float64
	Cases    []struct {
		Times               []float64
		Speed, Width, Delay float64
		Loop                bool
		Samples             []struct {
			Time   float64
			Curves [][][]float64
		}
	}
	Manim struct {
		Commit string
		Probes []struct {
			Speed, Time float64
			Lines       []struct {
				Duration, Clock, Alpha float64
				Bounds                 []float64
			}
		}
	}
}

func loadFlowReference(t *testing.T) flowReference {
	t.Helper()
	b, e := os.ReadFile("flow_reference.json")
	if e != nil {
		t.Fatal(e)
	}
	var ref flowReference
	if e = json.Unmarshal(b, &ref); e != nil {
		t.Fatal(e)
	}
	return ref
}
func TestFlowIndependentFractionBernsteinReference(t *testing.T) {
	ref := loadFlowReference(t)
	curves := make([]path.Cubic, len(ref.Controls))
	vec := func(a []float64) geom.Vec3 { return geom.V(a[0], a[1]) }
	for i, c := range ref.Controls {
		curves[i] = path.Cubic{P0: vec(c[0]), P1: vec(c[1]), P2: vec(c[2]), P3: vec(c[3])}
	}
	src := gomath.NewObject(path.Path{Contours: []path.Contour{{Start: curves[0].P0, Curves: curves}}})
	checks := 0
	for _, tc := range ref.Cases {
		o := field.DefaultFlowOptions()
		o.FlowSpeed = tc.Speed
		o.TraceTimeWidth = tc.Width
		o.StartDelays = []float64{tc.Delay}
		o.Loop = tc.Loop
		f, e := field.NewFlow([]field.FlowLine{{Times: tc.Times, Object: src}}, o)
		if e != nil {
			t.Fatal(e)
		}
		for _, sample := range tc.Samples {
			got := flowCurves(t, f, sample.Time)
			if len(got) != len(sample.Curves) {
				t.Fatalf("time %g speed %g loop %v: %d vs %d", sample.Time, tc.Speed, tc.Loop, len(got), len(sample.Curves))
			}
			for i, c := range got {
				for j, want := range sample.Curves[i] {
					closeVec(t, c.At(float64(j)/4), vec(want), 3e-14)
					checks++
				}
			}
		}
	}
	if checks < 200 {
		t.Fatal("reference unexpectedly incomplete", checks)
	}
}
func TestFlowPinnedManimClockSubsetAndExplicitDifference(t *testing.T) {
	ref := loadFlowReference(t)
	if ref.Manim.Commit != "16ea443e2a5f1d79db0a721161411ce916d6e733" {
		t.Fatal("unexpected reference revision")
	}
	o := field.DefaultFlowOptions()
	o.Loop = true
	o.FlowSpeed = 1.25
	o.TraceTimeWidth = .5
	f, e := field.NewFlow([]field.FlowLine{flowInput(0, 2)}, o)
	if e != nil {
		t.Fatal(e)
	}
	checks := 0
	// Matched narrow subset: all durations=virtual_time=2, zero random phase,
	// Manim speed=1,time_width=.25 maps to physical speed=1.25,width=.5.
	for _, p := range ref.Manim.Probes {
		if p.Speed != 1 {
			continue
		}
		c := flowCurves(t, f, p.Time)
		want := p.Lines[0].Bounds
		if want[0] == want[1] {
			if len(c) != 0 {
				t.Fatal("Manim cycle endpoint mismatch")
			}
		} else {
			if len(c) != 1 || math.Abs(c[0].P0.X-want[0]) > 1e-14 || math.Abs(c[0].P3.X-want[1]) > 1e-14 {
				t.Fatal("matched clock subset differs", p, c)
			}
		}
		checks++
	}
	if checks != 5 {
		t.Fatal("missing clock probes")
	}
	// Identical numeric speed parameters deliberately do not mean identical clocks.
	o.FlowSpeed = 2
	f, e = field.NewFlow([]field.FlowLine{flowInput(0, 2)}, o)
	if e != nil {
		t.Fatal(e)
	}
	c := flowCurves(t, f, .25)
	for _, p := range ref.Manim.Probes {
		if p.Speed == 2 && p.Time == .25 && math.Abs(c[0].P3.X-p.Lines[0].Bounds[1]) < .1 {
			t.Fatal("custom physical clock accidentally treated as Manim parity")
		}
	}
}
