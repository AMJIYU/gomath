package h264

import (
	"bytes"
	"math/rand/v2"
	"testing"
)

var motionSyntaxSink int

func motionSyntaxInputs(kind string) (interMB, [3][]uint8) {
	src, ref := newPicture420(16, 16), equalReference128(16, 16)
	for c := range 3 {
		copy(src.planes[c], ref.planes[c])
		if kind == "dense" {
			for i := range src.planes[c] {
				src.planes[c][i] = byte(i*73 + c*19)
			}
		}
	}
	if kind == "sparse" {
		src.planes[0][0], src.planes[2][0] = 255, 0
	}
	counts := [3][]uint8{make([]uint8, 64), make([]uint8, 16), make([]uint8, 16)}
	for _, c := range counts {
		for i := range c {
			c[i] = uint8(i % 17)
		}
	}
	return makeInterMB(src, ref, 0, 0, 24), counts
}

// One exact MB's complete syntax, with the same nonzero surrounding contexts
// and MVD on every iteration. Setup is excluded; output capacity is preallocated
// just like a motion trial. Keep this writer baseline unchanged after profiling.
func BenchmarkMotionSyntaxWriter(b *testing.B) {
	for _, kind := range []string{"zero", "sparse", "dense"} {
		b.Run(kind, func(b *testing.B) {
			mb, counts := motionSyntaxInputs(kind)
			buf := make([]byte, 4096)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				clearMBContext(counts, nil, 32, 16, 16)
				w := bits{data: buf[:0]}
				writeInterMBDelta(&w, &mb, counts, 32, 16, 16, -12, 8)
				motionSyntaxSink = bitLength(&w)
			}
		})
	}
}

func BenchmarkMotionSyntaxCounter(b *testing.B) {
	for _, kind := range []string{"zero", "sparse", "dense"} {
		b.Run(kind, func(b *testing.B) {
			mb, counts := motionSyntaxInputs(kind)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				clearMBContext(counts, nil, 32, 16, 16)
				motionSyntaxSink = interBitLength(&mb, counts, 32, 16, 16, -12, 8)
			}
		})
	}
}

func TestCAVLCBitLengthMatchesWriter(t *testing.T) {
	buf := make([]byte, 4096)
	for suffix := range 7 {
		for code := range 4111 {
			w := bits{data: buf[:0]}
			w.uint(5, 3)
			writeLevel(&w, code, suffix)
			if got := levelBitLength(code, suffix); got != bitLength(&w)-3 {
				t.Fatal("level length", code, suffix, got, bitLength(&w)-3)
			}
		}
	}
	check := func(coeff []int, nc, prefix int) {
		t.Helper()
		w := bits{data: buf[:0]}
		w.uint(0x55, prefix)
		wantTotal := cavlc(&w, coeff, nc)
		length, total := cavlcBitLength(coeff, nc)
		if total != wantTotal || length != bitLength(&w)-prefix {
			t.Fatal("CAVLC counter", coeff, nc, prefix, length, bitLength(&w)-prefix, total, wantTotal)
		}
	}
	contexts := []int{0, 1, 2, 3, 4, 7, 8, 16}
	for _, n := range []int{4, 15, 16} {
		var coeff [16]int
		// Every coefficient-presence mask exercises all run/zero patterns and
		// trailing-one counts; rotate signs, context tables and byte alignment.
		for mask := range 1 << n {
			for i := range n {
				coeff[i] = 0
				if mask&(1<<i) != 0 {
					coeff[i] = 1 - 2*((mask>>uint((i+3)%n))&1)
				}
			}
			nc := contexts[mask%len(contexts)]
			if n == 4 {
				nc = -1
			}
			check(coeff[:n], nc, mask%8)
		}
		rng := rand.New(rand.NewPCG(73, uint64(n)))
		for sample := range 512 {
			for i := range n {
				coeff[i] = 0
				if rng.Uint32()%3 != 0 {
					coeff[i] = int(rng.Uint32()%2047) - 1023
				}
			}
			if n == 4 {
				check(coeff[:n], -1, sample%8)
			} else {
				for _, nc := range contexts {
					check(coeff[:n], nc, sample%8)
				}
			}
		}
	}
}

func TestInterBitLengthMatchesWriter(t *testing.T) {
	buf := make([]byte, 4096)
	check := func(mb *interMB, mx, my, dx, dy, seed int) {
		t.Helper()
		a := [3][]uint8{make([]uint8, 144), make([]uint8, 36), make([]uint8, 36)}
		b := [3][]uint8{make([]uint8, 144), make([]uint8, 36), make([]uint8, 36)}
		for c := range a {
			for i := range a[c] {
				a[c][i] = uint8((i*13 + seed) % 17)
			}
			copy(b[c], a[c])
		}
		clearMBContext(a, nil, 48, mx, my)
		clearMBContext(b, nil, 48, mx, my)
		w := bits{data: buf[:0]}
		writeInterMBDelta(&w, mb, a, 48, mx, my, dx, dy)
		if got := interBitLength(mb, b, 48, mx, my, dx, dy); got != bitLength(&w) {
			t.Fatal("MB bit length", mb.cbp, mx, my, dx, dy, got, bitLength(&w))
		}
		for c := range a {
			if !bytes.Equal(a[c], b[c]) {
				t.Fatal("MB count grid", mb.cbp, c)
			}
		}
	}
	for cbp := range 48 {
		var mb interMB
		mb.cbp = uint8(cbp)
		for i := range 16 {
			for j := range 16 {
				mb.luma[i][j] = (i*37+j*13)%103 - 51
			}
		}
		for c := range 2 {
			for i := range 4 {
				mb.dc[c][i] = (i*23+c*17)%41 - 20
				for j := range 16 {
					mb.ac[c][i][j] = (i*7+j*23+c*17)%101 - 50
				}
			}
		}
		for _, pos := range [][2]int{{0, 0}, {32, 0}, {0, 32}, {32, 32}, {16, 16}} {
			for _, dx := range []int{-32, -4, 0, 4, 32} {
				for _, dy := range []int{-32, -4, 0, 4, 32} {
					check(&mb, pos[0], pos[1], dx, dy, cbp)
				}
			}
		}
	}
	// Actual quantizer extrema at every QP, not just hand-built syntax levels.
	for qp := range 52 {
		for seed := range 16 {
			src, ref := newPicture420(16, 16), newPicture420(16, 16)
			for c := range 3 {
				for i := range src.planes[c] {
					src.planes[c][i], ref.planes[c][i] = byte(i*73+seed*19+c*7), byte(i*11+seed*47+c*3)
				}
			}
			mb := makeInterMB(src, ref, 0, 0, qp)
			check(&mb, 16, 16, seed*4-32, 32-seed*4, seed)
		}
	}
}

func TestMotionTrialStorage(t *testing.T) {
	for radius := range 5 {
		e, err := NewPredictive(34, 38, 24, PredictiveOptions{QP: 24, GOP: 7, InterResidual: true, MotionSearchRadius: radius})
		if err != nil {
			t.Fatal(err)
		}
		for f := range 3 {
			if _, err := e.Encode(motionImage("pan", 34, 38, f)); err != nil {
				t.Fatal(err)
			}
		}
		if ws := e.workspace; ws == nil || (radius == 0) != (len(ws.interScratch) == 4096) {
			t.Fatal("unexpected inter trial byte buffer", radius)
		}
		e.Reset()
		if e.workspace != nil || e.reference.planes[0] != nil {
			t.Fatal("retained motion workspace")
		}
	}
}
