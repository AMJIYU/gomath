package mathtex

import (
	"fmt"
	"gomath"
	"gomath/geom"
	"math"
)

// Activate before parsing an operand. Glyphs and marks record their path nodes;
// auditOperand then charges only legacy structural paths not already counted.
// Hidden nested operands remain charged even after their objects are discarded.
func (p *parser) beginOperandAudit() {
	if p.charged == nil {
		p.charged = make(map[*gomath.Object]struct{})
	}
}

func (p *parser) recordCharged(node *gomath.Object) {
	if p.charged != nil && len(node.Path.Contours) > 0 {
		p.charged[node] = struct{}{}
	}
}

func (p *parser) auditOperand(b box) error {
	if b.object == nil || !finite(b.width) || !finite(b.height) || !finite(b.depth) || b.width < 0 || b.height < 0 || b.depth < 0 {
		return p.error("radical/box operand dimensions overflow or underflow")
	}
	// In particular, do not hide invalid paths/styles by throwing away ink.
	if err := b.object.Validate(); err != nil {
		return p.error(fmt.Sprintf("radical/box operand: %v", err))
	}
	b.object.Walk(func(node *gomath.Object) {
		if _, ok := p.charged[node]; ok {
			return
		}
		for _, contour := range node.Path.Contours {
			p.curves += len(contour.Curves)
		}
		p.recordCharged(node)
	})
	if p.curves > MaxLayoutCurves {
		return p.error("math exceeds 1048576 layout curves (including hidden operands)")
	}
	return nil
}

// Every new construct needs representable em-based clearances, even when its
// operand is empty. The old unindexed sqrt arithmetic remains independent.
func (p *parser) radicalBoxSize(size float64) error {
	if !finite(size) || size <= 0 || size*.0225 == 0 || size/65536 == 0 {
		return p.error("radical/box dimensions underflow or overflow")
	}
	return nil
}

func (p *parser) radicalBoxCommand(command string, size float64) (box, error) {
	if err := p.radicalBoxSize(size); err != nil {
		return box{}, err
	}
	p.beginOperandAudit()
	body, err := p.requiredGroup(size)
	if err != nil {
		return box{}, err
	}
	if err := p.auditOperand(body); err != nil {
		return box{}, err
	}
	if command != "boxed" {
		// Layout extents are semantic data, never an invisible path or Bounds().
		o := gomath.Group()
		o.Name = "Phantom " + command
		b := box{object: o, width: body.width, height: body.height, depth: body.depth, content: true}
		if command == "hphantom" {
			b.height, b.depth = 0, 0
		} else if command == "vphantom" {
			b.width = 0
		}
		return b, nil
	}
	// Padding is the clearance from the operand's layout box to the inside
	// edge of a .045em stroke. Its outside edge is contained in our box.
	padding, stroke := .2*size, .045*size
	inset := padding + stroke
	width := body.width + 2*inset
	height, depth := body.height+inset, body.depth+inset
	if !finite(width) || !finite(height) || !finite(depth) || width <= body.width || height <= body.height || depth <= body.depth {
		return box{}, p.error("boxed dimensions overflow or padding underflows")
	}
	border := gomath.Polygon(geom.V(stroke/2, -depth+stroke/2), geom.V(width-stroke/2, -depth+stroke/2),
		geom.V(width-stroke/2, height-stroke/2), geom.V(stroke/2, height-stroke/2)).
		SetFill(gomath.Transparent).SetStroke(p.options.Color, stroke)
	border.Name = "BoxedRule"
	border.Style.StrokeJoin = gomath.JoinRound
	if err := p.accentCurves(border); err != nil {
		return box{}, err
	}
	body.object.Shift(geom.V(inset, 0))
	body.object.Name = "BoxedBody"
	o := gomath.Group(border, body.object)
	o.Name = "Boxed"
	return box{object: o, width: width, height: height, depth: depth, content: true}, nil
}

func (p *parser) rootIndex(size float64) (box, error) {
	// The caller consumed '['. This row owns its own color/display scope;
	// nested brackets must be protected by a group, literal text or delimiter.
	oldDisplay, oldFractionEm := p.display, p.fractionEm
	p.display, p.fractionEm = false, 0
	defer func() { p.display, p.fractionEm = oldDisplay, oldFractionEm }()
	index, stop, err := p.row(rootIndexRow, size*.5)
	if err != nil {
		return box{}, err
	}
	if stop != rootIndexEnd || !index.content || index.width+index.height+index.depth <= 0 {
		return box{}, p.error("root index requires a nonempty mathematical row and closing ]")
	}
	p.position++
	return index, nil
}

func (p *parser) indexedRadical(size float64) (box, error) {
	if err := p.radicalBoxSize(size); err != nil {
		return box{}, err
	}
	p.beginOperandAudit()
	p.position++ // '['
	index, err := p.rootIndex(size)
	if err != nil {
		return box{}, err
	}
	if err := p.auditOperand(index); err != nil {
		return box{}, err
	}
	rad, err := p.requiredGroup(size)
	if err != nil {
		return box{}, err
	}
	if err := p.auditOperand(rad); err != nil {
		return box{}, err
	}
	// Reserve enough prefix for an arbitrarily wide index. Its right edge is
	// .18em into the radical hook, before the rising stroke at .24em; its
	// bottom clears the hook's .18em top. Radicand baseline stays y=0.
	stroke := .045 * size
	half := stroke / 2
	start := half + math.Max(0, index.width-.18*size)
	top := math.Max(.4*size, rad.height+.1*size)
	indexX := start + .18*size - index.width
	indexY := math.Max(.28*size, .55*top) + index.depth
	left := start + .48*size
	width := left + rad.width + .05*size + half
	height := math.Max(top+half, indexY+index.height)
	depth := rad.depth + .06*size + half
	if !finite(width) || !finite(height) || !finite(depth) || !finite(indexY) || width <= left || height <= top || depth <= rad.depth {
		return box{}, p.error("indexed radical dimensions overflow or clearance underflows")
	}
	outline := gomath.Polyline(geom.V(start, size*.12), geom.V(start+size*.12, size*.18),
		geom.V(start+size*.24, -rad.depth-size*.06), geom.V(start+size*.43, top),
		geom.V(left+rad.width+size*.05, top)).SetStroke(p.options.Color, stroke)
	outline.Name = "RadicalRule"
	// An acute miter at the radical knee could extend past the half-stroke
	// depth used by our layout box. Round joins keep that bound explicit.
	outline.Style.StrokeJoin = gomath.JoinRound
	if err := p.accentCurves(outline); err != nil {
		return box{}, err
	}
	index.object.Shift(geom.V(indexX, indexY))
	index.object.Name = "RootIndex"
	rad.object.Shift(geom.V(left, 0))
	rad.object.Name = "Radicand"
	o := gomath.Group(outline, rad.object, index.object)
	o.Name = "IndexedRadical"
	return box{object: o, width: width, height: height, depth: depth, content: true}, nil
}
