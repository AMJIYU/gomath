package render

import (
	"fmt"
	"gomath"
	"gomath/geom"
	"gomath/path"
	"image"
	"image/draw"
)

// Parity fills and all clip masks share the existing per-frame counters. There
// is one alpha rectangle shared with gradient coverage per worker, bounded
// before allocation (no extra retained pixel payload in SSAA estimates); errors
// propagate to Render, which returns no partially painted output.
func (r *Rasterizer) paintParityFill(dst *image.RGBA, outline path.Path, region image.Rectangle, style gomath.Style, opacity float64) error {
	pixels := region.Dx() * region.Dy()
	if pixels > MaxClipRasterPixels {
		return fmt.Errorf("evenodd fill exceeds shared mask pixel budget")
	}
	if err := r.evenOddClipWork(pixels); err != nil {
		return fmt.Errorf("evenodd fill: %w", err)
	}
	if r.gradientMask == nil || cap(r.gradientMask.Pix) < pixels {
		r.gradientMask = image.NewAlpha(image.Rect(0, 0, region.Dx(), region.Dy()))
	} else {
		r.gradientMask.Pix = r.gradientMask.Pix[:pixels]
		r.gradientMask.Stride = region.Dx()
		r.gradientMask.Rect = image.Rect(0, 0, region.Dx(), region.Dy())
		clear(r.gradientMask.Pix)
	}
	if err := r.paintEvenOddClip(r.gradientMask, outline); err != nil {
		return fmt.Errorf("evenodd fill: %w", err)
	}
	if style.FillGradient != nil {
		if err := r.gradientPaint(style.FillGradient, region); err != nil {
			return err
		}
		return r.compositeGradientMask(dst, style.FillGradient, region, opacity, r.gradientMask)
	}
	draw.DrawMask(dst, region, image.NewUniform(style.Fill.NRGBA(opacity)), image.Point{}, r.gradientMask, image.Point{}, draw.Over)
	return nil
}

// Stroke outlines always use the original nonzero stroke rasterizer. An
// evenodd object's centerline flattening is bounded and charged to the same
// point budget as its fill/ancestor masks, even for a stroke-only object.
func (r *Rasterizer) parityStrokeLines(outline path.Path) ([]path.Polyline, error) {
	var lines []path.Polyline
	inverse := r.config.FrameWidth / float64(r.config.Width)
	for _, contour := range outline.Contours {
		if r.clipPoints == MaxClipRasterPoints {
			return nil, fmt.Errorf("evenodd stroke exceeds shared flattened point budget")
		}
		r.clipPoints++
		if err := r.evenOddClipWork(1); err != nil {
			return nil, err
		}
		if _, _, err := r.clipPoint(contour.Start); err != nil {
			return nil, err
		}
		line := path.Polyline{Points: []geom.Vec3{contour.Start}, Closed: contour.Closed}
		emit := func(x, y float64) error {
			if err := r.evenOddClipWork(1); err != nil {
				return err
			}
			if len(line.Points) == cap(line.Points) {
				grown := make([]geom.Vec3, len(line.Points), min(MaxClipRasterPoints, 2*cap(line.Points)))
				copy(grown, line.Points)
				line.Points = grown
			}
			line.Points = append(line.Points, geom.V((x+float64(r.origin.X)-float64(r.config.Width)/2)*inverse,
				(float64(r.config.Height)/2-y-float64(r.origin.Y))*inverse))
			return nil
		}
		for _, curve := range contour.Curves {
			if err := r.clipCurveTo(curve, 0, emit); err != nil {
				return nil, fmt.Errorf("evenodd stroke: %w", err)
			}
		}
		if len(lines) == cap(lines) {
			grown := make([]path.Polyline, len(lines), min(MaxClipRasterPoints, max(1, 2*cap(lines))))
			copy(grown, lines)
			lines = grown
		}
		lines = append(lines, line)
	}
	return lines, nil
}
