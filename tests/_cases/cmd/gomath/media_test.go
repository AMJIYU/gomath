package main

import (
	"bytes"
	"gomath/audio"
	"gomath/subtitle"
	"os"
	"path/filepath"
	"testing"
)

func TestDemoMediaEndToEnd(t *testing.T) {
	for _, kind := range []string{"wav", "srt", "vtt"} {
		t.Run(kind, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "composition."+kind)
			if err := demo([]string{"-scene", "composition", "-format", kind, "-out", path}); err != nil {
				t.Fatal(err)
			}
			f, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			if kind == "wav" {
				pcm, err := audio.ReadWAV(f, audio.WAVReadOptions{})
				if err != nil {
					t.Fatal(err)
				}
				if pcm.Channels() != 2 || pcm.SampleRate() != 48000 || pcm.Frames() != 240000 {
					t.Fatal("CLI audio timing/format differs", pcm.Format(), pcm.Frames())
				}
				return
			}
			var track *subtitle.Track
			if kind == "srt" {
				track, err = subtitle.ReadSRT(f, subtitle.Options{})
			} else {
				track, err = subtitle.ReadWebVTT(f, subtitle.Options{})
			}
			if err != nil {
				t.Fatal(err)
			}
			if track.Len() != 2 {
				t.Fatal("CLI caption count differs", track.Len())
			}
		})
	}
}

func TestDemoRejectsIncompatibleMediaFlags(t *testing.T) {
	for _, args := range [][]string{
		{"-format", "mp4", "-codec", "pcm", "-mode-search"},
		{"-format", "png", "-mode-search"},
		{"-format", "mp4", "-audio", "does-not-exist.wav"},
		{"-format", "mp4", "-qp", "52"},
		{"-format", "mp4", "-codec", "unknown"},
		{"-format", "png", "-inter-residual"},
		{"-format", "mp4", "-codec", "intra", "-inter-residual"},
		{"-format", "mov", "-codec", "pcm", "-inter-residual"},
		{"-format", "wav", "-audio-remix"},
		{"-format", "wav", "-audio", "does-not-exist.wav", "-audio-remix", "-audio-channels", "3"},
		{"-format", "png", "-audio", "does-not-exist.wav", "-audio-remix"},
		{"-format", "png", "-motion-radius", "1"},
		{"-format", "mp4", "-codec", "predictive", "-motion-radius", "1"},
		{"-format", "mp4", "-codec", "predictive", "-inter-residual", "-motion-radius", "-1"},
		{"-format", "mp4", "-codec", "predictive", "-inter-residual", "-motion-radius", "5"},
		{"-format", "mov", "-codec", "intra", "-inter-residual", "-motion-radius", "1"},
	} {
		path := filepath.Join(t.TempDir(), "invalid-output")
		args = append(args, "-out", path)
		if err := demo(args); err == nil {
			t.Fatal("incompatible flags accepted", args)
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatal("invalid invocation published output", err)
		}
	}
}

func TestDemoNativeFLACInputAndMetadataPolicy(t *testing.T) {
	pcm, err := audio.NewPCM(audio.Format{SampleRate: 48000, Channels: 1}, []float64{.25, -.125, .375, -.5, 0, .125})
	if err != nil {
		t.Fatal(err)
	}
	var flac, wav bytes.Buffer
	if err = audio.WriteFLAC(&flac, pcm, audio.FLACWriteOptions{Encoding: audio.PCM24}); err != nil {
		t.Fatal(err)
	}
	if err = audio.WriteWAV(&wav, pcm, audio.WAVWriteOptions{Encoding: audio.PCM24}); err != nil {
		t.Fatal(err)
	}
	metadata := append([]byte(nil), flac.Bytes()[:42]...)
	metadata[4] &^= 128                        // STREAMINFO is no longer the last metadata block.
	metadata = append(metadata, 0x84, 0, 0, 8) // Last VORBIS_COMMENT, empty vendor/list.
	metadata = append(metadata, make([]byte, 8)...)
	metadata = append(metadata, flac.Bytes()[42:]...)
	dir := t.TempDir()
	var outputs [][]byte
	for _, c := range []struct {
		name    string
		data    []byte
		discard bool
	}{
		{"native-with-wrong-extension.wav", flac.Bytes(), false},
		{"native-with-wrong-extension.flac", wav.Bytes(), false},
		{"with-comment.flac", metadata, true},
	} {
		input, output := filepath.Join(dir, c.name), filepath.Join(dir, c.name+".out.wav")
		if err = os.WriteFile(input, c.data, 0600); err != nil {
			t.Fatal(err)
		}
		args := []string{"-scene", "reference", "-format", "wav", "-audio", input, "-audio-channels", "1", "-out", output}
		if c.discard {
			if err := demo(args); err == nil {
				t.Fatal("FLAC metadata silently discarded")
			}
			if _, err := os.Stat(output); !os.IsNotExist(err) {
				t.Fatal("failed import published output", err)
			}
			args = append(args, "-discard-audio-metadata")
		}
		if err := demo(args); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(output)
		if err != nil {
			t.Fatal(err)
		}
		outputs = append(outputs, data)
	}
	if !bytes.Equal(outputs[0], outputs[1]) || !bytes.Equal(outputs[0], outputs[2]) {
		t.Fatal("WAV/FLAC inputs produce different soundtrack samples")
	}
	got, err := audio.ReadWAV(bytes.NewReader(outputs[0]), audio.WAVReadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Frames() != 48000 || got.Channels() != 1 {
		t.Fatal("input changed scene audio clock")
	}
	values := got.Samples()
	if values[0] != .25 || values[1] != -.125 || values[3] != -.5 || values[100] != 0 {
		t.Fatal("input samples were dropped, shifted or changed", values[:6])
	}
	for _, data := range [][]byte{nil, []byte("bad!"), flac.Bytes()[:len(flac.Bytes())-1]} {
		input, output := filepath.Join(t.TempDir(), "bad-input"), filepath.Join(t.TempDir(), "output.wav")
		if err = os.WriteFile(input, data, 0600); err != nil {
			t.Fatal(err)
		}
		if err = demo([]string{"-scene", "reference", "-format", "wav", "-audio", input, "-out", output}); err == nil {
			t.Fatal("invalid audio imported")
		}
		if _, err = os.Stat(output); !os.IsNotExist(err) {
			t.Fatal("invalid audio published output", err)
		}
	}
}
