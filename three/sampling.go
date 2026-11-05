package three

import (
	"fmt"
	"image"
)

type sampleLayout struct {
	scale, width, height int
	rasterBytes          int64
}

// sampleSize validates the output before multiplying dimensions, then checks
// all internal limits using int64. Even adversarial int inputs cannot overflow
// a multiplication or cause a large allocation during renderer construction.
func sampleSize(c Config) (sampleLayout, error) {
	var layout sampleLayout
	if err := validateSize(c.Width, c.Height); err != nil {
		return layout, err
	}
	scale := c.Supersampling
	if scale == 0 {
		scale = 1
	}
	if scale != 1 && scale != 2 && scale != 4 {
		return layout, fmt.Errorf("three: supersampling must be 0 (default 1), 1, 2 or 4; got %d", c.Supersampling)
	}
	w, h := int64(c.Width)*int64(scale), int64(c.Height)*int64(scale)
	pixels := w * h
	if w > MaxSampleDimension || h > MaxSampleDimension {
		return layout, fmt.Errorf("three: supersampling %dx needs internal dimensions %dx%d; maximum per axis is %d", scale, w, h, MaxSampleDimension)
	}
	if pixels > MaxSamplePixels {
		return layout, fmt.Errorf("three: supersampling %dx needs %d internal pixels; maximum is %d", scale, pixels, MaxSamplePixels)
	}
	// Each sample has four uint8 color channels and one float64 depth value.
	// At scale 1 the sample image is also the output and is counted only once.
	bytes := pixels * 12
	if scale > 1 {
		bytes += int64(c.Width) * int64(c.Height) * 4
	}
	if bytes > MaxRasterBufferBytes {
		return layout, fmt.Errorf("three: raster buffers need %d bytes; maximum is %d", bytes, MaxRasterBufferBytes)
	}
	return sampleLayout{scale: scale, width: int(w), height: int(h), rasterBytes: bytes}, nil
}

// resolveSamples averages stored premultiplied channels directly. This keeps
// transparent coverage from acquiring dark fringes or unassociated RGB values.
// The integer sum has at most 16*255 per channel. Rounding is nearest, half up,
// identically for RGB and alpha; no unpremultiply or gamma conversion occurs.
// Only validated scales >1 and zero-origin internal buffers reach this helper.
func resolveSamples(src *image.RGBA, width, height, scale int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	count := uint32(scale * scale)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			var sum [4]uint32
			for sy := 0; sy < scale; sy++ {
				p := (y*scale+sy)*src.Stride + x*scale*4
				for sx := 0; sx < scale; sx++ {
					for channel := range 4 {
						sum[channel] += uint32(src.Pix[p+sx*4+channel])
					}
				}
			}
			p := y*dst.Stride + x*4
			for channel := range 4 {
				dst.Pix[p+channel] = uint8((sum[channel] + count/2) / count)
			}
		}
	}
	return dst
}
