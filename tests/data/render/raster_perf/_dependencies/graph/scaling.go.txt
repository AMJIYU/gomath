package graph

import (
	"fmt"
	"gomath"
	"gomath/geom"
	"gomath/path"
	"math"
	"strconv"
)

// Scale is an immutable increasing coordinate-to-data map. Its zero value is
// identity. Constructors support a positive linear factor or log base >1.
// For a logarithmic scale Forward(u)=base^u and Inverse(x)=log_base(x).
type Scale struct {
	kind                 uint8
	parameter, logarithm float64
}

func NewLinearScale(factor float64) (Scale, error) {
	if !finite(factor) || factor <= 0 {
		return Scale{}, fmt.Errorf("linear scale factor must be finite and positive")
	}
	return Scale{kind: 1, parameter: factor}, nil
}
func NewLogScale(base float64) (Scale, error) {
	if !finite(base) || base <= 1 {
		return Scale{}, fmt.Errorf("logarithmic scale base must be finite and greater than one")
	}
	return Scale{kind: 2, parameter: base, logarithm: math.Log(base)}, nil
}

func (s Scale) Forward(coordinate float64) (float64, error) {
	if !finite(coordinate) {
		return 0, fmt.Errorf("scale coordinate must be finite")
	}
	value := coordinate
	switch s.kind {
	case 1:
		value *= s.parameter
	case 2:
		value = math.Pow(s.parameter, coordinate)
	}
	if !finite(value) || s.kind == 2 && value <= 0 || s.kind == 1 && coordinate != 0 && value == 0 {
		return 0, fmt.Errorf("scaled value overflows or underflows at %g", coordinate)
	}
	return value, nil
}
func (s Scale) Inverse(value float64) (float64, error) {
	if !finite(value) {
		return 0, fmt.Errorf("scale data value must be finite")
	}
	coordinate := value
	switch s.kind {
	case 1:
		coordinate /= s.parameter
	case 2:
		if value <= 0 {
			return 0, fmt.Errorf("logarithmic scale requires positive data values")
		}
		coordinate = math.Log(value) / s.logarithm
	}
	if !finite(coordinate) || s.kind == 1 && value != 0 && coordinate == 0 {
		return 0, fmt.Errorf("inverse scale overflows or underflows at %g", value)
	}
	return coordinate, nil
}
func (s Scale) bounds(r Range) (float64, float64, error) {
	if err := validateRange(r); err != nil {
		return 0, 0, err
	}
	lo, err := s.Forward(r.Min)
	if err != nil {
		return 0, 0, err
	}
	hi, err := s.Forward(r.Max)
	if err != nil {
		return 0, 0, err
	}
	if hi <= lo {
		return 0, 0, fmt.Errorf("scaled bounds lose increasing data precision")
	}
	return lo, hi, nil
}

// ScaledNumberLine uses Range in coordinate units (exponents for log scales).
// Point/Coordinate and tick values use data units. Its mapping is fixed at
// construction; transforming Object alone does not change it.
type ScaledNumberLine struct {
	Object    *gomath.Object
	line      *NumberLine
	scale     Scale
	low, high float64
}

func NewScaledNumberLine(r Range, length float64, scale Scale, opt NumberLineOptions) (*ScaledNumberLine, error) {
	lo, hi, err := scale.bounds(r)
	if err != nil {
		return nil, err
	}
	values, err := tickValues(r)
	if err != nil {
		return nil, err
	}
	previous := math.Inf(-1)
	for _, value := range values {
		data, err := scale.Forward(value)
		if err != nil {
			return nil, err
		}
		if data <= previous {
			return nil, fmt.Errorf("scaled tick values lose increasing precision")
		}
		previous = data
	}
	if opt.Labels != nil {
		labels := *opt.Labels
		format := labels.Format
		labels.Format = func(coordinate float64) string {
			value, _ := scale.Forward(coordinate) // every tick was checked above
			if format != nil {
				return format(value)
			}
			return strconv.FormatFloat(value, 'g', -1, 64)
		}
		opt.Labels = &labels
	}
	line, err := NewNumberLine(r, length, opt)
	if err != nil {
		return nil, err
	}
	return &ScaledNumberLine{Object: line.Object, line: line, scale: scale, low: lo, high: hi}, nil
}
func (n *ScaledNumberLine) Point(value float64) (geom.Vec3, error) {
	if n == nil || n.line == nil {
		return geom.Vec3{}, fmt.Errorf("uninitialized scaled number line")
	}
	u, err := n.scale.Inverse(value)
	if err != nil {
		return geom.Vec3{}, err
	}
	p := n.line.Point(u)
	if !p.Finite() {
		return geom.Vec3{}, fmt.Errorf("number line point overflows")
	}
	return p, nil
}
func (n *ScaledNumberLine) Coordinate(point geom.Vec3) (float64, error) {
	if n == nil || n.line == nil || !point.Finite() {
		return 0, fmt.Errorf("scaled number line requires initialized mapping and finite point")
	}
	return n.scale.Forward(n.line.Coordinate(point))
}
func (n *ScaledNumberLine) DataBounds() (float64, float64) {
	if n == nil {
		return 0, 0
	}
	return n.low, n.high
}
func (n *ScaledNumberLine) Ticks() []NumberTick {
	if n == nil || n.line == nil {
		return nil
	}
	ticks := n.line.Ticks()
	for i := range ticks {
		ticks[i].Value, _ = n.scale.Forward(ticks[i].Value)
	}
	return ticks
}

// ScaledAxes places ticks uniformly in scale coordinates. Range values are
// exponents for log axes; the perpendicular axis crosses at scale coordinate 0
// (data 1 on a log axis). This is an independent checked API, not an alteration
// of Axes.Point's existing unchecked linear contract.
type ScaledAxes struct {
	Object         *gomath.Object
	axes           *Axes
	xScale, yScale Scale
}

func NewScaledAxes(x, y Range, width, height float64, xScale, yScale Scale) (*ScaledAxes, error) {
	if _, _, err := xScale.bounds(x); err != nil {
		return nil, fmt.Errorf("scaled X: %w", err)
	}
	if _, _, err := yScale.bounds(y); err != nil {
		return nil, fmt.Errorf("scaled Y: %w", err)
	}
	axes, err := NewAxes(x, y, width, height)
	if err != nil {
		return nil, err
	}
	return &ScaledAxes{Object: axes.Object, axes: axes, xScale: xScale, yScale: yScale}, nil
}
func (a *ScaledAxes) Point(x, y float64) (geom.Vec3, error) {
	if a == nil || a.axes == nil {
		return geom.Vec3{}, fmt.Errorf("uninitialized scaled axes")
	}
	u, err := a.xScale.Inverse(x)
	if err != nil {
		return geom.Vec3{}, fmt.Errorf("scaled X: %w", err)
	}
	v, err := a.yScale.Inverse(y)
	if err != nil {
		return geom.Vec3{}, fmt.Errorf("scaled Y: %w", err)
	}
	return a.axes.mappedPoint(u, v)
}
func (a *ScaledAxes) Coordinates(point geom.Vec3) (float64, float64, error) {
	if a == nil || a.axes == nil || !point.Finite() {
		return 0, 0, fmt.Errorf("scaled axes require initialized mapping and finite point")
	}
	u, v := a.axes.Coordinates(point)
	x, err := a.xScale.Forward(u)
	if err != nil {
		return 0, 0, err
	}
	y, err := a.yScale.Forward(v)
	if err != nil {
		return 0, 0, err
	}
	return x, y, nil
}

// Plot samples uniformly in X scale coordinates, calling f with actual data
// values. NaN/Inf function results break contours; finite values outside a log
// domain, arithmetic overflow or precision loss return no partial object.
func (a *ScaledAxes) Plot(f func(float64) float64, samples int) (*gomath.Object, error) {
	if a == nil || a.axes == nil || f == nil || samples < 2 || samples > 1_000_000 {
		return nil, fmt.Errorf("scaled plot requires initialized axes, a function and 2..1000000 samples")
	}
	var builder path.Builder
	open := false
	previous := math.Inf(-1)
	for i := range samples {
		u := partitionPoint(a.axes.X.Min, a.axes.X.Max, i, samples-1)
		x, err := a.xScale.Forward(u)
		if err != nil {
			return nil, err
		}
		if x <= previous {
			return nil, fmt.Errorf("scaled plot samples lose increasing data precision")
		}
		previous = x
		y := f(x)
		if !finite(y) {
			open = false
			continue
		}
		v, err := a.yScale.Inverse(y)
		if err != nil {
			return nil, err
		}
		p, err := a.axes.mappedPoint(u, v)
		if err != nil {
			return nil, err
		}
		if open {
			builder.LineTo(p)
		} else {
			builder.MoveTo(p)
			open = true
		}
	}
	object := gomath.NewObject(builder.Path()).SetStroke(gomath.Blue, .035)
	if err := object.Validate(); err != nil {
		return nil, err
	}
	return object, nil
}
