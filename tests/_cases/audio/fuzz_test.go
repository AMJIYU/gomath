package audio

import (
	"bytes"
	"math"
	"testing"
)

func FuzzReadWAV(f *testing.F) {
	f.Add(riff(riffChunk("fmt ", fixtureFormat(1, 24, 1, 8000)), riffChunk("JUNK", []byte{1}), riffChunk("data", []byte{0, 0, 128})))
	f.Add(riff(riffChunk("fmt ", fixtureFormat(3, 64, 2, 44100)), riffChunk("data", make([]byte, 16))))
	f.Add([]byte("RIFF\xff\xff\xff\xffWAVE"))
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		limits := Limits{MaxFrames: 4096, MaxSamples: 8192, MaxBytes: 64 << 10, MaxChunks: 64}
		p, err := ReadWAV(bytes.NewReader(data), WAVReadOptions{Limits: limits})
		if err != nil {
			if p != nil {
				t.Fatal("decoder returned partial PCM with error")
			}
			return
		}
		var encoded bytes.Buffer
		if err := WriteWAV(&encoded, p, WAVWriteOptions{Encoding: Float64, Limits: Limits{MaxBytes: 128 << 10}}); err != nil {
			t.Fatal("valid PCM cannot be encoded losslessly", err)
		}
		again, err := ReadWAV(&encoded, WAVReadOptions{Limits: Limits{MaxBytes: 128 << 10}})
		if err != nil || p.Format() != again.Format() {
			t.Fatal("valid PCM lost format on re-encoding", err)
		}
		for i, value := range p.samples {
			if math.Float64bits(value) != math.Float64bits(again.samples[i]) {
				t.Fatal("lossless float64 WAVE changed samples")
			}
		}
	})
}

func FuzzResampleFiniteAndBounded(f *testing.F) {
	f.Add([]byte{0, 1, 2, 255}, uint16(48000), uint16(16000), uint8(0))
	f.Add([]byte{1}, uint16(1), uint16(48000), uint8(2))
	f.Fuzz(func(t *testing.T, data []byte, from, to uint16, mode uint8) {
		if len(data) > 256 {
			data = data[:256]
		}
		samples := make([]float64, len(data))
		for i := range samples {
			samples[i] = float64(int8(data[i])) / 128
		}
		p := mustPCM(t, Format{int(from) + 1, 1, 0}, samples)
		out, err := p.Resample(int(to)+1, ResampleOptions{Edge: EdgeMode(mode % 3), MaxWork: 500000, Limits: Limits{MaxFrames: 1024, MaxSamples: 1024}})
		if err != nil {
			if out != nil {
				t.Fatal("resampler returned partial output")
			}
			return
		}
		want := (uint64(len(data))*uint64(int(to)+1) + uint64(from)) / uint64(int(from)+1)
		if uint64(out.Frames()) != want || out.Frames() > 1024 {
			t.Fatal("resampling frame bound/rounding failure")
		}
		for _, value := range out.samples {
			if !finite(value) {
				t.Fatal("resampler produced nonfinite output")
			}
		}
	})
}
