package path

import (
	"fmt"
	"gomath/geom"
	"sort"
)

func (e *booleanEngine) floatPoint(p booleanPoint) geom.Vec3 {
	x, _ := p.x.Float64()
	y, _ := p.y.Float64()
	return geom.V(x, y)
}

func (e *booleanEngine) verifyRounding(edges []booleanEdge) error {
	rounded := make(map[int]booleanPoint)
	positions := make(map[geom.Vec3]int)
	incident := make(map[int][]int)
	if !e.charge(1, len(edges)*256) {
		return e.err
	}
	for _, s := range edges {
		for _, id := range []int{s.a, s.b} {
			if _, ok := rounded[id]; ok {
				continue
			}
			v := e.floatPoint(e.points[id])
			if !v.Finite() {
				return fmt.Errorf("path boolean: output coordinate cannot be represented as finite float64")
			}
			if other, ok := positions[v]; ok && other != id {
				return fmt.Errorf("path boolean: float64 rounding merges distinct output vertices")
			}
			positions[v] = id
			rounded[id] = e.fromFloat(v)
		}
		incident[s.a] = append(incident[s.a], s.b)
		incident[s.b] = append(incident[s.b], s.a)
	}
	// Preserve cyclic ray order, rather than every pair's orientation sign.
	// Opposite collinear rays may bend slightly when their rational intersection
	// rounds, without changing the local embedding. Same-direction rays and a
	// permutation of three or more incident rays would change topology.
	for v, neighbors := range incident {
		type ray struct {
			id int
			d  booleanPoint
		}
		if !e.charge(1, len(neighbors)*128) {
			return e.err
		}
		before, after := make([]ray, 0, len(neighbors)), make([]ray, 0, len(neighbors))
		for _, id := range neighbors {
			before = append(before, ray{id, e.vec(e.points[id], e.points[v])})
			after = append(after, ray{id, e.vec(rounded[id], rounded[v])})
		}
		for _, rays := range [][]ray{before, after} {
			sort.Slice(rays, func(i, j int) bool {
				if !e.pair() {
					return false
				}
				return e.directionCompare(rays[i].d, rays[j].d) < 0
			})
			if e.err != nil {
				return e.err
			}
			for i := 1; i < len(rays); i++ {
				if e.directionCompare(rays[i-1].d, rays[i].d) == 0 {
					return fmt.Errorf("path boolean: float64 rounding creates overlapping boundary rays")
				}
			}
		}
		if e.err != nil {
			return e.err
		}
		start := 0
		for after[start].id != before[0].id {
			start++
		}
		for i := range before {
			if before[i].id != after[(start+i)%len(after)].id {
				return fmt.Errorf("path boolean: float64 rounding changes boundary ray order")
			}
		}
	}
	// After rounding, different atomic boundaries may meet only at their existing
	// shared vertex. New proper crossings, T contacts and overlaps are errors.
	for i, a := range edges {
		for _, b := range edges[i+1:] {
			if !e.pair() {
				return e.err
			}
			va, vb, vc, vd := e.floatPoint(e.points[a.a]), e.floatPoint(e.points[a.b]), e.floatPoint(e.points[b.a]), e.floatPoint(e.points[b.b])
			if max(va.X, vb.X) < min(vc.X, vd.X) || max(vc.X, vd.X) < min(va.X, vb.X) || max(va.Y, vb.Y) < min(vc.Y, vd.Y) || max(vc.Y, vd.Y) < min(va.Y, vb.Y) {
				continue
			}
			p, q, r, s := rounded[a.a], rounded[a.b], rounded[b.a], rounded[b.b]
			x, y, u, v := e.orient(p, q, r).Sign(), e.orient(p, q, s).Sign(), e.orient(r, s, p).Sign(), e.orient(r, s, q).Sign()
			if x*y < 0 && u*v < 0 {
				return fmt.Errorf("path boolean: float64 rounding creates a boundary crossing")
			}
			for _, check := range []struct {
				sign, id, a, b int
				p, aP, bP      booleanPoint
			}{
				{x, b.a, a.a, a.b, r, p, q}, {y, b.b, a.a, a.b, s, p, q}, {u, a.a, b.a, b.b, p, r, s}, {v, a.b, b.a, b.b, q, r, s},
			} {
				if check.sign == 0 && check.id != check.a && check.id != check.b && e.onSegment(check.p, check.aP, check.bP) {
					return fmt.Errorf("path boolean: float64 rounding creates a boundary overlap or contact")
				}
			}
			if e.err != nil {
				return e.err
			}
		}
	}
	// Rotation order alone cannot distinguish an isolated thin loop from its
	// reversed image, or a hole that rounded across a disjoint boundary. Verify
	// the canonical region winding on both sides of every rounded edge too.
	if !e.charge(1, len(e.points)*32+len(edges)*128) {
		return e.err
	}
	points := make([]booleanPoint, len(e.points))
	for id, p := range rounded {
		points[id] = p
	}
	segments := make([]booleanSegment, 0, len(edges))
	for _, s := range edges {
		segments = append(segments, booleanSegment{a: s.a, b: s.b, delta: e.vec(points[s.b], points[s.a])})
	}
	for _, s := range segments {
		l, r := e.sidesOf(points[s.a], points[s.b], points, segments)
		if e.err != nil {
			return e.err
		}
		if l[0] != 1 || r[0] != 0 {
			return fmt.Errorf("path boolean: float64 rounding changes region winding or orientation")
		}
	}
	return e.err
}
