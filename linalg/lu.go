package linalg

import (
	"fmt"
	"math"
)

// LUOptions specifies when a finite pivot is considered numerically singular.
// Use DefaultLUOptions for the default policy; the zero value intentionally
// selects exact-zero checking, not an implicit default.
type LUOptions struct {
	// RelativePivotTolerance must be finite and in [0,1). A pivot is rejected
	// when abs(pivot)/max(abs(original pivot row)) <= this tolerance.
	// Zero rejects only exactly zero pivots. Row scales follow row swaps and
	// are not recomputed from the evolving factors. This is a pivot policy,
	// not a rank or condition-number estimate; see docs/LINALG.md.
	RelativePivotTolerance float64
}

// DefaultLUOptions returns a relative pivot tolerance of 1e-14.
func DefaultLUOptions() LUOptions { return LUOptions{RelativePivotTolerance: 1e-14} }

// SingularError identifies the rejected zero-based elimination step and policy.
// Value is the signed pivot; Scale is its original row's maximum absolute entry.
// It matches ErrSingular through errors.Is.
type SingularError struct {
	Pivot                  int
	Value, Scale           float64
	RelativePivotTolerance float64
}

func (e *SingularError) Error() string {
	return fmt.Sprintf("%v: pivot %d value %g, row scale %g, tolerance %g", ErrSingular, e.Pivot, e.Value, e.Scale, e.RelativePivotTolerance)
}

func (e *SingularError) Unwrap() error { return ErrSingular }

// LU owns immutable packed factors and the final row permutation. It can be
// reused concurrently; solves allocate their own workspace. Its zero value is
// invalid. Factorize accepts square matrices only, including the empty matrix.
type LU struct {
	n      int
	data   []float64
	perm   []int
	parity int
	ready  bool
}

// Factorize computes P*A=L*U with ordinary partial (maximum absolute column)
// pivoting, using DefaultLUOptions. The input is never modified.
func Factorize(a *Dense) (*LU, error) { return FactorizeWithOptions(a, DefaultLUOptions()) }

// FactorizeWithOptions copies a and factors it. A rejected pivot returns nil
// with *SingularError; no partially computed factorization escapes on failure.
// Division by the pivot is direct, avoiding an overflowing reciprocal for
// subnormal pivots. Trailing updates use fused multiply-adds.
func FactorizeWithOptions(a *Dense, opt LUOptions) (*LU, error) {
	if a == nil {
		return nil, ErrInvalid
	}
	if a.rows != a.cols {
		return nil, fmt.Errorf("%w: LU requires square input, got %dx%d", ErrDimension, a.rows, a.cols)
	}
	tol := opt.RelativePivotTolerance
	if !finite(tol) || tol < 0 || tol >= 1 {
		return nil, fmt.Errorf("%w: %g, need [0,1)", ErrTolerance, tol)
	}
	var (
		n      = a.rows
		f      = &LU{n: n, data: a.Data(), perm: make([]int, n), parity: 1}
		scales = make([]float64, n)
	)
	for i := range n {
		f.perm[i] = i
		for _, v := range f.data[i*n : (i+1)*n] {
			scales[i] = math.Max(scales[i], math.Abs(v))
		}
	}
	for k := range n {
		pivotRow := k
		pivotAbs := math.Abs(f.data[k*n+k])
		for i := k + 1; i < n; i++ {
			if v := math.Abs(f.data[i*n+k]); v > pivotAbs {
				pivotAbs, pivotRow = v, i
			}
		}
		if pivotAbs == 0 || (tol > 0 && pivotAbs/scales[pivotRow] <= tol) {
			return nil, &SingularError{Pivot: k, Value: f.data[pivotRow*n+k], Scale: scales[pivotRow], RelativePivotTolerance: tol}
		}
		if pivotRow != k {
			for j := range n {
				f.data[k*n+j], f.data[pivotRow*n+j] = f.data[pivotRow*n+j], f.data[k*n+j]
			}
			f.perm[k], f.perm[pivotRow] = f.perm[pivotRow], f.perm[k]
			scales[k], scales[pivotRow] = scales[pivotRow], scales[k]
			f.parity = -f.parity
		}
		pivot := f.data[k*n+k]
		for i := k + 1; i < n; i++ {
			multiplier := f.data[i*n+k] / pivot
			f.data[i*n+k] = multiplier
			for j := k + 1; j < n; j++ {
				v := math.FMA(-multiplier, f.data[k*n+j], f.data[i*n+j])
				if !finite(v) {
					return nil, fmt.Errorf("%w: LU update (%d,%d) at pivot %d", ErrNonFinite, i, j, k)
				}
				f.data[i*n+j] = v
			}
		}
	}
	// Partial pivoting bounds each multiplier by one; all trailing entries
	// were checked above, and the immutable input contains only finite values.
	f.ready = true
	return f, nil
}

// Size returns n; nil and uninitialized factorizations report zero.
func (f *LU) Size() int {
	if f == nil {
		return 0
	}
	return f.n
}

// Pivots returns a copy of the final permutation, not a swap log:
// row i of P*A is original row Pivots()[i]. Invalid factors return nil.
func (f *LU) Pivots() []int {
	if f == nil || !f.ready {
		return nil
	}
	return append([]int(nil), f.perm...)
}

// Factors materializes independent L and U matrices satisfying P*A=L*U.
func (f *LU) Factors() (lower, upper *Dense, err error) {
	if f == nil || !f.ready {
		return nil, nil, ErrInvalid
	}
	lower, err = Identity(f.n)
	if err != nil {
		return nil, nil, err
	}
	upper, err = Zeros(f.n, f.n)
	if err != nil {
		return nil, nil, err
	}
	for i := range f.n {
		for j := range f.n {
			if i > j {
				lower.data[i*f.n+j] = f.data[i*f.n+j]
			} else {
				upper.data[i*f.n+j] = f.data[i*f.n+j]
			}
		}
	}
	return lower, upper, nil
}

// Solve solves A*x=b using this factorization, returning independent storage.
// A successful solve guarantees finite results, not a small forward error.
func (f *LU) Solve(b []float64) ([]float64, error) {
	if f == nil || !f.ready {
		return nil, ErrInvalid
	}
	if len(b) != f.n {
		return nil, fmt.Errorf("%w: solve size %d with vector length %d", ErrDimension, f.n, len(b))
	}
	if err := checkFinite(b, "solve RHS"); err != nil {
		return nil, err
	}
	x := make([]float64, f.n)
	for i, p := range f.perm {
		x[i] = b[p]
	}
	if err := f.solveInPlace(x, 1); err != nil {
		return nil, err
	}
	return x, nil
}

// SolveMatrix solves A*X=B, treating each column of B as a separate RHS.
// B and the factors remain unchanged, and the result has B's shape.
func (f *LU) SolveMatrix(b *Dense) (*Dense, error) {
	if f == nil || !f.ready || b == nil {
		return nil, ErrInvalid
	}
	if b.rows != f.n {
		return nil, fmt.Errorf("%w: solve size %d with RHS %dx%d", ErrDimension, f.n, b.rows, b.cols)
	}
	x, err := Zeros(b.rows, b.cols)
	if err != nil {
		return nil, err
	}
	if len(x.data) == 0 {
		return x, nil
	}
	for i, p := range f.perm {
		copy(x.data[i*b.cols:(i+1)*b.cols], b.data[p*b.cols:(p+1)*b.cols])
	}
	if err := f.solveInPlace(x.data, b.cols); err != nil {
		return nil, err
	}
	return x, nil
}

// solveInPlace consumes a permuted, owned, finite n-by-cols RHS.
func (f *LU) solveInPlace(x []float64, cols int) error {
	for i := range f.n {
		row := x[i*cols : (i+1)*cols]
		for j := 0; j < i; j++ {
			lij := f.data[i*f.n+j]
			for c := range cols {
				row[c] = math.FMA(-lij, x[j*cols+c], row[c])
			}
		}
		if err := checkFinite(row, "forward substitution"); err != nil {
			return err
		}
	}
	for i := f.n - 1; i >= 0; i-- {
		row := x[i*cols : (i+1)*cols]
		for j := i + 1; j < f.n; j++ {
			uij := f.data[i*f.n+j]
			for c := range cols {
				row[c] = math.FMA(-uij, x[j*cols+c], row[c])
			}
		}
		pivot := f.data[i*f.n+i]
		for c := range cols {
			row[c] /= pivot
		}
		if err := checkFinite(row, "back substitution"); err != nil {
			return err
		}
	}
	return nil
}

// Det returns the signed determinant, using scaled mantissa/exponent products
// to avoid intermediate product overflow/underflow. The empty determinant is 1.
// If the final nonzero value overflows to Inf or rounds to zero, it returns 0
// and ErrRange; use LogDet in that case. Subnormal nonzero results are allowed.
func (f *LU) Det() (float64, error) {
	if f == nil || !f.ready {
		return 0, ErrInvalid
	}
	mantissa, exponent := f.detParts()
	det := math.Ldexp(mantissa, exponent)
	if !finite(det) || det == 0 {
		return 0, ErrRange
	}
	return det, nil
}

// LogDet returns the determinant sign (+1 or -1) and log(abs(det)), without
// requiring that det itself fit in float64. The empty result is (+1,0).
func (f *LU) LogDet() (sign int, logAbs float64, err error) {
	if f == nil || !f.ready {
		return 0, 0, ErrInvalid
	}
	mantissa, exponent := f.detParts()
	sign = 1
	if mantissa < 0 {
		sign = -1
	}
	return sign, math.Log(math.Abs(mantissa)) + float64(exponent)*math.Ln2, nil
}

func (f *LU) detParts() (mantissa float64, exponent int) {
	mantissa = float64(f.parity)
	for i := range f.n {
		fraction, power := math.Frexp(f.data[i*f.n+i])
		mantissa *= fraction
		var adjustment int
		mantissa, adjustment = math.Frexp(mantissa)
		exponent += power + adjustment
	}
	return mantissa, exponent
}

// Inverse solves A*X=I using the stored factors. Prefer Solve/SolveMatrix when
// applying an inverse to RHS data, to avoid explicitly forming the inverse.
func (f *LU) Inverse() (*Dense, error) {
	if f == nil || !f.ready {
		return nil, ErrInvalid
	}
	x, err := Zeros(f.n, f.n)
	if err != nil {
		return nil, err
	}
	for i, p := range f.perm {
		x.data[i*f.n+p] = 1
	}
	if err := f.solveInPlace(x.data, f.n); err != nil {
		return nil, err
	}
	return x, nil
}

// Solve factors a with the default policy and solves a*x=b. For repeated solves
// or a custom pivot policy, call Factorize[WithOptions] once and reuse its LU.
func Solve(a *Dense, b []float64) ([]float64, error) {
	f, err := Factorize(a)
	if err != nil {
		return nil, err
	}
	return f.Solve(b)
}

// SolveMatrix factors a with the default policy and solves a*X=b.
func SolveMatrix(a, b *Dense) (*Dense, error) {
	f, err := Factorize(a)
	if err != nil {
		return nil, err
	}
	return f.SolveMatrix(b)
}

// Det factors a with the default policy. A rejected pivot returns (0, error),
// even for a mathematically singular matrix whose determinant is zero.
func Det(a *Dense) (float64, error) {
	f, err := Factorize(a)
	if err != nil {
		return 0, err
	}
	return f.Det()
}

// Inverse factors a with the default policy and solves a*X=I.
func Inverse(a *Dense) (*Dense, error) {
	f, err := Factorize(a)
	if err != nil {
		return nil, err
	}
	return f.Inverse()
}
