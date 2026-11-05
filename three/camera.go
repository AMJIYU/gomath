package three

import (
	"fmt"
	"gomath/geom"
	"math"
)

type Projection uint8

const (
	Perspective Projection = iota
	Orthographic
)

// CameraMode selects one explicit frame representation. LookAtCamera is the
// zero/default mode. OrientedCamera uses a camera-to-world quaternion: local
// +X right, +Y up, -Z forward. Both modes expose positive forward view depth.
type CameraMode uint8

const (
	LookAtCamera CameraMode = iota
	OrientedCamera
)

// Camera uses an explicit look-at or quaternion frame. OrientedCamera requires
// zero Target/Up; LookAtCamera requires zero Orientation. Position is the eye
// in both modes, with positive distance along the forward view axis.
// FOVY is a vertical angle in radians; OrthoHeight is a vertical world extent.
// Only the field belonging to Projection is used.
type Camera struct {
	Mode                 CameraMode
	Orientation          geom.Quat // OrientedCamera only; finite and nonzero
	Position, Target, Up geom.Vec3
	Projection           Projection
	Near, Far            float64
	FOVY, OrthoHeight    float64
}

type cameraFrame struct {
	right, up, forward geom.Vec3
	sx, sy             float64
	perspective        bool
}

func (c Camera) frame(aspect float64) (cameraFrame, error) {
	var f cameraFrame
	if !bounded(c.Position) || !bounded(c.Target) || !c.Up.Finite() || !finite(c.Near) || !finite(c.Far) || c.Near < 1e-6 || c.Far <= c.Near || c.Far > MaxCoordinate || !finite(aspect) || aspect <= 0 {
		return f, fmt.Errorf("three: camera needs finite bounded coordinates and 1e-6 <= near < far <= %g", MaxCoordinate)
	}
	switch c.Mode {
	case LookAtCamera:
		if c.Orientation != (geom.Quat{}) {
			return f, fmt.Errorf("three: look-at camera requires zero Orientation")
		}
		var ok bool
		f.forward, ok = unit(c.Target.Sub(c.Position))
		if !ok {
			return f, fmt.Errorf("three: camera position and target must differ")
		}
		u, ok := unit(c.Up)
		if !ok {
			return f, fmt.Errorf("three: camera up must be finite and nonzero")
		}
		r := f.forward.Cross(u)
		if r.Norm() < 1e-10 {
			return f, fmt.Errorf("three: camera up must not be parallel to view direction")
		}
		f.right, _ = unit(r)
		f.up, _ = unit(f.right.Cross(f.forward))
	case OrientedCamera:
		if c.Target != (geom.Vec3{}) || c.Up != (geom.Vec3{}) {
			return f, fmt.Errorf("three: oriented camera requires zero Target and Up")
		}
		m, err := c.Orientation.Matrix()
		if err != nil {
			return f, fmt.Errorf("three: camera orientation: %w", err)
		}
		f.right = geom.Vec3{X: m[0], Y: m[4], Z: m[8]}
		f.up = geom.Vec3{X: m[1], Y: m[5], Z: m[9]}
		f.forward = geom.Vec3{X: -m[2], Y: -m[6], Z: -m[10]}
	default:
		return f, fmt.Errorf("three: unknown camera mode %d", c.Mode)
	}
	switch c.Projection {
	case Perspective:
		if !finite(c.FOVY) || c.FOVY < 1e-4 || c.FOVY > math.Pi-1e-4 {
			return f, fmt.Errorf("three: vertical FOV must be in [1e-4, pi-1e-4] radians")
		}
		f.perspective = true
		f.sy = math.Tan(c.FOVY / 2)
	case Orthographic:
		if !finite(c.OrthoHeight) || c.OrthoHeight < 1e-6 || c.OrthoHeight > MaxCoordinate {
			return f, fmt.Errorf("three: ortho height must be in [1e-6, %g]", MaxCoordinate)
		}
		f.sy = c.OrthoHeight / 2
	default:
		return f, fmt.Errorf("three: unknown camera projection %d", c.Projection)
	}
	f.sx = f.sy * aspect
	if !finite(f.sx) || f.sx <= 0 {
		return f, fmt.Errorf("three: camera aspect overflows projection")
	}
	return f, nil
}

func (c Camera) Validate() error { _, err := c.frame(1); return err }

func (f cameraFrame) view(c Camera, p geom.Vec3) geom.Vec3 {
	p = p.Sub(c.Position)
	return geom.Vec3{X: p.Dot(f.right), Y: p.Dot(f.up), Z: p.Dot(f.forward)}
}

// WorldToView returns x right, y up, and positive z forward. In quaternion
// mode this equals diag(1,1,-1)*R(Orientation)^T*(p-Position).
func (c Camera) WorldToView(p geom.Vec3) (geom.Vec3, error) {
	f, err := c.frame(1)
	if err != nil {
		return geom.Vec3{}, err
	}
	if !bounded(p) {
		return geom.Vec3{}, fmt.Errorf("three: point outside coordinate bounds")
	}
	return f.view(c, p), nil
}

// ProjectedPoint uses image coordinates (x right, y down), with the viewport
// boundary at 0,width and 0,height. Depth is linear view distance, not NDC z.
type ProjectedPoint struct {
	X, Y, Depth float64
	Inside      bool
}

// Project projects a single point; points beyond clipping planes have Inside
// false. Perspective points on/behind the eye return an error. Rendering uses
// polygon clipping, not this point visibility flag.
func (c Camera) Project(p geom.Vec3, width, height int) (ProjectedPoint, error) {
	var out ProjectedPoint
	if err := validateSize(width, height); err != nil {
		return out, err
	}
	f, err := c.frame(float64(width) / float64(height))
	if err != nil {
		return out, err
	}
	if !bounded(p) {
		return out, fmt.Errorf("three: point outside coordinate bounds")
	}
	v := f.view(c, p)
	out.Depth = v.Z
	x, y := v.X/f.sx, v.Y/f.sy
	if f.perspective {
		if v.Z <= 0 {
			return out, fmt.Errorf("three: perspective point is on or behind the eye")
		}
		x /= v.Z
		y /= v.Z
	}
	out.X = (x + 1) * float64(width) / 2
	out.Y = (1 - y) * float64(height) / 2
	if !finite(out.X) || !finite(out.Y) {
		return ProjectedPoint{}, fmt.Errorf("three: point projection overflows")
	}
	out.Inside = v.Z >= c.Near && v.Z <= c.Far && x >= -1 && x <= 1 && y >= -1 && y <= 1
	return out, nil
}
