package scenes

import (
	"gomath"
	"gomath/geom"
	"gomath/text"
	"math"
)

func ArcConnections() (*gomath.Scene, error) {
	s := gomath.NewScene()
	heading, err := title("Arc connections / signed sweep, radius and curved arrowheads")
	if err != nil {
		return nil, err
	}
	heading.Scale(.9)
	if err = s.Add(heading); err != nil {
		return nil, err
	}
	var marks []*gomath.Object
	for i, options := range []gomath.ArcBetweenPointsOptions{{Angle: math.Pi / 2}, {Angle: -math.Pi / 2}, {Angle: 4 * math.Pi / 3}} {
		arc, err := gomath.NewArcBetweenPoints(geom.V(-.95, 0), geom.V(.95, 0), options)
		if err != nil {
			return nil, err
		}
		arc.Object.SetStroke([]gomath.Color{gomath.Blue, gomath.Pink, gomath.Green}[i], .045)
		mark := gomath.Group(arc.Object, gomath.Dot(arc.Start, .045), gomath.Dot(arc.End, .045)).Shift(geom.V(-4.3+4.3*float64(i), 1.2))
		marks = append(marks, mark)
	}
	for i := range 3 {
		options := gomath.DefaultCurvedArrowOptions()
		if i == 1 {
			options.Double, options.Arc.Angle = true, -math.Pi/2
		}
		if i == 2 {
			options.Arc.Radius = -3
		}
		arrow, err := gomath.NewCurvedArrow(geom.V(-1.2, 0), geom.V(1.2, 0), options)
		if err != nil {
			return nil, err
		}
		arrow.SetColor([]gomath.Color{gomath.Blue, gomath.Pink, gomath.Green}[i]).Shift(geom.V(-4.3+4.3*float64(i), -1.3))
		marks = append(marks, arrow)
	}
	for i, value := range []string{"counterclockwise", "clockwise", "reflex sweep", "one head", "two heads", "explicit negative radius"} {
		options := text.DefaultOptions()
		options.Size = .28
		label, err := text.New(value, text.DefaultFont(), options)
		if err != nil {
			return nil, err
		}
		y := 2.15
		if i >= 3 {
			y = -2.3
		}
		label.MoveTo(geom.V(-4.3+4.3*float64(i%3), y))
		if err = s.Add(label); err != nil {
			return nil, err
		}
	}
	animations := make([]gomath.Animation, len(marks))
	for i, mark := range marks {
		animations[i] = gomath.Create(mark)
	}
	if err = s.Play(1.5, gomath.LaggedGroup(.1, animations...)); err != nil {
		return nil, err
	}
	if err = s.Play(1.5, gomath.Indicate(marks[4]), gomath.Shift(marks[0], geom.V(.25, 0))); err != nil {
		return nil, err
	}
	if err = s.Wait(.5); err != nil {
		return nil, err
	}
	return s, nil
}
