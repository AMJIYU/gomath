package export_test

import (
	"bytes"
	"encoding/binary"
	"gomath/export"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func frame(width, height int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			img.SetRGBA(x, y, color.RGBA{uint8(x * 7), uint8(y * 13), 120, 255})
		}
	}
	return img
}
func TestStreamingGIFDecodeAndTiming(t *testing.T) {
	file := filepath.Join(t.TempDir(), "test.gif")
	g, e := export.NewGIF(file, 32, 20, 24)
	if e != nil {
		t.Fatal(e)
	}
	for i := range 24 {
		if e = g.WriteFrame(i, frame(32, 20)); e != nil {
			t.Fatal(e)
		}
	}
	if e = g.Close(); e != nil {
		t.Fatal(e)
	}
	data, e := os.ReadFile(file)
	if e != nil {
		t.Fatal(e)
	}
	decoded, e := gif.DecodeAll(bytes.NewReader(data))
	if e != nil {
		t.Fatal(e)
	}
	if len(decoded.Image) != 24 || decoded.LoopCount != 0 {
		t.Fatal("bad GIF frame count or loop")
	}
	delay := 0
	for _, d := range decoded.Delay {
		delay += d
	}
	if delay != 100 {
		t.Fatal("timing drift", delay)
	}
	if e = g.WriteFrame(24, frame(32, 20)); e == nil {
		t.Fatal("write after close accepted")
	}
}
func TestAtomicPNGAndNoOverwrite(t *testing.T) {
	file := filepath.Join(t.TempDir(), "test.png")
	if e := export.PNG(file, frame(32, 20)); e != nil {
		t.Fatal(e)
	}
	data, _ := os.ReadFile(file)
	img, e := png.Decode(bytes.NewReader(data))
	if e != nil || img.Bounds().Dx() != 32 {
		t.Fatal("bad PNG", e)
	}
	if export.PNG(file, frame(2, 2)) == nil {
		t.Fatal("existing output overwritten")
	}
	next, _ := os.ReadFile(file)
	if !bytes.Equal(data, next) {
		t.Fatal("existing output changed")
	}
}
func TestAbortRemovesPartialOutput(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "bad.mp4")
	m, e := export.NewMP4(file, 18, 22, 24)
	if e != nil {
		t.Fatal(e)
	}
	if e = m.WriteFrame(0, frame(18, 22)); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(file); !os.IsNotExist(e) {
		t.Fatal("partial output visible")
	}
	if e = m.Abort(); e != nil {
		t.Fatal(e)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatal("temporary output leaked")
	}
}

type mp4Box struct {
	kind    string
	payload []byte
	offset  int
}

func boxes(t *testing.T, data []byte) []mp4Box {
	t.Helper()
	var out []mp4Box
	for pos := 0; pos < len(data); {
		if len(data)-pos < 8 {
			t.Fatal("truncated box")
		}
		size := int(binary.BigEndian.Uint32(data[pos:]))
		header := 8
		if size == 1 {
			if len(data)-pos < 16 {
				t.Fatal("truncated extended box")
			}
			size = int(binary.BigEndian.Uint64(data[pos+8:]))
			header = 16
		}
		if size < header || size > len(data)-pos {
			t.Fatal("bad box length", size)
		}
		out = append(out, mp4Box{string(data[pos+4 : pos+8]), data[pos+header : pos+size], pos})
		pos += size
	}
	return out
}
func find(t *testing.T, bs []mp4Box, kind string) mp4Box {
	t.Helper()
	for _, b := range bs {
		if b.kind == kind {
			return b
		}
	}
	t.Fatal("missing box", kind)
	return mp4Box{}
}
func TestMP4ContainerTables(t *testing.T) {
	file := filepath.Join(t.TempDir(), "test.mp4")
	m, e := export.NewMP4(file, 18, 22, 24)
	if e != nil {
		t.Fatal(e)
	}
	for i := range 3 {
		if e = m.WriteFrame(i, frame(18, 22)); e != nil {
			t.Fatal(e)
		}
	}
	if e = m.Close(); e != nil {
		t.Fatal(e)
	}
	data, _ := os.ReadFile(file)
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
	offset := 0
	for i := range 3 {
		size := int(binary.BigEndian.Uint32(stsz.payload[12+4*i:]))
		n := int(binary.BigEndian.Uint32(mdat.payload[offset:]))
		if size != n+4 || mdat.payload[offset+4] != 0x65 {
			t.Fatal("bad sample size or NAL header")
		}
		offset += size
	}
	if offset != len(mdat.payload) {
		t.Fatal("unindexed sample data")
	}
	co64 := find(t, tables, "co64")
	if binary.BigEndian.Uint64(co64.payload[8:16]) != uint64(mdat.offset+16) {
		t.Fatal("wrong chunk offset")
	}
	stsd := find(t, tables, "stsd")
	avc1 := find(t, boxes(t, stsd.payload[8:]), "avc1")
	if binary.BigEndian.Uint16(avc1.payload[24:26]) != 18 || binary.BigEndian.Uint16(avc1.payload[26:28]) != 22 {
		t.Fatal("wrong video dimensions")
	}
	avcc := find(t, boxes(t, avc1.payload[78:]), "avcC")
	if avcc.payload[1] != 66 || avcc.payload[4] != 0xff {
		t.Fatal("invalid avcC")
	}
}
func TestInvalidVideoAndSequence(t *testing.T) {
	dir := t.TempDir()
	if _, e := export.NewMP4(filepath.Join(dir, "odd.mp4"), 17, 20, 24); e == nil {
		t.Fatal("odd dimensions accepted")
	}
	s, e := export.NewPNGSequence(filepath.Join(dir, "frames"))
	if e != nil {
		t.Fatal(e)
	}
	if e = s.WriteFrame(1, frame(2, 2)); e == nil {
		t.Fatal("unordered frame accepted")
	}
	if e = s.WriteFrame(0, frame(2, 2)); e != nil {
		t.Fatal(e)
	}
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(filepath.Join(dir, "frames", "000000.png")); e != nil {
		t.Fatal(e)
	}
}
