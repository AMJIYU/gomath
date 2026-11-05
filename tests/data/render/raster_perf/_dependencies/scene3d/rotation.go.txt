package scene3d

import (
	"fmt"
	"gomath/geom"
	"slices"
	"sort"
)

// RotationKey is an absolute-time local orientation. Nonzero finite quaternions
// normalize at construction; interiors use shortest-arc SLERP. With these keys,
// Axis must be zero and every PoseKey.Angle must be zero. Translation and signed
// scale still come from PoseKeys; the model is T*R(quaternion)*S.
type RotationKey struct {
	Time     float64
	Rotation geom.Quat
}

func validateRotations(id uint64, axis geom.Vec3, poses []PoseKey, keys []RotationKey, duration float64) error {
	if len(keys) == 0 {
		return nil
	}
	if axis != (geom.Vec3{}) {
		return fmt.Errorf("scene3d: track %d RotationKeys require zero Axis", id)
	}
	for _, p := range poses {
		if p.Pose.Angle != 0 {
			return fmt.Errorf("scene3d: track %d RotationKeys require zero Pose.Angle", id)
		}
	}
	for i, key := range keys {
		if !validTime(key.Time, duration) || i > 0 && key.Time <= keys[i-1].Time {
			return fmt.Errorf("scene3d: track %d rotation key %d needs strictly increasing times in [0,%g]", id, i, duration)
		}
		if _, err := key.Rotation.Normalize(); err != nil {
			return fmt.Errorf("scene3d: track %d rotation key %d: %w", id, i, err)
		}
	}
	return nil
}

func snapshotRotations(keys []RotationKey) []RotationKey {
	keys = slices.Clone(keys)
	for i := range keys {
		keys[i].Rotation, _ = keys[i].Rotation.Normalize()
	} // validated
	return keys
}

func rotationAt(keys []RotationKey, time float64) (geom.Quat, error) {
	i := sort.Search(len(keys), func(i int) bool { return keys[i].Time > time })
	if i == 0 {
		return keys[0].Rotation, nil
	}
	a := keys[i-1]
	if i == len(keys) || time == a.Time {
		return a.Rotation, nil
	}
	b := keys[i]
	return a.Rotation.Slerp(b.Rotation, (time-a.Time)/(b.Time-a.Time))
}

func trackModel(pose Pose, axis geom.Vec3, keys []RotationKey, time float64) (geom.Mat4, error) {
	if len(keys) == 0 {
		return pose.matrix(axis), nil
	} // retain original arithmetic/pixels
	q, err := rotationAt(keys, time)
	if err != nil {
		return geom.Mat4{}, err
	}
	r, err := q.Matrix()
	if err != nil {
		return geom.Mat4{}, err
	}
	return geom.Translation(pose.Position).Mul(r).Mul(geom.Scaling(pose.Scale.X, pose.Scale.Y, pose.Scale.Z)), nil
}

// RotationAt returns the effective local orientation of a mesh or point track,
// including hidden tracks. PoseAt continues to return the authored TRS values;
// its Angle is zero in quaternion mode. Legacy angle/full-turn tracks retain
// their original sampling; this query cannot expose their winding as a Quat.
func (s *Scene) RotationAt(id uint64, time float64) (geom.Quat, error) {
	pose, _, err := s.PoseAt(id, time)
	if err != nil {
		return geom.Quat{}, err
	}
	i := s.index[id]
	var axis geom.Vec3
	var keys []RotationKey
	if i < len(s.tracks) {
		axis, keys = s.tracks[i].Axis, s.tracks[i].RotationKeys
	} else {
		t := s.points[i-len(s.tracks)]
		axis, keys = t.axis, t.rotations
	}
	if len(keys) != 0 {
		return rotationAt(keys, time)
	}
	return geom.QuatFromAxisAngle(axis, pose.Angle)
}
