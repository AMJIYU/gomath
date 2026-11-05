package subtitle

import (
	"bytes"
	"io"
	"strconv"
	"testing"
)

// 1000 nonoverlapping cues, 999.5s extent, numeric IDs and two UTF-8 lines each.
// Input generation is excluded; parse allocation and writer validation included.
func BenchmarkCodec1000Cues(b *testing.B) {
	cues := make([]Cue, 1000)
	for i := range cues {
		cues[i] = Cue{ID: strconv.Itoa(i + 1), StartMS: int64(i) * 1000, EndMS: int64(i)*1000 + 500, Text: "你好，世界 👋\nSecond line with R&D and spaces  preserved."}
	}
	track := mustTrack(b, cues, Options{})
	for _, format := range []Format{SRT, WebVTT} {
		name := "SRT"
		encode, decode := WriteSRT, ReadSRT
		if format == WebVTT {
			name, encode, decode = "WebVTT", WriteWebVTT, ReadWebVTT
		}
		var buffer bytes.Buffer
		if err := encode(&buffer, track, WriteOptions{}); err != nil {
			b.Fatal(err)
		}
		data := bytes.Clone(buffer.Bytes())
		b.Run(name+"/read", func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(data)))
			for b.Loop() {
				if _, err := decode(bytes.NewReader(data), Options{}); err != nil {
					b.Fatal(err)
				}
			}
		})
		b.Run(name+"/write", func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(data)))
			for b.Loop() {
				if err := encode(io.Discard, track, WriteOptions{}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
