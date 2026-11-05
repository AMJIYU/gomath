package mathtex

import (
	"gomath"
	"gomath/geom"
	"gomath/text"
	"math"
)

// Only directly parsed operator commands carry this eligibility. Rows, accents
// and other wrappers intentionally clear it: a group is not an operator atom.
type operatorKind uint8

const (
	ordinaryAtom operatorKind = iota
	sumOperator
	productOperator
	limitOperator
	integralOperator
	namedSideOperator
	namedDisplayOperator
)

func (p *parser) operatorAtom(command string, size float64) (box, error) {
	var kind operatorKind
	value := command
	switch command {
	case "sum":
		kind, value = sumOperator, "∑"
	case "prod":
		kind, value = productOperator, "∏"
	case "int":
		kind, value = integralOperator, "∫"
	case "lim":
		kind = limitOperator
		old := p.style
		p.style &= text.Bold // upright operator text, retaining a bold context
		defer func() { p.style = old }()
	default:
		return box{}, p.error("unsupported limits operator \\" + command)
	}
	b, err := p.glyph(value, size)
	b.operator = kind
	return b, err
}

// Scripts enter text style even in display rows. Claim a nesting frame for an
// unbraced script atom as well: x^\hat\hat... must not bypass the shared limit.
func (p *parser) scriptAtom(size float64) (box, error) {
	oldDisplay := p.display
	oldFractionEm := p.fractionEm
	p.display = false
	p.fractionEm = 0
	p.depth++
	defer func() { p.display, p.fractionEm = oldDisplay, oldFractionEm; p.depth-- }()
	if p.depth > MaxNesting {
		return box{}, p.error("math nesting exceeds 128")
	}
	return p.atom(size * .65)
}

// Arguments enter text style at the requested local size. Legacy frac keeps
// its 0.8 scale. Local declarations are restored at this boundary.
func (p *parser) textStyleGroup(size float64) (box, error) {
	oldDisplay := p.display
	oldFractionEm := p.fractionEm
	p.display = false
	p.fractionEm = 0
	defer func() { p.display, p.fractionEm = oldDisplay, oldFractionEm }()
	return p.requiredGroup(size)
}

func (p *parser) scripts(b box, size float64) (box, error) {
	var sup, sub *box
	stacked := p.display && b.operator != ordinaryAtom && b.operator != integralOperator && b.operator != namedSideOperator
	directive := false
	for {
		p.skipSpace()
		command, end := p.commandAt()
		if command == "limits" || command == "nolimits" {
			if b.operator == ordinaryAtom {
				return box{}, p.error("\\" + command + " requires a directly preceding supported operator")
			}
			if directive {
				return box{}, p.error("duplicate operator limits directive")
			}
			p.position = end
			directive, stacked = true, command == "limits"
			continue
		}
		if p.position >= len(p.source) || (p.source[p.position] != '^' && p.source[p.position] != '_') {
			break
		}
		kind := p.source[p.position]
		p.position++
		if (kind == '^' && sup != nil) || (kind == '_' && sub != nil) {
			return box{}, p.error("duplicate script")
		}
		script, err := p.scriptAtom(size)
		if err != nil {
			return box{}, err
		}
		if kind == '^' {
			sup = &script
		} else {
			sub = &script
		}
	}
	b.operator = ordinaryAtom
	if stacked && (sup != nil || sub != nil) {
		return p.operatorLimits(b, sup, sub, size)
	}
	// Preserve the pre-existing inline side-script metrics and child order.
	width := 0.0
	if sup != nil {
		dy := math.Max(size*.65, b.height*.85) + sup.depth
		sup.object.Shift(geom.V(b.width+size*.04, dy))
		b.height = math.Max(b.height, dy+sup.height)
		width = math.Max(width, sup.width)
		b.object.Children = append(b.object.Children, sup.object)
	}
	if sub != nil {
		dy := -math.Max(size*.25, b.depth+size*.08) - sub.height
		sub.object.Shift(geom.V(b.width+size*.04, dy))
		b.depth = math.Max(b.depth, -dy+sub.depth)
		width = math.Max(width, sub.width)
		b.object.Children = append(b.object.Children, sub.object)
	}
	if width > 0 {
		b.width += size*.04 + width
	}
	return b, nil
}

// Center actual limit ink over the operator ink. Advance-only whitespace is
// retained in the enclosing width but cannot pull visible limits off center.
// The body may move horizontally to make room; its y=0 baseline never moves.
// Explicit stack commands reuse this geometry without operator eligibility.
func (p *parser) operatorLimits(b box, sup, sub *box, size float64) (box, error) {
	gap := size * .12
	ink := b.object.Bounds()
	if gap == 0 || !ink.Valid || ink.Size().X <= 0 || (ink.Size().Y <= 0 && b.height+b.depth <= 0) {
		return box{}, p.error("operator limits dimensions underflow")
	}
	center := ink.Center().X
	left, right := 0.0, b.width
	height, depth := b.height, b.depth
	items := []*gomath.Object{b.object}
	for i, script := range []*box{sup, sub} {
		if script == nil {
			continue
		}
		bounds := script.object.Bounds()
		sx := script.width / 2
		if bounds.Valid {
			sx = bounds.Center().X
		}
		dx, dy := center-sx, 0.0
		if bounds.Valid || script.height > 0 || script.depth > 0 {
			if i == 0 {
				dy = b.height + gap + script.depth
				height = math.Max(height, dy+script.height)
			} else {
				dy = -b.depth - gap - script.height
				depth = math.Max(depth, -dy+script.depth)
			}
		}
		script.object.Shift(geom.V(dx, dy))
		left, right = math.Min(left, dx), math.Max(right, dx+script.width)
		wrapper := gomath.Group(script.object)
		wrapper.Name = "UpperLimit"
		if i == 1 {
			wrapper.Name = "LowerLimit"
		}
		items = append(items, wrapper)
	}
	o := gomath.Group(items...)
	o.Name = "OperatorLimits"
	o.Shift(geom.V(-left, 0))
	if !finite(right-left) || !finite(height) || !finite(depth) {
		return box{}, p.error("operator limits dimensions overflow")
	}
	return box{object: o, width: right - left, height: height, depth: depth, content: true}, nil
}
