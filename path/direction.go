package path

// Point-order semantics are compared with Manim Community 0.22.1 (MIT),
// mobject.py and vectorized_mobject.py. Notices: LICENSES/manim-LICENSE*.txt.

import (
	"fmt"
	"gomath/geom"
	"math"
)

// Reversal limits apply to the complete path, including point-only contours.
// Storage charges borrowed input and independent output, excluding allocator
// overhead. Object wrappers additionally charge their complete tree staging.
const (
	MaxDirectionContours = 4096
	MaxDirectionCurves   = 65536
	MaxDirectionWork     = 1 << 20
	MaxDirectionStorage  = 12 << 20
)

// Reversed returns an independent contour with reversed explicit cubics and
// swapped handles. Its start is the true final endpoint, even for open paths.
// Closed and point-only contours survive. Finite coordinates within +/-1e9
// and exact stored joins are required. Closed contours with a nonzero implicit
// closing edge are rejected; no closing segment is inserted or approximated.
func (c Contour) Reversed() (Contour, error) {
	p, err := (Path{Contours: []Contour{c}}).Reversed()
	if err != nil {
		return Contour{}, err
	}
	return p.Contours[0], nil
}

// Reversed returns an independent path, reversing both contour order and each
// contour's explicit cubic order. This matches reversal of a complete Manim
// cubic point array, without flattening or approximate subpath discovery.
// Empty paths and point-only contours are preserved. All inputs are checked
// before output allocation; an error returns a zero Path and changes nothing.
func (p Path) Reversed() (Path, error) {
	if len(p.Contours) > MaxDirectionContours {
		return Path{}, fmt.Errorf("path direction: contour limit exceeded (%d)", MaxDirectionContours)
	}
	n := 0
	for _, c := range p.Contours {
		if len(c.Curves) > MaxDirectionCurves-n {
			return Path{}, fmt.Errorf("path direction: curve limit exceeded (%d)", MaxDirectionCurves)
		}
		n += len(c.Curves)
	}
	if 8*n+2*len(p.Contours) > MaxDirectionWork || 192*n+112*len(p.Contours) > MaxDirectionStorage {
		return Path{}, fmt.Errorf("path direction: work/storage budget exceeded")
	}
	for i, c := range p.Contours {
		if err := validateDirectionContour(c); err != nil {
			return Path{}, fmt.Errorf("path direction: contour %d: %w", i, err)
		}
	}
	var out Path
	if p.Contours != nil {
		out.Contours = make([]Contour, len(p.Contours))
	}
	for i, c := range p.Contours {
		out.Contours[len(p.Contours)-1-i] = reverseDirectionContour(c)
	}
	return out, nil
}

func validateDirectionContour(c Contour) error {
	valid := func(v geom.Vec3) bool {
		return v.Finite() && math.Abs(v.X) <= 1e9 && math.Abs(v.Y) <= 1e9 && math.Abs(v.Z) <= 1e9
	}
	if !valid(c.Start) {
		return fmt.Errorf("start is non-finite or outside +/-1e9")
	}
	previous := c.Start
	for i, v := range c.Curves {
		if !valid(v.P0) || !valid(v.P1) || !valid(v.P2) || !valid(v.P3) {
			return fmt.Errorf("curve %d has coordinates non-finite or outside +/-1e9", i)
		}
		if v.P0 != previous {
			return fmt.Errorf("curve %d is not exactly connected", i)
		}
		previous = v.P3
	}
	if c.Closed && previous != c.Start {
		return fmt.Errorf("implicit closing edge requires an explicit closing cubic")
	}
	return nil
}

func reverseDirectionContour(c Contour) Contour {
	out := Contour{Start: c.Start, Closed: c.Closed}
	if c.Curves != nil {
		out.Curves = make([]Cubic, len(c.Curves))
	}
	if len(c.Curves) > 0 {
		out.Start = c.Curves[len(c.Curves)-1].P3
	}
	for i, v := range c.Curves {
		out.Curves[len(c.Curves)-1-i] = Cubic{v.P3, v.P2, v.P1, v.P0}
	}
	return out
}
