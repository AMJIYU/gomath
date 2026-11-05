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

func checkComplexLU(t testing.TB, a *linalg.ComplexDense, f *linalg.ComplexLU) {
	t.Helper()
	n, _ := a.Dimensions()
	if f.Size() != n || len(f.Pivots()) != n || f.RelativeReconstructionError() > 1e-12 {
		t.Fatal("complex factor diagnostics")
	}
	lower, upper, e := f.Factors()
	if e != nil {
		t.Fatal(e)
	}
	ad, ld, ud := a.Data(), lower.Data(), upper.Data()
	pivots := f.Pivots()
	used := make([]bool, n)
	scale := 0.0
	for _, z := range ad {
		scale = math.Max(scale, math.Max(math.Abs(real(z)), math.Abs(imag(z))))
	}
	if scale == 0 {
		scale = 1
	}
	residual, norm := 0.0, 0.0
	for i, p := range pivots {
		if p < 0 || p >= n || used[p] {
			t.Fatal("complex permutation")
		}
		used[p] = true
		for j := range n {
			if i < j && ld[i*n+j] != 0 || i > j && ud[i*n+j] != 0 || i == j && ld[i*n+j] != 1 {
				t.Fatal("complex triangular structure")
			}
			want := ad[p*n+j]
			// Independent floating oracle scales U before summing. It checks
			// normwise reconstruction even if a tiny residual at an exact zero
			// entry would be unrepresentable as a ComplexDense product.
			var product complex128
			for k := range n {
				product += ld[i*n+k] * complex(real(ud[k*n+j])/scale, imag(ud[k*n+j])/scale)
			}
			d := product - complex(real(want)/scale, imag(want)/scale)
			residual = math.Hypot(residual, cmplx.Abs(d))
			norm = math.Hypot(norm, cmplx.Abs(complex(real(want)/scale, imag(want)/scale)))
		}
	}
	if norm != 0 && residual/norm > 2e-12 {
		t.Fatal("complex LU reconstruction", residual/norm)
	}
}
func TestComplexLUAnalytic(t *testing.T) {
	for _, tc := range []struct {
		name string
		n    int
		data []complex128
	}{
		{"empty", 0, nil}, {"imaginary", 1, []complex128{2i}},
		{"swap", 2, []complex128{0, 2i, 3 - 2i, 1i}},
		{"nonhermitian", 2, []complex128{1 + 2i, 3 - 4i, 2i, 7 + 1i}},
		{"hermitian_indefinite", 2, []complex128{1, 2 + 3i, 2 - 3i, -4}},
		{"mixed_scales", 3, []complex128{complex(1e-300, 1e-300), 0, 0, 0, complex(1e300, -1e300), 0, 0, 0, -2i}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := complexMatrix(t, tc.n, tc.n, tc.data)
			f, e := linalg.FactorizeComplex(a)
			if e != nil {
				t.Fatal(e)
			}
			checkComplexLU(t, a, f)
			xwant := make([]complex128, tc.n)
			for i := range xwant {
				xwant[i] = complex(float64(i+1), -1)
			}
			b, e := a.MulVec(xwant)
			if e != nil {
				t.Fatal(e)
			}
			x, res, e := f.Solve(b)
			if e != nil {
				t.Fatal(e)
			}
			if res > 1e-12 {
				t.Fatal(res)
			}
			equalComplex(t, x, xwant, 2e-13)
			inverse, e := f.Inverse()
			if e != nil {
				t.Fatal(e)
			}
			product, e := a.Mul(inverse)
			if e != nil {
				t.Fatal(e)
			}
			id, _ := linalg.ComplexIdentity(tc.n)
			equalComplex(t, product.Data(), id.Data(), 2e-12)
			phase, logabs, e := f.LogDet()
			if e != nil || math.Abs(cmplx.Abs(phase)-1) > 4e-16 || !finiteTest(logabs) {
				t.Fatal(phase, logabs, e)
			}
			if tc.n == 0 && (phase != 1 || logabs != 0) {
				t.Fatal("empty LogDet")
			}
		})
	}
	for _, scale := range []float64{math.SmallestNonzeroFloat64, 1e-300, 1e300, math.MaxFloat64} {
		t.Run("scalar_scale", func(t *testing.T) {
			z := complex(scale, scale)
			a := complexMatrix(t, 1, 1, []complex128{z})
			f, e := linalg.FactorizeComplex(a)
			if e != nil {
				t.Fatal(e)
			}
			x, res, e := f.Solve([]complex128{z})
			if e != nil || res > 1e-12 || len(x) != 1 || cmplx.Abs(x[0]-1) > 3e-16 {
				t.Fatal(scale, x, res, e)
			}
			phase, logabs, e := f.LogDet()
			if e != nil || cmplx.Abs(phase-complex(math.Sqrt(.5), math.Sqrt(.5))) > 4e-16 || math.Abs(logabs-(math.Log(scale)+.5*math.Ln2)) > 3e-13 {
				t.Fatal(scale, phase, logabs, e)
			}
		})
	}
	// Diagonal product magnitude overflows; LogDet is still well-defined.
	a := complexMatrix(t, 2, 2, []complex128{1e300i, 0, 0, 1e300i})
	f, e := linalg.FactorizeComplex(a)
	if e != nil {
		t.Fatal(e)
	}
	phase, logabs, e := f.LogDet()
	if e != nil || phase != -1 || math.Abs(logabs-600*math.Ln10) > 3e-13 {
		t.Fatal(phase, logabs, e)
	}
}
func TestComplexLUSingularAndRange(t *testing.T) {
	for _, data := range [][]complex128{{0}, {1, 1i, 2, 2i}} {
		n := 1
		if len(data) == 4 {
			n = 2
		}
		f, e := linalg.FactorizeComplex(complexMatrix(t, n, n, data))
		var singular *linalg.ComplexSingularError
		if f != nil || !errors.Is(e, linalg.ErrSingular) || !errors.As(e, &singular) || singular.Pivot < 0 {
			t.Fatal(f, e)
		}
	}
	tinyGap := 0x1p-50
	a := complexMatrix(t, 2, 2, []complex128{1, 1, 1, complex(1+tinyGap, 0)})
	if f, e := linalg.FactorizeComplex(a); f != nil || !errors.Is(e, linalg.ErrSingular) {
		t.Fatal(f, e)
	}
	opt := linalg.DefaultComplexLUOptions()
	opt.RelativePivotTolerance = 0
	f, e := linalg.FactorizeComplexWithOptions(a, opt)
	if e != nil {
		t.Fatal(e)
	}
	x, res, e := f.Solve([]complex128{0, complex(-tinyGap, 0)})
	if e != nil || res > 1e-12 {
		t.Fatal(e, res)
	}
	equalComplex(t, x, []complex128{1, -1}, 0)
	for _, a := range []*linalg.ComplexDense{
		complexMatrix(t, 2, 2, []complex128{1, complex(math.MaxFloat64, 0), -1, complex(math.MaxFloat64, 0)}),
		complexMatrix(t, 2, 2, []complex128{1e300, 0, 1e-300, 1}),
	} {
		if f, e := linalg.FactorizeComplexWithOptions(a, opt); f != nil || !errors.Is(e, linalg.ErrRange) {
			t.Fatal(f, e)
		}
	}
	f, e = linalg.FactorizeComplex(complexMatrix(t, 1, 1, []complex128{1e-300}))
	if e != nil {
		t.Fatal(e)
	}
	if x, res, e := f.Solve([]complex128{1e300}); x != nil || res != 0 || !errors.Is(e, linalg.ErrRange) {
		t.Fatal(x, res, e)
	}
	f, e = linalg.FactorizeComplex(complexMatrix(t, 1, 1, []complex128{complex(math.MaxFloat64, math.SmallestNonzeroFloat64)}))
	if e != nil {
		t.Fatal(e)
	}
	if phase, logabs, e := f.LogDet(); phase != 0 || logabs != 0 || !errors.Is(e, linalg.ErrRange) {
		t.Fatal(phase, logabs, e)
	}
}
func TestComplexLUValidation(t *testing.T) {
	a := complexMatrix(t, 2, 2, []complex128{1 + 2i, 3 - 1i, 5i, 6 + 7i})
	f, e := linalg.FactorizeComplex(a)
	if e != nil {
		t.Fatal(e)
	}
	for _, tc := range []struct {
		opt  linalg.ComplexLUOptions
		want error
	}{
		{linalg.ComplexLUOptions{RelativePivotTolerance: math.NaN()}, linalg.ErrTolerance},
		{linalg.ComplexLUOptions{ReconstructionTolerance: 1}, linalg.ErrTolerance},
		{linalg.ComplexLUOptions{RelativePivotTolerance: -1}, linalg.ErrTolerance},
		{linalg.ComplexLUOptions{Limits: linalg.ComplexLimits{MaxWork: 1}}, linalg.ErrComplexLimit},
		{linalg.ComplexLUOptions{Limits: linalg.ComplexLimits{MaxDimension: 1}}, linalg.ErrComplexLimit},
		{linalg.ComplexLUOptions{Limits: linalg.ComplexLimits{MaxDimension: -1}}, linalg.ErrBudget},
	} {
		if out, e := linalg.FactorizeComplexWithOptions(a, tc.opt); out != nil || !errors.Is(e, tc.want) {
			t.Fatal(out, e)
		}
	}
	strict := linalg.DefaultComplexLUOptions()
	strict.ReconstructionTolerance = 0
	// This matrix's complex multipliers are inexact, so exact reconstruction fails.
	if out, e := linalg.FactorizeComplexWithOptions(a, strict); out != nil || !errors.Is(e, linalg.ErrReconstruction) {
		t.Fatal(out, e)
	} else {
		var d *linalg.ComplexReconstructionError
		if !errors.As(e, &d) || d.Relative <= d.Limit {
			t.Fatal(e)
		}
	}
	if out, e := linalg.FactorizeComplex(nil); out != nil || !errors.Is(e, linalg.ErrInvalid) {
		t.Fatal(out, e)
	}
	if out, e := linalg.FactorizeComplex(complexMatrix(t, 1, 2, []complex128{1, 2})); out != nil || !errors.Is(e, linalg.ErrDimension) {
		t.Fatal(out, e)
	}
	if x, res, e := f.Solve([]complex128{1}); x != nil || res != 0 || !errors.Is(e, linalg.ErrDimension) {
		t.Fatal(x, res, e)
	}
	if x, res, e := f.Solve([]complex128{1, complex(math.Inf(1), 0)}); x != nil || res != 0 || !errors.Is(e, linalg.ErrNonFinite) {
		t.Fatal(x, res, e)
	}
	for _, opt := range []linalg.ComplexSolveOptions{{RelativeResidualTolerance: math.Inf(1)}, {RelativeResidualTolerance: -1}, {Limits: linalg.ComplexLimits{MaxWork: 1}}, {Limits: linalg.ComplexLimits{MaxElements: 1}}, {Limits: linalg.ComplexLimits{MaxDimension: -1}}} {
		if x, res, e := f.SolveWithOptions([]complex128{2, 4 - 1i}, opt); x != nil || res != 0 || e == nil {
			t.Fatal(x, res, e)
		}
		if x, e := f.InverseWithOptions(opt); x != nil || e == nil {
			t.Fatal(x, e)
		}
	}
	opt := linalg.DefaultComplexSolveOptions()
	opt.RelativeResidualTolerance = 0
	if x, res, e := f.SolveWithOptions([]complex128{2, 4 - 1i}, opt); x != nil || res != 0 || !errors.Is(e, linalg.ErrResidual) {
		t.Fatal(x, res, e)
	} else {
		var d *linalg.ComplexResidualError
		if !errors.As(e, &d) || d.RHS != 0 || d.Relative <= d.Limit {
			t.Fatal(e)
		}
	}
	wrong := complexMatrix(t, 1, 1, []complex128{1})
	if x, res, e := f.SolveMatrix(wrong); x != nil || res != nil || !errors.Is(e, linalg.ErrDimension) {
		t.Fatal(x, res, e)
	}
	if x, res, e := f.SolveMatrix(nil); x != nil || res != nil || !errors.Is(e, linalg.ErrInvalid) {
		t.Fatal(x, res, e)
	}
	if l, u, e := f.FactorsWithLimits(linalg.ComplexLimits{MaxWork: 1}); l != nil || u != nil || !errors.Is(e, linalg.ErrComplexLimit) {
		t.Fatal(l, u, e)
	}
	for _, invalid := range []*linalg.ComplexLU{nil, {}} {
		if invalid.Size() != 0 || invalid.Pivots() != nil || invalid.RelativeReconstructionError() != 0 {
			t.Fatal("invalid introspection")
		}
		if x, res, e := invalid.Solve(nil); x != nil || res != 0 || !errors.Is(e, linalg.ErrInvalid) {
			t.Fatal(x, res, e)
		}
		if x, res, e := invalid.SolveMatrix(nil); x != nil || res != nil || !errors.Is(e, linalg.ErrInvalid) {
			t.Fatal(x, res, e)
		}
		if x, e := invalid.Inverse(); x != nil || !errors.Is(e, linalg.ErrInvalid) {
			t.Fatal(x, e)
		}
		if l, u, e := invalid.Factors(); l != nil || u != nil || !errors.Is(e, linalg.ErrInvalid) {
			t.Fatal(l, u, e)
		}
		if phase, logabs, e := invalid.LogDet(); phase != 0 || logabs != 0 || !errors.Is(e, linalg.ErrInvalid) {
			t.Fatal(phase, logabs, e)
		}
	}
	empty, _ := linalg.ComplexZeros(0, 0)
	ef, e := linalg.FactorizeComplex(empty)
	if e != nil {
		t.Fatal(e)
	}
	rhs, _ := linalg.ComplexZeros(0, 3)
	solution, res, e := ef.SolveMatrix(rhs)
	if e != nil {
		t.Fatal(e)
	}
	m, n := solution.Dimensions()
	if m != 0 || n != 3 || !slices.Equal(res, []float64{0, 0, 0}) {
		t.Fatal(m, n, res)
	}
	zeroRHS, _ := linalg.ComplexZeros(2, 0)
	solution, res, e = f.SolveMatrix(zeroRHS)
	if e != nil || len(res) != 0 {
		t.Fatal(solution, res, e)
	}
}
func TestComplexLUOwnershipConcurrency(t *testing.T) {
	a := complexMatrix(t, 2, 2, []complex128{1 + 2i, 3 - 1i, 4i, 6 + 7i})
	before := a.Data()
	f, e := linalg.FactorizeComplex(a)
	if e != nil {
		t.Fatal(e)
	}
	b := []complex128{2, 4 - 1i}
	bb := slices.Clone(b)
	x, _, e := f.Solve(b)
	if e != nil {
		t.Fatal(e)
	}
	x[0] = 99
	p := f.Pivots()
	p[0] = 99
	lower, _, _ := f.Factors()
	data := lower.Data()
	data[0] = 99
	if !slices.Equal(a.Data(), before) || !slices.Equal(b, bb) || f.Pivots()[0] == 99 {
		t.Fatal("complex LU alias/mutation")
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 8 {
				local, e := linalg.FactorizeComplex(a)
				if e != nil {
					t.Error(e)
					return
				}
				checkComplexLU(t, a, local)
				x, res, e := f.Solve(b)
				if e != nil || res > 1e-12 {
					t.Error(e, res)
					return
				}
				x[0] = 100
				if !slices.Equal(a.Data(), before) || !slices.Equal(b, bb) {
					t.Error("complex concurrent mutation")
				}
			}
		})
	}
	wg.Wait()
}
func unpackComplex(values [][2]float64) []complex128 {
	out := make([]complex128, len(values))
	for i, z := range values {
		out[i] = complex(z[0], z[1])
	}
	return out
}

// Independently recompute A*X-B with ordinary scaled complex arithmetic, so
// residual validation is not tested only against its own returned diagnostic.
func checkComplexSolution(t testing.TB, a, b, x *linalg.ComplexDense) {
	t.Helper()
	n, _ := a.Dimensions()
	_, r := b.Dimensions()
	ad, bd, xd := a.Data(), b.Data(), x.Data()
	scale := 0.0
	for _, data := range [][]complex128{ad, bd} {
		for _, z := range data {
			scale = math.Max(scale, math.Max(math.Abs(real(z)), math.Abs(imag(z))))
		}
	}
	if scale == 0 {
		scale = 1
	}
	anorm := 0.0
	for _, z := range ad {
		anorm = math.Hypot(anorm, cmplx.Abs(complex(real(z)/scale, imag(z)/scale)))
	}
	for c := range r {
		residual, bnorm, xnorm := 0.0, 0.0, 0.0
		for i := range n {
			scaledB := complex(real(bd[i*r+c])/scale, imag(bd[i*r+c])/scale)
			sum := -scaledB
			for j := range n {
				sum += complex(real(ad[i*n+j])/scale, imag(ad[i*n+j])/scale) * xd[j*r+c]
			}
			residual = math.Hypot(residual, cmplx.Abs(sum))
			bnorm = math.Hypot(bnorm, cmplx.Abs(scaledB))
			xnorm = math.Hypot(xnorm, cmplx.Abs(xd[i*r+c]))
		}
		denominator := anorm*xnorm + bnorm
		if residual != 0 && (denominator == 0 || residual/denominator > 2e-12) {
			t.Fatalf("independent complex RHS %d residual %g / %g", c, residual, denominator)
		}
	}
}
func TestComplexSciPyReference(t *testing.T) {
	data, e := os.ReadFile("complex_scipy.json")
	if e != nil {
		t.Fatal(e)
	}
	var file struct {
		NumPy, SciPy string
		Cases        []struct {
			Name              string
			N, RHS            int
			Data, B, Solution [][2]float64
			Phase             [2]float64
			LogAbs, Tolerance float64
		}
		Arithmetic []struct {
			M, K, N                           int
			A, B, Product, Transpose, Adjoint [][2]float64
		}
	}
	if e = json.Unmarshal(data, &file); e != nil {
		t.Fatal(e)
	}
	if len(file.Cases) != 30 || len(file.Arithmetic) != 4 || file.NumPy == "" || file.SciPy == "" {
		t.Fatal("complex reference inventory")
	}
	for _, tc := range file.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			a := complexMatrix(t, tc.N, tc.N, unpackComplex(tc.Data))
			b := complexMatrix(t, tc.N, tc.RHS, unpackComplex(tc.B))
			f, e := linalg.FactorizeComplex(a)
			if e != nil {
				t.Fatal(e)
			}
			checkComplexLU(t, a, f)
			x, res, e := f.SolveMatrix(b)
			if e != nil {
				t.Fatal(e)
			}
			equalComplex(t, x.Data(), unpackComplex(tc.Solution), tc.Tolerance)
			checkComplexSolution(t, a, b, x)
			for _, v := range res {
				if v > 1e-12 {
					t.Fatal(v)
				}
			}
			phase, logabs, e := f.LogDet()
			if e != nil || cmplx.Abs(phase-complex(tc.Phase[0], tc.Phase[1])) > 2e-12 || math.Abs(logabs-tc.LogAbs) > 2e-11 {
				t.Fatal(phase, logabs, e, tc.Phase, tc.LogAbs)
			}
		})
	}
	for _, tc := range file.Arithmetic {
		a := complexMatrix(t, tc.M, tc.K, unpackComplex(tc.A))
		b := complexMatrix(t, tc.K, tc.N, unpackComplex(tc.B))
		p, e := a.Mul(b)
		if e != nil {
			t.Fatal(e)
		}
		equalComplex(t, p.Data(), unpackComplex(tc.Product), 3e-14)
		tr, e := a.Transpose()
		if e != nil {
			t.Fatal(e)
		}
		equalComplex(t, tr.Data(), unpackComplex(tc.Transpose), 0)
		adj, e := a.ConjTranspose()
		if e != nil {
			t.Fatal(e)
		}
		equalComplex(t, adj.Data(), unpackComplex(tc.Adjoint), 0)
	}
}
