package linalg_test

import (
	"fmt"
	"gomath/linalg"
	"math/rand/v2"
	"testing"
)

var benchComplexMatrix *linalg.ComplexDense
var benchComplexLU *linalg.ComplexLU

func BenchmarkComplex(b *testing.B) {
	for _, n := range []int{8, 32, 64} {
		rng := rand.New(rand.NewPCG(20841, uint64(n)))
		data := make([]complex128, n*n)
		rhs := make([]complex128, n*4)
		for i := range data {
			data[i] = complex(rng.NormFloat64(), rng.NormFloat64())
		}
		for i := range n {
			data[i*n+i] += complex(float64(2*n), 0)
		}
		for i := range rhs {
			rhs[i] = complex(rng.NormFloat64(), rng.NormFloat64())
		}
		a := complexMatrix(b, n, n, data)
		r := complexMatrix(b, n, 4, rhs)
		f, e := linalg.FactorizeComplex(a)
		if e != nil {
			b.Fatal(e)
		}
		b.Run(fmt.Sprintf("Mul/%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				var e error
				benchComplexMatrix, e = a.Mul(a)
				if e != nil {
					b.Fatal(e)
				}
			}
		})
		b.Run(fmt.Sprintf("LU/%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				var e error
				benchComplexLU, e = linalg.FactorizeComplex(a)
				if e != nil {
					b.Fatal(e)
				}
			}
		})
		b.Run(fmt.Sprintf("Solve4/%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				var e error
				benchComplexMatrix, _, e = f.SolveMatrix(r)
				if e != nil {
					b.Fatal(e)
				}
			}
		})
	}
}
