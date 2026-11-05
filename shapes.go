package gomath

import (
	"gomath/geom"
	"gomath/path"
	"math"
)

func Circle(radius float64) *Object { return Arc(radius, 0, 2*math.Pi, true) }
func Arc(radius, start, sweep float64, closed bool) *Object {
	var b path.Builder
	n := max(1, int(math.Ceil(math.Abs(sweep)/(math.Pi/2))))
	step := sweep / float64(n)
	point := func(a float64) geom.Vec3 { return geom.V(radius*math.Cos(a), radius*math.Sin(a)) }
	b.MoveTo(point(start))
	for i := range n {
		a := start + float64(i)*step
		z := a + step
		k := 4.0 / 3 * math.Tan(step/4) * radius
		p0, p3 := point(a), point(z)
		b.CubicTo(p0.Add(geom.V(-math.Sin(a)*k, math.Cos(a)*k)), p3.Sub(geom.V(-math.Sin(z)*k, math.Cos(z)*k)), p3)
	}
	if closed {
		b.Close()
	}
	return NewObject(b.Path())
}
func Rectangle(width, height float64) *Object {
	return Polygon(geom.V(-width/2, -height/2), geom.V(width/2, -height/2), geom.V(width/2, height/2), geom.V(-width/2, height/2))
}
func Square(side float64) *Object { return Rectangle(side, side) }
func Polygon(points ...geom.Vec3) *Object {
	var b path.Builder
	if len(points) > 0 {
		b.MoveTo(points[0])
		for _, p := range points[1:] {
			b.LineTo(p)
		}
		b.Close()
	}
	return NewObject(b.Path())
}
func Polyline(points ...geom.Vec3) *Object {
	var b path.Builder
	if len(points) > 0 {
		b.MoveTo(points[0])
		for _, p := range points[1:] {
			b.LineTo(p)
		}
	}
	return NewObject(b.Path())
}
func Line(a, b geom.Vec3) *Object { return Polyline(a, b) }
func Dot(position geom.Vec3, radius float64) *Object {
	return Circle(radius).SetFill(White).SetStroke(Transparent, 0).MoveTo(position)
}
func Arrow(a, b geom.Vec3, tipSize float64) *Object {
	u := b.Sub(a).Unit()
	v := geom.V(-u.Y, u.X)
	base := b.Sub(u.Mul(tipSize))
	shaft := Line(a, base)
	tip := Polygon(b, base.Add(v.Mul(tipSize*.4)), base.Sub(v.Mul(tipSize*.4))).SetFill(White).SetStroke(Transparent, 0)
	return Group(shaft, tip)
}
func RegularPolygon(sides int, radius float64) *Object {
	if sides < 3 {
		return Polygon()
	}
	points := make([]geom.Vec3, sides)
	for i := range sides {
		a := math.Pi/2 + float64(i)*2*math.Pi/float64(sides)
		points[i] = geom.V(radius*math.Cos(a), radius*math.Sin(a))
	}
	return Polygon(points...)
}
func CubicBezier(a, b, c, d geom.Vec3) *Object {
	return NewObject(path.Path{Contours: []path.Contour{{Start: a, Curves: []path.Cubic{{P0: a, P1: b, P2: c, P3: d}}}}})
}
