package scenes

import (
	"gomath"
	"gomath/geom"
	"gomath/mathtex"
)

func OperatorLayout() (*gomath.Scene, error) {
	s := gomath.NewScene()
	heading, err := title("Named operators / limits / stacked mathematical notation")
	if err != nil {
		return nil, err
	}
	if err = s.Add(heading); err != nil {
		return nil, err
	}
	formulas := []string{
		`\operatorname{rank}(A)=\operatorname{rank}(A^T)`,
		`\displaystyle\operatorname*{arg max}_{x\in R}\overset{\hat{n}}{f}(x)`,
		`a\stackrel{\operatorname{def}}{=}b+\underset{x\rightarrow0}{\lim}\frac{\sin x}{x}`,
	}
	var animations []gomath.Animation
	for i, source := range formulas {
		object, err := mathtex.New(source, mathtex.DefaultOptions())
		if err != nil {
			return nil, err
		}
		object.MoveTo(geom.V(0, 1.7-float64(i)*1.7))
		if i == 1 {
			object.SetColor(gomath.Blue)
		}
		animations = append(animations, gomath.Write(object))
	}
	if err = s.Play(3, gomath.LaggedGroup(.3, animations...)); err != nil {
		return nil, err
	}
	if err = s.Wait(1); err != nil {
		return nil, err
	}
	return s, nil
}
