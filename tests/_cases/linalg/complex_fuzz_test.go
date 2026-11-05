package linalg_test

import (
	"errors"
	"gomath/linalg"
	"math"
	"testing"
)

func FuzzComplexLU(f *testing.F) {
	f.Add([]byte{2, 128, 128, 1, 2, 3, 4, 5, 6, 7, 8, 9})
	f.Add([]byte{1, 0, 0, 0, 1, 0, 0, 2})
	f.Add([]byte{0, 255, 255, 127, 127})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) < 3 || len(data) > 131 {
			return
		}
		n := int(data[0]%8) + 1
		exponent := int((uint16(data[1])<<8|uint16(data[2]))%2001) - 1000
		entries := make([]complex128, n*n)
		for i := range entries {
			if 2*i+4 < len(data) {
				entries[i] = complex(math.Ldexp(float64(int8(data[2*i+3]))/128, exponent), math.Ldexp(float64(int8(data[2*i+4]))/128, exponent))
			}
		}
		a := complexMatrix(t, n, n, entries)
		f, e := linalg.FactorizeComplex(a)
		if e != nil {
			if f != nil || !(errors.Is(e, linalg.ErrSingular) || errors.Is(e, linalg.ErrRange) || errors.Is(e, linalg.ErrReconstruction)) {
				t.Fatal(f, e)
			}
			return
		}
		checkComplexLU(t, a, f)
		b := make([]complex128, n)
		for i := range b {
			b[i] = entries[i]
		}
		x, r, e := f.Solve(b)
		if e != nil {
			if x != nil || r != 0 || !(errors.Is(e, linalg.ErrRange) || errors.Is(e, linalg.ErrResidual)) {
				t.Fatal(x, r, e)
			}
			return
		}
		if len(x) != n || !finiteTest(r) || r > 1e-12 {
			t.Fatal(x, r)
		}
	})
}
