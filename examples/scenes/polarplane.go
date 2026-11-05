package scenes

import (
	"gomath"
	"gomath/geom"
	"gomath/graph"
	"gomath/mathtex"
	"gomath/text"
	"math"
)

func PolarPlane() (*gomath.Scene, error) {
	options := graph.DefaultPolarPlaneOptions()
	options.Rays = 12
	left, err := graph.NewPolarPlane(graph.Range{Min: 0, Max: 3, Step: 1}, 2.1, options)
	if err != nil {
		return nil, err
	}
	options.Clockwise, options.Offset = true, .25
	right, err := graph.NewPolarPlane(graph.Range{Min: 0, Max: 3, Step: 1}, 2.1, options)
	if err != nil {
		return nil, err
	}
	left.Object.Shift(geom.V(-3, -.1))
	right.Object.Shift(geom.V(3, -.1))
	radial := func(angle float64) float64 { return 1.8 + .65*math.Cos(3*angle) }
	lcurve, err := left.Plot(radial, 0, 2*math.Pi, 385)
	if err != nil {
		return nil, err
	}
	rcurve, err := right.Plot(radial, 0, 2*math.Pi, 385)
	if err != nil {
		return nil, err
	}
	lcurve.SetStroke(gomath.Blue, .04)
	rcurve.SetStroke(gomath.Pink, .04)
	heading, err := title("Polar coordinates / circular grids / explicit angular direction")
	if err != nil {
		return nil, err
	}
	heading.Scale(.9)
	txt := text.DefaultOptions()
	txt.Size = .29
	lcaption, err := text.New("CCW / phase zero", text.DefaultFont(), txt)
	if err != nil {
		return nil, err
	}
	rcaption, err := text.New("CW / phase 0.25 radians", text.DefaultFont(), txt)
	if err != nil {
		return nil, err
	}
	lcaption.MoveTo(geom.V(-3, 2.5))
	rcaption.MoveTo(geom.V(3, 2.5))
	formula, err := mathtex.New(`r(\theta)=1.8+0.65\cos(3\theta)`, mathtex.DefaultOptions())
	if err != nil {
		return nil, err
	}
	formula.Scale(.5).MoveTo(geom.V(0, -2.9))
	tracker, err := gomath.NewValueTracker(0)
	if err != nil {
		return nil, err
	}
	buildDots := func(angle float64) (*gomath.Object, error) {
		lp, err := left.Point(radial(angle), angle)
		if err != nil {
			return nil, err
		}
		rp, err := right.Point(radial(angle), angle)
		if err != nil {
			return nil, err
		}
		return gomath.Group(gomath.Dot(lp, .09).SetFill(gomath.Yellow), gomath.Dot(rp, .09).SetFill(gomath.Yellow)), nil
	}
	dots, err := buildDots(0)
	if err != nil {
		return nil, err
	}
	s := gomath.NewScene()
	if err = s.Add(heading, left.Object, right.Object, lcurve, rcurve, lcaption, rcaption, formula, tracker.Object, dots); err != nil {
		return nil, err
	}
	if err = s.AddRedraw(dots, func(frame *gomath.Frame, _ float64) (*gomath.Object, error) {
		angle, err := frame.Value(tracker.ID)
		if err != nil {
			return nil, err
		}
		return buildDots(angle)
	}); err != nil {
		return nil, err
	}
	if err = s.Wait(.5); err != nil {
		return nil, err
	}
	if err = s.Play(3, gomath.ValueTo(tracker, 2*math.Pi).WithRate(gomath.Linear)); err != nil {
		return nil, err
	}
	if err = s.Wait(.5); err != nil {
		return nil, err
	}
	return s, nil
}
