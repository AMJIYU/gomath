package scenes

import (
	"fmt"
	"gomath"
	"gomath/geom"
	"gomath/graph"
	"gomath/linalg"
	"gomath/mathtex"
	"math"
)

// Mathematics uses computed least-squares coefficients rather than a decorative
// line, and draws signed Riemann cells and an area from the graph APIs.
func Mathematics() (*gomath.Scene, error) {
	s := gomath.NewScene()
	label, err := title("Mathematics / signed integrals and Householder QR fit")
	if err != nil {
		return nil, err
	}
	left, err := graph.NewNumberPlane(graph.Range{Min: -2.5, Max: 2.5, Step: .5}, graph.Range{Min: -1.5, Max: 1.5, Step: .5}, 5.4, 3, graph.DefaultNumberPlaneOptions())
	if err != nil {
		return nil, err
	}
	left.Shift(geom.V(-3.1, -.5))
	ri := graph.DefaultRiemannOptions()
	ri.Rectangles = 24
	rectangles, err := left.RiemannRectangles(math.Sin, -2.5, 2.5, ri)
	if err != nil {
		return nil, err
	}
	curve, err := left.PlotAdaptive(math.Sin, -2.5, 2.5, graph.DefaultAdaptiveOptions())
	if err != nil {
		return nil, err
	}
	curve.Object.SetStroke(gomath.Yellow, .04)
	formula, err := mathtex.New(`\left[\begin{matrix}\mathbf{a}&b\\c&\mathit{d}\end{matrix}\right]\quad\int f(x)dx`, mathtex.DefaultOptions())
	if err != nil {
		return nil, err
	}
	formula.Scale(.55).MoveTo(geom.V(-3.1, 1.8))
	right, err := graph.NewNumberPlane(graph.Range{Min: 0, Max: 4, Step: 1}, graph.Range{Min: 0, Max: 5, Step: 1}, 5.1, 3, graph.DefaultNumberPlaneOptions())
	if err != nil {
		return nil, err
	}
	right.Shift(geom.V(3.2, -.5))
	a, err := linalg.NewDense(5, 2, []float64{1, 0, 1, 1, 1, 2, 1, 3, 1, 4})
	if err != nil {
		return nil, err
	}
	y := []float64{1.1, 1.8, 2.7, 3.0, 4.2}
	coeff, _, err := linalg.LeastSquares(a, y)
	if err != nil {
		return nil, err
	}
	fit := func(x float64) float64 { return coeff[0] + coeff[1]*x }
	line, err := right.PlotAdaptive(fit, 0, 4, graph.DefaultAdaptiveOptions())
	if err != nil {
		return nil, err
	}
	line.Object.SetStroke(gomath.Pink, .04)
	areaOptions := graph.DefaultAreaOptions()
	areaOptions.Samples = 25
	areaOptions.Style.Opacity = .18
	area, err := right.AreaBetween(fit, func(float64) float64 { return 0 }, 0, 4, areaOptions)
	if err != nil {
		return nil, err
	}
	var dots []*gomath.Object
	for i, v := range y {
		dots = append(dots, gomath.Dot(right.Point(float64(i), v), .07).SetFill(gomath.Yellow))
	}
	points := gomath.Group(dots...)
	fitLabel, err := mathtex.New(fmt.Sprintf(`y=%.3f+%.3fx`, coeff[0], coeff[1]), mathtex.DefaultOptions())
	if err != nil {
		return nil, err
	}
	fitLabel.Scale(.7).MoveTo(geom.V(3.2, 1.8))
	lo := graph.DefaultNumberLineOptions()
	lo.Origin = geom.V(0, -2.8)
	labels := graph.DefaultNumberLabelOptions()
	lo.Labels = &labels
	numbers, err := graph.NewNumberLine(graph.Range{Min: -2, Max: 2, Step: .5}, 10, lo)
	if err != nil {
		return nil, err
	}
	if err = s.Add(label, formula, fitLabel); err != nil {
		return nil, err
	}
	if err = s.Play(.7, gomath.Create(left.Object), gomath.Create(right.Object), gomath.Create(numbers.Object)); err != nil {
		return nil, err
	}
	if err = s.Play(1, gomath.FadeIn(rectangles.Object), gomath.FadeIn(area.Object), gomath.FadeIn(points)); err != nil {
		return nil, err
	}
	if err = s.Play(1, gomath.Create(curve.Object), gomath.Create(line.Object)); err != nil {
		return nil, err
	}
	if err = s.Wait(.5); err != nil {
		return nil, err
	}
	return s, nil
}

func Paths() (*gomath.Scene, error) {
	s := gomath.NewScene()
	label, err := title("Arc length / constant-speed motion on unequal cubics")
	if err != nil {
		return nil, err
	}
	route := gomath.Polyline(geom.V(-5, -1.5), geom.V(-4, 1.4), geom.V(2, 1.4), geom.V(5, -1.5)).SetStroke(gomath.Blue, .05)
	dot := gomath.Dot(geom.V(-5, -1.5), .14).SetFill(gomath.Pink)
	if err = s.Add(label, route, dot); err != nil {
		return nil, err
	}
	if err = s.Play(4, gomath.MoveAlongPath(dot, route)); err != nil {
		return nil, err
	}
	if err = s.Wait(.4); err != nil {
		return nil, err
	}
	return s, nil
}
