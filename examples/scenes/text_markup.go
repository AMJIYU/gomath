package scenes

import (
	"gomath"
	"gomath/geom"
	"gomath/text"
)

// TextMarkup uses the four embedded real font faces and author-time XML parsing.
func TextMarkup() (*gomath.Scene, error) {
	s := gomath.NewScene()
	heading, err := title("Native text markup / real font variants and scoped colors")
	if err != nil {
		return nil, err
	}
	heading.Scale(.9)
	if err = s.Add(heading); err != nil {
		return nil, err
	}
	options := text.DefaultMarkupOptions()
	options.Size = .57
	paragraph, err := text.NewMarkup(`Regular  <b>bold  <i>bold italic</i></b>  <i>italic</i>`+"\n"+`<span foreground="#58a6ff">Blue <b>bold</b></span>  <span foreground="#f778ba"><i>pink italic</i></span>  regular`+"\n"+`XML escapes: &lt;x &amp; y&gt;  /  α + β = γ`, options)
	if err != nil {
		return nil, err
	}
	paragraph.MoveTo(geom.V(0, .3))
	captionOptions := text.DefaultOptions()
	captionOptions.Size = .3
	caption, err := text.New("One editable glyph tree / same-font kerning survives color boundaries", text.DefaultFont(), captionOptions)
	if err != nil {
		return nil, err
	}
	caption.MoveTo(geom.V(0, -2.1))
	if err = s.Add(caption); err != nil {
		return nil, err
	}
	if err = s.Play(2, gomath.Write(paragraph)); err != nil {
		return nil, err
	}
	if err = s.Play(1, gomath.Shift(paragraph, geom.V(.2, .15))); err != nil {
		return nil, err
	}
	if err = s.Wait(.5); err != nil {
		return nil, err
	}
	return s, nil
}
