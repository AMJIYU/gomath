package linalg

import (
	"fmt"
	"math"
)

// SVDOptions controls economy one-sided Jacobi SVD for any rectangular shape.
// All tolerances must be finite and in [0,1). Its zero value permits no sweeps,
// requires exact column orthogonality and uses a zero-only numerical rank cutoff.
type SVDOptions struct {
	MaxSweeps int
	// RelativeTolerance bounds the maximum absolute normalized inner product
	// between different columns of A*V, or A^T*U for wide input.
	RelativeTolerance float64
	// RelativeReconstructionTolerance bounds ||A-U*S*V^T||F/||A||F, checked
	// against original A before success. It also permits a smaller unresolved
	// column to be zeroed if its required rotation is unrepresentable. This is
	// independent of the numerical rank cutoff. Zero disallows such deflation
	// and requires an exactly zero computed reconstruction residual.
	RelativeReconstructionTolerance float64
	// RelativeRankTolerance selects sigma>0 and sigma/sigmaMax>tolerance for
	// Rank, PseudoInverse and Solve. It does not truncate the decomposition.
	// This is a computed-rank policy, not an exact rank or condition estimate.
	RelativeRankTolerance float64
}

// DefaultSVDOptions selects 64 sweeps, 1e-14 orthogonality/rank thresholds and
// a separately checked 1e-12 relative Frobenius reconstruction tolerance.
func DefaultSVDOptions() SVDOptions {
	return SVDOptions{MaxSweeps: 64, RelativeTolerance: 1e-14, RelativeRankTolerance: 1e-14, RelativeReconstructionTolerance: 1e-12}
}

// ReconstructionError describes the measured decomposition residual for the
// original input. It matches ErrReconstruction and never returns partial factors.
type ReconstructionError struct {
	Relative, Tolerance float64
}

func (e *ReconstructionError) Error() string {
	return fmt.Sprintf("%v: relative Frobenius residual %g, tolerance %g", ErrReconstruction, e.Relative, e.Tolerance)
}

func (e *ReconstructionError) Unwrap() error { return ErrReconstruction }

// SVD owns A=U*diag(values)*V^T with economy m-by-k U, n-by-k V, k=min(m,n),
// and nonnegative descending singular values. Shapes for tall inputs are
// unchanged. Zero-value columns are completed with bounded reorthogonalization.
// Numerical rank truncation applies only to inverse/solve operations, not these
// factors. Signs and repeated/zero-value bases are unspecified. U, V and the
// original input copy have independent storage; factors allow concurrent reads.
type SVD struct {
	rows, cols          int
	values              []float64
	u, v                []float64
	original            []float64
	rankTol             float64
	reconstructionError float64
	deflated            int
	ready               bool
}

// FactorizeSVD uses DefaultSVDOptions without modifying a.
func FactorizeSVD(a *Dense) (*SVD, error) {
	return FactorizeSVDWithOptions(a, DefaultSVDOptions())
}

// FactorizeSVDWithOptions rotates A's columns directly, never forming A^T*A.
// Column norms and correlations use scaled entries. Underflowed required
// rotation angles return ErrRange unless the reconstruction policy permits
// disclosed residual-column deflation. Nonfinite results return ErrNonFinite.
// Wide inputs use the same core on A^T, swapping U and V. Zero/rank-deficient
// matrices are valid. Range, budget and validation errors return nil factors.
func FactorizeSVDWithOptions(a *Dense, opt SVDOptions) (*SVD, error) {
	if a == nil {
		return nil, ErrInvalid
	}
	if err := validateJacobi(opt.RelativeTolerance, opt.MaxSweeps); err != nil {
		return nil, err
	}
	if err := validateJacobi(opt.RelativeRankTolerance, 0); err != nil {
		return nil, err
	}
	if err := validateJacobi(opt.RelativeReconstructionTolerance, 0); err != nil {
		return nil, err
	}
	if err := checkFinite(a.data, "SVD input"); err != nil {
		return nil, err
	}
	core := a
	if a.rows < a.cols {
		var err error
		core, err = a.Transpose()
		if err != nil {
			return nil, err
		}
	}
	f, err := factorizeSVDTall(core, opt)
	if err != nil {
		return nil, err
	}
	if a.rows < a.cols {
		f.u, f.v = f.v, f.u
	}
	f.rows, f.cols = a.rows, a.cols
	f.reconstructionError = svdReconstructionResidual(a, f)
	if !finite(f.reconstructionError) {
		return nil, fmt.Errorf("%w: SVD reconstruction check", ErrNonFinite)
	}
	if f.reconstructionError > opt.RelativeReconstructionTolerance {
		return nil, &ReconstructionError{Relative: f.reconstructionError, Tolerance: opt.RelativeReconstructionTolerance}
	}
	f.original = a.Data()
	f.rankTol = opt.RelativeRankTolerance
	return f, nil
}

func factorizeSVDTall(a *Dense, opt SVDOptions) (*SVD, error) {
	var (
		m, n = a.rows, a.cols
		work = a.Data()
	)
	v, err := Identity(n)
	if err != nil {
		return nil, err
	}
	deflated := 0
	for sweep := 0; ; sweep++ {
		residual, err := svdResidual(work, m, n)
		if err != nil {
			return nil, err
		}
		if residual <= opt.RelativeTolerance {
			break
		}
		if sweep == opt.MaxSweeps {
			return nil, &ConvergenceError{Algorithm: "SVD", Sweeps: sweep, MaxSweeps: opt.MaxSweeps, Residual: residual, Tolerance: opt.RelativeTolerance}
		}
		for p := 0; p < n-1; p++ {
			for q := p + 1; q < n; q++ {
				correlation, np, nq, err := columnCorrelation(work, m, n, p, q)
				if err != nil {
					return nil, err
				}
				if math.Abs(correlation) <= opt.RelativeTolerance {
					continue
				}
				var delta, cross float64
				if np >= nq {
					ratio := nq / np
					delta, cross = (ratio*ratio-1)/2, ratio*correlation
				} else {
					ratio := np / nq
					delta, cross = (1-ratio*ratio)/2, ratio*correlation
				}
				c, s, _ := jacobiRotation(delta, cross)
				if cross == 0 || s == 0 {
					if opt.RelativeReconstructionTolerance > 0 && math.Min(np, nq)/math.Max(np, nq) <= opt.RelativeReconstructionTolerance {
						column := p
						if nq < np {
							column = q
						}
						for i := range m {
							work[i*n+column] = 0
						}
						deflated++
						continue
					}
					return nil, fmt.Errorf("%w: unrepresentable SVD rotation (%d,%d)", ErrRange, p, q)
				}
				if err := rotateColumns(work, m, n, p, q, c, s); err != nil {
					return nil, err
				}
				if err := rotateColumns(v.data, n, n, p, q, c, s); err != nil {
					return nil, err
				}
			}
		}
	}
	values := make([]float64, n)
	for j := range n {
		_, _, values[j] = columnNorm(work, m, n, 0, j)
		if !finite(values[j]) {
			return nil, fmt.Errorf("%w: SVD singular value %d", ErrNonFinite, j)
		}
	}
	order := descendingOrder(values)
	f := &SVD{rows: m, cols: n, values: make([]float64, n), u: make([]float64, len(work)), v: orderedColumns(v.data, n, n, order), deflated: deflated, ready: true}
	for j, old := range order {
		f.values[j] = values[old]
		if values[old] == 0 {
			if err := completeSVDColumn(f.u, m, n, j); err != nil {
				return nil, err
			}
			continue
		}
		scale, norm, _ := columnNorm(work, m, n, 0, old)
		for i := range m {
			f.u[i*n+j] = (work[i*n+old] / scale) / norm
		}
	}
	return f, nil
}

// RelativeReconstructionError reports the measured relative Frobenius error
// of the returned, untruncated decomposition against original A (0 for A=0).
// It is computed independently of rank selection. Invalid factors report 0.
func (f *SVD) RelativeReconstructionError() float64 {
	if f == nil || !f.ready {
		return 0
	}
	return f.reconstructionError
}

// DeflatedColumns reports unresolved residual columns explicitly set to zero
// because their required rotation angle was unrepresentable. Success still
// requires the separate original-matrix reconstruction check. Invalid reports 0.
func (f *SVD) DeflatedColumns() int {
	if f == nil || !f.ready {
		return 0
	}
	return f.deflated
}

func svdReconstructionResidual(a *Dense, f *SVD) float64 {
	var scale float64
	for _, x := range a.data {
		scale = math.Max(scale, math.Abs(x))
	}
	if scale == 0 {
		return 0
	}
	var residual, norm float64
	k := len(f.values)
	for i := range f.rows {
		for j := range f.cols {
			var sum float64
			for c, sigma := range f.values {
				sum = math.FMA(f.u[i*k+c]*(sigma/scale), f.v[j*k+c], sum)
			}
			x := a.data[i*f.cols+j] / scale
			residual = math.Hypot(residual, sum-x)
			norm = math.Hypot(norm, x)
		}
	}
	return residual / norm
}

// columnCorrelation computes a compensated dot product of separately scaled,
// normalized columns; no unscaled squares or products of their norms occur.
// A zero column contributes zero correlation and receives an orthogonal
// complement vector only after the nonzero columns have converged.
func columnCorrelation(data []float64, m, n, p, q int) (correlation, np, nq float64, err error) {
	sp, hp, np := columnNorm(data, m, n, 0, p)
	sq, hq, nq := columnNorm(data, m, n, 0, q)
	if !finite(np) || !finite(nq) {
		return 0, 0, 0, fmt.Errorf("%w: SVD column norms", ErrNonFinite)
	}
	if np == 0 || nq == 0 {
		return 0, np, nq, nil
	}
	var correction float64
	for i := range m {
		product := ((data[i*n+p] / sp) / hp) * ((data[i*n+q] / sq) / hq)
		y := product - correction
		sum := correlation + y
		correction = (sum - correlation) - y
		correlation = sum
	}
	return correlation, np, nq, nil
}

func svdResidual(data []float64, m, n int) (float64, error) {
	var residual float64
	for p := range n {
		// Include n=1 and zero columns in nonfinite-norm validation.
		if _, _, norm := columnNorm(data, m, n, 0, p); !finite(norm) {
			return 0, fmt.Errorf("%w: SVD column %d norm", ErrNonFinite, p)
		}
		for q := p + 1; q < n; q++ {
			correlation, _, _, err := columnCorrelation(data, m, n, p, q)
			if err != nil {
				return 0, err
			}
			residual = math.Max(residual, math.Abs(correlation))
		}
	}
	return residual, nil
}

// Dimensions returns the original shape, or (0,0) for invalid factors.
func (f *SVD) Dimensions() (rows, cols int) {
	if f == nil || !f.ready {
		return 0, 0
	}
	return f.rows, f.cols
}

// Values returns a copy of descending singular values, nil for invalid factors.
func (f *SVD) Values() []float64 {
	if f == nil || !f.ready {
		return nil
	}
	return append([]float64(nil), f.values...)
}

// Factors materializes independent m-by-k U and n-by-k V, k=min(m,n).
// V is returned, not V transpose; tall/square factor shapes are unchanged.
// Neither shares storage with the factorization or with the other result.
func (f *SVD) Factors() (u, v *Dense, err error) {
	if f == nil || !f.ready {
		return nil, nil, ErrInvalid
	}
	k := len(f.values)
	return &Dense{rows: f.rows, cols: k, data: append([]float64(nil), f.u...)},
		&Dense{rows: f.cols, cols: k, data: append([]float64(nil), f.v...)}, nil
}

// completeSVDColumn tries at most m coordinate axes, with two modified
// Gram-Schmidt passes per candidate. For j<m, at least one coordinate has an
// orthogonal projection norm >=1/sqrt(m). This deliberately rejects vanishing
// candidates instead of normalizing cancellation noise into a zero vector.
func completeSVDColumn(u []float64, m, k, j int) error {
	work := make([]float64, m)
	threshold := .5 / math.Sqrt(float64(m))
	for candidate := range m {
		clear(work)
		work[candidate] = 1
		for range 2 {
			for col := 0; col < j; col++ {
				var dot float64
				for i := range m {
					dot = math.FMA(u[i*k+col], work[i], dot)
				}
				for i := range m {
					work[i] = math.FMA(-dot, u[i*k+col], work[i])
				}
			}
		}
		var norm float64
		for _, x := range work {
			norm = math.Hypot(norm, x)
		}
		if !finite(norm) || norm < threshold {
			continue
		}
		for i := range m {
			work[i] /= norm
		}
		valid := true
		for col := 0; col < j; col++ {
			var dot float64
			for i := range m {
				dot = math.FMA(u[i*k+col], work[i], dot)
			}
			valid = valid && math.Abs(dot) <= 1e-14
		}
		if valid {
			for i := range m {
				u[i*k+j] = work[i]
			}
			return nil
		}
	}
	return fmt.Errorf("%w: SVD zero-column %d orthogonal completion exhausted %d coordinate candidates", ErrConvergence, j, m)
}
