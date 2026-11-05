package mathtex

import (
	"gomath"
	"gomath/geom"
	"gomath/path"
	"gomath/text"
	"math"
)

// Horizontal braces implement a finite em policy, not TeX's extensible glyph
// assembly. The eight cubic ribbon segments are original GoMath geometry.
// Manim CE 0.22.1 (MIT), commit 16ea443e2a5f1d79db0a721161411ce916d6e733,
// uses its external TeX pipeline for these commands; no TeX paths are copied.
func (p *parser) horizontalBrace(command string, size float64) (box, error) {
	if err := p.radicalBoxSize(size); err != nil {
		return box{}, err
	}
	p.beginOperandAudit()
	body, err := p.accentArgument(size)
	if err != nil {
		return box{}, err
	}
	if err := p.auditOperand(body); err != nil {
		return box{}, err
	}
	// Empty/space/phantom operands retain their semantic box. Even zero-width
	// operands receive a minimum .6em brace, centered on their advance box.
	width, height, thickness := math.Max(body.width, .6*size), .28*size, .045*size
	if p.style&text.Bold != 0 {
		thickness = .060 * size
	}
	var curve path.Builder
	point := func(x, y float64) geom.Vec3 { return geom.V(x*width, y) }
	curve.MoveTo(point(0, 0))
	curve.CubicTo(point(0, height/2), point(.04, height/2), point(.1, height/2))
	curve.CubicTo(point(.36, height/2), point(.46, height/2), point(.5, height))
	curve.CubicTo(point(.54, height/2), point(.64, height/2), point(.9, height/2))
	curve.CubicTo(point(.96, height/2), point(1, height/2), point(1, 0))
	curve.CubicTo(point(1, height/2-thickness), point(.96, height/2-thickness), point(.9, height/2-thickness))
	curve.CubicTo(point(.64, height/2-thickness), point(.54, height/2-thickness), point(.5, height-thickness))
	curve.CubicTo(point(.46, height/2-thickness), point(.36, height/2-thickness), point(.1, height/2-thickness))
	curve.CubicTo(point(.04, height/2-thickness), point(0, height/2-thickness), point(0, 0))
	curve.Close()
	mark := gomath.NewObject(curve.Path()).SetFill(p.options.Color).SetStroke(gomath.Transparent, 0)
	mark.Name = "BraceMark " + command
	if command == "underbrace" {
		mark.ApplyMatrix(geom.Scaling(1, -1, 1))
	}
	if err := p.accentCurves(mark); err != nil {
		return box{}, err
	}
	ink := body.object.Bounds()
	top, bottom := body.height, -body.depth
	if ink.Valid {
		top, bottom = math.Max(top, ink.Max.Y), math.Min(bottom, ink.Min.Y)
	}
	mb := mark.Bounds()
	if !mb.Valid || mb.Size().X <= 0 || mb.Size().Y <= 0 {
		return box{}, p.error("brace path dimensions underflow")
	}
	y := top + .1*size - mb.Min.Y
	if command == "underbrace" {
		y = bottom - .1*size - mb.Max.Y
	}
	mark.Shift(geom.V((body.width-width)/2, y))
	mb = mark.Bounds()
	left, right := math.Min(0, mb.Min.X), math.Max(body.width, mb.Max.X)
	if ink.Valid {
		left, right = math.Min(left, ink.Min.X), math.Max(right, ink.Max.X)
	}
	body.object.Shift(geom.V(-left, 0)) // preserve the operand baseline
	mark.Shift(geom.V(-left, 0))
	bodyWrapper := gomath.Group(body.object)
	bodyWrapper.Name = "BraceBody"
	o := gomath.Group(bodyWrapper, mark)
	o.Name = "HorizontalBrace " + command
	b := box{object: o, width: right - left, height: math.Max(top, mb.Max.Y), depth: math.Max(-bottom, -mb.Min.Y), content: true, brace: true}
	if err := p.auditOperand(b); err != nil {
		return box{}, err
	}
	return b, nil
}

// Both ^ and _ are centered limits on a directly parsed brace, in either
// order and in both row styles. Group/style wrappers clear eligibility as
// ordinary atoms do. Explicit limits/nolimits directives are outside this
// slice and fail rather than being ignored. scriptAtom shares all counters.
func (p *parser) braceScripts(b box, size float64) (box, error) {
	var sup, sub *box
	for {
		p.skipSpace()
		command, _ := p.commandAt()
		if command == "limits" || command == "nolimits" || command == "displaylimits" {
			return box{}, p.error("unsupported brace directive \\" + command)
		}
		if p.position == len(p.source) || (p.source[p.position] != '^' && p.source[p.position] != '_') {
			break
		}
		kind := p.source[p.position]
		p.position++
		if kind == '^' && sup != nil || kind == '_' && sub != nil {
			return box{}, p.error("duplicate brace label")
		}
		label, err := p.scriptAtom(size)
		if err != nil {
			return box{}, err
		}
		if err := p.auditOperand(label); err != nil {
			return box{}, err
		}
		if kind == '^' {
			sup = &label
		} else {
			sub = &label
		}
	}
	b.brace = false
	if sup == nil && sub == nil {
		return b, nil
	}
	// The brace spans its semantic advance and is always centered in its
	// owning box, including phantom-only bodies. A wider label reserves room
	// without stretching the brace or moving the body vertically.
	center, left, right := b.width/2, 0.0, b.width
	items := []*gomath.Object{b.object}
	height, depth := b.height, b.depth
	for i, label := range []*box{sup, sub} {
		if label == nil {
			continue
		}
		ink := label.object.Bounds()
		x := center - label.width/2
		if ink.Valid {
			x = center - ink.Center().X
		}
		y := b.height + .12*size + label.depth
		if i == 1 {
			y = -b.depth - .12*size - label.height
		}
		label.object.Shift(geom.V(x, y))
		left, right = math.Min(left, x), math.Max(right, x+label.width)
		height, depth = math.Max(height, y+label.height), math.Max(depth, -y+label.depth)
		wrapper := gomath.Group(label.object)
		wrapper.Name = "BraceUpperLabel"
		if i == 1 {
			wrapper.Name = "BraceLowerLabel"
		}
		items = append(items, wrapper)
	}
	o := gomath.Group(items...)
	o.Name = "BraceLabels"
	o.Shift(geom.V(-left, 0))
	b = box{object: o, width: right - left, height: height, depth: depth, content: true}
	if err := p.auditOperand(b); err != nil {
		return box{}, err
	}
	return b, nil
}
