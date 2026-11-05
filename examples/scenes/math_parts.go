package scenes

import (
	"gomath"
	"gomath/geom"
	"gomath/mathtex"
	"gomath/text"
)

func MathParts() (*gomath.Scene, error) {
	options := mathtex.DefaultOptions()
	options.Size = .95
	parts, err := mathtex.NewParts([]string{"x", `\quad+\quad`, "x", `\quad=\quad`, "2x"}, options)
	if err != nil {
		return nil, err
	}
	parts.Object().MoveTo(geom.V(0, .8))
	selection := parts.Select("x")
	detached, err := selection.Copy()
	if err != nil {
		return nil, err
	}
	detached.SetColor(gomath.Green).MoveTo(geom.V(0, -1.15))
	heading, err := title("Explicit math fragments / exact keys and stable live selections")
	if err != nil {
		return nil, err
	}
	textOptions := text.DefaultOptions()
	textOptions.Size = .3
	caption, err := text.New("Two exact x keys / animated handles above, independent copies below", text.DefaultFont(), textOptions)
	if err != nil {
		return nil, err
	}
	caption.MoveTo(geom.V(0, -2.5))
	scene := gomath.NewScene()
	if err = scene.Add(heading, caption); err != nil {
		return nil, err
	}
	if err = scene.Play(.7, gomath.Write(parts.Object())); err != nil {
		return nil, err
	}
	var animations []gomath.Animation
	for _, part := range selection.Objects() {
		animations = append(animations, gomath.Transform(part, part.Copy().Shift(geom.V(0, .35)).SetColor(gomath.Yellow)))
	}
	if err = scene.Play(1.3, animations...); err != nil {
		return nil, err
	}
	if err = scene.Play(.7, gomath.FadeIn(detached)); err != nil {
		return nil, err
	}
	if err = scene.Wait(.8); err != nil {
		return nil, err
	}
	return scene, nil
}
