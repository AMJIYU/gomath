package mathtex

import (
	"gomath/text"
	"unicode"
)

// MaxOperatorNameRunes includes every literal space and punctuation rune in a
// name, excluding its braces. The source and shared glyph-curve limits apply too.
const MaxOperatorNameRunes = 256

func (p *parser) namedOperator(size float64) (box, error) {
	p.skipSpace()
	kind := namedSideOperator
	if p.position < len(p.source) && p.source[p.position] == '*' {
		kind = namedDisplayOperator
		p.position++
	}
	name, err := p.operatorName()
	if err != nil {
		return box{}, err
	}
	old := p.style
	p.style &= text.Bold // actual upright font, retaining an enclosing bold style
	defer func() { p.style = old }()
	b, err := p.glyph(name, size)
	if err != nil {
		return box{}, err
	}
	if !visibleStackBody(b) {
		return box{}, p.error("operator name requires visible glyphs")
	}
	b.operator = kind
	return b, nil
}

// This is literal operator text, not a recursively interpreted math argument.
// No control sequences, nested groups, math scripts or shaping controls are
// accepted. In particular, do not silently flatten unsupported name semantics.
func (p *parser) operatorName() (string, error) {
	p.skipSpace()
	if p.position >= len(p.source) || p.source[p.position] != '{' {
		return "", p.error("operatorname requires a braced literal name")
	}
	p.position++
	p.depth++
	defer func() { p.depth-- }()
	if p.depth > MaxNesting {
		return "", p.error("math nesting exceeds 128")
	}
	start := p.position
	hasAlphanumeric := false
	for p.position < len(p.source) {
		r := p.source[p.position]
		if r == '}' {
			if p.position == start || !hasAlphanumeric {
				return "", p.error("operator name must contain a letter or decimal digit")
			}
			if p.source[start] == ' ' || p.source[p.position-1] == ' ' {
				return "", p.error("operator name cannot start or end with a space")
			}
			name := string(p.source[start:p.position])
			p.position++
			return name, nil
		}
		if p.position-start >= MaxOperatorNameRunes {
			return "", p.error("operator name exceeds 256 runes")
		}
		// Restrict literal names to scripts that this glyph-by-glyph layout
		// supports without shaping or bidirectional reordering.
		letter := unicode.IsLetter(r) && unicode.In(r, unicode.Latin, unicode.Greek, unicode.Cyrillic)
		alphanumeric := letter || r >= '0' && r <= '9'
		if !alphanumeric && r != ' ' && r != '-' && r != '.' {
			return "", p.error("unsupported operator-name rune; use Latin/Greek/Cyrillic letters, ASCII digits/spaces, hyphens or periods")
		}
		hasAlphanumeric = hasAlphanumeric || alphanumeric
		p.position++
	}
	return "", p.error("unclosed operator name")
}
