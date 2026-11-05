package gomath

import (
	"fmt"
	"gomath/geom"
	"math"
)

// DefaultArcBraceOptions places a brace outside its supporting circle.
func DefaultArcBraceOptions() BraceOptions {
	o := DefaultBraceOptions()
	o.Direction = geom.V(1, 0)
	return o
}

// NewArcBrace constructs a planar circular brace from explicit circle data.
// Positive/negative X Direction selects outside/inside; other directions reject.
// It matches the pinned Manim ArcBrace exponential construction, including its
// radius floor of .3 and its scaling of buffer/outline height for radius >= 1.
// The result is an ordinary 25-cubic Brace, not an exact exponential curve.
// No source arc is retained. Negative sweeps use the same angular interval as
// the corresponding reversed positive sweep, as in Manim's line-bounds fit.
// Derived from Manim CE0.22.1 brace.py (MIT; see LICENSES).
func NewArcBrace(center geom.Vec3, radius, startAngle, sweep float64, o BraceOptions) (*Brace, error) {
	if !boundedPrimitivePoint(center) || !finite(radius) || radius < 1e-9 || radius > 1e6 || !finite(startAngle) || math.Abs(startAngle) > 1e6 || !finite(sweep) || math.Abs(sweep) < 1e-9 || math.Abs(sweep) > 2*math.Pi {
		return nil, fmt.Errorf("arc brace requires a bounded center, radius [1e-9,1e6], start angle +/-1e6 and absolute sweep [1e-9,2pi]")
	}
	if !o.Direction.Finite() || o.Direction.X == 0 || o.Direction.Y != 0 || o.Direction.Z != 0 {
		return nil, fmt.Errorf("arc brace direction must be along positive or negative X")
	}
	endAngle := startAngle + sweep
	if !finite(endAngle) || math.Abs(endAngle) > 1e6 || endAngle == startAngle {
		return nil, fmt.Errorf("arc brace endpoint angle is outside bounds or unrepresentable")
	}
	scale := math.Max(1, radius)
	a, b := geom.V(0, startAngle*scale), geom.V(0, endAngle*scale)
	brace, err := NewBrace(Line(a, b), o)
	if err != nil {
		return nil, err
	}
	shift := math.Log(math.Max(.3, radius))
	// Preserve Manim VMobject.apply_complex_function's .01 pre-function
	// handle-to-anchor contraction and 100x restoration, instead of directly
	// exponentiating all four cubic controls (which changes tangency badly).
	point := func(p geom.Vec3) (geom.Vec3, error) {
		x, y := p.X/scale+shift, p.Y/scale
		if !finite(x) || !finite(y) || x < math.Log(1e-12) || x > math.Log(MaxPrimitiveCoordinate) {
			return geom.Vec3{}, fmt.Errorf("arc brace exponential radius outside [1e-12,%g]", MaxPrimitiveCoordinate)
		}
		r := math.Exp(x)
		sin, cos := math.Sincos(y)
		return geom.V(r*cos, r*sin), nil
	}
	contour := &brace.Object.Path.Contours[0]
	minimum, maximum := math.Inf(1), 1.0
	for i, c := range contour.Curves {
		h1 := c.P0.Add(c.P1.Sub(c.P0).Mul(.01))
		h2 := c.P3.Add(c.P2.Sub(c.P3).Mul(.01))
		var q [4]geom.Vec3
		for j, p := range [4]geom.Vec3{c.P0, h1, h2, c.P3} {
			q[j], err = point(p)
			if err != nil {
				return nil, err
			}
		}
		c.P0, c.P1 = q[0], q[0].Add(q[1].Sub(q[0]).Mul(100))
		c.P2, c.P3 = q[3].Add(q[2].Sub(q[3]).Mul(100)), q[3]
		for _, pair := range [][2]geom.Vec3{{c.P0, c.P1}, {c.P2, c.P3}} {
			if distance := pair[0].Sub(pair[1]).Norm(); distance > 0 {
				minimum = math.Min(minimum, distance)
			}
		}
		for _, p := range []geom.Vec3{c.P0, c.P1, c.P2, c.P3} {
			maximum = math.Max(maximum, math.Max(math.Abs(p.X)+math.Abs(center.X), math.Abs(p.Y)+math.Abs(center.Y)))
		}
		c.P0, c.P1, c.P2, c.P3 = c.P0.Add(center), c.P1.Add(center), c.P2.Add(center), c.P3.Add(center)
		if i > 0 {
			c.P0 = contour.Curves[i-1].P3
		}
		contour.Curves[i] = c
	}
	contour.Start = contour.Curves[0].P0
	contour.Curves[len(contour.Curves)-1].P3 = contour.Start
	if _, err := annotationBounds(brace.Object); err != nil {
		return nil, err
	}
	// Small caps must remain meaningful after exp and the final translation.
	// Test the stored tip and neighboring cap control points as well as the
	// shared coordinate preflight; rejecting lost detail is preferable to
	// silently returning a collapsed outline at a very large center.
	ulp := math.Nextafter(maximum, math.Inf(1)) - maximum
	if minimum < 32*ulp || !finite(minimum) {
		return nil, fmt.Errorf("arc brace cap detail is unrepresentable at center scale (requires32 ULPs)")
	}
	return brace, nil
}
