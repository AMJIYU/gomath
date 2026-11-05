package linalg_test

import (
	"fmt"
	"math/rand/v2"
	"testing"

	"gomath/linalg"
)

var benchGeneralEigen *linalg.Eigen

func BenchmarkGeneralEigen(b *testing.B) {
	for _, n := range []int{8, 32, 64} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			rng := rand.New(rand.NewPCG(39521, uint64(n)))
			data := make([]float64, n*n)
			for i := range data {
				data[i] = rng.NormFloat64()
			}
			a, err := linalg.NewDense(n, n, data)
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				benchGeneralEigen, err = linalg.FactorizeEigen(a)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
