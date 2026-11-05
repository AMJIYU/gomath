package gomath

import (
	"fmt"
	"gomath/geom"
	"gomath/path"
	"math"
)

// ArcBetweenPointsOptions uses a signed sweep in radians, strictly between
// -2*pi and 2*pi. Zero sweep produces a line. Nonzero Radius overrides Angle
// with the minor arc of that radius; a negative radius selects clockwise.
// Zero Normal selects +Z; otherwise it must be perpendicular to the chord.
type ArcBetweenPointsOptions struct {
	Angle, Radius float64
	Normal        geom.Vec3
}

func DefaultArcBetweenPointsOptions() ArcBetweenPointsOptions {
	return ArcBetweenPointsOptions{Angle: math.Pi / 2}
}

// ArcConnection owns geometry; other fields record construction only. Linear
// connections have zero Center/Normal/Radius and Sweep. Editing or animating
// Object does not recompute this metadata.
type ArcConnection struct {
	Object                     *Object
	Start, End, Center, Normal geom.Vec3
	Radius, Sweep              float64
	Linear                     bool
}

// NewArcBetweenPoints constructs eight tangent cubic segments with exact
// authored endpoints. Positive sweep is counterclockwise about Normal. The
// stable chord-frame formulas retain small bulges without subtracting nearly
// equal large circle-center coordinates. Curves approximate a circle, not an
// exact rational conic. Input/output coordinates and radius are bounded by
// MaxPrimitiveCoordinate; chords must have length at least 1e-9.
func NewArcBetweenPoints(start, end geom.Vec3, o ArcBetweenPointsOptions) (*ArcConnection, error) {
	if !boundedPrimitivePoint(start) || !boundedPrimitivePoint(end) || !finite(o.Angle) || !finite(o.Radius) || !o.Normal.Finite() {
		return nil, fmt.Errorf("arc connection requires bounded finite endpoints and options")
	}
	var (
		d      = end.Sub(start)
		length = math.Hypot(math.Hypot(d.X, d.Y), d.Z)
		sweep  = o.Angle
	)
	if length < 1e-9 {
		return nil, fmt.Errorf("arc connection chord must have length at least 1e-9")
	}
	if o.Radius != 0 {
		if !primitiveDimension(math.Abs(o.Radius)) || math.Abs(o.Radius) < length/2 {
			return nil, fmt.Errorf("arc connection radius must be bounded and at least half the chord")
		}
		sweep = math.Copysign(2*math.Asin((length/2)/math.Abs(o.Radius)), o.Radius)
	}
	if math.Abs(sweep) >= 2*math.Pi {
		return nil, fmt.Errorf("arc connection requires |sweep| < 2*pi")
	}
	if sweep == 0 {
		return &ArcConnection{Object: Line(start, end), Start: start, End: end, Linear: true}, nil
	}
	u := geom.Vec3{X: d.X / length, Y: d.Y / length, Z: d.Z / length}
	normal := o.Normal
	if normal == (geom.Vec3{}) {
		normal.Z = 1
	}
	norm := math.Hypot(math.Hypot(normal.X, normal.Y), normal.Z)
	if !finite(norm) || norm == 0 {
		return nil, fmt.Errorf("arc connection normal cannot be normalized")
	}
	normal = geom.Vec3{X: normal.X / norm, Y: normal.Y / norm, Z: normal.Z / norm}
	if math.Abs(normal.Dot(u)) > 1e-12 {
		return nil, fmt.Errorf("arc connection normal must be perpendicular to the chord")
	}
	v := normal.Cross(u)
	v = v.Mul(1 / math.Hypot(math.Hypot(v.X, v.Y), v.Z))
	normal = u.Cross(v)
	sinHalf := math.Sin(sweep / 2)
	radius := length / (2 * math.Abs(sinHalf))
	if !primitiveDimension(radius) {
		return nil, fmt.Errorf("arc connection radius derived from sweep exceeds bounds")
	}
	center := start.Add(d.Mul(.5)).Add(v.Mul(length * (.5 / math.Tan(sweep/2))))
	if !boundedPrimitivePoint(center) {
		return nil, fmt.Errorf("arc connection center exceeds coordinate bounds")
	}
	const count = 8
	step := sweep / count
	// This ratio remains finite even for tiny sweeps: tan(step/4) and
	// sin(sweep/2) vanish at the same rate.
	handle := length * ((2.0 / 3) * math.Tan(step/4) / sinHalf)
	point := func(i int) geom.Vec3 {
		if i == 0 {
			return start
		}
		if i == count {
			return end
		}
		angle := float64(i) * step
		x := math.Sin(angle/2) * math.Cos((angle-sweep)/2) / sinHalf
		y := -math.Sin(angle/2) * math.Sin((sweep-angle)/2) / sinHalf
		return start.Add(d.Mul(x)).Add(v.Mul(length * y))
	}
	tangent := func(i int) geom.Vec3 {
		angle := float64(i)*step - sweep/2
		return u.Mul(handle * math.Cos(angle)).Add(v.Mul(handle * math.Sin(angle)))
	}
	curves := make([]path.Cubic, count)
	for i := range count {
		a, b := point(i), point(i+1)
		curves[i] = path.Cubic{P0: a, P1: a.Add(tangent(i)), P2: b.Sub(tangent(i + 1)), P3: b}
		for _, p := range []geom.Vec3{curves[i].P0, curves[i].P1, curves[i].P2, curves[i].P3} {
			if !boundedPrimitivePoint(p) {
				return nil, fmt.Errorf("arc connection control point exceeds coordinate bounds")
			}
		}
	}
	object := NewObject(path.Path{Contours: []path.Contour{{Start: start, Curves: curves}}})
	return &ArcConnection{Object: object, Start: start, End: end, Center: center, Normal: normal, Radius: radius, Sweep: sweep}, nil
}

// CurvedArrowOptions specifies an arc and planar filled triangular tips.
// Zero TipLength/TipWidth select .25/.2 world units. TipLength is the distance
// trimmed along the cubic shaft for each head; the triangle joins that trimmed
// endpoint to the exact authored tip. Double adds a head at the start as well.
// Width is the full base width. This is not Manim's endpoint-rescaling tip API.
type CurvedArrowOptions struct {
	Arc                 ArcBetweenPointsOptions
	TipLength, TipWidth float64
	Double              bool
}

func DefaultCurvedArrowOptions() CurvedArrowOptions {
	return CurvedArrowOptions{Arc: DefaultArcBetweenPointsOptions()}
}

func NewCurvedDoubleArrow(start, end geom.Vec3, o CurvedArrowOptions) (*Object, error) {
	o.Double = true
	return NewCurvedArrow(start, end, o)
}

// NewCurvedArrow returns a group owning the trimmed shaft and one/two filled
// heads. Inputs are checked before publication. Heads follow the actual
// trimmed-shaft endpoint secants, with perpendicular bases in the arc plane.
func NewCurvedArrow(start, end geom.Vec3, o CurvedArrowOptions) (*Object, error) {
	if o.TipLength == 0 {
		o.TipLength = .25
	}
	if o.TipWidth == 0 {
		o.TipWidth = .2
	}
	if !primitiveDimension(o.TipLength) || !primitiveDimension(o.TipWidth) {
		return nil, fmt.Errorf("curved arrow requires bounded positive tip dimensions")
	}
	arc, err := NewArcBetweenPoints(start, end, o.Arc)
	if err != nil {
		return nil, err
	}
	normal := arc.Normal
	if arc.Linear {
		// Linear arrows still need an explicit plane for their heads.
		normal = o.Arc.Normal
		if normal == (geom.Vec3{}) {
			normal.Z = 1
		}
		norm := math.Hypot(math.Hypot(normal.X, normal.Y), normal.Z)
		if !finite(norm) || norm == 0 {
			return nil, fmt.Errorf("curved arrow normal cannot be normalized")
		}
		normal = geom.Vec3{X: normal.X / norm, Y: normal.Y / norm, Z: normal.Z / norm}
		d := end.Sub(start)
		if math.Abs(normal.Dot(d)/math.Hypot(math.Hypot(d.X, d.Y), d.Z)) > 1e-12 {
			return nil, fmt.Errorf("curved arrow normal must be perpendicular to the chord")
		}
	}
	estimate := end.Sub(start).Norm()
	if !arc.Linear {
		estimate = arc.Radius * math.Abs(arc.Sweep)
	}
	measure, err := arc.Object.Path.Measure(max(1e-12, estimate*1e-10))
	if err != nil {
		return nil, err
	}
	lo, hi := 0.0, measure.Length()-o.TipLength
	if o.Double {
		lo = o.TipLength
	}
	if hi <= lo {
		return nil, fmt.Errorf("curved arrow tips consume the entire shaft")
	}
	trimmed, err := measure.TrimByLength(lo, hi)
	if err != nil {
		return nil, err
	}
	shaft := NewObject(trimmed)
	head := func(tip geom.Vec3, distance float64) (*Object, error) {
		sample, err := measure.AtLength(distance)
		if err != nil {
			return nil, err
		}
		u := tip.Sub(sample.Point).Unit()
		v := normal.Cross(u).Unit().Mul(o.TipWidth / 2)
		if u == (geom.Vec3{}) || v == (geom.Vec3{}) || !boundedPrimitivePoint(sample.Point.Add(v)) || !boundedPrimitivePoint(sample.Point.Sub(v)) {
			return nil, fmt.Errorf("curved arrow head degenerates or exceeds coordinate bounds")
		}
		return Polygon(tip, sample.Point.Add(v), sample.Point.Sub(v)).SetFill(White).SetStroke(Transparent, 0), nil
	}
	endHead, err := head(end, hi)
	if err != nil {
		return nil, err
	}
	children := []*Object{shaft, endHead}
	if o.Double {
		startHead, err := head(start, lo)
		if err != nil {
			return nil, err
		}
		children = append(children, startHead)
	}
	return Group(children...), nil
}
