package scenes

import (
	"gomath"
	"gomath/geom"
	"gomath/text"
	"math"
)

func TransformPaths() (*gomath.Scene, error) {
	scene := gomath.NewScene()
	heading, err := title("Circular Transform paths / each control follows its own circle")
	if err != nil {
		return nil, err
	}
	options := text.DefaultOptions()
	options.Size = .3
	var sources, targets []*gomath.Object
	for i := range 2 {
		x := -5.4 + 6.4*float64(i)
		source := gomath.Square(.85).SetFill(gomath.Blue).SetStroke(gomath.Transparent, 0).Shift(geom.V(x, 0))
		target := source.Copy().Scale(1.2).Rotate(.25).SetFill(gomath.Green).MoveTo(geom.V(x+3.7, .5))
		label, err := text.New([]string{"arc = +pi/2", "arc = -pi/2"}[i], text.DefaultFont(), options)
		if err != nil {
			return nil, err
		}
		label.MoveTo(geom.V(x+1.85, -1.7))
		guide := gomath.Line(geom.V(x, 0), geom.V(x+3.7, .5)).SetStroke(gomath.RGB(.25, .3, .38), .018)
		if err = scene.Add(guide, label, source); err != nil {
			return nil, err
		}
		sources = append(sources, source)
		targets = append(targets, target)
	}
	caption, err := text.New("Shape morph and circular trajectories share the same captured timeline", text.DefaultFont(), options)
	if err != nil {
		return nil, err
	}
	caption.MoveTo(geom.V(0, -2.6))
	if err = scene.Add(heading, caption); err != nil {
		return nil, err
	}
	if err = scene.Play(2.5,
		gomath.Transform(sources[0], targets[0]).WithPathArc(math.Pi/2),
		gomath.Transform(sources[1], targets[1]).WithPathArc(-math.Pi/2)); err != nil {
		return nil, err
	}
	if err = scene.Wait(1); err != nil {
		return nil, err
	}
	return scene, nil
}
