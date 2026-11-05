package svg

import (
	"fmt"
	"gomath/geom"
	"gomath/path"
	"gomath/text"
)

// A scope measures original geometry in the target's own user space. Matrices
// travel alongside the expansion CTM, so neither inverse CTMs (singular ones
// are legal) nor transformed child AABBs are needed. The bounds are private to
// this expansion; clips, paint, stroke and opacity never enter this ledger.
type bboxScope struct {
	box    *geom.Bounds
	matrix geom.Mat4
}

func (ctx expansionContext) bboxTarget(s *expansion) (expansionContext, *geom.Bounds, error) {
	// Empty containers also charge work, independently of their descendants.
	if err := s.document.clipBudget.add(0, 0); err != nil {
		return ctx, nil, err
	}
	box := new(geom.Bounds)
	scopes := make([]bboxScope, len(ctx.bboxes), len(ctx.bboxes)+1)
	copy(scopes, ctx.bboxes)
	ctx.bboxes = append(scopes, bboxScope{box, geom.Identity()})
	return ctx, box, nil
}

func bboxOutlineTransform(p path.Path, matrix geom.Mat4) (path.Path, error) {
	if !finiteMatrix(matrix) {
		return path.Path{}, fmt.Errorf("SVG bbox transform exceeds finite coordinate range")
	}
	q := p.Transform(matrix)
	for i, contour := range p.Contours {
		for j, curve := range contour.Curves {
			if curve == path.Line(curve.P0, curve.P3) {
				// Preserve line provenance despite rounding of affine controls.
				q.Contours[i].Curves[j] = path.Line(matrix.Apply(curve.P0), matrix.Apply(curve.P3))
			}
		}
	}
	if err := q.Validate(); err != nil {
		return path.Path{}, fmt.Errorf("SVG bbox geometry: %w", err)
	}
	return q, nil
}

func (s *expansion) collectBBox(p path.Path, ctx expansionContext) error {
	for _, scope := range ctx.bboxes {
		// Charge the scan and temporary transformed outline before allocating.
		for range 2 {
			if err := s.document.clipBudget.geometry(p); err != nil {
				return err
			}
		}
		q, err := bboxOutlineTransform(p, scope.matrix)
		if err != nil {
			return err
		}
		box, err := bboxGeometryBounds(q)
		if err != nil {
			return err
		}
		if box.Valid {
			scope.box.Include(box.Min)
			scope.box.Include(box.Max)
		}
	}
	return nil
}

func bboxRectangle(x, y, width, height float64) path.Path {
	var b path.Builder
	b.MoveTo(geom.V(x, y))
	b.LineTo(geom.V(x+width, y))
	b.LineTo(geom.V(x+width, y+height))
	b.LineTo(geom.V(x, y+height))
	b.Close()
	return b.Path()
}

func directBBoxUse(e *element) bool {
	if e.target == nil {
		return false
	}
	switch e.target.name.Local {
	case "path", "rect", "circle", "ellipse", "line", "polygon", "polyline", "image":
		return true
	}
	return false
}

// Temporary glyph outlines used to obtain advances/kerning spend the same
// expansion, text, and clip geometry ledgers as the painted outlines. A cache
// hit avoids this work; it never returns a mutable outline to another instance.
func (s *expansion) bboxMetricLayout(value string, font *text.Font, size float64) (text.Metrics, error) {
	object, metrics, err := s.outlineText(value, font, size)
	if err != nil {
		return text.Metrics{}, err
	}
	s.text.glyphs += len(object.Children)
	if s.text.glyphs > maxTextGlyphs {
		return text.Metrics{}, fmt.Errorf("SVG text glyph budget exceeds %d (including bbox metrics)", maxTextGlyphs)
	}
	if err := s.textCount(len(object.Children)); err != nil {
		return text.Metrics{}, err
	}
	for _, glyph := range object.Children {
		if err := s.document.clipBudget.geometry(glyph.Path); err != nil {
			return text.Metrics{}, err
		}
		for _, contour := range glyph.Path.Contours {
			s.text.segments += 1 + len(contour.Curves)
		}
		if s.text.segments > maxTextSegments {
			return text.Metrics{}, fmt.Errorf("SVG text outline segment budget exceeds %d (including bbox metrics)", maxTextSegments)
		}
	}
	return metrics, nil
}

type bboxTextMetricKey struct {
	font *text.Font
	size float64
	a, b rune // b == 0: single-glyph metrics; otherwise a pair's kerning
}

// Metrics come from the SAME outline API and font instance as painting. The
// per-import cache has at most maxTextLayouts entries and every miss spends
// that existing layout budget; it is never a second unbounded font parser.
func (s *expansion) bboxTextMetrics(a, b rune, font *text.Font, size float64) (text.Metrics, error) {
	key := bboxTextMetricKey{font, size, a, b}
	if m, ok := s.bboxText[key]; ok {
		return m, nil
	}
	var m text.Metrics
	if b == 0 {
		metrics, err := s.bboxMetricLayout(string(a), font, size)
		if err != nil {
			return m, err
		}
		m = metrics
	} else {
		first, err := s.bboxTextMetrics(a, 0, font, size)
		if err != nil {
			return m, err
		}
		second, err := s.bboxTextMetrics(b, 0, font, size)
		if err != nil {
			return m, err
		}
		pair, err := s.bboxMetricLayout(string([]rune{a, b}), font, size)
		if err != nil {
			return m, err
		}
		m.Width = pair.Width - first.Width - second.Width
	}
	if s.bboxText == nil {
		s.bboxText = make(map[bboxTextMetricKey]text.Metrics)
	}
	s.bboxText[key] = m
	return m, nil
}

// SVG 2 BoundingBoxes specifies full glyph cells, including spaces, rather
// than ink. Horizontal cells use each glyph's advance and font ascent/descent.
// They retain character ownership across tspan boundaries and kerning runs.
func (s *expansion) bboxTextCells(run []textCharacter, font *text.Font, size, x, y float64) ([]placedTextGlyph, error) {
	needed := false
	for _, c := range run {
		needed = needed || len(c.node.ctx.bboxes) != 0
	}
	if !needed {
		return nil, nil
	}
	var cells []placedTextGlyph
	for i, c := range run {
		if i > 0 {
			kern, err := s.bboxTextMetrics(run[i-1].r, c.r, font, size)
			if err != nil {
				return nil, err
			}
			x += kern.Width
		}
		m, err := s.bboxTextMetrics(c.r, 0, font, size)
		if err != nil {
			return nil, err
		}
		if len(c.node.ctx.bboxes) != 0 {
			cells = append(cells, placedTextGlyph{bboxRectangle(x, y-m.Ascent, m.Width, m.Ascent+m.Descent), c})
		}
		x += m.Width
		if !valid(x) || !valid(y) {
			return nil, fmt.Errorf("SVG bbox text cursor exceeds finite coordinate range")
		}
	}
	return cells, nil
}
