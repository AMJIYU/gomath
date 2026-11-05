package mathtex

import (
	"fmt"
	"gomath"
	"gomath/geom"
	"math"
	"unicode"
)

// A deliberately literal preamble: no expansion, recursive specs, insertions,
// repeated rules or length syntax. Every boundary can have at most one rule.
type arraySpec struct {
	columns  []rune
	vertical []bool
}

func (p *parser) arrayPreamble() (arraySpec, error) {
	p.skipSpace()
	if p.position >= len(p.source) || p.source[p.position] != '{' {
		return arraySpec{}, p.error("array requires a braced l/c/r column specification")
	}
	p.position++
	s := arraySpec{vertical: []bool{false}}
	for p.position < len(p.source) {
		r := p.source[p.position]
		p.position++
		if unicode.IsSpace(r) {
			continue
		}
		switch r {
		case '}':
			if len(s.columns) == 0 {
				return arraySpec{}, p.error("array column specification must contain l/c/r")
			}
			return s, nil
		case '|':
			j := len(s.columns)
			if s.vertical[j] {
				return arraySpec{}, p.error("array supports only one vertical rule per column boundary")
			}
			s.vertical[j] = true
		case 'l', 'c', 'r':
			if len(s.columns) >= MaxEnvironmentColumns {
				return arraySpec{}, p.error("array exceeds 64 columns")
			}
			s.columns = append(s.columns, r)
			s.vertical = append(s.vertical, false)
		default:
			return arraySpec{}, p.error(fmt.Sprintf("unsupported array column specification rune %q; only l/c/r and single | are supported", r))
		}
	}
	return arraySpec{}, p.error("unclosed array column specification")
}

// Separate from legacy environment parsing: its trailing separators, struts,
// spacing and layout defaults remain byte-for-byte independent of arrays.
func (p *parser) arrayEnvironment(size float64) (box, error) {
	spec, err := p.arrayPreamble()
	if err != nil {
		return box{}, err
	}
	if !finite(size) || size <= 0 || size/65536 == 0 || size*.0225 == 0 {
		return box{}, p.error("array dimensions underflow or overflow")
	}
	// All cells, including discarded phantoms and legacy structural paths inside
	// them, use the SAME parser ledger. Nested audits never refund or double count.
	p.beginOperandAudit()
	var rows [][]box
	horizontal := []bool{false}
	finish := func() (box, error) {
		_, p.position = p.commandAt()
		name, err := p.environmentName()
		if err != nil {
			return box{}, err
		}
		if name != "array" {
			return box{}, p.error("mismatched environment end " + name + "; want array")
		}
		if len(rows) == 0 {
			return box{}, p.error("array requires at least one nonempty explicit row")
		}
		return p.arrayBox(spec, rows, horizontal, size)
	}
	for {
		p.skipSpace()
		command, end := p.commandAt()
		if command == "hline" {
			horizontal[len(rows)] = true
			p.position = end
			p.skipSpace()
			command, _ = p.commandAt()
			if command == "hline" {
				return box{}, p.error("array supports only one hline per row boundary")
			}
		}
		if command == "end" {
			return finish()
		}
		if len(rows) >= MaxEnvironmentRows {
			return box{}, p.error("array exceeds 64 rows")
		}
		var row []box
		for {
			if len(row) >= len(spec.columns) {
				return box{}, p.error("array row has more cells than its column specification")
			}
			if p.cells >= MaxEnvironmentCells {
				return box{}, p.error("formula exceeds 4096 environment/substack cells")
			}
			// Reserve the outer cell before parsing nested environments. Charging
			// afterward would let a final nested cell exhaust the shared ledger
			// and leave no capacity for this containing cell.
			p.cells++
			cell, stop, err := p.row(cellRow, size)
			if err != nil {
				return box{}, err
			}
			if len(row) == 0 && !cell.content && stop != columnEnd {
				return box{}, p.error("array rejects empty rows; use explicit empty cells")
			}
			if err := p.auditOperand(cell); err != nil {
				return box{}, err
			}
			row = append(row, cell)
			if stop == columnEnd {
				p.position++
				continue
			}
			if stop != matrixRowEnd && stop != environmentEnd {
				return box{}, p.error("unclosed array environment")
			}
			if len(row) != len(spec.columns) {
				return box{}, p.error(fmt.Sprintf("array row %d has %d cells, want %d", len(rows), len(row), len(spec.columns)))
			}
			rows = append(rows, row)
			horizontal = append(horizontal, false)
			if stop == environmentEnd {
				return finish()
			}
			_, p.position = p.commandAt()
			p.skipSpace()
			if p.position < len(p.source) && (p.source[p.position] == '[' || p.source[p.position] == '*') {
				return box{}, p.error("array optional/starred row spacing is unsupported")
			}
			break
		}
	}
}

// Finite layout policy in em: .25 per-side column padding, .15 outer vertical
// padding, .30 between-row clearance, .70/.20 strut and .045 rule thickness.
// Rule bands reserve their own space; boundary coordinates include half-stroke.
func (p *parser) arrayBox(spec arraySpec, rows [][]box, horizontal []bool, size float64) (box, error) {
	nc, nr := len(spec.columns), len(rows)
	widths := make([]float64, nc)
	ascents, depths := make([]float64, nr), make([]float64, nr)
	padX, padY, gap, stroke := size*.25, size*.15, size*.3, size*.045
	for j := range widths {
		widths[j] = size * .25
	}
	for i, row := range rows {
		ascents[i], depths[i] = size*.7, size*.2
		for j, cell := range row {
			widths[j] = math.Max(widths[j], cell.width)
			ascents[i] = math.Max(ascents[i], cell.height)
			depths[i] = math.Max(depths[i], cell.depth)
		}
	}
	starts := make([]float64, nc)
	verticalX := make([]float64, nc+1)
	width, total := 0.0, 2*padY+gap*float64(nr-1)
	for j, w := range widths {
		if spec.vertical[j] {
			verticalX[j] = width + stroke/2
			width += stroke
		}
		starts[j] = width + padX
		width += 2*padX + w
	}
	if spec.vertical[nc] {
		verticalX[nc] = width + stroke/2
		width += stroke
	}
	for i := range ascents {
		total += ascents[i] + depths[i]
	}
	for _, on := range horizontal {
		if on {
			total += stroke
		}
	}
	top := size*.16 + total/2
	depth := total - top
	if !finite(width) || !finite(total) || !finite(top) || !finite(depth) || width <= 0 || depth < 0 || top <= 0 {
		return box{}, p.error("array dimensions underflow or overflow")
	}
	var rules, cells []*gomath.Object
	addRule := func(name string, a, b geom.Vec3) error {
		if a == b {
			return p.error("array rule endpoints collapse")
		}
		line := gomath.Line(a, b).SetStroke(p.options.Color, stroke)
		line.Style.StrokeCap = gomath.CapButt
		line.Name = name
		if err := p.accentCurves(line); err != nil {
			return err
		}
		rules = append(rules, line)
		return nil
	}
	horizontalY := make([]float64, nr+1)
	y := top
	if horizontal[0] {
		horizontalY[0] = y - stroke/2
		y -= stroke
	}
	y -= padY
	for i, row := range rows {
		y -= ascents[i]
		for j, cell := range row {
			dx := 0.0
			switch spec.columns[j] {
			case 'c':
				dx = (widths[j] - cell.width) / 2
			case 'r':
				dx = widths[j] - cell.width
			}
			cell.object.Shift(geom.V(starts[j]+dx, y))
			cell.object.Name = fmt.Sprintf("array cell[%d,%d]", i, j)
			cells = append(cells, cell.object)
		}
		y -= depths[i]
		if i+1 < nr {
			if horizontal[i+1] {
				horizontalY[i+1] = y - gap/2 - stroke/2
				y -= stroke
			}
			y -= gap
		}
	}
	y -= padY
	if horizontal[nr] {
		horizontalY[nr] = y - stroke/2
	}
	for j, on := range spec.vertical {
		if on {
			if err := addRule(fmt.Sprintf("ArrayVRule[%d]", j), geom.V(verticalX[j], top), geom.V(verticalX[j], -depth)); err != nil {
				return box{}, err
			}
		}
	}
	for i, on := range horizontal {
		if on {
			if err := addRule(fmt.Sprintf("ArrayHRule[%d]", i), geom.V(0, horizontalY[i]), geom.V(width, horizontalY[i])); err != nil {
				return box{}, err
			}
		}
	}
	o := gomath.Group(append(rules, cells...)...)
	o.Name = "Environment array"
	return box{object: o, width: width, height: top, depth: depth, content: true}, nil
}
