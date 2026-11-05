package linalg

import (
	"errors"
	"math"
	"math/big"
	"math/cmplx"
	"math/rand/v2"
	"testing"
)

func TestGeneralEigenInternalRangeAndBudget(t *testing.T) {
	// Malformed finite invariant is only constructible inside the package.
	for _, x := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if f, e := FactorizeEigen(&Dense{rows: 1, cols: 1, data: []float64{x}}); f != nil || !errors.Is(e, ErrNonFinite) {
			t.Fatal(f, e)
		}
	}
	a, _ := NewDense(1, 1, []float64{2})
	opt := DefaultEigenOptions()
	reserve := uint64(128 + 64 + 32)
	opt.MaxWork = reserve
	if f, e := FactorizeEigenWithOptions(a, opt); f != nil || !errors.Is(e, ErrEigenLimit) {
		t.Fatal(f, e)
	}
	opt.MaxWork = reserve + 11 // Schur reserve=11, block scan=2: total=13.
	if f, e := FactorizeEigenWithOptions(a, opt); f != nil || !errors.Is(e, ErrSchurLimit) || !errors.Is(e, ErrEigenLimit) {
		t.Fatal(f, e)
	}
	opt.MaxWork = reserve + 13
	if _, e := FactorizeEigenWithOptions(a, opt); e != nil {
		t.Fatal(e)
	}
	empty, _ := Zeros(0, 0)
	opt.MaxWork = 1
	if f, e := FactorizeEigenWithOptions(empty, opt); e != nil || f.Size() != 0 {
		t.Fatal(f, e)
	}
	// An extended normalized component below binary64 is rejected, not silently 0.
	if v, e := eigenNormalize([]eigenComplex{{re: eigenReal(1)}, {re: svdBinary{frac: .5, exp: -2000}}}); v != nil || !errors.Is(e, ErrRange) {
		t.Fatal(v, e)
	}
	if v, e := eigenNormalize([]eigenComplex{{}}); v != nil || !errors.Is(e, ErrRange) {
		t.Fatal(v, e)
	}
	if got := eigenCheck([]float64{0}, 1, 0, []complex128{1}); got != 0 {
		t.Fatal(got)
	}
	// Residual arithmetic must not hide underflow under a literal zero policy.
	if got := eigenCheck([]float64{1, math.SmallestNonzeroFloat64, 0, 1}, 2, 1, []complex128{1, math.SmallestNonzeroFloat64}); got == 0 {
		t.Fatal("tiny residual lost")
	}
	// Long exact Jordan chain: the requested nonzero column tail cannot fit float64.
	n := 32
	data := make([]float64, n*n)
	for i := 0; i < n-1; i++ {
		data[i*n+i+1] = 1
	}
	jordan, _ := NewDense(n, n, data)
	if f, e := FactorizeEigen(jordan); f != nil || !errors.Is(e, ErrRange) {
		t.Fatal("expected explicit vector range failure", f, e)
	}
}

func TestGeneralEigenExtendedArithmetic(t *testing.T) {
	// Components need independent exponents: subtracting the real center must
	// expose the tiny imaginary part, then preserve products beyond float range.
	z := eigenScalar(complex(1, 1e-300)).sub(eigenScalar(1))
	got := z.div(eigenScalar(complex(0, 1e-300)))
	if math.Ldexp(got.re.frac, got.re.exp) != 1 || got.im.frac != 0 {
		t.Fatal(got)
	}
	huge := eigenScalar(complex(1e300, -1e300))
	quotient := huge.mul(huge).div(huge)
	if math.Abs(math.Ldexp(quotient.re.frac, quotient.re.exp)/1e300-1) > 4e-16 || math.Abs(math.Ldexp(quotient.im.frac, quotient.im.exp)/-1e300-1) > 4e-16 {
		t.Fatal(quotient)
	}
	// Complete pivoting, singular consistent, and singular inconsistent 2x2 paths.
	for _, tc := range []struct {
		a, b, c, d, r, s complex128
		singular         bool
	}{
		{0, 2, 3, 4, 2, 7, false}, {1, 10, 2, 3, 11, 5, false}, {1, 2, 3, 20, 3, 23, false},
		{0, 0, 0, 0, 0, 0, true}, {0, 0, 0, 0, 1, 2, true}, {1, 2, 2, 4, 3, 6, true}, {1, 2, 2, 4, 3, 7, true},
	} {
		x, y, regularized := eigenSolvePair(eigenScalar(tc.a), eigenScalar(tc.b), eigenScalar(tc.c), eigenScalar(tc.d), eigenScalar(tc.r), eigenScalar(tc.s), eigenReal(1e-16))
		if !tc.singular {
			xx := complex(math.Ldexp(x.re.frac, x.re.exp), math.Ldexp(x.im.frac, x.im.exp))
			yy := complex(math.Ldexp(y.re.frac, y.re.exp), math.Ldexp(y.im.frac, y.im.exp))
			if regularized || cmplx.Abs(tc.a*xx+tc.b*yy-tc.r) > 1e-13 || cmplx.Abs(tc.c*xx+tc.d*yy-tc.s) > 1e-13 {
				t.Fatal(tc, xx, yy)
			}
		}
	}
}

func TestGeneralEigenExactResidualCancellation(t *testing.T) {
	// Every possible product exponent fits the fixed 4352-bit checker even when
	// 1024 maximal products cancel after the smallest product was accumulated.
	var sum eigenExactSum
	sum.reset()
	sum.product(math.SmallestNonzeroFloat64, math.SmallestNonzeroFloat64, false)
	for range 1024 {
		sum.product(math.MaxFloat64, math.MaxFloat64, false)
	}
	for range 1024 {
		sum.product(-math.MaxFloat64, math.MaxFloat64, false)
	}
	result := sum.value()
	if result.re.frac != .5 || result.re.exp != -2147 || result.im.frac != 0 {
		t.Fatal("exact tiny product lost", result)
	}
	huge := &Dense{rows: math.MaxInt, cols: math.MaxInt}
	if f, e := FactorizeEigen(huge); f != nil || !errors.Is(e, ErrEigenLimit) {
		t.Fatal(f, e)
	}
}

func TestGeneralEigenExactAccumulatorIndependent(t *testing.T) {
	// Independent arbitrary-precision DEVELOPMENT oracle for fixed integer sums.
	// No big.Float participates in the production recovery/validation path.
	rng := rand.New(rand.NewPCG(3981, 1511))
	for trial := 0; trial < 128; trial++ {
		var got eigenExactSum
		got.reset()
		var re, im, left, right, product, mantissa big.Float
		for _, v := range []*big.Float{&re, &im, &left, &right, &product, &mantissa} {
			v.SetPrec(4352)
		}
		for k := 0; k < 128; k++ {
			a, b := math.Float64frombits(rng.Uint64()), math.Float64frombits(rng.Uint64())
			if !finite(a) || !finite(b) {
				continue
			}
			// Half of the trials heavily cancel a product after adding unrelated terms.
			repeat := 1
			if trial%2 != 0 {
				repeat = 2
			}
			for pass := 0; pass < repeat; pass++ {
				if pass == 1 {
					a = -a
				}
				got.product(a, b, k%2 != 0)
				left.SetFloat64(a)
				right.SetFloat64(b)
				product.Mul(&left, &right)
				if k%2 != 0 {
					im.Add(&im, &product)
				} else {
					re.Add(&re, &product)
				}
			}
		}
		expected := func(v *big.Float) svdBinary {
			e := v.MantExp(&mantissa)
			f, _ := mantissa.Float64()
			f, k := math.Frexp(f)
			return svdBinary{f, e + k}
		}
		result := got.value()
		if result.re != expected(&re) || result.im != expected(&im) {
			t.Fatalf("trial %d got %+v expected %+v %+v", trial, result, expected(&re), expected(&im))
		}
	}
	// Directed halfway cases exercise guard, sticky and ties-to-even rounding.
	for _, a := range []float64{1, math.Nextafter(1, 2), -1, -math.Nextafter(1, 2)} {
		var got eigenExactSum
		got.reset()
		got.product(a, 1, false)
		got.product(math.Copysign(0x1p-53, a), 1, false)
		re := got.value().re
		if math.Ldexp(re.frac, re.exp) != a+math.Copysign(0x1p-53, a) {
			t.Fatal("tie rounding", a, re)
		}
	}
}
