package scenes

import (
	"gomath"
	"gomath/geom"
	"gomath/mathtex"
	"gomath/text"
)

// RadicalBoxes exercises nested indices, a real vector border, and invisible
// layout operands through the same Write/Transform pipeline as ordinary math.
func RadicalBoxes() (*gomath.Scene, error) {
	s := gomath.NewScene()
	heading, err := title("Math layout / indexed roots, boxes and invisible spacing")
	if err != nil {
		return nil, err
	}
	heading.Scale(.9)
	if err = s.Add(heading); err != nil {
		return nil, err
	}
	formulas := []string{`\boxed{\sqrt[n_i]{\dfrac{x}{y}}}`, `A\phantom{WW}B`, `\dfrac{1}{\vphantom{g}x}+\sqrt[3]{g}`}
	captions := []string{"nested degree + boxed fraction", "phantom: spacing without ink", "vphantom: height without width"}
	colors := []gomath.Color{gomath.Blue, gomath.Yellow, gomath.Green}
	var objects []*gomath.Object
	for i, formula := range formulas {
		options := mathtex.DefaultOptions()
		options.Size = .7
		object, err := mathtex.New(formula, options)
		if err != nil {
			return nil, err
		}
		object.SetColor(colors[i]).MoveTo(geom.V(-4.3+4.3*float64(i), .3))
		objects = append(objects, object)
		labelOptions := text.DefaultOptions()
		labelOptions.Size = .28
		caption, err := text.New(captions[i], text.DefaultFont(), labelOptions)
		if err != nil {
			return nil, err
		}
		caption.MoveTo(geom.V(-4.3+4.3*float64(i), -1.7))
		if err = s.Add(caption); err != nil {
			return nil, err
		}
	}
	if err = s.Play(1.6, gomath.Write(objects[0]), gomath.Write(objects[1]), gomath.Write(objects[2])); err != nil {
		return nil, err
	}
	if err = s.Play(1.4, gomath.Transform(objects[0], objects[0].Copy().Scale(1.08).Rotate(.08)), gomath.Indicate(objects[1]), gomath.Indicate(objects[2])); err != nil {
		return nil, err
	}
	if err = s.Wait(.5); err != nil {
		return nil, err
	}
	return s, nil
}
