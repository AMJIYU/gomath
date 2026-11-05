package linalg_test

import (
	"encoding/json"
	"errors"
	"math"
	"math/cmplx"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"testing"

	"gomath/linalg"
)

func checkSchur(t testing.TB, a *linalg.Dense, f *linalg.Schur) {
	t.Helper()
	n, _ := a.Dimensions()
	if f.Size() != n {
		t.Fatal("Schur dimension")
	}
	q, s, err := f.Factors()
	if err != nil {
		t.Fatal(err)
	}
	qd, sd, ad := q.Data(), s.Data(), a.Data()
	checkSpectralOrthogonal(t, qd, n, n)
	for i := range n {
		for j := 0; j < i-1; j++ {
			if sd[i*n+j] != 0 {
				t.Fatalf("not upper Hessenberg at %d,%d: %g", i, j, sd[i*n+j])
			}
		}
	}
	for i := 1; i < n-1; i++ {
		if sd[i*n+i-1] != 0 && sd[(i+1)*n+i] != 0 {
			t.Fatal("overlapping Schur blocks")
		}
	}
	scale := 0.0
	for _, v := range ad {
		scale = math.Max(scale, math.Abs(v))
	}
	if scale == 0 {
		scale = 1
	}
	product := make([]float64, n*n)
	for i := range n {
		for j := range n {
			for k := range n {
				product[i*n+j] += qd[i*n+k] * (sd[k*n+j] / scale)
			}
		}
	}
	residual, norm := 0.0, 0.0
	for i := range n {
		for j := range n {
			value := 0.0
			for k := range n {
				value += product[i*n+k] * qd[j*n+k]
			}
			residual = math.Hypot(residual, value-ad[i*n+j]/scale)
			norm = math.Hypot(norm, ad[i*n+j]/scale)
		}
	}
	if norm == 0 {
		norm = 1
	}
	if residual/norm > 8e-13 || f.RelativeReconstructionError() > 1e-12 || f.OrthogonalityError() > 1e-12 {
		t.Fatalf("Schur residual %.4g diagnostic %.4g orthogonality %.4g", residual/norm, f.RelativeReconstructionError(), f.OrthogonalityError())
	}
	values := f.Values()
	if len(values) != n {
		t.Fatal("Schur eigenvalue count")
	}
	for i := 0; i < n; i++ {
		if !finiteTest(real(values[i])) || !finiteTest(imag(values[i])) {
			t.Fatal("nonfinite value")
		}
		if i+1 < n && sd[(i+1)*n+i] != 0 {
			if imag(values[i]) <= 0 || values[i+1] != cmplx.Conj(values[i]) {
				t.Fatal("pair order/conjugacy", values)
			}
			i++
		} else if imag(values[i]) != 0 {
			t.Fatal("imaginary value at real block")
		}
	}
}

func TestSchurSciPyReference(t *testing.T) {
	// The overlay retains source filenames: locate the module independently
	// of the process cwd; no shared TestMain or production testdata is needed.
	_, source, _, _ := runtime.Caller(0)
	var data []byte
	var err error
	for dir := filepath.Dir(source); ; dir = filepath.Dir(dir) {
		data, err = os.ReadFile(filepath.Join(dir, "tests", "data", "linalg", "schur_scipy.json"))
		if err == nil {
			break
		}
		if filepath.Dir(dir) == dir {
			t.Fatal(err)
		}
	}
	var reference struct {
		Cases []struct {
			Name                  string
			N                     int
			Data                  []float64
			Values                [][2]float64
			Scale, EigenTolerance float64
		}
	}
	if err := json.Unmarshal(data, &reference); err != nil || len(reference.Cases) < 40 {
		t.Fatal("reference", err)
	}
	worstReconstruction, worstOrthogonality := 0.0, 0.0
	for _, c := range reference.Cases {
		t.Run(c.Name, func(t *testing.T) {
			a, err := linalg.NewDense(c.N, c.N, c.Data)
			if err != nil {
				t.Fatal(err)
			}
			f, err := linalg.FactorizeSchur(a)
			if err != nil {
				t.Fatal(err)
			}
			checkSchur(t, a, f)
			worstReconstruction = math.Max(worstReconstruction, f.RelativeReconstructionError())
			worstOrthogonality = math.Max(worstOrthogonality, f.OrthogonalityError())
			values := f.Values()
			eigenScale := 0.0
			for _, value := range c.Values {
				eigenScale = math.Max(eigenScale, math.Hypot(value[0], value[1]))
			}
			if eigenScale == 0 {
				eigenScale = 1
			}
			// Bipartite matching avoids imposing ordering on repeated/nearby roots.
			assigned := make([]int, len(values))
			for i := range assigned {
				assigned[i] = -1
			}
			var match func(int, []bool) bool
			match = func(i int, seen []bool) bool {
				for j, expected := range c.Values {
					if seen[j] || cmplx.Abs(values[i]/complex(c.Scale, 0)-complex(expected[0], expected[1])) > c.EigenTolerance*eigenScale {
						continue
					}
					seen[j] = true
					if assigned[j] == -1 || match(assigned[j], seen) {
						assigned[j] = i
						return true
					}
				}
				return false
			}
			for i := range values {
				if !match(i, make([]bool, len(values))) {
					t.Fatalf("eigenvalue mismatch %v vs %v, scale %g", values, c.Values, c.Scale)
				}
			}
			if !slices.Equal(a.Data(), c.Data) {
				t.Fatal("mutated input")
			}
		})
	}
	t.Logf("%d SciPy cases: max relative reconstruction %.6g, max orthogonality Frobenius %.6g", len(reference.Cases), worstReconstruction, worstOrthogonality)
}

func TestSchurAnalyticExtremePairs(t *testing.T) {
	for _, scale := range []float64{math.SmallestNonzeroFloat64, 1e-300, 1, 1e300} {
		a := dense(t, 2, 2, 0, -scale, scale, 0)
		f, err := linalg.FactorizeSchur(a)
		if err != nil {
			t.Fatal("scaled rotation", scale, err)
		}
		checkSchur(t, a, f)
		values := f.Values()
		if values[0] != complex(0, scale) || values[1] != complex(0, -scale) {
			t.Fatal("exact conjugate pair", scale, values)
		}
	}
	for _, c := range []struct {
		data       []float64
		real, imag float64
	}{
		{[]float64{0, 1e100, -1e-100, 0}, 0, 1},
		{[]float64{1, 1e-300, -1e-300, 1}, 1, 1e-300},
		{[]float64{1, 1, -0x1p-52, 1}, 1, 0x1p-26},
		{[]float64{1, 1, -math.Nextafter(1, 2), -1}, 0, 0x1p-26},
	} {
		a := dense(t, 2, 2, c.data...)
		f, err := linalg.FactorizeSchur(a)
		if err != nil {
			t.Fatal(c.data, err)
		}
		checkSchur(t, a, f)
		v := f.Values()[0]
		if math.Abs(real(v)-c.real) > 1e-14 || math.Abs(imag(v)/c.imag-1) > 3e-15 {
			t.Fatal("analytic complex roots", c.data, v, c.real, c.imag)
		}
	}
}

func TestSchurExactDefectiveLower(t *testing.T) {
	for _, center := range []float64{0, 1, -2} {
		const n = 17
		data := make([]float64, n*n)
		for i := range n {
			data[i*n+i] = center
			if i > 0 {
				data[i*n+i-1] = 1
			}
		}
		a := dense(t, n, n, data...)
		f, err := linalg.FactorizeSchur(a)
		if err != nil {
			t.Fatal(center, err)
		}
		checkSchur(t, a, f)
		for _, value := range f.Values() {
			if value != complex(center, 0) {
				t.Fatal("strict lower Jordan acquired spurious root", center, value)
			}
		}
	}
}

func TestSchurBudgetsAndValidation(t *testing.T) {
	a := dense(t, 3, 3, 1, 2, 3, 4, 5, 6, 7, 8, 10)
	opt := linalg.DefaultSchurOptions()
	opt.MaxIterations = 0
	f, err := linalg.FactorizeSchurWithOptions(a, opt)
	var convergence *linalg.SchurConvergenceError
	if f != nil || !errors.Is(err, linalg.ErrConvergence) || !errors.As(err, &convergence) || convergence.Error() == "" {
		t.Fatal("convergence budget", f, err)
	}
	for _, opt := range []linalg.SchurOptions{
		{MaxIterations: -1}, {MaxSize: -1}, {MaxSize: 1025},
	} {
		if f, err := linalg.FactorizeSchurWithOptions(a, opt); f != nil || !errors.Is(err, linalg.ErrBudget) {
			t.Fatal("invalid budget", f, err)
		}
	}
	for _, tol := range []float64{-1, 1, math.NaN(), math.Inf(1)} {
		for _, opt := range []linalg.SchurOptions{{RelativeTolerance: tol}, {ReconstructionTolerance: tol}, {OrthogonalityTolerance: tol}} {
			if f, err := linalg.FactorizeSchurWithOptions(a, opt); f != nil || !errors.Is(err, linalg.ErrTolerance) {
				t.Fatal("invalid tolerance", f, err)
			}
		}
	}
	for _, opt := range []linalg.SchurOptions{{MaxWork: 1}, {MaxSize: 2}} {
		if f, err := linalg.FactorizeSchurWithOptions(a, opt); f != nil || !errors.Is(err, linalg.ErrSchurLimit) {
			t.Fatal("resource limit", f, err)
		}
	}
	if f, err := linalg.FactorizeSchur(nil); f != nil || !errors.Is(err, linalg.ErrInvalid) {
		t.Fatal("nil matrix", f, err)
	}
	if f, err := linalg.FactorizeSchur(dense(t, 2, 1, 1, 2)); f != nil || !errors.Is(err, linalg.ErrDimension) {
		t.Fatal("nonsquare matrix", f, err)
	}
	for _, data := range [][]float64{
		{math.MaxFloat64, 0, 0, math.SmallestNonzeroFloat64},
		{math.MaxFloat64, math.MaxFloat64, math.MaxFloat64, math.MaxFloat64},
	} {
		if f, err := linalg.FactorizeSchur(dense(t, 2, 2, data...)); f != nil || !errors.Is(err, linalg.ErrRange) {
			t.Fatal("numeric range", f, err)
		}
	}
	// A subnormal real pair can have representable nonzero rounded diagonals
	// but an inaccurate returned T. Validate the rounded output, not just an
	// accurate internal factor computed at a larger scale.
	sub := math.SmallestNonzeroFloat64
	if f, err := linalg.FactorizeSchur(dense(t, 2, 2, sub, sub, sub, 0)); f != nil || !(errors.Is(err, linalg.ErrRange) || errors.Is(err, linalg.ErrReconstruction)) {
		t.Fatal("subnormal output rounding escaped validation", f, err)
	}
	for _, invalid := range []*linalg.Schur{nil, {}} {
		if q, s, err := invalid.Factors(); q != nil || s != nil || !errors.Is(err, linalg.ErrInvalid) {
			t.Fatal("uninitialized factors", err)
		}
		if invalid.Size() != 0 || invalid.Iterations() != 0 || invalid.Values() != nil || invalid.RelativeReconstructionError() != 0 || invalid.OrthogonalityError() != 0 {
			t.Fatal("invalid introspection")
		}
	}
}

func TestSchurOwnershipAndConcurrency(t *testing.T) {
	a := dense(t, 4, 4, 1, 2, 3, 4, 5, 6, 7, 8, -2, 1, 0, 4, 1, -3, 5, 2)
	f, err := linalg.FactorizeSchur(a)
	if err != nil {
		t.Fatal(err)
	}
	want := f.Values()
	var wg sync.WaitGroup
	for range 12 {
		wg.Go(func() {
			for range 10 {
				g, err := linalg.FactorizeSchur(a)
				if err != nil || !slices.Equal(g.Values(), want) {
					t.Errorf("concurrent factorization %v", err)
					return
				}
				q, s, err := f.Factors()
				if err != nil {
					t.Error(err)
					return
				}
				qd, sd, values := q.Data(), s.Data(), f.Values()
				qd[0], sd[0], values[0] = 99, 99, 99
				if !slices.Equal(f.Values(), want) {
					t.Error("export aliases factor")
				}
			}
		})
	}
	wg.Wait()
	checkSchur(t, a, f)
}
