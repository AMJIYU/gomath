// Package graph builds mathematical coordinate systems and sampled plots.
package graph

import (
	"fmt"
	"gomath"
	"gomath/geom"
	"gomath/path"
	"math"
)

// Range specifies finite increasing bounds and a positive tick spacing. Tick
// locations are multiples of Step; endpoints within floating-point roundoff of
// such a multiple are included. Coordinate-system constructors bound tick count.
type Range struct{ Min, Max, Step float64 }
type Axes struct {
	X, Y          Range
	Width, Height float64
	Origin        geom.Vec3
	Object        *gomath.Object
}

func NewAxes(x, y Range, width, height float64) (*Axes, error) {
	a := &Axes{X: x, Y: y, Width: width, Height: height}
	if err := a.validateMapping(); err != nil {
		return nil, err
	}
	xTicks, err := tickValues(x)
	if err != nil {
		return nil, err
	}
	yTicks, err := tickValues(y)
	if err != nil {
		return nil, err
	}
	objects := []*gomath.Object{gomath.Line(a.Point(x.Min, 0), a.Point(x.Max, 0)), gomath.Line(a.Point(0, y.Min), a.Point(0, y.Max))}
	tick := .05
	for _, v := range xTicks {
		p := a.Point(v, 0)
		objects = append(objects, gomath.Line(p.Add(geom.V(0, -tick)), p.Add(geom.V(0, tick))))
	}
	for _, v := range yTicks {
		p := a.Point(0, v)
		objects = append(objects, gomath.Line(p.Add(geom.V(-tick, 0)), p.Add(geom.V(tick, 0))))
	}
	a.Object = gomath.Group(objects...).SetStroke(gomath.RGB(.4, .45, .55), .018)
	if err := a.Object.Validate(); err != nil {
		return nil, fmt.Errorf("axes geometry: %w", err)
	}
	return a, nil
}

// Point maps mathematical coordinates to scene coordinates, extrapolating
// outside X/Y. Direct transforms of Object do not change this mapping; use Shift
// to translate both geometry and mapping. Extreme extrapolation may overflow.
func (a *Axes) Point(x, y float64) geom.Vec3 {
	return a.Origin.Add(geom.V(((x-a.X.Min)/(a.X.Max-a.X.Min)-.5)*a.Width, ((y-a.Y.Min)/(a.Y.Max-a.Y.Min)-.5)*a.Height))
}
func (a *Axes) Coordinates(p geom.Vec3) (float64, float64) {
	p = p.Sub(a.Origin)
	return a.X.Min + (p.X/a.Width+.5)*(a.X.Max-a.X.Min), a.Y.Min + (p.Y/a.Height+.5)*(a.Y.Max-a.Y.Min)
}
func (a *Axes) Shift(delta geom.Vec3) *Axes {
	a.Origin = a.Origin.Add(delta)
	a.Object.Shift(delta)
	return a
}

// Plot samples f uniformly. Non-finite values break the path; discontinuities
// between finite samples must be split by the caller into separate domains.
func (a *Axes) Plot(f func(float64) float64, samples int) (*gomath.Object, error) {
	if f == nil || samples < 2 || samples > 1_000_000 {
		return nil, fmt.Errorf("plot requires a function and 2..1000000 samples")
	}
	if err := a.validateMapping(); err != nil {
		return nil, err
	}
	var b path.Builder
	open := false
	for i := range samples {
		x := partitionPoint(a.X.Min, a.X.Max, i, samples-1)
		y := f(x)
		if !finite(y) {
			open = false
			continue
		}
		p, err := a.mappedPoint(x, y)
		if err != nil {
			return nil, err
		}
		if !open {
			b.MoveTo(p)
			open = true
		} else {
			b.LineTo(p)
		}
	}
	return gomath.NewObject(b.Path()).SetStroke(gomath.Blue, .035), nil
}
func Parametric(f func(float64) geom.Vec3, min, max float64, samples int) (*gomath.Object, error) {
	if f == nil || !finite(min) || !finite(max) || !finite(max-min) || max <= min || samples < 2 || samples > 1_000_000 {
		return nil, fmt.Errorf("invalid parametric curve")
	}
	var b path.Builder
	open := false
	for i := range samples {
		p := f(partitionPoint(min, max, i, samples-1))
		if !p.Finite() {
			open = false
			continue
		}
		if !open {
			b.MoveTo(p)
			open = true
		} else {
			b.LineTo(p)
		}
	}
	return gomath.NewObject(b.Path()), nil
}
func finite(f float64) bool { return !math.IsNaN(f) && !math.IsInf(f, 0) }
