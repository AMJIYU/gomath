package h264

import (
	"bytes"
	"image"
	"testing"
)

func unescape(nal []byte) []byte {
	var out []byte
	zeros := 0
	for _, b := range nal[1:] {
		if zeros == 2 && b == 3 {
			zeros = 0
			continue
		}
		out = append(out, b)
		if b == 0 {
			zeros++
		} else {
			zeros = 0
		}
	}
	return out
}

type reader struct {
	p   []byte
	pos int
}

func (r *reader) bit() uint32 { n := uint32(r.p[r.pos/8] >> uint(7-r.pos%8) & 1); r.pos++; return n }
func (r *reader) uint(n int) uint32 {
	var v uint32
	for range n {
		v = v*2 + r.bit()
	}
	return v
}
func (r *reader) ue() uint32 {
	n := 0
	for r.bit() == 0 {
		n++
	}
	return (1 << uint(n)) - 1 + r.uint(n)
}
func TestSPSCroppingAndDimensions(t *testing.T) {
	for _, dim := range [][2]int{{16, 16}, {18, 22}, {320, 180}, {640, 360}} {
		e, err := New(dim[0], dim[1], 24)
		if err != nil {
			t.Fatal(err)
		}
		r := reader{p: unescape(e.SPS())}
		if r.uint(8) != 66 || r.uint(8) != 0xc0 || r.uint(8) != 51 {
			t.Fatal("bad profile")
		}
		r.ue()
		r.ue()
		if r.ue() != 2 {
			t.Fatal("unexpected POC type")
		}
		r.ue()
		r.bit()
		width, height := int(r.ue()+1)*16, int(r.ue()+1)*16
		if r.bit() != 1 {
			t.Fatal("not progressive")
		}
		r.bit()
		if r.bit() != 0 {
			left, right, top, bottom := r.ue(), r.ue(), r.ue(), r.ue()
			width -= 2 * int(left+right)
			height -= 2 * int(top+bottom)
		}
		if width != dim[0] || height != dim[1] {
			t.Fatal("wrong cropped dimensions", width, height)
		}
		if r.bit() != 1 {
			t.Fatal("VUI missing")
		}
		r.bit()
		r.bit()
		if r.bit() != 1 {
			t.Fatal("video signal missing")
		}
		r.uint(3)
		if r.bit() != 1 {
			t.Fatal("full range missing")
		}
	}
}
func TestNALPreventionAndPixelPayload(t *testing.T) {
	e, _ := New(16, 16, 24)
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for i := 3; i < len(img.Pix); i += 4 {
		img.Pix[i] = 255
	}
	data, err := e.Encode(img)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][]byte{{0, 0, 0}, {0, 0, 1}, {0, 0, 2}} {
		if bytes.Contains(data[1:], bad) {
			t.Fatalf("unescaped NAL sequence %v", bad)
		}
	}
	r := reader{p: unescape(data)}
	if r.ue() != 0 || r.ue() != 2 || r.ue() != 0 {
		t.Fatal("bad slice header")
	}
	r.uint(4)
	r.ue()
	r.bit()
	r.bit()
	r.ue()
	r.ue()
	if r.ue() != 25 {
		t.Fatal("not I_PCM")
	}
	for r.pos%8 != 0 {
		if r.bit() != 0 {
			t.Fatal("nonzero alignment")
		}
	}
	start := r.pos / 8
	payload := r.p[start : start+384]
	if !bytes.Equal(payload[:256], make([]byte, 256)) {
		t.Fatal("black luma wrong")
	}
	for _, v := range payload[256:] {
		if v != 128 {
			t.Fatal("black chroma wrong")
		}
	}
}
