package linalg_test

import (
	"errors"
	"testing"

	"gomath/linalg"
)

func FuzzSchur(f *testing.F) {
	f.Add([]byte{2, 1, 2, 3, 4, 5, 6, 7, 8, 9})
	f.Add([]byte{1, 0, 128, 127, 0})
	f.Add([]byte{7})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) == 0 || len(data) > 65 {
			return
		}
		n := int(data[0]%8) + 1
		entries := make([]float64, n*n)
		for i := range entries {
			if i+1 < len(data) {
				entries[i] = float64(int8(data[i+1])) / 8
			}
		}
		a, err := linalg.NewDense(n, n, entries)
		if err != nil {
			t.Fatal(err)
		}
		opt := linalg.DefaultSchurOptions()
		opt.MaxIterations = 256
		opt.MaxWork = 2 << 20
		decomposition, err := linalg.FactorizeSchurWithOptions(a, opt)
		if err != nil {
			if decomposition != nil || !(errors.Is(err, linalg.ErrConvergence) || errors.Is(err, linalg.ErrSchurLimit) || errors.Is(err, linalg.ErrRange)) {
				t.Fatal("unexpected error", decomposition, err)
			}
			return
		}
		checkSchur(t, a, decomposition)
	})
}
