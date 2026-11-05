package scenes

import (
	"gomath"
	"gomath/geom"
	"gomath/svg"
	"gomath/text"
	"strings"
)

func SVGBBoxClips() (*gomath.Scene, error) {
	const source = `<svg viewBox="0 0 600 200" xmlns="http://www.w3.org/2000/svg">
<defs>
 <clipPath id="half" clipPathUnits="objectBoundingBox"><rect width=".5" height="1"/></clipPath>
 <clipPath id="diamond" clipPathUnits="objectBoundingBox"><path d="M .5 0 L 1 .5 L .5 1 L 0 .5 Z"/></clipPath>
 <linearGradient id="paint"><stop stop-color="#58a6ff"/><stop offset="1" stop-color="#f778ba"/></linearGradient>
</defs>
<rect x="30" y="40" width="130" height="100" rx="12" fill="url(#paint)" transform="rotate(-12 95 90)" clip-path="url(#half)"/>
<ellipse cx="300" cy="90" rx="75" ry="55" fill="url(#paint)" clip-path="url(#diamond)"/>
<rect x="440" y="35" width="130" height="110" rx="12" fill="url(#paint)" transform="rotate(18 505 90)" clip-path="url(#half)"/>
</svg>`
	object, err := svg.Read(strings.NewReader(source), svg.Options{Width: 12.5})
	if err != nil {
		return nil, err
	}
	object.MoveTo(geom.V(0, .3))
	heading, err := title("SVG bbox clips / local geometry before target transforms")
	if err != nil {
		return nil, err
	}
	heading.Scale(.9)
	s := gomath.NewScene()
	if err = s.Add(heading, object); err != nil {
		return nil, err
	}
	for i, value := range []string{"left half / rotated -12 deg", "diamond / ellipse bbox", "left half / rotated +18 deg"} {
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
	if err = s.Play(2.5, gomath.Transform(object, object.Copy().Scale(.9).Rotate(.08).Shift(geom.V(.15, .1)))); err != nil {
		return nil, err
	}
	if err = s.Wait(.5); err != nil {
		return nil, err
	}
	return s, nil
}
