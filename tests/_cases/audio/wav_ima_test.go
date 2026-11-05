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

func imaTestFormat(channels, align, rate int) []byte {
	b := make([]byte, 20)
	spb := 1 + (align-4*channels)*2/channels
	binary.LittleEndian.PutUint16(b, 17)
	binary.LittleEndian.PutUint16(b[2:], uint16(channels))
	binary.LittleEndian.PutUint32(b[4:], uint32(rate))
	binary.LittleEndian.PutUint32(b[8:], uint32(rate*align/spb))
	binary.LittleEndian.PutUint16(b[12:], uint16(align))
	binary.LittleEndian.PutUint16(b[14:], 4)
	binary.LittleEndian.PutUint16(b[16:], 2)
	binary.LittleEndian.PutUint16(b[18:], uint16(spb))
	return b
}

func imaTestFact(frames int) []byte {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, uint32(frames))
	return riffChunk("fact", b)
}

func imaTestData(channels, align, blocks int) []byte {
	data := make([]byte, align*blocks)
	for b := 0; b < blocks; b++ {
		for c := 0; c < channels; c++ {
			start := b*align + c*4
			binary.LittleEndian.PutUint16(data[start:], uint16(1000+100*c+b))
		}
		for j := channels * 4; j < align; j++ {
			data[b*align+j] = 0x11
		}
	}
	return data
}

func imaTestWave(channels, align, blocks, frames int, factAfter bool) []byte {
	fmtChunk := riffChunk("fmt ", imaTestFormat(channels, align, 22050))
	data := riffChunk("data", imaTestData(channels, align, blocks))
	if factAfter {
		return riff(fmtChunk, data, imaTestFact(frames))
	}
	return riff(fmtChunk, imaTestFact(frames), data)
}

func TestWAVIMASpecRoundingInterleaveAndSaturation(t *testing.T) {
	// At step=7, code=1 adds (7>>2)+(7>>3)=1, not (3*7)>>3=2.
	for _, channels := range []int{1, 2} {
		align := 12 * channels
		data := imaTestData(channels, align, 2)
		for block := 0; block < 2; block++ {
			for ch := 0; ch < channels; ch++ {
				pred := int16(1000 + 100*ch + block)
				if ch == 1 {
					pred = -pred
				}
				binary.LittleEndian.PutUint16(data[block*align+4*ch:], uint16(pred))
				for group := 4 * channels; group < align; group += 4 * channels {
					for j := 0; j < 4; j++ {
						if ch == 1 {
							data[block*align+group+4*ch+j] = 0x99
						}
					}
				}
			}
		}
		wave := riff(riffChunk("fmt ", imaTestFormat(channels, align, 8000)), imaTestFact(34), riffChunk("data", data))
		p, err := ReadWAV(bytes.NewReader(wave), WAVReadOptions{})
		if err != nil || p.Format() != (Format{8000, channels, 0}) || p.Frames() != 34 {
			t.Fatal(p, err)
		}
		for frame := 0; frame < 34; frame++ {
			for ch := 0; ch < channels; ch++ {
				want := float64(1000+100*ch+frame/17+frame%17) / 32768
				if ch == 1 {
					want = -want
				}
				if got := p.samples[frame*channels+ch]; got != want {
					t.Fatal("step rounding/channel grouping/block reset", frame, ch, got, want)
				}
			}
		}
	}
	// Largest legal step must saturate both signs, without wrapping int16.
	data := []byte{0, 0, 88, 0, 0x77, 0xff, 0x77, 0xff}
	p, err := ReadWAV(bytes.NewReader(riff(riffChunk("fmt ", imaTestFormat(1, 8, 8000)), imaTestFact(9), riffChunk("data", data))), WAVReadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for i, got := range p.samples {
		want := float64([]int{0, 32767, 32767, -28669, -32768, 28668, 32767, -28669, -32768}[i]) / 32768
		if got != want {
			t.Fatal("signed saturation", i, got, want)
		}
	}
	// Nibble order is low then high, not one channel per nibble.
	data = []byte{0, 0, 0, 0, 0x91, 0x91, 0x91, 0x91}
	p, err = ReadWAV(bytes.NewReader(riff(riffChunk("fmt ", imaTestFormat(1, 8, 8000)), imaTestFact(9), riffChunk("data", data))), WAVReadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for i, got := range p.samples {
		if got != float64(i%2)/32768 {
			t.Fatal("nibble order", i, got)
		}
	}
}

func TestWAVIMAFactTailAndEmpty(t *testing.T) {
	for _, ch := range []int{1, 2} {
		for _, align := range []int{4 * ch, 8 * ch, 12 * ch} {
			spb := 1 + (align-4*ch)*2/ch
			for _, blocks := range []int{0, 1, 2, 3} {
				for _, after := range []bool{false, true} {
					for frames := 0; frames <= blocks*spb+1; frames++ {
						valid := (blocks == 0 && frames == 0) || (frames > (blocks-1)*spb && frames <= blocks*spb)
						p, err := ReadWAV(bytes.NewReader(imaTestWave(ch, align, blocks, frames, after)), WAVReadOptions{})
						if !valid {
							if p != nil || !errors.Is(err, ErrInvalidInput) {
								t.Fatal("invalid fact tail", ch, align, blocks, frames, err)
							}
						} else if err != nil || p.Frames() != frames {
							t.Fatal("valid fact tail", ch, align, blocks, frames, err)
						}
					}
				}
			}
		}
	}
	// Fact can precede fmt without altering parsing/timing semantics.
	wave := riff(imaTestFact(1), riffChunk("fmt ", imaTestFormat(1, 8, 8000)), riffChunk("data", imaTestData(1, 8, 1)))
	p, err := ReadWAV(bytes.NewReader(wave), WAVReadOptions{})
	if err != nil || p.Frames() != 1 || p.samples[0] != 1000.0/32768 {
		t.Fatal("fact before fmt", err)
	}
}

func TestWAVIMAStrictFormats(t *testing.T) {
	base := imaTestFormat(2, 16, 22050)
	check := func(name string, format []byte, category error) {
		t.Helper()
		p, err := ReadWAV(bytes.NewReader(riff(riffChunk("fmt ", format), imaTestFact(9), riffChunk("data", imaTestData(2, 16, 1)))), WAVReadOptions{})
		if p != nil || !errors.Is(err, category) {
			t.Fatal(name, "format accepted or wrong error", err)
		}
	}
	for _, n := range []int{0, 14, 16, 17, 18, 19, 21, 22, 40, 42} {
		b := make([]byte, n)
		copy(b, base)
		check(fmt.Sprint("length", n), b, ErrUnsupported)
	}
	for _, field := range []struct {
		offset int
		values []uint16
		kind   error
	}{
		{0, []uint16{2, 0x20, 0x69, 0xfffe}, ErrUnsupported},
		{2, []uint16{0, 3, 64}, ErrUnsupported},
		{12, []uint16{0, 1, 4, 7, 9, 15, 17, 65535}, ErrInvalidInput},
		{14, []uint16{0, 3, 5, 8, 16}, ErrUnsupported},
		{16, []uint16{0, 1, 3, 22}, ErrUnsupported},
		{18, []uint16{0, 1, 8, 10, 65535}, ErrInvalidInput},
	} {
		for _, value := range field.values {
			b := bytes.Clone(base)
			binary.LittleEndian.PutUint16(b[field.offset:], value)
			check(fmt.Sprint(field.offset, value), b, field.kind)
		}
	}
	for _, field := range []struct {
		offset int
		value  uint32
		kind   error
	}{{4, 0, ErrInvalidInput}, {4, MaxSampleRate + 1, ErrUnsupported}, {8, 0, ErrInvalidInput}} {
		b := bytes.Clone(base)
		binary.LittleEndian.PutUint32(b[field.offset:], field.value)
		check("rate", b, field.kind)
	}
	// Positive compressed average rates are advisory; never infer samples
	// from this field (FFmpeg's encoder can write its default target bitrate).
	for _, avg := range []uint32{1, 16000, math.MaxUint32} {
		b := bytes.Clone(base)
		binary.LittleEndian.PutUint32(b[8:], avg)
		if _, err := ReadWAV(bytes.NewReader(riff(riffChunk("fmt ", b), imaTestFact(9), riffChunk("data", imaTestData(2, 16, 1)))), WAVReadOptions{}); err != nil {
			t.Fatal("advisory average rate", avg, err)
		}
	}
	// Extensible GUID aliases are deliberately unsupported for compressed IMA.
	b := make([]byte, 40)
	copy(b, base)
	binary.LittleEndian.PutUint16(b, 0xfffe)
	binary.LittleEndian.PutUint16(b[16:], 22)
	binary.LittleEndian.PutUint16(b[18:], 4)
	binary.LittleEndian.PutUint16(b[24:], 17)
	copy(b[26:], []byte{0, 0, 0, 0, 0x10, 0, 0x80, 0, 0, 0xaa, 0, 0x38, 0x9b, 0x71})
	check("extensible IMA", b, ErrUnsupported)
}

func TestWAVIMAStrictBlocksAndChunks(t *testing.T) {
	fmtChunk := riffChunk("fmt ", imaTestFormat(2, 16, 8000))
	validData := imaTestData(2, 16, 2)
	for block := 0; block < 2; block++ {
		for channel := 0; channel < 2; channel++ {
			for _, field := range []struct{ offset, value int }{{2, 89}, {2, 255}, {3, 1}, {3, 255}} {
				data := bytes.Clone(validData)
				data[block*16+4*channel+field.offset] = byte(field.value)
				// Only the header sample of the final block is returned; bad
				// headers beyond the previous block must still be detected.
				wave := riff(fmtChunk, imaTestFact(10), riffChunk("data", data))
				if p, err := ReadWAV(bytes.NewReader(wave), WAVReadOptions{}); p != nil || !errors.Is(err, ErrInvalidInput) {
					t.Fatal("header index/reserved accepted", block, channel, field, err)
				}
			}
		}
	}
	for _, n := range []int{1, 4, 7, 8, 9, 12, 15, 17, 24, 31} {
		wave := riff(fmtChunk, imaTestFact(10), riffChunk("data", validData[:n]))
		if p, err := ReadWAV(bytes.NewReader(wave), WAVReadOptions{}); p != nil || !errors.Is(err, ErrUnsupported) {
			t.Fatal("partial block guessed", n, err)
		}
	}
	dataChunk := riffChunk("data", validData)
	for _, wave := range [][]byte{
		riff(fmtChunk, dataChunk), riff(fmtChunk, riffChunk("data", nil)),
		riff(fmtChunk, imaTestFact(10)), riff(imaTestFact(10), dataChunk, fmtChunk),
		riff(fmtChunk, imaTestFact(10), dataChunk, dataChunk),
		riff(fmtChunk, imaTestFact(10), dataChunk, fmtChunk),
		riff(fmtChunk, imaTestFact(10), dataChunk, imaTestFact(10)),
		riff(fmtChunk, riffChunk("fact", []byte{10, 0, 0}), dataChunk),
		riff(fmtChunk, riffChunk("fact", make([]byte, 8)), dataChunk),
		riff(fmtChunk, imaTestFact(10), dataChunk, riffChunk("smpl", nil)),
		riff(fmtChunk, imaTestFact(10), dataChunk, riffChunk("LIST", []byte("wavl"))),
		riff(fmtChunk, imaTestFact(10), dataChunk, riffChunk("unknown", nil)),
	} {
		if p, err := ReadWAV(bytes.NewReader(wave), WAVReadOptions{DiscardMetadata: true}); err == nil || p != nil {
			t.Fatal("missing/duplicate/semantic chunk accepted", wave, err)
		}
	}
	for _, metadata := range [][]byte{riffChunk("LIST", []byte("INFO")), riffChunk("iXML", []byte("xml"))} {
		wave := riff(fmtChunk, imaTestFact(10), dataChunk, metadata)
		if _, err := ReadWAV(bytes.NewReader(wave), WAVReadOptions{}); !errors.Is(err, ErrUnsupported) {
			t.Fatal("metadata default changed", err)
		}
		if _, err := ReadWAV(bytes.NewReader(wave), WAVReadOptions{DiscardMetadata: true}); err != nil {
			t.Fatal("metadata opt-in", err)
		}
	}
}

func TestWAVIMALimitsAndCompleteStream(t *testing.T) {
	for _, factAfter := range []bool{false, true} {
		wave := imaTestWave(2, 16, 2, 10, factAfter)
		exact := Limits{MaxFrames: 18, MaxSamples: 36, MaxBytes: int64(len(wave)), MaxChunks: 3}
		p, err := ReadWAV(bytes.NewReader(wave), WAVReadOptions{Limits: exact})
		if err != nil || p.Frames() != 10 {
			t.Fatal("exact coded capacity limit", err)
		}
		for _, limits := range []Limits{
			{MaxFrames: 17}, {MaxFrames: 10}, {MaxSamples: 35}, {MaxSamples: 20},
			{MaxBytes: int64(len(wave) - 1)}, {MaxChunks: 2},
		} {
			if p, err := ReadWAV(bytes.NewReader(wave), WAVReadOptions{Limits: limits}); p != nil || !errors.Is(err, ErrLimit) {
				t.Fatal("coded storage/work bound ignored", limits, err)
			}
		}
		for end := 0; end < len(wave); end++ {
			if p, err := ReadWAV(bytes.NewReader(wave[:end]), WAVReadOptions{}); p != nil || err == nil {
				t.Fatal("truncation accepted", end)
			}
		}
		for offset := 0; offset < len(wave); offset++ {
			r := io.MultiReader(bytes.NewReader(wave[:offset]), imaErrorReader{})
			if p, err := ReadWAV(r, WAVReadOptions{}); p != nil || !errors.Is(err, io.ErrClosedPipe) {
				t.Fatal("underlying I/O error lost", offset, err)
			}
		}
		reader := bytes.NewReader(append(bytes.Clone(wave), []byte("NEXT")...))
		if p, err := ReadWAV(oneByteReader{reader}, WAVReadOptions{}); err != nil || p.Frames() != 10 || reader.Len() != 4 {
			t.Fatal("stream read boundary", err, reader.Len())
		}
	}
	// With a huge declared data size, reject decoded capacity before reading
	// payload or allocating output; the RIFF byte limit is explicitly raised.
	wave := imaTestWave(2, 16, 0, 0, false)
	binary.LittleEndian.PutUint32(wave[4:], math.MaxUint32-7)
	binary.LittleEndian.PutUint32(wave[len(wave)-4:], math.MaxUint32-63)
	r := bytes.NewReader(wave)
	if p, err := ReadWAV(r, WAVReadOptions{Limits: Limits{MaxBytes: 1 << 33, MaxFrames: 1}}); p != nil || !errors.Is(err, ErrLimit) || r.Len() != 0 {
		t.Fatal("oversized coded capacity preflight", err, r.Len())
	}
}

type imaErrorReader struct{}

func (imaErrorReader) Read([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestWAVIMAIndependentFixtures(t *testing.T) {
	dir := os.Getenv("GOMATH_IMA_REFERENCE_DIR")
	if dir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		for root := cwd; ; root = filepath.Dir(root) {
			if st, err := os.Stat(filepath.Join(root, "go.mod")); err == nil && !st.IsDir() {
				dir = filepath.Join(root, "tests", "data", "audio", "ima_adpcm")
				break
			}
			if filepath.Dir(root) == root {
				t.Fatal("cannot find module root", cwd)
			}
		}
	}
	manifestData, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Cases []struct {
			File, PCMFile, EncodedSHA256, PCMSHA256 string
			Rate, Channels, Frames                  int
			DiscardMetadata                         bool
		}
	}
	if err := json.Unmarshal(manifestData, &manifest); err != nil || len(manifest.Cases) != 37 {
		t.Fatal("independent reference manifest", err, len(manifest.Cases))
	}
	for _, c := range manifest.Cases {
		t.Run(c.File, func(t *testing.T) {
			encoded, err := os.ReadFile(filepath.Join(dir, c.File))
			if err != nil {
				t.Fatal(err)
			}
			expected, err := os.ReadFile(filepath.Join(dir, c.PCMFile))
			if err != nil {
				t.Fatal(err)
			}
			for _, pair := range []struct {
				data []byte
				hash string
			}{{encoded, c.EncodedSHA256}, {expected, c.PCMSHA256}} {
				hash := sha256.Sum256(pair.data)
				if hex.EncodeToString(hash[:]) != pair.hash {
					t.Fatal("independent fixture hash changed")
				}
			}
			p, err := ReadWAV(bytes.NewReader(encoded), WAVReadOptions{DiscardMetadata: c.DiscardMetadata})
			if err != nil {
				t.Fatal(err)
			}
			if p.Format() != (Format{c.Rate, c.Channels, 0}) || p.Frames() != c.Frames || len(expected) != len(p.samples)*2 {
				t.Fatal("reference dimensions", p.Format(), p.Frames(), len(expected))
			}
			for i, got := range p.samples {
				want := float64(int16(binary.LittleEndian.Uint16(expected[2*i:]))) / 32768
				if got != want {
					t.Fatal("independent PCM sample", i, got, want)
				}
			}
		})
	}
}

func TestWAVIMAOwnershipAndConcurrentDecode(t *testing.T) {
	wave := imaTestWave(2, 16, 2, 10, true)
	p, err := ReadWAV(bytes.NewReader(wave), WAVReadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	want := p.Samples()
	want[0] = 0
	if p.samples[0] == 0 {
		t.Fatal("Samples aliased decoded storage")
	}
	want = p.Samples()
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 16 {
				got, err := ReadWAV(bytes.NewReader(wave), WAVReadOptions{})
				if err != nil {
					t.Error(err)
					return
				}
				for i, v := range got.samples {
					if v != want[i] {
						t.Error("concurrent sample mismatch", i)
						return
					}
				}
				got.samples[0] = 0 // each decode owns its output
			}
		}()
	}
	wg.Wait()
	if p.samples[0] != want[0] {
		t.Fatal("different decodes share output storage")
	}
	clear(wave)
	if p.samples[0] != want[0] {
		t.Fatal("input aliases output")
	}
}

func FuzzWAVIMA(f *testing.F) {
	for _, channels := range []int{1, 2} {
		for _, align := range []int{4 * channels, 8 * channels, 256} {
			spb := 1 + (align-4*channels)*2/channels
			f.Add(imaTestWave(channels, align, 2, spb+1, false))
		}
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		limits := Limits{MaxBytes: 1 << 16, MaxFrames: 4096, MaxSamples: 8192, MaxChunks: 16}
		p, err := ReadWAV(bytes.NewReader(b), WAVReadOptions{Limits: limits})
		if err != nil {
			if p != nil {
				t.Fatal("partial output on error")
			}
			return
		}
		if p.Frames() > limits.MaxFrames || len(p.samples) > limits.MaxSamples || p.validate() != nil {
			t.Fatal("invalid bounded output")
		}
	})
}

func BenchmarkReadWAVIMA(b *testing.B) {
	for _, ch := range []int{1, 2} {
		for _, align := range []int{256, 1024} {
			spb := 1 + (align-4*ch)*2/ch
			blocks := (48000 + spb - 1) / spb
			encoded := imaTestData(ch, align, blocks)
			seed := uint32(17)
			for block := 0; block < blocks; block++ {
				for channel := 0; channel < ch; channel++ {
					encoded[block*align+4*channel+2] = byte((block*13 + channel*37) % 89)
				}
				for j := 4 * ch; j < align; j++ {
					seed = seed*1664525 + 1013904223
					encoded[block*align+j] = byte(seed >> 24)
				}
			}
			data := riff(riffChunk("fmt ", imaTestFormat(ch, align, 48000)), imaTestFact(48000), riffChunk("data", encoded))
			b.Run(fmt.Sprintf("ch%d_block%d", ch, align), func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(len(data)))
				b.ResetTimer()
				for range b.N {
					p, err := ReadWAV(bytes.NewReader(data), WAVReadOptions{})
					if err != nil || p.Frames() != 48000 {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
