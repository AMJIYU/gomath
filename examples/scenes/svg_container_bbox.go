package scenes

import (
	"gomath"
	"gomath/geom"
	"gomath/svg"
	"gomath/text"
	"strings"
)

func SVGContainerBBox() (*gomath.Scene, error) {
	const source = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 600 220">
<defs>
 <clipPath id="slice" clipPathUnits="objectBoundingBox"><rect x=".12" y="0" width=".7" height="1"/></clipPath>
 <g id="shape"><path d="M 0 90 C 0 -5 120 -5 120 90 L 120 110 L 0 110 Z" fill="#58a6ff"/><circle cx="75" cy="110" r="38" fill="#f778ba"/></g>
 <use id="chain" href="#shape" x="15" y="5"/>
</defs>
<g clip-path="url(#slice)" transform="translate(30 30) rotate(-9 70 70)"><use href="#shape"/><path d="M -10 75 L 130 75" stroke="#8ddb8c" stroke-width="6"/></g>
<use href="#chain" x="235" y="30" clip-path="url(#slice)"/>
<text x="520" y="95" text-anchor="middle" font-size="35" fill="#efb546" clip-path="url(#slice)">AV <tspan fill="#8ddb8c">Math</tspan></text>
<text x="520" y="145" text-anchor="middle" font-size="35" fill="#58a6ff">A<tspan clip-path="url(#slice)" fill="#f778ba">Vg</tspan></text>
</svg>`
	object, err := svg.Read(strings.NewReader(source), svg.Options{Width: 12.5})
	if err != nil {
		return nil, err
	}
	object.MoveTo(geom.V(0, .2))
	heading, err := title("SVG container clips / curves, use chains and full text cells")
	if err != nil {
		return nil, err
	}
	heading.Scale(.92)
	scene := gomath.NewScene()
	if err = scene.Add(heading, object); err != nil {
		return nil, err
	}
	options := text.DefaultOptions()
	options.Size = .28
	for i, value := range []string{"transformed group geometry", "expanded use-chain bbox", "text / owned tspan cells"} {
		label, err := text.New(value, text.DefaultFont(), options)
		if err != nil {
			return nil, err
		}
		label.MoveTo(geom.V(-4.3+4.3*float64(i), -2.3))
		if err = scene.Add(label); err != nil {
			return nil, err
		}
	}
	if err = scene.Play(2.5, gomath.Transform(object, object.Copy().Scale(.93).Rotate(.06))); err != nil {
		return nil, err
	}
	if err = scene.Wait(1); err != nil {
		return nil, err
	}
	return scene, nil
}
