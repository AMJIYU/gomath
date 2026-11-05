package gomath

import (
	"fmt"
	"gomath/geom"
	"gomath/path"
	"math"
)

// AngleOptions describes an XY-plane angle between two infinite directed lines.
// Zero Radius selects Manim's endpoint-distance rule. Zero Quadrant means {1,1};
// otherwise each entry must be -1 or 1. OtherAngle selects the clockwise arc.
// Elbow draws two straight sides instead. Dot is available for arcs only.
// Zero DotRadius and DotDistance select radius/10 and .55 respectively.
// Use DefaultAngleOptions when a white dot is wanted; zero DotColor is transparent.
type AngleOptions struct {
	Radius, DotRadius, DotDistance float64
	Quadrant                       [2]int
	OtherAngle, Elbow, Dot         bool
	DotColor                       Color
}

func DefaultAngleOptions() AngleOptions { return AngleOptions{DotColor: White} }

// AngleMark owns independent geometry. Vertex, Radius and Sweep describe its
// construction and do not change when Object is subsequently animated or edited.
// Parallel lines produce an empty Object, Parallel=true and zero metadata.
type AngleMark struct {
	Object        *Object
	Vertex        geom.Vec3
	Radius, Sweep float64
	Parallel      bool
}

// Value returns the construction sweep in radians, or degrees if requested.
func (a *AngleMark) Value(degrees bool) float64 {
	if degrees {
		return a.Sweep * (180 / math.Pi)
	}
	return a.Sweep
}

// NewAngle reads a single open straight cubic from each object. Groups, bitmaps,
// curves and non-XY lines are rejected. Styles and clips do not affect the line
// intersection; the inputs are not retained or changed. The resulting mark has
// the ordinary default style and can be styled or added to a Scene via Object.
func NewAngle(line1, line2 *Object, options AngleOptions) (*AngleMark, error) {
	a, b, err := angleLineEndpoints(line1)
	if err != nil {
		return nil, fmt.Errorf("angle first line: %w", err)
	}
	c, d, err := angleLineEndpoints(line2)
	if err != nil {
		return nil, fmt.Errorf("angle second line: %w", err)
	}
	return newAngleEndpoints(a, b, c, d, options)
}

// NewAngleFromPoints constructs angle ABC, following rays B->A and B->C.
func NewAngleFromPoints(a, b, c geom.Vec3, options AngleOptions) (*AngleMark, error) {
	return newAngleEndpoints(b, a, b, c, options)
}

// NewRightAngle draws an elbow using the same radius/quadrant options. Like
// Manim's RightAngle, it permits oblique lines; the caller establishes whether
// the actual angle is a right angle. Dot and OtherAngle are rejected for elbows.
func NewRightAngle(line1, line2 *Object, options AngleOptions) (*AngleMark, error) {
	options.Elbow = true
	return NewAngle(line1, line2, options)
}

func angleLineEndpoints(o *Object) (geom.Vec3, geom.Vec3, error) {
	if o == nil || o.Bitmap != nil || len(o.Children) != 0 || len(o.Path.Contours) != 1 {
		return geom.Vec3{}, geom.Vec3{}, fmt.Errorf("requires one open straight line")
	}
	c := o.Path.Contours[0]
	if c.Closed || len(c.Curves) != 1 {
		return geom.Vec3{}, geom.Vec3{}, fmt.Errorf("requires one open straight cubic")
	}
	k := c.Curves[0]
	for _, p := range []geom.Vec3{c.Start, k.P0, k.P1, k.P2, k.P3} {
		if !boundedPrimitivePoint(p) || p.Z != 0 {
			return geom.Vec3{}, geom.Vec3{}, fmt.Errorf("requires bounded XY coordinates")
		}
	}
	tolerance := 1e-12 * max(1, math.Hypot(k.P3.X-k.P0.X, k.P3.Y-k.P0.Y))
	if c.Start != k.P0 || k.P1.Sub(k.P0.Lerp(k.P3, 1.0/3)).Norm() > tolerance || k.P2.Sub(k.P0.Lerp(k.P3, 2.0/3)).Norm() > tolerance {
		return geom.Vec3{}, geom.Vec3{}, fmt.Errorf("requires a line with uniform cubic handles")
	}
	return k.P0, k.P3, nil
}

func newAngleEndpoints(a, b, c, d geom.Vec3, o AngleOptions) (*AngleMark, error) {
	if o.Quadrant == [2]int{} {
		o.Quadrant = [2]int{1, 1}
	}
	for _, sign := range o.Quadrant {
		if sign != -1 && sign != 1 {
			return nil, fmt.Errorf("angle quadrant entries must each be -1 or 1")
		}
	}
	if (!primitiveDimension(o.Radius) && o.Radius != 0) || (!primitiveDimension(o.DotRadius) && o.DotRadius != 0) || !finite(o.DotDistance) || o.DotDistance < 0 || o.DotDistance > MaxPrimitiveCoordinate || !o.DotColor.valid() {
		return nil, fmt.Errorf("angle radius, dot radius, distance or color outside bounded finite range")
	}
	if o.Elbow && (o.Dot || o.OtherAngle) {
		return nil, fmt.Errorf("angle elbow does not support a dot or clockwise arc")
	}
	frame, err := intersectAngleLines(a, b, c, d)
	if err != nil {
		return nil, err
	}
	if frame.parallel {
		return &AngleMark{Object: NewObject(path.Path{}), Parallel: true}, nil
	}
	vertex, u, v := frame.vertex, frame.first, frame.second
	radius := o.Radius
	if radius == 0 {
		p, q := b, d
		if o.Quadrant[0] == -1 {
			p = a
		}
		if o.Quadrant[1] == -1 {
			q = c
		}
		distance := min(math.Hypot(p.X-vertex.X, p.Y-vertex.Y), math.Hypot(q.X-vertex.X, q.Y-vertex.Y))
		radius = .4
		if distance < .6 {
			radius = (2.0 / 3) * distance
		}
		if !primitiveDimension(radius) {
			return nil, fmt.Errorf("angle automatic radius is too small; supply an explicit radius")
		}
	}
	u, v = u.Mul(float64(o.Quadrant[0])), v.Mul(float64(o.Quadrant[1]))
	start, end := math.Atan2(u.Y, u.X), math.Atan2(v.Y, v.X)
	sweep := end - start
	if sweep <= 0 {
		sweep += 2 * math.Pi
	}
	if o.OtherAngle {
		sweep -= 2 * math.Pi
	}
	var object *Object
	if o.Elbow {
		object = Polyline(vertex.Add(u.Mul(radius)), vertex.Add(u.Add(v).Mul(radius)), vertex.Add(v.Mul(radius)))
	} else {
		object = NewObject(angleArcPath(vertex, radius, start, sweep, false))
		if o.Dot {
			dotRadius, distance := o.DotRadius, o.DotDistance
			if dotRadius == 0 {
				dotRadius = radius / 10
			}
			if distance == 0 {
				distance = .55
			}
			// Manim locates its dot along the arc anchor-point bounding-box
			// center, which differs from the bisector for some reflex arcs.
			var bounds geom.Bounds
			for _, k := range object.Path.Contours[0].Curves {
				for _, p := range []geom.Vec3{k.P0, k.P3} {
					bounds.Include(p)
				}
			}
			direction := bounds.Center().Sub(vertex)
			norm := math.Hypot(direction.X, direction.Y)
			if norm <= 1e-12*radius || !primitiveDimension(dotRadius) {
				return nil, fmt.Errorf("angle dot has degenerate direction or radius")
			}
			center := vertex.Add(direction.Mul(radius * distance / norm))
			dot := NewObject(angleArcPath(center, dotRadius, 0, 2*math.Pi, true)).SetFill(o.DotColor).SetStroke(Transparent, 0)
			object.Children = []*Object{dot}
		}
	}
	for _, item := range append([]*Object{object}, object.Children...) {
		for _, contour := range item.Path.Contours {
			for _, curve := range contour.Curves {
				for _, point := range []geom.Vec3{curve.P0, curve.P1, curve.P2, curve.P3} {
					if !boundedPrimitivePoint(point) {
						return nil, fmt.Errorf("angle or dot geometry exceeds coordinate bounds")
					}
				}
			}
		}
	}
	return &AngleMark{Object: object, Vertex: vertex, Radius: radius, Sweep: sweep}, nil
}

type angleLineFrame struct {
	vertex, first, second geom.Vec3
	parallel              bool
}

func intersectAngleLines(a, b, c, d geom.Vec3) (angleLineFrame, error) {
	for _, p := range []geom.Vec3{a, b, c, d} {
		if !boundedPrimitivePoint(p) || p.Z != 0 {
			return angleLineFrame{}, fmt.Errorf("angle requires bounded XY endpoints")
		}
	}
	var (
		u, v = b.Sub(a), d.Sub(c)
		nu   = math.Hypot(u.X, u.Y)
		nv   = math.Hypot(v.X, v.Y)
	)
	if nu < 1e-9 || nv < 1e-9 {
		return angleLineFrame{}, fmt.Errorf("angle line lengths must be at least 1e-9")
	}
	// Product residuals distinguish true parallel directions from a tiny
	// determinant hidden by rounding of two equal binary64 products. Such
	// nonparallel directions must take the ill-conditioned error path below.
	p, q := u.X*v.Y, u.Y*v.X
	if p == q && math.FMA(u.X, v.Y, -p) == math.FMA(u.Y, v.X, -q) {
		return angleLineFrame{parallel: true}, nil
	}
	u, v = u.Mul(1/nu), v.Mul(1/nv)
	den := u.X*v.Y - u.Y*v.X
	if math.Abs(den) <= 1e-12 {
		return angleLineFrame{}, fmt.Errorf("angle intersection is ill-conditioned (direction sine <= 1e-12)")
	}
	delta := c.Sub(a)
	vertex := a.Add(u.Mul((delta.X*v.Y - delta.Y*v.X) / den))
	if !boundedPrimitivePoint(vertex) {
		return angleLineFrame{}, fmt.Errorf("angle intersection exceeds coordinate bounds")
	}
	return angleLineFrame{vertex: vertex, first: u, second: v}, nil
}

// Eight tangent cubics match the pinned Manim CE Arc used by Angle. Ordinary
// Arc remains unchanged; this bounded construction never uses adaptive recursion.
func angleArcPath(center geom.Vec3, radius, start, sweep float64, closed bool) path.Path {
	const segments = 8
	var builder path.Builder
	step := sweep / segments
	k := (4.0 / 3) * math.Tan(step/4) * radius
	point := func(a float64) geom.Vec3 { return center.Add(geom.V(radius*math.Cos(a), radius*math.Sin(a))) }
	builder.MoveTo(point(start))
	for i := range segments {
		a, z := start+float64(i)*step, start+float64(i+1)*step
		p, q := point(a), point(z)
		builder.CubicTo(p.Add(geom.V(-k*math.Sin(a), k*math.Cos(a))), q.Sub(geom.V(-k*math.Sin(z), k*math.Cos(z))), q)
	}
	if closed {
		builder.Close()
	}
	return builder.Path()
}
