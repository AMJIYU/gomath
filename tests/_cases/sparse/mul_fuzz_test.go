package sparse

import (
	"errors"
	"math"
	"math/big"
	"testing"
)

// This oracle uses rational arithmetic and dense indexing only inside tests;
// production uses the independently implemented fixed-precision sparse heap.
func FuzzSpGEMM(f *testing.F) {
	f.Add([]byte{2, 3, 4, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10})
	f.Add([]byte{1, 1, 1, 255, 127, 1, 128})
	f.Add([]byte{0, 0, 0})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) < 3 {
			return
		}
		r, k, c := int(data[0]%6), int(data[1]%7), int(data[2]%6)
		offset := 3
		next := func() byte { v := data[offset%len(data)]; offset++; return v }
		build := func(rows, cols int) *CSR {
			p := make([]int, rows+1)
			var ci []int
			var v []float64
			for i := 0; i < rows; i++ {
				for j := 0; j < cols; j++ {
					code := next()
					exponent := next()
					if code&3 == 0 {
						continue
					}
					value := math.Ldexp(float64(int(int8(code))>>3), 8*int(int8(exponent)))
					ci = append(ci, j)
					v = append(v, value)
				}
				p[i+1] = len(v)
			}
			return must(t, rows, cols, p, ci, v)
		}
		a, b := build(r, k), build(k, c)
		expected := make([]float64, r*c)
		expectedNNZ := 0
		rangeError := false
		x, y, product, sum := new(big.Rat), new(big.Rat), new(big.Rat), new(big.Rat)
		for i := 0; i < r; i++ {
			for j := 0; j < c; j++ {
				sum.SetInt64(0)
				for inner := 0; inner < k; inner++ {
					av, e := a.At(i, inner)
					if e != nil {
						t.Fatal(e)
					}
					bv, e := b.At(inner, j)
					if e != nil {
						t.Fatal(e)
					}
					x.SetFloat64(av)
					y.SetFloat64(bv)
					product.Mul(x, y)
					sum.Add(sum, product)
				}
				value, _ := sum.Float64()
				if !finite(value) || (value == 0 && sum.Sign() != 0) {
					rangeError = true
				}
				expected[i*c+j] = value
				if value != 0 {
					expectedNNZ++
				}
			}
		}
		got, e := a.Mul(b)
		if rangeError {
			if got != nil || !errors.Is(e, ErrRange) {
				t.Fatalf("want range, got %v %v", got, e)
			}
			return
		}
		if e != nil {
			t.Fatal(e)
		}
		gr, gc := got.Dims()
		if gr != r || gc != c || got.NNZ() != expectedNNZ {
			t.Fatal("shape/count", gr, gc, got.NNZ(), expectedNNZ)
		}
		for i := 0; i < r; i++ {
			for j := 0; j < c; j++ {
				value, e := got.At(i, j)
				want := expected[i*c+j]
				if e != nil || math.Float64bits(value) != math.Float64bits(want) {
					t.Fatalf("[%d,%d] %.17g != %.17g: %v", i, j, value, want, e)
				}
			}
		}
		p, ci, v := got.Data()
		if _, e = NewCSR(r, c, p, ci, v); e != nil {
			t.Fatal("noncanonical result", e)
		}
		for _, value := range v {
			if value == 0 {
				t.Fatal("stored zero")
			}
		}
	})
}
