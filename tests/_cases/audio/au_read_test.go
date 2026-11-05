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

func auTestWidth(tag uint32) int {
	switch tag {
	case 1, 2, 27:
		return 1
	case 3:
		return 2
	case 4:
		return 3
	case 5:
		return 4
	}
	panic("test encoding")
}
func auTestWave(tag uint32, ch, frames int, unknown bool, annotation []byte) []byte {
	width := auTestWidth(tag)
	b := make([]byte, 24+len(annotation)+ch*frames*width)
	copy(b, ".snd")
	binary.BigEndian.PutUint32(b[4:], uint32(24+len(annotation)))
	size := uint32(ch * frames * width)
	if unknown {
		size = math.MaxUint32
	}
	binary.BigEndian.PutUint32(b[8:], size)
	binary.BigEndian.PutUint32(b[12:], tag)
	binary.BigEndian.PutUint32(b[16:], 48000)
	binary.BigEndian.PutUint32(b[20:], uint32(ch))
	copy(b[24:], annotation)
	for i := 24 + len(annotation); i < len(b); i++ {
		b[i] = byte(i*73 + 19)
	}
	return b
}
func auReject(t *testing.T, b []byte, o AUReadOptions, kind error) {
	t.Helper()
	p, e := ReadAU(bytes.NewReader(b), o)
	if p != nil || e == nil || (kind != nil && !errors.Is(e, kind)) {
		t.Fatalf("expected nil PCM / %v, got %v / %v", kind, p, e)
	}
}
func auFixtureDir(t testing.TB) string {
	t.Helper()
	if p := os.Getenv("GOMATH_AU_REFERENCE_DIR"); p != "" {
		return p
	}
	start, e := os.Getwd()
	if e != nil {
		t.Fatal(e)
	}
	for root := start; ; root = filepath.Dir(root) {
		if st, e := os.Stat(filepath.Join(root, "go.mod")); e == nil && !st.IsDir() {
			return filepath.Join(root, "tests", "data", "audio", "au")
		}
		if root == filepath.Dir(root) {
			t.Fatal("AU fixture root", start)
		}
	}
}
func TestReadAUExactIntegerEndpoints(t *testing.T) {
	for _, tag := range []uint32{2, 3, 4, 5} {
		width := auTestWidth(tag)
		scale := int64(1) << (width*8 - 1)
		values := []int64{0, scale - 1, -scale, -1, 1, scale / 2, -scale / 2}
		for _, unknown := range []bool{false, true} {
			b := auTestWave(tag, 1, len(values), unknown, nil)
			for i, v := range values {
				for j := range width {
					b[24+i*width+j] = byte(v >> uint(8*(width-j-1)))
				}
			}
			p, e := ReadAU(bytes.NewReader(b), AUReadOptions{AllowUnknownSize: unknown})
			if e != nil {
				t.Fatal(e)
			}
			if p.Format() != (Format{48000, 1, 0}) || p.Frames() != len(values) {
				t.Fatal("format/length", p.Format(), p.Frames())
			}
			for i, v := range values {
				want := float64(v) / float64(scale)
				if math.Float64bits(p.samples[i]) != math.Float64bits(want) {
					t.Fatal(tag, unknown, i, p.samples[i], want)
				}
			}
		}
	}
}
func TestReadAUG711AllCodesAndChannelOrder(t *testing.T) {
	b, e := os.ReadFile(filepath.Join(filepath.Dir(auFixtureDir(t)), "g711", "code_tables.json"))
	if e != nil {
		t.Fatal(e)
	}
	var tables struct{ ALaw, MuLaw []int }
	if e = json.Unmarshal(b, &tables); e != nil {
		t.Fatal(e)
	}
	for _, tag := range []uint32{1, 27} {
		table := tables.ALaw
		if tag == 1 {
			table = tables.MuLaw
		}
		if len(table) != 256 {
			t.Fatal("midpoint table")
		}
		for _, ch := range []int{1, 2, 3, 6, 64} {
			for _, unknown := range []bool{false, true} {
				w := auTestWave(tag, ch, 256, unknown, nil)
				for frame := range 256 {
					for c := range ch {
						w[24+frame*ch+c] = byte(frame + c*37)
					}
				}
				p, e := ReadAU(bytes.NewReader(w), AUReadOptions{AllowUnknownSize: unknown})
				if e != nil || p.Format() != (Format{48000, ch, 0}) {
					t.Fatal("indexed layout", tag, ch, e)
				}
				for frame := range 256 {
					for c := range ch {
						want := float64(table[byte(frame+c*37)]) / 32768
						if math.Float64bits(p.samples[frame*ch+c]) != math.Float64bits(want) {
							t.Fatal("code/channel mapping", tag, ch, frame, c)
						}
					}
				}
			}
		}
	}
}

func TestReadAUHeaderStrictSubset(t *testing.T) {
	if p, e := ReadAU(nil, AUReadOptions{}); p != nil || !errors.Is(e, ErrInvalidInput) {
		t.Fatal("nil reader", e)
	}
	base := auTestWave(3, 2, 3, false, nil)
	for _, magic := range []string{"dns.", ".sd\x00", "RIFF", "FORM", ".SND"} {
		b := bytes.Clone(base)
		copy(b, magic)
		auReject(t, b, AUReadOptions{}, ErrUnsupported)
	}
	for _, off := range []uint32{0, 1, 20, 23} {
		b := bytes.Clone(base)
		binary.BigEndian.PutUint32(b[4:], off)
		auReject(t, b, AUReadOptions{}, ErrInvalidInput)
	}
	for _, off := range []uint32{25, 26, 27} {
		b := bytes.Clone(base)
		binary.BigEndian.PutUint32(b[4:], off)
		auReject(t, b, AUReadOptions{DiscardMetadata: true}, ErrUnsupported)
	}
	for _, tag := range []uint32{0, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 28, 0xffffffff} {
		b := bytes.Clone(base)
		binary.BigEndian.PutUint32(b[12:], tag)
		auReject(t, b, AUReadOptions{DiscardMetadata: true, AllowUnknownSize: true}, ErrUnsupported)
	}
	for _, field := range []struct {
		off    int
		values []uint32
	}{{16, []uint32{0, MaxSampleRate + 1, math.MaxUint32}}, {20, []uint32{0, 65, math.MaxUint32}}} {
		for _, v := range field.values {
			b := bytes.Clone(base)
			binary.BigEndian.PutUint32(b[field.off:], v)
			kind := ErrUnsupported
			if v == 0 {
				kind = ErrInvalidInput
			}
			auReject(t, b, AUReadOptions{}, kind)
		}
	}
	for _, rate := range []uint32{1, 44100, MaxSampleRate} {
		for _, ch := range []int{1, 64} {
			b := auTestWave(5, ch, 1, false, nil)
			binary.BigEndian.PutUint32(b[16:], rate)
			p, e := ReadAU(bytes.NewReader(b), AUReadOptions{})
			if e != nil || p.Format() != (Format{int(rate), ch, 0}) {
				t.Fatal("rate/channel boundary", rate, ch, e)
			}
		}
	}
	for _, n := range []uint32{1, 2, 3, 5, 7, 9, 10, 11} {
		b := bytes.Clone(base)
		binary.BigEndian.PutUint32(b[8:], n)
		auReject(t, b, AUReadOptions{}, ErrInvalidInput)
	}
	for _, l := range []Limits{{MaxBytes: -1}, {MaxFrames: -1}, {MaxSamples: -1}, {MaxChunks: -1}} {
		auReject(t, base, AUReadOptions{Limits: l}, ErrInvalidInput)
	}
}

func TestReadAUAnnotationOffsetsAndKnownBoundary(t *testing.T) {
	for _, tag := range []uint32{1, 2, 3, 4, 5, 27} {
		for _, n := range []int{0, 4, 5, 7, 4096, 4097} {
			for _, unknown := range []bool{false, true} {
				w := auTestWave(tag, 3, 17, unknown, make([]byte, n))
				opt := AUReadOptions{AllowUnknownSize: unknown}
				p, e := ReadAU(bytes.NewReader(w), opt)
				if e != nil || p.Frames() != 17 {
					t.Fatal("zero padding / unaligned byte offset", tag, n, unknown, e)
				}
				if n > 0 {
					for _, at := range []int{0, n - 1} {
						b := bytes.Clone(w)
						b[24+at] = 255
						auReject(t, b, opt, ErrUnsupported)
						opt2 := opt
						opt2.DiscardMetadata = true
						p, e = ReadAU(bytes.NewReader(b), opt2)
						if e != nil || p.Frames() != 17 {
							t.Fatal("explicit binary annotation discard", n, e)
						}
					}
				}
			}
		}
		// A known-size object can be concatenated. Neither EOF nor a zero pad is
		// required after audio, even for a one-byte payload and odd data offset.
		w := auTestWave(tag, 1, 1, false, make([]byte, 5))
		next := auTestWave(3, 2, 3, false, nil)
		r := bytes.NewReader(append(bytes.Clone(w), next...))
		p, e := ReadAU(oneByteReader{r}, AUReadOptions{})
		if e != nil || p.Frames() != 1 || r.Len() != len(next) {
			t.Fatal("known object boundary", tag, e, r.Len())
		}
		p, e = ReadAU(r, AUReadOptions{})
		if e != nil || p.Frames() != 3 || r.Len() != 0 {
			t.Fatal("second object", e)
		}
		// Embedded zero does not terminate the annotation. Offset does.
		w = auTestWave(tag, 1, 1, false, []byte{'x', 0, 0, 0, 0, 0, 0, 'y'})
		auReject(t, w, AUReadOptions{}, ErrUnsupported)
		if _, e = ReadAU(bytes.NewReader(w), AUReadOptions{DiscardMetadata: true}); e != nil {
			t.Fatal(e)
		}
	}
}

func TestReadAUUnknownSizeEOFAndBudgetPolicy(t *testing.T) {
	for _, tag := range []uint32{1, 2, 3, 4, 5, 27} {
		for _, ch := range []int{1, 3, 64} {
			for _, frames := range []int{0, 1, 17, 12000} {
				w := auTestWave(tag, ch, frames, true, nil)
				width := auTestWidth(tag)
				auReject(t, w, AUReadOptions{}, ErrUnsupported)
				p, e := ReadAU(oneByteReader{bytes.NewReader(w)}, AUReadOptions{AllowUnknownSize: true})
				if e != nil || p.Frames() != frames {
					t.Fatal("EOF unknown", tag, ch, frames, e)
				}
				exact := Limits{MaxBytes: int64(len(w)), MaxFrames: max(frames, 1), MaxSamples: max(frames*ch, 1)}
				p, e = ReadAU(bytes.NewReader(w), AUReadOptions{Limits: exact, AllowUnknownSize: true})
				if e != nil || p.Frames() != frames {
					t.Fatal("exact EOF / budget probe", e)
				}
				// Probing for EOF consumes exactly one excess byte, then errors instead
				// of treating a budget as the end of the stream.
				r := bytes.NewReader(append(bytes.Clone(w), []byte{9, 8}...))
				p, e = ReadAU(r, AUReadOptions{Limits: exact, AllowUnknownSize: true})
				if p != nil || !errors.Is(e, ErrLimit) || r.Len() != 1 {
					t.Fatal("bounded one-byte probe", tag, ch, frames, e, r.Len())
				}
				if frames > 1 {
					for _, l := range []Limits{{MaxFrames: frames - 1}, {MaxSamples: frames*ch - 1}, {MaxBytes: int64(len(w) - 1)}} {
						auReject(t, w, AUReadOptions{Limits: l, AllowUnknownSize: true}, ErrLimit)
					}
				}
				if width*ch > 1 && frames > 0 {
					for cut := 1; cut < width*ch; cut++ {
						auReject(t, w[:len(w)-cut], AUReadOptions{AllowUnknownSize: true}, ErrInvalidInput)
					}
				}
			}
		}
	}
	// A trailing next object is DATA when EOF framing was explicitly selected.
	// The codec cannot discover concatenation boundaries from unknown size.
	a := auTestWave(2, 1, 1, true, nil)
	second := auTestWave(2, 1, 1, false, nil)
	p, e := ReadAU(bytes.NewReader(append(a, second...)), AUReadOptions{AllowUnknownSize: true})
	if e != nil || p.Frames() != 26 {
		t.Fatal("explicit unknown-size EOF semantics", e)
	}
}

type auNoProgressReader struct{}

func (auNoProgressReader) Read([]byte) (int, error) { return 0, nil }

type auInvalidCountReader struct{ count int }

func (r auInvalidCountReader) Read([]byte) (int, error) { return r.count, nil }

type auFinalErrorReader struct {
	data []byte
	err  error
}

func (r *auFinalErrorReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, r.err
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	if len(r.data) == 0 {
		return n, r.err
	}
	return n, nil
}

func TestReadAUIOErrorsTruncationAndProgress(t *testing.T) {
	for _, unknown := range []bool{false, true} {
		w := auTestWave(4, 3, 7, unknown, make([]byte, 7))
		o := AUReadOptions{AllowUnknownSize: unknown}
		for end := 0; end < len(w); end++ {
			p, e := ReadAU(bytes.NewReader(w[:end]), o)
			validUnknown := unknown && end >= 31 && (end-31)%9 == 0
			if validUnknown {
				if e != nil || p.Frames() != (end-31)/9 {
					t.Fatal("unknown clean early EOF", end, e)
				}
			} else if p != nil || e == nil {
				t.Fatal("truncated stream accepted", unknown, end, e)
			}
			r := io.MultiReader(bytes.NewReader(w[:end]), g711ErrorReader{})
			p, e = ReadAU(r, o)
			if p != nil || !errors.Is(e, io.ErrClosedPipe) {
				t.Fatal("underlying error", unknown, end, e)
			}
		}
		for _, sentinel := range []error{io.ErrClosedPipe, io.ErrUnexpectedEOF} {
			p, e := ReadAU(&auFinalErrorReader{bytes.Clone(w), sentinel}, o)
			if p != nil || !errors.Is(e, sentinel) {
				t.Fatal("full final read masked transport error", unknown, e)
			}
		}
		p, e := ReadAU(&auFinalErrorReader{bytes.Clone(w), io.EOF}, o)
		if e != nil || p.Frames() != 7 {
			t.Fatal("data+EOF single read", e)
		}
	}
	for _, reader := range []io.Reader{auNoProgressReader{}, io.MultiReader(bytes.NewReader(auTestWave(2, 1, 0, true, nil)), auNoProgressReader{})} {
		p, e := ReadAU(reader, AUReadOptions{AllowUnknownSize: true})
		if p != nil || !errors.Is(e, io.ErrNoProgress) {
			t.Fatal("bounded zero-read loop", e)
		}
	}
	for _, n := range []int{-1, 25} {
		p, e := ReadAU(auInvalidCountReader{n}, AUReadOptions{})
		if p != nil || !errors.Is(e, ErrInvalidInput) {
			t.Fatal("invalid io.Reader count", n, e)
		}
	}
	// Exercise EOF probe error, including an error returned alongside its byte.
	w := auTestWave(2, 1, 1, true, nil)
	for _, extra := range [][]byte{nil, {1}} {
		r := io.MultiReader(bytes.NewReader(w), &auFinalErrorReader{extra, io.ErrClosedPipe})
		p, e := ReadAU(r, AUReadOptions{AllowUnknownSize: true, Limits: Limits{MaxFrames: 1}})
		if p != nil || !errors.Is(e, io.ErrClosedPipe) {
			t.Fatal("probe transport error", e)
		}
	}
}

func TestReadAUKnownPreflightAndExactBudgets(t *testing.T) {
	for _, tag := range []uint32{1, 2, 3, 4, 5, 27} {
		for _, ch := range []int{1, 3, 64} {
			w := auTestWave(tag, ch, 17, false, make([]byte, 5))
			l := Limits{MaxBytes: int64(len(w)), MaxFrames: 17, MaxSamples: 17 * ch, MaxChunks: 1}
			if _, e := ReadAU(bytes.NewReader(w), AUReadOptions{Limits: l}); e != nil {
				t.Fatal("exact budgets", tag, ch, e)
			}
			for _, l := range []Limits{{MaxBytes: 23}, {MaxBytes: 28}, {MaxBytes: int64(len(w) - 1)}, {MaxFrames: 16}, {MaxSamples: 17*ch - 1}} {
				auReject(t, w, AUReadOptions{Limits: l}, ErrLimit)
			}
		}
	}
	for _, unknown := range []bool{false, true} {
		b := auTestWave(2, 1, 0, unknown, nil)
		binary.BigEndian.PutUint32(b[4:], 1<<30)
		r := bytes.NewReader(b)
		p, e := ReadAU(r, AUReadOptions{AllowUnknownSize: true})
		if p != nil || !errors.Is(e, ErrLimit) || r.Len() != 0 {
			t.Fatal("annotation preflight", e, r.Len())
		}
	}
	// Header-only huge declaration is rejected before annotation read/allocation.
	b := auTestWave(2, 1, 0, false, nil)
	binary.BigEndian.PutUint32(b[8:], math.MaxUint32-1)
	binary.BigEndian.PutUint32(b[4:], 4096)
	r := bytes.NewReader(b)
	p, e := ReadAU(r, AUReadOptions{Limits: Limits{MaxBytes: 1 << 34, MaxFrames: 1}})
	if p != nil || !errors.Is(e, ErrLimit) || r.Len() != 0 {
		t.Fatal("known capacity preflight", e, r.Len())
	}
	// Unknown input can use an exact, empty 24-byte object budget with a probe.
	b = auTestWave(2, 1, 0, true, nil)
	if p, e = ReadAU(bytes.NewReader(b), AUReadOptions{AllowUnknownSize: true, Limits: Limits{MaxBytes: 24}}); e != nil || p.Frames() != 0 {
		t.Fatal("empty unknown budget", e)
	}
}

type auReferenceCase struct {
	File, PCMFile, EncodedSHA256, PCMSHA256      string
	Encoding                                     uint32
	Channels, Rate, Frames                       int
	DiscardMetadata, AllowUnknownSize, Benchmark bool
	FFmpeg, Libsndfile                           struct {
		Accepted    bool
		WholeSHA256 string
	}
}

func auManifest(t testing.TB) ([]auReferenceCase, []struct{ File, EncodedSHA256, Category string }) {
	t.Helper()
	b, e := os.ReadFile(filepath.Join(auFixtureDir(t), "manifest.json"))
	if e != nil {
		t.Fatal(e)
	}
	var m struct {
		Cases         []auReferenceCase
		RejectedCases []struct{ File, EncodedSHA256, Category string }
	}
	if e = json.Unmarshal(b, &m); e != nil || len(m.Cases) != 106 || len(m.RejectedCases) != 10 {
		t.Fatal("AU independent manifest", e)
	}

	for _, c := range m.Cases {
		if !c.Libsndfile.Accepted || c.Libsndfile.WholeSHA256 != c.PCMSHA256 {
			t.Fatal("complete independent libsndfile evidence", c.File)
		}
		if c.FFmpeg.Accepted {
			if c.FFmpeg.WholeSHA256 != c.PCMSHA256 {
				t.Fatal("complete independent FFmpeg evidence", c.File)
			}
		} else if c.Frames != 0 {
			t.Fatal("unexpected FFmpeg rejection", c.File)
		}
	}
	return m.Cases, m.RejectedCases
}
func auReadFixture(t testing.TB, name, hash string) []byte {
	t.Helper()
	b, e := os.ReadFile(filepath.Join(auFixtureDir(t), name))
	if e != nil {
		t.Fatal(e)
	}
	h := sha256.Sum256(b)
	if hex.EncodeToString(h[:]) != hash {
		t.Fatal("AU fixture hash", name)
	}
	return b
}
func TestReadAUIndependentFixtures(t *testing.T) {
	cases, rejected := auManifest(t)
	for _, c := range cases {
		t.Run(c.File, func(t *testing.T) {
			w := auReadFixture(t, c.File, c.EncodedSHA256)
			expected := auReadFixture(t, c.PCMFile, c.PCMSHA256)
			p, e := ReadAU(bytes.NewReader(w), AUReadOptions{DiscardMetadata: c.DiscardMetadata, AllowUnknownSize: c.AllowUnknownSize})
			if e != nil || p.Format() != (Format{c.Rate, c.Channels, 0}) || p.Frames() != c.Frames || len(p.samples)*4 != len(expected) {
				t.Fatal("independent format/length", e)
			}
			for i, v := range p.samples {
				want := float64(int32(binary.LittleEndian.Uint32(expected[4*i:]))) / 2147483648
				if math.Float64bits(v) != math.Float64bits(want) {
					t.Fatal("independent complete samples", i, v, want)
				}
			}
			if c.DiscardMetadata {
				auReject(t, w, AUReadOptions{AllowUnknownSize: c.AllowUnknownSize}, ErrUnsupported)
			}
			if c.AllowUnknownSize {
				auReject(t, w, AUReadOptions{DiscardMetadata: c.DiscardMetadata}, ErrUnsupported)
			}
		})
	}
	for _, c := range rejected {
		var kind error
		switch c.Category {
		case "unsupported":
			kind = ErrUnsupported
		case "invalid":
			kind = ErrInvalidInput
		case "truncated":
			kind = io.ErrUnexpectedEOF
		default:
			t.Fatal("reference error kind", c.Category)
		}
		auReject(t, auReadFixture(t, c.File, c.EncodedSHA256), AUReadOptions{DiscardMetadata: true, AllowUnknownSize: true}, kind)
	}
}
func TestReadAUConcurrentAndStorageOwnership(t *testing.T) {
	for _, unknown := range []bool{false, true} {
		w := auTestWave(4, 3, 17003, unknown, nil)
		o := AUReadOptions{AllowUnknownSize: unknown}
		p, e := ReadAU(bytes.NewReader(w), o)
		if e != nil {
			t.Fatal(e)
		}
		want := p.Samples()
		p.Samples()[0] = 17
		var wg sync.WaitGroup
		for range 8 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				got, e := ReadAU(bytes.NewReader(w), o)
				if e != nil {
					t.Error(e)
					return
				}
				for i, v := range got.samples {
					if math.Float64bits(v) != math.Float64bits(want[i]) {
						t.Error("concurrent decode", i)
						return
					}
				}
				got.samples[0] = 17
			}()
		}
		wg.Wait()
		clear(w)
		if p.samples[0] != want[0] {
			t.Fatal("ownership")
		}
	}
}
func FuzzReadAU(f *testing.F) {
	for _, tag := range []uint32{1, 2, 3, 4, 5, 27} {
		for _, unknown := range []bool{false, true} {
			f.Add(auTestWave(tag, 3, 17, unknown, make([]byte, 4)))
		}
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		l := Limits{MaxBytes: 1 << 16, MaxFrames: 4096, MaxSamples: 8192, MaxChunks: 1}
		p, e := ReadAU(bytes.NewReader(b), AUReadOptions{Limits: l, DiscardMetadata: true, AllowUnknownSize: true})
		if e != nil {
			if p != nil {
				t.Fatal("partial result")
			}
			return
		}
		if p.validate() != nil || p.Frames() > l.MaxFrames || len(p.samples) > l.MaxSamples || cap(p.samples) > l.MaxSamples {
			t.Fatal("invalid / unbounded result")
		}
	})
}
func BenchmarkReadAU(b *testing.B) {
	cases, _ := auManifest(b)
	for _, c := range cases {
		if !c.Benchmark {
			continue
		}
		w := auReadFixture(b, c.File, c.EncodedSHA256)
		b.Run(fmt.Sprintf("tag%d_unknown%t", c.Encoding, c.AllowUnknownSize), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(w)))
			b.ResetTimer()
			for range b.N {
				p, e := ReadAU(bytes.NewReader(w), AUReadOptions{AllowUnknownSize: c.AllowUnknownSize})
				if e != nil || p.Frames() != 48000 {
					b.Fatal(e)
				}
			}
		})
	}
}
