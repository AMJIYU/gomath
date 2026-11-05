package graph

import (
	"fmt"
	"gomath/geom"
	"math"
)

const maxTicks = 10000

func validateRange(r Range) error {
	span := r.Max - r.Min
	if !finite(r.Min) || !finite(r.Max) || !finite(r.Step) || !finite(span) || span <= 0 || r.Step <= 0 || span/r.Step >= maxTicks || r.Min+r.Step <= r.Min || r.Max-r.Step >= r.Max {
		return fmt.Errorf("range requires finite Min < Max, advancing Step > 0 and fewer than %d tick intervals", maxTicks)
	}
	return nil
}

// Tick coordinates are multiples of Step, not repeated floating-point additions.
func tickValues(r Range) ([]float64, error) {
	if err := validateRange(r); err != nil {
		return nil, err
	}
	q := r.Min / r.Step
	first := math.Ceil(q)
	nearest := math.Round(q)
	if math.Abs(q-nearest) <= math.Min(1e-9, 8*math.Abs(math.Nextafter(q, math.Inf(1))-q)) {
		first = nearest
	}
	if !finite(first) {
		return nil, fmt.Errorf("tick origin overflows")
	}
	// Extra candidates cover endpoint rounding, with no unbounded float loop.
	n := int(math.Ceil((r.Max-r.Min)/r.Step)) + 2
	values := make([]float64, 0, min(maxTicks, n))
	closeTo := func(value, bound float64) bool {
		tolerance := math.Min(r.Step*1e-9, 8*math.Abs(math.Nextafter(bound, math.Inf(1))-bound))
		return math.Abs(value-bound) <= tolerance
	}
	for i := range n {
		v := (first + float64(i)) * r.Step
		if v < r.Min && closeTo(v, r.Min) {
			v = r.Min
		}
		if v > r.Max && closeTo(v, r.Max) {
			v = r.Max
		}
		if v > r.Max {
			break
		}
		if !finite(v) {
			return nil, fmt.Errorf("tick coordinate overflows")
		}
		if v < r.Min {
			continue
		}
		if len(values) > 0 && v <= values[len(values)-1] {
			return nil, fmt.Errorf("tick step loses coordinate precision")
		}
		if len(values) == maxTicks {
			return nil, fmt.Errorf("tick budget exceeded")
		}
		if v == 0 {
			v = 0
		} // canonicalize negative zero for labels
		values = append(values, v)
	}
	return values, nil
}

func (a *Axes) validateMapping() error {
	if a == nil {
		return fmt.Errorf("nil axes")
	}
	if err := validateRange(a.X); err != nil {
		return fmt.Errorf("X: %w", err)
	}
	if err := validateRange(a.Y); err != nil {
		return fmt.Errorf("Y: %w", err)
	}
	if !finite(a.Width) || !finite(a.Height) || a.Width <= 0 || a.Height <= 0 || !a.Origin.Finite() {
		return fmt.Errorf("axes require finite positive dimensions and a finite origin")
	}
	return nil
}

func (a *Axes) mappedPoint(x, y float64) (geom.Vec3, error) {
	p := a.Point(x, y)
	if !p.Finite() {
		return geom.Vec3{}, fmt.Errorf("coordinate mapping overflows at (%g,%g)", x, y)
	}
	return p, nil
}

// interval accepts either orientation and returns ascending bounds and its sign.
func interval(start, end float64) (lo, hi, orientation float64, err error) {
	if !finite(start) || !finite(end) || start == end || !finite(end-start) {
		return 0, 0, 0, fmt.Errorf("interval requires distinct finite endpoints and a finite span")
	}
	if start < end {
		return start, end, 1, nil
	}
	return end, start, -1, nil
}

func partitionPoint(lo, hi float64, i, n int) float64 {
	if i == 0 {
		return lo
	}
	if i == n {
		return hi
	}
	return lo + (hi-lo)*(float64(i)/float64(n))
}

type compensated struct{ sum, correction float64 }

func (s *compensated) add(value float64) error {
	if !finite(value) {
		return fmt.Errorf("integral contribution overflows")
	}
	y := value - s.correction
	t := s.sum + y
	s.correction = (t - s.sum) - y
	s.sum = t
	if !finite(s.sum) || !finite(s.correction) {
		return fmt.Errorf("integral sum overflows")
	}
	return nil
}
