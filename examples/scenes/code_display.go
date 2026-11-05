package scenes

import (
	"gomath"
	"gomath/code"
	"gomath/geom"
	"gomath/text"
)

// CodeDisplay renders actual Go Mono outlines. The listing is display data;
// constructing or animating it never executes the displayed Go source.
func CodeDisplay() (*gomath.Scene, error) {
	scene := gomath.NewScene()
	heading, err := title("Go source / lexical colors and editable vector glyphs")
	if err != nil {
		return nil, err
	}
	options := code.DefaultOptions()
	options.Size = .38
	options.LineNumbers, options.Background = true, true
	source, err := code.New("package mathdemo\n\n// A polynomial, shown as vector paths.\nfunc power(x float64) float64 {\n\treturn x * x\n}\n", options)
	if err != nil {
		return nil, err
	}
	target, err := code.New("package mathdemo\n\n// Change the formula through a morph.\nfunc power(x float64) float64 {\n\treturn x * x * x\n}\n", options)
	if err != nil {
		return nil, err
	}
	source.Object.MoveTo(geom.V(0, .1))
	target.Object.MoveTo(geom.V(0, .1))
	textOptions := text.DefaultOptions()
	textOptions.Size = .27
	caption, err := text.New("Pure Go scanner + real monospaced outlines / source is never executed", text.DefaultFont(), textOptions)
	if err != nil {
		return nil, err
	}
	caption.MoveTo(geom.V(0, -2.65))
	if err = scene.Add(heading, caption); err != nil {
		return nil, err
	}
	if err = scene.Play(1, gomath.Write(source.Object)); err != nil {
		return nil, err
	}
	if err = scene.Play(1.5, gomath.Transform(source.Object, target.Object)); err != nil {
		return nil, err
	}
	if err = scene.Wait(1); err != nil {
		return nil, err
	}
	return scene, nil
}
