package sparse

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
	"reflect"
	"slices"
	"strconv"
	"sync"
	"testing"
)

func matrixEqual(t testing.TB, a, b *CSR) {
	t.Helper()
	ar, ac := a.Dims()
	br, bc := b.Dims()
	if ar != br || ac != bc {
		t.Fatal("shape", ar, ac, br, bc)
	}
	ap, ai, av := a.Data()
	bp, bi, bv := b.Data()
	if !reflect.DeepEqual(ap, bp) || !reflect.DeepEqual(ai, bi) {
		t.Fatal("structure", ap, ai, bp, bi)
	}
	equalVec(t, av, bv)
}
func TestMulRectangularCanonicalAndOwnership(t *testing.T) {
	a := must(t, 2, 3, []int{0, 2, 4}, []int{0, 2, 1, 2}, []float64{2, -1, 3, 4})
	b := must(t, 3, 4, []int{0, 2, 4, 7}, []int{0, 3, 1, 2, 0, 1, 3}, []float64{1, 5, -1, 2, 2, 3, 1})
	want := must(t, 2, 4, []int{0, 2, 6}, []int{1, 3, 0, 1, 2, 3}, []float64{-3, 9, 8, 9, 6, 4})
	for run := 0; run < 8; run++ {
		got, e := a.Mul(b)
		if e != nil {
			t.Fatal(e)
		}
		matrixEqual(t, got, want)
		p, c, v := got.Data()
		p[0] = 99
		c[0] = 0
		v[0] = math.NaN()
		matrixEqual(t, got, want)
	}
	// The result retains neither operand's backing storage. Construction already
	// isolates source slices; Data mutations cannot affect products or inputs.
	ap, ai, av := a.Data()
	bp, bi, bv := b.Data()
	got, e := a.Mul(b)
	if e != nil {
		t.Fatal(e)
	}
	av[0] = 100
	bv[0] = 100
	ap[0] = 100
	ai[0] = 100
	bp[0] = 100
	bi[0] = 100
	matrixEqual(t, got, want)
	if v, e := a.At(0, 0); e != nil || v != 2 {
		t.Fatal(v, e)
	}
	if v, e := b.At(0, 0); e != nil || v != 1 {
		t.Fatal(v, e)
	}
}
func TestMulEmptyZerosAndShape(t *testing.T) {
	var zero CSR
	for _, dims := range [][3]int{{0, 0, 0}, {0, 5, 7}, {4, 0, 7}, {3, 2, 0}, {4, 3, 5}} {
		r, k, c := dims[0], dims[1], dims[2]
		a := must(t, r, k, make([]int, r+1), nil, nil)
		b := must(t, k, c, make([]int, k+1), nil, nil)
		got, e := a.Mul(b)
		if e != nil {
			t.Fatal(e)
		}
		gr, gc := got.Dims()
		p, ci, v := got.Data()
		if gr != r || gc != c || len(p) != r+1 || len(ci) != 0 || len(v) != 0 {
			t.Fatal("zero", gr, gc, p, ci, v)
		}
	}
	got, e := zero.Mul(&zero)
	if e != nil {
		t.Fatal(e)
	}
	matrixEqual(t, got, &zero)
	a := must(t, 1, 2, []int{0, 2}, []int{0, 1}, []float64{0, 2})
	b := must(t, 2, 2, []int{0, 2, 4}, []int{0, 1, 0, 1}, []float64{math.MaxFloat64, math.MaxFloat64, 0, 3})
	got, e = a.Mul(b)
	if e != nil {
		t.Fatal(e)
	}
	matrixEqual(t, got, must(t, 1, 2, []int{0, 1}, []int{1}, []float64{6}))
	var nilA *CSR
	for _, pair := range [][2]*CSR{{nilA, a}, {a, nilA}, {a, a}} {
		if out, e := pair[0].Mul(pair[1]); out != nil || !errors.Is(e, ErrShape) {
			t.Fatal(out, e)
		}
	}
}
func TestMulLimitsAndCheckedCounts(t *testing.T) {
	a := must(t, 1, 2, []int{0, 2}, []int{0, 1}, []float64{1, 1})
	b := must(t, 2, 1, []int{0, 1, 2}, []int{0, 0}, []float64{1, -1})
	// Two streams, 2 visited pairs, both passes: base=16, traversal=48.
	if out, e := a.MulWithLimits(b, Limits{MaxWork: 63}); out != nil || !errors.Is(e, ErrLimit) {
		t.Fatal(out, e)
	}
	if out, e := a.MulWithLimits(b, Limits{MaxWork: 64}); e != nil || out.NNZ() != 0 {
		t.Fatal(out, e)
	}
	// Cancellation leaves no output entries, but the sparse heap still costs
	// 2*24 bytes, pointers 16 bytes, numeric reserve 16 KiB.
	for _, bytes := range []uint64{productScratchBytes + 64 - 1, productScratchBytes + 64} {
		out, e := a.MulWithLimits(b, Limits{MaxStorageBytes: bytes})
		if bytes == productScratchBytes+64 {
			if e != nil || out.NNZ() != 0 {
				t.Fatal(out, e)
			}
		} else if out != nil || !errors.Is(e, ErrLimit) {
			t.Fatal(out, e)
		}
	}
	left := must(t, 2, 1, []int{0, 1, 2}, []int{0, 0}, []float64{1, 1})
	right := must(t, 1, 2, []int{0, 2}, []int{0, 1}, []float64{1, 1})
	if out, e := left.MulWithLimits(right, Limits{MaxNNZ: 2}); out != nil || !errors.Is(e, ErrLimit) {
		t.Fatal(out, e)
	}
	reserve := productScratchBytes + 24 + 24 + 4*16
	if out, e := left.MulWithLimits(right, Limits{MaxStorageBytes: reserve - 1}); out != nil || !errors.Is(e, ErrLimit) {
		t.Fatal(out, e)
	}
	if out, e := left.MulWithLimits(right, Limits{MaxStorageBytes: reserve}); e != nil || out.NNZ() != 4 {
		t.Fatal(out, e)
	}
	// Each operand is validated even if an empty/zero product could be returned.
	for _, pair := range [][2]*CSR{{a, must(t, 2, 1, []int{0, 1, 1}, []int{0}, []float64{1})}, {must(t, 1, 2, []int{0, 1}, []int{0}, []float64{1}), b}} {
		if out, e := pair[0].MulWithLimits(pair[1], Limits{MaxNNZ: 1}); out != nil || !errors.Is(e, ErrLimit) {
			t.Fatal(out, e)
		}
	}
	empty := must(t, 0, 2, []int{0}, nil, nil)
	if out, e := empty.MulWithLimits(b, Limits{MaxStorageBytes: 20}); out != nil || !errors.Is(e, ErrLimit) {
		t.Fatal(out, e)
	}
	// Left operand storage must also be checked when all result columns vanish.
	if out, e := a.MulWithLimits(must(t, 2, 0, []int{0, 0, 0}, nil, nil), Limits{MaxStorageBytes: 32}); out != nil || !errors.Is(e, ErrLimit) {
		t.Fatal(out, e)
	}

	for _, l := range []Limits{{MaxDimension: -1}, {MaxDimension: 4<<20 + 1}, {MaxNNZ: 16<<20 + 1}, {MaxWork: 1<<34 + 1}, {MaxStorageBytes: 512<<20 + 1}} {
		if out, e := a.MulWithLimits(b, l); out != nil || !errors.Is(e, ErrOptions) {
			t.Fatal(out, e)
		}
	}
	for _, f := range []func() (uint64, error){func() (uint64, error) { return productAdd(^uint64(0), 1) }, func() (uint64, error) { return productScale(^uint64(0), 2) }} {
		if n, e := f(); n != 0 || !errors.Is(e, ErrLimit) {
			t.Fatal(n, e)
		}
	}
	if n, e := productScale(^uint64(0), 0); e != nil || n != 0 {
		t.Fatal(n, e)
	}
	if n, e := productAdd(^uint64(0), 0); e != nil || n != ^uint64(0) {
		t.Fatal(n, e)
	}
}
func TestMulHugeSparseShape(t *testing.T) {
	const width = 4 << 20
	l := Limits{MaxDimension: width, MaxNNZ: 2, MaxStorageBytes: productScratchBytes + 16 + 24 + 32, MaxWork: 100}
	a := diagonal(t, []float64{2})
	b, e := NewCSRWithLimits(1, width, []int{0, 2}, []int{0, width - 1}, []float64{3, 4}, l)
	if e != nil {
		t.Fatal(e)
	}
	if out, e := a.Mul(b); out != nil || !errors.Is(e, ErrLimit) {
		t.Fatal(out, e)
	}
	out, e := a.MulWithLimits(b, l)
	if e != nil {
		t.Fatal(e)
	}
	r, c := out.Dims()
	p, ci, v := out.Data()
	if r != 1 || c != width || !reflect.DeepEqual(p, []int{0, 2}) || !reflect.DeepEqual(ci, []int{0, width - 1}) {
		t.Fatal(r, c, p, ci)
	}
	equalVec(t, v, []float64{6, 8})
}
func TestMulIndependentReferences(t *testing.T) {
	type mat struct {
		Rows, Cols     int
		Rowptr, Colind []int
		Values         []float64
	}
	var data struct {
		Numpy, Scipy string
		Cases        []struct {
			Name, Oracle, Error string
			A, B, Expected      mat
			Bits, Rationals     []string
		}
	}
	raw, e := os.ReadFile("product.json")
	if e != nil {
		t.Fatal(e)
	}
	if e = json.Unmarshal(raw, &data); e != nil {
		t.Fatal(e)
	}
	if len(data.Cases) < 12 || data.Scipy == "" {
		t.Fatal("reference provenance")
	}
	for _, c := range data.Cases {
		t.Run(c.Name, func(t *testing.T) {
			a := must(t, c.A.Rows, c.A.Cols, c.A.Rowptr, c.A.Colind, c.A.Values)
			b := must(t, c.B.Rows, c.B.Cols, c.B.Rowptr, c.B.Colind, c.B.Values)
			got, e := a.Mul(b)
			if c.Error == "range" {
				if got != nil || !errors.Is(e, ErrRange) {
					t.Fatal(got, e)
				}
				return
			}
			if e != nil {
				t.Fatal(e)
			}
			r, cols := got.Dims()
			p, ci, v := got.Data()
			if r != c.Expected.Rows || cols != c.Expected.Cols || !reflect.DeepEqual(p, c.Expected.Rowptr) || !slices.Equal(ci, c.Expected.Colind) {
				t.Fatal("structure", r, cols, p, ci, c.Expected)
			}
			if c.Oracle == "fraction" {
				if len(c.Bits) != len(v) || len(c.Rationals) != len(v) {
					t.Fatal("fraction data")
				}
				for i, x := range v {
					bits, e := strconv.ParseUint(c.Bits[i], 16, 64)
					if e != nil || math.Float64bits(x) != bits {
						t.Fatalf("[%d] bits %016x want %s", i, math.Float64bits(x), c.Bits[i])
					}
				}
			} else {
				closeVec(t, v, c.Expected.Values, 2e-13)
			}
		})
	}
}
func TestMulCancellationNoPartial(t *testing.T) {
	a := must(t, 2, 3, []int{0, 3, 5}, []int{0, 1, 2, 0, 2}, []float64{1, 2, 3, 4, 5})
	b := must(t, 3, 2, []int{0, 2, 3, 5}, []int{0, 1, 0, 0, 1}, []float64{1, 2, 3, 4, 5})
	if out, e := a.MulContext(nil, b, Limits{}); out != nil || !errors.Is(e, ErrOptions) {
		t.Fatal(out, e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	probe := &countedContext{Context: ctx, cancel: cancel, after: math.MaxInt64}
	if _, e := a.MulContext(probe, b, Limits{}); e != nil {
		t.Fatal(e)
	}
	cancel()
	total := probe.calls.Load()
	for after := int64(1); after <= total; after++ {
		ctx, cancel = context.WithCancel(context.Background())
		c := &countedContext{Context: ctx, cancel: cancel, after: after}
		out, e := a.MulContext(c, b, Limits{})
		cancel()
		if out != nil || !errors.Is(e, context.Canceled) {
			t.Fatalf("checkpoint %d/%d: %v %v", after, total, out, e)
		}
	}
	ctx, cancel = context.WithCancel(context.Background())
	cancel()
	if out, e := a.MulContext(ctx, b, Limits{}); out != nil || !errors.Is(e, context.Canceled) {
		t.Fatal(out, e)
	}
}
func TestMulConcurrentInputs(t *testing.T) {
	a := tridiagonal(t, 16)
	want, e := a.Mul(a)
	if e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 8; j++ {
				out, e := a.Mul(a)
				if e != nil {
					t.Error(e)
					return
				}
				matrixEqual(t, out, want)
				p, c, v := out.Data()
				p[0] = 99
				c[0] = 99
				v[0] = 99
			}
		}()
	}
	wg.Wait()
}
func TestMulDotRangeAndAllOrNothing(t *testing.T) {
	for _, scale := range []float64{1e-300, 1e300} {
		a := diagonal(t, []float64{scale})
		b := diagonal(t, []float64{1 / scale})
		out, e := a.Mul(b)
		if e != nil {
			t.Fatal(e)
		}
		v, e := out.At(0, 0)
		if e != nil || math.Abs(v-1) > 2e-16 {
			t.Fatal(v, e)
		}
	}
	a := diagonal(t, []float64{1, math.MaxFloat64})
	b := diagonal(t, []float64{1, 2})
	if out, e := a.Mul(b); out != nil || !errors.Is(e, ErrRange) {
		t.Fatal(out, e)
	}
	if out, e := a.MulWithLimits(b, Limits{MaxWork: 1}); out != nil || !errors.Is(e, ErrLimit) {
		t.Fatal(out, e)
	}
}
