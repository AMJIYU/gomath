package field

import (
	"gomath/geom"
	"math"
	"math/big"
)

// Rational Dormand-Prince 5(4) tableau, written with a common integer
// denominator per row. These are mathematical constants, not copied code.
// Common denominators preserve exact constant-field cancellation in the error
// estimate; rounding each fraction to binary64 would lose that property.
type adaptiveWeights struct {
	denominator int64
	numerator   [7]int64
}

var adaptiveRows = [6]adaptiveWeights{
	{5, [7]int64{1}},
	{40, [7]int64{3, 9}},
	{45, [7]int64{44, -168, 160}},
	{6561, [7]int64{19372, -76080, 64448, -1908}},
	{167904, [7]int64{477901, -1806240, 1495424, 46746, -45927}},
	{142464, [7]int64{12985, 0, 64000, 92750, -45927, 18656}},
}
var adaptiveError = adaptiveWeights{21369600, [7]int64{-26341, 0, 90880, -790230, 1086939, -895488, 534240}}
var adaptiveAbscissa = [6]float64{1.0 / 5, 3.0 / 10, 4.0 / 5, 8.0 / 9, 1, 1}

// 6400 fixed bits cover exact sums of at most seven binary64 products, their
// binary64 h factor and these <=21-bit integer weights over the entire exponent
// span. Division by a fixed denominator occurs only after numerator cancellation.
const adaptivePrecision = 6400
const adaptiveScratchBytes = 64 << 10

func adaptiveNumber() *big.Float {
	return new(big.Float).SetPrec(adaptivePrecision).SetMode(big.ToNearestEven)
}

type adaptiveArithmetic struct{ sum, term, factor, step, base, denominator big.Float }

func newAdaptiveArithmetic() *adaptiveArithmetic {
	a := new(adaptiveArithmetic)
	for _, v := range []*big.Float{&a.sum, &a.term, &a.factor, &a.step, &a.base, &a.denominator} {
		v.SetPrec(adaptivePrecision).SetMode(big.ToNearestEven)
	}
	return a
}
func (a *adaptiveArithmetic) component(p, h float64, k *[7]geom.Vec3, w adaptiveWeights, axis int, includePoint bool) *big.Float {
	a.sum.SetInt64(0)
	for j, n := range w.numerator {
		if n == 0 {
			continue
		}
		f := k[j].X
		if axis == 1 {
			f = k[j].Y
		}
		a.term.SetFloat64(f)
		a.factor.SetInt64(n)
		a.term.Mul(&a.term, &a.factor)
		a.sum.Add(&a.sum, &a.term)
	}
	a.step.SetFloat64(h)
	a.sum.Mul(&a.sum, &a.step)
	a.denominator.SetInt64(w.denominator)
	if includePoint {
		a.base.SetFloat64(p)
		a.base.Mul(&a.base, &a.denominator)
		a.sum.Add(&a.sum, &a.base)
	}
	return a.sum.Quo(&a.sum, &a.denominator)
}
func (a *adaptiveArithmetic) point(p geom.Vec3, h float64, k *[7]geom.Vec3, w adaptiveWeights) (geom.Vec3, bool) {
	x := a.component(p.X, h, k, w, 0, true)
	fx, _ := x.Float64()
	validX := finite(fx) && (fx != 0 || x.Sign() == 0)
	y := a.component(p.Y, h, k, w, 1, true)
	fy, _ := y.Float64()
	return geom.V(fx, fy), validX && finite(fy) && (fy != 0 || y.Sign() == 0)
}
func (a *adaptiveArithmetic) errorNorm(p, next geom.Vec3, h float64, k *[7]geom.Vec3, atol, rtol float64) float64 {
	squared, scale, tmp, ratio := adaptiveNumber(), adaptiveNumber(), adaptiveNumber(), adaptiveNumber()
	for axis := 0; axis < 2; axis++ {
		u, v := p.X, next.X
		if axis == 1 {
			u, v = p.Y, next.Y
		}
		scale.SetFloat64(max(math.Abs(u), math.Abs(v)))
		tmp.SetFloat64(rtol)
		scale.Mul(scale, tmp)
		tmp.SetFloat64(atol)
		scale.Add(scale, tmp)
		delta := a.component(0, h, k, adaptiveError, axis, false)
		if scale.Sign() == 0 {
			if delta.Sign() != 0 {
				return math.Inf(1)
			}
			continue
		}
		ratio.Quo(delta, scale)
		ratio.Mul(ratio, ratio)
		squared.Add(squared, ratio)
	}
	squared.Quo(squared, adaptiveNumber().SetInt64(2))
	squared.Sqrt(squared)
	out, _ := squared.Float64()
	return out
}
