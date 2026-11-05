package sparse

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func checkSolution(t testing.TB, a *CSR, b, want []float64, o CGOptions) *CGResult {
	t.Helper()
	r, e := SolveCG(context.Background(), a, b, o)
	if e != nil {
		t.Fatal(e)
	}
	closeVec(t, r.Solution(), want, 2e-8)
	// Independent direct float64 residual for modest-sized/scaled tests.
	x := r.Solution()
	num, den := 0., 0.
	for i := range b {
		ax := 0.
		for j := 0; j < len(x); j++ {
			v, e := a.At(i, j)
			if e != nil {
				t.Fatal(e)
			}
			ax += v * x[j]
		}
		num = math.Hypot(num, b[i]-ax)
		den = math.Hypot(den, b[i])
	}
	threshold := math.Max(o.AbsoluteTolerance, o.RelativeTolerance*den)
	if num > math.Max(threshold*2, den*2e-14) {
		t.Fatalf("independent residual %g threshold %g", num, threshold)
	}
	return r
}
func TestCGAnalyticAndScales(t *testing.T) {
	o := DefaultCGOptions()
	a := must(t, 2, 2, []int{0, 2, 4}, []int{0, 1, 0, 1}, []float64{4, 1, 1, 3})
	for _, pre := range []Preconditioner{NoPreconditioner, Jacobi} {
		o.Preconditioner = pre
		r := checkSolution(t, a, []float64{1, 2}, []float64{1. / 11, 7. / 11}, o)
		if r.Iterations() < 1 || r.RelativeResidual() > o.RelativeTolerance {
			t.Fatal(r)
		}
		f, e := r.ResidualNorm()
		if f < 0 || f >= 1 || math.Ldexp(f, e) > 1e-9 {
			t.Fatal(f, e)
		}
	}
	for _, scale := range []float64{1e-300, 1e300, math.SmallestNonzeroFloat64} {
		for _, pre := range []Preconditioner{NoPreconditioner, Jacobi} {
			o.Preconditioner = pre
			a := diagonal(t, []float64{scale, scale})
			r, e := SolveCG(context.Background(), a, []float64{scale, scale}, o)
			if e != nil {
				t.Fatalf("scale %g pre %d: %v", scale, pre, e)
			}
			equalVec(t, r.Solution(), []float64{1, 1})
		}
	}
	a = diagonal(t, []float64{1e-150, 1e150})
	o.Preconditioner = Jacobi
	r, e := SolveCG(context.Background(), a, []float64{2e-150, 3e150}, o)
	if e != nil {
		t.Fatal(e)
	}
	closeVec(t, r.Solution(), []float64{2, 3}, 1e-14)
	// The norm may exceed binary64 even though every input, solution and residual
	// component is representable. Scaled diagnostics remain meaningful.
	a = diagonal(t, []float64{1, 1, 1, 1})
	o = DefaultCGOptions()
	o.AbsoluteTolerance = math.MaxFloat64
	o.InitialGuess = []float64{0, 0, 0, 0}
	r, e = SolveCG(context.Background(), a, []float64{1e308, 1e308, 1e308, 1e308}, o)
	if e != nil {
		t.Fatal(e)
	}
	closeVec(t, r.Solution(), []float64{1e308, 1e308, 1e308, 1e308}, 1e-14)
}
func TestCGEmptyZeroInitialAndTolerance(t *testing.T) {
	var zero CSR
	o := DefaultCGOptions()
	r, e := SolveCG(context.Background(), &zero, nil, o)
	if e != nil || r.Iterations() != 0 || len(r.Solution()) != 0 {
		t.Fatal(r, e)
	}
	a := diagonal(t, []float64{2, 4})
	r = checkSolution(t, a, []float64{0, 0}, []float64{0, 0}, o)
	if r.Iterations() != 0 {
		t.Fatal(r.Iterations())
	}
	guess := []float64{1, 2}
	o.InitialGuess = guess
	r = checkSolution(t, a, []float64{2, 8}, guess, o)
	if r.Iterations() != 0 {
		t.Fatal("exact initial")
	}
	guess[0] = 99
	x := r.Solution()
	x[0] = 100
	equalVec(t, r.Solution(), []float64{1, 2})
	o = CGOptions{RelativeTolerance: 0, AbsoluteTolerance: 1e-5, InitialGuess: []float64{1e-7, 0}}
	r, e = SolveCG(context.Background(), a, []float64{0, 0}, o)
	if e != nil || r.Iterations() != 0 || !math.IsInf(r.RelativeResidual(), 1) {
		t.Fatal(r, e)
	}
	// Literal zero tolerances can succeed when the actual residual is exact.
	r, e = SolveCG(context.Background(), diagonal(t, []float64{2}), []float64{4}, CGOptions{})
	if e != nil {
		t.Fatal(e)
	}
	equalVec(t, r.Solution(), []float64{2})
	var nilResult *CGResult
	if nilResult.Solution() != nil || nilResult.Iterations() != 0 || nilResult.RelativeResidual() != 0 {
		t.Fatal("nil result")
	}
	if f, e := nilResult.ResidualNorm(); f != 0 || e != 0 {
		t.Fatal(f, e)
	}
}
func TestCGRejectsNoPartial(t *testing.T) {
	a := diagonal(t, []float64{2, 4})
	valid := DefaultCGOptions()
	cases := []struct {
		name string
		a    *CSR
		b    []float64
		o    CGOptions
		err  error
	}{
		{"nil", nil, []float64{1, 2}, valid, ErrShape},
		{"rectangular", must(t, 1, 2, []int{0, 1}, []int{0}, []float64{1}), []float64{1}, valid, ErrShape},
		{"rhslen", a, []float64{1}, valid, ErrShape},
		{"rhsnan", a, []float64{math.NaN(), 1}, valid, ErrNonFinite},
		{"asymmetric", must(t, 2, 2, []int{0, 2, 3}, []int{0, 1, 1}, []float64{2, 1, 2}), []float64{1, 2}, valid, ErrNotSymmetric},
		{"negative", diagonal(t, []float64{-1, -2}), []float64{1, 2}, valid, ErrBreakdown},
		{"zero", diagonal(t, []float64{0, 0}), []float64{1, 2}, valid, ErrBreakdown},
		{"singular", diagonal(t, []float64{1, 0}), []float64{0, 1}, valid, ErrBreakdown},
		{"range", diagonal(t, []float64{math.SmallestNonzeroFloat64, 1}), []float64{math.MaxFloat64, 0}, valid, ErrRange},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r, e := SolveCG(context.Background(), c.a, c.b, c.o)
			if r != nil || !errors.Is(e, c.err) {
				t.Fatalf("%v %v expected %v", r, e, c.err)
			}
		})
	}
	for _, mutate := range []func(*CGOptions){func(o *CGOptions) { o.MaxIterations = -1 }, func(o *CGOptions) { o.MaxIterations = 100001 }, func(o *CGOptions) { o.RelativeTolerance = -1 }, func(o *CGOptions) { o.AbsoluteTolerance = math.NaN() }, func(o *CGOptions) { o.RelativeTolerance = math.Inf(1) }, func(o *CGOptions) { o.Preconditioner = 9 }, func(o *CGOptions) { o.Limits.MaxDimension = -1 }} {
		o := valid
		mutate(&o)
		if r, e := SolveCG(context.Background(), a, []float64{1, 2}, o); r != nil || !errors.Is(e, ErrOptions) {
			t.Fatal(r, e)
		}
	}
	o := valid
	o.InitialGuess = []float64{1}
	if r, e := SolveCG(context.Background(), a, []float64{1, 2}, o); r != nil || !errors.Is(e, ErrShape) {
		t.Fatal(r, e)
	}
	o.InitialGuess = []float64{1, math.Inf(1)}
	if r, e := SolveCG(context.Background(), a, []float64{1, 2}, o); r != nil || !errors.Is(e, ErrNonFinite) {
		t.Fatal(r, e)
	}
	o = valid
	o.MaxIterations = 1
	if r, e := SolveCG(context.Background(), a, []float64{1, 2}, o); r != nil || !errors.Is(e, ErrConvergence) {
		t.Fatal(r, e)
	}
	o = valid
	o.Preconditioner = Jacobi
	if r, e := SolveCG(context.Background(), diagonal(t, []float64{0, 2}), []float64{1, 2}, o); r != nil || !errors.Is(e, ErrBreakdown) {
		t.Fatal(r, e)
	}
	for _, l := range []Limits{{MaxWork: 1}, {MaxWork: 100}, {MaxWork: 500}, {MaxDimension: 1}, {MaxStorageBytes: 80}, {MaxNNZ: 1}} {
		o = valid
		o.Limits = l
		if r, e := SolveCG(context.Background(), a, []float64{1, 2}, o); r != nil || !errors.Is(e, ErrLimit) {
			t.Fatal(r, e)
		}
	}
	if r, e := SolveCG(nil, a, []float64{1, 2}, valid); r != nil || !errors.Is(e, ErrOptions) {
		t.Fatal(r, e)
	}
	// Symmetric indefinite matrices can succeed for selected RHSs; this is not an
	// SPD certificate. Negative curvature on another RHS must fail.
	indef := diagonal(t, []float64{1, -1})
	checkSolution(t, indef, []float64{1, 0}, []float64{1, 0}, valid)
	if r, e := SolveCG(context.Background(), indef, []float64{0, 1}, valid); r != nil || !errors.Is(e, ErrBreakdown) {
		t.Fatal(r, e)
	}
}
func TestCGExtremeDynamicRangeRejects(t *testing.T) {
	o := DefaultCGOptions()
	a := diagonal(t, []float64{1e-300, 1e300})
	if r, e := SolveCG(context.Background(), a, []float64{1e-300, 1e300}, o); r != nil || !errors.Is(e, ErrRange) {
		t.Fatal(r, e)
	}
	o.Preconditioner = Jacobi
	if r, e := SolveCG(context.Background(), a, []float64{1e300, 1e300}, o); r != nil || !errors.Is(e, ErrRange) {
		t.Fatal(r, e)
	}
	// A finite absolute tolerance can accept a finite residual while its ratio
	// to a tiny nonzero RHS is unrepresentable. Do not return that overflow.
	o = DefaultCGOptions()
	o.AbsoluteTolerance = math.MaxFloat64
	o.InitialGuess = []float64{1e300}
	if r, e := SolveCG(context.Background(), diagonal(t, []float64{1}), []float64{1e-300}, o); r != nil || !errors.Is(e, ErrRange) {
		t.Fatal(r, e)
	}
	// An arbitrarily small asymmetry is still an explicit rejection.
	o = DefaultCGOptions()
	a = must(t, 2, 2, []int{0, 2, 4}, []int{0, 1, 0, 1}, []float64{2, 1, math.Nextafter(1, 2), 2})
	if r, e := SolveCG(context.Background(), a, []float64{1, 1}, o); r != nil || !errors.Is(e, ErrNotSymmetric) {
		t.Fatal(r, e)
	}
}

func TestCGOriginalResidualAndIllConditioned(t *testing.T) {
	// Analytic diagonal problem spanning 12 orders; Jacobi solves in one step.
	a := diagonal(t, []float64{1e-12, 1e-6, 1, 1e6})
	o := DefaultCGOptions()
	o.Preconditioner = Jacobi
	o.RelativeTolerance = 1e-13
	r := checkSolution(t, a, []float64{1e-12, 2e-6, 3, 4e6}, []float64{1, 2, 3, 4}, o)
	if r.Iterations() != 1 {
		t.Fatal(r.Iterations())
	}
	// Literal zero tolerances reject a nonzero true residual even if recurrence
	// residual drifts to zero. Analytic 1/3 cannot be represented exactly.
	o = CGOptions{MaxIterations: 4}
	a = diagonal(t, []float64{3})
	r, e := SolveCG(context.Background(), a, []float64{1}, o)
	if r != nil || !errors.Is(e, ErrConvergence) {
		t.Fatal(r, e)
	}
}

type countedContext struct {
	context.Context
	cancel context.CancelFunc
	calls  atomic.Int64
	after  int64
}

func (c *countedContext) Err() error {
	if c.calls.Add(1) >= c.after {
		c.cancel()
	}
	return c.Context.Err()
}
func TestCGCancellation(t *testing.T) {
	a := diagonal(t, []float64{1, 2, 3, 4})
	b := []float64{1, 2, 3, 4}
	o := DefaultCGOptions()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if r, e := SolveCG(ctx, a, b, o); r != nil || !errors.Is(e, context.Canceled) {
		t.Fatal(r, e)
	}
	ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	if r, e := SolveCG(ctx, a, b, o); r != nil || !errors.Is(e, context.DeadlineExceeded) {
		t.Fatal(r, e)
	}
	for _, after := range []int64{3, 9, 20, 30} {
		ctx, cancel = context.WithCancel(context.Background())
		c := &countedContext{Context: ctx, cancel: cancel, after: after}
		r, e := SolveCG(c, a, b, o)
		cancel()
		if r != nil || !errors.Is(e, context.Canceled) {
			t.Fatalf("after %d: %v %v", after, r, e)
		}
	}
}
func TestCGCancellationEveryCheckpoint(t *testing.T) {
	a := tridiagonal(t, 8)
	b := []float64{1, 2, 3, 4, 5, 6, 7, 8}
	o := DefaultCGOptions()
	ctx, cancel := context.WithCancel(context.Background())
	probe := &countedContext{Context: ctx, cancel: cancel, after: math.MaxInt64}
	if _, e := SolveCG(probe, a, b, o); e != nil {
		t.Fatal(e)
	}
	cancel()
	total := probe.calls.Load()
	for after := int64(1); after <= total; after++ {
		ctx, cancel = context.WithCancel(context.Background())
		c := &countedContext{Context: ctx, cancel: cancel, after: after}
		r, e := SolveCG(c, a, b, o)
		cancel()
		if r != nil || !errors.Is(e, context.Canceled) {
			t.Fatalf("checkpoint %d/%d: %v %v", after, total, r, e)
		}
	}
}
func TestCGLongResidualRestart(t *testing.T) {
	n := 80
	p := make([]int, n+1)
	var c []int
	var v []float64
	for i := 0; i < n; i++ {
		if i > 0 {
			c = append(c, i-1)
			v = append(v, -1)
		}
		c = append(c, i)
		v = append(v, 2)
		if i+1 < n {
			c = append(c, i+1)
			v = append(v, -1)
		}
		p[i+1] = len(v)
	}
	a := must(t, n, n, p, c, v)
	b := make([]float64, n)
	b[0] = 1
	b[n-1] = 1
	o := DefaultCGOptions()
	o.RelativeTolerance = 1e-9
	r, e := SolveCG(context.Background(), a, b, o)
	if e != nil {
		t.Fatal(e)
	}
	if r.Iterations() <= 32 {
		t.Fatal("restart not exercised", r.Iterations())
	}
	want := make([]float64, n)
	for i := range want {
		want[i] = 1
	}
	closeVec(t, r.Solution(), want, 1e-7)
	// Independent residual uses only three small products per row.
	x := r.Solution()
	rn := 0.
	for i := range x {
		ax := 2 * x[i]
		if i > 0 {
			ax -= x[i-1]
		}
		if i+1 < n {
			ax -= x[i+1]
		}
		rn = math.Hypot(rn, b[i]-ax)
	}
	if rn > o.RelativeTolerance*math.Sqrt2*1.01 {
		t.Fatal(rn)
	}
}

func TestCGSciPyReferences(t *testing.T) {
	var f struct {
		Numpy, Scipy string
		Cases        []struct {
			Name                        string
			Rows, Cols                  int
			Rowptr, Colind              []int
			Values, B, Solution, Matvec []float64
			OriginalResidual            float64 `json:"original_residual"`
		}
	}
	data, e := os.ReadFile("scipy.json")
	if e != nil {
		t.Fatal(e)
	}
	if e = json.Unmarshal(data, &f); e != nil {
		t.Fatal(e)
	}
	if len(f.Cases) < 4 || f.Scipy == "" {
		t.Fatal("missing provenance")
	}
	for _, c := range f.Cases {
		t.Run(c.Name, func(t *testing.T) {
			a := must(t, c.Rows, c.Cols, c.Rowptr, c.Colind, c.Values)
			x := make([]float64, c.Cols)
			for i := range x {
				x[i] = float64(i)
			}
			y, e := a.MulVec(x)
			if e != nil {
				t.Fatal(e)
			}
			for i := range y {
				if math.Abs(y[i]-c.Matvec[i]) > math.Max(math.Abs(c.Matvec[i]), math.Abs(c.Values[0]))*2e-14 {
					t.Fatal("matvec", i, y[i], c.Matvec[i])
				}
			}
			for _, pre := range []Preconditioner{NoPreconditioner, Jacobi} {
				o := DefaultCGOptions()
				o.RelativeTolerance = 1e-11
				o.Preconditioner = pre
				checkSolution(t, a, c.B, c.Solution, o)
			}
		})
	}
}
func TestCGConcurrentOwnership(t *testing.T) {
	a := diagonal(t, []float64{2, 3, 4, 5})
	b := []float64{2, 6, 12, 20}
	o := DefaultCGOptions()
	o.Preconditioner = Jacobi
	o.InitialGuess = []float64{0, 0, 0, 0}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 8; j++ {
				r, e := SolveCG(context.Background(), a, b, o)
				if e != nil {
					t.Error(e)
					return
				}
				x := r.Solution()
				x[0] = 100
				equalVec(t, r.Solution(), []float64{1, 2, 3, 4})
			}
		}()
	}
	wg.Wait()
	equalVec(t, b, []float64{2, 6, 12, 20})
	equalVec(t, o.InitialGuess, []float64{0, 0, 0, 0})
}
