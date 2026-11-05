package three

import (
	"gomath/geom"
	"math"
)

// Independent parameter-domain reference. It emits polygonal cylinder/cone
// triangles from closed-form positions, not from the production mesh or builder.
// The basis is obtained by two cross products, rather than projected seed math.
// The exact finite radial polygon is tested, not an infinite-resolution cone.
func segmentReference(start, end geom.Vec3, radius float64, radial, axial int, arrow *ArrowOptions) *Mesh {
	d := end.Sub(start)
	length := math.Sqrt(d.Dot(d))
	z := d.Mul(1 / length)
	seeds := []geom.Vec3{{X: 1}, {Y: 1}, {Z: 1}}
	seed := seeds[0]
	for _, candidate := range seeds[1:] {
		if math.Abs(z.Dot(candidate)) < math.Abs(z.Dot(seed)) {
			seed = candidate
		}
	}
	normalize := func(v geom.Vec3) geom.Vec3 { return v.Mul(1 / math.Sqrt(v.Dot(v))) }
	y := normalize(z.Cross(seed))
	x := y.Cross(z)
	m := &Mesh{}
	type vertex struct {
		p, n geom.Vec3
		uv   UV
	}
	lookup := make(map[vertex]int)
	emit := func(a, b, c vertex) {
		var tri Triangle
		for j, v := range []vertex{a, b, c} {
			i, ok := lookup[v]
			if !ok {
				i = len(m.Vertices)
				lookup[v] = i
				m.Vertices = append(m.Vertices, v.p)
				m.Normals = append(m.Normals, v.n)
				m.UVs = append(m.UVs, v.uv)
			}
			tri[j] = i
		}
		m.Triangles = append(m.Triangles, tri)
	}
	point := func(local geom.Vec3) geom.Vec3 {
		return start.Add(x.Mul(local.X)).Add(y.Mul(local.Y)).Add(z.Mul(local.Z))
	}
	vector := func(local geom.Vec3) geom.Vec3 { return x.Mul(local.X).Add(y.Mul(local.Y)).Add(z.Mul(local.Z)) }
	circle := func(i int) (float64, float64) {
		angle := 2 * math.Pi * float64(i%radial) / float64(radial)
		return math.Cos(angle), math.Sin(angle)
	}
	side := func(base, height, r float64, steps int, cone bool) {
		corner := func(i, j int) vertex {
			c, s := circle(i)
			v := float64(j) / float64(steps)
			rr := r
			n := geom.Vec3{X: c, Y: s}
			if cone {
				rr = r * (1 - v)
				slope := math.Sqrt(height*height + r*r)
				n = geom.Vec3{X: c * height / slope, Y: s * height / slope, Z: r / slope}
			}
			return vertex{point(geom.Vec3{X: rr * c, Y: rr * s, Z: base + height*v}), vector(n), UV{U: float64(i) / float64(radial), V: 1 - v}}
		}
		for j := 0; j < steps; j++ {
			for i := 0; i < radial; i++ {
				a, b := corner(i, j), corner(i+1, j)
				if cone && j == steps-1 {
					phi := 2 * math.Pi * (float64(i) + .5) / float64(radial)
					slope := math.Sqrt(height*height + r*r)
					tip := vertex{end, vector(geom.Vec3{X: math.Cos(phi) * height / slope, Y: math.Sin(phi) * height / slope, Z: r / slope}), UV{U: (float64(i) + .5) / float64(radial)}}
					emit(a, b, tip)
				} else {
					c, d := corner(i+1, j+1), corner(i, j+1)
					emit(a, b, c)
					emit(a, c, d)
				}
			}
		}
	}
	cap := func(height, r, sign float64) {
		n := z.Mul(sign)
		center := vertex{point(geom.Vec3{Z: height}), n, UV{U: .5, V: .5}}
		corner := func(i int) vertex {
			c, s := circle(i)
			return vertex{point(geom.Vec3{X: r * c, Y: r * s, Z: height}), n, UV{U: .5 + .5*c, V: .5 - sign*.5*s}}
		}
		for i := 0; i < radial; i++ {
			a, b := corner(i), corner(i+1)
			if sign < 0 {
				a, b = b, a
			}
			emit(center, a, b)
		}
	}
	if arrow == nil {
		side(0, length, radius, axial, false)
		cap(0, radius, -1)
		cap(length, radius, 1)
		return m
	}
	o := *arrow
	shaft := length - o.HeadLength
	side(0, shaft, o.ShaftRadius, o.ShaftSegments, false)
	side(shaft, o.HeadLength, o.HeadRadius, o.HeadSegments, true)
	cap(0, o.ShaftRadius, -1)
	shoulder := func(i int, r float64) vertex {
		c, s := circle(i)
		return vertex{point(geom.Vec3{X: r * c, Y: r * s, Z: shaft}), z.Mul(-1), UV{U: .5 + .5*r/o.HeadRadius*c, V: .5 + .5*r/o.HeadRadius*s}}
	}
	for i := 0; i < radial; i++ {
		a, b, c, d := shoulder(i, o.ShaftRadius), shoulder(i+1, o.ShaftRadius), shoulder(i+1, o.HeadRadius), shoulder(i, o.HeadRadius)
		emit(a, b, c)
		emit(a, c, d)
	}
	return m
}

// Independent Rodrigues/normal transform before the ray oracle; no production
// model/inverse/normal-matrix helpers participate in the expected geometry.
func segmentReferencePose(m *Mesh, pose polyhedronPose) *Mesh {
	copyMesh := &Mesh{Triangles: append([]Triangle(nil), m.Triangles...), UVs: append([]UV(nil), m.UVs...)}
	for _, p := range m.Vertices {
		p = geom.Vec3{X: p.X * pose.scale.X, Y: p.Y * pose.scale.Y, Z: p.Z * pose.scale.Z}
		copyMesh.Vertices = append(copyMesh.Vertices, polyhedronRotate(p, pose.axis, pose.angle).Add(pose.position))
	}
	for _, n := range m.Normals {
		n = geom.Vec3{X: n.X / pose.scale.X, Y: n.Y / pose.scale.Y, Z: n.Z / pose.scale.Z}
		n = polyhedronRotate(n, pose.axis, pose.angle)
		copyMesh.Normals = append(copyMesh.Normals, n.Mul(1/math.Sqrt(n.Dot(n))))
	}
	if pose.scale.X*pose.scale.Y*pose.scale.Z < 0 {
		for i := range copyMesh.Triangles {
			copyMesh.Triangles[i][1], copyMesh.Triangles[i][2] = copyMesh.Triangles[i][2], copyMesh.Triangles[i][1]
		}
	}
	return copyMesh
}
