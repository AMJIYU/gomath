package linalg

import (
	"fmt"
	"math"
	"math/bits"
)

type complexBinary struct {
	frac float64
	exp  int
}

// Norms and wide quotients carry separate real/imaginary exponents. Mantissas
// remain float64; fixed exact-product sums cover finite-input dot/update kernels.
type complexWide struct{ re, im complexBinary }

func complexReal(x float64) complexBinary { f, e := math.Frexp(x); return complexBinary{f, e} }
func complexScalar(x complex128) complexWide {
	return complexWide{complexReal(real(x)), complexReal(imag(x))}
}
func complexAddReal(a, b complexBinary) complexBinary {
	var sum complexScaledSum
	sum.add(a)
	sum.add(b)
	return sum.binary()
}
func complexNegReal(a complexBinary) complexBinary { a.frac = -a.frac; return a }
func complexMulReal(a, b complexBinary) complexBinary {
	if a.frac == 0 || b.frac == 0 {
		return complexBinary{}
	}
	f, e := math.Frexp(a.frac * b.frac)
	return complexBinary{f, a.exp + b.exp + e}
}
func complexDivReal(a, b complexBinary) complexBinary {
	if a.frac == 0 {
		return complexBinary{}
	}
	f, e := math.Frexp(a.frac / b.frac)
	return complexBinary{f, a.exp - b.exp + e}
}
func (a complexWide) mul(b complexWide) complexWide {
	return complexWide{complexAddReal(complexMulReal(a.re, b.re), complexNegReal(complexMulReal(a.im, b.im))),
		complexAddReal(complexMulReal(a.re, b.im), complexMulReal(a.im, b.re))}
}
func (a complexWide) div(b complexWide) complexWide {
	denominator := complexAddReal(complexMulReal(b.re, b.re), complexMulReal(b.im, b.im))
	re := complexAddReal(complexMulReal(a.re, b.re), complexMulReal(a.im, b.im))
	im := complexAddReal(complexMulReal(a.im, b.re), complexNegReal(complexMulReal(a.re, b.im)))
	return complexWide{complexDivReal(re, denominator), complexDivReal(im, denominator)}
}
func complexHypot(a, b complexBinary) complexBinary {
	if a.frac == 0 {
		b.frac = math.Abs(b.frac)
		return b
	}
	if b.frac == 0 {
		a.frac = math.Abs(a.frac)
		return a
	}
	exponent := max(a.exp, b.exp)
	f, e := math.Frexp(math.Hypot(math.Ldexp(a.frac, a.exp-exponent), math.Ldexp(b.frac, b.exp-exponent)))
	return complexBinary{f, exponent + e}
}
func (a complexWide) norm() complexBinary { return complexHypot(a.re, a.im) }

type complexScaledSum struct {
	sum, correction float64
	exp             int
	started         bool
}

func (s *complexScaledSum) add(v complexBinary) {
	if v.frac == 0 {
		return
	}
	if !s.started {
		s.sum, s.exp, s.started = v.frac, v.exp, true
		return
	}
	if v.exp > s.exp {
		s.sum = math.Ldexp(s.sum, s.exp-v.exp)
		s.correction = math.Ldexp(s.correction, s.exp-v.exp)
		s.exp = v.exp
	}
	x := math.Ldexp(v.frac, v.exp-s.exp)
	next := s.sum + x
	if math.Abs(s.sum) >= math.Abs(x) {
		s.correction += (s.sum - next) + x
	} else {
		s.correction += (x - next) + s.sum
	}
	s.sum = next
}

func (s *complexScaledSum) binary() complexBinary {
	f, e := math.Frexp(s.sum + s.correction)
	return complexBinary{frac: f, exp: s.exp + e}
}

// Fixed unsigned buckets separately collect each sign. Binary64 products have
// bit positions -2148..2047; 68 words (4352 bits) also cover <=8194-term carries.
// No precision/allocation depends on the input exponent. Only the final exact
// signed difference is rounded to a normalized float64 mantissa.
type complexExactSum struct{ rePositive, reNegative, imPositive, imNegative [68]uint64 }

func (s *complexExactSum) reset() { *s = complexExactSum{} }
func complexProductParts(x float64) (mantissa uint64, exponent int) {
	raw := math.Float64bits(x)
	mantissa = raw & ((1 << 52) - 1)
	exponent = int((raw >> 52) & 2047)
	if exponent == 0 {
		return mantissa, -1074
	}
	return mantissa | (1 << 52), exponent - 1075
}
func (s *complexExactSum) product(a, b float64, imaginary bool) {
	if a == 0 || b == 0 {
		return
	}
	am, ae := complexProductParts(a)
	bm, be := complexProductParts(b)
	negative := math.Signbit(a) != math.Signbit(b)
	bucket := &s.rePositive
	if imaginary {
		bucket = &s.imPositive
	}
	if negative {
		bucket = &s.reNegative
		if imaginary {
			bucket = &s.imNegative
		}
	}
	hi, lo := bits.Mul64(am, bm)
	offset := ae + be + 2148
	word, shift := offset/64, uint(offset%64)
	var parts [3]uint64
	if shift == 0 {
		parts[0], parts[1] = lo, hi
	} else {
		parts[0] = lo << shift
		parts[1] = (hi << shift) | (lo >> (64 - shift))
		parts[2] = hi >> (64 - shift)
	}
	carry := uint64(0)
	for i, part := range parts {
		bucket[word+i], carry = bits.Add64(bucket[word+i], part, carry)
	}
	for i := word + len(parts); carry != 0; i++ {
		bucket[i], carry = bits.Add64(bucket[i], 0, carry)
	}
}
func complexBucketDifference(positive, negative *[68]uint64) complexBinary {
	sign := 1.0
	larger, smaller := positive, negative
	top := len(positive) - 1
	for top >= 0 && positive[top] == negative[top] {
		top--
	}
	if top < 0 {
		return complexBinary{}
	}
	if positive[top] < negative[top] {
		sign = -1
		larger, smaller = negative, positive
	}
	var magnitude [68]uint64
	borrow := uint64(0)
	for i := range magnitude {
		magnitude[i], borrow = bits.Sub64(larger[i], smaller[i], borrow)
	}
	top = len(magnitude) - 1
	for magnitude[top] == 0 {
		top--
	}
	length := 64*top + bits.Len64(magnitude[top])
	if length <= 53 {
		return complexBinary{sign * math.Ldexp(float64(magnitude[0]), -length), length - 2148}
	}
	shift := length - 53
	word, offset := shift/64, uint(shift%64)
	leading := magnitude[word] >> offset
	if offset != 0 {
		leading |= magnitude[word+1] << (64 - offset)
	}
	// Round the discarded tail once, using nearest/ties-to-even.
	guardPosition := shift - 1
	guardWord, guardOffset := guardPosition/64, uint(guardPosition%64)
	guard := (magnitude[guardWord] >> guardOffset) & 1
	sticky := magnitude[guardWord]&((uint64(1)<<guardOffset)-1) != 0
	for i := 0; i < guardWord && !sticky; i++ {
		sticky = magnitude[i] != 0
	}
	if guard != 0 && (sticky || leading&1 != 0) {
		leading++
	}
	frac := math.Ldexp(float64(leading), -53)
	exponent := length - 2148
	if frac == 1 {
		frac = .5
		exponent++
	}
	return complexBinary{sign * frac, exponent}
}
func (s *complexExactSum) value() complexWide {
	return complexWide{complexBucketDifference(&s.rePositive, &s.reNegative), complexBucketDifference(&s.imPositive, &s.imNegative)}
}

func finiteComplex(z complex128) bool { return finite(real(z)) && finite(imag(z)) }
func checkComplex(data []complex128, operation string) error {
	for i, z := range data {
		if !finiteComplex(z) {
			return fmt.Errorf("%w: %s component %d", ErrNonFinite, operation, i)
		}
	}
	return nil
}
func complexTo128(z complexWide) (complex128, error) {
	re, im := math.Ldexp(z.re.frac, z.re.exp), math.Ldexp(z.im.frac, z.im.exp)
	if !finite(re) || !finite(im) || (z.re.frac != 0 && re == 0) || (z.im.frac != 0 && im == 0) {
		return 0, ErrRange
	}
	return complex(re, im), nil
}
func (s *complexExactSum) term(a, b complex128, subtract bool) {
	if subtract {
		a = -a
	}
	s.product(real(a), real(b), false)
	s.product(-imag(a), imag(b), false)
	s.product(real(a), imag(b), true)
	s.product(imag(a), real(b), true)
}
func (s *complexExactSum) scalar(a complex128) {
	s.product(real(a), 1, false)
	s.product(imag(a), 1, true)
}
func complexDivide(a, b complex128) (complex128, error) {
	var sum complexExactSum
	sum.product(real(b), real(b), false)
	sum.product(imag(b), imag(b), false)
	denominator := sum.value().re
	if denominator.frac == 0 {
		return 0, ErrSingular
	}
	sum.reset()
	sum.term(a, complex(real(b), -imag(b)), false)
	numerator := sum.value()
	return complexTo128(complexWide{complexDivReal(numerator.re, denominator), complexDivReal(numerator.im, denominator)})
}
func complexRatio(numerator, denominator complexBinary) float64 {
	if numerator.frac == 0 {
		return 0
	}
	if denominator.frac == 0 {
		return math.Inf(1)
	}
	ratio := complexDivReal(numerator, denominator)
	return math.Max(math.SmallestNonzeroFloat64, math.Ldexp(ratio.frac, ratio.exp))
}
