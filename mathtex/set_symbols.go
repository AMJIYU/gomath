package mathtex

import (
	"gomath"
	"gomath/geom"
	"gomath/path"
	"gomath/text"
	"math"
)

var setSymbolCommands = map[string]rune{
	"in": '∈', "notin": '∉', "ni": '∋', "owns": '∋',
	"subset": '⊂', "supset": '⊃', "subseteq": '⊆', "supseteq": '⊇',
	"cup": '∪', "cap": '∩', "emptyset": '∅', "setminus": '∖',
	"subsetneq": '⊊', "supsetneq": '⊋', "nsubseteq": '⊈', "nsupseteq": '⊉',
}

type setOperation struct {
	kind byte
	xy   [6]float64
}
type setOutline struct {
	advance    float64
	operations []setOperation
}

// Default Go faces do not contain most set relations. These commands explicitly
// select bundled, source-pinned upright mathematical outlines, with a real bold
// face; no synthetic slant, slash overlay or placeholder is substituted. Custom
// fonts and their explicit variants remain strict: their own glyph must exist.
func (p *parser) setSymbol(r rune, size float64) (box, error) {
	outlines, supplemental := supplementalSetOutlines[r]
	if !supplemental {
		return p.glyph(string(r), size) // native Go font cup/cap
	}
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
	face := 0
	if p.style&text.Bold != 0 {
		face = 1
	}
	outline := outlines[face]
	scale := size / 1000
	if scale <= 0 || !finite(scale) {
		return box{}, p.error("set-symbol dimensions underflow or overflow")
	}
	var b path.Builder
	for _, op := range outline.operations {
		point := func(i int) geom.Vec3 { return geom.V(op.xy[i]*scale, op.xy[i+1]*scale) }
		switch op.kind {
		case 'M':
			b.MoveTo(point(0))
		case 'L':
			b.LineTo(point(0))
		case 'C':
			b.CubicTo(point(0), point(2), point(4))
		case 'Z':
			b.Close()
		default:
			return box{}, p.error("invalid bundled set-symbol operation")
		}
	}
	o := gomath.NewObject(b.Path()).SetFill(p.options.Color).SetStroke(gomath.Transparent, 0)
	o.Name = string(r)
	if err := p.accentCurves(o); err != nil {
		return box{}, err
	}
	ink := o.Bounds()
	if !ink.Valid || ink.Size().X <= 0 || ink.Size().Y <= 0 {
		return box{}, p.error("set-symbol dimensions underflow")
	}
	left := math.Max(0, -ink.Min.X)
	if left > 0 {
		o.Shift(geom.V(left, 0))
	}
	return box{object: gomath.Group(o), width: math.Max(outline.advance*scale, ink.Max.X) + left,
		height: math.Max(0, ink.Max.Y), depth: math.Max(0, -ink.Min.Y), content: true}, nil
}
