package graph

import (
	"fmt"
	"gomath"
	"gomath/geom"
	"math"
)

// RegionStyle colors positive/negative heights (first function minus second,
// or function minus baseline), independently of integration orientation.
type RegionStyle struct {
	PositiveColor, NegativeColor gomath.Color
	StrokeColor                  gomath.Color
	StrokeWidth, Opacity         float64
}

func DefaultRegionStyle() RegionStyle {
	return RegionStyle{PositiveColor: gomath.Blue, NegativeColor: gomath.Red, StrokeColor: gomath.White, StrokeWidth: .008, Opacity: .6}
}

func (s RegionStyle) validate() error {
	style := gomath.DefaultStyle()
	style.Fill, style.Stroke, style.StrokeWidth, style.Opacity = s.PositiveColor, s.StrokeColor, s.StrokeWidth, s.Opacity
	if err := style.Validate(); err != nil {
		return err
	}
	style.Fill = s.NegativeColor
	return style.Validate()
}

func (s RegionStyle) apply(object *gomath.Object, height float64) *gomath.Object {
	color := s.PositiveColor
	if height < 0 {
		color = s.NegativeColor
	}
	return object.SetFill(color).SetStroke(s.StrokeColor, s.StrokeWidth).SetOpacity(s.Opacity)
}

type RiemannSample string

const (
	LeftSample  RiemannSample = "left"
	MidSample   RiemannSample = "mid"
	RightSample RiemannSample = "right"
)

type RiemannOptions struct {
	Rectangles int
	Sample     RiemannSample
	Baseline   float64
	Style      RegionStyle
}

func DefaultRiemannOptions() RiemannOptions {
	return RiemannOptions{Rectangles: 20, Sample: MidSample, Style: DefaultRegionStyle()}
}

type RiemannCell struct {
	Left, Right, SampleX, Value, Baseline, SignedArea float64
	Object                                            *gomath.Object // Borrowed child of RiemannRectangles.Object.
}

// RiemannRectangles is both a geometric object and a numerical sum in data
// units, not scene units. Cells are stored in ascending X order. SignedArea and
// each cell's contribution include the orientation of the requested interval.
type RiemannRectangles struct {
	Object     *gomath.Object
	SignedArea float64
	cells      []RiemannCell
}

func (r *RiemannRectangles) Cells() []RiemannCell { return append([]RiemannCell(nil), r.cells...) }

// RiemannRectangles constructs equal-width partitions of an explicit interval,
// approximating integral(f(x)-Baseline) dx. Reversed endpoints reverse the
// integral sign; geometry still spans the ascending interval. Values below a
// negative or positive baseline retain their negative height. Non-finite
// samples, arithmetic overflow and unrepresentable partitions return errors.
// Left/right sampling always refers to ascending X, including reversed bounds.
// Sampling cannot detect singularities or finite jumps between samples.
func (a *Axes) RiemannRectangles(f func(float64) float64, start, end float64, o RiemannOptions) (*RiemannRectangles, error) {
	if err := a.validateMapping(); err != nil {
		return nil, err
	}
	lo, hi, orientation, err := interval(start, end)
	if err != nil {
		return nil, err
	}
	if f == nil || o.Rectangles < 1 || o.Rectangles > 100_000 || !finite(o.Baseline) {
		return nil, fmt.Errorf("Riemann rectangles require a function, 1..100000 rectangles and a finite baseline")
	}
	if o.Sample != LeftSample && o.Sample != MidSample && o.Sample != RightSample {
		return nil, fmt.Errorf("unsupported Riemann sampling %q", o.Sample)
	}
	if err := o.Style.validate(); err != nil {
		return nil, err
	}
	r := &RiemannRectangles{cells: make([]RiemannCell, 0, o.Rectangles)}
	objects := make([]*gomath.Object, 0, o.Rectangles)
	var sum compensated
	for i := range o.Rectangles {
		left, right := partitionPoint(lo, hi, i, o.Rectangles), partitionPoint(lo, hi, i+1, o.Rectangles)
		if left >= right {
			return nil, fmt.Errorf("rectangle %d partition loses precision", i)
		}
		x := left
		switch o.Sample {
		case MidSample:
			x = left + (right-left)/2
			if x <= left || x >= right {
				return nil, fmt.Errorf("rectangle %d midpoint loses precision", i)
			}
		case RightSample:
			x = right
		}
		value := f(x)
		height := value - o.Baseline
		if !finite(value) || !finite(height) {
			return nil, fmt.Errorf("non-finite Riemann sample/height at %g", x)
		}
		contribution := orientation * height * (right - left)
		if err := sum.add(contribution); err != nil {
			return nil, err
		}
		object, err := a.regionPolygon(geom.V(left, o.Baseline), geom.V(right, o.Baseline), geom.V(right, value), geom.V(left, value))
		if err != nil {
			return nil, err
		}
		o.Style.apply(object, height)
		r.cells = append(r.cells, RiemannCell{Left: left, Right: right, SampleX: x, Value: value, Baseline: o.Baseline, SignedArea: contribution, Object: object})
		objects = append(objects, object)
	}
	r.SignedArea, r.Object = sum.sum, gomath.Group(objects...)
	return r, nil
}

type AreaOptions struct {
	Samples int // Uniform sample points, including both endpoints; 2..100001.
	Style   RegionStyle
}

func DefaultAreaOptions() AreaOptions { return AreaOptions{Samples: 101, Style: DefaultRegionStyle()} }

type AreaSample struct{ X, First, Second float64 }

// Area encloses two uniformly sampled piecewise-linear functions. Area is the
// nonnegative geometric area in data units, while SignedArea approximates the
// oriented integral(first-second) dx. Neither is a certified analytic integral.
type Area struct {
	Object           *gomath.Object
	Area, SignedArea float64
	samples          []AreaSample
}

func (r *Area) Samples() []AreaSample { return append([]AreaSample(nil), r.samples...) }

// AreaBetween samples both functions over a finite explicit interval. It splits
// trapezoids at crossings of the sampled linear interpolants, avoiding
// self-intersecting polygons and fill cancellation. Crossings/oscillations
// between samples remain unknown; callers must increase Samples or split the
// interval. Any sampled non-finite value is an error, never an implicit bridge.
func (a *Axes) AreaBetween(first, second func(float64) float64, start, end float64, o AreaOptions) (*Area, error) {
	if err := a.validateMapping(); err != nil {
		return nil, err
	}
	lo, hi, orientation, err := interval(start, end)
	if err != nil {
		return nil, err
	}
	if first == nil || second == nil || o.Samples < 2 || o.Samples > 100_001 {
		return nil, fmt.Errorf("area requires two functions and 2..100001 sample points")
	}
	if err := o.Style.validate(); err != nil {
		return nil, err
	}
	r := &Area{samples: make([]AreaSample, o.Samples)}
	for i := range r.samples {
		x := partitionPoint(lo, hi, i, o.Samples-1)
		if i > 0 && x <= r.samples[i-1].X {
			return nil, fmt.Errorf("area partition loses precision at sample %d", i)
		}
		f, g := first(x), second(x)
		if !finite(f) || !finite(g) || !finite(f-g) {
			return nil, fmt.Errorf("non-finite area sample/height at %g", x)
		}
		r.samples[i] = AreaSample{x, f, g}
	}
	objects := make([]*gomath.Object, 0, o.Samples-1)
	var signed, absolute compensated
	addStrip := func(left, right AreaSample) error {
		d0, d1 := left.First-left.Second, right.First-right.Second
		if d0 == 0 && d1 == 0 {
			return nil
		}
		contribution := (d0/2 + d1/2) * (right.X - left.X)
		if err := signed.add(orientation * contribution); err != nil {
			return err
		}
		if err := absolute.add(math.Abs(contribution)); err != nil {
			return err
		}
		points := []geom.Vec3{geom.V(left.X, left.Second), geom.V(right.X, right.Second)}
		if d1 != 0 {
			points = append(points, geom.V(right.X, right.First))
		}
		if d0 != 0 {
			points = append(points, geom.V(left.X, left.First))
		}
		object, err := a.regionPolygon(points...)
		if err != nil {
			return err
		}
		height := d0
		if height == 0 {
			height = d1
		}
		objects = append(objects, o.Style.apply(object, height))
		return nil
	}
	for i := 1; i < len(r.samples); i++ {
		left, right := r.samples[i-1], r.samples[i]
		d0, d1 := left.First-left.Second, right.First-right.Second
		if d0 < 0 && d1 > 0 || d0 > 0 && d1 < 0 {
			// Scale before dividing to avoid overflow in abs(d0)+abs(d1).
			scale := math.Max(math.Abs(d0), math.Abs(d1))
			t := (math.Abs(d0) / scale) / (math.Abs(d0)/scale + math.Abs(d1)/scale)
			x := left.X + (right.X-left.X)*t
			if x <= left.X || x >= right.X {
				return nil, fmt.Errorf("area crossing loses coordinate precision")
			}
			y := left.First*(1-t) + right.First*t
			crossing := AreaSample{X: x, First: y, Second: y}
			if !finite(y) {
				return nil, fmt.Errorf("area crossing overflows")
			}
			if err := addStrip(left, crossing); err != nil {
				return nil, err
			}
			if err := addStrip(crossing, right); err != nil {
				return nil, err
			}
		} else if err := addStrip(left, right); err != nil {
			return nil, err
		}
	}
	r.SignedArea, r.Area, r.Object = signed.sum, absolute.sum, gomath.Group(objects...)
	return r, nil
}

func (a *Axes) regionPolygon(data ...geom.Vec3) (*gomath.Object, error) {
	points := make([]geom.Vec3, len(data))
	for i, p := range data {
		var err error
		points[i], err = a.mappedPoint(p.X, p.Y)
		if err != nil {
			return nil, err
		}
		if i > 0 && data[i] != data[i-1] && points[i] == points[i-1] {
			return nil, fmt.Errorf("region edge loses coordinate precision")
		}
	}
	if len(data) > 1 && data[0] != data[len(data)-1] && points[0] == points[len(points)-1] {
		return nil, fmt.Errorf("region closing edge loses coordinate precision")
	}
	object := gomath.Polygon(points...)
	if err := object.Validate(); err != nil {
		return nil, err
	}
	return object, nil
}
