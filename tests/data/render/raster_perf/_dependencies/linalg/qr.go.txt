package linalg

import (
	"fmt"
	"math"
)

// QROptions controls computed column-rank rejection. Zero selects exact-zero
// checking; use DefaultQROptions for the default relative threshold.
type QROptions struct {
	// RelativeRankTolerance must be finite and in [0,1). At each step the
	// largest remaining column-tail norm is compared with the largest original
	// column norm. This is a rank heuristic, not a condition estimate or SVD.
	RelativeRankTolerance float64
}

// DefaultQROptions returns a relative rank threshold of 1e-14.
func DefaultQROptions() QROptions { return QROptions{RelativeRankTolerance: 1e-14} }

// RankError describes a rejected QR step. Rank is the number of accepted
// columns, not an exact mathematical rank. Norm is the largest remaining tail
// norm; Scale is the largest original column norm. It matches ErrRankDeficient.
type RankError struct {
	Rank, Columns         int
	Norm, Scale           float64
	RelativeRankTolerance float64
}

func (e *RankError) Error() string {
	return fmt.Sprintf("%v: accepted %d of %d columns, tail norm %g, scale %g, tolerance %g", ErrRankDeficient, e.Rank, e.Columns, e.Norm, e.Scale, e.RelativeRankTolerance)
}

func (e *RankError) Unwrap() error { return ErrRankDeficient }

// LeastSquaresOptions optionally rejects a fit with an excessive actual
// residual. A nonzero least-squares residual is normally valid, so the zero
// value disables rejection. Residual norms are returned with successful solves.
type LeastSquaresOptions struct {
	CheckResidual bool
	// RelativeResidualTolerance must be finite and nonnegative, even if the
	// check is disabled. When enabled, require ||A*x-b||2/||b||2 <= tolerance
	// independently for each RHS. Zero requires a computed zero residual.
	RelativeResidualTolerance float64
}

// ResidualError describes the first RHS rejected by an enabled residual check.
// RHS is a zero-based column index (zero for vector solves).
type ResidualError struct {
	RHS                       int
	Norm, Relative, Tolerance float64
}

func (e *ResidualError) Error() string {
	return fmt.Sprintf("%v: RHS %d norm %g, relative %g, tolerance %g", ErrResidual, e.RHS, e.Norm, e.Relative, e.Tolerance)
}

func (e *ResidualError) Unwrap() error { return ErrResidual }

// QR owns immutable compact Householder factors with column pivoting, plus an
// independent original-matrix copy for actual residual evaluation. It satisfies
// A*P=Q*R, with thin Q (m-by-n) and square upper-triangular R. Only m>=n,
// full-column-rank inputs are supported; no rank-deficient or minimum-norm
// fallback is used. Completed factors are safe for concurrent reuse.
type QR struct {
	rows, cols int
	data       []float64
	original   []float64
	tau        []float64
	perm       []int
	ready      bool
}

// FactorizeQR uses DefaultQROptions and never modifies a.
func FactorizeQR(a *Dense) (*QR, error) { return FactorizeQRWithOptions(a, DefaultQROptions()) }

// FactorizeQRWithOptions factors an m-by-n matrix with m>=n using column-pivoted
// Householder QR. Remaining tail norms are recomputed at every step. Scaled
// norms and reflector construction avoid squaring tiny/huge entries or forming
// an overflowing alpha-beta. A failed rank/arithmetic check returns nil.
func FactorizeQRWithOptions(a *Dense, opt QROptions) (*QR, error) {
	if a == nil {
		return nil, ErrInvalid
	}
	if a.rows < a.cols {
		return nil, fmt.Errorf("%w: QR least squares requires rows >= columns, got %dx%d", ErrDimension, a.rows, a.cols)
	}
	tol := opt.RelativeRankTolerance
	if !finite(tol) || tol < 0 || tol >= 1 {
		return nil, fmt.Errorf("%w: QR rank tolerance %g, need [0,1)", ErrTolerance, tol)
	}
	f := &QR{rows: a.rows, cols: a.cols, data: a.Data(), original: a.Data(), tau: make([]float64, a.cols), perm: make([]int, a.cols)}
	for i := range f.perm {
		f.perm[i] = i
	}
	var rankScale float64
	for k := range f.cols {
		pivot, bestNorm := k, 0.0
		for col := k; col < f.cols; col++ {
			_, _, norm := columnNorm(f.data, f.rows, f.cols, k, col)
			if !finite(norm) {
				return nil, fmt.Errorf("%w: QR column %d norm at step %d", ErrNonFinite, col, k)
			}
			if norm > bestNorm {
				pivot, bestNorm = col, norm
			}
		}
		if k == 0 {
			rankScale = bestNorm
		}
		if bestNorm == 0 || (tol > 0 && bestNorm/rankScale <= tol) {
			return nil, &RankError{Rank: k, Columns: f.cols, Norm: bestNorm, Scale: rankScale, RelativeRankTolerance: tol}
		}
		if pivot != k {
			for row := range f.rows {
				f.data[row*f.cols+k], f.data[row*f.cols+pivot] = f.data[row*f.cols+pivot], f.data[row*f.cols+k]
			}
			f.perm[k], f.perm[pivot] = f.perm[pivot], f.perm[k]
		}
		scale, scaledNorm, _ := columnNorm(f.data, f.rows, f.cols, k, k)
		alpha := f.data[k*f.cols+k]
		var hasTail bool
		for row := k + 1; row < f.rows; row++ {
			hasTail = hasTail || f.data[row*f.cols+k] != 0
		}
		if !hasTail {
			continue // Identity reflector, including a last scalar pivot.
		}
		var (
			scaledAlpha = alpha / scale
			scaledBeta  = -math.Copysign(scaledNorm, alpha)
			denominator = scaledAlpha - scaledBeta
		)
		f.tau[k] = 1 - scaledAlpha/scaledBeta
		for row := k + 1; row < f.rows; row++ {
			f.data[row*f.cols+k] = (f.data[row*f.cols+k] / scale) / denominator
		}
		f.data[k*f.cols+k] = scaledBeta * scale
		if err := applyReflector(f.data, f.rows, f.cols, k, f.tau[k], f.data, f.cols, k+1); err != nil {
			return nil, err
		}
	}
	f.ready = true
	return f, nil
}

// columnNorm returns max-entry scale, the norm after dividing by that scale,
// and their product. The product may be Inf; callers must check it. The scaled
// norm never squares unscaled entries, preserving nonzero subnormal columns.
func columnNorm(data []float64, rows, cols, start, col int) (scale, scaledNorm, norm float64) {
	for row := start; row < rows; row++ {
		scale = math.Max(scale, math.Abs(data[row*cols+col]))
	}
	if scale == 0 {
		return 0, 0, 0
	}
	for row := start; row < rows; row++ {
		scaledNorm = math.Hypot(scaledNorm, data[row*cols+col]/scale)
	}
	return scale, scaledNorm, scale * scaledNorm
}

// applyReflector applies I-tau*v*v^T, v[k]=1, to destination columns from
// firstCol onward. Rescaling each target column prevents an unnecessary dot
// overflow when the reflected entries themselves are representable.
func applyReflector(packed []float64, rows, cols, k int, tau float64, dst []float64, dstCols, firstCol int) error {
	if tau == 0 {
		return nil
	}
	for col := firstCol; col < dstCols; col++ {
		var scale float64
		for row := k; row < rows; row++ {
			scale = math.Max(scale, math.Abs(dst[row*dstCols+col]))
		}
		if scale == 0 {
			continue
		}
		dot := dst[k*dstCols+col] / scale
		for row := k + 1; row < rows; row++ {
			dot = math.FMA(packed[row*cols+k], dst[row*dstCols+col]/scale, dot)
		}
		weight := tau * dot
		for row := k; row < rows; row++ {
			v := 1.0
			if row != k {
				v = packed[row*cols+k]
			}
			if v == 0 {
				continue // Preserve small entries in unaffected coordinates.
			}
			i := row*dstCols + col
			value := math.FMA(-v, weight, dst[i]/scale) * scale
			if !finite(value) {
				return fmt.Errorf("%w: Householder result (%d,%d) at step %d", ErrNonFinite, row, col, k)
			}
			dst[i] = value
		}
	}
	return nil
}

// Dimensions returns the coefficient-matrix shape; invalid factors report 0,0.
func (f *QR) Dimensions() (rows, cols int) {
	if f == nil || !f.ready {
		return 0, 0
	}
	return f.rows, f.cols
}

// Pivots copies the final column permutation: column j of A*P is original
// column Pivots()[j]. Invalid factors return nil.
func (f *QR) Pivots() []int {
	if f == nil || !f.ready {
		return nil
	}
	return append([]int(nil), f.perm...)
}

// Factors materializes independent thin Q and upper R, satisfying A*P=Q*R.
// Q has orthonormal columns; a full m-by-m Q is not materialized.
func (f *QR) Factors() (q, r *Dense, err error) {
	if f == nil || !f.ready {
		return nil, nil, ErrInvalid
	}
	q, err = Zeros(f.rows, f.cols)
	if err != nil {
		return nil, nil, err
	}
	r, err = Zeros(f.cols, f.cols)
	if err != nil {
		return nil, nil, err
	}
	for i := range f.cols {
		q.data[i*f.cols+i] = 1
		copy(r.data[i*f.cols+i:(i+1)*f.cols], f.data[i*f.cols+i:(i+1)*f.cols])
	}
	for k := f.cols - 1; k >= 0; k-- {
		if err := applyReflector(f.data, f.rows, f.cols, k, f.tau[k], q.data, f.cols, 0); err != nil {
			return nil, nil, err
		}
	}
	return q, r, nil
}

// Solve minimizes ||A*x-b||2 and returns x and the actual computed residual
// norm ||A*x-b||2. Rank rejection occurs at factorization time, never by
// silently discarding dependent columns. Nonzero residuals are valid fits.
func (f *QR) Solve(b []float64) (x []float64, residual float64, err error) {
	return f.SolveWithOptions(b, LeastSquaresOptions{})
}

// SolveWithOptions additionally applies the caller's residual-limit policy.
// Any error, including a rejected fit, returns nil x and zero residual.
func (f *QR) SolveWithOptions(b []float64, opt LeastSquaresOptions) (x []float64, residual float64, err error) {
	if f == nil || !f.ready {
		return nil, 0, ErrInvalid
	}
	if len(b) != f.rows {
		return nil, 0, fmt.Errorf("%w: QR needs RHS length %d, got %d", ErrDimension, f.rows, len(b))
	}
	if err := checkFinite(b, "QR solve RHS"); err != nil {
		return nil, 0, err
	}
	// Borrow the read-only caller slice only for this call, just as MulVec does.
	xm, norms, err := f.SolveMatrixWithOptions(&Dense{rows: f.rows, cols: 1, data: b}, opt)
	if err != nil {
		return nil, 0, err
	}
	return xm.data, norms[0], nil
}

// SolveMatrix minimizes each column's ||A*X-B||2, returning an n-by-r matrix
// and r actual residual norms. B and the factorization are never modified.
func (f *QR) SolveMatrix(b *Dense) (x *Dense, residuals []float64, err error) {
	return f.SolveMatrixWithOptions(b, LeastSquaresOptions{})
}

// SolveMatrixWithOptions applies the fit limit independently per RHS. A failed
// check returns nil outputs and a *ResidualError identifying the first failure.
func (f *QR) SolveMatrixWithOptions(b *Dense, opt LeastSquaresOptions) (x *Dense, residuals []float64, err error) {
	if f == nil || !f.ready || b == nil {
		return nil, nil, ErrInvalid
	}
	if b.rows != f.rows {
		return nil, nil, fmt.Errorf("%w: QR has %d rows, RHS has %d", ErrDimension, f.rows, b.rows)
	}
	if !finite(opt.RelativeResidualTolerance) || opt.RelativeResidualTolerance < 0 {
		return nil, nil, fmt.Errorf("%w: residual tolerance must be finite and nonnegative", ErrTolerance)
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
	work := b.Data()
	for k := range f.cols {
		if err := applyReflector(f.data, f.rows, f.cols, k, f.tau[k], work, b.cols, 0); err != nil {
			return nil, nil, err
		}
	}
	for i := f.cols - 1; i >= 0; i-- {
		for col := range b.cols {
			value := work[i*b.cols+col]
			for j := i + 1; j < f.cols; j++ {
				value = math.FMA(-f.data[i*f.cols+j], work[j*b.cols+col], value)
			}
			value /= f.data[i*f.cols+i]
			if !finite(value) {
				return nil, nil, fmt.Errorf("%w: QR back substitution (%d,%d)", ErrNonFinite, i, col)
			}
			work[i*b.cols+col] = value
		}
	}
	for i, original := range f.perm {
		copy(x.data[original*b.cols:(original+1)*b.cols], work[i*b.cols:(i+1)*b.cols])
	}
	for col := range b.cols {
		var norm float64
		for row := range f.rows {
			value := -b.data[row*b.cols+col]
			for j := range f.cols {
				value = math.FMA(f.original[row*f.cols+j], x.data[j*b.cols+col], value)
			}
			if !finite(value) {
				return nil, nil, fmt.Errorf("%w: QR residual (%d,%d)", ErrNonFinite, row, col)
			}
			norm = math.Hypot(norm, value)
		}
		if !finite(norm) {
			return nil, nil, fmt.Errorf("%w: QR residual norm for RHS %d", ErrNonFinite, col)
		}
		residuals[col] = norm
		if opt.CheckResidual {
			// Normalize by the largest RHS entry before taking its norm, so
			// ||b||2 itself need not fit in a float64 to compare the ratio.
			scale, scaledNorm, _ := columnNorm(b.data, b.rows, b.cols, 0, col)
			relative := 0.0
			if norm != 0 {
				if scale == 0 {
					relative = math.Inf(1)
				} else {
					relative = norm / scale
					if math.IsInf(relative, 0) {
						relative = (norm / scaledNorm) / scale
					} else {
						relative /= scaledNorm
					}
				}
			}
			if (opt.RelativeResidualTolerance == 0 && norm != 0) || relative > opt.RelativeResidualTolerance {
				return nil, nil, &ResidualError{RHS: col, Norm: norm, Relative: relative, Tolerance: opt.RelativeResidualTolerance}
			}
		}
	}
	return x, residuals, nil
}

// LeastSquares factors a with the default QR rank policy and fits one RHS.
func LeastSquares(a *Dense, b []float64) (x []float64, residual float64, err error) {
	f, err := FactorizeQR(a)
	if err != nil {
		return nil, 0, err
	}
	return f.Solve(b)
}

// LeastSquaresMatrix is the one-shot, multi-RHS least-squares entry point.
func LeastSquaresMatrix(a, b *Dense) (x *Dense, residuals []float64, err error) {
	f, err := FactorizeQR(a)
	if err != nil {
		return nil, nil, err
	}
	return f.SolveMatrix(b)
}
