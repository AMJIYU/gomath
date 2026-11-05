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

// residual independently evaluates ||A*X-B||inf/(||A||inf*||X||inf+||B||inf)
// with ordinary products/sums, rather than invoking the kernel under test.
// These fixtures have moderate scale so the norm calculation cannot overflow.
func residual(a, x, b *linalg.Dense) float64 {
	var (
		n, k   = a.Dimensions()
		_, rhs = b.Dimensions()
		ad     = a.Data()
		xd     = x.Data()
		bd     = b.Data()
		normR  float64
	)
	for i := range n {
		var row float64
		for c := range rhs {
			var sum float64
			for j := range k {
				sum += ad[i*k+j] * xd[j*rhs+c]
			}
			row += math.Abs(sum - bd[i*rhs+c])
		}
		normR = math.Max(normR, row)
	}
	denominator := normInf(a)*normInf(x) + normInf(b)
	if denominator == 0 {
		return normR
	}
	return normR / denominator
}

func normInf(a *linalg.Dense) float64 {
	rows, cols := a.Dimensions()
	d := a.Data()
	var norm float64
	for i := range rows {
		var row float64
		for j := range cols {
			row += math.Abs(d[i*cols+j])
		}
		norm = math.Max(norm, row)
	}
	return norm
}

func requireResidual(t *testing.T, a, x, b *linalg.Dense, limit float64) {
	t.Helper()
	if r := residual(a, x, b); math.IsNaN(r) || math.IsInf(r, 0) || r > limit {
		t.Fatalf("relative infinity-norm residual %g > %g", r, limit)
	}
}

func TestPivotedFactorsAndSolves(t *testing.T) {
	// This requires row swaps both at the first step and after L is populated.
	a := dense(t, 3, 3, 0, 2, 1, 1, 1, 0, 2, 0, 1)
	original := a.Data()
	f, err := linalg.Factorize(a)
	if err != nil {
		t.Fatal(err)
	}
	if f.Size() != 3 || !slices.Equal(f.Pivots(), []int{2, 0, 1}) {
		t.Fatalf("size %d, permutation %v", f.Size(), f.Pivots())
	}
	lower, upper, err := f.Factors()
	if err != nil {
		t.Fatal(err)
	}
	pa := dense(t, 3, 3, 2, 0, 1, 0, 2, 1, 1, 1, 0)
	requireResidual(t, lower, upper, pa, 1e-15)
	for i := range 3 {
		for j := range 3 {
			lv, _ := lower.At(i, j)
			uv, _ := upper.At(i, j)
			if (i == j && lv != 1) || (i < j && lv != 0) || (i > j && uv != 0) {
				t.Fatal("invalid triangular factors")
			}
		}
	}
	b := []float64{7, 3, 5} // A*[1,2,3].
	for range 3 {
		x, err := f.Solve(b)
		if err != nil {
			t.Fatal(err)
		}
		requireResidual(t, a, dense(t, 3, 1, x...), dense(t, 3, 1, b...), 1e-15)
		for i, v := range x {
			if math.Abs(v-float64(i+1)) > 2e-15 {
				t.Fatalf("solution %v", x)
			}
		}
		x[0] = 99
	}
	if !slices.Equal(b, []float64{7, 3, 5}) || !slices.Equal(a.Data(), original) {
		t.Fatal("factorization or solve mutated inputs")
	}
	// Copies of factor exports cannot modify the factorization.
	p := f.Pivots()
	p[0] = 99
	ld, ud := lower.Data(), upper.Data()
	ld[0], ud[0] = math.NaN(), math.Inf(1)
	if _, err := f.Solve(b); err != nil {
		t.Fatal(err)
	}
	if d, err := f.Det(); err != nil || math.Abs(d+4) > 1e-14 {
		t.Fatalf("determinant %g, %v", d, err)
	}
}

func TestMultipleRHSAndInverse(t *testing.T) {
	a := dense(t, 3, 3, 0, 2, 1, 1, 1, 0, 2, 0, 1)
	b := dense(t, 3, 2, 7, -2, 3, 1, 5, 4)
	original := b.Data()
	f, err := linalg.Factorize(a)
	if err != nil {
		t.Fatal(err)
	}
	x, err := f.SolveMatrix(b)
	if err != nil {
		t.Fatal(err)
	}
	requireResidual(t, a, x, b, 1e-15)
	for c := range 2 {
		column := []float64{original[c], original[2+c], original[4+c]}
		xv, err := f.Solve(column)
		if err != nil {
			t.Fatal(err)
		}
		for i, v := range xv {
			got, _ := x.At(i, c)
			if math.Abs(got-v) > 1e-15 {
				t.Fatalf("RHS %d row %d: %g != %g", c, i, got, v)
			}
		}
	}
	if !slices.Equal(b.Data(), original) {
		t.Fatal("multi-RHS solve mutated B")
	}
	inv, err := f.Inverse()
	if err != nil {
		t.Fatal(err)
	}
	id, _ := linalg.Identity(3)
	requireResidual(t, a, inv, id, 1e-15)
	requireResidual(t, inv, a, id, 1e-15)
	// Exercise all one-shot integration hooks, independently of factor exports.
	x2, err := linalg.SolveMatrix(a, b)
	if err != nil {
		t.Fatal(err)
	}
	requireResidual(t, a, x2, b, 1e-15)
	inv2, err := linalg.Inverse(a)
	if err != nil {
		t.Fatal(err)
	}
	requireResidual(t, a, inv2, id, 1e-15)
	xv, err := linalg.Solve(a, []float64{7, 3, 5})
	if err != nil {
		t.Fatal(err)
	}
	requireResidual(t, a, dense(t, 3, 1, xv...), dense(t, 3, 1, 7, 3, 5), 1e-15)
	if d, err := linalg.Det(a); err != nil || math.Abs(d+4) > 1e-14 {
		t.Fatalf("one-shot det = %g, %v", d, err)
	}
	emptyRHS, _ := linalg.Zeros(3, 0)
	x0, err := f.SolveMatrix(emptyRHS)
	if err != nil {
		t.Fatal(err)
	}
	equalData(t, x0, 3, 0, nil)
}

// wellConditioned permutes a strictly diagonally dominant random matrix,
// giving known nonsingular inputs with both signs and nontrivial row pivoting.
func wellConditioned(t testing.TB, n int) *linalg.Dense {
	t.Helper()
	rng := rand.New(rand.NewPCG(uint64(n+1), 17))
	data := make([]float64, n*n)
	for i := range n {
		var rowSum float64
		for j := range n {
			if i != j {
				v := rng.Float64()*2 - 1
				data[i*n+j] = v
				rowSum += math.Abs(v)
			}
		}
		data[i*n+i] = rowSum + 1
	}
	for i := 0; i < n/2; i++ {
		for j := range n {
			data[i*n+j], data[(n-i-1)*n+j] = data[(n-i-1)*n+j], data[i*n+j]
		}
	}
	return dense(t, n, n, data...)
}

func TestResidualsAcrossSizes(t *testing.T) {
	for _, n := range []int{1, 2, 7, 31, 64} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			var (
				a    = wellConditioned(t, n)
				xdat = make([]float64, n*3)
				bdat = make([]float64, n*3)
				ad   = a.Data()
			)
			for i := range xdat {
				xdat[i] = math.Sin(float64(i + 1))
			}
			// Build RHS by an independent multiplication.
			for i := range n {
				for c := range 3 {
					for j := range n {
						bdat[i*3+c] += ad[i*n+j] * xdat[j*3+c]
					}
				}
			}
			b := dense(t, n, 3, bdat...)
			f, err := linalg.Factorize(a)
			if err != nil {
				t.Fatal(err)
			}
			x, err := f.SolveMatrix(b)
			if err != nil {
				t.Fatal(err)
			}
			requireResidual(t, a, x, b, 2e-15)
			for i, got := range x.Data() {
				if math.Abs(got-xdat[i]) > 2e-14 {
					t.Fatalf("forward error at %d: got %g, want %g", i, got, xdat[i])
				}
			}
			inv, err := f.Inverse()
			if err != nil {
				t.Fatal(err)
			}
			id, _ := linalg.Identity(n)
			requireResidual(t, a, inv, id, 2e-15)
			requireResidual(t, inv, a, id, 2e-15)
			lower, upper, err := f.Factors()
			if err != nil {
				t.Fatal(err)
			}
			permuted := make([]float64, n*n)
			for i, p := range f.Pivots() {
				copy(permuted[i*n:(i+1)*n], ad[p*n:(p+1)*n])
			}
			requireResidual(t, lower, upper, dense(t, n, n, permuted...), 2e-15)
		})
	}
}

func TestSingularityPolicies(t *testing.T) {
	for _, a := range []*linalg.Dense{
		dense(t, 1, 1, 0),
		dense(t, 2, 2, 0, 0, 0, 0),
		dense(t, 2, 2, 1, 2, 2, 4),
		dense(t, 3, 3, 1, 2, 3, 2, 4, 6, 0, 0, 0),
	} {
		for _, opt := range []linalg.LUOptions{linalg.DefaultLUOptions(), {}} {
			f, err := linalg.FactorizeWithOptions(a, opt)
			var singular *linalg.SingularError
			if f != nil || !errors.Is(err, linalg.ErrSingular) || !errors.As(err, &singular) {
				t.Fatalf("singular factor = %v, %v", f, err)
			}
			if singular.Value != 0 {
				t.Fatalf("exact singular pivot %g", singular.Value)
			}
			if singular.Error() == "" || singular.Pivot < 0 || singular.Pivot >= aRows(a) {
				t.Fatal("missing singular pivot diagnostics")
			}
		}
		if _, err := linalg.Det(a); !errors.Is(err, linalg.ErrSingular) {
			t.Fatalf("singular determinant: %v", err)
		}
		if _, err := linalg.Inverse(a); !errors.Is(err, linalg.ErrSingular) {
			t.Fatalf("singular inverse: %v", err)
		}
		if _, err := linalg.Solve(a, make([]float64, aRows(a))); !errors.Is(err, linalg.ErrSingular) {
			t.Fatalf("singular solve: %v", err)
		}
	}
	near := dense(t, 2, 2, 1, 1, 1, 1+2e-15)
	if _, err := linalg.Factorize(near); !errors.Is(err, linalg.ErrSingular) {
		t.Fatalf("default near singular: %v", err)
	}
	f, err := linalg.FactorizeWithOptions(near, linalg.LUOptions{})
	if err != nil {
		t.Fatal(err)
	}
	b := dense(t, 2, 1, 2, 2+2e-15)
	x, err := f.SolveMatrix(b)
	if err != nil {
		t.Fatal(err)
	}
	requireResidual(t, near, x, b, 1e-15)
	// A nonzero ratio can underflow to zero. Exact-zero mode must still accept.
	extreme := dense(t, 2, 2, 1e-300, 1e300, 0, 1)
	if _, err := linalg.Factorize(extreme); !errors.Is(err, linalg.ErrSingular) {
		t.Fatalf("relative policy should reject extreme row: %v", err)
	}
	f, err = linalg.FactorizeWithOptions(extreme, linalg.LUOptions{})
	if err != nil {
		t.Fatal(err)
	}
	xv, err := f.Solve([]float64{1e300, 1})
	if err != nil || !slices.Equal(xv, []float64{0, 1}) {
		t.Fatalf("exact-only extreme solve = %v, %v", xv, err)
	}
	for _, value := range []float64{.5, math.Nextafter(.5, 1)} {
		a := dense(t, 2, 2, value, 1, 0, 1)
		_, err := linalg.FactorizeWithOptions(a, linalg.LUOptions{RelativePivotTolerance: .5})
		if value == .5 && !errors.Is(err, linalg.ErrSingular) || value > .5 && err != nil {
			t.Fatalf("inclusive tolerance boundary %g: %v", value, err)
		}
	}
	// Scales must move with their original rows during pivoting. Otherwise
	// the small second pivot would incorrectly be compared to scale 1.
	swappedScales := dense(t, 2, 2, 0, 1e-6, 1, 1)
	f, err = linalg.FactorizeWithOptions(swappedScales, linalg.LUOptions{RelativePivotTolerance: .01})
	if err != nil {
		t.Fatal("row scale did not follow permutation", err)
	}
	xv, err = f.Solve([]float64{1e-6, 2})
	if err != nil || !slices.Equal(xv, []float64{1, 1}) {
		t.Fatalf("permuted scale solve = %v, %v", xv, err)
	}
}

func aRows(a *linalg.Dense) int { n, _ := a.Dimensions(); return n }

func TestScalingAndSubnormalPivots(t *testing.T) {
	for _, scale := range []float64{1e-300, 1, 1e300} {
		a := dense(t, 2, 2, 2*scale, scale, 4*scale, 5*scale)
		b := []float64{scale, -scale} // known solution [1,-1]
		x, err := linalg.Solve(a, b)
		if err != nil {
			t.Fatal(scale, err)
		}
		for i, want := range []float64{1, -1} {
			if math.Abs(x[i]-want) > 2e-15 {
				t.Fatalf("uniform scale %g solution %v", scale, x)
			}
		}
		// Divide each equation by scale before computing a residual, so the
		// residual check itself does not overflow/underflow at extreme scale.
		for i := range 2 {
			var r float64
			for j := range 2 {
				v, _ := a.At(i, j)
				r += (v / scale) * x[j]
			}
			if math.Abs(r-b[i]/scale) > 1e-14 {
				t.Fatalf("scale %g row %d residual %g", scale, i, r-b[i]/scale)
			}
		}
	}
	// A global absolute cutoff would wrongly reject the small diagonal.
	for _, values := range [][]float64{
		{1e-200, 0, 0, 1e200},
		{math.SmallestNonzeroFloat64, 0, 0, 2 * math.SmallestNonzeroFloat64},
	} {
		a := dense(t, 2, 2, values...)
		x, err := linalg.Solve(a, []float64{values[0], values[3]})
		if err != nil || !slices.Equal(x, []float64{1, 1}) {
			t.Fatalf("ill-scaled diagonal solution %v, %v", x, err)
		}
	}
}

func TestDeterminantScalingAndSign(t *testing.T) {
	for _, tc := range []struct {
		name string
		a    *linalg.Dense
		want float64
	}{
		{"odd-swap", dense(t, 2, 2, 0, 1, 1, 0), -1},
		{"negative", dense(t, 1, 1, -3), -3},
		{"avoid-intermediate-overflow", diagonal(t, 1e200, 1e200, 1e-200, 1e-200), 1},
		{"avoid-intermediate-underflow", diagonal(t, 1e-200, 1e-200, 1e200, 1e200), 1},
		{"subnormal", diagonal(t, math.SmallestNonzeroFloat64), math.SmallestNonzeroFloat64},
		{"max", diagonal(t, math.MaxFloat64), math.MaxFloat64},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := linalg.Det(tc.a)
			if err != nil || got == 0 || math.Abs((got-tc.want)/tc.want) > 1e-14 {
				t.Fatalf("det = %.17g, %v; want %.17g", got, err, tc.want)
			}
		})
	}
	for _, tc := range []struct {
		values []float64
		sign   int
		logAbs float64
	}{
		{[]float64{-1e200, 1e200}, -1, 400 * math.Ln10},
		{[]float64{1e-200, 1e-200}, 1, -400 * math.Ln10},
	} {
		f, err := linalg.Factorize(diagonal(t, tc.values...))
		if err != nil {
			t.Fatal(err)
		}
		if d, err := f.Det(); d != 0 || !errors.Is(err, linalg.ErrRange) {
			t.Fatalf("out-of-range determinant = %g, %v", d, err)
		}
		sign, logAbs, err := f.LogDet()
		if err != nil || sign != tc.sign || math.Abs(logAbs-tc.logAbs) > 1e-12 {
			t.Fatalf("LogDet = %d, %g, %v", sign, logAbs, err)
		}
	}
}

func TestDeterminantAgainstPermutationExpansion(t *testing.T) {
	// Independent O(n!) reference for small nonsymmetric matrices checks
	// determinant sign and row permutations without sharing the LU algorithm.
	for n := 1; n <= 6; n++ {
		a := wellConditioned(t, n)
		data := a.Data()
		permutation := make([]int, n)
		used := make([]bool, n)
		var (
			want   float64
			expand func(int)
		)
		expand = func(row int) {
			if row == n {
				term := 1.0
				for i, col := range permutation {
					term *= data[i*n+col]
					for j := 0; j < i; j++ {
						if permutation[j] > col {
							term = -term
						}
					}
				}
				want += term
				return
			}
			for col := range n {
				if !used[col] {
					permutation[row], used[col] = col, true
					expand(row + 1)
					used[col] = false
				}
			}
		}
		expand(0)
		got, err := linalg.Det(a)
		if err != nil || math.Abs((got-want)/want) > 5e-14 {
			t.Fatalf("n=%d det = %g, %v, expansion = %g", n, got, err, want)
		}
	}
}

func TestIllConditionedResidual(t *testing.T) {
	// Hilbert matrices separate backward stability from forward accuracy:
	// a tiny residual does not establish an accurate solution for every input.
	const n = 8
	var (
		data = make([]float64, n*n)
		b    = make([]float64, n)
	)
	for i := range n {
		for j := range n {
			data[i*n+j] = 1 / float64(i+j+1)
			b[i] += data[i*n+j]
		}
	}
	a := dense(t, n, n, data...)
	x, err := linalg.Solve(a, b)
	if err != nil {
		t.Fatal(err)
	}
	requireResidual(t, a, dense(t, n, 1, x...), dense(t, n, 1, b...), 1e-15)
	for i, v := range x {
		if math.Abs(v-1) > 1e-5 {
			t.Fatalf("Hilbert solution component %d = %g, expected near 1", i, v)
		}
	}
}

func diagonal(t testing.TB, values ...float64) *linalg.Dense {
	t.Helper()
	n := len(values)
	data := make([]float64, n*n)
	for i, v := range values {
		data[i*n+i] = v
	}
	return dense(t, n, n, data...)
}

func TestEmptyFactorization(t *testing.T) {
	var a linalg.Dense
	f, err := linalg.Factorize(&a)
	if err != nil {
		t.Fatal(err)
	}
	if d, err := f.Det(); d != 1 || err != nil {
		t.Fatal(d, err)
	}
	if sign, logAbs, err := f.LogDet(); sign != 1 || logAbs != 0 || err != nil {
		t.Fatal(sign, logAbs, err)
	}
	if x, err := f.Solve(nil); len(x) != 0 || err != nil {
		t.Fatal(x, err)
	}
	b, _ := linalg.Zeros(0, 4)
	x, err := f.SolveMatrix(b)
	if err != nil {
		t.Fatal(err)
	}
	equalData(t, x, 0, 4, nil)
	inv, err := f.Inverse()
	if err != nil {
		t.Fatal(err)
	}
	equalData(t, inv, 0, 0, nil)
	lower, upper, err := f.Factors()
	if err != nil {
		t.Fatal(err)
	}
	equalData(t, lower, 0, 0, nil)
	equalData(t, upper, 0, 0, nil)
}

func TestFactorizationAndSolveErrors(t *testing.T) {
	rect := dense(t, 2, 3, 1, 2, 3, 4, 5, 6)
	for _, shape := range []*linalg.Dense{rect, dense(t, 0, 2), dense(t, 2, 0)} {
		if _, err := linalg.Factorize(shape); !errors.Is(err, linalg.ErrDimension) {
			t.Fatalf("rectangular factorization: %v", err)
		}
	}
	id, _ := linalg.Identity(2)
	for _, tol := range []float64{-1, 1, 2, math.NaN(), math.Inf(1), math.Inf(-1)} {
		if _, err := linalg.FactorizeWithOptions(id, linalg.LUOptions{RelativePivotTolerance: tol}); !errors.Is(err, linalg.ErrTolerance) {
			t.Fatalf("bad tolerance %g: %v", tol, err)
		}
	}
	if _, err := linalg.Factorize(nil); !errors.Is(err, linalg.ErrInvalid) {
		t.Fatal(err)
	}
	f, err := linalg.Factorize(id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Solve([]float64{1}); !errors.Is(err, linalg.ErrDimension) {
		t.Fatal(err)
	}
	if _, err := f.SolveMatrix(rect); err != nil { // 2-by-3 is valid RHS.
		t.Fatal(err)
	}
	wrong, _ := linalg.Zeros(3, 2)
	if _, err := f.SolveMatrix(wrong); !errors.Is(err, linalg.ErrDimension) {
		t.Fatal(err)
	}
	if _, err := f.SolveMatrix(nil); !errors.Is(err, linalg.ErrInvalid) {
		t.Fatal(err)
	}
	for _, bad := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if x, err := f.Solve([]float64{bad, 1}); x != nil || !errors.Is(err, linalg.ErrNonFinite) {
			t.Fatalf("bad RHS = %v, %v", x, err)
		}
	}
	for _, invalid := range []*linalg.LU{nil, {}} {
		if _, err := invalid.Solve(nil); !errors.Is(err, linalg.ErrInvalid) {
			t.Fatal(err)
		}
		if _, err := invalid.SolveMatrix(nil); !errors.Is(err, linalg.ErrInvalid) {
			t.Fatal(err)
		}
		if _, err := invalid.Inverse(); !errors.Is(err, linalg.ErrInvalid) {
			t.Fatal(err)
		}
		if _, err := invalid.Det(); !errors.Is(err, linalg.ErrInvalid) {
			t.Fatal(err)
		}
		if _, _, err := invalid.LogDet(); !errors.Is(err, linalg.ErrInvalid) {
			t.Fatal(err)
		}
		if _, _, err := invalid.Factors(); !errors.Is(err, linalg.ErrInvalid) {
			t.Fatal(err)
		}
		if invalid.Size() != 0 || invalid.Pivots() != nil {
			t.Fatal("invalid introspection")
		}
	}
}

func TestFactorAndSolveOverflow(t *testing.T) {
	max := math.MaxFloat64
	if f, err := linalg.Factorize(dense(t, 2, 2, max, max, -max, max)); f != nil || !errors.Is(err, linalg.ErrNonFinite) {
		t.Fatalf("LU overflow = %v, %v", f, err)
	}
	for _, a := range []*linalg.Dense{
		dense(t, 1, 1, .5),
		dense(t, 2, 2, 1, 0, -1, 1),
		dense(t, 2, 2, 1, -1, 0, 1),
	} {
		f, err := linalg.Factorize(a)
		if err != nil {
			t.Fatal(err)
		}
		n := f.Size()
		b := make([]float64, n)
		for i := range b {
			b[i] = max
		}
		if x, err := f.Solve(b); x != nil || !errors.Is(err, linalg.ErrNonFinite) {
			t.Fatalf("solve overflow = %v, %v", x, err)
		}
		if x, err := f.SolveMatrix(dense(t, n, 1, b...)); x != nil || !errors.Is(err, linalg.ErrNonFinite) {
			t.Fatalf("multi-RHS overflow = %v, %v", x, err)
		}
		// A failed solve must not damage reusable factors.
		if _, err := f.Solve(make([]float64, n)); err != nil {
			t.Fatal("factor damaged by failed solve", err)
		}
	}
	f, err := linalg.Factorize(diagonal(t, math.SmallestNonzeroFloat64))
	if err != nil {
		t.Fatal(err)
	}
	if x, err := f.Inverse(); x != nil || !errors.Is(err, linalg.ErrNonFinite) {
		t.Fatalf("inverse overflow = %v, %v", x, err)
	}
}

func TestConcurrentFactorReuse(t *testing.T) {
	var (
		a     = wellConditioned(t, 17)
		b     = make([]float64, 17)
		group sync.WaitGroup
		errs  = make(chan error, 16)
	)
	for i := range b {
		b[i] = float64(i + 1)
	}
	f, err := linalg.Factorize(a)
	if err != nil {
		t.Fatal(err)
	}
	want, err := f.Solve(b)
	if err != nil {
		t.Fatal(err)
	}
	for range 16 {
		group.Go(func() {
			for range 10 {
				got, err := f.Solve(b)
				if err != nil || !slices.Equal(got, want) {
					errs <- fmt.Errorf("concurrent solve %v: %v", got, err)
					return
				}
				if _, err := a.Mul(a); err != nil {
					errs <- err
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
