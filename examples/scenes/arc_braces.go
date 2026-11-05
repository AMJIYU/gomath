package scenes

import (
	"gomath"
	"gomath/geom"
	"gomath/text"
)

func ArcBraces() (*gomath.Scene, error) {
	scene := gomath.NewScene()
	heading, err := title("Arc braces / exponential geometry / inside and outside")
	if err != nil {
		return nil, err
	}
	options := text.DefaultOptions()
	options.Size = .28
	var panels []*gomath.Object
	for i, radius := range []float64{.2, 1, 1.5} {
		center := geom.V(-4.2+4.2*float64(i), -.1)
		arc := gomath.Arc(radius, -1, 2, false).Shift(center).SetStroke(gomath.Blue, .035)
		outside, err := gomath.NewArcBrace(center, radius, -1, 2, gomath.DefaultArcBraceOptions())
		if err != nil {
			return nil, err
		}
		insideOptions := gomath.DefaultArcBraceOptions()
		insideOptions.Direction = geom.V(-1, 0)
		inside, err := gomath.NewArcBrace(center, radius, -1, 2, insideOptions)
		if err != nil {
			return nil, err
		}
		outside.Object.SetColor(gomath.Green)
		inside.Object.SetColor(gomath.Pink)
		label, err := text.New([]string{"r = .2 / radial floor .3", "r = 1 / scale boundary", "r = 1.5 / scaled cap and gap"}[i], text.DefaultFont(), options)
		if err != nil {
			return nil, err
		}
		label.MoveTo(geom.V(center.X, -2.1))
		panels = append(panels, gomath.Group(arc, outside.Object, inside.Object, label))
	}
	if err = scene.Add(heading); err != nil {
		return nil, err
	}
	if err = scene.Play(1.2, gomath.FadeIn(panels[0]), gomath.FadeIn(panels[1]), gomath.FadeIn(panels[2])); err != nil {
		return nil, err
	}
	if err = scene.Play(1.8, gomath.Rotate(panels[0], .1), gomath.Rotate(panels[1], -.1), gomath.Rotate(panels[2], .1)); err != nil {
		return nil, err
	}
	if err = scene.Wait(.5); err != nil {
		return nil, err
	}
	return scene, nil
}
