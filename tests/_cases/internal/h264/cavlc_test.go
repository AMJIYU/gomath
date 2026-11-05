package h264

import (
	"math/rand/v2"
	"slices"
	"testing"
)

// A direct implementation of the normative decoder, kept exclusively in
// tests. This uses prefix matching rather than the encoder's level/run logic.
func readVLC(r *reader, row []vlc) int {
	var value uint32
	for size := 1; size <= 16; size++ {
		value = value*2 + r.bit()
		for i, v := range row {
			if int(v.size) == size && uint32(v.value) == value {
				return i
			}
		}
	}
	panic("invalid VLC")
}

func readCAVLC(r *reader, n, nc int) []int {
	table := 0
	if nc < 0 {
		table = 4
	} else if nc >= 8 {
		table = 3
	} else if nc >= 4 {
		table = 2
	} else if nc >= 2 {
		table = 1
	}
	flat := make([]vlc, 17*4)
	for i, v := range coeffToken[table] {
		copy(flat[4*i:], v[:])
	}
	token := readVLC(r, flat)
	total, trailing := token/4, token%4
	coeff := make([]int, n)
	if total == 0 {
		return coeff
	}
	var levels, runs [16]int
	for i := range trailing {
		levels[i] = 1 - int(2*r.bit())
	}
	suffixLength := 0
	if total > 10 && trailing < 3 {
		suffixLength = 1
	}
	for i := trailing; i < total; i++ {
		prefix := 0
		for r.bit() == 0 {
			prefix++
		}
		if prefix > 15 {
			panic("level prefix exceeds Baseline syntax range")
		}
		size := suffixLength
		if prefix == 14 && suffixLength == 0 {
			size = 4
		} else if prefix >= 15 {
			size = prefix - 3
		}
		code := (min(prefix, 15) << suffixLength) + int(r.uint(size))
		if prefix >= 15 && suffixLength == 0 {
			code += 15
		}
		if i == trailing && trailing < 3 {
			code += 2
		}
		if code%2 == 0 {
			levels[i] = (code + 2) / 2
		} else {
			levels[i] = -(code + 1) / 2
		}
		if suffixLength == 0 {
			suffixLength = 1
		}
		if absInt(levels[i]) > 3<<(suffixLength-1) && suffixLength < 6 {
			suffixLength++
		}
	}
	zeros := 0
	if total < n {
		if nc < 0 {
			zeros = readVLC(r, chromaDCZeros[total-1][:])
		} else {
			zeros = readVLC(r, totalZeros[total-1][:])
		}
	}
	for i := 0; i < total-1; i++ {
		if zeros > 0 {
			runs[i] = readVLC(r, runBefore[min(zeros, 7)-1][:])
		}
		zeros -= runs[i]
	}
	runs[total-1] = zeros
	pos := -1
	for i := total - 1; i >= 0; i-- {
		pos += runs[i] + 1
		coeff[pos] = levels[i]
	}
	return coeff
}

func TestCAVLCResidualRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewPCG(18, 264))
	for _, nc := range []int{-1, 0, 1, 2, 3, 4, 7, 8, 16} {
		for _, n := range []int{4, 15, 16} {
			if (nc < 0) != (n == 4) {
				continue
			}
			for trial := range 1000 {
				coeff := make([]int, n)
				total := 0
				for i := range coeff {
					if trial%17 != 0 && rng.IntN(3) == 0 {
						continue
					}
					switch trial % 5 {
					case 0:
						coeff[i] = 0
					case 1:
						coeff[i] = rng.IntN(3) - 1
					case 2:
						coeff[i] = rng.IntN(33) - 16
					case 3:
						coeff[i] = rng.IntN(3001) - 1500
					case 4:
						coeff[i] = rng.IntN(7) - 3
					}
					if coeff[i] != 0 {
						total++
					}
				}
				var w bits
				if got := cavlc(&w, coeff, nc); got != total {
					t.Fatalf("total coefficients: got %d want %d", got, total)
				}
				used := (len(w.data)-1)*8 + int(w.used)
				if w.used == 0 {
					used = len(w.data) * 8
				}
				w.trailing()
				r := reader{p: w.data}
				got := readCAVLC(&r, n, nc)
				if !slices.Equal(got, coeff) || r.pos != used {
					t.Fatalf("nc=%d n=%d trial=%d: got %v want %v bits=%d/%d", nc, n, trial, got, coeff, r.pos, used)
				}
			}
		}
	}
}

func TestCAVLCNormativeTokens(t *testing.T) {
	for _, c := range []struct {
		coeff []int
		nc    int
		bits  string
	}{
		{make([]int, 16), 0, "1"},
		{make([]int, 16), 2, "11"},
		{make([]int, 16), 4, "1111"},
		{make([]int, 16), 8, "000011"},
		{make([]int, 4), -1, "01"},
		{[]int{1, 0, 0, 0}, -1, "101"}, // token=1 sign=0 total_zeros=1
		{[]int{-1, 0, 0, 0}, -1, "111"},
	} {
		var w bits
		cavlc(&w, c.coeff, c.nc)
		if (len(w.data)-1)*8+int(w.used) != len(c.bits) {
			t.Fatal("unexpected token length")
		}
		r := reader{p: w.data}
		for _, bit := range c.bits {
			if r.bit() != uint32(bit-'0') {
				t.Fatal("unexpected token bits", c)
			}
		}
	}
}

func TestTransformAndQuantizationBounds(t *testing.T) {
	var constant [16]int
	for i := range constant {
		constant[i] = 100
	}
	dc := forward4(constant)
	if dc[0] != 1600 || !slices.Equal(dc[1:], make([]int, 15)) {
		t.Fatal("constant transform", dc)
	}
	if inverse4(dequant4(quant4(dc, 0), 0)) != constant {
		t.Fatal("constant reconstruction")
	}
	for qp := range 52 {
		// Every sign pattern of extremal samples, a stronger transform bound
		// check than random inputs. All emitted levels fit Baseline escapes.
		for mask := range 1 << 16 {
			var v [16]int
			for i := range v {
				v[i] = -255
				if mask&(1<<i) != 0 {
					v[i] = 255
				}
			}
			q := quant4(forward4(v), qp)
			for _, level := range q {
				if absInt(level) > 2047 {
					t.Fatalf("QP=%d residual escapes Baseline range: %d", qp, level)
				}
			}
		}
		for mask := range 16 {
			var chroma [4]int
			for i := range chroma {
				chroma[i] = -16 * 255
				if mask&(1<<i) != 0 {
					chroma[i] = 16 * 255
				}
			}
			qpc := chromaQP(max(4, qp))
			for _, dc := range hadamard2(chroma) {
				if absInt(quantize(dc, forwardScale[qpc%6][0], 16+qpc/6)) > 2047 {
					t.Fatal("chroma DC exceeds Baseline CAVLC level bounds")
				}
			}
		}
	}
}
