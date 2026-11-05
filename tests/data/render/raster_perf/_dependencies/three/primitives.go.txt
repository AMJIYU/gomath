package three

import (
	"fmt"
	"gomath/geom"
	"math"
)

// Cylinder creates a closed cylinder along Z, centered at the origin. Radius
// is measured in XY; the caps are at Z = +/-height/2. Side U goes from +X
// around +Z counterclockwise, V from 1 at the bottom to 0 at the top. Caps
// have separate vertices and planar disk UVs. All triangles wind outward.
// radialSegments is in [3,256]; heightSegments in [1,256].
func Cylinder(radius, height float64, radialSegments, heightSegments int) (*Mesh, error) {
	return axialPrimitive(radius, height, radialSegments, heightSegments, false)
}

// Cone creates a closed cone along Z with base radius in XY at -height/2
// and tip at +height/2. Side UVs follow Cylinder; each tip sector gets its
// own vertex with midpoint U and limiting side normal at that angle. The
// base has separate hard normals and planar disk UVs. Winding is outward.
// radialSegments is in [3,256]; heightSegments in [1,256].
func Cone(radius, height float64, radialSegments, heightSegments int) (*Mesh, error) {
	return axialPrimitive(radius, height, radialSegments, heightSegments, true)
}

func primitiveDimension(x float64) bool {
	return finite(x) && x >= 1e-6 && x <= MaxCoordinate
}

// Counts are checked before allocation. Callers bound subdivision arguments
// before calculating these counts, so even an authored MaxInt cannot overflow.
func primitiveStorage(vertices, triangles int) (*Mesh, error) {
	if vertices < 3 || vertices > MaxVertices || triangles < 1 || triangles > MaxTriangles {
		return nil, fmt.Errorf("three: primitive budget exceeded: %d vertices, %d triangles (limits %d, %d)", vertices, triangles, MaxVertices, MaxTriangles)
	}
	return &Mesh{
		Vertices:  make([]geom.Vec3, 0, vertices),
		Normals:   make([]geom.Vec3, 0, vertices),
		UVs:       make([]UV, 0, vertices),
		Triangles: make([]Triangle, 0, triangles),
	}, nil
}

func (m *Mesh) primitiveVertex(p, n geom.Vec3, uv UV) {
	m.Vertices = append(m.Vertices, p)
	m.Normals = append(m.Normals, n)
	m.UVs = append(m.UVs, uv)
}

// The seam repeats exactly, including normals: sin(2*pi) rounding must not
// leave a microscopic crack in position-welded topology or alpha coverage.
func primitiveCircle(i, segments int) (sin, cos float64) {
	if i == segments {
		return 0, 1
	}
	return math.Sincos(2 * math.Pi * float64(i) / float64(segments))
}

func axialPrimitive(radius, height float64, radial, vertical int, cone bool) (*Mesh, error) {
	if !primitiveDimension(radius) || !primitiveDimension(height) || radial < 3 || radial > MaxSubdivisions || vertical < 1 || vertical > MaxSubdivisions {
		return nil, fmt.Errorf("three: cylinder/cone needs radius and height in [1e-6,%g], radial segments [3,%d], height segments [1,%d]", MaxCoordinate, MaxSubdivisions, MaxSubdivisions)
	}
	rings, caps := vertical+1, 2
	vertices := rings*(radial+1) + caps*(radial+1)
	triangles := 2 * radial * (vertical + 1)
	if cone {
		rings, caps = vertical, 1
		vertices = rings*(radial+1) + radial + caps*(radial+1)
		triangles = 2 * radial * vertical
	}
	m, err := primitiveStorage(vertices, triangles)
	if err != nil {
		return nil, err
	}
	slopeLength := math.Hypot(height, radius)
	for j := 0; j < rings; j++ {
		v := float64(j) / float64(vertical)
		r := radius
		if cone {
			// Subtract integer indices first to retain the last small ring.
			r *= float64(vertical-j) / float64(vertical)
		}
		z := height * (v - .5)
		for i := 0; i <= radial; i++ {
			s, c := primitiveCircle(i, radial)
			n := geom.Vec3{X: c, Y: s}
			if cone {
				n = geom.Vec3{X: c * (height / slopeLength), Y: s * (height / slopeLength), Z: radius / slopeLength}
			}
			m.primitiveVertex(geom.Vec3{X: r * c, Y: r * s, Z: z}, n, UV{U: float64(i) / float64(radial), V: 1 - v})
		}
	}
	for j := 0; j < rings-1; j++ {
		for i := 0; i < radial; i++ {
			a := j*(radial+1) + i
			d := a + radial + 1
			m.Triangles = append(m.Triangles, Triangle{a, a + 1, d + 1}, Triangle{a, d + 1, d})
		}
	}
	if cone {
		for i := 0; i < radial; i++ {
			theta := 2 * math.Pi * (float64(i) + .5) / float64(radial)
			s, c := math.Sincos(theta)
			tip := len(m.Vertices)
			m.primitiveVertex(geom.Vec3{Z: height / 2}, geom.Vec3{X: c * (height / slopeLength), Y: s * (height / slopeLength), Z: radius / slopeLength}, UV{U: (float64(i) + .5) / float64(radial)})
			a := (rings-1)*(radial+1) + i
			m.Triangles = append(m.Triangles, Triangle{a, a + 1, tip})
		}
	}
	for capIndex := 0; capIndex < caps; capIndex++ {
		sign := -1.0
		if capIndex == 1 {
			sign = 1
		}
		center := len(m.Vertices)
		n := geom.Vec3{Z: sign}
		m.primitiveVertex(geom.Vec3{Z: sign * height / 2}, n, UV{U: .5, V: .5})
		for i := 0; i < radial; i++ {
			s, c := primitiveCircle(i, radial)
			m.primitiveVertex(geom.Vec3{X: radius * c, Y: radius * s, Z: sign * height / 2}, n, UV{U: .5 + .5*c, V: .5 - sign*.5*s})
		}
		for i := 0; i < radial; i++ {
			a, b := center+1+i, center+1+(i+1)%radial
			if sign < 0 {
				a, b = b, a
			}
			m.Triangles = append(m.Triangles, Triangle{center, a, b})
		}
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return m, nil
}

// Torus creates a ring torus about Z. majorRadius is the origin-to-tube-center
// distance in XY; minorRadius is the tube radius, with majorRadius > minorRadius.
// U is the major angle from +X around +Z; V is the tube angle from the outward
// XY equator toward +Z. Both increase 0..1 over one turn, with split seams.
// Analytic tube normals and counterclockwise outward triangles are supplied.
// Both segment counts are in [3,256]; majorRadius+minorRadius must be <=1e12.
func Torus(majorRadius, minorRadius float64, majorSegments, minorSegments int) (*Mesh, error) {
	if !primitiveDimension(majorRadius) || !primitiveDimension(minorRadius) || majorRadius <= minorRadius || majorRadius+minorRadius > MaxCoordinate || majorSegments < 3 || majorSegments > MaxSubdivisions || minorSegments < 3 || minorSegments > MaxSubdivisions {
		return nil, fmt.Errorf("three: torus needs 1e-6 <= minor radius < major radius, sum <= %g and both segment counts in [3,%d]", MaxCoordinate, MaxSubdivisions)
	}
	// Distinct tube offsets must remain representable at the major radius.
	if majorRadius+minorRadius == majorRadius || majorRadius-minorRadius == majorRadius {
		return nil, fmt.Errorf("three: torus tube radius is unrepresentable at the major radius")
	}
	previousRadius := majorRadius + minorRadius
	for j := 1; j <= minorSegments; j++ {
		_, c := primitiveCircle(j, minorSegments)
		radius := majorRadius + minorRadius*c
		// For odd counts the two rings straddling phi=pi intentionally have
		// identical radii and opposite Z. Every other radial step must survive
		// rounding; otherwise tube arcs can fold into overlapping axial strips
		// even though the individual triangles still have nonzero area.
		if 2*j-1 != minorSegments && radius == previousRadius {
			return nil, fmt.Errorf("three: torus tube radial step %d is unrepresentable at the major radius", j)
		}
		previousRadius = radius
	}
	m, err := primitiveStorage((majorSegments+1)*(minorSegments+1), 2*majorSegments*minorSegments)
	if err != nil {
		return nil, err
	}
	for j := 0; j <= minorSegments; j++ {
		sv, cv := primitiveCircle(j, minorSegments)
		for i := 0; i <= majorSegments; i++ {
			su, cu := primitiveCircle(i, majorSegments)
			r := majorRadius + minorRadius*cv
			n := geom.Vec3{X: cu * cv, Y: su * cv, Z: sv}
			m.primitiveVertex(geom.Vec3{X: r * cu, Y: r * su, Z: minorRadius * sv}, n, UV{U: float64(i) / float64(majorSegments), V: float64(j) / float64(minorSegments)})
		}
	}
	for j := 0; j < minorSegments; j++ {
		for i := 0; i < majorSegments; i++ {
			a := j*(majorSegments+1) + i
			d := a + majorSegments + 1
			m.Triangles = append(m.Triangles, Triangle{a, a + 1, d + 1}, Triangle{a, d + 1, d})
		}
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return m, nil
}
