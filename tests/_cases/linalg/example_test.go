package linalg_test

import (
	"fmt"

	"gomath/linalg"
)

func ExampleFactorize() {
	a, err := linalg.NewDense(2, 2, []float64{0, 2, 1, 3})
	if err != nil {
		panic(err)
	}
	f, err := linalg.Factorize(a)
	if err != nil {
		panic(err)
	}
	x, err := f.Solve([]float64{4, 7})
	if err != nil {
		panic(err)
	}
	fmt.Println(x)
	det, err := f.Det()
	if err != nil {
		panic(err)
	}
	fmt.Println(det)
	// Output:
	// [1 2]
	// -2
}

func ExampleFactorizeQR() {
	a, err := linalg.NewDense(3, 2, []float64{1, 0, 1, 1, 1, 2})
	if err != nil {
		panic(err)
	}
	f, err := linalg.FactorizeQR(a)
	if err != nil {
		panic(err)
	}
	x, norm, err := f.Solve([]float64{1, 2, 2})
	if err != nil {
		panic(err)
	}
	fmt.Printf("intercept %.6f, slope %.6f, residual %.6f\n", x[0], x[1], norm)
	// Output:
	// intercept 1.166667, slope 0.500000, residual 0.408248
}

func ExampleFactorizeCholesky() {
	a, err := linalg.NewDense(2, 2, []float64{4, 2, 2, 3})
	if err != nil {
		panic(err)
	}
	f, err := linalg.FactorizeCholesky(a)
	if err != nil {
		panic(err)
	}
	x, err := f.Solve([]float64{2, -1})
	if err != nil {
		panic(err)
	}
	fmt.Printf("[%.6f %.6f]\n", x[0], x[1])
	// Output:
	// [1.000000 -1.000000]
}
