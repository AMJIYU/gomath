package audio

import (
	"bytes"
	"crypto/md5"
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
	"strings"
	"sync"
	"testing"
)

func TestReadFLACWriterRoundTrip(t *testing.T) {
	for _, depth := range []int{16, 24} {
		for _, ch := range []int{1, 2} {
			for _, n := range []int{1, 15, 16, 17, 257, 4096, 4097, 65536} {
				for _, pattern := range []string{"silence", "constant", "extremes", "ramp", "sine", "random"} {
					p, _ := flacIntegerPCM(t, depth, ch, n, 48000, pattern)
					e := PCM16
					if depth == 24 {
						e = PCM24
					}
					var b bytes.Buffer
					if err := WriteFLAC(&b, p, FLACWriteOptions{Encoding: e}); err != nil {
						t.Fatal(err)
					}
					got, err := ReadFLAC(oneByteReader{Reader: bytes.NewReader(b.Bytes())}, FLACReadOptions{})
					if err != nil {
						t.Fatalf("%d/%d/%d/%s: %v", depth, ch, n, pattern, err)
					}
					if got.Format() != p.Format() {
						t.Fatal("format changed", got.Format())
					}
					closeSamples(t, got.Samples(), p.Samples(), 0)
				}
			}
		}
	}
}

type flacTestBuilder struct {
	b []byte
	n int
}

func (w *flacTestBuilder) put(v uint64, n int) {
	for bit := n - 1; bit >= 0; bit-- {
		if w.n%8 == 0 {
			w.b = append(w.b, 0)
		}
		w.b[w.n/8] |= byte((v>>uint(bit))&1) << uint(7-w.n%8)
		w.n++
	}
}

// Self-contained bit fixtures include features the production writer does not
// emit. Their PCM, CRC and STREAMINFO MD5 are independently constructed.
func flacCustomStream(t testing.TB, depth, ch, n, assignment int, codes []int64, emit func(*flacTestBuilder)) []byte {
	t.Helper()
	w := flacTestBuilder{}
	w.put(0xfff8, 16)
	w.put(7, 4)
	w.put(0, 4)
	w.put(uint64(assignment), 4)
	w.put(0, 4) // Depth inherited, reserved bit zero.
	w.put(0, 8) // Frame number.
	w.put(uint64(n-1), 16)
	w.put(flacTestCRC(w.b, 8), 8)
	emit(&w)
	for w.n%8 != 0 {
		w.put(0, 1)
	}
	w.put(flacTestCRC(w.b, 16), 16)
	stream := make([]byte, 42)
	copy(stream, "fLaC")
	stream[4], stream[7] = 128, 34
	block := max(16, n)
	binary.BigEndian.PutUint16(stream[8:10], uint16(block))
	binary.BigEndian.PutUint16(stream[10:12], uint16(block))
	packed := uint64(48000)<<44 | uint64(ch-1)<<41 | uint64(depth-1)<<36 | uint64(n)
	binary.BigEndian.PutUint64(stream[18:26], packed)
	raw := make([]byte, len(codes)*(depth/8))
	for i, v := range codes {
		for j := 0; j < depth/8; j++ {
			raw[i*(depth/8)+j] = byte(v >> uint(j*8))
		}
	}
	sum := md5.Sum(raw)
	copy(stream[26:], sum[:])
	return append(stream, w.b...)
}

func testFLACPutRice(w *flacTestBuilder, residual []int64, method, partitionOrder int, parameters []int) {
	w.put(uint64(method), 2)
	w.put(uint64(partitionOrder), 4)
	for partition, parameter := range parameters {
		w.put(uint64(parameter), 4+method)
		start, end := partition*len(residual)/len(parameters), (partition+1)*len(residual)/len(parameters)
		if parameter == (1<<(4+method))-1 {
			w.put(31, 5)
			for _, r := range residual[start:end] {
				w.put(uint64(r), 31)
			}
		} else {
			for _, r := range residual[start:end] {
				u := uint64(r * 2)
				if r < 0 {
					u = uint64(-2*r - 1)
				}
				for q := u >> uint(parameter); q > 0; q-- {
					w.put(0, 1)
				}
				w.put(1, 1)
				w.put(u, parameter)
			}
		}
	}
}

func checkCustomFLAC(t testing.TB, data []byte, depth int, codes []int64) {
	t.Helper()
	p, err := ReadFLAC(bytes.NewReader(data), FLACReadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	want := make([]float64, len(codes))
	for i, v := range codes {
		want[i] = float64(v) / float64(int64(1)<<(depth-1))
	}
	closeSamples(t, p.Samples(), want, 0)
}

func TestReadFLACResidualPartitionsAndWastedBits(t *testing.T) {
	codes := []int64{-5, 0, 3, 8, -2, 6, 7, -1, 500, -700, 400, -200, 0, 1, -1, 100}
	for _, method := range []int{0, 1} {
		data := flacCustomStream(t, 24, 1, len(codes), 0, codes, func(w *flacTestBuilder) {
			w.put(16, 8) // fixed order zero
			testFLACPutRice(w, codes, method, 2, []int{2, 3, 10, (1 << (4 + method)) - 1})
		})
		checkCustomFLAC(t, data, 24, codes)
	}
	for _, kind := range []int{0, 1, 8} {
		original := []int64{-32768, -16, 16, 32752}
		if kind == 0 {
			original = []int64{-32768, -32768, -32768, -32768}
		}
		data := flacCustomStream(t, 16, 1, len(original), 0, original, func(w *flacTestBuilder) {
			w.put(uint64(kind<<1|1), 8)
			w.put(1, 4) // unary wasted-1 = 3 => four wasted bits
			if kind == 0 {
				w.put(uint64(original[0]>>4), 12)
			} else if kind == 1 {
				for _, v := range original {
					w.put(uint64(v>>4), 12)
				}
			} else {
				w.put(0, 6)
				w.put(15, 4)
				w.put(12, 5)
				for _, v := range original {
					w.put(uint64(v>>4), 12)
				}
			}
		})
		checkCustomFLAC(t, data, 16, original)
	}
}

func TestReadFLACStereoAndLPC(t *testing.T) {
	for _, assignment := range []int{1, 8, 9, 10} {
		codes := []int64{-32768, 32767, 32767, -32768, -3, -2, 2, 3, 0, 0}
		data := flacCustomStream(t, 16, 2, len(codes)/2, assignment, codes, func(w *flacTestBuilder) {
			for c := 0; c < 2; c++ {
				w.put(2, 8) // verbatim
				width := 16
				if (assignment == 8 || assignment == 10) && c == 1 || assignment == 9 && c == 0 {
					width++
				}
				for i := 0; i < len(codes); i += 2 {
					a, b := codes[i], codes[i+1]
					var pair [2]int64
					switch assignment {
					case 1:
						pair = [2]int64{a, b}
					case 8:
						pair = [2]int64{a, a - b}
					case 9:
						pair = [2]int64{a - b, b}
					case 10:
						pair = [2]int64{(a + b) >> 1, a - b}
					}
					w.put(uint64(pair[c]), width)
				}
			}
		})
		checkCustomFLAC(t, data, 16, codes)
	}
	for _, order := range []int{1, 2, 12, 32} {
		codes := make([]int64, 64)
		for i := range codes {
			codes[i] = int64(i) - 35
		}
		data := flacCustomStream(t, 24, 1, len(codes), 0, codes, func(w *flacTestBuilder) {
			w.put(uint64((31+order)<<1), 8)
			for _, v := range codes[:order] {
				w.put(uint64(v), 24)
			}
			w.put(2, 4) // 3-bit coefficients
			w.put(1, 5) // right shift one
			w.put(2, 3) // predict previous sample
			for i := 1; i < order; i++ {
				w.put(0, 3)
			}
			w.put(0, 6)
			w.put(0, 4)
			for i := order; i < len(codes); i++ {
				w.put(1, 3) // folded residual 2: 001
			}
		})
		checkCustomFLAC(t, data, 24, codes)
	}
}

func TestReadFLACRejectsInvalidSubframesAndNumbers(t *testing.T) {
	cases := []struct {
		name string
		err  error
		emit func(*flacTestBuilder)
	}{
		{"padding", ErrInvalidInput, func(w *flacTestBuilder) { w.put(128, 8); w.put(0, 16) }},
		{"reservedType", ErrUnsupported, func(w *flacTestBuilder) { w.put(4, 8) }},
		{"wasted", ErrInvalidInput, func(w *flacTestBuilder) { w.put(1, 8); w.put(0, 25) }},
		{"partition", ErrInvalidInput, func(w *flacTestBuilder) { w.put(16, 8); w.put(15, 6) }},
		{"residualMethod", ErrUnsupported, func(w *flacTestBuilder) { w.put(16, 8); w.put(2, 2); w.put(0, 4) }},
		{"lpcPrecision", ErrInvalidInput, func(w *flacTestBuilder) { w.put(64, 8); w.put(0, 16); w.put(15, 4); w.put(0, 5) }},
		{"lpcShift", ErrInvalidInput, func(w *flacTestBuilder) { w.put(64, 8); w.put(0, 16); w.put(2, 4); w.put(31, 5) }},
		{"lpcOrder", ErrInvalidInput, func(w *flacTestBuilder) { w.put(126, 8) }},
		{"foldOverflow", ErrInvalidInput, func(w *flacTestBuilder) {
			w.put(16, 8)
			w.put(1, 2)
			w.put(0, 4)
			w.put(30, 5)
			w.put(1, 4)
			w.put((1<<30)-1, 30)
		}},
		{"sampleOverflow", ErrInvalidInput, func(w *flacTestBuilder) {
			w.put(16, 8)
			w.put(0, 6)
			w.put(15, 4)
			w.put(31, 5)
			w.put(uint64(1<<30), 31)
		}},
		{"predictorOverflow", ErrInvalidInput, func(w *flacTestBuilder) {
			w.put(64, 8)
			w.put(32767, 16)
			w.put(14, 4)
			w.put(0, 5)
			w.put(16383, 15)
			w.put(0, 6)
			w.put(15, 4)
			w.put(0, 5)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data := flacCustomStream(t, 16, 1, 16, 0, make([]int64, 16), tc.emit)
			if p, err := ReadFLAC(bytes.NewReader(data), FLACReadOptions{}); !errors.Is(err, tc.err) || p != nil {
				t.Fatal("invalid subframe accepted/misclassified", err)
			}
		})
	}
	malicious := flacCustomStream(t, 16, 1, 1, 0, []int64{0}, func(w *flacTestBuilder) {
		w.put(16, 8)
		w.put(0, 10)
		for range 1000 {
			w.put(0, 8)
		}
	})
	if _, err := ReadFLAC(bytes.NewReader(malicious), FLACReadOptions{MaxWork: 512}); !errors.Is(err, ErrLimit) {
		t.Fatal("unary work budget not enforced", err)
	}
}

func TestReadFLACVariableBlocks(t *testing.T) {
	codes := make([]int64, 24)
	for i := range codes {
		codes[i] = int64(i) - 12
	}
	emit := func(values []int64) func(*flacTestBuilder) {
		return func(w *flacTestBuilder) {
			w.put(2, 8)
			for _, v := range values {
				w.put(uint64(v), 16)
			}
		}
	}
	first := flacCustomStream(t, 16, 1, 16, 0, codes, emit(codes[:16]))
	second := flacCustomStream(t, 16, 1, 8, 0, codes[16:], emit(codes[16:]))[42:]
	binary.BigEndian.PutUint16(first[10:12], 32)
	packed := binary.BigEndian.Uint64(first[18:26])
	binary.BigEndian.PutUint64(first[18:26], packed&^((1<<36)-1)|24)
	for index, frame := range [][]byte{first[42:], second} {
		frame[1] = 249
		frame[4] = byte(index * 16)
		frame[7] = byte(flacTestCRC(frame[:7], 8))
		binary.BigEndian.PutUint16(frame[len(frame)-2:], uint16(flacTestCRC(frame[:len(frame)-2], 16)))
	}
	checkCustomFLAC(t, append(first, second...), 16, codes)
}

func TestReadFLACHeaderCodesAndStreamInfo(t *testing.T) {
	constant := func(w *flacTestBuilder) { w.put(0, 8); w.put(0, 16) }
	reheader := func(data, header []byte) []byte {
		frame := append(bytes.Clone(header), byte(flacTestCRC(header, 8)))
		frame = append(frame, data[50:len(data)-2]...)
		crc := flacTestCRC(frame, 16)
		frame = append(frame, byte(crc>>8), byte(crc))
		return append(bytes.Clone(data[:42]), frame...)
	}
	base := flacCustomStream(t, 16, 1, 16, 0, make([]int64, 16), constant)
	for code := 1; code < 16; code++ {
		var n int
		header := []byte{255, 248, byte(code << 4), 0, 0}
		switch code {
		case 1:
			n = 192
		case 2, 3, 4, 5:
			n = 144 << code
		case 6:
			n = 16
			header = append(header, 15)
		case 7:
			n = 16
			header = append(header, 0, 15)
		default:
			n = 1 << code
		}
		codes := make([]int64, n)
		data := flacCustomStream(t, 16, 1, n, 0, codes, constant)
		checkCustomFLAC(t, reheader(data, header), 16, codes)
	}
	for _, tc := range []struct {
		code  byte
		extra []byte
	}{
		{0, nil}, {10, nil}, {12, []byte{48}}, {13, []byte{0xbb, 0x80}}, {14, []byte{0x12, 0xc0}},
	} {
		h := []byte{255, 248, 0x70 | tc.code, 0, 0, 0, 15}
		h = append(h, tc.extra...)
		checkCustomFLAC(t, reheader(base, h), 16, make([]int64, 16))
	}
	for _, number := range [][]byte{{1}, {0xc0, 0x80}, {0xc2, 0x7f}, {0xff}, {0xfe, 0x82, 0x80, 0x80, 0x80, 0x80, 0x80}} {
		h := append([]byte{255, 248, 0x70, 0}, number...)
		h = append(h, 0, 15)
		if _, err := ReadFLAC(bytes.NewReader(reheader(base, h)), FLACReadOptions{}); !errors.Is(err, ErrInvalidInput) {
			t.Fatal("invalid number accepted", number, err)
		}
	}
	for _, offset := range []int{42, 43, 44, 45} {
		bad := bytes.Clone(base)
		switch offset {
		case 42:
			bad[offset] = 0
		case 43:
			bad[offset] = 0xfc
		case 44:
			bad[offset] = 0x0f
		case 45:
			bad[offset] = 0xff
		}
		if _, err := ReadFLAC(bytes.NewReader(bad), FLACReadOptions{}); err == nil {
			t.Fatal("bad header accepted", offset)
		}
	}
	for _, depth := range []int{1, 8, 12, 20, 32} {
		bad := bytes.Clone(base)
		packed := binary.BigEndian.Uint64(bad[18:26]) &^ (uint64(31) << 36)
		binary.BigEndian.PutUint64(bad[18:26], packed|uint64(depth-1)<<36)
		if _, err := ReadFLAC(bytes.NewReader(bad), FLACReadOptions{}); err == nil {
			t.Fatal("unsupported depth", depth)
		}
	}
	for _, total := range []uint64{15, 17} {
		bad := bytes.Clone(base)
		packed := binary.BigEndian.Uint64(bad[18:26]) &^ ((1 << 36) - 1)
		binary.BigEndian.PutUint64(bad[18:26], packed|total)
		if _, err := ReadFLAC(bytes.NewReader(bad), FLACReadOptions{}); !errors.Is(err, ErrInvalidInput) {
			t.Fatal("bad total", total, err)
		}
	}
}

func TestReadFLACMD5Availability(t *testing.T) {
	if _, err := md5.New().Write(nil); err == nil {
		t.Skip("select with GODEBUG=fips140=only")
	}
	data, err := os.ReadFile(filepath.Join("flac", "pcm16_1ch_silence_n1_b16.flac"))
	if err != nil {
		t.Fatal(err)
	}
	if p, err := ReadFLAC(bytes.NewReader(data), FLACReadOptions{}); !errors.Is(err, ErrUnsupported) || p != nil {
		t.Fatal("unavailable MD5", err)
	}
}

func TestReadFLACIntegrityAndLimits(t *testing.T) {
	p, _ := flacIntegerPCM(t, 24, 2, 257, 48000, "random")
	var encoded bytes.Buffer
	if err := WriteFLAC(&encoded, p, FLACWriteOptions{Encoding: PCM24, BlockSize: 256}); err != nil {
		t.Fatal(err)
	}
	data := encoded.Bytes()
	for n := 0; n < len(data); n++ {
		if got, err := ReadFLAC(bytes.NewReader(data[:n]), FLACReadOptions{}); err == nil || got != nil {
			t.Fatalf("accepted truncation at %d", n)
		}
	}
	for _, offset := range []int{26, 41, 49, 50, 51, 55, len(data) - 1, len(data) - 2} {
		bad := bytes.Clone(data)
		bad[offset] ^= 1
		if got, err := ReadFLAC(bytes.NewReader(bad), FLACReadOptions{}); err == nil || got != nil {
			t.Fatal("accepted corruption", offset)
		}
	}
	for _, extra := range [][]byte{{0}, []byte("fLaC"), data} {
		bad := append(bytes.Clone(data), extra...)
		if _, err := ReadFLAC(bytes.NewReader(bad), FLACReadOptions{}); err == nil {
			t.Fatal("accepted trailing/concatenated content")
		}
	}
	for _, opts := range []FLACReadOptions{
		{Limits: Limits{MaxBytes: int64(len(data) - 1)}},
		{Limits: Limits{MaxFrames: 256}}, {Limits: Limits{MaxSamples: 513}}, {MaxWork: 8},
	} {
		if _, err := ReadFLAC(bytes.NewReader(data), opts); !errors.Is(err, ErrLimit) {
			t.Fatal("resource bound did not fail", opts, err)
		}
	}
	if _, err := ReadFLAC(bytes.NewReader(data), FLACReadOptions{Limits: Limits{MaxBytes: int64(len(data)), MaxChunks: 1}}); err != nil {
		t.Fatal("exact byte bound", err)
	}
	if _, err := ReadFLAC(bytes.NewReader(append(bytes.Clone(data), 0)), FLACReadOptions{Limits: Limits{MaxBytes: int64(len(data))}}); !errors.Is(err, ErrLimit) {
		t.Fatal("EOF probe must detect byte excess", err)
	}
	if _, err := ReadFLAC(nil, FLACReadOptions{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatal(err)
	}
	if _, err := ReadFLAC(bytes.NewReader(data), FLACReadOptions{Limits: Limits{MaxChunks: -1}}); !errors.Is(err, ErrInvalidInput) {
		t.Fatal(err)
	}
	unknown := bytes.Clone(data)
	packed := binary.BigEndian.Uint64(unknown[18:26]) &^ ((1 << 36) - 1)
	binary.BigEndian.PutUint64(unknown[18:26], packed)
	if _, err := ReadFLAC(bytes.NewReader(unknown), FLACReadOptions{}); err != nil {
		t.Fatal("unknown total with checked MD5 should work", err)
	}
	clear(unknown[26:42])
	if _, err := ReadFLAC(bytes.NewReader(unknown), FLACReadOptions{}); !errors.Is(err, ErrUnsupported) {
		t.Fatal("missing MD5 must be explicit", err)
	}
}

func flacAddMetadata(data []byte, kind byte, payload []byte) []byte {
	b := bytes.Clone(data[:42])
	b[4] = 0
	b = append(b, 128|kind, byte(len(payload)>>16), byte(len(payload)>>8), byte(len(payload)))
	b = append(b, payload...)
	return append(b, data[42:]...)
}

func flacTestComments(fields ...string) []byte {
	var b bytes.Buffer
	binary.Write(&b, binary.LittleEndian, uint32(4))
	b.WriteString("test")
	binary.Write(&b, binary.LittleEndian, uint32(len(fields)))
	for _, field := range fields {
		binary.Write(&b, binary.LittleEndian, uint32(len(field)))
		b.WriteString(field)
	}
	return b.Bytes()
}

func TestReadFLACMetadataPolicy(t *testing.T) {
	p, _ := flacIntegerPCM(t, 16, 1, 17, 48000, "sine")
	var b bytes.Buffer
	if err := WriteFLAC(&b, p, FLACWriteOptions{}); err != nil {
		t.Fatal(err)
	}
	var picture bytes.Buffer
	for _, v := range []uint32{3, 3} {
		binary.Write(&picture, binary.BigEndian, v)
	}
	picture.WriteString("-->")
	for range 5 {
		binary.Write(&picture, binary.BigEndian, uint32(0))
	}
	binary.Write(&picture, binary.BigEndian, uint32(9))
	picture.WriteString("cover.png")
	for _, tc := range []struct {
		kind byte
		b    []byte
	}{
		{2, []byte("testopaque")}, {3, nil}, {4, flacTestComments("TITLE=你好\nworld")}, {6, picture.Bytes()},
	} {
		data := flacAddMetadata(b.Bytes(), tc.kind, tc.b)
		if _, err := ReadFLAC(bytes.NewReader(data), FLACReadOptions{}); !errors.Is(err, ErrUnsupported) {
			t.Fatal("metadata loss must require opt-in", tc.kind, err)
		}
		if _, err := ReadFLAC(bytes.NewReader(data), FLACReadOptions{DiscardMetadata: true}); err != nil {
			t.Fatal("explicit metadata discard", tc.kind, err)
		}
		if _, err := ReadFLAC(bytes.NewReader(data), FLACReadOptions{DiscardMetadata: true, Limits: Limits{MaxChunks: 1}}); !errors.Is(err, ErrLimit) {
			t.Fatal("metadata count not bounded", err)
		}
	}
	for _, kind := range []byte{5, 7, 126, 127} {
		if _, err := ReadFLAC(bytes.NewReader(flacAddMetadata(b.Bytes(), kind, nil)), FLACReadOptions{DiscardMetadata: true}); err == nil {
			t.Fatal("unsupported/reserved metadata skipped", kind)
		}
	}
	for _, tc := range []struct {
		kind byte
		b    []byte
	}{
		{1, []byte{1}}, {2, nil}, {3, []byte{1}}, {4, []byte{0}}, {6, nil},
		{4, flacTestComments("bad")}, {4, flacTestComments("=bad")}, {4, flacTestComments("X=\xff")},
		{4, flacTestComments("WAVEFORMATEXTENSIBLE_CHANNEL_MASK=0x1")},
	} {
		if _, err := ReadFLAC(bytes.NewReader(flacAddMetadata(b.Bytes(), tc.kind, tc.b)), FLACReadOptions{DiscardMetadata: true}); err == nil {
			t.Fatal("bad/semantic metadata accepted", tc.kind)
		}
	}
	if _, err := ReadFLAC(bytes.NewReader(flacAddMetadata(b.Bytes(), 1, make([]byte, 7))), FLACReadOptions{}); err != nil {
		t.Fatal("zero padding is safe", err)
	}
	duplicate := flacAddMetadata(b.Bytes(), 4, flacTestComments())
	duplicate[42] = 4
	end := 42 + 4 + len(flacTestComments())
	duplicate = append(append(append(bytes.Clone(duplicate[:end]), 132, 0, 0, byte(len(flacTestComments()))), flacTestComments()...), duplicate[end:]...)
	if _, err := ReadFLAC(bytes.NewReader(duplicate), FLACReadOptions{DiscardMetadata: true}); !errors.Is(err, ErrInvalidInput) {
		t.Fatal("duplicate comment metadata accepted", err)
	}
}

func TestReadFLACIndependentStorageAndConcurrent(t *testing.T) {
	p, _ := flacIntegerPCM(t, 24, 2, 4097, 48000, "sine")
	var b bytes.Buffer
	if err := WriteFLAC(&b, p, FLACWriteOptions{Encoding: PCM24}); err != nil {
		t.Fatal(err)
	}
	data := b.Bytes()
	var wg sync.WaitGroup
	for range 6 {
		wg.Go(func() {
			got, err := ReadFLAC(bytes.NewReader(data), FLACReadOptions{})
			if err != nil {
				t.Error(err)
				return
			}
			closeSamples(t, got.Samples(), p.Samples(), 0)
			x := got.Samples()
			x[0] = 0.99
			closeSamples(t, got.Samples(), p.Samples(), 0)
		})
	}
	wg.Wait()
	got, err := ReadFLAC(bytes.NewReader(data), FLACReadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	clear(data)
	closeSamples(t, got.Samples(), p.Samples(), 0)
}

// Manifest files are supplied by the development libFLAC/FFmpeg generator.
// Ordinary tests consume only committed fixtures; optional full matrices use
// GOMATH_FLAC_DECODE_INPUT for this same Go-only comparison.
func TestReadFLACLibFLACFixtures(t *testing.T) {
	directory := os.Getenv("GOMATH_FLAC_DECODE_INPUT")
	fullMatrix := directory != ""
	if directory == "" {
		directory = filepath.Join("flac_decode")
	}
	data, err := os.ReadFile(filepath.Join(directory, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var entries []struct {
		File, SHA256, PCMSHA256       string
		Rate, Channels, Depth, Frames int
	}
	if err := json.Unmarshal(data, &entries); err != nil || len(entries) == 0 {
		t.Fatal("missing independent decoder fixtures", err)
	}
	for _, entry := range entries {
		t.Run(entry.File, func(t *testing.T) {
			encoded, err := os.ReadFile(filepath.Join(directory, entry.File))
			if err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256(encoded)
			if hex.EncodeToString(sum[:]) != entry.SHA256 {
				t.Fatal("independent encoded fixture hash changed")
			}
			if _, err := ReadFLAC(bytes.NewReader(encoded), FLACReadOptions{}); !errors.Is(err, ErrUnsupported) {
				t.Fatal("libFLAC metadata should require explicit discard", err)
			}
			got, err := ReadFLAC(bytes.NewReader(encoded), FLACReadOptions{DiscardMetadata: true})
			if err != nil {
				t.Fatal(err)
			}
			if got.Format() != (Format{entry.Rate, entry.Channels, 0}) || got.Frames() != entry.Frames {
				t.Fatal("independent format mismatch", got.Format(), got.Frames())
			}
			raw := make([]byte, len(got.samples)*(entry.Depth/8))
			for i, v := range got.samples {
				code := int64(math.Ldexp(v, entry.Depth-1))
				for j := 0; j < entry.Depth/8; j++ {
					raw[i*(entry.Depth/8)+j] = byte(code >> uint(j*8))
				}
			}
			sum = sha256.Sum256(raw)
			if hex.EncodeToString(sum[:]) != entry.PCMSHA256 {
				t.Fatal("Go-decoded samples differ from integer input and independent FFmpeg decode")
			}
			if fullMatrix {
				want, err := os.ReadFile(filepath.Join(directory, strings.TrimSuffix(entry.File, ".flac")+".raw"))
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(raw, want) {
					t.Fatal("complete Go-decoded PCM bytes differ from independent raw input")
				}
			}
		})
	}
}

func FuzzReadFLAC(f *testing.F) {
	f.Add([]byte("fLaC"))
	for _, name := range []string{"pcm16_1ch_silence_n1_b16.flac", "pcm24_1ch_escape1_n512_b4096.flac"} {
		b, err := os.ReadFile(filepath.Join("flac", name))
		if err != nil {
			f.Fatal(err)
		}
		f.Add(b)
	}
	for _, name := range []string{"libflac_pcm24_1ch_wasted_n17_l0.flac", "libflac_pcm24_2ch_sine_n8193_l8_lpc32.flac"} {
		b, err := os.ReadFile(filepath.Join("flac_decode", name))
		if err != nil {
			f.Fatal(err)
		}
		f.Add(b)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 32768 {
			t.Skip()
		}
		p, err := ReadFLAC(bytes.NewReader(data), FLACReadOptions{DiscardMetadata: true, Limits: Limits{MaxBytes: 32768, MaxFrames: 16384, MaxSamples: 32768, MaxChunks: 16}, MaxWork: 1 << 20})
		if err == nil {
			if p.Frames() > 16384 || len(p.samples) > 32768 {
				t.Fatal("decoded limits exceeded")
			}
			for _, v := range p.samples {
				if !finite(v) || v < -1 || v >= 1 {
					t.Fatal("decoded sample outside normalized integer PCM range")
				}
			}
		} else if p != nil {
			t.Fatal("error returned partial PCM")
		}
	})
}

func BenchmarkReadFLAC48kStereo(b *testing.B) {
	for _, depth := range []int{16, 24} {
		p, _ := flacIntegerPCM(b, depth, 2, 48000, 48000, "sine")
		e := PCM16
		if depth == 24 {
			e = PCM24
		}
		var encoded bytes.Buffer
		if err := WriteFLAC(&encoded, p, FLACWriteOptions{Encoding: e}); err != nil {
			b.Fatal(err)
		}
		b.Run(fmt.Sprintf("PCM%d", depth), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := ReadFLAC(bytes.NewReader(encoded.Bytes()), FLACReadOptions{}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func TestReadFLACReaderErrors(t *testing.T) {
	if _, err := ReadFLAC(&flacErrorReader{}, FLACReadOptions{}); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal("underlying error lost", err)
	}
}

type flacErrorReader struct{}

func (*flacErrorReader) Read([]byte) (int, error) { return 0, io.ErrClosedPipe }
