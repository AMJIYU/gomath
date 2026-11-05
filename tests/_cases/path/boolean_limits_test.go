package path_test

import (
	"gomath/geom"
	"gomath/path"
	"math"
	"strings"
	"testing"
)

func TestBooleanLimitsAndInvalidOptionsReturnNoPartialResult(t *testing.T) {
	a, b := boolRect(0, 0, 8, 8), boolRect(2, 2, 10, 10)
	for _, tc := range []struct {
		name   string
		limits path.BooleanLimits
		want   string
	}{
		{"contours", path.BooleanLimits{Contours: 1}, "contours"},
		{"segments", path.BooleanLimits{Segments: 4}, "segments"},
		{"pairs", path.BooleanLimits{Pairs: 1}, "pair"},
		{"vertices", path.BooleanLimits{Vertices: 2}, "vertices"},
		{"edges", path.BooleanLimits{Edges: 1}, "edges"},
		{"cuts", path.BooleanLimits{SplitPoints: 2}, "split"},
		{"work", path.BooleanLimits{Work: 1}, "work"},
		{"rational precision", path.BooleanLimits{RationalBits: 2}, "rational"},
		{"allocation", path.BooleanLimits{AllocationBytes: 1}, "allocation"},
		{"negative", path.BooleanLimits{Work: -1}, "limit"},
		{"hard ceiling", path.BooleanLimits{Segments: 2049}, "limit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := path.Boolean(a, b, path.BooleanUnion, path.BooleanOptions{Limits: tc.limits})
			if len(got.Contours) != 0 || err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got=%v error=%v", got, err)
			}
		})
	}
	for _, tc := range []struct {
		op  path.BooleanOp
		opt path.BooleanOptions
	}{{path.BooleanOp(9), path.BooleanOptions{}}, {path.BooleanUnion, path.BooleanOptions{RuleA: 9}}, {path.BooleanUnion, path.BooleanOptions{RuleB: 9}}} {
		if p, e := path.Boolean(a, b, tc.op, tc.opt); e == nil || len(p.Contours) != 0 {
			t.Fatal(p, e)
		}
	}
}

func TestBooleanExactDyadicExtremesAndImplicitClosure(t *testing.T) {
	// There is no snapping threshold: the smallest representable square is still
	// an exact nonempty filled region, even though its float64 area underflows.
	tiny := math.SmallestNonzeroFloat64
	a := boolRect(0, 0, tiny, tiny)
	// Builder.Close deliberately omits its final line below 1e-12; Boolean uses
	// the Closed flag as an exact implicit closing edge, preserving that sliver.
	got, err := path.Boolean(a, path.Path{}, path.BooleanUnion, path.BooleanOptions{})
	if err != nil || len(got.Contours) != 1 || len(got.Contours[0].Curves) != 4 {
		t.Fatal(got, err)
	}
	for _, c := range got.Contours {
		for _, s := range c.Curves {
			if s.P0 == s.P3 {
				t.Fatal("collapsed dyadic edge")
			}
		}
	}
	// Unsupported precision fails rather than turning a nonempty region empty.
	if p, e := path.Boolean(a, path.Path{}, path.BooleanUnion, path.BooleanOptions{Limits: path.BooleanLimits{RationalBits: 128}}); e == nil || len(p.Contours) != 0 {
		t.Fatal(p, e)
	}
}

func TestBooleanRejectsUnrepresentableRoundedTopology(t *testing.T) {
	const m = 1e16
	triangle := boolPolygon(geom.V(m, 0), geom.V(m+4, 0), geom.V(m, 4))
	strip := boolRect(m-2, 3.5, m+6, 3.75)
	p, err := path.Boolean(triangle, strip, path.BooleanIntersection, path.BooleanOptions{})
	if err == nil || len(p.Contours) != 0 || !strings.Contains(err.Error(), "rounding") {
		t.Fatalf("unrepresentable sliver: %+v %v", p, err)
	}
}
