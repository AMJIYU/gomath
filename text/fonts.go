package text

import (
	"fmt"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gobolditalic"
	"golang.org/x/image/font/gofont/goitalic"
	"golang.org/x/image/font/gofont/goregular"
	"sync"
)

// FontStyle identifies actual embedded Go font outlines, not synthetic strokes
// or a shear. Bold and Italic can be combined as BoldItalic.
type FontStyle uint8

const (
	Regular FontStyle = iota
	Bold
	Italic
	BoldItalic
)

var embeddedFonts = [4]func() *Font{
	cachedFont(goregular.TTF), cachedFont(gobold.TTF),
	cachedFont(goitalic.TTF), cachedFont(gobolditalic.TTF),
}

func cachedFont(data []byte) func() *Font {
	return sync.OnceValue(func() *Font {
		f, err := Parse(data)
		if err != nil {
			panic("invalid embedded Go font: " + err.Error())
		}
		return f
	})
}

// DefaultFont returns the same immutable parsed regular font on every call.
func DefaultFont() *Font { return embeddedFonts[Regular]() }

// DefaultFontForStyle returns a lazily parsed immutable embedded Go font.
// All workers share parsed fonts; glyph buffers and objects remain call-local.
func DefaultFontForStyle(style FontStyle) (*Font, error) {
	if style > BoldItalic {
		return nil, fmt.Errorf("unsupported font style %d", style)
	}
	return embeddedFonts[style](), nil
}
