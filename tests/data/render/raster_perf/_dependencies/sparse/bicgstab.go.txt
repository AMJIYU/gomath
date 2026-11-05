package sparse

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/big"
)

var (
	// ErrBiCGSTABBreakdown indicates a zero/near-orthogonal BiCGSTAB denominator
	// or a zero Jacobi diagonal. It does not assert anything about definiteness.
	ErrBiCGSTABBreakdown = errors.New("sparse: BiCGSTAB breakdown")
	// ErrBiCGSTABConvergence is iteration exhaustion without a verified solution.
	ErrBiCGSTABConvergence = errors.New("sparse: BiCGSTAB did not converge")
)

// BiCGSTABOptions bounds a nonsymmetric solve. Zero MaxIterations selects 1000
// (hard cap 100000). Tolerances are literal, finite and nonnegative; use the
// default constructor for rtol=1e-10 and BreakdownTolerance=1e-30. BreakdownTolerance
// must be less than one; zero tests exact breakdown only. A dot(u,v) is considered
// near breakdown when |dot(u,v)| <= BreakdownTolerance*||u||*||v||.
// InitialGuess is copied. Jacobi is right preconditioning and requires nonzero
// (possibly negative) diagonal entries, unlike CG's positive-diagonal policy.
// Stop only when ||b-A*x|| <= max(atol,rtol*||b||), using the original CSR.
// Both stopping tolerances zero request an exactly zero computed residual.
// Reuse of Preconditioner does not add any SPD or symmetry prerequisite.
type BiCGSTABOptions struct {
	Limits                               Limits
	MaxIterations                        int
	RelativeTolerance, AbsoluteTolerance float64
	BreakdownTolerance                   float64
	Preconditioner                       Preconditioner
	InitialGuess                         []float64
}

func DefaultBiCGSTABOptions() BiCGSTABOptions {
	return BiCGSTABOptions{MaxIterations: 1000, RelativeTolerance: 1e-10, BreakdownTolerance: 1e-30}
}

// BiCGSTABResult is an immutable successful solve, not a conditioning or forward
// error certificate. Errors return nil, never a partial solution.
type BiCGSTABResult struct{ value CGResult }

func (r *BiCGSTABResult) Solution() []float64 {
	if r == nil {
		return nil
	}
	return r.value.Solution()
}
func (r *BiCGSTABResult) Iterations() int {
	if r == nil {
		return 0
	}
	return r.value.Iterations()
}

// ResidualNorm returns ||b-A*x|| as fraction*2^exponent, without norm overflow.
func (r *BiCGSTABResult) ResidualNorm() (float64, int) {
	if r == nil {
		return 0, 0
	}
	return r.value.ResidualNorm()
}

// RelativeResidual is zero for zero RHS/zero residual and +Inf for zero RHS
// accepted with a nonzero residual by absolute tolerance alone.
func (r *BiCGSTABResult) RelativeResidual() float64 {
	if r == nil {
		return 0
	}
	return r.value.RelativeResidual()
}

// Internal vector components have 53-bit significands, with MantExp exponents
// restricted to [-8192,8192]. The larger accumulator holds exact sums of <=16M
// products of those components: full exponent span + 106 product bits + 24
// summation bits. Range checks prevent precision from becoming input-dependent.
const biPrecision = 33024
const biExponent = 8192
const biScratchBytes = 256 << 10

func biNumber() *big.Float { return new(big.Float).SetPrec(biPrecision).SetMode(big.ToNearestEven) }

type biVector []big.Float

func biNew(n int, ctx context.Context) (biVector, error) {
	v := make(biVector, n)
	for i := range v {
		if e := tick(ctx, i); e != nil {
			return nil, e
		}
		v[i].SetPrec(53).SetMode(big.ToNearestEven)
	}
	return v, canceled(ctx)
}
func biRange(v *big.Float) error {
	if v.Sign() != 0 {
		e := v.MantExp(nil)
		if v.IsInf() || e < -biExponent || e > biExponent {
			return ErrRange
		}
	}
	return nil
}
func biCopy(dst, src biVector, ctx context.Context) error {
	for i := range dst {
		if e := tick(ctx, i); e != nil {
			return e
		}
		dst[i].Set(&src[i])
	}
	return canceled(ctx)
}
func biLoad(dst biVector, src []float64, ctx context.Context) error {
	for i := range dst {
		if e := tick(ctx, i); e != nil {
			return e
		}
		dst[i].SetFloat64(src[i])
	}
	return canceled(ctx)
}
func biDot(a, b biVector, ctx context.Context) (*big.Float, error) {
	s, p := biNumber(), biNumber()
	for i := range a {
		if e := tick(ctx, i); e != nil {
			return nil, e
		}
		p.Mul(&a[i], &b[i])
		s.Add(s, p)
	}
	return s, canceled(ctx)
}
func biAxpy(dst, x, y biVector, c *big.Float, ctx context.Context) error {
	v, compact := biNumber(), scalar()
	for i := range dst {
		if e := tick(ctx, i); e != nil {
			return e
		}
		v.Mul(c, &y[i])
		v.Add(v, &x[i])
		// Float.Set copies the source mantissa before rounding. Round through
		// one scratch scalar first, so wide accumulator capacity cannot be retained
		// independently by every vector component.
		compact.Set(v)
		dst[i].Set(compact)
		if e := biRange(&dst[i]); e != nil {
			return e
		}
	}
	return canceled(ctx)
}
func biApply(dst, src biVector, diag []float64, ctx context.Context) error {
	d := scalar()
	for i := range dst {
		if e := tick(ctx, i); e != nil {
			return e
		}
		d.SetFloat64(diag[i])
		dst[i].Quo(&src[i], d)
		if e := biRange(&dst[i]); e != nil {
			return e
		}
	}
	return canceled(ctx)
}
func biMultiply(a *CSR, dst, src biVector, ctx context.Context) error {
	s, p, c, compact := biNumber(), biNumber(), scalar(), scalar()
	step := 0
	for i := 0; i < a.rows; i++ {
		if e := tick(ctx, i); e != nil {
			return e
		}
		s.SetInt64(0)
		for j := a.ptr[i]; j < a.ptr[i+1]; j++ {
			if e := tick(ctx, step); e != nil {
				return e
			}
			step++
			c.SetFloat64(a.val[j])
			p.Mul(c, &src[a.col[j]])
			s.Add(s, p)
		}
		compact.Set(s)
		dst[i].Set(compact)
		if e := biRange(&dst[i]); e != nil {
			return e
		}
	}
	return canceled(ctx)
}
func biSmall(d *big.Float, u, v biVector, tol float64, ctx context.Context) (bool, error) {
	if d.Sign() == 0 {
		return true, canceled(ctx)
	}
	if tol == 0 {
		return false, canceled(ctx)
	}
	un, e := biDot(u, u, ctx)
	if e != nil {
		return false, e
	}
	vn, e := biDot(v, v, ctx)
	if e != nil {
		return false, e
	}
	// Compare squares to avoid square-root rounding at the policy boundary.
	// Two exact dot sums can each occupy biPrecision bits. Their product,
	// followed by the square of a binary64 tolerance, needs up to another 106
	// bits. This comparison precision is fixed, not an adaptive allocation.
	threshold := new(big.Float).SetPrec(2*biPrecision+128).Mul(un, vn)
	q := new(big.Float).SetPrec(2*biPrecision + 128).SetFloat64(tol)
	q.Mul(q, q)
	threshold.Mul(threshold, q)
	q.Mul(d, d)
	return q.Cmp(threshold) <= 0, canceled(ctx)
}

// SolveBiCGSTAB uses bounded right-preconditioned BiCGSTAB without dense
// conversion or a symmetry check. Internal vector arithmetic rounds to 53-bit
// significands with bounded extended exponents; dot/matrix product sums are exact
// before that rounding. Every half/full candidate is rounded to binary64 and
// checked using the original system. Nonzero binary64 underflow is an error.
// The residual and shadow/search directions restart every 32 full iterations,
// or after a vanishing recurrence residual that fails the original-system check.
// ctx must be nonnil. Breakdown, range, iteration/work/storage exhaustion and
// cancellation return nil; nonsingular matrices need not converge by this method.
// Limits charge 2560*n+256KiB new storage in addition to checking the input CSR
// separately, and reserve 4*matWork+512*n+1 work units per iteration (see SPARSE.md).
func SolveBiCGSTAB(ctx context.Context, a *CSR, b []float64, o BiCGSTABOptions) (*BiCGSTABResult, error) {
	if ctx == nil {
		return nil, ErrOptions
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	l, e := o.Limits.normalized()
	if e != nil {
		return nil, e
	}
	if e = a.check(l); e != nil {
		return nil, e
	}
	n := a.rows
	if n != a.cols || len(b) != n || (o.InitialGuess != nil && len(o.InitialGuess) != n) {
		return nil, ErrShape
	}
	iterations := o.MaxIterations
	if iterations == 0 {
		iterations = 1000
	}
	if iterations < 1 || iterations > 100000 || !finite(o.RelativeTolerance) || !finite(o.AbsoluteTolerance) || !finite(o.BreakdownTolerance) || o.RelativeTolerance < 0 || o.AbsoluteTolerance < 0 || o.BreakdownTolerance < 0 || o.BreakdownTolerance >= 1 || o.Preconditioner > Jacobi {
		return nil, ErrOptions
	}
	if e = l.storage(2560*uint64(n) + biScratchBytes); e != nil {
		return nil, e
	}
	budget := &solveBudget{ctx, l.MaxWork}
	if e = budget.spend(4 * (uint64(n) + uint64(len(a.val)) + 1)); e != nil {
		return nil, e
	}
	// Validate with context checkpoints, including caller vectors, before copying.
	for _, v := range [][]float64{b, o.InitialGuess} {
		for i, f := range v {
			if e = tick(ctx, i); e != nil {
				return nil, e
			}
			if !finite(f) {
				return nil, ErrNonFinite
			}
		}
	}
	// Nine extended n-vectors and three float64 n-vectors. The conservative reserve
	// includes big.Float headers/53-bit limbs and bounded numeric scratch, not RSS
	// or cumulative GC allocation traffic. No allocation scales with n*n.
	var workspace [9]biVector
	for i := range workspace {
		workspace[i], e = biNew(n, ctx)
		if e != nil {
			return nil, e
		}
	}
	x, r, shadow, p, v, s, t, phat, shat := workspace[0], workspace[1], workspace[2], workspace[3], workspace[4], workspace[5], workspace[6], workspace[7], workspace[8]
	candidate, actual, diag := make([]float64, n), make([]float64, n), make([]float64, n)
	if o.InitialGuess != nil {
		if e = biLoad(x, o.InitialGuess, ctx); e != nil {
			return nil, e
		}
	}
	for i := range diag {
		if e = tick(ctx, i); e != nil {
			return nil, e
		}
		diag[i] = 1
		if o.Preconditioner == Jacobi {
			diag[i] = a.at(i, i)
			if diag[i] == 0 {
				return nil, fmt.Errorf("%w: Jacobi diagonal %d", ErrBiCGSTABBreakdown, i)
			}
		}
	}
	matWork := 8 * (uint64(n) + uint64(len(a.val)) + 1)
	if e = budget.spend(matWork + 16*uint64(n) + 1); e != nil {
		return nil, e
	}
	bn, e := norm(b, ctx)
	if e != nil {
		return nil, e
	}
	threshold := number().Mul(number().SetFloat64(o.RelativeTolerance), bn)
	atol := number().SetFloat64(o.AbsoluteTolerance)
	if atol.Cmp(threshold) > 0 {
		threshold.Set(atol)
	}
	// candidate/actual are private and reused; only a verified successful candidate
	// transfers into the immutable result. A failed half-step never escapes.
	verify := func() (*big.Float, error) {
		for i := range x {
			if e := tick(ctx, i); e != nil {
				return nil, e
			}
			f, e := rounded(&x[i])
			if e != nil {
				return nil, e
			}
			candidate[i] = f
		}
		if e := a.multiply(actual, candidate, b, ctx); e != nil {
			return nil, e
		}
		return norm(actual, ctx)
	}
	finish := func(k int, rn *big.Float) (*BiCGSTABResult, error) {
		if e := ctx.Err(); e != nil {
			return nil, e
		}
		mantissa := number()
		ex := rn.MantExp(mantissa)
		f, _ := mantissa.Float64()
		if f == 1 {
			f = .5
			ex++
		}
		relative := ratio(rn, bn)
		if bn.Sign() != 0 && math.IsInf(relative, 0) {
			return nil, ErrRange
		}
		return &BiCGSTABResult{CGResult{candidate, k, f, ex, relative}}, nil
	}
	rn, e := verify()
	if e != nil {
		return nil, e
	}
	if rn.Cmp(threshold) <= 0 {
		return finish(0, rn)
	}
	restart := true
	rhoOld, alpha, omega := scalar().SetInt64(1), scalar().SetInt64(1), scalar().SetInt64(1)
	for k := 1; k <= iterations; k++ {
		if e = budget.spend(4*matWork + 512*uint64(n) + 1); e != nil {
			return nil, e
		}
		if restart {
			// Align extended x with the binary64 candidate whose residual was checked.
			if e = biLoad(x, candidate, ctx); e != nil {
				return nil, e
			}
			if e = biLoad(r, actual, ctx); e != nil {
				return nil, e
			}
			if e = biCopy(shadow, r, ctx); e != nil {
				return nil, e
			}
		}
		rho, e := biDot(shadow, r, ctx)
		if e != nil {
			return nil, e
		}
		small, e := biSmall(rho, shadow, r, o.BreakdownTolerance, ctx)
		if e != nil {
			return nil, e
		}
		if small {
			return nil, fmt.Errorf("%w: rho at iteration %d", ErrBiCGSTABBreakdown, k)
		}
		if restart {
			if e = biCopy(p, r, ctx); e != nil {
				return nil, e
			}
		} else {
			beta := scalar().Quo(rho, rhoOld)
			beta.Mul(beta, scalar().Quo(alpha, omega))
			if e = biRange(beta); e != nil {
				return nil, e
			}
			if e = biAxpy(p, p, v, scalar().Neg(omega), ctx); e != nil {
				return nil, e
			}
			if e = biAxpy(p, r, p, beta, ctx); e != nil {
				return nil, e
			}
		}
		if e = biApply(phat, p, diag, ctx); e != nil {
			return nil, e
		}
		if e = biMultiply(a, v, phat, ctx); e != nil {
			return nil, e
		}
		denominator, e := biDot(shadow, v, ctx)
		if e != nil {
			return nil, e
		}
		small, e = biSmall(denominator, shadow, v, o.BreakdownTolerance, ctx)
		if e != nil {
			return nil, e
		}
		if small {
			return nil, fmt.Errorf("%w: alpha at iteration %d", ErrBiCGSTABBreakdown, k)
		}
		alpha = scalar().Quo(rho, denominator)
		if e = biRange(alpha); e != nil {
			return nil, e
		}
		if e = biAxpy(s, r, v, scalar().Neg(alpha), ctx); e != nil {
			return nil, e
		}
		if e = biAxpy(x, x, phat, alpha, ctx); e != nil {
			return nil, e
		}
		rn, e = verify()
		if e != nil {
			return nil, e
		}
		if rn.Cmp(threshold) <= 0 {
			return finish(k, rn)
		}
		ss, e := biDot(s, s, ctx)
		if e != nil {
			return nil, e
		}
		if ss.Sign() == 0 {
			restart = true
			continue
		}
		if e = biApply(shat, s, diag, ctx); e != nil {
			return nil, e
		}
		if e = biMultiply(a, t, shat, ctx); e != nil {
			return nil, e
		}
		ts, e := biDot(t, s, ctx)
		if e != nil {
			return nil, e
		}
		tt, e := biDot(t, t, ctx)
		if e != nil {
			return nil, e
		}
		small, e = biSmall(ts, t, s, o.BreakdownTolerance, ctx)
		if e != nil {
			return nil, e
		}
		if tt.Sign() == 0 || small {
			return nil, fmt.Errorf("%w: omega at iteration %d", ErrBiCGSTABBreakdown, k)
		}
		omega = scalar().Quo(ts, tt)
		if e = biRange(omega); e != nil {
			return nil, e
		}
		if e = biAxpy(x, x, shat, omega, ctx); e != nil {
			return nil, e
		}
		rn, e = verify()
		if e != nil {
			return nil, e
		}
		if rn.Cmp(threshold) <= 0 {
			return finish(k, rn)
		}
		if e = biAxpy(r, s, t, scalar().Neg(omega), ctx); e != nil {
			return nil, e
		}
		rr, e := biDot(r, r, ctx)
		if e != nil {
			return nil, e
		}
		restart = k%32 == 0 || rr.Sign() == 0
		rhoOld = scalar().Set(rho)
	}
	if e = ctx.Err(); e != nil {
		return nil, e
	}
	return nil, fmt.Errorf("%w: %d iterations, relative residual %.6g", ErrBiCGSTABConvergence, iterations, ratio(rn, bn))
}
