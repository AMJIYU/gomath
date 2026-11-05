package linalg_test

import (
	"errors"
	"math"
	"testing"

	"gomath/linalg"
)

func FuzzGeneralEigen(f *testing.F) {
	f.Add([]byte{2, 128, 128, 1, 2, 3, 4, 5, 6, 7, 8, 9})
	f.Add([]byte{1, 0, 0, 1, 0, 255, 1})
	f.Add([]byte{3, 255, 255, 0, 1, 0, 0, 255, 0, 1, 0, 0, 0, 0, 1, 255, 0, 0, 0})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) < 3 || len(data) > 67 {
			return
		}
		n := int(data[0]%8) + 1
		exponent := int((uint16(data[1])<<8|uint16(data[2]))%2001) - 1000
		entries := make([]float64, n*n)
		for i := range entries {
			if i+3 < len(data) {
				entries[i] = math.Ldexp(float64(int8(data[i+3]))/128, exponent)
			}
		}
		a, err := linalg.NewDense(n, n, entries)
		if err != nil {
			t.Fatal(err)
		}
		opt := linalg.DefaultEigenOptions()
		opt.Schur.MaxIterations = 256
		opt.MaxWork = 2 << 20
		result, err := linalg.FactorizeEigenWithOptions(a, opt)
		if err != nil {
			if result != nil || !(errors.Is(err, linalg.ErrConvergence) || errors.Is(err, linalg.ErrSchurLimit) || errors.Is(err, linalg.ErrEigenLimit) || errors.Is(err, linalg.ErrRange) || errors.Is(err, linalg.ErrResidual) || errors.Is(err, linalg.ErrReconstruction)) {
				t.Fatal("unexpected error", result, err)
			}
			return
		}
		checkGeneralEigen(t, a, result)
	})
}
