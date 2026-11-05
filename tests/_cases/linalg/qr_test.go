package linalg_test

import (
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"testing"

	"gomath/linalg"
)

func rectangular(t testing.TB, m, n int) *linalg.Dense {
	t.Helper()
	rng := rand.New(rand.NewPCG(uint64(m+1), uint64(n+19)))
	data := make([]float64, m*n)
	for i := range m {
		for j := range n {
			data[i*n+j] = rng.Float64()*2 - 1
			if i == j {
				data[i*n+j] += float64(n + 1)
			}
		}
	}
	return dense(t, m, n, data...)
}

func checkQRFactors(t *testing.T, a *linalg.Dense, f *linalg.QR) {
	t.Helper()
	m, n := a.Dimensions()
	if fm, fn := f.Dimensions(); fm != m || fn != n {
		t.Fatal("QR dimensions", fm, fn)
	}
	q, r, err := f.Factors()
	if err != nil {
		t.Fatal(err)
	}
	if qm, qn := q.Dimensions(); qm != m || qn != n {
		t.Fatal("thin Q dimensions", qm, qn)
	}
	if rm, rn := r.Dimensions(); rm != n || rn != n {
		t.Fatal("R dimensions", rm, rn)
	}
	var (
		qd = q.Data()
		rd = r.Data()
		ad = a.Data()
		ap = make([]float64, len(ad))
	)
	seen := make([]bool, n)
	for j, p := range f.Pivots() {
		if p < 0 || p >= n || seen[p] {
			t.Fatal("invalid column permutation", f.Pivots())
		}
		seen[p] = true
		for i := range m {
			ap[i*n+j] = ad[i*n+p]
		}
	}
	requireResidual(t, q, r, dense(t, m, n, ap...), 3e-15)
	// Independent Q^T*Q evaluation, without calling the multiplication kernel.
	for i := range n {
		for j := range n {
			var sum float64
			for k := range m {
				sum += qd[k*n+i] * qd[k*n+j]
			}
			want := 0.0
			if i == j {
				want = 1
			}
			if math.Abs(sum-want) > 6e-15 {
				t.Fatalf("Q^T*Q[%d,%d]=%g", i, j, sum)
			}
			if i > j && rd[i*n+j] != 0 {
				t.Fatal("R not upper triangular")
			}
		}
	}
}

func actualResidual(a, x, b *linalg.Dense, col int) float64 {
	var (
		m, n   = a.Dimensions()
		_, rhs = b.Dimensions()
		ad     = a.Data()
		xd     = x.Data()
		bd     = b.Data()
		norm   float64
	)
	for i := range m {
		var sum float64
		for j := range n {
			sum += ad[i*n+j] * xd[j*rhs+col]
		}
		norm = math.Hypot(norm, sum-bd[i*rhs+col])
	}
	return norm
}

func TestQRLineFitAndOwnership(t *testing.T) {
	a := dense(t, 3, 2, 1, 0, 1, 1, 1, 2)
	original := a.Data()
	b := []float64{1, 2, 2}
	f, err := linalg.FactorizeQR(a)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(f.Pivots(), []int{1, 0}) {
		t.Fatal("expected column pivot", f.Pivots())
	}
	checkQRFactors(t, a, f)
	for range 3 {
		x, norm, err := f.Solve(b)
		if err != nil || math.Abs(x[0]-7.0/6) > 2e-15 || math.Abs(x[1]-.5) > 2e-15 || math.Abs(norm-math.Sqrt(1.0/6)) > 1e-15 {
			t.Fatalf("fit = %v, residual %g, %v", x, norm, err)
		}
		x[0] = 99
	}
	q, r, err := f.Factors()
	if err != nil {
		t.Fatal(err)
	}
	qd, rd, p := q.Data(), r.Data(), f.Pivots()
	qd[0], rd[0], p[0] = math.NaN(), math.Inf(1), 99
	checkQRFactors(t, a, f)
	if !slices.Equal(a.Data(), original) || !slices.Equal(b, []float64{1, 2, 2}) {
		t.Fatal("QR modified inputs")
	}
	x, norm, err := linalg.LeastSquares(a, b)
	if err != nil || math.Abs(x[0]-7.0/6) > 2e-15 || math.Abs(norm-math.Sqrt(1.0/6)) > 1e-15 {
		t.Fatal("one-shot least squares", x, norm, err)
	}
}

func TestQRRectangularMultiRHS(t *testing.T) {
	for _, shape := range [][2]int{{1, 1}, {3, 1}, {5, 3}, {32, 7}, {64, 17}} {
		t.Run(fmt.Sprintf("%dx%d", shape[0], shape[1]), func(t *testing.T) {
			var (
				m, n = shape[0], shape[1]
				a    = rectangular(t, m, n)
				ad   = a.Data()
				bd   = make([]float64, m*2)
			)
			for i := range m {
				for c := range 2 {
					for j := range n {
						bd[i*2+c] += ad[i*n+j] * math.Sin(float64(j+c+1))
					}
				}
				bd[i*2+1] += .1 * math.Cos(float64(i+1))
			}
			b := dense(t, m, 2, bd...)
			f, err := linalg.FactorizeQR(a)
			if err != nil {
				t.Fatal(err)
			}
			checkQRFactors(t, a, f)
			x, norms, err := f.SolveMatrix(b)
			if err != nil || len(norms) != 2 {
				t.Fatal(norms, err)
			}
			for j := range n {
				got, _ := x.At(j, 0)
				if math.Abs(got-math.Sin(float64(j+1))) > 2e-14 {
					t.Fatalf("exact column solution[%d] = %g", j, got)
				}
			}
			for c, norm := range norms {
				want := actualResidual(a, x, b, c)
				if math.Abs(norm-want) > 5e-13 || !finiteTest(norm) {
					t.Fatalf("residual %g, independent %g", norm, want)
				}
				column := make([]float64, m)
				for i := range m {
					column[i] = bd[i*2+c]
				}
				xv, rv, err := f.Solve(column)
				if err != nil || math.Abs(rv-norm) > 1e-13 {
					t.Fatal("vector vs matrix solve", rv, norm, err)
				}
				for j, v := range xv {
					got, _ := x.At(j, c)
					if got != v {
						t.Fatal("vector and multi-RHS differ")
					}
				}
				// Independent first-order optimality test: A^T*(A*x-b)=0.
				for j := range n {
					var stationarity float64
					for i := range m {
						e := -bd[i*2+c]
						for k := range n {
							v, _ := x.At(k, c)
							e += ad[i*n+k] * v
						}
						stationarity += ad[i*n+j] * e
					}
					if math.Abs(stationarity) > 3e-11 {
						t.Fatalf("stationarity column %d component %d: %g", c, j, stationarity)
					}
				}
			}
			oneShot, oneNorms, err := linalg.LeastSquaresMatrix(a, b)
			if err != nil || !slices.Equal(oneShot.Data(), x.Data()) || !slices.Equal(oneNorms, norms) {
				t.Fatal("one-shot multi-RHS mismatch", err)
			}
			norms[0] = 99
			xd := x.Data()
			xd[0] = math.NaN()
			if !slices.Equal(a.Data(), ad) || !slices.Equal(b.Data(), bd) {
				t.Fatal("QR modified matrix input")
			}
		})
	}
}

func finiteTest(x float64) bool { return !math.IsNaN(x) && !math.IsInf(x, 0) }

func TestQRRankPolicy(t *testing.T) {
	for _, a := range []*linalg.Dense{
		dense(t, 3, 1, 0, 0, 0),
		dense(t, 3, 2, 1, 1, 0, 0, 0, 0),
		dense(t, 3, 2, 1, 2, 2, 4, 3, 6),
	} {
		f, err := linalg.FactorizeQR(a)
		var rank *linalg.RankError
		if f != nil || !errors.Is(err, linalg.ErrRankDeficient) || !errors.As(err, &rank) || rank.Rank >= rank.Columns || rank.Error() == "" {
			t.Fatalf("rank error = %v, %v", f, err)
		}
		if _, _, err := linalg.LeastSquares(a, make([]float64, aRows(a))); !errors.Is(err, linalg.ErrRankDeficient) {
			t.Fatal(err)
		}
		if _, _, err := linalg.LeastSquaresMatrix(a, a); !errors.Is(err, linalg.ErrRankDeficient) {
			t.Fatal(err)
		}
	}
	near := dense(t, 3, 2, 1, 1, 0, 1e-16, 0, 0)
	if _, err := linalg.FactorizeQR(near); !errors.Is(err, linalg.ErrRankDeficient) {
		t.Fatal("near-dependent columns were not rejected", err)
	}
	for _, tol := range []float64{0, 1e-17} {
		f, err := linalg.FactorizeQRWithOptions(near, linalg.QROptions{RelativeRankTolerance: tol})
		if err != nil {
			t.Fatal(err)
		}
		x, norm, err := f.Solve([]float64{2, 1e-16, 1e-20})
		if err != nil || !slices.Equal(x, []float64{1, 1}) || math.Abs(norm-1e-20) > 1e-35 {
			t.Fatal("explicit near-dependent solve", x, norm, err)
		}
	}
	// Near dependence after nontrivial Householder updates, rather than just
	// an already triangular example. The relaxed policy is not an accuracy
	// promise for the ill-conditioned individual solution coordinates.
	near = dense(t, 4, 2, 1, 1, 1, 1+2e-15, 1, 1-2e-15, 1, 1+4e-15)
	if _, err := linalg.FactorizeQR(near); !errors.Is(err, linalg.ErrRankDeficient) {
		t.Fatal("reflected near-dependent system accepted", err)
	}
	nearFactor, err := linalg.FactorizeQRWithOptions(near, linalg.QROptions{RelativeRankTolerance: 1e-16})
	if err != nil {
		t.Fatal(err)
	}
	nearB := []float64{1, 1 - 2e-15, 1 + 2e-15, 1 - 4e-15}
	nearX, nearNorm, err := nearFactor.Solve(nearB)
	if err != nil || !finiteTest(nearNorm) {
		t.Fatal("reflected near-dependent solve", nearX, nearNorm, err)
	}
	requireResidual(t, near, dense(t, 2, 1, nearX...), dense(t, 4, 1, nearB...), 1e-15)
	for _, v := range []float64{.5, math.Nextafter(.5, 1)} {
		_, err := linalg.FactorizeQRWithOptions(diagonal(t, 1, v), linalg.QROptions{RelativeRankTolerance: .5})
		if v == .5 && !errors.Is(err, linalg.ErrRankDeficient) || v > .5 && err != nil {
			t.Fatal("rank threshold boundary", v, err)
		}
	}
	extreme := diagonal(t, 1e-300, 1e300)
	if _, err := linalg.FactorizeQR(extreme); !errors.Is(err, linalg.ErrRankDeficient) {
		t.Fatal("global rank policy should reject disparate column norms", err)
	}
	f, err := linalg.FactorizeQRWithOptions(extreme, linalg.QROptions{})
	if err != nil {
		t.Fatal(err)
	}
	// This checks acceptance, not recovery of a tiny coordinate in an RHS
	// whose entries span 600 orders of magnitude (beyond float64 conditioning).
	x, _, err := f.Solve([]float64{0, 1e300})
	if err != nil || !slices.Equal(x, []float64{0, 1}) {
		t.Fatal("exact-zero rank policy", x, err)
	}
}

func TestQRResidualPolicy(t *testing.T) {
	a := dense(t, 3, 2, 1, 0, 1, 1, 1, 2)
	f, err := linalg.FactorizeQR(a)
	if err != nil {
		t.Fatal(err)
	}
	b := []float64{1, 2, 2}
	for _, tolerance := range []float64{0, .1} {
		x, norm, err := f.SolveWithOptions(b, linalg.LeastSquaresOptions{CheckResidual: true, RelativeResidualTolerance: tolerance})
		var re *linalg.ResidualError
		if x != nil || norm != 0 || !errors.Is(err, linalg.ErrResidual) || !errors.As(err, &re) || re.RHS != 0 || re.Norm <= 0 || re.Error() == "" {
			t.Fatal("residual rejection", x, norm, err)
		}
	}
	if _, _, err := f.SolveWithOptions(b, linalg.LeastSquaresOptions{CheckResidual: true, RelativeResidualTolerance: .2}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.SolveWithOptions([]float64{0, 0, 0}, linalg.LeastSquaresOptions{CheckResidual: true}); err != nil {
		t.Fatal("zero residual rejected", err)
	}
	bm := dense(t, 3, 2, 1, 1, 1, 2, 1, 2)
	x, norms, err := f.SolveMatrixWithOptions(bm, linalg.LeastSquaresOptions{CheckResidual: true, RelativeResidualTolerance: .1})
	var re *linalg.ResidualError
	if x != nil || norms != nil || !errors.As(err, &re) || re.RHS != 1 {
		t.Fatal("wrong rejected RHS", x, norms, err)
	}
	// A zero-tolerance check tests actual residuals, not just the transformed
	// tail (which would be empty for this square underflowing solve).
	f, err = linalg.FactorizeQR(diagonal(t, 1e300))
	if err != nil {
		t.Fatal(err)
	}
	xv, norm, err := f.Solve([]float64{math.SmallestNonzeroFloat64})
	if err != nil || xv[0] != 0 || norm != math.SmallestNonzeroFloat64 {
		t.Fatal("solution underflow must retain actual residual", xv, norm, err)
	}
	if _, _, err := f.SolveWithOptions([]float64{math.SmallestNonzeroFloat64}, linalg.LeastSquaresOptions{CheckResidual: true}); !errors.Is(err, linalg.ErrResidual) {
		t.Fatal("nonzero underflow residual accepted", err)
	}
	// The RHS norm can exceed MaxFloat64 while the solution and residual
	// norms fit. Relative-residual checking must compare scaled norms.
	f, err = linalg.FactorizeQR(dense(t, 3, 1, 1, 0, 0))
	if err != nil {
		t.Fatal(err)
	}
	largeB := []float64{math.MaxFloat64, 1e308, 1e308}
	if _, norm, err := f.SolveWithOptions(largeB, linalg.LeastSquaresOptions{CheckResidual: true, RelativeResidualTolerance: .7}); err != nil || !finiteTest(norm) {
		t.Fatal("scaled relative-residual check", norm, err)
	}
	if _, _, err := f.SolveWithOptions(largeB, linalg.LeastSquaresOptions{CheckResidual: true, RelativeResidualTolerance: .6}); !errors.Is(err, linalg.ErrResidual) {
		t.Fatal("scaled relative-residual threshold", err)
	}
}

func TestQRScaledAndSubnormal(t *testing.T) {
	for _, scale := range []float64{1e-300, 1, 1e300} {
		a := dense(t, 3, 2, scale, 0, scale, scale, 0, 2*scale)
		x, norm, err := linalg.LeastSquares(a, []float64{scale, -scale, -4 * scale})
		if err != nil || math.Abs(x[0]-1) > 2e-14 || math.Abs(x[1]+2) > 2e-14 || norm/scale > 2e-14 {
			t.Fatal("uniform scale", scale, x, norm, err)
		}
	}
	s := math.SmallestNonzeroFloat64
	a := dense(t, 3, 2, s, 0, 0, 2*s, 0, 0)
	x, norm, err := linalg.LeastSquares(a, []float64{s, 2 * s, s})
	if err != nil || !slices.Equal(x, []float64{1, 1}) || norm != s {
		t.Fatal("subnormal QR", x, norm, err)
	}
	// A naive alpha-beta or unscaled sum of squares would overflow here.
	a = dense(t, 2, 1, 1e308, 1e307)
	f, err := linalg.FactorizeQR(a)
	if err != nil {
		t.Fatal(err)
	}
	x, norm, err = f.Solve([]float64{1e308, 1e307})
	if err != nil || math.Abs(x[0]-1) > 1e-15 || norm/1e308 > 2e-15 {
		t.Fatal("safe reflector construction", x, norm, err)
	}
	// Reflection of very large entries can be finite even when a naive
	// reflector dot product would overflow. The column scaling handles this.
	a = dense(t, 2, 2, 1e308, 1e308, 1e308, -1e308)
	f, err = linalg.FactorizeQR(a)
	if err != nil {
		t.Fatal(err)
	}
	x, norm, err = f.Solve([]float64{1e308, 0})
	if err != nil || math.Abs(x[0]-.5) > 1e-15 || math.Abs(x[1]-.5) > 1e-15 || norm/1e308 > 2e-15 {
		t.Fatal("safe reflector application", x, norm, err)
	}
}

func TestQREmpty(t *testing.T) {
	for _, m := range []int{0, 3} {
		a, _ := linalg.Zeros(m, 0)
		f, err := linalg.FactorizeQR(a)
		if err != nil {
			t.Fatal(err)
		}
		checkQRFactors(t, a, f)
		b := make([]float64, m)
		for i := range b {
			b[i] = float64(i + 1)
		}
		x, norm, err := f.Solve(b)
		want := 0.0
		for _, v := range b {
			want = math.Hypot(want, v)
		}
		if err != nil || len(x) != 0 || norm != want {
			t.Fatal("empty design", x, norm, err)
		}
		bm, _ := linalg.Zeros(m, 4)
		xm, norms, err := f.SolveMatrix(bm)
		if err != nil || len(norms) != 4 {
			t.Fatal(norms, err)
		}
		equalData(t, xm, 0, 4, nil)
	}
	a := rectangular(t, 4, 2)
	f, err := linalg.FactorizeQR(a)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := linalg.Zeros(4, 0)
	x, norms, err := f.SolveMatrix(b)
	if err != nil || len(norms) != 0 {
		t.Fatal(norms, err)
	}
	equalData(t, x, 2, 0, nil)
	// An empty shape must not bypass overflow checks for residual storage.
	empty, _ := linalg.Zeros(0, 0)
	f, _ = linalg.FactorizeQR(empty)
	hugeRHS, _ := linalg.Zeros(0, int(^uint(0)>>1))
	if _, _, err := f.SolveMatrix(hugeRHS); !errors.Is(err, linalg.ErrDimension) {
		t.Fatal("residual allocation overflow", err)
	}
}

func TestQRInvalidAndOverflow(t *testing.T) {
	if _, err := linalg.FactorizeQR(nil); !errors.Is(err, linalg.ErrInvalid) {
		t.Fatal(err)
	}
	wide := dense(t, 1, 2, 1, 2)
	if _, err := linalg.FactorizeQR(wide); !errors.Is(err, linalg.ErrDimension) {
		t.Fatal("underdetermined input accepted", err)
	}
	id, _ := linalg.Identity(2)
	for _, tol := range []float64{-1, 1, math.NaN(), math.Inf(1), math.Inf(-1)} {
		if _, err := linalg.FactorizeQRWithOptions(id, linalg.QROptions{RelativeRankTolerance: tol}); !errors.Is(err, linalg.ErrTolerance) {
			t.Fatal(err)
		}
	}
	f, _ := linalg.FactorizeQR(id)
	for _, tol := range []float64{-1, math.NaN(), math.Inf(1), math.Inf(-1)} {
		if _, _, err := f.SolveWithOptions([]float64{1, 2}, linalg.LeastSquaresOptions{RelativeResidualTolerance: tol}); !errors.Is(err, linalg.ErrTolerance) {
			t.Fatal(err)
		}
	}
	if _, _, err := f.Solve([]float64{1}); !errors.Is(err, linalg.ErrDimension) {
		t.Fatal(err)
	}
	if _, _, err := f.SolveMatrix(wide); !errors.Is(err, linalg.ErrDimension) {
		t.Fatal(err)
	}
	if _, _, err := f.SolveMatrix(nil); !errors.Is(err, linalg.ErrInvalid) {
		t.Fatal(err)
	}
	for _, bad := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if _, _, err := f.Solve([]float64{bad, 1}); !errors.Is(err, linalg.ErrNonFinite) {
			t.Fatal(err)
		}
	}
	for _, invalid := range []*linalg.QR{nil, {}} {
		if _, _, err := invalid.Solve(nil); !errors.Is(err, linalg.ErrInvalid) {
			t.Fatal(err)
		}
		if _, _, err := invalid.SolveMatrix(nil); !errors.Is(err, linalg.ErrInvalid) {
			t.Fatal(err)
		}
		if _, _, err := invalid.Factors(); !errors.Is(err, linalg.ErrInvalid) {
			t.Fatal(err)
		}
		if m, n := invalid.Dimensions(); m != 0 || n != 0 || invalid.Pivots() != nil {
			t.Fatal("invalid introspection")
		}
	}
	max := math.MaxFloat64
	if f, err := linalg.FactorizeQR(dense(t, 2, 1, max, max)); f != nil || !errors.Is(err, linalg.ErrNonFinite) {
		t.Fatal("unrepresentable R norm", f, err)
	}
	f, _ = linalg.FactorizeQR(dense(t, 2, 2, 1, 1, 1, -1))
	if x, norm, err := f.Solve([]float64{max, max}); x != nil || norm != 0 || !errors.Is(err, linalg.ErrNonFinite) {
		t.Fatal("reflector overflow", x, norm, err)
	}
	f, _ = linalg.FactorizeQR(diagonal(t, .5))
	if x, _, err := f.Solve([]float64{max}); x != nil || !errors.Is(err, linalg.ErrNonFinite) {
		t.Fatal("QR solve overflow", x, err)
	}
	empty, _ := linalg.Zeros(2, 0)
	f, _ = linalg.FactorizeQR(empty)
	if x, _, err := f.Solve([]float64{max, max}); x != nil || !errors.Is(err, linalg.ErrNonFinite) {
		t.Fatal("residual norm overflow", x, err)
	}
}
