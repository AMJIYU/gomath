package sparse

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
	"reflect"
	"sync"
	"testing"
)

func biCheck(t testing.TB, a *CSR, b []float64, r *BiCGSTABResult, o BiCGSTABOptions) {
	t.Helper()
	if r == nil {
		t.Fatal("nil success")
	}
	x := r.Solution()
	res := make([]float64, len(b))
	if e := a.multiply(res, x, b, context.Background()); e != nil {
		t.Fatal(e)
	}
	rn, e := norm(res, context.Background())
	if e != nil {
		t.Fatal(e)
	}
	bn, e := norm(b, context.Background())
	if e != nil {
		t.Fatal(e)
	}
	threshold := number().Mul(number().SetFloat64(o.RelativeTolerance), bn)
	atol := number().SetFloat64(o.AbsoluteTolerance)
	if atol.Cmp(threshold) > 0 {
		threshold.Set(atol)
	}
	if rn.Cmp(threshold) > 0 {
		t.Fatalf("unverified residual %g", ratio(rn, bn))
	}
	f, ex := r.ResidualNorm()
	if f != 0 && (f < .5 || f >= 1) {
		t.Fatalf("bad mantissa %g", f)
	}
	if ratio(rn, bn) != r.RelativeResidual() {
		t.Fatal("wrong residual diagnostic")
	}
	if rn.Sign() == 0 && (f != 0 || ex != 0) {
		t.Fatal("bad zero norm")
	}
}

func TestBiCGSTABSciPy(t *testing.T) {
	raw, e := os.ReadFile("bicgstab.json")
	if e != nil {
		t.Fatal(e)
	}
	var fixture struct {
		Cases []struct {
			Name                        string
			Rows                        int
			Rowptr, Colind              []int
			Values, B, Solution, Direct []float64
			Jacobi                      bool
		}
	}
	if e = json.Unmarshal(raw, &fixture); e != nil {
		t.Fatal(e)
	}
	if len(fixture.Cases) != 11 {
		t.Fatal("missing independent cases")
	}
	for _, c := range fixture.Cases {
		t.Run(c.Name, func(t *testing.T) {
			a := must(t, c.Rows, c.Rows, c.Rowptr, c.Colind, c.Values)
			o := DefaultBiCGSTABOptions()
			o.RelativeTolerance = 1e-11
			if c.Jacobi {
				o.Preconditioner = Jacobi
			}
			r, e := SolveBiCGSTAB(context.Background(), a, c.B, o)
			if e != nil {
				t.Fatal(e)
			}
			biCheck(t, a, c.B, r, o)
			closeVec(t, r.Solution(), c.Solution, 1e-9)
			closeVec(t, r.Solution(), c.Direct, 1e-9)
		})
	}
}

func TestBiCGSTABAnalyticScales(t *testing.T) {
	for _, scale := range []float64{1, 1e-300, 1e300} {
		for _, pre := range []Preconditioner{NoPreconditioner, Jacobi} {
			a := must(t, 2, 2, []int{0, 2, 4}, []int{0, 1, 0, 1}, []float64{2 * scale, scale, -scale, 3 * scale})
			b := []float64{4 * scale, 5 * scale}
			o := DefaultBiCGSTABOptions()
			o.Preconditioner = pre
			o.AbsoluteTolerance = 0
			r, e := SolveBiCGSTAB(context.Background(), a, b, o)
			if e != nil {
				t.Fatalf("scale %g pre %v: %v", scale, pre, e)
			}
			closeVec(t, r.Solution(), []float64{1, 2}, 1e-9)
			biCheck(t, a, b, r, o)
		}
	}
	oSub := DefaultBiCGSTABOptions()
	rSub, eSub := SolveBiCGSTAB(context.Background(), diagonal(t, []float64{math.SmallestNonzeroFloat64}), []float64{2 * math.SmallestNonzeroFloat64}, oSub)
	if eSub != nil {
		t.Fatal(eSub)
	}
	equalVec(t, rSub.Solution(), []float64{2})
	// Overflowing ordinary dot products and matrix products are not premature failures.
	a := must(t, 2, 2, []int{0, 2, 4}, []int{0, 1, 0, 1}, []float64{1e308, 1e308, -1e308, 1e308})
	o := DefaultBiCGSTABOptions()
	r, e := SolveBiCGSTAB(context.Background(), a, []float64{1e308, 0}, o)
	if e != nil {
		t.Fatal(e)
	}
	closeVec(t, r.Solution(), []float64{.5, .5}, 1e-10)
}

func TestBiCGSTABZeroEmptyGuess(t *testing.T) {
	ctx := context.Background()
	o := DefaultBiCGSTABOptions()
	for _, a := range []*CSR{new(CSR), diagonal(t, []float64{2, -3})} {
		n, _ := a.Dims()
		b := make([]float64, n)
		r, e := SolveBiCGSTAB(ctx, a, b, o)
		if e != nil {
			t.Fatal(e)
		}
		if r.Iterations() != 0 {
			t.Fatal("zero RHS iterated")
		}
		biCheck(t, a, b, r, o)
	}
	a := diagonal(t, []float64{2, -3})
	b := []float64{2, -6}
	o.InitialGuess = []float64{1, 2}
	r, e := SolveBiCGSTAB(ctx, a, b, o)
	if e != nil || r.Iterations() != 0 {
		t.Fatalf("exact guess %v %v", r, e)
	}
	o.InitialGuess = []float64{.25, -1}
	r, e = SolveBiCGSTAB(ctx, a, b, o)
	if e != nil {
		t.Fatal(e)
	}
	biCheck(t, a, b, r, o)
	// Literal zero tolerances, and documented zero-RHS +Inf diagnostic.
	o = BiCGSTABOptions{}
	r, e = SolveBiCGSTAB(ctx, diagonal(t, []float64{2}), []float64{4}, o)
	if e != nil {
		t.Fatal(e)
	}
	equalVec(t, r.Solution(), []float64{2})
	o = DefaultBiCGSTABOptions()
	o.InitialGuess = []float64{1}
	o.AbsoluteTolerance = 3
	r, e = SolveBiCGSTAB(ctx, diagonal(t, []float64{2}), []float64{0}, o)
	if e != nil || !math.IsInf(r.RelativeResidual(), 1) {
		t.Fatalf("atol diagnostic %v %v", r, e)
	}
	var nilResult *BiCGSTABResult
	if nilResult.Solution() != nil || nilResult.Iterations() != 0 || nilResult.RelativeResidual() != 0 {
		t.Fatal("nil methods")
	}
	f, ex := nilResult.ResidualNorm()
	if f != 0 || ex != 0 {
		t.Fatal("nil norm")
	}
}

func TestBiCGSTABOwnershipConcurrent(t *testing.T) {
	a := must(t, 3, 3, []int{0, 2, 5, 7}, []int{0, 1, 0, 1, 2, 1, 2}, []float64{4, 1, -2, 5, 1, -1, 3})
	b := []float64{6, 11, 7}
	o := DefaultBiCGSTABOptions()
	o.InitialGuess = []float64{.5, .5, .5}
	before := append([]float64(nil), b...)
	guess := append([]float64(nil), o.InitialGuess...)
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for k := 0; k < 8; k++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := SolveBiCGSTAB(context.Background(), a, b, o)
			if e != nil {
				errs <- e
				return
			}
			x := r.Solution()
			x[0] = 1e99
			if r.Solution()[0] == 1e99 {
				errs <- errors.New("solution aliases")
			}
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(b, before) || !reflect.DeepEqual(o.InitialGuess, guess) {
		t.Fatal("input mutated")
	}
	r, e := SolveBiCGSTAB(context.Background(), a, b, o)
	if e != nil {
		t.Fatal(e)
	}
	saved := r.Solution()
	b[0] = 999
	o.InitialGuess[0] = 999
	if !reflect.DeepEqual(saved, r.Solution()) {
		t.Fatal("result retains caller data")
	}
}

func TestBiCGSTABRestartAndMixedScale(t *testing.T) {
	ctx := context.Background()
	a := biConvection(t, 97)
	b := make([]float64, 97)
	b[0] = 1
	o := DefaultBiCGSTABOptions()
	o.Limits.MaxWork = 1 << 30
	r, e := SolveBiCGSTAB(ctx, a, b, o)
	if e != nil {
		t.Fatal(e)
	}
	if r.Iterations() <= 32 {
		t.Fatalf("restart path not exercised: %d", r.Iterations())
	}
	biCheck(t, a, b, r, o)
	// A's entries span 1800 binary exponents: neither A nor the RHS is silently
	// rescaled or stripped of its small entries. Right Jacobi is explicitly chosen.
	a = must(t, 2, 2, []int{0, 2, 4}, []int{0, 1, 0, 1}, []float64{math.Ldexp(2, 900), math.Ldexp(1, -900), math.Ldexp(-1, 900), math.Ldexp(3, -900)})
	// x has reciprocal column scales, so both columns contribute at ordinary size.
	b = []float64{4, 5}
	o.Preconditioner = Jacobi
	r, e = SolveBiCGSTAB(ctx, a, b, o)
	if e != nil {
		t.Fatal(e)
	}
	x := r.Solution()
	if math.Abs(math.Ldexp(x[0], 900)-1) > 1e-9 || math.Abs(math.Ldexp(x[1], -900)-2) > 1e-9 {
		t.Fatalf("mixed scale solution %v", x)
	}
	biCheck(t, a, b, r, o)
}

func TestBiCGSTABZeroRHSNonzeroGuess(t *testing.T) {
	a := diagonal(t, []float64{2, 2})
	o := DefaultBiCGSTABOptions()
	o.InitialGuess = []float64{1, 2}
	r, e := SolveBiCGSTAB(context.Background(), a, []float64{0, 0}, o)
	if e != nil {
		t.Fatal(e)
	}
	equalVec(t, r.Solution(), []float64{0, 0})
	if r.RelativeResidual() != 0 || r.Iterations() != 1 {
		t.Fatal("zero RHS solve diagnostics")
	}
}
