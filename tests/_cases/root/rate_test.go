package gomath_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"gomath"
	"gomath/geom"
	"math"
	"os"
	"testing"
)

func TestPinnedManimRatesAndOvershoot(t *testing.T) {
	data, err := os.ReadFile("tests/data/root/manim-rates-0.22.1.json")
	must(t, err)
	var fixture struct {
		Source struct {
			Version, Commit string
			GeneratorHash   string `json:"generator_sha256"`
		}
		Rates []struct {
			Name         string
			Alpha, Value float64
		}
		Animations []struct {
			Name     string
			Alpha, X float64
		}
	}
	must(t, json.Unmarshal(data, &fixture))
	if fixture.Source.Commit != "16ea443e2a5f1d79db0a721161411ce916d6e733" || fixture.Source.Version != "0.22.1" || len(fixture.Rates) != 728 || len(fixture.Animations) != 55 {
		t.Fatal("unexpected reference source or record count")
	}
	generator, err := os.ReadFile("tests/reference/rates.py")
	must(t, err)
	sum := sha256.Sum256(generator)
	if hex.EncodeToString(sum[:]) != fixture.Source.GeneratorHash {
		t.Fatal("reference generator differs from recorded SHA256")
	}
	rates := map[string]gomath.RateFunc{
		"linear": gomath.Linear, "smooth": gomath.Smooth, "smoothstep": gomath.SmoothStep,
		"smootherstep": gomath.SmootherStep, "smoothererstep": gomath.SmoothererStep,
		"rush_into": gomath.RushInto, "rush_from": gomath.RushFrom, "slow_into": gomath.SlowInto,
		"double_smooth": gomath.DoubleSmooth, "there_and_back": gomath.ThereAndBack,
		"there_and_back_with_pause": gomath.ThereAndBackWithPause, "running_start": gomath.RunningStart,
		"wiggle": gomath.Wiggle, "lingering": gomath.Lingering, "exponential_decay": gomath.ExponentialDecay,
		"ease_in_sine": gomath.EaseInSine, "ease_out_sine": gomath.EaseOutSine, "ease_in_out_sine": gomath.EaseInOutSine,
		"ease_in_quad": gomath.EaseInQuad, "ease_out_quad": gomath.EaseOutQuad, "ease_in_out_quad": gomath.EaseInOutQuad,
		"ease_in_cubic": gomath.EaseInCubic, "ease_out_cubic": gomath.EaseOutCubic, "ease_in_out_cubic": gomath.EaseInOutCubic,
		"ease_in_quart": gomath.EaseInQuart, "ease_out_quart": gomath.EaseOutQuart, "ease_in_out_quart": gomath.EaseInOutQuart,
		"ease_in_quint": gomath.EaseInQuint, "ease_out_quint": gomath.EaseOutQuint, "ease_in_out_quint": gomath.EaseInOutQuint,
		"ease_in_expo": gomath.EaseInExpo, "ease_out_expo": gomath.EaseOutExpo, "ease_in_out_expo": gomath.EaseInOutExpo,
		"ease_in_circ": gomath.EaseInCirc, "ease_out_circ": gomath.EaseOutCirc, "ease_in_out_circ": gomath.EaseInOutCirc,
		"ease_in_back": gomath.EaseInBack, "ease_out_back": gomath.EaseOutBack, "ease_in_out_back": gomath.EaseInOutBack,
		"ease_in_elastic": gomath.EaseInElastic, "ease_out_elastic": gomath.EaseOutElastic, "ease_in_out_elastic": gomath.EaseInOutElastic,
		"ease_in_bounce": gomath.EaseInBounce, "ease_out_bounce": gomath.EaseOutBounce, "ease_in_out_bounce": gomath.EaseInOutBounce,
	}
	add := func(name string, rate gomath.RateFunc, err error) { t.Helper(); must(t, err); rates[name] = rate }
	fn, err := gomath.SmoothRate(3)
	add("smooth_3", fn, err)
	fn, err = gomath.ThereAndBackRate(3)
	add("there_and_back_3", fn, err)
	fn, err = gomath.RushIntoRate(3)
	add("rush_into_3", fn, err)
	fn, err = gomath.RushFromRate(3)
	add("rush_from_3", fn, err)
	fn, err = gomath.PausedRate(.5)
	add("pause_half", fn, err)
	fn, err = gomath.WiggleRate(3)
	add("wiggle_3", fn, err)
	fn, err = gomath.RunningStartRate(-.25)
	add("running_start_quarter", fn, err)
	fn, err = gomath.ExponentialDecayRate(.25)
	add("decay_quarter", fn, err)
	fn, err = gomath.NotQuiteThere(gomath.Smooth, .7)
	add("partial", fn, err)
	fn, err = gomath.SquishRate(gomath.Smooth, .4, .6)
	add("squish", fn, err)
	fn, err = gomath.SquishRate(gomath.Smooth, .4, .4)
	add("squish_equal", fn, err)
	for _, c := range fixture.Rates {
		t.Run(fmt.Sprintf("%s/%g", c.Name, c.Alpha), func(t *testing.T) {
			fn := rates[c.Name]
			if fn == nil {
				t.Fatal("missing rate", c.Name)
			}
			got := fn(c.Alpha)
			if math.IsNaN(got) || math.Abs(got-c.Value) > 2e-14 {
				t.Fatalf("got %.17g want %.17g", got, c.Value)
			}
		})
	}
	for _, c := range fixture.Animations {
		t.Run(fmt.Sprintf("transform/%s/%g", c.Name, c.Alpha), func(t *testing.T) {
			s := gomath.NewScene()
			o := gomath.Line(geom.V(0, 0), geom.V(1, 0))
			must(t, s.Add(o))
			anim := gomath.Shift(o, geom.V(4, 0))
			if c.Name != "smooth" {
				anim = anim.WithRate(rates[c.Name])
			}
			must(t, s.Play(1, anim))
			got := sample(t, s, c.Alpha).Find(o.ID).Center().X
			if math.Abs(got-c.X) > 1e-12 {
				t.Fatalf("got %.17g want %.17g", got, c.X)
			}
		})
	}
}

func TestConfiguredRatesValidationAndExtremeInflection(t *testing.T) {
	for _, x := range []float64{0, -1, math.Inf(1), math.NaN()} {
		if _, err := gomath.SmoothRate(x); err == nil {
			t.Fatal("invalid inflection", x)
		}
		if _, err := gomath.ExponentialDecayRate(x); err == nil {
			t.Fatal("invalid half life", x)
		}
	}
	for _, x := range []float64{-1, 1, math.NaN(), math.Inf(1)} {
		if _, err := gomath.PausedRate(x); err == nil {
			t.Fatal("invalid pause", x)
		}
	}
	for _, p := range [][2]float64{{-.1, .5}, {.8, .2}, {.2, 1.1}, {math.NaN(), 1}} {
		if _, err := gomath.SquishRate(gomath.Linear, p[0], p[1]); err == nil {
			t.Fatal("invalid squish", p)
		}
	}
	if _, err := gomath.SquishRate(nil, 0, 1); err == nil {
		t.Fatal("nil squish")
	}
	if _, err := gomath.NotQuiteThere(nil, 1); err == nil {
		t.Fatal("nil partial")
	}
	if _, err := gomath.NotQuiteThere(gomath.Linear, math.NaN()); err == nil {
		t.Fatal("nonfinite partial")
	}
	for _, x := range []float64{math.SmallestNonzeroFloat64, 1e-9, 3, 10, 1e300} {
		fn, err := gomath.SmoothRate(x)
		must(t, err)
		previous := 0.0
		for i := 0; i <= 100; i++ {
			got := fn(float64(i) / 100)
			if math.IsNaN(got) || got < previous || got > 1 {
				t.Fatalf("invalid smooth(%g): %g", x, got)
			}
			previous = got
		}
	}
	for _, fn := range []gomath.RateFunc{gomath.ThereAndBack, gomath.Wiggle, gomath.ThereAndBackWithPause} {
		near(t, fn(math.Inf(-1)), 0)
		near(t, fn(math.Inf(1)), 0)
	}
}
