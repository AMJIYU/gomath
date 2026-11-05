package render

import (
	"fmt"
	"gomath"
	"gomath/geom"
	"image"
)

const (
	// One byte of reusable coverage scratch per pixel; no full RGBA paint image.
	MaxGradientRasterPixels = 16 << 20
	MaxGradientRasterWork   = 64 << 20
)

func (r *Rasterizer) gradientPaint(g *gomath.Gradient, region image.Rectangle) error {
	pixels := region.Dx() * region.Dy()
	if pixels > MaxGradientRasterPixels || pixels > MaxGradientRasterWork-r.gradientWork {
		return fmt.Errorf("gradient raster pixel/work budget exceeded")
	}
	scale := float64(r.config.Width) / r.config.FrameWidth
	if !finite(scale) || scale <= 0 {
		return fmt.Errorf("gradient raster scale exceeds numeric range")
	}
	for _, x := range []int{region.Min.X, region.Max.X - 1} {
		for _, y := range []int{region.Min.Y, region.Max.Y - 1} {
			p := geom.V((float64(x)+.5-float64(r.config.Width)/2)/scale, (float64(r.config.Height)/2-float64(y)-.5)/scale)
			if _, err := g.ColorAt(p); err != nil {
				return err
			}
		}
	}
	r.gradientWork += pixels
	return nil
}

// paintGradient samples the field through the legacy nonzero vector coverage.
// The same path is used for standalone and clipped vector layers,
// with absolute pixel origins. Direct RGBA compositing avoids image.Image's
// general source/destination interface allocations for every covered pixel.
func (r *Rasterizer) paintGradient(dst *image.RGBA, g *gomath.Gradient, region image.Rectangle, opacity float64) error {
	if err := r.gradientPaint(g, region); err != nil {
		return err
	}
	pixels := region.Dx() * region.Dy()
	if r.gradientMask == nil || cap(r.gradientMask.Pix) < pixels {
		r.gradientMask = image.NewAlpha(image.Rect(0, 0, region.Dx(), region.Dy()))
	} else {
		r.gradientMask.Pix = r.gradientMask.Pix[:pixels]
		r.gradientMask.Stride = region.Dx()
		r.gradientMask.Rect = image.Rect(0, 0, region.Dx(), region.Dy())
		clear(r.gradientMask.Pix)
	}
	mask := r.gradientMask
	r.fill.Draw(mask, mask.Bounds(), image.White, image.Point{})
	return r.compositeGradientMask(dst, g, region, opacity, mask)
}

// The caller validates/charges gradientPaint before compositing. Parity fills
// supply their own bounded coverage instead of reading the nonzero rasterizer.
func (r *Rasterizer) compositeGradientMask(dst *image.RGBA, g *gomath.Gradient, region image.Rectangle, opacity float64, mask *image.Alpha) error {
	inverseScale := r.config.FrameWidth / float64(r.config.Width)
	for y := range region.Dy() {
		wy := (float64(r.config.Height)/2 - float64(y+region.Min.Y) - .5) * inverseScale
		for x := range region.Dx() {
			coverage := uint32(mask.Pix[y*mask.Stride+x]) * 0x101
			if coverage == 0 {
				continue
			}
			wx := (float64(x+region.Min.X) + .5 - float64(r.config.Width)/2) * inverseScale
			sample, err := g.ColorAt(geom.V(wx, wy))
			if err != nil {
				return err
			}
			c := sample.NRGBA(opacity)
			sr, sg, sb, sa := c.RGBA()
			a := 0xffff - sa*coverage/0xffff
			i := dst.PixOffset(x+region.Min.X, y+region.Min.Y)
			for channel, source := range [4]uint32{sr, sg, sb, sa} {
				dst.Pix[i+channel] = uint8(((uint32(dst.Pix[i+channel])*0x101*a + source*coverage) / 0xffff) >> 8)
			}
		}
	}
	return nil
}
