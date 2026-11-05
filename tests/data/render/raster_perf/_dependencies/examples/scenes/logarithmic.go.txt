package scenes

import (
	"fmt"
	"gomath"
	"gomath/geom"
	"gomath/graph"
	"gomath/mathtex"
	"gomath/text"
	"math"
)

func Logarithmic() (*gomath.Scene, error) {
	s := gomath.NewScene()
	heading, err := title("Logarithmic coordinates / actual data mapping / power law")
	if err != nil {
		return nil, err
	}
	scale, err := graph.NewLogScale(10)
	if err != nil {
		return nil, err
	}
	axes, err := graph.NewScaledAxes(graph.Range{Min: -2, Max: 2, Step: 1}, graph.Range{Min: -4, Max: 4, Step: 2}, 8, 4, scale, scale)
	if err != nil {
		return nil, err
	}
	objects := []*gomath.Object{heading, axes.Object}
	labelOpt := text.DefaultOptions()
	labelOpt.Size = .25
	for exponent := -2; exponent <= 2; exponent++ {
		value := math.Pow(10, float64(exponent))
		bottom, err := axes.Point(value, 1e-4)
		if err != nil {
			return nil, err
		}
		top, err := axes.Point(value, 1e4)
		if err != nil {
			return nil, err
		}
		objects = append(objects, gomath.Line(bottom, top).SetStroke(gomath.RGB(.15, .2, .27), .015))
		label, err := text.New(fmt.Sprintf("10^%d", exponent), text.DefaultFont(), labelOpt)
		if err != nil {
			return nil, err
		}
		objects = append(objects, label.MoveTo(bottom.Add(geom.V(0, -.25))))
	}
	for exponent := -4; exponent <= 4; exponent += 2 {
		value := math.Pow(10, float64(exponent))
		left, err := axes.Point(.01, value)
		if err != nil {
			return nil, err
		}
		right, err := axes.Point(100, value)
		if err != nil {
			return nil, err
		}
		objects = append(objects, gomath.Line(left, right).SetStroke(gomath.RGB(.15, .2, .27), .015))
		label, err := text.New(fmt.Sprintf("10^%d", exponent), text.DefaultFont(), labelOpt)
		if err != nil {
			return nil, err
		}
		objects = append(objects, label.MoveTo(left.Add(geom.V(-.6, 0))))
	}
	curve, err := axes.Plot(func(x float64) float64 { return x * x }, 81)
	if err != nil {
		return nil, err
	}
	curve.SetStroke(gomath.Blue, .055)
	curve.ZIndex = 1
	formula, err := mathtex.New(`y=x^2`, mathtex.DefaultOptions())
	if err != nil {
		return nil, err
	}
	formula.Scale(.75).MoveTo(geom.V(5.2, 1.4))
	tracker, err := gomath.NewValueTracker(-2)
	if err != nil {
		return nil, err
	}
	start, err := axes.Point(.01, .0001)
	if err != nil {
		return nil, err
	}
	dot := gomath.Dot(start, .11).SetFill(gomath.Yellow)
	dot.ZIndex = 2
	objects = append(objects, curve, formula, tracker.Object, dot)
	if err = s.Add(objects...); err != nil {
		return nil, err
	}
	if err = s.AddRedraw(dot, func(frame *gomath.Frame, _ float64) (*gomath.Object, error) {
		exponent, err := frame.Value(tracker.ID)
		if err != nil {
			return nil, err
		}
		x := math.Pow(10, exponent)
		point, err := axes.Point(x, x*x)
		if err != nil {
			return nil, err
		}
		return gomath.Dot(point, .11).SetFill(gomath.Yellow), nil
	}); err != nil {
		return nil, err
	}
	if err = s.Play(3, gomath.ValueTo(tracker, 2)); err != nil {
		return nil, err
	}
	if err = s.Wait(.8); err != nil {
		return nil, err
	}
	return s, nil
}
