package linalg

import (
	"fmt"
	"math"
)

// EigenSymOptions controls real symmetric cyclic Jacobi iteration. Tolerances
// must be finite and in [0,1). Zero MaxSweeps allows only the initial convergence
// check; zero RelativeTolerance requires exactly zero off-diagonal entries.
type EigenSymOptions struct {
	MaxSweeps int
	// RelativeTolerance bounds max(abs(off-diagonal))/max(abs(all entries))
	// of the iterated matrix, not the relative accuracy of each eigenvalue.
	RelativeTolerance float64
	// RelativeSymmetryTolerance uses the same pairwise policy as Cholesky.
	// Accepted unequal entries are averaged before iteration.
	RelativeSymmetryTolerance float64
}

// DefaultEigenSymOptions selects 64 sweeps, 1e-14 convergence tolerance and
// exact input symmetry. It does not promise relative accuracy of tiny values.
func DefaultEigenSymOptions() EigenSymOptions {
	return EigenSymOptions{MaxSweeps: 64, RelativeTolerance: 1e-14}
}

// EigenSym owns real eigenvalues in descending algebraic order and independent
// orthonormal eigenvector columns: A=V*diag(values)*V^T. For repeated eigenvalues
// the basis and its signs are unspecified. Completed factors are immutable and
// safe for concurrent readers; the zero value is uninitialized.
type EigenSym struct {
	n       int
	values  []float64
	vectors []float64
	ready   bool
}

// FactorizeEigenSym factors a real symmetric matrix with default options.
func FactorizeEigenSym(a *Dense) (*EigenSym, error) {
	return FactorizeEigenSymWithOptions(a, DefaultEigenSymOptions())
}

// FactorizeEigenSymWithOptions uses bounded cyclic Jacobi rotations without
// modifying a. Nonsquare, nonsymmetric, nonfinite, arithmetic-range and budget
// failures return nil factors. Indefinite and rank-deficient inputs are valid.
func FactorizeEigenSymWithOptions(a *Dense, opt EigenSymOptions) (*EigenSym, error) {
	if a == nil {
		return nil, ErrInvalid
	}
	if a.rows != a.cols {
		return nil, fmt.Errorf("%w: symmetric eigendecomposition needs square input", ErrDimension)
	}
	if err := validateJacobi(opt.RelativeTolerance, opt.MaxSweeps); err != nil {
		return nil, err
	}
	if err := validateJacobi(opt.RelativeSymmetryTolerance, 0); err != nil {
		return nil, err
	}
	if err := checkFinite(a.data, "symmetric eigen input"); err != nil {
		return nil, err
	}
	var (
		n    = a.rows
		work = a.Data()
	)
	for i := range n {
		for j := 0; j < i; j++ {
			lower, upper := work[i*n+j], work[j*n+i]
			if lower == upper {
				continue
			}
			scale := math.Max(math.Abs(lower), math.Abs(upper))
			relative := math.Abs(lower/scale - upper/scale)
			if opt.RelativeSymmetryTolerance == 0 || relative > opt.RelativeSymmetryTolerance {
				return nil, &SymmetryError{Row: i, Column: j, Lower: lower, Upper: upper, Relative: relative, Tolerance: opt.RelativeSymmetryTolerance}
			}
			var average float64
			if scale > math.MaxFloat64/2 {
				average = lower/2 + upper/2
			} else {
				average = (lower + upper) / 2
			}
			work[i*n+j], work[j*n+i] = average, average
		}
	}
	v, err := Identity(n)
	if err != nil {
		return nil, err
	}
	for sweep := 0; ; sweep++ {
		residual, scale := eigenResidual(work, n)
		if residual <= opt.RelativeTolerance {
			break
		}
		if sweep == opt.MaxSweeps {
			return nil, &ConvergenceError{Algorithm: "EigenSym", Sweeps: sweep, MaxSweeps: opt.MaxSweeps, Residual: residual, Tolerance: opt.RelativeTolerance}
		}
		for p := 0; p < n-1; p++ {
			for q := p + 1; q < n; q++ {
				var (
					pp, qq, pq = p*n + p, q*n + q, p*n + q
					cross      = work[pq]
				)
				if cross == 0 || (opt.RelativeTolerance > 0 && math.Abs(cross)/scale <= opt.RelativeTolerance) {
					continue
				}
				pairScale := math.Max(math.Abs(cross), math.Max(math.Abs(work[pp]), math.Abs(work[qq])))
				if cross/pairScale == 0 {
					return nil, fmt.Errorf("%w: unrepresentable eigen pair (%d,%d)", ErrRange, p, q)
				}
				c, s, tangent := jacobiRotation((work[qq]/pairScale-work[pp]/pairScale)/2, cross/pairScale)
				if s == 0 {
					return nil, fmt.Errorf("%w: unrepresentable eigen rotation (%d,%d)", ErrRange, p, q)
				}
				work[pp], work[qq] = math.FMA(-tangent, cross, work[pp]), math.FMA(tangent, cross, work[qq])
				if !finite(work[pp]) || !finite(work[qq]) {
					return nil, fmt.Errorf("%w: eigen diagonal update", ErrNonFinite)
				}
				work[pq], work[q*n+p] = 0, 0
				for k := range n {
					if k == p || k == q {
						continue
					}
					x, y := work[k*n+p], work[k*n+q]
					x, y = math.FMA(c, x, -s*y), math.FMA(s, x, c*y)
					if !finite(x) || !finite(y) {
						return nil, fmt.Errorf("%w: symmetric Jacobi update", ErrNonFinite)
					}
					work[k*n+p], work[p*n+k] = x, x
					work[k*n+q], work[q*n+k] = y, y
				}
				if err := rotateColumns(v.data, n, n, p, q, c, s); err != nil {
					return nil, err
				}
			}
		}
	}
	values := make([]float64, n)
	for i := range n {
		values[i] = work[i*n+i]
	}
	order := descendingOrder(values)
	f := &EigenSym{n: n, values: make([]float64, n), vectors: orderedColumns(v.data, n, n, order), ready: true}
	for i, old := range order {
		f.values[i] = values[old]
	}
	return f, nil
}

func eigenResidual(data []float64, n int) (relative, scale float64) {
	var off float64
	for i := range n {
		for j := range n {
			x := math.Abs(data[i*n+j])
			scale = math.Max(scale, x)
			if i != j {
				off = math.Max(off, x)
			}
		}
	}
	if off == 0 {
		return 0, scale
	}
	// Preserve a nonzero residual when the ratio underflows (zero tolerance).
	return math.Max(off/scale, math.SmallestNonzeroFloat64), scale
}

// Size returns the square input size, or zero for invalid factors.
func (f *EigenSym) Size() int {
	if f == nil || !f.ready {
		return 0
	}
	return f.n
}

// Values returns a copy of descending eigenvalues, or nil for invalid factors.
func (f *EigenSym) Values() []float64 {
	if f == nil || !f.ready {
		return nil
	}
	return append([]float64(nil), f.values...)
}

// Vectors materializes independent eigenvector columns, in Values order.
func (f *EigenSym) Vectors() (*Dense, error) {
	if f == nil || !f.ready {
		return nil, ErrInvalid
	}
	return &Dense{rows: f.n, cols: f.n, data: append([]float64(nil), f.vectors...)}, nil
}
