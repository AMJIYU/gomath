package linalg_test

import (
	"errors"
	"math"
	"math/cmplx"
	"slices"
	"sync"
	"testing"

	"gomath/linalg"
)

func complexMatrix(t testing.TB, m, n int, data []complex128) *linalg.ComplexDense {
	t.Helper()
	a, e := linalg.NewComplexDense(m, n, data)
	if e != nil {
		t.Fatal(e)
	}
	return a
}
func equalComplex(t testing.TB, got, want []complex128, tol float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatal("complex length", len(got), len(want))
	}
	for i, z := range want {
		if !finiteTest(real(got[i])) || !finiteTest(imag(got[i])) || cmplx.Abs(got[i]-z) > tol*(1+cmplx.Abs(z)) {
			t.Fatalf("complex entry %d got %v want %v", i, got[i], z)
		}
	}
}
func TestComplexDenseArithmetic(t *testing.T) {
	a := complexMatrix(t, 2, 3, []complex128{1 + 2i, 3 - 4i, 5, 6i, -7, 8 - 9i})
	tr, e := a.Transpose()
	if e != nil {
		t.Fatal(e)
	}
	equalComplex(t, tr.Data(), []complex128{1 + 2i, 6i, 3 - 4i, -7, 5, 8 - 9i}, 0)
	adj, e := a.ConjTranspose()
	if e != nil {
		t.Fatal(e)
	}
	equalComplex(t, adj.Data(), []complex128{1 - 2i, -6i, 3 + 4i, -7, 5, 8 + 9i}, 0)
	back, _ := adj.ConjTranspose()
	equalComplex(t, back.Data(), a.Data(), 0)
	b := complexMatrix(t, 3, 2, []complex128{1, 2i, 3, 4, 5i, 6})
	product, e := a.Mul(b)
	if e != nil {
		t.Fatal(e)
	}
	equalComplex(t, product.Data(), []complex128{10 + 15i, 38 - 14i, 24 + 46i, 8 - 54i}, 0)
	vector, e := a.MulVec([]complex128{1, -1i, 2})
	if e != nil {
		t.Fatal(e)
	}
	equalComplex(t, vector, []complex128{7 - 1i, 16 - 5i}, 0)
	twice, e := a.Add(a)
	if e != nil {
		t.Fatal(e)
	}
	difference, e := twice.Sub(a)
	if e != nil {
		t.Fatal(e)
	}
	equalComplex(t, difference.Data(), a.Data(), 0)
	id, _ := linalg.ComplexIdentity(3)
	p, e := a.Mul(id)
	if e != nil {
		t.Fatal(e)
	}
	equalComplex(t, p.Data(), a.Data(), 0)
	z, _ := linalg.ComplexZeros(2, 0)
	right, _ := linalg.ComplexZeros(0, 3)
	p, e = z.Mul(right)
	if e != nil {
		t.Fatal(e)
	}
	r, c := p.Dimensions()
	if r != 2 || c != 3 {
		t.Fatal(r, c)
	}
	equalComplex(t, p.Data(), make([]complex128, 6), 0)
	var empty linalg.ComplexDense
	if p, e := empty.Mul(&empty); e != nil || len(p.Data()) != 0 {
		t.Fatal(p, e)
	}
	value, e := a.At(1, 2)
	if e != nil || value != 8-9i {
		t.Fatal(value, e)
	}
}
func TestComplexDenseRange(t *testing.T) {
	max := math.MaxFloat64
	// Products overflow binary64 individually, but their exact dot sum is finite.
	a := complexMatrix(t, 1, 3, []complex128{complex(max, max), complex(max, max), 1})
	b := complexMatrix(t, 3, 1, []complex128{complex(max, -max), complex(-max, max), 3 - 2i})
	p, e := a.Mul(b)
	if e != nil {
		t.Fatal(e)
	}
	equalComplex(t, p.Data(), []complex128{3 - 2i}, 0)
	v, e := a.MulVec(b.Data())
	if e != nil {
		t.Fatal(e)
	}
	equalComplex(t, v, []complex128{3 - 2i}, 0)
	one := complexMatrix(t, 1, 1, []complex128{complex(max, max)})
	if p, e := one.Add(one); p != nil || !errors.Is(e, linalg.ErrRange) {
		t.Fatal(p, e)
	}
	if p, e := one.Sub(complexMatrix(t, 1, 1, []complex128{complex(-max, -max)})); p != nil || !errors.Is(e, linalg.ErrRange) {
		t.Fatal(p, e)
	}
	if p, e := one.Mul(one); p != nil || !errors.Is(e, linalg.ErrRange) {
		t.Fatal(p, e)
	}
	tiny := complexMatrix(t, 1, 1, []complex128{complex(math.SmallestNonzeroFloat64, 0)})
	if p, e := tiny.Mul(tiny); p != nil || !errors.Is(e, linalg.ErrRange) {
		t.Fatal(p, e)
	}
	if v, e := tiny.MulVec(tiny.Data()); v != nil || !errors.Is(e, linalg.ErrRange) {
		t.Fatal(v, e)
	}
}
func TestComplexDenseBoundsAndInvalid(t *testing.T) {
	for _, shape := range [][2]int{{-1, 0}, {0, -1}, {math.MaxInt, 2}, {2, math.MaxInt}, {math.MaxInt, 0}, {0, math.MaxInt}} {
		a, e := linalg.ComplexZeros(shape[0], shape[1])
		if a != nil || !(errors.Is(e, linalg.ErrDimension) || errors.Is(e, linalg.ErrComplexLimit)) {
			t.Fatal(a, e)
		}
	}
	for _, z := range []complex128{complex(math.NaN(), 0), complex(0, math.Inf(1)), complex(math.Inf(-1), 1)} {
		if a, e := linalg.NewComplexDense(1, 1, []complex128{z}); a != nil || !errors.Is(e, linalg.ErrNonFinite) {
			t.Fatal(a, e)
		}
	}
	if a, e := linalg.NewComplexDense(2, 2, []complex128{1}); a != nil || !errors.Is(e, linalg.ErrDimension) {
		t.Fatal(a, e)
	}
	for _, l := range []linalg.ComplexLimits{{MaxDimension: -1}, {MaxDimension: 4097}, {MaxElements: 16<<20 + 1}} {
		if a, e := linalg.ComplexZerosWithLimits(0, 0, l); a != nil || !errors.Is(e, linalg.ErrBudget) {
			t.Fatal(a, e)
		}
	}
	a := complexMatrix(t, 2, 2, []complex128{1, 2i, 3 - 1i, 4})
	b := complexMatrix(t, 2, 2, []complex128{2, 1, 3i, 4})
	low := linalg.ComplexLimits{MaxWork: 1}
	for _, call := range []func() (*linalg.ComplexDense, error){func() (*linalg.ComplexDense, error) { return linalg.NewComplexDenseWithLimits(2, 2, a.Data(), low) }, func() (*linalg.ComplexDense, error) { return linalg.ComplexZerosWithLimits(2, 2, low) }, func() (*linalg.ComplexDense, error) { return a.TransposeWithLimits(low) }, func() (*linalg.ComplexDense, error) { return a.ConjTransposeWithLimits(low) }, func() (*linalg.ComplexDense, error) { return a.AddWithLimits(b, low) }, func() (*linalg.ComplexDense, error) { return a.SubWithLimits(b, low) }, func() (*linalg.ComplexDense, error) { return a.MulWithLimits(b, low) }} {
		if out, e := call(); out != nil || !errors.Is(e, linalg.ErrComplexLimit) {
			t.Fatal(out, e)
		}
	}
	if out, e := a.MulVecWithLimits([]complex128{1, 2}, low); out != nil || !errors.Is(e, linalg.ErrComplexLimit) {
		t.Fatal(out, e)
	}
	if out, e := linalg.ComplexZerosWithLimits(2, 2, linalg.ComplexLimits{MaxElements: 3}); out != nil || !errors.Is(e, linalg.ErrComplexLimit) {
		t.Fatal(out, e)
	}
	if out, e := a.MulWithLimits(b, linalg.ComplexLimits{MaxDimension: 1}); out != nil || !errors.Is(e, linalg.ErrComplexLimit) {
		t.Fatal(out, e)
	}
	rectangular := complexMatrix(t, 1, 2, []complex128{1, 2})
	if out, e := a.Add(rectangular); out != nil || !errors.Is(e, linalg.ErrDimension) {
		t.Fatal(out, e)
	}
	if out, e := a.Sub(rectangular); out != nil || !errors.Is(e, linalg.ErrDimension) {
		t.Fatal(out, e)
	}
	if out, e := rectangular.Mul(rectangular); out != nil || !errors.Is(e, linalg.ErrDimension) {
		t.Fatal(out, e)
	}
	if out, e := a.MulVec([]complex128{1}); out != nil || !errors.Is(e, linalg.ErrDimension) {
		t.Fatal(out, e)
	}
	if out, e := a.MulVec([]complex128{complex(0, math.NaN()), 1}); out != nil || !errors.Is(e, linalg.ErrNonFinite) {
		t.Fatal(out, e)
	}
	for _, index := range [][2]int{{-1, 0}, {0, -1}, {2, 0}, {0, 2}} {
		if z, e := a.At(index[0], index[1]); z != 0 || !errors.Is(e, linalg.ErrDimension) {
			t.Fatal(z, e)
		}
	}
	var nilMatrix *linalg.ComplexDense
	r, c := nilMatrix.Dimensions()
	if r != 0 || c != 0 || nilMatrix.Data() != nil {
		t.Fatal("nil introspection")
	}
	if z, e := nilMatrix.At(0, 0); z != 0 || !errors.Is(e, linalg.ErrInvalid) {
		t.Fatal(z, e)
	}
	for _, call := range []func() (*linalg.ComplexDense, error){nilMatrix.Transpose, nilMatrix.ConjTranspose, func() (*linalg.ComplexDense, error) { return nilMatrix.Add(a) }, func() (*linalg.ComplexDense, error) { return a.Sub(nil) }, func() (*linalg.ComplexDense, error) { return a.Mul(nil) }} {
		if out, e := call(); out != nil || !errors.Is(e, linalg.ErrInvalid) {
			t.Fatal(out, e)
		}
	}
	if out, e := nilMatrix.MulVec(nil); out != nil || !errors.Is(e, linalg.ErrInvalid) {
		t.Fatal(out, e)
	}
}
func TestComplexDenseOwnershipConcurrency(t *testing.T) {
	data := []complex128{1 + 2i, 3, 4i, 5}
	a := complexMatrix(t, 2, 2, data)
	before := slices.Clone(data)
	data[0] = 99
	out := a.Data()
	out[0] = 100
	if !slices.Equal(a.Data(), before) {
		t.Fatal("constructor/export alias")
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 12 {
				b, e := a.ConjTranspose()
				if e != nil {
					t.Error(e)
					return
				}
				c, e := a.Mul(b)
				if e != nil {
					t.Error(e)
					return
				}
				v := c.Data()
				v[0] = 101
				if !slices.Equal(a.Data(), before) {
					t.Error("mutated input")
				}
			}
		})
	}
	wg.Wait()
}
