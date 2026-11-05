package scenes

import (
	"gomath/geom"
	"math"
	"reflect"
	"testing"
)

func TestFlowDemoTraceEndpointsFollowPhysicalClock(t *testing.T) {
	lines, flow, err := streamlineFlowDemo()
	if err != nil {
		t.Fatal(err)
	}
	if flow.Duration() != 5 || len(lines.Lines()) != 12 {
		t.Fatal("wrong flow example clock or seeds")
	}
	for _, time := range []float64{.7, 1, 2.2} {
		frame, err := flow.Sample(time)
		if err != nil {
			t.Fatal(err)
		}
		if len(frame.Objects) != 1 || len(frame.Objects[0].Children) != 12 {
			t.Fatal("flow example line topology")
		}
		for i, object := range frame.Objects[0].Children {
			head := (time - float64(i)*.035) * 1.5
			tail := max(0, head-1)
			curves := object.Path.Contours[0].Curves
			seed := lines.Lines()[i].Result.Points()[0]
			analytic := func(physical float64) geom.Vec3 {
				return geom.RotationZ(physical).Apply(seed).Mul(math.Exp(-.12 * physical))
			}
			if curves[0].P0.Sub(analytic(tail)).Norm() > .004 || curves[len(curves)-1].P3.Sub(analytic(head)).Norm() > .004 {
				t.Fatal("trace uses incorrect physical time window", time, i)
			}
		}
	}
	first, err := flow.Sample(1)
	if err != nil {
		t.Fatal(err)
	}
	want, err := flow.Sample(1)
	if err != nil {
		t.Fatal(err)
	}
	first.Objects[0].Children[0].Path.Contours[0].Curves[0].P0.X = 123
	got, err := flow.Sample(1)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatal("modified demo frame leaked into future sampling")
	}
}
