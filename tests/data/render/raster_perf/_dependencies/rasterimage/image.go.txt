// Package rasterimage decodes PNG, JPEG and the first frame of GIF using Go's
// standard library. It creates immutable assets for affine scene image objects.
package rasterimage

import (
	"bytes"
	"fmt"
	"gomath"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
)

const MaxEncodedBytes = 64 * 1024 * 1024

func New(source image.Image, worldWidth float64) (*gomath.Object, error) {
	bitmap, err := gomath.NewBitmap(source)
	if err != nil {
		return nil, err
	}
	return gomath.NewImage(bitmap, worldWidth)
}

// Decode reads at most 64 MiB plus one byte and validates header dimensions
// before allocating the decoded image. Animated GIFs use the first frame only.
func Decode(reader io.Reader, worldWidth float64) (*gomath.Object, error) {
	if reader == nil {
		return nil, fmt.Errorf("nil image reader")
	}
	data, err := io.ReadAll(io.LimitReader(reader, MaxEncodedBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxEncodedBytes {
		return nil, fmt.Errorf("encoded image exceeds %d bytes", MaxEncodedBytes)
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("image header: %w", err)
	}
	if config.Width <= 0 || config.Height <= 0 || config.Width > gomath.MaxBitmapPixels/config.Height {
		return nil, fmt.Errorf("decoded image exceeds %d pixels or is empty", gomath.MaxBitmapPixels)
	}
	decoded, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}
	return New(decoded, worldWidth)
}

func Load(filename string, worldWidth float64) (*gomath.Object, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	o, err := Decode(f, worldWidth)
	if err == nil {
		o.Name = filename
	}
	return o, err
}
