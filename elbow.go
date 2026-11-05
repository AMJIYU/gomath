package gomath

import (
	"fmt"
	"gomath/geom"
	"math"
)

// Elbow draws the two sides (0,width)->(width,width)->(width,0), rotated
// counterclockwise about world zero. Width is the side length, not the rotated
// bounding-box width. Geometry follows Manim CE's Elbow (MIT notices in LICENSES).
// Width must be in [1e-9,1e12], angle finite, all resulting points within +/-1e12.
func Elbow(width, angle float64) (*Object, error) {
	if !primitiveDimension(width) || !finite(angle) {
		return nil, fmt.Errorf("elbow requires bounded positive width and finite angle")
	}
	s, c := math.Sincos(angle)
	points := [3]geom.Vec3{geom.V(-width*s, width*c), geom.V(width*(c-s), width*(s+c)), geom.V(width*c, width*s)}
	for _, point := range points {
		if !boundedPrimitivePoint(point) {
			return nil, fmt.Errorf("rotated elbow exceeds coordinate bounds")
		}
	}
	return Polyline(points[:]...), nil
}
