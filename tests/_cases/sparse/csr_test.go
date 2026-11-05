package sparse

import (
	"errors"
	"math"
	"reflect"
	"sync"
	"testing"
)

func must(t testing.TB, r, c int, p, ci []int, v []float64) *CSR {
	t.Helper()
	a, e := NewCSR(r, c, p, ci, v)
	if e != nil {
		t.Fatal(e)
	}
	return a
}
func diagonal(t testing.TB, v []float64) *CSR {
	t.Helper()
	p, c := make([]int, len(v)+1), make([]int, len(v))
	for i := range v {
		p[i] = i
		c[i] = i
	}
	p[len(v)] = len(v)
	return must(t, len(v), len(v), p, c, v)
}
func equalVec(t testing.TB, got, want []float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("length %d != %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("[%d] %.17g != %.17g", i, got[i], want[i])
		}
	}
}
func closeVec(t testing.TB, got, want []float64, tol float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatal("length")
	}
	for i := range got {
		if !finite(got[i]) || math.Abs(got[i]-want[i]) > tol*math.Max(1, math.Abs(want[i])) {
			t.Fatalf("[%d] %.17g != %.17g", i, got[i], want[i])
		}
	}
}
func TestCSRValidation(t *testing.T) {
	cases := []struct {
		name  string
		r, c  int
		p, ci []int
		v     []float64
		err   error
	}{
		{"negative", -1, 2, nil, nil, nil, ErrShape},
		{"overflow", int(^uint(0) >> 1), 1, nil, nil, nil, ErrLimit},
		{"missingptr", 0, 0, nil, nil, nil, ErrStructure},
		{"ptrstart", 1, 1, []int{1, 1}, nil, nil, ErrStructure},
		{"ptrend", 1, 1, []int{0, 2}, []int{0}, []float64{1}, ErrStructure},
		{"ptrnegative", 2, 1, []int{0, -1, 0}, nil, nil, ErrStructure},
		{"ptrdecrease", 3, 1, []int{0, 1, 0, 1}, []int{0}, []float64{1}, ErrStructure},
		{"colnegative", 1, 1, []int{0, 1}, []int{-1}, []float64{1}, ErrStructure},
		{"colrange", 1, 1, []int{0, 1}, []int{1}, []float64{1}, ErrStructure},
		{"duplicate", 1, 2, []int{0, 2}, []int{1, 1}, []float64{1, 2}, ErrStructure},
		{"unsorted", 1, 2, []int{0, 2}, []int{1, 0}, []float64{1, 2}, ErrStructure},
		{"mismatch", 1, 1, []int{0, 1}, nil, []float64{1}, ErrStructure},
		{"nan", 1, 1, []int{0, 1}, []int{0}, []float64{math.NaN()}, ErrNonFinite},
		{"inf", 1, 1, []int{0, 1}, []int{0}, []float64{math.Inf(1)}, ErrNonFinite},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, e := NewCSR(c.r, c.c, c.p, c.ci, c.v)
			if a != nil || !errors.Is(e, c.err) {
				t.Fatalf("%v %v expected %v", a, e, c.err)
			}
		})
	}
	for _, l := range []Limits{{MaxDimension: -1}, {MaxDimension: 4<<20 + 1}, {MaxNNZ: 16<<20 + 1}, {MaxStorageBytes: 512<<20 + 1}, {MaxWork: 1<<34 + 1}} {
		if a, e := NewCSRWithLimits(0, 0, []int{0}, nil, nil, l); a != nil || !errors.Is(e, ErrOptions) {
			t.Fatal(a, e)
		}
	}
	for _, l := range []Limits{{MaxDimension: 1}, {MaxNNZ: 1}, {MaxStorageBytes: 1}, {MaxWork: 1}} {
		if a, e := NewCSRWithLimits(2, 2, []int{0, 1, 2}, []int{0, 1}, []float64{1, 2}, l); a != nil || !errors.Is(e, ErrLimit) {
			t.Fatal(a, e)
		}
	}
}
func TestCSRArithmeticOwnership(t *testing.T) {
	p, ci, v := []int{0, 2, 3}, []int{0, 2, 1}, []float64{2, -3, 4}
	a := must(t, 2, 3, p, ci, v)
	p[1] = 0
	ci[0] = 2
	v[0] = 999
	p, ci, v = a.Data()
	p[1] = 0
	ci[0] = 2
	v[0] = 999
	if x, e := a.At(0, 0); e != nil || x != 2 {
		t.Fatal(x, e)
	}
	if x, e := a.At(1, 2); e != nil || x != 0 {
		t.Fatal(x, e)
	}
	for _, rc := range [][2]int{{-1, 0}, {2, 0}, {0, 3}} {
		if _, e := a.At(rc[0], rc[1]); !errors.Is(e, ErrShape) {
			t.Fatal(e)
		}
	}
	y, e := a.MulVec([]float64{1, 2, 3})
	if e != nil {
		t.Fatal(e)
	}
	equalVec(t, y, []float64{-7, 8})
	y[0] = 100
	at, e := a.Transpose()
	if e != nil {
		t.Fatal(e)
	}
	p, ci, v = at.Data()
	if !reflect.DeepEqual(p, []int{0, 1, 2, 3}) || !reflect.DeepEqual(ci, []int{0, 1, 0}) {
		t.Fatal(p, ci)
	}
	equalVec(t, v, []float64{2, 4, -3})
	back, e := at.Transpose()
	if e != nil {
		t.Fatal(e)
	}
	p, ci, v = back.Data()
	ap, ac, av := a.Data()
	if !reflect.DeepEqual(p, ap) || !reflect.DeepEqual(ci, ac) {
		t.Fatal(p, ci)
	}
	equalVec(t, v, av)
	b := must(t, 2, 3, []int{0, 2, 4}, []int{0, 1, 1, 2}, []float64{-2, 1, 2, 5})
	sum, e := a.Add(b)
	if e != nil {
		t.Fatal(e)
	}
	p, ci, v = sum.Data()
	if !reflect.DeepEqual(p, []int{0, 2, 4}) || !reflect.DeepEqual(ci, []int{1, 2, 1, 2}) {
		t.Fatal(p, ci)
	}
	equalVec(t, v, []float64{1, -3, 6, 5})
	diff, e := a.Sub(b)
	if e != nil {
		t.Fatal(e)
	}
	y, e = diff.MulVec([]float64{1, 2, 3})
	if e != nil {
		t.Fatal(e)
	}
	equalVec(t, y, []float64{-7, -11})
	zero, e := a.SubWithLimits(a, Limits{MaxNNZ: 3})
	if e != nil || zero.NNZ() != 0 {
		t.Fatal(zero, e)
	}
	explicit := must(t, 1, 2, []int{0, 1}, []int{1}, []float64{0})
	z, e := explicit.Transpose()
	if e != nil || z.NNZ() != 1 {
		t.Fatal(z, e)
	}
}
func TestCSREmptyNilAndBudgets(t *testing.T) {
	var z CSR
	var nilA *CSR
	p, c, v := z.Data()
	if !reflect.DeepEqual(p, []int{0}) || len(c) != 0 || len(v) != 0 {
		t.Fatal(p, c, v)
	}
	if p, c, v = nilA.Data(); p != nil || c != nil || v != nil {
		t.Fatal("nil data")
	}
	if r, c := nilA.Dims(); r != 0 || c != 0 || nilA.NNZ() != 0 {
		t.Fatal("nil dims")
	}
	for _, a := range []*CSR{&z, must(t, 0, 0, []int{0}, nil, nil)} {
		if out, e := a.Transpose(); e != nil || out.NNZ() != 0 {
			t.Fatal(e)
		}
		if out, e := a.Add(a); e != nil || out.NNZ() != 0 {
			t.Fatal(e)
		}
		if out, e := a.MulVec(nil); e != nil || len(out) != 0 {
			t.Fatal(e)
		}
	}
	r := must(t, 0, 5, []int{0}, nil, nil)
	at, e := r.Transpose()
	if e != nil {
		t.Fatal(e)
	}
	if rows, cols := at.Dims(); rows != 5 || cols != 0 {
		t.Fatal(rows, cols)
	}
	if out, e := nilA.Transpose(); out != nil || !errors.Is(e, ErrShape) {
		t.Fatal(out, e)
	}
	if out, e := z.Add(nilA); out != nil || !errors.Is(e, ErrShape) {
		t.Fatal(out, e)
	}
	if _, e := nilA.At(0, 0); !errors.Is(e, ErrShape) {
		t.Fatal(e)
	}
	a := diagonal(t, []float64{1, 2})
	if out, e := a.Add(r); out != nil || !errors.Is(e, ErrShape) {
		t.Fatal(out, e)
	}
	if out, e := a.MulVec([]float64{1}); out != nil || !errors.Is(e, ErrShape) {
		t.Fatal(out, e)
	}
	if out, e := a.MulVec([]float64{1, math.Inf(1)}); out != nil || !errors.Is(e, ErrNonFinite) {
		t.Fatal(out, e)
	}
	for _, l := range []Limits{{MaxWork: 1}, {MaxStorageBytes: 1}, {MaxDimension: 1}, {MaxNNZ: 1}, {MaxDimension: -1}} {
		if out, e := a.TransposeWithLimits(l); out != nil || e == nil {
			t.Fatal(out, e)
		}
		if out, e := a.AddWithLimits(a, l); out != nil || e == nil {
			t.Fatal(out, e)
		}
		if out, e := a.MulVecWithLimits([]float64{1, 1}, l); out != nil || e == nil {
			t.Fatal(out, e)
		}
	}
	if out, e := a.AddWithLimits(must(t, 2, 2, []int{0, 1, 2}, []int{1, 0}, []float64{1, 1}), Limits{MaxStorageBytes: 60}); out != nil || !errors.Is(e, ErrLimit) {
		t.Fatal(out, e)
	}
}
func TestCSRResultBudgets(t *testing.T) {
	a := diagonal(t, []float64{1, 2})
	b := must(t, 2, 2, []int{0, 1, 2}, []int{1, 0}, []float64{1, 2})
	if out, e := a.AddWithLimits(b, Limits{MaxNNZ: 2}); out != nil || !errors.Is(e, ErrLimit) {
		t.Fatal(out, e)
	}
	if out, e := a.TransposeWithLimits(Limits{MaxStorageBytes: 60}); out != nil || !errors.Is(e, ErrLimit) {
		t.Fatal(out, e)
	}
}
func TestCSRExactProductsAndRange(t *testing.T) {
	a := must(t, 1, 3, []int{0, 3}, []int{0, 1, 2}, []float64{1e308, -1e308, 1e-300})
	y, e := a.MulVec([]float64{1e308, 1e308, 1})
	if e != nil {
		t.Fatal(e)
	}
	equalVec(t, y, []float64{1e-300})
	for _, p := range [][2]float64{{math.MaxFloat64, 2}, {1e-300, 1e-300}} {
		a := diagonal(t, []float64{p[0]})
		if out, e := a.MulVec([]float64{p[1]}); out != nil || !errors.Is(e, ErrRange) {
			t.Fatal(out, e)
		}
	}
	a = diagonal(t, []float64{math.MaxFloat64})
	if out, e := a.Add(a); out != nil || !errors.Is(e, ErrRange) {
		t.Fatal(out, e)
	}
	tiny := diagonal(t, []float64{math.SmallestNonzeroFloat64})
	y, e = tiny.MulVec([]float64{1})
	if e != nil {
		t.Fatal(e)
	}
	equalVec(t, y, []float64{math.SmallestNonzeroFloat64})
}
func TestCSRConcurrentIsolation(t *testing.T) {
	a := diagonal(t, []float64{1, 2, 3, 4})
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				p, c, v := a.Data()
				p[0] = 99
				c[0] = 3
				v[0] = math.NaN()
				y, e := a.MulVec([]float64{1, 2, 3, 4})
				if e != nil {
					t.Error(e)
					return
				}
				equalVec(t, y, []float64{1, 4, 9, 16})
				sum, e := a.Add(a)
				if e != nil {
					t.Error(e)
					return
				}
				if sum.NNZ() != 4 {
					t.Error("sum")
				}
			}
		}()
	}
	wg.Wait()
}
