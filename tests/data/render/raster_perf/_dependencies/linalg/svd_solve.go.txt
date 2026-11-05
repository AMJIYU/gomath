package linalg

import (
	"fmt"
	"math"
)

// Rank counts computed positive singular values whose ratio to the largest
// exceeds the factorization's RelativeRankTolerance. Invalid factors report 0.
// This is numerical rank under a caller policy, not exact mathematical rank.
func (f *SVD) Rank() int {
	if f == nil || !f.ready {
		return 0
	}
	return f.rankAt(f.rankTol)
}

// RankWithTolerance uses an explicit finite relative cutoff in [0,1), without
// modifying stored factors or the default rank policy. Zero retains every
// computed positive singular value, including numerical roundoff artifacts.
func (f *SVD) RankWithTolerance(relativeTolerance float64) (int, error) {
	if f == nil || !f.ready {
		return 0, ErrInvalid
	}
	if err := validateJacobi(relativeTolerance, 0); err != nil {
		return 0, err
	}
	return f.rankAt(relativeTolerance), nil
}

func (f *SVD) rankAt(tol float64) int {
	for i, s := range f.values {
		if s == 0 || (tol > 0 && s/f.values[0] <= tol) {
			return i
		}
	}
	return len(f.values)
}

// PseudoInverse returns V_r*diag(1/s_r)*U_r^T under the stored rank policy.
// It is the Moore-Penrose inverse of the truncated approximation A_r. If a
// nonzero value is discarded, A*P*A need not equal A within rounding error.
func (f *SVD) PseudoInverse() (*Dense, error) {
	if f == nil || !f.ready {
		return nil, ErrInvalid
	}
	return f.PseudoInverseWithTolerance(f.rankTol)
}

// PseudoInverseWithTolerance applies an explicit rank cutoff without changing
// Factors or Values. The result is an independent cols-by-rows Dense. It never
// divides by zero or constructs normal equations. Unrepresentable output
// entries return ErrNonFinite, including when an inverse of a tiny value grows
// beyond float64; ordinary underflow follows float64 rounding.
func (f *SVD) PseudoInverseWithTolerance(relativeTolerance float64) (*Dense, error) {
	rank, err := f.RankWithTolerance(relativeTolerance)
	if err != nil {
		return nil, err
	}
	out, err := Zeros(f.cols, f.rows)
	if err != nil {
		return nil, err
	}
	k := len(f.values)
	if rank == 0 || len(out.data) == 0 {
		return out, nil
	}
	for i := range f.cols {
		for j := range f.rows {
			var sum svdScaledSum
			for c := 0; c < rank; c++ {
				sum.add(svdQuotient(f.v[i*k+c], f.u[j*k+c], f.values[c]))
			}
			value := sum.value()
			if !finite(value) {
				return nil, fmt.Errorf("%w: SVD pseudoinverse (%d,%d)", ErrNonFinite, i, j)
			}
			out.data[i*f.rows+j] = value
		}
	}
	return out, nil
}

// Solve returns the minimum-Euclidean-norm least-squares solution for the
// truncated matrix A_r and the actual ||A*x-b||2 for the original stored A.
// With nonzero truncation, this need not minimize the residual for original A.
func (f *SVD) Solve(b []float64) (x []float64, residual float64, err error) {
	if f == nil || !f.ready {
		return nil, 0, ErrInvalid
	}
	return f.SolveWithTolerance(b, f.rankTol)
}

// SolveWithTolerance selects the numerical rank for this vector solve only.
func (f *SVD) SolveWithTolerance(b []float64, relativeTolerance float64) (x []float64, residual float64, err error) {
	if f == nil || !f.ready {
		return nil, 0, ErrInvalid
	}
	if len(b) != f.rows {
		return nil, 0, fmt.Errorf("%w: SVD needs RHS length %d, got %d", ErrDimension, f.rows, len(b))
	}
	xm, norms, err := f.SolveMatrixWithTolerance(&Dense{rows: f.rows, cols: 1, data: b}, relativeTolerance)
	if err != nil {
		return nil, 0, err
	}
	return xm.data, norms[0], nil
}

// SolveMatrix solves one RHS per column using the stored numerical rank. The
// independent result is n-by-r, with r actual residual norms for original A.
// Underdetermined, rank-deficient, zero and empty matrices are supported.
func (f *SVD) SolveMatrix(b *Dense) (x *Dense, residuals []float64, err error) {
	if f == nil || !f.ready {
		return nil, nil, ErrInvalid
	}
	return f.SolveMatrixWithTolerance(b, f.rankTol)
}

// SolveMatrixWithTolerance applies an explicit relative rank cutoff. It uses
// U_r^T*B, division by retained singular values and V_r, without materializing
// a pseudoinverse. No partial solution escapes on a validation or range error.
func (f *SVD) SolveMatrixWithTolerance(b *Dense, relativeTolerance float64) (x *Dense, residuals []float64, err error) {
	rank, err := f.RankWithTolerance(relativeTolerance)
	if err != nil {
		return nil, nil, err
	}
	if b == nil {
		return nil, nil, ErrInvalid
	}
	if b.rows != f.rows {
		return nil, nil, fmt.Errorf("%w: SVD has %d rows, RHS has %d", ErrDimension, f.rows, b.rows)
	}
	if err := checkFinite(b.data, "SVD solve RHS"); err != nil {
		return nil, nil, err
	}
	if _, err := elementCount(1, b.cols); err != nil {
		return nil, nil, err
	}
	x, err = Zeros(f.cols, b.cols)
	if err != nil {
		return nil, nil, err
	}
	residuals = make([]float64, b.cols)
	if b.cols == 0 {
		return x, residuals, nil
	}
	var (
		k            = len(f.values)
		coefficients = make([]svdBinary, rank)
	)
	for rhs := range b.cols {
		for c := 0; c < rank; c++ {
			var dot svdScaledSum
			for i := range f.rows {
				dot.add(svdQuotient(f.u[i*k+c], b.data[i*b.cols+rhs], 1))
			}
			// Carry the projection's exponent into division, preserving tiny
			// coordinates in a diagonal RHS with widely separated scales.
			coefficients[c] = dot.binary().divide(f.values[c])
		}
		for i := range f.cols {
			var sum svdScaledSum
			for c, coefficient := range coefficients {
				sum.add(coefficient.times(f.v[i*k+c]))
			}
			value := sum.value()
			if !finite(value) {
				return nil, nil, fmt.Errorf("%w: SVD solution (%d,%d)", ErrNonFinite, i, rhs)
			}
			x.data[i*b.cols+rhs] = value
		}
		// The scaled accumulator permits finite residuals even when separate
		// products or intermediate sums overflow. Evaluate original A, not A_r.
		var norm float64
		for i := range f.rows {
			var sum svdScaledSum
			sum.add(svdQuotient(-b.data[i*b.cols+rhs], 1, 1))
			for j := range f.cols {
				sum.add(svdQuotient(f.original[i*f.cols+j], x.data[j*b.cols+rhs], 1))
			}
			norm = math.Hypot(norm, sum.value())
		}
		if !finite(norm) {
			return nil, nil, fmt.Errorf("%w: SVD residual norm for RHS %d", ErrNonFinite, rhs)
		}
		residuals[rhs] = norm
	}
	return x, residuals, nil
}

// svdBinary carries finite mantissa/exponent pairs without materializing an
// overflowing reciprocal or projection. These are float64 computations, not
// arbitrary precision; separated low-order contributions can still round away.
type svdBinary struct {
	frac float64
	exp  int
}

func svdQuotient(a, b, denominator float64) svdBinary {
	if a == 0 || b == 0 {
		return svdBinary{}
	}
	af, ae := math.Frexp(a)
	bf, be := math.Frexp(b)
	df, de := math.Frexp(denominator)
	f, e := math.Frexp((af * bf) / df)
	return svdBinary{frac: f, exp: ae + be - de + e}
}

func (v svdBinary) times(x float64) svdBinary {
	if x == 0 || v.frac == 0 {
		return svdBinary{}
	}
	xf, xe := math.Frexp(x)
	f, e := math.Frexp(v.frac * xf)
	return svdBinary{frac: f, exp: v.exp + xe + e}
}

func (v svdBinary) divide(x float64) svdBinary {
	if v.frac == 0 {
		return svdBinary{}
	}
	xf, xe := math.Frexp(x)
	f, e := math.Frexp(v.frac / xf)
	return svdBinary{frac: f, exp: v.exp - xe + e}
}

type svdScaledSum struct {
	sum, correction float64
	exp             int
	started         bool
}

func (s *svdScaledSum) add(v svdBinary) {
	if v.frac == 0 {
		return
	}
	if !s.started {
		s.sum, s.exp, s.started = v.frac, v.exp, true
		return
	}
	if v.exp > s.exp {
		s.sum = math.Ldexp(s.sum, s.exp-v.exp)
		s.correction = math.Ldexp(s.correction, s.exp-v.exp)
		s.exp = v.exp
	}
	x := math.Ldexp(v.frac, v.exp-s.exp)
	next := s.sum + x
	if math.Abs(s.sum) >= math.Abs(x) {
		s.correction += (s.sum - next) + x
	} else {
		s.correction += (x - next) + s.sum
	}
	s.sum = next
}

func (s *svdScaledSum) value() float64 {
	return math.Ldexp(s.sum+s.correction, s.exp)
}

func (s *svdScaledSum) binary() svdBinary {
	f, e := math.Frexp(s.sum + s.correction)
	return svdBinary{frac: f, exp: s.exp + e}
}
