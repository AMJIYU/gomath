package scenes

import (
	"gomath"
	"gomath/geom"
	"gomath/text"
	"math"
)

func AngleMarks() (*gomath.Scene, error) {
	heading, err := title("Angle marks / quadrants, signed sweeps and elbows")
	if err != nil {
		return nil, err
	}
	first, second := gomath.Line(geom.V(-1.5, 0), geom.V(1.5, 0)), gomath.Line(geom.V(0, -1.5), geom.V(0, 1.5))
	options := gomath.DefaultAngleOptions()
	options.Dot, options.Radius = true, .8
	angle, err := gomath.NewAngle(first, second, options)
	if err != nil {
		return nil, err
	}
	angle.Object.SetStroke(gomath.Yellow, .045)
	left := gomath.Group(first, second, angle.Object).Shift(geom.V(-4.3, 0))
	r1, r2 := gomath.Line(geom.V(-1.4, -.6), geom.V(1.4, .6)), gomath.Line(geom.V(-.4, -1.4), geom.V(.4, 1.4))
	positive, err := gomath.NewAngle(r1, r2, gomath.AngleOptions{Radius: .7})
	if err != nil {
		return nil, err
	}
	negative, err := gomath.NewAngle(r1, r2, gomath.AngleOptions{Radius: 1, OtherAngle: true})
	if err != nil {
		return nil, err
	}
	positive.Object.SetStroke(gomath.Green, .05)
	negative.Object.SetStroke(gomath.Pink, .05)
	middle := gomath.Group(r1, r2, positive.Object, negative.Object)
	x, y := gomath.Line(geom.V(-1.5, 0), geom.V(1.5, 0)), gomath.Line(geom.V(0, -1.5), geom.V(0, 1.5))
	parts := []*gomath.Object{x, y}
	for i, q := range [][2]int{{1, 1}, {-1, 1}, {-1, -1}, {1, -1}} {
		mark, err := gomath.NewRightAngle(x, y, gomath.AngleOptions{Radius: .5 + float64(i)*.12, Quadrant: q})
		if err != nil {
			return nil, err
		}
		mark.Object.SetStroke([]gomath.Color{gomath.Blue, gomath.Green, gomath.Pink, gomath.Yellow}[i], .045)
		parts = append(parts, mark.Object)
	}
	right := gomath.Group(parts...).Shift(geom.V(4.3, 0))
	s := gomath.NewScene()
	if err = s.Add(heading); err != nil {
		return nil, err
	}
	for i, caption := range []string{"angle + dot", "counterclockwise / clockwise", "four quadrants"} {
		opts := text.DefaultOptions()
		opts.Size = .28
		label, err := text.New(caption, text.DefaultFont(), opts)
		if err != nil {
			return nil, err
		}
		label.MoveTo(geom.V(-4.3+4.3*float64(i), -2.2))
		if err = s.Add(label); err != nil {
			return nil, err
		}
	}
	if err = s.Play(1.5, gomath.LaggedGroup(.15, gomath.Create(left), gomath.Create(middle), gomath.Create(right))); err != nil {
		return nil, err
	}
	if err = s.Play(1.5, gomath.Rotate(middle, math.Pi/5), gomath.Indicate(right)); err != nil {
		return nil, err
	}
	if err = s.Wait(.5); err != nil {
		return nil, err
	}
	return s, nil
}
