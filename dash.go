package gomath

import (
	"fmt"
	"math"
)

// MaxDashEntries bounds the normalized, even-length dash/gap list.
const MaxDashEntries = 1024

// DashPattern is an immutable alternating dash/gap pattern in world units.
// The zero value is invalid; nil in Style.Dash selects a solid stroke. Construct
// with NewDashPattern. Copies and sampled frames may safely share the pointer.
type DashPattern struct {
	lengths        []float64
	period, offset float64
}

// NewDashPattern copies lengths, repeats odd lists, and reduces a finite offset
// modulo the period. Positive offsets advance into the pattern (SVG semantics).
// Entries must be finite and nonnegative, with a positive finite sum. Individual
// zero entries are allowed: zero dashes produce dots with round/square caps.
func NewDashPattern(lengths []float64, offset float64) (*DashPattern, error) {
	n := len(lengths)
	if n%2 != 0 {
		n *= 2
	}
	if n == 0 || n > MaxDashEntries {
		return nil, fmt.Errorf("dash pattern requires 1..%d normalized entries", MaxDashEntries)
	}
	if !finite(offset) {
		return nil, fmt.Errorf("dash offset must be finite")
	}
	sum := 0.0
	for _, v := range lengths {
		if !finite(v) || v < 0 {
			return nil, fmt.Errorf("dash lengths must be finite and nonnegative")
		}
		sum += v
	}
	if n != len(lengths) {
		sum *= 2
	}
	if !finite(sum) || sum <= 0 {
		return nil, fmt.Errorf("dash period must be finite and positive; all-zero patterns are unsupported")
	}
	phase := math.Mod(offset, sum)
	if phase < 0 {
		phase += sum
	}
	if phase == sum || phase == 0 {
		phase = 0
	}
	values := make([]float64, n)
	for i := range values {
		values[i] = lengths[i%len(lengths)]
	}
	return &DashPattern{lengths: values, period: sum, offset: phase}, nil
}

// Lengths returns an independent copy of the normalized dash/gap list.
func (d *DashPattern) Lengths() []float64 {
	if d == nil {
		return nil
	}
	return append([]float64(nil), d.lengths...)
}
func (d *DashPattern) Offset() float64 {
	if d == nil {
		return 0
	}
	return d.offset
}
func (d *DashPattern) Period() float64 {
	if d == nil {
		return 0
	}
	return d.period
}

// Validate checks construction; private storage cannot be mutated by callers.
func (d *DashPattern) Validate() error {
	if d == nil {
		return nil
	}
	if len(d.lengths) == 0 || len(d.lengths) > MaxDashEntries || len(d.lengths)%2 != 0 || !finite(d.period) || d.period <= 0 || !finite(d.offset) || d.offset < 0 || d.offset >= d.period {
		return fmt.Errorf("invalid dash pattern; use NewDashPattern")
	}
	return nil
}

// Scaled creates another immutable pattern for a positive uniform scale.
// A nil pattern remains solid. Underflow/overflow or a collapsed scale errors.
func (d *DashPattern) Scaled(scale float64) (*DashPattern, error) {
	if d == nil {
		return nil, nil
	}
	if err := d.Validate(); err != nil {
		return nil, err
	}
	if !finite(scale) || scale <= 0 {
		return nil, fmt.Errorf("dash scale must be finite and positive")
	}
	if scale == 1 {
		return d, nil
	}
	values := d.Lengths()
	for i, v := range values {
		values[i] = v * scale
		if !finite(values[i]) || v > 0 && values[i] == 0 {
			return nil, fmt.Errorf("scaled dash length exceeds numeric range")
		}
	}
	offset := d.offset * scale
	if !finite(offset) || d.offset > 0 && offset == 0 {
		return nil, fmt.Errorf("scaled dash offset exceeds numeric range")
	}
	return NewDashPattern(values, offset)
}
