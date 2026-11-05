package render

import (
	"fmt"
	"golang.org/x/image/vector"
	"gomath"
	"gomath/geom"
	"gomath/path"
	"image"
	"image/draw"
	"math"
	"sort"
)

type drawable struct {
	object  *gomath.Object
	opacity float64
	order   int
	clips   []*gomath.Clip
}

// clipRasterScratch belongs to one Rasterizer/worker. paintClipped calls paint,
// which never recursively clips; these images are borrowed only until that
// drawable has been composited. No returned frame aliases this storage.
// One shared allocation retains at most 6*MaxClipRasterPixels bytes, matching
// the existing RGBA + two-alpha scratch budget (also charged by supersampling).
// Rebuild tight strides/rectangles on every use, including when an equally sized
// rectangle changes shape or has a different absolute origin.
type clipRasterScratch struct {
	pixels     []byte
	layer      image.RGBA
	mask, part image.Alpha
}

// Rasterizer is reusable within one worker, but is not safe for concurrent use.
type Rasterizer struct {
	config               Config
	fill, stroke         *vector.Rasterizer
	origin               image.Point
	clipWork, clipPoints int
	gradientWork         int
	gradientMask         *image.Alpha
	clipScratch          *clipRasterScratch
}

func NewRasterizer(c Config) (*Rasterizer, error) {
	if e := c.Validate(); e != nil {
		return nil, e
	}
	return &Rasterizer{config: c, fill: vector.NewRasterizer(0, 0), stroke: vector.NewRasterizer(0, 0)}, nil
}
func (r *Rasterizer) point(v geom.Vec3) (float32, float32) {
	s := float64(r.config.Width) / r.config.FrameWidth
	return float32(float64(r.config.Width)/2 + v.X*s - float64(r.origin.X)), float32(float64(r.config.Height)/2 - v.Y*s - float64(r.origin.Y))
}

// Render returns a fresh output-sized frame. Supersampling is explicit and
// never changes the caller's config or persists a camera's effective view.
func (r *Rasterizer) Render(frame *gomath.Frame) (*image.RGBA, error) {
	if r.config.samplingScale() == 1 {
		return r.renderFrame(frame)
	}
	workingConfig, err := r.config.samplingConfig()
	if err != nil {
		return nil, err
	}
	// The private working copy shares reusable raster scratch, but keeps high
	// dimensions, camera state, origin and counters out of the base Rasterizer.
	// Call renderFrame directly: it renders once and cannot recursively sample.
	working := *r
	working.config = workingConfig
	high, err := working.renderFrame(frame)
	r.gradientMask = working.gradientMask
	r.clipScratch = working.clipScratch
	if err != nil {
		return nil, err
	}
	return resolveBox(high, r.config.Width, r.config.Height, r.config.samplingScale()), nil
}

func (r *Rasterizer) renderFrame(frame *gomath.Frame) (*image.RGBA, error) {
	if frame == nil {
		return nil, fmt.Errorf("nil frame")
	}
	projected, effective, err := ProjectFrame(frame, r.config)
	if err != nil {
		return nil, err
	}
	previous := r.config
	r.config = effective
	defer func() { r.config = previous }()
	frame = projected
	r.clipWork, r.clipPoints = 0, 0
	r.gradientWork = 0
	c := r.config
	dst := image.NewRGBA(image.Rect(0, 0, c.Width, c.Height))
	draw.Draw(dst, dst.Bounds(), image.NewUniform(c.Background.NRGBA(1)), image.Point{}, draw.Src)
	var items []drawable
	seen := map[*gomath.Object]bool{}
	var walk func(*gomath.Object, float64, []*gomath.Clip) error
	walk = func(o *gomath.Object, opacity float64, clips []*gomath.Clip) error {
		if o == nil {
			return fmt.Errorf("nil frame object")
		}
		if seen[o] {
			return fmt.Errorf("frame contains repeated object or cycle")
		}
		seen[o] = true
		if o.Clip != nil {
			if len(clips) == gomath.MaxClipDepth {
				return fmt.Errorf("render exceeds %d intersecting clips", gomath.MaxClipDepth)
			}
			// Each drawable keeps its own ancestry; later siblings must not
			// overwrite a prior leaf's clip slice through shared capacity.
			clips = append(append([]*gomath.Clip(nil), clips...), o.Clip)
		}
		if e := o.ValidateVisual(); e != nil {
			return e
		}
		opacity *= o.Style.Opacity
		if math.IsNaN(opacity) || math.IsInf(opacity, 0) || opacity < 0 || opacity > 1 {
			return fmt.Errorf("invalid opacity")
		}
		if len(o.Path.Contours) > 0 && opacity > 0 {
			items = append(items, drawable{object: o, opacity: opacity, order: len(items), clips: clips})
		}
		for _, child := range o.Children {
			if e := walk(child, opacity, clips); e != nil {
				return e
			}
		}
		return nil
	}
	for _, o := range frame.Objects {
		if e := walk(o, 1, nil); e != nil {
			return nil, e
		}
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].object.ZIndex < items[j].object.ZIndex })
	for _, item := range items {
		if len(item.clips) > 0 {
			if err := r.paintClipped(dst, item); err != nil {
				return nil, err
			}
			continue
		}
		if e := r.paint(dst, item); e != nil {
			return nil, e
		}
	}
	return dst, nil
}
func (r *Rasterizer) paint(dst *image.RGBA, item drawable) error {
	o := item.object
	if o.Bitmap != nil {
		return r.paintBitmap(dst, item)
	}
	s := o.Style
	if !finite(s.Reveal) || s.Reveal < 0 || s.Reveal > 1 || !finite(s.StrokeWidth) || s.StrokeWidth < 0 {
		return fmt.Errorf("invalid reveal or stroke width")
	}
	if s.Reveal <= 0 {
		return nil
	}
	p := o.Path
	if s.Reveal < 1 {
		p = p.Partial(s.Reveal)
	}
	c := r.config
	// Rasterize only affected pixels, including the configured cap/miter extent
	// and the antialias fringe.
	bounds := p.Bounds()
	if !bounds.Valid {
		return nil
	}
	scale := float64(c.Width) / c.FrameWidth
	pad := 2.0
	if s.Stroke.A > 0 {
		pad += s.StrokeExtent() * scale
	}
	clip := func(v float64, limit int) float64 { return math.Max(0, math.Min(float64(limit), v)) }
	x0 := clip(float64(c.Width)/2+bounds.Min.X*scale-pad, c.Width)
	x1 := clip(float64(c.Width)/2+bounds.Max.X*scale+pad, c.Width)
	y0 := clip(float64(c.Height)/2-bounds.Max.Y*scale-pad, c.Height)
	y1 := clip(float64(c.Height)/2-bounds.Min.Y*scale+pad, c.Height)
	region := image.Rect(int(math.Floor(x0)), int(math.Floor(y0)), int(math.Ceil(x1)), int(math.Ceil(y1))).Intersect(dst.Bounds())
	if region.Empty() {
		return nil
	}
	r.origin = region.Min
	tolerance := c.CurveTolerance
	if tolerance == 0 {
		tolerance = .02
	}
	var lines []path.Polyline
	if s.FillRule == path.NonZero {
		lines = p.Flatten(tolerance / scale)
	} else if s.Stroke.A > 0 && s.StrokeWidth > 0 {
		var err error
		lines, err = r.parityStrokeLines(p)
		if err != nil {
			return err
		}
	}
	if s.Fill.A > 0 || s.FillGradient != nil {
		if s.FillRule == path.EvenOdd {
			if err := r.paintParityFill(dst, p, region, s, item.opacity); err != nil {
				return err
			}
		} else {
			z := r.fill
			z.Reset(region.Dx(), region.Dy())
			for _, contour := range lines {
				if len(contour.Points) == 0 {
					continue
				}
				x, y := r.point(contour.Points[0])
				z.MoveTo(x, y)
				for _, v := range contour.Points[1:] {
					x, y := r.point(v)
					z.LineTo(x, y)
				}
				z.ClosePath()
			}
			if s.FillGradient == nil {
				z.Draw(dst, region, image.NewUniform(s.Fill.NRGBA(item.opacity)), image.Point{})
			} else {
				if err := r.paintGradient(dst, s.FillGradient, region, item.opacity); err != nil {
					return err
				}
			}
		}
	}
	if s.Stroke.A > 0 && s.StrokeWidth > 0 {
		z := r.stroke
		z.Reset(region.Dx(), region.Dy())
		width := s.StrokeWidth * float64(c.Width) / c.FrameWidth
		if s.Dash == nil {
			for _, line := range lines {
				if err := r.strokeStyledLine(z, line, width/2, s); err != nil {
					return err
				}
			}
		} else {
			pieces, err := dashLines(lines, s.Dash)
			if err != nil {
				return err
			}
			solid := s
			solid.Dash = nil
			for _, piece := range pieces {
				if err := r.strokeDashPiece(z, piece, width/2, solid); err != nil {
					return err
				}
			}
		}
		z.Draw(dst, region, image.NewUniform(s.Stroke.NRGBA(item.opacity)), image.Point{})
	}
	return nil
}
func finite(f float64) bool { return !math.IsNaN(f) && !math.IsInf(f, 0) }
