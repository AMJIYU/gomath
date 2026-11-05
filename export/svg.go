package export

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"gomath"
	"gomath/path"
	"gomath/render"
	"html"
	"image/png"
	"io"
	"sort"
)

// MaxSVGGradientStops bounds emitted paint-server data per document.
const MaxSVGGradientStops = 65536

func SVG(target string, frame *gomath.Frame, c render.Config) (err error) {
	if frame == nil {
		return fmt.Errorf("nil frame")
	}
	if e := c.Validate(); e != nil {
		return e
	}
	frame, c, err = render.ProjectFrame(frame, c)
	if err != nil {
		return err
	}
	f, e := newAtomicFile(target)
	if e != nil {
		return e
	}
	defer func() {
		if err != nil {
			_ = f.abort()
		}
	}()
	if e := writeSVG(f.file, frame, c); e != nil {
		return e
	}
	return f.commit()
}
func writeSVG(w io.Writer, frame *gomath.Frame, c render.Config) error {
	var writeErr error
	put := func(format string, args ...any) {
		if writeErr == nil {
			_, writeErr = fmt.Fprintf(w, format, args...)
		}
	}
	put("<svg xmlns=\"http://www.w3.org/2000/svg\" width=\"%d\" height=\"%d\" viewBox=\"0 0 %d %d\">\n", c.Width, c.Height, c.Width, c.Height)
	bg := c.Background.NRGBA(1)
	put("<rect width=\"100%%\" height=\"100%%\" fill=\"#%02x%02x%02x\" fill-opacity=\"%.6g\"/>\n", bg.R, bg.G, bg.B, c.Background.A)
	type item struct {
		o       *gomath.Object
		opacity float64
		clips   []*gomath.Clip
	}
	var items []item
	clipIDs := map[*gomath.Clip]int{}
	var clipOrder []*gomath.Clip
	seen := map[*gomath.Object]bool{}
	var walk func(*gomath.Object, float64, []*gomath.Clip) error
	walk = func(o *gomath.Object, a float64, clips []*gomath.Clip) error {
		if o == nil || seen[o] {
			return fmt.Errorf("invalid object tree")
		}
		seen[o] = true
		if e := o.ValidateVisual(); e != nil {
			return e
		}
		if o.Clip != nil {
			if len(clips) == gomath.MaxClipDepth {
				return fmt.Errorf("SVG exceeds %d intersecting clips", gomath.MaxClipDepth)
			}
			clips = append(append([]*gomath.Clip(nil), clips...), o.Clip)
			if _, exists := clipIDs[o.Clip]; !exists {
				clipIDs[o.Clip] = len(clipOrder)
				clipOrder = append(clipOrder, o.Clip)
			}
		}
		a *= o.Style.Opacity
		items = append(items, item{o, a, clips})
		for _, child := range o.Children {
			if e := walk(child, a, clips); e != nil {
				return e
			}
		}
		return nil
	}
	for _, o := range frame.Objects {
		if e := walk(o, 1, nil); e != nil {
			return e
		}
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].o.ZIndex < items[j].o.ZIndex })
	scale := float64(c.Width) / c.FrameWidth
	writePath := func(outline path.Path, closeContours bool) {
		for _, contour := range outline.Contours {
			put("M%.9g %.9g", contour.Start.X, contour.Start.Y)
			for _, curve := range contour.Curves {
				put("C%.9g %.9g %.9g %.9g %.9g %.9g", curve.P1.X, curve.P1.Y, curve.P2.X, curve.P2.Y, curve.P3.X, curve.P3.Y)
			}
			if closeContours || contour.Closed {
				put("Z")
			}
		}
	}
	if len(clipOrder) > 0 {
		put("<defs>\n")
		for _, clip := range clipOrder {
			rule := "nonzero"
			if clip.FillRule() == path.EvenOdd {
				rule = "evenodd"
			}
			put("<clipPath id=\"gomath-clip-%d\" clipPathUnits=\"userSpaceOnUse\"><path clip-rule=\"%s\" transform=\"translate(%g %g) scale(%g %g)\" d=\"", clipIDs[clip], rule, float64(c.Width)/2, float64(c.Height)/2, scale, -scale)
			writePath(clip.Path(), true)
			put("\"/></clipPath>\n")
		}
		put("</defs>\n")
	}
	var gradientOrder []*gomath.Gradient
	gradientIDs := map[*gomath.Gradient]int{}
	totalStops := 0
	for _, it := range items {
		g := it.o.Style.FillGradient
		if g == nil {
			continue
		}
		if _, exists := gradientIDs[g]; exists {
			continue
		}
		stops := g.Stops()
		if len(stops) > MaxSVGGradientStops-totalStops {
			return fmt.Errorf("SVG gradient stop budget exceeded")
		}
		totalStops += len(stops)
		gradientIDs[g] = len(gradientOrder)
		gradientOrder = append(gradientOrder, g)
	}
	if len(gradientOrder) > 0 {
		put("<defs>\n")
		for _, g := range gradientOrder {
			tag, coords := "linearGradient", `x1="0" y1="0" x2="1" y2="0"`
			if g.Kind() == gomath.RadialGradient {
				tag, coords = "radialGradient", `cx="0" cy="0" r="1" fx="0" fy="0"`
			}
			m := g.Matrix()
			put("<%s id=\"gomath-gradient-%d\" gradientUnits=\"userSpaceOnUse\" spreadMethod=\"pad\" color-interpolation=\"sRGB\" %s gradientTransform=\"matrix(%.17g %.17g %.17g %.17g %.17g %.17g)\">", tag, gradientIDs[g], coords, m[0], m[1], m[2], m[3], m[4], m[5])
			for _, stop := range g.Stops() {
				c := stop.Color
				put("<stop offset=\"%.17g\" stop-color=\"rgb(%.12g%%,%.12g%%,%.12g%%)\" stop-opacity=\"%.17g\"/>", stop.Offset, 100*c.R, 100*c.G, 100*c.B, c.A)
			}
			put("</%s>\n", tag)
		}
		put("</defs>\n")
	}
	encoded := map[*gomath.Bitmap]string{}
	for _, it := range items {
		o := it.o
		s := o.Style
		if s.Reveal <= 0 {
			continue
		}
		for _, clip := range it.clips {
			put("<g clip-path=\"url(#gomath-clip-%d)\">", clipIDs[clip])
		}
		closeClips := func() {
			for range it.clips {
				put("</g>")
			}
			if len(it.clips) > 0 {
				put("\n")
			}
		}
		if o.Bitmap != nil {
			tl, tr, bl, err := o.ImageCorners()
			if err != nil {
				return err
			}
			data, ok := encoded[o.Bitmap]
			if !ok {
				var buffer bytes.Buffer
				if err := png.Encode(&buffer, o.Bitmap); err != nil {
					return err
				}
				data = base64.StdEncoding.EncodeToString(buffer.Bytes())
				encoded[o.Bitmap] = data
			}
			width, height := o.Bitmap.Bounds().Dx(), o.Bitmap.Bounds().Dy()
			u, v := tr.Sub(tl).Mul(scale/float64(width)), bl.Sub(tl).Mul(scale/float64(height))
			put("<image aria-label=\"%s\" width=\"%d\" height=\"%d\" opacity=\"%.9g\" preserveAspectRatio=\"none\" transform=\"matrix(%.9g %.9g %.9g %.9g %.9g %.9g)\" href=\"data:image/png;base64,%s\"/>\n",
				html.EscapeString(o.Name), width, height, it.opacity, u.X, -u.Y, v.X, -v.Y,
				float64(c.Width)/2+tl.X*scale, float64(c.Height)/2-tl.Y*scale, data)
			closeClips()
			continue
		}
		p := o.Path
		if s.Reveal < 1 {
			p = p.Partial(s.Reveal)
		}
		fill, stroke := s.Fill.NRGBA(1), s.Stroke.NRGBA(1)
		attributes, err := StrokeAttributes(s)
		if err != nil {
			return err
		}
		if s.FillRule == path.EvenOdd {
			attributes += ` fill-rule="evenodd"`
		}
		if s.FillGradient == nil {
			put("<path aria-label=\"%s\" fill=\"#%02x%02x%02x\" fill-opacity=\"%.6g\" stroke=\"#%02x%02x%02x\" stroke-opacity=\"%.6g\" stroke-width=\"%.6g\" %s transform=\"translate(%g %g) scale(%g %g)\" d=\"", html.EscapeString(o.Name), fill.R, fill.G, fill.B, s.Fill.A*it.opacity, stroke.R, stroke.G, stroke.B, s.Stroke.A*it.opacity, s.StrokeWidth, attributes, float64(c.Width)/2, float64(c.Height)/2, scale, -scale)
		} else {
			put("<path aria-label=\"%s\" fill=\"url(#gomath-gradient-%d)\" fill-opacity=\"%.17g\" stroke=\"#%02x%02x%02x\" stroke-opacity=\"%.6g\" stroke-width=\"%.6g\" %s transform=\"translate(%g %g) scale(%g %g)\" d=\"", html.EscapeString(o.Name), gradientIDs[s.FillGradient], it.opacity, stroke.R, stroke.G, stroke.B, s.Stroke.A*it.opacity, s.StrokeWidth, attributes, float64(c.Width)/2, float64(c.Height)/2, scale, -scale)
		}
		writePath(p, false)
		put("\"/>\n")
		closeClips()
	}
	put("</svg>\n")
	return writeErr
}
