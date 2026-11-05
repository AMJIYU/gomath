package sparse

import (
	"context"
	"fmt"
	"math"
	"math/big"
	"math/bits"
)

// Preconditioner is an explicit choice; Jacobi requires positive diagonal
// entries and never changes the original matrix used for residual validation.
type Preconditioner uint8

const (
	NoPreconditioner Preconditioner = iota
	Jacobi
)

// CGOptions bounds the solve. Zero MaxIterations selects 1000 (hard cap 100000).
// Tolerances are literal, finite, nonnegative values, including zero; use
// DefaultCGOptions for relative tolerance 1e-10. InitialGuess is copied.
// Stop only when ||b-A*x||2 <= max(AbsoluteTolerance, RelativeTolerance*||b||2).
// Both tolerances zero request an exactly zero computed original residual.
type CGOptions struct {
	Limits                               Limits
	MaxIterations                        int
	RelativeTolerance, AbsoluteTolerance float64
	Preconditioner                       Preconditioner
	InitialGuess                         []float64
}

func DefaultCGOptions() CGOptions { return CGOptions{MaxIterations: 1000, RelativeTolerance: 1e-10} }

// CGResult is immutable. No result is returned on any error. It does not certify
// positive definiteness, conditioning, forward error or a complete factorization.
type CGResult struct {
	x                []float64
	iterations       int
	residualFraction float64
	residualExponent int
	relativeResidual float64
}

func (r *CGResult) Solution() []float64 {
	if r == nil {
		return nil
	}
	return append([]float64(nil), r.x...)
}
func (r *CGResult) Iterations() int {
	if r == nil {
		return 0
	}
	return r.iterations
}

// ResidualNorm is ||b-A*x||2 as fraction*2^exponent, avoiding norm overflow.
func (r *CGResult) ResidualNorm() (fraction float64, exponent int) {
	if r == nil {
		return 0, 0
	}
	return r.residualFraction, r.residualExponent
}

// RelativeResidual is ||b-A*x||2/||b||2. For a zero RHS it is zero if the
// residual is zero, and +Inf otherwise (an absolute-tolerance-only success).
func (r *CGResult) RelativeResidual() float64 {
	if r == nil {
		return 0
	}
	return r.relativeResidual
}

type solveBudget struct {
	ctx       context.Context
	remaining uint64
}

func (b *solveBudget) spend(n uint64) error {
	if e := canceled(b.ctx); e != nil {
		return e
	}
	if n > b.remaining {
		return ErrLimit
	}
	b.remaining -= n
	return nil
}

// SolveCG solves a real square system by bounded preconditioned conjugate
// gradients. A must be SPD: exact symmetry is checked, positive definiteness is
// a caller prerequisite, not proved. Only encountered curvature is tested.
// Recurrence coefficients use 53-bit significands with extended exponents;
// original residual product sums retain exact binary64-product accumulation.
// Every candidate's original residual is checked. Residual replacement restarts
// directions every 32 iterations or when a recurrence residual vanishes.
// ctx must be nonnil. Cancellation and all failures return a nil result.
func SolveCG(ctx context.Context, a *CSR, b []float64, options CGOptions) (*CGResult, error) {
	if ctx == nil {
		return nil, ErrOptions
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	l, e := options.Limits.normalized()
	if e != nil {
		return nil, e
	}
	if e = a.check(l); e != nil {
		return nil, e
	}
	n := a.rows
	if n != a.cols || len(b) != n || (options.InitialGuess != nil && len(options.InitialGuess) != n) {
		return nil, ErrShape
	}
	iterations := options.MaxIterations
	if iterations == 0 {
		iterations = 1000
	}
	if iterations < 1 || iterations > 100000 || !finite(options.RelativeTolerance) || !finite(options.AbsoluteTolerance) || options.RelativeTolerance < 0 || options.AbsoluteTolerance < 0 || options.Preconditioner > Jacobi {
		return nil, ErrOptions
	}
	if e = l.storage(80 * uint64(n)); e != nil {
		return nil, e
	}
	budget := &solveBudget{ctx, l.MaxWork}
	validation := 4*(uint64(n)+1) + uint64(len(a.val))*uint64(bits.Len(uint(n))+2)
	if e = budget.spend(validation); e != nil {
		return nil, e
	}
	if e = vector(b); e != nil {
		return nil, e
	}
	if e = vector(options.InitialGuess); e != nil {
		return nil, e
	}
	step := 0
	for i := 0; i < n; i++ {
		if e = tick(ctx, i); e != nil {
			return nil, e
		}
		for j := a.ptr[i]; j < a.ptr[i+1]; j++ {
			if e = tick(ctx, step); e != nil {
				return nil, e
			}
			step++
			if a.val[j] != a.at(a.col[j], i) {
				return nil, ErrNotSymmetric
			}
		}
	}
	// Seven private n-vectors; x transfers to the result without another copy.
	x, r, actual, z, p, ap, diag := make([]float64, n), make([]float64, n), make([]float64, n), make([]float64, n), make([]float64, n), make([]float64, n), make([]float64, n)
	copy(x, options.InitialGuess)
	for i := range diag {
		if e = tick(ctx, i); e != nil {
			return nil, e
		}
		diag[i] = 1
		if options.Preconditioner == Jacobi {
			diag[i] = a.at(i, i)
			if diag[i] <= 0 {
				return nil, fmt.Errorf("%w: Jacobi diagonal %d", ErrBreakdown, i)
			}
		}
	}
	matWork := 8 * (uint64(n) + uint64(len(a.val)) + 1)
	if e = budget.spend(matWork + 8*uint64(n) + 1); e != nil {
		return nil, e
	}
	bn, e := norm(b, ctx)
	if e != nil {
		return nil, e
	}
	threshold := number().Mul(number().SetFloat64(options.RelativeTolerance), bn)
	abs := number().SetFloat64(options.AbsoluteTolerance)
	if abs.Cmp(threshold) > 0 {
		threshold.Set(abs)
	}
	if e = a.multiply(actual, x, b, ctx); e != nil {
		return nil, e
	}
	finish := func(k int, rn *big.Float) (*CGResult, error) {
		if e := ctx.Err(); e != nil {
			return nil, e
		}
		f := number()
		ex := rn.MantExp(f)
		fraction, _ := f.Float64()
		if fraction == 1 {
			fraction = 0.5
			ex++
		}
		relative := ratio(rn, bn)
		if bn.Sign() != 0 && math.IsInf(relative, 0) {
			return nil, ErrRange
		}
		return &CGResult{x, k, fraction, ex, relative}, nil
	}
	rn, e := norm(actual, ctx)
	if e != nil {
		return nil, e
	}
	if rn.Cmp(threshold) <= 0 {
		return finish(0, rn)
	}
	copy(r, actual)
	apply := func() error {
		v, w := number(), number()
		for i := range r {
			if e := tick(ctx, i); e != nil {
				return e
			}
			v.SetFloat64(r[i])
			w.SetFloat64(diag[i])
			v.Quo(v, w)
			f, e := rounded(v)
			if e != nil {
				return e
			}
			z[i] = f
		}
		return nil
	}
	if e = budget.spend(16*uint64(n) + 1); e != nil {
		return nil, e
	}
	if e = apply(); e != nil {
		return nil, e
	}
	rho, e := dot(r, z, ctx)
	if e != nil {
		return nil, e
	}
	if rho.Sign() <= 0 {
		return nil, ErrBreakdown
	}
	scale, e := direction(p, z, nil, nil, ctx)
	if e != nil {
		return nil, e
	}
	for k := 1; k <= iterations; k++ {
		// Reserve the complete iteration, including original residual, norm checks,
		// recurrence/true-residual replacement, preconditioner and direction update.
		if e = budget.spend(2*matWork + 96*uint64(n) + 1); e != nil {
			return nil, e
		}
		if e = a.multiply(ap, p, nil, ctx); e != nil {
			return nil, e
		}
		curvature, e := dot(p, ap, ctx)
		if e != nil {
			return nil, e
		}
		if curvature.Sign() <= 0 {
			return nil, fmt.Errorf("%w: iteration %d", ErrBreakdown, k)
		}
		alpha := scalar().Quo(rho, number().Mul(scale, curvature))
		if e = axpy(x, x, p, alpha, ctx); e != nil {
			return nil, e
		}
		if e = a.multiply(actual, x, b, ctx); e != nil {
			return nil, e
		}
		rn, e = norm(actual, ctx)
		if e != nil {
			return nil, e
		}
		if rn.Cmp(threshold) <= 0 {
			return finish(k, rn)
		}
		restart := k%32 == 0
		if restart {
			copy(r, actual)
		} else {
			if e = axpy(r, r, ap, number().Neg(alpha), ctx); e != nil {
				return nil, e
			}
			zero := true
			for _, v := range r {
				if v != 0 {
					zero = false
					break
				}
			}
			if zero {
				copy(r, actual)
				restart = true
			}
		}
		if e = apply(); e != nil {
			return nil, e
		}
		next, e := dot(r, z, ctx)
		if e != nil {
			return nil, e
		}
		if next.Sign() <= 0 {
			return nil, ErrBreakdown
		}
		if restart {
			scale, e = direction(p, z, nil, nil, ctx)
		} else {
			coefficient := scalar().Mul(number().Quo(next, rho), scale)
			scale, e = direction(p, z, p, coefficient, ctx)
		}
		if e != nil {
			return nil, e
		}
		rho = next
	}
	if e = ctx.Err(); e != nil {
		return nil, e
	}
	return nil, fmt.Errorf("%w: %d iterations, relative residual %.6g", ErrConvergence, iterations, ratio(rn, bn))
}
