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

// Test-side multiplication deliberately avoids the production Dense kernel.
func svdTestProduct(a []float64, m, k int, b []float64, n int) []float64 {
	out := make([]float64, m*n)
	for i := range m {
		for j := range n {
			for c := range k {
				out[i*n+j] += a[i*k+c] * b[c*n+j]
			}
		}
	}
	return out
}

func svdTestClose(t testing.TB, a, b []float64, tolerance float64) {
	t.Helper()
	if len(a) != len(b) {
		t.Fatal("array lengths", len(a), len(b))
	}
	var scale float64
	for _, x := range b {
		scale = math.Max(scale, math.Abs(x))
	}
	for i, x := range a {
		if !finiteTest(x) || math.Abs(x-b[i]) > tolerance*math.Max(1, scale) {
			t.Fatalf("entry %d: %.17g, want %.17g", i, x, b[i])
		}
	}
}

func checkSVDPenrose(t testing.TB, a, p *linalg.Dense) {
	t.Helper()
	m, n := a.Dimensions()
	pm, pn := p.Dimensions()
	if pm != n || pn != m {
		t.Fatal("pseudoinverse shape", pm, pn, m, n)
	}
	var (
		ad, pd = a.Data(), p.Data()
		ap     = svdTestProduct(ad, m, n, pd, m)
		pa     = svdTestProduct(pd, n, m, ad, n)
		apa    = svdTestProduct(ap, m, m, ad, n)
		pap    = svdTestProduct(pa, n, n, pd, m)
	)
	svdTestClose(t, apa, ad, 2e-11)
	svdTestClose(t, pap, pd, 2e-11)
	for _, tc := range []struct {
		x []float64
		n int
	}{{ap, m}, {pa, n}} {
		for i := range tc.n {
			for j := range tc.n {
				if math.Abs(tc.x[i*tc.n+j]-tc.x[j*tc.n+i]) > 2e-11 {
					t.Fatal("Penrose projector symmetry", i, j, tc.x)
				}
			}
		}
	}
}

func TestSVDEconomyShapesAndZeroCompletion(t *testing.T) {
	for _, shape := range [][2]int{{0, 0}, {0, 4}, {4, 0}, {1, 3}, {3, 1}, {3, 3}, {3, 7}, {7, 3}} {
		for _, zero := range []bool{false, true} {
			t.Run(fmt.Sprintf("%dx%d/zero=%v", shape[0], shape[1], zero), func(t *testing.T) {
				m, n := shape[0], shape[1]
				data := make([]float64, m*n)
				if !zero && min(m, n) != 0 {
					// An oblique nonzero column followed by exact zero columns
					// exercises completion against an existing non-axis basis.
					for i := range m {
						data[i*n] = float64(i + 1)
					}
				}
				a := dense(t, m, n, data...)
				f, err := linalg.FactorizeSVD(a)
				if err != nil {
					t.Fatal(err)
				}
				checkSVD(t, a, f)
				wantRank := 0
				if !zero && min(m, n) != 0 {
					wantRank = 1
				}
				if f.Rank() != wantRank {
					t.Fatal("rank", f.Rank(), wantRank)
				}
				p, err := f.PseudoInverse()
				if err != nil {
					t.Fatal(err)
				}
				checkSVDPenrose(t, a, p)
				bdata := make([]float64, m*2)
				for i := range bdata {
					bdata[i] = float64(i%5 - 2)
				}
				b := dense(t, m, 2, bdata...)
				x, norms, err := f.SolveMatrix(b)
				if err != nil || len(norms) != 2 {
					t.Fatal(norms, err)
				}
				xm, xn := x.Dimensions()
				if xm != n || xn != 2 {
					t.Fatal("solve shape", xm, xn)
				}
				svdTestClose(t, x.Data(), svdTestProduct(p.Data(), n, m, bdata, 2), 2e-13)
				for c, norm := range norms {
					if math.Abs(norm-actualResidual(a, x, b, c)) > 1e-12 {
						t.Fatal("actual residual", norm)
					}
				}
			})
		}
	}
}

func TestSVDEconomyAnalyticMinimumNorm(t *testing.T) {
	for _, tc := range []struct {
		m, n int
		a, p []float64
		b, x []float64
		null []float64
	}{
		{3, 2, []float64{1, 2, 2, 4, 3, 6}, []float64{1.0 / 70, 2.0 / 70, 3.0 / 70, 2.0 / 70, 4.0 / 70, 6.0 / 70},
			[]float64{1, 2, 2, 0, 3, 1}, []float64{.2, 1.0 / 14, .4, 1.0 / 7}, []float64{-2, 1}},
		{2, 3, []float64{1, 0, 1, 0, 1, 1}, []float64{2.0 / 3, -1.0 / 3, -1.0 / 3, 2.0 / 3, 1.0 / 3, 1.0 / 3},
			[]float64{1, 2, 2, -1}, []float64{0, 5.0 / 3, 1, -4.0 / 3, 1, 1.0 / 3}, []float64{-1, -1, 1}},
	} {
		a := dense(t, tc.m, tc.n, tc.a...)
		f, err := linalg.FactorizeSVD(a)
		if err != nil {
			t.Fatal(err)
		}
		checkSVD(t, a, f)
		p, err := f.PseudoInverse()
		if err != nil {
			t.Fatal(err)
		}
		svdTestClose(t, p.Data(), tc.p, 2e-14)
		checkSVDPenrose(t, a, p)
		b := dense(t, tc.m, 2, tc.b...)
		x, norms, err := f.SolveMatrix(b)
		if err != nil {
			t.Fatal(err)
		}
		svdTestClose(t, x.Data(), tc.x, 2e-14)
		for c := range 2 {
			column := make([]float64, tc.m)
			for i := range tc.m {
				column[i] = tc.b[i*2+c]
			}
			xv, norm, err := f.Solve(column)
			if err != nil || math.Abs(norm-norms[c]) > 1e-13 {
				t.Fatal(err, norm, norms)
			}
			var nullDot, xNorm, shiftedNorm float64
			for i, value := range xv {
				if math.Abs(value-tc.x[i*2+c]) > 2e-14 {
					t.Fatal("vector vs known minimum-norm solution", xv)
				}
				nullDot += value * tc.null[i]
				xNorm = math.Hypot(xNorm, value)
				shiftedNorm = math.Hypot(shiftedNorm, value+tc.null[i])
			}
			if math.Abs(nullDot) > 2e-14 || shiftedNorm <= xNorm {
				t.Fatal("solution is not perpendicular to null space", nullDot, xNorm, shiftedNorm)
			}
			// Normal-equation stationarity is checked only as an independent
			// test of the untruncated rank-deficient least-squares result.
			xd := x.Data()
			for j := range tc.n {
				var stationarity float64
				for i := range tc.m {
					r := -tc.b[i*2+c]
					for k := range tc.n {
						r += tc.a[i*tc.n+k] * xd[k*2+c]
					}
					stationarity += tc.a[i*tc.n+j] * r
				}
				if math.Abs(stationarity) > 2e-12 {
					t.Fatal("least-squares stationarity", stationarity)
				}
			}
		}
	}
}

func TestSVDEconomyRandomRankDeficiency(t *testing.T) {
	rng := rand.New(rand.NewPCG(1203, 6501))
	for _, shape := range [][2]int{{3, 3}, {8, 5}, {5, 8}, {17, 11}, {11, 17}, {32, 17}} {
		m, n := shape[0], shape[1]
		for _, rank := range []int{1, min(m, n) / 2, min(m, n)} {
			t.Run(fmt.Sprintf("%dx%d/rank%d", m, n, rank), func(t *testing.T) {
				left, right := make([]float64, m*rank), make([]float64, rank*n)
				for i := range left {
					left[i] = float64(rng.IntN(5) - 2)
				}
				for i := range right {
					right[i] = float64(rng.IntN(5) - 2)
				}
				// Independent integer full-rank minors prevent accidental rank
				// loss in the random fixture; every product entry is exact.
				for i := range rank {
					for j := range rank {
						left[i*rank+j], right[i*n+j] = 0, 0
					}
					left[i*rank+i], right[i*n+i] = 1, 1
				}
				data := svdTestProduct(left, m, rank, right, n)
				a := dense(t, m, n, data...)
				f, err := linalg.FactorizeSVD(a)
				if err != nil {
					t.Fatal(err)
				}
				if f.Rank() != rank {
					t.Fatal("known integer rank", f.Rank(), rank, f.Values())
				}
				checkSVD(t, a, f)
				p, err := f.PseudoInverse()
				if err != nil {
					t.Fatal(err)
				}
				checkSVDPenrose(t, a, p)
			})
		}
	}
}

func TestSVDTruncationIsIndependentOfDecomposition(t *testing.T) {
	for _, shape := range [][2]int{{3, 2}, {2, 3}} {
		m, n := shape[0], shape[1]
		data := make([]float64, m*n)
		data[0], data[n+1] = 1, 1e-6
		a := dense(t, m, n, data...)
		opt := linalg.DefaultSVDOptions()
		opt.RelativeRankTolerance = 1e-4
		f, err := linalg.FactorizeSVDWithOptions(a, opt)
		if err != nil || f.Rank() != 1 {
			t.Fatal(f, err)
		}
		checkSVD(t, a, f)
		if f.Values()[1] != 1e-6 {
			t.Fatal("rank cutoff truncated decomposition")
		}
		p, err := f.PseudoInverse()
		if err != nil {
			t.Fatal(err)
		}
		truncated := append([]float64(nil), data...)
		truncated[n+1] = 0
		checkSVDPenrose(t, dense(t, m, n, truncated...), p)
		// The discarded direction is a 1e-6 approximation error, not a
		// successful 1e-12 reconstruction of the original matrix.
		ap := svdTestProduct(data, m, n, p.Data(), m)
		apa := svdTestProduct(ap, m, m, data, n)
		if math.Abs(apa[n+1]-data[n+1]) != 1e-6 {
			t.Fatal("truncation error concealed", apa)
		}
		full, err := f.PseudoInverseWithTolerance(0)
		if err != nil {
			t.Fatal(err)
		}
		checkSVDPenrose(t, a, full)
		if rank, err := f.RankWithTolerance(0); err != nil || rank != 2 || f.Rank() != 1 {
			t.Fatal("explicit rank policy mutated default", rank, err)
		}
		b := make([]float64, m)
		b[1] = 1e-6
		x, residual, err := f.Solve(b)
		if err != nil || residual != 1e-6 || x[1] != 0 {
			t.Fatal("truncated solve must expose original residual", x, residual, err)
		}
		x, residual, err = f.SolveWithTolerance(b, 0)
		if err != nil || math.Abs(x[1]-1) > 1e-15 || residual > 1e-20 {
			t.Fatal("untruncated solve", x, residual, err)
		}
	}
}

func TestSVDReconstructionPolicyAndDeflation(t *testing.T) {
	a := dense(t, 2, 2, 1, 2, 3, 4)
	opt := linalg.DefaultSVDOptions()
	opt.RelativeReconstructionTolerance = 0
	f, err := linalg.FactorizeSVDWithOptions(a, opt)
	var re *linalg.ReconstructionError
	if f != nil || !errors.Is(err, linalg.ErrReconstruction) || !errors.As(err, &re) || re.Relative <= 0 || re.Tolerance != 0 || re.Error() == "" {
		t.Fatal("exact reconstruction policy", f, err)
	}
	f, err = linalg.FactorizeSVD(a)
	if err != nil || f.RelativeReconstructionError() > 1e-12 || f.DeflatedColumns() != 0 {
		t.Fatal("normal reconstruction policy", f, err)
	}
	// Compute relative Frobenius error independently of the implementation's
	// diagnostic, using ordinary scalar products on this moderate input.
	u, v, _ := f.Factors()
	ud, vd, s := u.Data(), v.Data(), f.Values()
	var norm, residual float64
	for i := range 2 {
		for j := range 2 {
			var value float64
			for c := range 2 {
				value += ud[i*2+c] * s[c] * vd[j*2+c]
			}
			original := a.Data()[i*2+j]
			norm = math.Hypot(norm, original)
			residual = math.Hypot(residual, original-value)
		}
	}
	if math.Abs(residual/norm-f.RelativeReconstructionError()) > 1e-15 {
		t.Fatal("incorrect reconstruction diagnostic", residual/norm, f.RelativeReconstructionError())
	}
	a = dense(t, 2, 2, 1e300, 1e-300, 0, 1e-300)
	f, err = linalg.FactorizeSVD(a)
	if err != nil || f.DeflatedColumns() != 1 || f.Values()[1] != 0 || f.RelativeReconstructionError() > 1e-12 {
		t.Fatal("permitted deflation must be disclosed", f, err)
	}
	opt = linalg.DefaultSVDOptions()
	opt.RelativeReconstructionTolerance = 0
	if f, err := linalg.FactorizeSVDWithOptions(a, opt); f != nil || !errors.Is(err, linalg.ErrRange) {
		t.Fatal("strict policy must forbid deflation", f, err)
	}
	for _, invalid := range []*linalg.SVD{nil, {}} {
		if invalid.DeflatedColumns() != 0 || invalid.RelativeReconstructionError() != 0 {
			t.Fatal("invalid reconstruction introspection")
		}
	}
}

func TestSVDSolveRangeAndScaling(t *testing.T) {
	for _, scale := range []float64{1e-300, 1, 1e300} {
		a := dense(t, 2, 3, scale, 0, scale, 0, scale, scale)
		f, err := linalg.FactorizeSVD(a)
		if err != nil {
			t.Fatal(err)
		}
		x, norm, err := f.Solve([]float64{scale, 2 * scale})
		if err != nil || norm/scale > 5e-14 {
			t.Fatal(scale, x, norm, err)
		}
		svdTestClose(t, x, []float64{0, 1, 1}, 2e-14)
	}
	f, err := linalg.FactorizeSVD(diagonal(t, 1e300, 1e-300))
	if err != nil {
		t.Fatal(err)
	}
	p, err := f.PseudoInverseWithTolerance(0)
	if err != nil || math.Abs(p.Data()[0]/1e-300-1) > 1e-14 || math.Abs(p.Data()[3]/1e300-1) > 1e-14 {
		t.Fatal("separated diagonal inverse", p, err)
	}
	x, norm, err := f.SolveWithTolerance([]float64{1e300, 1e-300}, 0)
	if err != nil || norm != 0 || !slices.Equal(x, []float64{1, 1}) {
		t.Fatal("separated RHS coordinates", x, norm, err)
	}
	// The reciprocal of sigma overflows, but each pseudoinverse entry fits.
	data := make([]float64, 100)
	for i := range data {
		data[i] = 1e-310
	}
	f, err = linalg.FactorizeSVD(dense(t, 100, 1, data...))
	if err != nil {
		t.Fatal(err)
	}
	p, err = f.PseudoInverse()
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range p.Data() {
		if !finiteTest(x) || math.Abs(x/1e308-1) > 1e-12 {
			t.Fatal("representable subnormal inverse entry", x)
		}
	}
	f, _ = linalg.FactorizeSVD(dense(t, 2, 1, 1, 1))
	x, norm, err = f.Solve([]float64{math.MaxFloat64, math.MaxFloat64})
	if err != nil || !finiteTest(norm) || math.Abs(x[0]/math.MaxFloat64-1) > 1e-14 {
		t.Fatal("overflow-safe projection", x, norm, err)
	}
	f, _ = linalg.FactorizeSVD(diagonal(t, math.SmallestNonzeroFloat64))
	if p, err := f.PseudoInverse(); p != nil || !errors.Is(err, linalg.ErrNonFinite) {
		t.Fatal("unrepresentable inverse", p, err)
	}
	if x, norm, err := f.Solve([]float64{1}); x != nil || norm != 0 || !errors.Is(err, linalg.ErrNonFinite) {
		t.Fatal("unrepresentable solution", x, norm, err)
	}
	f, _ = linalg.FactorizeSVD(dense(t, 2, 1, 0, 0))
	if x, norms, err := f.SolveMatrix(dense(t, 2, 1, math.MaxFloat64, math.MaxFloat64)); x != nil || norms != nil || !errors.Is(err, linalg.ErrNonFinite) {
		t.Fatal("unrepresentable original residual", x, norms, err)
	}
	f, _ = linalg.FactorizeSVD(diagonal(t, 1e300))
	x, norm, err = f.Solve([]float64{math.SmallestNonzeroFloat64})
	if err != nil || x[0] != 0 || norm != math.SmallestNonzeroFloat64 {
		t.Fatal("underflow retains actual residual", x, norm, err)
	}
}

func TestSVDEconomySolveValidation(t *testing.T) {
	f, _ := linalg.FactorizeSVD(diagonal(t, 1, 0))
	b := []float64{1, 1}
	for _, tol := range []float64{-1, 1, math.NaN(), math.Inf(1)} {
		if _, err := f.RankWithTolerance(tol); !errors.Is(err, linalg.ErrTolerance) {
			t.Fatal(err)
		}
		if p, err := f.PseudoInverseWithTolerance(tol); p != nil || !errors.Is(err, linalg.ErrTolerance) {
			t.Fatal(p, err)
		}
		if x, r, err := f.SolveWithTolerance(b, tol); x != nil || r != 0 || !errors.Is(err, linalg.ErrTolerance) {
			t.Fatal(x, r, err)
		}
	}
	for _, bad := range []*linalg.SVD{nil, {}} {
		if bad.Rank() != 0 {
			t.Fatal("invalid rank")
		}
		if _, err := bad.RankWithTolerance(0); !errors.Is(err, linalg.ErrInvalid) {
			t.Fatal(err)
		}
		if _, err := bad.PseudoInverse(); !errors.Is(err, linalg.ErrInvalid) {
			t.Fatal(err)
		}
		if _, err := bad.PseudoInverseWithTolerance(0); !errors.Is(err, linalg.ErrInvalid) {
			t.Fatal(err)
		}
		if _, _, err := bad.Solve(nil); !errors.Is(err, linalg.ErrInvalid) {
			t.Fatal(err)
		}
		if _, _, err := bad.SolveWithTolerance(nil, 0); !errors.Is(err, linalg.ErrInvalid) {
			t.Fatal(err)
		}
		if _, _, err := bad.SolveMatrix(nil); !errors.Is(err, linalg.ErrInvalid) {
			t.Fatal(err)
		}
		if _, _, err := bad.SolveMatrixWithTolerance(nil, 0); !errors.Is(err, linalg.ErrInvalid) {
			t.Fatal(err)
		}
	}
	if _, _, err := f.Solve([]float64{1}); !errors.Is(err, linalg.ErrDimension) {
		t.Fatal(err)
	}
	if _, _, err := f.Solve([]float64{math.NaN(), 1}); !errors.Is(err, linalg.ErrNonFinite) {
		t.Fatal(err)
	}
	if _, _, err := f.SolveMatrix(nil); !errors.Is(err, linalg.ErrInvalid) {
		t.Fatal(err)
	}
	if _, _, err := f.SolveMatrix(dense(t, 1, 1, 1)); !errors.Is(err, linalg.ErrDimension) {
		t.Fatal(err)
	}
	empty, _ := linalg.Zeros(2, 0)
	x, norms, err := f.SolveMatrix(empty)
	if err != nil || len(norms) != 0 {
		t.Fatal(norms, err)
	}
	equalData(t, x, 2, 0, nil)
	f, _ = linalg.FactorizeSVD(new(linalg.Dense))
	hugeRHS, _ := linalg.Zeros(0, int(^uint(0)>>1))
	if x, r, err := f.SolveMatrix(hugeRHS); x != nil || r != nil || !errors.Is(err, linalg.ErrDimension) {
		t.Fatal("residual storage overflow", x, r, err)
	}
}

func TestSVDEconomyConcurrentInverseAndSolve(t *testing.T) {
	a := dense(t, 2, 3, 1, 0, 1, 0, 1, 1)
	f, err := linalg.FactorizeSVD(a)
	if err != nil {
		t.Fatal(err)
	}
	b := dense(t, 2, 2, 1, 2, 2, -1)
	original, rhs := a.Data(), b.Data()
	var wg sync.WaitGroup
	failures := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 4 {
				p, err := f.PseudoInverse()
				if err != nil {
					failures <- err
					return
				}
				x, norms, err := f.SolveMatrix(b)
				if err != nil {
					failures <- err
					return
				}
				pd, xd := p.Data(), x.Data()
				pd[0], xd[0], norms[0] = math.NaN(), math.NaN(), math.NaN()
				if f.Rank() != 2 {
					failures <- fmt.Errorf("rank mutated")
					return
				}
			}
		}()
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	if !slices.Equal(a.Data(), original) || !slices.Equal(b.Data(), rhs) {
		t.Fatal("concurrent solve mutated inputs")
	}
}
