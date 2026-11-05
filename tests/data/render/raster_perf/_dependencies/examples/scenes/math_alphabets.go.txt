package scenes

import (
	"gomath"
	"gomath/geom"
	"gomath/mathtex"
)

func MathAlphabets() (*gomath.Scene, error) {
	s := gomath.NewScene()
	heading, err := title("Real math alphabets / double-struck / Script / Fraktur")
	if err != nil {
		return nil, err
	}
	if err = s.Add(heading); err != nil {
		return nil, err
	}
	for i, source := range []string{`\mathbb{R}\subset\mathbb{C}\quad\mathbb{N}\subset\mathbb{Z}`,
		`\mathcal{F}:\mathbb{R}^{n}\rightarrow\mathbb{R}`,
		`\mathcal{L}_{\mathfrak{g}}=\mathfrak{h}+\mathbb{C}^{2}`} {
		formula, err := mathtex.New(source, mathtex.DefaultOptions())
		if err != nil {
			return nil, err
		}
		formula.MoveTo(geom.V(0, 1.8-float64(i)*1.55))
		formula.SetColor([]gomath.Color{gomath.Blue, gomath.Yellow, gomath.Pink}[i])
		if err = s.Play(.7, gomath.Write(formula)); err != nil {
			return nil, err
		}
	}
	if err = s.Wait(1); err != nil {
		return nil, err
	}
	return s, nil
}
