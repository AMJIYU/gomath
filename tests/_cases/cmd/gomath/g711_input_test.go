package main

import (
	"bytes"
	"encoding/binary"
	"gomath/audio"
	"os"
	"path/filepath"
	"testing"
)

func cliG711Fixture(tag uint16, codes []byte) []byte {
	chunk := func(id string, data []byte) []byte {
		var out bytes.Buffer
		out.WriteString(id)
		binary.Write(&out, binary.LittleEndian, uint32(len(data)))
		out.Write(data)
		if len(data)%2 != 0 {
			out.WriteByte(0)
		}
		return out.Bytes()
	}
	var format bytes.Buffer
	binary.Write(&format, binary.LittleEndian, tag)
	binary.Write(&format, binary.LittleEndian, uint16(1))
	binary.Write(&format, binary.LittleEndian, uint32(48000))
	binary.Write(&format, binary.LittleEndian, uint32(48000))
	for _, v := range []uint16{1, 8, 0} {
		binary.Write(&format, binary.LittleEndian, v)
	}
	var fact bytes.Buffer
	binary.Write(&fact, binary.LittleEndian, uint32(len(codes)))
	var body bytes.Buffer
	body.WriteString("WAVE")
	body.Write(chunk("fmt ", format.Bytes()))
	body.Write(chunk("fact", fact.Bytes()))
	body.Write(chunk("data", codes))
	var file bytes.Buffer
	file.WriteString("RIFF")
	binary.Write(&file, binary.LittleEndian, uint32(body.Len()))
	file.Write(body.Bytes())
	return file.Bytes()
}

func TestDemoG711DecodeRemixAndOriginalSceneClock(t *testing.T) {
	for _, tc := range []struct {
		tag      uint16
		codes    []byte
		expected []int16
	}{
		{6, []byte{0xd5, 0x55, 0, 0x80, 0xaa, 0x2a}, []int16{8, -8, -5504, 5504, 32256, -32256}},
		{7, []byte{0xff, 0x7f, 0, 0x80, 0xfe, 0x7e}, []int16{0, 0, -32124, 32124, 8, -8}},
	} {
		dir := t.TempDir()
		input, output := filepath.Join(dir, "wrong.aiff"), filepath.Join(dir, "output.wav")
		if err := os.WriteFile(input, cliG711Fixture(tc.tag, tc.codes), 0600); err != nil {
			t.Fatal(err)
		}
		if err := demo([]string{"-scene", "reference", "-format", "wav", "-audio", input, "-audio-remix", "-out", output}); err != nil {
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
		if pcm.Frames() != 48000 || pcm.Channels() != 2 {
			t.Fatal("G711 scene clock", pcm.Format(), pcm.Frames())
		}
		for i, value := range pcm.Samples() {
			want := 0.0
			if i < 2*len(tc.expected) {
				want = float64(tc.expected[i/2]) / 32768
			}
			if value != want {
				t.Fatalf("tag%d sample%d got%g want%g", tc.tag, i, value, want)
			}
		}
	}
}
