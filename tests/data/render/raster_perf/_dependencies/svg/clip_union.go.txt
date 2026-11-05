package svg

import (
	"fmt"
	"gomath"
	"gomath/geom"
	"gomath/path"
)

func unionOutlineSize(p path.Path) (segments, contours int) {
	contours = len(p.Contours)
	for _, c := range p.Contours {
		segments += len(c.Curves)
		if len(c.Curves) > 0 && c.Curves[len(c.Curves)-1].P3 != c.Start {
			segments++
		}
	}
	return
}

func (c *clipCompiler) unionBoolean(a, b path.Path, ruleA, ruleB path.FillRule) (path.Path, error) {
	as, ac := unionOutlineSize(a)
	bs, bc := unionOutlineSize(b)
	if as+bs > maxClipUnionOperandSegments || ac+bc > maxClipUnionOperandContours {
		return path.Path{}, fmt.Errorf("SVG clip union Boolean operands exceed %d segments or %d contours", maxClipUnionOperandSegments, maxClipUnionOperandContours)
	}
	if c.union.calls >= maxClipUnionCalls {
		return path.Path{}, fmt.Errorf("SVG clip union shared Boolean reservation budget exceeds %d calls", maxClipUnionCalls)
	}
	c.union.calls++
	out, err := path.Boolean(a, b, path.BooleanUnion, path.BooleanOptions{RuleA: ruleA, RuleB: ruleB, Limits: clipUnionLimits()})
	if err != nil {
		return path.Path{}, fmt.Errorf("SVG clip union: %w", err)
	}
	s, n := unionOutlineSize(out)
	if s > maxClipUnionOperandSegments || n > maxClipUnionOperandContours {
		return path.Path{}, fmt.Errorf("SVG clip union result exceeds %d segments or %d contours", maxClipUnionOperandSegments, maxClipUnionOperandContours)
	}
	if err := c.doc.clipBudget.geometry(out); err != nil {
		return path.Path{}, err
	}
	return out, nil
}

// This branch does not flatten curves or reinterpret their controls as lines.
// Accepted SVG syntax itself proves that every segment is straight. Affine
// transforms are applied to endpoints before constructing canonical Line cubics,
// avoiding noncollinearity caused solely by separately rounded control points.
func unionLinearShape(e *element) (path.Path, error) {
	switch e.name.Local {
	case "rect":
		for _, key := range []string{"rx", "ry"} {
			v, err := length(e.attrs, key, 0)
			if err != nil || v != 0 {
				return path.Path{}, fmt.Errorf("SVG multi-child clip union requires sharp rect; %s unsupported", key)
			}
		}
	case "polygon":
	case "path":
		s := scanner{data: e.attrs["d"]}
		for {
			s.space()
			if s.pos == len(s.data) {
				break
			}
			if letter(s.data[s.pos]) {
				switch s.data[s.pos] {
				case 'M', 'm', 'L', 'l', 'H', 'h', 'V', 'v', 'Z', 'z':
				default:
					return path.Path{}, fmt.Errorf("SVG multi-child clip union requires M/L/H/V/Z linear paths; command %q unsupported", s.data[s.pos])
				}
				s.pos++
			} else if _, err := s.number(); err != nil {
				return path.Path{}, err
			}
		}
	default:
		return path.Path{}, fmt.Errorf("SVG multi-child clip union supports sharp rect/polygon/closed linear path or direct use, got <%s>", e.name.Local)
	}
	p, err := shape(e.name.Local, e.attrs)
	if err != nil {
		return path.Path{}, err
	}
	if len(p.Contours) == 0 {
		return path.Path{}, fmt.Errorf("SVG multi-child clip union requires nonempty closed polygon contours")
	}
	for _, contour := range p.Contours {
		if !contour.Closed || len(contour.Curves) < 2 {
			return path.Path{}, fmt.Errorf("SVG multi-child clip union requires closed polygon contours with at least three edges")
		}
		segments := len(contour.Curves)
		if contour.Curves[len(contour.Curves)-1].P3 != contour.Start {
			segments++
		}
		if segments < 3 {
			return path.Path{}, fmt.Errorf("SVG multi-child clip union requires at least three polygon edges")
		}
		for _, v := range contour.Curves {
			if v.P0 == v.P3 {
				return path.Path{}, fmt.Errorf("SVG multi-child clip union rejects zero-length edges")
			}
		}
	}
	return p, nil
}

func (c *clipCompiler) unionChild(e *element, parent inheritedStyle, chain []*element, matrix geom.Mat4, bbox bool, referenced bool) (clipUnionInput, error) {
	fail := func(err error) (clipUnionInput, error) { return clipUnionInput{}, err }
	if e.name.Space != "" && e.name.Space != svgNamespace {
		return fail(fmt.Errorf("unsupported SVG union clipping graphic namespace %q", e.name.Space))
	}
	if len(e.foreignAttrs) != 0 {
		return fail(fmt.Errorf("unsupported SVG union clipping graphic namespaced attribute %s", e.foreignAttrs[0].Local))
	}
	for key, value := range e.attrs {
		if len(value) > maxClipUnionAttributeBytes {
			return fail(fmt.Errorf("SVG clip union geometry attribute exceeds %d bytes", maxClipUnionAttributeBytes))
		}
		switch key {
		case "id", "class", "style", "transform", "xmlns":
		default:
			if !cssProperty(key) && !clipShapeAttribute(e.name.Local, key) {
				return fail(fmt.Errorf("unsupported SVG union clipping graphic attribute %q", key))
			}
		}
	}
	if bbox {
		if err := validateBBoxClipLengths(e); err != nil {
			return fail(err)
		}
	}
	if e.hasText {
		return fail(fmt.Errorf("SVG union clipping graphic cannot contain character data"))
	}
	for _, child := range e.children {
		if !definitionOnly(child) {
			return fail(fmt.Errorf("unsupported SVG union clipping graphic child <%s>", child.name.Local))
		}
	}
	chain = append(chain, e)
	attrs, err := c.doc.css.attributes(e, chain)
	if err != nil {
		return fail(err)
	}
	style, _, err := parseStyle(parent, attrs)
	if err != nil {
		return fail(err)
	}
	if style.clipPath != "" {
		return fail(fmt.Errorf("SVG multi-child clip union nested clip-path on a child/use target is unsupported"))
	}
	local, err := transform(e.attrs["transform"])
	matrix = matrix.Mul(local)
	if err != nil || !finiteMatrix(matrix) {
		return fail(fmt.Errorf("invalid SVG union clipping graphic transform"))
	}
	if e.name.Local == "use" {
		if referenced || e.target == nil || e.target.name.Local == "use" {
			return fail(fmt.Errorf("SVG multi-child clip union use requires a direct sharp rect/polygon/linear path target"))
		}
		x, err := length(e.attrs, "x", 0)
		if err != nil {
			return fail(err)
		}
		y, err := length(e.attrs, "y", 0)
		if err != nil {
			return fail(err)
		}
		for _, key := range []string{"width", "height"} {
			v, err := length(e.attrs, key, 0)
			if err != nil || v < 0 {
				return fail(fmt.Errorf("invalid SVG union clip use %s", key))
			}
		}
		result, err := c.unionChild(e.target, style, nil, matrix.Mul(geom.Translation(geom.V(x, y))), bbox, true)
		result.hidden = result.hidden || style.displayNone
		return result, err
	}
	p, err := unionLinearShape(e)
	if err != nil {
		return fail(err)
	}
	segments, contours := unionOutlineSize(p)
	if c.union.inputs >= maxClipUnionInputs || segments > maxClipUnionSourceSegments-c.union.segments || contours > maxClipUnionSourceContours-c.union.contours {
		return fail(fmt.Errorf("SVG clip union shared source budget exceeds %d inputs, %d segments or %d contours", maxClipUnionInputs, maxClipUnionSourceSegments, maxClipUnionSourceContours))
	}
	c.union.inputs++
	c.union.segments += segments
	c.union.contours += contours
	if err := c.doc.clipBudget.geometry(p); err != nil {
		return fail(err)
	}
	if err := c.doc.clipBudget.geometry(p); err != nil {
		return fail(err)
	}
	q := path.Path{Contours: make([]path.Contour, len(p.Contours))}
	for i, contour := range p.Contours {
		out := path.Contour{Start: matrix.Apply(contour.Start), Closed: true, Curves: make([]path.Cubic, len(contour.Curves))}
		for j, v := range contour.Curves {
			out.Curves[j] = path.Line(matrix.Apply(v.P0), matrix.Apply(v.P3))
		}
		q.Contours[i] = out
	}
	if err := q.Validate(); err != nil {
		return fail(fmt.Errorf("SVG clip union transformed geometry: %w", err))
	}
	return clipUnionInput{q, clipFillRule(style.clipRule), style.displayNone || style.visibilityHidden}, nil
}

func (c *clipCompiler) compileUnion(e *element, children []*element, style inheritedStyle, chain []*element, matrix geom.Mat4, depth int) ([]*gomath.Clip, error) {
	if len(children) > maxClipUnionChildren {
		return nil, fmt.Errorf("SVG clip union child count exceeds %d", maxClipUnionChildren)
	}
	if depth != 1 || style.clipPath != "" {
		return nil, fmt.Errorf("SVG multi-child clip union nested clip-path references are unsupported")
	}
	if c.union == nil {
		c.union = &clipUnionBudget{plans: make(map[*element]bool)}
	}
	// Reserve availability before any geometry/Boolean work. Individual calls
	// consume these same shared reservations; there is no per-definition reset.
	if 2*len(children)-1 > maxClipUnionCalls-c.union.calls {
		return nil, fmt.Errorf("SVG clip union shared Boolean reservation budget exceeds %d calls", maxClipUnionCalls)
	}
	// Validate/charge ALL source geometry before any expensive Boolean operation.
	inputs := make([]clipUnionInput, len(children))
	for i, child := range children {
		input, err := c.unionChild(child, style, chain, matrix, bboxClip(e), false)
		if err != nil {
			return nil, err
		}
		segments, contours := unionOutlineSize(input.outline)
		if segments > maxClipUnionOperandSegments || contours > maxClipUnionOperandContours {
			return nil, fmt.Errorf("SVG clip union Boolean operand exceeds %d segments or %d contours", maxClipUnionOperandSegments, maxClipUnionOperandContours)
		}
		inputs[i] = input
	}
	var result path.Path
	for i, input := range inputs {
		normalized, err := c.unionBoolean(input.outline, path.Path{}, input.rule, path.NonZero)
		if err != nil {
			return nil, err
		}
		if input.hidden {
			normalized = path.Path{}
		}
		if i == 0 {
			result = normalized
			continue
		}
		result, err = c.unionBoolean(result, normalized, path.NonZero, path.NonZero)
		if err != nil {
			return nil, err
		}
	}
	if err := c.doc.clipBudget.geometry(result); err != nil {
		return nil, err
	}
	native, err := gomath.NewClip(result)
	if err != nil {
		return nil, fmt.Errorf("SVG clip union result: %w", err)
	}
	c.union.plans[e] = true
	return []*gomath.Clip{native}, nil
}
