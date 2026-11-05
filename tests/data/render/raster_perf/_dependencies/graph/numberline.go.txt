package graph

import (
	"fmt"
	"gomath"
	"gomath/geom"
	"gomath/text"
	"math"
	"strconv"
	"unicode/utf8"
)

type NumberLabelOptions struct {
	Font *text.Font
	Text text.Options
	Gap  float64 // Scene-space gap between the tick end and label ink bounds.
	// Format defaults to 12..17 significant digits, depending on tick spacing.
	// Empty or whitespace-only output is rejected because it would hide a label.
	Format func(float64) string
}

func DefaultNumberLabelOptions() NumberLabelOptions {
	o := text.DefaultOptions()
	o.Size = .25
	return NumberLabelOptions{Font: text.DefaultFont(), Text: o, Gap: .1}
}

type NumberLineOptions struct {
	Origin, Direction     geom.Vec3
	IncludeTicks          bool
	TickSize, StrokeWidth float64
	Color                 gomath.Color
	Labels                *NumberLabelOptions // nil disables labels
}

func DefaultNumberLineOptions() NumberLineOptions {
	return NumberLineOptions{Direction: geom.V(1, 0), IncludeTicks: true, TickSize: .12, StrokeWidth: .025, Color: gomath.White}
}

type NumberTick struct {
	Value       float64
	Tick, Label *gomath.Object // Borrowed children, nil when disabled.
}

// NumberLine owns a single renderable Object. Mapping parameters are fixed at
// construction; transforming Object alone does not update Point/Coordinate.
// Origin is the midpoint of the range. Labels stay upright on the right-hand
// side of Direction (below a horizontal line), using pure-Go text outlines.
type NumberLine struct {
	Object            *gomath.Object
	domain            Range
	length            float64
	origin, direction geom.Vec3
	ticks             []NumberTick
}

func NewNumberLine(r Range, length float64, o NumberLineOptions) (*NumberLine, error) {
	values, err := tickValues(r)
	if err != nil {
		return nil, err
	}
	if !finite(length) || length <= 0 || !o.Origin.Finite() || o.Origin.Z != 0 || !o.Direction.Finite() || o.Direction.Z != 0 || !finite(o.TickSize) || o.TickSize < 0 || o.IncludeTicks && o.TickSize == 0 || !finite(o.StrokeWidth) || o.StrokeWidth <= 0 {
		return nil, fmt.Errorf("invalid number line dimensions, origin or direction")
	}
	component := math.Max(math.Abs(o.Direction.X), math.Abs(o.Direction.Y))
	if component == 0 {
		return nil, fmt.Errorf("number line direction cannot be zero")
	}
	d := geom.V(o.Direction.X/component, o.Direction.Y/component)
	d = d.Mul(1 / math.Hypot(d.X, d.Y))
	style := gomath.DefaultStyle()
	style.Stroke = o.Color
	if err := style.Validate(); err != nil {
		return nil, err
	}
	if o.Labels != nil {
		if len(values) == 0 {
			return nil, fmt.Errorf("labels requested but the range has no tick locations")
		}
		if len(values) > 1000 {
			return nil, fmt.Errorf("number line supports at most 1000 labels")
		}
		if err := o.Labels.Font.Validate(); err != nil {
			return nil, err
		}
		if !finite(o.Labels.Gap) || o.Labels.Gap < 0 {
			return nil, fmt.Errorf("label gap must be finite and nonnegative")
		}
	}
	n := &NumberLine{domain: r, length: length, origin: o.Origin, direction: d, ticks: make([]NumberTick, 0, len(values))}
	start, end := n.Point(r.Min), n.Point(r.Max)
	if !start.Finite() || !end.Finite() || start == end {
		return nil, fmt.Errorf("number line endpoints overflow or lose precision")
	}
	objects := []*gomath.Object{gomath.Line(start, end).SetStroke(o.Color, o.StrokeWidth)}
	normal := geom.V(-d.Y, d.X)
	labelRunes := 0
	for _, value := range values {
		tick := NumberTick{Value: value}
		p := n.Point(value)
		if !p.Finite() {
			return nil, fmt.Errorf("tick position overflows at %g", value)
		}
		if o.IncludeTicks {
			delta := normal.Mul(o.TickSize / 2)
			if p.Add(delta) == p.Sub(delta) {
				return nil, fmt.Errorf("tick at %g loses precision", value)
			}
			tick.Tick = gomath.Line(p.Sub(delta), p.Add(delta)).SetStroke(o.Color, o.StrokeWidth)
			objects = append(objects, tick.Tick)
		}
		if o.Labels != nil {
			digits := 12
			if ratio := math.Abs(value) / r.Step; ratio > 1 {
				digits = min(17, max(digits, int(math.Ceil(math.Log10(ratio)))+1))
			}
			label := strconv.FormatFloat(value, 'g', digits, 64)
			if o.Labels.Format != nil {
				label = o.Labels.Format(value)
			}
			if len(label) > 4*16384 {
				return nil, fmt.Errorf("number line label exceeds 16384 runes")
			}
			labelRunes += utf8.RuneCountInString(label)
			if labelRunes > 16384 {
				return nil, fmt.Errorf("number line labels exceed 16384 total runes")
			}
			object, err := text.New(label, o.Labels.Font, o.Labels.Text)
			if err != nil {
				return nil, fmt.Errorf("label at %g: %w", value, err)
			}
			b := object.Bounds()
			if !b.Valid {
				return nil, fmt.Errorf("label at %g has no visible glyphs", value)
			}
			extent := (math.Abs(normal.X)*b.Size().X + math.Abs(normal.Y)*b.Size().Y) / 2
			tickExtent := 0.0
			if o.IncludeTicks {
				tickExtent = o.TickSize / 2
			}
			object.MoveTo(p.Sub(normal.Mul(tickExtent + o.Labels.Gap + extent)))
			tick.Label = object
			objects = append(objects, object)
		}
		n.ticks = append(n.ticks, tick)
	}
	n.Object = gomath.Group(objects...)
	if err := n.Object.Validate(); err != nil {
		return nil, err
	}
	return n, nil
}

// Point maps a value to the line, extrapolating outside the configured range.
func (n *NumberLine) Point(value float64) geom.Vec3 {
	t := (value-n.domain.Min)/(n.domain.Max-n.domain.Min) - .5
	return n.origin.Add(n.direction.Mul(t * n.length))
}

// Coordinate projects p onto the line. Perpendicular offsets are ignored.
func (n *NumberLine) Coordinate(p geom.Vec3) float64 {
	t := p.Sub(n.origin).Dot(n.direction)/n.length + .5
	return n.domain.Min + t*(n.domain.Max-n.domain.Min)
}

func (n *NumberLine) Ticks() []NumberTick { return append([]NumberTick(nil), n.ticks...) }
