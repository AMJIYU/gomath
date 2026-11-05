package main

import (
	"bytes"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestDemoRasterSupersamplingKeepsOutputDimensions(t *testing.T) {
	dir := t.TempDir()
	var pixels [][]byte
	for _, scale := range []string{"1", "4"} {
		output := filepath.Join(dir, "ssaa"+scale+".png")
		if err := demo([]string{"-scene", "fractions", "-format", "png", "-time", ".35", "-width", "161", "-height", "91", "-supersampling", scale, "-out", output}); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(output)
		if err != nil {
			t.Fatal(err)
		}
		image, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		if image.Bounds().Dx() != 161 || image.Bounds().Dy() != 91 {
			t.Fatal("supersampling changed output size", image.Bounds())
		}
		pixels = append(pixels, data)
	}
	if bytes.Equal(pixels[0], pixels[1]) {
		t.Fatal("CLI quality option was ignored")
	}
	output := filepath.Join(dir, "invalid.png")
	if err := demo([]string{"-scene", "reference", "-format", "png", "-time", ".5", "-supersampling", "3", "-out", output}); err == nil {
		t.Fatal("invalid supersampling accepted")
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatal("invalid supersampling published output", err)
	}
}

func TestDemoStillImageUsesOneSupersamplingWorker(t *testing.T) {
	// The 640x360x8 canvas fits one worker's pixel budget but not four.
	// PNG/SVG use no frame queue, so the CLI should charge their actual single
	// worker. Video output must retain its configured worker budget instead.
	dir := t.TempDir()
	for _, format := range []string{"png", "svg"} {
		output := filepath.Join(dir, "single."+format)
		if err := demo([]string{"-scene", "reference", "-format", format, "-time", ".5", "-width", "640", "-height", "360", "-supersampling", "8", "-workers", "4", "-out", output}); err != nil {
			t.Fatal("serial output charged unused workers", format, err)
		}
	}
	output := filepath.Join(dir, "overbudget.gif")
	if err := demo([]string{"-scene", "reference", "-format", "gif", "-width", "640", "-height", "360", "-supersampling", "8", "-workers", "4", "-out", output}); err == nil {
		t.Fatal("video ignored configured workers")
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatal("overbudget render published output", err)
	}
	for _, workers := range []string{"0", "257"} {
		output := filepath.Join(dir, "invalid-workers"+workers+".png")
		if err := demo([]string{"-scene", "reference", "-format", "png", "-time", ".5", "-workers", workers, "-out", output}); err == nil {
			t.Fatal("serial normalization bypassed worker validation")
		}
		if _, err := os.Stat(output); !os.IsNotExist(err) {
			t.Fatal("invalid worker count published output", err)
		}
	}
}
