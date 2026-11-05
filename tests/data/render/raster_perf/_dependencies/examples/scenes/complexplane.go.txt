package scenes

import (
	"gomath"
	"gomath/geom"
	"gomath/graph"
	"gomath/mathtex"
	"math"
	"math/cmplx"
)

func ComplexPlane() (*gomath.Scene, error) {
	plane, err := graph.NewComplexPlane(graph.Range{Min: -3, Max: 3, Step: 1}, graph.Range{Min: -3, Max: 3, Step: 1}, 5.2, 5.2, graph.DefaultNumberPlaneOptions())
	if err != nil {
		return nil, err
	}
	plane.Object.Shift(geom.V(-2.4, -.2))
	trajectory := func(t float64) complex128 { return complex(1+t/8, 0) * cmplx.Exp(complex(0, t)) }
	curve, err := plane.Plot(trajectory, 0, 2*math.Pi, 257)
	if err != nil {
		return nil, err
	}
	curve.SetStroke(gomath.Blue, .04)
	formula, err := mathtex.New(`z(t)=\left(1+\frac{t}{8}\right)e^{it}`, mathtex.DefaultOptions())
	if err != nil {
		return nil, err
	}
	formula.Scale(.62).MoveTo(geom.V(3.2, .8))
	bounds, err := mathtex.New(`0\le t\le 2\pi`, mathtex.DefaultOptions())
	if err != nil {
		return nil, err
	}
	bounds.Scale(.55).MoveTo(geom.V(3.2, -.5))
	heading, err := title("Complex plane / affine coordinates / native complex trajectory")
	if err != nil {
		return nil, err
	}
	heading.Scale(.9)
	value, err := gomath.NewValueTracker(0)
	if err != nil {
		return nil, err
	}
	initial, err := plane.Point(trajectory(0))
	if err != nil {
		return nil, err
	}
	dot := gomath.Dot(initial, .1).SetFill(gomath.Yellow)
	s := gomath.NewScene()
	if err = s.Add(heading, plane.Object, curve, formula, bounds, value.Object, dot); err != nil {
		return nil, err
	}
	if err = s.AddRedraw(dot, func(frame *gomath.Frame, _ float64) (*gomath.Object, error) {
		t, err := frame.Value(value.ID)
		if err != nil {
			return nil, err
		}
		p, err := plane.Point(trajectory(t))
		if err != nil {
			return nil, err
		}
		return gomath.Dot(p, .1).SetFill(gomath.Yellow), nil
	}); err != nil {
		return nil, err
	}
	if err = s.Wait(.5); err != nil {
		return nil, err
	}
	if err = s.Play(3, gomath.ValueTo(value, 2*math.Pi).WithRate(gomath.Linear)); err != nil {
		return nil, err
	}
	if err = s.Wait(.5); err != nil {
		return nil, err
	}
	return s, nil
}
