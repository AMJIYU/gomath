package sparse

import (
	"context"
	"errors"
	"math"
	"math/big"
	"testing"
)

func biFailure(t *testing.T, ctx context.Context, a *CSR, b []float64, o BiCGSTABOptions, want error) {
	t.Helper()
	r, e := SolveBiCGSTAB(ctx, a, b, o)
	if r != nil || !errors.Is(e, want) {
		t.Fatalf("want nil/%v, got %v/%v", want, r, e)
	}
}

func TestBiCGSTABInvalidPolicies(t *testing.T) {
	ctx := context.Background()
	a := diagonal(t, []float64{2})
	o := DefaultBiCGSTABOptions()
	biFailure(t, nil, a, []float64{1}, o, ErrOptions)
	biFailure(t, ctx, nil, []float64{1}, o, ErrShape)
	biFailure(t, ctx, must(t, 1, 2, []int{0, 1}, []int{0}, []float64{1}), []float64{1}, o, ErrShape)
	biFailure(t, ctx, a, nil, o, ErrShape)
	q := o
	q.InitialGuess = []float64{}
	biFailure(t, ctx, a, []float64{1}, q, ErrShape)
	biFailure(t, ctx, a, []float64{math.NaN()}, o, ErrNonFinite)
	q = o
	q.InitialGuess = []float64{math.Inf(1)}
	biFailure(t, ctx, a, []float64{1}, q, ErrNonFinite)
	for _, mutate := range []func(*BiCGSTABOptions){
		func(o *BiCGSTABOptions) { o.MaxIterations = -1 }, func(o *BiCGSTABOptions) { o.MaxIterations = 100001 },
		func(o *BiCGSTABOptions) { o.RelativeTolerance = -1 }, func(o *BiCGSTABOptions) { o.RelativeTolerance = math.Inf(1) },
		func(o *BiCGSTABOptions) { o.AbsoluteTolerance = -1 }, func(o *BiCGSTABOptions) { o.AbsoluteTolerance = math.NaN() },
		func(o *BiCGSTABOptions) { o.BreakdownTolerance = -1 }, func(o *BiCGSTABOptions) { o.BreakdownTolerance = 1 }, func(o *BiCGSTABOptions) { o.BreakdownTolerance = math.NaN() },
		func(o *BiCGSTABOptions) { o.Preconditioner = 2 }, func(o *BiCGSTABOptions) { o.Limits.MaxDimension = -1 },
		func(o *BiCGSTABOptions) { o.Limits.MaxWork = 1<<34 + 1 }, func(o *BiCGSTABOptions) { o.Limits.MaxNNZ = 16<<20 + 1 }, func(o *BiCGSTABOptions) { o.Limits.MaxStorageBytes = 512<<20 + 1 },
	} {
		q = o
		mutate(&q)
		biFailure(t, ctx, a, []float64{1}, q, ErrOptions)
	}
}

func TestBiCGSTABBreakdown(t *testing.T) {
	ctx := context.Background()
	o := DefaultBiCGSTABOptions()
	// A nonsingular rotation is a genuine BiCGSTAB alpha breakdown for b=e1.
	rotation := must(t, 2, 2, []int{0, 1, 2}, []int{1, 0}, []float64{1, -1})
	biFailure(t, ctx, rotation, []float64{1, 0}, o, ErrBiCGSTABBreakdown)
	// Omega breakdown (alpha is fine): t and s are nonzero but orthogonal.
	omega := must(t, 2, 2, []int{0, 2, 3}, []int{0, 1, 0}, []float64{1, 1, 1})
	biFailure(t, ctx, omega, []float64{1, 0}, o, ErrBiCGSTABBreakdown)
	// Nonzero but nearly orthogonal alpha denominator triggers explicit policy.
	near := must(t, 2, 2, []int{0, 2, 4}, []int{0, 1, 0, 1}, []float64{1e-16, 1, 1, 2})
	q := o
	q.BreakdownTolerance = 1e-12
	biFailure(t, ctx, near, []float64{1, 0}, q, ErrBiCGSTABBreakdown)
	// The comparison policy can be disabled without asserting convergence.
	u, v := biTestNew(t, 2), biTestNew(t, 2)
	u[0].SetInt64(1)
	v[0].SetFloat64(1e-16)
	v[1].SetInt64(1)
	d, e := biDot(u, v, ctx)
	if e != nil {
		t.Fatal(e)
	}
	small, e := biSmall(d, u, v, 0, ctx)
	if e != nil || small {
		t.Fatal("literal zero near-breakdown policy")
	}
	zeroDiagonal := must(t, 2, 2, []int{0, 1, 3}, []int{1, 0, 1}, []float64{1, 1, 2})
	q = o
	q.Preconditioner = Jacobi
	biFailure(t, ctx, zeroDiagonal, []float64{1, 3}, q, ErrBiCGSTABBreakdown)
	// Zero diagonal is allowed without Jacobi; an exact initial guess succeeds.
	q = o
	q.InitialGuess = []float64{1, 1}
	r, e := SolveBiCGSTAB(ctx, zeroDiagonal, []float64{1, 3}, q)
	if e != nil || r.Iterations() != 0 {
		t.Fatalf("zero diagonal wrongly disallowed: %v", e)
	}
	// Nonsymmetric indefinite systems can converge; no symmetry/SPD preflight.
	a := must(t, 2, 2, []int{0, 2, 4}, []int{0, 1, 0, 1}, []float64{-2, 1, 3, 4})
	q = o
	q.Preconditioner = Jacobi
	r, e = SolveBiCGSTAB(ctx, a, []float64{0, 11}, q)
	if e != nil {
		t.Fatal(e)
	}
	closeVec(t, r.Solution(), []float64{1, 2}, 1e-9)
	biCheck(t, a, []float64{0, 11}, r, q)
}

func TestBiCGSTABRangesAndVerifiedResidual(t *testing.T) {
	ctx := context.Background()
	o := DefaultBiCGSTABOptions()
	biFailure(t, ctx, diagonal(t, []float64{math.SmallestNonzeroFloat64}), []float64{1}, o, ErrRange)
	biFailure(t, ctx, diagonal(t, []float64{1e308}), []float64{math.SmallestNonzeroFloat64}, o, ErrRange)
	sub := math.SmallestNonzeroFloat64
	a := must(t, 2, 2, []int{0, 2, 4}, []int{0, 1, 0, 1}, []float64{2 * sub, sub, -sub, 3 * sub})
	biFailure(t, ctx, a, []float64{4 * sub, 5 * sub}, o, ErrRange)
	// binary64 x=1/3 makes recurrence residual zero, but original exact product
	// residual is nonzero. A zero stopping tolerance must never accept it.
	q := BiCGSTABOptions{MaxIterations: 3}
	biFailure(t, ctx, diagonal(t, []float64{3}), []float64{1}, q, ErrBiCGSTABConvergence)
	q = DefaultBiCGSTABOptions()
	r, e := SolveBiCGSTAB(ctx, diagonal(t, []float64{3}), []float64{1}, q)
	if e != nil {
		t.Fatal(e)
	}
	want := new(big.Rat).Sub(big.NewRat(1, 1), new(big.Rat).Mul(big.NewRat(3, 1), new(big.Rat).SetFloat64(r.Solution()[0])))
	f, ex := r.ResidualNorm()
	got := new(big.Rat).SetFloat64(math.Ldexp(f, ex))
	if got.Cmp(new(big.Rat).Abs(want)) != 0 {
		t.Fatalf("residual not original: got %s want %s", got, want)
	}
	// Norm larger than MaxFloat64 is exposed as a scaled value, never Inf.
	q = DefaultBiCGSTABOptions()
	q.AbsoluteTolerance = math.MaxFloat64
	q.RelativeTolerance = 1
	r, e = SolveBiCGSTAB(ctx, diagonal(t, []float64{1, 1}), []float64{1.7e308, 1.7e308}, q)
	if e != nil {
		t.Fatal(e)
	}
	f, ex = r.ResidualNorm()
	if f < .5 || f >= 1 || ex <= 1024 {
		t.Fatalf("unscaled norm %g %d", f, ex)
	}
	// +Inf relative diagnostic is only allowed for zero RHS; finite norm ratio
	// overflow for nonzero RHS is a strict range error even if atol would accept.
	q = DefaultBiCGSTABOptions()
	q.InitialGuess = []float64{1}
	q.AbsoluteTolerance = 2
	biFailure(t, ctx, diagonal(t, []float64{1}), []float64{math.SmallestNonzeroFloat64}, q, ErrRange)
}

func TestBiCGSTABBudgets(t *testing.T) {
	ctx := context.Background()
	o := DefaultBiCGSTABOptions()
	a := diagonal(t, []float64{2})
	b := []float64{4}
	// Precisely documented full-iteration work reserve, including half-step success.
	const initial = 4*(1+1+1) + (8*(1+1+1) + 16*1 + 1)
	const step = 4*(8*(1+1+1)) + 512*1 + 1
	q := o
	q.Limits.MaxWork = initial + step
	q.Limits.MaxStorageBytes = biScratchBytes + 2560
	r, e := SolveBiCGSTAB(ctx, a, b, q)
	if e != nil || r.Iterations() != 1 {
		t.Fatalf("exact reserve %v %v", r, e)
	}
	q.Limits.MaxWork--
	biFailure(t, ctx, a, b, q, ErrLimit)
	q = o
	q.Limits.MaxWork = 1
	biFailure(t, ctx, a, b, q, ErrLimit)
	q = o
	q.Limits.MaxWork = initial - 1
	biFailure(t, ctx, a, b, q, ErrLimit)
	q = o
	q.Limits.MaxStorageBytes = biScratchBytes + 2559
	biFailure(t, ctx, a, b, q, ErrLimit)
	q = o
	q.Limits.MaxDimension = 1
	biFailure(t, ctx, diagonal(t, []float64{1, 2}), []float64{1, 1}, q, ErrLimit)
	q = o
	q.Limits.MaxNNZ = 1
	biFailure(t, ctx, diagonal(t, []float64{1, 2}), []float64{1, 1}, q, ErrLimit)
	// Existing-operand storage is checked separately even for an exact guess.
	dense := must(t, 2, 2, []int{0, 2, 4}, []int{0, 1, 0, 1}, []float64{2, 1, 1, 2})
	q = o
	q.Limits.MaxStorageBytes = 87
	biFailure(t, ctx, dense, []float64{0, 0}, q, ErrLimit)
	a = diagonal(t, []float64{1, 2, 3, 4})
	q = o
	q.MaxIterations = 1
	biFailure(t, ctx, a, []float64{1, 1, 1, 1}, q, ErrBiCGSTABConvergence)
}

func TestBiCGSTABCancellationCheckpoints(t *testing.T) {
	a := must(t, 3, 3, []int{0, 2, 5, 7}, []int{0, 1, 0, 1, 2, 1, 2}, []float64{4, 1, -2, 5, 1, -1, 3})
	b := []float64{6, 11, 7}
	o := DefaultBiCGSTABOptions()
	o.Preconditioner = Jacobi
	// Discover the exact number of Err checkpoints on a successful path, then
	// cancel at each one. Existing CG's test context is shared unchanged.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	counter := &countedContext{Context: ctx, cancel: cancel, after: math.MaxInt64}
	r, e := SolveBiCGSTAB(counter, a, b, o)
	if e != nil {
		t.Fatal(e)
	}
	biCheck(t, a, b, r, o)
	total := counter.calls.Load()
	for k := int64(1); k <= total; k++ {
		ctx, cancel := context.WithCancel(context.Background())
		c := &countedContext{Context: ctx, cancel: cancel, after: k}
		biFailure(t, c, a, b, o, context.Canceled)
		cancel()
	}
	// Canceled entry takes priority over allocation/validation.
	ctx2, cancel2 := context.WithCancel(context.Background())
	cancel2()
	biFailure(t, ctx2, nil, nil, o, context.Canceled)
}

func TestBiCGSTABEmptyBudgetAndSingular(t *testing.T) {
	ctx := context.Background()
	o := DefaultBiCGSTABOptions()
	o.Limits.MaxStorageBytes = biScratchBytes
	o.Limits.MaxWork = 13
	r, e := SolveBiCGSTAB(ctx, new(CSR), nil, o)
	if e != nil || r.Iterations() != 0 {
		t.Fatalf("empty exact budget %v %v", r, e)
	}
	o.Limits.MaxWork = 12
	biFailure(t, ctx, new(CSR), nil, o, ErrLimit)
	o.Limits.MaxWork = 13
	o.Limits.MaxStorageBytes = biScratchBytes - 1
	biFailure(t, ctx, new(CSR), nil, o, ErrLimit)
	a := must(t, 2, 2, []int{0, 1, 2}, []int{0, 0}, []float64{1, 1})
	biFailure(t, ctx, a, []float64{1, 0}, DefaultBiCGSTABOptions(), ErrBiCGSTABBreakdown)
}
