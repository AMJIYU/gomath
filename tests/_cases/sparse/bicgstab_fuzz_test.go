package sparse

import (
	"context"
	"errors"
	"math"
	"math/big"
	"testing"
)

// Construct an exactly known integer solution to strictly diagonally dominant
// nonsymmetric systems. Powers of two scale both A and b without introducing
// reference rounding. Rational residual checks are independent of the solver.
func FuzzBiCGSTAB(f *testing.F) {
	f.Add([]byte{4, 1, 3, 5, 7, 9, 11, 13, 17, 19})
	f.Add([]byte{1, 255, 127})
	f.Add([]byte{6, 0, 0, 0})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) < 3 {
			return
		}
		n := 1 + int(data[0]%6)
		at := 1
		next := func() byte { v := data[at%len(data)]; at++; return v }
		ex := 7 * int(int8(next()))
		p := make([]int, n+1)
		var c []int
		var v []float64
		known := make([]float64, n)
		for i := range known {
			known[i] = float64(int(next()%7) - 3)
		}
		b := make([]float64, n)
		for i := 0; i < n; i++ {
			row := make([]int, n)
			sum := 1
			for j := 0; j < n; j++ {
				if j != i {
					row[j] = int(next()%5) - 2
					sum += int(math.Abs(float64(row[j])))
				}
			}
			row[i] = sum
			for j, value := range row {
				if value != 0 {
					c = append(c, j)
					v = append(v, math.Ldexp(float64(value), ex))
				}
				b[i] += float64(value) * known[j]
			}
			b[i] = math.Ldexp(b[i], ex)
			p[i+1] = len(v)
		}
		a := must(t, n, n, p, c, v)
		o := DefaultBiCGSTABOptions()
		o.RelativeTolerance = 1e-11
		o.MaxIterations = 160
		if next()&1 != 0 {
			o.Preconditioner = Jacobi
		}
		r, e := SolveBiCGSTAB(context.Background(), a, b, o)
		if e != nil {
			if r != nil {
				t.Fatal("partial result")
			}
			if !errors.Is(e, ErrBiCGSTABBreakdown) && !errors.Is(e, ErrBiCGSTABConvergence) {
				t.Fatal(e)
			}
			return
		}
		x := r.Solution()
		closeVec(t, x, known, 1e-8)
		squaredResidual, squaredB := new(big.Rat), new(big.Rat)
		u, w, prod, sum := new(big.Rat), new(big.Rat), new(big.Rat), new(big.Rat)
		for i := 0; i < n; i++ {
			sum.SetFloat64(b[i])
			for j := p[i]; j < p[i+1]; j++ {
				u.SetFloat64(v[j])
				w.SetFloat64(x[c[j]])
				prod.Mul(u, w)
				sum.Sub(sum, prod)
			}
			squaredResidual.Add(squaredResidual, prod.Mul(sum, sum))
			u.SetFloat64(b[i])
			squaredB.Add(squaredB, prod.Mul(u, u))
		}
		tol := new(big.Rat).SetFloat64(o.RelativeTolerance)
		tol.Mul(tol, tol)
		bound := new(big.Rat).Mul(tol, squaredB)
		// The public policy checks rounded binary64 residual components; allow two
		// ulps of residual rounding when comparing with the exact rational oracle.
		bound.Mul(bound, big.NewRat(1000000000001, 1000000000000))
		if squaredResidual.Cmp(bound) > 0 {
			t.Fatalf("rational original residual exceeds tolerance: %s > %s", squaredResidual, bound)
		}
	})
}
