package sparse

import (
	"context"
	"fmt"
	"testing"
)

func biConvection(t testing.TB, n int) *CSR {
	t.Helper()
	p := make([]int, n+1)
	var ci []int
	var values []float64
	for i := 0; i < n; i++ {
		if i > 0 {
			ci = append(ci, i-1)
			values = append(values, -1.25)
		}
		ci = append(ci, i)
		values = append(values, 2.1)
		if i+1 < n {
			ci = append(ci, i+1)
			values = append(values, -.75)
		}
		p[i+1] = len(ci)
	}
	return must(t, n, n, p, ci, values)
}

func BenchmarkBiCGSTAB(b *testing.B) {
	for _, n := range []int{32, 128} {
		for _, pre := range []Preconditioner{NoPreconditioner, Jacobi} {
			b.Run(fmt.Sprintf("Convection_n%d_pre%d", n, pre), func(b *testing.B) {
				a := biConvection(b, n)
				rhs := make([]float64, n)
				rhs[0] = 1
				o := DefaultBiCGSTABOptions()
				o.Preconditioner = pre
				o.Limits.MaxWork = 1 << 30
				b.ReportAllocs()
				for b.Loop() {
					r, e := SolveBiCGSTAB(context.Background(), a, rhs, o)
					if e != nil {
						b.Fatal(e)
					}
					b.ReportMetric(float64(r.Iterations()), "iterations/op")
				}
			})
		}
	}
}
