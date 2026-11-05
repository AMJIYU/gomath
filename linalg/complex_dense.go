package linalg

import (
	"errors"
	"fmt"
	"math/cmplx"
)

// ErrComplexLimit matches a complex-matrix dimension/element/work policy limit.
// Arithmetic size overflow instead matches ErrDimension; invalid policies match ErrBudget.
var ErrComplexLimit = errors.New("linalg: complex matrix resource limit exceeded")

// ComplexLimits bounds each call, not a cumulative account retained by a matrix.
// Zero fields select defaults: MaxDimension=1024, MaxElements=1M, MaxWork=256M.
// MaxDimension must be 1..4096; MaxElements <=16M, measured in complex entries
// per matrix/vector buffer. Work is a conservative bounded-kernel count, not
// FLOPs or elapsed time. All dimensions (including zero-storage shapes) are capped.
// See docs/LINALG.md for operation reserves and total workspace bounds.
type ComplexLimits struct {
	MaxDimension         int
	MaxElements, MaxWork uint64
}

// DefaultComplexLimits selects dimension 1024, 1M entries/buffer and 256M work units.
func DefaultComplexLimits() ComplexLimits { return ComplexLimits{1024, 1 << 20, 256 << 20} }
func (l ComplexLimits) normalized() (ComplexLimits, error) {
	if l.MaxDimension == 0 {
		l.MaxDimension = 1024
	}
	if l.MaxElements == 0 {
		l.MaxElements = 1 << 20
	}
	if l.MaxWork == 0 {
		l.MaxWork = 256 << 20
	}
	if l.MaxDimension < 1 || l.MaxDimension > 4096 || l.MaxElements > 16<<20 {
		return l, fmt.Errorf("%w: complex limits", ErrBudget)
	}
	return l, nil
}
func complexCount(rows, cols int) (int, error) {
	maxElements := int(^uint(0)>>1) / 16
	if rows < 0 || cols < 0 || (cols != 0 && rows > maxElements/cols) {
		return 0, fmt.Errorf("%w: complex storage %dx%d", ErrDimension, rows, cols)
	}
	return rows * cols, nil
}
func (l ComplexLimits) shape(rows, cols int) (int, error) {
	count, err := complexCount(rows, cols)
	if err != nil {
		return 0, err
	}
	if rows > l.MaxDimension || cols > l.MaxDimension || uint64(count) > l.MaxElements {
		return 0, fmt.Errorf("%w: complex shape %dx%d", ErrComplexLimit, rows, cols)
	}
	return count, nil
}
func (l ComplexLimits) work(units uint64) error {
	if units > l.MaxWork {
		return fmt.Errorf("%w: complex work needs %d, limit %d", ErrComplexLimit, units, l.MaxWork)
	}
	return nil
}

// ComplexDense is an immutable finite complex128 row-major matrix. Constructors
// and Data copy; operations allocate. The nonnil zero value is a valid empty
// 0x0 matrix, and a nil receiver is invalid for operations. Completed matrices
// can be shared by concurrent readers. Limits are explicit per-call policies;
// default operations use defaults even if an input was made with larger limits.
type ComplexDense struct {
	rows, cols int
	data       []complex128
}

// NewComplexDense copies finite row-major values under default resource limits.
func NewComplexDense(rows, cols int, data []complex128) (*ComplexDense, error) {
	return NewComplexDenseWithLimits(rows, cols, data, ComplexLimits{})
}

// NewComplexDenseWithLimits validates shape, resources and components before copying.
func NewComplexDenseWithLimits(rows, cols int, data []complex128, limits ComplexLimits) (*ComplexDense, error) {
	l, err := limits.normalized()
	if err != nil {
		return nil, err
	}
	count, err := l.shape(rows, cols)
	if err != nil {
		return nil, err
	}
	if len(data) != count {
		return nil, fmt.Errorf("%w: complex shape needs %d entries, got %d", ErrDimension, count, len(data))
	}
	if err = l.work(uint64(count)); err != nil {
		return nil, err
	}
	if err = checkComplex(data, "new complex matrix"); err != nil {
		return nil, err
	}
	return &ComplexDense{rows, cols, append([]complex128(nil), data...)}, nil
}

// ComplexZeros constructs an independent zero matrix under default limits.
func ComplexZeros(rows, cols int) (*ComplexDense, error) {
	return ComplexZerosWithLimits(rows, cols, ComplexLimits{})
}

// ComplexZerosWithLimits constructs zeros after checking the explicit budget.
func ComplexZerosWithLimits(rows, cols int, limits ComplexLimits) (*ComplexDense, error) {
	l, err := limits.normalized()
	if err != nil {
		return nil, err
	}
	count, err := l.shape(rows, cols)
	if err != nil {
		return nil, err
	}
	if err = l.work(uint64(count)); err != nil {
		return nil, err
	}
	return &ComplexDense{rows, cols, make([]complex128, count)}, nil
}

// ComplexIdentity constructs a bounded square identity, including order zero.
func ComplexIdentity(n int) (*ComplexDense, error) {
	return ComplexIdentityWithLimits(n, ComplexLimits{})
}

// ComplexIdentityWithLimits constructs an identity with explicit resource limits.
func ComplexIdentityWithLimits(n int, limits ComplexLimits) (*ComplexDense, error) {
	a, err := ComplexZerosWithLimits(n, n, limits)
	if err != nil {
		return nil, err
	}
	for i := range n {
		a.data[i*n+i] = 1
	}
	return a, nil
}

// Dimensions returns the shape; nil reports (0,0) but remains invalid for operations.
func (a *ComplexDense) Dimensions() (int, int) {
	if a == nil {
		return 0, 0
	}
	return a.rows, a.cols
}

// Data copies row-major complex entries; nil/empty matrices return nil.
func (a *ComplexDense) Data() []complex128 {
	if a == nil {
		return nil
	}
	return append([]complex128(nil), a.data...)
}

// At reads a zero-based entry, checking indices before access.
func (a *ComplexDense) At(row, col int) (complex128, error) {
	if a == nil {
		return 0, ErrInvalid
	}
	if row < 0 || col < 0 || row >= a.rows || col >= a.cols {
		return 0, fmt.Errorf("%w: complex index (%d,%d)", ErrDimension, row, col)
	}
	return a.data[row*a.cols+col], nil
}

// Transpose copies the ordinary transpose without conjugating entries.
func (a *ComplexDense) Transpose() (*ComplexDense, error) {
	return a.TransposeWithLimits(ComplexLimits{})
}

// ConjTranspose copies the conjugate transpose.
func (a *ComplexDense) ConjTranspose() (*ComplexDense, error) {
	return a.ConjTransposeWithLimits(ComplexLimits{})
}

// TransposeWithLimits applies explicit resources to an independent transpose.
func (a *ComplexDense) TransposeWithLimits(l ComplexLimits) (*ComplexDense, error) {
	return a.transpose(l, false)
}

// ConjTransposeWithLimits applies explicit resources to a conjugate transpose.
func (a *ComplexDense) ConjTransposeWithLimits(l ComplexLimits) (*ComplexDense, error) {
	return a.transpose(l, true)
}
func (a *ComplexDense) transpose(limits ComplexLimits, conjugate bool) (*ComplexDense, error) {
	if a == nil {
		return nil, ErrInvalid
	}
	out, err := ComplexZerosWithLimits(a.cols, a.rows, limits)
	if err != nil {
		return nil, err
	}
	if len(a.data) == 0 {
		return out, nil
	}
	for i := range a.rows {
		for j := range a.cols {
			z := a.data[i*a.cols+j]
			if conjugate {
				z = cmplx.Conj(z)
			}
			out.data[j*a.rows+i] = z
		}
	}
	return out, nil
}

// Add returns a+b with default limits, without modifying either operand.
func (a *ComplexDense) Add(b *ComplexDense) (*ComplexDense, error) {
	return a.AddWithLimits(b, ComplexLimits{})
}

// Sub returns a-b with default limits, without modifying either operand.
func (a *ComplexDense) Sub(b *ComplexDense) (*ComplexDense, error) {
	return a.SubWithLimits(b, ComplexLimits{})
}

// AddWithLimits performs bounded same-shape elementwise addition.
func (a *ComplexDense) AddWithLimits(b *ComplexDense, l ComplexLimits) (*ComplexDense, error) {
	return a.combine(b, l, false)
}

// SubWithLimits performs bounded same-shape elementwise subtraction.
func (a *ComplexDense) SubWithLimits(b *ComplexDense, l ComplexLimits) (*ComplexDense, error) {
	return a.combine(b, l, true)
}
func (a *ComplexDense) combine(b *ComplexDense, limits ComplexLimits, subtract bool) (*ComplexDense, error) {
	if a == nil || b == nil {
		return nil, ErrInvalid
	}
	if a.rows != b.rows || a.cols != b.cols {
		return nil, ErrDimension
	}
	l, err := limits.normalized()
	if err != nil {
		return nil, err
	}
	count, err := l.shape(a.rows, a.cols)
	if err != nil {
		return nil, err
	}
	if err = l.work(3 * uint64(count)); err != nil {
		return nil, err
	}
	out := &ComplexDense{a.rows, a.cols, make([]complex128, count)}
	for i, x := range a.data {
		y := b.data[i]
		if subtract {
			y = -y
		}
		z := x + y
		if !finiteComplex(z) {
			return nil, fmt.Errorf("%w: complex add/subtract entry %d", ErrRange, i)
		}
		out.data[i] = z
	}
	return out, nil
}

// Mul computes the bilinear product, without implicit conjugation.
func (a *ComplexDense) Mul(b *ComplexDense) (*ComplexDense, error) {
	return a.MulWithLimits(b, ComplexLimits{})
}

// MulWithLimits bounds all shapes/work and uses exact finite-input product sums.
func (a *ComplexDense) MulWithLimits(b *ComplexDense, limits ComplexLimits) (*ComplexDense, error) {
	if a == nil || b == nil {
		return nil, ErrInvalid
	}
	if a.cols != b.rows {
		return nil, ErrDimension
	}
	l, err := limits.normalized()
	if err != nil {
		return nil, err
	}
	for _, shape := range [][2]int{{a.rows, a.cols}, {b.rows, b.cols}, {a.rows, b.cols}} {
		if _, err = l.shape(shape[0], shape[1]); err != nil {
			return nil, err
		}
	}
	m, k, n := uint64(a.rows), uint64(a.cols), uint64(b.cols)
	if err = l.work(m*n*(k+1) + m*k + k*n); err != nil {
		return nil, err
	}
	out := &ComplexDense{a.rows, b.cols, make([]complex128, a.rows*b.cols)}
	if len(out.data) == 0 || a.cols == 0 {
		return out, nil
	}
	for i := range a.rows {
		for j := range b.cols {
			var sum complexExactSum
			for p := range a.cols {
				sum.term(a.data[i*a.cols+p], b.data[p*b.cols+j], false)
			}
			z, e := complexTo128(sum.value())
			if e != nil {
				return nil, fmt.Errorf("complex multiply (%d,%d): %w", i, j, e)
			}
			out.data[i*b.cols+j] = z
		}
	}
	return out, nil
}

// MulVec computes a*x without conjugation under default limits.
func (a *ComplexDense) MulVec(x []complex128) ([]complex128, error) {
	return a.MulVecWithLimits(x, ComplexLimits{})
}

// MulVecWithLimits validates finite vector input and explicit resources.
func (a *ComplexDense) MulVecWithLimits(x []complex128, limits ComplexLimits) ([]complex128, error) {
	if a == nil {
		return nil, ErrInvalid
	}
	if len(x) != a.cols {
		return nil, ErrDimension
	}
	l, err := limits.normalized()
	if err != nil {
		return nil, err
	}
	if _, err = l.shape(1, len(x)); err != nil {
		return nil, err
	}
	if _, err = l.shape(a.rows, a.cols); err != nil {
		return nil, err
	}
	if _, err = l.shape(a.rows, 1); err != nil {
		return nil, err
	}
	if err = l.work(uint64(a.rows)*uint64(a.cols+1) + uint64(a.cols)); err != nil {
		return nil, err
	}
	if err = checkComplex(x, "complex matrix-vector RHS"); err != nil {
		return nil, err
	}
	out := make([]complex128, a.rows)
	for i := range a.rows {
		var sum complexExactSum
		for j, z := range x {
			sum.term(a.data[i*a.cols+j], z, false)
		}
		out[i], err = complexTo128(sum.value())
		if err != nil {
			return nil, fmt.Errorf("complex matrix-vector row %d: %w", i, err)
		}
	}
	return out, nil
}
