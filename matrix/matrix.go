// Package matrix constructs editable vector matrix notation. It performs visual
// layout; it is not a numerical linear-algebra package or a full TeX engine.
package matrix

import (
	"fmt"
	"gomath"
	"gomath/geom"
	"gomath/internal/grid"
	"gomath/mathtex"
	"gomath/path"
	"math"
	"strconv"
)

type Bracket string

const (
	Square Bracket = "square"
	Round  Bracket = "round"
	Curly  Bracket = "curly"
	Angle  Bracket = "angle"
	Bar    Bracket = "bar"
	None   Bracket = "none"
)

type Options struct {
	Math mathtex.Options
	// Gaps are added to each column/row's largest ink extent. Half a gap
	// remains at the outside of the grid, including around blank cells.
	ColumnGap, RowGap          float64
	Bracket                    Bracket
	BracketGap, BracketPadding float64
	BracketWidth, StrokeWidth  float64
}

func DefaultOptions() Options {
	return Options{Math: mathtex.DefaultOptions(), ColumnGap: .5, RowGap: .3,
		Bracket: Square, BracketGap: .08, BracketPadding: .08, BracketWidth: .18, StrokeWidth: .035}
}

// Matrix owns one ordinary object tree. Entry/Row/Column return live references
// suitable for individual animations. Do not replace its Children directly or
// morph the whole matrix to a different tree topology and then reuse selections.
type Matrix struct {
	*gomath.Object
	cells    *grid.Grid
	brackets *gomath.Object
}

func New(values [][]string, opt Options) (*Matrix, error) {
	if _, _, err := grid.Shape(values); err != nil {
		return nil, err
	}
	if err := opt.validate(); err != nil {
		return nil, err
	}
	objects := make([][]*gomath.Object, len(values))
	for i, row := range values {
		objects[i] = make([]*gomath.Object, len(row))
		for j, value := range row {
			o, err := mathtex.New(value, opt.Math)
			if err != nil {
				return nil, fmt.Errorf("matrix entry [%d,%d]: %w", i, j, err)
			}
			objects[i][j] = o
		}
	}
	return NewObjects(objects, opt)
}

// NewObjects copies every cell; the same source may safely appear repeatedly.
// Input objects and their original positions remain unchanged.
func NewObjects(objects [][]*gomath.Object, opt Options) (*Matrix, error) {
	if err := opt.validate(); err != nil {
		return nil, err
	}
	g, err := grid.New(objects, grid.Options{HorizontalGap: opt.ColumnGap,
		VerticalGap: opt.RowGap, MinCellWidth: .25 * opt.Math.Size, MinCellHeight: .7 * opt.Math.Size})
	if err != nil {
		return nil, err
	}
	brackets := gomath.Group()
	if opt.Bracket != None {
		width := opt.BracketWidth
		if opt.Bracket == Bar {
			width = 0
		}
		left := bracket(opt.Bracket, width, g.Height+2*opt.BracketPadding)
		left.Shift(geom.V(-g.Width/2-opt.BracketGap-width, 0))
		right := left.Copy().ApplyMatrix(geom.Scaling(-1, 1, 1))
		brackets.Children = []*gomath.Object{left, right}
		brackets.SetStroke(opt.Math.Color, opt.StrokeWidth)
	}
	o := gomath.Group(g.Object, brackets)
	o.Name = "Matrix"
	if err := o.Validate(); err != nil {
		return nil, fmt.Errorf("matrix layout: %w", err)
	}
	return &Matrix{Object: o, cells: g, brackets: brackets}, nil
}

func NewIntegers(values [][]int64, opt Options) (*Matrix, error) {
	if _, _, err := grid.Shape(values); err != nil {
		return nil, err
	}
	strings := make([][]string, len(values))
	for i, row := range values {
		strings[i] = make([]string, len(row))
		for j, value := range row {
			strings[i][j] = strconv.FormatInt(value, 10)
		}
	}
	return New(strings, opt)
}

// NewDecimals uses Go's fixed-point formatting, with 0..15 fractional digits.
// It rejects NaN/Inf and does not claim Manim DecimalNumber formatting parity.
func NewDecimals(values [][]float64, digits int, opt Options) (*Matrix, error) {
	if digits < 0 || digits > 15 {
		return nil, fmt.Errorf("decimal digits must be in [0,15]")
	}
	if _, _, err := grid.Shape(values); err != nil {
		return nil, err
	}
	strings := make([][]string, len(values))
	for i, row := range values {
		strings[i] = make([]string, len(row))
		for j, value := range row {
			if math.IsNaN(value) || math.IsInf(value, 0) {
				return nil, fmt.Errorf("matrix entry [%d,%d] is not finite", i, j)
			}
			strings[i][j] = strconv.FormatFloat(value, 'f', digits, 64)
		}
	}
	return New(strings, opt)
}

// Indices are zero-based, with row zero at the top and column zero at the left.
func (m *Matrix) Dimensions() (rows, columns int) { return m.cells.Rows, m.cells.Columns }
func (m *Matrix) Entry(row, column int) (*gomath.Object, error) {
	return m.cells.Entry(row, column)
}
func (m *Matrix) Row(row int) ([]*gomath.Object, error)       { return m.cells.Row(row) }
func (m *Matrix) Column(column int) ([]*gomath.Object, error) { return m.cells.Column(column) }
func (m *Matrix) Brackets() *gomath.Object                    { return m.brackets }

// Color operations validate before mutating any cell. They leave brackets alone.
func (m *Matrix) SetRowColor(row int, color gomath.Color) error {
	objects, err := m.Row(row)
	return colorSelection(objects, color, err)
}
func (m *Matrix) SetColumnColor(column int, color gomath.Color) error {
	objects, err := m.Column(column)
	return colorSelection(objects, color, err)
}
func colorSelection(objects []*gomath.Object, color gomath.Color, err error) error {
	if err != nil {
		return err
	}
	if err := (gomath.Style{Fill: color}).Validate(); err != nil {
		return err
	}
	for _, o := range objects {
		o.SetColor(color)
	}
	return nil
}

func (opt Options) validate() error {
	for _, v := range []float64{opt.ColumnGap, opt.RowGap, opt.BracketGap, opt.BracketPadding, opt.BracketWidth, opt.StrokeWidth} {
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
			return fmt.Errorf("matrix spacing and strokes must be finite and nonnegative")
		}
	}
	if math.IsNaN(opt.Math.Size) || math.IsInf(opt.Math.Size, 0) || opt.Math.Size <= 0 {
		return fmt.Errorf("matrix size must be finite and positive")
	}
	if err := (gomath.Style{Fill: opt.Math.Color}).Validate(); err != nil {
		return err
	}
	switch opt.Bracket {
	case Square, Round, Curly, Angle:
		if opt.BracketWidth <= 0 || opt.StrokeWidth <= 0 {
			return fmt.Errorf("visible brackets need positive width and stroke")
		}
	case Bar:
		if opt.StrokeWidth <= 0 {
			return fmt.Errorf("bars need a positive stroke")
		}
	case None:
	default:
		return fmt.Errorf("unsupported matrix bracket %q", opt.Bracket)
	}
	return nil
}

// bracket returns the left delimiter in [0,width] x [-height/2,height/2].
// Curves are geometric outlines, independent of font-specific TeX delimiters.
func bracket(kind Bracket, width, height float64) *gomath.Object {
	var b path.Builder
	h := height / 2
	switch kind {
	case Square:
		b.MoveTo(geom.V(width, h))
		b.LineTo(geom.V(0, h))
		b.LineTo(geom.V(0, -h))
		b.LineTo(geom.V(width, -h))
	case Round:
		b.MoveTo(geom.V(width, h))
		b.CubicTo(geom.V(width*.15, h*.7), geom.V(0, h*.4), geom.V(0, 0))
		b.CubicTo(geom.V(0, -h*.4), geom.V(width*.15, -h*.7), geom.V(width, -h))
	case Curly:
		b.MoveTo(geom.V(width, h))
		b.CubicTo(geom.V(width*.5, h), geom.V(width*.5, h*.8), geom.V(width*.5, h*.5))
		b.CubicTo(geom.V(width*.5, h*.15), geom.V(width*.5, h*.1), geom.V(0, 0))
		b.CubicTo(geom.V(width*.5, -h*.1), geom.V(width*.5, -h*.15), geom.V(width*.5, -h*.5))
		b.CubicTo(geom.V(width*.5, -h*.8), geom.V(width*.5, -h), geom.V(width, -h))
	case Angle:
		b.MoveTo(geom.V(width, h))
		b.LineTo(geom.V(0, 0))
		b.LineTo(geom.V(width, -h))
	case Bar:
		b.MoveTo(geom.V(0, h))
		b.LineTo(geom.V(0, -h))
	}
	return gomath.NewObject(b.Path())
}
