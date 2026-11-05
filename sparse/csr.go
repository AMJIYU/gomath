// Package sparse provides immutable finite real CSR matrices, products and bounded iterative solvers.
// It uses only portable Go and the standard library.
package sparse

import (
	"errors"
	"fmt"
	"math"
	"sort"
)

var (
	ErrShape        = errors.New("sparse: invalid shape or index")
	ErrStructure    = errors.New("sparse: invalid CSR structure")
	ErrNonFinite    = errors.New("sparse: nonfinite input")
	ErrLimit        = errors.New("sparse: resource budget exceeded")
	ErrOptions      = errors.New("sparse: invalid options")
	ErrRange        = errors.New("sparse: numerical result outside float64 range")
	ErrNotSymmetric = errors.New("sparse: matrix is not symmetric")
	ErrBreakdown    = errors.New("sparse: CG breakdown or nonpositive curvature")
	ErrConvergence  = errors.New("sparse: CG did not converge")
)

// Limits bounds each call, not a cumulative account retained by a matrix.
// Zero fields select defaults. MaxStorageBytes counts new array payloads,
// excluding caller-owned inputs. Most operations exclude fixed numeric scratch;
// SpGEMM and BiCGSTAB additionally charge fixed numeric-scratch reserves.
// Work units count bounded kernels, not FLOPs; see docs/SPARSE.md.
type Limits struct {
	MaxDimension                     int
	MaxNNZ, MaxStorageBytes, MaxWork uint64
}

func DefaultLimits() Limits { return Limits{1 << 20, 1 << 20, 64 << 20, 16 << 20} }
func (l Limits) normalized() (Limits, error) {
	d := DefaultLimits()
	if l.MaxDimension == 0 {
		l.MaxDimension = d.MaxDimension
	}
	if l.MaxNNZ == 0 {
		l.MaxNNZ = d.MaxNNZ
	}
	if l.MaxStorageBytes == 0 {
		l.MaxStorageBytes = d.MaxStorageBytes
	}
	if l.MaxWork == 0 {
		l.MaxWork = d.MaxWork
	}
	if l.MaxDimension < 1 || l.MaxDimension > 4<<20 || l.MaxNNZ > 16<<20 || l.MaxStorageBytes > 512<<20 || l.MaxWork > 1<<34 {
		return l, ErrOptions
	}
	return l, nil
}
func (l Limits) shape(r, c int) error {
	if r < 0 || c < 0 {
		return ErrShape
	}
	if r > l.MaxDimension || c > l.MaxDimension {
		return ErrLimit
	}
	return nil
}
func (l Limits) storage(bytes uint64) error {
	if bytes > l.MaxStorageBytes || bytes > uint64(^uint(0)>>1) {
		return ErrLimit
	}
	return nil
}
func (l Limits) work(units uint64) error {
	if units > l.MaxWork {
		return ErrLimit
	}
	return nil
}
func (l Limits) matrix(r, c, nnz int, extra uint64) error {
	if err := l.shape(r, c); err != nil {
		return err
	}
	if nnz < 0 || uint64(nnz) > l.MaxNNZ {
		return ErrLimit
	}
	return l.storage(8*(uint64(r)+1) + 16*uint64(nnz) + extra)
}
func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
func vector(v []float64) error {
	for _, x := range v {
		if !finite(x) {
			return ErrNonFinite
		}
	}
	return nil
}

// CSR stores entries by row with strictly increasing column indices. Explicit
// zeros are allowed and preserved by construction/transpose. Add/Sub/Mul omit exact
// zero results. The nonnil zero value is a valid 0x0 matrix; nil is invalid.
// Matrices can be shared by concurrent readers; all exposed arrays are copies.
type CSR struct {
	rows, cols int
	ptr, col   []int
	val        []float64
}

func NewCSR(rows, cols int, rowPtr, colInd []int, values []float64) (*CSR, error) {
	return NewCSRWithLimits(rows, cols, rowPtr, colInd, values, Limits{})
}
func NewCSRWithLimits(rows, cols int, rowPtr, colInd []int, values []float64, limits Limits) (*CSR, error) {
	l, e := limits.normalized()
	if e != nil {
		return nil, e
	}
	if e = l.matrix(rows, cols, len(values), 0); e != nil {
		return nil, e
	}
	if len(rowPtr) != rows+1 || len(colInd) != len(values) {
		return nil, ErrStructure
	}
	if e = l.work(uint64(rows) + uint64(len(values)) + 1); e != nil {
		return nil, e
	}
	if rowPtr[0] != 0 || rowPtr[rows] != len(values) {
		return nil, ErrStructure
	}
	for i := 0; i < rows; i++ {
		lo, hi := rowPtr[i], rowPtr[i+1]
		if lo < 0 || hi < lo || hi > len(values) {
			return nil, fmt.Errorf("%w: row %d", ErrStructure, i)
		}
		prev := -1
		for j := lo; j < hi; j++ {
			if colInd[j] < 0 || colInd[j] >= cols || colInd[j] <= prev {
				return nil, fmt.Errorf("%w: column at %d", ErrStructure, j)
			}
			prev = colInd[j]
			if !finite(values[j]) {
				return nil, ErrNonFinite
			}
		}
	}
	return &CSR{rows, cols, append([]int(nil), rowPtr...), append([]int(nil), colInd...), append([]float64(nil), values...)}, nil
}
func (a *CSR) Dims() (int, int) {
	if a == nil {
		return 0, 0
	}
	return a.rows, a.cols
}
func (a *CSR) NNZ() int {
	if a == nil {
		return 0
	}
	return len(a.val)
}

// Data returns independent row pointers, column indices and values.
func (a *CSR) Data() ([]int, []int, []float64) {
	if a == nil {
		return nil, nil, nil
	}
	p := append([]int(nil), a.ptr...)
	if len(p) == 0 {
		p = []int{0}
	}
	return p, append([]int(nil), a.col...), append([]float64(nil), a.val...)
}
func (a *CSR) at(r, c int) float64 {
	lo, hi := a.ptr[r], a.ptr[r+1]
	k := lo + sort.Search(hi-lo, func(i int) bool { return a.col[lo+i] >= c })
	if k < hi && a.col[k] == c {
		return a.val[k]
	}
	return 0
}
func (a *CSR) At(r, c int) (float64, error) {
	if a == nil || r < 0 || c < 0 || r >= a.rows || c >= a.cols {
		return 0, ErrShape
	}
	return a.at(r, c), nil
}
func (a *CSR) check(l Limits) error {
	if a == nil {
		return ErrShape
	}
	return l.matrix(a.rows, a.cols, len(a.val), 0)
}

func (a *CSR) Transpose() (*CSR, error) { return a.TransposeWithLimits(Limits{}) }
func (a *CSR) TransposeWithLimits(limits Limits) (*CSR, error) {
	l, e := limits.normalized()
	if e != nil {
		return nil, e
	}
	if e = a.check(l); e != nil {
		return nil, e
	}
	if e = l.matrix(a.cols, a.rows, len(a.val), 8*uint64(a.cols)); e != nil {
		return nil, e
	}
	if e = l.work(4 * (uint64(a.rows) + uint64(a.cols) + uint64(len(a.val)) + 1)); e != nil {
		return nil, e
	}
	p := make([]int, a.cols+1)
	for _, c := range a.col {
		p[c+1]++
	}
	for i := 1; i < len(p); i++ {
		p[i] += p[i-1]
	}
	next := append([]int(nil), p[:a.cols]...)
	c := make([]int, len(a.col))
	v := make([]float64, len(a.val))
	for r := 0; r < a.rows; r++ {
		for j := a.ptr[r]; j < a.ptr[r+1]; j++ {
			k := next[a.col[j]]
			c[k] = r
			v[k] = a.val[j]
			next[a.col[j]]++
		}
	}
	return &CSR{a.cols, a.rows, p, c, v}, nil
}
func (a *CSR) Add(b *CSR) (*CSR, error)                     { return a.AddWithLimits(b, Limits{}) }
func (a *CSR) Sub(b *CSR) (*CSR, error)                     { return a.SubWithLimits(b, Limits{}) }
func (a *CSR) AddWithLimits(b *CSR, l Limits) (*CSR, error) { return a.merge(b, l, 1) }
func (a *CSR) SubWithLimits(b *CSR, l Limits) (*CSR, error) { return a.merge(b, l, -1) }
func (a *CSR) merge(b *CSR, limits Limits, sign float64) (*CSR, error) {
	l, e := limits.normalized()
	if e != nil {
		return nil, e
	}
	if e = a.check(l); e != nil {
		return nil, e
	}
	if e = b.check(l); e != nil {
		return nil, e
	}
	if a.rows != b.rows || a.cols != b.cols {
		return nil, ErrShape
	}
	if e = l.work(4 * (uint64(a.rows) + uint64(len(a.val)) + uint64(len(b.val)) + 1)); e != nil {
		return nil, e
	}
	// Two passes avoid overallocating the union or rejecting a small cancellation result.
	walk := func(emit func(int, int, float64) error) error {
		for r := 0; r < a.rows; r++ {
			i, j := a.ptr[r], b.ptr[r]
			ie, je := a.ptr[r+1], b.ptr[r+1]
			for i < ie || j < je {
				var c int
				var v float64
				if j == je || (i < ie && a.col[i] < b.col[j]) {
					c, v = a.col[i], a.val[i]
					i++
				} else if i == ie || b.col[j] < a.col[i] {
					c, v = b.col[j], sign*b.val[j]
					j++
				} else {
					c, v = a.col[i], a.val[i]+sign*b.val[j]
					i++
					j++
				}
				if !finite(v) {
					return ErrRange
				}
				if v != 0 {
					if err := emit(r, c, v); err != nil {
						return err
					}
				}
			}
		}
		return nil
	}
	n := 0
	e = walk(func(_, _ int, _ float64) error {
		n++
		if uint64(n) > l.MaxNNZ {
			return ErrLimit
		}
		return nil
	})
	if e != nil {
		return nil, e
	}
	if e = l.matrix(a.rows, a.cols, n, 0); e != nil {
		return nil, e
	}
	p, c, v := make([]int, a.rows+1), make([]int, n), make([]float64, n)
	k := 0
	e = walk(func(r, col int, x float64) error { p[r+1]++; c[k] = col; v[k] = x; k++; return nil })
	if e != nil {
		return nil, e
	}
	for i := 1; i < len(p); i++ {
		p[i] += p[i-1]
	}
	return &CSR{a.rows, a.cols, p, c, v}, nil
}
func (a *CSR) MulVec(x []float64) ([]float64, error) { return a.MulVecWithLimits(x, Limits{}) }
func (a *CSR) MulVecWithLimits(x []float64, limits Limits) ([]float64, error) {
	l, e := limits.normalized()
	if e != nil {
		return nil, e
	}
	if e = a.check(l); e != nil {
		return nil, e
	}
	if len(x) != a.cols {
		return nil, ErrShape
	}
	if e = l.storage(8 * uint64(a.rows)); e != nil {
		return nil, e
	}
	if e = l.work(8 * (uint64(a.rows) + uint64(a.cols) + uint64(len(a.val)) + 1)); e != nil {
		return nil, e
	}
	if e = vector(x); e != nil {
		return nil, e
	}
	out := make([]float64, a.rows)
	if e = a.multiply(out, x, nil, nil); e != nil {
		return nil, e
	}
	return out, nil
}
