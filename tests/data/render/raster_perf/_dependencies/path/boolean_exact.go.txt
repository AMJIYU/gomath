package path

import (
	"fmt"
	"gomath/geom"
	"math/big"
	"sort"
)

type booleanPoint struct{ x, y *big.Rat }
type booleanSegment struct {
	a, b                   int
	delta                  booleanPoint
	operand                int
	cuts                   map[int]bool
	minX, maxX, minY, maxY float64 // source endpoints only: safe ordered comparisons
}
type booleanEngine struct {
	limits                                        BooleanLimits
	work, allocated, pairs, contours, splitPoints int
	err                                           error
	points                                        []booleanPoint
	keys                                          []string
	ids                                           map[string]int
	segments                                      []booleanSegment
}

func newBooleanEngine(l BooleanLimits) *booleanEngine {
	return &booleanEngine{limits: l, ids: make(map[string]int)}
}
func (e *booleanEngine) charge(work, bytes int) bool {
	if e.err != nil {
		return false
	}
	if work > e.limits.Work-e.work {
		e.err = fmt.Errorf("path boolean: work budget exceeds %d", e.limits.Work)
		return false
	}
	if bytes > e.limits.AllocationBytes-e.allocated {
		e.err = fmt.Errorf("path boolean: allocation charge exceeds %d bytes", e.limits.AllocationBytes)
		return false
	}
	e.work += work
	e.allocated += bytes
	return true
}
func (e *booleanEngine) check(r *big.Rat) *big.Rat {
	if r.Num().BitLen() > e.limits.RationalBits || r.Denom().BitLen() > e.limits.RationalBits {
		e.err = fmt.Errorf("path boolean: rational bits exceed %d", e.limits.RationalBits)
	}
	return r
}

// Charge generous limb capacity and arithmetic scratch per operation, including
// comparisons. Charges are cumulative even when the actual temporary is reused
// or collected. Operand/result bit limits independently bound exact arithmetic.
func (e *booleanEngine) reserve(a, b *big.Rat) bool {
	bits := a.Num().BitLen() + a.Denom().BitLen()
	if b != nil {
		bits += b.Num().BitLen() + b.Denom().BitLen()
	}
	return e.charge(1, 256+64*((bits+63)/64))
}
func (e *booleanEngine) add(a, b *big.Rat) *big.Rat {
	if !e.reserve(a, b) {
		return new(big.Rat)
	}
	return e.check(new(big.Rat).Add(a, b))
}
func (e *booleanEngine) sub(a, b *big.Rat) *big.Rat {
	if !e.reserve(a, b) {
		return new(big.Rat)
	}
	return e.check(new(big.Rat).Sub(a, b))
}
func (e *booleanEngine) mul(a, b *big.Rat) *big.Rat {
	if !e.reserve(a, b) {
		return new(big.Rat)
	}
	return e.check(new(big.Rat).Mul(a, b))
}
func (e *booleanEngine) div(a, b *big.Rat) *big.Rat {
	if b.Sign() == 0 {
		if e.err == nil {
			e.err = fmt.Errorf("path boolean: zero exact denominator")
		}
		return new(big.Rat)
	}
	if !e.reserve(a, b) {
		return new(big.Rat)
	}
	return e.check(new(big.Rat).Quo(a, b))
}
func (e *booleanEngine) cmp(a, b *big.Rat) int {
	if !e.reserve(a, b) {
		return 0
	}
	return a.Cmp(b)
}
func (e *booleanEngine) vec(a, b booleanPoint) booleanPoint {
	return booleanPoint{e.sub(a.x, b.x), e.sub(a.y, b.y)}
}
func (e *booleanEngine) cross(a, b booleanPoint) *big.Rat {
	return e.sub(e.mul(a.x, b.y), e.mul(a.y, b.x))
}
func (e *booleanEngine) orient(a, b, c booleanPoint) *big.Rat {
	return e.cross(e.vec(b, a), e.vec(c, a))
}
func (e *booleanEngine) fromFloat(v geom.Vec3) booleanPoint {
	if !e.charge(1, 512) {
		return booleanPoint{new(big.Rat), new(big.Rat)}
	}
	return booleanPoint{e.check(new(big.Rat).SetFloat64(v.X)), e.check(new(big.Rat).SetFloat64(v.Y))}
}
func (e *booleanEngine) point(p booleanPoint) int {
	if e.err != nil {
		return 0
	}
	bytes := 128 + (p.x.Num().BitLen()+p.x.Denom().BitLen()+p.y.Num().BitLen()+p.y.Denom().BitLen())/2
	if !e.charge(1, bytes) {
		return 0
	}
	key := p.x.RatString() + "," + p.y.RatString()
	if id, ok := e.ids[key]; ok {
		return id
	}
	if len(e.points) >= e.limits.Vertices {
		e.err = fmt.Errorf("path boolean: vertices exceed %d", e.limits.Vertices)
		return 0
	}
	id := len(e.points)
	e.points = append(e.points, p)
	e.keys = append(e.keys, key)
	e.ids[key] = id
	return id
}
func (e *booleanEngine) cut(s *booleanSegment, id int) {
	if e.err != nil || s.cuts[id] {
		return
	}
	if e.splitPoints >= e.limits.SplitPoints {
		e.err = fmt.Errorf("path boolean: split points exceed %d", e.limits.SplitPoints)
		return
	}
	if !e.charge(1, 96) {
		return
	}
	s.cuts[id] = true
	e.splitPoints++
}
func (e *booleanEngine) input(p Path, operand int) error {
	if len(p.Contours) > e.limits.Contours-e.contours {
		return fmt.Errorf("path boolean: contours exceed %d", e.limits.Contours)
	}
	e.contours += len(p.Contours)
	// Reject oversized source slices before walking controls or allocating rationals.
	count := len(e.segments)
	for _, c := range p.Contours {
		if len(c.Curves) > e.limits.Segments-count {
			return fmt.Errorf("path boolean: segments exceed %d", e.limits.Segments)
		}
		count += len(c.Curves)
	}
	for i, c := range p.Contours {
		if !c.Closed || len(c.Curves) == 0 {
			return fmt.Errorf("path boolean: contour %d must be closed and contain nonzero segments", i)
		}
		if !booleanXY(c.Start) {
			return fmt.Errorf("path boolean: contour %d must have finite XY coordinates with Z=0", i)
		}

		prev := c.Start
		firstDirection := booleanPoint{}
		collinear := true
		add := func(v geom.Vec3) error {
			if len(e.segments) >= e.limits.Segments {
				return fmt.Errorf("path boolean: segments exceed %d", e.limits.Segments)
			}
			if v == prev {
				return fmt.Errorf("path boolean: zero-length edge in contour %d", i)
			}
			a, b := e.point(e.fromFloat(prev)), e.point(e.fromFloat(v))
			if e.err != nil {
				return e.err
			}
			d := e.vec(e.points[b], e.points[a])
			if firstDirection.x == nil {
				firstDirection = d
			} else if e.cross(firstDirection, d).Sign() != 0 {
				collinear = false
			}
			if !e.charge(1, 256) {
				return e.err
			}
			s := booleanSegment{a: a, b: b, delta: d, operand: operand, cuts: make(map[int]bool), minX: min(prev.X, v.X), maxX: max(prev.X, v.X), minY: min(prev.Y, v.Y), maxY: max(prev.Y, v.Y)}
			e.cut(&s, a)
			e.cut(&s, b)
			e.segments = append(e.segments, s)
			prev = v
			return e.err
		}
		for j, v := range c.Curves {
			if !booleanXY(v.P0) || !booleanXY(v.P1) || !booleanXY(v.P2) || !booleanXY(v.P3) {
				return fmt.Errorf("path boolean: contour %d curve %d must be finite XY with Z=0", i, j)
			}
			if v.P0 != prev {
				return fmt.Errorf("path boolean: contour %d curve %d is not exactly connected", i, j)
			}
			if v.P0 == v.P3 {
				return fmt.Errorf("path boolean: zero-length or looping cubic in contour %d curve %d", i, j)
			}
			reverse := Line(v.P3, v.P0)
			if v != Line(v.P0, v.P3) && !(v.P1 == reverse.P2 && v.P2 == reverse.P1) {
				a, b := e.fromFloat(v.P0), e.fromFloat(v.P3)
				for _, control := range []geom.Vec3{v.P1, v.P2} {
					q := e.fromFloat(control)
					if e.orient(a, b, q).Sign() != 0 || !e.onSegment(q, a, b) {
						return fmt.Errorf("path boolean: nonlinear cubic at contour %d curve %d; explicitly approximate curves before Boolean", i, j)
					}
				}
			}
			if e.err != nil {
				return e.err
			}
			if err := add(v.P3); err != nil {
				return err
			}
		}
		if prev != c.Start {
			if err := add(c.Start); err != nil {
				return err
			}
		}
		if e.err != nil {
			return e.err
		}
		if collinear {
			return fmt.Errorf("path boolean: contour %d is entirely collinear (degenerate area)", i)
		}
	}
	return e.err
}
func (e *booleanEngine) onSegment(p, a, b booleanPoint) bool {
	if e.cmp(a.x, b.x) != 0 {
		return e.between(p.x, a.x, b.x)
	}
	return e.between(p.y, a.y, b.y)
}
func (e *booleanEngine) between(v, a, b *big.Rat) bool {
	if e.cmp(a, b) > 0 {
		a, b = b, a
	}
	return e.cmp(v, a) >= 0 && e.cmp(v, b) <= 0
}
func (e *booleanEngine) pair() bool {
	if e.pairs >= e.limits.Pairs {
		e.err = fmt.Errorf("path boolean: pair budget exceeds %d", e.limits.Pairs)
		return false
	}
	e.pairs++
	return e.charge(1, 0)
}
func (e *booleanEngine) split() error {
	for i := range e.segments {
		for j := i + 1; j < len(e.segments); j++ {
			if !e.pair() {
				return e.err
			}
			a, b := &e.segments[i], &e.segments[j]
			if a.maxX < b.minX || b.maxX < a.minX || a.maxY < b.minY || b.maxY < a.minY {
				continue
			}
			p, q := e.points[a.a], e.points[b.a]
			offset := e.vec(q, p)
			den := e.cross(a.delta, b.delta)
			if den.Sign() == 0 {
				if e.cross(offset, a.delta).Sign() != 0 {
					continue
				}
				for _, id := range []int{a.a, a.b} {
					if e.onSegment(e.points[id], e.points[b.a], e.points[b.b]) {
						e.cut(b, id)
					}
				}
				for _, id := range []int{b.a, b.b} {
					if e.onSegment(e.points[id], e.points[a.a], e.points[a.b]) {
						e.cut(a, id)
					}
				}
			} else {
				t, u := e.div(e.cross(offset, b.delta), den), e.div(e.cross(offset, a.delta), den)
				one := big.NewRat(1, 1)
				if t.Sign() >= 0 && e.cmp(t, one) <= 0 && u.Sign() >= 0 && e.cmp(u, one) <= 0 {
					id := e.point(booleanPoint{e.add(p.x, e.mul(a.delta.x, t)), e.add(p.y, e.mul(a.delta.y, t))})
					e.cut(a, id)
					e.cut(b, id)
				}
			}
			if e.err != nil {
				return e.err
			}
		}
	}
	return e.err
}

func (e *booleanEngine) sortedCuts(s booleanSegment) []int {
	if !e.charge(1, len(s.cuts)*16) {
		return nil
	}
	ids := make([]int, 0, len(s.cuts))
	for id := range s.cuts {
		ids = append(ids, id)
	}
	axis := func(id int) *big.Rat {
		if s.delta.x.Sign() != 0 {
			return e.points[id].x
		}
		return e.points[id].y
	}
	sort.Slice(ids, func(i, j int) bool { return e.cmp(axis(ids[i]), axis(ids[j])) < 0 })
	return ids
}
