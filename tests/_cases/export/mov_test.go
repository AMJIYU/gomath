package export_test

import (
	"bytes"
	"encoding/binary"
	"gomath/audio"
	"gomath/export"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestMOVAudioTrackSamplesAndExactTiming(t *testing.T) {
	samples := make([]float64, 1001*2)
	for i := 0; i < 1001; i++ {
		samples[2*i] = .5 * math.Sin(2*math.Pi*440*float64(i)/8000)
		samples[2*i+1] = float64(i%7-3) / 4
	}
	pcm, err := audio.NewPCM(audio.Format{SampleRate: 8000, Channels: 2, ChannelMask: 3}, samples)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "audio.mov")
	if dir := os.Getenv("GOMATH_MOV_REFERENCE_OUT"); dir != "" {
		file = filepath.Join(dir, "audio.mov")
	}
	m, err := export.NewMOVWithAudio(file, 66, 34, 24, pcm, export.DefaultMP4Options())
	if err != nil {
		t.Fatal(err)
	}
	for i := range 3 {
		if err = m.WriteFrame(i, frame(66, 34)); err != nil {
			t.Fatal(err)
		}
	}
	if err = m.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	top := boxes(t, data)
	ftyp := find(t, top, "ftyp")
	if string(ftyp.payload[:4]) != "qt  " {
		t.Fatal("wrong container brand")
	}
	mdat := find(t, top, "mdat")
	expected := make([]byte, len(samples)*2)
	for i, x := range samples {
		q := min(int(math.Round(x*32768)), 32767)
		binary.LittleEndian.PutUint16(expected[2*i:], uint16(int16(q)))
	}
	if !bytes.Equal(mdat.payload[:len(expected)], expected) {
		t.Fatal("PCM sample values/order changed")
	}
	if os.Getenv("GOMATH_MOV_REFERENCE_OUT") != "" {
		if err := os.WriteFile(filepath.Join(filepath.Dir(file), "expected.s16le"), expected, 0644); err != nil {
			t.Fatal(err)
		}
	}
	moov := find(t, top, "moov")
	parts := boxes(t, moov.payload)
	mvhd := find(t, parts, "mvhd")
	if mvhd.payload[0] != 1 || binary.BigEndian.Uint32(mvhd.payload[20:24]) != 24000 || binary.BigEndian.Uint64(mvhd.payload[24:32]) != 3003 {
		t.Fatal("movie timeline lost sub-video-frame precision")
	}
	found := map[string]bool{}
	for _, track := range parts {
		if track.kind != "trak" {
			continue
		}
		trackBoxes := boxes(t, track.payload)
		mdia := find(t, trackBoxes, "mdia")
		media := boxes(t, mdia.payload)
		hdlr := find(t, media, "hdlr")
		kind := string(hdlr.payload[8:12])
		found[kind] = true
		minf := find(t, media, "minf")
		stbl := find(t, boxes(t, minf.payload), "stbl")
		tables := boxes(t, stbl.payload)
		co64 := find(t, tables, "co64")
		offset := binary.BigEndian.Uint64(co64.payload[8:16])
		if kind == "soun" {
			if offset != uint64(mdat.offset+16) {
				t.Fatal("wrong PCM chunk offset")
			}
			mdhd := find(t, media, "mdhd")
			if binary.BigEndian.Uint32(mdhd.payload[12:16]) != 8000 || binary.BigEndian.Uint32(mdhd.payload[16:20]) != 1001 {
				t.Fatal("wrong audio clock")
			}
			stsz := find(t, tables, "stsz")
			if binary.BigEndian.Uint32(stsz.payload[4:8]) != 4 || binary.BigEndian.Uint32(stsz.payload[8:12]) != 1001 {
				t.Fatal("wrong PCM sample table")
			}
			stsd := find(t, tables, "stsd")
			entry := find(t, boxes(t, stsd.payload[8:]), "sowt")
			if binary.BigEndian.Uint16(entry.payload[16:18]) != 2 || binary.BigEndian.Uint16(entry.payload[18:20]) != 16 || binary.BigEndian.Uint32(entry.payload[24:28]) != 8000<<16 {
				t.Fatal("wrong PCM sample description")
			}
		} else if kind == "vide" {
			if offset != uint64(mdat.offset+16+len(expected)) {
				t.Fatal("video chunk overlaps audio")
			}
		} else {
			t.Fatal("unexpected track", kind)
		}
	}
	if len(found) != 2 || !found["soun"] || !found["vide"] {
		t.Fatal("missing A/V tracks")
	}
}

func TestMOVAndWAVValidationPublication(t *testing.T) {
	newPCM := func(f audio.Format, samples []float64) *audio.PCM {
		t.Helper()
		p, err := audio.NewPCM(f, samples)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	invalid := []*audio.PCM{nil, {}, newPCM(audio.Format{SampleRate: 8000, Channels: 1}, nil),
		newPCM(audio.Format{SampleRate: 96000, Channels: 1}, []float64{0}),
		newPCM(audio.Format{SampleRate: 8000, Channels: 3}, []float64{0, 0, 0}),
		newPCM(audio.Format{SampleRate: 8000, Channels: 1}, []float64{2}),
		newPCM(audio.Format{SampleRate: 8000, Channels: 1, ChannelMask: 1}, []float64{0}),
	}
	for _, p := range invalid {
		dir := t.TempDir()
		if m, err := export.NewMOVWithAudio(filepath.Join(dir, "bad.mov"), 32, 32, 24, p, export.DefaultMP4Options()); err == nil || m != nil {
			t.Fatal("invalid MOV audio accepted")
		}
		entries, _ := os.ReadDir(dir)
		if len(entries) != 0 {
			t.Fatal("invalid MOV leaked files")
		}
	}
	dir := t.TempDir()
	file := filepath.Join(dir, "sound.wav")
	p := newPCM(audio.Format{SampleRate: 8000, Channels: 1}, []float64{-.5, 0, .5})
	if err := export.WAV(file, p, audio.WAVWriteOptions{}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if err = export.WAV(file, p, audio.WAVWriteOptions{}); err == nil {
		t.Fatal("WAV overwrite accepted")
	}
	after, _ := os.ReadFile(file)
	if !bytes.Equal(before, after) {
		t.Fatal("existing WAV changed")
	}
	if err = export.WAV(filepath.Join(dir, "bad.wav"), nil, audio.WAVWriteOptions{}); err == nil {
		t.Fatal("nil WAV accepted")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatal("failed WAV leaked temporary output")
	}
}
