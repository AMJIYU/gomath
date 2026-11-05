package geom

import (
	"fmt"
	"math"
)

// Quat stores scalar-first W,X,Y,Z Hamilton components. Unit quaternions act
// actively on column vectors in a right-handed frame: +Z by pi/2 sends +X to
// +Y. q and -q represent the same rotation. The zero value is not a rotation.
type Quat struct{ W, X, Y, Z float64 }

func IdentityQuat() Quat { return Quat{W: 1} }

// Normalize accepts every finite nonzero magnitude, including subnormals and
// components whose unscaled squared norm would overflow. It preserves sign.
func (q Quat) Normalize() (Quat, error) {
	if !finite(q.W) || !finite(q.X) || !finite(q.Y) || !finite(q.Z) {
		return Quat{}, fmt.Errorf("geom: quaternion must be finite and nonzero")
	}
	s := math.Max(math.Max(math.Abs(q.W), math.Abs(q.X)), math.Max(math.Abs(q.Y), math.Abs(q.Z)))
	if s == 0 {
		return Quat{}, fmt.Errorf("geom: quaternion must be finite and nonzero")
	}
	q = Quat{q.W / s, q.X / s, q.Y / s, q.Z / s}
	n := math.Sqrt(q.W*q.W + q.X*q.X + q.Y*q.Y + q.Z*q.Z)
	return Quat{q.W / n, q.X / n, q.Y / n, q.Z / n}, nil
}

// QuatFromAxisAngle creates a unit quaternion from any finite nonzero axis
// and finite angle in radians. A zero axis errors even for a zero angle.
func QuatFromAxisAngle(axis Vec3, angle float64) (Quat, error) {
	if !axis.Finite() || !finite(angle) {
		return Quat{}, fmt.Errorf("geom: axis and angle must be finite")
	}
	u, err := (Quat{X: axis.X, Y: axis.Y, Z: axis.Z}).Normalize()
	if err != nil {
		return Quat{}, fmt.Errorf("geom: rotation axis must be nonzero")
	}
	s, c := math.Sincos(angle / 2)
	return (Quat{c, u.X * s, u.Y * s, u.Z * s}).Normalize()
}

// Mul is the raw Hamilton product: a.Mul(b) applies b then a for unit inputs.
// It does not normalize or validate. Arbitrary component arithmetic follows
// IEEE-754 (including overflow); normalize inputs first for rotation products.
func (a Quat) Mul(b Quat) Quat {
	return Quat{
		a.W*b.W - a.X*b.X - a.Y*b.Y - a.Z*b.Z,
		a.W*b.X + a.X*b.W + a.Y*b.Z - a.Z*b.Y,
		a.W*b.Y - a.X*b.Z + a.Y*b.W + a.Z*b.X,
		a.W*b.Z + a.X*b.Y - a.Y*b.X + a.Z*b.W,
	}
}

// Conjugate negates vector components. It is the inverse for unit inputs.
func (q Quat) Conjugate() Quat { return Quat{q.W, -q.X, -q.Y, -q.Z} }

// Matrix normalizes q and returns a row-major homogeneous rotation matrix,
// compatible with Mat4.Apply and Translation.Mul(rotation).Mul(Scaling).
func (q Quat) Matrix() (Mat4, error) {
	q, err := q.Normalize()
	if err != nil {
		return Mat4{}, err
	}
	x, y, z, w := q.X, q.Y, q.Z, q.W
	return Mat4{
		1 - 2*(y*y+z*z), 2 * (x*y - z*w), 2 * (x*z + y*w), 0,
		2 * (x*y + z*w), 1 - 2*(x*x+z*z), 2 * (y*z - x*w), 0,
		2 * (x*z - y*w), 2 * (y*z + x*w), 1 - 2*(x*x+y*y), 0,
		0, 0, 0, 1,
	}, nil
}

// Rotate normalizes q and rotates a finite vector. Overflow in the resulting
// vector is reported; there is no zero/identity fallback for invalid inputs.
func (q Quat) Rotate(v Vec3) (Vec3, error) {
	m, err := q.Matrix()
	if err != nil {
		return Vec3{}, err
	}
	if !v.Finite() {
		return Vec3{}, fmt.Errorf("geom: rotated vector must be finite")
	}
	v = m.Apply(v)
	if !v.Finite() {
		return Vec3{}, fmt.Errorf("geom: rotated vector overflow")
	}
	return v, nil
}

// Slerp normalizes endpoints and follows their shortest rotational arc for
// finite t in [0,1]. Antipodal quaternions are the same rotation. At exactly
// 180 degrees (dot==0), choose the relative quaternion whose first nonzero
// X,Y,Z component is positive, as in SciPy Rotation's canonical rotvec.
// Exact endpoints retain their normalized input signs. Floating roundoff can
// choose either equally short arc for endpoints extremely close to 180 degrees.
func (a Quat) Slerp(b Quat, t float64) (Quat, error) {
	if !finite(t) || t < 0 || t > 1 {
		return Quat{}, fmt.Errorf("geom: slerp parameter must be in [0,1]")
	}
	a, err := a.Normalize()
	if err != nil {
		return Quat{}, err
	}
	b, err = b.Normalize()
	if err != nil {
		return Quat{}, err
	}
	if t == 0 {
		return a, nil
	}
	if t == 1 {
		return b, nil
	}
	dot := a.W*b.W + a.X*b.X + a.Y*b.Y + a.Z*b.Z
	flip := dot < 0
	if dot == 0 {
		relative := a.Conjugate().Mul(b)
		flip = relative.X < 0 || relative.X == 0 && (relative.Y < 0 || relative.Y == 0 && relative.Z < 0)
	}
	if flip {
		b, dot = (Quat{-b.W, -b.X, -b.Y, -b.Z}), -dot
	}
	dot = math.Min(1, dot)
	// The limiting linear blend avoids division by tiny sin(theta). Use a
	// tight threshold so the constant-speed error stays near roundoff.
	x, y := 1-t, t
	if dot < 1-1e-12 {
		theta := math.Acos(dot)
		s := math.Sin(theta)
		x, y = math.Sin((1-t)*theta)/s, math.Sin(t*theta)/s
	}
	return (Quat{a.W*x + b.W*y, a.X*x + b.X*y, a.Y*x + b.Y*y, a.Z*x + b.Z*y}).Normalize()
}
