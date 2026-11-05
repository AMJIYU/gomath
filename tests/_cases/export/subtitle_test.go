package export_test

import (
	"bytes"
	"gomath/export"
	"gomath/subtitle"
	"os"
	"path/filepath"
	"testing"
)

func TestSubtitleAtomicPublicationAndRoundTrip(t *testing.T) {
	track, err := subtitle.New([]subtitle.Cue{{StartMS: 125, EndMS: 1126, Text: "数学动画\n第二行"}}, subtitle.Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, format := range []subtitle.Format{subtitle.SRT, subtitle.WebVTT} {
		dir := t.TempDir()
		file := filepath.Join(dir, "captions")
		if err := export.Subtitles(file, track, format, subtitle.WriteOptions{}); err != nil {
			t.Fatal(err)
		}
		original, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var got *subtitle.Track
		if format == subtitle.SRT {
			got, err = subtitle.ReadSRT(bytes.NewReader(original), subtitle.Options{})
		} else {
			got, err = subtitle.ReadWebVTT(bytes.NewReader(original), subtitle.Options{})
		}
		if err != nil {
			t.Fatal(err)
		}
		cue, err := got.Cue(0)
		if err != nil || cue.StartMS != 125 || cue.EndMS != 1126 || cue.Text != "数学动画\n第二行" {
			t.Fatal("roundtrip changed timing/text", cue, err)
		}
		if err := export.Subtitles(file, track, format, subtitle.WriteOptions{}); err == nil {
			t.Fatal("overwrite accepted")
		}
		after, _ := os.ReadFile(file)
		if !bytes.Equal(original, after) {
			t.Fatal("existing subtitles changed")
		}
		if err := export.Subtitles(filepath.Join(dir, "invalid"), nil, format, subtitle.WriteOptions{}); err == nil {
			t.Fatal("nil track accepted")
		}
		if err := export.Subtitles(filepath.Join(dir, "budget"), track, format, subtitle.WriteOptions{Limits: subtitle.Limits{MaxBytes: 1}}); err == nil {
			t.Fatal("output budget ignored")
		}
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) != 1 {
			t.Fatal("failed export leaked temporary files", err)
		}
	}
	if err := export.Subtitles(filepath.Join(t.TempDir(), "bad"), track, subtitle.Format(99), subtitle.WriteOptions{}); err == nil {
		t.Fatal("unknown format accepted")
	}
}
