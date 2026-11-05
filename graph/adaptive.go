package graph

import (
	"errors"
	"fmt"
	"gomath"
	"gomath/geom"
	"gomath/path"
	"math"
)

var (
	ErrAdaptiveBudget    = errors.New("adaptive plot evaluation budget exhausted")
	ErrAdaptiveDepth     = errors.New("adaptive plot recursion limit reached before tolerance")
	ErrAdaptivePrecision = errors.New("adaptive plot interval cannot be subdivided in float64")
)

type AdaptiveOptions struct {
	Tolerance       float64 // Scene-space midpoint deviation, not a certified bound.
	InitialSegments int     // 1..4096 uniform seed intervals.
	MaxDepth        int     // Additional subdivision levels per seed; 0..24.
	MaxEvaluations  int     // Hard function-call budget, 2..1000000.
}

func DefaultAdaptiveOptions() AdaptiveOptions {
	return AdaptiveOptions{Tolerance: .005, InitialSegments: 8, MaxDepth: 16, MaxEvaluations: 50000}
}

// PlotGap is a conservative interval omitted because at least one of its
// sampled endpoints or midpoint was non-finite. It is not an analytic domain
// boundary. Adjacent omitted intervals are merged.
type PlotGap struct{ Min, Max float64 }

type AdaptivePlot struct {
	Object           *gomath.Object
	Evaluations      int
	NonFiniteSamples int
	Gaps             []PlotGap
}

type adaptiveSample struct {
	x     float64
	point geom.Vec3
	valid bool
}

// PlotAdaptive builds an XY polyline over an ascending explicit finite
// interval. Each accepted interval samples its midpoint and compares it with
// the chord midpoint in scene units. Non-finite samples always break contours;
// mixed finite/non-finite intervals are bisected to MaxDepth before omission.
// An interval with three non-finite samples is omitted immediately.
//
// This sampling heuristic cannot discover arbitrary finite discontinuities,
// narrow finite islands, singularities or oscillations between sampled points.
// Tolerance is not a proof of maximum error. Supply separate intervals for known
// discontinuities. Exhausting the function-call budget or reaching MaxDepth
// with unresolved finite error returns an error and no partial object. The
// function is evaluated only during construction, never by frame sampling.
func (a *Axes) PlotAdaptive(f func(float64) float64, start, end float64, o AdaptiveOptions) (*AdaptivePlot, error) {
	if err := a.validateMapping(); err != nil {
		return nil, err
	}
	if f == nil || !finite(start) || !finite(end) || end <= start || !finite(end-start) {
		return nil, fmt.Errorf("adaptive plot requires a function and finite start < end")
	}
	if !finite(o.Tolerance) || o.Tolerance <= 0 || o.InitialSegments < 1 || o.InitialSegments > 4096 || o.MaxDepth < 0 || o.MaxDepth > 24 || o.MaxEvaluations < 2 || o.MaxEvaluations > 1_000_000 {
		return nil, fmt.Errorf("invalid adaptive plot tolerance, seed count, depth or evaluation budget")
	}
	r := &AdaptivePlot{}
	cache := make(map[float64]adaptiveSample)
	eval := func(x float64) (adaptiveSample, error) {
		if sample, ok := cache[x]; ok {
			return sample, nil
		}
		if r.Evaluations == o.MaxEvaluations {
			return adaptiveSample{}, fmt.Errorf("%w (%d calls)", ErrAdaptiveBudget, r.Evaluations)
		}
		y := f(x)
		r.Evaluations++
		sample := adaptiveSample{x: x, valid: finite(y)}
		if sample.valid {
			var err error
			sample.point, err = a.mappedPoint(x, y)
			if err != nil {
				return adaptiveSample{}, err
			}
		} else {
			r.NonFiniteSamples++
		}
		cache[x] = sample
		return sample, nil
	}
	var builder path.Builder
	var last geom.Vec3
	open := false
	addGap := func(lo, hi float64) {
		open = false
		if n := len(r.Gaps); n > 0 && r.Gaps[n-1].Max == lo {
			r.Gaps[n-1].Max = hi
		} else {
			r.Gaps = append(r.Gaps, PlotGap{lo, hi})
		}
	}
	var refine func(left, right adaptiveSample, depth int) error
	refine = func(left, right adaptiveSample, depth int) error {
		midX := left.x + (right.x-left.x)/2
		if midX <= left.x || midX >= right.x {
			if !left.valid || !right.valid {
				addGap(left.x, right.x)
				return nil
			}
			return fmt.Errorf("%w on [%g,%g]", ErrAdaptivePrecision, left.x, right.x)
		}
		mid, err := eval(midX)
		if err != nil {
			return err
		}
		if !left.valid || !mid.valid || !right.valid {
			if !left.valid && !mid.valid && !right.valid || depth == o.MaxDepth {
				addGap(left.x, right.x)
				return nil
			}
			if err := refine(left, mid, depth+1); err != nil {
				return err
			}
			return refine(mid, right, depth+1)
		}
		if left.point.X >= mid.point.X || mid.point.X >= right.point.X {
			return fmt.Errorf("%w in scene-space mapping", ErrAdaptivePrecision)
		}
		expected := left.point.Lerp(right.point, .5)
		deviation := mid.point.Sub(expected)
		errorEstimate := math.Hypot(deviation.X, deviation.Y)
		if !finite(errorEstimate) {
			return fmt.Errorf("adaptive plot deviation overflows on [%g,%g]", left.x, right.x)
		}
		if errorEstimate > o.Tolerance {
			if depth == o.MaxDepth {
				return fmt.Errorf("%w on [%g,%g] (deviation %g)", ErrAdaptiveDepth, left.x, right.x, errorEstimate)
			}
			if err := refine(left, mid, depth+1); err != nil {
				return err
			}
			return refine(mid, right, depth+1)
		}
		if !open || last != left.point {
			builder.MoveTo(left.point)
		}
		builder.LineTo(mid.point)
		builder.LineTo(right.point)
		last, open = right.point, true
		return nil
	}
	left, err := eval(start)
	if err != nil {
		return nil, err
	}
	for i := range o.InitialSegments {
		x := partitionPoint(start, end, i+1, o.InitialSegments)
		if x <= left.x {
			return nil, fmt.Errorf("%w in initial partition", ErrAdaptivePrecision)
		}
		right, err := eval(x)
		if err != nil {
			return nil, err
		}
		if err := refine(left, right, 0); err != nil {
			return nil, err
		}
		left = right
	}
	r.Object = gomath.NewObject(builder.Path()).SetStroke(gomath.Blue, .035)
	if err := r.Object.Validate(); err != nil {
		return nil, err
	}
	return r, nil
}
