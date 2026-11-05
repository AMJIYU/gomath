package sparse

import (
	"context"
	"errors"
	"math"
	"testing"
)

func FuzzCSR(f *testing.F) {
	f.Add([]byte{3, 4, 1, 0, 0, 3, 1, 2, 2, 2, 3, 3, 4, 4, 5, 1})
	f.Add([]byte{0, 0})
	f.Add([]byte{1, 1, 0, 1, 0, 0, 0, 0})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) < 2 {
			return
		}
		r, c := int(data[0]%9), int(data[1]%9)
		p := make([]int, r+1)
		var ci []int
		var v []float64
		dense := make([]float64, r*c)
		k := 2
		for i := 0; i < r; i++ {
			for j := 0; j < c; j++ {
				if k >= len(data) {
					break
				}
				b := data[k]
				k++
				if b&1 != 0 {
					ci = append(ci, j)
					v = append(v, float64(int8(b)))
					dense[i*c+j] = float64(int8(b))
				}
			}
			p[i+1] = len(v)
		}
		a, e := NewCSR(r, c, p, ci, v)
		if e != nil {
			t.Fatal(e)
		}
		at, e := a.Transpose()
		if e != nil {
			t.Fatal(e)
		}
		x := make([]float64, c)
		for i := range x {
			x[i] = float64(i - 3)
		}
		out, e := a.MulVec(x)
		if e != nil {
			t.Fatal(e)
		}
		for i := 0; i < r; i++ {
			want := 0.
			for j := 0; j < c; j++ {
				want += dense[i*c+j] * x[j]
				got, e := at.At(j, i)
				if e != nil || got != dense[i*c+j] {
					t.Fatal("transpose", got, e)
				}
			}
			if out[i] != want {
				t.Fatal("product", out[i], want)
			}
		}
		// Mutate arbitrary pointers/columns; success must still obey all invariants.
		if len(data) > 2 && len(p) > 1 {
			p[int(data[2])%len(p)] = int(int8(data[len(data)-1]))
		}
		if len(ci) > 0 && len(data) > 3 {
			ci[int(data[3])%len(ci)] = int(int8(data[2]))
		}
		b, e := NewCSR(r, c, p, ci, v)
		if e != nil {
			if b != nil {
				t.Fatal("partial")
			}
			return
		}
		rp, col, val := b.Data()
		if rp[0] != 0 || rp[r] != len(val) {
			t.Fatal("row pointers")
		}
		for i := 0; i < r; i++ {
			prev := -1
			for j := rp[i]; j < rp[i+1]; j++ {
				if col[j] <= prev || col[j] < 0 || col[j] >= c {
					t.Fatal("column")
				}
				prev = col[j]
			}
		}
	})
}
func FuzzCGDiagonal(f *testing.F) {
	f.Add([]byte{1, 2, 3, 4, 20, 40, 8, 2})
	f.Add([]byte{0})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) == 0 {
			return
		}
		n := 1 + int(data[0]%12)
		d, b, want := make([]float64, n), make([]float64, n), make([]float64, n)
		for i := range d {
			v := data[i%len(data)]
			d[i] = math.Ldexp(1, int(v%81)-40)
			want[i] = float64(int(v%31) - 15)
			b[i] = d[i] * want[i]
		}
		a := diagonal(t, d)
		o := DefaultCGOptions()
		o.Preconditioner = Jacobi
		r, e := SolveCG(context.Background(), a, b, o)
		if e != nil {
			t.Fatal(e)
		}
		closeVec(t, r.Solution(), want, 2e-14)
	})
}
func FuzzMulVecRange(f *testing.F) {
	f.Add(uint64(0x7fefffffffffffff), uint64(0x7fefffffffffffff))
	f.Add(uint64(1), uint64(1))
	f.Fuzz(func(t *testing.T, u, v uint64) {
		x, y := math.Float64frombits(u), math.Float64frombits(v)
		if !finite(x) || !finite(y) {
			return
		}
		a := diagonal(t, []float64{x})
		out, e := a.MulVec([]float64{y})
		if e != nil {
			if out != nil || !errors.Is(e, ErrRange) {
				t.Fatal(out, e)
			}
			return
		}
		if len(out) != 1 || !finite(out[0]) {
			t.Fatal(out)
		}
		// Hardware multiplication is correctly rounded, independently of big.Float.
		if out[0] != x*y {
			t.Fatal(out[0], x*y)
		}
	})
}

// Repeated entries and exact cancellation are important independently of the
// random-diagonal family: this includes the parent scene's Poisson workload.
func FuzzCGPoisson(f *testing.F) {
	f.Add(uint8(30), int16(1))
	f.Add(uint8(31), int16(-1))
	f.Add(uint8(0), int16(0))
	f.Fuzz(func(t *testing.T, size uint8, amplitude int16) {
		n := 1 + int(size%40)
		rhs := float64(amplitude) / 1024
		p := make([]int, n+1)
		var c []int
		var v []float64
		b := make([]float64, n)
		for i := range b {
			if i > 0 {
				c = append(c, i-1)
				v = append(v, -1)
			}
			c = append(c, i)
			v = append(v, 2)
			if i+1 < n {
				c = append(c, i+1)
				v = append(v, -1)
			}
			p[i+1] = len(v)
			b[i] = rhs
		}
		a := must(t, n, n, p, c, v)
		o := DefaultCGOptions()
		if size&1 == 0 {
			o.Preconditioner = Jacobi
		}
		result, e := SolveCG(context.Background(), a, b, o)
		if e != nil {
			t.Fatal(e)
		}
		x := result.Solution()
		rn, bn := 0., 0.
		for i, xi := range x {
			want := rhs * float64((i+1)*(n-i)) / 2
			if math.Abs(xi-want) > 1e-9*math.Max(1, math.Abs(want)) {
				t.Fatalf("x[%d]=%.17g want %.17g", i, xi, want)
			}
			ax := 2 * xi
			if i > 0 {
				ax -= x[i-1]
			}
			if i+1 < n {
				ax -= x[i+1]
			}
			rn = math.Hypot(rn, b[i]-ax)
			bn = math.Hypot(bn, b[i])
		}
		if rn > o.RelativeTolerance*bn*1.01 {
			t.Fatalf("original residual %g exceeds %g", rn, o.RelativeTolerance*bn)
		}
	})
}
