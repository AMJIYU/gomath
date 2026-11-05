package h264

import "image"

// Vectors are integer luma samples internally; syntax uses four times these
// components. Every partition in this bounded encoder is a complete 16x16 MB.
type motionVector struct{ x, y int }
type motionInfo struct {
	mv    motionVector
	inter bool // false means an available intra MB, with refIdx=-1 and mv=0
}

// H.264 8.4.1.3 / 8.4.1.3.1, restricted to progressive P_L0_16x16/ref0.
// Availability is distinct from intra: C-to-D fallback and the A-only rule
// depend on availability, while unique-reference matching depends on inter.
func predictMotion(grid []motionInfo, width, x, y int, skip bool) motionVector {
	neighbor := func(nx, ny int) (motionInfo, bool) {
		if nx < 0 || nx >= width || ny < 0 {
			return motionInfo{}, false
		}
		return grid[ny*width+nx], true
	}
	a, aa := neighbor(x-1, y)
	b, ba := neighbor(x, y-1)
	c, ca := neighbor(x+1, y-1)
	if !ca {
		c, ca = neighbor(x-1, y-1)
	}
	zero := motionVector{}
	// 8.4.1.1: these conditions force P_SKIP to mv0, not to the general MVP.
	if skip && (!aa || !ba || a.inter && a.mv == zero || b.inter && b.mv == zero) {
		return zero
	}
	if !ba && !ca && aa {
		b, c = a, a
	}
	n := 0
	var matching motionVector
	for _, v := range [3]motionInfo{a, b, c} {
		if v.inter {
			n++
			matching = v.mv
		}
	}
	if n == 1 {
		return matching
	}
	median := func(a, b, c int) int { return a + b + c - min(a, b, c) - max(a, b, c) }
	return motionVector{median(a.mv.x, b.mv.x, c.mv.x), median(a.mv.y, b.mv.y, c.mv.y)}
}

// One compact MB, using the same plane layout/stride as a 16x16 picture.
func motionBlock(p *[384]byte) picture420 {
	return picture420{stride: 16, planes: [3][]byte{p[:256], p[256:320], p[320:]}}
}

func sourceMotionBlock(dst, src picture420, mx, my int) {
	for c := range 3 {
		s, n, x, y := src.stride, 16, mx, my
		if c != 0 {
			s, n, x, y = s/2, 8, x/2, y/2
		}
		for j := range n {
			copy(dst.planes[c][j*n:(j+1)*n], src.planes[c][(y+j)*s+x:(y+j)*s+x+n])
		}
	}
}

// Integer luma requires no luma filter, but odd integer displacements require
// half-chroma interpolation in 4:2:0. This is 8.4.2.2.2 with fracC=0 or 4,
// algebraically reduced to denominator4. Each tap clips to the CODED reference
// plane, including reconstructed padding, not the cropped display rectangle.
func compensateMotion(dst, ref picture420, mx, my int, mv motionVector) {
	for c := range 3 {
		stride, n, x, y, dx, dy := ref.stride, 16, mx, my, mv.x, mv.y
		fx, fy := 0, 0
		if c != 0 {
			stride, n, x, y = stride/2, 8, x/2, y/2
			fx, fy, dx, dy = dx&1, dy&1, dx>>1, dy>>1
		}
		height := len(ref.planes[c]) / stride
		for j := range n {
			ya, yb := max(0, min(height-1, y+j+dy)), max(0, min(height-1, y+j+dy+1))
			for i := range n {
				xa, xb := max(0, min(stride-1, x+i+dx)), max(0, min(stride-1, x+i+dx+1))
				a := int(ref.planes[c][ya*stride+xa])
				if fx == 0 && fy == 0 {
					dst.planes[c][j*n+i] = byte(a)
					continue
				}
				b, cc, d := int(ref.planes[c][ya*stride+xb]), int(ref.planes[c][yb*stride+xa]), int(ref.planes[c][yb*stride+xb])
				dst.planes[c][j*n+i] = byte(((2-fx)*(2-fy)*a + fx*(2-fy)*b + (2-fx)*fy*cc + fx*fy*d + 2) >> 2)
			}
		}
	}
}

// Exhaustive full RDO, bounded to at most81 inter candidates and one intra per
// MB. No SAD shortlist, early exit or coefficient dropping changes quality.
func (e *PredictiveEncoder) encodePMotion(img *image.RGBA, frameNum uint8) ([]byte, picture420, int) {
	width, height := e.intra.geometry.Width, e.intra.geometry.Height
	pw, ph := (width+15)/16*16, (height+15)/16*16
	ws := e.residualWorkspace(pw, ph)
	if ws.motion == nil {
		ws.motion = make([]motionInfo, pw*ph/256)
	}
	clear(ws.motion)
	src, rec := ws.src, ws.rec
	fillPicture420(src, img, width, height, pw, ph)
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
	var sourceBytes, predictionBytes [384]byte
	source, prediction := motionBlock(&sourceBytes), motionBlock(&predictionBytes)
	for my := 0; my < ph; my += 16 {
		for mx := 0; mx < pw; mx += 16 {
			i := my/16*(pw/16) + mx/16
			mvp := predictMotion(ws.motion, pw/16, mx/16, my/16, false)
			skipMV := predictMotion(ws.motion, pw/16, mx/16, my/16, true)
			sourceMotionBlock(source, src, mx, my)
			var best interMB
			var bestMV motionVector
			bestCost, bestSkip := int64(1<<63-1), false
			trial := func(mv motionVector) {
				compensateMotion(prediction, e.reference, mx, my, mv)
				mb := makeInterMB(source, prediction, 0, 0, e.intra.qp)
				clearMBContext(ws.counts, nil, pw, mx, my)
				length := interBitLength(&mb, ws.counts, pw, mx, my, 4*(mv.x-mvp.x), 4*(mv.y-mvp.y))
				isSkip := mb.cbp == 0 && mv == skipMV
				rate := ueLength(run) + length
				if isSkip {
					rate = ueLength(run + 1)
				}
				cost := mb.sse*2048 + lambda*int64(rate)
				if cost < bestCost {
					best, bestMV, bestCost, bestSkip = mb, mv, cost, isSkip
				}
			}
			// Deterministic ties favor zero, then raster-order displacement.
			trial(motionVector{})
			for dy := -e.motionRadius; dy <= e.motionRadius; dy++ {
				for dx := -e.motionRadius; dx <= e.motionRadius; dx++ {
					if dx != 0 || dy != 0 {
						trial(motionVector{dx, dy})
					}
				}
			}
			clearMBContext(ws.counts, ws.modes, pw, mx, my)
			intra := bits{data: ws.intraScratch[:0]}
			intra.ue(5)
			e.intra.writeIntraMB(&intra, src, rec, ws.counts, ws.modes, ws.scratch, mx, my)
			intraCost := macroblockSSE(src, rec, mx, my)*2048 + lambda*int64(ueLength(run)+bitLength(&intra))
			if intraCost < bestCost {
				w.ue(uint32(run))
				run = 0
				appendBits(&w, &intra)
				ws.motion[i] = motionInfo{}
				continue
			}
			clearMBContext(ws.counts, ws.modes, pw, mx, my)
			best.reconstruct(rec, mx, my)
			ws.motion[i] = motionInfo{mv: bestMV, inter: true}
			if bestSkip {
				run++
				skipped++
			} else {
				w.ue(uint32(run))
				run = 0
				writeInterMBDelta(&w, &best, ws.counts, pw, mx, my, 4*(bestMV.x-mvp.x), 4*(bestMV.y-mvp.y))
			}
		}
	}
	if run > 0 {
		w.ue(uint32(run))
	}
	w.trailing()
	ws.rbsp, ws.rec = w.data, e.reference
	return nal(0x41, w.data), rec, skipped
}
