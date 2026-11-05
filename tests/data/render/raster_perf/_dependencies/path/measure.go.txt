package path

import (
	"fmt"
	"gomath/geom"
)

type measuredCurve struct {
	lengthCubic
	start, end, length       float64
	contour, index           int
	lookupStart, lookupCount int
}
type measuredContour struct {
	start              geom.Vec3
	closed             bool
	first, count       int // Includes a synthetic closing line, if necessary.
	explicit           int
	startLength, total float64
}

// ArcLength is an immutable snapshot of a path and its cumulative lengths.
// Build it once with Path.Measure for repeated animation queries. Queries use
// bounded quadrature and safeguarded Newton/bisection, with a bounded table of
// accepted integration leaves to localize repeated inverse queries. Successful queries
// allocate no memory and are safe concurrently. No references to the author's
// curve slices are kept.
//
// Contour gaps add no length. At a positive-distance contour/curve boundary,
// queries choose the incoming positive-length cubic. Distance zero chooses the
// first positive-length cubic. Zero-length contours are skipped unless the
// entire path is stationary, in which case its first start point is returned.
type ArcLength struct {
	curves    []measuredCurve
	contours  []measuredContour
	total     float64
	tolerance float64
	lookup    []lengthKnot
}

func contourLengthCount(c Contour) int {
	n := len(c.Curves)
	if n > 0 && c.Closed && c.Curves[n-1].P3 != c.Start {
		n++
	}
	return n
}
func contourLengthCurve(c Contour, i int) Cubic {
	if i < len(c.Curves) {
		return c.Curves[i]
	}
	return Line(c.Curves[len(c.Curves)-1].P3, c.Start)
}
func (p Path) lengthCount() int {
	n := 0
	for _, c := range p.Contours {
		n += contourLengthCount(c)
	}
	return n
}

// Length sums contour lengths, including implicit closing lines and excluding
// jumps between contours. The absolute tolerance is shared across all cubics.
func (p Path) Length(tolerance float64) (float64, error) {
	if err := checkLengthTolerance(tolerance); err != nil {
		return 0, err
	}
	if err := p.Validate(); err != nil {
		return 0, err
	}
	n := p.lengthCount()
	if n == 0 {
		return 0, nil
	}
	budget, err := lengthBudget(tolerance, 4*float64(n))
	if err != nil {
		return 0, err
	}
	var sum, compensation float64
	for i, contour := range p.Contours {
		for j := range contourLengthCount(contour) {
			q, err := prepareLengthCubic(contourLengthCurve(contour, j))
			if err != nil {
				return 0, fmt.Errorf("contour %d curve %d: %w", i, j, err)
			}
			length, err := q.integral(0, 1, budget)
			if err != nil {
				return 0, fmt.Errorf("contour %d curve %d: %w", i, j, err)
			}
			y := length - compensation
			t := sum + y
			compensation, sum = (t-sum)-y, t
			if !lengthFinite(sum) {
				return 0, fmt.Errorf("path length exceeds finite float64 range")
			}
		}
	}
	return sum, nil
}

// Length measures a contour, including its closing edge when Closed is true
// even if the edge was not explicitly stored as a cubic.
func (c Contour) Length(tolerance float64) (float64, error) {
	return (Path{Contours: []Contour{c}}).Length(tolerance)
}

// Measure builds an independent length snapshot. Its length error budget is
// tolerance/4, leaving room for cumulative and inverse-query error within the
// requested absolute tolerance. Use AtFraction for constant-speed motion.
// DefaultMeasureOptions bounds optional lookup storage; use MeasureWithOptions
// with a zero lookup budget to disable it without changing the error policy.
func (p Path) Measure(tolerance float64) (*ArcLength, error) {
	return p.MeasureWithOptions(tolerance, DefaultMeasureOptions())
}

// MeasureWithOptions selects the optional bounded inverse-query lookup budget.
// Construction records already-accepted quadrature leaves without extra speed
// evaluations. Query accuracy/iteration limits are the same with tables off.
func (p Path) MeasureWithOptions(tolerance float64, opt MeasureOptions) (*ArcLength, error) {
	if opt.MaxLookupSamples < 0 || opt.MaxLookupSamples > MaxLengthLookupSamples {
		return nil, fmt.Errorf("length lookup sample budget must be in [0,%d]", MaxLengthLookupSamples)
	}
	if err := checkLengthTolerance(tolerance); err != nil {
		return nil, err
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	n := p.lengthCount()
	budget := tolerance
	if n > 0 {
		var err error
		budget, err = lengthBudget(tolerance, 4*float64(n))
		if err != nil {
			return nil, err
		}
	}
	m := &ArcLength{
		curves: make([]measuredCurve, 0, n), contours: make([]measuredContour, 0, len(p.Contours)),
		tolerance: tolerance,
	}
	var compensation float64
	lookup := lengthLookupBuilder{limit: opt.MaxLookupSamples}
	for i, c := range p.Contours {
		meta := measuredContour{start: c.Start, closed: c.Closed, first: len(m.curves), count: contourLengthCount(c), explicit: len(c.Curves), startLength: m.total}
		for j := range meta.count {
			q, err := prepareLengthCubic(contourLengthCurve(c, j))
			if err != nil {
				return nil, fmt.Errorf("contour %d curve %d: %w", i, j, err)
			}
			var record *lengthLookupBuilder
			if lookup.limit-len(lookup.knots) >= 2 {
				lookup.begin()
				record = &lookup
			}
			length, err := q.integralRecorded(0, 1, budget, record)
			if err != nil {
				return nil, fmt.Errorf("contour %d curve %d: %w", i, j, err)
			}
			start := m.total
			y := length - compensation
			total := m.total + y
			compensation, m.total = (total-m.total)-y, total
			if !lengthFinite(total) {
				return nil, fmt.Errorf("path length exceeds finite float64 range")
			}
			curve := measuredCurve{lengthCubic: q, start: start, end: total, length: length, contour: i, index: j}
			if record != nil && !lookup.failed {
				curve.lookupStart = lookup.start
				curve.lookupCount = len(lookup.knots) - lookup.start
			}
			m.curves = append(m.curves, curve)
		}
		meta.total = m.total
		m.contours = append(m.contours, meta)
	}
	m.lookup = lookup.knots
	return m, nil
}

func (c Contour) Measure(tolerance float64) (*ArcLength, error) {
	return (Path{Contours: []Contour{c}}).Measure(tolerance)
}

// Length returns the snapshot's total length; a nil/zero snapshot is empty.
func (m *ArcLength) Length() float64 {
	if m == nil {
		return 0
	}
	return m.total
}

// AtLength samples an absolute distance in [0,Length()]. It never extrapolates
// or clamps out-of-range input. No arc length is assigned to pen-up moves.
func (m *ArcLength) AtLength(distance float64) (LengthSample, error) {
	if m == nil || len(m.contours) == 0 {
		return LengthSample{}, ErrEmptyPath
	}
	if err := checkLengthRange(distance, distance, m.total); err != nil {
		return LengthSample{}, err
	}
	if m.total == 0 {
		index := -1
		if m.contours[0].explicit > 0 {
			index = 0
		}
		return LengthSample{Point: m.contours[0].start, Curve: index}, nil
	}
	lo, hi := 0, len(m.curves)
	if distance == 0 {
		// Skip leading constant curves and point-only contours.
		for lo < hi && m.curves[lo].end == 0 {
			lo++
		}
	} else {
		for lo < hi {
			mid := lo + (hi-lo)/2
			if m.curves[mid].end < distance {
				lo = mid + 1
			} else {
				hi = mid
			}
		}
	}
	if lo >= len(m.curves) {
		return LengthSample{}, fmt.Errorf("%w: cumulative length lookup", ErrLengthConvergence)
	}
	segment := &m.curves[lo]
	local := distance - segment.start
	// At a cumulative endpoint, use the exact cubic endpoint instead of a
	// subtraction rounded differently from the stored local length.
	if distance == segment.end {
		local = segment.length
	}
	if local > segment.length && local-segment.length <= m.tolerance/4 {
		local = segment.length
	}
	t, err := m.parameterForCurve(segment, local)
	if err != nil {
		return LengthSample{}, err
	}
	return segment.sample(distance, t, segment.contour, segment.index)
}

// AtFraction samples normalized distance in [0,1], suitable for MoveAlongPath.
func (m *ArcLength) AtFraction(alpha float64) (LengthSample, error) {
	if !lengthFinite(alpha) || alpha < 0 || alpha > 1 {
		return LengthSample{}, fmt.Errorf("arc length fraction must be finite and within [0,1]")
	}
	return m.AtLength(alpha * m.Length())
}

func (p Path) AtLength(distance, tolerance float64) (LengthSample, error) {
	m, err := p.Measure(tolerance)
	if err != nil {
		return LengthSample{}, err
	}
	return m.AtLength(distance)
}
func (c Contour) AtLength(distance, tolerance float64) (LengthSample, error) {
	m, err := c.Measure(tolerance)
	if err != nil {
		return LengthSample{}, err
	}
	return m.AtLength(distance)
}

// Resample returns count samples uniformly spaced in arc length, including both
// endpoints (also for a closed contour). Count must be in [2,MaxLengthSamples].
// Samples retain contour indexes: callers must not connect a contour jump.
func (m *ArcLength) Resample(count int) ([]LengthSample, error) {
	if count < 2 || count > MaxLengthSamples {
		return nil, fmt.Errorf("arc length sample count must be in [2,%d]", MaxLengthSamples)
	}
	if m == nil || len(m.contours) == 0 {
		return nil, ErrEmptyPath
	}
	out := make([]LengthSample, count)
	for i := range out {
		var err error
		out[i], err = m.AtFraction(float64(i) / float64(count-1))
		if err != nil {
			return nil, fmt.Errorf("arc length sample %d: %w", i, err)
		}
	}
	return out, nil
}
func (p Path) Resample(count int, tolerance float64) ([]LengthSample, error) {
	m, err := p.Measure(tolerance)
	if err != nil {
		return nil, err
	}
	return m.Resample(count)
}
func (c Contour) Resample(count int, tolerance float64) ([]LengthSample, error) {
	m, err := c.Measure(tolerance)
	if err != nil {
		return nil, err
	}
	return m.Resample(count)
}
func (c Cubic) Resample(count int, tolerance float64) ([]LengthSample, error) {
	return (Contour{Start: c.P0, Curves: []Cubic{c}}).Resample(count, tolerance)
}
