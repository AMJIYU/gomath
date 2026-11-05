package linalg_test

import (
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"sync"
	"testing"

	"gomath/linalg"
)

func spd(t testing.TB, n int) *linalg.Dense {
	t.Helper()
	var (
		rng  = rand.New(rand.NewPCG(uint64(n+1), 79))
		g    = make([]float64, n*n)
		data = make([]float64, n*n)
	)
	for i := range g {
		g[i] = rng.Float64()*2 - 1
	}
	// Independent SPD fixture construction: G*G^T+I. This is not used by
	// least-squares solvers and does not construct normal equations for a fit.
	for i := range n {
		for j := 0; j <= i; j++ {
			var value float64
			for k := range n {
				value += g[i*n+k] * g[j*n+k]
			}
			if i == j {
				value++
			}
			data[i*n+j], data[j*n+i] = value, value
		}
	}
	return dense(t, n, n, data...)
}

func checkCholesky(t *testing.T, a *linalg.Dense, f *linalg.Cholesky) {
	t.Helper()
	n, _ := a.Dimensions()
	if f.Size() != n {
		t.Fatal("factor size", f.Size(), n)
	}
	l, err := f.Lower()
	if err != nil {
		t.Fatal(err)
	}
	lt, err := l.Transpose()
	if err != nil {
		t.Fatal(err)
	}
	requireResidual(t, l, lt, a, 3e-15)
	ld := l.Data()
	for i := range n {
		if ld[i*n+i] <= 0 {
			t.Fatal("nonpositive Cholesky diagonal")
		}
		for j := i + 1; j < n; j++ {
			if ld[i*n+j] != 0 {
				t.Fatal("nonzero upper Cholesky entry")
			}
		}
	}
}

func TestCholeskyKnownFactorAndSolves(t *testing.T) {
	a := dense(t, 3, 3, 4, 12, -16, 12, 37, -43, -16, -43, 98)
	original := a.Data()
	f, err := linalg.FactorizeCholesky(a)
	if err != nil {
		t.Fatal(err)
	}
	checkCholesky(t, a, f)
	l, err := f.Lower()
	if err != nil {
		t.Fatal(err)
	}
	equalData(t, l, 3, 3, []float64{2, 0, 0, 6, 1, 0, -8, 5, 3})
	b := []float64{-20, -43, 192}
	for range 3 {
		x, err := f.Solve(b)
		if err != nil || !slices.Equal(x, []float64{1, 2, 3}) {
			t.Fatal("Cholesky solution", x, err)
		}
		x[0] = 99
	}
	ld := l.Data()
	ld[0] = math.NaN()
	checkCholesky(t, a, f)
	if !slices.Equal(a.Data(), original) || !slices.Equal(b, []float64{-20, -43, 192}) {
		t.Fatal("Cholesky modified input")
	}
	x, err := linalg.SolveSPD(a, b)
	if err != nil || !slices.Equal(x, []float64{1, 2, 3}) {
		t.Fatal("one-shot Cholesky solution", x, err)
	}
}

func TestCholeskyMultiRHSResiduals(t *testing.T) {
	for _, n := range []int{1, 2, 7, 32, 64} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			var (
				a    = spd(t, n)
				ad   = a.Data()
				xdat = make([]float64, n*2)
				bdat = make([]float64, n*2)
			)
			for i := range xdat {
				xdat[i] = math.Sin(float64(i + 1))
			}
			for i := range n {
				for c := range 2 {
					for j := range n {
						bdat[i*2+c] += ad[i*n+j] * xdat[j*2+c]
					}
				}
			}
			b := dense(t, n, 2, bdat...)
			f, err := linalg.FactorizeCholesky(a)
			if err != nil {
				t.Fatal(err)
			}
			checkCholesky(t, a, f)
			x, err := f.SolveMatrix(b)
			if err != nil {
				t.Fatal(err)
			}
			requireResidual(t, a, x, b, 3e-15)
			for i, got := range x.Data() {
				if math.Abs(got-xdat[i]) > 3e-14 {
					t.Fatal("Cholesky forward error", i, got, xdat[i])
				}
			}
			for c := range 2 {
				column := make([]float64, n)
				for i := range n {
					column[i] = bdat[i*2+c]
				}
				xv, err := f.Solve(column)
				if err != nil {
					t.Fatal(err)
				}
				for i, v := range xv {
					got, _ := x.At(i, c)
					if got != v {
						t.Fatal("vector/multi-RHS solve mismatch")
					}
				}
			}
			oneShot, err := linalg.SolveSPDMatrix(a, b)
			if err != nil || !slices.Equal(oneShot.Data(), x.Data()) {
				t.Fatal("one-shot matrix solve", err)
			}
			if !slices.Equal(a.Data(), ad) || !slices.Equal(b.Data(), bdat) {
				t.Fatal("Cholesky modified A or B")
			}
		})
	}
}

func TestCholeskyPositiveDefinitePolicy(t *testing.T) {
	for _, a := range []*linalg.Dense{
		dense(t, 1, 1, 0),
		dense(t, 1, 1, -1),
		dense(t, 2, 2, 1, 1, 1, 1),
		dense(t, 2, 2, 1, 2, 2, 1),
	} {
		for _, opt := range []linalg.CholeskyOptions{linalg.DefaultCholeskyOptions(), {}} {
			f, err := linalg.FactorizeCholeskyWithOptions(a, opt)
			var pd *linalg.PositiveDefiniteError
			if f != nil || !errors.Is(err, linalg.ErrNotPositiveDefinite) || !errors.As(err, &pd) || pd.Value > 0 || pd.Error() == "" {
				t.Fatal("positive definite rejection", f, err)
			}
		}
		if _, err := linalg.SolveSPD(a, make([]float64, aRows(a))); !errors.Is(err, linalg.ErrNotPositiveDefinite) {
			t.Fatal(err)
		}
		if _, err := linalg.SolveSPDMatrix(a, a); !errors.Is(err, linalg.ErrNotPositiveDefinite) {
			t.Fatal(err)
		}
	}
	near := dense(t, 2, 2, 1, 1, 1, 1+2e-15)
	if _, err := linalg.FactorizeCholesky(near); !errors.Is(err, linalg.ErrNotPositiveDefinite) {
		t.Fatal("near-semidefinite pivot accepted by default", err)
	}
	f, err := linalg.FactorizeCholeskyWithOptions(near, linalg.CholeskyOptions{})
	if err != nil {
		t.Fatal(err)
	}
	x, err := f.Solve([]float64{2, 2 + 2e-15})
	if err != nil {
		t.Fatal(err)
	}
	requireResidual(t, near, dense(t, 2, 1, x...), dense(t, 2, 1, 2, 2+2e-15), 1e-15)
	checkCholesky(t, near, f)
}

func TestCholeskySymmetryPolicy(t *testing.T) {
	a := dense(t, 2, 2, 2, 1, 1+1e-15, 2)
	original := a.Data()
	f, err := linalg.FactorizeCholesky(a)
	var symmetry *linalg.SymmetryError
	if f != nil || !errors.Is(err, linalg.ErrNotSymmetric) || !errors.As(err, &symmetry) || symmetry.Row != 1 || symmetry.Column != 0 || symmetry.Error() == "" {
		t.Fatal("exact symmetry rejection", f, err)
	}
	f, err = linalg.FactorizeCholeskyWithOptions(a, linalg.CholeskyOptions{RelativeSymmetryTolerance: 1e-14})
	if err != nil {
		t.Fatal(err)
	}
	midpoint := (original[2] + original[1]) / 2
	symmetric := dense(t, 2, 2, 2, midpoint, midpoint, 2)
	checkCholesky(t, symmetric, f)
	x, err := f.Solve([]float64{2 + midpoint, 2 + midpoint})
	if err != nil {
		t.Fatal(err)
	}
	requireResidual(t, symmetric, dense(t, 2, 1, x...), dense(t, 2, 1, 2+midpoint, 2+midpoint), 1e-15)
	if !slices.Equal(a.Data(), original) {
		t.Fatal("approximate symmetry changed the input")
	}
	for _, pair := range [][2]float64{{0, math.SmallestNonzeroFloat64}, {math.MaxFloat64, -math.MaxFloat64}} {
		a := dense(t, 2, 2, 1, pair[0], pair[1], 1)
		_, err := linalg.FactorizeCholeskyWithOptions(a, linalg.CholeskyOptions{RelativeSymmetryTolerance: .5})
		if !errors.As(err, &symmetry) || !finiteTest(symmetry.Relative) {
			t.Fatal("safe pair-relative symmetry validation", err)
		}
	}
	// An accepted subnormal pair is averaged before halving, rather than
	// rounding each half or half their one-subnormal difference to zero.
	s := math.SmallestNonzeroFloat64
	f, err = linalg.FactorizeCholeskyWithOptions(dense(t, 2, 2, 16*s, 3*s, 4*s, 16*s), linalg.CholeskyOptions{RelativeSymmetryTolerance: .3})
	if err != nil {
		t.Fatal(err)
	}
	l, err := f.Lower()
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := l.At(1, 0); got != 4*s/math.Sqrt(16*s) {
		t.Fatal("subnormal midpoint", got)
	}
	// Huge accepted pairs use the other midpoint path; their sum alone
	// would overflow even though the average and the SPD factors are finite.
	large := math.MaxFloat64 * .75
	a = dense(t, 2, 2, math.MaxFloat64, large, math.Nextafter(large, 0), math.MaxFloat64)
	if _, err := linalg.FactorizeCholeskyWithOptions(a, linalg.CholeskyOptions{RelativeSymmetryTolerance: 1e-14}); err != nil {
		t.Fatal("large midpoint overflow", err)
	}
}

func TestCholeskyScalingAndUnderflow(t *testing.T) {
	for _, scale := range []float64{1e-300, 1, 1e300} {
		a := dense(t, 2, 2, 4*scale, 2*scale, 2*scale, 3*scale)
		x, err := linalg.SolveSPD(a, []float64{2 * scale, -scale})
		if err != nil || math.Abs(x[0]-1) > 2e-15 || math.Abs(x[1]+1) > 2e-15 {
			t.Fatal("scaled SPD solve", scale, x, err)
		}
	}
	for _, values := range [][]float64{
		{1e-200, 1e200},
		{math.SmallestNonzeroFloat64, math.MaxFloat64},
	} {
		x, err := linalg.SolveSPD(diagonal(t, values...), values)
		if err != nil {
			t.Fatal(err)
		}
		for _, v := range x {
			if math.Abs(v-1) > 5e-16 {
				t.Fatal("extreme diagonal solution", x)
			}
		}
	}
	// Subnormal off-diagonal entries must not be halved to zero during exact
	// symmetry validation/copying. The lower factor retains a nonzero entry.
	s := math.SmallestNonzeroFloat64
	f, err := linalg.FactorizeCholesky(dense(t, 2, 2, 4*s, s, s, 4*s))
	if err != nil {
		t.Fatal(err)
	}
	l, err := f.Lower()
	if err != nil {
		t.Fatal(err)
	}
	if value, _ := l.At(1, 0); value == 0 {
		t.Fatal("subnormal off-diagonal lost by averaging")
	}
	// Ordinary float64 solution underflow remains allowed, as for LU/Dense.
	x, err := linalg.SolveSPD(diagonal(t, math.MaxFloat64), []float64{s})
	if err != nil || x[0] != 0 {
		t.Fatal("finite solution underflow", x, err)
	}
}

func TestCholeskyEmptyAndErrors(t *testing.T) {
	empty, _ := linalg.Zeros(0, 0)
	f, err := linalg.FactorizeCholesky(empty)
	if err != nil {
		t.Fatal(err)
	}
	checkCholesky(t, empty, f)
	if x, err := f.Solve(nil); err != nil || len(x) != 0 {
		t.Fatal(x, err)
	}
	b, _ := linalg.Zeros(0, 4)
	x, err := f.SolveMatrix(b)
	if err != nil {
		t.Fatal(err)
	}
	equalData(t, x, 0, 4, nil)
	if _, err := linalg.FactorizeCholesky(nil); !errors.Is(err, linalg.ErrInvalid) {
		t.Fatal(err)
	}
	rect := dense(t, 1, 2, 1, 2)
	if _, err := linalg.FactorizeCholesky(rect); !errors.Is(err, linalg.ErrDimension) {
		t.Fatal(err)
	}
	id, _ := linalg.Identity(2)
	for _, tol := range []float64{-1, 1, math.NaN(), math.Inf(1), math.Inf(-1)} {
		for _, opt := range []linalg.CholeskyOptions{{RelativePivotTolerance: tol}, {RelativeSymmetryTolerance: tol}} {
			if _, err := linalg.FactorizeCholeskyWithOptions(id, opt); !errors.Is(err, linalg.ErrTolerance) {
				t.Fatal(err)
			}
		}
	}
	f, _ = linalg.FactorizeCholesky(id)
	if _, err := f.Solve([]float64{1}); !errors.Is(err, linalg.ErrDimension) {
		t.Fatal(err)
	}
	if _, err := f.SolveMatrix(rect); !errors.Is(err, linalg.ErrDimension) {
		t.Fatal(err)
	}
	if _, err := f.SolveMatrix(nil); !errors.Is(err, linalg.ErrInvalid) {
		t.Fatal(err)
	}
	zeroRHS, _ := linalg.Zeros(2, 0)
	x, err = f.SolveMatrix(zeroRHS)
	if err != nil {
		t.Fatal(err)
	}
	equalData(t, x, 2, 0, nil)
	for _, bad := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if _, err := f.Solve([]float64{1, bad}); !errors.Is(err, linalg.ErrNonFinite) {
			t.Fatal(err)
		}
	}
	for _, invalid := range []*linalg.Cholesky{nil, {}} {
		if invalid.Size() != 0 {
			t.Fatal("invalid factor size")
		}
		if _, err := invalid.Lower(); !errors.Is(err, linalg.ErrInvalid) {
			t.Fatal(err)
		}
		if _, err := invalid.Solve(nil); !errors.Is(err, linalg.ErrInvalid) {
			t.Fatal(err)
		}
		if _, err := invalid.SolveMatrix(nil); !errors.Is(err, linalg.ErrInvalid) {
			t.Fatal(err)
		}
	}
}

func TestCholeskyOverflow(t *testing.T) {
	max := math.MaxFloat64
	a := dense(t, 2, 2, math.SmallestNonzeroFloat64, max, max, 1)
	if f, err := linalg.FactorizeCholesky(a); f != nil || !errors.Is(err, linalg.ErrNonFinite) {
		t.Fatal("Cholesky factor arithmetic overflow", f, err)
	}
	a = dense(t, 2, 2, math.SmallestNonzeroFloat64, 1, 1, 1)
	if f, err := linalg.FactorizeCholesky(a); f != nil || !errors.Is(err, linalg.ErrNonFinite) {
		t.Fatal("Cholesky Schur-update overflow", f, err)
	}
	for _, a := range []*linalg.Dense{
		diagonal(t, .5),
		diagonal(t, math.SmallestNonzeroFloat64),
		dense(t, 2, 2, 1, -1, -1, 2),
	} {
		f, err := linalg.FactorizeCholesky(a)
		if err != nil {
			t.Fatal(err)
		}
		b := make([]float64, f.Size())
		for i := range b {
			b[i] = max
		}
		if x, err := f.Solve(b); x != nil || !errors.Is(err, linalg.ErrNonFinite) {
			t.Fatal("Cholesky solve overflow", x, err)
		}
		if x, err := f.SolveMatrix(dense(t, f.Size(), 1, b...)); x != nil || !errors.Is(err, linalg.ErrNonFinite) {
			t.Fatal("Cholesky matrix solve overflow", x, err)
		}
		if _, err := f.Solve(make([]float64, f.Size())); err != nil {
			t.Fatal("failed solve changed factors", err)
		}
	}
}

func TestQRAndCholeskyConcurrentReuse(t *testing.T) {
	var (
		qra   = rectangular(t, 21, 7)
		chola = spd(t, 7)
		group sync.WaitGroup
		errs  = make(chan error, 8)
	)
	qr, err := linalg.FactorizeQR(qra)
	if err != nil {
		t.Fatal(err)
	}
	chol, err := linalg.FactorizeCholesky(chola)
	if err != nil {
		t.Fatal(err)
	}
	qb, cb := make([]float64, 21), make([]float64, 7)
	for i := range qb {
		qb[i] = float64(i + 1)
	}
	copy(cb, qb)
	qx, qnorm, err := qr.Solve(qb)
	if err != nil {
		t.Fatal(err)
	}
	cx, err := chol.Solve(cb)
	if err != nil {
		t.Fatal(err)
	}
	for range 8 {
		group.Go(func() {
			for range 5 {
				x, norm, err := qr.Solve(qb)
				if err != nil || !slices.Equal(x, qx) || norm != qnorm {
					errs <- fmt.Errorf("concurrent QR solve differs: %v", err)
					return
				}
				y, err := chol.Solve(cb)
				if err != nil || !slices.Equal(y, cx) {
					errs <- fmt.Errorf("concurrent Cholesky solve differs: %v", err)
					return
				}
			}
		})
	}
	group.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}
