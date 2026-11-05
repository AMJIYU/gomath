package scenes

import (
	"gomath"
	"gomath/geom"
	"gomath/mathtex"
	"gomath/text"
)

func MathBraces() (*gomath.Scene, error) {
	heading, err := title("Math braces / vector operands, centered labels and nesting")
	if err != nil {
		return nil, err
	}
	options := mathtex.DefaultOptions()
	options.Size = .8
	var objects []*gomath.Object
	for i, source := range []string{
		`\overbrace{x_1+x_2+x_3}^{3\text{ terms}}`,
		`\underbrace{\frac{x_i^2}{\sqrt[3]{g}}}_{k^2}`,
		`\overbrace{\underbrace{x+y}_{m}}^{n_i}`,
	} {
		o, err := mathtex.New(source, options)
		if err != nil {
			return nil, err
		}
		o.MoveTo(geom.V(-4+4*float64(i), .15))
		o.SetColor([]gomath.Color{gomath.Blue, gomath.Yellow, gomath.Pink}[i])
		objects = append(objects, o)
	}
	style := text.DefaultOptions()
	style.Size = .3
	caption, err := text.New("Pure Go finite em layout / advance and phantom dimensions retained", text.DefaultFont(), style)
	if err != nil {
		return nil, err
	}
	caption.MoveTo(geom.V(0, -2.5))
	s := gomath.NewScene()
	if err = s.Add(heading, caption); err != nil {
		return nil, err
	}
	if err = s.Play(1.5, gomath.Write(objects[0]), gomath.Write(objects[1]), gomath.Write(objects[2])); err != nil {
		return nil, err
	}
	if err = s.Play(1.5, gomath.Transform(objects[0], objects[0].Copy().Scale(1.06).Rotate(.06)), gomath.Indicate(objects[1]), gomath.Indicate(objects[2])); err != nil {
		return nil, err
	}
	if err = s.Wait(.6); err != nil {
		return nil, err
	}
	return s, nil
}
