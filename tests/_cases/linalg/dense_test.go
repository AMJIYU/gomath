package linalg_test

import (
	"errors"
	"math"
	"slices"
	"testing"

	"gomath/linalg"
)

func dense(t testing.TB, rows, cols int, values ...float64) *linalg.Dense {
	t.Helper()
	a, err := linalg.NewDense(rows, cols, values)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func equalData(t *testing.T, a *linalg.Dense, rows, cols int, want []float64) {
	t.Helper()
	r, c := a.Dimensions()
	if r != rows || c != cols || !slices.Equal(a.Data(), want) {
		t.Fatalf("got %dx%d %v, want %dx%d %v", r, c, a.Data(), rows, cols, want)
	}
}

func TestOwnershipAndAccess(t *testing.T) {
	source := []float64{1, 2, 3, 4, 5, 6}
	a, err := linalg.NewDense(2, 3, source)
	if err != nil {
		t.Fatal(err)
	}
	source[0] = math.NaN()
	copyOfData := a.Data()
	copyOfData[1] = math.Inf(1)
	equalData(t, a, 2, 3, []float64{1, 2, 3, 4, 5, 6})
	for i := range 2 {
		for j := range 3 {
			v, err := a.At(i, j)
			if err != nil || v != float64(i*3+j+1) {
				t.Fatalf("At(%d,%d) = %g, %v", i, j, v, err)
			}
		}
	}
	for _, index := range [][2]int{{-1, 0}, {0, -1}, {2, 0}, {0, 3}} {
		if _, err := a.At(index[0], index[1]); !errors.Is(err, linalg.ErrDimension) {
			t.Fatalf("invalid index %v: %v", index, err)
		}
	}
	transposed, err := a.Transpose()
	if err != nil {
		t.Fatal(err)
	}
	equalData(t, transposed, 3, 2, []float64{1, 4, 2, 5, 3, 6})
	double, err := transposed.Transpose()
	if err != nil {
		t.Fatal(err)
	}
	equalData(t, double, 2, 3, a.Data())
}

func TestRectangularArithmetic(t *testing.T) {
	var (
		a = dense(t, 2, 3, 1, 2, 3, 4, 5, 6)
		b = dense(t, 2, 3, -1, 1, 2, -3, 2, 1)
		c = dense(t, 3, 2, 7, 8, 9, 10, 11, 12)
	)
	sum, err := a.Add(b)
	if err != nil {
		t.Fatal(err)
	}
	equalData(t, sum, 2, 3, []float64{0, 3, 5, 1, 7, 7})
	diff, err := a.Sub(b)
	if err != nil {
		t.Fatal(err)
	}
	equalData(t, diff, 2, 3, []float64{2, 1, 1, 7, 3, 5})
	product, err := a.Mul(c)
	if err != nil {
		t.Fatal(err)
	}
	equalData(t, product, 2, 2, []float64{58, 64, 139, 154})
	x := []float64{2, -1, 3}
	y, err := a.MulVec(x)
	if err != nil || !slices.Equal(y, []float64{9, 21}) {
		t.Fatalf("MulVec = %v, %v", y, err)
	}
	y[0] = 99
	if !slices.Equal(x, []float64{2, -1, 3}) {
		t.Fatal("vector input modified")
	}
	equalData(t, a, 2, 3, []float64{1, 2, 3, 4, 5, 6})
	equalData(t, b, 2, 3, []float64{-1, 1, 2, -3, 2, 1})
	equalData(t, c, 3, 2, []float64{7, 8, 9, 10, 11, 12})
}

func TestEmptyMatrixShapes(t *testing.T) {
	var zero linalg.Dense
	equalData(t, &zero, 0, 0, nil)
	for _, shape := range [][2]int{{0, 0}, {0, 3}, {2, 0}} {
		a, err := linalg.Zeros(shape[0], shape[1])
		if err != nil {
			t.Fatal(err)
		}
		at, err := a.Transpose()
		if err != nil {
			t.Fatal(err)
		}
		equalData(t, at, shape[1], shape[0], nil)
		sum, err := a.Add(a)
		if err != nil {
			t.Fatal(err)
		}
		equalData(t, sum, shape[0], shape[1], nil)
	}
	a, _ := linalg.Zeros(2, 0)
	b, _ := linalg.Zeros(0, 3)
	c, err := a.Mul(b)
	if err != nil {
		t.Fatal(err)
	}
	equalData(t, c, 2, 3, make([]float64, 6))
	x, err := a.MulVec(nil)
	if err != nil || !slices.Equal(x, []float64{0, 0}) {
		t.Fatalf("empty dot products: %v, %v", x, err)
	}
	maxInt := int(^uint(0) >> 1)
	hugeEmpty, err := linalg.Zeros(maxInt, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := hugeEmpty.Transpose(); err != nil {
		t.Fatal(err)
	}
	if _, err := hugeEmpty.MulVec(nil); !errors.Is(err, linalg.ErrDimension) {
		t.Fatalf("vector allocation overflow: %v", err)
	}
	if _, err := hugeEmpty.Mul(b); !errors.Is(err, linalg.ErrDimension) {
		t.Fatalf("product allocation overflow: %v", err)
	}
}

func TestInvalidMatrices(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	for _, tc := range []struct {
		rows, cols int
		data       []float64
		want       error
	}{
		{-1, 2, nil, linalg.ErrDimension},
		{2, -1, nil, linalg.ErrDimension},
		{2, 2, []float64{1}, linalg.ErrDimension},
		{0, 0, []float64{1}, linalg.ErrDimension},
		{maxInt, 2, nil, linalg.ErrDimension},
		{1, maxInt, nil, linalg.ErrDimension},
		{1, 1, []float64{math.NaN()}, linalg.ErrNonFinite},
		{1, 1, []float64{math.Inf(1)}, linalg.ErrNonFinite},
		{1, 1, []float64{math.Inf(-1)}, linalg.ErrNonFinite},
	} {
		if a, err := linalg.NewDense(tc.rows, tc.cols, tc.data); a != nil || !errors.Is(err, tc.want) {
			t.Fatalf("NewDense(%d,%d,%v) = %v, %v", tc.rows, tc.cols, tc.data, a, err)
		}
	}
	if _, err := linalg.Identity(-1); !errors.Is(err, linalg.ErrDimension) {
		t.Fatal(err)
	}
	if _, err := linalg.Zeros(maxInt, maxInt); !errors.Is(err, linalg.ErrDimension) {
		t.Fatal(err)
	}
	a := dense(t, 2, 3, 1, 2, 3, 4, 5, 6)
	b := dense(t, 3, 2, 1, 2, 3, 4, 5, 6)
	for _, op := range []func() (*linalg.Dense, error){
		func() (*linalg.Dense, error) { return a.Add(b) },
		func() (*linalg.Dense, error) { return a.Sub(b) },
		func() (*linalg.Dense, error) { return a.Mul(a) },
	} {
		if out, err := op(); out != nil || !errors.Is(err, linalg.ErrDimension) {
			t.Fatalf("shape mismatch = %v, %v", out, err)
		}
	}
	if _, err := a.MulVec([]float64{1}); !errors.Is(err, linalg.ErrDimension) {
		t.Fatal(err)
	}
	for _, bad := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if out, err := a.MulVec([]float64{1, bad, 1}); out != nil || !errors.Is(err, linalg.ErrNonFinite) {
			t.Fatalf("nonfinite vector = %v, %v", out, err)
		}
	}
}

func TestArithmeticOverflow(t *testing.T) {
	var (
		a   = dense(t, 1, 1, math.MaxFloat64)
		neg = dense(t, 1, 1, -math.MaxFloat64)
		two = dense(t, 1, 1, 2)
	)
	for _, op := range []func() (*linalg.Dense, error){
		func() (*linalg.Dense, error) { return a.Add(a) },
		func() (*linalg.Dense, error) { return a.Sub(neg) },
		func() (*linalg.Dense, error) { return a.Mul(two) },
	} {
		if out, err := op(); out != nil || !errors.Is(err, linalg.ErrNonFinite) {
			t.Fatalf("overflow = %v, %v", out, err)
		}
	}
	if out, err := a.MulVec([]float64{2}); out != nil || !errors.Is(err, linalg.ErrNonFinite) {
		t.Fatalf("vector overflow = %v, %v", out, err)
	}
}

func TestNilMatrices(t *testing.T) {
	var nilMatrix *linalg.Dense
	a := dense(t, 1, 1, 1)
	for _, op := range []func() (*linalg.Dense, error){
		nilMatrix.Transpose,
		func() (*linalg.Dense, error) { return nilMatrix.Add(a) },
		func() (*linalg.Dense, error) { return a.Sub(nil) },
		func() (*linalg.Dense, error) { return nilMatrix.Mul(a) },
		func() (*linalg.Dense, error) { return a.Mul(nil) },
	} {
		if out, err := op(); out != nil || !errors.Is(err, linalg.ErrInvalid) {
			t.Fatalf("nil matrix = %v, %v", out, err)
		}
	}
	if _, err := nilMatrix.At(0, 0); !errors.Is(err, linalg.ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := nilMatrix.MulVec(nil); !errors.Is(err, linalg.ErrInvalid) {
		t.Fatal(err)
	}
	if r, c := nilMatrix.Dimensions(); r != 0 || c != 0 || nilMatrix.Data() != nil {
		t.Fatal("nil introspection")
	}
}
