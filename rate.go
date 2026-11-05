// Rate formulas follow Manim Community 0.22.1 utils/rate_functions.py.
// Copyright (c) 2024 the Manim Community Developers; MIT notices in LICENSES/.
package gomath

import (
	"fmt"
	"math"
)

// RateFunc maps normalized animation time to progress. Geometry and scalar
// transforms preserve finite overshoot; styles and path traversal remain bounded.
// Functions captured by a Scene must be deterministic and concurrency safe.
type RateFunc func(float64) float64

func Linear(t float64) float64 { return clamp(t) }

// Smooth is Manim's normalized logistic easing with inflection 10. The former
// cubic GoMath easing remains available as SmoothStep.
func Smooth(t float64) float64 { return logisticSmooth(t, 10) }
func logisticSmooth(t, inflection float64) float64 {
	t = clamp(t)
	if t == 0 || t == 1 {
		return t
	}
	if inflection <= 1e-8 {
		return t
	}
	// tanh avoids cancellation for small inflections and overflow for large ones.
	return clamp(.5 + .5*math.Tanh(inflection*(t-.5)/2)/math.Tanh(inflection/4))
}
func SmoothStep(t float64) float64 {
	t = clamp(t)
	return t * t * (3 - 2*t)
}
func SmootherStep(t float64) float64 {
	t = clamp(t)
	return t * t * t * (t*(t*6-15) + 10)
}
func SmoothererStep(t float64) float64 {
	t = clamp(t)
	return t * t * t * t * (35 + t*(-84+t*(70-20*t)))
}
func Smoother(t float64) float64 { return SmootherStep(t) }
func RushInto(t float64) float64 { return 2 * Smooth(clamp(t)/2) }
func RushFrom(t float64) float64 { return 2*Smooth(clamp(t)/2+.5) - 1 }
func SlowInto(t float64) float64 {
	t = clamp(t)
	return math.Sqrt(1 - (1-t)*(1-t))
}
func DoubleSmooth(t float64) float64 {
	t = clamp(t)
	if t < .5 {
		return .5 * Smooth(2*t)
	}
	return .5 * (1 + Smooth(2*t-1))
}
func ThereAndBack(t float64) float64 {
	if t < 0 || t > 1 {
		return 0
	}
	return Smooth(1 - math.Abs(2*t-1))
}
func ThereAndBackWithPause(t float64) float64 { return paused(t, 1.0/3) }
func paused(t, ratio float64) float64 {
	if t < 0 || t > 1 {
		return 0
	}
	a := 2 / (1 - ratio)
	if t < .5-ratio/2 {
		return Smooth(a * t)
	}
	if t < .5+ratio/2 {
		return 1
	}
	return Smooth(a - a*t)
}
func RunningStart(t float64) float64 { return runningStart(t, -.5) }
func runningStart(t, pull float64) float64 {
	t = clamp(t)
	u := 1 - t
	return 15*t*t*u*u*u*u*pull + 20*t*t*t*u*u*u*pull + 15*t*t*t*t*u*u + 6*t*t*t*t*t*u + t*t*t*t*t*t
}
func Wiggle(t float64) float64 {
	if t < 0 || t > 1 {
		return 0
	}
	return ThereAndBack(t) * math.Sin(2*math.Pi*t)
}
func Lingering(t float64) float64 {
	if t < 0 || t > 1 {
		return clamp(t)
	}
	return math.Min(t/.8, 1)
}
func ExponentialDecay(t float64) float64 {
	if t < 0 || t > 1 {
		return clamp(t)
	}
	return -math.Expm1(-t / .1)
}

// SmoothRate constructs normalized logistic easing with a positive inflection.
func SmoothRate(inflection float64) (RateFunc, error) {
	if !finite(inflection) || inflection <= 0 {
		return nil, fmt.Errorf("smooth inflection must be finite and positive")
	}
	return func(t float64) float64 { return logisticSmooth(t, inflection) }, nil
}
func ThereAndBackRate(inflection float64) (RateFunc, error) {
	rate, err := SmoothRate(inflection)
	if err != nil {
		return nil, err
	}
	return func(t float64) float64 {
		if t < 0 || t > 1 {
			return 0
		}
		return rate(1 - math.Abs(2*t-1))
	}, nil
}
func RushIntoRate(inflection float64) (RateFunc, error) {
	rate, err := SmoothRate(inflection)
	if err != nil {
		return nil, err
	}
	return func(t float64) float64 { return 2 * rate(clamp(t)/2) }, nil
}
func RushFromRate(inflection float64) (RateFunc, error) {
	rate, err := SmoothRate(inflection)
	if err != nil {
		return nil, err
	}
	return func(t float64) float64 { return 2*rate(clamp(t)/2+.5) - 1 }, nil
}
func PausedRate(pauseRatio float64) (RateFunc, error) {
	if !finite(pauseRatio) || pauseRatio < 0 || pauseRatio >= 1 {
		return nil, fmt.Errorf("pause ratio must be finite in [0,1)")
	}
	return func(t float64) float64 { return paused(t, pauseRatio) }, nil
}
func WiggleRate(wiggles float64) (RateFunc, error) {
	if !finite(wiggles) || wiggles < 0 || wiggles > 1e6 {
		return nil, fmt.Errorf("wiggles must be finite in [0,1000000]")
	}
	return func(t float64) float64 {
		if t < 0 || t > 1 {
			return 0
		}
		return ThereAndBack(t) * math.Sin(wiggles*math.Pi*t)
	}, nil
}
func RunningStartRate(pull float64) (RateFunc, error) {
	if !finite(pull) {
		return nil, fmt.Errorf("running start pull must be finite")
	}
	return func(t float64) float64 { return runningStart(t, pull) }, nil
}
func ExponentialDecayRate(halfLife float64) (RateFunc, error) {
	if !finite(halfLife) || halfLife <= 0 {
		return nil, fmt.Errorf("decay half life must be finite and positive")
	}
	return func(t float64) float64 {
		if t < 0 || t > 1 {
			return clamp(t)
		}
		return -math.Expm1(-t / halfLife)
	}, nil
}
func NotQuiteThere(rate RateFunc, proportion float64) (RateFunc, error) {
	if rate == nil || !finite(proportion) {
		return nil, fmt.Errorf("partial rate requires a function and finite proportion")
	}
	return func(t float64) float64 { return proportion * rate(t) }, nil
}

// SquishRate preserves the upstream a==b behavior: the constant value a.
func SquishRate(rate RateFunc, a, b float64) (RateFunc, error) {
	if rate == nil || !finite(a) || !finite(b) || a < 0 || b > 1 || a > b {
		return nil, fmt.Errorf("squish rate requires a function and 0 <= a <= b <= 1")
	}
	return func(t float64) float64 {
		if a == b {
			return a
		}
		return rate(clamp((t - a) / (b - a)))
	}, nil
}

func EaseInSine(t float64) float64     { return 1 - math.Cos(clamp(t)*math.Pi/2) }
func EaseOutSine(t float64) float64    { return math.Sin(clamp(t) * math.Pi / 2) }
func EaseInOutSine(t float64) float64  { return (1 - math.Cos(clamp(t)*math.Pi)) / 2 }
func EaseInQuad(t float64) float64     { t = clamp(t); return t * t }
func EaseOutQuad(t float64) float64    { t = 1 - clamp(t); return 1 - t*t }
func EaseInOutQuad(t float64) float64  { return inOutPower(t, 2) }
func EaseInCubic(t float64) float64    { t = clamp(t); return t * t * t }
func EaseOutCubic(t float64) float64   { t = 1 - clamp(t); return 1 - t*t*t }
func EaseInOutCubic(t float64) float64 { return inOutPower(t, 3) }
func EaseInQuart(t float64) float64    { t = clamp(t); return t * t * t * t }
func EaseOutQuart(t float64) float64   { t = 1 - clamp(t); return 1 - t*t*t*t }
func EaseInOutQuart(t float64) float64 { return inOutPower(t, 4) }
func EaseInQuint(t float64) float64    { t = clamp(t); return t * t * t * t * t }
func EaseOutQuint(t float64) float64   { t = 1 - clamp(t); return 1 - t*t*t*t*t }
func EaseInOutQuint(t float64) float64 { return inOutPower(t, 5) }
func inOutPower(t float64, n int) float64 {
	t = clamp(t)
	if t < .5 {
		return math.Pow(2*t, float64(n)) / 2
	}
	return 1 - math.Pow(2-2*t, float64(n))/2
}
func EaseInExpo(t float64) float64 {
	t = clamp(t)
	if t == 0 {
		return 0
	}
	return math.Exp2(10*t - 10)
}
func EaseOutExpo(t float64) float64 {
	t = clamp(t)
	if t == 1 {
		return 1
	}
	return 1 - math.Exp2(-10*t)
}
func EaseInOutExpo(t float64) float64 {
	t = clamp(t)
	if t == 0 || t == 1 {
		return t
	}
	if t < .5 {
		return math.Exp2(20*t-10) / 2
	}
	return 1 - math.Exp2(-20*t+10)/2
}
func EaseInCirc(t float64) float64  { t = clamp(t); return 1 - math.Sqrt(1-t*t) }
func EaseOutCirc(t float64) float64 { t = clamp(t) - 1; return math.Sqrt(1 - t*t) }
func EaseInOutCirc(t float64) float64 {
	t = clamp(t)
	if t < .5 {
		return (1 - math.Sqrt(1-4*t*t)) / 2
	}
	u := 2 - 2*t
	return (1 + math.Sqrt(1-u*u)) / 2
}
func EaseInBack(t float64) float64 {
	t = clamp(t)
	return 2.70158*t*t*t - 1.70158*t*t
}
func EaseOutBack(t float64) float64 {
	t = clamp(t) - 1
	return 1 + 2.70158*t*t*t + 1.70158*t*t
}
func EaseInOutBack(t float64) float64 {
	t = clamp(t)
	c := 1.70158 * 1.525
	if t < .5 {
		return 2 * t * t * ((c+1)*2*t - c)
	}
	u := 2*t - 2
	return (u*u*((c+1)*u+c) + 2) / 2
}
func EaseInElastic(t float64) float64 {
	t = clamp(t)
	if t == 0 || t == 1 {
		return t
	}
	return -math.Exp2(10*t-10) * math.Sin((10*t-10.75)*(2*math.Pi/3))
}
func EaseOutElastic(t float64) float64 {
	t = clamp(t)
	if t == 0 || t == 1 {
		return t
	}
	return math.Exp2(-10*t)*math.Sin((10*t-.75)*(2*math.Pi/3)) + 1
}
func EaseInOutElastic(t float64) float64 {
	t = clamp(t)
	if t == 0 || t == 1 {
		return t
	}
	if t < .5 {
		return -math.Exp2(20*t-10) * math.Sin((20*t-11.125)*(2*math.Pi/4.5)) / 2
	}
	return math.Exp2(-20*t+10)*math.Sin((20*t-11.125)*(2*math.Pi/4.5))/2 + 1
}
func EaseOutBounce(t float64) float64 {
	t = clamp(t)
	const n, d = 7.5625, 2.75
	if t < 1/d {
		return n * t * t
	}
	if t < 2/d {
		t -= 1.5 / d
		return n*t*t + .75
	}
	if t < 2.5/d {
		t -= 2.25 / d
		return n*t*t + .9375
	}
	t -= 2.625 / d
	return n*t*t + .984375
}
func EaseInBounce(t float64) float64 { return 1 - EaseOutBounce(1-clamp(t)) }
func EaseInOutBounce(t float64) float64 {
	t = clamp(t)
	if t < .5 {
		return (1 - EaseOutBounce(1-2*t)) / 2
	}
	return (1 + EaseOutBounce(2*t-1)) / 2
}
