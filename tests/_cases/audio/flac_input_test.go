package audio

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func flacInputDirectory() string {
	if dir := os.Getenv("GOMATH_FLAC_INPUT_DIR"); dir != "" {
		return dir
	}
	return "flac_input" // TestMain relocates to tests/data/audio, including fuzz workers.
}

func TestReadFLACInputFixtures(t *testing.T) {
	dir := flacInputDirectory()
	b, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var entries []struct {
		File, SHA256, PCMSHA256       string
		Rate, Depth, Channels, Frames int
	}
	if err := json.Unmarshal(b, &entries); err != nil || len(entries) != 194 {
		t.Fatal("complete independent matrix required", len(entries), err)
	}
	var seen [4][8]bool
	for _, e := range entries {
		t.Run(e.File, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(dir, e.File))
			if err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256(data)
			if hex.EncodeToString(sum[:]) != e.SHA256 {
				t.Fatal("encoded fixture hash mismatch")
			}
			p, err := ReadFLAC(bytes.NewReader(data), FLACReadOptions{DiscardMetadata: true})
			if err != nil {
				t.Fatal(err)
			}
			if p.Format() != (Format{SampleRate: e.Rate, Channels: e.Channels}) || p.Frames() != e.Frames {
				t.Fatal("format/frame count changed", p.Format(), p.Frames())
			}
			want, err := os.ReadFile(filepath.Join(dir, e.File[:len(e.File)-5]+".raw"))
			if err != nil || len(want) != e.Frames*e.Channels*(e.Depth/8) {
				t.Fatal("missing complete integer oracle", err)
			}
			sum = sha256.Sum256(want)
			if hex.EncodeToString(sum[:]) != e.PCMSHA256 {
				t.Fatal("integer oracle hash mismatch")
			}
			// Compare every scalar before packing: casting alone could hide a
			// non-integer result or a high-bit/wrapping normalization error.
			for i, v := range p.samples {
				var code int64
				for j := e.Depth/8 - 1; j >= 0; j-- {
					code = code<<8 | int64(want[i*(e.Depth/8)+j])
				}
				code = code << uint(64-e.Depth) >> uint(64-e.Depth)
				if v != math.Ldexp(float64(code), 1-e.Depth) {
					t.Fatalf("sample %d: %.17g differs from code %d", i, v, code)
				}
			}
			seen[e.Depth/8-1][e.Channels-1] = true
		})
	}
	for d := range seen {
		for c, ok := range seen[d] {
			if !ok {
				t.Fatalf("missing %d-bit/%dch independent coverage", (d+1)*8, c+1)
			}
		}
	}
}

type flacInputHandCase struct {
	name                       string
	depth, channels, n, assign int
	codes                      []int64
	emit                       func(*flacTestBuilder)
}

func flacInputVerbatim(w *flacTestBuilder, codes []int64, depth, wasted int) {
	w.put(uint64(2|flacInputBoolBit(wasted != 0)), 8)
	if wasted != 0 {
		for range wasted - 1 {
			w.put(0, 1)
		}
		w.put(1, 1)
	}
	for _, v := range codes {
		w.put(uint64(v>>uint(wasted)), depth-wasted)
	}
}

func flacInputBoolBit(b bool) int {
	if b {
		return 1
	}
	return 0
}

// These cases independently construct features an encoder is free not to
// select, especially full 33-bit stereo sides and extreme legal residuals.
func flacInputHandCases() []flacInputHandCase {
	var cases []flacInputHandCase
	for _, depth := range []int{8, 16, 24, 32} {
		for ch := 1; ch <= 8; ch++ {
			codes := make([]int64, 17*ch)
			scale := int64(1) << (depth - 1)
			for i := range codes {
				codes[i] = []int64{-scale, scale - 1, -1, 0, 1, int64(ch)}[i%6]
			}
			cases = append(cases, flacInputHandCase{
				fmt.Sprintf("verbatim%d_%dch", depth, ch), depth, ch, 17, ch - 1, codes,
				func(w *flacTestBuilder) {
					for c := 0; c < ch; c++ {
						x := make([]int64, 17)
						for f := range x {
							x[f] = codes[f*ch+c]
						}
						flacInputVerbatim(w, x, depth, 0)
					}
				},
			})
		}
	}
	for _, assign := range []int{8, 9, 10} {
		codes := make([]int64, 32)
		for f := 0; f < 16; f++ {
			pairs := [][2]int64{{math.MinInt32, math.MaxInt32}, {math.MaxInt32, math.MinInt32}, {-3, 2}, {2, -3}, {-3, -2}, {-2, -3}, {0, 0}, {math.MinInt32, math.MinInt32}}
			codes[f*2], codes[f*2+1] = pairs[f%len(pairs)][0], pairs[f%len(pairs)][1]
		}
		cases = append(cases, flacInputHandCase{
			fmt.Sprintf("side33_assignment%d", assign), 32, 2, 16, assign, codes,
			func(w *flacTestBuilder) {
				for c := 0; c < 2; c++ {
					x := make([]int64, 16)
					depth := 32
					for f := range x {
						l, r := codes[f*2], codes[f*2+1]
						switch {
						case (assign == 8 || assign == 10) && c == 1, assign == 9 && c == 0:
							x[f], depth = l-r, 33
						case assign == 10:
							x[f] = (l + r) >> 1
						default:
							x[f] = codes[f*2+c]
						}
					}
					flacInputVerbatim(w, x, depth, 0)
				}
			},
		})
	}
	for _, wasted := range []int{1, 16, 31, 32} {
		// A 32-wasted-bit side has only one effective sign bit. Its only
		// valid value is zero: -2^32 exceeds the possible L-R range.
		codes := make([]int64, 32)
		for f := 0; f < 16; f++ {
			if wasted < 32 && f%2 == 0 {
				codes[f*2], codes[f*2+1] = -(int64(1) << (wasted - 1)), int64(1)<<(wasted-1)
			}
		}
		cases = append(cases, flacInputHandCase{
			fmt.Sprintf("side33_wasted%d", wasted), 32, 2, 16, 8, codes,
			func(w *flacTestBuilder) {
				l, side := make([]int64, 16), make([]int64, 16)
				for f := range l {
					l[f], side[f] = codes[f*2], codes[f*2]-codes[f*2+1]
				}
				flacInputVerbatim(w, l, 32, 0)
				flacInputVerbatim(w, side, 33, wasted)
			},
		})
	}
	for order := 1; order <= 4; order++ {
		codes := make([]int64, 128)
		for f := 0; f < 64; f++ {
			codes[f*2], codes[f*2+1] = math.MaxInt32, math.MinInt32
		}
		cases = append(cases, flacInputHandCase{
			fmt.Sprintf("side33_fixed%d", order), 32, 2, 64, 8, codes,
			func(w *flacTestBuilder) {
				w.put(0, 8)
				w.put(math.MaxInt32, 32)
				w.put(uint64((8+order)<<1), 8)
				for range order {
					w.put(0xffffffff, 33)
				}
				w.put(0, 2) // Rice, one partition, k=0, all zero residuals.
				w.put(0, 4)
				w.put(0, 4)
				for range 64 - order {
					w.put(1, 1)
				}
			},
		})
	}
	for _, order := range []int{2, 12, 32} {
		codes := make([]int64, 128)
		for f := 0; f < 64; f++ {
			codes[f*2], codes[f*2+1] = math.MinInt32, math.MaxInt32
		}
		cases = append(cases, flacInputHandCase{
			fmt.Sprintf("side33_LPC%d_shift15", order), 32, 2, 64, 9, codes,
			func(w *flacTestBuilder) {
				w.put(uint64((31+order)<<1), 8)
				for range order {
					w.put(uint64(0x100000001), 33) // -4294967295
				}
				w.put(14, 4) // precision15, maximum legal shift15
				w.put(15, 5)
				// Coefficients sum to 32768 for LPC12/32; LPC2 has sum
				// 32766 with a nonzero residual. LPC32 accumulates large
				// positive and negative products before cancellation.
				for j := 0; j < order; j++ {
					v := int64(1024)
					if order == 2 {
						v = 16383
					} else if order == 12 {
						v = 2730
						if j == 0 {
							v += 8
						}
					} else if order == 32 {
						v = 16383
						if j >= 16 {
							v = -14335
						}
					}
					w.put(uint64(v), 15)
				}
				residual := int64(0)
				if order == 2 {
					residual = -262143
				}
				testFLACPutRice(w, repeatFLACValue(residual, 64-order), 1, 0, []int{30})
				w.put(0, 8)
				w.put(math.MaxInt32, 32)
			},
		})
	}
	codes := make([]int64, 128)
	for f := 0; f < 64; f++ {
		codes[f*2], codes[f*2+1] = math.MinInt32, math.MaxInt32
	}
	cases = append(cases, flacInputHandCase{
		"side33_LPC1_shift0", 32, 2, 64, 9, codes,
		func(w *flacTestBuilder) {
			w.put(64, 8)
			w.put(0x100000001, 33)
			w.put(1, 4) // precision2, coefficient1
			w.put(0, 5)
			w.put(1, 2)
			testFLACPutRice(w, make([]int64, 63), 0, 0, []int{0})
			w.put(0, 8)
			w.put(math.MaxInt32, 32)
		},
	})
	for _, escape := range []bool{false, true} {
		codes := repeatFLACValue(0, 16)
		for i := range codes {
			if escape {
				codes[i] = []int64{-(1 << 30), (1 << 30) - 1, 0, -1}[i%4]
			} else {
				codes[i] = []int64{-math.MaxInt32, math.MaxInt32, 0, -1}[i%4]
			}
		}
		cases = append(cases, flacInputHandCase{
			fmt.Sprintf("residual32_escape%t", escape), 32, 1, 16, 0, codes,
			func(w *flacTestBuilder) {
				w.put(16, 8) // fixed0, not verbatim
				parameter := 30
				if escape {
					parameter = 31
				}
				testFLACPutRice(w, codes, 1, 0, []int{parameter})
			},
		})
	}
	return cases
}

func repeatFLACValue(v int64, n int) []int64 {
	x := make([]int64, n)
	for i := range x {
		x[i] = v
	}
	return x
}

func TestReadFLACInputHandVectors(t *testing.T) {
	var manifest []map[string]any
	for _, tc := range flacInputHandCases() {
		t.Run(tc.name, func(t *testing.T) {
			data := flacCustomStream(t, tc.depth, tc.channels, tc.n, tc.assign, tc.codes, tc.emit)
			checkCustomFLAC(t, data, tc.depth, tc.codes)
			// Explicit frame depth codes must decode exactly like inherited
			// STREAMINFO depth. Repair both checksums after header mutation.
			code := map[int]byte{8: 1, 16: 4, 24: 6, 32: 7}[tc.depth]
			explicit := bytes.Clone(data)
			explicit[45] |= code << 1
			explicit[49] = byte(flacTestCRC(explicit[42:49], 8))
			binary.BigEndian.PutUint16(explicit[len(explicit)-2:], uint16(flacTestCRC(explicit[42:len(explicit)-2], 16)))
			checkCustomFLAC(t, explicit, tc.depth, tc.codes)
			raw := make([]byte, len(tc.codes)*(tc.depth/8))
			for i, v := range tc.codes {
				for j := 0; j < tc.depth/8; j++ {
					raw[i*(tc.depth/8)+j] = byte(v >> uint(j*8))
				}
			}
			files := map[string][]byte{".flac": data, ".raw": raw, "_explicit.flac": explicit}
			if dir := os.Getenv("GOMATH_FLAC_HAND_DUMP"); dir != "" {
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatal(err)
				}
				for suffix, b := range files {
					if err := os.WriteFile(filepath.Join(dir, tc.name+suffix), b, 0644); err != nil {
						t.Fatal(err)
					}
				}
				manifest = append(manifest, map[string]any{"File": tc.name + ".flac", "Depth": tc.depth, "Channels": tc.channels, "Frames": tc.n})
			} else {
				for suffix, want := range files {
					b, err := os.ReadFile(filepath.Join(flacInputDirectory(), "hand", tc.name+suffix))
					if err != nil || !bytes.Equal(b, want) {
						t.Fatal("independently decoded hand oracle changed", tc.name+suffix, err)
					}
				}
			}
		})
	}
	if dir := os.Getenv("GOMATH_FLAC_HAND_DUMP"); dir != "" {
		b, err := json.MarshalIndent(manifest, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "manifest.json"), b, 0644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestReadFLACInputBudgetsAndConcurrency(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(flacInputDirectory(), "pcm32_8ch_random_n255_b256.flac"))
	if err != nil {
		t.Fatal(err)
	}
	options := FLACReadOptions{DiscardMetadata: true, Limits: Limits{MaxBytes: int64(len(data)), MaxFrames: 255, MaxSamples: 255 * 8}}
	p, err := ReadFLAC(oneByteReader{Reader: bytes.NewReader(data)}, options)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"bytes", "frames", "samples", "work"} {
		t.Run(field, func(t *testing.T) {
			o := options
			switch field {
			case "bytes":
				o.Limits.MaxBytes--
			case "frames":
				o.Limits.MaxFrames--
			case "samples":
				o.Limits.MaxSamples--
			case "work":
				o.MaxWork = 1
			}
			if p, err := ReadFLAC(bytes.NewReader(data), o); !errors.Is(err, ErrLimit) || p != nil {
				t.Fatal("limit did not reject nil", p, err)
			}
		})
	}
	// Work accounting is independent of buffered I/O chunking. Find the exact
	// boundary and require both sides; this exercises the encoded-bit/CRC
	// path with no hidden uncharged rescue.
	lo, hi := uint64(1), uint64(1<<24)
	for lo < hi {
		mid := lo + (hi-lo)/2
		o := options
		o.MaxWork = mid
		if _, err := ReadFLAC(bytes.NewReader(data), o); errors.Is(err, ErrLimit) {
			lo = mid + 1
		} else if err != nil {
			t.Fatal(err)
		} else {
			hi = mid
		}
	}
	options.MaxWork = lo
	if _, err := ReadFLAC(oneByteReader{Reader: bytes.NewReader(data)}, options); err != nil {
		t.Fatal("exact work budget", err)
	}
	options.MaxWork--
	if p, err := ReadFLAC(bytes.NewReader(data), options); !errors.Is(err, ErrLimit) || p != nil {
		t.Fatal("one below work budget", p, err)
	}
	options.MaxWork = 0
	unknown := bytes.Clone(data)
	packed := binary.BigEndian.Uint64(unknown[18:26])
	binary.BigEndian.PutUint64(unknown[18:26], packed & ^uint64((1<<36)-1))
	if _, err := ReadFLAC(bytes.NewReader(unknown), options); err != nil {
		t.Fatal("unknown total", err)
	}
	options.Limits.MaxFrames--
	if p, err := ReadFLAC(bytes.NewReader(unknown), options); !errors.Is(err, ErrLimit) || p != nil {
		t.Fatal("unknown total bypassed budget", p, err)
	}
	// On 64-bit hosts the old MaxFrames*8 expression wraps to zero at this
	// exact limit; a valid small file must still decode without a slice panic.
	options.Limits = Limits{MaxFrames: int(^uint(0)>>1)/4 + 1, MaxSamples: int(^uint(0) >> 1)}
	if _, err := ReadFLAC(bytes.NewReader(data), options); err != nil {
		t.Fatal("large capacity budget", err)
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := ReadFLAC(bytes.NewReader(data), options)
			if err != nil {
				t.Error(err)
				return
			}
			if len(got.samples) != len(p.samples) {
				t.Error("concurrent sample count changed")
				return
			}
			for i, v := range p.samples {
				if got.samples[i] != v {
					t.Errorf("concurrent sample %d changed", i)
					return
				}
			}
			got.samples[0] = 55
		}()
	}
	wg.Wait()
	if p.samples[0] == 55 {
		t.Fatal("decoder storage aliased")
	}
}

func TestReadFLACInputPredictorWorkBudget(t *testing.T) {
	for _, tc := range flacInputHandCases() {
		if tc.name != "side33_LPC32_shift15" {
			continue
		}
		data := flacCustomStream(t, tc.depth, tc.channels, tc.n, tc.assign, tc.codes, tc.emit)
		// All encoded bits + all reconstructed scalar samples + precisely
		// (block-order)*order multiply/add pairs in the side LPC subframe.
		work := uint64(len(data)*8 + tc.n*tc.channels + (tc.n-32)*32)
		if _, err := ReadFLAC(bytes.NewReader(data), FLACReadOptions{MaxWork: work}); err != nil {
			t.Fatal("exact LPC work budget", err)
		}
		if p, err := ReadFLAC(bytes.NewReader(data), FLACReadOptions{MaxWork: work - 1}); !errors.Is(err, ErrLimit) || p != nil {
			t.Fatal("one below LPC work budget", p, err)
		}
		return
	}
	t.Fatal("missing LPC32 work oracle")
}

func TestReadFLACInputInvalid(t *testing.T) {
	base := flacCustomStream(t, 32, 8, 16, 7, make([]int64, 128), func(w *flacTestBuilder) {
		for range 8 {
			w.put(0, 8)
			w.put(0, 32)
		}
	})
	for end := 0; end < len(base); end++ {
		if p, err := ReadFLAC(bytes.NewReader(base[:end]), FLACReadOptions{}); err == nil || p != nil {
			t.Fatalf("truncation at %d returned PCM", end)
		}
	}
	for _, field := range []string{"MD5", "CRC8", "CRC16", "depth12", "depth20", "depth31", "assignment8", "assignment9", "assignment10", "assignment11", "assignment15", "trailing", "concatenated"} {
		t.Run(field, func(t *testing.T) {
			b := bytes.Clone(base)
			switch field {
			case "MD5":
				b[26] ^= 1
			case "CRC8":
				b[49] ^= 1
			case "CRC16":
				b[len(b)-1] ^= 1
			case "depth12", "depth20", "depth31":
				depth := map[string]int{"depth12": 12, "depth20": 20, "depth31": 31}[field]
				v := binary.BigEndian.Uint64(b[18:26])
				v = v & ^uint64(31<<36) | uint64(depth-1)<<36
				binary.BigEndian.PutUint64(b[18:26], v)
			case "assignment8", "assignment9", "assignment10", "assignment11", "assignment15":
				assign := map[string]byte{"assignment8": 8, "assignment9": 9, "assignment10": 10, "assignment11": 11, "assignment15": 15}[field]
				b[45] = assign << 4
				b[49] = byte(flacTestCRC(b[42:49], 8))
				binary.BigEndian.PutUint16(b[len(b)-2:], uint16(flacTestCRC(b[42:len(b)-2], 16)))
			case "trailing":
				b = append(b, 0)
			case "concatenated":
				b = append(b, base...)
			}
			if p, err := ReadFLAC(bytes.NewReader(b), FLACReadOptions{}); err == nil || p != nil {
				t.Fatal("invalid input returned PCM", p, err)
			}
		})
	}
	for _, tc := range []struct {
		name       string
		ch, assign int
		emit       func(*flacTestBuilder)
	}{
		{"side_positive_restoration", 2, 8, func(w *flacTestBuilder) {
			w.put(0, 8)
			w.put(math.MaxInt32, 32)
			w.put(0, 8)
			w.put(0x100000001, 33)
		}},
		{"side_min33_restoration", 2, 8, func(w *flacTestBuilder) {
			w.put(0, 8)
			w.put(0, 32)
			w.put(0, 8)
			w.put(0x100000000, 33)
		}},
		{"predictor_overflow", 1, 0, func(w *flacTestBuilder) {
			w.put(64, 8)
			w.put(math.MaxInt32, 32)
			w.put(14, 4)
			w.put(0, 5)
			w.put(16383, 15)
			testFLACPutRice(w, make([]int64, 15), 0, 0, []int{0})
		}},
		{"side_predictor_overflow", 2, 8, func(w *flacTestBuilder) {
			w.put(0, 8)
			w.put(0, 32)
			w.put(64, 8)
			w.put(0xffffffff, 33)
			w.put(14, 4)
			w.put(0, 5)
			w.put(16383, 15)
			testFLACPutRice(w, make([]int64, 15), 0, 0, []int{0})
		}},
		{"residual_min32", 1, 0, func(w *flacTestBuilder) {
			w.put(16, 8)
			w.put(1, 2)
			w.put(0, 4)
			w.put(30, 5)
			for range 3 {
				w.put(0, 1)
			}
			w.put(1, 1)
			w.put(0x3fffffff, 30)
		}},
		{"wasted33", 2, 8, func(w *flacTestBuilder) {
			w.put(0, 8)
			w.put(0, 32)
			w.put(1, 8)
			for range 32 {
				w.put(0, 1)
			}
			w.put(1, 1)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := flacCustomStream(t, 32, tc.ch, 16, tc.assign, make([]int64, 16*tc.ch), tc.emit)
			if p, err := ReadFLAC(bytes.NewReader(data), FLACReadOptions{}); !errors.Is(err, ErrInvalidInput) || p != nil {
				t.Fatal("numeric invalidity returned PCM", p, err)
			}
		})
	}
	// Full signed15-bit coefficients, all 32 terms at a signed33-bit
	// warm-up extreme: the sum approaches the proven 2^51 accumulator
	// bound. It must fail the sample-width check before becoming history.
	wide := flacCustomStream(t, 32, 2, 64, 9, make([]int64, 128), func(w *flacTestBuilder) {
		w.put(126, 8) // LPC32
		for range 32 {
			w.put(0x100000000, 33)
		}
		w.put(14, 4)
		w.put(0, 5)
		for range 32 {
			w.put(0x4000, 15) // -16384
		}
		testFLACPutRice(w, make([]int64, 32), 0, 0, []int{0})
		w.put(0, 8)
		w.put(0, 32)
	})
	if p, err := ReadFLAC(bytes.NewReader(wide), FLACReadOptions{}); !errors.Is(err, ErrInvalidInput) || p != nil {
		t.Fatal("LPC32 numeric bound", p, err)
	}
	for _, ch := range []int{1, 3, 4, 5, 6, 7, 8} {
		for assign := 8; assign <= 10; assign++ {
			data := flacCustomStream(t, 32, ch, 16, assign, make([]int64, 16*ch), func(w *flacTestBuilder) {})
			if p, err := ReadFLAC(bytes.NewReader(data), FLACReadOptions{}); !errors.Is(err, ErrUnsupported) || p != nil {
				t.Fatalf("stereo assignment%d/channels%d: %v %v", assign, ch, p, err)
			}
		}
	}
	// I/O errors at a valid complete frame boundary must not be hidden by a
	// known total or matching MD5. The entire source must reach EOF.
	r := io.MultiReader(bytes.NewReader(base), &flacErrorReader{})
	if p, err := ReadFLAC(r, FLACReadOptions{}); !errors.Is(err, io.ErrClosedPipe) || p != nil {
		t.Fatal("complete stream boundary lost I/O error", p, err)
	}
}

func FuzzReadFLACInput(f *testing.F) {
	for _, file := range []string{"pcm8_8ch_extremes_n17_b16.flac", "pcm32_8ch_random_n255_b256.flac", "pcm32_2ch_wasted_n257_b256.flac"} {
		b, err := os.ReadFile(filepath.Join(flacInputDirectory(), file))
		if err != nil {
			f.Fatal(err)
		}
		f.Add(b)
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 32768 {
			t.Skip()
		}
		p, err := ReadFLAC(bytes.NewReader(b), FLACReadOptions{DiscardMetadata: true, Limits: Limits{MaxBytes: 32768, MaxFrames: 4096, MaxSamples: 32768, MaxChunks: 16}, MaxWork: 1 << 20})
		if err != nil {
			if p != nil {
				t.Fatal("partial PCM")
			}
			return
		}
		if p.Frames() > 4096 || len(p.samples) > 32768 || p.Format().Channels > 8 {
			t.Fatal("format/storage bounds escaped")
		}
		for _, v := range p.samples {
			if !finite(v) || v < -1 || v >= 1 {
				t.Fatal("invalid integer normalization")
			}
		}
	})
}

func BenchmarkReadFLACInput(b *testing.B) {
	for _, file := range []string{"pcm8_8ch_sine_n1025_b256.flac", "pcm16_8ch_sine_n1025_b256.flac", "pcm24_8ch_sine_n1025_b256.flac", "pcm32_8ch_sine_n1025_b256.flac", "pcm32_2ch_wasted_n65535_b65535.flac"} {
		data, err := os.ReadFile(filepath.Join(flacInputDirectory(), file))
		if err != nil {
			b.Fatal(err)
		}
		b.Run(file, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(data)))
			for i := 0; i < b.N; i++ {
				if _, err := ReadFLAC(bytes.NewReader(data), FLACReadOptions{DiscardMetadata: true}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
