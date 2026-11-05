package sparse

import (
	"context"
	"fmt"
	"math"
	"testing"
)

// Exact parent integration workload; keep its RHS, options and tolerances.
func TestCGPoissonDirichlet31(t *testing.T) {
	const n = 31
	h := 1 / float64(n+1)
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
		b[i] = h * h
	}
	a := must(t, n, n, p, c, v)
	o := DefaultCGOptions()
	o.Preconditioner = Jacobi
	result, e := SolveCG(context.Background(), a, b, o)
	if e != nil {
		t.Fatal(e)
	}
	x := result.Solution()
	rn, bn := 0., 0.
	for i, xi := range x {
		coord := float64(i+1) * h
		want := coord * (1 - coord) / 2
		if math.Abs(xi-want) > 2e-12 {
			t.Fatalf("solution[%d]=%.17g want %.17g", i, xi, want)
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
	if rn > o.RelativeTolerance*bn {
		t.Fatalf("original residual %g exceeds %g", rn, o.RelativeTolerance*bn)
	}
	t.Logf("iterations=%d original relative residual=%.17g", result.Iterations(), rn/bn)
}

func TestCGPoissonSizesScalesAndPreconditioners(t *testing.T) {
	for _, n := range []int{1, 2, 3, 4, 7, 15, 30, 31, 32, 33, 63, 64, 65, 95} {
		for _, scale := range []float64{1e-200, 1, 1e200} {
			for _, pre := range []Preconditioner{NoPreconditioner, Jacobi} {
				t.Run(fmt.Sprintf("n%d_scale%g_pre%d", n, scale, pre), func(t *testing.T) {
					h := 1 / float64(n+1)
					p := make([]int, n+1)
					var c []int
					var v []float64
					b := make([]float64, n)
					for i := range b {
						if i > 0 {
							c = append(c, i-1)
							v = append(v, -scale)
						}
						c = append(c, i)
						v = append(v, 2*scale)
						if i+1 < n {
							c = append(c, i+1)
							v = append(v, -scale)
						}
						p[i+1] = len(v)
						b[i] = scale * h * h
					}
					a := must(t, n, n, p, c, v)
					o := DefaultCGOptions()
					o.Preconditioner = pre
					result, e := SolveCG(context.Background(), a, b, o)
					if e != nil {
						t.Fatal(e)
					}
					x := result.Solution()
					rn, bn := 0., 0.
					for i, xi := range x {
						coord := float64(i+1) * h
						want := coord * (1 - coord) / 2
						if math.Abs(xi-want) > 2e-10 {
							t.Fatalf("x[%d]=%.17g want %.17g", i, xi, want)
						}
						// Independently evaluate after removing the uniform scale. Do not use
						// the solver's matrix-vector or accumulator implementation as oracle.
						ax := 2 * xi
						if i > 0 {
							ax -= x[i-1]
						}
						if i+1 < n {
							ax -= x[i+1]
						}
						rhs := b[i] / scale
						rn = math.Hypot(rn, rhs-ax)
						bn = math.Hypot(bn, rhs)
					}
					if rn > o.RelativeTolerance*bn*1.01 {
						t.Fatalf("original scaled residual %.17g threshold %.17g", rn, o.RelativeTolerance*bn)
					}
				})
			}
		}
	}
}
