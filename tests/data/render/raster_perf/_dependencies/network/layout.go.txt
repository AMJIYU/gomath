// Package network provides planar simple graphs as ordinary GoMath object trees.
// Layouts and edge updates are explicit authoring operations; no background
// updater or external graph engine is installed.
package network

import (
	"fmt"
	"gomath/geom"
	"math"
	"math/rand/v2"
)

type LayoutKind string

const (
	Circular LayoutKind = "circular"
	Grid     LayoutKind = "grid"
	Spring   LayoutKind = "spring"
)

// LayoutOptions describes a deterministic layout in the XY plane. Scale is the
// radius for Circular and the maximum half-extent for Grid and Spring. Columns
// is used only by Grid (zero chooses ceil(sqrt(n))). Seed and Iterations are
// used only by Spring. Iterations must be in [1,1000]; work is additionally
// limited to 20 million vertex-pair iterations. Center.Z must be zero.
type LayoutOptions struct {
	Kind       LayoutKind
	Center     geom.Vec3
	Scale      float64
	Columns    int
	Seed       uint64
	Iterations int
}

func DefaultLayout() LayoutOptions {
	return LayoutOptions{Kind: Circular, Scale: 2, Iterations: 100, Seed: 1}
}

func (o LayoutOptions) validate(n, edges int) error {
	if !planar(o.Center) || !finite(o.Scale) || o.Scale <= 0 || !finite(o.Scale*4) || !o.Center.Add(geom.V(o.Scale*4, o.Scale*4)).Finite() || !o.Center.Sub(geom.V(o.Scale*4, o.Scale*4)).Finite() {
		return fmt.Errorf("layout requires a finite planar center and positive finite scale")
	}
	switch o.Kind {
	case Circular:
	case Grid:
		if o.Columns < 0 || o.Columns > 4096 {
			return fmt.Errorf("grid columns must be in [0,4096]")
		}
	case Spring:
		work := int64(n)*int64(n) + int64(edges)
		if o.Iterations < 1 || o.Iterations > 1000 || work*int64(o.Iterations) > 20_000_000 {
			return fmt.Errorf("spring layout requires 1..1000 iterations and at most 20000000 pair/edge iterations")
		}
	default:
		return fmt.Errorf("unsupported layout %q", o.Kind)
	}
	return nil
}

func layoutPositions(n int, edges [][2]int, o LayoutOptions) ([]geom.Vec3, error) {
	if err := o.validate(n, len(edges)); err != nil {
		return nil, err
	}
	p := make([]geom.Vec3, n)
	switch o.Kind {
	case Circular:
		if n > 1 {
			for i := range p {
				a := math.Pi/2 + 2*math.Pi*float64(i)/float64(n)
				p[i] = geom.V(math.Cos(a), math.Sin(a))
			}
		}
	case Grid:
		cols := o.Columns
		if cols == 0 {
			cols = int(math.Ceil(math.Sqrt(float64(n))))
		}
		cols = max(1, min(cols, max(1, n)))
		rows := (n + cols - 1) / cols
		denom := float64(max(1, max(cols-1, rows-1)))
		for i := range p {
			p[i] = geom.V((2*float64(i%cols)-float64(cols-1))/denom, (float64(rows-1)-2*float64(i/cols))/denom)
		}
	case Spring:
		if n > 1 {
			rng := rand.New(rand.NewPCG(o.Seed, 0x9e3779b97f4a7c15))
			for i := range p {
				p[i] = geom.V(2*rng.Float64()-1, 2*rng.Float64()-1)
			}
			k := math.Sqrt(4 / float64(n))
			forces := make([]geom.Vec3, n)
			for iter := range o.Iterations {
				clear(forces)
				for i := range p {
					for j := i + 1; j < n; j++ {
						delta := p[i].Sub(p[j])
						d := math.Hypot(delta.X, delta.Y)
						if d < 1e-9 {
							// Deterministic separation even for coincident vertices.
							delta, d = geom.V(1e-9, 0), 1e-9
						}
						f := delta.Mul(k * k / (d * d))
						forces[i], forces[j] = forces[i].Add(f), forces[j].Sub(f)
					}
				}
				for _, e := range edges {
					delta := p[e[0]].Sub(p[e[1]])
					f := delta.Mul(math.Hypot(delta.X, delta.Y) / k)
					forces[e[0]], forces[e[1]] = forces[e[0]].Sub(f), forces[e[1]].Add(f)
				}
				temperature := .15 * (1 - float64(iter)/float64(o.Iterations))
				for i, f := range forces {
					d := math.Hypot(f.X, f.Y)
					if d > 0 {
						p[i] = p[i].Add(f.Mul(min(d, temperature) / d))
					}
					p[i].X = max(-1, min(1, p[i].X))
					p[i].Y = max(-1, min(1, p[i].Y))
				}
			}
		}
	}
	for i := range p {
		p[i] = o.Center.Add(p[i].Mul(o.Scale))
		if !planar(p[i]) {
			return nil, fmt.Errorf("layout overflow at vertex %d", i)
		}
	}
	return p, nil
}

func finite(v float64) bool   { return !math.IsNaN(v) && !math.IsInf(v, 0) }
func planar(p geom.Vec3) bool { return p.Finite() && p.Z == 0 }
