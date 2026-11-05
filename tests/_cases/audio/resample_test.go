package audio

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"testing"
)

func sinePCM(t testing.TB, rate, frames, channels int, frequency float64) *PCM {
	t.Helper()
	samples := make([]float64, frames*channels)
	for frame := range frames {
		samples[frame*channels] = .5 * math.Sin(2*math.Pi*frequency*float64(frame)/float64(rate))
	}
	return mustPCM(t, Format{rate, channels, 0}, samples)
}

func TestResampleSineFrequencyAmplitudeAndChannelIsolation(t *testing.T) {
	for _, rates := range [][2]int{{48000, 16000}, {44100, 48000}, {48000, 44100}, {16000, 48000}, {12345, 23456}, {96000, 22050}} {
		t.Run(fmt.Sprintf("%d_to_%d", rates[0], rates[1]), func(t *testing.T) {
			p := sinePCM(t, rates[0], rates[0]/4, 2, 1000)
			p.format.ChannelMask = 3
			out, err := p.Resample(rates[1], ResampleOptions{})
			if err != nil {
				t.Fatal(err)
			}
			wantFrames := (p.Frames()*rates[1] + rates[0] - 1) / rates[0]
			if out.Frames() != wantFrames || out.Format() != (Format{rates[1], 2, 3}) || out.Duration() < p.Duration() || out.Duration()-p.Duration() >= 1.0/float64(rates[1])+1e-15 {
				t.Fatal("resampled metadata/duration", out.Format(), out.Frames(), wantFrames)
			}
			squareError, maxError := 0.0, 0.0
			crossings := 0
			for frame := 256; frame < out.Frames()-256; frame++ {
				want := .5 * math.Sin(2*math.Pi*1000*float64(frame)/float64(rates[1]))
				error := out.samples[frame*2] - want
				squareError += error * error
				maxError = math.Max(maxError, math.Abs(error))
				if out.samples[frame*2] > 0 && out.samples[(frame-1)*2] <= 0 {
					crossings++
				}
			}
			rms := math.Sqrt(squareError / float64(out.Frames()-512))
			frequency := float64(crossings) * float64(rates[1]) / float64(out.Frames()-512)
			if rms > 2e-5 || maxError > 5e-5 || math.Abs(frequency-1000) > 6 {
				t.Fatalf("sine mismatch: rms=%g max=%g frequency≈%g", rms, maxError, frequency)
			}
			for frame := range out.Frames() {
				if out.samples[frame*2+1] != 0 {
					t.Fatal("resampler leaked between channels")
				}
			}
			t.Logf("1000Hz sine: rate %d→%d, RMS error %.3g, maximum error %.3g, counted frequency %.3fHz", rates[0], rates[1], rms, maxError, frequency)
		})
	}
}

func TestResampleSuppressesAliases(t *testing.T) {
	for _, test := range []struct {
		in, out   int
		frequency float64
	}{
		{48000, 16000, 9000}, {48000, 16000, 12000}, {48000, 16000, 15000},
		{96000, 22050, 16000}, {44100, 16000, 12000},
	} {
		t.Run(fmt.Sprintf("%d_to_%d_tone_%g", test.in, test.out, test.frequency), func(t *testing.T) {
			p := sinePCM(t, test.in, test.in/4, 1, test.frequency)
			out, err := p.Resample(test.out, ResampleOptions{})
			if err != nil {
				t.Fatal(err)
			}
			energy := 0.0
			for _, value := range out.samples[256 : out.Frames()-256] {
				energy += value * value
			}
			rms := math.Sqrt(energy / float64(out.Frames()-512))
			attenuation := 20 * math.Log10(rms/(.5/math.Sqrt2))
			if attenuation > -75 {
				t.Fatalf("alias suppression %.2fdB, want <= -75dB", attenuation)
			}
			t.Logf("%gHz input: residual RMS %.3g, suppression %.2fdB (interior, 256 output frames excluded at each edge)", test.frequency, rms, attenuation)
		})
	}
}

func TestResampleEdgesIdentityImpulseAndTinyClips(t *testing.T) {
	for _, edge := range []EdgeMode{EdgeReflect, EdgeHold} {
		for _, frames := range []int{1, 2, 100} {
			samples := make([]float64, frames*2)
			for i := range frames {
				samples[2*i], samples[2*i+1] = .5, -.25
			}
			p := mustPCM(t, Format{100, 2, 0}, samples)
			out, err := p.Resample(173, ResampleOptions{Edge: edge, Radius: 48, Rolloff: .9})
			if err != nil {
				t.Fatal(err)
			}
			for frame := range out.Frames() {
				closeSamples(t, out.samples[frame*2:frame*2+2], []float64{.5, -.25}, 2e-15)
			}
		}
	}
	p := mustPCM(t, Format{100, 1, 0}, []float64{1})
	zero, err := p.Resample(200, ResampleOptions{Edge: EdgeZero})
	if err != nil || zero.samples[0] >= 1 || zero.samples[1] >= .8 || zero.samples[1] <= .5 {
		t.Fatal("zero extension did not preserve its boundary transient", err)
	}
	out, err := p.Resample(100, ResampleOptions{Edge: EdgeZero, Rolloff: .5})
	if err != nil || !slices.Equal(out.samples, p.samples) {
		t.Fatal("same-rate resample filtered input", err)
	}
	out.samples[0] = 2
	if p.samples[0] != 1 {
		t.Fatal("same-rate output aliases input")
	}
	empty := mustPCM(t, Format{48000, 1, 0}, nil)
	out, err = empty.Resample(1, ResampleOptions{})
	if err != nil || out.Frames() != 0 || out.SampleRate() != 1 {
		t.Fatal("empty resample", err)
	}
	impulse := make([]float64, 960)
	impulse[480] = 1
	out, err = mustPCM(t, Format{48000, 1, 0}, impulse).Resample(16000, ResampleOptions{})
	if err != nil {
		t.Fatal(err)
	}
	peak := 0
	for frame, value := range out.samples {
		if math.Abs(value) > math.Abs(out.samples[peak]) {
			peak = frame
		}
	}
	if peak != 160 {
		t.Fatal("filter introduced latency", peak)
	}
	for frame := 1; frame < 30; frame++ {
		if math.Abs(out.samples[160-frame]-out.samples[160+frame]) > 1e-14 {
			t.Fatal("impulse response lost symmetry")
		}
	}
}

func TestResampleQualityControlsChangeTheFilter(t *testing.T) {
	energy := func(p *PCM) float64 {
		total := 0.0
		for _, sample := range p.samples[128 : p.Frames()-128] {
			total += sample * sample
		}
		return math.Sqrt(total / float64(p.Frames()-256))
	}
	// 6.5kHz remains in the default passband but is outside a .7 rolloff's
	// 5.6kHz cutoff; the option must change signal processing, not just metadata.
	passband := sinePCM(t, 48000, 4800, 1, 6500)
	wide, err := passband.Resample(16000, ResampleOptions{})
	if err != nil {
		t.Fatal(err)
	}
	narrow, err := passband.Resample(16000, ResampleOptions{Rolloff: .7})
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(energy(wide)-.5/math.Sqrt2) > 1e-4 || energy(narrow) > energy(wide)*1e-3 {
		t.Fatal("rolloff did not control the low-pass transition", energy(wide), energy(narrow))
	}
	stopband := sinePCM(t, 48000, 4800, 1, 9000)
	short, err := stopband.Resample(16000, ResampleOptions{Radius: 4})
	if err != nil {
		t.Fatal(err)
	}
	long, err := stopband.Resample(16000, ResampleOptions{Radius: 32})
	if err != nil {
		t.Fatal(err)
	}
	if energy(long) >= energy(short)*.01 {
		t.Fatal("larger kernel did not improve transition-band rejection", energy(short), energy(long))
	}
}

func TestResampleOptionsAndResourceBounds(t *testing.T) {
	p := sinePCM(t, 48000, 128, 2, 1000)
	for _, options := range []ResampleOptions{
		{Radius: -1}, {Radius: 3}, {Radius: 257}, {Rolloff: -1}, {Rolloff: 1.1},
		{Rolloff: math.NaN()}, {Rolloff: math.Inf(1)}, {Edge: 99}, {MaxTaps: -1}, {MaxTaps: 65538},
		{Limits: Limits{MaxFrames: -1}},
	} {
		if _, err := p.Resample(16000, options); !errors.Is(err, ErrInvalidInput) {
			t.Fatal("invalid resampler options accepted", options, err)
		}
	}
	for _, options := range []ResampleOptions{
		{MaxTaps: 50}, {MaxWork: 10}, {Limits: Limits{MaxSamples: 20}}, {Limits: Limits{MaxFrames: 100}},
	} {
		if _, err := p.Resample(16000, options); !errors.Is(err, ErrLimit) {
			t.Fatal("resample resource limit ignored", options, err)
		}
	}
	if _, err := p.Resample(MaxSampleRate, ResampleOptions{Limits: Limits{MaxFrames: 1000}}); !errors.Is(err, ErrLimit) {
		t.Fatal("upsampled output limit ignored", err)
	}
	if _, err := p.Resample(1, ResampleOptions{}); !errors.Is(err, ErrLimit) {
		t.Fatal("extreme downsampling silently reduced quality", err)
	}
	if _, err := p.Resample(16000, ResampleOptions{Rolloff: math.SmallestNonzeroFloat64}); !errors.Is(err, ErrLimit) {
		t.Fatal("unbounded support accepted", err)
	}
	for _, rate := range []int{-1, 0, MaxSampleRate + 1} {
		if _, err := p.Resample(rate, ResampleOptions{}); !errors.Is(err, ErrInvalidInput) {
			t.Fatal("invalid output rate accepted", err)
		}
	}
	if _, err := p.Resample(48000, ResampleOptions{MaxWork: 1}); !errors.Is(err, ErrLimit) {
		t.Fatal("identity copy ignored work limit", err)
	}
	// A small output is still actual sinc filtering, configurable up to max quality.
	if _, err := p.Resample(24000, ResampleOptions{Radius: 256, Rolloff: .85}); err != nil {
		t.Fatal("valid high-quality request rejected", err)
	}
}
