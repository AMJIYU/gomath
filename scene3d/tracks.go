package scene3d

import (
	"fmt"
	"gomath/geom"
	"gomath/three"
	"sort"
)

// CameraKey holds a complete look-at camera. Projection, Up, Near and Far must
// be identical across keys. Position, Target, FOVY and OrthoHeight interpolate
// linearly; the sampled camera is validated even between valid endpoints.
type CameraKey struct {
	Time   float64
	Camera three.Camera
}

// Visibility is [Add,Remove). Remove=0 means no removal, including the exact
// scene endpoint. Entries must be ordered and disjoint; touching is allowed.
type Visibility struct{ Add, Remove float64 }

// MeshTrack has a scene-local, positive, unique ID. New snapshots every mesh
// slice and the keys/visibility slices. Material is copied; its immutable
// Texture may be shared. Axis is fixed in local coordinates; zero means +Z.
// No Keys means identity for the whole scene. No Visible entries means always
// visible, including the endpoint. Keys use absolute scene seconds, not local
// elapsed time since an Add event; removal/re-addition does not restart motion.
// Exactly one of Mesh or nonempty MeshKeys is required. Geometry keys have fixed
// topology. MaterialKeys replace only Color; Texture and Sampler remain fixed.
// Optional RotationKeys replace fixed-axis rotation and require zero Axis and
// zero PoseKey angles. PoseKeys still provide translation and signed scale.
type MeshTrack struct {
	ID           uint64
	Mesh         *three.Mesh
	MeshKeys     []MeshKey
	Material     three.Material
	MaterialKeys []MaterialKey
	RotationKeys []RotationKey
	Axis         geom.Vec3
	Keys         []PoseKey
	Visible      []Visibility
}

func validTime(t, duration float64) bool { return finite(t) && t >= 0 && t <= duration }

func validateCameraKeys(keys []CameraKey, duration float64) error {
	if len(keys) == 0 {
		return fmt.Errorf("scene3d: at least one camera key is required")
	}
	base := keys[0].Camera
	for i, key := range keys {
		if !validTime(key.Time, duration) || (i > 0 && key.Time <= keys[i-1].Time) {
			return fmt.Errorf("scene3d: camera key %d needs strictly increasing times in [0,%g]", i, duration)
		}
		c := key.Camera
		if c.Mode != three.LookAtCamera {
			return fmt.Errorf("scene3d: camera key %d requires look-at mode; use NewWithOrientedCamera", i)
		}
		if err := c.Validate(); err != nil {
			return fmt.Errorf("scene3d: camera key %d: %w", i, err)
		}
		if !finite(c.FOVY) || !finite(c.OrthoHeight) {
			return fmt.Errorf("scene3d: camera key %d requires finite FOVY and OrthoHeight, including the unused field", i)
		}
		if c.Projection != base.Projection || c.Up != base.Up || c.Near != base.Near || c.Far != base.Far {
			return fmt.Errorf("scene3d: camera key %d changes fixed Projection, Up, Near or Far", i)
		}
	}
	return nil
}

func (t MeshTrack) validate(duration float64) error {
	if t.ID == 0 {
		return fmt.Errorf("scene3d: mesh track ID must be positive")
	}
	if err := validateRotations(t.ID, t.Axis, t.Keys, t.RotationKeys, duration); err != nil {
		return err
	}
	base, err := t.meshSource()
	if err != nil {
		return err
	}
	if err := base.Validate(); err != nil {
		return fmt.Errorf("scene3d: track %d mesh: %w", t.ID, err)
	}
	for i, key := range t.MeshKeys {
		if !validTime(key.Time, duration) || i > 0 && key.Time <= t.MeshKeys[i-1].Time {
			return fmt.Errorf("scene3d: track %d mesh key %d needs strictly increasing times in [0,%g]", t.ID, i, duration)
		}
		if err := key.Mesh.Validate(); err != nil {
			return fmt.Errorf("scene3d: track %d mesh key %d: %w", t.ID, i, err)
		}
		if len(key.Mesh.Vertices) != len(base.Vertices) || len(key.Mesh.Normals) != len(base.Normals) || len(key.Mesh.UVs) != len(base.UVs) || !sameTriangles(key.Mesh.Triangles, base.Triangles) {
			return fmt.Errorf("scene3d: track %d mesh key %d changes vertex count, triangle topology/order or normals/UV presence", t.ID, i)
		}
	}
	for i, key := range t.MaterialKeys {
		if !validTime(key.Time, duration) || i > 0 && key.Time <= t.MaterialKeys[i-1].Time {
			return fmt.Errorf("scene3d: track %d material key %d needs strictly increasing times in [0,%g]", t.ID, i, duration)
		}
	}
	s := t.Material.Sampler
	if (s.Filter != three.Nearest && s.Filter != three.Bilinear) || (s.AddressU != three.Clamp && s.AddressU != three.Repeat) || (s.AddressV != three.Clamp && s.AddressV != three.Repeat) {
		return fmt.Errorf("scene3d: track %d has unsupported texture sampler", t.ID)
	}
	if tex := t.Material.Texture; tex != nil {
		if _, err := tex.Sample(three.UV{}, s); err != nil {
			return fmt.Errorf("scene3d: track %d texture: %w", t.ID, err)
		}
		if len(base.UVs) != len(base.Vertices) {
			return fmt.Errorf("scene3d: track %d texture requires one UV per vertex; split seam vertices", t.ID)
		}
	}
	return validateMotion(t.ID, t.Keys, t.Visible, duration)
}

func validateMotion(id uint64, keys []PoseKey, spans []Visibility, duration float64) error {
	if id == 0 {
		return fmt.Errorf("scene3d: track ID must be positive")
	}
	for i, key := range keys {
		if !validTime(key.Time, duration) || (i > 0 && key.Time <= keys[i-1].Time) {
			return fmt.Errorf("scene3d: track %d key %d needs strictly increasing times in [0,%g]", id, i, duration)
		}
		if err := key.Pose.validate(); err != nil {
			return fmt.Errorf("scene3d: track %d key %d: %w", id, i, err)
		}
		if i > 0 {
			a, b := keys[i-1].Pose.Scale, key.Pose.Scale
			if a.X*b.X <= 0 || a.Y*b.Y <= 0 || a.Z*b.Z <= 0 {
				return fmt.Errorf("scene3d: track %d scale signs must stay fixed; interpolation through zero is unsupported", id)
			}
		}
	}
	for i, span := range spans {
		if !validTime(span.Add, duration) || !validTime(span.Remove, duration) || (span.Remove != 0 && span.Remove <= span.Add) || (i > 0 && (spans[i-1].Remove == 0 || span.Add < spans[i-1].Remove)) {
			return fmt.Errorf("scene3d: track %d visibility %d must be ordered, disjoint [Add,Remove), or Remove=0 through endpoint", id, i)
		}
	}
	return nil
}

func (t MeshTrack) at(time float64) (Pose, bool) {
	return poseAt(t.Keys, time), visibleAt(t.Visible, time)
}

func visibleAt(spans []Visibility, time float64) bool {
	if len(spans) == 0 {
		return true
	}
	i := sort.Search(len(spans), func(i int) bool { return spans[i].Add > time }) - 1
	return i >= 0 && (spans[i].Remove == 0 || time < spans[i].Remove)
}

func poseAt(keys []PoseKey, time float64) Pose {
	if len(keys) == 0 {
		return IdentityPose()
	}
	i := sort.Search(len(keys), func(i int) bool { return keys[i].Time > time })
	if i == 0 {
		return keys[0].Pose
	}
	a := keys[i-1]
	if i == len(keys) || time == a.Time {
		return a.Pose
	}
	b := keys[i]
	return interpolatePose(a.Pose, b.Pose, (time-a.Time)/(b.Time-a.Time))
}

func cameraAt(keys []CameraKey, time float64) (three.Camera, error) {
	i := sort.Search(len(keys), func(i int) bool { return keys[i].Time > time })
	if i == 0 {
		return keys[0].Camera, nil
	}
	a := keys[i-1]
	if i == len(keys) || time == a.Time {
		return a.Camera, nil
	}
	b := keys[i]
	u := (time - a.Time) / (b.Time - a.Time)
	c := a.Camera
	c.Position, c.Target = c.Position.Lerp(b.Camera.Position, u), c.Target.Lerp(b.Camera.Target, u)
	c.FOVY, c.OrthoHeight = lerp(c.FOVY, b.Camera.FOVY, u), lerp(c.OrthoHeight, b.Camera.OrthoHeight, u)
	if err := c.Validate(); err != nil {
		return three.Camera{}, fmt.Errorf("scene3d: interpolated camera at %g: %w", time, err)
	}
	return c, nil
}
