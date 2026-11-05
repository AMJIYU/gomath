// Package table builds editable vector tables from text or ordinary objects.
// All entries, backgrounds and rules participate in normal scene animations.
package table

import (
	"fmt"
	"gomath"
	"gomath/geom"
	"gomath/internal/grid"
	"gomath/text"
	"math"
)

type Options struct {
	Text                        text.Options
	Font                        *text.Font
	PaddingX, PaddingY          float64
	MinCellWidth, MinCellHeight float64
	InnerLines, OuterLines      bool
	LineColor                   gomath.Color
	LineWidth                   float64
	// Labels are copied into an additional left column and/or top row. Empty
	// slices mean absent. Corner requires both sets of labels; otherwise the
	// intersection is a blank owned group. Data indices always exclude labels.
	RowLabels, ColumnLabels []*gomath.Object
	Corner                  *gomath.Object
}

// MaxLayoutCells includes labels and a possible blank corner. Check this before
// constructing text or copying objects so oversized inputs have bounded cost.
const MaxLayoutCells = 65536

func DefaultOptions() Options {
	return Options{Text: text.DefaultOptions(), Font: text.DefaultFont(),
		PaddingX: .25, PaddingY: .18, MinCellWidth: .25, MinCellHeight: .4,
		InnerLines: true, OuterLines: true, LineColor: gomath.White, LineWidth: .025}
}

// Table owns copies of its inputs. Cell coordinates are zero-based. Layout is
// computed once: changing entry geometry does not automatically reflow rules or
// backgrounds. Transforming the whole Object moves every component together.
type Table struct {
	*gomath.Object
	cells                                  *grid.Grid
	backgrounds                            [][]*gomath.Object
	lines                                  *gomath.Object
	rows, columns, rowOffset, columnOffset int
	rowLabels, columnLabels                []*gomath.Object
	corner                                 *gomath.Object
}

func New(values [][]string, opt Options) (*Table, error) {
	rows, columns, err := grid.Shape(values)
	if err != nil {
		return nil, err
	}
	if err := opt.validate(); err != nil {
		return nil, err
	}
	if err := opt.validateLabels(rows, columns); err != nil {
		return nil, err
	}
	objects := make([][]*gomath.Object, len(values))
	for i, row := range values {
		objects[i] = make([]*gomath.Object, len(row))
		for j, value := range row {
			o, err := text.New(value, opt.Font, opt.Text)
			if err != nil {
				return nil, fmt.Errorf("table entry [%d,%d]: %w", i, j, err)
			}
			objects[i][j] = o
		}
	}
	return NewObjects(objects, opt)
}

// NewObjects can contain formulae, geometry and nested groups. Blank groups are
// valid cells. A repeated source object receives an independent copy per cell.
func NewObjects(objects [][]*gomath.Object, opt Options) (*Table, error) {
	if err := opt.validate(); err != nil {
		return nil, err
	}
	rows, columns, err := grid.Shape(objects)
	if err != nil {
		return nil, err
	}
	if err := opt.validateLabels(rows, columns); err != nil {
		return nil, err
	}
	rowOffset, columnOffset := 0, 0
	if len(opt.ColumnLabels) != 0 {
		rowOffset = 1
	}
	if len(opt.RowLabels) != 0 {
		columnOffset = 1
	}
	layout := make([][]*gomath.Object, rows+rowOffset)
	if rowOffset != 0 {
		layout[0] = make([]*gomath.Object, columns+columnOffset)
		copy(layout[0][columnOffset:], opt.ColumnLabels)
		if columnOffset != 0 {
			layout[0][0] = opt.Corner
			if layout[0][0] == nil {
				layout[0][0] = gomath.Group()
			}
		}
	}
	for i, row := range objects {
		layout[i+rowOffset] = make([]*gomath.Object, columns+columnOffset)
		copy(layout[i+rowOffset][columnOffset:], row)
		if columnOffset != 0 {
			layout[i+rowOffset][0] = opt.RowLabels[i]
		}
	}
	g, err := grid.New(layout, grid.Options{HorizontalGap: 2 * opt.PaddingX,
		VerticalGap: 2 * opt.PaddingY, MinCellWidth: opt.MinCellWidth, MinCellHeight: opt.MinCellHeight})
	if err != nil {
		return nil, err
	}
	backgrounds := make([][]*gomath.Object, g.Rows)
	var backgroundObjects, rules []*gomath.Object
	for i := range g.Rows {
		backgrounds[i] = make([]*gomath.Object, g.Columns)
		for j := range g.Columns {
			x0, x1 := g.XEdges[j], g.XEdges[j+1]
			y0, y1 := g.YEdges[i], g.YEdges[i+1]
			o := gomath.Rectangle(x1-x0, y0-y1).MoveTo(geom.V((x0+x1)/2, (y0+y1)/2))
			o.SetFill(gomath.Transparent).SetStroke(gomath.Transparent, 0)
			backgrounds[i][j] = o
			backgroundObjects = append(backgroundObjects, o)
		}
	}
	for j, x := range g.XEdges {
		outer := j == 0 || j == g.Columns
		if (outer && opt.OuterLines) || (!outer && opt.InnerLines) {
			rules = append(rules, gomath.Line(geom.V(x, g.Height/2), geom.V(x, -g.Height/2)))
		}
	}
	for i, y := range g.YEdges {
		outer := i == 0 || i == g.Rows
		if (outer && opt.OuterLines) || (!outer && opt.InnerLines) {
			rules = append(rules, gomath.Line(geom.V(-g.Width/2, y), geom.V(g.Width/2, y)))
		}
	}
	lines := gomath.Group(rules...).SetStroke(opt.LineColor, opt.LineWidth)
	o := gomath.Group(gomath.Group(backgroundObjects...), g.Object, lines)
	o.Name = "Table"
	if err := o.Validate(); err != nil {
		return nil, fmt.Errorf("table layout: %w", err)
	}
	t := &Table{Object: o, cells: g, backgrounds: backgrounds, lines: lines,
		rows: rows, columns: columns, rowOffset: rowOffset, columnOffset: columnOffset}
	if columnOffset != 0 {
		for i := range rows {
			label, _ := g.Entry(i+rowOffset, 0)
			t.rowLabels = append(t.rowLabels, label)
		}
	}
	if rowOffset != 0 {
		for j := range columns {
			label, _ := g.Entry(0, j+columnOffset)
			t.columnLabels = append(t.columnLabels, label)
		}
	}
	if opt.Corner != nil {
		t.corner, _ = g.Entry(0, 0)
	}
	return t, nil
}

func (t *Table) Dimensions() (rows, columns int) { return t.rows, t.columns }
func (t *Table) Entry(row, column int) (*gomath.Object, error) {
	if row < 0 || row >= t.rows || column < 0 || column >= t.columns {
		return nil, fmt.Errorf("cell [%d,%d] outside %dx%d data table", row, column, t.rows, t.columns)
	}
	return t.cells.Entry(row+t.rowOffset, column+t.columnOffset)
}
func (t *Table) Row(row int) ([]*gomath.Object, error) {
	if _, err := t.Entry(row, 0); err != nil {
		return nil, err
	}
	entries := make([]*gomath.Object, t.columns)
	for j := range entries {
		entries[j], _ = t.Entry(row, j)
	}
	return entries, nil
}
func (t *Table) Column(column int) ([]*gomath.Object, error) {
	if _, err := t.Entry(0, column); err != nil {
		return nil, err
	}
	entries := make([]*gomath.Object, t.rows)
	for i := range entries {
		entries[i], _ = t.Entry(i, column)
	}
	return entries, nil
}
func (t *Table) Lines() *gomath.Object { return t.lines }
func (t *Table) CellBackground(row, column int) (*gomath.Object, error) {
	if _, err := t.Entry(row, column); err != nil {
		return nil, err
	}
	return t.backgrounds[row+t.rowOffset][column+t.columnOffset], nil
}

// GridDimensions and GridEntry use full zero-based layout coordinates including
// the header row/column. A missing corner is represented by an empty group.
func (t *Table) GridDimensions() (rows, columns int)               { return t.cells.Rows, t.cells.Columns }
func (t *Table) GridEntry(row, column int) (*gomath.Object, error) { return t.cells.Entry(row, column) }
func (t *Table) GridCellBackground(row, column int) (*gomath.Object, error) {
	if _, err := t.cells.Entry(row, column); err != nil {
		return nil, err
	}
	return t.backgrounds[row][column], nil
}

// Label selections contain references to the actual owned objects, with fresh
// slice storage. They are not extra parents to add alongside the table.
func (t *Table) RowLabels() []*gomath.Object { return append([]*gomath.Object(nil), t.rowLabels...) }
func (t *Table) ColumnLabels() []*gomath.Object {
	return append([]*gomath.Object(nil), t.columnLabels...)
}
func (t *Table) Corner() *gomath.Object { return t.corner }
func (t *Table) Labels() []*gomath.Object {
	labels := make([]*gomath.Object, 0, len(t.rowLabels)+len(t.columnLabels)+1)
	if t.corner != nil {
		labels = append(labels, t.corner)
	}
	labels = append(labels, t.columnLabels...)
	return append(labels, t.rowLabels...)
}

func (opt Options) validateLabels(rows, columns int) error {
	if len(opt.RowLabels) != 0 && len(opt.RowLabels) != rows {
		return fmt.Errorf("table needs %d row labels, got %d", rows, len(opt.RowLabels))
	}
	if len(opt.ColumnLabels) != 0 && len(opt.ColumnLabels) != columns {
		return fmt.Errorf("table needs %d column labels, got %d", columns, len(opt.ColumnLabels))
	}
	if opt.Corner != nil && (len(opt.RowLabels) == 0 || len(opt.ColumnLabels) == 0) {
		return fmt.Errorf("table corner requires both row and column labels")
	}
	if len(opt.RowLabels) != 0 {
		columns++
	}
	if len(opt.ColumnLabels) != 0 {
		rows++
	}
	if rows > MaxLayoutCells || columns > MaxLayoutCells || rows > MaxLayoutCells/columns {
		return fmt.Errorf("table layout exceeds %d cells", MaxLayoutCells)
	}
	return nil
}
func (t *Table) HighlightCell(row, column int, color gomath.Color) error {
	background, err := t.CellBackground(row, column)
	if err != nil {
		return err
	}
	if err := (gomath.Style{Fill: color}).Validate(); err != nil {
		return err
	}
	background.SetFill(color)
	return nil
}

func (opt Options) validate() error {
	for _, v := range []float64{opt.PaddingX, opt.PaddingY, opt.MinCellWidth, opt.MinCellHeight, opt.LineWidth} {
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
			return fmt.Errorf("table dimensions must be finite and nonnegative")
		}
	}
	if (opt.InnerLines || opt.OuterLines) && opt.LineWidth <= 0 {
		return fmt.Errorf("visible table rules need a positive width")
	}
	if err := (gomath.Style{Fill: opt.LineColor}).Validate(); err != nil {
		return err
	}
	if err := (gomath.Style{Fill: opt.Text.Color}).Validate(); err != nil {
		return err
	}
	return nil
}
