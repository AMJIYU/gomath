package linalg_test

import (
	"fmt"
	"testing"

	"gomath/linalg"
)

var (
	benchDense  *linalg.Dense
	benchLU     *linalg.LU
	benchVector []float64
	benchScalar float64
)

// Fixture generation is outside timing. All result/workspace allocation and
// finite-result validation in the kernel under test are included in timing.
func BenchmarkDenseMul(b *testing.B) {
	for _, n := range []int{8, 32, 128, 256} {
		b.Run(fmt.Sprintf("N=%d", n), func(b *testing.B) {
			a := wellConditioned(b, n)
			b.ReportAllocs()
			for b.Loop() {
				var err error
				benchDense, err = a.Mul(a)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkLUFactorize(b *testing.B) {
	for _, n := range []int{8, 32, 128, 256} {
		b.Run(fmt.Sprintf("N=%d", n), func(b *testing.B) {
			a := wellConditioned(b, n)
			b.ReportAllocs()
			for b.Loop() {
				var err error
				benchLU, err = linalg.Factorize(a)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkLUSolveReuse(b *testing.B) {
	for _, n := range []int{8, 32, 128, 256} {
		b.Run(fmt.Sprintf("N=%d/RHS=1", n), func(b *testing.B) {
			f, err := linalg.Factorize(wellConditioned(b, n))
			if err != nil {
				b.Fatal(err)
			}
			rhs := make([]float64, n)
			for i := range rhs {
				rhs[i] = float64(i + 1)
			}
			b.ReportAllocs()
			for b.Loop() {
				benchVector, err = f.Solve(rhs)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkLUSolveMatrixReuse(b *testing.B) {
	for _, n := range []int{8, 32, 128, 256} {
		b.Run(fmt.Sprintf("N=%d/RHS=8", n), func(b *testing.B) {
			f, err := linalg.Factorize(wellConditioned(b, n))
			if err != nil {
				b.Fatal(err)
			}
			data := make([]float64, n*8)
			for i := range data {
				data[i] = float64(i%17 - 8)
			}
			rhs := dense(b, n, 8, data...)
			b.ReportAllocs()
			for b.Loop() {
				benchDense, err = f.SolveMatrix(rhs)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkLUInverseReuse(b *testing.B) {
	for _, n := range []int{8, 32, 128} {
		b.Run(fmt.Sprintf("N=%d", n), func(b *testing.B) {
			f, err := linalg.Factorize(wellConditioned(b, n))
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			for b.Loop() {
				benchDense, err = f.Inverse()
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkLUDetReuse(b *testing.B) {
	f, err := linalg.Factorize(wellConditioned(b, 32))
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		benchScalar, err = f.Det()
		if err != nil {
			b.Fatal(err)
		}
	}
}
