package gomath

import (
	"fmt"
	"gomath/geom"
	"gomath/path"
	"math"
)

const (
	MaxPrimitiveCoordinate = 1e12
	MaxDashSegments        = 4096
)

func primitiveDimension(x float64) bool {
	return finite(x) && x >= 1e-9 && x <= MaxPrimitiveCoordinate
}

// Ellipse is centered at the origin, with full width and height. Its four cubic
// arcs approximate the ellipse; these are not exact rational conic segments.
func Ellipse(width, height float64) (*Object, error) {
	if !primitiveDimension(width) || !primitiveDimension(height) {
		return nil, fmt.Errorf("ellipse dimensions must be in [1e-9,%g]", MaxPrimitiveCoordinate)
	}
	return Circle(1).ApplyMatrix(geom.Scaling(width/2, height/2, 1)), nil
}

// Annulus uses oppositely wound contours so the ordinary nonzero fill leaves
// a real hole. Zero inner radius creates a disk. Stroke never crosses the gap.
func Annulus(innerRadius, outerRadius float64) (*Object, error) {
	return AnnularSector(innerRadius, outerRadius, 0, 2*math.Pi)
}

// Sector is a disk sector, centered at the origin. Angles are radians.
func Sector(radius, start, sweep float64) (*Object, error) {
	return AnnularSector(0, radius, start, sweep)
}

// AnnularSector follows the outer arc, then the reverse inner arc. Sweep must
// have magnitude in (0,2*pi]. Full turns use separate closed contours to avoid
// a visible radial stroke seam. Partial sectors include both radial edges.
func AnnularSector(innerRadius, outerRadius, start, sweep float64) (*Object, error) {
	if !primitiveDimension(outerRadius) || !finite(innerRadius) || innerRadius < 0 || innerRadius >= outerRadius || !finite(start) || !finite(sweep) || sweep == 0 || math.Abs(sweep) > 2*math.Pi {
		return nil, fmt.Errorf("annular sector requires 0 <= inner < outer, bounded outer radius, finite start and 0 < |sweep| <= 2*pi")
	}
	start = math.Remainder(start, 2*math.Pi)
	if math.Abs(sweep) == 2*math.Pi {
		outer := Arc(outerRadius, start, sweep, true)
		if innerRadius > 0 {
			inner := Arc(innerRadius, start, -sweep, true)
			outer.Path.Contours = append(outer.Path.Contours, inner.Path.Contours...)
		}
		return outer, nil
	}
	var b path.Builder
	outer := Arc(outerRadius, start, sweep, false).Path.Contours[0]
	b.MoveTo(outer.Start)
	appendCurves(&b, outer.Curves)
	if innerRadius == 0 {
		b.LineTo(geom.Vec3{})
	} else {
		inner := Arc(innerRadius, start+sweep, -sweep, false).Path.Contours[0]
		b.LineTo(inner.Start)
		appendCurves(&b, inner.Curves)
	}
	b.Close()
	return NewObject(b.Path()), nil
}

func appendCurves(b *path.Builder, curves []path.Cubic) {
	for _, c := range curves {
		b.CubicTo(c.P1, c.P2, c.P3)
	}
}

// RoundedRectangle uses four circular quarter-arc corners. Radius zero is an
// ordinary rectangle. Radius cannot exceed half the smaller full dimension.
func RoundedRectangle(width, height, radius float64) (*Object, error) {
	if !primitiveDimension(width) || !primitiveDimension(height) || !finite(radius) || radius < 0 || radius > min(width, height)/2 {
		return nil, fmt.Errorf("rounded rectangle requires bounded positive dimensions and radius in [0,min(width,height)/2]")
	}
	if radius == 0 {
		return Rectangle(width, height), nil
	}
	x, y := width/2, height/2
	centers := []geom.Vec3{geom.V(x-radius, -y+radius), geom.V(x-radius, y-radius), geom.V(-x+radius, y-radius), geom.V(-x+radius, -y+radius)}
	var b path.Builder
	b.MoveTo(geom.V(-x+radius, -y))
	for i, center := range centers {
		c := Arc(radius, -math.Pi/2+float64(i)*math.Pi/2, math.Pi/2, false).Path.Contours[0]
		b.LineTo(c.Start.Add(center))
		for _, curve := range c.Curves {
			b.CubicTo(curve.P1.Add(center), curve.P2.Add(center), curve.P3.Add(center))
		}
	}
	b.Close()
	return NewObject(b.Path()), nil
}

// DashedLine constructs real line children with a fixed world-space period.
// dashLength is the painted length; paintedRatio is in (0,1], giving period
// dashLength/paintedRatio. The last painted dash truncates at the endpoint;
// there may be a final gap. This is not a dash-array stroke or Manim's fitted
// dash distribution. At most MaxDashSegments children are constructed.
func DashedLine(start, end geom.Vec3, dashLength, paintedRatio float64) (*Object, error) {
	if !boundedPrimitivePoint(start) || !boundedPrimitivePoint(end) || !primitiveDimension(dashLength) || !finite(paintedRatio) || paintedRatio <= 0 || paintedRatio > 1 {
		return nil, fmt.Errorf("dashed line requires bounded points, positive dash length and painted ratio in (0,1]")
	}
	d := end.Sub(start)
	length := math.Hypot(math.Hypot(d.X, d.Y), d.Z)
	period := dashLength / paintedRatio
	if !primitiveDimension(length) || !finite(period) {
		return nil, fmt.Errorf("dashed line length or period outside bounded finite range")
	}
	n := math.Ceil(length / period)
	if n > MaxDashSegments {
		return nil, fmt.Errorf("dashed line exceeds %d painted segments", MaxDashSegments)
	}
	children := make([]*Object, 0, int(n))
	for i := 0; i < int(n); i++ {
		lo := float64(i) * period
		hi := min(lo+dashLength, length)
		if hi <= lo {
			continue
		}
		line := Line(start.Add(d.Mul(lo/length)), start.Add(d.Mul(hi/length)))
		line.Style.StrokeCap = CapButt
		children = append(children, line)
	}
	return Group(children...), nil
}

func boundedPrimitivePoint(p geom.Vec3) bool {
	return p.Finite() && math.Abs(p.X) <= MaxPrimitiveCoordinate && math.Abs(p.Y) <= MaxPrimitiveCoordinate && math.Abs(p.Z) <= MaxPrimitiveCoordinate
}

// DoubleArrow creates an XY-plane shaft and two filled triangle tips. Endpoints
// must share Z; tipSize is strictly smaller than half the shaft's full length.
func DoubleArrow(start, end geom.Vec3, tipSize float64) (*Object, error) {
	d := end.Sub(start)
	length := math.Hypot(d.X, d.Y)
	if !boundedPrimitivePoint(start) || !boundedPrimitivePoint(end) || start.Z != end.Z || !primitiveDimension(length) || !primitiveDimension(tipSize) || tipSize >= length/2 {
		return nil, fmt.Errorf("double arrow requires bounded distinct coplanar endpoints and 0 < tip size < length/2")
	}
	u := d.Mul(1 / length)
	v := geom.V(-u.Y, u.X).Mul(tipSize * .4)
	a, b := start.Add(u.Mul(tipSize)), end.Sub(u.Mul(tipSize))
	head := func(tip, base geom.Vec3) *Object {
		return Polygon(tip, base.Add(v), base.Sub(v)).SetFill(White).SetStroke(Transparent, 0)
	}
	return Group(Line(a, b), head(start, a), head(end, b)), nil
}
