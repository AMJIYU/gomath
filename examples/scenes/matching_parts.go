package scenes

import (
	"gomath"
	"gomath/geom"
	"gomath/mathtex"
	"gomath/text"
)

func MatchingParts() (*gomath.Scene, error) {
	options := mathtex.DefaultOptions()
	options.Size = 1.0
	var formulas []*mathtex.Parts
	for _, source := range [][]string{
		{"x", `\quad+\quad`, "x", `\quad=\quad`, "y"},
		{"y", `\quad=\quad`, "2", "x"},
		{"2", "x", `\quad-\quad`, "y", `\quad=\quad`, "0"},
	} {
		parts, err := mathtex.NewParts(source, options)
		if err != nil {
			return nil, err
		}
		parts.Object().MoveTo(geom.V(0, .2))
		for _, x := range parts.Select("x").Objects() {
			x.SetColor(gomath.Yellow)
		}
		for _, y := range parts.Select("y").Objects() {
			y.SetColor(gomath.Pink)
		}
		formulas = append(formulas, parts)
	}
	heading, err := title("Formula matching / repeated keys, reorder and replace")
	if err != nil {
		return nil, err
	}
	style := text.DefaultOptions()
	style.Size = .3
	caption, err := text.New("Exact fragments / real target objects and stable live handles", text.DefaultFont(), style)
	if err != nil {
		return nil, err
	}
	caption.MoveTo(geom.V(0, -2.5))
	s := gomath.NewScene()
	if err = s.Add(heading, caption); err != nil {
		return nil, err
	}
	if err = s.Play(.6, gomath.Write(formulas[0].Object())); err != nil {
		return nil, err
	}
	if err = s.Wait(.4); err != nil {
		return nil, err
	}
	for i := 0; i < 2; i++ {
		animation, err := mathtex.TransformMatchingParts(formulas[i], formulas[i+1])
		if err != nil {
			return nil, err
		}
		if err = s.Play(1.5, animation.WithPathArc(.55)); err != nil {
			return nil, err
		}
		if err = s.Wait(.4); err != nil {
			return nil, err
		}
	}
	x, _ := formulas[2].Part(1)
	if err = s.Play(.5, gomath.Indicate(x)); err != nil {
		return nil, err
	}
	if err = s.Wait(.3); err != nil {
		return nil, err
	}
	return s, nil
}
