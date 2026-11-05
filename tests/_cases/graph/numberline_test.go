package graph_test

import (
	"gomath"
	"gomath/geom"
	"gomath/graph"
	"gomath/render"
	"math"
	"testing"
)

func nearPoint(t *testing.T, got, want geom.Vec3) {
	t.Helper()
	if got.Sub(want).Norm() > 1e-10 {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestNumberLineMappingTicksAndUprightLabels(t *testing.T) {
	o := graph.DefaultNumberLineOptions()
	o.Origin, o.Direction = geom.V(1, 1), geom.V(0, 2)
	labels := graph.DefaultNumberLabelOptions()
	o.Labels = &labels
	n, err := graph.NewNumberLine(graph.Range{Min: -2, Max: 4, Step: 1}, 6, o)
	if err != nil {
		t.Fatal(err)
	}
	nearPoint(t, n.Point(-2), geom.V(1, -2))
	nearPoint(t, n.Point(4), geom.V(1, 4))
	for _, value := range []float64{-3, -1.5, 0, 2.75, 5} {
		p := n.Point(value).Add(geom.V(2, 0))
		if math.Abs(n.Coordinate(p)-value) > 1e-12 {
			t.Fatal("number line round trip", value)
		}
	}
	if len(n.Ticks()) != 7 {
		t.Fatal("wrong tick count")
	}
	for i, tick := range n.Ticks() {
		if tick.Value != float64(i)-2 || tick.Tick == nil || tick.Label == nil {
			t.Fatal("missing tick or label", tick)
		}
		nearPoint(t, tick.Tick.Center(), n.Point(tick.Value))
		b := tick.Label.Bounds()
		if math.Abs(b.Min.X-(1+o.TickSize/2+labels.Gap)) > 1e-10 {
			t.Fatal("label gap/direction incorrect", b)
		}
		if b.Size().Y <= 0 || b.Size().X <= 0 {
			t.Fatal("label has no glyph outlines")
		}
	}
	if err := n.Object.Validate(); err != nil {
		t.Fatal(err)
	}
	ticks := n.Ticks()
	ticks[0].Value = 500
	if n.Ticks()[0].Value == 500 {
		t.Fatal("tick records alias internal state")
	}
}

func TestNumberLineDecimalLabelsAndNoTicks(t *testing.T) {
	o := graph.DefaultNumberLineOptions()
	o.IncludeTicks = false
	labels := graph.DefaultNumberLabelOptions()
	o.Labels = &labels
	n, err := graph.NewNumberLine(graph.Range{Min: 0, Max: .5, Step: .1}, 5, o)
	if err != nil {
		t.Fatal(err)
	}
	for i, tick := range n.Ticks() {
		if tick.Tick != nil || tick.Label == nil {
			t.Fatal("disabled tick or enabled label missing")
		}
		if i == 3 && tick.Label.Name != "0.3" {
			t.Fatal("binary noise in decimal label", tick.Label.Name)
		}
		if math.Abs(tick.Label.Bounds().Max.Y+labels.Gap) > 1e-10 {
			t.Fatal("label gap without ticks incorrect")
		}
	}
	o.Labels = nil
	n, err = graph.NewNumberLine(graph.Range{Min: .1, Max: .2, Step: 1}, 2, o)
	if err != nil || len(n.Ticks()) != 0 || len(n.Object.Children) != 1 {
		t.Fatal("tick-free interval failed", err)
	}
	n, err = graph.NewNumberLine(graph.Range{Min: -.3, Max: .3, Step: .1}, 6, graph.DefaultNumberLineOptions())
	if err != nil || len(n.Ticks()) != 7 || n.Ticks()[0].Value != -.3 || n.Ticks()[6].Value != .3 {
		t.Fatal("decimal endpoint ticks lost to roundoff", err)
	}
	n, err = graph.NewNumberLine(graph.Range{Min: 1, Max: math.Nextafter(1, 2), Step: 1}, 4, graph.DefaultNumberLineOptions())
	if err != nil || len(n.Ticks()) != 1 || n.Ticks()[0].Value != 1 {
		t.Fatal("endpoint snapping moved a correctly represented tick", err)
	}
}

func TestLargeOffsetAxesAvoidMidpointOverflow(t *testing.T) {
	a, err := graph.NewAxes(graph.Range{Min: 1e308, Max: 1.1e308, Step: 1e307}, graph.Range{Min: -1, Max: 1, Step: 1}, 10, 4)
	if err != nil {
		t.Fatal(err)
	}
	nearPoint(t, a.Point(a.X.Min, 0), geom.V(-5, 0))
	nearPoint(t, a.Point(a.X.Max, 0), geom.V(5, 0))
	for _, x := range []float64{a.X.Min, a.X.Min + (a.X.Max-a.X.Min)/2, a.X.Max} {
		got, _ := a.Coordinates(a.Point(x, 0))
		if !a.Point(x, 0).Finite() || math.Abs((got-x)/x) > 1e-15 {
			t.Fatal("large offset coordinate mapping overflowed", got, x)
		}
	}
}

func TestNumberPlaneGridMappingOwnershipAndShift(t *testing.T) {
	x, y := graph.Range{Min: -2, Max: 2, Step: 1}, graph.Range{Min: -1, Max: 1, Step: .5}
	p, err := graph.NewNumberPlane(x, y, 8, 4, graph.DefaultNumberPlaneOptions())
	if err != nil {
		t.Fatal(err)
	}
	lines := p.GridLines()
	if len(lines) != 10 {
		t.Fatal("wrong grid line count", len(lines))
	}
	for i, line := range lines {
		c := line.Path.Contours[0]
		end := c.Curves[0].P3
		if i < 5 {
			nearPoint(t, c.Start, p.Point(-2+float64(i), -1))
			nearPoint(t, end, p.Point(-2+float64(i), 1))
		} else {
			nearPoint(t, c.Start, p.Point(-2, -1+float64(i-5)*.5))
			nearPoint(t, end, p.Point(2, -1+float64(i-5)*.5))
		}
	}
	old := lines[0]
	p.Shift(geom.V(.5, -.25))
	if p.GridLines()[0] != old {
		t.Fatal("shift replaced grid identity")
	}
	nearPoint(t, old.Path.Contours[0].Start, p.Point(-2, -1))
	xv, yv := p.Coordinates(p.Point(.3, -.7))
	if math.Abs(xv-.3) > 1e-12 || math.Abs(yv+.7) > 1e-12 {
		t.Fatal("plane mapping failed")
	}
	s := gomath.NewScene()
	if err := s.Add(p.Object, p.Object.Copy()); err != nil {
		t.Fatal("plane tree has shared children", err)
	}
	if err := s.Add(lines[0]); err == nil {
		t.Fatal("borrowed grid child accepted as root")
	}
	o := graph.DefaultNumberPlaneOptions()
	o.ShowAxes = false
	p, err = graph.NewNumberPlane(x, y, 8, 4, o)
	if err != nil || len(p.Object.Children) != 10 {
		t.Fatal("hidden axes remain in grid-only plane", err)
	}
}

func TestCoordinateObjectsRejectInvalidInputs(t *testing.T) {
	for _, mutate := range []func(*graph.NumberLineOptions){
		func(o *graph.NumberLineOptions) { o.Direction = geom.V(0, 0) },
		func(o *graph.NumberLineOptions) { o.Direction.Z = 1 },
		func(o *graph.NumberLineOptions) { o.Origin.X = math.Inf(1) },
		func(o *graph.NumberLineOptions) { o.Origin.X = 1e20 },
		func(o *graph.NumberLineOptions) { o.TickSize = 0 },
		func(o *graph.NumberLineOptions) { o.Color.A = math.NaN() },
		func(o *graph.NumberLineOptions) {
			labels := graph.DefaultNumberLabelOptions()
			labels.Font = nil
			o.Labels = &labels
		},
		func(o *graph.NumberLineOptions) {
			labels := graph.DefaultNumberLabelOptions()
			labels.Format = func(float64) string { return " \n " }
			o.Labels = &labels
		},
		func(o *graph.NumberLineOptions) {
			labels := graph.DefaultNumberLabelOptions()
			labels.Gap = -1
			o.Labels = &labels
		},
	} {
		o := graph.DefaultNumberLineOptions()
		mutate(&o)
		if _, err := graph.NewNumberLine(graph.Range{Min: -1, Max: 1, Step: 1}, 4, o); err == nil {
			t.Fatal("invalid number line options accepted")
		}
	}
	o := graph.DefaultNumberLineOptions()
	labels := graph.DefaultNumberLabelOptions()
	o.Labels = &labels
	if _, err := graph.NewNumberLine(graph.Range{Min: -500, Max: 500, Step: 1}, 4, o); err == nil {
		t.Fatal("unbounded labels accepted")
	}
	if _, err := graph.NewNumberLine(graph.Range{Min: .1, Max: .2, Step: 1}, 4, o); err == nil {
		t.Fatal("labels without tick locations accepted")
	}
	for _, r := range []graph.Range{{Min: 0, Max: 1, Step: 0}, {Min: -1e308, Max: 1e308, Step: 1e308}, {Min: 1, Max: 2, Step: 1e-30}, {Min: 0, Max: math.Inf(1), Step: 1}} {
		if _, err := graph.NewAxes(r, graph.Range{Min: -1, Max: 1, Step: 1}, 4, 4); err == nil {
			t.Fatal("invalid axes range accepted", r)
		}
	}
	po := graph.DefaultNumberPlaneOptions()
	po.GridWidth = 0
	if _, err := graph.NewNumberPlane(graph.Range{Min: -1, Max: 1, Step: 1}, graph.Range{Min: -1, Max: 1, Step: 1}, 4, 4, po); err == nil {
		t.Fatal("invalid grid style accepted")
	}
}

func TestCoordinateObjectsRenderActualGridTicksAndLabels(t *testing.T) {
	po := graph.DefaultNumberPlaneOptions()
	po.ShowAxes = false
	po.GridColor = gomath.Blue
	po.GridWidth = .06
	p, err := graph.NewNumberPlane(graph.Range{Min: -2, Max: 2, Step: 1}, graph.Range{Min: -1, Max: 1, Step: 1}, 8, 4, po)
	if err != nil {
		t.Fatal(err)
	}
	no := graph.DefaultNumberLineOptions()
	no.Origin = geom.V(0, -1)
	no.StrokeWidth = .06
	labels := graph.DefaultNumberLabelOptions()
	labels.Text.Size = .4
	no.Labels = &labels
	n, err := graph.NewNumberLine(graph.Range{Min: -2, Max: 2, Step: 1}, 8, no)
	if err != nil {
		t.Fatal(err)
	}
	s := gomath.NewScene()
	if err := s.Add(p.Object, n.Object); err != nil {
		t.Fatal(err)
	}
	f, err := s.Sample(0)
	if err != nil {
		t.Fatal(err)
	}
	c := render.DefaultConfig()
	c.Width, c.Height, c.FrameWidth, c.Background = 200, 150, 10, gomath.Black
	r, err := render.NewRasterizer(c)
	if err != nil {
		t.Fatal(err)
	}
	img, err := r.Render(f)
	if err != nil {
		t.Fatal(err)
	}
	if img.RGBAAt(60, 65).B < 120 || img.RGBAAt(80, 95).R < 100 {
		t.Fatal("grid/number line did not render")
	}
	// White glyph ink below the line, outside the grid's blue strokes.
	white := 0
	for y := 99; y < 112; y++ {
		for x := 53; x < 147; x++ {
			p := img.RGBAAt(x, y)
			if p.R > 160 && p.G > 160 && p.B > 160 {
				white++
			}
		}
	}
	if white < 5 {
		t.Fatal("pure-Go label outlines did not render", white)
	}
}
