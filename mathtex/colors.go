package mathtex

import (
	"gomath"
	"strings"
)

// A finite opaque sRGB palette. These are the basic TeX/xcolor named RGB
// values, not GoMath's animation palette (whose Red/Green/Blue differ).
var mathColors = map[string]gomath.Color{
	"black": gomath.RGB(0, 0, 0), "blue": gomath.RGB(0, 0, 1),
	"brown": gomath.RGB(.75, .5, .25), "cyan": gomath.RGB(0, 1, 1),
	"darkgray": gomath.RGB(.25, .25, .25), "gray": gomath.RGB(.5, .5, .5),
	"green": gomath.RGB(0, 1, 0), "lightgray": gomath.RGB(.75, .75, .75),
	"lime": gomath.RGB(.75, 1, 0), "magenta": gomath.RGB(1, 0, 1),
	"olive": gomath.RGB(.5, .5, 0), "orange": gomath.RGB(1, .5, 0),
	"pink": gomath.RGB(1, .75, .75), "purple": gomath.RGB(.75, 0, .25),
	"red": gomath.RGB(1, 0, 0), "teal": gomath.RGB(0, .5, .5),
	"violet": gomath.RGB(.5, 0, .5), "white": gomath.RGB(1, 1, 1),
	"yellow": gomath.RGB(1, 1, 0),
}

// Color/model tokens are data, never math or macro input. The small bound is
// independent of the shared source bound; nested groups/commands are rejected.
func (p *parser) colorToken(open, close rune) (string, error) {
	p.skipSpace()
	if p.position == len(p.source) || p.source[p.position] != open {
		return "", p.error("color requires a delimited literal token")
	}
	p.position++
	start := p.position
	for p.position < len(p.source) && p.source[p.position] != close {
		r := p.source[p.position]
		if p.position-start == 16 {
			return "", p.error("color token exceeds 16 runes")
		}
		if r > 127 || !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == ' ') {
			return "", p.error("color token must contain only ASCII letters, digits or spaces")
		}
		p.position++
	}
	if p.position == len(p.source) {
		return "", p.error("unclosed color token")
	}
	value := strings.TrimSpace(string(p.source[start:p.position]))
	p.position++
	if value == "" {
		return "", p.error("empty color token")
	}
	return value, nil
}

func (p *parser) colorSpec() (gomath.Color, error) {
	p.skipSpace()
	model := ""
	if p.position < len(p.source) && p.source[p.position] == '[' {
		var err error
		model, err = p.colorToken('[', ']')
		if err != nil {
			return gomath.Color{}, err
		}
		if model != "HTML" {
			return gomath.Color{}, p.error("unsupported color model; only HTML is accepted")
		}
	}
	value, err := p.colorToken('{', '}')
	if err != nil {
		return gomath.Color{}, err
	}
	if model == "HTML" {
		if len(value) != 6 {
			return gomath.Color{}, p.error("HTML color requires exactly six hex digits, without # or alpha")
		}
		color, err := gomath.Hex(value)
		if err != nil {
			return gomath.Color{}, p.error("invalid HTML color hex digits")
		}
		return color, nil
	}
	if color, ok := mathColors[value]; ok {
		return color, nil
	}
	return gomath.Color{}, p.error("unsupported named color " + value)
}

func (p *parser) coloredGroup(size float64) (box, error) {
	color, err := p.colorSpec()
	if err != nil {
		return box{}, err
	}
	oldColor := p.options.Color
	p.options.Color = color
	defer func() { p.options.Color = oldColor }()
	return p.requiredGroup(size)
}
