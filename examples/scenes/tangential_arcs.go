package scenes

import (
	"gomath"
	"gomath/geom"
	"gomath/text"
	"math"
)

// TangentialArcs keeps each fillet and its supporting lines in one transformable
// group. The finite lines show the two selected infinite-line rays.
func TangentialArcs() (*gomath.Scene, error) {
	s := gomath.NewScene()
	heading, err := title("Tangential arcs / circular fillets between selected line rays")
	if err != nil {
		return nil, err
	}
	heading.Scale(.9)
	if err = s.Add(heading); err != nil {
		return nil, err
	}
	var groups []*gomath.Object
	for i, corner := range [][2]int{{1, 1}, {-1, 1}, {-1, -1}} {
		first := gomath.Line(geom.V(-1.5, 0), geom.V(1.5, 0))
		second := gomath.Line(geom.V(0, -1.5), geom.V(0, 1.5))
		if i == 1 {
			second = gomath.Line(geom.V(-.5, -1.5), geom.V(.5, 1.5))
		}
		arc, err := gomath.NewTangentialArc(first, second, gomath.TangentialArcOptions{Radius: .7, Corner: corner})
		if err != nil {
			return nil, err
		}
		color := []gomath.Color{gomath.Blue, gomath.Pink, gomath.Green}[i]
		first.SetStroke(gomath.RGB(.28, .32, .4), .025)
		second.SetStroke(gomath.RGB(.28, .32, .4), .025)
		arc.Object.SetStroke(color, .065)
		group := gomath.Group(first, second, arc.Object,
			gomath.Dot(arc.Start, .06).SetFill(color), gomath.Dot(arc.End, .06).SetFill(color),
			gomath.Dot(arc.Center, .045).SetFill(gomath.Yellow)).Shift(geom.V(-4.3+4.3*float64(i), 0))
		groups = append(groups, group)
		options := text.DefaultOptions()
		options.Size = .3
		caption, err := text.New([]string{"corner (+,+)", "corner (-,+) / oblique", "corner (-,-)"}[i], text.DefaultFont(), options)
		if err != nil {
			return nil, err
		}
		caption.MoveTo(geom.V(-4.3+4.3*float64(i), -2.25))
		if err = s.Add(caption); err != nil {
			return nil, err
		}
	}
	if err = s.Play(1.3, gomath.LaggedGroup(.1, gomath.Create(groups[0]), gomath.Create(groups[1]), gomath.Create(groups[2]))); err != nil {
		return nil, err
	}
	if err = s.Play(1.5, gomath.Rotate(groups[1], math.Pi/6), gomath.Indicate(groups[2])); err != nil {
		return nil, err
	}
	if err = s.Wait(.7); err != nil {
		return nil, err
	}
	return s, nil
}
