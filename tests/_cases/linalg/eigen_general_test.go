package linalg_test

import (
	"encoding/json"
	"errors"
	"math"
	"math/cmplx"
	"os"
	"slices"
	"sync"
	"testing"

	"gomath/linalg"
)

func checkGeneralEigen(t testing.TB, a *linalg.Dense, f *linalg.Eigen) {
	t.Helper()
	n, _ := a.Dimensions()
	if f.Size() != n || len(f.Values()) != n || len(f.Residuals()) != n || len(f.RegularizedColumns()) != n || f.BasisRank() < 0 || f.BasisRank() > n {
		t.Fatal("eigen shape/diagnostics")
	}
	ad := a.Data()
	scale := 0.0
	for _, v := range ad {
		scale = math.Max(scale, math.Abs(v))
	}
	if scale == 0 {
		scale = 1
	}
	anorm := 0.0
	for _, v := range ad {
		anorm = math.Hypot(anorm, v/scale)
	}
	values := f.Values()
	for col, lambda := range values {
		vector, err := f.RightVector(col)
		if err != nil || len(vector) != n {
			t.Fatal(err)
		}
		norm := 0.0
		pivot := 0
		for i, v := range vector {
			if !finiteTest(real(v)) || !finiteTest(imag(v)) {
				t.Fatal("nonfinite eigenvector")
			}
			norm = math.Hypot(norm, cmplx.Abs(v))
			if cmplx.Abs(v) > cmplx.Abs(vector[pivot]) {
				pivot = i
			}
		}
		phaseOK := false
		for _, v := range vector {
			if cmplx.Abs(v) >= cmplx.Abs(vector[pivot])*(1-8*0x1p-52) && real(v) >= 0 && imag(v) == 0 {
				phaseOK = true
			}
		}
		if math.Abs(norm-1) > 2e-14 || !phaseOK {
			t.Fatalf("column %d norm/phase: %g %v", col, norm, vector)
		}
		scaledLambda := complex(real(lambda)/scale, imag(lambda)/scale)
		residual := 0.0
		for i := range n {
			sum := -scaledLambda * vector[i]
			for j := range n {
				sum += complex(ad[i*n+j]/scale, 0) * vector[j]
			}
			residual = math.Hypot(residual, cmplx.Abs(sum))
		}
		relative := residual / ((anorm + cmplx.Abs(scaledLambda)) * norm)
		if anorm == 0 {
			relative = residual
		}
		if relative > 2e-12 || f.Residuals()[col] > 1e-12 {
			t.Fatalf("column %d residual %g/%g lambda %v vector %v", col, relative, f.Residuals()[col], lambda, vector)
		}
		if imag(lambda) > 0 {
			if col+1 >= n || values[col+1] != cmplx.Conj(lambda) {
				t.Fatal("eigenpair ordering")
			}
			conjugate, _ := f.RightVector(col + 1)
			for i, v := range vector {
				if conjugate[i] != cmplx.Conj(v) {
					t.Fatal("not exactly conjugate columns")
				}
			}
		}
	}
}

func TestGeneralEigenAnalytic(t *testing.T) {
	cases := []struct {
		name string
		n    int
		data []float64
		rank int
	}{
		{"empty", 0, nil, 0}, {"zero", 3, make([]float64, 9), 3},
		{"negative", 1, []float64{-math.MaxFloat64}, 1},
		{"diagonal_repeat", 3, []float64{2, 0, 0, 0, 2, 0, 0, 0, -1}, 3},
		{"nonnormal_distinct", 3, []float64{1, 9, 3, 0, 2, 7, 0, 0, -3}, 3},
		{"unequal_complex", 2, []float64{4, 10, -1, 2}, 2},
		{"tiny_imaginary", 2, []float64{1, 1e-300, -1e-300, 1}, 2},
		{"near_coalescent", 2, []float64{1, 1, -math.Nextafter(1, 2), -1}, 2},
		{"unbalanced", 2, []float64{0, 1e100, -1e-100, 0}, 1},
		{"jordan", 2, []float64{1, 1, 0, 1}, 1},
		{"lower_jordan", 3, []float64{1, 0, 0, 1, 1, 0, 0, 1, 1}, 1},
		{"defective_complex", 4, []float64{0, 1, 1, 0, -1, 0, 0, 1, 0, 0, 0, 1, 0, 0, -1, 0}, 2},
		{"repeated_complex_semisimple", 4, []float64{0, 1, 0, 0, -1, 0, 0, 0, 0, 0, 0, 1, 0, 0, -1, 0}, 4},
		{"repeated_semisimple", 3, []float64{1, 1, 1, 0, 2, 1, 0, 0, 1}, 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, err := linalg.NewDense(tc.n, tc.n, tc.data)
			if err != nil {
				t.Fatal(err)
			}
			f, err := linalg.FactorizeEigen(a)
			if err != nil {
				t.Fatal(err)
			}
			checkGeneralEigen(t, a, f)
			if f.BasisRank() != tc.rank {
				t.Fatalf("rank %d want %d vectors %v regularized %v", f.BasisRank(), tc.rank, f.Values(), f.RegularizedColumns())
			}
			if tc.name == "tiny_imaginary" {
				v, _ := f.RightVector(0)
				if math.Abs(cmplx.Abs(v[0])-math.Sqrt(.5)) > 1e-15 || cmplx.Abs(v[1]/v[0]-1i) > 1e-15 {
					t.Fatal("tiny imaginary shift lost", v)
				}
			}
			if tc.name == "jordan" && !slices.Contains(f.RegularizedColumns(), true) {
				t.Fatal("singular solve not reported")
			}
		})
	}
	for _, scale := range []float64{math.SmallestNonzeroFloat64, 1e-300, 1e300, math.MaxFloat64} {
		a, _ := linalg.NewDense(2, 2, []float64{0, scale, -scale, 0})
		f, err := linalg.FactorizeEigen(a)
		if err != nil {
			t.Fatal(scale, err)
		}
		checkGeneralEigen(t, a, f)
	}
}

func TestGeneralEigenSchurFixtures(t *testing.T) {
	data, err := os.ReadFile("schur_scipy.json")
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Cases []struct {
			Name string
			N    int
			Data []float64
		}
	}
	if err = json.Unmarshal(data, &file); err != nil {
		t.Fatal(err)
	}
	for _, tc := range file.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			a, _ := linalg.NewDense(tc.N, tc.N, tc.Data)
			f, err := linalg.FactorizeEigen(a)
			if err != nil {
				t.Fatal(err)
			}
			checkGeneralEigen(t, a, f)
		})
	}
}

func TestGeneralEigenValidation(t *testing.T) {
	a, _ := linalg.NewDense(3, 3, []float64{1, 2, 3, 4, 5, 6, 7, 8, 0})
	opt := linalg.DefaultEigenOptions()
	tests := []struct {
		name   string
		change func(*linalg.EigenOptions)
		want   error
	}{
		{"work", func(o *linalg.EigenOptions) { o.MaxWork = 1 }, linalg.ErrEigenLimit},
		{"schur_work", func(o *linalg.EigenOptions) { o.Schur.MaxWork = 1 }, linalg.ErrSchurLimit},
		{"size", func(o *linalg.EigenOptions) { o.Schur.MaxSize = 2 }, linalg.ErrEigenLimit},
		{"invalid_size", func(o *linalg.EigenOptions) { o.Schur.MaxSize = 1025 }, linalg.ErrBudget},
		{"negative_size", func(o *linalg.EigenOptions) { o.Schur.MaxSize = -1 }, linalg.ErrBudget},
		{"iterations", func(o *linalg.EigenOptions) { o.Schur.MaxIterations = 0 }, linalg.ErrConvergence},
		{"negative_iterations", func(o *linalg.EigenOptions) { o.Schur.MaxIterations = -1 }, linalg.ErrBudget},
		{"residual_nan", func(o *linalg.EigenOptions) { o.RelativeResidualTolerance = math.NaN() }, linalg.ErrTolerance},
		{"residual_inf", func(o *linalg.EigenOptions) { o.RelativeResidualTolerance = math.Inf(1) }, linalg.ErrTolerance},
		{"residual_negative", func(o *linalg.EigenOptions) { o.RelativeResidualTolerance = -1 }, linalg.ErrTolerance},
		{"basis_one", func(o *linalg.EigenOptions) { o.RelativeBasisTolerance = 1 }, linalg.ErrTolerance},
		{"basis_nan", func(o *linalg.EigenOptions) { o.RelativeBasisTolerance = math.NaN() }, linalg.ErrTolerance},
		{"residual_zero", func(o *linalg.EigenOptions) { o.RelativeResidualTolerance = 0 }, linalg.ErrResidual},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			o := opt
			tc.change(&o)
			f, err := linalg.FactorizeEigenWithOptions(a, o)
			if f != nil || !errors.Is(err, tc.want) {
				t.Fatalf("result %v error %v want %v", f, err, tc.want)
			}
			if tc.name == "residual_zero" {
				var e *linalg.EigenResidualError
				if !errors.As(err, &e) || e.Column < 0 || e.Residual <= e.Limit {
					t.Fatal("missing residual diagnostics", err)
				}
			}
		})
	}
	if f, e := linalg.FactorizeEigen(nil); f != nil || !errors.Is(e, linalg.ErrInvalid) {
		t.Fatal(f, e)
	}
	rectangular, _ := linalg.Zeros(2, 3)
	if f, e := linalg.FactorizeEigen(rectangular); f != nil || !errors.Is(e, linalg.ErrDimension) {
		t.Fatal(f, e)
	}
	mixed, _ := linalg.NewDense(2, 2, []float64{math.MaxFloat64, math.SmallestNonzeroFloat64, 0, 1})
	if f, e := linalg.FactorizeEigen(mixed); f != nil || !errors.Is(e, linalg.ErrRange) {
		t.Fatal(f, e)
	}
	defective, _ := linalg.NewDense(2, 2, []float64{1, 1, 0, 1})
	opt.RequireIndependent = true
	if f, e := linalg.FactorizeEigenWithOptions(defective, opt); f != nil || !errors.Is(e, linalg.ErrRankDeficient) {
		t.Fatal(f, e)
	}
	opt = linalg.DefaultEigenOptions()
	opt.MaxWork = 0
	opt.Schur.MaxSize = 0
	opt.Schur.MaxWork = 0
	if _, e := linalg.FactorizeEigenWithOptions(a, opt); e != nil {
		t.Fatal(e)
	}
	for _, f := range []*linalg.Eigen{nil, {}} {
		if f.Size() != 0 || f.BasisRank() != 0 || f.Values() != nil || f.Residuals() != nil || f.RegularizedColumns() != nil || f.BasisTolerance() != 0 || f.SchurIterations() != 0 {
			t.Fatal("invalid diagnostics")
		}
		if v, e := f.RightVector(0); v != nil || !errors.Is(e, linalg.ErrInvalid) {
			t.Fatal(v, e)
		}
	}
	f, _ := linalg.FactorizeEigen(a)
	for _, i := range []int{-1, 3, math.MaxInt} {
		if v, e := f.RightVector(i); v != nil || !errors.Is(e, linalg.ErrDimension) {
			t.Fatal(v, e)
		}
	}
}

func TestGeneralEigenOwnershipConcurrency(t *testing.T) {
	data := []float64{4, 10, 1, -1, 2, 3, 0, 0, 5}
	a, _ := linalg.NewDense(3, 3, data)
	before := a.Data()
	f, err := linalg.FactorizeEigen(a)
	if err != nil {
		t.Fatal(err)
	}
	values, residuals, flags := f.Values(), f.Residuals(), f.RegularizedColumns()
	values[0] = 99
	residuals[0] = 99
	flags[0] = !flags[0]
	v, _ := f.RightVector(0)
	old := slices.Clone(v)
	v[0] = 99
	data[0] = 99
	other := a.Data()
	other[0] = 100
	if !slices.Equal(a.Data(), before) {
		t.Fatal("input mutation")
	}
	got, _ := f.RightVector(0)
	if !slices.Equal(got, old) || f.Values()[0] == 99 || f.Residuals()[0] == 99 || f.RegularizedColumns()[0] == flags[0] {
		t.Fatal("aliased output")
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 8 {
				local, e := linalg.FactorizeEigen(a)
				if e != nil {
					t.Error(e)
					return
				}
				checkGeneralEigen(t, a, local)
				checkGeneralEigen(t, a, f)
				v, _ := f.RightVector(0)
				v[0] = 101
			}
		})
	}
	wg.Wait()
}
