package code

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"gomath"
	"gomath/geom"
	"gomath/path"
	"gomath/text"
)

// Metrics describes the initial local grid, independent of later Object edits.
// First source baseline is (0,0); subsequent baselines descend by LineAdvance.
// ContentWidth retains whitespace. Left/Right/Top/Bottom include the number
// gutter (digits plus two cells) and Padding, even with Background disabled.
type Metrics struct {
	CellWidth, LineAdvance, Ascent, Descent, ContentWidth float64
	Left, Right, Top, Bottom                              float64
	Lines, Columns, Glyphs, Curves                        int
}

// Glyph is an ink-bearing cell. Offset is an original source byte offset;
// columns count runes and expanded tabs, not bytes or terminal display widths.
// Line-number glyphs are selected through LineNumber, not Glyphs.
type Glyph struct {
	Rune                        rune
	Offset, Line, Column, Token int
	Object                      *gomath.Object
}

// Code owns an editable vector tree. Authoring is single-threaded; freeze with
// Snapshot or a Scene before concurrent sampling. Lines/tokens retain stable
// child references through affine transforms, color edits and compatible
// gomath.Transform operations. Do not replace Object or change child topology.
type Code struct {
	Object                     *gomath.Object
	source                     string
	config                     config
	metrics                    Metrics
	lines                      []Line
	tokens                     []Token
	glyphs                     []Glyph
	lineObjects, numberObjects []*gomath.Object
	tokenObjects               [][]*gomath.Object
	background                 *gomath.Object
	nodes                      []ownedNode
}

type ownedNode struct {
	object   *gomath.Object
	children []*gomath.Object
}

// New validates source, options, lexical limits, exact used-glyph advances and
// combined source/line-number glyph and curve budgets before publishing a tree.
// Lexically valid fragments are accepted; parsing, type checking and execution
// are deliberately outside this display API.
func New(source string, opt Options) (*Code, error) {
	c, err := normalize(opt)
	if err != nil {
		return nil, err
	}
	lines, cells, err := inspect(source, c)
	if err != nil {
		return nil, err
	}
	// A small caller substring may retain a much larger backing string.
	// Retain only this bounded source and derive all token spans from it.
	source = strings.Clone(source)
	tokens, err := lex(source, lines, c)
	if err != nil {
		return nil, err
	}
	if c.Font == nil {
		c.Font, err = DefaultFont()
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrFont, err)
		}
	}
	if err = c.Font.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrFont, err)
	}
	to := text.DefaultOptions()
	to.Size = 1
	_, space, err := text.Layout(" ", c.Font, to)
	if err != nil {
		return nil, fmt.Errorf("%w: space metrics: %v", ErrFont, err)
	}
	m := Metrics{CellWidth: space.Width * c.Size, LineAdvance: c.Size * c.LineHeight, Ascent: space.Ascent * c.Size, Descent: space.Descent * c.Size, Lines: len(lines)}
	if !finite(m.CellWidth) || m.CellWidth <= 0 || !finite(m.Ascent) || !finite(m.Descent) || m.Ascent <= 0 || m.Descent < 0 {
		return nil, fmt.Errorf("%w: invalid cell or vertical font metrics", ErrFont)
	}
	numberGlyphs, digits := 0, 0
	if c.LineNumbers {
		digits = len(strconv.Itoa(lines[len(lines)-1].Number))
		for _, line := range lines {
			numberGlyphs += len(strconv.Itoa(line.Number))
		}
	}
	if len(cells)+numberGlyphs > c.limits.Glyphs {
		return nil, fmt.Errorf("%w: source and line-number glyphs > %d", ErrLimit, c.limits.Glyphs)
	}
	for _, line := range lines {
		m.Columns = max(m.Columns, line.Columns)
	}
	m.ContentWidth = float64(m.Columns) * m.CellWidth
	m.Left = -c.Padding
	if c.LineNumbers {
		m.Left -= float64(digits+2) * m.CellWidth
	}
	m.Right = m.ContentWidth + c.Padding
	m.Top = m.Ascent + c.Padding
	m.Bottom = -float64(len(lines)-1)*m.LineAdvance - m.Descent - c.Padding
	for _, value := range []float64{m.Left, m.Right, m.Top, m.Bottom, m.Right - m.Left, m.Top - m.Bottom} {
		if !finite(value) || math.Abs(value) > MaxCoordinate {
			return nil, fmt.Errorf("%w: layout dimensions exceed %g", ErrLimit, MaxCoordinate)
		}
	}
	// Unit-em templates keep font extraction independent of caller size and
	// prevent whitespace, kerning or ligatures from changing the glyph grid.
	templates := make(map[rune]path.Path)
	curves := 0
	outline := func(r rune, x, y float64, paint gomath.Color) (*gomath.Object, error) {
		p, ok := templates[r]
		if !ok {
			o, metrics, e := text.Layout(string(r), c.Font, to)
			if e != nil {
				return nil, fmt.Errorf("%w: glyph %U: %v", ErrFont, r, e)
			}
			if math.Abs(metrics.Width-space.Width) > 1e-12 {
				return nil, fmt.Errorf("%w: glyph %U advance differs from space; require monospaced glyphs", ErrFont, r)
			}
			if len(o.Children) != 1 || len(o.Children[0].Path.Contours) == 0 {
				return nil, fmt.Errorf("%w: visible rune %U has no outline", ErrFont, r)
			}
			p = o.Children[0].Path
			templates[r] = p
		}
		count := 0
		for _, contour := range p.Contours {
			count += len(contour.Curves)
		}
		if count > c.limits.Curves-curves {
			return nil, fmt.Errorf("%w: glyph curves > %d", ErrLimit, c.limits.Curves)
		}
		curves += count
		matrix := geom.Translation(geom.V(x, y)).Mul(geom.Scaling(c.Size, c.Size, c.Size))
		p = p.Transform(matrix)
		for _, contour := range p.Contours {
			for _, curve := range contour.Curves {
				for _, v := range []geom.Vec3{curve.P0, curve.P1, curve.P2, curve.P3} {
					if !v.Finite() || math.Abs(v.X) > MaxCoordinate || math.Abs(v.Y) > MaxCoordinate {
						return nil, fmt.Errorf("%w: glyph coordinates exceed %g", ErrLimit, MaxCoordinate)
					}
				}
			}
		}
		return gomath.NewObject(p).SetFill(paint).SetStroke(gomath.Transparent, 0), nil
	}
	listing := &Code{source: source, config: c, metrics: m, lines: lines, tokens: tokens,
		lineObjects: make([]*gomath.Object, len(lines)), tokenObjects: make([][]*gomath.Object, len(tokens))}
	for i, line := range lines {
		listing.lineObjects[i] = gomath.Group()
		listing.lineObjects[i].Name = source[line.Start:line.End]
	}
	tokenIndex := 0
	for _, cell := range cells {
		for tokenIndex < len(tokens) && tokens[tokenIndex].End <= cell.offset {
			tokenIndex++
		}
		if tokenIndex >= len(tokens) || cell.offset < tokens[tokenIndex].Start {
			return nil, fmt.Errorf("%w: unclassified visible source at byte %d", ErrSource, cell.offset)
		}
		glyph, e := outline(cell.r, float64(cell.column)*m.CellWidth, -float64(cell.line)*m.LineAdvance, c.palette.color(tokens[tokenIndex].Category))
		if e != nil {
			return nil, e
		}
		glyph.Name = string(cell.r)
		listing.glyphs = append(listing.glyphs, Glyph{cell.r, cell.offset, cell.line, cell.column, tokenIndex, glyph})
		line := listing.lineObjects[cell.line]
		line.Children = append(line.Children, glyph)
		listing.tokenObjects[tokenIndex] = append(listing.tokenObjects[tokenIndex], glyph)
	}
	var children []*gomath.Object
	if c.Background {
		if c.limits.Curves-curves < 4 {
			return nil, fmt.Errorf("%w: background curves > %d", ErrLimit, c.limits.Curves)
		}
		var b path.Builder
		b.MoveTo(geom.V(m.Left, m.Bottom))
		b.LineTo(geom.V(m.Right, m.Bottom))
		b.LineTo(geom.V(m.Right, m.Top))
		b.LineTo(geom.V(m.Left, m.Top))
		b.Close()
		listing.background = gomath.NewObject(b.Path()).SetFill(c.palette.Background).SetStroke(gomath.Transparent, 0)
		listing.background.Name = "Code background"
		children = append(children, listing.background)
		curves += 4
	}
	if c.LineNumbers {
		for _, line := range lines {
			label := strconv.Itoa(line.Number)
			number := gomath.Group()
			number.Name = label
			for j, r := range label {
				g, e := outline(r, float64(j-len(label)-2)*m.CellWidth, -float64(line.Index)*m.LineAdvance, c.palette.LineNumber)
				if e != nil {
					return nil, e
				}
				g.Name = string(r)
				number.Children = append(number.Children, g)
			}
			listing.numberObjects = append(listing.numberObjects, number)
		}
		numbers := gomath.Group(listing.numberObjects...)
		numbers.Name = "Code line numbers"
		children = append(children, numbers)
	}
	body := gomath.Group(listing.lineObjects...)
	body.Name = "Code lines"
	children = append(children, body)
	listing.Object = gomath.Group(children...)
	listing.Object.Name = "Code"
	listing.metrics.Glyphs = len(cells) + numberGlyphs
	listing.metrics.Curves = curves
	listing.Object.Walk(func(o *gomath.Object) {
		listing.nodes = append(listing.nodes, ownedNode{o, append([]*gomath.Object(nil), o.Children...)})
	})
	if err := listing.validate(); err != nil {
		return nil, err
	}
	return listing, nil
}

func (c *Code) Text() string {
	if c == nil {
		return ""
	}
	return c.source
}
func (c *Code) Metrics() Metrics {
	if c == nil {
		return Metrics{}
	}
	return c.metrics
}
func (c *Code) Lines() []Line {
	if c == nil {
		return nil
	}
	return append([]Line(nil), c.lines...)
}
func (c *Code) Tokens() []Token {
	if c == nil {
		return nil
	}
	return append([]Token(nil), c.tokens...)
}

// Glyphs copies the selection slice, retaining live authoring glyph pointers.
func (c *Code) Glyphs() []Glyph {
	if c == nil {
		return nil
	}
	return append([]Glyph(nil), c.glyphs...)
}

func (c *Code) Line(index int) (*gomath.Object, error) {
	if c == nil || index < 0 || index >= len(c.lineObjects) {
		return nil, fmt.Errorf("%w: line %d", ErrSelection, index)
	}
	return c.lineObjects[index], nil
}

// Token returns a fresh slice of live ink glyph references in source order.
// Multiline tokens may refer to several line groups. Do not reparent these
// glyphs into the same scene; animate them directly, or use TokenCopy.
func (c *Code) Token(index int) ([]*gomath.Object, error) {
	if c == nil || index < 0 || index >= len(c.tokenObjects) {
		return nil, fmt.Errorf("%w: token %d", ErrSelection, index)
	}
	return append([]*gomath.Object(nil), c.tokenObjects[index]...), nil
}
func (c *Code) LineNumber(index int) (*gomath.Object, error) {
	if c == nil || index < 0 || index >= len(c.numberObjects) {
		return nil, fmt.Errorf("%w: unavailable line number %d", ErrSelection, index)
	}
	return c.numberObjects[index], nil
}

// Background returns the live rectangle or nil when it was disabled.
func (c *Code) Background() *gomath.Object {
	if c == nil {
		return nil
	}
	return c.background
}

func (c *Code) TokenCopy(index int) (*gomath.Object, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}
	glyphs, err := c.Token(index)
	if err != nil {
		return nil, err
	}
	for i, g := range glyphs {
		glyphs[i] = g.Copy()
	}
	o := gomath.Group(glyphs...)
	o.Name = c.tokens[index].Text
	return o, nil
}
