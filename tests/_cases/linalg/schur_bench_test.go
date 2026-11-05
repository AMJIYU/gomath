package linalg_test

import (
	"fmt"
	"math/rand/v2"
	"testing"

	"gomath/linalg"
)

var benchSchur *linalg.Schur

func BenchmarkSchur(b *testing.B) {
	for _, n := range []int{8, 32, 64} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			rng := rand.New(rand.NewPCG(9639, uint64(n)))
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
				benchSchur, err = linalg.FactorizeSchur(a)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
