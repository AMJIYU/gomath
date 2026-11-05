package audio

import (
	"bytes"
	"errors"
	"math"
	"slices"
	"sync"
	"testing"
)

func mustPCM(t testing.TB, format Format, samples []float64) *PCM {
	t.Helper()
	p, err := NewPCM(format, samples)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func closeSamples(t testing.TB, got, want []float64, tolerance float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("sample count %d, want %d", len(got), len(want))
	}
	for i := range got {
		if !finite(got[i]) || math.Abs(got[i]-want[i]) > tolerance {
			t.Fatalf("sample %d: %.17g, want %.17g ± %g", i, got[i], want[i], tolerance)
		}
	}
}

func TestPCMOwnershipAndLayout(t *testing.T) {
	samples := []float64{1, -1, .5, -.5, .25, -.25}
	format := Format{SampleRate: 48000, Channels: 2, ChannelMask: 3}
	p := mustPCM(t, format, samples)
	samples[0] = 99
	copy := p.Samples()
	copy[1] = 99
	format.Channels = 1
	if p.Format() != (Format{48000, 2, 3}) || p.SampleRate() != 48000 || p.Channels() != 2 || p.Frames() != 3 || p.Duration() != 3.0/48000 {
		t.Fatal("PCM metadata changed")
	}
	for frame := range 3 {
		for channel := range 2 {
			got, err := p.Sample(frame, channel)
			if err != nil || got != []float64{1, -1, .5, -.5, .25, -.25}[frame*2+channel] {
				t.Fatal("interleaving or ownership failed", frame, channel, got, err)
			}
		}
	}
	for _, index := range [][2]int{{-1, 0}, {3, 0}, {0, -1}, {0, 2}} {
		if _, err := p.Sample(index[0], index[1]); !errors.Is(err, ErrInvalidInput) {
			t.Fatal("invalid sample index accepted", index, err)
		}
	}
	trim, err := p.TrimFrames(1, 3)
	if err != nil {
		t.Fatal(err)
	}
	trim.samples[0] = 17 // Private test verifies actual independent storage.
	if p.samples[2] != .5 {
		t.Fatal("trim aliases source")
	}
	empty := mustPCM(t, Format{SampleRate: 1, Channels: 1}, nil)
	if empty.Frames() != 0 || empty.Duration() != 0 {
		t.Fatal("invalid empty PCM")
	}
}

func TestPCMValidationAndLimits(t *testing.T) {
	for _, format := range []Format{
		{}, {SampleRate: -1, Channels: 1}, {SampleRate: MaxSampleRate + 1, Channels: 1},
		{SampleRate: 1}, {SampleRate: 1, Channels: MaxChannels + 1},
		{SampleRate: 1, Channels: 2, ChannelMask: 1}, {SampleRate: 1, Channels: 1, ChannelMask: 1 << 31},
	} {
		if _, err := NewPCM(format, nil); !errors.Is(err, ErrInvalidInput) {
			t.Fatal("invalid format accepted", format, err)
		}
	}
	for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if _, err := NewPCM(Format{1, 1, 0}, []float64{value}); !errors.Is(err, ErrInvalidInput) {
			t.Fatal("nonfinite sample accepted", err)
		}
	}
	if _, err := NewPCM(Format{1, 2, 0}, []float64{0}); !errors.Is(err, ErrInvalidInput) {
		t.Fatal("partial frame accepted", err)
	}
	for _, limits := range []Limits{{MaxFrames: 1}, {MaxSamples: 3}} {
		if _, err := NewPCMWithLimits(Format{1, 2, 0}, make([]float64, 4), limits); !errors.Is(err, ErrLimit) {
			t.Fatal("PCM storage limit ignored", err)
		}
	}
	for _, limits := range []Limits{{MaxFrames: -1}, {MaxSamples: -1}, {MaxBytes: -1}, {MaxChunks: -1}} {
		if _, err := NewPCMWithLimits(Format{1, 1, 0}, nil, limits); !errors.Is(err, ErrInvalidInput) {
			t.Fatal("negative limit accepted", err)
		}
	}
	if _, err := NewPCMWithLimits(Format{1, 2, 0}, make([]float64, 4), Limits{MaxFrames: 2, MaxSamples: 4}); err != nil {
		t.Fatal("exact bound rejected", err)
	}
	// PCM headroom is finite rather than restricted to integer WAVE's range.
	mustPCM(t, Format{1, 1, 0}, []float64{-2, math.MaxFloat64})
	for _, p := range []*PCM{nil, {}} {
		if _, err := p.Gain(1); !errors.Is(err, ErrInvalidInput) {
			t.Fatal("invalid PCM receiver accepted", err)
		}
	}
}

func TestConcurrentIndependentOperations(t *testing.T) {
	samples := make([]float64, 2048)
	for i := 0; i < len(samples)/2; i++ {
		samples[i*2] = .3 * math.Sin(2*math.Pi*440*float64(i)/48000)
		samples[i*2+1] = -.5 * samples[i*2]
	}
	p := mustPCM(t, Format{48000, 2, 3}, samples)
	want, err := p.Resample(32000, ResampleOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 12 {
		wg.Go(func() {
			for range 3 {
				got, err := p.Resample(32000, ResampleOptions{})
				if err != nil || !slices.Equal(got.samples, want.samples) {
					t.Errorf("concurrent resampling differs: %v", err)
					return
				}
				got, err = got.Envelope([]GainPoint{{Time: 0, Gain: .5}, {Time: got.Duration(), Gain: .1}})
				if err != nil {
					t.Error(err)
					return
				}
				var buf bytes.Buffer
				if err := WriteWAV(&buf, got, WAVWriteOptions{Encoding: Float64}); err != nil {
					t.Error(err)
					return
				}
				decoded, err := ReadWAV(&buf, WAVReadOptions{})
				if err != nil || !slices.Equal(decoded.samples, got.samples) {
					t.Errorf("concurrent round trip differs: %v", err)
					return
				}
				got.samples[0] = 100 // A result remains exclusive to its caller.
				publicCopy := p.Samples()
				publicCopy[0] = 100
			}
		})
	}
	wg.Wait()
	closeSamples(t, p.samples, samples, 0)
}
