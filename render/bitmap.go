package render

import (
	"fmt"
	"gomath"
	"image"
	"math"
)

// paintBitmap inverse-maps pixel centers through an affine image plane. An
// independently rasterized coverage mask antialiases the transformed boundary.
// Samples and composition use premultiplied sRGB, matching vector composition.
func (r *Rasterizer) paintBitmap(dst *image.RGBA, item drawable) error {
	o := item.object
	tl, tr, bl, err := o.ImageCorners()
	if err != nil {
		return err
	}
	c := r.config
	b := o.Path.Bounds()
	scale := float64(c.Width) / c.FrameWidth
	clip := func(v float64, limit int) float64 { return math.Max(0, math.Min(float64(limit), v)) }
	x0 := clip(float64(c.Width)/2+b.Min.X*scale-1, c.Width)
	x1 := clip(float64(c.Width)/2+b.Max.X*scale+1, c.Width)
	y0 := clip(float64(c.Height)/2-b.Max.Y*scale-1, c.Height)
	y1 := clip(float64(c.Height)/2-b.Min.Y*scale+1, c.Height)
	region := image.Rect(int(math.Floor(x0)), int(math.Floor(y0)), int(math.Ceil(x1)), int(math.Ceil(y1))).Intersect(dst.Bounds())
	if region.Empty() {
		return nil
	}
	r.origin = region.Min
	px, py := r.point(tl)
	qx, qy := r.point(tr)
	sx, sy := r.point(bl)
	ux, uy := float64(qx-px), float64(qy-py)
	vx, vy := float64(sx-px), float64(sy-py)
	det := ux*vy - uy*vx
	if !finite(det) || !finite(float64(px)) || !finite(float64(py)) {
		return fmt.Errorf("image projection exceeds raster coordinate range")
	}
	if math.Abs(det) < 1e-12 {
		return nil // A collapsed affine image plane has no visible area.
	}
	z := r.fill
	z.Reset(region.Dx(), region.Dy())
	contour := o.Path.Contours[0]
	x, y := r.point(contour.Start)
	z.MoveTo(x, y)
	for _, curve := range contour.Curves {
		x, y = r.point(curve.P3)
		z.LineTo(x, y)
	}
	z.ClosePath()
	mask := image.NewAlpha(image.Rect(0, 0, region.Dx(), region.Dy()))
	z.Draw(mask, mask.Bounds(), image.White, image.Point{})
	width, height := o.Bitmap.Bounds().Dx(), o.Bitmap.Bounds().Dy()
	for y := range region.Dy() {
		for x := range region.Dx() {
			coverage := mask.Pix[y*mask.Stride+x]
			if coverage == 0 {
				continue
			}
			dx, dy := float64(x)+.5-float64(px), float64(y)+.5-float64(py)
			u, v := (dx*vy-dy*vx)/det, (dy*ux-dx*uy)/det
			sample := bilinear(o.Bitmap, u*float64(width)-.5, v*float64(height)-.5)
			factor := float64(coverage) / 255 * item.opacity
			alpha := sample[3] * factor / 255
			offset := dst.PixOffset(x+region.Min.X, y+region.Min.Y)
			for channel := range 4 {
				value := sample[channel]*factor + float64(dst.Pix[offset+channel])*(1-alpha)
				dst.Pix[offset+channel] = uint8(math.Round(math.Max(0, math.Min(255, value))))
			}
		}
	}
	return nil
}

// Edge extension keeps partially covered edge pixels from darkening during
// filtering. The geometric coverage mask determines actual image coverage.
func bilinear(bitmap *gomath.Bitmap, x, y float64) [4]float64 {
	b := bitmap.Bounds()
	x = math.Max(0, math.Min(float64(b.Dx()-1), x))
	y = math.Max(0, math.Min(float64(b.Dy()-1), y))
	x0, y0 := int(math.Floor(x)), int(math.Floor(y))
	x1, y1 := min(b.Dx()-1, x0+1), min(b.Dy()-1, y0+1)
	dx, dy := x-float64(x0), y-float64(y0)
	colors := [4][4]uint8{}
	for i, p := range [4]image.Point{{x0, y0}, {x1, y0}, {x0, y1}, {x1, y1}} {
		c := bitmap.RGBAAt(p.X, p.Y)
		colors[i] = [4]uint8{c.R, c.G, c.B, c.A}
	}
	weights := [4]float64{(1 - dx) * (1 - dy), dx * (1 - dy), (1 - dx) * dy, dx * dy}
	var result [4]float64
	for i, weight := range weights {
		for channel := range 4 {
			result[channel] += float64(colors[i][channel]) * weight
		}
	}
	return result
}
