package main

import (
	"bytes"
	"encoding/binary"
	"gomath/audio"
	"os"
	"path/filepath"
	"testing"
)

func cliAIFFChunk(id string, data []byte) []byte {
	var out bytes.Buffer
	out.WriteString(id)
	binary.Write(&out, binary.BigEndian, uint32(len(data)))
	out.Write(data)
	if len(data)%2 != 0 {
		out.WriteByte(0)
	}
	return out.Bytes()
}
func cliAIFFFixture(form, compression string, metadata bool) []byte {
	var common bytes.Buffer
	binary.Write(&common, binary.BigEndian, uint16(1))
	binary.Write(&common, binary.BigEndian, uint32(6))
	binary.Write(&common, binary.BigEndian, uint16(16))
	common.Write([]byte{0x40, 0x0e, 0xbb, 0x80, 0, 0, 0, 0, 0, 0}) // exact48000Hz extended80
	if form == "AIFC" {
		common.WriteString(compression)
		common.Write([]byte{3, 'P', 'C', 'M'})
	}
	var sound bytes.Buffer
	sound.Write(make([]byte, 8))
	for _, code := range []int16{8192, -4096, 12288, -16384, 0, 4096} {
		if compression == "sowt" {
			binary.Write(&sound, binary.LittleEndian, code)
		} else {
			binary.Write(&sound, binary.BigEndian, code)
		}
	}
	var payload bytes.Buffer
	payload.WriteString(form)
	if form == "AIFC" {
		payload.Write(cliAIFFChunk("FVER", []byte{0xa2, 0x80, 0x51, 0x40}))
	}
	payload.Write(cliAIFFChunk("COMM", common.Bytes()))
	if metadata {
		payload.Write(cliAIFFChunk("NAME", []byte("fixture")))
	}
	payload.Write(cliAIFFChunk("SSND", sound.Bytes()))
	var output bytes.Buffer
	output.WriteString("FORM")
	binary.Write(&output, binary.BigEndian, uint32(payload.Len()))
	output.Write(payload.Bytes())
	return output.Bytes()
}

func TestDemoAIFFSignatureClockRemixAndMetadata(t *testing.T) {
	for _, tc := range []struct {
		form, codec string
		metadata    bool
	}{
		{"AIFF", "", false}, {"AIFC", "NONE", false}, {"AIFC", "sowt", false}, {"AIFF", "", true},
	} {
		t.Run(tc.form+tc.codec, func(t *testing.T) {
			dir := t.TempDir()
			input, output := filepath.Join(dir, "wrong-extension.flac"), filepath.Join(dir, "output.wav")
			if err := os.WriteFile(input, cliAIFFFixture(tc.form, tc.codec, tc.metadata), 0600); err != nil {
				t.Fatal(err)
			}
			args := []string{"-scene", "reference", "-format", "wav", "-audio", input, "-audio-channels", "2", "-audio-remix", "-out", output}
			if tc.metadata {
				if err := demo(args); err == nil {
					t.Fatal("AIFF metadata discarded by default")
				}
				if _, err := os.Stat(output); !os.IsNotExist(err) {
					t.Fatal("metadata failure published output", err)
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
			got, err := audio.ReadWAV(bytes.NewReader(data), audio.WAVReadOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if got.Frames() != 48000 || got.SampleRate() != 48000 || got.Channels() != 2 {
				t.Fatal("AIFF changed clock", got.Format(), got.Frames())
			}
			samples := got.Samples()
			for i, want := range []float64{.25, -.125, .375, -.5, 0, .125} {
				if samples[2*i] != want || samples[2*i+1] != want {
					t.Fatal("AIFF/remix samples", samples[:12])
				}
			}
			for _, value := range samples[12:] {
				if value != 0 {
					t.Fatal("unexpected tail")
				}
			}
		})
	}
}

func TestAudioInputRejectsTrailingOrMalformedFORM(t *testing.T) {
	var wav, flac bytes.Buffer
	pcm, err := audio.NewPCM(audio.Format{SampleRate: 48000, Channels: 1}, []float64{.25})
	if err != nil {
		t.Fatal(err)
	}
	if err = audio.WriteWAV(&wav, pcm, audio.WAVWriteOptions{}); err != nil {
		t.Fatal(err)
	}
	if err = audio.WriteFLAC(&flac, pcm, audio.FLACWriteOptions{}); err != nil {
		t.Fatal(err)
	}
	aiff := cliAIFFFixture("AIFF", "", false)
	badFORM := append([]byte(nil), aiff...)
	copy(badFORM[8:12], "ILBM")
	for _, data := range [][]byte{append(append([]byte(nil), wav.Bytes()...), 1), append(append([]byte(nil), flac.Bytes()...), 1), append(append([]byte(nil), aiff...), 1), badFORM, aiff[:len(aiff)-1]} {
		dir := t.TempDir()
		input, output := filepath.Join(dir, "input"), filepath.Join(dir, "output.wav")
		if err = os.WriteFile(input, data, 0600); err != nil {
			t.Fatal(err)
		}
		if err = demo([]string{"-scene", "reference", "-format", "wav", "-audio", input, "-audio-channels", "1", "-out", output}); err == nil {
			t.Fatal("malformed/trailing input accepted")
		}
		if _, err = os.Stat(output); !os.IsNotExist(err) {
			t.Fatal("invalid import published output", err)
		}
	}
}

func TestCompiled3DTimelineAcceptsAudioWithFullClock(t *testing.T) {
	for _, scene := range []string{"scene3d-timeline", "point-morph", "mesh-morph", "quaternion", "oriented-camera", "point-lights"} {
		dir := t.TempDir()
		input, output := filepath.Join(dir, "input.aiff"), filepath.Join(dir, "output.wav")
		if err := os.WriteFile(input, cliAIFFFixture("AIFF", "", false), 0600); err != nil {
			t.Fatal(err)
		}
		if err := demo([]string{"-scene", scene, "-format", "wav", "-audio", input, "-audio-remix", "-out", output}); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(output)
		if err != nil {
			t.Fatal(err)
		}
		pcm, err := audio.ReadWAV(bytes.NewReader(data), audio.WAVReadOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if pcm.SampleRate() != 48000 || pcm.Channels() != 2 || pcm.Frames() != 240000 {
			t.Fatal("3D/media duration mismatch", pcm.Format(), pcm.Frames())
		}
		samples := pcm.Samples()
		if samples[0] != .25 || samples[1] != .25 || samples[6] != -.5 || samples[7] != -.5 || samples[len(samples)-1] != 0 {
			t.Fatal("3D media shifted or lost input")
		}
	}
}
