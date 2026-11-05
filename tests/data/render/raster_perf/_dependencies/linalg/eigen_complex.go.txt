package linalg

import (
	"fmt"
	"math"
	"math/bits"
	"math/cmplx"
)

// Each component carries its own exponent: a common complex exponent can erase
// a tiny imaginary shift before an equal real diagonal cancels. Mantissas remain
// float64, not arbitrary precision; compensated dot sums reduce cancellation.
type eigenComplex struct{ re, im svdBinary }
type eigenSum struct{ re, im svdScaledSum }

func eigenReal(x float64) svdBinary { f, e := math.Frexp(x); return svdBinary{f, e} }
func eigenScalar(x complex128) eigenComplex {
	return eigenComplex{eigenReal(real(x)), eigenReal(imag(x))}
}
func eigenAddReal(a, b svdBinary) svdBinary {
	var sum svdScaledSum
	sum.add(a)
	sum.add(b)
	return sum.binary()
}
func eigenNegReal(a svdBinary) svdBinary { a.frac = -a.frac; return a }
func eigenMulReal(a, b svdBinary) svdBinary {
	if a.frac == 0 || b.frac == 0 {
		return svdBinary{}
	}
	f, e := math.Frexp(a.frac * b.frac)
	return svdBinary{f, a.exp + b.exp + e}
}
func eigenDivReal(a, b svdBinary) svdBinary {
	if a.frac == 0 {
		return svdBinary{}
	}
	f, e := math.Frexp(a.frac / b.frac)
	return svdBinary{f, a.exp - b.exp + e}
}
func (a eigenComplex) zero() bool        { return a.re.frac == 0 && a.im.frac == 0 }
func (a eigenComplex) neg() eigenComplex { return eigenComplex{eigenNegReal(a.re), eigenNegReal(a.im)} }
func (a eigenComplex) sub(b eigenComplex) eigenComplex {
	return eigenComplex{eigenAddReal(a.re, eigenNegReal(b.re)), eigenAddReal(a.im, eigenNegReal(b.im))}
}
func (a eigenComplex) mul(b eigenComplex) eigenComplex {
	return eigenComplex{eigenAddReal(eigenMulReal(a.re, b.re), eigenNegReal(eigenMulReal(a.im, b.im))),
		eigenAddReal(eigenMulReal(a.re, b.im), eigenMulReal(a.im, b.re))}
}
func (a eigenComplex) div(b eigenComplex) eigenComplex {
	denominator := eigenAddReal(eigenMulReal(b.re, b.re), eigenMulReal(b.im, b.im))
	re := eigenAddReal(eigenMulReal(a.re, b.re), eigenMulReal(a.im, b.im))
	im := eigenAddReal(eigenMulReal(a.im, b.re), eigenNegReal(eigenMulReal(a.re, b.im)))
	return eigenComplex{eigenDivReal(re, denominator), eigenDivReal(im, denominator)}
}
func (s *eigenSum) add(v eigenComplex)  { s.re.add(v.re); s.im.add(v.im) }
func (s *eigenSum) value() eigenComplex { return eigenComplex{s.re.binary(), s.im.binary()} }
func eigenHypot(a, b svdBinary) svdBinary {
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
	return svdBinary{f, exponent + e}
}
func (a eigenComplex) norm() svdBinary { return eigenHypot(a.re, a.im) }
func eigenLarger(a, b eigenComplex) bool {
	x, y := a.norm(), b.norm()
	if x.frac == 0 {
		return false
	}
	if y.frac == 0 {
		return true
	}
	return x.exp > y.exp || x.exp == y.exp && x.frac > y.frac
}

func eigenSeed(t []float64, n, k int, lambda eigenComplex) (eigenComplex, eigenComplex) {
	a := eigenScalar(complex(t[k*n+k], 0)).sub(lambda)
	b := eigenScalar(complex(t[k*n+k+1], 0))
	c := eigenScalar(complex(t[(k+1)*n+k], 0))
	d := eigenScalar(complex(t[(k+1)*n+k+1], 0)).sub(lambda)
	// The null vector perpendicular to the stronger row avoids a tiny divisor.
	if eigenLarger(eigenComplex{re: eigenHypot(a.norm(), b.norm())}, eigenComplex{re: eigenHypot(c.norm(), d.norm())}) {
		return b, a.neg()
	}
	return d.neg(), c
}

func eigenSolvePair(a, b, c, d, r, s eigenComplex, floor svdBinary) (eigenComplex, eigenComplex, bool) {
	// Complete pivoting keeps the elimination multiplier bounded. The determinant
	// formula can catastrophically cancel even when the shifted block is solvable.
	swapped := false
	if eigenLarger(c, a) {
		a, c = c, a
		b, d = d, b
		r, s = s, r
	}
	if eigenLarger(b, a) || eigenLarger(d, a) {
		a, b = b, a
		c, d = d, c
		swapped = true
		if eigenLarger(c, a) {
			a, c = c, a
			b, d = d, b
			r, s = s, r
		}
	}
	regularized := false
	if a.zero() { // all four entries are zero
		if r.zero() && s.zero() {
			return eigenComplex{}, eigenComplex{}, false
		}
		a = eigenComplex{re: floor}
		regularized = true
	}
	multiplier := c.div(a)
	d = d.sub(multiplier.mul(b))
	s = s.sub(multiplier.mul(r))
	var y eigenComplex
	if d.zero() {
		if !s.zero() {
			d = eigenComplex{re: floor}
			regularized = true
			y = s.div(d)
		}
	} else {
		y = s.div(d)
	}
	x := r.sub(b.mul(y)).div(a)
	if swapped {
		return y, x, regularized
	}
	return x, y, regularized
}

func eigenNormalize(x []eigenComplex) ([]complex128, error) {
	var norm svdBinary
	for _, v := range x {
		norm = eigenHypot(norm, v.norm())
	}
	if norm.frac == 0 || !finite(norm.frac) {
		return nil, fmt.Errorf("%w: zero/nonfinite eigenvector", ErrRange)
	}
	vector := make([]complex128, len(x))
	pivot := 0
	for i, v := range x {
		re, im := eigenDivReal(v.re, norm), eigenDivReal(v.im, norm)
		r, s := math.Ldexp(re.frac, re.exp), math.Ldexp(im.frac, im.exp)
		if !finite(r) || !finite(s) || (re.frac != 0 && r == 0) || (im.frac != 0 && s == 0) {
			return nil, fmt.Errorf("%w: normalized eigenvector component %d", ErrRange, i)
		}
		vector[i] = complex(r, s)
		if cmplx.Abs(vector[i]) > cmplx.Abs(vector[pivot]) {
			pivot = i
		}
	}
	phase := cmplx.Conj(vector[pivot]) / complex(cmplx.Abs(vector[pivot]), 0)
	for i, v := range vector {
		rotated := eigenScalar(v).mul(eigenScalar(phase))
		re, im := math.Ldexp(rotated.re.frac, rotated.re.exp), math.Ldexp(rotated.im.frac, rotated.im.exp)
		if (rotated.re.frac != 0 && re == 0) || (rotated.im.frac != 0 && im == 0) {
			return nil, fmt.Errorf("%w: eigenvector phase underflow at component %d", ErrRange, i)
		}
		vector[i] = complex(re, im)
	}
	vector[pivot] = complex(cmplx.Abs(vector[pivot]), 0)
	return vector, nil
}

func eigenCheck(a []float64, n int, lambda complex128, v []complex128) float64 {
	var anorm, vnorm, residual svdBinary
	for _, x := range a {
		anorm = eigenHypot(anorm, eigenReal(x))
	}
	for _, x := range v {
		vnorm = eigenHypot(vnorm, eigenScalar(x).norm())
	}
	// A fixed-size exact binary64-product sum is used ONLY for validation.
	// The exponent span of products of finite binary64 inputs is at most 4196
	// bits; 4352 bits also covers summing <=2*1024+2 terms. This prevents a tiny
	// nonzero residual disappearing when large terms cancel under tolerance 0.
	var check eigenExactSum
	for i := range n {
		check.reset()
		for j := range n {
			check.product(a[i*n+j], real(v[j]), false)
			check.product(a[i*n+j], imag(v[j]), true)
		}
		check.product(-real(lambda), real(v[i]), false)
		check.product(imag(lambda), imag(v[i]), false)
		check.product(-real(lambda), imag(v[i]), true)
		check.product(-imag(lambda), real(v[i]), true)
		residual = eigenHypot(residual, check.value().norm())
	}
	if residual.frac == 0 {
		return 0
	}
	denominator := eigenMulReal(eigenAddReal(anorm, eigenScalar(lambda).norm()), vnorm)
	if denominator.frac == 0 {
		return math.Inf(1)
	}
	ratio := eigenDivReal(residual, denominator)
	return math.Max(math.SmallestNonzeroFloat64, math.Ldexp(ratio.frac, ratio.exp))
}

func eigenColumnRank(v []complex128, n int, tolerance float64) int {
	work := append([]complex128(nil), v...)
	first := 0.0
	for k := range n {
		pivot, norm := k, 0.0
		for j := k; j < n; j++ {
			columnNorm := 0.0
			for _, x := range work[j*n : (j+1)*n] {
				columnNorm = math.Hypot(columnNorm, cmplx.Abs(x))
			}
			if columnNorm > norm {
				norm = columnNorm
				pivot = j
			}
		}
		if k == 0 {
			first = norm
		}
		if norm == 0 || norm <= tolerance*first {
			return k
		}
		for i := range n {
			work[k*n+i], work[pivot*n+i] = work[pivot*n+i], work[k*n+i]
		}
		q := work[k*n : (k+1)*n]
		for i := range q {
			q[i] /= complex(norm, 0)
		}
		for j := k + 1; j < n; j++ {
			column := work[j*n : (j+1)*n]
			for pass := 0; pass < 2; pass++ {
				var re, im svdScaledSum
				for i, x := range q {
					value := cmplx.Conj(x) * column[i]
					re.add(eigenReal(real(value)))
					im.add(eigenReal(imag(value)))
				}
				dot := complex(re.value(), im.value())
				for i, x := range q {
					column[i] -= x * dot
				}
			}
		}
	}
	return n
}

// Fixed unsigned buckets separately collect each sign. Binary64 products have
// bit positions -2148..2047; 68 words (4352 bits) also cover <=2050-term carries.
// No precision/allocation depends on the input exponent. Only the final exact
// signed difference is rounded to a normalized float64 mantissa.
type eigenExactSum struct{ rePositive, reNegative, imPositive, imNegative [68]uint64 }

func (s *eigenExactSum) reset() { *s = eigenExactSum{} }
func eigenProductParts(x float64) (mantissa uint64, exponent int) {
	raw := math.Float64bits(x)
	mantissa = raw & ((1 << 52) - 1)
	exponent = int((raw >> 52) & 2047)
	if exponent == 0 {
		return mantissa, -1074
	}
	return mantissa | (1 << 52), exponent - 1075
}
func (s *eigenExactSum) product(a, b float64, imaginary bool) {
	if a == 0 || b == 0 {
		return
	}
	am, ae := eigenProductParts(a)
	bm, be := eigenProductParts(b)
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
func eigenBucketDifference(positive, negative *[68]uint64) svdBinary {
	sign := 1.0
	larger, smaller := positive, negative
	top := len(positive) - 1
	for top >= 0 && positive[top] == negative[top] {
		top--
	}
	if top < 0 {
		return svdBinary{}
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
		return svdBinary{sign * math.Ldexp(float64(magnitude[0]), -length), length - 2148}
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
	return svdBinary{sign * frac, exponent}
}
func (s *eigenExactSum) value() eigenComplex {
	return eigenComplex{eigenBucketDifference(&s.rePositive, &s.reNegative), eigenBucketDifference(&s.imPositive, &s.imNegative)}
}
