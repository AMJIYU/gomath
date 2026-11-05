package mathtex

import (
	"gomath"
	"gomath/geom"
	"gomath/path"
	"math"
)

// Delimiter shape is independent of its left/right position: mixed pairs such
// as \left(x\right] and evaluation bars with an invisible side are intentional.
func (p *parser) delimiter() (string, error) {
	p.skipSpace()
	if p.position >= len(p.source) {
		return "", p.error("missing delimiter")
	}
	r := p.source[p.position]
	if r != '\\' {
		p.position++
		switch r {
		case '(', ')', '[', ']', '{', '}', '|', '.', '<', '>', '/':
			return string(r), nil
		}
		return "", p.error("unsupported delimiter " + string(r))
	}
	command, end := p.commandAt()
	p.position = end
	if value, ok := delimiterNames[command]; ok {
		return value, nil
	}
	return "", p.error("unsupported delimiter command \\" + command)
}

var delimiterNames = map[string]string{
	"{": "{", "}": "}", "lbrace": "{", "rbrace": "}",
	"lparen": "(", "rparen": ")", "lbrack": "[", "rbrack": "]",
	"langle": "<", "rangle": ">", "vert": "|", "lvert": "|", "rvert": "|",
	"|": "||", "Vert": "||", "lVert": "||", "rVert": "||",
	"lfloor": "floorL", "rfloor": "floorR", "lceil": "ceilL", "rceil": "ceilR",
	"backslash": "backslash",
}

func (p *parser) enclose(inner box, left, right string, size float64) box {
	axis := size * .16
	radius := math.Max(size*.45, math.Max(inner.height-axis, inner.depth+axis)) + size*.1
	top, bottom := axis+radius, axis-radius
	l := p.delimiterBox(left, top, bottom, size)
	r := p.delimiterBox(right, top, bottom, size)
	gapL, gapR := 0.0, 0.0
	if left != "." {
		gapL = size * .1
	}
	if right != "." {
		gapR = size * .1
	}
	inner.object.Shift(geom.V(l.width+gapL, 0))
	r.object.Shift(geom.V(l.width+gapL+inner.width+gapR, 0))
	o := gomath.Group(l.object, inner.object, r.object)
	o.Name = "Delimited"
	return box{object: o, width: l.width + gapL + inner.width + gapR + r.width, height: math.Max(inner.height, math.Max(l.height, r.height)), depth: math.Max(inner.depth, math.Max(l.depth, r.depth)), content: true}
}

// Visible delimiters are stroked vector curves with a fixed em-based stroke,
// scaled in height to the entire inner box including scripts and fractions.
func (p *parser) delimiterBox(kind string, top, bottom, size float64) box {
	if kind == "." {
		o := gomath.Group()
		o.Name = "Delimiter ."
		return box{object: o}
	}
	stroke := size * .045
	width := math.Min(size*.6, size*.28+(top-bottom)*.04)
	if kind == "|" {
		width = stroke
	}
	if kind == "||" {
		width = size * .2
	}
	// Geometric bounds include the stroke inside the box dimensions.
	a, b := stroke/2, width-stroke/2
	high, low := top-stroke/2, bottom+stroke/2
	mid := (high + low) / 2
	half := (high - low) / 2
	var pth path.Builder
	right := kind == ")" || kind == "]" || kind == "}" || kind == ">" || kind == "floorR" || kind == "ceilR"
	x := func(v float64) float64 {
		if right {
			return a + b - v
		}
		return v
	}
	point := func(px, py float64) geom.Vec3 { return geom.V(x(px), py) }
	switch kind {
	case "[", "]", "floorL", "floorR", "ceilL", "ceilR":
		if kind == "floorL" || kind == "floorR" {
			pth.MoveTo(point(a, high))
		} else {
			pth.MoveTo(point(b, high))
			pth.LineTo(point(a, high))
		}
		pth.LineTo(point(a, low))
		if kind != "ceilL" && kind != "ceilR" {
			pth.LineTo(point(b, low))
		}
	case "(", ")":
		pth.MoveTo(point(b, high))
		pth.CubicTo(point(a+(b-a)*.15, high-half*.3), point(a, mid+half*.4), point(a, mid))
		pth.CubicTo(point(a, mid-half*.4), point(a+(b-a)*.15, low+half*.3), point(b, low))
	case "{", "}":
		c := (a + b) / 2
		pth.MoveTo(point(b, high))
		pth.CubicTo(point(c, high), point(c, mid+half*.8), point(c, mid+half*.5))
		pth.CubicTo(point(c, mid+half*.15), point(c, mid+half*.1), point(a, mid))
		pth.CubicTo(point(c, mid-half*.1), point(c, mid-half*.15), point(c, mid-half*.5))
		pth.CubicTo(point(c, mid-half*.8), point(c, low), point(b, low))
	case "<", ">":
		pth.MoveTo(point(b, high))
		pth.LineTo(point(a, mid))
		pth.LineTo(point(b, low))
	case "|":
		pth.MoveTo(geom.V(a, high))
		pth.LineTo(geom.V(a, low))
	case "||":
		pth.MoveTo(geom.V(a, high))
		pth.LineTo(geom.V(a, low))
		pth.MoveTo(geom.V(b, high))
		pth.LineTo(geom.V(b, low))
	case "/":
		pth.MoveTo(geom.V(a, low))
		pth.LineTo(geom.V(b, high))
	case "backslash":
		pth.MoveTo(geom.V(a, high))
		pth.LineTo(geom.V(b, low))
	}
	o := gomath.NewObject(pth.Path()).SetStroke(p.options.Color, stroke)
	o.Name = "Delimiter " + kind
	return box{object: o, width: width, height: math.Max(0, top), depth: math.Max(0, -bottom), content: true}
}
