package path

import (
	"fmt"
	"math/big"
	"sort"
)

type booleanEdge struct {
	a, b  int
	delta booleanPoint
}

func booleanLexSign(constant, infinitesimal int) int {
	if constant != 0 {
		return constant
	}
	return infinitesimal
}
func booleanWinding(a, b, orientation int) int {
	if a <= 0 && b > 0 && orientation > 0 {
		return 1
	}
	if a > 0 && b <= 0 && orientation < 0 {
		return -1
	}
	return 0
}

// Winding at midpoint +/- epsilon * left normal. Epsilon is symbolic and
// positive: signs compare exact constant terms first, then first derivatives.
// All crossings have been split, so an atomic edge's open interior cannot meet
// a non-collinear source edge. No floating sample distance or snap is involved.
func (e *booleanEngine) sides(a, b int) ([2]int, [2]int) {
	return e.sidesOf(e.points[a], e.points[b], e.points, e.segments)
}

func (e *booleanEngine) sidesOf(p, q booleanPoint, points []booleanPoint, segments []booleanSegment) ([2]int, [2]int) {
	half := big.NewRat(1, 2)
	m := booleanPoint{e.mul(e.add(p.x, q.x), half), e.mul(e.add(p.y, q.y), half)}
	d := e.vec(q, p)
	n := booleanPoint{e.sub(new(big.Rat), d.y), d.x}
	var left, right [2]int
	for _, s := range segments {
		ay, by := e.cmp(points[s.a].y, m.y), e.cmp(points[s.b].y, m.y)
		al, bl := booleanLexSign(ay, -n.y.Sign()), booleanLexSign(by, -n.y.Sign())
		ar, br := booleanLexSign(ay, n.y.Sign()), booleanLexSign(by, n.y.Sign())
		if (al <= 0 && bl > 0) || (al > 0 && bl <= 0) || (ar <= 0 && br > 0) || (ar > 0 && br <= 0) {
			orientation := e.cross(s.delta, e.vec(m, points[s.a])).Sign()
			ol, or := orientation, orientation
			if orientation == 0 {
				derivative := e.cross(s.delta, n).Sign()
				ol, or = derivative, -derivative
			}
			left[s.operand] += booleanWinding(al, bl, ol)
			right[s.operand] += booleanWinding(ar, br, or)
		}
		if e.err != nil {
			return left, right
		}
	}
	return left, right
}

func (e *booleanEngine) boundary(op BooleanOp, ra, rb FillRule) ([]booleanEdge, error) {
	atoms := make(map[[2]int]bool)
	for _, s := range e.segments {
		cuts := e.sortedCuts(s)
		if e.err != nil {
			return nil, e.err
		}
		for i := 1; i < len(cuts); i++ {
			a, b := cuts[i-1], cuts[i]
			if a > b {
				a, b = b, a
			}
			key := [2]int{a, b}
			if !atoms[key] {
				if len(atoms) >= e.limits.Edges {
					return nil, fmt.Errorf("path boolean: atomic edges exceed %d", e.limits.Edges)
				}
				if !e.charge(1, 192) {
					return nil, e.err
				}
				atoms[key] = true
			}
		}
	}
	if !e.charge(1, len(atoms)*64) {
		return nil, e.err
	}
	ordered := make([][2]int, 0, len(atoms))
	for pair := range atoms {
		if e.keys[pair[0]] > e.keys[pair[1]] {
			pair[0], pair[1] = pair[1], pair[0]
		}
		ordered = append(ordered, pair)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if !e.charge(1, 0) {
			return false
		}
		a, b := ordered[i], ordered[j]
		if e.keys[a[0]] != e.keys[b[0]] {
			return e.keys[a[0]] < e.keys[b[0]]
		}
		return e.keys[a[1]] < e.keys[b[1]]
	})
	var selected []booleanEdge
	for _, pair := range ordered {
		a, b := pair[0], pair[1]
		l, r := e.sides(a, b)
		if e.err != nil {
			return nil, e.err
		}
		left := booleanCombine(booleanInside(l[0], ra), booleanInside(l[1], rb), op)
		right := booleanCombine(booleanInside(r[0], ra), booleanInside(r[1], rb), op)
		if left == right {
			continue
		}
		if !left {
			a, b = b, a
		}
		if !e.charge(1, 128) {
			return nil, e.err
		}
		selected = append(selected, booleanEdge{a: a, b: b, delta: e.vec(e.points[b], e.points[a])})
	}
	return selected, e.err
}

func booleanHalf(d booleanPoint) int {
	if d.y.Sign() > 0 || d.y.Sign() == 0 && d.x.Sign() >= 0 {
		return 0
	}
	return 1
}
func (e *booleanEngine) directionCompare(a, b booleanPoint) int {
	if !e.charge(1, 0) {
		return 0
	}
	ha, hb := booleanHalf(a), booleanHalf(b)
	if ha != hb {
		if ha < hb {
			return -1
		}
		return 1
	}
	return -e.cross(a, b).Sign()
}

func (e *booleanEngine) successors(edges []booleanEdge) ([]int, error) {
	if !e.charge(1, len(edges)*256) {
		return nil, e.err
	}
	outgoing := make(map[int][]int)
	incoming := make(map[int]int)
	for i, s := range edges {
		outgoing[s.a] = append(outgoing[s.a], i)
		incoming[s.b]++
	}
	for vertex, list := range outgoing {
		if incoming[vertex] != len(list) {
			return nil, fmt.Errorf("path boolean: inconsistent boundary degree")
		}
		sort.Slice(list, func(i, j int) bool { return e.directionCompare(edges[list[i]].delta, edges[list[j]].delta) < 0 })
		for i := 1; i < len(list); i++ {
			if e.directionCompare(edges[list[i-1]].delta, edges[list[i]].delta) == 0 {
				return nil, fmt.Errorf("path boolean: overlapping boundary rays")
			}
		}
	}
	next := make([]int, len(edges))
	used := make([]bool, len(edges))
	for i, s := range edges {
		list := outgoing[s.b]
		if len(list) == 0 {
			return nil, fmt.Errorf("path boolean: open output boundary")
		}
		inverse := booleanPoint{e.sub(new(big.Rat), s.delta.x), e.sub(new(big.Rat), s.delta.y)}
		at := sort.Search(len(list), func(j int) bool { return e.directionCompare(edges[list[j]].delta, inverse) >= 0 })
		if at == 0 {
			at = len(list)
		}
		j := list[at-1]
		if used[j] {
			return nil, fmt.Errorf("path boolean: inconsistent boundary pairing")
		}
		used[j] = true
		next[i] = j
		if e.err != nil {
			return nil, e.err
		}
	}
	return next, e.err
}

func (e *booleanEngine) output(edges []booleanEdge) (Path, error) {
	next, err := e.successors(edges)
	if err != nil {
		return Path{}, err
	}
	if !e.charge(1, len(edges)*192) {
		return Path{}, e.err
	}
	seen := make([]bool, len(edges))
	var result Path
	for i := range edges {
		if seen[i] {
			continue
		}
		first := edges[i].a
		start := e.floatPoint(e.points[first])
		contour := Contour{Start: start, Closed: true}
		at := i
		for {
			if !e.charge(1, 0) {
				return Path{}, e.err
			}
			if seen[at] {
				if at != i {
					return Path{}, fmt.Errorf("path boolean: boundary walk merges cycles")
				}
				break
			}
			seen[at] = true
			s := edges[at]
			a, b := e.floatPoint(e.points[s.a]), e.floatPoint(e.points[s.b])
			line := Line(a, b)
			if !line.P1.Finite() || !line.P2.Finite() {
				return Path{}, fmt.Errorf("path boolean: output line controls exceed float64 range")
			}
			contour.Curves = append(contour.Curves, line)
			at = next[at]
		}
		if len(contour.Curves) < 3 || edges[at].a != first {
			return Path{}, fmt.Errorf("path boolean: degenerate output boundary")
		}
		result.Contours = append(result.Contours, contour)
	}
	return result, e.err
}
