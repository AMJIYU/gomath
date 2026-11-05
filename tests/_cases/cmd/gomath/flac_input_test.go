package main

import (
	"bytes"
	"fmt"
	"gomath/audio"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func cliFLACInputFixture(t *testing.T, depth, channels int) ([]byte, []float64) {
	t.Helper()
	stem := fmt.Sprintf("pcm%d_%dch_extremes_n17_b16", depth, channels)
	dir := filepath.Join("..", "..", "tests", "data", "audio", "flac_input")
	encoded, err := os.ReadFile(filepath.Join(dir, stem+".flac"))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, stem+".raw"))
	if err != nil {
		t.Fatal(err)
	}
	width := depth / 8
	values := make([]float64, len(raw)/width)
	for i := range values {
		var unsigned uint64
		for j := range width {
			unsigned |= uint64(raw[i*width+j]) << uint(8*j)
		}
		signed := int64(unsigned<<uint(64-depth)) >> uint(64-depth)
		values[i] = float64(signed) / math.Ldexp(1, depth-1)
	}
	return encoded, values
}

func TestDemoExpandedFLACDepthChannelsAndWAVClock(t *testing.T) {
	for _, depth := range []int{8, 32} {
		for _, channels := range []int{1, 2, 8} {
			t.Run(fmt.Sprintf("%dbit-%dch", depth, channels), func(t *testing.T) {
				encoded, want := cliFLACInputFixture(t, depth, channels)
				dir := t.TempDir()
				input, output := filepath.Join(dir, "input.aiff"), filepath.Join(dir, "output.wav")
				if err := os.WriteFile(input, encoded, 0600); err != nil {
					t.Fatal(err)
				}
				pcm, err := readAudioInput(input, true)
				if err != nil {
					t.Fatal(err)
				}
				if pcm.Channels() != channels || pcm.Frames() != 17 || pcm.Format().ChannelMask != 0 {
					t.Fatal("FLAC indexed format changed")
				}
				for i, value := range pcm.Samples() {
					if value != want[i] {
						t.Fatal("FLAC signature input differs from independent raw PCM", i)
					}
				}
				if err = demo([]string{"-scene", "reference", "-format", "wav", "-audio", input, "-audio-rate", "48000", "-audio-channels", strconv.Itoa(channels), "-discard-audio-metadata", "-out", output}); err != nil {
					t.Fatal(err)
				}
				data, err := os.ReadFile(output)
				if err != nil {
					t.Fatal(err)
				}
				result, err := audio.ReadWAV(bytes.NewReader(data), audio.WAVReadOptions{})
				if err != nil {
					t.Fatal(err)
				}
				if result.Frames() != 48000 || result.Channels() != channels || result.SampleRate() != 48000 {
					t.Fatal("FLAC input changed complete scene clock", result.Format(), result.Frames())
				}
				for i, value := range result.Samples() {
					expected := 0.0
					if i < len(want) {
						expected = math.Min(math.Round(want[i]*32768), 32767) / 32768
					}
					if value != expected {
						t.Fatal("FLAC to PCM16 WAV sample differs", i, value, expected)
					}
				}
			})
		}
	}
}

func TestDemoExpandedFLACRejectsUnsupportedRoutingAndCorruptionAtomically(t *testing.T) {
	encoded, _ := cliFLACInputFixture(t, 32, 8)
	for _, kind := range []string{"remix", "mismatch", "flac-writer", "mov-writer", "crc", "trailing"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			input, output := filepath.Join(dir, "input.flac"), filepath.Join(dir, "result")
			data := append([]byte(nil), encoded...)
			args := []string{"-scene", "reference", "-format", "wav", "-audio", input, "-audio-rate", "48000", "-audio-channels", "8", "-discard-audio-metadata", "-out", output}
			switch kind {
			case "remix":
				args[9] = "2"
				args = append(args, "-audio-remix")
			case "mismatch":
				args[9] = "2"
			case "flac-writer":
				args[3] = "flac"
			case "mov-writer":
				args[3] = "mov"
			case "crc":
				data[len(data)-1] ^= 1
			case "trailing":
				data = append(data, 1)
			}
			if err := os.WriteFile(input, data, 0600); err != nil {
				t.Fatal(err)
			}
			if err := demo(args); err == nil {
				t.Fatal("unsupported/corrupt FLAC CLI input accepted", kind)
			}
			if _, err := os.Stat(output); !os.IsNotExist(err) {
				t.Fatal("failed FLAC CLI transaction published output", kind, err)
			}
		})
	}
}
