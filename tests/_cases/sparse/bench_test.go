package sparse

import (
	"context"
	"testing"
)

func tridiagonal(t testing.TB, n int) *CSR {
	p := make([]int, n+1)
	var c []int
	var v []float64
	for i := 0; i < n; i++ {
		if i > 0 {
			c = append(c, i-1)
			v = append(v, -1)
		}
		c = append(c, i)
		v = append(v, 4)
		if i+1 < n {
			c = append(c, i+1)
			v = append(v, -1)
		}
		p[i+1] = len(v)
	}
	return must(t, n, n, p, c, v)
}
func BenchmarkCSR(b *testing.B) {
	a := tridiagonal(b, 1024)
	x := make([]float64, 1024)
	for i := range x {
		x[i] = float64(i % 7)
	}
	b.Run("Transpose_n1024_nnz3070", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, e := a.Transpose(); e != nil {
				b.Fatal(e)
			}
		}
	})
	b.Run("Add_n1024_nnz3070", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, e := a.Add(a); e != nil {
				b.Fatal(e)
			}
		}
	})
	b.Run("MulVec_n1024_nnz3070", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, e := a.MulVec(x); e != nil {
				b.Fatal(e)
			}
		}
	})
}
func BenchmarkCG(b *testing.B) {
	a := tridiagonal(b, 64)
	rhs := make([]float64, 64)
	for i := range rhs {
		rhs[i] = 1
	}
	o := DefaultCGOptions()
	o.Preconditioner = Jacobi
	b.ReportAllocs()
	for b.Loop() {
		r, e := SolveCG(context.Background(), a, rhs, o)
		if e != nil {
			b.Fatal(e)
		}
		b.ReportMetric(float64(r.Iterations()), "iterations/op")
	}
}
