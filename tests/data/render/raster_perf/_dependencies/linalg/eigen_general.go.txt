package linalg

import (
	"errors"
	"fmt"
	"math"
	"math/cmplx"
)

// ErrEigenLimit matches exhaustion of the combined Schur/eigenvector work bound.
var ErrEigenLimit = errors.New("linalg: general eigenvector resource limit exceeded")

// EigenOptions controls general real-matrix right eigenvectors. Begin with
// DefaultEigenOptions; WithOptions treats zero tolerances and Schur fields
// literally. Tolerances must be finite in [0,1). MaxWork=0 selects 512M units.
// MaxWork includes a conservative vector-stage reserve and the Schur budget;
// Schur.MaxWork and Schur.MaxSize remain additional independent limits.
// RelativeResidualTolerance bounds ||A*v-lambda*v||2/((||A||F+|lambda|)*||v||2).
// RelativeBasisTolerance is the relative pivot threshold for rank-revealing
// two-pass modified Gram-Schmidt of the normalized output columns. It does NOT
// certify exact diagonalizability or the true geometric multiplicities.
// RequireIndependent rejects a numerical column rank below the matrix order.
type EigenOptions struct {
	Schur                     SchurOptions
	MaxWork                   uint64
	RelativeResidualTolerance float64
	RelativeBasisTolerance    float64
	RequireIndependent        bool
}

// DefaultEigenOptions selects default Schur policies, 512M combined work units,
// 1e-12 relative residual tolerance and 1e-10 numerical basis tolerance.
func DefaultEigenOptions() EigenOptions {
	return EigenOptions{Schur: DefaultSchurOptions(), MaxWork: 512 << 20,
		RelativeResidualTolerance: 1e-12, RelativeBasisTolerance: 1e-10}
}

// EigenResidualError identifies an unacceptable original-input eigenpair.
// It matches ErrResidual. No partial result is returned.
type EigenResidualError struct {
	Column          int
	Residual, Limit float64
}

func (e *EigenResidualError) Error() string {
	return fmt.Sprintf("linalg: eigenvector column %d residual %g exceeds %g", e.Column, e.Residual, e.Limit)
}
func (e *EigenResidualError) Unwrap() error { return ErrResidual }

// Eigen owns normalized complex right eigenvector columns, NOT Schur vectors.
// Values follow unsorted Schur block order. A positive-imaginary eigenvalue is
// immediately followed by its conjugate, with exactly conjugate vector columns.
// Each column has Euclidean norm one; its first largest-magnitude component is
// real and nonnegative (up to ties/rounding). Signs, phases and bases for repeated
// roots are not mathematical invariants and may change under perturbations.
//
// A defective/ill-conditioned input may produce dependent columns. BasisRank
// reports the numerical rank of THESE columns, not an eigenspace dimension.
// Rank deficiency does not supply a complete eigenbasis or generalized vectors.
// Even full numerical rank is not a proof of exact diagonalizability.
// Completed results are immutable and safe for concurrent readers. Zero is invalid.
type Eigen struct {
	n, rank, iterations int
	values, vectors     []complex128 // vectors are column-major
	residuals           []float64
	regularized         []bool
	basisTolerance      float64
	ready               bool
}

// FactorizeEigen computes general complex right eigenvectors with defaults.
func FactorizeEigen(a *Dense) (*Eigen, error) {
	return FactorizeEigenWithOptions(a, DefaultEigenOptions())
}

// FactorizeEigenWithOptions computes real Schur factors, solves their 1x1/2x2
// blocks backwards using exponent-carrying complex arithmetic, and applies Q.
// Exact singular pivots with a nonzero right side are replaced by epsilon times
// the Schur max-entry scale, and reported by RegularizedColumns.
// Zero singular right sides select zero free components. Nonzero tiny pivots
// are not floored. Every exported column is verified against the original A.
// Scaling/range, convergence, work, residual or required-rank failure returns nil.
// This is not a left/generalized eigenvector or diagonalization API.
func FactorizeEigenWithOptions(a *Dense, opt EigenOptions) (*Eigen, error) {
	if a == nil {
		return nil, ErrInvalid
	}
	if a.rows != a.cols {
		return nil, fmt.Errorf("%w: eigenvectors need square input", ErrDimension)
	}
	for _, tol := range []float64{opt.RelativeResidualTolerance, opt.RelativeBasisTolerance} {
		if !finite(tol) || tol < 0 || tol >= 1 {
			return nil, fmt.Errorf("%w: eigen tolerance needs [0,1)", ErrTolerance)
		}
	}
	// Validate/cap order before evaluating the cubic reserve (also on 32-bit).
	maxSize := opt.Schur.MaxSize
	if maxSize == 0 {
		maxSize = 256
	}
	if maxSize < 1 || maxSize > 1024 {
		return nil, fmt.Errorf("%w: eigen Schur MaxSize needs 1..1024", ErrBudget)
	}
	if a.rows > maxSize {
		return nil, fmt.Errorf("%w: %w: order %d exceeds %d", ErrEigenLimit, ErrSchurLimit, a.rows, maxSize)
	}
	if opt.MaxWork == 0 {
		opt.MaxWork = 512 << 20
	}
	n := a.rows
	size := uint64(n)
	reserve := 128*size*size*size + 64*size*size + 32*size
	if reserve >= opt.MaxWork && n != 0 {
		return nil, fmt.Errorf("%w: vector-stage reserve", ErrEigenLimit)
	}
	remaining := opt.MaxWork - reserve
	schurOpt := opt.Schur
	if schurOpt.MaxWork == 0 {
		schurOpt.MaxWork = 256 << 20
	}
	if schurOpt.MaxWork > remaining {
		schurOpt.MaxWork = remaining
	}
	s, err := FactorizeSchurWithOptions(a, schurOpt)
	if err != nil {
		if errors.Is(err, ErrSchurLimit) {
			return nil, fmt.Errorf("%w: %w", ErrEigenLimit, err)
		}
		return nil, err
	}
	f := &Eigen{n: n, iterations: s.iterations, values: s.Values(), vectors: make([]complex128, n*n),
		residuals: make([]float64, n), regularized: make([]bool, n), basisTolerance: opt.RelativeBasisTolerance}
	scale := 0.0
	for _, x := range s.t {
		scale = math.Max(scale, math.Abs(x))
	}
	floor := eigenReal(scale)
	floor.exp -= 52 // kept in exponent form even below the subnormal float range
	x := make([]eigenComplex, n)
	y := make([]eigenComplex, n)
	for col := 0; col < n; col++ {
		clear(x)
		end := col
		lambda := eigenScalar(f.values[col])
		if imag(f.values[col]) != 0 {
			end++
			x[col], x[end] = eigenSeed(s.t, n, col, lambda)
		} else {
			x[col] = eigenScalar(1)
		}
		regularized := false
		for row := col - 1; row >= 0; {
			first := row
			if row > 0 && s.t[row*n+row-1] != 0 {
				first--
			}
			var rhs [2]eigenComplex
			for k := first; k <= row; k++ {
				var sum eigenSum
				for j := row + 1; j <= end; j++ {
					sum.add(x[j].mul(eigenScalar(complex(s.t[k*n+j], 0))))
				}
				rhs[k-first] = sum.value().neg()
			}
			if first == row {
				pivot := eigenScalar(complex(s.t[row*n+row], 0)).sub(lambda)
				if pivot.zero() {
					if rhs[0].zero() {
						x[row] = eigenComplex{}
						row--
						continue
					}
					pivot = eigenComplex{re: floor}
					regularized = true
				}
				x[row] = rhs[0].div(pivot)
			} else {
				aa := eigenScalar(complex(s.t[first*n+first], 0)).sub(lambda)
				bb := eigenScalar(complex(s.t[first*n+row], 0))
				cc := eigenScalar(complex(s.t[row*n+first], 0))
				dd := eigenScalar(complex(s.t[row*n+row], 0)).sub(lambda)
				u, v, perturbed := eigenSolvePair(aa, bb, cc, dd, rhs[0], rhs[1], floor)
				x[first], x[row] = u, v
				regularized = regularized || perturbed
			}
			row = first - 1
		}
		for i := range n {
			var sum eigenSum
			for j := 0; j <= end; j++ {
				sum.add(x[j].mul(eigenScalar(complex(s.q[i*n+j], 0))))
			}
			y[i] = sum.value()
		}
		vector, err := eigenNormalize(y)
		if err != nil {
			return nil, err
		}
		copy(f.vectors[col*n:(col+1)*n], vector)
		f.regularized[col] = regularized
		if end != col {
			for i, v := range vector {
				f.vectors[end*n+i] = cmplx.Conj(v)
			}
			f.regularized[end] = regularized
			col = end
		}
	}
	// Check each actual output independently, including conjugate columns.
	for col := range n {
		residual := eigenCheck(a.data, n, f.values[col], f.vectors[col*n:(col+1)*n])
		if !finite(residual) {
			return nil, fmt.Errorf("%w: eigen residual", ErrRange)
		}
		if residual > opt.RelativeResidualTolerance {
			return nil, &EigenResidualError{col, residual, opt.RelativeResidualTolerance}
		}
		f.residuals[col] = residual
	}
	f.rank = eigenColumnRank(f.vectors, n, opt.RelativeBasisTolerance)
	if opt.RequireIndependent && f.rank < n {
		return nil, fmt.Errorf("%w: eigenvector numerical column rank %d/%d at tolerance %g", ErrRankDeficient, f.rank, n, opt.RelativeBasisTolerance)
	}
	f.ready = true
	return f, nil
}

// Size returns the input order, or zero for an invalid factor.
func (f *Eigen) Size() int {
	if f == nil || !f.ready {
		return 0
	}
	return f.n
}

// Values returns an independent complex eigenvalue copy, or nil if invalid.
func (f *Eigen) Values() []complex128 {
	if f == nil || !f.ready {
		return nil
	}
	return append([]complex128(nil), f.values...)
}

// RightVector copies one normalized right eigenvector, indexed as Values.
func (f *Eigen) RightVector(column int) ([]complex128, error) {
	if f == nil || !f.ready {
		return nil, ErrInvalid
	}
	if column < 0 || column >= f.n {
		return nil, fmt.Errorf("%w: eigenvector column %d", ErrDimension, column)
	}
	return append([]complex128(nil), f.vectors[column*f.n:(column+1)*f.n]...), nil
}

// Residuals copies the checked original-input relative residual of every column.
func (f *Eigen) Residuals() []float64 {
	if f == nil || !f.ready {
		return nil
	}
	return append([]float64(nil), f.residuals...)
}

// BasisRank returns the numerical rank of the returned columns (zero if invalid).
func (f *Eigen) BasisRank() int {
	if f == nil || !f.ready {
		return 0
	}
	return f.rank
}

// BasisTolerance returns the rank threshold used (zero if invalid).
func (f *Eigen) BasisTolerance() float64 {
	if f == nil || !f.ready {
		return 0
	}
	return f.basisTolerance
}

// RegularizedColumns copies flags for exact-singular nonzero-right-side solves.
// Such columns satisfy the requested residual policy, not an exact equation.
func (f *Eigen) RegularizedColumns() []bool {
	if f == nil || !f.ready {
		return nil
	}
	return append([]bool(nil), f.regularized...)
}

// SchurIterations returns the underlying total implicit QR sweeps.
func (f *Eigen) SchurIterations() int {
	if f == nil || !f.ready {
		return 0
	}
	return f.iterations
}
