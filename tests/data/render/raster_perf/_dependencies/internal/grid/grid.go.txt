// Package grid owns copied cells and measures their ink bounds for matrix and
// table layout. It has no font or typesetting dependency.
package grid

import (
	"fmt"
	"gomath"
	"gomath/geom"
	"math"
)

type Options struct {
	HorizontalGap, VerticalGap  float64
	MinCellWidth, MinCellHeight float64
}

// Grid is a construction-time layout. Edges describe the initial centered
// rectangle; they are not updated when its Object is transformed later.
type Grid struct {
	Object         *gomath.Object
	Rows, Columns  int
	Width, Height  float64
	XEdges, YEdges []float64
	entries        [][]*gomath.Object
}

// Shape checks all row lengths before any objects are copied or laid out.
func Shape[T any](rows [][]T) (int, int, error) {
	if len(rows) == 0 || len(rows[0]) == 0 {
		return 0, 0, fmt.Errorf("grid must have at least one row and column")
	}
	for i, row := range rows {
		if len(row) != len(rows[0]) {
			return 0, 0, fmt.Errorf("row %d has %d columns; want %d", i, len(row), len(rows[0]))
		}
	}
	return len(rows), len(rows[0]), nil
}

// New always copies input cells, including repeated source pointers. It never
// repositions the caller's objects. Blank cells occupy the minimum dimensions.
func New(rows [][]*gomath.Object, opt Options) (*Grid, error) {
	nr, nc, err := Shape(rows)
	if err != nil {
		return nil, err
	}
	for _, v := range []float64{opt.HorizontalGap, opt.VerticalGap, opt.MinCellWidth, opt.MinCellHeight} {
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
			return nil, fmt.Errorf("grid dimensions must be finite and nonnegative")
		}
	}
	var (
		widths  = make([]float64, nc)
		heights = make([]float64, nr)
	)
	for i, row := range rows {
		for j, o := range row {
			if err := o.Validate(); err != nil {
				return nil, fmt.Errorf("cell [%d,%d]: %w", i, j, err)
			}
			size := o.Bounds().Size()
			if !size.Finite() {
				return nil, fmt.Errorf("cell [%d,%d] bounds overflow", i, j)
			}
			widths[j] = math.Max(widths[j], math.Max(opt.MinCellWidth, size.X))
			heights[i] = math.Max(heights[i], math.Max(opt.MinCellHeight, size.Y))
		}
	}
	g := &Grid{Rows: nr, Columns: nc, XEdges: make([]float64, nc+1), YEdges: make([]float64, nr+1), entries: make([][]*gomath.Object, nr)}
	for _, w := range widths {
		g.Width += w + opt.HorizontalGap
	}
	for _, h := range heights {
		g.Height += h + opt.VerticalGap
	}
	if g.Width <= 0 || g.Height <= 0 || math.IsInf(g.Width, 0) || math.IsInf(g.Height, 0) {
		return nil, fmt.Errorf("grid needs finite positive total dimensions")
	}
	g.XEdges[0], g.YEdges[0] = -g.Width/2, g.Height/2
	for j, w := range widths {
		g.XEdges[j+1] = g.XEdges[j] + w + opt.HorizontalGap
	}
	for i, h := range heights {
		g.YEdges[i+1] = g.YEdges[i] - h - opt.VerticalGap
	}
	var cells []*gomath.Object
	for i, row := range rows {
		g.entries[i] = make([]*gomath.Object, nc)
		for j, o := range row {
			copy := o.Copy()
			copy.MoveTo(geom.V((g.XEdges[j]+g.XEdges[j+1])/2, (g.YEdges[i]+g.YEdges[i+1])/2))
			g.entries[i][j] = copy
			cells = append(cells, copy)
		}
	}
	g.Object = gomath.Group(cells...)
	if err := g.Object.Validate(); err != nil {
		return nil, fmt.Errorf("laid out grid: %w", err)
	}
	return g, nil
}

func (g *Grid) Entry(row, column int) (*gomath.Object, error) {
	if row < 0 || row >= g.Rows || column < 0 || column >= g.Columns {
		return nil, fmt.Errorf("cell [%d,%d] outside %dx%d grid", row, column, g.Rows, g.Columns)
	}
	return g.entries[row][column], nil
}

// Row and Column return fresh slices containing references to owned cells.
// These are selections, not additional parents to add to the same scene.
func (g *Grid) Row(row int) ([]*gomath.Object, error) {
	if row < 0 || row >= g.Rows {
		return nil, fmt.Errorf("row %d outside [0,%d)", row, g.Rows)
	}
	return append([]*gomath.Object(nil), g.entries[row]...), nil
}
func (g *Grid) Column(column int) ([]*gomath.Object, error) {
	if column < 0 || column >= g.Columns {
		return nil, fmt.Errorf("column %d outside [0,%d)", column, g.Columns)
	}
	objects := make([]*gomath.Object, g.Rows)
	for i := range objects {
		objects[i] = g.entries[i][column]
	}
	return objects, nil
}
