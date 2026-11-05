package main

import (
	"bytes"
	"gomath/audio"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
)

func TestDemoExplicitAudioRemix(t *testing.T) {
	for _, tc := range []struct {
		name       string
		input, out int
		mask       uint32
		samples    []float64
		want       []float64
	}{
		{"mono-to-stereo", 1, 2, 4, []float64{.25, -.5, .125}, []float64{.25, .25, -.5, -.5, .125, .125}},
		{"stereo-to-mono", 2, 1, 3, []float64{.25, .5, -.5, .25, .125, -.125}, []float64{.375, -.125, 0}},
		{"indexed-mono-to-stereo", 1, 2, 0, []float64{.25, -.5}, []float64{.25, .25, -.5, -.5}},
		{"mono-unchanged", 1, 1, 4, []float64{.25, -.5}, []float64{.25, -.5}},
		{"stereo-unchanged", 2, 2, 0, []float64{.25, -.5, .125, .375}, []float64{.25, -.5, .125, .375}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pcm, err := audio.NewPCM(audio.Format{SampleRate: 48000, Channels: tc.input, ChannelMask: tc.mask}, tc.samples)
			if err != nil {
				t.Fatal(err)
			}
			var encoded bytes.Buffer
			if err = audio.WriteWAV(&encoded, pcm, audio.WAVWriteOptions{Encoding: audio.Float64}); err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			input, output := filepath.Join(dir, "input.wav"), filepath.Join(dir, "output.wav")
			if err = os.WriteFile(input, encoded.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
			args := []string{"-scene", "reference", "-format", "wav", "-audio", input, "-audio-channels", strconv.Itoa(tc.out), "-out", output}
			if tc.input != tc.out {
				if err = demo(args); err == nil {
					t.Fatal("channel conversion happened without explicit opt-in")
				}
				if _, err = os.Stat(output); !os.IsNotExist(err) {
					t.Fatal("failed conversion published output", err)
				}
			}
			if err = demo(append(args, "-audio-remix")); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(output)
			if err != nil {
				t.Fatal(err)
			}
			got, err := audio.ReadWAV(bytes.NewReader(data), audio.WAVReadOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if got.Channels() != tc.out || got.SampleRate() != 48000 || got.Frames() != 48000 {
				t.Fatal("remix changed scene clock or output format", got.Format(), got.Frames())
			}
			samples := got.Samples()
			if !reflect.DeepEqual(samples[:len(tc.want)], tc.want) {
				t.Fatalf("remix samples = %v, want %v", samples[:len(tc.want)], tc.want)
			}
			for _, sample := range samples[len(tc.want):] {
				if sample != 0 {
					t.Fatal("remix introduced a sound tail")
				}
			}
			if !reflect.DeepEqual(pcm.Samples(), tc.samples) {
				t.Fatal("remix changed input")
			}
		})
	}
}

func TestAudioRemixRejectsAmbiguousLayouts(t *testing.T) {
	for _, format := range []audio.Format{
		{SampleRate: 48000, Channels: 1, ChannelMask: 1},
		{SampleRate: 48000, Channels: 2, ChannelMask: 12},
		{SampleRate: 48000, Channels: 3},
	} {
		pcm, err := audio.NewPCM(format, make([]float64, format.Channels))
		if err != nil {
			t.Fatal(err)
		}
		for _, channels := range []int{1, 2} {
			if got, err := remixAudioInput(pcm, channels); err == nil || got != nil {
				t.Fatal("ambiguous input layout accepted", format, channels)
			}
		}
	}
	for _, pcm := range []*audio.PCM{nil, {}} {
		if got, err := remixAudioInput(pcm, 2); err == nil || got != nil {
			t.Fatal("invalid PCM accepted")
		}
	}
	pcm, _ := audio.NewPCM(audio.Format{SampleRate: 48000, Channels: 1}, nil)
	if got, err := remixAudioInput(pcm, 3); err == nil || got != nil {
		t.Fatal("unsupported output channels accepted")
	}
}
