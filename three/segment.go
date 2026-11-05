package three

import (
	"fmt"
	"gomath/geom"
	"math"
)

// ArrowOptions describes a closed cylinder/cone union, in world units before
// any Instance.Model transform. No defaults or automatic short-arrow scaling
// are applied. HeadRadius must exceed ShaftRadius by at least 1e-6, HeadLength
// must leave at least 1e-6 of shaft. All dimensions are in [1e-6,MaxCoordinate].
// RadialSegments is in [3,256], ShaftSegments/HeadSegments in [1,256].
type ArrowOptions struct {
	ShaftRadius, HeadRadius, HeadLength         float64
	RadialSegments, ShaftSegments, HeadSegments int
}

// CylinderBetween constructs a closed cylinder whose cap centers are start and
// end. Endpoints and all generated positions must be within +/-MaxCoordinate;
// the length must be >=1e-6. Radius is in [1e-6,MaxCoordinate], radialSegments
// in [3,256], heightSegments in [1,256]. Unrepresentable detail returns an error.
//
// Local +Z points start-to-end. Local +X is the projection onto its perpendicular
// plane of the least-aligned world axis (ties X,Y,Z); local +Y is Z cross X.
// This defines deterministic roll, including +/-Z. Side U increases 0..1 about
// +Z, V is 1 at start and 0 at end. Hard caps have independent planar disk UVs.
// Output is caller-owned, with no shared storage or retained input.
func CylinderBetween(start, end geom.Vec3, radius float64, radialSegments, heightSegments int) (*Mesh, error) {
	f, err := newSegmentFrame(start, end)
	if err != nil {
		return nil, err
	}
	if !primitiveDimension(radius) || !segmentCounts(radialSegments, heightSegments) {
		return nil, fmt.Errorf("three: CylinderBetween needs radius [1e-6,%g], radial segments [3,%d], height segments [1,%d]", MaxCoordinate, MaxSubdivisions, MaxSubdivisions)
	}
	if err := f.resolution(radius, radius, radialSegments, f.length/float64(heightSegments)); err != nil {
		return nil, err
	}
	m, err := primitiveStorage((heightSegments+3)*(radialSegments+1), 2*radialSegments*(heightSegments+1))
	if err != nil {
		return nil, err
	}
	b := segmentBuilder{m, f, radialSegments}
	b.side(start, end, radius, heightSegments, false)
	b.cap(start, -1, 0)
	b.cap(end, 1, heightSegments*(radialSegments+1))
	return finishSegment(m)
}

// ArrowBetween constructs one closed arrow with the tail cap centered at start
// and the conical tip at end. The shaft ends at end-direction*HeadLength. Its
// join to the wider head is an outward-facing annular shoulder with normal
// -direction. There are no overlapping/internal cap triangles. One Mesh means
// one material; face, UV and normal seams are split, with outward winding.
//
// Frame/roll and ownership follow CylinderBetween. Shaft and cone sides each
// independently map U around the axis and V from 1 at their base to 0 at their
// end. Tail and shoulder use disk UVs; the shoulder uses HeadRadius as its scale.
// The tip has one limiting normal/UV per sector. Numeric and mesh limits return
// nil/error without clamping dimensions, dropping surfaces or partial output.
func ArrowBetween(start, end geom.Vec3, options ArrowOptions) (*Mesh, error) {
	f, err := newSegmentFrame(start, end)
	if err != nil {
		return nil, err
	}
	o := options
	if !primitiveDimension(o.ShaftRadius) || !primitiveDimension(o.HeadRadius) || !primitiveDimension(o.HeadLength) || o.HeadRadius-o.ShaftRadius < 1e-6 || !segmentCounts(o.RadialSegments, o.ShaftSegments) || o.HeadSegments < 1 || o.HeadSegments > MaxSubdivisions {
		return nil, fmt.Errorf("three: ArrowBetween needs finite dimensions [1e-6,%g], head radius at least shaft radius+1e-6, radial [3,%d], shaft/head segments [1,%d]", MaxCoordinate, MaxSubdivisions, MaxSubdivisions)
	}
	shaftLength := f.length - o.HeadLength
	if shaftLength < 1e-6 {
		return nil, fmt.Errorf("three: ArrowBetween head length must leave shaft length >=1e-6")
	}
	if err := f.resolution(o.HeadRadius/float64(o.HeadSegments), o.HeadRadius, o.RadialSegments, math.Min(shaftLength/float64(o.ShaftSegments), o.HeadLength/float64(o.HeadSegments))); err != nil {
		return nil, err
	}
	if err := f.resolution(o.ShaftRadius, o.HeadRadius, o.RadialSegments, o.HeadRadius-o.ShaftRadius); err != nil {
		return nil, err
	}
	join := end.Sub(f.z.Mul(o.HeadLength))
	m, err := primitiveStorage((o.ShaftSegments+o.HeadSegments+4)*(o.RadialSegments+1)+o.RadialSegments, 2*o.RadialSegments*(o.ShaftSegments+o.HeadSegments+1))
	if err != nil {
		return nil, err
	}
	b := segmentBuilder{m, f, o.RadialSegments}
	b.side(start, join, o.ShaftRadius, o.ShaftSegments, false)
	b.side(join, end, o.HeadRadius, o.HeadSegments, true)
	b.cap(start, -1, 0)
	b.shoulder(o.ShaftRadius, o.HeadRadius, o.ShaftSegments*(o.RadialSegments+1), (o.ShaftSegments+1)*(o.RadialSegments+1))
	return finishSegment(m)
}

func segmentCounts(radial, axial int) bool {
	return radial >= 3 && radial <= MaxSubdivisions && axial >= 1 && axial <= MaxSubdivisions
}

type segmentFrame struct {
	x, y, z           geom.Vec3
	length, magnitude float64
}

func newSegmentFrame(start, end geom.Vec3) (segmentFrame, error) {
	if !bounded(start) || !bounded(end) {
		return segmentFrame{}, fmt.Errorf("three: segment endpoints must be finite within +/- %g", MaxCoordinate)
	}
	d := end.Sub(start)
	length := math.Hypot(math.Hypot(d.X, d.Y), d.Z)
	if length < 1e-6 {
		return segmentFrame{}, fmt.Errorf("three: segment length must be >=1e-6")
	}
	z, _ := unit(d)
	seed := geom.Vec3{X: 1}
	if math.Abs(z.Y) < math.Abs(z.X) && math.Abs(z.Y) <= math.Abs(z.Z) {
		seed = geom.Vec3{Y: 1}
	} else if math.Abs(z.Z) < math.Abs(z.X) && math.Abs(z.Z) < math.Abs(z.Y) {
		seed = geom.Vec3{Z: 1}
	}
	x, _ := unit(seed.Sub(z.Mul(seed.Dot(z))))
	magnitude := math.Max(math.Max(math.Abs(start.X), math.Abs(end.X)), math.Max(math.Max(math.Abs(start.Y), math.Abs(end.Y)), math.Max(math.Abs(start.Z), math.Abs(end.Z))))
	return segmentFrame{x, z.Cross(x), z, length, magnitude}, nil
}

// Reject details below 32 ULPs at the endpoint-plus-radius scale, before mesh
// allocation. Sagitta (not just edge length) must survive rounding so neighboring
// radial panels cannot fold into the same plane. This is a conservative policy,
// not a promise to accept every mathematically representable translated solid.
func (f segmentFrame) resolution(radius, maxRadius float64, radial int, axialDetail float64) error {
	magnitude := f.magnitude + maxRadius
	ulp := math.Nextafter(magnitude, math.Inf(1)) - magnitude
	s := math.Sin(math.Pi / float64(radial))
	if 2*radius*s*s < 32*ulp || axialDetail < 32*ulp {
		return fmt.Errorf("three: segment detail is unrepresentable at endpoint/radius scale (requires >=32 ULPs)")
	}
	return nil
}

type segmentBuilder struct {
	mesh   *Mesh
	frame  segmentFrame
	radial int
}

func segmentLerp(a, b geom.Vec3, t float64) geom.Vec3 {
	if t <= .5 {
		return a.Add(b.Sub(a).Mul(t))
	}
	return b.Sub(b.Sub(a).Mul(1 - t))
}

func (b segmentBuilder) radialVector(s, c float64) geom.Vec3 {
	return b.frame.x.Mul(c).Add(b.frame.y.Mul(s))
}

func (b segmentBuilder) side(start, end geom.Vec3, radius float64, axial int, cone bool) {
	rings := axial + 1
	if cone {
		rings = axial
	}
	base := len(b.mesh.Vertices)
	height := end.Sub(start).Dot(b.frame.z)
	slope := math.Hypot(height, radius)
	for j := 0; j < rings; j++ {
		v := float64(j) / float64(axial)
		r := radius
		if cone {
			r *= float64(axial-j) / float64(axial)
		}
		center := segmentLerp(start, end, v)
		for i := 0; i <= b.radial; i++ {
			s, c := primitiveCircle(i, b.radial)
			n := b.radialVector(s, c)
			p := center.Add(n.Mul(r))
			if cone {
				n = n.Mul(height / slope).Add(b.frame.z.Mul(radius / slope))
			}
			b.mesh.primitiveVertex(p, n, UV{U: float64(i) / float64(b.radial), V: 1 - v})
		}
	}
	for j := 0; j < rings-1; j++ {
		for i := 0; i < b.radial; i++ {
			a := base + j*(b.radial+1) + i
			d := a + b.radial + 1
			b.mesh.Triangles = append(b.mesh.Triangles, Triangle{a, a + 1, d + 1}, Triangle{a, d + 1, d})
		}
	}
	if cone {
		for i := 0; i < b.radial; i++ {
			s, c := math.Sincos(2 * math.Pi * (float64(i) + .5) / float64(b.radial))
			n := b.radialVector(s, c).Mul(height / slope).Add(b.frame.z.Mul(radius / slope))
			tip := len(b.mesh.Vertices)
			b.mesh.primitiveVertex(end, n, UV{U: (float64(i) + .5) / float64(b.radial)})
			a := base + (rings-1)*(b.radial+1) + i
			b.mesh.Triangles = append(b.mesh.Triangles, Triangle{a, a + 1, tip})
		}
	}
}

func (b segmentBuilder) cap(center geom.Vec3, sign float64, sideRing int) {
	base := len(b.mesh.Vertices)
	n := b.frame.z.Mul(sign)
	b.mesh.primitiveVertex(center, n, UV{U: .5, V: .5})
	for i := 0; i < b.radial; i++ {
		s, c := primitiveCircle(i, b.radial)
		// Copy the already emitted position: independent equivalent arithmetic
		// can differ by an ULP when compilers fuse multiply/add expressions.
		b.mesh.primitiveVertex(b.mesh.Vertices[sideRing+i], n, UV{U: .5 + .5*c, V: .5 - sign*.5*s})
	}
	for i := 0; i < b.radial; i++ {
		a, c := base+1+i, base+1+(i+1)%b.radial
		if sign < 0 {
			a, c = c, a
		}
		b.mesh.Triangles = append(b.mesh.Triangles, Triangle{base, a, c})
	}
}

func (b segmentBuilder) shoulder(inner, outer float64, innerRing, outerRing int) {
	base := len(b.mesh.Vertices)
	for ring, r := range []float64{inner, outer} {
		source := innerRing
		if ring == 1 {
			source = outerRing
		}
		for i := 0; i <= b.radial; i++ {
			s, c := primitiveCircle(i, b.radial)
			b.mesh.primitiveVertex(b.mesh.Vertices[source+i], b.frame.z.Mul(-1), UV{U: .5 + .5*(r/outer)*c, V: .5 + .5*(r/outer)*s})
		}
	}
	for i := 0; i < b.radial; i++ {
		a := base + i
		d := a + b.radial + 1
		b.mesh.Triangles = append(b.mesh.Triangles, Triangle{a, a + 1, d + 1}, Triangle{a, d + 1, d})
	}
}

func finishSegment(m *Mesh) (*Mesh, error) {
	if err := m.Validate(); err != nil {
		return nil, fmt.Errorf("three: segment geometry: %w", err)
	}
	return m, nil
}
