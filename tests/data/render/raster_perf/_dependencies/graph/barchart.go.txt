package graph

import (
	"fmt"
	"gomath"
	"gomath/geom"
	"gomath/path"
	"gomath/text"
	"math"
	"unicode/utf8"
)

const (
	MaxBars           = 4096
	MaxBarLabelCurves = 1 << 20
)

type BarChartOptions struct {
	BarWidth, FillOpacity, StrokeWidth float64
	Colors                             []gomath.Color
	Names                              []string // Optional; exactly one per bar.
	Labels                             NumberLabelOptions
}

func DefaultBarChartOptions() BarChartOptions {
	return BarChartOptions{BarWidth: .6, FillOpacity: .7, StrokeWidth: .018,
		Colors: []gomath.Color{gomath.Blue, gomath.Pink, gomath.Yellow}, Labels: DefaultNumberLabelOptions()}
}

// BarChart owns signed rectangular bars and an affine chart frame. Transforming
// the whole Object moves that frame too; later Point/SetValues/TargetBars calls
// follow it. Removing/replacing the frame or bar children is unsupported.
// Every bar keeps four edges, including zero, so value morphs pass through zero
// at the baseline. Category labels are fixed snapshots below the plot.
type BarChart struct {
	Object         *gomath.Object
	frame, bars    *gomath.Object
	entries, names []*gomath.Object
	domain         Range
	barWidth       float64
}

// NewBarChart takes explicit data bounds and plot dimensions; the Y range must
// include zero and all values. It does not infer or rescale a range or clip bars.
func NewBarChart(values []float64, y Range, width, height float64, opt BarChartOptions) (*BarChart, error) {
	if len(values) < 1 || len(values) > MaxBars || len(opt.Names) != 0 && len(opt.Names) != len(values) {
		return nil, fmt.Errorf("bar chart requires 1..%d values and matching optional names", MaxBars)
	}
	if err := validateRange(y); err != nil {
		return nil, err
	}
	if y.Min > 0 || y.Max < 0 || !finite(opt.BarWidth) || opt.BarWidth <= 0 || opt.BarWidth > 1 || !finite(opt.FillOpacity) || opt.FillOpacity < 0 || opt.FillOpacity > 1 || !finite(opt.StrokeWidth) || opt.StrokeWidth < 0 || len(opt.Colors) < 1 || len(opt.Colors) > MaxBars {
		return nil, fmt.Errorf("invalid bar chart range, width, opacity, stroke or palette")
	}
	for _, color := range opt.Colors {
		style := gomath.DefaultStyle()
		style.Fill = color
		if err := style.Validate(); err != nil {
			return nil, err
		}
	}
	for _, value := range values {
		if !finite(value) || value < y.Min || value > y.Max {
			return nil, fmt.Errorf("bar value outside finite Y range")
		}
	}
	if len(opt.Names) != 0 {
		runes := 0
		for _, name := range opt.Names {
			if len(name) > 4*16384 || !utf8.ValidString(name) {
				return nil, fmt.Errorf("invalid or oversized bar name")
			}
			runes += utf8.RuneCountInString(name)
			if runes > 16384 {
				return nil, fmt.Errorf("bar names exceed 16384 total runes")
			}
		}
		if !finite(opt.Labels.Gap) || opt.Labels.Gap < 0 {
			return nil, fmt.Errorf("bar label gap must be finite and nonnegative")
		}
		if err := opt.Labels.Font.Validate(); err != nil {
			return nil, err
		}
	}
	axes, err := NewAxes(Range{Min: 0, Max: float64(len(values)), Step: 1}, y, width, height)
	if err != nil {
		return nil, err
	}
	// All three reference points lie inside plot bounds, so invisible mapping
	// geometry does not enlarge the plot's bounds.
	p0, p1, p2 := axes.Point(0, y.Min), axes.Point(float64(len(values)), y.Min), axes.Point(0, y.Max)
	frame := gomath.NewObject(path.Path{Contours: []path.Contour{{Start: p0, Curves: []path.Cubic{path.Line(p0, p1), path.Line(p1, p2)}}}}).SetFill(gomath.Transparent).SetStroke(gomath.Transparent, 0)
	frame.Name = "BarChart affine frame"
	c := &BarChart{frame: frame, bars: gomath.Group(), domain: y, barWidth: opt.BarWidth}
	c.entries = make([]*gomath.Object, len(values))
	c.Object = gomath.Group(axes.Object, frame, c.bars)
	labelCurves := 0
	for i, value := range values {
		outline, err := c.barPath(i, value)
		if err != nil {
			return nil, err
		}
		u := 0.0
		if len(values) > 1 {
			u = float64(i) * float64(len(opt.Colors)-1) / float64(len(values)-1)
		}
		index := min(int(u), len(opt.Colors)-1)
		color := opt.Colors[index]
		if index+1 < len(opt.Colors) {
			color = color.Lerp(opt.Colors[index+1], u-float64(index))
		}
		fill := color
		fill.A *= opt.FillOpacity
		bar := gomath.NewObject(outline).SetFill(fill).SetStroke(color, opt.StrokeWidth)
		bar.Name = fmt.Sprintf("Bar[%d]", i)
		c.entries[i] = bar
		c.bars.Children = append(c.bars.Children, bar)
		if len(opt.Names) != 0 {
			label, err := text.New(opt.Names[i], opt.Labels.Font, opt.Labels.Text)
			if err != nil {
				return nil, fmt.Errorf("bar name %d: %w", i, err)
			}
			pending := []*gomath.Object{label}
			for len(pending) != 0 {
				last := pending[len(pending)-1]
				pending = pending[:len(pending)-1]
				for _, contour := range last.Path.Contours {
					if len(contour.Curves) > MaxBarLabelCurves-labelCurves {
						return nil, fmt.Errorf("bar labels exceed %d total curves", MaxBarLabelCurves)
					}
					labelCurves += len(contour.Curves)
				}
				pending = append(pending, last.Children...)
			}
			bounds := label.Bounds()
			if !bounds.Valid {
				return nil, fmt.Errorf("bar name %d has no visible glyphs", i)
			}
			label.MoveTo(axes.Point(float64(i)+.5, y.Min).Add(geom.V(0, -opt.Labels.Gap-bounds.Size().Y/2-.05)))
			c.names = append(c.names, label)
			c.Object.Children = append(c.Object.Children, label)
		}
	}
	if err := c.Object.Validate(); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *BarChart) basis() (origin, x, y geom.Vec3, err error) {
	if c == nil || c.frame == nil || len(c.entries) == 0 || len(c.frame.Path.Contours) != 1 || len(c.frame.Path.Contours[0].Curves) != 2 {
		return origin, x, y, fmt.Errorf("invalid bar chart frame")
	}
	p := c.frame.Path.Contours[0]
	origin, x, y = p.Start, p.Curves[0].P3.Sub(p.Start), p.Curves[1].P3.Sub(p.Start)
	scale := math.Max(math.Max(math.Abs(x.X), math.Abs(x.Y)), math.Max(math.Abs(x.Z), math.Max(math.Abs(y.X), math.Max(math.Abs(y.Y), math.Abs(y.Z)))))
	if !origin.Finite() || !x.Finite() || !y.Finite() || scale == 0 {
		return origin, x, y, fmt.Errorf("bar chart frame is nonfinite or collapsed")
	}
	a, b := geom.Vec3{X: x.X / scale, Y: x.Y / scale, Z: x.Z / scale}, geom.Vec3{X: y.X / scale, Y: y.Y / scale, Z: y.Z / scale}
	if a.Cross(b).Norm() == 0 {
		return origin, x, y, fmt.Errorf("bar chart frame axes are collinear")
	}
	return origin, x, y, nil
}

// Point maps category coordinates (bar i is centered at i+.5) and actual data.
// Finite extrapolation is allowed; authored bars must remain in the Y range.
func (c *BarChart) Point(category, value float64) (geom.Vec3, error) {
	origin, x, y, err := c.basis()
	if err != nil {
		return geom.Vec3{}, err
	}
	if !finite(category) || !finite(value) {
		return geom.Vec3{}, fmt.Errorf("nonfinite bar chart coordinate")
	}
	p := origin.Add(x.Mul(category / float64(len(c.entries)))).Add(y.Mul((value - c.domain.Min) / (c.domain.Max - c.domain.Min)))
	if !p.Finite() {
		return geom.Vec3{}, fmt.Errorf("bar chart coordinate overflows")
	}
	return p, nil
}

func (c *BarChart) barPath(i int, value float64) (path.Path, error) {
	if !finite(value) || value < c.domain.Min || value > c.domain.Max {
		return path.Path{}, fmt.Errorf("bar %d outside finite Y range", i)
	}
	left := float64(i) + .5 - c.barWidth/2
	right := float64(i) + .5 + c.barWidth/2
	var corners [4]geom.Vec3
	for k, pair := range [][2]float64{{left, 0}, {right, 0}, {right, value}, {left, value}} {
		p, err := c.Point(pair[0], pair[1])
		if err != nil {
			return path.Path{}, err
		}
		corners[k] = p
	}
	if corners[0] == corners[1] || value != 0 && corners[0] == corners[3] {
		return path.Path{}, fmt.Errorf("bar %d loses coordinate precision", i)
	}
	curves := make([]path.Cubic, 4)
	for k := range curves {
		curves[k] = path.Line(corners[k], corners[(k+1)%4])
	}
	return path.Path{Contours: []path.Contour{{Start: corners[0], Curves: curves, Closed: true}}}, nil
}

// BarsObject returns the borrowed group used by Transform. TargetBars creates
// its independent checked target; neither method changes authoring data.
func (c *BarChart) BarsObject() *gomath.Object {
	if c == nil {
		return nil
	}
	return c.bars
}
func (c *BarChart) Bars() []*gomath.Object {
	if c == nil {
		return nil
	}
	return append([]*gomath.Object(nil), c.entries...)
}
func (c *BarChart) CategoryLabels() []*gomath.Object {
	if c == nil {
		return nil
	}
	return append([]*gomath.Object(nil), c.names...)
}

func (c *BarChart) targetPaths(values []float64) ([]path.Path, error) {
	if c == nil || c.Object == nil || c.bars == nil || len(values) != len(c.entries) || len(c.bars.Children) != len(c.entries) {
		return nil, fmt.Errorf("bar values must match the chart's fixed count")
	}
	foundFrame, foundBars := false, false
	for _, child := range c.Object.Children {
		foundFrame = foundFrame || child == c.frame
		foundBars = foundBars || child == c.bars
	}
	if !foundFrame || !foundBars {
		return nil, fmt.Errorf("bar chart child topology changed")
	}
	for i, bar := range c.entries {
		if c.bars.Children[i] != bar {
			return nil, fmt.Errorf("bar chart child topology changed")
		}
	}
	if err := c.Object.Validate(); err != nil {
		return nil, err
	}
	paths := make([]path.Path, len(values))
	for i, value := range values {
		p, err := c.barPath(i, value)
		if err != nil {
			return nil, err
		}
		preview := *c.entries[i]
		preview.Path = p
		if err = preview.ValidateVisual(); err != nil {
			return nil, err
		}
		paths[i] = p
	}
	return paths, nil
}

// TargetBars creates independent fixed-topology value targets in the current
// affine frame. All paths validate before copying any source objects.
func (c *BarChart) TargetBars(values []float64) (*gomath.Object, error) {
	paths, err := c.targetPaths(values)
	if err != nil {
		return nil, err
	}
	result := c.bars.Copy()
	for i, outline := range paths {
		result.Children[i].Path = outline
	}
	if err := result.Validate(); err != nil {
		return nil, err
	}
	return result, nil
}

// SetValues commits only after every target validates. Existing bar pointers,
// IDs, styles and child references remain unchanged. Sampling a frozen Scene
// still uses its own snapshots; this method is for serial scene authoring.
func (c *BarChart) SetValues(values []float64) error {
	paths, err := c.targetPaths(values)
	if err != nil {
		return err
	}
	for i, bar := range c.entries {
		bar.Path = paths[i]
	}
	return nil
}
