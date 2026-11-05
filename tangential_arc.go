package gomath

import (
	"fmt"
	"math"
)

// TangentialArcOptions chooses one corner of two directed infinite XY lines.
// Radius is explicit, positive and bounded; zero Corner selects {1,1}.
type TangentialArcOptions struct {
	Radius float64
	Corner [2]int
}

// NewTangentialArc creates a circular fillet tangent to the two selected rays.
// It follows pinned Manim's endpoint order: the selected rays' cross product
// decides which tangent point comes first, giving a positive minor sweep.
// Lines are read by the same bounded single-straight-cubic contract as NewAngle,
// but parallel/collinear inputs are errors here because no fillet is defined.
// Tangent points need not fall within the input segments. Inputs are not kept.
func NewTangentialArc(first, second *Object, o TangentialArcOptions) (*ArcConnection, error) {
	if !primitiveDimension(o.Radius) {
		return nil, fmt.Errorf("tangential arc requires a bounded positive radius")
	}
	if o.Corner == [2]int{} {
		o.Corner = [2]int{1, 1}
	}
	for _, sign := range o.Corner {
		if sign != -1 && sign != 1 {
			return nil, fmt.Errorf("tangential arc corner entries must be -1 or 1")
		}
	}
	a, b, err := angleLineEndpoints(first)
	if err != nil {
		return nil, fmt.Errorf("tangential arc first line: %w", err)
	}
	c, d, err := angleLineEndpoints(second)
	if err != nil {
		return nil, fmt.Errorf("tangential arc second line: %w", err)
	}
	frame, err := intersectAngleLines(a, b, c, d)
	if err != nil {
		return nil, err
	}
	if frame.parallel {
		return nil, fmt.Errorf("tangential arc requires a unique line intersection")
	}
	u, v := frame.first.Mul(float64(o.Corner[0])), frame.second.Mul(float64(o.Corner[1]))
	cross := u.X*v.Y - u.Y*v.X
	angle := math.Atan2(math.Abs(cross), u.Dot(v))
	distance := o.Radius / math.Tan(angle/2)
	start, end := frame.vertex.Add(u.Mul(distance)), frame.vertex.Add(v.Mul(distance))
	if cross >= 0 {
		start, end = end, start
	}
	return NewArcBetweenPoints(start, end, ArcBetweenPointsOptions{Radius: o.Radius})
}
