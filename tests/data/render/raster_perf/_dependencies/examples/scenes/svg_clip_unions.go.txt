package scenes

import (
	"gomath"
	"gomath/geom"
	"gomath/svg"
	"gomath/text"
	"strings"
)

func SVGClipUnions() (*gomath.Scene, error) {
	const source = `<svg viewBox="0 0 600 220" xmlns="http://www.w3.org/2000/svg">
<defs>
 <clipPath id="overlap"><polygon points="25,55 135,55 135,165 25,165"/><polygon points="75,25 175,110 75,195"/></clipPath>
 <clipPath id="holes"><path clip-rule="evenodd" d="M 230 35 H 370 V 185 H 230 Z M 260 65 H 340 V 155 H 260 Z"/><polygon points="280,90 390,90 390,135 280,135"/></clipPath>
 <clipPath id="bbox" clipPathUnits="objectBoundingBox"><polygon points="0,.25 .75,.25 .75,.75 0,.75"/><polygon points=".5,0 1,.5 .5,1"/></clipPath>
 <linearGradient id="paint"><stop stop-color="#58a6ff"/><stop offset="1" stop-color="#f778ba"/></linearGradient>
</defs>
<rect width="195" height="220" fill="url(#paint)" clip-path="url(#overlap)"/>
<rect x="210" width="195" height="220" fill="url(#paint)" clip-path="url(#holes)"/>
<rect x="440" y="35" width="135" height="150" fill="url(#paint)" transform="rotate(12 507.5 110)" clip-path="url(#bbox)"/>
</svg>`
	object, err := svg.Read(strings.NewReader(source), svg.Options{Width: 12.5})
	if err != nil {
		return nil, err
	}
	object.MoveTo(geom.V(0, .3))
	heading, err := title("SVG polygon clips / per-child rules followed by geometric union")
	if err != nil {
		return nil, err
	}
	heading.Scale(.88)
	s := gomath.NewScene()
	if err = s.Add(heading, object); err != nil {
		return nil, err
	}
	for i, value := range []string{"overlapping children", "evenodd hole + nonzero bridge", "local bbox + affine target"} {
		options := text.DefaultOptions()
		options.Size = .28
		caption, err := text.New(value, text.DefaultFont(), options)
		if err != nil {
			return nil, err
		}
		caption.MoveTo(geom.V(-4.3+4.3*float64(i), -2))
		if err = s.Add(caption); err != nil {
			return nil, err
		}
	}
	if err = s.Wait(.5); err != nil {
		return nil, err
	}
	if err = s.Play(2.5, gomath.Transform(object, object.Copy().Scale(.92).Rotate(-.08).Shift(geom.V(.15, .1)))); err != nil {
		return nil, err
	}
	if err = s.Wait(.5); err != nil {
		return nil, err
	}
	return s, nil
}
