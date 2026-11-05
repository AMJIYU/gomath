package linalg_test

import (
	"fmt"
	"testing"

	"gomath/linalg"
)

var (
	benchQR       *linalg.QR
	benchCholesky *linalg.Cholesky
	benchNorms    []float64
)

func BenchmarkQRFactorize(b *testing.B) {
	for _, n := range []int{8, 32, 128} {
		b.Run(fmt.Sprintf("M=%d/N=%d", 2*n, n), func(b *testing.B) {
			a := rectangular(b, 2*n, n)
			b.ReportAllocs()
			for b.Loop() {
				var err error
				benchQR, err = linalg.FactorizeQR(a)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkQRSolveReuse(b *testing.B) {
	for _, n := range []int{8, 32, 128} {
		b.Run(fmt.Sprintf("M=%d/N=%d/RHS=1", 2*n, n), func(b *testing.B) {
			f, err := linalg.FactorizeQR(rectangular(b, 2*n, n))
			if err != nil {
				b.Fatal(err)
			}
			rhs := make([]float64, 2*n)
			for i := range rhs {
				rhs[i] = float64(i%17 - 8)
			}
			b.ReportAllocs()
			for b.Loop() {
				benchVector, benchScalar, err = f.Solve(rhs)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkQRSolveMatrixReuse(b *testing.B) {
	for _, n := range []int{8, 32, 128} {
		b.Run(fmt.Sprintf("M=%d/N=%d/RHS=8", 2*n, n), func(b *testing.B) {
			f, err := linalg.FactorizeQR(rectangular(b, 2*n, n))
			if err != nil {
				b.Fatal(err)
			}
			data := make([]float64, 2*n*8)
			for i := range data {
				data[i] = float64(i%17 - 8)
			}
			rhs := dense(b, 2*n, 8, data...)
			b.ReportAllocs()
			for b.Loop() {
				benchDense, benchNorms, err = f.SolveMatrix(rhs)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkCholeskyFactorize(b *testing.B) {
	for _, n := range []int{8, 32, 128} {
		b.Run(fmt.Sprintf("N=%d", n), func(b *testing.B) {
			a := spd(b, n)
			b.ReportAllocs()
			for b.Loop() {
				var err error
				benchCholesky, err = linalg.FactorizeCholesky(a)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkCholeskySolveReuse(b *testing.B) {
	for _, n := range []int{8, 32, 128} {
		b.Run(fmt.Sprintf("N=%d/RHS=1", n), func(b *testing.B) {
			f, err := linalg.FactorizeCholesky(spd(b, n))
			if err != nil {
				b.Fatal(err)
			}
			rhs := make([]float64, n)
			for i := range rhs {
				rhs[i] = float64(i%17 - 8)
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

func BenchmarkCholeskySolveMatrixReuse(b *testing.B) {
	for _, n := range []int{8, 32, 128} {
		b.Run(fmt.Sprintf("N=%d/RHS=8", n), func(b *testing.B) {
			f, err := linalg.FactorizeCholesky(spd(b, n))
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
