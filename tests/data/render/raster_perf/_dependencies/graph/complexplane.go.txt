package graph

import (
	"fmt"
	"gomath"
	"gomath/geom"
	"gomath/path"
	"math"
)

// ComplexPlane owns a Cartesian grid and an affine complex-coordinate frame.
// Whole-Object affine transforms move both. The mapping ignores modifications
// to individual grid lines; removing or editing its private frame is unsupported.
type ComplexPlane struct {
	Object *gomath.Object
	plane  *NumberPlane
	frame  *gomath.Object
	x, y   Range
}

func NewComplexPlane(x, y Range, width, height float64, options NumberPlaneOptions) (*ComplexPlane, error) {
	plane, err := NewNumberPlane(x, y, width, height, options)
	if err != nil {
		return nil, err
	}
	p0 := plane.Point(x.Min, y.Min)
	p1 := plane.Point(x.Max, y.Min)
	p2 := plane.Point(x.Min, y.Max)
	frame := gomath.NewObject(path.Path{Contours: []path.Contour{{Start: p0, Curves: []path.Cubic{path.Line(p0, p1), path.Line(p1, p2)}}}}).
		SetFill(gomath.Transparent).SetStroke(gomath.Transparent, 0)
	frame.Name = "ComplexPlane affine frame"
	c := &ComplexPlane{Object: gomath.Group(plane.Object, frame), plane: plane, frame: frame, x: x, y: y}
	if _, _, _, err = c.basis(); err != nil {
		return nil, err
	}
	if err = c.Object.Validate(); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *ComplexPlane) basis() (origin, x, y geom.Vec3, err error) {
	bad := func() (geom.Vec3, geom.Vec3, geom.Vec3, error) {
		return geom.Vec3{}, geom.Vec3{}, geom.Vec3{}, fmt.Errorf("complex plane frame is missing, collapsed or ill-conditioned")
	}
	if c == nil || c.Object == nil || c.frame == nil || len(c.frame.Path.Contours) != 1 || len(c.frame.Path.Contours[0].Curves) != 2 {
		return bad()
	}
	found := false
	for _, child := range c.Object.Children {
		if child == c.frame {
			found = true
			break
		}
	}
	if !found {
		return bad()
	}
	p := c.frame.Path.Contours[0]
	origin, x, y = p.Start, p.Curves[0].P3.Sub(p.Start), p.Curves[1].P3.Sub(p.Start)
	sx, sy := complexVectorScale(x), complexVectorScale(y)
	if !origin.Finite() || !x.Finite() || !y.Finite() || sx == 0 || sy == 0 {
		return bad()
	}
	// Independently scale each axis, retaining strongly anisotropic frames.
	u, v := complexDivide(x, sx), complexDivide(y, sy)
	n := u.Cross(v)
	if n.Norm() <= 1e-12*u.Norm()*v.Norm() {
		return bad()
	}
	return origin, x, y, nil
}

// Point maps finite complex data, including extrapolation outside the grid.
func (c *ComplexPlane) Point(z complex128) (geom.Vec3, error) {
	origin, x, y, err := c.basis()
	if err != nil {
		return geom.Vec3{}, err
	}
	return c.point(z, origin, x, y)
}

func (c *ComplexPlane) point(z complex128, origin, x, y geom.Vec3) (geom.Vec3, error) {
	if !finite(real(z)) || !finite(imag(z)) {
		return geom.Vec3{}, fmt.Errorf("complex coordinate must be finite")
	}
	p := origin.Add(x.Mul((real(z) - c.x.Min) / (c.x.Max - c.x.Min))).Add(y.Mul((imag(z) - c.y.Min) / (c.y.Max - c.y.Min)))
	if !p.Finite() {
		return geom.Vec3{}, fmt.Errorf("complex coordinate mapping overflows")
	}
	return p, nil
}

// Number inverts the current affine mapping. A point off the plane is first
// projected orthogonally onto it. Frames within 1e-12 sine angle of collinearity
// reject; large finite extrapolation can also exceed binary64 range.
func (c *ComplexPlane) Number(p geom.Vec3) (complex128, error) {
	origin, x, y, err := c.basis()
	if err != nil {
		return 0, err
	}
	d := p.Sub(origin)
	if !p.Finite() || !d.Finite() {
		return 0, fmt.Errorf("complex inverse requires finite representable displacement")
	}
	sx, sy := complexVectorScale(x), complexVectorScale(y)
	if d == (geom.Vec3{}) {
		return complex(c.x.Min, c.y.Min), nil
	}
	u, v := complexDivide(x, sx), complexDivide(y, sy)
	n := u.Cross(v)
	nn := n.Dot(n)
	// Apply dual coordinates componentwise: scaling the whole displacement
	// by its largest component would erase an orthogonal tiny-axis component.
	dualX, dualY := v.Cross(n).Mul(1/nn), n.Cross(u).Mul(1/nn)
	a := complexDotRatio(d, dualX, sx)
	b := complexDotRatio(d, dualY, sy)
	z := complex(c.x.Min+a*(c.x.Max-c.x.Min), c.y.Min+b*(c.y.Max-c.y.Min))
	if !finite(a) || !finite(b) || !finite(real(z)) || !finite(imag(z)) {
		return 0, fmt.Errorf("complex inverse overflows")
	}
	return z, nil
}

// GridLines returns borrowed grid children in an independent selection slice.
func (c *ComplexPlane) GridLines() []*gomath.Object {
	if c == nil || c.plane == nil {
		return nil
	}
	return c.plane.GridLines()
}

// Plot samples a complex trajectory at uniform parameter intervals. Nonfinite
// callback values break contours; finite mapped overflow returns an error.
// The result is a geometry snapshot and does not follow later plane transforms.
func (c *ComplexPlane) Plot(f func(float64) complex128, start, end float64, samples int) (*gomath.Object, error) {
	if f == nil || !finite(start) || !finite(end) || end <= start || !finite(end-start) || samples < 2 || samples > 1_000_000 {
		return nil, fmt.Errorf("complex plot requires a finite increasing interval and 2..1000000 samples")
	}
	origin, x, y, err := c.basis()
	if err != nil {
		return nil, err
	}
	var builder path.Builder
	open := false
	for i := range samples {
		z := f(partitionPoint(start, end, i, samples-1))
		if !finite(real(z)) || !finite(imag(z)) {
			open = false
			continue
		}
		p, err := c.point(z, origin, x, y)
		if err != nil {
			return nil, err
		}
		if open {
			builder.LineTo(p)
		} else {
			builder.MoveTo(p)
			open = true
		}
	}
	return gomath.NewObject(builder.Path()).SetStroke(gomath.Blue, .035), nil
}

func complexVectorScale(v geom.Vec3) float64 {
	return math.Max(math.Abs(v.X), math.Max(math.Abs(v.Y), math.Abs(v.Z)))
}

func complexDivide(v geom.Vec3, scale float64) geom.Vec3 {
	return geom.Vec3{X: v.X / scale, Y: v.Y / scale, Z: v.Z / scale}
}

func complexDotRatio(v, dual geom.Vec3, denominator float64) float64 {
	return complexScaledRatio(v.X, dual.X, denominator) +
		complexScaledRatio(v.Y, dual.Y, denominator) +
		complexScaledRatio(v.Z, dual.Z, denominator)
}

// Avoid overflowing scale/denominator before a small coefficient is applied.
func complexScaledRatio(value, scale, denominator float64) float64 {
	if value == 0 {
		return value
	}
	mv, ev := math.Frexp(value)
	ms, es := math.Frexp(scale)
	md, ed := math.Frexp(denominator)
	return math.Ldexp(mv*ms/md, ev+es-ed)
}
