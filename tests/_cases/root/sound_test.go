package gomath_test

import (
	"errors"
	"gomath"
	"gomath/audio"
	"math"
	"reflect"
	"sync"
	"testing"
)

func TestSoundTimelineMixGainTailsAndReplay(t *testing.T) {
	f := audio.Format{SampleRate: 10, Channels: 1}
	clip, err := audio.NewPCM(f, []float64{.5, 1, -.5})
	must(t, err)
	s := gomath.NewScene()
	must(t, s.Wait(.2))
	must(t, s.AddSound(clip, -.1))
	must(t, s.AddSoundWithOptions(clip, gomath.SoundOptions{Offset: .1, GainDB: 20 * math.Log10(.5)}))
	near(t, s.Duration(), .2)
	pcm, err := s.Soundtrack(f, gomath.SoundtrackOptions{})
	must(t, err)
	want := []float64{0, .5, 1, -.25, .5, -.25}
	if !reflect.DeepEqual(pcm.Samples(), want) {
		t.Fatal("incorrect offsets/gain/tail", pcm.Samples())
	}
	replay, err := gomath.CompileReplay(s, gomath.DefaultReplayOptions())
	must(t, err)
	must(t, s.AddSound(clip, 0))
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p, err := replay.Soundtrack(f, gomath.SoundtrackOptions{})
			if err != nil || !reflect.DeepEqual(p.Samples(), want) {
				t.Error("frozen concurrent soundtrack differs", err)
			}
		}()
	}
	wg.Wait()
	samples := pcm.Samples()
	samples[0] = 99
	near(t, clip.Samples()[0], .5)
}

func TestSoundResampleAndSilence(t *testing.T) {
	f := audio.Format{SampleRate: 8000, Channels: 2, ChannelMask: 3}
	clip, err := audio.NewPCM(f, []float64{.25, -.25, .25, -.25, .25, -.25})
	must(t, err)
	s := gomath.NewScene()
	must(t, s.Wait(.002))
	must(t, s.AddSound(clip, -.002))
	out, err := s.Soundtrack(audio.Format{SampleRate: 16000, Channels: 2, ChannelMask: 3}, gomath.SoundtrackOptions{})
	must(t, err)
	if out.Frames() != 32 {
		t.Fatal("scene silence not preserved", out.Frames())
	}
	for i := range 6 {
		for c := range 2 {
			x, err := out.Sample(i, c)
			must(t, err)
			near(t, x, []float64{.25, -.25}[c])
		}
	}
	for i := 6; i < 32; i++ {
		for c := range 2 {
			x, err := out.Sample(i, c)
			must(t, err)
			near(t, x, 0)
		}
	}
	empty, err := audio.NewPCM(f, nil)
	must(t, err)
	must(t, s.AddSound(empty, 1e100))
	out, err = s.Soundtrack(f, gomath.SoundtrackOptions{})
	must(t, err)
	if out.Frames() != 16 {
		t.Fatal("empty clip extended output")
	}
}

func TestSoundValidationAndBudgets(t *testing.T) {
	f := audio.Format{SampleRate: 10, Channels: 1}
	clip, err := audio.NewPCM(f, []float64{.5, 1})
	must(t, err)
	s := gomath.NewScene()
	for _, opt := range []gomath.SoundOptions{{Offset: -1}, {Offset: math.NaN()}, {Offset: math.Inf(1)}, {GainDB: math.NaN()}, {GainDB: math.Inf(1)}, {GainDB: 100000}} {
		if err := s.AddSoundWithOptions(clip, opt); err == nil {
			t.Fatal("invalid sound accepted", opt)
		}
	}
	if s.AddSound(nil, 0) == nil || s.AddSound(&audio.PCM{}, 0) == nil {
		t.Fatal("invalid PCM accepted")
	}
	must(t, s.Wait(1))
	must(t, s.AddSound(clip, 0))
	for _, opt := range []gomath.SoundtrackOptions{
		{Resample: audio.ResampleOptions{Limits: audio.Limits{MaxSamples: 4}}},
		{Resample: audio.ResampleOptions{Limits: audio.Limits{MaxFrames: 4}}},
		{Resample: audio.ResampleOptions{Limits: audio.Limits{MaxFrames: -1}}},
		{Resample: audio.ResampleOptions{Radius: 1}},
		{MaxMixSamples: 10},
	} {
		if _, err := s.Soundtrack(f, opt); err == nil {
			t.Fatal("invalid/over-budget soundtrack accepted", opt)
		}
	}
	if _, err := s.Soundtrack(audio.Format{SampleRate: 10, Channels: 2}, gomath.SoundtrackOptions{}); err == nil {
		t.Fatal("implicit channel conversion accepted")
	}
	if _, err := s.Soundtrack(f, gomath.SoundtrackOptions{MaxMixSamples: 1}); !errors.Is(err, audio.ErrLimit) {
		t.Fatal("work limit error", err)
	}
	for i := 1; i < gomath.MaxSoundEvents; i++ {
		must(t, s.AddSound(clip, 0))
	}
	if s.AddSound(clip, 0) == nil {
		t.Fatal("event count limit ignored")
	}
	var replay *gomath.Replay
	if _, err := replay.Soundtrack(f, gomath.SoundtrackOptions{}); err == nil {
		t.Fatal("nil replay accepted")
	}
}
