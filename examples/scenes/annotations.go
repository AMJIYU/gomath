package scenes

import (
	"gomath"
	"gomath/geom"
	"gomath/mathtex"
	"gomath/text"
)

func Annotations() (*gomath.Scene, error) {
	s := gomath.NewScene()
	heading, err := title("Annotations / surrounding boxes, backgrounds, underline and cross")
	if err != nil {
		return nil, err
	}
	heading.Scale(.86)
	if err = s.Add(heading); err != nil {
		return nil, err
	}
	var groups []*gomath.Object
	for i, formula := range []string{`x^2+y^2=r^2`, `A^{-1}A=I`, `0=1`} {
		options := mathtex.DefaultOptions()
		options.Size = .65
		equation, err := mathtex.New(formula, options)
		if err != nil {
			return nil, err
		}
		equation.MoveTo(geom.V(-4.3+4.3*float64(i), .25))
		var group *gomath.Object
		switch i {
		case 0:
			box, err := gomath.NewSurroundingRectangle(equation, gomath.SurroundingRectangleOptions{PaddingX: .22, PaddingY: .2, CornerRadius: .12})
			if err != nil {
				return nil, err
			}
			group = gomath.Group(equation, box)
		case 1:
			backgroundOptions := gomath.DefaultBackgroundRectangleOptions()
			backgroundOptions.Rectangle = gomath.SurroundingRectangleOptions{PaddingX: .3, PaddingY: .25, CornerRadius: .1}
			backgroundOptions.Fill = gomath.Color{R: .1, G: .22, B: .4, A: .75}
			background, err := gomath.NewBackgroundRectangle(equation, backgroundOptions)
			if err != nil {
				return nil, err
			}
			underline, err := gomath.NewUnderline(equation, gomath.UnderlineOptions{Buffer: .16})
			if err != nil {
				return nil, err
			}
			underline.SetStroke(gomath.Blue, .045)
			group = gomath.Group(background, equation, underline)
		case 2:
			cross, err := gomath.NewCross(equation, gomath.CrossOptions{Scale: 1.3})
			if err != nil {
				return nil, err
			}
			group = gomath.Group(equation, cross)
		}
		groups = append(groups, group)
		labelOptions := text.DefaultOptions()
		labelOptions.Size = .29
		caption, err := text.New([]string{"rounded surrounding box", "background + underline", "cross out a false statement"}[i], text.DefaultFont(), labelOptions)
		if err != nil {
			return nil, err
		}
		caption.MoveTo(geom.V(-4.3+4.3*float64(i), -1.8))
		if err = s.Add(caption); err != nil {
			return nil, err
		}
	}
	if err = s.Play(1.5, gomath.LaggedGroup(.12, gomath.FadeIn(groups[0]), gomath.FadeIn(groups[1]), gomath.FadeIn(groups[2]))); err != nil {
		return nil, err
	}
	if err = s.Play(1.5, gomath.Transform(groups[0], groups[0].Copy().Scale(1.08).Rotate(.12)), gomath.Indicate(groups[2])); err != nil {
		return nil, err
	}
	if err = s.Wait(.5); err != nil {
		return nil, err
	}
	return s, nil
}
