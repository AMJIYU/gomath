// Package code builds bounded, syntax-colored Go listings from real font
// outlines. It never executes source or invokes an external highlighter.
package code

import (
	"errors"
	"fmt"
	"math"
	"sync"

	"golang.org/x/image/font/gofont/gomono"
	"gomath"
	"gomath/text"
)

const (
	MaxSourceBytes   = 256 << 10
	MaxRunes         = 32768
	MaxLines         = 2048
	MaxColumns       = 4096
	MaxTokens        = 16384
	MaxDepth         = 64
	MaxGlyphs        = 16384
	MaxCurves        = 1 << 20
	DefaultMaxCurves = 1 << 18
	MaxCoordinate    = 1e6
)

var (
	ErrSource    = errors.New("invalid code source")
	ErrLanguage  = errors.New("unsupported code language")
	ErrOptions   = errors.New("invalid code options")
	ErrLimit     = errors.New("code limit exceeded")
	ErrFont      = errors.New("code font error")
	ErrSelection = errors.New("invalid code selection")
	ErrObject    = errors.New("invalid code object")
)

// Category is lexical, not semantic: builtins, types, functions, true and nil
// are identifiers. Whitespace has cells but no token or painted outline.
type Category string

const (
	Keyword     Category = "keyword"
	Identifier  Category = "identifier"
	Number      Category = "number"
	String      Category = "string"
	Comment     Category = "comment"
	Operator    Category = "operator"
	Punctuation Category = "punctuation"
)

// Palette is explicit sRGB. New copies its value; later caller edits are safe.
type Palette struct {
	Keyword, Identifier, Number, String, Comment, Operator, Punctuation gomath.Color
	LineNumber, Background                                              gomath.Color
}

func DefaultPalette() Palette {
	rgb := func(r, g, b float64) gomath.Color { return gomath.RGB(r/255, g/255, b/255) }
	return Palette{
		Keyword: rgb(198, 120, 221), Identifier: rgb(220, 223, 228), Number: rgb(209, 154, 102),
		String: rgb(152, 195, 121), Comment: rgb(127, 139, 153), Operator: rgb(86, 182, 194),
		Punctuation: rgb(171, 178, 191), LineNumber: rgb(127, 139, 153), Background: rgb(30, 34, 42),
	}
}

func (p Palette) color(c Category) gomath.Color {
	switch c {
	case Keyword:
		return p.Keyword
	case Number:
		return p.Number
	case String:
		return p.String
	case Comment:
		return p.Comment
	case Operator:
		return p.Operator
	case Punctuation:
		return p.Punctuation
	default:
		return p.Identifier
	}
}

// Limits may only lower hard limits. Zero uses the hard limit, except Curves
// which uses DefaultMaxCurves. Depth bounds lexical delimiter nesting, even
// though the scanner and layout do not recurse over the source.
type Limits struct{ SourceBytes, Runes, Lines, Columns, Tokens, Depth, Glyphs, Curves int }

type Options struct {
	Language        string  // "" or "go"; no guessing, aliases, or other languages.
	Size            float64 // em size, [1e-6,1e3]; use DefaultOptions.
	LineHeight      float64 // baseline advance in em, [1,64].
	TabWidth        int     // tabs advance to the next multiple of this many cells, [1,64].
	LineNumbers     bool
	FirstLineNumber int        // zero defaults to 1; otherwise [1,1e9].
	Background      bool       // a plain, filled rectangle behind all line boxes.
	Padding         float64    // world units, [0,1e4].
	Font            *text.Font // nil uses licensed embedded Go Mono; used runes must have the space advance.
	Palette         *Palette   // nil uses DefaultPalette; all channels must be valid.
	Limits          Limits
}

func DefaultOptions() Options {
	return Options{Language: "go", Size: .5, LineHeight: 1.3, TabWidth: 4, FirstLineNumber: 1, Padding: .2}
}

var monoFont = sync.OnceValues(func() (*text.Font, error) { return text.Parse(gomono.TTF) })

// DefaultFont returns the shared immutable parsed Go Mono regular face.
func DefaultFont() (*text.Font, error) { return monoFont() }

type config struct {
	Options
	palette Palette
	limits  Limits
}

func normalize(o Options) (config, error) {
	if o.Language != "" && o.Language != "go" {
		return config{}, fmt.Errorf("%w: %q (supported: go)", ErrLanguage, o.Language)
	}
	if !finite(o.Size) || o.Size < 1e-6 || o.Size > 1e3 || !finite(o.LineHeight) || o.LineHeight < 1 || o.LineHeight > 64 ||
		!finite(o.Padding) || o.Padding < 0 || o.Padding > 1e4 || o.TabWidth < 1 || o.TabWidth > 64 {
		return config{}, fmt.Errorf("%w: size, line height, padding or tab width out of range", ErrOptions)
	}
	if o.FirstLineNumber == 0 {
		o.FirstLineNumber = 1
	}
	if o.FirstLineNumber < 1 || o.FirstLineNumber > 1e9 {
		return config{}, fmt.Errorf("%w: first line number must be in [1,1e9]", ErrOptions)
	}
	p := DefaultPalette()
	if o.Palette != nil {
		p = *o.Palette
	}
	for _, color := range []gomath.Color{p.Keyword, p.Identifier, p.Number, p.String, p.Comment, p.Operator, p.Punctuation, p.LineNumber, p.Background} {
		if err := (gomath.Style{Fill: color}).Validate(); err != nil {
			return config{}, fmt.Errorf("%w: palette: %v", ErrOptions, err)
		}
	}
	l := o.Limits
	for _, bound := range []struct {
		value          *int
		hard, fallback int
		name           string
	}{
		{&l.SourceBytes, MaxSourceBytes, MaxSourceBytes, "source bytes"}, {&l.Runes, MaxRunes, MaxRunes, "runes"},
		{&l.Lines, MaxLines, MaxLines, "lines"}, {&l.Columns, MaxColumns, MaxColumns, "columns"},
		{&l.Tokens, MaxTokens, MaxTokens, "tokens"}, {&l.Depth, MaxDepth, MaxDepth, "depth"},
		{&l.Glyphs, MaxGlyphs, MaxGlyphs, "glyphs"}, {&l.Curves, MaxCurves, DefaultMaxCurves, "curves"},
	} {
		if *bound.value < 0 || *bound.value > bound.hard {
			return config{}, fmt.Errorf("%w: %s limit must be in [0,%d]", ErrOptions, bound.name, bound.hard)
		}
		if *bound.value == 0 {
			*bound.value = bound.fallback
		}
	}
	// Do not retain the caller's palette pointer.
	o.Palette = nil
	o.Limits = l
	o.Language = "go"
	return config{Options: o, palette: p, limits: l}, nil
}

func finite(x float64) bool { return !math.IsNaN(x) && !math.IsInf(x, 0) }
