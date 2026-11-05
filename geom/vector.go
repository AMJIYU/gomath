// Package geom contains allocation-free geometric primitives.
package geom

import "math"

type Vec3 struct{ X, Y, Z float64 }

func V(x, y float64) Vec3         { return Vec3{X: x, Y: y} }
func (a Vec3) Add(b Vec3) Vec3    { return Vec3{a.X + b.X, a.Y + b.Y, a.Z + b.Z} }
func (a Vec3) Sub(b Vec3) Vec3    { return Vec3{a.X - b.X, a.Y - b.Y, a.Z - b.Z} }
func (a Vec3) Mul(s float64) Vec3 { return Vec3{a.X * s, a.Y * s, a.Z * s} }
func (a Vec3) Dot(b Vec3) float64 { return a.X*b.X + a.Y*b.Y + a.Z*b.Z }
func (a Vec3) Cross(b Vec3) Vec3 {
	return Vec3{a.Y*b.Z - a.Z*b.Y, a.Z*b.X - a.X*b.Z, a.X*b.Y - a.Y*b.X}
}
func (a Vec3) Norm() float64 { return math.Sqrt(a.Dot(a)) }
func (a Vec3) Unit() Vec3 {
	if n := a.Norm(); n > 0 {
		return a.Mul(1 / n)
	}
	return Vec3{}
}
func (a Vec3) Lerp(b Vec3, t float64) Vec3 { return a.Mul(1 - t).Add(b.Mul(t)) }
func (a Vec3) Finite() bool                { return finite(a.X) && finite(a.Y) && finite(a.Z) }
func finite(v float64) bool                { return !math.IsNaN(v) && !math.IsInf(v, 0) }

// Mat4 is a row-major matrix applied to column vectors.
type Mat4 [16]float64

func Identity() Mat4               { return Mat4{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1} }
func Translation(v Vec3) Mat4      { m := Identity(); m[3], m[7], m[11] = v.X, v.Y, v.Z; return m }
func Scaling(x, y, z float64) Mat4 { m := Identity(); m[0], m[5], m[10] = x, y, z; return m }
func RotationZ(a float64) Mat4 {
	c, s := math.Cos(a), math.Sin(a)
	return Mat4{c, -s, 0, 0, s, c, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}
}
func Rotation(axis Vec3, angle float64) Mat4 {
	if axis.Norm() == 0 {
		return Identity()
	}
	u := axis.Unit()
	c, s := math.Cos(angle), math.Sin(angle)
	t := 1 - c
	x, y, z := u.X, u.Y, u.Z
	return Mat4{t*x*x + c, t*x*y - s*z, t*x*z + s*y, 0, t*x*y + s*z, t*y*y + c, t*y*z - s*x, 0, t*x*z - s*y, t*y*z + s*x, t*z*z + c, 0, 0, 0, 0, 1}
}
func (a Mat4) Mul(b Mat4) Mat4 {
	var c Mat4
	for i := range 4 {
		for j := range 4 {
			for k := range 4 {
				c[4*i+j] += a[4*i+k] * b[4*k+j]
			}
		}
	}
	return c
}
func (a Mat4) Apply(v Vec3) Vec3 {
	x := a[0]*v.X + a[1]*v.Y + a[2]*v.Z + a[3]
	y := a[4]*v.X + a[5]*v.Y + a[6]*v.Z + a[7]
	z := a[8]*v.X + a[9]*v.Y + a[10]*v.Z + a[11]
	w := a[12]*v.X + a[13]*v.Y + a[14]*v.Z + a[15]
	if w != 0 && w != 1 {
		x /= w
		y /= w
		z /= w
	}
	return Vec3{x, y, z}
}
func About(point Vec3, m Mat4) Mat4 { return Translation(point).Mul(m).Mul(Translation(point.Mul(-1))) }

type Bounds struct {
	Min, Max Vec3
	Valid    bool
}

func (b *Bounds) Include(p Vec3) {
	if !b.Valid {
		b.Min, b.Max, b.Valid = p, p, true
		return
	}
	b.Min = Vec3{math.Min(b.Min.X, p.X), math.Min(b.Min.Y, p.Y), math.Min(b.Min.Z, p.Z)}
	b.Max = Vec3{math.Max(b.Max.X, p.X), math.Max(b.Max.Y, p.Y), math.Max(b.Max.Z, p.Z)}
}
func (b Bounds) Center() Vec3 { return b.Min.Lerp(b.Max, .5) }
func (b Bounds) Size() Vec3   { return b.Max.Sub(b.Min) }
