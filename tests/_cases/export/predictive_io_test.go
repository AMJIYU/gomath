package export

import (
	"image"
	"os"
	"path/filepath"
	"testing"
)

func TestPredictiveWriteFailureAbortsReferenceStream(t *testing.T) {
	dir := t.TempDir()
	options := DefaultMP4Options()
	options.Codec = H264Predictive
	m, err := NewMP4WithOptions(filepath.Join(dir, "broken.mp4"), 32, 32, 24, options)
	if err != nil {
		t.Fatal(err)
	}
	if err = m.out.file.Close(); err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	if err = m.WriteFrame(0, img); err == nil {
		t.Fatal("closed-file write succeeded")
	}
	if err = m.WriteFrame(0, img); err == nil {
		t.Fatal("reference stream continued after dropped output")
	}
	if err = m.Close(); err == nil {
		t.Fatal("failed output published")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatal("failed stream leaked output", err)
	}
}
