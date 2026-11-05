// Package three implements a portable CPU triangle renderer. World coordinates
// are right handed, triangle fronts are counterclockwise, and camera depth is
// positive toward the target. See README.md for numerical and material limits.
package three

import (
	"fmt"
	"gomath/geom"
	"math"
)

const (
	MaxVertices     = 262144
	MaxTriangles    = 524288
	MaxSubdivisions = 256
	// MaxCoordinate bounds mesh, camera and transformed world coordinates.
	MaxCoordinate = 1e12
	// MaxUVCoordinate bounds finite authored texture coordinates before
	// interpolation/addressing. Repeat does not iterate over the number of wraps.
	MaxUVCoordinate = 1e6
)

type Triangle [3]int

// UV uses normalized image coordinates: U increases right, V increases down.
// Texel centers are ((x+.5)/width, (y+.5)/height). Values outside [0,1] are
// addressed by the material sampler. Components must be within +/-1e6.
type UV struct{ U, V float64 }

// Mesh is an indexed triangle list. Normals may be nil for flat shading or have
// one nonzero normal per vertex for smooth shading. UVs may be absent, otherwise
// they must match Vertices exactly. Each index selects position, normal and UV
// together: split vertices at UV seams (including any corresponding normals).
// Textured materials require UVs. Callers own the slices; finish editing them
// before rendering concurrently. Cylinder, Cone and Torus supply split-seam UVs;
// Box, Sphere and ParametricSurface retain their original UV-free topology.
type Mesh struct {
	Vertices  []geom.Vec3
	Normals   []geom.Vec3
	Triangles []Triangle
	UVs       []UV
}

func finite(x float64) bool { return !math.IsNaN(x) && !math.IsInf(x, 0) }
func bounded(v geom.Vec3) bool {
	return v.Finite() && math.Abs(v.X) <= MaxCoordinate && math.Abs(v.Y) <= MaxCoordinate && math.Abs(v.Z) <= MaxCoordinate
}
func unit(v geom.Vec3) (geom.Vec3, bool) {
	n := math.Hypot(math.Hypot(v.X, v.Y), v.Z)
	if math.IsInf(n, 0) && v.Finite() {
		// Finite components may have an unrepresentably large magnitude.
		// Scale first so arbitrary finite authored normals/up vectors work.
		s := math.Max(math.Abs(v.X), math.Max(math.Abs(v.Y), math.Abs(v.Z)))
		return unit(geom.Vec3{X: v.X / s, Y: v.Y / s, Z: v.Z / s})
	}
	if !finite(n) || n == 0 {
		return geom.Vec3{}, false
	}
	// Divide components separately: 1/n can overflow for subnormal normals.
	return geom.Vec3{X: v.X / n, Y: v.Y / n, Z: v.Z / n}, true
}

// Validate rejects empty, nonfinite, oversized and degenerate geometry. It does
// not require a closed/manifold mesh or check normals against winding.
func (m *Mesh) Validate() error {
	if m == nil || len(m.Vertices) < 3 || len(m.Vertices) > MaxVertices || len(m.Triangles) == 0 || len(m.Triangles) > MaxTriangles {
		return fmt.Errorf("three: mesh needs 3..%d vertices and 1..%d triangles", MaxVertices, MaxTriangles)
	}
	if len(m.Normals) != 0 && len(m.Normals) != len(m.Vertices) {
		return fmt.Errorf("three: normals must be absent or match vertex count")
	}
	if len(m.UVs) != 0 && len(m.UVs) != len(m.Vertices) {
		return fmt.Errorf("three: UVs must be absent or match vertex count; split vertices at UV seams")
	}
	for i, uv := range m.UVs {
		if !validUV(uv) {
			return fmt.Errorf("three: UV %d must be finite with components in [-%g,%g]", i, MaxUVCoordinate, MaxUVCoordinate)
		}
	}
	for i, p := range m.Vertices {
		if !bounded(p) {
			return fmt.Errorf("three: vertex %d is nonfinite or outside coordinate bounds", i)
		}
	}
	for i, n := range m.Normals {
		if _, ok := unit(n); !ok {
			return fmt.Errorf("three: normal %d must be finite and nonzero", i)
		}
	}
	for i, t := range m.Triangles {
		for _, index := range t {
			if index < 0 || index >= len(m.Vertices) {
				return fmt.Errorf("three: triangle %d index %d out of range", i, index)
			}
		}
		n := m.Vertices[t[1]].Sub(m.Vertices[t[0]]).Cross(m.Vertices[t[2]].Sub(m.Vertices[t[0]]))
		if _, ok := unit(n); !ok {
			return fmt.Errorf("three: triangle %d is degenerate", i)
		}
	}
	return nil
}

// Box creates an origin-centered box with outward winding and hard face normals.
func Box(width, height, depth float64) (*Mesh, error) {
	for _, v := range []float64{width, height, depth} {
		if !finite(v) || v < 1e-6 || v > MaxCoordinate {
			return nil, fmt.Errorf("three: box dimensions must be in [1e-6, %g]", MaxCoordinate)
		}
	}
	x, y, z := width/2, height/2, depth/2
	faces := [6][4]geom.Vec3{
		{{X: -x, Y: -y, Z: z}, {X: x, Y: -y, Z: z}, {X: x, Y: y, Z: z}, {X: -x, Y: y, Z: z}},
		{{X: x, Y: -y, Z: -z}, {X: -x, Y: -y, Z: -z}, {X: -x, Y: y, Z: -z}, {X: x, Y: y, Z: -z}},
		{{X: x, Y: -y, Z: z}, {X: x, Y: -y, Z: -z}, {X: x, Y: y, Z: -z}, {X: x, Y: y, Z: z}},
		{{X: -x, Y: -y, Z: -z}, {X: -x, Y: -y, Z: z}, {X: -x, Y: y, Z: z}, {X: -x, Y: y, Z: -z}},
		{{X: -x, Y: y, Z: z}, {X: x, Y: y, Z: z}, {X: x, Y: y, Z: -z}, {X: -x, Y: y, Z: -z}},
		{{X: -x, Y: -y, Z: -z}, {X: x, Y: -y, Z: -z}, {X: x, Y: -y, Z: z}, {X: -x, Y: -y, Z: z}},
	}
	m := &Mesh{}
	for _, f := range faces {
		base := len(m.Vertices)
		n, _ := unit(f[1].Sub(f[0]).Cross(f[2].Sub(f[0])))
		m.Vertices = append(m.Vertices, f[:]...)
		for range 4 {
			m.Normals = append(m.Normals, n)
		}
		m.Triangles = append(m.Triangles, Triangle{base, base + 1, base + 2}, Triangle{base, base + 2, base + 3})
	}
	return m, m.Validate()
}

// Sphere creates a UV sphere with unique poles, wrapped longitude indices and
// radial unit normals. latitude is the number of pole-to-pole intervals.
func Sphere(radius float64, longitude, latitude int) (*Mesh, error) {
	if !finite(radius) || radius < 1e-6 || radius > MaxCoordinate || longitude < 3 || longitude > MaxSubdivisions || latitude < 2 || latitude > MaxSubdivisions {
		return nil, fmt.Errorf("three: sphere needs radius [1e-6, %g], longitude [3,%d], latitude [2,%d]", MaxCoordinate, MaxSubdivisions, MaxSubdivisions)
	}
	m := &Mesh{Vertices: []geom.Vec3{{Z: radius}}, Normals: []geom.Vec3{{Z: 1}}}
	for j := 1; j < latitude; j++ {
		theta := math.Pi * float64(j) / float64(latitude)
		for i := 0; i < longitude; i++ {
			phi := 2 * math.Pi * float64(i) / float64(longitude)
			n := geom.Vec3{X: math.Sin(theta) * math.Cos(phi), Y: math.Sin(theta) * math.Sin(phi), Z: math.Cos(theta)}
			m.Normals = append(m.Normals, n)
			m.Vertices = append(m.Vertices, n.Mul(radius))
		}
	}
	bottom := len(m.Vertices)
	m.Vertices = append(m.Vertices, geom.Vec3{Z: -radius})
	m.Normals = append(m.Normals, geom.Vec3{Z: -1})
	for i := 0; i < longitude; i++ {
		next := (i + 1) % longitude
		m.Triangles = append(m.Triangles, Triangle{0, 1 + i, 1 + next})
		for j := 0; j < latitude-2; j++ {
			a, b := 1+j*longitude+i, 1+(j+1)*longitude+i
			c, d := 1+(j+1)*longitude+next, 1+j*longitude+next
			m.Triangles = append(m.Triangles, Triangle{a, b, c}, Triangle{a, c, d})
		}
		last := 1 + (latitude-2)*longitude
		m.Triangles = append(m.Triangles, Triangle{last + i, bottom, last + next})
	}
	return m, m.Validate()
}

// ParametricSurface samples an inclusive rectangular parameter grid. Winding
// and area-weighted normals follow dP/du cross dP/dv. Seams are not welded;
// degenerate cells (including collapsed poles) return an error. The callback
// runs synchronously during construction, never during rendering.
func ParametricSurface(uMin, uMax, vMin, vMax float64, uSteps, vSteps int, sample func(u, v float64) geom.Vec3) (*Mesh, error) {
	if sample == nil || !finite(uMin) || !finite(uMax) || !finite(vMin) || !finite(vMax) || !finite(uMax-uMin) || !finite(vMax-vMin) || uMax <= uMin || vMax <= vMin || uSteps < 1 || vSteps < 1 || uSteps > MaxSubdivisions || vSteps > MaxSubdivisions {
		return nil, fmt.Errorf("three: surface needs increasing finite parameter ranges, a callback and subdivisions [1,%d]", MaxSubdivisions)
	}
	m := &Mesh{}
	for j := 0; j <= vSteps; j++ {
		for i := 0; i <= uSteps; i++ {
			u := uMin + (uMax-uMin)*(float64(i)/float64(uSteps))
			v := vMin + (vMax-vMin)*(float64(j)/float64(vSteps))
			m.Vertices = append(m.Vertices, sample(u, v))
		}
	}
	for j := 0; j < vSteps; j++ {
		for i := 0; i < uSteps; i++ {
			a := j*(uSteps+1) + i
			b := a + 1
			d := a + uSteps + 1
			c := d + 1
			m.Triangles = append(m.Triangles, Triangle{a, b, c}, Triangle{a, c, d})
		}
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	m.Normals = make([]geom.Vec3, len(m.Vertices))
	for _, t := range m.Triangles {
		n := m.Vertices[t[1]].Sub(m.Vertices[t[0]]).Cross(m.Vertices[t[2]].Sub(m.Vertices[t[0]]))
		for _, i := range t {
			m.Normals[i] = m.Normals[i].Add(n)
		}
	}
	for i, n := range m.Normals {
		var ok bool
		m.Normals[i], ok = unit(n)
		if !ok {
			return nil, fmt.Errorf("three: surface normal cancels at vertex %d", i)
		}
	}
	return m, m.Validate()
}
