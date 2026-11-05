package path

import "fmt"

const MaxLengthLookupSamples = 1 << 20

// MeasureOptions controls a bounded optional inverse-query acceleration table.
// MaxLookupSamples counts parameter/length knots over the entire path. Zero
// disables lookup tables. If a curve cannot fit the remaining budget, that
// curve keeps the original accurate quadrature/Newton path; accuracy is never
// traded for table size. Tables store accepted quadrature leaves, not a fixed
// sampling approximation. Each stored knot is two float64s.
type MeasureOptions struct{ MaxLookupSamples int }

func DefaultMeasureOptions() MeasureOptions { return MeasureOptions{MaxLookupSamples: 65536} }

type lengthKnot struct{ parameter, length float64 }

type lengthLookupBuilder struct {
	knots        []lengthKnot
	limit, start int
	prefix       float64
	failed       bool
}

func (b *lengthLookupBuilder) begin() {
	b.start = len(b.knots)
	b.prefix = 0
	b.failed = false
	b.add(0, 0)
}

func (b *lengthLookupBuilder) add(parameter, length float64) {
	if b.failed {
		return
	}
	if len(b.knots) == b.limit {
		b.knots = b.knots[:b.start]
		b.failed = true
		return
	}
	if len(b.knots) == cap(b.knots) {
		n := min(b.limit, max(32, cap(b.knots)*2))
		next := make([]lengthKnot, len(b.knots), n)
		copy(next, b.knots)
		b.knots = next
	}
	b.knots = append(b.knots, lengthKnot{parameter: parameter, length: length})
}

func (b *lengthLookupBuilder) correctEnd(length float64) {
	if !b.failed && len(b.knots) > b.start {
		b.knots[len(b.knots)-1].length = length
	}
}

func (m *ArcLength) parameterForCurve(c *measuredCurve, distance float64) (float64, error) {
	if c.lookupCount < 2 || distance == 0 || distance == c.length {
		return c.parameterAtLength(distance, c.length, m.tolerance)
	}
	if err := checkLengthRange(distance, distance, c.length); err != nil {
		return 0, err
	}
	knots := m.lookup[c.lookupStart : c.lookupStart+c.lookupCount]
	lo, hi := 1, len(knots)
	for lo < hi {
		mid := lo + (hi-lo)/2
		if knots[mid].length < distance {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	if lo == len(knots) {
		return 0, fmt.Errorf("%w: length lookup range", ErrLengthConvergence)
	}
	a, b := knots[lo-1], knots[lo]
	return c.parameterInLengthInterval(distance, m.tolerance, a.parameter, b.parameter, a.length, b.length)
}
