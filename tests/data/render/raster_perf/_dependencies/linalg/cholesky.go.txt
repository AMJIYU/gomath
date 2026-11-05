package linalg

import (
	"fmt"
	"math"
)

// CholeskyOptions selects symmetry and computed-positive-pivot policies.
// Both tolerances must be finite and in [0,1). Its zero value requires exact
// symmetry and strictly positive computed pivots, with no relative cutoff.
type CholeskyOptions struct {
	// RelativeSymmetryTolerance compares abs(aij/s-aji/s), where
	// s=max(abs(aij),abs(aji)). Accepted unequal entries are averaged;
	// factorization and solves then describe that symmetric matrix.
	RelativeSymmetryTolerance float64
	// RelativePivotTolerance rejects a positive Schur pivot if
	// pivot/original_diagonal <= tolerance. It is not a condition estimate.
	RelativePivotTolerance float64
}

// DefaultCholeskyOptions requires exact symmetry and uses a 1e-14 relative
// positive-pivot threshold. Approximate symmetry is always an explicit choice.
func DefaultCholeskyOptions() CholeskyOptions {
	return CholeskyOptions{RelativePivotTolerance: 1e-14}
}

// SymmetryError identifies the first unequal pair rejected by the policy.
// Row>Column; Lower=a[Row,Column], Upper=a[Column,Row].
type SymmetryError struct {
	Row, Column                       int
	Lower, Upper, Relative, Tolerance float64
}

func (e *SymmetryError) Error() string {
	return fmt.Sprintf("%v: (%d,%d) lower %g, upper %g, relative difference %g, tolerance %g", ErrNotSymmetric, e.Row, e.Column, e.Lower, e.Upper, e.Relative, e.Tolerance)
}

func (e *SymmetryError) Unwrap() error { return ErrNotSymmetric }

// PositiveDefiniteError identifies the rejected zero-based Schur pivot.
// Value is its computed value before taking a square root; Scale is the
// corresponding original diagonal entry. It matches ErrNotPositiveDefinite.
type PositiveDefiniteError struct {
	Pivot                  int
	Value, Scale           float64
	RelativePivotTolerance float64
}

func (e *PositiveDefiniteError) Error() string {
	return fmt.Sprintf("%v: pivot %d value %g, diagonal scale %g, tolerance %g", ErrNotPositiveDefinite, e.Pivot, e.Value, e.Scale, e.RelativePivotTolerance)
}

func (e *PositiveDefiniteError) Unwrap() error { return ErrNotPositiveDefinite }

// Cholesky owns immutable lower-triangular factors satisfying A=L*L^T (or the
// explicitly averaged symmetric matrix when a symmetry tolerance is enabled).
// The zero value is invalid; completed factorizations allow concurrent solves.
type Cholesky struct {
	n     int
	data  []float64
	ready bool
}

// FactorizeCholesky uses DefaultCholeskyOptions and leaves a unchanged.
func FactorizeCholesky(a *Dense) (*Cholesky, error) {
	return FactorizeCholeskyWithOptions(a, DefaultCholeskyOptions())
}

// FactorizeCholeskyWithOptions validates both triangles before factoring a
// private copy. Nonpositive or policy-rejected Schur pivots return nil with
// *PositiveDefiniteError; nonfinite arithmetic returns ErrNonFinite.
func FactorizeCholeskyWithOptions(a *Dense, opt CholeskyOptions) (*Cholesky, error) {
	if a == nil {
		return nil, ErrInvalid
	}
	if a.rows != a.cols {
		return nil, fmt.Errorf("%w: Cholesky needs square input, got %dx%d", ErrDimension, a.rows, a.cols)
	}
	for _, tol := range []float64{opt.RelativeSymmetryTolerance, opt.RelativePivotTolerance} {
		if !finite(tol) || tol < 0 || tol >= 1 {
			return nil, fmt.Errorf("%w: Cholesky tolerances must be finite and in [0,1)", ErrTolerance)
		}
	}
	n := a.rows
	for i := range n {
		for j := 0; j < i; j++ {
			lower, upper := a.data[i*n+j], a.data[j*n+i]
			if lower == upper {
				continue
			}
			scale := math.Max(math.Abs(lower), math.Abs(upper))
			relative := math.Abs(lower/scale - upper/scale)
			if opt.RelativeSymmetryTolerance == 0 || relative > opt.RelativeSymmetryTolerance {
				return nil, &SymmetryError{Row: i, Column: j, Lower: lower, Upper: upper, Relative: relative, Tolerance: opt.RelativeSymmetryTolerance}
			}
		}
	}
	lower, err := Zeros(n, n)
	if err != nil {
		return nil, err
	}
	f := &Cholesky{n: n, data: lower.data}
	for i := range n {
		for j := 0; j <= i; j++ {
			// Keep equal entries exactly. Sum small values before halving
			// (preserving subnormals), but halve huge values before summing
			// (avoiding overflow). Validation has already checked both sides.
			value := a.data[i*n+j]
			if other := a.data[j*n+i]; value != other {
				if math.Abs(value) > math.MaxFloat64/2 || math.Abs(other) > math.MaxFloat64/2 {
					value = value/2 + other/2
				} else {
					value = (value + other) / 2
				}
			}
			for k := 0; k < j; k++ {
				value = math.FMA(-f.data[i*n+k], f.data[j*n+k], value)
			}
			if !finite(value) {
				return nil, fmt.Errorf("%w: Cholesky update (%d,%d)", ErrNonFinite, i, j)
			}
			if i == j {
				scale := a.data[i*n+i]
				if scale <= 0 || value <= 0 || (opt.RelativePivotTolerance > 0 && value/scale <= opt.RelativePivotTolerance) {
					return nil, &PositiveDefiniteError{Pivot: i, Value: value, Scale: scale, RelativePivotTolerance: opt.RelativePivotTolerance}
				}
				f.data[i*n+i] = math.Sqrt(value)
			} else {
				value /= f.data[j*n+j]
				if !finite(value) {
					return nil, fmt.Errorf("%w: Cholesky division (%d,%d)", ErrNonFinite, i, j)
				}
				f.data[i*n+j] = value
			}
		}
	}
	f.ready = true
	return f, nil
}

// Size returns n, or zero for invalid factors.
func (f *Cholesky) Size() int {
	if f == nil || !f.ready {
		return 0
	}
	return f.n
}

// Lower materializes an independent n-by-n lower factor with zero upper entries.
func (f *Cholesky) Lower() (*Dense, error) {
	if f == nil || !f.ready {
		return nil, ErrInvalid
	}
	return &Dense{rows: f.n, cols: f.n, data: append([]float64(nil), f.data...)}, nil
}

// Solve solves A*x=b without modifying b or the factors.
func (f *Cholesky) Solve(b []float64) ([]float64, error) {
	if f == nil || !f.ready {
		return nil, ErrInvalid
	}
	if len(b) != f.n {
		return nil, fmt.Errorf("%w: Cholesky size %d with RHS length %d", ErrDimension, f.n, len(b))
	}
	if err := checkFinite(b, "Cholesky solve RHS"); err != nil {
		return nil, err
	}
	x := append([]float64(nil), b...)
	if err := f.solveInPlace(x, 1); err != nil {
		return nil, err
	}
	return x, nil
}

// SolveMatrix solves one RHS per column, allocating an independent result.
func (f *Cholesky) SolveMatrix(b *Dense) (*Dense, error) {
	if f == nil || !f.ready || b == nil {
		return nil, ErrInvalid
	}
	if b.rows != f.n {
		return nil, fmt.Errorf("%w: Cholesky size %d with RHS rows %d", ErrDimension, f.n, b.rows)
	}
	x := &Dense{rows: b.rows, cols: b.cols, data: b.Data()}
	if len(x.data) == 0 {
		return x, nil
	}
	if err := f.solveInPlace(x.data, b.cols); err != nil {
		return nil, err
	}
	return x, nil
}

func (f *Cholesky) solveInPlace(x []float64, cols int) error {
	for i := range f.n {
		row := x[i*cols : (i+1)*cols]
		for j := 0; j < i; j++ {
			lij := f.data[i*f.n+j]
			for c := range cols {
				row[c] = math.FMA(-lij, x[j*cols+c], row[c])
			}
		}
		for c := range cols {
			row[c] /= f.data[i*f.n+i]
		}
		if err := checkFinite(row, "Cholesky forward substitution"); err != nil {
			return err
		}
	}
	for i := f.n - 1; i >= 0; i-- {
		row := x[i*cols : (i+1)*cols]
		for j := i + 1; j < f.n; j++ {
			lji := f.data[j*f.n+i]
			for c := range cols {
				row[c] = math.FMA(-lji, x[j*cols+c], row[c])
			}
		}
		for c := range cols {
			row[c] /= f.data[i*f.n+i]
		}
		if err := checkFinite(row, "Cholesky back substitution"); err != nil {
			return err
		}
	}
	return nil
}

// SolveSPD factors a using the default Cholesky policy and solves one RHS.
func SolveSPD(a *Dense, b []float64) ([]float64, error) {
	f, err := FactorizeCholesky(a)
	if err != nil {
		return nil, err
	}
	return f.Solve(b)
}

// SolveSPDMatrix is the one-shot multi-RHS Cholesky solve.
func SolveSPDMatrix(a, b *Dense) (*Dense, error) {
	f, err := FactorizeCholesky(a)
	if err != nil {
		return nil, err
	}
	return f.SolveMatrix(b)
}
