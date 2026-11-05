package path

import (
	"fmt"
	"gomath/geom"
)

// BooleanOp combines filled planar areas. Point and line contacts have no area.
type BooleanOp uint8

const (
	BooleanUnion BooleanOp = iota
	BooleanIntersection
	BooleanDifference
	BooleanXor
)

// FillRule interprets the winding of each entire input, including all contours.
type FillRule uint8

const (
	NonZero FillRule = iota
	EvenOdd
)

// BooleanLimits bounds one invocation. Zero fields select defaults. Positive
// fields override defaults, but cannot exceed their documented hard ceilings.
// AllocationBytes is a conservative cumulative charge, not a Go process RSS cap.
type BooleanLimits struct {
	Contours, Segments, Pairs, Vertices, Edges, SplitPoints int
	Work, RationalBits, AllocationBytes                     int
}

// BooleanOptions applies each fill rule to its whole operand. Zero options use
// NonZero for both operands and the default resource limits.
type BooleanOptions struct {
	RuleA, RuleB FillRule
	Limits       BooleanLimits
}

// DefaultBooleanOptions returns independent value options for one invocation.
func DefaultBooleanOptions() BooleanOptions {
	return BooleanOptions{Limits: BooleanLimits{Contours: 1024, Segments: 512, Pairs: 262144, Vertices: 8192, Edges: 8192, SplitPoints: 16384, Work: 2000000, RationalBits: 4096, AllocationBytes: 128 << 20}}
}

// Boolean returns a fresh, closed, line-segment path with filled area on the
// left of each boundary. Inputs are never modified. Only finite XY (Z=0), exactly
// connected, closed polygon contours are accepted. Canonical Line cubics and
// exactly collinear controls within their endpoint segment represent lines;
// other cubics require caller-side approximation and are rejected here.
//
// Topology and winding use exact rational arithmetic, with no snap epsilon.
// Output coordinates round to float64. A second exact arrangement check rejects
// collapsed vertices, new crossings/overlaps, changed cyclic ray order, and
// reversed or nested regions with incorrect winding.
// No exact Bezier Boolean or topology guarantee for caller-flattened curves is
// implied. Errors return an empty Path, never a partial output.
func Boolean(a, b Path, op BooleanOp, opt BooleanOptions) (Path, error) {
	if op > BooleanXor || opt.RuleA > EvenOdd || opt.RuleB > EvenOdd {
		return Path{}, fmt.Errorf("path boolean: invalid operation or fill rule")
	}
	limits, err := booleanLimits(opt.Limits)
	if err != nil {
		return Path{}, err
	}
	e := newBooleanEngine(limits)
	if err = e.input(a, 0); err != nil {
		return Path{}, err
	}
	if err = e.input(b, 1); err != nil {
		return Path{}, err
	}
	if err = e.split(); err != nil {
		return Path{}, err
	}
	edges, err := e.boundary(op, opt.RuleA, opt.RuleB)
	if err != nil {
		return Path{}, err
	}
	if err = e.verifyRounding(edges); err != nil {
		return Path{}, err
	}
	result, err := e.output(edges)
	if err != nil {
		return Path{}, err
	}
	return result, nil
}

func booleanLimits(l BooleanLimits) (BooleanLimits, error) {
	d := DefaultBooleanOptions().Limits
	fields := []struct {
		v       *int
		d, hard int
		name    string
	}{
		{&l.Contours, d.Contours, 4096, "contours"}, {&l.Segments, d.Segments, 2048, "segments"}, {&l.Pairs, d.Pairs, 1048576, "pairs"},
		{&l.Vertices, d.Vertices, 16384, "vertices"}, {&l.Edges, d.Edges, 16384, "edges"}, {&l.SplitPoints, d.SplitPoints, 65536, "split points"},
		{&l.Work, d.Work, 8000000, "work"}, {&l.RationalBits, d.RationalBits, 8192, "rational bits"}, {&l.AllocationBytes, d.AllocationBytes, 512 << 20, "allocation bytes"},
	}
	for _, f := range fields {
		if *f.v == 0 {
			*f.v = f.d
		}
		if *f.v < 1 || *f.v > f.hard {
			return l, fmt.Errorf("path boolean: %s limit must be in [1,%d] or zero for default", f.name, f.hard)
		}
	}
	return l, nil
}

func booleanInside(w int, r FillRule) bool {
	if r == EvenOdd {
		return w%2 != 0
	}
	return w != 0
}
func booleanCombine(a, b bool, op BooleanOp) bool {
	switch op {
	case BooleanUnion:
		return a || b
	case BooleanIntersection:
		return a && b
	case BooleanDifference:
		return a && !b
	default:
		return a != b
	}
}

func booleanXY(v geom.Vec3) bool { return v.Finite() && v.Z == 0 }
