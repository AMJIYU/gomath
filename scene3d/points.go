package scene3d

import (
	"fmt"
	"gomath/geom"
	"gomath/three"
	"image/color"
	"math"
	"slices"
	"sort"
)

// MaxPointBufferBytes bounds all owned point snapshots, including every key.
// Each point consumes 36 logical bytes: position 24, NRGBA 4, pixel radius 8.
const MaxPointBufferBytes = 64 * 1024 * 1024

const pointBytes = 36

// PointKey holds a complete local-space point buffer. All keys of a track must
// have the same positive point count; index i corresponds throughout the track.
// Positions/radii interpolate linearly, colors in premultiplied sRGB bytes.
// Exact keys retain their original NRGBA values, even alpha-zero hidden RGB.
type PointKey struct {
	Time  float64
	Cloud *three.PointCloud
}

// PointTrack uses exactly one of Cloud or nonempty PointKeys. NewWithPoints
// snapshots all buffer contents; even caller replacement of *Cloud is isolated.
// Positions are local coordinates transformed by T*R*S, using the same pose
// and visibility policy as MeshTrack. Radii are output pixels independent of
// model scale, camera depth and SSAA. Points are unlit disks, with no shadows.
// IDs share one positive unique namespace with MeshTrack IDs.
// Optional RotationKeys replace fixed-axis rotation and require zero Axis and
// zero PoseKey angles, using the same orientation semantics as MeshTrack.
type PointTrack struct {
	ID           uint64
	Cloud        *three.PointCloud
	PointKeys    []PointKey
	RotationKeys []RotationKey
	Axis         geom.Vec3
	Keys         []PoseKey
	Visible      []Visibility
}

type pointBuffer struct {
	positions []geom.Vec3
	colors    []color.NRGBA
	radii     []float64
}

type pointBufferKey struct {
	time float64
	pointBuffer
}

type pointTrack struct {
	id        uint64
	axis      geom.Vec3
	keys      []PoseKey
	visible   []Visibility
	buffers   []pointBufferKey
	rotations []RotationKey
}

// budget checks lengths before any point snapshot allocations. bytes includes
// repeated input clouds separately: Scene owns each authored buffer snapshot.
func (t PointTrack) budget(duration float64, remainingBytes int) (count, bytes int, err error) {
	if err = validateRotations(t.ID, t.Axis, t.Keys, t.RotationKeys, duration); err != nil {
		return 0, 0, err
	}
	if (t.Cloud != nil) == (len(t.PointKeys) != 0) {
		return 0, 0, fmt.Errorf("scene3d: point track %d needs exactly one of Cloud or PointKeys", t.ID)
	}
	if _, err = rotationAxis(t.Axis); err != nil {
		return 0, 0, fmt.Errorf("scene3d: point track %d: %w", t.ID, err)
	}
	if err = validateMotion(t.ID, t.Keys, t.Visible, duration); err != nil {
		return 0, 0, err
	}
	check := func(cloud *three.PointCloud) error {
		n := cloud.Len()
		if n < 1 || n > three.MaxPoints || count != 0 && count != n {
			return fmt.Errorf("scene3d: point track %d requires valid buffers with the same count in [1,%d]", t.ID, three.MaxPoints)
		}
		count = n
		if n > (remainingBytes-bytes)/pointBytes {
			return fmt.Errorf("scene3d: point buffer budget exceeds %d bytes", MaxPointBufferBytes)
		}
		bytes += n * pointBytes
		return nil
	}
	if t.Cloud != nil {
		err = check(t.Cloud)
	} else {
		for i, key := range t.PointKeys {
			if !validTime(key.Time, duration) || i > 0 && key.Time <= t.PointKeys[i-1].Time {
				return 0, 0, fmt.Errorf("scene3d: point track %d buffer key %d needs strictly increasing times in [0,%g]", t.ID, i, duration)
			}
			if err = check(key.Cloud); err != nil {
				break
			}
		}
	}
	return
}

func snapshotPointTrack(t PointTrack) (pointTrack, error) {
	axis, _ := rotationAxis(t.Axis) // validated before allocation
	p := pointTrack{id: t.ID, axis: axis, keys: slices.Clone(t.Keys), visible: slices.Clone(t.Visible)}
	p.rotations = snapshotRotations(t.RotationKeys)
	keys := t.PointKeys
	if t.Cloud != nil {
		keys = []PointKey{{Cloud: t.Cloud}}
	}
	p.buffers = make([]pointBufferKey, len(keys))
	for i, key := range keys {
		n := key.Cloud.Len()
		b := pointBufferKey{time: key.Time, pointBuffer: pointBuffer{make([]geom.Vec3, n), make([]color.NRGBA, n), make([]float64, n)}}
		for j := range n {
			var err error
			b.positions[j], b.colors[j], b.radii[j], err = key.Cloud.Point(j)
			if err != nil {
				return pointTrack{}, fmt.Errorf("scene3d: point track %d buffer %d: %w", t.ID, i, err)
			}
		}
		p.buffers[i] = b
	}
	return p, nil
}

// Interpolate premultiplied color before unassociation and byte quantization.
// This prevents invisible endpoint RGB from bleeding into intermediate colors.
func interpolatePointColor(a, b color.NRGBA, u float64) color.NRGBA {
	alpha := lerp(float64(a.A), float64(b.A), u)
	if alpha == 0 {
		return color.NRGBA{}
	}
	channel := func(x, y uint8) uint8 {
		v := lerp(float64(x)*float64(a.A), float64(y)*float64(b.A), u) / alpha
		return uint8(math.Round(math.Max(0, math.Min(255, v))))
	}
	return color.NRGBA{R: channel(a.R, b.R), G: channel(a.G, b.G), B: channel(a.B, b.B), A: uint8(math.Round(alpha))}
}

func (t pointTrack) cloudAt(time float64, pose Pose) (*three.PointCloud, error) {
	i := sort.Search(len(t.buffers), func(i int) bool { return t.buffers[i].time > time })
	a, b, u := t.buffers[0], t.buffers[0], 0.0
	if i > 0 {
		a, b = t.buffers[i-1], t.buffers[i-1]
		if i < len(t.buffers) && time != a.time {
			b = t.buffers[i]
			u = (time - a.time) / (b.time - a.time)
		}
	}
	n := len(a.positions)
	positions, colors, radii := make([]geom.Vec3, n), make([]color.NRGBA, n), make([]float64, n)
	model, err := trackModel(pose, t.axis, t.rotations, time)
	if err != nil {
		return nil, fmt.Errorf("scene3d: point track %d at %g: %w", t.id, time, err)
	}
	for j := range n {
		position, c, radius := a.positions[j], a.colors[j], a.radii[j]
		if u != 0 {
			position = position.Lerp(b.positions[j], u)
			c = interpolatePointColor(c, b.colors[j], u)
			radius = lerp(radius, b.radii[j], u)
		}
		positions[j], colors[j], radii[j] = model.Apply(position), c, radius
	}
	// The renderer's immutable-cloud constructor validates transformed bounds,
	// even alpha-zero/clipped points, then copies the world buffers. No state
	// or scratch is retained across samples. Both copies are explicitly bounded.
	cloud, err := three.NewPointCloud(positions, colors, radii)
	if err != nil {
		return nil, fmt.Errorf("scene3d: point track %d at %g: %w", t.id, time, err)
	}
	return cloud, nil
}
