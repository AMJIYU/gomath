// Package path represents vector outlines as cubic Bezier contours.
package path

import (
	"fmt"
	"gomath/geom"
	"math"
)

type Cubic struct{ P0, P1, P2, P3 geom.Vec3 }

func Line(a, b geom.Vec3) Cubic { return Cubic{a, a.Lerp(b, 1.0/3), a.Lerp(b, 2.0/3), b} }
func (c Cubic) At(t float64) geom.Vec3 {
	a := c.P0.Lerp(c.P1, t)
	b := c.P1.Lerp(c.P2, t)
	d := c.P2.Lerp(c.P3, t)
	return a.Lerp(b, t).Lerp(b.Lerp(d, t), t)
}
func (c Cubic) Split(t float64) (Cubic, Cubic) {
	a, b, d := c.P0.Lerp(c.P1, t), c.P1.Lerp(c.P2, t), c.P2.Lerp(c.P3, t)
	e, f := a.Lerp(b, t), b.Lerp(d, t)
	p := e.Lerp(f, t)
	return Cubic{c.P0, a, e, p}, Cubic{p, f, d, c.P3}
}
func (c Cubic) Transform(m geom.Mat4) Cubic {
	return Cubic{m.Apply(c.P0), m.Apply(c.P1), m.Apply(c.P2), m.Apply(c.P3)}
}
func (c Cubic) Lerp(b Cubic, t float64) Cubic {
	return Cubic{c.P0.Lerp(b.P0, t), c.P1.Lerp(b.P1, t), c.P2.Lerp(b.P2, t), c.P3.Lerp(b.P3, t)}
}
func (c Cubic) controlLength() float64 {
	return c.P1.Sub(c.P0).Norm() + c.P2.Sub(c.P1).Norm() + c.P3.Sub(c.P2).Norm()
}

type Contour struct {
	Start  geom.Vec3
	Curves []Cubic
	Closed bool
}
type Path struct{ Contours []Contour }

func (p Path) Clone() Path {
	q := Path{Contours: make([]Contour, len(p.Contours))}
	for i, c := range p.Contours {
		q.Contours[i] = c
		q.Contours[i].Curves = append([]Cubic(nil), c.Curves...)
	}
	return q
}
func (p Path) Transform(m geom.Mat4) Path {
	q := p.Clone()
	for i := range q.Contours {
		c := &q.Contours[i]
		c.Start = m.Apply(c.Start)
		for j := range c.Curves {
			c.Curves[j] = c.Curves[j].Transform(m)
		}
	}
	return q
}
func (p Path) Bounds() geom.Bounds {
	var b geom.Bounds
	for _, c := range p.Contours {
		b.Include(c.Start)
		for _, v := range c.Curves {
			b.Include(v.P0)
			b.Include(v.P3)
			for axis := range 3 {
				coord := func(v geom.Vec3) float64 {
					switch axis {
					case 0:
						return v.X
					case 1:
						return v.Y
					default:
						return v.Z
					}
				}
				a, b1, c1, d := coord(v.P0), coord(v.P1), coord(v.P2), coord(v.P3)
				A, B, C := -a+3*b1-3*c1+d, 2*(a-2*b1+c1), b1-a
				for _, t := range quadraticRoots(A, B, C) {
					if t > 0 && t < 1 {
						b.Include(v.At(t))
					}
				}
			}
		}
	}
	return b
}
func quadraticRoots(a, b, c float64) []float64 {
	if math.Abs(a) < 1e-14 {
		if math.Abs(b) < 1e-14 {
			return nil
		}
		return []float64{-c / b}
	}
	d := b*b - 4*a*c
	if d < 0 {
		return nil
	}
	r := math.Sqrt(d)
	return []float64{(-b - r) / (2 * a), (-b + r) / (2 * a)}
}
func (p Path) Validate() error {
	for i, c := range p.Contours {
		if !c.Start.Finite() {
			return fmt.Errorf("contour %d has non-finite start", i)
		}
		prev := c.Start
		for j, v := range c.Curves {
			if !v.P0.Finite() || !v.P1.Finite() || !v.P2.Finite() || !v.P3.Finite() {
				return fmt.Errorf("contour %d curve %d has non-finite coordinates", i, j)
			}
			if v.P0.Sub(prev).Norm() > 1e-8 {
				return fmt.Errorf("contour %d curve %d is disconnected", i, j)
			}
			prev = v.P3
		}
	}
	return nil
}

type Builder struct{ p Path }

func (b *Builder) MoveTo(p geom.Vec3) { b.p.Contours = append(b.p.Contours, Contour{Start: p}) }
func (b *Builder) last() *Contour {
	if len(b.p.Contours) == 0 {
		b.MoveTo(geom.Vec3{})
	}
	return &b.p.Contours[len(b.p.Contours)-1]
}
func (b *Builder) current() geom.Vec3 {
	c := b.last()
	if len(c.Curves) == 0 {
		return c.Start
	}
	return c.Curves[len(c.Curves)-1].P3
}
func (b *Builder) LineTo(p geom.Vec3) {
	a := b.current()
	c := b.last()
	c.Curves = append(c.Curves, Line(a, p))
}
func (b *Builder) CubicTo(a, c, p geom.Vec3) {
	s := b.current()
	v := b.last()
	v.Curves = append(v.Curves, Cubic{s, a, c, p})
}
func (b *Builder) QuadTo(c, p geom.Vec3) {
	s := b.current()
	b.CubicTo(s.Lerp(c, 2.0/3), p.Lerp(c, 2.0/3), p)
}
func (b *Builder) Close() {
	c := b.last()
	if b.current().Sub(c.Start).Norm() > 1e-12 {
		b.LineTo(c.Start)
	}
	c.Closed = true
}
func (b *Builder) Path() Path { return b.p.Clone() }

type Polyline struct {
	Points []geom.Vec3
	Closed bool
}

// Flatten adaptively subdivides using a control-polygon error bound. Tolerance
// is in path units; a depth cap also bounds pathological input.
func (p Path) Flatten(tolerance float64) []Polyline {
	if tolerance <= 0 || math.IsNaN(tolerance) {
		tolerance = 1e-3
	}
	out := make([]Polyline, 0, len(p.Contours))
	for _, c := range p.Contours {
		v := Polyline{Points: []geom.Vec3{c.Start}, Closed: c.Closed}
		for _, s := range c.Curves {
			flatten(s, tolerance, 0, &v.Points)
		}
		out = append(out, v)
	}
	return out
}
func flatten(c Cubic, tol float64, depth int, out *[]geom.Vec3) {
	d := c.P3.Sub(c.P0)
	n := d.Norm()
	err := math.Max(c.P1.Sub(c.P0).Norm(), c.P2.Sub(c.P0).Norm())
	if n > 1e-15 {
		err = math.Max(c.P1.Sub(c.P0).Cross(d).Norm()/n, c.P2.Sub(c.P0).Cross(d).Norm()/n) // Avoid flattening collinear curves that double back.
		if c.controlLength()-n > tol {
			err = math.Max(err, (c.controlLength()-n)/2)
		}
	}
	if err <= tol || depth >= 18 {
		*out = append(*out, c.P3)
		return
	}
	a, b := c.Split(.5)
	flatten(a, tol, depth+1, out)
	flatten(b, tol, depth+1, out)
}

// Partial returns an ordered prefix in cubic parameter space, used by Create.
func (p Path) Partial(alpha float64) Path {
	if alpha >= 1 {
		return p.Clone()
	}
	if alpha <= 0 {
		return Path{}
	}
	total := 0
	for _, c := range p.Contours {
		total += len(c.Curves)
	}
	remain := alpha * float64(total)
	var out Path
	for _, c := range p.Contours {
		if remain <= 0 {
			break
		}
		q := Contour{Start: c.Start}
		for _, v := range c.Curves {
			if remain >= 1 {
				q.Curves = append(q.Curves, v)
				remain--
				continue
			}
			if remain > 0 {
				a, _ := v.Split(remain)
				q.Curves = append(q.Curves, a)
				remain = 0
			}
			break
		}
		q.Closed = c.Closed && len(q.Curves) == len(c.Curves) && remain >= 0 && len(q.Curves) > 0 && q.Curves[len(q.Curves)-1].P3.Sub(q.Start).Norm() < 1e-12
		out.Contours = append(out.Contours, q)
	}
	return out
}

// Align preserves each cubic exactly using de Casteljau subdivision. Contours
// currently correspond by index; shape-semantic matching is a separate layer.
func Align(a, b Path) (Path, Path) {
	a, b = a.Clone(), b.Clone()
	n := max(len(a.Contours), len(b.Contours))
	centerA, centerB := a.Bounds().Center(), b.Bounds().Center()
	for len(a.Contours) < n {
		a.Contours = append(a.Contours, Contour{Start: centerA})
	}
	for len(b.Contours) < n {
		b.Contours = append(b.Contours, Contour{Start: centerB})
	}
	for i := range n {
		ca, cb := &a.Contours[i], &b.Contours[i]
		nseg := max(len(ca.Curves), len(cb.Curves))
		subdivide(ca, nseg)
		subdivide(cb, nseg)
	}
	return a, b
}
func subdivide(c *Contour, n int) {
	if n == 0 {
		return
	}
	if len(c.Curves) == 0 {
		c.Curves = []Cubic{Line(c.Start, c.Start)}
	}
	for len(c.Curves) < n {
		idx := 0
		for i := 1; i < len(c.Curves); i++ {
			if c.Curves[i].controlLength() > c.Curves[idx].controlLength() {
				idx = i
			}
		}
		a, b := c.Curves[idx].Split(.5)
		c.Curves = append(c.Curves, Cubic{})
		copy(c.Curves[idx+2:], c.Curves[idx+1:])
		c.Curves[idx], c.Curves[idx+1] = a, b
	}
}
func InterpolateAligned(a, b Path, t float64) Path {
	q := a.Clone()
	for i := range q.Contours {
		q.Contours[i].Start = a.Contours[i].Start.Lerp(b.Contours[i].Start, t)
		q.Contours[i].Closed = a.Contours[i].Closed && b.Contours[i].Closed
		for j := range q.Contours[i].Curves {
			q.Contours[i].Curves[j] = a.Contours[i].Curves[j].Lerp(b.Contours[i].Curves[j], t)
		}
	}
	return q
}
