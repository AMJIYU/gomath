package export_test

import (
	"bytes"
	"gomath/audio"
	"gomath/export"
	"os"
	"path/filepath"
	"testing"
)

func TestFLACAtomicOutputAndFailureCleanup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sound.flac")
	pcm, err := audio.NewPCM(audio.Format{SampleRate: 8000, Channels: 1}, []float64{-.5, 0, .5, 1, -1})
	if err != nil {
		t.Fatal(err)
	}
	if err = export.FLAC(path, pcm, audio.FLACWriteOptions{}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 42 || string(data[:4]) != "fLaC" {
		t.Fatal("native FLAC stream missing")
	}
	if err = export.FLAC(path, pcm, audio.FLACWriteOptions{}); err == nil {
		t.Fatal("FLAC overwrite accepted")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(data, after) {
		t.Fatal("existing FLAC changed")
	}
	if err = export.FLAC(filepath.Join(dir, "bad.flac"), nil, audio.FLACWriteOptions{}); err == nil {
		t.Fatal("nil PCM accepted")
	}
	if err = export.FLAC(filepath.Join(dir, "budget.flac"), pcm, audio.FLACWriteOptions{Limits: audio.Limits{MaxBytes: 42}}); err == nil {
		t.Fatal("FLAC byte limit ignored")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatal("failed FLAC leaked output", err)
	}
}
