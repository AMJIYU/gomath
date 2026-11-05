// Package linalg provides immutable dense float64 matrices and reusable,
// pivoted LU, Householder QR, Cholesky, real Schur, complex right eigenvectors,
// symmetric eigen and Jacobi SVD using only the Go standard library. It implements
// direct, least-squares and truncated minimum-norm solves, not a general
// numerical computing system.
package linalg

import (
	"errors"
	"fmt"
	"math"
)

var (
	ErrDimension           = errors.New("linalg: incompatible or invalid dimensions")
	ErrNonFinite           = errors.New("linalg: nonfinite input or arithmetic result")
	ErrSingular            = errors.New("linalg: singular matrix under pivot policy")
	ErrInvalid             = errors.New("linalg: nil matrix or uninitialized factorization")
	ErrTolerance           = errors.New("linalg: invalid pivot tolerance")
	ErrRange               = errors.New("linalg: result or required rotation outside float64 range")
	ErrRankDeficient       = errors.New("linalg: deficient column rank under factorization policy")
	ErrResidual            = errors.New("linalg: least-squares residual exceeds tolerance")
	ErrNotSymmetric        = errors.New("linalg: matrix is not symmetric under symmetry policy")
	ErrNotPositiveDefinite = errors.New("linalg: matrix is not positive definite under pivot policy")
	ErrReconstruction      = errors.New("linalg: SVD reconstruction exceeds tolerance")
)

// Dense is an immutable matrix with private, contiguous row-major storage.
// NewDense copies its input; Data returns a copy. Operations allocate independent
// results, so matrices can be shared by concurrent readers. The zero value is a
// valid 0-by-0 matrix. A nil *Dense is invalid for numerical operations.
type Dense struct {
	rows, cols int
	data       []float64
}

// NewDense copies exactly rows*cols finite values in row-major order.
// Dimensions may be zero, but cannot be negative or overflow storage arithmetic.
func NewDense(rows, cols int, data []float64) (*Dense, error) {
	n, err := elementCount(rows, cols)
	if err != nil {
		return nil, err
	}
	if len(data) != n {
		return nil, fmt.Errorf("%w: %dx%d needs %d values, got %d", ErrDimension, rows, cols, n, len(data))
	}
	if err := checkFinite(data, "new matrix"); err != nil {
		return nil, err
	}
	return &Dense{rows: rows, cols: cols, data: append([]float64(nil), data...)}, nil
}

// Zeros constructs a rows-by-cols matrix filled with zeros.
func Zeros(rows, cols int) (*Dense, error) {
	n, err := elementCount(rows, cols)
	if err != nil {
		return nil, err
	}
	return &Dense{rows: rows, cols: cols, data: make([]float64, n)}, nil
}

// Identity constructs the n-by-n identity, including the empty identity at n=0.
func Identity(n int) (*Dense, error) {
	a, err := Zeros(n, n)
	if err != nil {
		return nil, err
	}
	for i := range n {
		a.data[i*n+i] = 1
	}
	return a, nil
}

// Dimensions returns the immutable shape. A nil receiver reports (0,0), but
// remains invalid for operations; use a nonnil zero value for an empty matrix.
func (a *Dense) Dimensions() (rows, cols int) {
	if a == nil {
		return 0, 0
	}
	return a.rows, a.cols
}

// Data returns an independent row-major copy (nil for nil or empty matrices).
func (a *Dense) Data() []float64 {
	if a == nil {
		return nil
	}
	return append([]float64(nil), a.data...)
}

// At returns one entry, with zero-based indices checked before accessing storage.
func (a *Dense) At(row, col int) (float64, error) {
	if a == nil {
		return 0, ErrInvalid
	}
	if row < 0 || row >= a.rows || col < 0 || col >= a.cols {
		return 0, fmt.Errorf("%w: index (%d,%d) outside %dx%d", ErrDimension, row, col, a.rows, a.cols)
	}
	return a.data[row*a.cols+col], nil
}

// Transpose materializes a new matrix; it does not expose a shared storage view.
func (a *Dense) Transpose() (*Dense, error) {
	if a == nil {
		return nil, ErrInvalid
	}
	out, err := Zeros(a.cols, a.rows)
	if err != nil {
		return nil, err
	}
	if len(a.data) == 0 {
		return out, nil
	}
	for i := range a.rows {
		for j := range a.cols {
			out.data[j*a.rows+i] = a.data[i*a.cols+j]
		}
	}
	return out, nil
}

// Add computes a+b. Neither operand is modified.
func (a *Dense) Add(b *Dense) (*Dense, error) { return a.combine(b, false) }

// Sub computes a-b. Neither operand is modified.
func (a *Dense) Sub(b *Dense) (*Dense, error) { return a.combine(b, true) }

func (a *Dense) combine(b *Dense, subtract bool) (*Dense, error) {
	if a == nil || b == nil {
		return nil, ErrInvalid
	}
	if a.rows != b.rows || a.cols != b.cols {
		return nil, fmt.Errorf("%w: elementwise %dx%d and %dx%d", ErrDimension, a.rows, a.cols, b.rows, b.cols)
	}
	out, err := Zeros(a.rows, a.cols)
	if err != nil {
		return nil, err
	}
	for i, x := range a.data {
		if subtract {
			out.data[i] = x - b.data[i]
		} else {
			out.data[i] = x + b.data[i]
		}
	}
	if err := checkFinite(out.data, "add/subtract"); err != nil {
		return nil, err
	}
	return out, nil
}

// Mul computes a*b using contiguous row updates and fused multiply-adds.
// Any nonfinite arithmetic result is an error. Finite underflow follows normal
// float64 rounding; this routine does not equilibrate or rescale operands.
func (a *Dense) Mul(b *Dense) (*Dense, error) {
	if a == nil || b == nil {
		return nil, ErrInvalid
	}
	if a.cols != b.rows {
		return nil, fmt.Errorf("%w: multiply %dx%d by %dx%d", ErrDimension, a.rows, a.cols, b.rows, b.cols)
	}
	out, err := Zeros(a.rows, b.cols)
	if err != nil {
		return nil, err
	}
	if len(out.data) == 0 || a.cols == 0 {
		return out, nil
	}
	for i := range a.rows {
		row := out.data[i*b.cols : (i+1)*b.cols]
		for k := range a.cols {
			x := a.data[i*a.cols+k]
			brow := b.data[k*b.cols : (k+1)*b.cols]
			for j, y := range brow {
				row[j] = math.FMA(x, y, row[j])
			}
		}
	}
	if err := checkFinite(out.data, "multiply"); err != nil {
		return nil, err
	}
	return out, nil
}

// MulVec computes a*x, copying neither input into the result's storage.
func (a *Dense) MulVec(x []float64) ([]float64, error) {
	if a == nil {
		return nil, ErrInvalid
	}
	if len(x) != a.cols {
		return nil, fmt.Errorf("%w: %dx%d times vector length %d", ErrDimension, a.rows, a.cols, len(x))
	}
	if err := checkFinite(x, "matrix-vector input"); err != nil {
		return nil, err
	}
	if _, err := elementCount(a.rows, 1); err != nil {
		return nil, err
	}
	out := make([]float64, a.rows)
	if a.cols == 0 {
		return out, nil
	}
	for i := range a.rows {
		var sum float64
		for j, v := range x {
			sum = math.FMA(a.data[i*a.cols+j], v, sum)
		}
		out[i] = sum
	}
	if err := checkFinite(out, "matrix-vector multiply"); err != nil {
		return nil, err
	}
	return out, nil
}

func elementCount(rows, cols int) (int, error) {
	// Limit both entry count and byte count before allocation/index arithmetic.
	maxElements := int(^uint(0)>>1) / 8
	if rows < 0 || cols < 0 || (cols != 0 && rows > maxElements/cols) {
		return 0, fmt.Errorf("%w: invalid storage size %dx%d", ErrDimension, rows, cols)
	}
	return rows * cols, nil
}

func finite(x float64) bool { return !math.IsNaN(x) && !math.IsInf(x, 0) }

func checkFinite(data []float64, operation string) error {
	for i, x := range data {
		if !finite(x) {
			return fmt.Errorf("%w: %s at row-major index %d", ErrNonFinite, operation, i)
		}
	}
	return nil
}
