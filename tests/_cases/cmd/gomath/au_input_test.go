package main

import (
	"bytes"
	"encoding/binary"
	"gomath/audio"
	"os"
	"path/filepath"
	"testing"
)

// Hand-authored big-endian AU payloads. G.711 silence/sign codewords follow
// the companding equations, independently of the production AU decoder.
func cliAUFixture(encoding uint32, annotation []byte) ([]byte, []float64) {
	var out bytes.Buffer
	var data []byte
	want := []float64{.25, -.125, .375, -.5, 0, .125}
	switch encoding {
	case 2:
		data = []byte{32, 240, 48, 192, 0, 16}
	case 3, 4, 5:
		width := int(encoding - 1)
		for _, value := range []int32{0x20000000, -0x10000000, 0x30000000, -0x40000000, 0, 0x10000000} {
			for i := range width {
				data = append(data, byte(uint32(value)>>uint(24-8*i)))
			}
		}
	case 1:
		data = []byte{0xff, 0x7f, 0x80, 0x00, 0xfe, 0x7e}
		want = []float64{0, 0, 32124.0 / 32768, -32124.0 / 32768, 8.0 / 32768, -8.0 / 32768}
	case 27:
		data = []byte{0xd5, 0x55, 0xaa, 0x2a, 0xd4, 0x54}
		want = []float64{8.0 / 32768, -8.0 / 32768, 32256.0 / 32768, -32256.0 / 32768, 24.0 / 32768, -24.0 / 32768}
	}
	out.WriteString(".snd")
	for _, value := range []uint32{uint32(24 + len(annotation)), uint32(len(data)), encoding, 8000, 1} {
		binary.Write(&out, binary.BigEndian, value)
	}
	out.Write(annotation)
	out.Write(data)
	return out.Bytes(), want
}

func TestDemoAUSignatureSamplesClockRemixAndAnnotation(t *testing.T) {
	for _, encoding := range []uint32{1, 2, 3, 4, 5, 27} {
		for _, annotated := range []bool{false, true} {
			var annotation []byte
			if annotated {
				annotation = []byte("note")
			}
			data, want := cliAUFixture(encoding, annotation)
			dir := t.TempDir()
			input, output := filepath.Join(dir, "wrong-extension.wav"), filepath.Join(dir, "output.wav")
			if err := os.WriteFile(input, data, 0600); err != nil {
				t.Fatal(err)
			}
			args := []string{"-scene", "reference", "-format", "wav", "-audio", input, "-audio-rate", "8000", "-audio-remix", "-out", output}
			if annotated {
				if err := demo(args); err == nil {
					t.Fatal("AU annotation discarded by default")
				}
				if _, err := os.Stat(output); !os.IsNotExist(err) {
					t.Fatal("AU annotation rejection published output", err)
				}
				args = append(args, "-discard-audio-metadata")
			}
			if err := demo(args); err != nil {
				t.Fatal(err)
			}
			result, err := os.ReadFile(output)
			if err != nil {
				t.Fatal(err)
			}
			pcm, err := audio.ReadWAV(bytes.NewReader(result), audio.WAVReadOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if pcm.Frames() != 8000 || pcm.SampleRate() != 8000 || pcm.Channels() != 2 {
				t.Fatal("AU changed scene clock/remix", pcm.Format(), pcm.Frames())
			}
			for i, value := range pcm.Samples() {
				expected := 0.0
				if i/2 < len(want) {
					expected = want[i/2]
				}
				if value != expected {
					t.Fatalf("AU encoding%d sample%d =%g, want%g", encoding, i, value, expected)
				}
			}
		}
	}
}

func TestDemoAURejectsMalformedUnknownAndTrailingAtomically(t *testing.T) {
	for _, kind := range []string{"offset", "size", "encoding", "rate", "channels", "unknown", "truncated", "trailing"} {
		t.Run(kind, func(t *testing.T) {
			data, _ := cliAUFixture(3, nil)
			switch kind {
			case "offset":
				binary.BigEndian.PutUint32(data[4:8], 25)
			case "size":
				binary.BigEndian.PutUint32(data[8:12], 11)
			case "encoding":
				binary.BigEndian.PutUint32(data[12:16], 6) // float32 is unsupported.
			case "rate":
				binary.BigEndian.PutUint32(data[16:20], 0)
			case "channels":
				binary.BigEndian.PutUint32(data[20:24], 0)
			case "unknown":
				binary.BigEndian.PutUint32(data[8:12], ^uint32(0))
			case "truncated":
				data = data[:len(data)-1]
			case "trailing":
				data = append(data, 0)
			}
			dir := t.TempDir()
			input, output := filepath.Join(dir, "input"), filepath.Join(dir, "output.wav")
			if err := os.WriteFile(input, data, 0600); err != nil {
				t.Fatal(err)
			}
			if err := demo([]string{"-scene", "reference", "-format", "wav", "-audio", input, "-audio-remix", "-out", output}); err == nil {
				t.Fatal("malformed/unknown/trailing AU accepted", kind)
			}
			if _, err := os.Stat(output); !os.IsNotExist(err) {
				t.Fatal("invalid AU published output", err)
			}
		})
	}
}
