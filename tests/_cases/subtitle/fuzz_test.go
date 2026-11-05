package subtitle

import (
	"bytes"
	"math"
	"slices"
	"testing"
)

func FuzzSubtitleStreams(f *testing.F) {
	f.Add([]byte("1\n00:00:00,001 --> 00:00:00,002\n你好\nnext\n\n"), false)
	f.Add([]byte("WEBVTT\n\nid\n00:00.001 --> 00:00.002\n&lt;b&gt; &amp;\n\n"), true)
	f.Add([]byte("WEBVTT\n\n00:00.000 --> 00:01.000 position:50%\ntext\n"), true)
	f.Add([]byte("WEBVTT\n\nSTYLE\n::cue{color:red}\n"), true)
	f.Add([]byte{}, false)
	f.Fuzz(func(t *testing.T, data []byte, vtt bool) {
		limits := Limits{MaxCues: 32, MaxTextBytes: 1024, MaxTotalBytes: 8192, MaxLineBytes: 1024, MaxIDBytes: 128, MaxBytes: 16384, MaxTimestampMS: math.MaxInt64}
		options := Options{Order: OrderStableSort, Overlap: OverlapAllow, Limits: limits}
		decode, encode := ReadSRT, WriteSRT
		if vtt {
			decode, encode = ReadWebVTT, WriteWebVTT
		}
		track, err := decode(bytes.NewReader(data), options)
		if err != nil {
			if track != nil {
				t.Fatal("partial track returned on error")
			}
			return
		}
		// Canonical hour timestamps and entity escaping may grow the stream,
		// independently of the input's strict byte/line limits.
		limits.MaxBytes = 128 << 10
		limits.MaxLineBytes = 8192
		options.Limits = limits
		var encoded bytes.Buffer
		if err := encode(&encoded, track, WriteOptions{Limits: limits}); err != nil {
			t.Fatal("accepted subtitle cannot be encoded", err)
		}
		back, err := decode(&encoded, options)
		if err != nil || !slices.Equal(back.cues, track.cues) {
			t.Fatal("subtitle round trip changed cue data", err)
		}
	})
}

func FuzzTimestampExactRoundTrip(f *testing.F) {
	f.Add(int64(0), false)
	f.Add(int64(3723004), true)
	f.Add(int64(math.MaxInt64), true)
	f.Fuzz(func(t *testing.T, ms int64, vtt bool) {
		format := SRT
		if vtt {
			format = WebVTT
		}
		text, err := FormatTimestamp(ms, format)
		if ms < 0 {
			if err == nil {
				t.Fatal("negative time accepted")
			}
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		back, err := ParseTimestamp(text, format)
		if err != nil || back != ms {
			t.Fatal("integer timestamp lost precision", text, back, err)
		}
	})
}
