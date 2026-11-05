package h264

// writeVLC emits a normative table entry, including leading zero bits.
func (w *bits) writeVLC(v vlc) { w.uint(uint32(v.value), int(v.size)) }

// cavlc writes residual_block_cavlc (H.264 9.2). Coefficients are already in
// frame scan order. nC == -1 selects the 4:2:0 chroma DC table; other blocks
// contain 16 luma or 15 chroma AC coefficients. The return value is TotalCoeff.
func cavlc(w *bits, coeff []int, nC int) int {
	var (
		positions [16]int
		levels    [16]int
		total     int
	)
	for i, c := range coeff {
		if c != 0 {
			positions[total], levels[total] = i, c
			total++
		}
	}
	trailing := 0
	for trailing < min(3, total) && absInt(levels[total-1-trailing]) == 1 {
		trailing++
	}
	table := 0
	switch {
	case nC == -1:
		table = 4
	case nC >= 8:
		table = 3
	case nC >= 4:
		table = 2
	case nC >= 2:
		table = 1
	}
	w.writeVLC(coeffToken[table][total][trailing])
	if total == 0 {
		return 0
	}
	for i := 0; i < trailing; i++ {
		if levels[total-1-i] < 0 {
			w.bit(1)
		} else {
			w.bit(0)
		}
	}
	suffix := 0
	if total > 10 && trailing < 3 {
		suffix = 1
	}
	for i := total - 1 - trailing; i >= 0; i-- {
		level := levels[i]
		code := 2*absInt(level) - 2
		if level < 0 {
			code++
		}
		if i == total-1-trailing && trailing < 3 {
			code -= 2
		}
		writeLevel(w, code, suffix)
		if suffix == 0 {
			suffix = 1
		}
		if suffix < 6 && absInt(level) > 3<<(suffix-1) {
			suffix++
		}
	}
	zeros := positions[total-1] + 1 - total
	if total < len(coeff) {
		if nC == -1 {
			w.writeVLC(chromaDCZeros[total-1][zeros])
		} else {
			w.writeVLC(totalZeros[total-1][zeros])
		}
	}
	for i := total - 1; i > 0 && zeros > 0; i-- {
		run := positions[i] - positions[i-1] - 1
		w.writeVLC(runBefore[min(zeros, 7)-1][run])
		zeros -= run
	}
	return total
}

// writeLevel inverts 9.2.2, including the suffixLength==0 prefix-14 special
// case. 8-bit residuals from our quantizer fit Baseline's prefix <= 15 limit.
func writeLevel(w *bits, code, suffix int) {
	prefix, size, tail := code>>suffix, suffix, code&((1<<suffix)-1)
	if suffix == 0 && code >= 14 && code < 30 {
		prefix, size, tail = 14, 4, code-14
	} else if prefix >= 15 {
		prefix, size, tail = 15, 12, code-(15<<suffix)
		if suffix == 0 {
			tail -= 15
		}
	}
	w.uint(0, prefix)
	w.bit(1)
	w.uint(uint32(tail), size)
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
