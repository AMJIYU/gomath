package scenes

import (
	_ "embed"
	"gomath"
	"gomath/geom"
	"gomath/text"
	"sync"
)

//go:embed assets/GomathDemoMath.otf
var fallbackDemoFontBytes []byte

var fallbackDemoFont = sync.OnceValues(func() (*text.Font, error) {
	return text.Parse(fallbackDemoFontBytes)
})

func FontFallback() (*gomath.Scene, error) {
	fallback, err := fallbackDemoFont()
	if err != nil {
		return nil, err
	}
	heading, err := title("Font fallback / ordered coverage / shared baseline")
	if err != nil {
		return nil, err
	}
	options := text.DefaultOptions()
	options.Size = .85
	line, err := text.NewWithFallback("x in ℝ     𝒜 + 𝔄", []*text.Font{text.DefaultFont(), fallback}, options)
	if err != nil {
		return nil, err
	}
	line.MoveTo(geom.V(0, .6))
	for _, glyph := range line.Children {
		if glyph.Name == "ℝ" || glyph.Name == "𝒜" || glyph.Name == "𝔄" {
			glyph.SetFill(gomath.Yellow)
		}
	}
	options.Size, options.Color = .32, gomath.White
	caption, err := text.New("Go Regular: ASCII       /       explicit math face: yellow glyphs\nMissing in every configured font: an error unless AllowMissing is explicit", text.DefaultFont(), options)
	if err != nil {
		return nil, err
	}
	caption.MoveTo(geom.V(0, -1.5))
	s := gomath.NewScene()
	if err = s.Add(heading, caption); err != nil {
		return nil, err
	}
	if err = s.Play(2, gomath.Write(line)); err != nil {
		return nil, err
	}
	if err = s.Play(1, gomath.Indicate(line)); err != nil {
		return nil, err
	}
	if err = s.Wait(.5); err != nil {
		return nil, err
	}
	return s, nil
}
