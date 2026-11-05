package audio

import "testing"

// One-second 48kHz clips; creation excluded, validation/output allocation
// included. Remix includes every zero coefficient in its dense matrix work.
func BenchmarkRemix48k(b *testing.B) {
	for _, test := range []struct {
		name   string
		input  int
		matrix [][]float64
	}{
		{"stereo_to_mono", 2, [][]float64{{.5, .5}}},
		{"six_to_stereo", 6, [][]float64{{1, 0, .5, .25, .5, 0}, {0, 1, .5, .25, 0, .5}}},
	} {
		b.Run(test.name, func(b *testing.B) {
			p := sinePCM(b, 48000, 48000, test.input, 1000)
			format := Format{48000, len(test.matrix), 0}
			b.ReportAllocs()
			b.SetBytes(int64(p.Frames() * format.Channels * 8))
			b.ResetTimer()
			for b.Loop() {
				if _, err := p.Remix(format, test.matrix, RemixOptions{}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkSequence48kStereo(b *testing.B) {
	p := sinePCM(b, 48000, 48000, 2, 1000)
	b.Run("concat_four", func(b *testing.B) {
		clips := []*PCM{p, p, p}
		b.ReportAllocs()
		b.SetBytes(int64(len(p.samples) * 8 * 4))
		for b.Loop() {
			if _, err := p.Concat(clips, Limits{}); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("repeat_four", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(p.samples) * 8 * 4))
		for b.Loop() {
			if _, err := p.Repeat(4, Limits{}); err != nil {
				b.Fatal(err)
			}
		}
	})
}
