package h264

import "image"

// Table 9-4(a), Inter column: codeNum -> coded_block_pattern for 4:2:0.
var interCBP = [48]uint8{
	0, 16, 1, 2, 4, 8, 32, 3, 5, 10, 12, 15, 47, 7, 11, 13,
	14, 6, 9, 31, 35, 37, 42, 44, 33, 34, 36, 40, 39, 43, 45, 46,
	17, 18, 20, 24, 19, 21, 26, 28, 23, 27, 29, 30, 22, 25, 38, 41,
}

var interCBPCode = func() (out [48]uint32) {
	for code, cbp := range interCBP {
		out[cbp] = uint32(code)
	}
	return
}()

type interMB struct {
	luma   [16][16]int // row-major transform coefficients
	dc     [2][4]int
	ac     [2][4][16]int
	pixels [3][256]byte // compact 16x16 Y, 8x8 Cb/Cr in the first 64 entries
	cbp    uint8
	sse    int64
}

// makeInterMB compares against the actual reference reconstruction. The
// forward quantizer is an encoder choice; the flat inverse scaling, chroma DC
// Hadamard and clipping are the same normative operations as intra.
func makeInterMB(src, ref picture420, mx, my, qp int) interMB {
	var mb interMB
	for c := range 3 {
		stride, size, bx, by, qpc, blocks := src.stride, 16, mx, my, qp, 16
		if c != 0 {
			stride, size, bx, by, qpc, blocks = stride/2, 8, mx/2, my/2, chromaQP(max(4, qp)), 4
		}
		// Quantize directly into the returned coefficients. Every active row is
		// overwritten by quant4, so a second zeroed 16x16 temporary and copies
		// are unnecessary (especially for the four-block chroma planes).
		q := mb.luma[:]
		if c != 0 {
			q = mb.ac[c-1][:]
		}
		var dc [4]int
		for b := range blocks {
			x, y := (b/4%2)*8+b%2*4, b/8*8+(b/2%2)*4
			if c != 0 {
				x, y = b%2*4, b/2*4
			}
			var v [16]int
			for j := range 4 {
				for i := range 4 {
					p := (by+y+j)*stride + bx + x + i
					v[j*4+i] = int(src.planes[c][p]) - int(ref.planes[c][p])
				}
			}
			v = forward4(v)
			if c != 0 {
				dc[b], v[0] = v[0], 0
			}
			q[b] = quant4(v, qpc)
			for _, level := range q[b] {
				if level != 0 {
					if c == 0 {
						mb.cbp |= 1 << (b / 4)
					} else {
						mb.cbp |= 32 // chroma DC+AC present for BOTH components
					}
				}
			}
		}
		if c != 0 {
			dc = hadamard2(dc)
			for i := range dc {
				dc[i] = quantize(dc[i], forwardScale[qpc%6][0], 16+qpc/6)
				if dc[i] != 0 && mb.cbp/16 == 0 {
					mb.cbp |= 16 // DC only, unless either plane needs AC
				}
			}
			mb.dc[c-1] = dc
			dc = hadamard2(dc)
		}
		for b := range blocks {
			x, y := (b/4%2)*8+b%2*4, b/8*8+(b/2%2)*4
			if c != 0 {
				x, y = b%2*4, b/2*4
			}
			v := dequant4(q[b], qpc)
			if c != 0 {
				v[0] = dc[b] * (inverseScale[qpc%6][0] << (qpc / 6)) >> 1
			}
			v = inverse4(v)
			for j := range 4 {
				for i := range 4 {
					p := (by+y+j)*stride + bx + x + i
					pixel := max(0, min(255, int(ref.planes[c][p])+v[j*4+i]))
					mb.pixels[c][(y+j)*size+x+i] = byte(pixel)
					d := int64(int(src.planes[c][p]) - pixel)
					mb.sse += d * d
				}
			}
		}
	}
	// If Cb had DC and Cr needs AC (or vice versa), the shared chroma CBP
	// must be 2, never the reserved value 3.
	if mb.cbp/16 == 3 {
		mb.cbp = mb.cbp&15 | 32
	}
	return mb
}

// writeInterMB updates only this macroblock's counts. A single active L0
// reference infers ref_idx=0. All available inter vectors on this legacy path are
// zero, so every predictor branch infers zero and mvd=(0,0) yields mv=(0,0).
func writeInterMB(w *bits, mb *interMB, counts [3][]uint8, pw, mx, my int) {
	writeInterMBDelta(w, mb, counts, pw, mx, my, 0, 0)
}

// Differences are signed quarter-luma units, relative to the normative spatial
// predictor. The one active L0 reference infers ref_idx_l0=0.
func writeInterMBDelta(w *bits, mb *interMB, counts [3][]uint8, pw, mx, my, dx, dy int) {
	w.ue(0) // P_L0_16x16, one partition
	w.se(int32(dx))
	w.se(int32(dy))
	w.ue(interCBPCode[mb.cbp])
	if mb.cbp == 0 {
		return // no mb_qp_delta or residual syntax
	}
	w.se(0)
	for b := range 16 {
		if mb.cbp&(1<<(b/4)) == 0 {
			continue // zero coefficients/counts inferred for the whole 8x8
		}
		x, y := mx+(b/4%2)*8+b%2*4, my+b/8*8+(b/2%2)*4
		var scan [16]int
		for i, pos := range frameScan {
			scan[i] = mb.luma[b][pos]
		}
		nc := neighborCount(counts[0], pw/4, x/4, y/4)
		counts[0][y/4*(pw/4)+x/4] = uint8(cavlc(w, scan[:], nc))
	}
	if mb.cbp/16 != 0 {
		for c := range 2 {
			cavlc(w, mb.dc[c][:], -1)
		}
	}
	if mb.cbp/16 == 2 {
		for c := range 2 {
			for b := range 4 {
				x, y := mx/2+b%2*4, my/2+b/2*4
				var scan [15]int
				for i, pos := range frameScan[1:] {
					scan[i] = mb.ac[c][b][pos]
				}
				nc := neighborCount(counts[c+1], pw/8, x/4, y/4)
				counts[c+1][y/4*(pw/8)+x/4] = uint8(cavlc(w, scan[:], nc))
			}
		}
	}
}

func clearMBContext(counts [3][]uint8, modes []uint8, pw, mx, my int) {
	for c := range 3 {
		stride, x, y, n := pw/4, mx/4, my/4, 4
		if c != 0 {
			stride, x, y, n = stride/2, x/2, y/2, 2
		}
		for j := range n {
			for i := range n {
				counts[c][(y+j)*stride+x+i] = 0
				if c == 0 && modes != nil {
					modes[(y+j)*stride+x+i] = intraDC
				}
			}
		}
	}
}

func (mb *interMB) reconstruct(rec picture420, mx, my int) {
	for c := range 3 {
		stride, size, x, y := rec.stride, 16, mx, my
		if c != 0 {
			stride, size, x, y = stride/2, 8, x/2, y/2
		}
		for j := range size {
			copy(rec.planes[c][(y+j)*stride+x:(y+j)*stride+x+size], mb.pixels[c][j*size:(j+1)*size])
		}
	}
}

func macroblockSSE(a, b picture420, mx, my int) int64 {
	var sum int64
	for c := range 3 {
		stride, size, x, y := a.stride, 16, mx, my
		if c != 0 {
			stride, size, x, y = stride/2, 8, x/2, y/2
		}
		for j := range size {
			for i := range size {
				p := (y+j)*stride + x + i
				d := int64(a.planes[c][p]) - int64(b.planes[c][p])
				sum += d * d
			}
		}
	}
	return sum
}

func bitLength(w *bits) int {
	n := len(w.data) * 8
	if w.used != 0 {
		n -= 8 - int(w.used)
	}
	return n
}

func appendBits(w, src *bits) {
	if w.used == 0 {
		w.data = append(w.data, src.data...)
		w.used = src.used
		return
	}
	full := len(src.data)
	if src.used != 0 {
		full--
	}
	// The destination alignment stays fixed for each complete source byte.
	// Carry its low bits into a new partial byte instead of expanding 8 bits.
	used := w.used
	for _, b := range src.data[:full] {
		w.data[len(w.data)-1] |= b >> used
		w.data = append(w.data, b<<(8-used))
	}
	if src.used != 0 {
		w.uint(uint32(src.data[full]>>(8-src.used)), int(src.used))
	}
}

func ueLength(v int) int {
	n := 1
	for v++; v > 1; v >>= 1 {
		n += 2
	}
	return n
}

// The mutex-owned workspace retains conversion/context/syntax buffers and one
// spare reconstruction. Reference and target rotate after successful P coding;
// returned NALs are independently allocated. Reset releases the whole workspace.
type interWorkspace struct {
	src, rec                         picture420
	counts                           [3][]uint8
	modes, scratch                   []byte
	interScratch, intraScratch, rbsp []byte
	motion                           []motionInfo
}

func (e *PredictiveEncoder) residualWorkspace(pw, ph int) *interWorkspace {
	if e.workspace == nil {
		ws := &interWorkspace{
			src: newPicture420(pw, ph), rec: newPicture420(pw, ph),
			counts:       [3][]uint8{make([]uint8, pw*ph/16), make([]uint8, pw*ph/64), make([]uint8, pw*ph/64)},
			intraScratch: make([]byte, 4096), rbsp: make([]byte, 0, pw*ph/2),
		}
		if e.motionRadius == 0 {
			ws.interScratch = make([]byte, 4096)
		}
		if e.intra.modeSearch {
			ws.modes, ws.scratch = make([]byte, pw*ph/16), make([]byte, 128)
		}
		e.workspace = ws
	}
	ws := e.workspace
	for _, counts := range ws.counts {
		clear(counts)
	}
	for i := range ws.modes {
		ws.modes[i] = intraDC
	}
	// Pixels need no clear: conversion overwrites every source sample; every
	// macroblock trial/selection fully writes its target before it can be used.
	return ws
}

func (e *PredictiveEncoder) encodePResidual(img *image.RGBA, frameNum uint8) ([]byte, picture420, int) {
	width, height := e.intra.geometry.Width, e.intra.geometry.Height
	pw, ph := (width+15)/16*16, (height+15)/16*16
	ws := e.residualWorkspace(pw, ph)
	src, rec := ws.src, ws.rec
	fillPicture420(src, img, width, height, pw, ph)
	counts, modes, scratch := ws.counts, ws.modes, ws.scratch
	interScratch, intraScratch := ws.interScratch, ws.intraScratch
	w := bits{data: ws.rbsp[:0]}
	w.ue(0)
	w.ue(0)
	w.ue(0)
	w.uint(uint32(frameNum), 4)
	w.bit(0)
	w.bit(0)
	w.bit(0)
	w.se(int32(e.intra.qp - 26))
	w.ue(1)
	step := int64(inverseScale[e.intra.qp%6][0] << (e.intra.qp / 6))
	lambda := step * step
	run, skipped := 0, 0
	for my := 0; my < ph; my += 16 {
		for mx := 0; mx < pw; mx += 16 {
			if equalMacroblock(src, e.reference, mx, my) {
				copyMacroblock(rec, e.reference, mx, my)
				run++
				skipped++
				continue
			}
			mb := makeInterMB(src, e.reference, mx, my, e.intra.qp)
			inter := bits{data: interScratch[:0]}
			writeInterMB(&inter, &mb, counts, pw, mx, my)
			interCost := mb.sse*2048 + lambda*int64(ueLength(run)+bitLength(&inter))
			// A zero-CBP inter reconstruction is IDENTICAL to a legal P_SKIP.
			// This is the only non-exact skip candidate. No nonzero coefficient
			// is removed to manufacture a cheaper skip.
			if mb.cbp == 0 {
				interCost = mb.sse*2048 + lambda*int64(ueLength(run+1))
			}
			clearMBContext(counts, modes, pw, mx, my)
			intra := bits{data: intraScratch[:0]}
			intra.ue(5)
			e.intra.writeIntraMB(&intra, src, rec, counts, modes, scratch, mx, my)
			intraCost := macroblockSSE(src, rec, mx, my)*2048 + lambda*int64(ueLength(run)+bitLength(&intra))
			if intraCost < interCost { // ties prefer inter/skip deterministically
				w.ue(uint32(run))
				run = 0
				appendBits(&w, &intra)
				continue
			}
			// Discard the intra trial's pixels/counts/modes; only its own MB
			// was touched. Contexts of previous selected MBs remain intact.
			clearMBContext(counts, modes, pw, mx, my)
			mb.reconstruct(rec, mx, my)
			if mb.cbp == 0 {
				run++
				skipped++
			} else {
				w.ue(uint32(run))
				run = 0
				writeInterMB(&w, &mb, counts, pw, mx, my)
			}
		}
	}
	if run > 0 {
		w.ue(uint32(run))
	}
	w.trailing()
	ws.rbsp = w.data
	// e.reference is still the old reference here. Encode commits rec only
	// after return, so the next target is always distinct from its reference.
	ws.rec = e.reference
	return nal(0x41, w.data), rec, skipped
}
