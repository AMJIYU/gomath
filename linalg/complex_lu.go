package linalg

import (
	"fmt"
	"math"
)

// ComplexLUOptions controls independent complex partial-pivot LU. Zero
// tolerances are literal; zero Limits fields select ComplexLimits defaults.
// RelativePivotTolerance compares pivot magnitude with the maximum original
// magnitude in that pivot row (scales follow row swaps). This is not a condition
// number/rank estimate. ReconstructionTolerance bounds ||P*A-L*U||F/||A||F.
// Both tolerances must be finite in [0,1).
type ComplexLUOptions struct {
	Limits                  ComplexLimits
	RelativePivotTolerance  float64
	ReconstructionTolerance float64
}

// DefaultComplexLUOptions selects default limits and 1e-14 pivot/1e-12 reconstruction policies.
func DefaultComplexLUOptions() ComplexLUOptions {
	return ComplexLUOptions{DefaultComplexLimits(), 1e-14, 1e-12}
}

// ComplexSingularError reports a rejected complex pivot, matching ErrSingular.
// The row magnitude is ScaleMantissa*2^ScaleExponent; its norm need not fit
// float64 even though every real and imaginary input component is finite.
type ComplexSingularError struct {
	Pivot                            int
	Value                            complex128
	ScaleMantissa                    float64
	ScaleExponent                    int
	Relative, RelativePivotTolerance float64
}

func (e *ComplexSingularError) Error() string {
	return fmt.Sprintf("linalg: complex singular pivot %d value %v, scale %g*2^%d, relative %g, tolerance %g", e.Pivot, e.Value, e.ScaleMantissa, e.ScaleExponent, e.Relative, e.RelativePivotTolerance)
}
func (e *ComplexSingularError) Unwrap() error { return ErrSingular }

// ComplexReconstructionError matches ErrReconstruction, without changing the
// existing real SVD/Schur diagnostic types or contracts.
type ComplexReconstructionError struct{ Relative, Limit float64 }

func (e *ComplexReconstructionError) Error() string {
	return fmt.Sprintf("linalg: complex LU reconstruction %g exceeds %g", e.Relative, e.Limit)
}
func (e *ComplexReconstructionError) Unwrap() error { return ErrReconstruction }

// ComplexSolveOptions controls each solve's resources and original-system
// normwise backward residual: ||A*x-b||2/(||A||F*||x||2+||b||2).
// RelativeResidualTolerance is finite in [0,1); zero requires exact zero product
// sum before norm rounding. Zero Limits fields select defaults, not factor limits.
type ComplexSolveOptions struct {
	Limits                    ComplexLimits
	RelativeResidualTolerance float64
}

// DefaultComplexSolveOptions selects default limits and a 1e-12 backward residual limit.
func DefaultComplexSolveOptions() ComplexSolveOptions {
	return ComplexSolveOptions{DefaultComplexLimits(), 1e-12}
}

// ComplexResidualError identifies the first rejected RHS, matching ErrResidual.
// No partial solution or residual slice is returned.
type ComplexResidualError struct {
	RHS             int
	Relative, Limit float64
}

func (e *ComplexResidualError) Error() string {
	return fmt.Sprintf("linalg: complex RHS %d relative residual %g exceeds %g", e.RHS, e.Relative, e.Limit)
}
func (e *ComplexResidualError) Unwrap() error { return ErrResidual }

// ComplexLU owns independent immutable packed factors and original coefficients,
// with P*A≈L*U. Ordinary partial pivoting selects the largest complex modulus
// in each column, with first-row ties. It does not require Hermitian input.
// Zero/nil factors are invalid. Completed factors/solves are concurrent-safe.
type ComplexLU struct {
	n                int
	packed, original []complex128
	perm             []int
	parity           int
	reconstruction   float64
	ready            bool
}

// FactorizeComplex computes checked partial-pivot complex LU with default options.
func FactorizeComplex(a *ComplexDense) (*ComplexLU, error) {
	return FactorizeComplexWithOptions(a, DefaultComplexLUOptions())
}

// FactorizeComplexWithOptions checks resources, factors a copy and validates P*A-L*U.
// Every error returns nil; no partial factors escape.
func FactorizeComplexWithOptions(a *ComplexDense, opt ComplexLUOptions) (*ComplexLU, error) {
	if a == nil {
		return nil, ErrInvalid
	}
	if a.rows != a.cols {
		return nil, fmt.Errorf("%w: complex LU requires square input", ErrDimension)
	}
	if err := complexTolerance(opt.RelativePivotTolerance); err != nil {
		return nil, err
	}
	if err := complexTolerance(opt.ReconstructionTolerance); err != nil {
		return nil, err
	}
	l, err := opt.Limits.normalized()
	if err != nil {
		return nil, err
	}
	if _, err = l.shape(a.rows, a.cols); err != nil {
		return nil, err
	}
	n := a.rows
	size := uint64(n)
	if err = l.work(4*size*size*size + 16*size*size + 4*size); err != nil {
		return nil, err
	}
	if err = checkComplex(a.data, "complex LU input"); err != nil {
		return nil, err
	}
	f := &ComplexLU{n: n, packed: a.Data(), original: a.Data(), perm: make([]int, n), parity: 1}
	scales := make([]complexBinary, n)
	for i := range n {
		f.perm[i] = i
		for _, z := range a.data[i*n : (i+1)*n] {
			m := complexScalar(z).norm()
			if complexGreater(m, scales[i]) {
				scales[i] = m
			}
		}
	}
	for k := range n {
		row := k
		pivotNorm := complexScalar(f.packed[k*n+k]).norm()
		for i := k + 1; i < n; i++ {
			m := complexScalar(f.packed[i*n+k]).norm()
			if complexGreater(m, pivotNorm) {
				row, pivotNorm = i, m
			}
		}
		relative := complexRatio(pivotNorm, scales[row])
		if pivotNorm.frac == 0 || (opt.RelativePivotTolerance > 0 && relative <= opt.RelativePivotTolerance) {
			return nil, &ComplexSingularError{k, f.packed[row*n+k], scales[row].frac, scales[row].exp, relative, opt.RelativePivotTolerance}
		}
		if row != k {
			for j := range n {
				f.packed[k*n+j], f.packed[row*n+j] = f.packed[row*n+j], f.packed[k*n+j]
			}
			f.perm[k], f.perm[row] = f.perm[row], f.perm[k]
			scales[k], scales[row] = scales[row], scales[k]
			f.parity = -f.parity
		}
		pivot := f.packed[k*n+k]
		for i := k + 1; i < n; i++ {
			multiplier, e := complexDivide(f.packed[i*n+k], pivot)
			if e != nil {
				return nil, fmt.Errorf("complex LU multiplier (%d,%d): %w", i, k, e)
			}
			f.packed[i*n+k] = multiplier
			for j := k + 1; j < n; j++ {
				var sum complexExactSum
				sum.scalar(f.packed[i*n+j])
				sum.term(multiplier, f.packed[k*n+j], true)
				value, e := complexTo128(sum.value())
				if e != nil {
					return nil, fmt.Errorf("complex LU update (%d,%d): %w", i, j, e)
				}
				f.packed[i*n+j] = value
			}
		}
	}
	var norm, residual complexBinary
	for _, z := range f.original {
		norm = complexHypot(norm, complexScalar(z).norm())
	}
	for i := range n {
		for j := range n {
			var sum complexExactSum
			sum.scalar(f.original[f.perm[i]*n+j])
			for k := 0; k <= min(i, j); k++ {
				lower := complex(1, 0)
				if i != k {
					lower = f.packed[i*n+k]
				}
				sum.term(lower, f.packed[k*n+j], true)
			}
			residual = complexHypot(residual, sum.value().norm())
		}
	}
	f.reconstruction = complexRatio(residual, norm)
	if !finite(f.reconstruction) {
		return nil, fmt.Errorf("%w: complex LU reconstruction ratio", ErrRange)
	}
	if f.reconstruction > opt.ReconstructionTolerance {
		return nil, &ComplexReconstructionError{f.reconstruction, opt.ReconstructionTolerance}
	}
	f.ready = true
	return f, nil
}
func complexTolerance(tol float64) error {
	if !finite(tol) || tol < 0 || tol >= 1 {
		return fmt.Errorf("%w: complex tolerance needs [0,1)", ErrTolerance)
	}
	return nil
}
func complexGreater(a, b complexBinary) bool {
	if a.frac == 0 {
		return false
	}
	if b.frac == 0 {
		return true
	}
	return a.exp > b.exp || a.exp == b.exp && a.frac > b.frac
}

// Size returns the order; invalid factors return zero.
func (f *ComplexLU) Size() int {
	if f == nil || !f.ready {
		return 0
	}
	return f.n
}

// Pivots copies the final row permutation, or nil for invalid/empty factors.
func (f *ComplexLU) Pivots() []int {
	if f == nil || !f.ready {
		return nil
	}
	return append([]int(nil), f.perm...)
}

// RelativeReconstructionError returns the checked factor error, or zero if invalid.
func (f *ComplexLU) RelativeReconstructionError() float64 {
	if f == nil || !f.ready {
		return 0
	}
	return f.reconstruction
}

// Factors copies unit-lower L and upper U under default limits.
func (f *ComplexLU) Factors() (*ComplexDense, *ComplexDense, error) {
	return f.FactorsWithLimits(ComplexLimits{})
}

// FactorsWithLimits exports independent L/U after explicit resource checks.
func (f *ComplexLU) FactorsWithLimits(limits ComplexLimits) (*ComplexDense, *ComplexDense, error) {
	if f == nil || !f.ready {
		return nil, nil, ErrInvalid
	}
	l, err := limits.normalized()
	if err != nil {
		return nil, nil, err
	}
	count, err := l.shape(f.n, f.n)
	if err != nil {
		return nil, nil, err
	}
	if err = l.work(2 * uint64(count)); err != nil {
		return nil, nil, err
	}
	lower := &ComplexDense{f.n, f.n, make([]complex128, count)}
	upper := &ComplexDense{f.n, f.n, make([]complex128, count)}
	for i := range f.n {
		lower.data[i*f.n+i] = 1
		for j := range f.n {
			if i > j {
				lower.data[i*f.n+j] = f.packed[i*f.n+j]
			} else {
				upper.data[i*f.n+j] = f.packed[i*f.n+j]
			}
		}
	}
	return lower, upper, nil
}

// Solve returns one independent solution and its checked normwise backward residual.
func (f *ComplexLU) Solve(b []complex128) ([]complex128, float64, error) {
	return f.SolveWithOptions(b, DefaultComplexSolveOptions())
}

// SolveWithOptions applies explicit resources/residual tolerance; errors return nil,0.
func (f *ComplexLU) SolveWithOptions(b []complex128, opt ComplexSolveOptions) ([]complex128, float64, error) {
	if f == nil || !f.ready {
		return nil, 0, ErrInvalid
	}
	if len(b) != f.n {
		return nil, 0, ErrDimension
	}
	x, residuals, err := f.solveMatrix(&ComplexDense{f.n, 1, b}, opt, 0)
	if err != nil {
		return nil, 0, err
	}
	return x.data, residuals[0], nil
}

// SolveMatrix solves one RHS per column with a checked backward residual per column.
func (f *ComplexLU) SolveMatrix(b *ComplexDense) (*ComplexDense, []float64, error) {
	return f.SolveMatrixWithOptions(b, DefaultComplexSolveOptions())
}

// SolveMatrixWithOptions returns no columns/residuals if any RHS fails its policy.
func (f *ComplexLU) SolveMatrixWithOptions(b *ComplexDense, opt ComplexSolveOptions) (*ComplexDense, []float64, error) {
	return f.solveMatrix(b, opt, 0)
}
func (f *ComplexLU) solvePreflight(cols int, opt ComplexSolveOptions, extra uint64) (ComplexLimits, error) {
	if f == nil || !f.ready {
		return ComplexLimits{}, ErrInvalid
	}
	if err := complexTolerance(opt.RelativeResidualTolerance); err != nil {
		return ComplexLimits{}, err
	}
	l, err := opt.Limits.normalized()
	if err != nil {
		return l, err
	}
	for _, shape := range [][2]int{{f.n, f.n}, {f.n, cols}, {1, cols}} {
		if _, err = l.shape(shape[0], shape[1]); err != nil {
			return l, err
		}
	}
	n, r := uint64(f.n), uint64(cols)
	return l, l.work(8*n*n*r + 16*n*r + 4*n*n + 4*r + extra)
}
func (f *ComplexLU) solveMatrix(b *ComplexDense, opt ComplexSolveOptions, extra uint64) (*ComplexDense, []float64, error) {
	if f == nil || !f.ready || b == nil {
		return nil, nil, ErrInvalid
	}
	if b.rows != f.n {
		return nil, nil, ErrDimension
	}
	if _, err := f.solvePreflight(b.cols, opt, extra); err != nil {
		return nil, nil, err
	}
	if err := checkComplex(b.data, "complex solve RHS"); err != nil {
		return nil, nil, err
	}
	n, r := f.n, b.cols
	x := &ComplexDense{n, r, make([]complex128, n*r)}
	residuals := make([]float64, r)
	for i, p := range f.perm {
		copy(x.data[i*r:(i+1)*r], b.data[p*r:(p+1)*r])
	}
	if r == 0 {
		return x, residuals, nil
	}
	for i := range n {
		for c := range r {
			var sum complexExactSum
			sum.scalar(x.data[i*r+c])
			for j := 0; j < i; j++ {
				sum.term(f.packed[i*n+j], x.data[j*r+c], true)
			}
			value, err := complexTo128(sum.value())
			if err != nil {
				return nil, nil, fmt.Errorf("complex forward substitution (%d,%d): %w", i, c, err)
			}
			x.data[i*r+c] = value
		}
	}
	for i := n - 1; i >= 0; i-- {
		for c := range r {
			var sum complexExactSum
			sum.scalar(x.data[i*r+c])
			for j := i + 1; j < n; j++ {
				sum.term(f.packed[i*n+j], x.data[j*r+c], true)
			}
			value, err := complexTo128(sum.value().div(complexScalar(f.packed[i*n+i])))
			if err != nil {
				return nil, nil, fmt.Errorf("complex back substitution (%d,%d): %w", i, c, err)
			}
			x.data[i*r+c] = value
		}
	}
	var anorm complexBinary
	for _, z := range f.original {
		anorm = complexHypot(anorm, complexScalar(z).norm())
	}
	for c := range r {
		var bnorm, xnorm, residual complexBinary
		for i := range n {
			bnorm = complexHypot(bnorm, complexScalar(b.data[i*r+c]).norm())
			xnorm = complexHypot(xnorm, complexScalar(x.data[i*r+c]).norm())
			var sum complexExactSum
			sum.scalar(-b.data[i*r+c])
			for j := range n {
				sum.term(f.original[i*n+j], x.data[j*r+c], false)
			}
			residual = complexHypot(residual, sum.value().norm())
		}
		relative := complexRatio(residual, complexAddReal(complexMulReal(anorm, xnorm), bnorm))
		if !finite(relative) {
			return nil, nil, fmt.Errorf("%w: complex RHS %d residual", ErrRange, c)
		}
		if relative > opt.RelativeResidualTolerance {
			return nil, nil, &ComplexResidualError{c, relative, opt.RelativeResidualTolerance}
		}
		residuals[c] = relative
	}
	return x, residuals, nil
}

// LogDet returns unit complex determinant phase and log(abs(det)). It avoids
// constructing the determinant magnitude, even when that magnitude cannot fit
// float64. The empty determinant gives (1,0). Component range failures give
// zero scalars with ErrRange; no claim of relative phase accuracy is made near
// cancellation. This is not the multivalued complex logarithm of a determinant.
func (f *ComplexLU) LogDet() (phase complex128, logAbs float64, err error) {
	if f == nil || !f.ready {
		return 0, 0, ErrInvalid
	}
	phase = complex(float64(f.parity), 0)
	correction := 0.0
	for i := range f.n {
		pivot := complexScalar(f.packed[i*f.n+i])
		norm := pivot.norm()
		unit, e := complexTo128(complexWide{complexDivReal(pivot.re, norm), complexDivReal(pivot.im, norm)})
		if e != nil {
			return 0, 0, e
		}
		product := complexScalar(phase).mul(complexScalar(unit))
		normPhase := product.norm()
		phase, e = complexTo128(complexWide{complexDivReal(product.re, normPhase), complexDivReal(product.im, normPhase)})
		if e != nil {
			return 0, 0, e
		}
		term := math.Log(norm.frac) + float64(norm.exp)*math.Ln2
		y := term - correction
		next := logAbs + y
		correction = (next - logAbs) - y
		logAbs = next
	}
	return phase, logAbs, nil
}

// Inverse performs a checked identity solve; prefer Solve/SolveMatrix for applications.
func (f *ComplexLU) Inverse() (*ComplexDense, error) {
	return f.InverseWithOptions(DefaultComplexSolveOptions())
}

// InverseWithOptions includes identity setup in the explicit solve work budget.
func (f *ComplexLU) InverseWithOptions(opt ComplexSolveOptions) (*ComplexDense, error) {
	if f == nil || !f.ready {
		return nil, ErrInvalid
	}
	extra := uint64(f.n) * uint64(f.n)
	if _, err := f.solvePreflight(f.n, opt, extra); err != nil {
		return nil, err
	}
	identity := &ComplexDense{f.n, f.n, make([]complex128, f.n*f.n)}
	for i := range f.n {
		identity.data[i*f.n+i] = 1
	}
	result, _, err := f.solveMatrix(identity, opt, extra)
	return result, err
}
