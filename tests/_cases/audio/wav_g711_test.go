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

func g711TestFormat(tag uint16, channels, rate int) []byte {
	b := append(fixtureFormat(tag, 8, channels, rate), 0, 0)
	return b
}

func g711TestFact(frames uint32) []byte {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], frames)
	return riffChunk("fact", b[:])
}

func g711TestData(channels, frames int) []byte {
	b := make([]byte, channels*frames)
	for frame := range frames {
		for ch := range channels {
			b[frame*channels+ch] = byte(frame + 37*ch)
		}
	}
	return b
}

func g711TestWave(tag uint16, channels, frames int, factAfter bool) []byte {
	f := riffChunk("fmt ", g711TestFormat(tag, channels, 8000))
	d := riffChunk("data", g711TestData(channels, frames))
	if factAfter {
		return riff(f, d, g711TestFact(uint32(frames)))
	}
	return riff(f, g711TestFact(uint32(frames)), d)
}

func g711TestFixtureDir(t testing.TB) string {
	t.Helper()
	if dir := os.Getenv("GOMATH_G711_REFERENCE_DIR"); dir != "" {
		return dir
	}
	start, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for root := start; ; root = filepath.Dir(root) {
		if st, err := os.Stat(filepath.Join(root, "go.mod")); err == nil && !st.IsDir() {
			return filepath.Join(root, "tests", "data", "audio", "g711")
		}
		if filepath.Dir(root) == root {
			t.Fatal("cannot locate G.711 fixtures from", start)
		}
	}
}

func TestWAVG711AllCodesAndSignedExtremes(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(g711TestFixtureDir(t), "code_tables.json"))
	if err != nil {
		t.Fatal(err)
	}
	var tables struct{ ALaw, MuLaw []int }
	if err := json.Unmarshal(b, &tables); err != nil || len(tables.ALaw) != 256 || len(tables.MuLaw) != 256 {
		t.Fatal("independent midpoint tables", err)
	}
	for _, item := range []struct {
		tag   uint16
		codes []int
	}{{6, tables.ALaw}, {7, tables.MuLaw}} {
		for _, channels := range []int{1, 2, 3, 6, 64} {
			p, err := ReadWAV(bytes.NewReader(g711TestWave(item.tag, channels, 256, false)), WAVReadOptions{})
			if err != nil || p.Format() != (Format{8000, channels, 0}) || p.Frames() != 256 {
				t.Fatal("indexed channel format", item.tag, channels, err)
			}
			for frame := range 256 {
				for ch := range channels {
					code := byte(frame + 37*ch)
					want := float64(item.codes[code]) / 32768
					got := p.samples[frame*channels+ch]
					if math.Float64bits(got) != math.Float64bits(want) {
						t.Fatal("ITU full code mapping", item.tag, channels, frame, ch, code, got, want)
					}
				}
			}
		}
	}
	for _, item := range []struct {
		tag    uint16
		code   byte
		sample int16
	}{{6, 0xd5, 8}, {6, 0x55, -8}, {6, 0xaa, 32256}, {6, 0x2a, -32256},
		{6, 0x00, -5504}, {6, 0x80, 5504}, {7, 0xff, 0}, {7, 0x7f, 0},
		{7, 0x00, -32124}, {7, 0x80, 32124}, {7, 0xfe, 8}, {7, 0x7e, -8}} {
		wave := riff(riffChunk("fmt ", g711TestFormat(item.tag, 1, 8000)), g711TestFact(1), riffChunk("data", []byte{item.code}))
		p, err := ReadWAV(bytes.NewReader(wave), WAVReadOptions{})
		if err != nil || math.Float64bits(p.samples[0]) != math.Float64bits(float64(item.sample)/32768) {
			t.Fatal("zero/sign/extreme mapping", item, err)
		}
	}
}

func TestWAVG711StrictFactAndFramePolicy(t *testing.T) {
	for _, tag := range []uint16{6, 7} {
		for _, ch := range []int{1, 2, 3, 6, 64} {
			for _, frames := range []int{0, 1, 17} {
				f := riffChunk("fmt ", g711TestFormat(tag, ch, 8000))
				d := riffChunk("data", g711TestData(ch, frames))
				for _, after := range []bool{false, true} {
					p, err := ReadWAV(bytes.NewReader(g711TestWave(tag, ch, frames, after)), WAVReadOptions{})
					if err != nil || p.Frames() != frames {
						t.Fatal("exact fact/empty", tag, ch, frames, err)
					}
				}
				p, err := ReadWAV(bytes.NewReader(riff(g711TestFact(uint32(frames)), f, d)), WAVReadOptions{})
				if err != nil || p.Frames() != frames {
					t.Fatal("fact before fmt", err)
				}
				if p, err := ReadWAV(bytes.NewReader(riff(f, d)), WAVReadOptions{}); p != nil || !errors.Is(err, ErrInvalidInput) {
					t.Fatal("non-PCM fact required even for empty data", tag, err)
				}
				for _, count := range []uint32{0, 1, 16, 18, math.MaxUint32} {
					if count == uint32(frames) {
						continue
					}
					for _, wave := range [][]byte{riff(f, g711TestFact(count), d), riff(f, d, g711TestFact(count))} {
						if p, err := ReadWAV(bytes.NewReader(wave), WAVReadOptions{}); p != nil || !errors.Is(err, ErrInvalidInput) {
							t.Fatal("fact cannot trim/insert/hide codes", tag, ch, frames, count, err)
						}
					}
				}
				if ch > 1 {
					wave := riff(f, g711TestFact(1), riffChunk("data", make([]byte, ch+1)))
					if p, err := ReadWAV(bytes.NewReader(wave), WAVReadOptions{}); p != nil || !errors.Is(err, ErrInvalidInput) {
						t.Fatal("pad/fact cannot repair partial channel frame", tag, ch, err)
					}
				}
			}
		}
	}
}

func TestWAVG711StrictFormat(t *testing.T) {
	for _, tag := range []uint16{6, 7} {
		base := g711TestFormat(tag, 2, 8000)
		check := func(name string, format []byte, category error) {
			t.Helper()
			wave := riff(riffChunk("fmt ", format), g711TestFact(1), riffChunk("data", []byte{0xff, 0x7f}))
			if p, err := ReadWAV(bytes.NewReader(wave), WAVReadOptions{}); p != nil || !errors.Is(err, category) {
				t.Fatal("format accepted or wrong error", tag, name, err)
			}
		}
		for _, n := range []int{0, 14, 16, 17, 19, 20, 22, 40, 42} {
			b := make([]byte, n)
			copy(b, base)
			check(fmt.Sprint("length", n), b, ErrUnsupported)
		}
		for _, field := range []struct {
			offset int
			values []uint16
			kind   error
		}{
			{0, []uint16{2, 8, 0x10, 0x31, 0xfffe}, ErrUnsupported},
			{2, []uint16{0, 65, 65535}, ErrInvalidInput},
			{12, []uint16{0, 1, 3, 16, 65535}, ErrInvalidInput},
			{14, []uint16{0, 4, 7, 16, 24, 32, 64}, ErrUnsupported},
			{16, []uint16{1, 2, 22, 65535}, ErrUnsupported},
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
		}{{4, 0, ErrInvalidInput}, {4, MaxSampleRate + 1, ErrUnsupported},
			{8, 0, ErrInvalidInput}, {8, 15999, ErrInvalidInput}, {8, 16001, ErrInvalidInput}, {8, math.MaxUint32, ErrInvalidInput}} {
			b := bytes.Clone(base)
			binary.LittleEndian.PutUint32(b[field.offset:], field.value)
			check("rate", b, field.kind)
		}
		for _, rate := range []int{1, 44100, MaxSampleRate} {
			wave := riff(riffChunk("fmt ", g711TestFormat(tag, 64, rate)), g711TestFact(1), riffChunk("data", make([]byte, 64)))
			p, err := ReadWAV(bytes.NewReader(wave), WAVReadOptions{})
			if err != nil || p.Format() != (Format{rate, 64, 0}) {
				t.Fatal("legal channel/rate limit", rate, err)
			}
		}
	}
}

func TestWAVG711ChunksPadsMetadataAndOldPolicy(t *testing.T) {
	for _, tag := range []uint16{6, 7} {
		f := riffChunk("fmt ", g711TestFormat(tag, 1, 8000))
		d := riffChunk("data", []byte{0xff})
		for _, wave := range [][]byte{
			riff(f, d, g711TestFact(1), g711TestFact(1)), riff(f, g711TestFact(1), d, d),
			riff(f, g711TestFact(1), d, f), riff(d, f, g711TestFact(1)),
			riff(f, g711TestFact(1)), riff(g711TestFact(1), d),
			riff(f, g711TestFact(1), d, riffChunk("LIST", []byte("wavl"))),
			riff(f, g711TestFact(1), d, riffChunk("LIST", nil)),
			riff(f, g711TestFact(1), d, riffChunk("smpl", nil)),
			riff(f, g711TestFact(1), d, riffChunk("slnt", nil)),
			riff(f, g711TestFact(1), d, riffChunk("what", nil)),
		} {
			if p, err := ReadWAV(bytes.NewReader(wave), WAVReadOptions{DiscardMetadata: true}); p != nil || err == nil {
				t.Fatal("duplicate/missing/unknown chunk accepted", tag, err)
			}
		}
		for _, n := range []int{0, 1, 2, 3, 5, 6, 8} {
			wave := riff(f, riffChunk("fact", make([]byte, n)), d)
			if p, err := ReadWAV(bytes.NewReader(wave), WAVReadOptions{}); p != nil || !errors.Is(err, ErrUnsupported) {
				t.Fatal("fact extension guessed", n, err)
			}
		}
		for _, id := range []string{"LIST", "bext", "iXML", "axml", "ID3 ", "id3 "} {
			payload := []byte{1, 2, 3}
			if id == "LIST" {
				payload = []byte("INFO")
			}
			wave := riff(f, g711TestFact(1), d, riffChunk(id, payload))
			if _, err := ReadWAV(bytes.NewReader(wave), WAVReadOptions{}); !errors.Is(err, ErrUnsupported) {
				t.Fatal("metadata not opt-in", id, err)
			}
			if _, err := ReadWAV(bytes.NewReader(wave), WAVReadOptions{DiscardMetadata: true}); err != nil {
				t.Fatal("metadata opt-in failed", id, err)
			}
		}
		for _, id := range []string{"JUNK", "PAD ", "data"} {
			badPad := riffChunk(id, []byte{0xff})
			badPad[len(badPad)-1] = 0x91
			for _, beforeFmt := range []bool{true, false} {
				var wave []byte
				if id == "data" {
					wave = riff(f, g711TestFact(1), badPad)
				} else if beforeFmt {
					wave = riff(badPad, f, g711TestFact(1), d)
				} else {
					wave = riff(f, g711TestFact(1), d, badPad)
				}
				if p, err := ReadWAV(bytes.NewReader(wave), WAVReadOptions{}); p != nil || !errors.Is(err, ErrInvalidInput) {
					t.Fatal("nonzero RIFF pad accepted", id, beforeFmt, err)
				}
			}
		}
		wave := riff(f, g711TestFact(1), d)
		missingPad := bytes.Clone(wave[:len(wave)-1])
		binary.LittleEndian.PutUint32(missingPad[4:], uint32(len(missingPad)-8))
		if p, err := ReadWAV(bytes.NewReader(missingPad), WAVReadOptions{}); p != nil || !errors.Is(err, ErrInvalidInput) {
			t.Fatal("missing data pad inside RIFF boundary", err)
		}
	}
	// New G.711 zero-pad policy must not tighten the older PCM/IEEE/IMA
	// acceptance or apply G.711's mandatory fact to PCM/IEEE.
	badPad := riffChunk("JUNK", []byte{1})
	badPad[len(badPad)-1] = 0x91
	for _, f := range [][]byte{fixtureFormat(1, 16, 1, 8000), fixtureFormat(3, 32, 1, 8000), fixtureFormat(3, 64, 1, 8000)} {
		width := int(binary.LittleEndian.Uint16(f[14:])) / 8
		if _, err := ReadWAV(bytes.NewReader(riff(badPad, riffChunk("fmt ", f), riffChunk("data", make([]byte, width)))), WAVReadOptions{}); err != nil {
			t.Fatal("legacy PCM/IEEE policy changed", err)
		}
	}
	ima := imaTestWave(1, 8, 1, 1, false)
	ima = riff(badPad, ima[12:])
	if p, err := ReadWAV(bytes.NewReader(ima), WAVReadOptions{}); err != nil || p.Frames() != 1 {
		t.Fatal("legacy IMA pad/tail policy changed", err)
	}
}

func TestWAVG711LimitsTruncationsAndIO(t *testing.T) {
	for _, tag := range []uint16{6, 7} {
		for _, ch := range []int{1, 3} {
			for _, after := range []bool{false, true} {
				wave := g711TestWave(tag, ch, 3, after)
				limits := Limits{MaxFrames: 3, MaxSamples: 3 * ch, MaxBytes: int64(len(wave)), MaxChunks: 3}
				if _, err := ReadWAV(bytes.NewReader(wave), WAVReadOptions{Limits: limits}); err != nil {
					t.Fatal("exact limits", err)
				}
				for _, l := range []Limits{{MaxFrames: 2}, {MaxSamples: 3*ch - 1}, {MaxBytes: int64(len(wave) - 1)}, {MaxChunks: 2}} {
					if p, err := ReadWAV(bytes.NewReader(wave), WAVReadOptions{Limits: l}); p != nil || !errors.Is(err, ErrLimit) {
						t.Fatal("resource bound ignored", tag, ch, l, err)
					}
				}
				for end := 0; end < len(wave); end++ {
					if p, err := ReadWAV(bytes.NewReader(wave[:end]), WAVReadOptions{}); p != nil || err == nil {
						t.Fatal("truncated G.711 accepted", tag, ch, end)
					}
					r := io.MultiReader(bytes.NewReader(wave[:end]), g711ErrorReader{})
					if p, err := ReadWAV(r, WAVReadOptions{}); p != nil || !errors.Is(err, io.ErrClosedPipe) {
						t.Fatal("underlying error lost", tag, ch, end, err)
					}
				}
				reader := bytes.NewReader(append(bytes.Clone(wave), []byte("NEXT")...))
				if p, err := ReadWAV(oneByteReader{reader}, WAVReadOptions{}); err != nil || p.Frames() != 3 || reader.Len() != 4 {
					t.Fatal("short reads or stream boundary", err, reader.Len())
				}
			}
		}
		wave := g711TestWave(tag, 1, 0, false)
		binary.LittleEndian.PutUint32(wave[4:], uint32(len(wave)-8)+(1<<30))
		binary.LittleEndian.PutUint32(wave[len(wave)-4:], 1<<30)
		r := bytes.NewReader(wave)
		if p, err := ReadWAV(r, WAVReadOptions{Limits: Limits{MaxFrames: 1, MaxBytes: 1 << 33}}); p != nil || !errors.Is(err, ErrLimit) || r.Len() != 0 {
			t.Fatal("large decoded capacity not checked before read/allocation", err, r.Len())
		}
	}
}

type g711ErrorReader struct{}

func (g711ErrorReader) Read([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestWAVG711IndependentFixtures(t *testing.T) {
	dir := g711TestFixtureDir(t)
	b, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Cases []struct {
			File, PCMFile, EncodedSHA256, PCMSHA256, FFmpegWholeSHA256, LibsndfileWholeSHA256 string
			Tag                                                                               uint16
			Channels, Rate, Frames                                                            int
			DiscardMetadata                                                                   bool
		}
		RejectedCases []struct{ File, EncodedSHA256, Category string }
	}
	if err := json.Unmarshal(b, &manifest); err != nil || len(manifest.Cases) != 42 || len(manifest.RejectedCases) != 16 {
		t.Fatal("incomplete independent reference manifest", err)
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
			encodedHash := sha256.Sum256(encoded)
			rawHash := sha256.Sum256(expected)
			if hex.EncodeToString(encodedHash[:]) != c.EncodedSHA256 || hex.EncodeToString(rawHash[:]) != c.PCMSHA256 || c.PCMSHA256 != c.FFmpegWholeSHA256 || c.PCMSHA256 != c.LibsndfileWholeSHA256 {
				t.Fatal("independent complete reference hashes disagree")
			}
			p, err := ReadWAV(bytes.NewReader(encoded), WAVReadOptions{DiscardMetadata: c.DiscardMetadata})
			if err != nil || p.Format() != (Format{c.Rate, c.Channels, 0}) || p.Frames() != c.Frames || len(p.samples)*2 != len(expected) {
				t.Fatal("independent format/length", c, err)
			}
			for i, got := range p.samples {
				want := float64(int16(binary.LittleEndian.Uint16(expected[2*i:]))) / 32768
				if math.Float64bits(got) != math.Float64bits(want) {
					t.Fatal("independent complete PCM", i, got, want)
				}
			}
			if c.DiscardMetadata {
				if p, err := ReadWAV(bytes.NewReader(encoded), WAVReadOptions{}); p != nil || !errors.Is(err, ErrUnsupported) {
					t.Fatal("independent metadata discarded implicitly", err)
				}
			}
		})
	}
	for _, c := range manifest.RejectedCases {
		t.Run(c.File, func(t *testing.T) {
			encoded, err := os.ReadFile(filepath.Join(dir, c.File))
			if err != nil {
				t.Fatal(err)
			}
			hash := sha256.Sum256(encoded)
			if hex.EncodeToString(hash[:]) != c.EncodedSHA256 {
				t.Fatal("rejected fixture hash changed")
			}
			want := ErrInvalidInput
			if c.Category == "unsupported" {
				want = ErrUnsupported
			}
			for _, discard := range []bool{false, true} {
				p, err := ReadWAV(bytes.NewReader(encoded), WAVReadOptions{DiscardMetadata: discard})
				// Real FFmpeg extensible files also have LIST/INFO. Without
				// opt-in either metadata or GUID rejection is unsupported.
				if p != nil || !errors.Is(err, want) {
					t.Fatal("invalid/unsupported reference accepted", discard, err)
				}
			}
		})
	}
}

func TestWAVG711ConcurrentOwnership(t *testing.T) {
	for _, tag := range []uint16{6, 7} {
		wave := g711TestWave(tag, 3, 32769, true)
		p, err := ReadWAV(bytes.NewReader(wave), WAVReadOptions{})
		if err != nil {
			t.Fatal(err)
		}
		want := p.Samples()
		p.Samples()[0] = 0
		if p.samples[0] != want[0] {
			t.Fatal("Samples aliases decoded storage")
		}
		var wg sync.WaitGroup
		for range 8 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				got, err := ReadWAV(bytes.NewReader(wave), WAVReadOptions{})
				if err != nil {
					t.Error(err)
					return
				}
				for i, sample := range got.samples {
					if math.Float64bits(sample) != math.Float64bits(want[i]) {
						t.Error("concurrent mapping", i)
						return
					}
				}
				got.samples[0] = 0
			}()
		}
		wg.Wait()
		clear(wave)
		if p.samples[0] != want[0] {
			t.Fatal("input/independent outputs alias")
		}
	}
}

func FuzzWAVG711(f *testing.F) {
	for _, tag := range []uint16{6, 7} {
		for _, ch := range []int{1, 3, 64} {
			f.Add(g711TestWave(tag, ch, 17, false))
			f.Add(g711TestWave(tag, ch, 1, true))
		}
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		limits := Limits{MaxBytes: 1 << 16, MaxFrames: 4096, MaxSamples: 8192, MaxChunks: 16}
		p, err := ReadWAV(bytes.NewReader(b), WAVReadOptions{Limits: limits, DiscardMetadata: true})
		if err != nil {
			if p != nil {
				t.Fatal("partial output on error")
			}
			return
		}
		if p.validate() != nil || p.Frames() > limits.MaxFrames || len(p.samples) > limits.MaxSamples {
			t.Fatal("invalid/budget-exceeding output")
		}
	})
}

func BenchmarkReadWAVG711(b *testing.B) {
	for _, tag := range []uint16{6, 7} {
		for _, ch := range []int{1, 2, 6} {
			wave := riff(riffChunk("fmt ", g711TestFormat(tag, ch, 48000)), g711TestFact(48000), riffChunk("data", g711TestData(ch, 48000)))
			b.Run(fmt.Sprintf("tag%d_ch%d", tag, ch), func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(len(wave)))
				b.ResetTimer()
				for range b.N {
					p, err := ReadWAV(bytes.NewReader(wave), WAVReadOptions{})
					if err != nil || p.Frames() != 48000 {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
