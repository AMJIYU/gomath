// Package h264 implements pure-Go Constrained Baseline H.264 encoders.
// New preserves uncompressed I_PCM samples; NewIntra compresses input using
// spatial prediction, integer transforms, quantization and CAVLC. NewPredictive
// adds single-reference P_SKIP and optional residual/integer-motion coding.
package h264

import (
	"fmt"
	"image"
	"image/color"
)

type Encoder struct{ Width, Height, FPS int }

func New(width, height, fps int) (*Encoder, error) {
	if width < 2 || height < 2 || width > 4096 || height > 4096 || width%2 != 0 || height%2 != 0 || fps < 1 || fps > 120 {
		return nil, fmt.Errorf("H.264 I_PCM requires even dimensions 2..4096 and FPS 1..120")
	}
	return &Encoder{width, height, fps}, nil
}

type bits struct {
	data []byte
	used uint8
}

func (w *bits) bit(v uint32) {
	if w.used == 0 {
		w.data = append(w.data, 0)
	}
	w.data[len(w.data)-1] |= byte(v&1) << (7 - w.used)
	w.used = (w.used + 1) % 8
}

// Write whole byte fragments rather than repeatedly appending a single bit.
// Mask each partial fragment: callers may supply set bits above the width.
func (w *bits) uint(v uint32, n int) {
	for n > 0 {
		if w.used == 0 {
			if n >= 8 {
				n -= 8
				w.data = append(w.data, byte(v>>uint(n)))
				continue
			}
			w.data = append(w.data, byte(v<<uint(8-n)))
			w.used = uint8(n)
			return
		}
		free := 8 - int(w.used)
		take := min(n, free)
		n -= take
		fragment := byte(v>>uint(n)) & byte((1<<take)-1)
		w.data[len(w.data)-1] |= fragment << uint(free-take)
		w.used = (w.used + uint8(take)) & 7
	}
}
func (w *bits) ue(v uint32) {
	v++
	n := 0
	for z := v; z > 1; z >>= 1 {
		n++
	}
	w.uint(0, n)
	w.uint(v, n+1)
}
func (w *bits) se(v int32) {
	if v <= 0 {
		w.ue(uint32(-2 * v))
	} else {
		w.ue(uint32(2*v - 1))
	}
}
func (w *bits) align() {
	for w.used != 0 {
		w.bit(0)
	}
}
func (w *bits) trailing() { w.bit(1); w.align() }
func nal(header byte, rbsp []byte) []byte {
	out := make([]byte, 1, 1+len(rbsp)+len(rbsp)/100)
	out[0] = header
	zeros := 0
	for _, b := range rbsp {
		if zeros >= 2 && b <= 3 {
			out = append(out, 3)
			zeros = 0
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
func (e *Encoder) SPS() []byte {
	return e.sps(51)
}

func (e *Encoder) sps(level uint32) []byte {
	return e.spsReferences(level, 0)
}

func (e *Encoder) spsReferences(level, references uint32) []byte {
	var w bits
	w.uint(66, 8)
	w.uint(0xc0, 8)
	w.uint(level, 8)
	w.ue(0)
	w.ue(0)
	w.ue(2)
	w.ue(references) // max_num_ref_frames
	w.bit(0)
	w.ue(uint32((e.Width+15)/16 - 1))
	w.ue(uint32((e.Height+15)/16 - 1))
	w.bit(1)
	w.bit(1)
	right, bottom := ((e.Width+15)/16*16-e.Width)/2, ((e.Height+15)/16*16-e.Height)/2
	if right != 0 || bottom != 0 {
		w.bit(1)
		w.ue(0)
		w.ue(uint32(right))
		w.ue(0)
		w.ue(uint32(bottom))
	} else {
		w.bit(0)
	}
	w.bit(1)     // vui_parameters_present_flag
	w.bit(0)     // aspect_ratio_info_present_flag
	w.bit(0)     // overscan_info_present_flag
	w.bit(1)     // video_signal_type_present_flag
	w.uint(5, 3) // unspecified video format
	w.bit(1)     // video_full_range_flag
	w.bit(0)     // colour_description_present_flag
	w.bit(0)     // chroma_loc_info_present_flag
	w.bit(1)
	w.uint(1, 32)
	w.uint(uint32(e.FPS*2), 32)
	w.bit(1) // fixed frame timing
	w.bit(0)
	w.bit(0)
	w.bit(0)
	w.bit(0)
	w.trailing()
	return nal(0x67, w.data)
}
func (e *Encoder) PPS() []byte {
	return e.pps(0)
}

func (e *Encoder) pps(chromaOffset int32) []byte {
	var w bits
	w.ue(0)
	w.ue(0)
	w.bit(0)
	w.bit(0)
	w.ue(0)
	w.ue(0)
	w.ue(0)
	w.bit(0)
	w.uint(0, 2)
	w.se(0)
	w.se(0)
	w.se(chromaOffset)
	w.bit(1)
	w.bit(0)
	w.bit(0)
	w.trailing()
	return nal(0x68, w.data)
}
func (e *Encoder) Encode(img *image.RGBA) ([]byte, error) {
	if img == nil || img.Rect.Dx() != e.Width || img.Rect.Dy() != e.Height {
		return nil, fmt.Errorf("H.264 frame dimensions do not match encoder")
	}
	pw, ph := (e.Width+15)/16*16, (e.Height+15)/16*16
	yPlane := make([]byte, pw*ph)
	cbPlane := make([]byte, pw*ph/4)
	crPlane := make([]byte, pw*ph/4)
	for y := 0; y < ph; y += 2 {
		for x := 0; x < pw; x += 2 {
			var cbSum, crSum int
			for dy := range 2 {
				for dx := range 2 {
					sx, sy := min(x+dx, e.Width-1)+img.Rect.Min.X, min(y+dy, e.Height-1)+img.Rect.Min.Y
					i := img.PixOffset(sx, sy) // Premultiplied RGB naturally composites transparency onto black.
					Y, Cb, Cr := color.RGBToYCbCr(img.Pix[i], img.Pix[i+1], img.Pix[i+2])
					yPlane[(y+dy)*pw+x+dx] = Y
					cbSum += int(Cb)
					crSum += int(Cr)
				}
			}
			cbPlane[y/2*(pw/2)+x/2] = byte((cbSum + 2) / 4)
			crPlane[y/2*(pw/2)+x/2] = byte((crSum + 2) / 4)
		}
	}
	var w bits
	w.data = make([]byte, 0, pw*ph*3/2+pw*ph/128)
	w.ue(0)
	w.ue(2)
	w.ue(0)
	w.uint(0, 4)
	w.ue(0)
	w.bit(0)
	w.bit(0)
	w.se(0)
	w.ue(1)
	for my := 0; my < ph; my += 16 {
		for mx := 0; mx < pw; mx += 16 {
			w.ue(25)
			w.align()
			for y := range 16 {
				start := (my+y)*pw + mx
				w.data = append(w.data, yPlane[start:start+16]...)
			}
			for _, plane := range [][]byte{cbPlane, crPlane} {
				for y := range 8 {
					start := (my/2+y)*(pw/2) + mx/2
					w.data = append(w.data, plane[start:start+8]...)
				}
			}
		}
	}
	w.trailing()
	return nal(0x65, w.data), nil
}
