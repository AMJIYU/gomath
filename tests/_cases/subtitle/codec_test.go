package subtitle

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
)

func readFixture(t testing.TB, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestSRTFixturesExactUTF8MultilineAndRoundTrip(t *testing.T) {
	want := []Cue{
		{ID: "7", StartMS: 1, EndMS: 1234, Text: "你好，世界 👋\nSecond line with R&D"},
		{ID: "3", StartMS: 1234, EndMS: 60000, Text: "保留  空格"},
		{ID: "9", StartMS: 3723004, EndMS: 3724005, Text: "第三条字幕"},
	}
	for _, name := range []string{"multiline.srt", "multiline-bom-crlf.srt"} {
		t.Run(name, func(t *testing.T) {
			data := readFixture(t, name)
			track, err := ReadSRT(oneByteReader{bytes.NewReader(data)}, Options{})
			if err != nil || !slices.Equal(track.Cues(), want) {
				t.Fatal("SRT fixture decode", err)
			}
			for range 3 {
				var encoded bytes.Buffer
				if err := WriteSRT(&encoded, track, WriteOptions{}); err != nil {
					t.Fatal(err)
				}
				if bytes.Contains(encoded.Bytes(), []byte("\r")) || !strings.Contains(encoded.String(), "01:02:03,004") {
					t.Fatal("canonical line ending/timestamp failed")
				}
				track, err = ReadSRT(&encoded, Options{})
				if err != nil || !slices.Equal(track.Cues(), want) {
					t.Fatal("SRT round trip changed cue", err)
				}
			}
		})
	}
}

func TestWebVTTRoundTripLiteralEntitiesAndIdentifiers(t *testing.T) {
	want := []Cue{
		{ID: "clip-a", StartMS: 1, EndMS: 1234, Text: "你好，世界 👋 & friends\n<b>literal, not a style</b>"},
		{StartMS: 1234, EndMS: 60000, Text: "Keep  spaces \u00a0 \u200e \u200f"},
		{ID: "chapter-3", StartMS: 3723004, EndMS: 3724005, Text: "第三条字幕"},
	}
	data := readFixture(t, "plain.vtt")
	track, err := ReadWebVTT(oneByteReader{bytes.NewReader(data)}, Options{})
	if err != nil || !slices.Equal(track.Cues(), want) {
		t.Fatal("VTT fixture decode", err)
	}
	for range 3 {
		var encoded bytes.Buffer
		if err := WriteWebVTT(&encoded, track, WriteOptions{}); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(encoded.String(), "<b>") || !strings.Contains(encoded.String(), "&lt;b&gt;") || !strings.Contains(encoded.String(), "00:00:00.001") {
			t.Fatal("plain text was turned into markup or timestamp rounded")
		}
		track, err = ReadWebVTT(&encoded, Options{})
		if err != nil || !slices.Equal(track.Cues(), want) {
			t.Fatal("VTT round trip changed cue", err)
		}
	}
	literal := mustTrack(t, []Cue{{ID: "literal", EndMS: 1, Text: "<v speaker> &amp; &#123; --> 1 < 2 > 0\n带\t制表符"}}, Options{})
	var encoded bytes.Buffer
	if err := WriteWebVTT(&encoded, literal, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	parsed, err := ReadWebVTT(&encoded, Options{})
	if err != nil || !slices.Equal(parsed.cues, literal.cues) {
		t.Fatal("literal special characters did not round-trip", err)
	}
	if _, err := ReadWebVTT(strings.NewReader("\ufeffWEBVTT\r\n\r\n00:00.000 --> 00:00.001\r\n你好\r\n"), Options{}); err != nil {
		t.Fatal("VTT BOM/CRLF rejected", err)
	}
}

func TestExplicitCrossFormatIdentifiersAndNoSilentLoss(t *testing.T) {
	track := mustTrack(t, []Cue{{ID: "chapter", EndMS: 1, Text: "plain"}}, Options{})
	var b bytes.Buffer
	if err := WriteSRT(&b, track, WriteOptions{}); !errors.Is(err, ErrUnsupported) || b.Len() != 0 {
		t.Fatal("opaque VTT identifier silently discarded", err)
	}
	numbered, err := track.Renumber()
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteSRT(&b, numbered, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	back, err := ReadSRT(&b, Options{})
	if err != nil || !slices.Equal(back.cues, numbered.cues) || track.cues[0].ID != "chapter" {
		t.Fatal("explicit cross-format renumber failed", err)
	}
	anonymous := mustTrack(t, []Cue{{EndMS: 1, Text: "a"}, {StartMS: 1, EndMS: 2, Text: "b"}}, Options{})
	if err := WriteSRT(&b, anonymous, WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(b.String(), "1\n") || !strings.Contains(b.String(), "\n2\n") {
		t.Fatal("anonymous SRT cues not numbered")
	}
	b.Reset()
	collision := mustTrack(t, []Cue{{ID: "2", EndMS: 1, Text: "a"}, {StartMS: 1, EndMS: 2, Text: "b"}}, Options{})
	if err := WriteSRT(&b, collision, WriteOptions{}); !errors.Is(err, ErrInvalidInput) || b.Len() != 0 {
		t.Fatal("generated SRT identifier collision", err)
	}
	for _, text := range []string{"<b>bold</b>", "&amp;", "&#39;"} {
		b.Reset()
		plain := mustTrack(t, []Cue{{EndMS: 1, Text: text}}, Options{})
		if err := WriteSRT(&b, plain, WriteOptions{}); !errors.Is(err, ErrUnsupported) || b.Len() != 0 {
			t.Fatal("ambiguous SubRip markup emitted silently", text, err)
		}
	}
	for _, id := range []string{"NOTE", "NOTE ignored", "STYLE", "REGION custom"} {
		b.Reset()
		reserved := mustTrack(t, []Cue{{ID: id, EndMS: 1, Text: "visible"}}, Options{})
		if err := WriteWebVTT(&b, reserved, WriteOptions{}); !errors.Is(err, ErrUnsupported) || b.Len() != 0 {
			t.Fatal("ID became an ignored WebVTT block", id, err)
		}
	}
}

func TestRejectVisibleWebVTTFeaturesAndMalformedStreams(t *testing.T) {
	for _, name := range []string{"unsupported-markup.vtt", "unsupported-position.vtt"} {
		if track, err := ReadWebVTT(bytes.NewReader(readFixture(t, name)), Options{}); !errors.Is(err, ErrUnsupported) || track != nil {
			t.Fatal("visible VTT feature silently discarded", name, err)
		}
	}
	for _, payload := range []string{
		"<b>bold</b>", "<i>italic</i>", "<u>underline</u>", "<c.red>class</c>",
		"<v Voice>speech</v>", "<lang zh>语言</lang>", "<ruby>漢<rt>kan</rt></ruby>",
		"karaoke <00:00.500>text", "&#39;", "&#x27;", "&copy;", "&amp", "a & b", "a --> b",
	} {
		data := "WEBVTT\n\n00:00.000 --> 00:01.000\n" + payload + "\n\n"
		if track, err := ReadWebVTT(strings.NewReader(data), Options{}); err == nil || track != nil {
			t.Fatal("VTT payload accepted without support", payload)
		}
	}
	for _, settings := range []string{"line:90%", "position:10%", "align:start", "size:50%", "vertical:rl", "region:subtitle", "unknown:value"} {
		data := "WEBVTT\n\n00:00.000 --> 00:01.000 " + settings + "\nvisible\n"
		if _, err := ReadWebVTT(strings.NewReader(data), Options{}); !errors.Is(err, ErrUnsupported) {
			t.Fatal("VTT cue setting stripped", settings, err)
		}
	}
	for _, block := range []string{"NOTE", "NOTE author", "NOTE\tcomment", "STYLE\n::cue {color: red}", "REGION\nid:region"} {
		if _, err := ReadWebVTT(strings.NewReader("WEBVTT\n\n"+block+"\n\n"), Options{}); !errors.Is(err, ErrUnsupported) {
			t.Fatal("unrepresented VTT block discarded", block, err)
		}
	}
	for _, data := range []string{
		"", "WEBVTT", "WEBVTT\n", "WEBVTT annotation\n\n", "WEBVTT\nX-TIMESTAMP-MAP=MPEGTS:0\n\n",
		"webvtt\n\n", " WEBVTT\n\n", "WEBVTT\n00:00.000 --> 00:01.000\nx\n",
		"WEBVTT\n\nidentifier\n", "WEBVTT\n\n00:00.000 --> 00:00.000\nx\n",
		"WEBVTT\n\n00:01.000 --> 00:00.000\nx\n", "WEBVTT\n\n00:00.000 --> 00:01.000\n\n",
		"WEBVTT\n\n00:00.000 --> 00:01.000\n \t\n", "WEBVTT\n\n00:00.000 --> 00:01.000\n\xff\n",
		"WEBVTT\n\n00:00.000 --> 00:01.000\nx\x00y\n", "WEBVTT\n\n00:00.000 --> 00:01.000\nx\ry\n",
	} {
		if track, err := ReadWebVTT(strings.NewReader(data), Options{}); err == nil || track != nil {
			t.Fatalf("malformed VTT accepted %q", data)
		}
	}
	for _, data := range []string{
		"0\n00:00:00,000 --> 00:00:01,000\nx\n", "01\n00:00:00,000 --> 00:00:01,000\nx\n",
		"id\n00:00:00,000 --> 00:00:01,000\nx\n", "1\n", "1\n\n",
		"1\n00:00:00,000 --> 00:00:00,000\nx\n", "1\n00:00:00,000 --> 00:00:01,000\n",
		"1\n00:00:00,000-->00:00:01,000\nx\n", "1\n00:00:00,000 --> 00:00:01,000 X1:0\nx\n",
		"1\n00:00:00.000 --> 00:00:01.000\nx\n", "1\n00:00:00,000 --> 00:00:01,000\n<b>x</b>\n",
		"1\n00:00:00,000 --> 00:00:01,000\n&amp;\n", "1\n00:00:00,000 --> 00:00:01,000\n\xff\n",
		"1\r00:00:00,000 --> 00:00:01,000\rx\r", " \n",
	} {
		if track, err := ReadSRT(strings.NewReader(data), Options{}); err == nil || track != nil {
			t.Fatalf("malformed SRT accepted %q", data)
		}
	}
}

func TestParserOrderingOverlapAndDuplicatePolicies(t *testing.T) {
	for _, format := range []Format{SRT, WebVTT} {
		prefix := ""
		if format == WebVTT {
			prefix = "WEBVTT\n\n"
		}
		block := func(id string, start, end int64) string {
			a, _ := FormatTimestamp(start, format)
			b, _ := FormatTimestamp(end, format)
			return id + "\n" + a + " --> " + b + "\nx\n\n"
		}
		decode := ReadSRT
		if format == WebVTT {
			decode = ReadWebVTT
		}
		unordered := prefix + block("2", 1000, 2000) + block("1", 0, 1000)
		if _, err := decode(strings.NewReader(unordered), Options{}); !errors.Is(err, ErrOrder) {
			t.Fatal("parse silently reordered", format, err)
		}
		sorted, err := decode(strings.NewReader(unordered), Options{Order: OrderStableSort})
		if err != nil || sorted.cues[0].ID != "1" || sorted.cues[1].ID != "2" {
			t.Fatal("explicit parse sort failed", format, err)
		}
		overlapping := prefix + block("1", 0, 2000) + block("2", 1000, 3000)
		if _, err := decode(strings.NewReader(overlapping), Options{}); !errors.Is(err, ErrOverlap) {
			t.Fatal("parse silently removed overlap", format, err)
		}
		allowed, err := decode(strings.NewReader(overlapping), Options{Overlap: OverlapAllow})
		if err != nil || allowed.Len() != 2 {
			t.Fatal("explicit overlap retention failed", format, err)
		}
		duplicate := prefix + block("1", 0, 1000) + block("1", 1000, 2000)
		if _, err := decode(strings.NewReader(duplicate), Options{}); !errors.Is(err, ErrInvalidInput) {
			t.Fatal("duplicate ID parse", format, err)
		}
	}
}

func TestStreamResourceLimitsAndNoPartialWrites(t *testing.T) {
	cues := []Cue{{ID: "1", StartMS: 1, EndMS: 2, Text: "12345"}, {ID: "2", StartMS: 2, EndMS: 3, Text: "67890"}}
	track := mustTrack(t, cues, Options{})
	for _, format := range []Format{SRT, WebVTT} {
		decode, encode := ReadSRT, WriteSRT
		if format == WebVTT {
			decode, encode = ReadWebVTT, WriteWebVTT
		}
		var b bytes.Buffer
		if err := encode(&b, track, WriteOptions{}); err != nil {
			t.Fatal(err)
		}
		data := b.String()
		for _, limits := range []Limits{
			{MaxCues: 1}, {MaxTextBytes: 4}, {MaxIDBytes: 1, MaxTotalBytes: 11},
			{MaxLineBytes: 28}, {MaxTimestampMS: 2}, {MaxBytes: int64(len(data) - 1)},
		} {
			if _, err := decode(strings.NewReader(data), Options{Limits: limits}); !errors.Is(err, ErrLimit) {
				t.Fatal("read bound ignored", format, limits, err)
			}
			b.Reset()
			if err := encode(&b, track, WriteOptions{Limits: limits}); !errors.Is(err, ErrLimit) || b.Len() != 0 {
				t.Fatal("write bound not checked before I/O", format, limits, err)
			}
		}
		exact := Limits{MaxCues: 2, MaxTextBytes: 5, MaxTotalBytes: 12, MaxLineBytes: 29, MaxIDBytes: 1, MaxBytes: int64(len(data)), MaxTimestampMS: 3}
		if _, err := decode(strings.NewReader(data), Options{Limits: exact}); err != nil {
			t.Fatal("exact read bound rejected", format, err)
		}
		b.Reset()
		if err := encode(&b, track, WriteOptions{Limits: exact}); err != nil || b.String() != data {
			t.Fatal("exact write bound rejected", format, err)
		}
		if _, err := decode(nil, Options{}); !errors.Is(err, ErrInvalidInput) {
			t.Fatal("nil reader accepted", err)
		}
		if err := encode(nil, track, WriteOptions{}); !errors.Is(err, ErrInvalidInput) {
			t.Fatal("nil writer accepted", err)
		}
		if err := encode(io.Discard, nil, WriteOptions{}); !errors.Is(err, ErrInvalidInput) {
			t.Fatal("nil track accepted", err)
		}
		if _, err := decode(strings.NewReader(data), Options{Limits: Limits{MaxBytes: -1}}); !errors.Is(err, ErrInvalidInput) {
			t.Fatal("invalid reader limits accepted", err)
		}
		if err := encode(io.Discard, track, WriteOptions{Limits: Limits{MaxBytes: -1}}); !errors.Is(err, ErrInvalidInput) {
			t.Fatal("invalid writer limits accepted", err)
		}
		if err := encode(shortWriter{}, track, WriteOptions{}); !errors.Is(err, io.ErrShortWrite) {
			t.Fatal("short write hidden", err)
		}
		if err := encode(failedWriter{}, track, WriteOptions{}); !errors.Is(err, io.ErrClosedPipe) {
			t.Fatal("write error hidden", err)
		}
		if _, err := decode(io.MultiReader(strings.NewReader(data), failedReader{}), Options{}); !errors.Is(err, io.ErrClosedPipe) {
			t.Fatal("reader error lost after complete cue", err)
		}
	}
	var b bytes.Buffer
	needsEscaping := mustTrack(t, []Cue{{EndMS: 1, Text: strings.Repeat("&", 8)}}, Options{})
	if err := WriteWebVTT(&b, needsEscaping, WriteOptions{Limits: Limits{MaxLineBytes: 29}}); !errors.Is(err, ErrLimit) || b.Len() != 0 {
		t.Fatal("escaped line bound ignored", err)
	}
	if _, err := ReadWebVTT(strings.NewReader("WEBVTT\n\n00:00.000 --> 00:00.001\n&lt;\n"), Options{Limits: Limits{MaxTotalBytes: 1}}); err != nil {
		t.Fatal("decoded text bound counted encoded reference bytes", err)
	}
	for _, decode := range []func(io.Reader, Options) (*Track, error){ReadSRT, ReadWebVTT} {
		infinite := &endlessReader{}
		if _, err := decode(infinite, Options{Limits: Limits{MaxLineBytes: 32}}); !errors.Is(err, ErrLimit) || infinite.read > 4096 {
			t.Fatal("overlong line read was unbounded", infinite.read, err)
		}
		infinite = &endlessReader{}
		if _, err := decode(infinite, Options{Limits: Limits{MaxBytes: 100}}); !errors.Is(err, ErrLimit) || infinite.read != 101 {
			t.Fatal("stream limit did not stop reads at limit+1", infinite.read, err)
		}
	}
}

func TestLongUTF8LinesAndExactReaderBoundaries(t *testing.T) {
	text := strings.Repeat("中文", 2000)
	track := mustTrack(t, []Cue{{ID: "1", EndMS: 1, Text: text}}, Options{})
	for _, format := range []Format{SRT, WebVTT} {
		encode, decode := WriteSRT, ReadSRT
		if format == WebVTT {
			encode, decode = WriteWebVTT, ReadWebVTT
		}
		var b bytes.Buffer
		if err := encode(&b, track, WriteOptions{}); err != nil {
			t.Fatal(err)
		}
		got, err := decode(&b, Options{Limits: Limits{MaxLineBytes: len(text)}})
		if err != nil || got.cues[0].Text != text {
			t.Fatal("UTF-8 line spanning several input buffers", format, err)
		}
	}
	for _, n := range []int{1, 4092, 4093, 4094, 4095, 4096, 4097} {
		text := strings.Repeat("x", n)
		data := "\ufeff" + text + "\r\n"
		limits := DefaultLimits()
		limits.MaxLineBytes, limits.MaxBytes = n, int64(len(data))
		reader := newLineReader(strings.NewReader(data), limits)
		got, err := reader.next()
		if err != nil || got != text {
			t.Fatal("BOM/CRLF exact line size at buffer boundary", n, err)
		}
		if _, err := reader.next(); !errors.Is(err, io.EOF) {
			t.Fatal("reader did not reach exact EOF", n, err)
		}
	}
}

func TestEmptyTracksAndConcurrentRoundTrips(t *testing.T) {
	empty := mustTrack(t, nil, Options{})
	for _, format := range []Format{SRT, WebVTT} {
		encode, decode := WriteSRT, ReadSRT
		if format == WebVTT {
			encode, decode = WriteWebVTT, ReadWebVTT
		}
		var b bytes.Buffer
		if err := encode(&b, empty, WriteOptions{}); err != nil {
			t.Fatal(err)
		}
		got, err := decode(&b, Options{})
		if err != nil || got.Len() != 0 {
			t.Fatal("empty round trip failed", err)
		}
	}
	track := mustTrack(t, []Cue{{ID: "1", StartMS: 0, EndMS: 1, Text: "你好\nR&D"}}, Options{})
	want := track.Cues()
	var wg sync.WaitGroup
	for worker := range 12 {
		wg.Go(func() {
			for iteration := range 10 {
				id := fmt.Sprintf("%d-%d", worker, iteration)
				derived, err := track.Append(Cue{ID: id, StartMS: 1, EndMS: 2, Text: "独立\n<literal> &"})
				if err != nil {
					t.Error(err)
					return
				}
				var b bytes.Buffer
				if err := WriteWebVTT(&b, derived, WriteOptions{}); err != nil {
					t.Error(err)
					return
				}
				got, err := ReadWebVTT(&b, Options{})
				if err != nil || !slices.Equal(got.cues, derived.cues) {
					t.Errorf("concurrent round trip: %v", err)
					return
				}
				got.cues[0].Text = "owned result"
			}
		})
	}
	wg.Wait()
	if !slices.Equal(track.Cues(), want) {
		t.Fatal("concurrent derivation mutated original")
	}
}

// Optional development artifacts only. Standard tests never call or depend on
// an external decoder; developers can inspect these files independently.
func TestIndependentDecoderArtifacts(t *testing.T) {
	dir := os.Getenv("GOMATH_SUBTITLE_REFERENCE_OUT")
	if dir == "" {
		t.Skip("set GOMATH_SUBTITLE_REFERENCE_OUT to emit independent decoder artifacts")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	cues := []Cue{
		{ID: "1", StartMS: 1, EndMS: 1234, Text: "你好，世界 👋\nR&D and spaces  preserved"},
		{ID: "2", StartMS: 1234, EndMS: 60001, Text: "第二条字幕"},
		{ID: "3", StartMS: 3723004, EndMS: 3724005, Text: "Third cue\nExact milliseconds"},
	}
	track := mustTrack(t, cues, Options{})
	for _, format := range []Format{SRT, WebVTT} {
		name, encode := "generated.srt", WriteSRT
		if format == WebVTT {
			name, encode = "generated.vtt", WriteWebVTT
		}
		var b bytes.Buffer
		if err := encode(&b, track, WriteOptions{}); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), b.Bytes(), 0644); err != nil {
			t.Fatal(err)
		}
	}
	manifest, err := json.Marshal(cues)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "expected.json"), manifest, 0644); err != nil {
		t.Fatal(err)
	}
}

type oneByteReader struct{ io.Reader }

func (r oneByteReader) Read(b []byte) (int, error) { return r.Reader.Read(b[:min(len(b), 1)]) }

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) { return len(p) - 1, nil }

type failedWriter struct{}

func (failedWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

type failedReader struct{}

func (failedReader) Read([]byte) (int, error) { return 0, io.ErrClosedPipe }

type endlessReader struct{ read int }

func (r *endlessReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'a'
	}
	r.read += len(p)
	return len(p), nil
}
