package linalg

import (
	"errors"
	"math"
	"math/big"
	"math/rand/v2"
	"testing"
)

func TestComplexExactBudgets(t *testing.T) {
	a, _ := NewComplexDense(1, 1, []complex128{2 + 3i})
	opt := DefaultComplexLUOptions()
	opt.Limits.MaxWork = 23
	if f, e := FactorizeComplexWithOptions(a, opt); f != nil || !errors.Is(e, ErrComplexLimit) {
		t.Fatal(f, e)
	}
	opt.Limits.MaxWork = 24
	f, e := FactorizeComplexWithOptions(a, opt)
	if e != nil {
		t.Fatal(e)
	}
	solve := DefaultComplexSolveOptions()
	solve.Limits.MaxWork = 31
	if x, r, e := f.SolveWithOptions([]complex128{2 + 3i}, solve); x != nil || r != 0 || !errors.Is(e, ErrComplexLimit) {
		t.Fatal(x, r, e)
	}
	solve.Limits.MaxWork = 32
	if x, r, e := f.SolveWithOptions([]complex128{2 + 3i}, solve); e != nil || len(x) != 1 || x[0] != 1 || r != 0 {
		t.Fatal(x, r, e)
	}
	if x, e := f.InverseWithOptions(solve); x != nil || !errors.Is(e, ErrComplexLimit) {
		t.Fatal(x, e)
	}
	solve.Limits.MaxWork = 33
	if _, e = f.InverseWithOptions(solve); e != nil {
		t.Fatal(e)
	}
	if l, u, e := f.FactorsWithLimits(ComplexLimits{MaxWork: 1}); l != nil || u != nil || !errors.Is(e, ErrComplexLimit) {
		t.Fatal(l, u, e)
	}
	if _, _, e := f.FactorsWithLimits(ComplexLimits{MaxWork: 2}); e != nil {
		t.Fatal(e)
	}
	m, _ := ComplexIdentity(2)
	if p, e := m.MulWithLimits(m, ComplexLimits{MaxWork: 19}); p != nil || !errors.Is(e, ErrComplexLimit) {
		t.Fatal(p, e)
	}
	if _, e := m.MulWithLimits(m, ComplexLimits{MaxWork: 20}); e != nil {
		t.Fatal(e)
	}
	if p, e := m.AddWithLimits(m, ComplexLimits{MaxWork: 11}); p != nil || !errors.Is(e, ErrComplexLimit) {
		t.Fatal(p, e)
	}
	if _, e := m.AddWithLimits(m, ComplexLimits{MaxWork: 12}); e != nil {
		t.Fatal(e)
	}
	if p, e := m.MulVecWithLimits([]complex128{1, 2}, ComplexLimits{MaxWork: 7}); p != nil || !errors.Is(e, ErrComplexLimit) {
		t.Fatal(p, e)
	}
	if _, e := m.MulVecWithLimits([]complex128{1, 2}, ComplexLimits{MaxWork: 8}); e != nil {
		t.Fatal(e)
	}
	// Limits are per call, not silently inherited from either operand.
	wide, e := NewComplexDenseWithLimits(1, 1025, make([]complex128, 1025), ComplexLimits{MaxDimension: 2048})
	if e != nil {
		t.Fatal(e)
	}
	if p, e := wide.Transpose(); p != nil || !errors.Is(e, ErrComplexLimit) {
		t.Fatal(p, e)
	}
	if _, e := wide.TransposeWithLimits(ComplexLimits{MaxDimension: 2048}); e != nil {
		t.Fatal(e)
	}
	for _, z := range []complex128{complex(math.NaN(), 0), complex(0, math.Inf(1))} {
		if f, e := FactorizeComplex(&ComplexDense{1, 1, []complex128{z}}); f != nil || !errors.Is(e, ErrNonFinite) {
			t.Fatal(f, e)
		}
	}
	huge := &ComplexDense{math.MaxInt, math.MaxInt, nil}
	if f, e := FactorizeComplex(huge); f != nil || !errors.Is(e, ErrDimension) {
		t.Fatal(f, e)
	}
	if z, e := complexDivide(1, 0); z != 0 || !errors.Is(e, ErrSingular) {
		t.Fatal(z, e)
	}
}
func TestComplexExactAccumulatorOracle(t *testing.T) {
	rng := rand.New(rand.NewPCG(20841, 4501))
	for trial := 0; trial < 64; trial++ {
		var sum complexExactSum
		var re, im, left, right, product, mantissa big.Float
		for _, v := range []*big.Float{&re, &im, &left, &right, &product, &mantissa} {
			v.SetPrec(4352)
		}
		for k := 0; k < 128; k++ {
			a, b := math.Float64frombits(rng.Uint64()), math.Float64frombits(rng.Uint64())
			if !finite(a) || !finite(b) {
				continue
			}
			sum.product(a, b, k%2 != 0)
			left.SetFloat64(a)
			right.SetFloat64(b)
			product.Mul(&left, &right)
			if k%2 == 0 {
				re.Add(&re, &product)
			} else {
				im.Add(&im, &product)
			}
		}
		expect := func(v *big.Float) complexBinary {
			e := v.MantExp(&mantissa)
			f, _ := mantissa.Float64()
			f, k := math.Frexp(f)
			return complexBinary{f, e + k}
		}
		got := sum.value()
		if got.re != expect(&re) || got.im != expect(&im) {
			t.Fatal("complex exact sum oracle", trial, got, expect(&re), expect(&im))
		}
	}
	var sum complexExactSum
	sum.product(math.SmallestNonzeroFloat64, math.SmallestNonzeroFloat64, false)
	for range 8194 {
		sum.product(math.MaxFloat64, math.MaxFloat64, false)
	}
	for range 8194 {
		sum.product(-math.MaxFloat64, math.MaxFloat64, false)
	}
	if got := sum.value(); got.re.frac != .5 || got.re.exp != -2147 || got.im.frac != 0 {
		t.Fatal("full product span/carry", got)
	}
	for _, z := range []complex128{complex(math.MaxFloat64, math.SmallestNonzeroFloat64), complex(math.SmallestNonzeroFloat64, math.MaxFloat64), complex(math.SmallestNonzeroFloat64, math.SmallestNonzeroFloat64), complex(1e300, -1e300)} {
		if q, e := complexDivide(z, z); e != nil || q != 1 {
			t.Fatal(z, q, e)
		}
	}
}
func TestComplexNoPartialRHS(t *testing.T) {
	a, _ := NewComplexDense(2, 2, []complex128{1 + 2i, 3 - 1i, 5i, 6 + 7i})
	f, e := FactorizeComplex(a)
	if e != nil {
		t.Fatal(e)
	}
	rhs, _ := NewComplexDense(2, 2, []complex128{0, 2, 0, 4 - 1i})
	opt := DefaultComplexSolveOptions()
	opt.RelativeResidualTolerance = 0
	x, res, e := f.SolveMatrixWithOptions(rhs, opt)
	var d *ComplexResidualError
	if x != nil || res != nil || !errors.As(e, &d) || d.RHS != 1 {
		t.Fatal(x, res, e)
	}
	tiny, _ := NewComplexDense(1, 1, []complex128{complex(math.SmallestNonzeroFloat64, 0)})
	tf, e := FactorizeComplex(tiny)
	if e != nil {
		t.Fatal(e)
	}
	if x, e := tf.Inverse(); x != nil || !errors.Is(e, ErrRange) {
		t.Fatal(x, e)
	}
}
