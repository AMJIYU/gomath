package export_test

import (
	"encoding/binary"
	"gomath/export"
	"gomath/internal/h264"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"
)

func TestCompressedMP4TablesAndBudget(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "compressed.mp4")
	options := export.DefaultMP4Options()
	options.MaxFrames = 3
	m, err := export.NewMP4WithOptions(file, 66, 34, 24, options)
	if err != nil {
		t.Fatal(err)
	}
	for i := range 3 {
		im := image.NewRGBA(image.Rect(0, 0, 66, 34))
		for y := range 34 {
			for x := range 66 {
				im.SetRGBA(x, y, color.RGBA{uint8(i * 60), 80, 120, 255})
			}
		}
		if err = m.WriteFrame(i, im); err != nil {
			t.Fatal(err)
		}
	}
	if err = m.WriteFrame(3, frame(66, 34)); err == nil {
		t.Fatal("frame budget ignored")
	}
	if _, err = os.Stat(file); !os.IsNotExist(err) {
		t.Fatal("uncommitted output visible")
	}
	if err = m.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	top := boxes(t, data)
	mdat := find(t, top, "mdat")
	moov := find(t, top, "moov")
	trak := find(t, boxes(t, moov.payload), "trak")
	mdia := find(t, boxes(t, trak.payload), "mdia")
	minf := find(t, boxes(t, mdia.payload), "minf")
	stbl := find(t, boxes(t, minf.payload), "stbl")
	tables := boxes(t, stbl.payload)
	stsz := find(t, tables, "stsz")
	if binary.BigEndian.Uint32(stsz.payload[8:12]) != 3 {
		t.Fatal("wrong sample count")
	}
	pos := 0
	for i := range 3 {
		size := int(binary.BigEndian.Uint32(stsz.payload[12+4*i:]))
		if size < 5 || pos+size > len(mdat.payload) || binary.BigEndian.Uint32(mdat.payload[pos:]) != uint32(size-4) || mdat.payload[pos+4] != 0x65 {
			t.Fatal("invalid compressed sample", i)
		}
		pos += size
	}
	if pos != len(mdat.payload) {
		t.Fatal("unindexed data")
	}
	co64 := find(t, tables, "co64")
	if binary.BigEndian.Uint64(co64.payload[8:16]) != uint64(mdat.offset+16) {
		t.Fatal("wrong chunk offset")
	}
	// Actual transform-coded flat pictures should be much smaller than the raw
	// padded 80x48 4:2:0 picture samples. This is not a general bitrate guarantee.
	if len(mdat.payload) >= 3*80*48*3/2/4 {
		t.Fatal("flat pictures did not compress", len(mdat.payload))
	}
	if m.Close() == nil || m.WriteFrame(3, frame(66, 34)) == nil {
		t.Fatal("closed sink accepted write/close")
	}
}

func TestMP4OptionsRejectBeforeCreatingOutput(t *testing.T) {
	for i, opt := range []export.MP4Options{
		{Codec: 99}, {QP: -1}, {QP: 52}, {MaxFrames: -1}, {MaxFrames: 1 << 27}, {Codec: export.H264PCM, QP: 24}, {Codec: export.H264PCM, ModeSearch: true},
	} {
		dir := t.TempDir()
		if m, err := export.NewMP4WithOptions(filepath.Join(dir, "bad.mp4"), 32, 32, 24, opt); err == nil || m != nil {
			t.Fatalf("invalid option %d accepted", i)
		}
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) != 0 {
			t.Fatal("invalid options leaked output", err)
		}
	}
}

func TestMP4ModeSearchReachesVideoSamples(t *testing.T) {
	img := frame(66, 34)
	options := export.DefaultMP4Options()
	options.ModeSearch = true
	path := filepath.Join(t.TempDir(), "searched.mp4")
	m, err := export.NewMP4WithOptions(path, 66, 34, 24, options)
	if err != nil {
		t.Fatal(err)
	}
	if err = m.WriteFrame(0, img); err != nil {
		t.Fatal(err)
	}
	if err = m.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sample := find(t, boxes(t, data), "mdat").payload
	encoder, err := h264.NewIntra(66, 34, 24, h264.IntraOptions{QP: 24, ModeSearch: true})
	if err != nil {
		t.Fatal(err)
	}
	want, err := encoder.Encode(img)
	if err != nil {
		t.Fatal(err)
	}
	if string(sample[4:]) != string(want) {
		t.Fatal("container did not use requested prediction search")
	}
	dc, err := h264.NewIntra(66, 34, 24, h264.IntraOptions{QP: 24})
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := dc.Encode(img)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) == string(baseline) {
		t.Fatal("fixture does not distinguish mode search")
	}
}
