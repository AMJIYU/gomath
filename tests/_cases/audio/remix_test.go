package audio

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"slices"
	"sync"
	"testing"
)

func TestRemixRationalReference(t *testing.T) {
	data, err := os.ReadFile("remix_reference.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Cases []struct {
			Name     string
			Matrix   [][]float64
			Samples  []float64
			Expected []float64
		}
	}
	if err := json.Unmarshal(data, &fixture); err != nil || len(fixture.Cases) != 3 {
		t.Fatal("invalid rational reference", err)
	}
	for _, test := range fixture.Cases {
		t.Run(test.Name, func(t *testing.T) {
			p := mustPCM(t, Format{48000, len(test.Matrix[0]), 0}, test.Samples)
			output := Format{48000, len(test.Matrix), 0}
			got, err := p.Remix(output, test.Matrix, RemixOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if got.Format() != output || got.Frames() != p.Frames() {
				t.Fatal("remix format or duration differs")
			}
			closeSamples(t, got.Samples(), test.Expected, 0)
			// Changing the caller's matrix after return changes neither input
			// nor result. Every result sample has its own backing storage.
			test.Matrix[0][0] = 99
			closeSamples(t, got.samples, test.Expected, 0)
			got.samples[0] = 99
			closeSamples(t, p.samples, test.Samples, 0)
		})
	}
}

func TestRemixLayoutsAndCopies(t *testing.T) {
	p := mustPCM(t, Format{48000, 2, 3}, []float64{2, -3, .5, -.25})
	for _, output := range []Format{{48000, 2, 0}, {48000, 2, 3}, {48000, 2, 0x30}} {
		got, err := p.Remix(output, [][]float64{{0, 1}, {1, 0}}, RemixOptions{})
		if err != nil || got.Format() != output {
			t.Fatal("explicit output mask not preserved", output, err)
		}
		closeSamples(t, got.samples, []float64{-3, 2, -.25, .5}, 0)
	}
	mono, err := p.Remix(Format{48000, 1, 4}, [][]float64{{.5, .5}}, RemixOptions{})
	if err != nil {
		t.Fatal(err)
	}
	closeSamples(t, mono.samples, []float64{-.5, .125}, 0)
	identity, err := p.Remix(p.Format(), [][]float64{{1, 0}, {0, 1}}, RemixOptions{})
	if err != nil {
		t.Fatal(err)
	}
	identity.samples[0] = 99
	if p.samples[0] != 2 {
		t.Fatal("identity matrix aliases source")
	}
	// A zero mask input can also be explicitly assigned speakers by caller.
	indexed := mustPCM(t, Format{48000, 1, 0}, []float64{2})
	got, err := indexed.Remix(Format{48000, 2, 3}, [][]float64{{2}, {-3}}, RemixOptions{})
	if err != nil {
		t.Fatal(err)
	}
	closeSamples(t, got.samples, []float64{4, -6}, 0)
}

func TestRemixValidationAndBounds(t *testing.T) {
	p := mustPCM(t, Format{48000, 2, 3}, []float64{1, -1, .5, -.5})
	output := Format{48000, 1, 4}
	for _, matrix := range [][][]float64{nil, {{1}}, {{1, 0, 0}}, {{1, 0}, {0, 1}}, {nil}, {{math.NaN(), 0}}, {{0, math.Inf(1)}}, {{math.Inf(-1), 0}}} {
		if got, err := p.Remix(output, matrix, RemixOptions{}); got != nil || !errors.Is(err, ErrInvalidInput) {
			t.Fatal("invalid matrix accepted", matrix, got, err)
		}
	}
	for _, format := range []Format{{}, {44100, 1, 4}, {48000, 1, 3}, {48000, MaxChannels + 1, 0}} {
		if _, err := p.Remix(format, [][]float64{{1, 0}}, RemixOptions{}); !errors.Is(err, ErrInvalidInput) {
			t.Fatal("invalid format accepted", format, err)
		}
	}
	for _, bad := range []*PCM{nil, {}} {
		if _, err := bad.Remix(output, [][]float64{{1, 0}}, RemixOptions{}); !errors.Is(err, ErrInvalidInput) {
			t.Fatal("invalid receiver", err)
		}
	}
	for _, limits := range []Limits{{MaxFrames: -1}, {MaxSamples: -1}, {MaxBytes: -1}, {MaxChunks: -1}} {
		if _, err := p.Remix(output, [][]float64{{1, 0}}, RemixOptions{Limits: limits}); !errors.Is(err, ErrInvalidInput) {
			t.Fatal("negative limit", limits, err)
		}
	}
	for _, options := range []RemixOptions{{Limits: Limits{MaxFrames: 1}}, {Limits: Limits{MaxSamples: 1}}, {MaxWork: 5}} {
		if _, err := p.Remix(output, [][]float64{{1, 0}}, options); !errors.Is(err, ErrLimit) {
			t.Fatal("bound ignored", options, err)
		}
	}
	// Cost = 2 coefficients * (1 validation + 2 frames) = 6; encoded
	// byte/chunk fields are normalized but do not constrain PCM operations.
	got, err := p.Remix(output, [][]float64{{1, 0}}, RemixOptions{MaxWork: 6, Limits: Limits{MaxFrames: 2, MaxSamples: 2, MaxBytes: 1, MaxChunks: 1}})
	if err != nil {
		t.Fatal("exact budget rejected", err)
	}
	closeSamples(t, got.samples, []float64{1, .5}, 0)
	// Limits concern output storage: a narrower output may use fewer samples.
	empty := mustPCM(t, p.Format(), nil)
	if got, err := empty.Remix(output, [][]float64{{1, 0}}, RemixOptions{MaxWork: 2}); err != nil || got.Frames() != 0 || got.Format() != output {
		t.Fatal("empty remix", err)
	}
	if _, err := empty.Remix(output, [][]float64{{1, 0}}, RemixOptions{MaxWork: 1}); !errors.Is(err, ErrLimit) {
		t.Fatal("empty bypassed coefficient work", err)
	}
	if _, err := empty.Remix(output, [][]float64{{math.NaN(), 0}}, RemixOptions{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatal("empty bypassed validation", err)
	}
	// The largest legal matrix is still bounded and retains channel indices.
	many := mustPCM(t, Format{48000, MaxChannels, 0}, make([]float64, MaxChannels))
	matrix := make([][]float64, MaxChannels)
	for c := range matrix {
		matrix[c] = make([]float64, MaxChannels)
		matrix[c][c] = 1
	}
	if got, err := many.Remix(many.Format(), matrix, RemixOptions{MaxWork: 8192}); err != nil || got.Channels() != MaxChannels {
		t.Fatal("maximum channel dimensions", err)
	}
	if _, err := many.Remix(many.Format(), matrix, RemixOptions{MaxWork: 8191}); !errors.Is(err, ErrLimit) {
		t.Fatal("maximum matrix work limit ignored", err)
	}
	// Expansion must bound output storage as well as frame count.
	if _, err := p.Remix(Format{48000, 3, 0}, [][]float64{{1, 0}, {0, 1}, {1, 1}}, RemixOptions{Limits: Limits{MaxSamples: 5}}); !errors.Is(err, ErrLimit) {
		t.Fatal("expanded storage bound ignored", err)
	}
}

func TestRemixOverflowAndRounding(t *testing.T) {
	for _, test := range []struct {
		samples []float64
		weights []float64
	}{
		{[]float64{math.MaxFloat64}, []float64{2}},                                          // product
		{[]float64{math.MaxFloat64, math.MaxFloat64}, []float64{1, 1}},                      // sum
		{[]float64{math.MaxFloat64, math.MaxFloat64, -math.MaxFloat64}, []float64{1, 1, 1}}, // later cancellation
		{[]float64{-math.MaxFloat64}, []float64{2}},
	} {
		p := mustPCM(t, Format{1, len(test.samples), 0}, test.samples)
		if got, err := p.Remix(Format{1, 1, 0}, [][]float64{test.weights}, RemixOptions{}); got != nil || !errors.Is(err, ErrInvalidInput) {
			t.Fatal("nonfinite result accepted", got, err)
		}
		closeSamples(t, p.samples, test.samples, 0)
	}
	// Without separate product rounding, a fused multiply/add could return
	// -2^-54 for the second term instead of this specified exact zero.
	p := mustPCM(t, Format{1, 2, 0}, []float64{-1, 1 + math.Ldexp(1, -27)})
	got, err := p.Remix(Format{1, 1, 0}, [][]float64{{1, 1 - math.Ldexp(1, -27)}}, RemixOptions{})
	if err != nil || got.samples[0] != 0 {
		t.Fatal("multiply/add not separately rounded", err)
	}
}

func TestConcurrentRemixAndSequence(t *testing.T) {
	p := mustPCM(t, Format{48000, 2, 3}, []float64{1, -2, 3, -4, 5, -6})
	matrix := [][]float64{{2, -.5}, {-.25, 1}}
	want := []float64{3, -2.25, 8, -4.75, 13, -7.25}
	var wg sync.WaitGroup
	for range 12 {
		wg.Go(func() {
			for range 20 {
				got, err := p.Remix(p.Format(), matrix, RemixOptions{})
				if err != nil || !slices.Equal(got.samples, want) {
					t.Errorf("concurrent remix: %v", err)
					return
				}
				joined, err := p.Concat([]*PCM{p, p}, Limits{})
				if err != nil {
					t.Error(err)
					return
				}
				repeated, err := p.Repeat(3, Limits{})
				if err != nil || !slices.Equal(joined.samples, repeated.samples) {
					t.Errorf("concurrent sequence: %v", err)
					return
				}
				got.samples[0], joined.samples[0], repeated.samples[0] = 99, 99, 99
			}
		})
	}
	wg.Wait()
	closeSamples(t, p.samples, []float64{1, -2, 3, -4, 5, -6}, 0)
	closeSamples(t, matrix[0], []float64{2, -.5}, 0)
	closeSamples(t, matrix[1], []float64{-.25, 1}, 0)
}
