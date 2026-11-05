package linalg_test

import (
	"math/rand/v2"
	"testing"

	"gomath/linalg"
)

func svdEconomyBenchFixture(b *testing.B) *linalg.Dense {
	b.Helper()
	const m, n, rank = 16, 32, 8
	rng := rand.New(rand.NewPCG(572, 431))
	left, right := make([]float64, m*rank), make([]float64, rank*n)
	for i := range left {
		left[i] = float64(rng.IntN(5) - 2)
	}
	for i := range right {
		right[i] = float64(rng.IntN(5) - 2)
	}
	return dense(b, m, n, svdTestProduct(left, m, rank, right, n)...)
}

func BenchmarkSVDEconomyWideRankDeficient(b *testing.B) {
	a := svdEconomyBenchFixture(b)
	b.ReportAllocs()
	for b.Loop() {
		var err error
		benchSVD, err = linalg.FactorizeSVD(a)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSVDPseudoInverseReuse(b *testing.B) {
	f, err := linalg.FactorizeSVD(svdEconomyBenchFixture(b))
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		benchDense, err = f.PseudoInverse()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSVDSolveMatrixReuse(b *testing.B) {
	f, err := linalg.FactorizeSVD(svdEconomyBenchFixture(b))
	if err != nil {
		b.Fatal(err)
	}
	data := make([]float64, 16*4)
	for i := range data {
		data[i] = float64(i%11 - 5)
	}
	rhs := dense(b, 16, 4, data...)
	b.ReportAllocs()
	for b.Loop() {
		benchDense, benchNorms, err = f.SolveMatrix(rhs)
		if err != nil {
			b.Fatal(err)
		}
	}
}
