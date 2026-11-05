package gomath

import "gomath/path"

// MoveAlongPath translates an object's bounding center along a captured vector
// path at uniform arc-length speed before easing. It preserves orientation and
// subtree identity. Route geometry is frozen by Play, so later route changes do
// not affect past segments. Separate contours are pen-up jumps, not connectors.
//
// This uses numerical inverse arc length, unlike Manim's approximate length-
// weighted cubic parameter mapping. Those timings can differ on curved cubics.
func MoveAlongPath(object, route *Object) Animation {
	return MoveAlongPathWithTolerance(object, route, path.DefaultLengthTolerance)
}

// MoveAlongPathWithTolerance supplies an absolute accuracy budget in world units.
// Invalid routes/tolerances and empty paths fail atomically when passed to Play.
func MoveAlongPathWithTolerance(object, route *Object, tolerance float64) Animation {
	return Animation{object: object, route: route, pathTolerance: tolerance, mode: pathMove, rate: Linear, end: 1}
}
