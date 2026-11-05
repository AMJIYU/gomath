package audio

import (
	"bytes"
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand"
	"sync"
	"testing"
)

// This small test-only reader covers the emitted subset, independently of the
// writer's bit, predictor, checksum and quantization helpers. It is not a public
// FLAC decoder. FFmpeg full decoding is a separate development validation.
type flacTestReader struct {
	b []byte
	i int
}

func (r *flacTestReader) read(t testing.TB, n int) uint64 {
	t.Helper()
	if n < 0 || n > 64 || r.i+n > len(r.b)*8 {
		t.Fatal("invalid/truncated test bitstream", r.i, n)
	}
	var v uint64
	for range n {
		v = v<<1 | uint64((r.b[r.i/8]>>uint(7-r.i%8))&1)
		r.i++
	}
	return v
}

func (r *flacTestReader) signed(t testing.TB, n int) int64 {
	if n == 0 {
		return 0
	}
	v := r.read(t, n)
	return int64(v<<uint(64-n)) >> uint(64-n)
}

// Polynomial division, rather than the production shift-register recurrence.
func flacTestCRC(b []byte, width uint) uint64 {
	poly := uint64(0x107)
	if width == 16 {
		poly = 0x18005
	}
	var rem uint64
	for _, v := range b {
		rem = rem<<8 | uint64(v)
		for bit := int(width) + 7; bit >= int(width); bit-- {
			if rem&(1<<uint(bit)) != 0 {
				rem ^= poly << uint(bit-int(width))
			}
		}
	}
	for range width {
		rem <<= 1
		if rem&(1<<width) != 0 {
			rem ^= poly
		}
	}
	return rem
}

type flacTestInfo struct {
	Rate, Channels, Depth, Block int
	Lengths, Kinds, Rice         []int
	Samples                      []int64
}

func inspectTestFLAC(t testing.TB, encoded []byte) flacTestInfo {
	t.Helper()
	if len(encoded) < 42 || string(encoded[:4]) != "fLaC" || !bytes.Equal(encoded[4:8], []byte{0x80, 0, 0, 34}) {
		t.Fatal("invalid native FLAC/STREAMINFO")
	}
	packed := binary.BigEndian.Uint64(encoded[18:26])
	info := flacTestInfo{
		Rate: int(packed >> 44), Channels: int((packed>>41)&7) + 1,
		Depth: int((packed>>36)&31) + 1, Block: int(binary.BigEndian.Uint16(encoded[8:10])),
	}
	if info.Block < 16 || info.Block > 65535 || int(binary.BigEndian.Uint16(encoded[10:12])) != info.Block {
		t.Fatal("invalid fixed block STREAMINFO")
	}
	var raw bytes.Buffer
	minSize, maxSize := len(encoded), 0
	for pos, number := 42, 0; pos < len(encoded); number++ {
		start := pos
		if pos+4 >= len(encoded) || encoded[pos] != 255 || encoded[pos+1] != 248 || encoded[pos+2] != 112 {
			t.Fatal("unexpected frame header", pos)
		}
		depthCode := 4
		if info.Depth == 24 {
			depthCode = 6
		}
		if int(encoded[pos+3]) != (info.Channels-1)*16+depthCode*2 {
			t.Fatal("channel/depth header mismatch")
		}
		pos += 4
		first := encoded[pos]
		pos++
		num, extra := uint64(first), 0
		if first >= 128 {
			mask := byte(128)
			for first&mask != 0 {
				extra++
				mask >>= 1
			}
			num = uint64(first & (mask - 1))
			extra--
		}
		for range extra {
			if pos >= len(encoded) || encoded[pos]&192 != 128 {
				t.Fatal("invalid frame number continuation")
			}
			num = num<<6 | uint64(encoded[pos]&63)
			pos++
		}
		if num != uint64(number) || pos+3 > len(encoded) {
			t.Fatal("incorrect frame number", num, number)
		}
		n := int(binary.BigEndian.Uint16(encoded[pos:pos+2])) + 1
		pos += 2
		if flacTestCRC(encoded[start:pos], 8) != uint64(encoded[pos]) {
			t.Fatal("header CRC mismatch")
		}
		pos++
		r := flacTestReader{b: encoded, i: pos * 8}
		decoded := make([][]int64, info.Channels)
		for c := range decoded {
			if r.read(t, 1) != 0 {
				t.Fatal("nonzero subframe padding")
			}
			kind := int(r.read(t, 6))
			info.Kinds = append(info.Kinds, kind)
			if r.read(t, 1) != 0 {
				t.Fatal("unexpected wasted bits")
			}
			x := make([]int64, n)
			switch {
			case kind == 0:
				v := r.signed(t, info.Depth)
				for i := range x {
					x[i] = v
				}
			case kind == 1:
				for i := range x {
					x[i] = r.signed(t, info.Depth)
				}
			case kind >= 8 && kind <= 12:
				order := kind - 8
				if order >= n {
					t.Fatal("predictor order exceeds residual sample count")
				}
				for i := 0; i < order; i++ {
					x[i] = r.signed(t, info.Depth)
				}
				if r.read(t, 2) != 0 || r.read(t, 4) != 0 {
					t.Fatal("unexpected residual method/partition")
				}
				k := int(r.read(t, 4))
				info.Rice = append(info.Rice, k)
				width := 0
				if k == 15 {
					width = int(r.read(t, 5))
				}
				coeff := [][]int64{{}, {1}, {2, -1}, {3, -3, 1}, {4, -6, 4, -1}}[order]
				for i := order; i < n; i++ {
					var residual int64
					if k == 15 {
						residual = r.signed(t, width)
					} else {
						var unary uint64
						for r.read(t, 1) == 0 {
							unary++
							if unary > uint64(len(encoded))*8 {
								t.Fatal("unbounded unary code")
							}
						}
						u := unary<<uint(k) | r.read(t, k)
						residual = int64(u / 2)
						if u%2 != 0 {
							residual = -residual - 1
						}
					}
					x[i] = residual
					for j, v := range coeff {
						x[i] += v * x[i-j-1]
					}
				}
			default:
				t.Fatal("unsupported emitted subframe", kind)
			}
			decoded[c] = x
		}
		for r.i%8 != 0 {
			if r.read(t, 1) != 0 {
				t.Fatal("nonzero frame alignment")
			}
		}
		pos = r.i / 8
		if pos+2 > len(encoded) || flacTestCRC(encoded[start:pos], 16) != uint64(binary.BigEndian.Uint16(encoded[pos:pos+2])) {
			t.Fatal("frame CRC mismatch")
		}
		pos += 2
		if n > info.Block || (pos < len(encoded) && n != info.Block) {
			t.Fatal("invalid final/full frame length", n, info.Block)
		}
		minSize, maxSize = min(minSize, pos-start), max(maxSize, pos-start)
		info.Lengths = append(info.Lengths, n)
		for i := 0; i < n; i++ {
			for c := range decoded {
				v := decoded[c][i]
				if v < -(1<<(info.Depth-1)) || v >= 1<<(info.Depth-1) {
					t.Fatal("decoded sample overflow")
				}
				info.Samples = append(info.Samples, v)
				for b := 0; b < info.Depth/8; b++ {
					raw.WriteByte(byte(v >> uint(b*8)))
				}
			}
		}
	}
	read24 := func(b []byte) int { return int(b[0])<<16 | int(b[1])<<8 | int(b[2]) }
	if read24(encoded[12:15]) != minSize || read24(encoded[15:18]) != maxSize {
		t.Fatal("frame size extrema do not match STREAMINFO", minSize, maxSize)
	}
	if packed&((1<<36)-1) != uint64(len(info.Samples)/info.Channels) {
		t.Fatal("total interchannel samples mismatch")
	}
	sum := md5.Sum(raw.Bytes())
	if !bytes.Equal(sum[:], encoded[26:42]) {
		t.Fatal("unencoded PCM MD5 mismatch")
	}
	return info
}

func flacIntegerPCM(t testing.TB, depth, channels, n, rate int, pattern string) (*PCM, []int64) {
	t.Helper()
	scale := int64(1) << (depth - 1)
	rng := rand.New(rand.NewSource(int64(n*31 + depth*7 + channels)))
	codes, samples := make([]int64, n*channels), make([]float64, n*channels)
	for f := 0; f < n; f++ {
		for c := 0; c < channels; c++ {
			var v int64
			x := int64(f % 25)
			switch pattern {
			case "silence":
			case "constant":
				v = scale/3 + int64(c)
			case "extremes":
				v = []int64{-scale, scale - 1, 0, -1, 1}[(f+c)%5]
			case "ramp":
				v = int64(f%1000)*int64(c+1) - 500
			case "polynomial":
				v = x*x*x*x/2 - 100000 + int64(c)
				if depth == 16 {
					v /= 8
				}
			case "sine":
				v = int64(math.Round(float64(scale-1) * 0.8 * math.Sin(2*math.Pi*float64(991+c*107)*float64(f)/float64(rate))))
			default:
				v = rng.Int63n(scale*2) - scale
			}
			codes[f*channels+c] = v
			samples[f*channels+c] = float64(v) / float64(scale)
		}
	}
	return mustPCM(t, Format{SampleRate: rate, Channels: channels}, samples), codes
}

func equalFLACCodes(t testing.TB, got, want []int64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatal("sample count", len(got), len(want))
	}
	for i, v := range want {
		if got[i] != v {
			t.Fatalf("sample %d: got %d want %d", i, got[i], v)
		}
	}
}

func TestFLACRoundTripAndIntegrity(t *testing.T) {
	lengths := []int{1, 2, 3, 4, 5, 15, 16, 17, 31, 32, 33, 255, 256, 257, 4095, 4096, 4097, 8192, 8193}
	patterns := []string{"silence", "constant", "extremes", "ramp", "polynomial", "sine", "random"}
	for _, depth := range []int{16, 24} {
		for _, channels := range []int{1, 2} {
			for i, n := range lengths {
				t.Run(fmt.Sprintf("%d/%d/%d", depth, channels, n), func(t *testing.T) {
					p, codes := flacIntegerPCM(t, depth, channels, n, 48000, patterns[i%len(patterns)])
					e := PCM16
					if depth == 24 {
						e = PCM24
					}
					var b bytes.Buffer
					if err := WriteFLAC(&b, p, FLACWriteOptions{Encoding: e, BlockSize: 256}); err != nil {
						t.Fatal(err)
					}
					info := inspectTestFLAC(t, b.Bytes())
					if info.Rate != 48000 || info.Depth != depth || info.Channels != channels || info.Block != 256 {
						t.Fatal("incorrect format", info)
					}
					equalFLACCodes(t, info.Samples, codes)
					if len(info.Lengths) != (n+255)/256 || info.Lengths[len(info.Lengths)-1] != (n-1)%256+1 {
						t.Fatal("missing samples in final frame")
					}
				})
			}
		}
	}
}

func TestFLACBlocksRatesAndLargeFrameNumbers(t *testing.T) {
	for _, block := range []int{16, 17, 257, 4096, 65535} {
		for _, n := range []int{block - 1, block, block + 1} {
			p, codes := flacIntegerPCM(t, 24, 2, n, 768000, "extremes")
			var b bytes.Buffer
			if err := WriteFLAC(&b, p, FLACWriteOptions{Encoding: PCM24, BlockSize: block}); err != nil {
				t.Fatal(err)
			}
			equalFLACCodes(t, inspectTestFLAC(t, b.Bytes()).Samples, codes)
		}
	}
	// Cross both one- and two-byte frame-number boundaries (128 and 2048).
	p, codes := flacIntegerPCM(t, 16, 1, 16*2050+1, 1, "ramp")
	var b bytes.Buffer
	if err := WriteFLAC(&b, p, FLACWriteOptions{BlockSize: 16}); err != nil {
		t.Fatal(err)
	}
	equalFLACCodes(t, inspectTestFLAC(t, b.Bytes()).Samples, codes)
}

func TestFLACExtendedFrameNumberCodes(t *testing.T) {
	for _, tc := range []struct {
		n   uint32
		hex string
	}{
		{0, "00"}, {127, "7f"}, {128, "c280"}, {2047, "dfbf"},
		{2048, "e0a080"}, {65535, "efbfbf"}, {65536, "f0908080"},
		{0x1fffff, "f7bfbfbf"}, {0x200000, "f888808080"},
		{0x3ffffff, "fbbfbfbfbf"}, {0x4000000, "fc8480808080"}, {0x7fffffff, "fdbfbfbfbfbf"},
	} {
		got := flacAppendNumber(nil, tc.n)
		if hex.EncodeToString(got) != tc.hex {
			t.Fatalf("frame number %x: %x want %s", tc.n, got, tc.hex)
		}
	}
}

type flacFailWriter struct {
	calls int
	short bool
}

func (w *flacFailWriter) Write(b []byte) (int, error) {
	w.calls++
	if w.calls == 2 {
		if w.short {
			return len(b) - 1, nil
		}
		return 0, io.ErrClosedPipe
	}
	return len(b), nil
}

func TestFLACFrameWriteFailure(t *testing.T) {
	p, _ := flacIntegerPCM(t, 16, 1, 4097, 48000, "sine")
	for _, short := range []bool{true, false} {
		w := &flacFailWriter{short: short}
		want := io.ErrClosedPipe
		if short {
			want = io.ErrShortWrite
		}
		if err := WriteFLAC(w, p, FLACWriteOptions{}); !errors.Is(err, want) || w.calls != 2 {
			t.Fatal("frame I/O error must stop output", err, w.calls)
		}
	}
}

// Development verification can select this test with GODEBUG=fips140=only.
// No child processes or global cryptography setting changes occur in tests.
func TestFLACMD5Availability(t *testing.T) {
	if _, err := md5.New().Write([]byte{0}); err == nil {
		t.Skip("select with GODEBUG=fips140=only to verify unavailable MD5")
	}
	p := mustPCM(t, Format{48000, 1, 0}, []float64{0})
	var b bytes.Buffer
	if err := WriteFLAC(&b, p, FLACWriteOptions{}); !errors.Is(err, ErrUnsupported) || b.Len() != 0 {
		t.Fatal("unavailable MD5 must reject before any output", err, b.Len())
	}
}

func TestFLACQuantization(t *testing.T) {
	for _, depth := range []int{16, 24} {
		scale := float64(int64(1) << (depth - 1))
		x := []float64{-1, 1, -0.0, math.Copysign(0, -1), .5 / scale, -.5 / scale, 1.5 / scale, -1.5 / scale, .5, -.5}
		p := mustPCM(t, Format{SampleRate: 44100, Channels: 2, ChannelMask: 3}, x)
		e := PCM16
		if depth == 24 {
			e = PCM24
		}
		var b bytes.Buffer
		if err := WriteFLAC(&b, p, FLACWriteOptions{Encoding: e}); err != nil {
			t.Fatal(err)
		}
		equalFLACCodes(t, inspectTestFLAC(t, b.Bytes()).Samples, []int64{-int64(scale), int64(scale) - 1, 0, 0, 1, -1, 2, -2, int64(scale) / 2, -int64(scale) / 2})
	}
}

func TestFLACRejectsBeforeWriting(t *testing.T) {
	p, _ := flacIntegerPCM(t, 24, 2, 300, 48000, "random")
	var baseline bytes.Buffer
	if err := WriteFLAC(&baseline, p, FLACWriteOptions{}); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		p    *PCM
		opts FLACWriteOptions
		err  error
	}{
		{"nil", nil, FLACWriteOptions{}, ErrInvalidInput},
		{"zero", &PCM{}, FLACWriteOptions{}, ErrInvalidInput},
		{"empty", mustPCM(t, Format{48000, 1, 0}, nil), FLACWriteOptions{}, ErrInvalidInput},
		{"range", mustPCM(t, Format{48000, 1, 0}, []float64{1.0000001}), FLACWriteOptions{}, ErrInvalidInput},
		{"negativeRange", mustPCM(t, Format{48000, 1, 0}, []float64{-1.0000001}), FLACWriteOptions{}, ErrInvalidInput},
		{"nonfinite", &PCM{Format{48000, 1, 0}, []float64{math.NaN()}}, FLACWriteOptions{}, ErrInvalidInput},
		{"channels", mustPCM(t, Format{48000, 3, 0}, []float64{0, 0, 0}), FLACWriteOptions{}, ErrUnsupported},
		{"monoMask", mustPCM(t, Format{48000, 1, 1}, []float64{0}), FLACWriteOptions{}, ErrUnsupported},
		{"stereoMask", mustPCM(t, Format{48000, 2, 12}, []float64{0, 0}), FLACWriteOptions{}, ErrUnsupported},
		{"pcm32", p, FLACWriteOptions{Encoding: PCM32}, ErrUnsupported},
		{"float32", p, FLACWriteOptions{Encoding: Float32}, ErrUnsupported},
		{"float64", p, FLACWriteOptions{Encoding: Float64}, ErrUnsupported},
		{"encoding", p, FLACWriteOptions{Encoding: 255}, ErrUnsupported},
		{"smallBlock", p, FLACWriteOptions{BlockSize: 15}, ErrInvalidInput},
		{"largeBlock", p, FLACWriteOptions{BlockSize: 65536}, ErrInvalidInput},
		{"negativeBlock", p, FLACWriteOptions{BlockSize: -1}, ErrInvalidInput},
		{"negativeLimit", p, FLACWriteOptions{Limits: Limits{MaxBytes: -1}}, ErrInvalidInput},
		{"frames", p, FLACWriteOptions{Limits: Limits{MaxFrames: 299}}, ErrLimit},
		{"samples", p, FLACWriteOptions{Limits: Limits{MaxSamples: 599}}, ErrLimit},
		{"bytesHeader", p, FLACWriteOptions{Limits: Limits{MaxBytes: 41}}, ErrLimit},
		{"bytes", p, FLACWriteOptions{Limits: Limits{MaxBytes: int64(baseline.Len() - 1)}}, ErrLimit},
		{"work", p, FLACWriteOptions{MaxWork: uint64(len(p.samples))*256 + 127}, ErrLimit},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var b bytes.Buffer
			if err := WriteFLAC(&b, tc.p, tc.opts); !errors.Is(err, tc.err) || b.Len() != 0 {
				t.Fatal("expected preflight rejection", err, b.Len())
			}
		})
	}
	if err := WriteFLAC(nil, p, FLACWriteOptions{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatal(err)
	}
	var exact bytes.Buffer
	if err := WriteFLAC(&exact, p, FLACWriteOptions{Limits: Limits{MaxBytes: int64(baseline.Len()), MaxChunks: 1}, MaxWork: uint64(len(p.samples))*256 + 128}); err != nil || !bytes.Equal(exact.Bytes(), baseline.Bytes()) {
		t.Fatal("exact byte/work budget", err)
	}
	if err := WriteFLAC(shortWriter{}, p, FLACWriteOptions{}); !errors.Is(err, io.ErrShortWrite) {
		t.Fatal(err)
	}
	if err := WriteFLAC(errorWriter{}, p, FLACWriteOptions{}); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal(err)
	}
}

func TestFLACPredictorsAndFallbacks(t *testing.T) {
	// All five fixed orders are searched. Degree order-1 gives zero residuals;
	// a step signal selects order one without invoking the constant shortcut.
	for order := 0; order <= 4; order++ {
		x := flacPredictorCodes(order)
		plan := flacChoose(x, 1, len(x), 24)
		if plan.kind != 8+order {
			t.Fatalf("order %d selected %+v", order, plan)
		}
		p := make([]float64, len(x))
		for i, v := range x {
			p[i] = float64(v) / (1 << 23)
		}
		var b bytes.Buffer
		if err := WriteFLAC(&b, mustPCM(t, Format{48000, 1, 4}, p), FLACWriteOptions{Encoding: PCM24}); err != nil {
			t.Fatal(err)
		}
		equalFLACCodes(t, inspectTestFLAC(t, b.Bytes()).Samples, x)
	}
	p, _ := flacIntegerPCM(t, 24, 2, 4096, 48000, "random")
	var b bytes.Buffer
	if err := WriteFLAC(&b, p, FLACWriteOptions{Encoding: PCM24}); err != nil {
		t.Fatal(err)
	}
	info := inspectTestFLAC(t, b.Bytes())
	if info.Kinds[0] != 1 || info.Kinds[1] != 1 {
		t.Fatal("expected incompressible verbatim fallback", info.Kinds)
	}
	// Signed-width edges: uniform ±1-bit residuals favor an escape partition.
	x := make([]int64, 4096)
	for i := range x {
		x[i] = int64(i%2) - 1
	}
	plan := flacChoose(x, 1, len(x), 24)
	if plan.kind != 8 || plan.rice != 15 || plan.width != 1 {
		t.Fatal("expected adaptive escape", plan)
	}
}

func flacPredictorCodes(order int) []int64 {
	x := make([]int64, 160)
	for i := range x {
		if order == 0 {
			x[i] = int64(i%3) - 1
		} else if order == 1 {
			x[i] = 10000 + int64(i/40)
		} else {
			x[i] = 1
			for range order - 1 {
				x[i] *= int64(i)
			}
		}
	}
	return x
}

func TestFLACConcurrentDeterministic(t *testing.T) {
	p, _ := flacIntegerPCM(t, 24, 2, 4097, 48000, "sine")
	before := p.Samples()
	opts := FLACWriteOptions{Encoding: PCM24}
	var want bytes.Buffer
	if err := WriteFLAC(&want, p, opts); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			var got bytes.Buffer
			if err := WriteFLAC(&got, p, opts); err != nil || !bytes.Equal(got.Bytes(), want.Bytes()) {
				t.Error("nondeterministic concurrent encoding", err)
			}
		})
	}
	wg.Wait()
	closeSamples(t, p.Samples(), before, 0)
}

func FuzzFLACIntegerRoundTrip(f *testing.F) {
	f.Add([]byte{0, 128, 255, 1, 2, 3, 4}, uint16(16), false, false)
	f.Add([]byte{0, 0, 0, 0}, uint16(17), true, true)
	f.Fuzz(func(t *testing.T, data []byte, block uint16, stereo, wide bool) {
		if len(data) == 0 || len(data) > 2048 {
			t.Skip()
		}
		channels, depth, encoding := 1, 16, PCM16
		if stereo {
			channels = 2
		}
		if wide {
			depth, encoding = 24, PCM24
		}
		codes, samples := make([]int64, len(data)*channels), make([]float64, len(data)*channels)
		for i := range samples {
			j := i / channels
			v := int64(data[j])<<uint(depth-8) | int64(data[(j+1)%len(data)])<<uint(depth-16)
			if depth == 24 {
				v |= int64(data[(j+2)%len(data)])
			}
			v -= 1 << (depth - 1)
			if i%channels == 1 {
				v = -v - 1
			}
			codes[i], samples[i] = v, float64(v)/float64(int64(1)<<(depth-1))
		}
		p := mustPCM(t, Format{48000, channels, 0}, samples)
		var b bytes.Buffer
		if err := WriteFLAC(&b, p, FLACWriteOptions{Encoding: encoding, BlockSize: max(16, int(block))}); err != nil {
			t.Fatal(err)
		}
		equalFLACCodes(t, inspectTestFLAC(t, b.Bytes()).Samples, codes)
	})
}
