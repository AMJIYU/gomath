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

// MaxMeshBufferBytes bounds owned vector/UV/index payload across static meshes
// and every mesh key. Repeated pointers are snapshotted and charged separately.
// Logical sizes are position/normal 24, UV 16 and triangle 24 bytes.
const MaxMeshBufferBytes = 64 * 1024 * 1024

// MeshKey is a full local-space mesh snapshot. Counts, attribute presence and
// exact triangle indices/order must match all keys. Vertices and UVs interpolate
// linearly. Authored normal components interpolate linearly then normalize;
// exact endpoints retain original values. There is no automatic normal rebuild,
// topology matching, seam unwrapping or triangle/winding repair.
type MeshKey struct {
	Time float64
	Mesh *three.Mesh
}

// MaterialKey overrides only Material.Color. Texture and Sampler remain fixed.
// Keys hold endpoints and interpolate premultiplied sRGB bytes/alpha, then
// unassociate and round to NRGBA, using the same policy as PointKey colors.
type MaterialKey struct {
	Time  float64
	Color color.NRGBA
}

func (t MeshTrack) meshSource() (*three.Mesh, error) {
	if (t.Mesh != nil) == (len(t.MeshKeys) != 0) {
		return nil, fmt.Errorf("scene3d: track %d needs exactly one of Mesh or MeshKeys", t.ID)
	}
	if t.Mesh != nil {
		return t.Mesh, nil
	}
	if t.MeshKeys[0].Mesh == nil {
		return nil, fmt.Errorf("scene3d: track %d has nil mesh key", t.ID)
	}
	return t.MeshKeys[0].Mesh, nil
}

func sameTriangles(a, b []three.Triangle) bool { return slices.Equal(a, b) }

func meshBytes(m *three.Mesh) (int, error) {
	if m == nil || len(m.Vertices) > three.MaxVertices || len(m.Normals) > three.MaxVertices || len(m.UVs) > three.MaxVertices || len(m.Triangles) > three.MaxTriangles {
		return 0, fmt.Errorf("scene3d: nil or oversized mesh snapshot")
	}
	return 24*(len(m.Vertices)+len(m.Normals)+len(m.Triangles)) + 16*len(m.UVs), nil
}

func (t MeshTrack) meshBudget(remaining int) (int, error) {
	if t.Mesh != nil {
		n, err := meshBytes(t.Mesh)
		if err != nil {
			return 0, err
		}
		if n > remaining {
			return 0, fmt.Errorf("scene3d: mesh snapshot budget exceeds %d bytes", MaxMeshBufferBytes)
		}
		return n, nil
	}
	total := 0
	for _, key := range t.MeshKeys {
		n, err := meshBytes(key.Mesh)
		if err != nil {
			return 0, err
		}
		if n > remaining-total {
			return 0, fmt.Errorf("scene3d: mesh snapshot budget exceeds %d bytes", MaxMeshBufferBytes)
		}
		total += n
	}
	return total, nil
}

func cloneMesh(m *three.Mesh) *three.Mesh {
	return &three.Mesh{Vertices: slices.Clone(m.Vertices), Normals: slices.Clone(m.Normals), UVs: slices.Clone(m.UVs), Triangles: slices.Clone(m.Triangles)}
}

func snapshotMeshTrack(t MeshTrack) MeshTrack {
	t.RotationKeys = snapshotRotations(t.RotationKeys)
	if t.Mesh != nil {
		t.Mesh = cloneMesh(t.Mesh)
	}
	t.MeshKeys = slices.Clone(t.MeshKeys)
	for i := range t.MeshKeys {
		t.MeshKeys[i].Mesh = cloneMesh(t.MeshKeys[i].Mesh)
	}
	t.Keys, t.Visible, t.MaterialKeys = slices.Clone(t.Keys), slices.Clone(t.Visible), slices.Clone(t.MaterialKeys)
	t.Axis, _ = rotationAxis(t.Axis) // validated before allocation
	return t
}

func normalizedNormal(v geom.Vec3) (geom.Vec3, bool) {
	if !v.Finite() {
		return geom.Vec3{}, false
	}
	scale := math.Max(math.Abs(v.X), math.Max(math.Abs(v.Y), math.Abs(v.Z)))
	if scale == 0 {
		return geom.Vec3{}, false
	}
	v = geom.Vec3{X: v.X / scale, Y: v.Y / scale, Z: v.Z / scale}
	n := math.Sqrt(v.Dot(v))
	return geom.Vec3{X: v.X / n, Y: v.Y / n, Z: v.Z / n}, true
}

func (t MeshTrack) meshAt(time float64) (*three.Mesh, error) {
	if len(t.MeshKeys) == 0 {
		return t.Mesh, nil // original static geometry, with no per-sample copy
	}
	i := sort.Search(len(t.MeshKeys), func(i int) bool { return t.MeshKeys[i].Time > time })
	if i == 0 {
		return t.MeshKeys[0].Mesh, nil
	}
	a := t.MeshKeys[i-1]
	if i == len(t.MeshKeys) || time == a.Time {
		return a.Mesh, nil
	}
	b := t.MeshKeys[i]
	u := (time - a.Time) / (b.Time - a.Time)
	m := &three.Mesh{Vertices: make([]geom.Vec3, len(a.Mesh.Vertices)), Triangles: a.Mesh.Triangles}
	for i, p := range a.Mesh.Vertices {
		m.Vertices[i] = p.Lerp(b.Mesh.Vertices[i], u)
	}
	if len(a.Mesh.Normals) != 0 {
		m.Normals = make([]geom.Vec3, len(a.Mesh.Normals))
		for i, n := range a.Mesh.Normals {
			var ok bool
			m.Normals[i], ok = normalizedNormal(n.Lerp(b.Mesh.Normals[i], u))
			if !ok {
				return nil, fmt.Errorf("scene3d: track %d at %g interpolated normal %d is zero or nonfinite", t.ID, time, i)
			}
		}
	}
	if len(a.Mesh.UVs) != 0 {
		m.UVs = make([]three.UV, len(a.Mesh.UVs))
		for i, uv := range a.Mesh.UVs {
			m.UVs[i] = three.UV{U: lerp(uv.U, b.Mesh.UVs[i].U, u), V: lerp(uv.V, b.Mesh.UVs[i].V, u)}
		}
	}
	if err := m.Validate(); err != nil {
		return nil, fmt.Errorf("scene3d: track %d at %g interpolated mesh: %w", t.ID, time, err)
	}
	return m, nil
}

func (t MeshTrack) materialAt(time float64) three.Material {
	m := t.Material
	if len(t.MaterialKeys) == 0 {
		return m
	}
	i := sort.Search(len(t.MaterialKeys), func(i int) bool { return t.MaterialKeys[i].Time > time })
	if i == 0 {
		m.Color = t.MaterialKeys[0].Color
	} else {
		a := t.MaterialKeys[i-1]
		m.Color = a.Color
		if i < len(t.MaterialKeys) && time != a.Time {
			b := t.MaterialKeys[i]
			m.Color = interpolatePointColor(a.Color, b.Color, (time-a.Time)/(b.Time-a.Time))
		}
	}
	return m
}
