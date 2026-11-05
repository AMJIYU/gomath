package scenes

import (
	"gomath"
	"gomath/geom"
	"gomath/mathtex"
	"gomath/text"
)

func MathArrays() (*gomath.Scene, error) {
	scene := gomath.NewScene()
	heading, err := title("Math arrays / column alignment, real rules and shared operand budgets")
	if err != nil {
		return nil, err
	}
	heading.Scale(.81)
	if err = scene.Add(heading); err != nil {
		return nil, err
	}
	var objects []*gomath.Object
	for i, formula := range []string{
		`\begin{array}{|l|r|}\hline x&x^2\\\hline 1&1\\2&4\\\hline\end{array}`,
		`\begin{array}{lcr}\mathbf{A}&=&\frac{n}{g}\\\mathit{W}&=&\phantom{W}i\end{array}`,
	} {
		options := mathtex.DefaultOptions()
		options.Size = .6
		object, err := mathtex.New(formula, options)
		if err != nil {
			return nil, err
		}
		object.MoveTo(geom.V(-3.4+6.8*float64(i), .3))
		objects = append(objects, object)
		labelOptions := text.DefaultOptions()
		labelOptions.Size = .28
		caption, err := text.New([]string{"left / right columns + hline", "left / center / right + phantom"}[i], text.DefaultFont(), labelOptions)
		if err != nil {
			return nil, err
		}
		caption.MoveTo(geom.V(-3.4+6.8*float64(i), -2.1))
		if err = scene.Add(caption); err != nil {
			return nil, err
		}
	}
	if err = scene.Play(1.6, gomath.Write(objects[0]), gomath.Write(objects[1])); err != nil {
		return nil, err
	}
	if err = scene.Play(1.4, gomath.Transform(objects[0], objects[0].Copy().SetColor(gomath.Blue)), gomath.Transform(objects[1], objects[1].Copy().Scale(1.08).Rotate(.05))); err != nil {
		return nil, err
	}
	if err = scene.Wait(.5); err != nil {
		return nil, err
	}
	return scene, nil
}
