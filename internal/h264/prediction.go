package h264

const (
	intraVertical   uint8 = 0
	intraHorizontal uint8 = 1
	intraDC         uint8 = 2
)

type searchedLumaMB struct {
	coeff   [16][16]int // quantized coefficients in frame scan order
	context [16]int     // nC at the point each block was reconstructed
}

type lumaChoice struct {
	mode   uint8
	coeff  [16]int
	pixels [16]byte
	total  uint8
}

// Intra blocks belong to one progressive slice; unavailable A/B neighbors
// occur at picture edges. Callers store DC for inter neighbors. H.264 8.3.1.1 makes
// the predicted mode DC if EITHER neighbor is unavailable; substituting DC
// for just one input to min(A,B) would be incorrect at the picture edges.
func predictedIntra4x4(modes []uint8, stride, bx, by int) uint8 {
	if bx == 0 || by == 0 {
		return intraDC
	}
	return min(modes[by*stride+bx-1], modes[(by-1)*stride+bx])
}

func writeIntra4x4Mode(w *bits, predicted, mode uint8) {
	if predicted == mode {
		w.bit(1) // prev_intra4x4_pred_mode_flag
		return
	}
	w.bit(0)
	rem := mode
	if mode > predicted {
		rem--
	}
	w.uint(uint32(rem), 3) // rem_intra4x4_pred_mode excludes the predicted mode
}

func searchLumaMB(w *bits, src, rec []byte, counts, modes []uint8, stride, mx, my, qp int, scratch []byte) searchedLumaMB {
	var mb searchedLumaMB
	for block := range 16 {
		x := mx + (block/4%2)*8 + (block%2)*4
		y := my + (block/8)*8 + (block/2%2)*4
		bx, by := x/4, y/4
		predicted := predictedIntra4x4(modes, stride/4, bx, by)
		nc := neighborCount(counts, stride/4, bx, by)
		choice := chooseLuma4x4(src, rec, stride, x, y, qp, nc, predicted, scratch)
		writeIntra4x4Mode(w, predicted, choice.mode)
		mb.coeff[block], mb.context[block] = choice.coeff, nc
		modes[by*(stride/4)+bx] = choice.mode
		counts[by*(stride/4)+bx] = choice.total
		for j := range 4 {
			copy(rec[(y+j)*stride+x:(y+j)*stride+x+4], choice.pixels[j*4:j*4+4])
		}
	}
	return mb
}

// chooseLuma4x4 evaluates only legal modes, using reconstructed neighbors and
// the same quantizer/inverse as the final bitstream. The integer cost is
// SSE*2048 + step^2*(CAVLC bits + mode bits), with step the DC inverse scale.
// This is a bounded, greedy block decision, not whole-frame optimization;
// emulation-prevention bytes and effects on future blocks are not included.
func chooseLuma4x4(src, rec []byte, stride, x, y, qp, nc int, predicted uint8, scratch []byte) lumaChoice {
	var target [16]int
	for j := range 4 {
		for i := range 4 {
			target[j*4+i] = int(src[(y+j)*stride+x+i])
		}
	}
	dc := lumaDC(rec, stride, x, y)
	step := int64(inverseScale[qp%6][0] << (qp / 6))
	bestCost := int64(1<<63 - 1)
	var best lumaChoice
	// DC first gives a deterministic tie break. Directional modes are illegal
	// when their reference row/column is unavailable, even on padded samples.
	for _, mode := range [...]uint8{intraDC, intraVertical, intraHorizontal} {
		if mode == intraVertical && y == 0 || mode == intraHorizontal && x == 0 {
			continue
		}
		var prediction, residual [16]int
		for j := range 4 {
			for i := range 4 {
				v := dc
				switch mode {
				case intraVertical:
					v = int(rec[(y-1)*stride+x+i])
				case intraHorizontal:
					v = int(rec[(y+j)*stride+x-1])
				}
				prediction[j*4+i] = v
				residual[j*4+i] = target[j*4+i] - v
			}
		}
		q := quant4(forward4(residual), qp)
		choice := lumaChoice{mode: mode}
		for i, pos := range frameScan {
			choice.coeff[i] = q[pos]
		}
		// A block has at most 16 levels, each <= 28 bits, plus VLC tokens.
		// Reuse one frame-local buffer instead of allocating per candidate.
		// Measure actual CAVLC syntax with the current neighbor context, not an NNZ
		// proxy that misses long levels and runs.
		trial := bits{data: scratch[:0]}
		choice.total = uint8(cavlc(&trial, choice.coeff[:], nc))
		rate := len(trial.data) * 8
		if trial.used != 0 {
			rate -= 8 - int(trial.used)
		}
		if mode == predicted {
			rate++
		} else {
			rate += 4
		}
		rebuilt := inverse4(dequant4(q, qp))
		var distortion int64
		for i := range rebuilt {
			v := max(0, min(255, prediction[i]+rebuilt[i]))
			choice.pixels[i] = byte(v)
			d := int64(target[i] - v)
			distortion += d * d
		}
		cost := distortion*2048 + step*step*int64(rate)
		if cost < bestCost {
			best, bestCost = choice, cost
		}
	}
	return best
}
