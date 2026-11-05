package audio

import (
	"errors"
	"math"
	"testing"
)

func TestTrimAndOffsetMix(t *testing.T) {
	format := Format{SampleRate: 10, Channels: 2, ChannelMask: 3}
	p := mustPCM(t, format, []float64{1, -1, .5, -.5, .25, -.25, 0, 0})
	trim, err := p.Trim(.05, .25)
	if err != nil {
		t.Fatal(err)
	}
	closeSamples(t, trim.samples, []float64{.5, -.5, .25, -.25}, 0)
	other := mustPCM(t, format, []float64{.75, .5, .25, -.5})
	for _, test := range []struct {
		offset float64
		want   []float64
	}{
		{.1, []float64{1, -1, 1.25, 0, .5, -.75, 0, 0}},
		{.5, []float64{1, -1, .5, -.5, .25, -.25, 0, 0, 0, 0, .75, .5, .25, -.5}},
		{0, []float64{1.75, -.5, .75, -1, .25, -.25, 0, 0}},
	} {
		got, err := p.Mix(other, test.offset)
		if err != nil || got.Format() != format {
			t.Fatal("mix", err)
		}
		closeSamples(t, got.samples, test.want, 0)
		got.samples[0] = 100
	}
	closeSamples(t, p.samples, []float64{1, -1, .5, -.5, .25, -.25, 0, 0}, 0)
	empty := mustPCM(t, format, nil)
	got, err := p.MixFrames(empty, int(^uint(0)>>1), Limits{})
	if err != nil || got.Frames() != p.Frames() {
		t.Fatal("empty overlay created extra duration", err)
	}
	got.samples[0] = 99
	if p.samples[0] != 1 {
		t.Fatal("empty mix aliases receiver")
	}
	for _, bounds := range [][2]float64{{-.1, .1}, {0, .5}, {.3, .2}, {math.NaN(), .2}, {0, math.Inf(1)}} {
		if _, err := p.Trim(bounds[0], bounds[1]); err == nil {
			t.Fatal("invalid trim accepted", bounds)
		}
	}
	for _, offset := range []float64{-1, math.Inf(1), math.NaN(), math.MaxFloat64} {
		if _, err := p.Mix(other, offset); err == nil {
			t.Fatal("invalid offset accepted", offset)
		}
	}
	for _, format := range []Format{{11, 2, 3}, {10, 1, 0}, {10, 2, 0}} {
		if _, err := p.Mix(mustPCM(t, format, nil), 0); !errors.Is(err, ErrInvalidInput) {
			t.Fatal("implicit rate/channel/layout conversion", err)
		}
	}
	if _, err := p.MixFrames(other, int(^uint(0)>>1), Limits{}); !errors.Is(err, ErrLimit) {
		t.Fatal("overflowing offset accepted", err)
	}
	if _, err := p.MixWithLimits(other, .5, Limits{MaxFrames: 6}); !errors.Is(err, ErrLimit) {
		t.Fatal("mix frame limit ignored", err)
	}
	if _, err := p.MixFrames(other, 0, Limits{MaxSamples: 7}); !errors.Is(err, ErrLimit) {
		t.Fatal("mix sample limit ignored", err)
	}
	if _, err := p.MixFrames(other, -1, Limits{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatal("negative frame accepted", err)
	}
	if _, err := p.TrimFrames(0, 5); !errors.Is(err, ErrInvalidInput) {
		t.Fatal("invalid trim frames accepted", err)
	}
	zero, err := p.TrimFrames(p.Frames(), p.Frames())
	if err != nil || zero.Frames() != 0 {
		t.Fatal("empty trim", err)
	}
}

func TestGainAndEnvelope(t *testing.T) {
	p := mustPCM(t, Format{4, 2, 0}, []float64{1, -1, 1, -1, 1, -1, 1, -1})
	gain, err := p.Gain(-.5)
	if err != nil {
		t.Fatal(err)
	}
	closeSamples(t, gain.samples, []float64{-.5, .5, -.5, .5, -.5, .5, -.5, .5}, 0)
	for _, test := range []struct {
		points []GainPoint
		want   []float64
	}{
		{[]GainPoint{{0, 0}, {1, 1}}, []float64{0, 0, .25, -.25, .5, -.5, .75, -.75}},
		{[]GainPoint{{.25, .5}, {.5, 1}}, []float64{.5, -.5, .5, -.5, 1, -1, 1, -1}},
		{[]GainPoint{{0, 0}, {.25, 1}, {.5, 0}, {1, -1}}, []float64{0, 0, 1, -1, 0, 0, -.5, .5}},
		{[]GainPoint{{.5, 2}}, []float64{2, -2, 2, -2, 2, -2, 2, -2}},
	} {
		got, err := p.Envelope(test.points)
		if err != nil {
			t.Fatal(err)
		}
		closeSamples(t, got.samples, test.want, 1e-15)
		got.samples[0] = 99
	}
	if p.samples[0] != 1 {
		t.Fatal("gain/envelope changed source")
	}
	for _, points := range [][]GainPoint{
		nil, {{0, 1}, {0, 2}}, {{.5, 1}, {.25, 2}}, {{-.1, 1}}, {{1.1, 1}}, {{math.NaN(), 1}}, {{0, math.Inf(1)}},
	} {
		if _, err := p.Envelope(points); !errors.Is(err, ErrInvalidInput) {
			t.Fatal("invalid envelope accepted", points, err)
		}
	}
	if _, err := p.Envelope(make([]GainPoint, 4097)); !errors.Is(err, ErrLimit) {
		t.Fatal("envelope point bound ignored", err)
	}
	for _, gain := range []float64{math.NaN(), math.Inf(-1)} {
		if _, err := p.Gain(gain); !errors.Is(err, ErrInvalidInput) {
			t.Fatal("nonfinite gain accepted", err)
		}
	}
	huge := mustPCM(t, Format{4, 1, 0}, []float64{math.MaxFloat64, math.MaxFloat64})
	if _, err := huge.Gain(2); !errors.Is(err, ErrInvalidInput) {
		t.Fatal("gain overflow accepted", err)
	}
	if _, err := huge.Mix(huge, 0); !errors.Is(err, ErrInvalidInput) {
		t.Fatal("mix overflow accepted", err)
	}
	if _, err := huge.Envelope([]GainPoint{{0, 2}}); !errors.Is(err, ErrInvalidInput) {
		t.Fatal("envelope overflow accepted", err)
	}
	unit := mustPCM(t, Format{4, 1, 0}, []float64{1, 1, 1})
	interpolated, err := unit.Envelope([]GainPoint{{0, math.MaxFloat64}, {.5, -math.MaxFloat64}})
	if err != nil || interpolated.samples[1] != 0 {
		t.Fatal("finite interpolation overflowed", err)
	}
}
