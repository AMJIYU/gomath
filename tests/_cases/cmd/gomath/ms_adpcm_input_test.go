package main

import (
	"bytes"
	"encoding/binary"
	"gomath/audio"
	"os"
	"path/filepath"
	"testing"
)

// Independent MS ADPCM: predictor (256,0), delta16, initial samples1000,
// and zero residuals reconstruct four constant samples. fact retains three.
func cliMSADPCMFixture(factAfter bool) []byte {
	chunk := func(id string, payload []byte) []byte {
		var out bytes.Buffer
		out.WriteString(id)
		binary.Write(&out, binary.LittleEndian, uint32(len(payload)))
		out.Write(payload)
		if len(payload)%2 != 0 {
			out.WriteByte(0)
		}
		return out.Bytes()
	}
	var format bytes.Buffer
	for _, value := range []uint16{2, 1} {
		binary.Write(&format, binary.LittleEndian, value)
	}
	binary.Write(&format, binary.LittleEndian, uint32(8000))
	binary.Write(&format, binary.LittleEndian, uint32(16000))
	for _, value := range []uint16{8, 4, 32, 4, 7} {
		binary.Write(&format, binary.LittleEndian, value)
	}
	for _, pair := range [][2]int16{{256, 0}, {512, -256}, {0, 0}, {192, 64}, {240, 0}, {460, -208}, {392, -232}} {
		binary.Write(&format, binary.LittleEndian, pair)
	}
	var body bytes.Buffer
	body.WriteString("WAVE")
	body.Write(chunk("fmt ", format.Bytes()))
	fact := chunk("fact", []byte{3, 0, 0, 0})
	if !factAfter {
		body.Write(fact)
	}
	body.Write(chunk("data", []byte{0, 16, 0, 0xe8, 3, 0xe8, 3, 0}))
	if factAfter {
		body.Write(fact)
	}
	var file bytes.Buffer
	file.WriteString("RIFF")
	binary.Write(&file, binary.LittleEndian, uint32(body.Len()))
	file.Write(body.Bytes())
	return file.Bytes()
}

func TestDemoMSADPCMFactTrimRemixAndAtomicFailures(t *testing.T) {
	for _, factAfter := range []bool{false, true} {
		dir := t.TempDir()
		input, output := filepath.Join(dir, "wrong.flac"), filepath.Join(dir, "output.wav")
		if err := os.WriteFile(input, cliMSADPCMFixture(factAfter), 0600); err != nil {
			t.Fatal(err)
		}
		if err := demo([]string{"-scene", "reference", "-format", "wav", "-audio", input, "-audio-rate", "8000", "-audio-remix", "-out", output}); err != nil {
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
		if pcm.Frames() != 8000 || pcm.Channels() != 2 || pcm.SampleRate() != 8000 {
			t.Fatal("MS ADPCM scene clock", pcm.Format(), pcm.Frames())
		}
		for i, sample := range pcm.Samples() {
			want := 0.0
			if i < 6 {
				want = 1000.0 / 32768
			}
			if sample != want {
				t.Fatalf("MS ADPCM remixed sample%d got%g want%g", i, sample, want)
			}
		}
	}
	for _, kind := range []string{"predictor", "samples-per-block", "coefficient-count", "fact", "truncated", "trailing"} {
		t.Run(kind, func(t *testing.T) {
			data := cliMSADPCMFixture(false)
			switch kind {
			case "predictor":
				data[bytes.Index(data, []byte("data"))+8] = 7
			case "samples-per-block":
				data[38] = 5
			case "coefficient-count":
				data[40] = 0
			case "fact":
				data[bytes.Index(data, []byte("fact"))+8] = 5
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
				t.Fatal("invalid MS ADPCM input accepted")
			}
			if _, err := os.Stat(output); !os.IsNotExist(err) {
				t.Fatal("invalid MS ADPCM published output", err)
			}
		})
	}
}
