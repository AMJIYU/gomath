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
	"testing"
)

func TestIndependentWAVFixtures(t *testing.T) {
	root := "." // TestMain centralizes fixture reads and fuzz failure corpora.
	manifest, err := os.ReadFile(filepath.Join(root, "fixtures.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		File     string
		Rate     int
		Channels int
		Mask     uint32
		Samples  []float64
		SHA256   string
	}
	if err := json.Unmarshal(manifest, &fixtures); err != nil {
		t.Fatal(err)
	}
	if len(fixtures) != 6 {
		t.Fatal("missing independent fixtures")
	}
	for _, fixture := range fixtures {
		t.Run(fixture.File, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(root, fixture.File))
			if err != nil {
				t.Fatal(err)
			}
			hash := sha256.Sum256(data)
			if hex.EncodeToString(hash[:]) != fixture.SHA256 {
				t.Fatal("fixture hash differs from recorded independent input")
			}
			// FFmpeg adds LIST/INFO encoder metadata; dropping it is opt-in.
			if _, err := ReadWAV(bytes.NewReader(data), WAVReadOptions{}); !errors.Is(err, ErrUnsupported) {
				t.Fatal("fixture metadata discarded without opt-in", err)
			}
			p, err := ReadWAV(bytes.NewReader(data), WAVReadOptions{DiscardMetadata: true})
			if err != nil {
				t.Fatal(err)
			}
			if p.Format() != (Format{fixture.Rate, fixture.Channels, fixture.Mask}) {
				t.Fatal("incorrect fixture format", p.Format())
			}
			closeSamples(t, p.samples, fixture.Samples, 0)
		})
	}
}

// These helpers construct byte-level RIFF inputs independently of WriteWAV.
func riffChunk(id string, payload []byte) []byte {
	chunk := make([]byte, 8+len(payload)+(len(payload)&1))
	copy(chunk, id)
	binary.LittleEndian.PutUint32(chunk[4:8], uint32(len(payload)))
	copy(chunk[8:], payload)
	return chunk
}

func riff(chunks ...[]byte) []byte {
	data := []byte("RIFF\x00\x00\x00\x00WAVE")
	for _, chunk := range chunks {
		data = append(data, chunk...)
	}
	binary.LittleEndian.PutUint32(data[4:8], uint32(len(data)-8))
	return data
}

func fixtureFormat(tag, bits uint16, channels, rate int) []byte {
	data := make([]byte, 16)
	binary.LittleEndian.PutUint16(data[0:2], tag)
	binary.LittleEndian.PutUint16(data[2:4], uint16(channels))
	binary.LittleEndian.PutUint32(data[4:8], uint32(rate))
	binary.LittleEndian.PutUint32(data[8:12], uint32(rate*channels*int(bits/8)))
	binary.LittleEndian.PutUint16(data[12:14], uint16(channels*int(bits/8)))
	binary.LittleEndian.PutUint16(data[14:16], bits)
	return data
}

func TestWAVChunksPaddingAndStreamBoundary(t *testing.T) {
	fmtChunk := riffChunk("fmt ", fixtureFormat(1, 24, 1, 8000))
	dataChunk := riffChunk("data", []byte{0, 0, 0x80})
	wave := riff(riffChunk("JUNK", []byte{1, 2, 3}), fmtChunk, riffChunk("PAD ", []byte{0}), dataChunk, riffChunk("JUNK", nil))
	reader := bytes.NewReader(append(wave, []byte("NEXT")...))
	// io.Reader implementations may legitimately return only one byte per read.
	p, err := ReadWAV(oneByteReader{reader}, WAVReadOptions{})
	if err != nil || p.Frames() != 1 || p.samples[0] != -1 || reader.Len() != 4 {
		t.Fatal("chunk pad or stream boundary failed", err)
	}
	for _, encoding := range []WAVEncoding{PCM16, PCM24, PCM32, Float32, Float64} {
		for _, format := range []Format{{8000, 1, 0}, {44100, 2, 3}, {48000, 6, 63}, {1, 64, 0}} {
			t.Run(fmt.Sprintf("encoding_%d_channels_%d", encoding, format.Channels), func(t *testing.T) {
				in := make([]float64, 3*format.Channels)
				for i := range in {
					in[i] = float64(i%9-4) / 4
				}
				original := mustPCM(t, format, in)
				var buf bytes.Buffer
				if err := WriteWAV(&buf, original, WAVWriteOptions{Encoding: encoding}); err != nil {
					t.Fatal(err)
				}
				if uint32(buf.Len()-8) != binary.LittleEndian.Uint32(buf.Bytes()[4:8]) || buf.Len()%2 != 0 {
					t.Fatal("invalid RIFF size/pad")
				}
				decoded, err := ReadWAV(&buf, WAVReadOptions{})
				if err != nil || decoded.Format() != format {
					t.Fatal("layout round trip", err)
				}
				_, bits, _ := encoding.parameters()
				tolerance := 0.0
				if encoding <= PCM32 {
					tolerance = math.Ldexp(1, 1-int(bits))
				}
				closeSamples(t, decoded.samples, in, tolerance)
			})
		}
	}
}

func TestWAVQuantizationAndFloatHeadroom(t *testing.T) {
	for _, encoding := range []WAVEncoding{PCM16, PCM24, PCM32} {
		_, bits, _ := encoding.parameters()
		scale := math.Ldexp(1, int(bits)-1)
		codes := []int64{-int64(scale), -int64(scale) + 1, -1, 0, 1, int64(scale) - 1}
		samples := make([]float64, len(codes))
		for i, code := range codes {
			samples[i] = float64(code) / scale
		}
		var b bytes.Buffer
		if err := WriteWAV(&b, mustPCM(t, Format{8000, 1, 0}, samples), WAVWriteOptions{Encoding: encoding}); err != nil {
			t.Fatal(err)
		}
		decoded, err := ReadWAV(&b, WAVReadOptions{})
		if err != nil {
			t.Fatal(err)
		}
		closeSamples(t, decoded.samples, samples, 0)
		samples = []float64{-1, -.5 / scale, .5 / scale, 1}
		b.Reset()
		if err := WriteWAV(&b, mustPCM(t, Format{8000, 1, 0}, samples), WAVWriteOptions{Encoding: encoding}); err != nil {
			t.Fatal(err)
		}
		decoded, err = ReadWAV(&b, WAVReadOptions{})
		if err != nil {
			t.Fatal(err)
		}
		closeSamples(t, decoded.samples, []float64{-1, -1 / scale, 1 / scale, 1 - 1/scale}, 0)
	}
	for _, encoding := range []WAVEncoding{Float32, Float64} {
		samples := []float64{math.Copysign(0, -1), -3.25, 4.5, 1.0 / 3, math.SmallestNonzeroFloat64}
		if encoding == Float64 {
			samples = append(samples, math.MaxFloat64)
		}
		var b bytes.Buffer
		if err := WriteWAV(&b, mustPCM(t, Format{8000, 1, 0}, samples), WAVWriteOptions{Encoding: encoding}); err != nil {
			t.Fatal(err)
		}
		decoded, err := ReadWAV(&b, WAVReadOptions{})
		if err != nil {
			t.Fatal(err)
		}
		for i, sample := range samples {
			want := sample
			if encoding == Float32 {
				want = float64(float32(want))
			}
			if math.Float64bits(want) != math.Float64bits(decoded.samples[i]) {
				t.Fatal("floating-point bits not retained", encoding, i)
			}
		}
	}
	// A block crossing exercises 24-bit sample boundaries in the streaming buffers.
	large := make([]float64, 22001*3)
	for i := range large {
		large[i] = float64(i%100-50) / 64
	}
	var b bytes.Buffer
	if err := WriteWAV(&b, mustPCM(t, Format{48000, 3, 7}, large), WAVWriteOptions{Encoding: PCM24}); err != nil {
		t.Fatal(err)
	}
	decoded, err := ReadWAV(&b, WAVReadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	closeSamples(t, decoded.samples, large, 0)
}

func TestWAVRejectsUnsupportedAndMalformed(t *testing.T) {
	validFormat := fixtureFormat(1, 16, 1, 8000)
	fmtChunk := riffChunk("fmt ", validFormat)
	dataChunk := riffChunk("data", []byte{0, 0})
	valid := riff(fmtChunk, dataChunk)
	wrongAlign := bytes.Clone(valid)
	binary.LittleEndian.PutUint16(wrongAlign[32:34], 3)
	wrongRate := bytes.Clone(valid)
	binary.LittleEndian.PutUint32(wrongRate[28:32], 0)
	shortRIFF := bytes.Clone(valid)
	binary.LittleEndian.PutUint32(shortRIFF[4:8], 3)
	badRIFF := bytes.Clone(valid)
	copy(badRIFF, "RF64")
	wrongFact := riff(fmtChunk, riffChunk("fact", []byte{2, 0, 0, 0}), dataChunk)
	missingPad := riff(fmtChunk, riffChunk("data", []byte{1}))
	missingPad = missingPad[:len(missingPad)-1]
	binary.LittleEndian.PutUint32(missingPad[4:8], uint32(len(missingPad)-8))
	for _, data := range [][]byte{
		nil, []byte("invalid WAVE"), badRIFF, shortRIFF, wrongAlign, wrongRate,
		riff(fmtChunk), riff(dataChunk, fmtChunk), riff(fmtChunk, dataChunk, dataChunk),
		riff(fmtChunk, fmtChunk, dataChunk), riff(fmtChunk, riffChunk("data", []byte{0})),
		riff(riffChunk("fmt ", fixtureFormat(1, 8, 1, 8000)), dataChunk),
		riff(riffChunk("fmt ", fixtureFormat(6, 16, 1, 8000)), dataChunk),
		riff(fmtChunk, riffChunk("LIST", []byte("wavl")), dataChunk),
		riff(fmtChunk, dataChunk, riffChunk("plst", nil)),
		riff(fmtChunk, dataChunk, riffChunk("slnt", []byte{1, 0, 0, 0})),
		riff(fmtChunk, dataChunk, riffChunk("smpl", nil)),
		riff(fmtChunk, dataChunk, riffChunk("what", nil)),
		wrongFact, missingPad, riff(fmtChunk, dataChunk, riffChunk("fact", nil)),
	} {
		if p, err := ReadWAV(bytes.NewReader(data), WAVReadOptions{DiscardMetadata: true}); err == nil || p != nil {
			t.Fatalf("malformed WAVE accepted: %x", data)
		}
	}
	for end := 0; end < len(valid); end++ {
		if _, err := ReadWAV(bytes.NewReader(valid[:end]), WAVReadOptions{}); err == nil {
			t.Fatal("truncation accepted", end)
		}
	}
	for _, encoding := range []WAVEncoding{Float32, Float64} {
		_, bits, _ := encoding.parameters()
		for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
			var data [8]byte
			if bits == 32 {
				binary.LittleEndian.PutUint32(data[:], math.Float32bits(float32(value)))
			} else {
				binary.LittleEndian.PutUint64(data[:], math.Float64bits(value))
			}
			wave := riff(riffChunk("fmt ", fixtureFormat(3, bits, 1, 8000)), riffChunk("data", data[:bits/8]))
			if _, err := ReadWAV(bytes.NewReader(wave), WAVReadOptions{}); !errors.Is(err, ErrInvalidInput) {
				t.Fatal("nonfinite WAVE sample accepted", err)
			}
		}
	}
	for _, chunk := range [][]byte{riffChunk("LIST", []byte("INFO")), riffChunk("iXML", []byte("<xml/>"))} {
		wave := riff(chunk, fmtChunk, dataChunk)
		if _, err := ReadWAV(bytes.NewReader(wave), WAVReadOptions{}); !errors.Is(err, ErrUnsupported) {
			t.Fatal("metadata silently discarded", err)
		}
		if _, err := ReadWAV(bytes.NewReader(wave), WAVReadOptions{DiscardMetadata: true}); err != nil {
			t.Fatal("explicit metadata discard failed", err)
		}
	}
	// Extensible data must retain its exact speaker mapping and full valid bits.
	var ext bytes.Buffer
	if err := WriteWAV(&ext, mustPCM(t, Format{8000, 2, 3}, []float64{0, 0}), WAVWriteOptions{Encoding: PCM24}); err != nil {
		t.Fatal(err)
	}
	for _, mutation := range []func([]byte){
		func(b []byte) { binary.LittleEndian.PutUint16(b[38:40], 20) },
		func(b []byte) { binary.LittleEndian.PutUint16(b[36:38], 23) },
		func(b []byte) { binary.LittleEndian.PutUint32(b[40:44], 1) },
		func(b []byte) { b[59] ^= 1 },
	} {
		data := bytes.Clone(ext.Bytes())
		mutation(data)
		if _, err := ReadWAV(bytes.NewReader(data), WAVReadOptions{}); err == nil {
			t.Fatal("unsupported extensible WAVE accepted")
		}
	}
}

func TestWAVResourceBoundsAndIOFailures(t *testing.T) {
	pcm := mustPCM(t, Format{8000, 2, 0}, []float64{-.5, .5, .25, -.25})
	var b bytes.Buffer
	if err := WriteWAV(&b, pcm, WAVWriteOptions{}); err != nil {
		t.Fatal(err)
	}
	data := bytes.Clone(b.Bytes())
	for _, limits := range []Limits{{MaxBytes: 11}, {MaxBytes: int64(len(data) - 1)}, {MaxFrames: 1}, {MaxSamples: 3}, {MaxChunks: 1}} {
		if _, err := ReadWAV(bytes.NewReader(data), WAVReadOptions{Limits: limits}); !errors.Is(err, ErrLimit) {
			t.Fatal("read bound ignored", limits, err)
		}
		b.Reset()
		if err := WriteWAV(&b, pcm, WAVWriteOptions{Limits: limits}); !errors.Is(err, ErrLimit) || b.Len() != 0 {
			t.Fatal("writer did not validate bound before I/O", limits, err)
		}
	}
	exact := Limits{MaxBytes: int64(len(data)), MaxFrames: 2, MaxSamples: 4, MaxChunks: 2}
	if _, err := ReadWAV(bytes.NewReader(data), WAVReadOptions{Limits: exact}); err != nil {
		t.Fatal("exact RIFF bound rejected", err)
	}
	// Reject an oversized RIFF declaration without consuming any body bytes.
	huge := bytes.Clone(data)
	binary.LittleEndian.PutUint32(huge[4:8], math.MaxUint32)
	r := bytes.NewReader(huge)
	if _, err := ReadWAV(r, WAVReadOptions{}); !errors.Is(err, ErrLimit) || r.Len() != len(huge)-12 {
		t.Fatal("RIFF size checked too late", err)
	}
	// Reject declared decoded storage before trying to read the data body.
	limited := bytes.NewReader(data)
	if _, err := ReadWAV(limited, WAVReadOptions{Limits: Limits{MaxSamples: 1}}); !errors.Is(err, ErrLimit) || limited.Len() != 8 {
		t.Fatal("decoded size checked too late", limited.Len(), err)
	}
	for _, encoding := range []WAVEncoding{PCM16, PCM24, PCM32, Float32} {
		b.Reset()
		if err := WriteWAV(&b, mustPCM(t, Format{8000, 1, 0}, []float64{math.MaxFloat64}), WAVWriteOptions{Encoding: encoding}); !errors.Is(err, ErrInvalidInput) || b.Len() != 0 {
			t.Fatal("encoding overflow silently clipped", encoding, err)
		}
	}
	if err := WriteWAV(io.Discard, pcm, WAVWriteOptions{Encoding: 99}); !errors.Is(err, ErrUnsupported) {
		t.Fatal("unknown encoding accepted", err)
	}
	if err := WriteWAV(shortWriter{}, pcm, WAVWriteOptions{}); !errors.Is(err, io.ErrShortWrite) {
		t.Fatal("short write not reported", err)
	}
	if err := WriteWAV(errorWriter{}, pcm, WAVWriteOptions{}); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal("writer error not retained", err)
	}
	if _, err := ReadWAV(nil, WAVReadOptions{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatal("nil reader accepted", err)
	}
	if err := WriteWAV(nil, pcm, WAVWriteOptions{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatal("nil writer accepted", err)
	}
	for _, encoding := range []WAVEncoding{PCM16, PCM24, PCM32, Float32, Float64} {
		b.Reset()
		if err := WriteWAV(&b, mustPCM(t, Format{1, 1, 0}, nil), WAVWriteOptions{Encoding: encoding}); err != nil {
			t.Fatal(err)
		}
		p, err := ReadWAV(&b, WAVReadOptions{})
		if err != nil || p.Frames() != 0 {
			t.Fatal("empty WAVE round trip failed", err)
		}
	}
}

// The external decoder is development-only. This test emits artifacts only
// when requested; regular tests neither invoke nor require an external codec.
func TestWAVIndependentDecoderArtifacts(t *testing.T) {
	dir := os.Getenv("GOMATH_AUDIO_REFERENCE_OUT")
	if dir == "" {
		t.Skip("set GOMATH_AUDIO_REFERENCE_OUT to emit WAVE for an independent decoder")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	var manifest []struct {
		File     string    `json:"file"`
		Rate     int       `json:"rate"`
		Channels int       `json:"channels"`
		Samples  []float64 `json:"samples"`
	}
	for _, encoding := range []WAVEncoding{PCM16, PCM24, PCM32, Float32, Float64} {
		for _, format := range []Format{{8000, 1, 0}, {44100, 2, 0}, {48000, 6, 63}} {
			samples := make([]float64, format.Channels*3)
			codes := []float64{-1, -.75, -.25, 0, .125, .25, .75, .875}
			for i := range samples {
				samples[i] = codes[i%len(codes)]
			}
			if encoding >= Float32 {
				samples[1] = 2.5
			}
			name := fmt.Sprintf("encoding_%d_channels_%d.wav", encoding, format.Channels)
			var b bytes.Buffer
			if err := WriteWAV(&b, mustPCM(t, format, samples), WAVWriteOptions{Encoding: encoding}); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, name), b.Bytes(), 0644); err != nil {
				t.Fatal(err)
			}
			manifest = append(manifest, struct {
				File     string    `json:"file"`
				Rate     int       `json:"rate"`
				Channels int       `json:"channels"`
				Samples  []float64 `json:"samples"`
			}{name, format.SampleRate, format.Channels, samples})
		}
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
}

type oneByteReader struct{ io.Reader }

func (r oneByteReader) Read(b []byte) (int, error) {
	return r.Reader.Read(b[:min(1, len(b))])
}

type shortWriter struct{}

func (shortWriter) Write(b []byte) (int, error) { return len(b) - 1, nil }

type errorWriter struct{}

func (errorWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
