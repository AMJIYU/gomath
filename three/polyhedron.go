package three

import (
	"encoding/binary"
	"fmt"
	"gomath/geom"
	"math"
	"slices"
)

const (
	MaxPolyhedronVertices     = 1024
	MaxPolyhedronFaces        = 2048
	MaxPolyhedronFaceVertices = 256
	MaxPolyhedronCorners      = 8192
	// PolyhedronTolerance is a distance relative to the largest bounding-box
	// extent. It bounds distinctness, planarity, convexity and interior margin.
	PolyhedronTolerance = 1e-10
	MinPolyhedronSpan   = 1e-9
)

// Polyhedron constructs a closed convex mesh from unique positions and polygon
// boundary cycles. Every face must be planar, strictly convex and wound outward
// (counterclockwise from outside). Every input vertex must be used; every edge
// must occur twice in opposite directions. Open, disconnected, nonmanifold,
// duplicate, concave, self-intersecting or inward surfaces return nil/error.
// No hull, polygon repair, orientation repair or general tessellation occurs.
//
// Positions must be finite within +/-MaxCoordinate. The largest bounding-box
// extent must be >=MinPolyhedronSpan. Geometry indistinguishable at the relative
// PolyhedronTolerance is rejected. Counts are bounded before geometry work.
// Each face is fan-triangulated only after validation, with duplicated boundary
// vertices and constant outward unit normals. Face-local planar UVs map its
// projected bounding rectangle to [0,1]^2: U follows its first edge, V decreases
// along normal cross U. Faces have independent seams, not a global UV atlas.
//
// Output slices own their storage; no input is retained or mutated. Like other
// Mesh constructors, the returned Mesh is caller-owned and mutable. Finish
// authoring before construction/rendering concurrently.
func Polyhedron(vertices []geom.Vec3, faces [][]int) (*Mesh, error) {
	n := len(vertices)
	if n < 4 || n > MaxPolyhedronVertices || len(faces) < 4 || len(faces) > MaxPolyhedronFaces {
		return nil, fmt.Errorf("three: polyhedron needs 4..%d vertices and 4..%d faces", MaxPolyhedronVertices, MaxPolyhedronFaces)
	}
	corners, triangles := 0, 0
	for i, face := range faces {
		if len(face) < 3 || len(face) > MaxPolyhedronFaceVertices || len(face) > MaxPolyhedronCorners-corners {
			return nil, fmt.Errorf("three: polyhedron face %d needs 3..%d corners; total at most %d", i, MaxPolyhedronFaceVertices, MaxPolyhedronCorners)
		}
		corners += len(face)
		triangles += len(face) - 2
	}
	lo, hi := vertices[0], vertices[0]
	for i, p := range vertices {
		if !bounded(p) {
			return nil, fmt.Errorf("three: polyhedron vertex %d must be finite within +/- %g", i, MaxCoordinate)
		}
		lo.X, lo.Y, lo.Z = math.Min(lo.X, p.X), math.Min(lo.Y, p.Y), math.Min(lo.Z, p.Z)
		hi.X, hi.Y, hi.Z = math.Max(hi.X, p.X), math.Max(hi.Y, p.Y), math.Max(hi.Z, p.Z)
	}
	span := math.Max(hi.X-lo.X, math.Max(hi.Y-lo.Y, hi.Z-lo.Z))
	if span < MinPolyhedronSpan {
		return nil, fmt.Errorf("three: polyhedron span must be >= %g", MinPolyhedronSpan)
	}
	origin := lo.Mul(.5).Add(hi.Mul(.5))
	q := make([]geom.Vec3, n)
	center := geom.Vec3{}
	for i, p := range vertices {
		q[i] = p.Sub(origin).Mul(1 / span)
		center = center.Add(q[i].Mul(1 / float64(n)))
		for j := 0; j < i; j++ {
			if q[i].Sub(q[j]).Norm() <= PolyhedronTolerance {
				return nil, fmt.Errorf("three: polyhedron vertices %d and %d duplicate or too close", j, i)
			}
		}
	}
	type edgeKey struct{ a, b int }
	type edgeUse struct{ from, to, face, count int }
	edges := make(map[edgeKey]edgeUse, corners/2)
	faceKeys := make(map[string]bool, len(faces))
	adj := make([][]int, len(faces))
	links := make([][][2]int, n)
	normals := make([]geom.Vec3, len(faces))
	seen := make([]int, n)
	used := make([]bool, n)
	for fi, face := range faces {
		for _, i := range face {
			if i < 0 || i >= n {
				return nil, fmt.Errorf("three: polyhedron face %d index %d outside vertex count", fi, i)
			}
			if seen[i] == fi+1 {
				return nil, fmt.Errorf("three: polyhedron face %d repeats vertex %d", fi, i)
			}
			seen[i], used[i] = fi+1, true
		}
		sorted := slices.Clone(face)
		slices.Sort(sorted)
		key := make([]byte, 2*len(face))
		for j, i := range sorted {
			binary.LittleEndian.PutUint16(key[j*2:], uint16(i))
		}
		if faceKeys[string(key)] {
			return nil, fmt.Errorf("three: polyhedron duplicate face %d", fi)
		}
		faceKeys[string(key)] = true
		p := q[face[0]]
		area := geom.Vec3{}
		for j := 1; j+1 < len(face); j++ {
			area = area.Add(q[face[j]].Sub(p).Cross(q[face[j+1]].Sub(p)))
		}
		normal, ok := unit(area)
		if !ok || area.Norm() <= PolyhedronTolerance*PolyhedronTolerance {
			return nil, fmt.Errorf("three: polyhedron face %d has degenerate area", fi)
		}
		normals[fi] = normal
		for _, i := range face {
			if math.Abs(normal.Dot(q[i].Sub(p))) > PolyhedronTolerance {
				return nil, fmt.Errorf("three: polyhedron face %d is nonplanar", fi)
			}
		}
		if normal.Dot(p.Sub(center)) <= PolyhedronTolerance {
			return nil, fmt.Errorf("three: polyhedron face %d is inward or has insufficient solid interior", fi)
		}
		for j, a := range face {
			b := face[(j+1)%len(face)]
			e := q[b].Sub(q[a])
			length := e.Norm()
			if length <= PolyhedronTolerance {
				return nil, fmt.Errorf("three: polyhedron face %d has a degenerate edge", fi)
			}
			for _, k := range face {
				if k != a && k != b && normal.Dot(e.Cross(q[k].Sub(q[a])))/length <= PolyhedronTolerance {
					return nil, fmt.Errorf("three: polyhedron face %d must be strictly convex with a simple boundary", fi)
				}
			}
			key := edgeKey{min(a, b), max(a, b)}
			old := edges[key]
			if old.count == 0 {
				edges[key] = edgeUse{a, b, fi, 1}
			} else {
				if old.count != 1 {
					return nil, fmt.Errorf("three: polyhedron edge %d,%d is nonmanifold", a, b)
				}
				if old.from != b || old.to != a {
					return nil, fmt.Errorf("three: polyhedron edge %d,%d has inconsistent winding", a, b)
				}
				adj[fi] = append(adj[fi], old.face)
				adj[old.face] = append(adj[old.face], fi)
				old.count = 2
				edges[key] = old
			}
			links[a] = append(links[a], [2]int{face[(j+len(face)-1)%len(face)], b})
		}
		for i, point := range q {
			if normal.Dot(point.Sub(p)) > PolyhedronTolerance {
				return nil, fmt.Errorf("three: polyhedron is nonconvex: vertex %d outside face %d", i, fi)
			}
		}
	}
	for i, ok := range used {
		if !ok {
			return nil, fmt.Errorf("three: polyhedron unused vertex %d", i)
		}
	}
	for key, e := range edges {
		if e.count != 2 {
			return nil, fmt.Errorf("three: polyhedron is open at edge %d,%d", key.a, key.b)
		}
	}
	if n-len(edges)+len(faces) != 2 {
		return nil, fmt.Errorf("three: polyhedron Euler characteristic must be 2")
	}
	if !polyhedronConnected(adj) {
		return nil, fmt.Errorf("three: polyhedron faces must form one connected surface")
	}
	// An edge-manifold can still pinch at a vertex. Every vertex link must be
	// one cycle, rather than two disjoint loops sharing the same position.
	for vertex, link := range links {
		neighbors := make(map[int][]int, len(link))
		for _, pair := range link {
			neighbors[pair[0]] = append(neighbors[pair[0]], pair[1])
			neighbors[pair[1]] = append(neighbors[pair[1]], pair[0])
		}
		if len(neighbors) < 3 {
			return nil, fmt.Errorf("three: polyhedron vertex %d needs at least three neighbors", vertex)
		}
		start := link[0][0]
		visited := map[int]bool{start: true}
		queue := []int{start}
		for i := 0; i < len(queue); i++ {
			v := queue[i]
			if len(neighbors[v]) != 2 {
				return nil, fmt.Errorf("three: polyhedron vertex %d link is nonmanifold", vertex)
			}
			for _, next := range neighbors[v] {
				if !visited[next] {
					visited[next] = true
					queue = append(queue, next)
				}
			}
		}
		if len(visited) != len(neighbors) {
			return nil, fmt.Errorf("three: polyhedron vertex %d link is disconnected", vertex)
		}
	}
	m, err := primitiveStorage(corners, triangles)
	if err != nil {
		return nil, err
	}
	for fi, face := range faces {
		p := q[face[0]]
		u, _ := unit(q[face[1]].Sub(p))
		v := normals[fi].Cross(u)
		minU, maxU, minV, maxV := math.Inf(1), math.Inf(-1), math.Inf(1), math.Inf(-1)
		for _, i := range face {
			a, b := q[i].Sub(p).Dot(u), q[i].Sub(p).Dot(v)
			minU, maxU = math.Min(minU, a), math.Max(maxU, a)
			minV, maxV = math.Min(minV, b), math.Max(maxV, b)
		}
		if maxU-minU <= PolyhedronTolerance || maxV-minV <= PolyhedronTolerance {
			return nil, fmt.Errorf("three: polyhedron face %d UV span is degenerate", fi)
		}
		base := len(m.Vertices)
		for _, i := range face {
			a, b := q[i].Sub(p).Dot(u), q[i].Sub(p).Dot(v)
			uv := UV{U: math.Max(0, math.Min(1, (a-minU)/(maxU-minU))), V: math.Max(0, math.Min(1, 1-(b-minV)/(maxV-minV)))}
			m.primitiveVertex(vertices[i], normals[fi], uv)
		}
		for j := 1; j+1 < len(face); j++ {
			m.Triangles = append(m.Triangles, Triangle{base, base + j, base + j + 1})
		}
	}
	if err = m.Validate(); err != nil {
		return nil, fmt.Errorf("three: polyhedron triangulation: %w", err)
	}
	return m, nil
}

func polyhedronConnected(adj [][]int) bool {
	visited := make([]bool, len(adj))
	visited[0] = true
	queue := []int{0}
	for i := 0; i < len(queue); i++ {
		for _, next := range adj[queue[i]] {
			if !visited[next] {
				visited[next] = true
				queue = append(queue, next)
			}
		}
	}
	return len(queue) == len(adj)
}
