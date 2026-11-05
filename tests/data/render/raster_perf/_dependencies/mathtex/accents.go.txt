package mathtex

import (
	"gomath"
	"gomath/geom"
	"gomath/text"
	"math"
)

// accentArgument consumes exactly one math atom. A braced group already owns
// a row nesting frame; unbraced recursion must claim a frame too, so a chain
// such as \hat\bar\vec x cannot bypass MaxNesting. Scripts outside that atom
// are left for the enclosing row; scripts inside braces are part of the base.
func (p *parser) accentArgument(size float64) (box, error) {
	p.skipSpace()
	if p.position < len(p.source) && p.source[p.position] == '{' {
		return p.requiredGroup(size)
	}
	p.depth++
	defer func() { p.depth-- }()
	if p.depth > MaxNesting {
		return box{}, p.error("math nesting exceeds 128")
	}
	return p.atom(size)
}

func (p *parser) accent(command string, size float64) (box, error) {
	base, err := p.accentArgument(size)
	if err != nil {
		return box{}, err
	}
	ink := base.object.Bounds()
	if !ink.Valid || ink.Size().X <= 0 || ink.Size().Y <= 0 {
		return box{}, p.error("accent requires a nonempty visible math atom")
	}
	thickness, gap := size*.045, size*.10
	if p.style&text.Bold != 0 {
		thickness = size * .060
	}
	if thickness == 0 || gap == 0 || !finite(base.width) || !finite(base.height) || !finite(base.depth) {
		return box{}, p.error("accent dimensions underflow or overflow")
	}
	var mark *gomath.Object
	width, height := size*.42, size*.18
	switch command {
	case "overline", "underline":
		width = base.width
		mark = gomath.Rectangle(width, thickness).Shift(geom.V(width/2, thickness/2))
	case "bar":
		width = size * .34
		mark = gomath.Rectangle(width, thickness).Shift(geom.V(width/2, thickness/2))
	case "hat", "widehat":
		if command == "widehat" {
			width = math.Max(width, base.width)
			height = size * .22
		}
		// A filled roof with an inner edge, rather than a text caret or a
		// stroked centerline whose visible thickness lies outside Bounds().
		mark = gomath.Polygon(geom.V(0, 0), geom.V(width/2, height), geom.V(width, 0),
			geom.V(width-thickness, 0), geom.V(width/2, height-thickness), geom.V(thickness, 0))
	case "vec":
		width = size * .58
		tip := size * .18
		mark = gomath.Polygon(geom.V(0, (height-thickness)/2), geom.V(width-tip, (height-thickness)/2),
			geom.V(width-tip, 0), geom.V(width, height/2), geom.V(width-tip, height),
			geom.V(width-tip, (height+thickness)/2), geom.V(0, (height+thickness)/2))
	case "dot", "ddot":
		radius := thickness
		width = 2 * radius
		mark = gomath.Dot(geom.V(radius, radius), radius)
		if command == "ddot" {
			separation := size * .18
			width += separation
			mark = gomath.Group(mark, gomath.Dot(geom.V(radius+separation, radius), radius))
		}
	default:
		return box{}, p.error("unsupported accent \\" + command)
	}
	mark.SetFill(p.options.Color).SetStroke(gomath.Transparent, 0)
	mark.Name = "AccentMark " + command
	if err := p.accentCurves(mark); err != nil {
		return box{}, err
	}
	// Short marks center on visible ink, preserving italic overhang and
	// ignoring trailing advance-only spaces. Long rules and widehat span the
	// whole advance box, including any scripts inside the argument.
	x := ink.Center().X - width/2
	if command == "overline" || command == "underline" {
		x = 0
	} else if command == "widehat" {
		x = (base.width - width) / 2
	}
	markBounds := mark.Bounds()
	if !markBounds.Valid || markBounds.Size().X <= 0 || markBounds.Size().Y <= 0 {
		return box{}, p.error("accent path dimensions underflow")
	}
	top, bottom := math.Max(base.height, ink.Max.Y), math.Min(-base.depth, ink.Min.Y)
	y := top + gap - markBounds.Min.Y
	if command == "underline" {
		y = bottom - gap - markBounds.Max.Y
	}
	mark.Shift(geom.V(x, y))
	markBounds = mark.Bounds()
	left := math.Min(0, math.Min(ink.Min.X, markBounds.Min.X))
	right := math.Max(base.width, math.Max(ink.Max.X, markBounds.Max.X))
	if left < 0 {
		// Only horizontal movement: the base's baseline remains y=0.
		base.object.Shift(geom.V(-left, 0))
		mark.Shift(geom.V(-left, 0))
	}
	o := gomath.Group(base.object, mark)
	o.Name = "Accent " + command
	return box{object: o, width: right - left, height: math.Max(top, markBounds.Max.Y), depth: math.Max(-bottom, -markBounds.Min.Y), content: true}, nil
}

// Charge only newly created marks, not their already-accounted base subtrees.
// Glyph paths continue to share the same parser-local curve budget.
func (p *parser) accentCurves(mark *gomath.Object) error {
	mark.Walk(func(node *gomath.Object) {
		p.recordCharged(node)
		for _, contour := range node.Path.Contours {
			p.curves += len(contour.Curves)
		}
	})
	if p.curves > MaxLayoutCurves {
		return p.error("math exceeds 1048576 glyph/accent curves")
	}
	return nil
}
