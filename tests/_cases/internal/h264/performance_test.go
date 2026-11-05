package h264

import (
	"bytes"
	"fmt"
	"testing"
)

// Compare packed writes with a direct single-bit specification, including set
// bits above the requested width, empty widths and widths exceeding uint32.
func TestBitsBulkWrites(t *testing.T) {
	for prefix := range 8 {
		for n := range 65 {
			for _, v := range []uint32{0, 1, 0xffffffff, 0x89abcdef, 0x12345678, 0x80000000} {
				var w bits
				var want []byte
				used := 0
				write := func(bit byte) {
					if used == 0 {
						want = append(want, 0)
					}
					want[len(want)-1] |= (bit & 1) << uint(7-used)
					used = (used + 1) % 8
				}
				for i := range prefix {
					w.bit(uint32(i))
					write(byte(i))
				}
				w.uint(v, n)
				for i := n - 1; i >= 0; i-- {
					write(byte(v >> uint(i)))
				}
				if !bytes.Equal(w.data, want) || int(w.used) != used {
					t.Fatal("bulk bit write", prefix, n, v, w.data, want, w.used, used)
				}
			}
		}
	}
}

func pictureCapacity(p picture420) int {
	n := 0
	for _, plane := range p.planes {
		n += cap(plane)
	}
	return n
}

// Dynamic byte capacities retained by one encoder, excluding slice headers,
// scalar struct fields, returned NALs, caller inputs and the runtime allocator.
func encoderRetainedCapacity(e *PredictiveEncoder) int {
	n := pictureCapacity(e.reference)
	if ws := e.workspace; ws != nil {
		n += pictureCapacity(ws.src) + pictureCapacity(ws.rec)
		for _, count := range ws.counts {
			n += cap(count)
		}
		for _, buf := range [][]byte{ws.modes, ws.scratch, ws.interScratch, ws.intraScratch, ws.rbsp} {
			n += cap(buf)
		}
	}
	return n
}

func TestInterWorkspaceLifetime(t *testing.T) {
	e, _ := NewPredictive(34, 38, 24, PredictiveOptions{QP: 24, ModeSearch: true, GOP: 7, InterResidual: true})
	var nals, saved [][]byte
	for f := range 40 {
		out, err := e.Encode(interImage("drift", 34, 38, f))
		if err != nil {
			t.Fatal(err)
		}
		nals = append(nals, out.NAL)
		saved = append(saved, bytes.Clone(out.NAL))
		if ws := e.workspace; ws != nil {
			for c := range 3 {
				if &e.reference.planes[c][0] == &ws.rec.planes[c][0] || &e.reference.planes[c][0] == &ws.src.planes[c][0] || &ws.rec.planes[c][0] == &ws.src.planes[c][0] {
					t.Fatal("reference/target/source alias", f, c)
				}
			}
		}
	}
	ws := e.workspace
	if ws == nil {
		t.Fatal("workspace not used")
	}
	e.ForceIDR()
	if _, err := e.Encode(nil); err == nil || e.workspace != ws {
		t.Fatal("error consumed/replaced workspace")
	}
	out, err := e.Encode(interImage("static", 34, 38, 0))
	if err != nil || !out.Keyframe {
		t.Fatal("forced IDR recovery", err)
	}
	if e.workspace != ws {
		t.Fatal("ForceIDR discarded reusable storage")
	}
	e.Reset()
	if e.workspace != nil || e.reference.planes[0] != nil || encoderRetainedCapacity(e) != 0 {
		t.Fatal("Reset retained workspace/reference")
	}
	for f := range 3 {
		if _, err := e.Encode(interImage("static", 34, 38, f)); err != nil {
			t.Fatal(err)
		}
	}
	if e.workspace == ws {
		t.Fatal("Reset reused retained workspace")
	}
	for i, nal := range nals {
		if !bytes.Equal(nal, saved[i]) {
			t.Fatal("returned NAL was overwritten", i)
		}
	}
}

func TestInterRetainedCapacity(t *testing.T) {
	for _, search := range []bool{false, true} {
		for _, enabled := range []bool{false, true} {
			t.Run(fmt.Sprintf("search%t/inter%t", search, enabled), func(t *testing.T) {
				e, _ := NewPredictive(320, 180, 24, PredictiveOptions{QP: 24, ModeSearch: search, GOP: 60, InterResidual: enabled})
				im := interImage("static", 320, 180, 0)
				for range 60 {
					if _, err := e.Encode(im); err != nil {
						t.Fatal(err)
					}
				}
				if !enabled && e.workspace != nil {
					t.Fatal("disabled path allocated workspace")
				}
				t.Logf("retained dynamic byte capacity=%d", encoderRetainedCapacity(e))
				e.Reset()
				if encoderRetainedCapacity(e) != 0 {
					t.Fatal("Reset retained dynamic storage")
				}
			})
		}
	}
}
