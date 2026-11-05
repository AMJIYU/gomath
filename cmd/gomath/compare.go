package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image"
	_ "image/png"
	"math"
	"os"
)

func compare(args []string) error {
	fs := flag.NewFlagSet("compare", flag.ContinueOnError)
	reference := fs.String("reference", "", "reference PNG")
	actual := fs.String("actual", "", "actual PNG")
	threshold := fs.Int("threshold", 1, "per-channel difference threshold, 0..255")
	if e := fs.Parse(args); e != nil {
		return e
	}
	if *threshold < 0 || *threshold > 255 {
		return fmt.Errorf("threshold must be in [0,255]")
	}
	read := func(name string) (image.Image, error) {
		f, e := os.Open(name)
		if e != nil {
			return nil, e
		}
		defer f.Close()
		img, _, e := image.Decode(f)
		return img, e
	}
	a, e := read(*reference)
	if e != nil {
		return e
	}
	b, e := read(*actual)
	if e != nil {
		return e
	}
	if a.Bounds().Size() != b.Bounds().Size() {
		return fmt.Errorf("image dimensions differ")
	}
	width, height := a.Bounds().Dx(), a.Bounds().Dy()
	var absolute, squared float64
	maxDiff, mismatchedPixels, mismatchedChannels := 0, 0, 0
	for y := range height {
		for x := range width {
			ar, ag, ab, _ := a.At(a.Bounds().Min.X+x, a.Bounds().Min.Y+y).RGBA()
			br, bg, bb, _ := b.At(b.Bounds().Min.X+x, b.Bounds().Min.Y+y).RGBA()
			av, bv := [3]uint32{ar, ag, ab}, [3]uint32{br, bg, bb}
			mismatch := false
			for i := range 3 {
				diff := int(av[i]>>8) - int(bv[i]>>8)
				if diff < 0 {
					diff = -diff
				}
				absolute += float64(diff)
				squared += float64(diff * diff)
				maxDiff = max(maxDiff, diff)
				if diff > *threshold {
					mismatchedChannels++
					mismatch = true
				}
			}
			if mismatch {
				mismatchedPixels++
			}
		}
	}
	channels := float64(width * height * 3)
	mse := squared / channels
	var psnr any = "infinite"
	if mse > 0 {
		psnr = 10 * math.Log10(255*255/mse)
	}
	result := map[string]any{"width": width, "height": height, "channels": "RGB; alpha excluded", "threshold": *threshold, "mean_absolute_channel_error": absolute / channels, "max_channel_error": maxDiff, "mismatched_pixel_ratio": float64(mismatchedPixels) / float64(width*height), "mismatched_channel_ratio": float64(mismatchedChannels) / channels, "psnr_db": psnr, "interpretation": "metrics only; no automatic compatibility claim"}
	data, e := json.MarshalIndent(result, "", "  ")
	if e != nil {
		return e
	}
	fmt.Println(string(data))
	return nil
}
