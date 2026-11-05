package sparse

import (
	"context"
	"fmt"
	"math"
	"math/big"
)

// 4352 bits hold exact sums of at most 16M float64 products, including
// cancellation across the complete binary64 exponent span (4196+24 bits).
const precision = 4352

func number() *big.Float { return new(big.Float).SetPrec(precision).SetMode(big.ToNearestEven) }

// CG scalar coefficients have binary64 significand precision but an extended
// exponent range. Dot-product precision must not determine recurrence precision:
// using thousands of coefficient bits leaves artificial, unrepresentable tails
// at ordinary cancellation sites in vector recurrences. Keeping coefficients
// and direction scales at 53 bits gives explicit binary64 iteration semantics
// while extended exponents still avoid premature scalar overflow/underflow.
func scalar() *big.Float { return new(big.Float).SetPrec(53).SetMode(big.ToNearestEven) }

type accumulator struct{ s, p, x, y big.Float }

func newAccumulator() *accumulator {
	a := new(accumulator)
	for _, v := range []*big.Float{&a.s, &a.p, &a.x, &a.y} {
		v.SetPrec(precision)
	}
	return a
}
func (a *accumulator) reset() { a.s.SetInt64(0) }
func (a *accumulator) add(x, y float64) {
	a.x.SetFloat64(x)
	a.y.SetFloat64(y)
	a.p.Mul(&a.x, &a.y)
	a.s.Add(&a.s, &a.p)
}
func rounded(x *big.Float) (float64, error) {
	v, _ := x.Float64()
	if !finite(v) || (v == 0 && x.Sign() != 0) {
		return 0, ErrRange
	}
	return v, nil
}
func canceled(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}
func tick(ctx context.Context, k int) error {
	if k&255 == 0 {
		return canceled(ctx)
	}
	return nil
}
func (a *CSR) multiply(out, x, subtract []float64, ctx context.Context) error {
	acc := newAccumulator()
	step := 0
	for i := 0; i < a.rows; i++ {
		if e := tick(ctx, i); e != nil {
			return e
		}
		acc.reset()
		if subtract != nil {
			acc.add(subtract[i], 1)
		}
		for j := a.ptr[i]; j < a.ptr[i+1]; j++ {
			if e := tick(ctx, step); e != nil {
				return e
			}
			step++
			s := a.val[j]
			if subtract != nil {
				s = -s
			}
			acc.add(s, x[a.col[j]])
		}
		v, e := rounded(&acc.s)
		if e != nil {
			return fmt.Errorf("%w: sparse multiply row %d", e, i)
		}
		out[i] = v
	}
	return canceled(ctx)
}
func dot(x, y []float64, ctx context.Context) (*big.Float, error) {
	a := newAccumulator()
	for i := range x {
		if e := tick(ctx, i); e != nil {
			return nil, e
		}
		a.add(x[i], y[i])
	}
	return number().Set(&a.s), canceled(ctx)
}
func axpy(out, x, y []float64, scale *big.Float, ctx context.Context) error {
	v, w := number(), number()
	for i := range out {
		if e := tick(ctx, i); e != nil {
			return e
		}
		v.SetFloat64(y[i])
		v.Mul(v, scale)
		w.SetFloat64(x[i])
		v.Add(v, w)
		f, e := rounded(v)
		if e != nil {
			return fmt.Errorf("%w: axpy component %d", e, i)
		}
		out[i] = f
	}
	return canceled(ctx)
}
func norm(x []float64, ctx context.Context) (*big.Float, error) {
	v, e := dot(x, x, ctx)
	if e != nil {
		return nil, e
	}
	return v.Sqrt(v), nil
}
func ratio(a, b *big.Float) float64 {
	if b.Sign() == 0 {
		if a.Sign() == 0 {
			return 0
		}
		return math.Inf(1)
	}
	f, _ := number().Quo(a, b).Float64()
	return f
}

// direction forms z + coefficient*old in extended range, then normalizes its
// largest component to one. It returns the scale of the unnormalized vector.
func direction(out, z, old []float64, coefficient *big.Float, ctx context.Context) (*big.Float, error) {
	v, w, max := number(), number(), number()
	component := func(i int) {
		v.SetFloat64(z[i])
		if old != nil {
			w.SetFloat64(old[i])
			w.Mul(w, coefficient)
			v.Add(v, w)
		}
	}
	for i := range out {
		if e := tick(ctx, i); e != nil {
			return nil, e
		}
		component(i)
		w.Abs(v)
		if w.Cmp(max) > 0 {
			max.Set(w)
		}
	}
	if max.Sign() == 0 {
		return nil, ErrBreakdown
	}
	for i := range out {
		if e := tick(ctx, i); e != nil {
			return nil, e
		}
		component(i)
		v.Quo(v, max)
		f, e := rounded(v)
		if e != nil {
			return nil, fmt.Errorf("%w: direction component %d", e, i)
		}
		out[i] = f
	}
	return scalar().Set(max), canceled(ctx)
}
