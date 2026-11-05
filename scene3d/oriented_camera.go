package scene3d

import (
	"fmt"
	"gomath/geom"
	"gomath/three"
	"slices"
	"sort"
)

// OrientedCameraKey uses an eye and a camera-to-world quaternion. Local axes
// are +X right, +Y up, -Z forward. Eye/lens fields interpolate linearly;
// orientation follows geom.Quat.Slerp, including its half-turn tie policy.
// Both size fields must be finite, even the unused one. Exact/held keys keep
// the normalized authored orientation; there is no look-at target or up key.
type OrientedCameraKey struct {
	Time              float64
	Position          geom.Vec3
	Orientation       geom.Quat
	FOVY, OrthoHeight float64
}

// OrientedCameraTrack fixes projection and near/far throughout the scene.
// At least one key is required. Keys count in MaxKeyframes with every other
// camera/mesh/point key, and are copied/normalized by construction.
type OrientedCameraTrack struct {
	Projection three.Projection
	Near, Far  float64
	Keys       []OrientedCameraKey
}

// NewWithOrientedCamera is an explicit alternative to New/NewWithPoints.
// It supports the same immutable mesh/point tracks and frame source, with
// no mixed look-at keys. Pass nil points for a mesh-only timeline.
func NewWithOrientedCamera(config Config, camera OrientedCameraTrack, meshes []MeshTrack, points []PointTrack) (*Scene, error) {
	return newScene(config, nil, &camera, meshes, points)
}

func (t OrientedCameraTrack) camera(k OrientedCameraKey) three.Camera {
	return three.Camera{Mode: three.OrientedCamera, Position: k.Position, Orientation: k.Orientation, Projection: t.Projection, Near: t.Near, Far: t.Far, FOVY: k.FOVY, OrthoHeight: k.OrthoHeight}
}

func (t OrientedCameraTrack) validate(duration float64) error {
	if len(t.Keys) == 0 {
		return fmt.Errorf("scene3d: oriented camera needs at least one key")
	}
	for i, k := range t.Keys {
		if !validTime(k.Time, duration) || i > 0 && k.Time <= t.Keys[i-1].Time {
			return fmt.Errorf("scene3d: oriented camera key %d needs strictly increasing times in [0,%g]", i, duration)
		}
		if !finite(k.FOVY) || !finite(k.OrthoHeight) {
			return fmt.Errorf("scene3d: oriented camera key %d requires finite lens fields", i)
		}
		if err := t.camera(k).Validate(); err != nil {
			return fmt.Errorf("scene3d: oriented camera key %d: %w", i, err)
		}
	}
	return nil
}

func snapshotOrientedCamera(t OrientedCameraTrack) *OrientedCameraTrack {
	t.Keys = slices.Clone(t.Keys)
	for i := range t.Keys {
		t.Keys[i].Orientation, _ = t.Keys[i].Orientation.Normalize()
	} // validated
	return &t
}

func (t OrientedCameraTrack) at(time float64) (three.Camera, error) {
	i := sort.Search(len(t.Keys), func(i int) bool { return t.Keys[i].Time > time })
	k := t.Keys[0]
	if i > 0 {
		k = t.Keys[i-1]
		if i < len(t.Keys) && time != k.Time {
			b := t.Keys[i]
			u := (time - k.Time) / (b.Time - k.Time)
			var err error
			k.Orientation, err = k.Orientation.Slerp(b.Orientation, u)
			if err != nil {
				return three.Camera{}, fmt.Errorf("scene3d: oriented camera at %g: %w", time, err)
			}
			k.Position = k.Position.Lerp(b.Position, u)
			k.FOVY, k.OrthoHeight = lerp(k.FOVY, b.FOVY, u), lerp(k.OrthoHeight, b.OrthoHeight, u)
		}
	}
	c := t.camera(k)
	if err := c.Validate(); err != nil {
		return three.Camera{}, fmt.Errorf("scene3d: oriented camera at %g: %w", time, err)
	}
	return c, nil
}
