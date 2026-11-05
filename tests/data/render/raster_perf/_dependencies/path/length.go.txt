package path

import (
	"errors"
	"fmt"
	"gomath/geom"
	"math"
)

// DefaultLengthTolerance is an absolute error budget in path units. Length
// APIs require an explicit finite, positive tolerance; zero is not a default.
const DefaultLengthTolerance = 1e-6

// MaxLengthSamples bounds the allocation requested by Resample.
const MaxLengthSamples = 1 << 20

var (
	// ErrLengthConvergence means the requested absolute accuracy could not be
	// reached within float64 precision or the bounded numerical work budget.
	ErrLengthConvergence = errors.New("arc length did not converge within tolerance")
	ErrEmptyPath         = errors.New("cannot sample an empty path")
)

const (
	maxLengthDepth     = 48
	maxLengthIntervals = 4096
	maxInverseSteps    = 64
	lengthEpsilon      = 0x1p-52
)

// LengthSample identifies a point at an absolute distance along a path. Curve
// is the authored cubic index; an implicit closing line uses len(Curves).
// A contour with no curves uses Curve=-1. Parameter is the cubic parameter.
// Tangent is a unit direction, or zero at a stationary interior point or on a
// constant curve. Stationary endpoints use their one-sided limiting direction.
type LengthSample struct {
	Point, Tangent      geom.Vec3
	Distance, Parameter float64
	Contour, Curve      int
}

type lengthCubic struct {
	curve    Cubic
	velocity [3]geom.Vec3
	cuts     [11]float64 // Endpoints, derivative component roots and extrema.
	ncuts    int
}

func lengthFinite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
func lengthNorm(v geom.Vec3) float64 {
	return math.Hypot(math.Hypot(v.X, v.Y), v.Z)
}
func checkLengthTolerance(tolerance float64) error {
	if !lengthFinite(tolerance) || tolerance <= 0 {
		return fmt.Errorf("arc length tolerance must be finite and positive")
	}
	return nil
}
func checkLengthRange(start, end, total float64) error {
	if !lengthFinite(start) || !lengthFinite(end) || start < 0 || end < start || end > total {
		return fmt.Errorf("arc length interval [%g,%g] must be ordered and within [0,%g]", start, end, total)
	}
	return nil
}
func lengthBudget(tolerance, divisor float64) (float64, error) {
	budget := tolerance / divisor
	if budget == 0 {
		return 0, fmt.Errorf("%w: tolerance budget underflow", ErrLengthConvergence)
	}
	return budget, nil
}

func prepareLengthCubic(c Cubic) (lengthCubic, error) {
	if !c.P0.Finite() || !c.P1.Finite() || !c.P2.Finite() || !c.P3.Finite() {
		return lengthCubic{}, fmt.Errorf("arc length cubic has non-finite coordinates")
	}
	q := lengthCubic{curve: c, velocity: [3]geom.Vec3{
		c.P1.Sub(c.P0).Mul(3), c.P2.Sub(c.P1).Mul(3), c.P3.Sub(c.P2).Mul(3),
	}}
	for _, v := range q.velocity {
		if !v.Finite() || !lengthFinite(lengthNorm(v)) {
			return lengthCubic{}, fmt.Errorf("arc length derivative exceeds finite float64 range")
		}
	}
	q.cuts[0], q.cuts[1], q.ncuts = 0, 1, 2
	// Split at derivative component roots/extrema before adaptive sampling.
	// Otherwise both embedded rules can miss the same tiny sign-changing
	// sliver near an endpoint of a collinear, retracing cubic.
	for axis := range 3 {
		coord := func(v geom.Vec3) float64 {
			switch axis {
			case 0:
				return v.X
			case 1:
				return v.Y
			default:
				return v.Z
			}
		}
		a, b, d := coord(q.velocity[0]), coord(q.velocity[1]), coord(q.velocity[2])
		scale := math.Max(math.Abs(a), math.Max(math.Abs(b), math.Abs(d)))
		if scale == 0 {
			continue
		}
		a, b, d = a/scale, b/scale, d/scale
		A, B, C := a-2*b+d, 2*(b-a), a
		if A == 0 {
			if B != 0 {
				q.addCut(-C / B)
			}
			continue
		}
		q.addCut(-B / (2 * A))
		discriminant := math.FMA(B, B, -4*A*C)
		if discriminant < 0 {
			continue
		}
		root := math.Sqrt(discriminant)
		if root == 0 {
			q.addCut(-B / (2 * A))
			continue
		}
		r := -.5 * (B + math.Copysign(root, B))
		q.addCut(r / A)
		q.addCut(C / r)
	}
	return q, nil
}
func (c *lengthCubic) addCut(t float64) {
	if !lengthFinite(t) || t <= 0 || t >= 1 {
		return
	}
	i := 0
	for i < c.ncuts && c.cuts[i] < t {
		i++
	}
	if i < c.ncuts && c.cuts[i] == t {
		return
	}
	copy(c.cuts[i+1:c.ncuts+1], c.cuts[i:c.ncuts])
	c.cuts[i] = t
	c.ncuts++
}
func (c lengthCubic) derivative(t float64) geom.Vec3 {
	return c.velocity[0].Lerp(c.velocity[1], t).Lerp(c.velocity[1].Lerp(c.velocity[2], t), t)
}
func (c lengthCubic) speed(t float64) float64 { return lengthNorm(c.derivative(t)) }
func (c lengthCubic) tangent(t float64) geom.Vec3 {
	v := c.derivative(t)
	if lengthNorm(v) == 0 && (t == 0 || t == 1) {
		for j := range 3 {
			i := j
			if t == 1 {
				i = 2 - j
			}
			if lengthNorm(c.velocity[i]) != 0 {
				v = c.velocity[i]
				break
			}
		}
	}
	if n := lengthNorm(v); n > 0 {
		return geom.Vec3{X: v.X / n, Y: v.Y / n, Z: v.Z / n}
	}
	return geom.Vec3{}
}

// quadrature15 uses an embedded Gauss 7 / Kronrod 15 pair. The variation-based
// error estimate and roundoff floor prevent a coincidental small difference
// between the two rules from being treated as arbitrary precision.
func (c lengthCubic) quadrature15(a, b float64) (value, estimate float64) {
	x := [...]float64{.9914553711208126392, .9491079123427585245, .8648644233597690728, .7415311855993944399, .5860872354676911303, .4058451513773971669, .2077849550078984676}
	wk := [...]float64{.02293532201052922496, .06309209262997855329, .10479001032225018384, .14065325971552591875, .169004726639267902826, .19035057806478540991, .20443294007529889241, .209482141084727828}
	wg := [...]float64{.12948496616886969327, .2797053914892766679, .38183005050511894495, .41795918367346938775}
	center, half := (a+b)/2, (b-a)/2
	fc := c.speed(center)
	// Accumulate half the usual rule so a finite speed near MaxFloat64 does
	// not overflow an intermediate sum over the reference interval [-1,1].
	k, g := wk[7]*(fc/2), wg[3]*(fc/2)
	var left, right [7]float64
	for i := range x {
		left[i], right[i] = c.speed(center-half*x[i]), c.speed(center+half*x[i])
		pair := left[i]/2 + right[i]/2
		k += wk[i] * pair
		if i%2 == 1 {
			g += wg[i/2] * pair
		}
	}
	variation := wk[7] * (math.Abs(fc-k) / 2)
	for i := range x {
		variation += wk[i] * (math.Abs(left[i]-k)/2 + math.Abs(right[i]-k)/2)
	}
	width := b - a
	value = width * k
	estimate = width * math.Abs(k-g)
	variation *= width
	if variation > 0 && estimate > 0 {
		estimate = variation * math.Min(1, math.Pow(200*(estimate/variation), 1.5))
	}
	estimate = math.Max(estimate, (50*lengthEpsilon)*math.Abs(value))
	return value, estimate
}

// integral is iterative: both subdivision depth and interval evaluations have
// hard bounds. Exhaustion returns an error instead of an unchecked estimate.
func (c lengthCubic) integral(start, end, tolerance float64) (float64, error) {
	return c.integralRecorded(start, end, tolerance, nil)
}

func (c lengthCubic) integralRecorded(start, end, tolerance float64, lookup *lengthLookupBuilder) (float64, error) {
	if start == end {
		return 0, nil
	}
	if tolerance <= 0 {
		return 0, fmt.Errorf("%w: tolerance budget underflow", ErrLengthConvergence)
	}
	pieces := 1
	for i := 1; i < c.ncuts-1; i++ {
		if c.cuts[i] > start && c.cuts[i] < end {
			pieces++
		}
	}
	budget, err := lengthBudget(tolerance, float64(pieces))
	if err != nil {
		return 0, err
	}
	left, sum, compensation, evaluations := start, 0.0, 0.0, 0
	for i := 1; i <= c.ncuts; i++ {
		right := end
		if i < c.ncuts && c.cuts[i] < end {
			right = c.cuts[i]
		}
		if right <= left {
			continue
		}
		if lookup != nil {
			lookup.prefix = sum
		}
		value, err := c.integralInterval(left, right, budget, &evaluations, lookup)
		if err != nil {
			return 0, err
		}
		y := value - compensation
		next := sum + y
		compensation, sum = (next-sum)-y, next
		if lookup != nil {
			lookup.correctEnd(sum)
		}
		if !lengthFinite(sum) {
			return 0, fmt.Errorf("arc length integral exceeds finite float64 range")
		}
		left = right
		if left == end {
			break
		}
	}
	return sum, nil
}

func (c lengthCubic) integralInterval(start, end, tolerance float64, evaluations *int, lookup *lengthLookupBuilder) (float64, error) {
	type interval struct {
		a, b, tolerance float64
		depth           int
	}
	var stack [maxLengthDepth + 1]interval
	stack[0] = interval{a: start, b: end, tolerance: tolerance}
	n := 1
	var sum, compensation float64
	for n > 0 {
		n--
		part := stack[n]
		if *evaluations >= 2*maxLengthIntervals-1 {
			return 0, fmt.Errorf("%w: quadrature work limit", ErrLengthConvergence)
		}
		value, estimate := c.quadrature15(part.a, part.b)
		*evaluations++
		if !lengthFinite(value) || !lengthFinite(estimate) {
			return 0, fmt.Errorf("arc length integral exceeds finite float64 range")
		}
		if estimate <= part.tolerance {
			// Kahan summation limits drift when an integral needs many leaves.
			y := value - compensation
			t := sum + y
			compensation, sum = (t-sum)-y, t
			if lookup != nil {
				lookup.add(part.b, lookup.prefix+sum)
			}
			continue
		}
		mid := (part.a + part.b) / 2
		if part.depth >= maxLengthDepth || mid == part.a || mid == part.b || part.tolerance/2 == 0 {
			return 0, fmt.Errorf("%w: quadrature work or precision limit", ErrLengthConvergence)
		}
		stack[n] = interval{a: mid, b: part.b, tolerance: part.tolerance / 2, depth: part.depth + 1}
		stack[n+1] = interval{a: part.a, b: mid, tolerance: part.tolerance / 2, depth: part.depth + 1}
		n += 2
	}
	if !lengthFinite(sum) {
		return 0, fmt.Errorf("arc length integral exceeds finite float64 range")
	}
	return sum, nil
}

// Length returns the cubic's arc length with an absolute tolerance in path
// units, using adaptive quadrature of the analytic derivative's speed.
func (c Cubic) Length(tolerance float64) (float64, error) {
	return c.LengthBetween(0, 1, tolerance)
}

// LengthBetween measures a parameter interval, where 0 <= start <= end <= 1.
func (c Cubic) LengthBetween(start, end, tolerance float64) (float64, error) {
	if err := checkLengthTolerance(tolerance); err != nil {
		return 0, err
	}
	if err := checkLengthRange(start, end, 1); err != nil {
		return 0, err
	}
	q, err := prepareLengthCubic(c)
	if err != nil {
		return 0, err
	}
	budget, err := lengthBudget(tolerance, 4)
	if err != nil {
		return 0, err
	}
	return q.integral(start, end, budget)
}

func (c lengthCubic) parameterAtLength(distance, total, tolerance float64) (float64, error) {
	if err := checkLengthRange(distance, distance, total); err != nil {
		return 0, err
	}
	if distance == 0 {
		return 0, nil
	}
	if distance == total {
		return 1, nil
	}
	return c.parameterInLengthInterval(distance, tolerance, 0, 1, 0, total)
}

// The interval endpoints and prefix come from accepted adaptive quadrature
// leaves. Only the remaining local prefix is integrated during each iteration;
// cached interpolation provides a bracket/initial guess, never the final answer.
func (c lengthCubic) parameterInLengthInterval(distance, tolerance, first, last, prefix, endLength float64) (float64, error) {
	if distance == prefix {
		return first, nil
	}
	if distance == endLength {
		return last, nil
	}
	budget, err := lengthBudget(tolerance, 16)
	if err != nil {
		return 0, err
	}
	local := distance - prefix
	lo, hi := first, last
	t := first + (last-first)*(local/(endLength-prefix))
	for range maxInverseSteps {
		length, err := c.integral(first, t, budget)
		if err != nil {
			return 0, err
		}
		residual := length - local
		if math.Abs(residual) <= tolerance/4 {
			return t, nil
		}
		if residual < 0 {
			lo = t
		} else {
			hi = t
		}
		next := t - residual/c.speed(t)
		if !lengthFinite(next) || next <= lo || next >= hi {
			next = lo + (hi-lo)/2
		}
		if next == t {
			break
		}
		t = next
	}
	return 0, fmt.Errorf("%w: inverse iteration or precision limit", ErrLengthConvergence)
}

// AtLength returns point, unit tangent and cubic parameter at an absolute
// distance. Constant cubics accept only distance zero and return a zero tangent.
func (c Cubic) AtLength(distance, tolerance float64) (LengthSample, error) {
	if err := checkLengthTolerance(tolerance); err != nil {
		return LengthSample{}, err
	}
	q, err := prepareLengthCubic(c)
	if err != nil {
		return LengthSample{}, err
	}
	budget, err := lengthBudget(tolerance, 4)
	if err != nil {
		return LengthSample{}, err
	}
	total, err := q.integral(0, 1, budget)
	if err != nil {
		return LengthSample{}, err
	}
	t, err := q.parameterAtLength(distance, total, tolerance)
	if err != nil {
		return LengthSample{}, err
	}
	return q.sample(distance, t, 0, 0)
}

// ParameterAtLength inverts arc length without assuming that cubic parameter
// advances at constant speed.
func (c Cubic) ParameterAtLength(distance, tolerance float64) (float64, error) {
	s, err := c.AtLength(distance, tolerance)
	return s.Parameter, err
}

func (c lengthCubic) sample(distance, t float64, contour, curve int) (LengthSample, error) {
	p := c.curve.At(t)
	if !p.Finite() {
		return LengthSample{}, fmt.Errorf("arc length point exceeds finite float64 range")
	}
	return LengthSample{Point: p, Tangent: c.tangent(t), Distance: distance, Parameter: t, Contour: contour, Curve: curve}, nil
}
