package mathtex

import (
	"fmt"
	"gomath"
	"gomath/geom"
	"math"
)

const MaxSubstackRows = 64

// One centered column at no more than script-size em. Baseline struts keep
// descenders and nested boxes apart. Rows share existing source/depth/cell/curve
// budgets, including when nested inside environments or other substacks.
func (p *parser) substack(size float64) (box, error) {
	p.skipSpace()
	if p.position == len(p.source) || p.source[p.position] != '{' {
		return box{}, p.error("substack requires a braced group")
	}
	p.position++
	rowSize := math.Min(size, p.options.Size*.65)
	if rowSize <= 0 || !finite(rowSize) || rowSize*.2 == 0 {
		return box{}, p.error("substack dimensions underflow or overflow")
	}
	oldDisplay, oldFractionEm := p.display, p.fractionEm
	p.display, p.fractionEm = false, 0
	defer func() { p.display, p.fractionEm = oldDisplay, oldFractionEm }()
	var rows []box
	for {
		cell, stop, err := p.row(substackRow, rowSize)
		if err != nil {
			return box{}, err
		}
		if !visibleStackBody(cell) {
			return box{}, p.error("substack requires nonempty visible rows")
		}
		if len(rows) == MaxSubstackRows {
			return box{}, p.error("substack exceeds 64 rows")
		}
		p.cells++
		if p.cells > MaxEnvironmentCells {
			return box{}, p.error("formula exceeds 4096 environment/substack cells")
		}
		rows = append(rows, cell)
		switch stop {
		case groupEnd:
			p.position++
			return p.substackBox(rows, rowSize)
		case matrixRowEnd:
			_, p.position = p.commandAt()
			p.skipSpace()
			if p.position < len(p.source) && p.source[p.position] == '[' {
				return box{}, p.error("optional substack row spacing is unsupported")
			}
		default:
			return box{}, p.error("unclosed substack")
		}
	}
}

func (p *parser) substackBox(rows []box, size float64) (box, error) {
	gap := size * .2
	width, total := 0.0, gap*float64(len(rows)-1)
	for i := range rows {
		rows[i].height = math.Max(size*.7, rows[i].height)
		rows[i].depth = math.Max(size*.2, rows[i].depth)
		width = math.Max(width, rows[i].width)
		total += rows[i].height + rows[i].depth
	}
	top := size*.16 + total/2
	if !finite(width) || !finite(top) || !finite(total) || width <= 0 || total <= 0 {
		return box{}, p.error("substack dimensions underflow or overflow")
	}
	y := top
	children := make([]*gomath.Object, 0, len(rows))
	for i, row := range rows {
		y -= row.height
		row.object.Shift(geom.V((width-row.width)/2, y))
		row.object.Name = fmt.Sprintf("SubstackRow[%d]", i)
		children = append(children, row.object)
		y -= row.depth + gap
	}
	o := gomath.Group(children...)
	o.Name = "Substack"
	return box{object: o, width: width, height: math.Max(0, top), depth: math.Max(0, total-top), content: true}, nil
}
