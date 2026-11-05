package network

import (
	"fmt"
	"gomath"
	"gomath/geom"
	"gomath/path"
	"math"
	"sort"
)

func (g *CurvedDiGraph) geometry(bounds []geom.Bounds) ([]edgePaths, error) {
	for _, b := range bounds {
		if !b.Valid || !curvedPoint(b.Min) || !curvedPoint(b.Max) || !curvedDimension(b.Size().X) || !curvedDimension(b.Size().Y) {
			return nil, ErrCurvedGeometry
		}
		eps := boxEpsilon(b)
		if b.Size().X <= 64*eps || b.Size().Y <= 64*eps {
			return nil, fmt.Errorf("%w: bbox below coordinate resolution", ErrCurvedGeometry)
		}
	}
	out := make([]edgePaths, len(g.routes))
	for i, e := range g.routes {
		a, b := bounds[g.graph.index[e.From]], bounds[g.graph.index[e.To]]
		if min(g.options.Options.TipSize, g.options.TipWidth) <= 16*max(boxEpsilon(a), boxEpsilon(b)) {
			return nil, fmt.Errorf("%w: tip below coordinate resolution", ErrCurvedGeometry)
		}
		var p edgePaths
		var err error
		if e.Loop != nil {
			p, err = g.loopGeometry(a, *e.Loop)
		} else {
			p, err = g.connectionGeometry(a, b, e.Sweep)
		}
		if err != nil {
			return nil, fmt.Errorf("edge %d (%q,%q): %w", i, e.From, e.To, err)
		}
		if err = curvedValidatePath(p.shaft); err != nil {
			return nil, err
		}
		if err = curvedValidatePath(p.tip); err != nil {
			return nil, err
		}
		if len(p.shaft.Contours) != 1 || len(p.shaft.Contours[0].Curves) == 0 || len(p.shaft.Contours[0].Curves) > 8 || len(p.tip.Contours) != 1 || len(p.tip.Contours[0].Curves) != 3 {
			return nil, ErrCurvedGeometry
		}
		// Validate the actual cubic/triangle geometry, not a control-polygon sample.
		if err = certifyOutside(p.shaft, []geom.Bounds{a, b}); err != nil {
			return nil, err
		}
		if err = certifyOutside(p.tip, []geom.Bounds{a, b}); err != nil {
			return nil, err
		}
		out[i] = p
	}
	return out, nil
}

func boxEpsilon(b geom.Bounds) float64 {
	scale := max(1, math.Abs(b.Min.X), math.Abs(b.Min.Y), math.Abs(b.Max.X), math.Abs(b.Max.Y))
	return max(1e-12, 16*(math.Nextafter(scale, math.Inf(1))-scale))
}
func insideBox(p geom.Vec3, b geom.Bounds, eps float64) bool {
	return p.X > b.Min.X+eps && p.X < b.Max.X-eps && p.Y > b.Min.Y+eps && p.Y < b.Max.Y-eps
}
func onBox(p geom.Vec3, b geom.Bounds) bool {
	eps := boxEpsilon(b)
	return p.X >= b.Min.X-eps && p.X <= b.Max.X+eps && p.Y >= b.Min.Y-eps && p.Y <= b.Max.Y+eps && min(math.Abs(p.X-b.Min.X), math.Abs(p.X-b.Max.X), math.Abs(p.Y-b.Min.Y), math.Abs(p.Y-b.Max.Y)) <= eps
}

func (g *CurvedDiGraph) connectionGeometry(a, b geom.Bounds, sweep float64) (edgePaths, error) {
	if a.Min.X <= b.Max.X && b.Min.X <= a.Max.X && a.Min.Y <= b.Max.Y && b.Min.Y <= a.Max.Y {
		return edgePaths{}, fmt.Errorf("%w: endpoint bboxes touch or overlap", ErrCurvedGeometry)
	}
	arc, err := gomath.NewArcBetweenPoints(a.Center(), b.Center(), gomath.ArcBetweenPointsOptions{Angle: sweep})
	if err != nil {
		return edgePaths{}, fmt.Errorf("%w: %v", ErrCurvedGeometry, err)
	}
	p := arc.Object.Path
	cuts := pathBoxCuts(p, []geom.Bounds{a, b})
	begin, end := -1.0, -1.0
	active := false
	components := 0
	for i := 1; i < len(cuts); i++ {
		lo, hi := cuts[i-1], cuts[i]
		if hi <= lo {
			continue
		}
		mid := pathPoint(p, (lo+hi)/2)
		outside := !insideBox(mid, a, 0) && !insideBox(mid, b, 0)
		if outside {
			if !active {
				components++
				begin = lo
			}
			end = hi
		}
		active = outside
	}
	if components != 1 || end <= begin {
		return edgePaths{}, fmt.Errorf("%w: no unique exposed interval", ErrCurvedGeometry)
	}
	n := float64(len(p.Contours[0].Curves))
	exposed, err := p.TrimByParameter(begin/n, end/n)
	if err != nil {
		return edgePaths{}, fmt.Errorf("%w: %v", ErrCurvedGeometry, err)
	}
	if len(exposed.Contours) != 1 || len(exposed.Contours[0].Curves) == 0 {
		return edgePaths{}, ErrCurvedGeometry
	}
	curves := exposed.Contours[0].Curves
	start, tip := curves[0].P0, curves[len(curves)-1].P3
	if !onBox(start, a) || !onBox(tip, b) || start.Sub(tip).Norm() < 1e-9 {
		return edgePaths{}, fmt.Errorf("%w: unresolved bbox boundary", ErrCurvedGeometry)
	}
	return g.trimHead(exposed, tip)
}

// Same explicit physical trim/triangle semantics as NewCurvedArrow, retaining
// the clipped original cubics instead of rebuilding a subtly different arc.
func (g *CurvedDiGraph) trimHead(p path.Path, tip geom.Vec3) (edgePaths, error) {
	estimate := 0.0
	for _, c := range p.Contours[0].Curves {
		estimate += c.P1.Sub(c.P0).Norm() + c.P2.Sub(c.P1).Norm() + c.P3.Sub(c.P2).Norm()
	}
	m, err := p.MeasureWithOptions(max(1e-12, min(estimate*1e-10, g.options.Options.TipSize*1e-6)), path.MeasureOptions{})
	if err != nil {
		return edgePaths{}, fmt.Errorf("%w: %w", ErrCurvedGeometry, err)
	}
	distance := m.Length() - g.options.Options.TipSize
	if distance <= 0 || distance == m.Length() {
		return edgePaths{}, fmt.Errorf("%w: tip consumes shaft or loses precision", ErrCurvedGeometry)
	}
	shaft, err := m.TrimByLength(0, distance)
	if err != nil {
		return edgePaths{}, fmt.Errorf("%w: %w", ErrCurvedGeometry, err)
	}
	sample, err := m.AtLength(distance)
	if err != nil {
		return edgePaths{}, fmt.Errorf("%w: %w", ErrCurvedGeometry, err)
	}
	u := tip.Sub(sample.Point).Unit()
	v := geom.V(-u.Y, u.X).Mul(g.options.TipWidth / 2)
	if u == (geom.Vec3{}) || v == (geom.Vec3{}) || sample.Point.Add(v) == sample.Point.Sub(v) {
		return edgePaths{}, ErrCurvedGeometry
	}
	head := gomath.Polygon(tip, sample.Point.Add(v), sample.Point.Sub(v))
	return edgePaths{shaft: shaft, tip: head.Path}, nil
}

func (g *CurvedDiGraph) loopGeometry(b geom.Bounds, o LoopOptions) (edgePaths, error) {
	n := geom.V(0, 1)
	half := b.Size().X / 2
	offset := b.Size().Y / 2
	switch o.Side {
	case LoopRight:
		n = geom.V(1, 0)
		half = b.Size().Y / 2
		offset = b.Size().X / 2
	case LoopBottom:
		n = geom.V(0, -1)
	case LoopLeft:
		n = geom.V(-1, 0)
		half = b.Size().Y / 2
		offset = b.Size().X / 2
	}
	side := geom.V(n.Y, -n.X)
	spread := min(half*.75, o.Radius*.5)
	ratio := spread / o.Radius
	if ratio < 1e-5 || spread <= boxEpsilon(b)*64 {
		return edgePaths{}, fmt.Errorf("%w: loop radius/face resolution", ErrCurvedGeometry)
	}
	center := b.Center().Add(n.Mul(offset))
	start, end := center.Add(side.Mul(spread)), center.Sub(side.Mul(spread))
	sweep := 2*math.Pi - 2*math.Asin(ratio)
	if max(1e-12, o.Radius*sweep*1e-10) > g.options.Options.TipSize*1e-6 {
		return edgePaths{}, fmt.Errorf("%w: inherited loop trim tolerance exceeds tip resolution", ErrCurvedGeometry)
	}
	if o.Clockwise {
		start, end = end, start
		sweep = -sweep
	}
	arrow, err := gomath.NewCurvedArrow(start, end, gomath.CurvedArrowOptions{Arc: gomath.ArcBetweenPointsOptions{Angle: sweep}, TipLength: g.options.Options.TipSize, TipWidth: g.options.TipWidth})
	if err != nil {
		return edgePaths{}, fmt.Errorf("%w: %v", ErrCurvedGeometry, err)
	}
	if !onBox(start, b) || !onBox(end, b) {
		return edgePaths{}, ErrCurvedGeometry
	}
	return edgePaths{shaft: arrow.Children[0].Path, tip: arrow.Children[1].Path}, nil
}

func pathPoint(p path.Path, t float64) geom.Vec3 {
	c := p.Contours[0].Curves
	if t <= 0 {
		return c[0].P0
	}
	if t >= float64(len(c)) {
		return c[len(c)-1].P3
	}
	i := int(t)
	return c[i].At(t - float64(i))
}

// Each cubic has at most three roots against each rectangle face. Derivative
// isolation gives monotone intervals; each sign-changing root uses <=64 binary
// iterations. These roots also certify that no unsampled bbox crossing exists.
func cubicFaceRoots(c path.Cubic, axis int, face float64) []float64 {
	coord := func(p geom.Vec3) float64 {
		if axis == 0 {
			return p.X
		}
		return p.Y
	}
	q := [4]float64{coord(c.P0) - face, coord(c.P1) - face, coord(c.P2) - face, coord(c.P3) - face}
	scale := max(math.Abs(q[0]), math.Abs(q[1]), math.Abs(q[2]), math.Abs(q[3]))
	if scale == 0 {
		return []float64{0, 1}
	}
	for i := range q {
		q[i] /= scale
	}
	a := -q[0] + 3*q[1] - 3*q[2] + q[3]
	b := 3*q[0] - 6*q[1] + 3*q[2]
	d := 3 * (q[1] - q[0])
	cuts := []float64{0, 1}
	add := func(t float64) {
		if finite(t) && t > 0 && t < 1 {
			cuts = append(cuts, t)
		}
	}
	A, B, C := 3*a, 2*b, d
	if A == 0 {
		if B != 0 {
			add(-C / B)
		}
	} else {
		disc := math.FMA(B, B, -4*A*C)
		if disc >= 0 {
			s := math.Sqrt(disc)
			z := -.5 * (B + math.Copysign(s, B))
			if z == 0 {
				add(-B / (2 * A))
			} else {
				add(z / A)
				add(C / z)
			}
		}
	}
	sort.Float64s(cuts)
	value := func(t float64) float64 { u := 1 - t; return u*u*u*q[0] + 3*u*u*t*q[1] + 3*u*t*t*q[2] + t*t*t*q[3] }
	roots := make([]float64, 0, 3)
	for i := 1; i < len(cuts); i++ {
		lo, hi := cuts[i-1], cuts[i]
		v, w := value(lo), value(hi)
		if v == 0 {
			roots = append(roots, lo)
		}
		if (v < 0 && w > 0) || (v > 0 && w < 0) {
			for range 64 {
				mid := (lo + hi) / 2
				if mid == lo || mid == hi {
					break
				}
				f := value(mid)
				if f == 0 {
					lo = mid
					hi = mid
					break
				}
				if (f < 0) == (v < 0) {
					lo = mid
					v = f
				} else {
					hi = mid
				}
			}
			roots = append(roots, (lo+hi)/2)
		}
		if i == len(cuts)-1 && w == 0 {
			roots = append(roots, hi)
		}
	}
	return roots
}

func pathBoxCuts(p path.Path, boxes []geom.Bounds) []float64 {
	c := p.Contours[0].Curves
	cuts := make([]float64, 0, len(c)*32+2)
	cuts = append(cuts, 0, float64(len(c)))
	for i, v := range c {
		for _, b := range boxes {
			for axis := range 2 {
				lo, hi := b.Min.X, b.Max.X
				if axis == 1 {
					lo, hi = b.Min.Y, b.Max.Y
				}
				for _, face := range []float64{lo, hi} {
					for _, t := range cubicFaceRoots(v, axis, face) {
						cuts = append(cuts, float64(i)+t)
					}
				}
			}
		}
	}
	sort.Float64s(cuts)
	out := cuts[:0]
	for _, v := range cuts {
		if len(out) == 0 || v-out[len(out)-1] > 2e-14 {
			out = append(out, v)
		}
	}
	return out
}
func certifyOutside(p path.Path, boxes []geom.Bounds) error {
	cuts := pathBoxCuts(p, boxes)
	for i := 1; i < len(cuts); i++ {
		point := pathPoint(p, (cuts[i-1]+cuts[i])/2)
		for _, b := range boxes {
			if insideBox(point, b, boxEpsilon(b)) {
				return fmt.Errorf("%w: shaft/head enters endpoint bbox", ErrCurvedGeometry)
			}
		}
	}
	return nil
}
