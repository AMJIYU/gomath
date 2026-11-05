package sparse

import (
	"context"
	"errors"
	"math/big"
	"testing"
)

func biTestNew(t testing.TB, n int) biVector {
	t.Helper()
	v, e := biNew(n, context.Background())
	if e != nil {
		t.Fatal(e)
	}
	return v
}

func TestBiCGSTABExtendedAccumulatorBounds(t *testing.T) {
	ctx := context.Background()
	u, v := biTestNew(t, 3), biTestNew(t, 3)
	high := scalar().SetMantExp(scalar().SetInt64(1), biExponent-1)
	low := scalar().SetMantExp(scalar().SetInt64(1), -biExponent-1)
	u[0].Set(high)
	u[1].Set(low)
	u[2].Set(high)
	v[0].Set(high)
	v[1].Set(low)
	v[2].Neg(high)
	got, e := biDot(u, v, ctx)
	if e != nil {
		t.Fatal(e)
	}
	want := new(big.Float).SetPrec(2*biPrecision).Mul(low, low)
	if got.Cmp(want) != 0 {
		t.Fatalf("lost extreme cancellation: %s vs %s", got.Text('p', 0), want.Text('p', 0))
	}
	if e = biRange(&u[0]); e != nil {
		t.Fatal(e)
	}
	if e = biRange(&u[1]); e != nil {
		t.Fatal(e)
	}
	outside := scalar().SetMantExp(scalar().SetInt64(1), biExponent)
	if !errors.Is(biRange(outside), ErrRange) {
		t.Fatal("upper bound")
	}
	outside.SetMantExp(scalar().SetInt64(1), -biExponent-2)
	if !errors.Is(biRange(outside), ErrRange) {
		t.Fatal("lower bound")
	}
	dst := biTestNew(t, 3)
	if e = biAxpy(dst, u, u, high, ctx); !errors.Is(e, ErrRange) {
		t.Fatalf("extended overflow %v", e)
	}
	diag := []float64{1e-300, 1, 1}
	if e = biApply(dst, u, diag, ctx); !errors.Is(e, ErrRange) {
		t.Fatalf("preconditioner exponent budget %v", e)
	}
	a := diagonal(t, []float64{1e300, 1, 1})
	if e = biMultiply(a, dst, u, ctx); !errors.Is(e, ErrRange) {
		t.Fatalf("matrix exponent budget %v", e)
	}
	// Cancellation is checked inside every extended kernel too.
	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()
	if _, e = biDot(u, v, canceledCtx); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if e = biCopy(dst, u, canceledCtx); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if e = biLoad(dst, []float64{1, 2, 3}, canceledCtx); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if e = biAxpy(dst, u, v, scalar().SetInt64(1), canceledCtx); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if e = biApply(dst, u, []float64{1, 1, 1}, canceledCtx); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if e = biMultiply(a, dst, u, canceledCtx); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if _, e = biSmall(want, u, v, 1e-20, canceledCtx); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
