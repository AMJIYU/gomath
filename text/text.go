// Package text converts OpenType glyphs to editable vector objects without a
// native font engine. The initial API supports horizontal kerned glyph runs;
// complex-script shaping and bidi are tracked separately in the parity ledger.
package text

import (
	"fmt"
	"golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
	"gomath"
	"gomath/geom"
	"gomath/path"
	"math"
	"unicode/utf8"
)

// Font is immutable after parsing and may be shared by render workers. Buffers
// used while laying out a string are local to each call.
type Font struct{ font *sfnt.Font }

// MaxFontBytes and MaxRunes bound per-call font input and text allocation.
const MaxFontBytes = 64 << 20
const MaxRunes = 65536
const MaxGlyphSegments = 1 << 20

func Parse(data []byte) (*Font, error) {
	if len(data) > MaxFontBytes {
		return nil, fmt.Errorf("font exceeds %d bytes", MaxFontBytes)
	}
	// sfnt keeps the supplied data; copy it so the caller cannot mutate a Font.
	f, e := sfnt.Parse(append([]byte(nil), data...))
	if e != nil {
		return nil, fmt.Errorf("parse OpenType font: %w", e)
	}
	return &Font{font: f}, nil
}

// Validate reports a nil or uninitialized parsed font.
func (f *Font) Validate() error {
	if f == nil || f.font == nil {
		return fmt.Errorf("nil or uninitialized font")
	}
	return nil
}

type Options struct {
	Size, LineHeight float64
	Color            gomath.Color
	AllowMissing     bool
}

func DefaultOptions() Options { return Options{Size: .5, LineHeight: 1.3, Color: gomath.White} }

// Metrics includes advance width, including leading and trailing whitespace.
// Height is the line-box height; Ascent and Descent are font metrics, not ink
// bounds. Width is the widest line advance and Lines includes empty lines.
type Metrics struct {
	Width, Height, Ascent, Descent float64
	Lines                          int
}

func New(value string, f *Font, opt Options) (*gomath.Object, error) {
	object, _, err := Layout(value, f, opt)
	return object, err
}

// Layout produces the same editable objects as New and retains whitespace in
// its returned advance metrics. Tabs use one space, as in New. Layout is a
// horizontal kerned run with explicit newlines, without shaping or bidi.
func Layout(value string, f *Font, opt Options) (object *gomath.Object, metrics Metrics, err error) {
	return layout(value, f, nil, opt)
}

// layout retains the single-font geometry path. An optional fallback snapshot
// changes only glyph selection, cross-font kerning, and line metric aggregation.
func layout(value string, f *Font, fallbacks []*Font, opt Options) (object *gomath.Object, metrics Metrics, err error) {
	return layoutRun(value, f, fallbacks, opt, nil)
}

func layoutRun(value string, f *Font, fallbacks []*Font, opt Options, markup *markupPlan) (object *gomath.Object, metrics Metrics, err error) {
	if !utf8.ValidString(value) {
		return nil, Metrics{}, fmt.Errorf("text is not valid UTF-8")
	}
	if err := f.Validate(); err != nil {
		return nil, Metrics{}, err
	}
	if opt.Size <= 0 || opt.Size > 1e6 || math.IsNaN(opt.Size) || math.IsInf(opt.Size, 0) {
		return nil, Metrics{}, fmt.Errorf("font size must be finite and positive")
	}
	if opt.LineHeight <= 0 || opt.LineHeight > 1e6 || math.IsNaN(opt.LineHeight) || math.IsInf(opt.LineHeight, 0) {
		return nil, Metrics{}, fmt.Errorf("line height must be finite and positive")
	}
	if err := (gomath.Style{Fill: opt.Color}).Validate(); err != nil {
		return nil, Metrics{}, fmt.Errorf("text color: %w", err)
	}
	if len(value) > 4*MaxRunes || utf8.RuneCountInString(value) > MaxRunes {
		return nil, Metrics{}, fmt.Errorf("text exceeds %d runes", MaxRunes)
	}
	var buf sfnt.Buffer
	const ppem = fixed.Int26_6(64 * 1024)
	factor := opt.Size / (1024 * 64)
	lineAdvance := opt.Size * opt.LineHeight
	if factor <= 0 || lineAdvance <= 0 {
		return nil, Metrics{}, fmt.Errorf("font size or line height underflows layout coordinates")
	}
	fm, e := f.font.Metrics(&buf, ppem, font.HintingNone)
	if e != nil {
		return nil, Metrics{}, e
	}
	metrics = Metrics{Ascent: float64(fm.Ascent) * factor, Descent: float64(fm.Descent) * factor, Lines: 1}
	metricFonts := fallbacks[min(1, len(fallbacks)):]
	if markup != nil {
		metricFonts = markup.fonts[1:]
	}
	for _, candidate := range metricFonts {
		fm, e := candidate.font.Metrics(&buf, ppem, font.HintingNone)
		if e != nil {
			if markup != nil {
				return nil, Metrics{}, fmt.Errorf("markup font metrics: %w", e)
			}
			return nil, Metrics{}, fmt.Errorf("fallback font metrics: %w", e)
		}
		metrics.Ascent = math.Max(metrics.Ascent, float64(fm.Ascent)*factor)
		metrics.Descent = math.Max(metrics.Descent, float64(fm.Descent)*factor)
	}
	var choices map[rune]fallbackGlyph
	if markup != nil {
		if e := prepareMarkup(markup, opt.AllowMissing, &buf, ppem); e != nil {
			return nil, Metrics{}, e
		}
	}
	if len(fallbacks) > 1 {
		choices, e = prepareFallback(value, fallbacks, opt.AllowMissing, &buf, ppem)
		if e != nil {
			return nil, Metrics{}, e
		}
	}
	var x, y float64
	totalSegments := 0
	var previous sfnt.GlyphIndex
	var previousFont *Font
	var hasPrevious bool
	var glyphs []*gomath.Object
	runeIndex := 0
	for _, r := range value {
		selected, paint := f, opt.Color
		var idx sfnt.GlyphIndex
		if markup != nil {
			current := markup.runes[runeIndex]
			runeIndex++
			selected, paint, idx = current.font, current.color, current.index
		}
		if r == '\n' {
			metrics.Width = math.Max(metrics.Width, x)
			metrics.Lines++
			x = 0
			y -= lineAdvance
			hasPrevious = false
			continue
		}
		if r == '\t' {
			r = ' '
		}
		if markup == nil {
			if choices != nil {
				selected, idx = choices[r].font, choices[r].index
			} else {
				idx, e = selected.font.GlyphIndex(&buf, r)
				if e != nil {
					return nil, Metrics{}, e
				}
			}
		}
		if idx == 0 && !opt.AllowMissing {
			return nil, Metrics{}, fmt.Errorf("font has no glyph for %U; supply a font covering this character", r)
		}
		if hasPrevious && previousFont == selected {
			kern, e := selected.font.Kern(&buf, previous, idx, ppem, font.HintingNone)
			if e != nil && e != sfnt.ErrNotFound {
				return nil, Metrics{}, e
			}
			x += float64(kern) * factor
		}
		segments, e := selected.font.LoadGlyph(&buf, idx, ppem, nil)
		if e != nil {
			return nil, Metrics{}, fmt.Errorf("load glyph %U: %w", r, e)
		}
		totalSegments += len(segments)
		if totalSegments > MaxGlyphSegments {
			return nil, Metrics{}, fmt.Errorf("text exceeds %d glyph segments", MaxGlyphSegments)
		}
		var b path.Builder
		open := false
		point := func(p fixed.Point26_6) geom.Vec3 { return geom.V(x+float64(p.X)*factor, y-float64(p.Y)*factor) }
		for _, s := range segments {
			switch s.Op {
			case sfnt.SegmentOpMoveTo:
				if open {
					b.Close()
				}
				b.MoveTo(point(s.Args[0]))
				open = true
			case sfnt.SegmentOpLineTo:
				b.LineTo(point(s.Args[0]))
			case sfnt.SegmentOpQuadTo:
				b.QuadTo(point(s.Args[0]), point(s.Args[1]))
			case sfnt.SegmentOpCubeTo:
				b.CubicTo(point(s.Args[0]), point(s.Args[1]), point(s.Args[2]))
			default:
				return nil, Metrics{}, fmt.Errorf("unsupported glyph segment operation %d", s.Op)
			}
		}
		if open {
			b.Close()
		}
		p := b.Path()
		if len(p.Contours) > 0 {
			glyph := gomath.NewObject(p).SetFill(paint).SetStroke(gomath.Transparent, 0)
			glyph.Name = string(r)
			glyphs = append(glyphs, glyph)
		}
		advance, e := selected.font.GlyphAdvance(&buf, idx, ppem, font.HintingNone)
		if e != nil {
			return nil, Metrics{}, e
		}
		x += float64(advance) * factor
		previous, hasPrevious = idx, true
		previousFont = selected
	}
	o := gomath.Group(glyphs...)
	o.Name = value
	metrics.Width = math.Max(metrics.Width, x)
	metrics.Height = metrics.Ascent + metrics.Descent + float64(metrics.Lines-1)*lineAdvance
	if e := o.Validate(); e != nil {
		return nil, Metrics{}, fmt.Errorf("text layout: %w", e)
	}
	return o, metrics, nil
}
