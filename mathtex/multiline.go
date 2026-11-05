package mathtex

import (
	"fmt"
	"gomath"
	"gomath/geom"
	"math"
)

// The new environments have their own strict grammar/layout. Keep the existing
// matrix/cases path unchanged, including its historical spacing and defaults.
func (p *parser) multilineEnvironment(name string, size float64) (box, error) {
	p.skipSpace()
	if p.position < len(p.source) && p.source[p.position] == '[' {
		return box{}, p.error("optional multiline environment positioning is unsupported")
	}
	oldDisplay, oldFractionEm := p.display, p.fractionEm
	defer func() { p.display, p.fractionEm = oldDisplay, oldFractionEm }()
	var rows [][]box
	var row []box
	visible := false
	for {
		if len(rows) == MaxEnvironmentRows {
			return box{}, p.error("multiline environment exceeds 64 rows")
		}
		if len(row) == MaxEnvironmentColumns {
			return box{}, p.error("multiline environment exceeds 64 columns")
		}
		// Each cell starts display mode at its inherited local size. Explicit
		// declarations in a cell restore before siblings and after this box.
		p.display, p.fractionEm = true, 0
		cell, stop, err := p.row(cellRow, size)
		if err != nil {
			return box{}, err
		}
		if name == "gathered" && stop == columnEnd {
			return box{}, p.error("gathered does not accept alignment columns")
		}
		p.cells++
		if p.cells > MaxEnvironmentCells {
			return box{}, p.error("formula exceeds 4096 environment/substack cells")
		}
		row = append(row, cell)
		visible = visible || visibleStackBody(cell)
		switch stop {
		case columnEnd:
			p.position++
			continue
		case matrixRowEnd, environmentEnd:
			if !visible {
				return box{}, p.error("multiline environment requires visible nonempty rows; trailing separators are unsupported")
			}
			if name == "aligned" && (len(row) < 2 || len(row)%2 != 0) {
				return box{}, p.error("aligned requires explicit right/left column pairs")
			}
			if len(rows) > 0 && len(row) != len(rows[0]) {
				return box{}, p.error(fmt.Sprintf("ragged %s row %d: got %d columns, want %d", name, len(rows), len(row), len(rows[0])))
			}
			rows = append(rows, row)
			row, visible = nil, false
			if stop == matrixRowEnd {
				_, p.position = p.commandAt()
				p.skipSpace()
				if p.position < len(p.source) && p.source[p.position] == '[' {
					return box{}, p.error("optional multiline row spacing is unsupported")
				}
				if p.position < len(p.source) && p.source[p.position] == '*' {
					return box{}, p.error("starred multiline row breaks are unsupported")
				}
				continue
			}
			_, p.position = p.commandAt()
			end, err := p.environmentName()
			if err != nil {
				return box{}, err
			}
			if end != name {
				return box{}, p.error("mismatched environment end " + end + "; want " + name)
			}
			return p.multilineBox(name, rows, size)
		default:
			return box{}, p.error("unclosed multiline environment " + name)
		}
	}
}

// Alignment uses full advances, not just visible ink. A common row baseline
// accommodates every cell's ascent/depth, with no clipping of nested formulas.
func (p *parser) multilineBox(name string, rows [][]box, size float64) (box, error) {
	widths := make([]float64, len(rows[0]))
	heights, depths := make([]float64, len(rows)), make([]float64, len(rows))
	gap, pairGap, rowGap := size*.12, size, size*.3
	if gap == 0 || rowGap == 0 || !finite(size) || size <= 0 {
		return box{}, p.error("multiline dimensions underflow or overflow")
	}
	for i, row := range rows {
		heights[i], depths[i] = size*.7, size*.2
		for j, cell := range row {
			widths[j] = math.Max(widths[j], cell.width)
			heights[i] = math.Max(heights[i], cell.height)
			depths[i] = math.Max(depths[i], cell.depth)
		}
	}
	xs := make([]float64, len(widths))
	width, total := 0.0, rowGap*float64(len(rows)-1)
	for j, w := range widths {
		if j > 0 {
			if j%2 != 0 {
				width += gap
			} else {
				width += pairGap
			}
		}
		xs[j] = width
		width += w
	}
	for i := range heights {
		total += heights[i] + depths[i]
	}
	top := size*.16 + total/2
	if !finite(width) || !finite(total) || !finite(top) || width <= 0 || total <= 0 {
		return box{}, p.error("multiline dimensions underflow or overflow")
	}
	y := top
	children := make([]*gomath.Object, 0, len(rows)*len(widths))
	for i, row := range rows {
		y -= heights[i]
		for j, cell := range row {
			dx := 0.0
			if name == "gathered" {
				dx = (widths[j] - cell.width) / 2
			} else if j%2 == 0 {
				dx = widths[j] - cell.width
			}
			cell.object.Shift(geom.V(xs[j]+dx, y))
			cell.object.Name = fmt.Sprintf("%s cell[%d,%d]", name, i, j)
			children = append(children, cell.object)
		}
		y -= depths[i] + rowGap
	}
	o := gomath.Group(children...)
	o.Name = "Environment " + name
	return box{object: o, width: width, height: math.Max(0, top), depth: math.Max(0, total-top), content: true}, nil
}
