package graph

import (
	"fmt"
	"gomath"
)

type NumberPlaneOptions struct {
	GridColor, AxisColor gomath.Color
	GridWidth, AxisWidth float64
	ShowAxes             bool
}

func DefaultNumberPlaneOptions() NumberPlaneOptions {
	return NumberPlaneOptions{GridColor: gomath.RGB(.2, .3, .4), AxisColor: gomath.White, GridWidth: .012, AxisWidth: .025, ShowAxes: true}
}

// NumberPlane is a Cartesian grid with Axes coordinate mapping. It owns only
// the embedded Axes.Object tree, including grid lines and optional axis/ticks.
// Axes.Shift moves both that tree and its mapping. GridLines returns borrowed
// nodes; do not add those as separate scene roots.
type NumberPlane struct {
	*Axes
	grid []*gomath.Object
}

func NewNumberPlane(x, y Range, width, height float64, o NumberPlaneOptions) (*NumberPlane, error) {
	if !finite(o.GridWidth) || o.GridWidth <= 0 || !finite(o.AxisWidth) || o.AxisWidth <= 0 {
		return nil, fmt.Errorf("plane stroke widths must be finite and positive")
	}
	style := gomath.DefaultStyle()
	style.Stroke, style.Fill = o.GridColor, o.AxisColor
	if err := style.Validate(); err != nil {
		return nil, err
	}
	a, err := NewAxes(x, y, width, height)
	if err != nil {
		return nil, err
	}
	xTicks, err := tickValues(x)
	if err != nil {
		return nil, err
	}
	yTicks, err := tickValues(y)
	if err != nil {
		return nil, err
	}
	p := &NumberPlane{Axes: a}
	for _, value := range xTicks {
		p.grid = append(p.grid, gomath.Line(a.Point(value, y.Min), a.Point(value, y.Max)).SetStroke(o.GridColor, o.GridWidth))
	}
	for _, value := range yTicks {
		p.grid = append(p.grid, gomath.Line(a.Point(x.Min, value), a.Point(x.Max, value)).SetStroke(o.GridColor, o.GridWidth))
	}
	children := append([]*gomath.Object(nil), p.grid...)
	if o.ShowAxes {
		a.Object.SetStroke(o.AxisColor, o.AxisWidth)
		children = append(children, a.Object.Children...)
	}
	a.Object.Children = children
	if err := a.Object.Validate(); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *NumberPlane) GridLines() []*gomath.Object { return append([]*gomath.Object(nil), p.grid...) }
