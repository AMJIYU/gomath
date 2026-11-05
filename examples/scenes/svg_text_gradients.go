package scenes

import (
	"gomath"
	"gomath/geom"
	"gomath/svg"
	"gomath/text"
	"strings"
)

func SVGTextGradients() (*gomath.Scene, error) {
	const source = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 600 220"><defs>
 <linearGradient id="g"><stop stop-color="#f778ba"/><stop offset="1" stop-color="#58a6ff"/></linearGradient>
 <radialGradient id="r"><stop stop-color="#efb546"/><stop offset="1" stop-color="#8ddb8c"/></radialGradient>
 <text id="word" x="0" y="0" font-size="45">AV<tspan fill="#efb546"> Math</tspan> Go</text>
 </defs>
 <use href="#word" x="30" y="70" fill="url(#g)"/>
 <use href="#word" x="340" y="70" font-size="32" fill="url(#r)" transform="rotate(5 430 55)"/>
 <text x="300" y="160" font-size="50" text-anchor="middle" fill="url(#g)">Pure<tspan fill="none"> Space </tspan>Go</text>
 </svg>`
	object, err := svg.Read(strings.NewReader(source), svg.Options{Width: 12})
	if err != nil {
		return nil, err
	}
	object.MoveTo(geom.V(0, .15))
	heading, err := title("SVG text gradients / one full glyph-cell bbox per text")
	if err != nil {
		return nil, err
	}
	opt := text.DefaultOptions()
	opt.Size = .3
	caption, err := text.New("Spans share the enclosing text paint frame / invisible spaces keep their cells", text.DefaultFont(), opt)
	if err != nil {
		return nil, err
	}
	caption.MoveTo(geom.V(0, -2.5))
	scene := gomath.NewScene()
	if err = scene.Add(heading, caption, object); err != nil {
		return nil, err
	}
	if err = scene.Play(2.5, gomath.Transform(object, object.Copy().Scale(.92).Rotate(-.06))); err != nil {
		return nil, err
	}
	if err = scene.Wait(1); err != nil {
		return nil, err
	}
	return scene, nil
}
