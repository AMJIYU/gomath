package h264

// cavlcBitLength computes the exact length of the same syntax emitted by
// cavlc, including tokens, signs, adaptive levels, total_zeros and run_before.
// Losing RDO candidates need this length and TotalCoeff, but not a byte buffer.
// Keep the writer as the byte-producing authority; differential tests compare
// this counter and its contexts with that writer across the supported domain.
func cavlcBitLength(coeff []int, nC int) (length, total int) {
	var positions, levels [16]int
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
	length = int(coeffToken[table][total][trailing].size)
	if total == 0 {
		return
	}
	length += trailing
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
		length += levelBitLength(code, suffix)
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
			length += int(chromaDCZeros[total-1][zeros].size)
		} else {
			length += int(totalZeros[total-1][zeros].size)
		}
	}
	for i := total - 1; i > 0 && zeros > 0; i-- {
		run := positions[i] - positions[i-1] - 1
		length += int(runBefore[min(zeros, 7)-1][run].size)
		zeros -= run
	}
	return
}

func levelBitLength(code, suffix int) int {
	prefix, size := code>>suffix, suffix
	if suffix == 0 && code >= 14 && code < 30 {
		prefix, size = 14, 4
	} else if prefix >= 15 {
		prefix, size = 15, 12
	}
	return prefix + 1 + size
}

func signedBitLength(v int) int {
	if v > 0 {
		return ueLength(2*v - 1)
	}
	return ueLength(-2 * v)
}

// Exact counterpart of writeInterMBDelta. Only current-MB counts are updated;
// the caller starts them at zero, retaining the selected neighboring contexts.
func interBitLength(mb *interMB, counts [3][]uint8, pw, mx, my, dx, dy int) int {
	length := 1 + signedBitLength(dx) + signedBitLength(dy) + ueLength(int(interCBPCode[mb.cbp]))
	if mb.cbp == 0 {
		return length
	}
	length++ // mb_qp_delta=0
	for b := range 16 {
		if mb.cbp&(1<<(b/4)) == 0 {
			continue
		}
		x, y := mx+(b/4%2)*8+b%2*4, my+b/8*8+(b/2%2)*4
		var scan [16]int
		for i, pos := range frameScan {
			scan[i] = mb.luma[b][pos]
		}
		n, total := cavlcBitLength(scan[:], neighborCount(counts[0], pw/4, x/4, y/4))
		length += n
		counts[0][y/4*(pw/4)+x/4] = uint8(total)
	}
	if mb.cbp/16 != 0 {
		for c := range 2 {
			n, _ := cavlcBitLength(mb.dc[c][:], -1)
			length += n
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
				n, total := cavlcBitLength(scan[:], neighborCount(counts[c+1], pw/8, x/4, y/4))
				length += n
				counts[c+1][y/4*(pw/8)+x/4] = uint8(total)
			}
		}
	}
	return length
}
