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
	"math/bits"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func aiffTestChunk(id string, data []byte) []byte {
	b := make([]byte, 8+len(data)+(len(data)&1))
	copy(b, id)
	binary.BigEndian.PutUint32(b[4:8], uint32(len(data)))
	copy(b[8:], data)
	return b
}

func aiffTestForm(kind string, chunks ...[]byte) []byte {
	b := []byte("FORM\x00\x00\x00\x00" + kind)
	for _, c := range chunks {
		b = append(b, c...)
	}
	binary.BigEndian.PutUint32(b[4:8], uint32(len(b)-8))
	return b
}

func aiffTestRate(rate int) []byte {
	b := make([]byte, 10)
	e := bits.Len(uint(rate)) - 1
	binary.BigEndian.PutUint16(b, uint16(e+16383))
	binary.BigEndian.PutUint64(b[2:], uint64(rate)<<uint(63-e))
	return b
}

func aiffTestCommon(depth, channels, frames, rate int, compression string, name []byte) []byte {
	b := make([]byte, 18)
	binary.BigEndian.PutUint16(b, uint16(channels))
	binary.BigEndian.PutUint32(b[2:], uint32(frames))
	binary.BigEndian.PutUint16(b[6:], uint16(depth))
	copy(b[8:], aiffTestRate(rate))
	if compression != "" {
		b = append(b, []byte(compression)...)
		b = append(b, byte(len(name)))
		b = append(b, name...)
		if len(b)&1 != 0 {
			b = append(b, 0)
		}
	}
	return aiffTestChunk("COMM", b)
}

func aiffTestSound(data []byte, offset, tail, block int) []byte {
	b := make([]byte, 8+offset)
	binary.BigEndian.PutUint32(b[:4], uint32(offset))
	binary.BigEndian.PutUint32(b[4:8], uint32(block))
	for i := range offset {
		b[8+i] = byte(i*13 + 7) // unused storage need not contain zeros
	}
	b = append(b, data...)
	for i := range tail {
		b = append(b, byte(77+i))
	}
	return aiffTestChunk("SSND", b)
}

func aiffTestVersion() []byte {
	return aiffTestChunk("FVER", []byte{0xa2, 0x80, 0x51, 0x40})
}

func aiffTestCodes(depth, channels, frames int, little bool) ([]byte, []float64) {
	width := depth / 8
	scale := int64(1) << (depth - 1)
	extremes := []int64{-scale, -scale + 1, -scale / 2, -1, 0, 1, scale / 2, scale - 1}
	raw, want := make([]byte, frames*channels*width), make([]float64, frames*channels)
	for i := range want {
		code := extremes[(i+i/len(extremes))%len(extremes)]
		if i >= len(extremes) {
			code = int64(uint64(i)*0x9e3779b9&uint64(2*scale-1)) - scale
		}
		want[i] = float64(code) / float64(scale)
		for j := range width {
			index := width - 1 - j
			if little {
				index = j
			}
			raw[i*width+index] = byte(code >> uint(8*j))
		}
	}
	return raw, want
}

func TestReadAIFFDepthsOrderPaddingAndOwnership(t *testing.T) {
	for _, compression := range []string{"", "NONE", "sowt"} {
		for _, depth := range []int{8, 16, 24, 32} {
			if compression == "sowt" && depth != 16 {
				continue
			}
			for _, ch := range []int{1, 2, 3, 6, 64} {
				for _, frames := range []int{0, 1, 3, 17, 10923} {
					if ch > 6 && frames > 17 {
						continue
					}
					for _, reverse := range []bool{false, true} {
						t.Run(fmt.Sprintf("%s/%d/%d/%d/reverse%t", compression, depth, ch, frames, reverse), func(t *testing.T) {
							raw, want := aiffTestCodes(depth, ch, frames, compression == "sowt")
							comm := aiffTestCommon(depth, ch, frames, 48000, compression, []byte("arbitrary display name"))
							ssnd := aiffTestSound(raw, 3, 5, 512)
							chunks := [][]byte{comm, ssnd}
							if reverse {
								chunks = [][]byte{ssnd, comm}
							}
							kind := "AIFF"
							if compression != "" {
								kind = "AIFC"
								chunks = append(chunks, aiffTestVersion()) // arbitrary FVER order
							}
							encoded := aiffTestForm(kind, chunks...)
							reader := bytes.NewReader(append(bytes.Clone(encoded), []byte("NEXT")...))
							var input io.Reader = reader
							if frames < 17 {
								input = oneByteReader{Reader: reader}
							}
							p, err := ReadAIFF(input, AIFFReadOptions{})
							if err != nil {
								t.Fatal(err)
							}
							if p.Format() != (Format{SampleRate: 48000, Channels: ch}) || p.Frames() != frames || reader.Len() != 4 {
								t.Fatal("format, frames or FORM boundary", p.Format(), p.Frames(), reader.Len())
							}
							closeSamples(t, p.Samples(), want, 0)
							clear(encoded)
							copied := p.Samples()
							clear(copied)
							closeSamples(t, p.Samples(), want, 0)
							for _, sample := range p.Samples() {
								if sample < -1 || sample >= 1 || !finite(sample) {
									t.Fatal("normalization", sample)
								}
							}
						})
					}
				}
			}
		}
	}
	for _, compression := range []string{"", "NONE", "sowt"} {
		kind := "AIFF"
		chunks := [][]byte{aiffTestCommon(16, 2, 0, 1, compression, nil)}
		if compression != "" {
			kind = "AIFC"
			chunks = append(chunks, aiffTestVersion())
		}
		p, err := ReadAIFF(bytes.NewReader(aiffTestForm(kind, chunks...)), AIFFReadOptions{})
		if err != nil || p.Frames() != 0 || p.SampleRate() != 1 {
			t.Fatal("empty without SSND", err)
		}
	}
}

func TestReadAIFFExtendedSampleRates(t *testing.T) {
	// All supported integers, plus the fixed 44100-Hz Appendix-A rate.
	known, _ := hex.DecodeString("400eac44000000000000")
	if !bytes.Equal(aiffTestRate(44100), known) {
		t.Fatal("canonical rate construction")
	}
	for rate := 1; rate <= MaxSampleRate; rate++ {
		got, err := aiffSampleRate(aiffTestRate(rate))
		if err != nil || got != rate {
			t.Fatal(rate, got, err)
		}
	}
	for _, rate := range []int{1, 8000, 11025, 22050, 44100, 48000, 96000, 192000, MaxSampleRate} {
		b := aiffTestRate(rate)
		b[9] = 1 // below float64 precision: must NOT round into an integer
		if _, err := aiffSampleRate(b); !errors.Is(err, ErrUnsupported) {
			t.Fatal("fraction rounded", rate, err)
		}
	}
	for _, hexRate := range []string{
		"00000000000000000000", "80000000000000000000", // +/- zero
		"bfff8000000000000000", "7fff8000000000000000", // negative, infinity
		"7fffc000000000000001", "400e0000000000000001", // NaN, unnormal
		"00008000000000000000", // pseudo-denormal
	} {
		b, _ := hex.DecodeString(hexRate)
		if _, err := aiffSampleRate(b); !errors.Is(err, ErrInvalidInput) {
			t.Fatal(hexRate, err)
		}
	}
	for _, hexRate := range []string{
		"00000000000000000001", // denormal
		"3ffe8000000000000000", // 0.5 Hz
		"401eb71b000000000000", // out of supported range
	} {
		b, _ := hex.DecodeString(hexRate)
		if _, err := aiffSampleRate(b); !errors.Is(err, ErrUnsupported) {
			t.Fatal(hexRate, err)
		}
	}
	if _, err := aiffSampleRate(aiffTestRate(MaxSampleRate + 1)); !errors.Is(err, ErrUnsupported) {
		t.Fatal("rate limit", err)
	}
}

func TestReadAIFFMetadataAndCompressionPolicy(t *testing.T) {
	comm := aiffTestCommon(16, 1, 1, 44100, "", nil)
	ssnd := aiffTestSound([]byte{0x80, 0}, 0, 0, 0)
	for _, id := range []string{"NAME", "AUTH", "(c) ", "ANNO"} {
		file := aiffTestForm("AIFF", aiffTestChunk(id, []byte("abc")), comm, ssnd)
		if _, err := ReadAIFF(bytes.NewReader(file), AIFFReadOptions{}); !errors.Is(err, ErrUnsupported) {
			t.Fatal("metadata silently lost", id, err)
		}
		p, err := ReadAIFF(bytes.NewReader(file), AIFFReadOptions{DiscardMetadata: true})
		if err != nil || p.samples[0] != -1 {
			t.Fatal("explicit text discard", id, err)
		}
		duplicate := aiffTestForm("AIFF", comm, ssnd, aiffTestChunk(id, nil), aiffTestChunk(id, nil))
		_, err = ReadAIFF(bytes.NewReader(duplicate), AIFFReadOptions{DiscardMetadata: true})
		if (id == "ANNO") != (err == nil) {
			t.Fatal("text multiplicity", id, err)
		}
	}
	for _, id := range []string{"MARK", "INST", "COMT", "APPL", "MIDI", "AESD", "CHAN", "ID3 ", "JUNK", "????", "FORM"} {
		for _, discard := range []bool{false, true} {
			file := aiffTestForm("AIFF", comm, ssnd, aiffTestChunk(id, nil))
			if p, err := ReadAIFF(bytes.NewReader(file), AIFFReadOptions{DiscardMetadata: discard}); p != nil || !errors.Is(err, ErrUnsupported) {
				t.Fatal("unknown/semantic chunk silently discarded", id, discard, err)
			}
		}
	}
	file := aiffTestForm("AIFF", comm, ssnd, aiffTestChunk("NAME", []byte{0x80}))
	if _, err := ReadAIFF(bytes.NewReader(file), AIFFReadOptions{DiscardMetadata: true}); !errors.Is(err, ErrUnsupported) {
		t.Fatal("text outside supported ASCII", err)
	}
	for _, compression := range []string{"ulaw", "alaw", "ima4", "fl32", "fl64", "twos", "raw "} {
		file := aiffTestForm("AIFC", aiffTestVersion(), aiffTestCommon(16, 1, 1, 44100, compression, nil), ssnd)
		if _, err := ReadAIFF(bytes.NewReader(file), AIFFReadOptions{DiscardMetadata: true}); !errors.Is(err, ErrUnsupported) {
			t.Fatal("compression", compression, err)
		}
	}
	for _, depth := range []int{0, 1, 7, 12, 20, 31, 33, 64} {
		file := aiffTestForm("AIFF", aiffTestCommon(depth, 1, 1, 44100, "", nil), ssnd)
		if _, err := ReadAIFF(bytes.NewReader(file), AIFFReadOptions{}); !errors.Is(err, ErrUnsupported) {
			t.Fatal("depth", depth, err)
		}
	}
	for _, depth := range []int{8, 24, 32} {
		file := aiffTestForm("AIFC", aiffTestVersion(), aiffTestCommon(depth, 1, 1, 44100, "sowt", nil), ssnd)
		if _, err := ReadAIFF(bytes.NewReader(file), AIFFReadOptions{}); !errors.Is(err, ErrUnsupported) {
			t.Fatal("sowt depth", depth, err)
		}
	}
}

func TestReadAIFFMalformedAndTruncated(t *testing.T) {
	comm := aiffTestCommon(24, 1, 3, 44100, "", nil)
	ssnd := aiffTestSound(make([]byte, 9), 3, 5, 16)
	valid := aiffTestForm("AIFF", comm, ssnd)
	for _, file := range [][]byte{valid, aiffTestForm("AIFF", ssnd, comm), aiffTestForm("AIFC", aiffTestVersion(), aiffTestCommon(16, 1, 1, 1, "NONE", nil), aiffTestSound([]byte{0, 0}, 0, 0, 0))} {
		for end := 0; end < len(file); end++ {
			if p, err := ReadAIFF(bytes.NewReader(file[:end]), AIFFReadOptions{}); err == nil || p != nil {
				t.Fatal("truncation accepted", end)
			}
		}
	}
	cases := map[string][]byte{
		"no-COMM":                      aiffTestForm("AIFF", ssnd),
		"no-SSND":                      aiffTestForm("AIFF", comm),
		"COMM-duplicate":               aiffTestForm("AIFF", comm, comm, ssnd),
		"SSND-duplicate":               aiffTestForm("AIFF", comm, ssnd, ssnd),
		"SSND-short":                   aiffTestForm("AIFF", comm, aiffTestChunk("SSND", make([]byte, 7))),
		"insufficient-frames":          aiffTestForm("AIFF", comm, aiffTestSound(make([]byte, 8), 0, 0, 0)),
		"deferred-insufficient-frames": aiffTestForm("AIFF", aiffTestSound(make([]byte, 8), 0, 0, 0), comm),
		"COMM-short":                   aiffTestForm("AIFF", aiffTestChunk("COMM", make([]byte, 17)), ssnd),
		"missing-FVER":                 aiffTestForm("AIFC", aiffTestCommon(24, 1, 3, 44100, "NONE", nil), ssnd),
		"duplicate-FVER":               aiffTestForm("AIFC", aiffTestVersion(), aiffTestVersion(), aiffTestCommon(24, 1, 3, 44100, "NONE", nil), ssnd),
		"bad-FVER-size":                aiffTestForm("AIFC", aiffTestChunk("FVER", nil)),
		"short-AIFC-COMM":              aiffTestForm("AIFC", aiffTestChunk("COMM", make([]byte, 22))),
		"overlong-AIFC-COMM":           aiffTestForm("AIFC", aiffTestChunk("COMM", make([]byte, 280))),
		"zero-channels":                aiffTestForm("AIFF", aiffTestCommon(16, 0, 1, 44100, "", nil), ssnd),
		"excess-channels":              aiffTestForm("AIFF", aiffTestCommon(16, 65, 1, 44100, "", nil), ssnd),
	}
	mutate := func(name string, source []byte, edit func([]byte)) {
		b := bytes.Clone(source)
		edit(b)
		cases[name] = b
	}
	mutate("FORM-too-small", valid, func(b []byte) { binary.BigEndian.PutUint32(b[4:8], 3) })
	mutate("FORM-negative", valid, func(b []byte) { binary.BigEndian.PutUint32(b[4:8], 0x80000000) })
	mutate("FORM-odd", valid, func(b []byte) { binary.BigEndian.PutUint32(b[4:8], uint32(len(b)-9)) })
	mutate("partial-chunk-header", aiffTestForm("AIFF", []byte{0, 0}), func(b []byte) {})
	mutate("chunk-negative", valid, func(b []byte) { binary.BigEndian.PutUint32(b[16:20], 0x80000000) })
	mutate("chunk-boundary", valid, func(b []byte) { binary.BigEndian.PutUint32(b[16:20], uint32(len(b))) })
	mutate("offset-boundary", valid, func(b []byte) { binary.BigEndian.PutUint32(b[46:50], 0xffffffff) })
	mutate("nonfinite-COMM-rate", valid, func(b []byte) { binary.BigEndian.PutUint16(b[28:30], 0x7fff) })
	mutate("bad-SSND-pad", valid, func(b []byte) { b[len(b)-1] = 1 })
	mutate("missing-SSND-pad", valid[:len(valid)-1], func(b []byte) { binary.BigEndian.PutUint32(b[4:8], uint32(len(b)-8)) })
	aifc := aiffTestForm("AIFC", aiffTestVersion(), aiffTestCommon(24, 1, 3, 44100, "NONE", nil), ssnd)
	mutate("bad-pstring-length", aifc, func(b []byte) { b[54] = 255 })
	mutate("bad-pstring-pad", aifc, func(b []byte) { b[55] = 1 })
	mutate("odd-AIFC-COMM", aifc, func(b []byte) { binary.BigEndian.PutUint32(b[28:32], 23) })
	for name, file := range cases {
		t.Run(name, func(t *testing.T) {
			if p, err := ReadAIFF(bytes.NewReader(file), AIFFReadOptions{DiscardMetadata: true}); p != nil || !errors.Is(err, ErrInvalidInput) {
				t.Fatal("expected invalid", err)
			}
		})
	}
	for _, file := range [][]byte{
		aiffTestForm("AIFF", aiffTestVersion(), comm, ssnd),
		aiffTestForm("AIFC", aiffTestChunk("FVER", []byte{0, 0, 0, 0}), aiffTestCommon(24, 1, 3, 44100, "NONE", nil), ssnd),
		[]byte("RIFF\x04\x00\x00\x00WAVE"),
	} {
		if p, err := ReadAIFF(bytes.NewReader(file), AIFFReadOptions{}); p != nil || !errors.Is(err, ErrUnsupported) {
			t.Fatal("unsupported", err)
		}
	}
	// All 256 legal pstring lengths, including non-ASCII localized names.
	for n := range 256 {
		file := aiffTestForm("AIFC", aiffTestVersion(), aiffTestCommon(16, 1, 0, 8000, "NONE", bytes.Repeat([]byte{0x81}, n)))
		if _, err := ReadAIFF(bytes.NewReader(file), AIFFReadOptions{}); err != nil {
			t.Fatal("legal pstring", n, err)
		}
	}
}

func TestReadAIFFResourceBudgetsAndIO(t *testing.T) {
	raw, _ := aiffTestCodes(32, 2, 17, false)
	comm := aiffTestCommon(32, 2, 17, 48000, "", nil)
	ssnd := aiffTestSound(raw, 7, 11, 512)
	for _, file := range [][]byte{aiffTestForm("AIFF", comm, ssnd), aiffTestForm("AIFF", ssnd, comm)} {
		exact := AIFFReadOptions{Limits: Limits{MaxFrames: 17, MaxSamples: 34, MaxBytes: int64(len(file)), MaxChunks: 2}, MaxWork: uint64(len(file) + 34)}
		if _, err := ReadAIFF(bytes.NewReader(file), exact); err != nil {
			t.Fatal("exact budgets", err)
		}
		for _, field := range []string{"frames", "samples", "bytes", "chunks", "work"} {
			o := exact
			switch field {
			case "frames":
				o.Limits.MaxFrames--
			case "samples":
				o.Limits.MaxSamples--
			case "bytes":
				o.Limits.MaxBytes--
			case "chunks":
				o.Limits.MaxChunks--
			case "work":
				o.MaxWork--
			}
			if p, err := ReadAIFF(bytes.NewReader(file), o); p != nil || !errors.Is(err, ErrLimit) {
				t.Fatal(field, err)
			}
		}
		for cut := range len(file) {
			sentinel := errors.New("read failed")
			r := io.MultiReader(bytes.NewReader(file[:cut]), aiffErrorReader{sentinel})
			if p, err := ReadAIFF(r, AIFFReadOptions{}); p != nil || !errors.Is(err, sentinel) {
				t.Fatal("underlying I/O chain", cut, err)
			}
		}
	}
	for _, l := range []Limits{{MaxFrames: -1}, {MaxSamples: -1}, {MaxBytes: -1}, {MaxChunks: -1}} {
		if _, err := ReadAIFF(bytes.NewReader(nil), AIFFReadOptions{Limits: l}); !errors.Is(err, ErrInvalidInput) {
			t.Fatal("negative budget", err)
		}
	}
	for _, o := range []AIFFReadOptions{{Limits: Limits{MaxBytes: 11}}, {MaxWork: 11}} {
		r := bytes.NewReader(aiffTestForm("AIFF", comm, ssnd))
		before := r.Len()
		if _, err := ReadAIFF(r, o); !errors.Is(err, ErrLimit) || r.Len() != before {
			t.Fatal("header budget must precede I/O", err)
		}
	}
	if _, err := ReadAIFF(nil, AIFFReadOptions{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatal("nil reader", err)
	}
	// Full-width declared frame count must fail storage checks before allocation.
	file := aiffTestForm("AIFF", aiffTestCommon(32, 64, 0, 48000, "", nil), ssnd)
	binary.BigEndian.PutUint32(file[22:26], 0xffffffff)
	if _, err := ReadAIFF(bytes.NewReader(file), AIFFReadOptions{}); !errors.Is(err, ErrLimit) {
		t.Fatal("declared huge frames", err)
	}
	// Huge deferred SSND, but no actual payload: bounded I/O precedes growth.
	header := []byte("FORM\x7f\xff\xff\xfeAIFFSSND\x7f\xff\xff\xf0\x00\x00\x00\x00\x00\x00\x00\x00")
	if _, err := ReadAIFF(bytes.NewReader(header), AIFFReadOptions{Limits: Limits{MaxBytes: 1 << 32}, MaxWork: 1 << 33}); !errors.Is(err, io.EOF) {
		t.Fatal("deferred truncated allocation", err)
	}
	// Long discarded text crosses scratch boundaries and still validates bytes.
	text := bytes.Repeat([]byte("a"), 9001)
	long := aiffTestForm("AIFF", aiffTestChunk("ANNO", text), comm, ssnd)
	if _, err := ReadAIFF(bytes.NewReader(long), AIFFReadOptions{DiscardMetadata: true}); err != nil {
		t.Fatal(err)
	}
	textError := errors.New("text read failed")
	if _, err := ReadAIFF(io.MultiReader(bytes.NewReader(long[:33]), aiffErrorReader{textError}), AIFFReadOptions{DiscardMetadata: true}); !errors.Is(err, textError) {
		t.Fatal("discarded-text I/O chain", err)
	}
	if _, err := ReadAIFF(bytes.NewReader(long), AIFFReadOptions{MaxWork: uint64(len(long) - 1), DiscardMetadata: true}); !errors.Is(err, ErrLimit) {
		t.Fatal("FORM work budget", err)
	}
	_, err := bufferAIFFSound(bytes.NewReader(nil), math.MaxInt64)
	if int(^uint(0)>>1) == math.MaxInt32 && !errors.Is(err, ErrLimit) || int(^uint(0)>>1) != math.MaxInt32 && !errors.Is(err, io.EOF) {
		t.Fatal("platform storage bound", err)
	}
}

type aiffErrorReader struct{ err error }

func (r aiffErrorReader) Read([]byte) (int, error) { return 0, r.err }

// Resolve independently of TestMain's cwd relocation and overlay package cwd.
func aiffTestFixtureDir(start string) (string, error) {
	for dir := start; ; dir = filepath.Dir(dir) {
		if info, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil && !info.IsDir() {
			return filepath.Join(dir, "tests", "data", "audio", "aiff"), nil
		}
		if filepath.Dir(dir) == dir {
			return "", fmt.Errorf("AIFF fixtures: cannot locate repository from %s", start)
		}
	}
}

func TestReadAIFFFixtureDirectory(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	want, err := aiffTestFixtureDir(cwd)
	if err != nil {
		t.Fatal(err)
	}
	root := want
	for range 4 {
		root = filepath.Dir(root)
	}
	for _, start := range []string{root, filepath.Join(root, "audio"), filepath.Join(root, "tests", "data", "audio")} {
		got, err := aiffTestFixtureDir(start)
		if err != nil || got != want {
			t.Fatal("fixture lookup depends on test cwd", start, got, err)
		}
		if _, err := os.Stat(filepath.Join(got, "manifest.json")); err != nil {
			t.Fatal(err)
		}
	}
}

func TestReadAIFFIndependentFixtures(t *testing.T) {
	dir := os.Getenv("GOMATH_AIFF_REFERENCE_DIR")
	if dir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		dir, err = aiffTestFixtureDir(cwd)
		if err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Cases []struct {
			File, EncodedSHA256, RawSHA256 string
			Unsupported                    string
			Rate, Depth, Channels, Frames  int
			Codes                          []int64
			DiscardMetadata                bool
		}
	}
	if err := json.Unmarshal(data, &manifest); err != nil || len(manifest.Cases) < 12 {
		t.Fatal("independent manifest", err)
	}
	for _, v := range manifest.Cases {
		t.Run(v.File, func(t *testing.T) {
			encoded, err := os.ReadFile(filepath.Join(dir, v.File))
			if err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256(encoded)
			if hex.EncodeToString(sum[:]) != v.EncodedSHA256 {
				t.Fatal("encoded hash")
			}
			if v.Unsupported != "" {
				for _, discard := range []bool{false, true} {
					if p, err := ReadAIFF(bytes.NewReader(encoded), AIFFReadOptions{DiscardMetadata: discard}); p != nil || !errors.Is(err, ErrUnsupported) {
						t.Fatal("reference contains unsupported semantics", v.Unsupported, err)
					}
				}
				return
			}
			if v.DiscardMetadata {
				if _, err := ReadAIFF(bytes.NewReader(encoded), AIFFReadOptions{}); !errors.Is(err, ErrUnsupported) {
					t.Fatal("fixture metadata default", err)
				}
			}
			p, err := ReadAIFF(bytes.NewReader(encoded), AIFFReadOptions{DiscardMetadata: v.DiscardMetadata})
			if err != nil || p.Format() != (Format{SampleRate: v.Rate, Channels: v.Channels}) || p.Frames() != v.Frames {
				t.Fatal("independent format", err)
			}
			if len(p.samples) != len(v.Codes) {
				t.Fatal("sample count")
			}
			raw := make([]byte, len(v.Codes)*4) // FFmpeg pcm_s32le, exact left alignment
			for i, code := range v.Codes {
				want := float64(code) / float64(uint64(1)<<(v.Depth-1))
				if p.samples[i] != want {
					t.Fatal("independent exact sample", i, p.samples[i], want)
				}
				binary.LittleEndian.PutUint32(raw[i*4:], uint32(int32(code<<uint(32-v.Depth))))
			}
			sum = sha256.Sum256(raw)
			if hex.EncodeToString(sum[:]) != v.RawSHA256 {
				t.Fatal("independent FFmpeg PCM digest")
			}
		})
	}
}

func TestReadAIFFConcurrentIndependentStorage(t *testing.T) {
	raw, want := aiffTestCodes(24, 3, 10923, false)
	file := aiffTestForm("AIFF", aiffTestSound(raw, 3, 5, 1024), aiffTestCommon(24, 3, 10923, 48000, "", nil))
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			p, err := ReadAIFF(bytes.NewReader(file), AIFFReadOptions{})
			if err != nil {
				t.Error(err)
				return
			}
			closeSamples(t, p.Samples(), want, 0)
			clear(p.samples)
		})
	}
	wg.Wait()
}

func FuzzReadAIFF(f *testing.F) {
	for _, compression := range []string{"", "NONE", "sowt"} {
		kind := "AIFF"
		chunks := [][]byte{aiffTestCommon(16, 1, 1, 48000, compression, nil), aiffTestSound([]byte{0x80, 0}, 1, 3, 512)}
		if compression != "" {
			kind = "AIFC"
			chunks = append(chunks, aiffTestVersion())
		}
		f.Add(aiffTestForm(kind, chunks...), false)
		f.Add(aiffTestForm(kind, append([][]byte{aiffTestChunk("ANNO", []byte("text"))}, chunks...)...), true)
	}
	f.Add([]byte("FORM"), true)
	f.Fuzz(func(t *testing.T, data []byte, discard bool) {
		p, err := ReadAIFF(bytes.NewReader(data), AIFFReadOptions{Limits: Limits{MaxBytes: 1 << 20, MaxFrames: 4096, MaxSamples: 8192, MaxChunks: 32}, MaxWork: 1 << 21, DiscardMetadata: discard})
		if err != nil {
			if p != nil {
				t.Fatal("partial PCM on error")
			}
			return
		}
		if p.Frames() > 4096 || len(p.samples) > 8192 || p.validate() != nil {
			t.Fatal("invalid successful PCM")
		}
		for _, v := range p.samples {
			if !finite(v) || v < -1 || v >= 1 {
				t.Fatal("invalid integer normalization")
			}
		}
	})
}

func BenchmarkReadAIFF(b *testing.B) {
	for _, depth := range []int{8, 16, 24, 32} {
		for _, reverse := range []bool{false, true} {
			b.Run(fmt.Sprintf("pcm%d/SSNDfirst%t", depth, reverse), func(b *testing.B) {
				raw, _ := aiffTestCodes(depth, 2, 48000, false)
				comm, ssnd := aiffTestCommon(depth, 2, 48000, 48000, "", nil), aiffTestSound(raw, 0, 0, 0)
				chunks := [][]byte{comm, ssnd}
				if reverse {
					chunks = [][]byte{ssnd, comm}
				}
				file := aiffTestForm("AIFF", chunks...)
				b.ReportAllocs()
				b.SetBytes(int64(len(raw)))
				b.ResetTimer()
				for range b.N {
					p, err := ReadAIFF(bytes.NewReader(file), AIFFReadOptions{})
					if err != nil || p.Frames() != 48000 {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
