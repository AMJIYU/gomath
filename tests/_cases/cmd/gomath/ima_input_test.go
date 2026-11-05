package main

import (
	"bytes"
	"encoding/binary"
	"gomath/audio"
	"os"
	"path/filepath"
	"testing"
)

// The independent mono block starts at 1000 with step index zero. Every
// zero nibble adds floor(7/8)=0, so all nine coded frames equal 1000. fact
// trims two padding frames. This exercises the CLI without an encoder oracle.
func cliIMAFixture(factAfter bool) []byte {
	chunk := func(id string, data []byte) []byte {
		var b bytes.Buffer
		b.WriteString(id)
		binary.Write(&b, binary.LittleEndian, uint32(len(data)))
		b.Write(data)
		if len(data)%2 != 0 {
			b.WriteByte(0)
		}
		return b.Bytes()
	}
	var format bytes.Buffer
	for _, value := range []uint16{0x11, 1} {
		binary.Write(&format, binary.LittleEndian, value)
	}
	binary.Write(&format, binary.LittleEndian, uint32(8000))
	binary.Write(&format, binary.LittleEndian, uint32(7111))
	for _, value := range []uint16{8, 4, 2, 9} {
		binary.Write(&format, binary.LittleEndian, value)
	}
	var payload bytes.Buffer
	payload.WriteString("WAVE")
	payload.Write(chunk("fmt ", format.Bytes()))
	fact := chunk("fact", []byte{7, 0, 0, 0})
	if !factAfter {
		payload.Write(fact)
	}
	payload.Write(chunk("data", []byte{0xe8, 3, 0, 0, 0, 0, 0, 0}))
	if factAfter {
		payload.Write(fact)
	}
	var file bytes.Buffer
	file.WriteString("RIFF")
	binary.Write(&file, binary.LittleEndian, uint32(payload.Len()))
	file.Write(payload.Bytes())
	return file.Bytes()
}

func TestDemoIMAImportFactTrimRemixAndFailures(t *testing.T) {
	for _, factAfter := range []bool{false, true} {
		dir := t.TempDir()
		input, output := filepath.Join(dir, "input.aiff"), filepath.Join(dir, "output.wav")
		if err := os.WriteFile(input, cliIMAFixture(factAfter), 0600); err != nil {
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
			t.Fatal("IMA import changed scene clock", pcm.Format(), pcm.Frames())
		}
		for i, sample := range pcm.Samples() {
			want := 0.0
			if i < 14 {
				want = 1000.0 / 32768
			}
			if sample != want {
				t.Fatalf("IMA decoded/remixed frame %d channel %d: %g, want %g", i/2, i%2, sample, want)
			}
		}
	}
	for _, kind := range []string{"index", "reserved", "samples-per-block", "fact", "truncated", "trailing"} {
		t.Run(kind, func(t *testing.T) {
			data := cliIMAFixture(false)
			switch kind {
			case "index":
				data[len(data)-6] = 89
			case "reserved":
				data[len(data)-5] = 1
			case "samples-per-block":
				data[38] = 10
			case "fact":
				data[48] = 10
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
				t.Fatal("invalid IMA input accepted")
			}
			if _, err := os.Stat(output); !os.IsNotExist(err) {
				t.Fatal("invalid IMA input published output", err)
			}
		})
	}
}
