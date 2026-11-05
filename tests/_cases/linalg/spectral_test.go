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

func checkSpectralOrthogonal(t testing.TB, data []float64, rows, cols int) {
	t.Helper()
	for i := range cols {
		for j := range cols {
			var dot float64
			for k := range rows {
				dot += data[k*cols+i] * data[k*cols+j]
			}
			want := 0.0
			if i == j {
				want = 1
			}
			if !finiteTest(dot) || math.Abs(dot-want) > 6e-13 {
				t.Fatalf("orthogonality [%d,%d]: %.17g", i, j, dot)
			}
		}
	}
}

func checkEigenSym(t testing.TB, a *linalg.Dense, f *linalg.EigenSym) {
	t.Helper()
	n, _ := a.Dimensions()
	if f.Size() != n {
		t.Fatal("eigen size", f.Size(), n)
	}
	v, err := f.Vectors()
	if err != nil {
		t.Fatal(err)
	}
	vm, vn := v.Dimensions()
	if vm != n || vn != n {
		t.Fatal("eigenvector dimensions", vm, vn)
	}
	var (
		values = f.Values()
		vd     = v.Data()
		ad     = a.Data()
		scale  float64
	)
	if len(values) != n {
		t.Fatal("eigenvalue count")
	}
	for i, value := range values {
		if !finiteTest(value) || i > 0 && value > values[i-1] {
			t.Fatal("eigenvalue order", values)
		}
	}
	for _, x := range ad {
		scale = math.Max(scale, math.Abs(x))
	}
	checkSpectralOrthogonal(t, vd, n, n)
	if scale == 0 {
		scale = 1
	}
	// Independent scalar reconstruction and A*v=lambda*v, scaled first to
	// avoid test-side overflow. No production multiplication kernel is used.
	for i := range n {
		for j := range n {
			var reconstructed, av float64
			for k := range n {
				reconstructed += vd[i*n+k] * (values[k] / scale) * vd[j*n+k]
				av += (ad[i*n+k] / scale) * vd[k*n+j]
			}
			if math.Abs(reconstructed-ad[i*n+j]/scale) > 4e-12 || math.Abs(av-(values[j]/scale)*vd[i*n+j]) > 4e-12 {
				t.Fatalf("eigen reconstruction/equation [%d,%d]: %g / %g", i, j, reconstructed-ad[i*n+j]/scale, av-(values[j]/scale)*vd[i*n+j])
			}
		}
	}
}

func checkSVD(t testing.TB, a *linalg.Dense, f *linalg.SVD) {
	t.Helper()
	m, n := a.Dimensions()
	k := min(m, n)
	fm, fn := f.Dimensions()
	if fm != m || fn != n {
		t.Fatal("SVD dimensions", fm, fn)
	}
	u, v, err := f.Factors()
	if err != nil {
		t.Fatal(err)
	}
	um, un := u.Dimensions()
	vm, vn := v.Dimensions()
	if um != m || un != k || vm != n || vn != k {
		t.Fatal("SVD factor shapes", um, un, vm, vn)
	}
	var (
		s      = f.Values()
		ud, vd = u.Data(), v.Data()
		ad     = a.Data()
		scale  float64
	)
	if len(s) != k {
		t.Fatal("singular value count")
	}
	for i, value := range s {
		if !finiteTest(value) || value < 0 || i > 0 && value > s[i-1] {
			t.Fatal("nonnegative descending singular values", s)
		}
	}
	checkSpectralOrthogonal(t, ud, m, k)
	checkSpectralOrthogonal(t, vd, n, k)
	for _, x := range ad {
		scale = math.Max(scale, math.Abs(x))
	}
	if scale == 0 {
		scale = 1
	}
	for i := range m {
		for j := range n {
			var reconstructed float64
			for c := range k {
				reconstructed += ud[i*k+c] * (s[c] / scale) * vd[j*k+c]
			}
			if !finiteTest(reconstructed) || math.Abs(reconstructed-ad[i*n+j]/scale) > 4e-12 {
				t.Fatalf("SVD reconstruction [%d,%d]: %g vs %g", i, j, reconstructed, ad[i*n+j]/scale)
			}
		}
	}
}

func TestEigenSymAnalytic(t *testing.T) {
	for _, tc := range []struct {
		n      int
		data   []float64
		values []float64
	}{
		{1, []float64{-7}, []float64{-7}},
		{2, []float64{2, 1, 1, 2}, []float64{3, 1}},
		{2, []float64{0, -2, -2, 0}, []float64{2, -2}},
		{3, []float64{2, 1, 1, 1, 2, 1, 1, 1, 2}, []float64{4, 1, 1}},
		{3, make([]float64, 9), []float64{0, 0, 0}},
	} {
		a := dense(t, tc.n, tc.n, tc.data...)
		f, err := linalg.FactorizeEigenSym(a)
		if err != nil {
			t.Fatal(err)
		}
		checkEigenSym(t, a, f)
		for i, got := range f.Values() {
			if math.Abs(got-tc.values[i]) > 2e-14 {
				t.Fatal("analytic eigenvalues", f.Values(), tc.values)
			}
		}
	}
	// Toeplitz tridiagonal has a closed-form spectrum independent of Jacobi.
	for _, n := range []int{3, 9, 25} {
		data := make([]float64, n*n)
		for i := range n {
			data[i*n+i] = 2
			if i+1 < n {
				data[i*n+i+1], data[(i+1)*n+i] = -1, -1
			}
		}
		a := dense(t, n, n, data...)
		f, err := linalg.FactorizeEigenSym(a)
		if err != nil {
			t.Fatal(err)
		}
		checkEigenSym(t, a, f)
		for i, got := range f.Values() {
			want := 2 - 2*math.Cos(float64(n-i)*math.Pi/float64(n+1))
			if math.Abs(got-want) > 5e-14 {
				t.Fatal("Toeplitz spectrum", n, i, got, want)
			}
		}
	}
}

func TestSVDAnalyticAndIllConditioned(t *testing.T) {
	for _, tc := range []struct {
		m, n   int
		data   []float64
		values []float64
	}{
		{1, 1, []float64{-7}, []float64{7}},
		{3, 1, []float64{3, 4, 0}, []float64{5}},
		{3, 2, []float64{1.8, 2.4, 2.4, 3.2, -1.6, 1.2}, []float64{5, 2}},
		{3, 3, []float64{0, 0, -2, 3, 0, 0, 0, -3, 0}, []float64{3, 3, 2}},
	} {
		a := dense(t, tc.m, tc.n, tc.data...)
		f, err := linalg.FactorizeSVD(a)
		if err != nil {
			t.Fatal(err)
		}
		checkSVD(t, a, f)
		for i, got := range f.Values() {
			if math.Abs(got-tc.values[i]) > 1e-14 {
				t.Fatal("analytic singular values", f.Values(), tc.values)
			}
		}
	}
	// An A^T*A eigensolver loses eps^2 here, even though A retains eps.
	// Check the small value with the analytic determinant/product identity.
	for _, epsilon := range []float64{1e-8, 1e-12, 1e-18} {
		a := dense(t, 3, 2, 1, 1, 0, epsilon, 0, 0)
		opt := linalg.DefaultSVDOptions()
		opt.RelativeRankTolerance = 0
		f, err := linalg.FactorizeSVDWithOptions(a, opt)
		if err != nil {
			t.Fatal(epsilon, err)
		}
		checkSVD(t, a, f)
		s := f.Values()
		if math.Abs(s[0]-math.Sqrt2) > 1e-14 || math.Abs(s[1]/(epsilon/math.Sqrt2)-1) > 2e-14 {
			t.Fatalf("near dependent epsilon %g: %v", epsilon, s)
		}
	}
}

func TestSpectralRandomReconstruction(t *testing.T) {
	rng := rand.New(rand.NewPCG(2047, 991))
	for _, n := range []int{2, 3, 8, 17, 32} {
		for trial := range 4 {
			t.Run(fmt.Sprintf("n%d_trial%d", n, trial), func(t *testing.T) {
				data := make([]float64, n*n)
				for i := range n {
					for j := 0; j <= i; j++ {
						x := rng.NormFloat64()
						data[i*n+j], data[j*n+i] = x, x
					}
				}
				a := dense(t, n, n, data...)
				f, err := linalg.FactorizeEigenSym(a)
				if err != nil {
					t.Fatal(err)
				}
				checkEigenSym(t, a, f)
				m := n + trial
				data = make([]float64, m*n)
				for i := range data {
					data[i] = rng.NormFloat64()
				}
				a = dense(t, m, n, data...)
				svd, err := linalg.FactorizeSVD(a)
				if err != nil {
					t.Fatal(err)
				}
				checkSVD(t, a, svd)
			})
		}
	}
}

func TestSpectralScalingAndRange(t *testing.T) {
	for _, scale := range []float64{1e-300, 1, 1e300} {
		a := dense(t, 2, 2, 2*scale, scale, scale, 2*scale)
		f, err := linalg.FactorizeEigenSym(a)
		if err != nil {
			t.Fatal(scale, err)
		}
		checkEigenSym(t, a, f)
		values := f.Values()
		if math.Abs(values[0]/scale-3) > 1e-14 || math.Abs(values[1]/scale-1) > 1e-14 {
			t.Fatal("scaled eigenvalues", scale, values)
		}
		a = dense(t, 3, 2, 3*scale, 0, 0, 4*scale, 0, 0)
		svd, err := linalg.FactorizeSVD(a)
		if err != nil {
			t.Fatal(scale, err)
		}
		checkSVD(t, a, svd)
		values = svd.Values()
		if math.Abs(values[0]/scale-4) > 1e-14 || math.Abs(values[1]/scale-3) > 1e-14 {
			t.Fatal("scaled singular values", scale, values)
		}
		// Nonorthogonal columns exercise scaled correlations and rotations.
		a = dense(t, 3, 2, scale, scale, scale, 2*scale, 0, scale)
		svd, err = linalg.FactorizeSVD(a)
		if err != nil {
			t.Fatal(scale, err)
		}
		checkSVD(t, a, svd)
	}
	small := math.SmallestNonzeroFloat64
	a := diagonal(t, 2*small, small)
	e, err := linalg.FactorizeEigenSym(a)
	if err != nil || !slices.Equal(e.Values(), []float64{2 * small, small}) {
		t.Fatal("subnormal eigenvalues", e, err)
	}
	s, err := linalg.FactorizeSVD(a)
	if err != nil || !slices.Equal(s.Values(), []float64{2 * small, small}) {
		t.Fatal("subnormal singular values", s, err)
	}
	checkSVD(t, a, s)
	a = diagonal(t, 1e300, 1e-300)
	opt := linalg.DefaultSVDOptions()
	opt.RelativeRankTolerance = 0
	s, err = linalg.FactorizeSVDWithOptions(a, opt)
	if err != nil || !slices.Equal(s.Values(), []float64{1e300, 1e-300}) {
		t.Fatal("disparate orthogonal columns", s, err)
	}
	a = dense(t, 2, 2, 1e300, 1e-300, 0, 1e-300)
	opt.RelativeReconstructionTolerance = 0
	if f, err := linalg.FactorizeSVDWithOptions(a, opt); f != nil || !errors.Is(err, linalg.ErrRange) {
		t.Fatal("unrepresentable SVD angle", f, err)
	}
	eopt := linalg.DefaultEigenSymOptions()
	eopt.RelativeTolerance = 0
	a = dense(t, 2, 2, 1e300, 1e-300, 1e-300, 0)
	if f, err := linalg.FactorizeEigenSymWithOptions(a, eopt); f != nil || !errors.Is(err, linalg.ErrRange) {
		t.Fatal("unrepresentable eigen angle", f, err)
	}
	max := math.MaxFloat64
	if f, err := linalg.FactorizeEigenSym(dense(t, 2, 2, max, max, max, max)); f != nil || !errors.Is(err, linalg.ErrNonFinite) {
		t.Fatal("unrepresentable eigenvalue", f, err)
	}
	if f, err := linalg.FactorizeSVD(dense(t, 2, 1, max, max)); f != nil || !errors.Is(err, linalg.ErrNonFinite) {
		t.Fatal("unrepresentable singular value", f, err)
	}
}

func TestSpectralKnownRotatedSpectrum(t *testing.T) {
	// Discrete sine transform bases have an analytic orthogonality identity;
	// no production QR/eigensolver is used to construct these input spectra.
	sine := func(size, i, j int) float64 {
		return math.Sqrt(2/float64(size+1)) * math.Sin(float64((i+1)*(j+1))*math.Pi/float64(size+1))
	}
	for _, condition := range []float64{1, 1e4, 1e12} {
		const m, n = 11, 7
		values := make([]float64, n)
		for i := range n {
			values[i] = math.Pow(condition, -float64(i)/float64(n-1))
		}
		data := make([]float64, m*n)
		for i := range m {
			for j := range n {
				for k := range n {
					data[i*n+j] += sine(m, i, k) * values[k] * sine(n, j, k)
				}
			}
		}
		a := dense(t, m, n, data...)
		f, err := linalg.FactorizeSVD(a)
		if err != nil {
			t.Fatal(condition, err)
		}
		checkSVD(t, a, f)
		for i, got := range f.Values() {
			// Absolute error is appropriate for a rounded, rotated input.
			// Relative accuracy near 1e-12 is deliberately not overclaimed.
			if math.Abs(got-values[i]) > 5e-15 {
				t.Fatal("rotated known singular spectrum", condition, i, got, values[i])
			}
		}
		data = make([]float64, n*n)
		for i := range n {
			for j := 0; j <= i; j++ {
				var x float64
				for k := range n {
					sign := 1.0
					if k%2 != 0 {
						sign = -1
					}
					x += sine(n, i, k) * sign * values[k] * sine(n, j, k)
				}
				data[i*n+j], data[j*n+i] = x, x
			}
		}
		a = dense(t, n, n, data...)
		e, err := linalg.FactorizeEigenSym(a)
		if err != nil {
			t.Fatal(condition, err)
		}
		checkEigenSym(t, a, e)
		want := append([]float64(nil), values...)
		for i := range want {
			if i%2 != 0 {
				want[i] = -want[i]
			}
		}
		slices.Sort(want)
		slices.Reverse(want)
		for i, got := range e.Values() {
			if math.Abs(got-want[i]) > 3e-14 {
				t.Fatal("rotated known eigen spectrum", condition, i, got, want[i])
			}
		}
	}
}

func TestSpectralSymmetryAndRankPolicy(t *testing.T) {
	a := dense(t, 2, 2, 2, 1, 1+1e-8, 2)
	if f, err := linalg.FactorizeEigenSym(a); f != nil || !errors.Is(err, linalg.ErrNotSymmetric) {
		t.Fatal("asymmetry accepted", f, err)
	}
	opt := linalg.DefaultEigenSymOptions()
	opt.RelativeSymmetryTolerance = 1e-7
	f, err := linalg.FactorizeEigenSymWithOptions(a, opt)
	if err != nil {
		t.Fatal(err)
	}
	checkEigenSym(t, dense(t, 2, 2, 2, 1+5e-9, 1+5e-9, 2), f)
	for _, a := range []*linalg.Dense{
		dense(t, 3, 1, 0, 0, 0),
		dense(t, 3, 2, 1, 1, 0, 0, 0, 0),
		dense(t, 3, 2, 1, 2, 2, 4, 3, 6),
		diagonal(t, 1, 1e-16),
	} {
		f, err := linalg.FactorizeSVD(a)
		if err != nil || f.Rank() >= len(f.Values()) {
			t.Fatal("SVD rank policy", f, err)
		}
		checkSVD(t, a, f)
	}
	for _, x := range []float64{.5, math.Nextafter(.5, 1)} {
		opt := linalg.DefaultSVDOptions()
		opt.RelativeRankTolerance = .5
		f, err := linalg.FactorizeSVDWithOptions(diagonal(t, 1, x), opt)
		if err != nil || x == .5 && f.Rank() != 1 || x > .5 && f.Rank() != 2 {
			t.Fatal("SVD rank boundary", x, f, err)
		}
	}
}

func TestSpectralBudgetAndValidation(t *testing.T) {
	a := dense(t, 3, 3, 3, 1, .2, 1, 2, .7, .2, .7, 1)
	for _, budget := range []int{0, 1} {
		eopt := linalg.DefaultEigenSymOptions()
		eopt.MaxSweeps = budget
		f, err := linalg.FactorizeEigenSymWithOptions(a, eopt)
		var ce *linalg.ConvergenceError
		if f != nil || !errors.Is(err, linalg.ErrConvergence) || !errors.As(err, &ce) || ce.Sweeps != budget || ce.MaxSweeps != budget || ce.Residual <= ce.Tolerance || ce.Error() == "" {
			t.Fatal("eigen budget", f, err)
		}
		sopt := linalg.DefaultSVDOptions()
		sopt.MaxSweeps = budget
		s, err := linalg.FactorizeSVDWithOptions(a, sopt)
		if s != nil || !errors.As(err, &ce) || ce.Sweeps != budget {
			t.Fatal("SVD budget", s, err)
		}
	}
	// Already diagonal/orthogonal inputs do not consume a sweep.
	id, _ := linalg.Identity(3)
	if _, err := linalg.FactorizeEigenSymWithOptions(id, linalg.EigenSymOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := linalg.FactorizeSVDWithOptions(id, linalg.SVDOptions{}); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []float64{-1, 1, math.NaN(), math.Inf(1), math.Inf(-1)} {
		for _, opt := range []linalg.EigenSymOptions{{RelativeTolerance: invalid}, {RelativeSymmetryTolerance: invalid}} {
			if f, err := linalg.FactorizeEigenSymWithOptions(a, opt); f != nil || !errors.Is(err, linalg.ErrTolerance) {
				t.Fatal("eigen tolerance", f, err)
			}
		}
		for _, opt := range []linalg.SVDOptions{{RelativeTolerance: invalid}, {RelativeRankTolerance: invalid}, {RelativeReconstructionTolerance: invalid}} {
			if f, err := linalg.FactorizeSVDWithOptions(a, opt); f != nil || !errors.Is(err, linalg.ErrTolerance) {
				t.Fatal("SVD tolerance", f, err)
			}
		}
	}
	if _, err := linalg.FactorizeEigenSymWithOptions(a, linalg.EigenSymOptions{MaxSweeps: -1}); !errors.Is(err, linalg.ErrBudget) {
		t.Fatal(err)
	}
	if _, err := linalg.FactorizeSVDWithOptions(a, linalg.SVDOptions{MaxSweeps: -1}); !errors.Is(err, linalg.ErrBudget) {
		t.Fatal(err)
	}
	if _, err := linalg.FactorizeEigenSym(nil); !errors.Is(err, linalg.ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := linalg.FactorizeSVD(nil); !errors.Is(err, linalg.ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := linalg.FactorizeEigenSym(dense(t, 2, 1, 1, 2)); !errors.Is(err, linalg.ErrDimension) {
		t.Fatal(err)
	}
	if _, err := linalg.FactorizeSVD(dense(t, 1, 2, 1, 2)); err != nil {
		t.Fatal(err)
	}
	for _, f := range []*linalg.EigenSym{nil, {}} {
		if f.Size() != 0 || f.Values() != nil {
			t.Fatal("invalid eigen introspection")
		}
		if _, err := f.Vectors(); !errors.Is(err, linalg.ErrInvalid) {
			t.Fatal(err)
		}
	}
	for _, f := range []*linalg.SVD{nil, {}} {
		m, n := f.Dimensions()
		if m != 0 || n != 0 || f.Values() != nil {
			t.Fatal("invalid SVD introspection")
		}
		if _, _, err := f.Factors(); !errors.Is(err, linalg.ErrInvalid) {
			t.Fatal(err)
		}
	}
	for _, m := range []int{0, 4} {
		a, _ := linalg.Zeros(m, 0)
		f, err := linalg.FactorizeSVD(a)
		if err != nil {
			t.Fatal(err)
		}
		checkSVD(t, a, f)
	}
	empty := new(linalg.Dense)
	e, err := linalg.FactorizeEigenSym(empty)
	if err != nil {
		t.Fatal(err)
	}
	checkEigenSym(t, empty, e)
}

func TestSpectralOwnershipAndConcurrentReaders(t *testing.T) {
	a := dense(t, 3, 3, 3, 1, .2, 1, 2, .7, .2, .7, 1)
	original := a.Data()
	e, err := linalg.FactorizeEigenSym(a)
	if err != nil {
		t.Fatal(err)
	}
	s, err := linalg.FactorizeSVD(a)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 4 {
				checkEigenSym(t, a, e)
				checkSVD(t, a, s)
				ev, sv := e.Values(), s.Values()
				ev[0], sv[0] = math.NaN(), math.Inf(1)
				v, err := e.Vectors()
				if err != nil {
					t.Error(err)
					return
				}
				vd := v.Data()
				vd[0] = math.NaN()
				u, v, err := s.Factors()
				if err != nil {
					t.Error(err)
					return
				}
				ud, vd := u.Data(), v.Data()
				ud[0], vd[0] = math.NaN(), math.NaN()
				// Independent factorizations must also be safe on a shared input.
				if _, err := linalg.FactorizeEigenSym(a); err != nil {
					t.Error(err)
				}
				if _, err := linalg.FactorizeSVD(a); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	wg.Wait()
	if !slices.Equal(a.Data(), original) {
		t.Fatal("spectral factorization modified input")
	}
}
