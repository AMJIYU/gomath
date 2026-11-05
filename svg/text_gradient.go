package svg

import (
	"fmt"
	"gomath"
	"gomath/geom"
)

// SVG paint servers on a tspan use the entire enclosing text's bbox, not
// the span's clip bbox or each glyph's ink. The bbox stays in text user space;
// its own transform (including use placement) is applied only when painting.
type textGradientScope struct {
	bounds *geom.Bounds
	paints []textGradientPaint
}

type textGradientPaint struct {
	object   *gomath.Object
	target   *element
	plan     *gradientPlan
	viewport [2]float64
	matrix   geom.Mat4
	opacity  float64
}

// Create one extra geometry ledger only when an effective character fill
// needs it. Every laid-out character contributes, including spaces, hidden
// glyphs, solid/none overrides and zero opacity. display:none characters were
// removed by collect; tspan transforms remain explicitly unsupported.
func (s *expansion) textGradientTarget(chars []textCharacter, ctx expansionContext) (*textGradientScope, error) {
	needed := false
	for _, c := range chars {
		id := c.node.ctx.style.fillServer
		if id == "" {
			continue
		}
		target, err := s.document.gradientTarget(id)
		if err != nil {
			return nil, err
		}
		plan := s.document.gradients[target]
		if plan == nil {
			return nil, fmt.Errorf("SVG gradient #%s has no validated plan", id)
		}
		units := plan.attrs["gradientUnits"]
		needed = needed || units == "" || units == "objectBoundingBox"
	}
	if !needed {
		return nil, nil
	}
	scoped, box, err := ctx.bboxTarget(s)
	if err != nil {
		return nil, err
	}
	paint := &textGradientScope{bounds: box}
	scope := scoped.bboxes[len(scoped.bboxes)-1]
	for _, c := range chars {
		node := c.node
		if node.ctx.textPaint == paint {
			continue
		}
		// Parent text segments may resume after a nested span. Never append
		// into a sibling's inherited backing slice or duplicate a scope.
		scopes := make([]bboxScope, len(node.ctx.bboxes), len(node.ctx.bboxes)+1)
		copy(scopes, node.ctx.bboxes)
		node.ctx.bboxes = append(scopes, scope)
		node.ctx.textPaint = paint
	}
	return paint, nil
}

func (scope *textGradientScope) finish() error {
	if scope == nil {
		return nil
	}
	for _, paint := range scope.paints {
		gradient, color, err := paint.plan.paint(paint.target.name.Local, *scope.bounds, paint.viewport, paint.matrix, paint.opacity)
		if err != nil {
			return fmt.Errorf("SVG text gradient #%s: %w", paint.target.attrs["id"], err)
		}
		if gradient == nil {
			paint.object.SetFill(color)
		} else {
			paint.object.SetFillGradient(gradient)
		}
		if err := paint.object.ValidateVisual(); err != nil {
			return fmt.Errorf("invalid SVG text gradient: %w", err)
		}
	}
	return nil
}
