package render

import (
	"fmt"
	"image"
)

const (
	// MaxSupersamplingPixels bounds the internal high-resolution canvas when
	// Supersampling is greater than one. Existing 0/1 rendering is unchanged.
	MaxSupersamplingPixels = 16 << 20
	// MaxSupersamplingWorkerBytes bounds conservative simultaneously live pixel
	// payload per worker, including resolve output and vector/layer scratch.
	// It excludes input geometry/assets, Go allocator/GC and sink-owned images.
	MaxSupersamplingWorkerBytes int64 = 448 << 20
	// MaxSupersamplingTotalBytes includes every configured worker and the
	// RenderScene queue of up to twice that worker count of output frames.
	MaxSupersamplingTotalBytes int64 = 1 << 30
)

func (c Config) samplingScale() int {
	if c.Supersampling == 0 {
		return 1
	}
	return c.Supersampling
}

// samplingConfig checks arithmetic and budgets before any raster allocation.
// The caller has validated ordinary dimensions/workers first. Returned config
// is private working state; FrameWidth and CurveTolerance retain world/view and
// high-resolution-pixel semantics, respectively.
func (c Config) samplingConfig() (Config, error) {
	scale := c.samplingScale()
	if scale != 1 && scale != 2 && scale != 4 && scale != 8 {
		return c, fmt.Errorf("supersampling must be 0 (default), 1, 2, 4 or 8")
	}
	if scale == 1 {
		return c, nil
	}
	if c.Width > 16384/scale || c.Height > 16384/scale {
		return c, fmt.Errorf("supersampling working dimensions exceed 16384")
	}
	w, h := c.Width*scale, c.Height*scale
	pixels := int64(w) * int64(h)
	if pixels > MaxSupersamplingPixels {
		return c, fmt.Errorf("supersampling working pixels exceed %d", MaxSupersamplingPixels)
	}
	output := int64(c.Width) * int64(c.Height)
	// At most 28 bytes per working pixel: high canvas 4; both vector
	// rasterizers with retained fixed/float/mask scratch 16; gradient mask 1;
	// clipped RGBA layer and two clip alpha masks 6; bitmap alpha mask 1.
	// Per worker include 4 bytes/output pixel for resolve; aggregate includes
	// another 8 for the two outstanding output frames per configured worker.
	workerBytes := 28*pixels + 4*output
	if workerBytes > MaxSupersamplingWorkerBytes {
		return c, fmt.Errorf("supersampling worker pixel payload exceeds %d bytes", MaxSupersamplingWorkerBytes)
	}
	totalBytes := (workerBytes + 8*output) * int64(c.Workers)
	if totalBytes > MaxSupersamplingTotalBytes {
		return c, fmt.Errorf("supersampling configured-worker pixel payload exceeds %d bytes", MaxSupersamplingTotalBytes)
	}
	c.Width, c.Height, c.Supersampling = w, h, 1
	return c, nil
}

// resolveBox averages already composited premultiplied sRGB bytes, including
// alpha, rounding each channel to nearest with half upward. No unpremultiplying
// or additional alpha weighting is permitted here: that would create fringes.
// Width/height are exact integer multiples even for odd output dimensions.
func resolveBox(src *image.RGBA, width, height, scale int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	count := uint32(scale * scale)
	for y := range height {
		for x := range width {
			var sum [4]uint32
			for sy := range scale {
				i := (y*scale+sy)*src.Stride + x*scale*4
				for range scale {
					for channel := range 4 {
						sum[channel] += uint32(src.Pix[i+channel])
					}
					i += 4
				}
			}
			i := y*dst.Stride + x*4
			for channel := range 4 {
				dst.Pix[i+channel] = uint8((sum[channel] + count/2) / count)
			}
		}
	}
	return dst
}
