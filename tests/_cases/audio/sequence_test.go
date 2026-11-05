package audio

import (
	"errors"
	"math"
	"testing"
)

func TestConcatOrderAndOwnership(t *testing.T) {
	format := Format{48000, 2, 3}
	p := mustPCM(t, format, []float64{2, -3, .5, -.25})
	other := mustPCM(t, format, []float64{4, -5})
	empty := mustPCM(t, format, nil)
	clips := []*PCM{empty, other, p}
	got, err := p.Concat(clips, Limits{MaxFrames: 5, MaxSamples: 10})
	if err != nil || got.Format() != format || got.Frames() != 5 {
		t.Fatal("concat format/length", err)
	}
	want := []float64{2, -3, .5, -.25, 4, -5, 2, -3, .5, -.25}
	closeSamples(t, got.samples, want, 0)
	clips[1] = empty
	closeSamples(t, got.samples, want, 0)
	got.samples[0], got.samples[4], got.samples[6] = 99, 99, 99
	closeSamples(t, p.samples, []float64{2, -3, .5, -.25}, 0)
	closeSamples(t, other.samples, []float64{4, -5}, 0)
	copy, err := p.Concat(nil, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	copy.samples[0] = 99
	if p.samples[0] != 2 {
		t.Fatal("single clip concat aliases receiver")
	}
	got, err = empty.Concat([]*PCM{empty, p, empty}, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	closeSamples(t, got.samples, p.samples, 0)
	got.samples[0] = 99
	if p.samples[0] != 2 {
		t.Fatal("empty receiver aliases appended clip")
	}
	if got, err := empty.Concat([]*PCM{empty}, Limits{}); err != nil || got.Frames() != 0 || got.Format() != format {
		t.Fatal("empty concat", err)
	}
}

func TestConcatValidationAndBounds(t *testing.T) {
	p := mustPCM(t, Format{48000, 2, 3}, []float64{1, -1, 2, -2})
	for _, other := range []*PCM{nil, {}, mustPCM(t, Format{44100, 2, 3}, nil), mustPCM(t, Format{48000, 1, 4}, nil), mustPCM(t, Format{48000, 2, 0}, nil), mustPCM(t, Format{48000, 2, 0x30}, nil)} {
		if got, err := p.Concat([]*PCM{other}, Limits{}); got != nil || !errors.Is(err, ErrInvalidInput) {
			t.Fatal("invalid or different empty format accepted", got, err)
		}
	}
	for _, bad := range []*PCM{nil, {}} {
		if _, err := bad.Concat(nil, Limits{}); !errors.Is(err, ErrInvalidInput) {
			t.Fatal("invalid concat receiver", err)
		}
	}
	for _, limits := range []Limits{{MaxFrames: -1}, {MaxSamples: -1}, {MaxBytes: -1}, {MaxChunks: -1}} {
		if _, err := p.Concat(nil, limits); !errors.Is(err, ErrInvalidInput) {
			t.Fatal("negative limits", limits, err)
		}
	}
	for _, test := range []struct {
		clips []*PCM
		limit Limits
	}{
		{nil, Limits{MaxFrames: 1}},
		{nil, Limits{MaxSamples: 3}},
		{[]*PCM{p}, Limits{MaxFrames: 3}},
		{[]*PCM{p}, Limits{MaxSamples: 7}},
	} {
		if _, err := p.Concat(test.clips, test.limit); !errors.Is(err, ErrLimit) {
			t.Fatal("concat output bound ignored", test.limit, err)
		}
	}
	empty := mustPCM(t, p.Format(), nil)
	clips := make([]*PCM, MaxConcatClips-1)
	for i := range clips {
		clips[i] = empty
	}
	if got, err := empty.Concat(clips, Limits{MaxChunks: 1, MaxBytes: 1}); err != nil || got.Frames() != 0 {
		t.Fatal("exact clip limit rejected or encoded limit applied", err)
	}
	if _, err := empty.Concat(append(clips, empty), Limits{}); !errors.Is(err, ErrLimit) {
		t.Fatal("empty clip count bypasses bound", err)
	}
}

func TestRepeatOrderOwnershipAndEmpty(t *testing.T) {
	format := Format{48000, 2, 3}
	p := mustPCM(t, format, []float64{2, -3, .5, -.25})
	for _, count := range []int{0, 1, 2, 3, 5, 17} {
		got, err := p.Repeat(count, Limits{})
		if err != nil || got.Format() != format || got.Frames() != 2*count {
			t.Fatal("repeat format/length", count, err)
		}
		for i, value := range got.samples {
			if value != p.samples[i%len(p.samples)] {
				t.Fatalf("count %d sample %d differs", count, i)
			}
		}
		for i := range got.samples {
			got.samples[i] = 99
		}
		closeSamples(t, p.samples, []float64{2, -3, .5, -.25}, 0)
	}
	empty := mustPCM(t, format, nil)
	for _, count := range []int{0, 1, int(^uint(0) >> 1)} {
		got, err := empty.Repeat(count, Limits{MaxFrames: 1, MaxSamples: 1})
		if err != nil || got.Frames() != 0 || got.Format() != format {
			t.Fatal("empty repeat should finish without count-sized loop", count, err)
		}
	}
	// Bounds apply to output, so zero times can discard a larger input.
	if got, err := p.Repeat(0, Limits{MaxFrames: 1, MaxSamples: 1}); err != nil || got.Frames() != 0 {
		t.Fatal("zero count", err)
	}
	// Copies preserve finite extrema and headroom bit for bit.
	samples := []float64{math.MaxFloat64, -math.MaxFloat64, math.Copysign(0, -1), math.SmallestNonzeroFloat64}
	extremes := mustPCM(t, Format{1, 1, 0}, samples)
	got, err := extremes.Repeat(3, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	for i, v := range got.samples {
		if math.Float64bits(v) != math.Float64bits(samples[i%len(samples)]) {
			t.Fatal("copy changed sample bits", i)
		}
	}
}

func TestRepeatValidationAndOverflow(t *testing.T) {
	p := mustPCM(t, Format{48000, 2, 3}, []float64{1, -1, 2, -2})
	for _, count := range []int{-1, -int(^uint(0)>>1) - 1} {
		if got, err := p.Repeat(count, Limits{}); got != nil || !errors.Is(err, ErrInvalidInput) {
			t.Fatal("negative count accepted", got, err)
		}
	}
	for _, bad := range []*PCM{nil, {}} {
		if _, err := bad.Repeat(0, Limits{}); !errors.Is(err, ErrInvalidInput) {
			t.Fatal("zero bypassed invalid receiver", err)
		}
	}
	for _, limits := range []Limits{{MaxFrames: -1}, {MaxSamples: -1}, {MaxBytes: -1}, {MaxChunks: -1}} {
		if _, err := p.Repeat(0, limits); !errors.Is(err, ErrInvalidInput) {
			t.Fatal("zero bypassed invalid limits", limits, err)
		}
	}
	maxInt := int(^uint(0) >> 1)
	for _, test := range []struct {
		count int
		limit Limits
	}{
		{2, Limits{MaxFrames: 3}},
		{2, Limits{MaxSamples: 7}},
		{maxInt, Limits{}},
		{maxInt, Limits{MaxFrames: maxInt, MaxSamples: maxInt}},     // product overflow
		{maxInt / 2, Limits{MaxFrames: maxInt, MaxSamples: maxInt}}, // sample/byte overflow
	} {
		if got, err := p.Repeat(test.count, test.limit); got != nil || !errors.Is(err, ErrLimit) {
			t.Fatal("repeat count/storage bound ignored", test, got, err)
		}
	}
	// Isolate the float64 byte-size bound before attempting an allocation.
	mono := mustPCM(t, Format{1, 1, 0}, []float64{1})
	if _, err := mono.Repeat(maxInt/8+1, Limits{MaxFrames: maxInt, MaxSamples: maxInt}); !errors.Is(err, ErrLimit) {
		t.Fatal("architecture storage bound ignored", err)
	}
	if got, err := p.Repeat(2, Limits{MaxFrames: 4, MaxSamples: 8, MaxBytes: 1, MaxChunks: 1}); err != nil || got.Frames() != 4 {
		t.Fatal("exact repeat bounds", err)
	}
}
