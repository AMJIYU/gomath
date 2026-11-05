package audio

import (
	"bytes"
	"io"
	"testing"
)

// Workload: one second of 48kHz stereo PCM, channel 0 a .5-amplitude 1kHz sine,
// channel 1 silence. Input creation is excluded; output allocation is included.
func BenchmarkResample48kStereo(b *testing.B) {
	p := sinePCM(b, 48000, 48000, 2, 1000)
	for _, test := range []struct {
		name string
		rate int
	}{{"to_16k", 16000}, {"to_44k1", 44100}} {
		b.Run(test.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(p.samples) * 8))
			b.ResetTimer()
			for b.Loop() {
				if _, err := p.Resample(test.rate, ResampleOptions{}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkWAV48kStereo(b *testing.B) {
	p := sinePCM(b, 48000, 48000, 2, 1000)
	var buf bytes.Buffer
	if err := WriteWAV(&buf, p, WAVWriteOptions{Encoding: PCM24}); err != nil {
		b.Fatal(err)
	}
	encoded := bytes.Clone(buf.Bytes())
	b.Run("decode_pcm24", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(encoded)))
		for b.Loop() {
			if _, err := ReadWAV(bytes.NewReader(encoded), WAVReadOptions{}); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("encode_pcm24", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(encoded)))
		for b.Loop() {
			if err := WriteWAV(io.Discard, p, WAVWriteOptions{Encoding: PCM24}); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkMix48kStereo(b *testing.B) {
	p := sinePCM(b, 48000, 48000, 2, 1000)
	b.ReportAllocs()
	b.SetBytes(int64(len(p.samples) * 8))
	for b.Loop() {
		if _, err := p.Mix(p, .5); err != nil {
			b.Fatal(err)
		}
	}
}
