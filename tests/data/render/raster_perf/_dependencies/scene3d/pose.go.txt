package scene3d

import (
	"fmt"
	"gomath/geom"
	"math"
)

const (
	MinScale = 1e-6
	MaxScale = 1e6
	// MaxScaleRatio keeps TRS models safely away from three's singular-model
	// threshold. Each component's sign must stay fixed along a track.
	MaxScaleRatio = 1e4
	MaxAngle      = 1e6 * math.Pi
)

// Pose is T(Position)*R(track.Axis,Angle)*S(Scale), about the mesh's local
// origin. Angles are radians, interpolated as written, including full turns.
// A zero Pose has invalid scale; use IdentityPose or omit a track's Keys.
// In RotationKeys mode Angle must be zero; orientation comes from those keys.
type Pose struct {
	Position geom.Vec3
	Scale    geom.Vec3
	Angle    float64
}

func IdentityPose() Pose { return Pose{Scale: geom.Vec3{X: 1, Y: 1, Z: 1}} }

type PoseKey struct {
	Time float64
	Pose Pose
}

func finite(x float64) bool { return !math.IsNaN(x) && !math.IsInf(x, 0) }

func bounded(v geom.Vec3) bool {
	return v.Finite() && math.Abs(v.X) <= 1e12 && math.Abs(v.Y) <= 1e12 && math.Abs(v.Z) <= 1e12
}

func (p Pose) validate() error {
	if !bounded(p.Position) || !finite(p.Angle) || math.Abs(p.Angle) > MaxAngle {
		return fmt.Errorf("scene3d: pose needs position components within +/-1e12 and angle within +/- %g", MaxAngle)
	}
	a, b, c := math.Abs(p.Scale.X), math.Abs(p.Scale.Y), math.Abs(p.Scale.Z)
	hi, lo := math.Max(a, math.Max(b, c)), math.Min(a, math.Min(b, c))
	if !p.Scale.Finite() || lo < MinScale || hi > MaxScale || hi/lo > MaxScaleRatio {
		return fmt.Errorf("scene3d: absolute scale components must be in [%g,%g], with largest/smallest <= %g", MinScale, MaxScale, MaxScaleRatio)
	}
	return nil
}

func rotationAxis(axis geom.Vec3) (geom.Vec3, error) {
	if !bounded(axis) {
		return geom.Vec3{}, fmt.Errorf("scene3d: rotation axis must be finite with components within +/-1e12")
	}
	if axis == (geom.Vec3{}) {
		return geom.Vec3{Z: 1}, nil
	}
	n := math.Hypot(math.Hypot(axis.X, axis.Y), axis.Z)
	return geom.Vec3{X: axis.X / n, Y: axis.Y / n, Z: axis.Z / n}, nil
}

func (p Pose) matrix(axis geom.Vec3) geom.Mat4 {
	return geom.Translation(p.Position).Mul(geom.Rotation(axis, p.Angle)).Mul(geom.Scaling(p.Scale.X, p.Scale.Y, p.Scale.Z))
}

func lerp(a, b, u float64) float64 { return a*(1-u) + b*u }

func interpolatePose(a, b Pose, u float64) Pose {
	return Pose{Position: a.Position.Lerp(b.Position, u), Scale: a.Scale.Lerp(b.Scale, u), Angle: lerp(a.Angle, b.Angle, u)}
}
