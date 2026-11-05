package linalg

import (
	"errors"
	"fmt"
	"math"
)

// ErrSchurLimit matches Schur size or scalar-work budget exhaustion.
var ErrSchurLimit = errors.New("linalg: Schur resource limit exceeded")

// SchurOptions controls bounded real Schur decomposition. Unlike the default
// constructor, WithOptions treats zero iteration/tolerance fields literally.
// MaxIterations counts total implicit QR sweeps, not sweeps per eigenvalue.
// RelativeTolerance is the local deflation tolerance, not eigenvalue accuracy.
// ReconstructionTolerance bounds ||A-Q*T*Q^T||F/||A||F (zero A uses zero error).
// OrthogonalityTolerance bounds ||Q^T*Q-I||F. All tolerances are finite in [0,1).
// MaxSize=0 selects 256; allowed 1..1024. MaxWork=0 selects 256M scalar units.
// Work counts matrix dot/update terms conservatively, including final checks;
// details are in docs/LINALG.md. Size and the initial work reserve are checked
// before allocating factors; iterative work is charged before each transform.
type SchurOptions struct {
	MaxIterations           int
	RelativeTolerance       float64
	ReconstructionTolerance float64
	OrthogonalityTolerance  float64
	MaxSize                 int
	MaxWork                 uint64
}

// DefaultSchurOptions selects 4096 total QR iterations, an 8*epsilon local
// deflation tolerance, 1e-12 final error checks, order 256 and 256M work units.
func DefaultSchurOptions() SchurOptions {
	return SchurOptions{MaxIterations: 4096, RelativeTolerance: 8 * 0x1p-52,
		ReconstructionTolerance: 1e-12, OrthogonalityTolerance: 1e-12, MaxSize: 256, MaxWork: 256 << 20}
}

// SchurConvergenceError reports the remaining active block on QR exhaustion.
// It matches ErrConvergence; no partially converged factor is returned.
type SchurConvergenceError struct {
	Iterations, MaxIterations int
	ActiveStart, ActiveEnd    int // inclusive, zero-based
}

func (e *SchurConvergenceError) Error() string {
	return fmt.Sprintf("linalg: Schur failed to converge after %d/%d iterations, active block [%d,%d]", e.Iterations, e.MaxIterations, e.ActiveStart, e.ActiveEnd)
}
func (e *SchurConvergenceError) Unwrap() error { return ErrConvergence }

// SchurValidationError matches ErrReconstruction for a failed reconstruction or
// orthogonality check; neither case returns partial factors.
type SchurValidationError struct {
	Check             string
	ErrorValue, Limit float64
}

func (e *SchurValidationError) Error() string {
	return fmt.Sprintf("linalg: Schur %s error %g exceeds %g", e.Check, e.ErrorValue, e.Limit)
}
func (e *SchurValidationError) Unwrap() error { return ErrReconstruction }

// Schur owns real orthogonal Q and real upper quasi-triangular T, A≈Q*T*Q^T.
// Nonzero subdiagonals occur only in disjoint 2x2 conjugate-pair blocks; their
// two diagonals need not be equal. Real pairs are triangularized into 1x1 blocks.
// Q contains Schur vectors, NOT general eigenvectors. Values are in block order,
// without sorting, and each complex pair has its positive imaginary part first.
// Completed factors are immutable/concurrent-safe; their zero value is invalid.
type Schur struct {
	n, iterations                 int
	q, t                          []float64
	values                        []complex128
	reconstruction, orthogonality float64
	ready                         bool
}

// FactorizeSchur decomposes a square real matrix using DefaultSchurOptions.
func FactorizeSchur(a *Dense) (*Schur, error) {
	return FactorizeSchurWithOptions(a, DefaultSchurOptions())
}

// FactorizeSchurWithOptions uses power-of-two global scaling, Householder
// Hessenberg reduction and implicit double-shift QR with exceptional shifts.
// It accepts empty, singular, repeated/defective and nonsymmetric real matrices.
// Lost nonzero entries during scaling or unrepresentable outputs give ErrRange.
// No balancing, block ordering, eigenvector recovery or external runtime is used.
func FactorizeSchurWithOptions(a *Dense, opt SchurOptions) (*Schur, error) {
	if a == nil {
		return nil, ErrInvalid
	}
	if a.rows != a.cols {
		return nil, fmt.Errorf("%w: Schur needs square input", ErrDimension)
	}
	if opt.MaxIterations < 0 {
		return nil, fmt.Errorf("%w: negative Schur iterations", ErrBudget)
	}
	for _, tol := range []float64{opt.RelativeTolerance, opt.ReconstructionTolerance, opt.OrthogonalityTolerance} {
		if !finite(tol) || tol < 0 || tol >= 1 {
			return nil, fmt.Errorf("%w: Schur tolerance needs [0,1)", ErrTolerance)
		}
	}
	if opt.MaxSize == 0 {
		opt.MaxSize = 256
	}
	if opt.MaxWork == 0 {
		opt.MaxWork = 256 << 20
	}
	if opt.MaxSize < 1 || opt.MaxSize > 1024 {
		return nil, fmt.Errorf("%w: Schur MaxSize needs 1..1024", ErrBudget)
	}
	if a.rows > opt.MaxSize {
		return nil, fmt.Errorf("%w: order %d exceeds %d", ErrSchurLimit, a.rows, opt.MaxSize)
	}
	if err := checkFinite(a.data, "Schur input"); err != nil {
		return nil, err
	}
	n := a.rows
	budget := schurBudget{left: opt.MaxWork}
	// Reserve scans and three cubic products for complete a posteriori checks.
	if err := budget.charge(3*uint64(n)*uint64(n)*uint64(n) + 8*uint64(n)*uint64(n)); err != nil {
		return nil, err
	}
	q, _ := Identity(n)
	h := a.Data()
	scale := 0.0
	for _, x := range h {
		scale = math.Max(scale, math.Abs(x))
	}
	exponent := 0
	if scale != 0 {
		_, exponent = math.Frexp(scale)
	}
	for i, x := range h {
		h[i] = math.Ldexp(x, -exponent)
		if math.Ldexp(h[i], exponent) != x {
			return nil, fmt.Errorf("%w: Schur input scaling loses precision at entry %d", ErrRange, i)
		}
	}
	w := schurWorkspace{n: n, h: h, q: q.data, budget: &budget}
	v := make([]float64, n)
	for k := 0; k < n-2; k++ {
		for i := k + 1; i < n; i++ {
			v[i-k-1] = h[i*n+k]
		}
		vector := v[:n-k-1]
		alpha, err := w.transform(k+1, vector)
		if err != nil {
			return nil, err
		}
		h[(k+1)*n+k] = alpha
		for i := k + 2; i < n; i++ {
			h[i*n+k] = 0
		}
	}
	iterations, stalled := 0, 0
	for hi := n - 1; hi >= 0; {
		if err := budget.charge(uint64(n) + 1); err != nil {
			return nil, err
		}
		lo := hi
		for lo > 0 {
			if schurDeflate(h, n, lo, opt.RelativeTolerance) {
				h[lo*n+lo-1] = 0
				break
			}
			lo--
		}
		if lo == hi {
			hi--
			stalled = 0
			continue
		}
		if lo == hi-1 {
			if err := w.finishPair(lo); err != nil {
				return nil, err
			}
			hi -= 2
			stalled = 0
			continue
		}
		if iterations == opt.MaxIterations {
			return nil, &SchurConvergenceError{iterations, opt.MaxIterations, lo, hi}
		}
		if err := w.sweep(lo, hi, stalled); err != nil {
			return nil, err
		}
		iterations++
		stalled++
	}
	if err := checkFinite(h, "Schur iteration"); err != nil {
		return nil, err
	}
	if err := checkFinite(q.data, "Schur orthogonal factor"); err != nil {
		return nil, err
	}
	// Quantize to the actual returned T BEFORE checking reconstruction. Power
	// scaling is exact in normal range, but subnormal output can round. A check
	// of the unquantized internal factor would conceal this output error.
	for i, x := range h {
		output := math.Ldexp(x, exponent)
		if !finite(output) || x != 0 && output == 0 {
			return nil, fmt.Errorf("%w: Schur factor entry %d", ErrRange, i)
		}
		h[i] = math.Ldexp(output, -exponent)
	}
	reconstruction, orthogonality := schurChecks(a.data, h, q.data, n, exponent)
	if !finite(reconstruction) || !finite(orthogonality) {
		return nil, fmt.Errorf("%w: Schur validation", ErrRange)
	}
	if reconstruction > opt.ReconstructionTolerance {
		return nil, &SchurValidationError{"reconstruction", reconstruction, opt.ReconstructionTolerance}
	}
	if orthogonality > opt.OrthogonalityTolerance {
		return nil, &SchurValidationError{"orthogonality", orthogonality, opt.OrthogonalityTolerance}
	}
	values := make([]complex128, n)
	for i := 0; i < n; i++ {
		if i+1 < n && h[(i+1)*n+i] != 0 {
			re, im, realPair, err := schurPair(h[i*n+i], h[i*n+i+1], h[(i+1)*n+i], h[(i+1)*n+i+1])
			if err != nil {
				return nil, err
			}
			if realPair {
				return nil, fmt.Errorf("%w: inconsistent Schur block", ErrRange)
			}
			normalizedReal := re
			re, im = math.Ldexp(re, exponent), math.Ldexp(im, exponent)
			if !finite(re) || !finite(im) || im == 0 || re == 0 && normalizedReal != 0 {
				return nil, fmt.Errorf("%w: Schur complex value", ErrRange)
			}
			values[i], values[i+1] = complex(re, im), complex(re, -im)
			i++
		} else {
			value := math.Ldexp(h[i*n+i], exponent)
			if !finite(value) || value == 0 && h[i*n+i] != 0 {
				return nil, fmt.Errorf("%w: Schur real value", ErrRange)
			}
			values[i] = complex(value, 0)
		}
	}
	for i, x := range h {
		h[i] = math.Ldexp(x, exponent)
		if !finite(h[i]) || x != 0 && h[i] == 0 {
			return nil, fmt.Errorf("%w: Schur factor entry %d", ErrRange, i)
		}
	}
	return &Schur{n: n, iterations: iterations, q: q.data, t: h, values: values,
		reconstruction: reconstruction, orthogonality: orthogonality, ready: true}, nil
}

type schurBudget struct{ left uint64 }

func (b *schurBudget) charge(amount uint64) error {
	if amount > b.left {
		return fmt.Errorf("%w: scalar-work budget", ErrSchurLimit)
	}
	b.left -= amount
	return nil
}

type schurWorkspace struct {
	n      int
	h, q   []float64
	budget *schurBudget
}

// schurReflector replaces x with a unit Householder vector; returns H*x's
// original leading value. Hypot and scaled normalization handle tiny vectors.
func schurReflector(x []float64) (alpha float64, active bool) {
	norm := 0.0
	for _, y := range x {
		norm = math.Hypot(norm, y)
	}
	if norm == 0 {
		return 0, false
	}
	alpha = -math.Copysign(norm, x[0])
	// Divide first, then subtract the unit signed norm to avoid doubling a
	// subnormal (or overflowing an otherwise finite large Householder vector).
	for i := range x {
		x[i] /= norm
	}
	x[0] += math.Copysign(1, x[0])
	norm = 0
	for _, y := range x {
		norm = math.Hypot(norm, y)
	}
	for i := range x {
		x[i] /= norm
	}
	return alpha, true
}

func (w *schurWorkspace) transform(start int, vector []float64) (float64, error) {
	// A vector with a zero leading entry and a single nonzero tail entry
	// admits an exact signed permutation. Avoid approximating sqrt(1/2):
	// strict nilpotent/Jordan inputs otherwise acquire artificial small roots.
	axis := -1
	for j, x := range vector {
		if x != 0 {
			if axis != -1 {
				axis = -2
				break
			}
			axis = j
		}
	}
	if axis > 0 {
		if err := w.budget.charge(uint64(6*w.n*len(vector) + 8*len(vector))); err != nil {
			return 0, err
		}
		n, end := w.n, start+axis
		sign := -math.Copysign(1, vector[0]) * math.Copysign(1, vector[axis])
		alpha := sign * vector[axis]
		for j := range n {
			a, b := w.h[start*n+j], w.h[end*n+j]
			w.h[start*n+j], w.h[end*n+j] = sign*b, sign*a
		}
		for i := range n {
			for _, data := range [][]float64{w.h, w.q} {
				a, b := data[i*n+start], data[i*n+end]
				data[i*n+start], data[i*n+end] = sign*b, sign*a
			}
		}
		return alpha, nil
	}
	alpha, active := schurReflector(vector)
	if !active {
		return 0, nil
	}
	return alpha, w.reflect(start, vector)
}

func (w *schurWorkspace) reflect(start int, v []float64) error {
	if err := w.budget.charge(uint64(6*w.n*len(v) + 8*len(v))); err != nil {
		return err
	}
	n := w.n
	for j := range n {
		dot := 0.0
		for k, x := range v {
			dot = math.FMA(x, w.h[(start+k)*n+j], dot)
		}
		for k, x := range v {
			at := (start+k)*n + j
			w.h[at] = math.FMA(-2*x, dot, w.h[at])
		}
	}
	for i := range n {
		for _, data := range [][]float64{w.h, w.q} {
			dot := 0.0
			for k, x := range v {
				dot = math.FMA(data[i*n+start+k], x, dot)
			}
			for k, x := range v {
				at := i*n + start + k
				data[at] = math.FMA(-2*x, dot, data[at])
			}
		}
	}
	return nil
}

// Deflation uses a relative small-subdiagonal test followed by a conservative
// product test (Ahues/Tisseur's criterion). No absolute floor discards a tiny
// conjugate pair just because a different block has a much larger scale.
func schurDeflate(h []float64, n, k int, tol float64) bool {
	sub := math.Abs(h[k*n+k-1])
	if sub == 0 {
		return true
	}
	if tol == 0 {
		return false
	}
	a, d := h[(k-1)*n+k-1], h[k*n+k]
	s := math.Abs(a) + math.Abs(d)
	if s == 0 {
		s = math.Abs(h[(k-1)*n+k])
	}
	if sub > tol*s {
		return false
	}
	ab := math.Max(sub, math.Abs(h[(k-1)*n+k]))
	ba := math.Min(sub, math.Abs(h[(k-1)*n+k]))
	aa := math.Max(math.Abs(d), math.Abs(a-d))
	bb := math.Min(math.Abs(d), math.Abs(a-d))
	// Cancel the common positive denominator aa+ab, then compare scaled
	// mantissas/exponents. Directly multiplying these values can underflow
	// both sides to zero and silently erase a meaningful tiny complex pair.
	if ba == 0 || ab == 0 {
		return true
	}
	if bb == 0 || aa == 0 {
		return false
	}
	lm, le := math.Frexp(ba)
	m, e := math.Frexp(ab)
	lm, adjust := math.Frexp(lm * m)
	le += e + adjust
	rm, re := math.Frexp(bb)
	m, e = math.Frexp(aa)
	rm *= m
	re += e
	m, e = math.Frexp(tol)
	rm *= m
	re += e
	rm, adjust = math.Frexp(rm)
	re += adjust
	return le < re || le == re && lm <= rm
}

func (w *schurWorkspace) sweep(lo, hi, stalled int) error {
	n, h := w.n, w.h
	local := [...]float64{h[(hi-1)*n+hi-1], h[(hi-1)*n+hi], h[hi*n+hi-1], h[hi*n+hi], h[lo*n+lo], h[(lo+1)*n+lo], h[(lo+1)*n+lo+1], h[lo*n+lo+1], h[(lo+2)*n+lo+1], h[(hi-1)*n+hi-2]}
	if _, err := schurScale(local[:]); err != nil {
		return err
	}
	a, b, c, d, u, v, z := local[0], local[1], local[2], local[3], local[4], local[5], local[6]
	trace, determinant := a+d, math.FMA(a, d, -b*c)
	var initial [3]float64
	initial[0] = math.FMA(u, u, math.FMA(local[7], v, determinant)) - trace*u
	initial[1] = v * (u + z - trace)
	initial[2] = v * local[8]
	width := 3
	if stalled > 0 && stalled%20 == 0 || initial == [3]float64{} {
		// A real exceptional shift breaks periodic double-shift cycles.
		shift := d
		if stalled > 0 {
			shift += .75 * (math.Abs(c) + math.Abs(local[9]))
		}
		initial = [3]float64{u - shift, v, 0}
		width = 2
	}
	for k := lo; k < hi; k++ {
		length := min(width, hi-k+1)
		vector := initial[:length]
		if k != lo {
			for j := range vector {
				vector[j] = h[(k+j)*n+k-1]
			}
		}
		alpha, err := w.transform(k, vector)
		if err != nil {
			return err
		}
		if k != lo {
			h[k*n+k-1] = alpha
			for j := 1; j < length; j++ {
				h[(k+j)*n+k-1] = 0
			}
		}
	}
	return nil
}

// schurPair computes center and discriminant magnitude without squaring tiny
// off-diagonals. realPair distinguishes real roots (center±magnitude) from
// center±i*magnitude. The block is locally scaled before this computation.
func schurPair(a, b, c, d float64) (center, magnitude float64, realPair bool, err error) {
	local := [...]float64{a, b, c, d}
	exponent, err := schurScale(local[:])
	if err != nil {
		return 0, 0, false, err
	}
	a, b, c, d = local[0], local[1], local[2], local[3]
	center = math.Ldexp(a/2+d/2, exponent)
	delta := a/2 - d/2
	cross := math.Sqrt(math.Abs(b)) * math.Sqrt(math.Abs(c))
	if b == 0 || c == 0 || math.Signbit(b) == math.Signbit(c) {
		return center, math.Ldexp(math.Hypot(delta, cross), exponent), true, nil
	}
	// Compensate the off-diagonal product before testing the discriminant:
	// sqrt(b)*sqrt(c) alone can misclassify a pair near a repeated real root.
	product := b * c
	if product != 0 || delta*delta != 0 {
		discriminant := math.FMA(delta, delta, product) + math.FMA(b, c, -product)
		return center, math.Ldexp(math.Sqrt(math.Abs(discriminant)), exponent), discriminant >= 0, nil
	}
	realPair = math.Abs(delta) >= cross
	magnitude = math.Ldexp(math.Sqrt(math.Abs(math.Abs(delta)-cross))*math.Sqrt(math.Abs(delta)+cross), exponent)
	return center, magnitude, realPair, nil
}

func schurScale(values []float64) (int, error) {
	scale := 0.0
	for _, x := range values {
		scale = math.Max(scale, math.Abs(x))
	}
	exponent := 0
	if scale != 0 {
		_, exponent = math.Frexp(scale)
	}
	for i, x := range values {
		values[i] = math.Ldexp(x, -exponent)
		if math.Ldexp(values[i], exponent) != x {
			return 0, fmt.Errorf("%w: Schur local scaling loses precision", ErrRange)
		}
	}
	return exponent, nil
}

func (w *schurWorkspace) finishPair(k int) error {
	n, h := w.n, w.h
	a, b, c, d := h[k*n+k], h[k*n+k+1], h[(k+1)*n+k], h[(k+1)*n+k+1]
	center, magnitude, realPair, err := schurPair(a, b, c, d)
	if err != nil {
		return err
	}
	if !realPair {
		return nil
	}
	// Choose the eigenvalue nearest a; choose the more accurate of two
	// algebraically equivalent eigenvector formulas, then normalize by Hypot.
	lambda := center + math.Copysign(magnitude, a-d)
	x, y := b, lambda-a
	if math.Hypot(lambda-d, c) > math.Hypot(x, y) {
		x, y = lambda-d, c
	}
	if x == 0 && y == 0 {
		return nil
	}
	norm := math.Hypot(x, y)
	x, y = x/norm, y/norm
	if err := w.budget.charge(uint64(12*n + 32)); err != nil {
		return err
	}
	// G=[x,-y;y,x]; similarity G^T*H*G and accumulation Q*G.
	for j := range n {
		u, v := h[k*n+j], h[(k+1)*n+j]
		h[k*n+j], h[(k+1)*n+j] = math.FMA(x, u, y*v), math.FMA(-y, u, x*v)
	}
	for i := range n {
		for _, data := range [][]float64{h, w.q} {
			u, v := data[i*n+k], data[i*n+k+1]
			data[i*n+k], data[i*n+k+1] = math.FMA(x, u, y*v), math.FMA(-y, u, x*v)
		}
	}
	h[(k+1)*n+k] = 0
	return nil
}

func schurChecks(original, h, q []float64, n, exponent int) (relative, orthogonal float64) {
	product := make([]float64, n*n)
	for i := range n {
		for j := range n {
			for k := range n {
				product[i*n+j] = math.FMA(q[i*n+k], h[k*n+j], product[i*n+j])
			}
		}
	}
	norm, residual := 0.0, 0.0
	for i := range n {
		for j := range n {
			value, dot := 0.0, 0.0
			for k := range n {
				value = math.FMA(product[i*n+k], q[j*n+k], value)
				dot = math.FMA(q[k*n+i], q[k*n+j], dot)
			}
			if i == j {
				dot--
			}
			orthogonal = math.Hypot(orthogonal, dot)
			entry := math.Ldexp(original[i*n+j], -exponent)
			norm = math.Hypot(norm, entry)
			residual = math.Hypot(residual, value-entry)
		}
	}
	if norm != 0 {
		relative = residual / norm
	}
	return relative, orthogonal
}

// Size returns the input order, or zero for nil/uninitialized factors.
func (f *Schur) Size() int {
	if f == nil || !f.ready {
		return 0
	}
	return f.n
}

// Iterations returns total implicit QR sweeps (excluding direct 2x2 rotations).
func (f *Schur) Iterations() int {
	if f == nil || !f.ready {
		return 0
	}
	return f.iterations
}

// RelativeReconstructionError reports the checked relative Frobenius residual
// of the actual returned T, including any subnormal output rounding.
func (f *Schur) RelativeReconstructionError() float64 {
	if f == nil || !f.ready {
		return 0
	}
	return f.reconstruction
}

// OrthogonalityError returns the checked ||Q^T*Q-I||F (zero for invalid factors).
func (f *Schur) OrthogonalityError() float64 {
	if f == nil || !f.ready {
		return 0
	}
	return f.orthogonality
}

// Values copies eigenvalues in Schur block order, or returns nil if invalid.
// Tiny or ill-conditioned eigenvalues have no promised relative forward error.
func (f *Schur) Values() []complex128 {
	if f == nil || !f.ready {
		return nil
	}
	return append([]complex128(nil), f.values...)
}

// Factors copies Q and T into independent Dense matrices. No returned matrix
// shares backing storage with the input, another export, or the factorization.
func (f *Schur) Factors() (q, t *Dense, err error) {
	if f == nil || !f.ready {
		return nil, nil, ErrInvalid
	}
	return &Dense{rows: f.n, cols: f.n, data: append([]float64(nil), f.q...)}, &Dense{rows: f.n, cols: f.n, data: append([]float64(nil), f.t...)}, nil
}
