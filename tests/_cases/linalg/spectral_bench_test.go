package linalg_test

import (
	"fmt"
	"math/rand/v2"
	"testing"

	"gomath/linalg"
)

var (
	benchEigenSym *linalg.EigenSym
	benchSVD      *linalg.SVD
)

func BenchmarkEigenSymFactorize(b *testing.B) {
	for _, n := range []int{8, 32} {
		b.Run(fmt.Sprintf("N=%d", n), func(b *testing.B) {
			rng := rand.New(rand.NewPCG(uint64(n), 147))
			data := make([]float64, n*n)
			for i := range n {
				for j := 0; j <= i; j++ {
					x := rng.NormFloat64()
					data[i*n+j], data[j*n+i] = x, x
				}
			}
			a := dense(b, n, n, data...)
			b.ReportAllocs()
			for b.Loop() {
				var err error
				benchEigenSym, err = linalg.FactorizeEigenSym(a)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkSVDFactorize(b *testing.B) {
	for _, n := range []int{8, 32} {
		b.Run(fmt.Sprintf("M=%d/N=%d", 2*n, n), func(b *testing.B) {
			rng := rand.New(rand.NewPCG(uint64(n), 541))
			data := make([]float64, 2*n*n)
			for i := range data {
				data[i] = rng.NormFloat64()
			}
			a := dense(b, 2*n, n, data...)
			b.ReportAllocs()
			for b.Loop() {
				var err error
				benchSVD, err = linalg.FactorizeSVD(a)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
