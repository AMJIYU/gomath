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

func msTestFormat(ch, align, count int) []byte {
	b := make([]byte, 22+4*count)
	binary.LittleEndian.PutUint16(b, 2)
	binary.LittleEndian.PutUint16(b[2:], uint16(ch))
	binary.LittleEndian.PutUint32(b[4:], 48000)
	spb := 2 + (align-7*ch)*2/ch
	binary.LittleEndian.PutUint32(b[8:], uint32(48000*align/spb))
	binary.LittleEndian.PutUint16(b[12:], uint16(align))
	binary.LittleEndian.PutUint16(b[14:], 4)
	binary.LittleEndian.PutUint16(b[16:], uint16(4+4*count))
	binary.LittleEndian.PutUint16(b[18:], uint16(spb))
	binary.LittleEndian.PutUint16(b[20:], uint16(count))
	pairs := [][2]int16{{256, 0}, {512, -256}, {0, 0}, {192, 64}, {240, 0}, {460, -208}, {392, -232}}
	for i := range count {
		pair := [2]int16{32767, -32768}
		if i < 7 {
			pair = pairs[i]
		}
		binary.LittleEndian.PutUint16(b[22+4*i:], uint16(pair[0]))
		binary.LittleEndian.PutUint16(b[24+4*i:], uint16(pair[1]))
	}
	return b
}
func msTestBlock(ch, align int) []byte {
	b := make([]byte, align)
	for c := range ch {
		b[c] = byte(c)
		binary.LittleEndian.PutUint16(b[ch+2*c:], 16)
		binary.LittleEndian.PutUint16(b[3*ch+2*c:], uint16(1000+100*c))
		binary.LittleEndian.PutUint16(b[5*ch+2*c:], uint16(2000+100*c))
	}
	for i := 7 * ch; i < align; i++ {
		b[i] = []byte{0x01, 0xf0, 0x23, 0x0e}[i%4]
	}
	return b
}
func msTestWave(ch, align, blocks, fact int, after bool) []byte {
	f := riffChunk("fmt ", msTestFormat(ch, align, 7))
	data := bytes.Repeat(msTestBlock(ch, align), blocks)
	d := riffChunk("data", data)
	if after {
		return riff(f, d, g711TestFact(uint32(fact)))
	}
	return riff(f, g711TestFact(uint32(fact)), d)
}
func msFixtureDir(t testing.TB) string {
	t.Helper()
	if d := os.Getenv("GOMATH_MS_ADPCM_REFERENCE_DIR"); d != "" {
		return d
	}
	start, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for root := start; ; root = filepath.Dir(root) {
		if st, err := os.Stat(filepath.Join(root, "go.mod")); err == nil && !st.IsDir() {
			return filepath.Join(root, "tests", "data", "audio", "ms_adpcm")
		}
		if root == filepath.Dir(root) {
			t.Fatal("fixture root", start)
		}
	}
}
func msReject(t *testing.T, w []byte, o WAVReadOptions, kind error) {
	t.Helper()
	p, e := ReadWAV(bytes.NewReader(w), o)
	if p != nil || e == nil || (kind != nil && !errors.Is(e, kind)) {
		t.Fatalf("expected nil PCM/%v, got %v/%v", kind, p, e)
	}
}

func TestWAVMSHeaderNibbleInterleaveAndRounding(t *testing.T) {
	for _, ch := range []int{1, 2} {
		align := 7*ch + 2
		b := msTestBlock(ch, align)
		for c := range ch {
			b[c] = 0
		}
		copy(b[7*ch:], []byte{0x1f, 0x28})
		w := riff(riffChunk("fmt ", msTestFormat(ch, align, 7)), g711TestFact(uint32(2+4/ch)), riffChunk("data", b))
		p, e := ReadWAV(bytes.NewReader(w), WAVReadOptions{})
		if e != nil {
			t.Fatal(e)
		}
		want := []int{2000, 1000, 1016, 1000, 1032, 904}
		if ch == 2 {
			want = []int{2000, 2100, 1000, 1100, 1016, 1084, 1048, 956}
		}
		for i, v := range want {
			if p.samples[i] != float64(v)/32768 {
				t.Fatal(ch, i, p.samples[i], v)
			}
		}
	}
	// Negative fractional predictor is truncated toward zero before residual.
	b := msTestBlock(1, 8)
	b[0] = 3
	binary.LittleEndian.PutUint16(b[3:], 65535)
	binary.LittleEndian.PutUint16(b[5:], 0)
	b[7] = 0
	w := riff(riffChunk("fmt ", msTestFormat(1, 8, 7)), g711TestFact(4), riffChunk("data", b))
	p, e := ReadWAV(bytes.NewReader(w), WAVReadOptions{})
	if e != nil {
		t.Fatal(e)
	}
	closeSamples(t, p.samples, []float64{0, -1.0 / 32768, 0, 0}, 0)
	// Header delta is used literally for first nibble, then min16 applies.
	for _, delta := range []uint16{0, 1, 15, 16, 32767} {
		b := msTestBlock(1, 8)
		b[0] = 2
		binary.LittleEndian.PutUint16(b[1:], delta)
		b[7] = 0x11
		p, e := ReadWAV(bytes.NewReader(riff(riffChunk("fmt ", msTestFormat(1, 8, 7)), g711TestFact(4), riffChunk("data", b))), WAVReadOptions{})
		if e != nil {
			t.Fatal(e)
		}
		if p.samples[2] != float64(delta)/32768 || p.samples[3] != float64(max(16, int(delta)*230/256))/32768 {
			t.Fatal("literal/adaptive delta", delta, p.samples)
		}
	}
}

func TestWAVMSFactBlockAndHeaderOnly(t *testing.T) {
	for _, ch := range []int{1, 2} {
		for _, align := range []int{7 * ch, 7*ch + 1, 255, 256, 1024} {
			spb := 2 + (align-7*ch)*2/ch
			for _, blocks := range []int{0, 1, 2} {
				capacity := spb * blocks
				for fact := 0; fact <= capacity+1; fact++ {
					valid := fact == 0 && capacity == 0 || fact <= capacity && fact > capacity-spb
					for _, after := range []bool{false, true} {
						w := msTestWave(ch, align, blocks, fact, after)
						p, e := ReadWAV(bytes.NewReader(w), WAVReadOptions{})
						if !valid {
							if p != nil || !errors.Is(e, ErrInvalidInput) {
								t.Fatal("fact acceptance", ch, align, blocks, fact, e)
							}
							continue
						}
						if e != nil || p.Frames() != fact || p.Format() != (Format{48000, ch, 0}) {
							t.Fatal("fact/format", ch, align, blocks, fact, e)
						}
					}
				}
			}
		}
	}
	// Every block resets histories and adaptive state.
	p, e := ReadWAV(bytes.NewReader(msTestWave(2, 32, 2, 40, false)), WAVReadOptions{})
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(msPCMBytes(p.samples[:40]), msPCMBytes(p.samples[40:])) {
		t.Fatal("block reset")
	}
}
func msPCMBytes(samples []float64) []byte {
	b := make([]byte, 2*len(samples))
	for i, v := range samples {
		binary.LittleEndian.PutUint16(b[2*i:], uint16(int16(v*32768)))
	}
	return b
}

func TestWAVMSStrictFormatAndPredictors(t *testing.T) {
	base := msTestFormat(2, 32, 7)
	check := func(f []byte, kind error) {
		t.Helper()
		msReject(t, riff(riffChunk("fmt ", f), g711TestFact(20), riffChunk("data", msTestBlock(2, 32))), WAVReadOptions{}, kind)
	}
	for _, n := range []int{0, 16, 18, 20, 22, 40, 49, 51, 54, 1047, 1048} {
		b := make([]byte, n)
		copy(b, base)
		check(b, ErrUnsupported)
	}
	for _, field := range []struct {
		off    int
		values []uint16
		kind   error
	}{
		{0, []uint16{1, 3, 6, 7, 17, 0xfffe}, ErrUnsupported}, {2, []uint16{0, 3, 64, 65535}, ErrUnsupported},
		{12, []uint16{0, 13, 15, 31, 33, 65535}, ErrInvalidInput}, {14, []uint16{0, 3, 8, 16}, ErrUnsupported},
		{16, []uint16{0, 30, 31, 33, 65535}, ErrUnsupported}, {18, []uint16{0, 1, 19, 21, 65535}, ErrInvalidInput},
		{20, []uint16{0, 6, 8, 256, 257, 65535}, ErrUnsupported},
	} {
		for _, v := range field.values {
			b := bytes.Clone(base)
			binary.LittleEndian.PutUint16(b[field.off:], v)
			check(b, field.kind)
		}
	}
	for i := range 7 {
		b := bytes.Clone(base)
		b[22+4*i] ^= 1
		check(b, ErrInvalidInput)
	}
	for _, rate := range []uint32{0, MaxSampleRate + 1, math.MaxUint32} {
		b := bytes.Clone(base)
		binary.LittleEndian.PutUint32(b[4:], rate)
		kind := ErrUnsupported
		if rate == 0 {
			kind = ErrInvalidInput
		}
		check(b, kind)
	}
	b := bytes.Clone(base)
	clear(b[8:12])
	check(b, ErrInvalidInput)
	// Byte rate is advisory, not used as a data count. Preserve real encoders.
	b = bytes.Clone(base)
	binary.LittleEndian.PutUint32(b[8:], 1)
	if _, e := ReadWAV(bytes.NewReader(riff(riffChunk("fmt ", b), g711TestFact(20), riffChunk("data", msTestBlock(2, 32)))), WAVReadOptions{}); e != nil {
		t.Fatal(e)
	}
	for _, ch := range []int{1, 2} {
		for _, count := range []int{7, 8, 256} {
			align := 7*ch + 1
			data := msTestBlock(ch, align)
			for c := range ch {
				data[c] = byte(count - 1)
			}
			w := riff(riffChunk("fmt ", msTestFormat(ch, align, count)), g711TestFact(uint32(2+2/ch)), riffChunk("data", data))
			if _, e := ReadWAV(bytes.NewReader(w), WAVReadOptions{}); e != nil {
				t.Fatal("coefficient table limit", ch, count, e)
			}
			if count < 256 {
				data[0] = byte(count)
				msReject(t, riff(riffChunk("fmt ", msTestFormat(ch, align, count)), g711TestFact(1), riffChunk("data", data)), WAVReadOptions{}, ErrInvalidInput)
			}
		}
	}
	// Largest uint16 samplesPerBlock subset and exact rate boundaries.
	for _, ch := range []int{1, 2} {
		align := 32773
		if ch == 2 {
			align = 65535
		}
		spb := 2 + (align-7*ch)*2/ch
		for _, rate := range []uint32{1, MaxSampleRate} {
			f := msTestFormat(ch, align, 7)
			binary.LittleEndian.PutUint32(f[4:], rate)
			p, e := ReadWAV(bytes.NewReader(riff(riffChunk("fmt ", f), g711TestFact(uint32(spb)), riffChunk("data", msTestBlock(ch, align)))), WAVReadOptions{})
			if e != nil || p.Format().SampleRate != int(rate) {
				t.Fatal("format maxima", e)
			}
		}
	}
	f := msTestFormat(1, 32774, 7)
	msReject(t, riff(riffChunk("fmt ", f), g711TestFact(1), riffChunk("data", msTestBlock(1, 32774))), WAVReadOptions{}, ErrInvalidInput)
}

func TestWAVMSCustomCoefficientInt64Intermediate(t *testing.T) {
	f := msTestFormat(1, 8, 8)
	binary.LittleEndian.PutUint16(f[50:], 32768)
	binary.LittleEndian.PutUint16(f[52:], 32768)
	b := msTestBlock(1, 8)
	b[0] = 7
	binary.LittleEndian.PutUint16(b[3:], 32768)
	binary.LittleEndian.PutUint16(b[5:], 32768)
	b[7] = 0
	p, e := ReadWAV(bytes.NewReader(riff(riffChunk("fmt ", f), g711TestFact(4), riffChunk("data", b))), WAVReadOptions{})
	if e != nil {
		t.Fatal(e)
	}
	if p.samples[2] != 32767.0/32768 {
		t.Fatal("signed int32 intermediate overflow", p.samples)
	}
}

func TestWAVMSArithmeticAndPaddedTailValidation(t *testing.T) {
	for _, ch := range []int{1, 2} {
		align := 7*ch + 128
		data := msTestBlock(ch, align)
		for i := 7 * ch; i < len(data); i++ {
			data[i] = 0x88
		}
		for c := range ch {
			binary.LittleEndian.PutUint16(data[ch+2*c:], 32767)
		}
		// Even fact=1 cannot conceal unsafe adaptive arithmetic in discarded tail.
		for _, fact := range []uint32{1, uint32(2 + 256/ch)} {
			msReject(t, riff(riffChunk("fmt ", msTestFormat(ch, align, 7)), g711TestFact(fact), riffChunk("data", data)), WAVReadOptions{}, ErrLimit)
		}
		data = msTestBlock(ch, 7*ch+1)
		for _, delta := range []uint16{32768, 65535} {
			binary.LittleEndian.PutUint16(data[ch:], delta)
			msReject(t, riff(riffChunk("fmt ", msTestFormat(ch, len(data), 7)), g711TestFact(1), riffChunk("data", data)), WAVReadOptions{}, ErrUnsupported)
		}
		data = msTestBlock(ch, 7*ch+1)
		bad := bytes.Clone(data)
		bad[0] = 7
		msReject(t, riff(riffChunk("fmt ", msTestFormat(ch, len(data), 7)), g711TestFact(uint32(2+2/ch+1)), riffChunk("data", append(data, bad...))), WAVReadOptions{}, ErrInvalidInput)
	}
}

func TestWAVMSSecondChannelAdaptiveOverflow(t *testing.T) {
	data := msTestBlock(2, 270)
	binary.LittleEndian.PutUint16(data[4:], 32767)
	for i := 14; i < len(data); i++ {
		data[i] = 0x08
	}
	msReject(t, riff(riffChunk("fmt ", msTestFormat(2, len(data), 7)), g711TestFact(1), riffChunk("data", data)), WAVReadOptions{}, ErrLimit)
}

func TestWAVMSContainersLimitsAndIO(t *testing.T) {
	for _, ch := range []int{1, 2} {
		for _, after := range []bool{false, true} {
			align := 7*ch + 1
			spb := 2 + 2/ch
			w := msTestWave(ch, align, 2, spb+1, after)
			l := Limits{MaxFrames: 2 * spb, MaxSamples: 2 * spb * ch, MaxBytes: int64(len(w)), MaxChunks: 3}
			if _, e := ReadWAV(bytes.NewReader(w), WAVReadOptions{Limits: l}); e != nil {
				t.Fatal("exact budgets", e)
			}
			for _, l := range []Limits{{MaxFrames: 2*spb - 1}, {MaxSamples: 2*spb*ch - 1}, {MaxBytes: int64(len(w) - 1)}, {MaxChunks: 2}} {
				msReject(t, w, WAVReadOptions{Limits: l}, ErrLimit)
			}
			for end := 0; end < len(w); end++ {
				msReject(t, w[:end], WAVReadOptions{}, nil)
				p, e := ReadWAV(io.MultiReader(bytes.NewReader(w[:end]), g711ErrorReader{}), WAVReadOptions{})
				if p != nil || !errors.Is(e, io.ErrClosedPipe) {
					t.Fatal("IO cause", end, e)
				}
			}
			r := bytes.NewReader(append(bytes.Clone(w), []byte("NEXT")...))
			p, e := ReadWAV(oneByteReader{r}, WAVReadOptions{})
			if e != nil || p.Frames() != spb+1 || r.Len() != 4 {
				t.Fatal("stream boundary", e, r.Len())
			}
		}
	}
	f := riffChunk("fmt ", msTestFormat(1, 9, 7))
	fact := g711TestFact(6)
	d := riffChunk("data", msTestBlock(1, 9))
	for _, w := range [][]byte{riff(f, d), riff(f, fact, d, d), riff(f, fact, d, f), riff(f, fact, d, fact), riff(d, f, fact), riff(f, fact), riff(fact, d)} {
		msReject(t, w, WAVReadOptions{}, nil)
	}
	for _, n := range []int{0, 1, 3, 5, 8} {
		msReject(t, riff(f, riffChunk("fact", make([]byte, n)), d), WAVReadOptions{}, ErrUnsupported)
	}
	// fact may precede fmt; physical partial blocks are never synthesized.
	if _, e := ReadWAV(bytes.NewReader(riff(fact, f, d)), WAVReadOptions{}); e != nil {
		t.Fatal(e)
	}
	for n := 1; n < 9; n++ {
		msReject(t, riff(f, fact, riffChunk("data", make([]byte, n))), WAVReadOptions{}, ErrUnsupported)
	}
	for _, id := range []string{"smpl", "wavl", "cue ", "slnt", "what"} {
		msReject(t, riff(f, fact, d, riffChunk(id, nil)), WAVReadOptions{DiscardMetadata: true}, ErrUnsupported)
	}
	for _, id := range []string{"LIST", "bext", "iXML", "axml", "ID3 ", "id3 "} {
		payload := []byte{1, 2, 3}
		if id == "LIST" {
			payload = []byte("INFO")
		}
		w := riff(f, fact, d, riffChunk(id, payload))
		msReject(t, w, WAVReadOptions{}, ErrUnsupported)
		if _, e := ReadWAV(bytes.NewReader(w), WAVReadOptions{DiscardMetadata: true}); e != nil {
			t.Fatal("metadata opt-in", id, e)
		}
	}
	for _, id := range []string{"JUNK", "PAD ", "data"} {
		for _, before := range []bool{false, true} {
			pad := riffChunk(id, []byte{1})
			pad[len(pad)-1] = 1
			var w []byte
			if id == "data" {
				pad = bytes.Clone(d)
				pad[len(pad)-1] = 1
				w = riff(f, fact, pad)
			} else if before {
				w = riff(pad, f, fact, d)
			} else {
				w = riff(f, fact, d, pad)
			}
			msReject(t, w, WAVReadOptions{}, ErrInvalidInput)
		}
	}
	w := riff(f, fact, d)
	w = w[:len(w)-1]
	binary.LittleEndian.PutUint32(w[4:], uint32(len(w)-8))
	msReject(t, w, WAVReadOptions{}, ErrInvalidInput)
	// Coded capacity is checked before any block read/allocation, even fact=1.
	w = riff(f, g711TestFact(1), riffChunk("data", nil))
	binary.LittleEndian.PutUint32(w[len(w)-4:], 900000000)
	binary.LittleEndian.PutUint32(w[4:], uint32(len(w)-8)+900000000)
	r := bytes.NewReader(w)
	p, e := ReadWAV(r, WAVReadOptions{Limits: Limits{MaxFrames: 1, MaxBytes: 1 << 33}})
	if p != nil || !errors.Is(e, ErrLimit) || r.Len() != 0 {
		t.Fatal("capacity preflight", e, r.Len())
	}
}

type msReferenceCase struct {
	File, PCMFile, EncodedSHA256, PCMSHA256 string
	Channels, Rate, Frames, Capacity        int
	DiscardMetadata, Benchmark              bool
}

func msManifest(t testing.TB) ([]msReferenceCase, []struct{ File, EncodedSHA256, Category string }) {
	t.Helper()
	b, e := os.ReadFile(filepath.Join(msFixtureDir(t), "manifest.json"))
	if e != nil {
		t.Fatal(e)
	}
	var m struct {
		Cases         []msReferenceCase
		RejectedCases []struct{ File, EncodedSHA256, Category string }
	}
	if e = json.Unmarshal(b, &m); e != nil || len(m.Cases) != 50 || len(m.RejectedCases) != 3 {
		t.Fatal("reference manifest", e)
	}
	return m.Cases, m.RejectedCases
}
func msReadFixture(t testing.TB, name, hash string) []byte {
	t.Helper()
	b, e := os.ReadFile(filepath.Join(msFixtureDir(t), name))
	if e != nil {
		t.Fatal(e)
	}
	h := sha256.Sum256(b)
	if hex.EncodeToString(h[:]) != hash {
		t.Fatal("fixture hash", name)
	}
	return b
}
func TestWAVMSIndependentFixtures(t *testing.T) {
	cases, rejected := msManifest(t)
	for _, c := range cases {
		t.Run(c.File, func(t *testing.T) {
			w := msReadFixture(t, c.File, c.EncodedSHA256)
			expected := msReadFixture(t, c.PCMFile, c.PCMSHA256)
			p, e := ReadWAV(bytes.NewReader(w), WAVReadOptions{DiscardMetadata: c.DiscardMetadata})
			if e != nil || p.Format() != (Format{c.Rate, c.Channels, 0}) || p.Frames() != c.Frames || !bytes.Equal(msPCMBytes(p.samples), expected) {
				t.Fatal("normative complete decode", c.File, e)
			}
			for i, v := range p.samples {
				want := float64(int16(binary.LittleEndian.Uint16(expected[2*i:]))) / 32768
				if math.Float64bits(v) != math.Float64bits(want) {
					t.Fatal("float normalization", i, v, want)
				}
			}
			if c.DiscardMetadata {
				msReject(t, w, WAVReadOptions{}, ErrUnsupported)
			}
		})
	}
	for _, c := range rejected {
		kind := ErrInvalidInput
		if c.Category == "unsupported" {
			kind = ErrUnsupported
		}
		msReject(t, msReadFixture(t, c.File, c.EncodedSHA256), WAVReadOptions{DiscardMetadata: true}, kind)
	}
}
func TestWAVMSConcurrentOwnership(t *testing.T) {
	w := msTestWave(2, 256, 4, 974, false)
	p, e := ReadWAV(bytes.NewReader(w), WAVReadOptions{})
	if e != nil {
		t.Fatal(e)
	}
	expected := p.Samples()
	p.Samples()[0] = 0
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, e := ReadWAV(bytes.NewReader(w), WAVReadOptions{})
			if e != nil {
				t.Error(e)
				return
			}
			for i, v := range got.samples {
				if v != expected[i] {
					t.Error("concurrent", i)
					return
				}
			}
			got.samples[0] = 0
		}()
	}
	wg.Wait()
	clear(w)
	if p.samples[0] != expected[0] {
		t.Fatal("alias")
	}
}
func FuzzWAVMS(f *testing.F) {
	for _, ch := range []int{1, 2} {
		for _, a := range []int{7 * ch, 7*ch + 1, 256} {
			f.Add(msTestWave(ch, a, 1, 1, false))
		}
	}
	cases, _ := msManifest(f)
	for _, c := range cases {
		if c.Capacity < 4096 {
			f.Add(msReadFixture(f, c.File, c.EncodedSHA256))
		}
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		l := Limits{MaxBytes: 1 << 16, MaxFrames: 4096, MaxSamples: 8192, MaxChunks: 16}
		p, e := ReadWAV(bytes.NewReader(b), WAVReadOptions{Limits: l, DiscardMetadata: true})
		if e != nil {
			if p != nil {
				t.Fatal("partial PCM")
			}
			return
		}
		if p.validate() != nil || p.Frames() > l.MaxFrames || len(p.samples) > l.MaxSamples {
			t.Fatal("invalid output")
		}
	})
}
func BenchmarkReadWAVMS(b *testing.B) {
	cases, _ := msManifest(b)
	for _, c := range cases {
		if !c.Benchmark {
			continue
		}
		w := msReadFixture(b, c.File, c.EncodedSHA256)
		b.Run(fmt.Sprintf("%s", c.File), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(w)))
			b.ResetTimer()
			for range b.N {
				p, e := ReadWAV(bytes.NewReader(w), WAVReadOptions{DiscardMetadata: c.DiscardMetadata})
				if e != nil || p.Frames() != 48000 {
					b.Fatal(e)
				}
			}
		})
	}
}
