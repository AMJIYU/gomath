package mathtex

import (
	"fmt"
	"gomath"
	"gomath/geom"
	"gomath/path"
	"gomath/text"
	"math"
	"strings"
	"unicode"
)

type mathAlphabet uint8

const (
	ordinaryAlphabet mathAlphabet = iota
	doubleStruckAlphabet
	scriptAlphabet
	frakturAlphabet
)

const alphabetInputs = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

func (p *parser) alphabetGroup(command string, size float64) (box, error) {
	oldAlphabet, oldStyle := p.alphabet, p.style
	switch command {
	case "mathbb":
		p.alphabet = doubleStruckAlphabet
	case "mathcal":
		p.alphabet = scriptAlphabet
	case "mathfrak":
		p.alphabet = frakturAlphabet
	}
	// A math-alphabet command selects its own face. Nested existing face commands
	// select theirs instead; neither synthesizes bolding or slant on these glyphs.
	p.style = text.Regular
	defer func() { p.alphabet, p.style = oldAlphabet, oldStyle }()
	return p.requiredGroup(size)
}

func (p *parser) mathGlyph(value string, size float64) (box, error) {
	runes := []rune(value)
	if len(runes) == 1 {
		r := runes[0]
		if _, exists := mathAlphabetOutlines[r]; exists {
			return p.alphabetGlyph(r, size)
		}
		if p.alphabet != ordinaryAlphabet {
			index := strings.IndexRune(alphabetInputs, r)
			mapped := mathAlphabetRunes[p.alphabet]
			if index >= 0 && index < len(mapped) {
				return p.alphabetGlyph(mapped[index], size)
			}
			if unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsMark(r) {
				return box{}, p.error(fmt.Sprintf("unsupported mathematical alphabet character %U", r))
			}
		}
	}
	return p.glyph(value, size)
}

// Encoded mathematical alphabet runes select their exact source glyph, without
// a synthetic bold/italic variant. Custom font selections remain strict and
// must contain that Unicode glyph themselves, as for supplemental set symbols.
func (p *parser) alphabetGlyph(r rune, size float64) (box, error) {
	f, err := p.font()
	if err != nil {
		return box{}, err
	}
	embedded, err := text.DefaultFontForStyle(p.style)
	if err != nil {
		return box{}, err
	}
	if f != embedded {
		return p.glyph(string(r), size)
	}
	outline, ok := mathAlphabetOutlines[r]
	if !ok {
		return box{}, p.error(fmt.Sprintf("unsupported mathematical alphabet glyph %U", r))
	}
	scale := size / 1000
	if scale <= 0 || !finite(scale) {
		return box{}, p.error("math-alphabet dimensions underflow or overflow")
	}
	var builder path.Builder
	for _, op := range outline.operations {
		point := func(i int) geom.Vec3 { return geom.V(op.xy[i]*scale, op.xy[i+1]*scale) }
		switch op.kind {
		case 'M':
			builder.MoveTo(point(0))
		case 'L':
			builder.LineTo(point(0))
		case 'C':
			builder.CubicTo(point(0), point(2), point(4))
		case 'Z':
			builder.Close()
		default:
			return box{}, p.error("invalid bundled math-alphabet operation")
		}
	}
	o := gomath.NewObject(builder.Path()).SetFill(p.options.Color).SetStroke(gomath.Transparent, 0)
	o.Name = string(r)
	if err := p.accentCurves(o); err != nil {
		return box{}, err
	}
	ink := o.Bounds()
	if !ink.Valid || ink.Size().X <= 0 || ink.Size().Y <= 0 {
		return box{}, p.error("math-alphabet dimensions underflow")
	}
	left := math.Max(0, -ink.Min.X)
	if left > 0 {
		o.Shift(geom.V(left, 0))
	}
	return box{object: gomath.Group(o), width: math.Max(outline.advance*scale, ink.Max.X) + left,
		height: math.Max(0, ink.Max.Y), depth: math.Max(0, -ink.Min.Y), content: true}, nil
}
