package audio

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"testing"
)

type flacReferenceEntry struct {
	File, Raw, Pattern, SHA256 string
	Rate, Channels, Depth      int
	Frames, Block              int
}

// Generates development artifacts only. External programs are never invoked
// by the Go tests or audio package. The independent script owns decoder runs.
func TestFLACIndependentDecoderArtifacts(t *testing.T) {
	dir := os.Getenv("GOMATH_FLAC_REFERENCE_OUT")
	if dir == "" {
		t.Skip("set GOMATH_FLAC_REFERENCE_OUT for development decoder artifacts")
	}
	if !filepath.IsAbs(dir) {
		t.Fatal("reference output must be absolute")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	var manifest []flacReferenceEntry
	emit := func(p *PCM, codes []int64, depth, block int, pattern string) {
		t.Helper()
		encoding := PCM16
		if depth == 24 {
			encoding = PCM24
		}
		var b bytes.Buffer
		if err := WriteFLAC(&b, p, FLACWriteOptions{Encoding: encoding, BlockSize: block}); err != nil {
			t.Fatal(err)
		}
		// Expected raw samples originate from integer generators, never from an
		// encoder or test decoder. This catches matching encoder/reader mistakes.
		raw := make([]byte, len(codes)*(depth/8))
		for i, code := range codes {
			for j := 0; j < depth/8; j++ {
				raw[i*(depth/8)+j] = byte(code >> uint(j*8))
			}
		}
		name := fmt.Sprintf("pcm%d_%dch_%s_n%d_b%d", depth, p.Channels(), pattern, p.Frames(), block)
		file, rawFile := name+".flac", name+fmt.Sprintf(".s%dle", depth)
		if err := os.WriteFile(filepath.Join(dir, file), b.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, rawFile), raw, 0o644); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(raw)
		manifest = append(manifest, flacReferenceEntry{file, rawFile, pattern, hex.EncodeToString(sum[:]), p.SampleRate(), p.Channels(), depth, p.Frames(), block})
	}
	lengths := []int{1, 2, 3, 4, 5, 15, 16, 17, 31, 255, 256, 257, 4095, 4096, 4097, 8192, 8193, 65534, 65535, 65536}
	patterns := []string{"silence", "constant", "extremes", "ramp", "polynomial", "sine", "random"}
	rates := []int{1, 8000, 44100, 48000, 96000, 192000, 768000}
	for _, depth := range []int{16, 24} {
		for _, channels := range []int{1, 2} {
			for i, n := range lengths {
				block := []int{16, 257, 4096, 65535}[i%4]
				p, codes := flacIntegerPCM(t, depth, channels, n, rates[i%len(rates)], patterns[i%len(patterns)])
				emit(p, codes, depth, block, patterns[i%len(patterns)])
			}
			for _, pattern := range patterns {
				p, codes := flacIntegerPCM(t, depth, channels, 12345, 48000, pattern)
				emit(p, codes, depth, 4096, pattern)
			}
			// Exercise all six legal frame-number code lengths independently in
			// unit tests; this fixture crosses the first two transitions in a stream.
			p, codes := flacIntegerPCM(t, depth, channels, 16*2050+1, 44100, "ramp")
			emit(p, codes, depth, 16, "numbering")
			scale := float64(int64(1) << (depth - 1))
			quantSamples := []float64{-1, 1, 0, math.Copysign(0, -1), .5 / scale, -.5 / scale, 1.5 / scale, -1.5 / scale, .5, -.5}
			quantCodes := []int64{-int64(scale), int64(scale) - 1, 0, 0, 1, -1, 2, -2, int64(scale) / 2, -int64(scale) / 2}
			emit(mustPCM(t, Format{44100, channels, 0}, quantSamples), quantCodes, depth, 4096, "rounding")
			if depth == 24 {
				for order := 0; order <= 4; order++ {
					x := flacPredictorCodes(order)
					samples, codes := make([]float64, len(x)*channels), make([]int64, len(x)*channels)
					for f, v := range x {
						for c := 0; c < channels; c++ {
							codes[f*channels+c] = v
							samples[f*channels+c] = float64(v) / (1 << 23)
						}
					}
					emit(mustPCM(t, Format{48000, channels, 0}, samples), codes, depth, 4096, fmt.Sprintf("predictor%d", order))
				}
			}
			escapeSamples, escapeCodes := make([]float64, 512*channels), make([]int64, 512*channels)
			for i := range escapeSamples {
				escapeCodes[i] = int64((i/channels)%2) - 1
				escapeSamples[i] = float64(escapeCodes[i]) / scale
			}
			emit(mustPCM(t, Format{48000, channels, 0}, escapeSamples), escapeCodes, depth, 4096, "escape1")
		}
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "flac_manifest.json"), append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("emitted %d independent-decoder cases to %s", len(manifest), dir)
}

func TestFLACVerifiedFixtures(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("flac", "fixtures.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		File, SHA256, FLACSHA256      string
		Rate, Channels, Depth, Frames int
		Block                         int
	}
	if err := json.Unmarshal(data, &fixtures); err != nil || len(fixtures) != 6 {
		t.Fatal("missing verified fixtures", err)
	}
	for _, fixture := range fixtures {
		t.Run(fixture.File, func(t *testing.T) {
			encoded, err := os.ReadFile(filepath.Join("flac", fixture.File))
			if err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256(encoded)
			if hex.EncodeToString(sum[:]) != fixture.FLACSHA256 {
				t.Fatal("fixture differs from FFmpeg-verified stream")
			}
			info := inspectTestFLAC(t, encoded)
			if info.Rate != fixture.Rate || info.Channels != fixture.Channels || info.Depth != fixture.Depth || len(info.Samples) != fixture.Frames*fixture.Channels || info.Block != fixture.Block {
				t.Fatal("fixture format differs")
			}
			raw := make([]byte, len(info.Samples)*(info.Depth/8))
			samples := make([]float64, len(info.Samples))
			for i, v := range info.Samples {
				for j := 0; j < info.Depth/8; j++ {
					raw[i*(info.Depth/8)+j] = byte(v >> uint(j*8))
				}
				samples[i] = float64(v) / float64(int64(1)<<(info.Depth-1))
			}
			sum = sha256.Sum256(raw)
			if hex.EncodeToString(sum[:]) != fixture.SHA256 {
				t.Fatal("PCM differs from independent full decode")
			}
			encoding := PCM16
			if info.Depth == 24 {
				encoding = PCM24
			}
			var b bytes.Buffer
			if err := WriteFLAC(&b, mustPCM(t, Format{info.Rate, info.Channels, 0}, samples), FLACWriteOptions{Encoding: encoding, BlockSize: info.Block}); err != nil {
				t.Fatal(err)
			}
			equalFLACCodes(t, inspectTestFLAC(t, b.Bytes()).Samples, info.Samples)
		})
	}
}

func BenchmarkFLAC48kStereo(b *testing.B) {
	for _, depth := range []int{16, 24} {
		for _, pattern := range []string{"silence", "sine", "random"} {
			b.Run(fmt.Sprintf("PCM%d/%s", depth, pattern), func(b *testing.B) {
				p, _ := flacIntegerPCM(b, depth, 2, 48000, 48000, pattern)
				encoding := PCM16
				if depth == 24 {
					encoding = PCM24
				}
				b.ReportAllocs()
				b.SetBytes(int64(48000 * 2 * (depth / 8)))
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if err := WriteFLAC(io.Discard, p, FLACWriteOptions{Encoding: encoding}); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
