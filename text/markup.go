package text

import (
	"encoding/xml"
	"fmt"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
	"gomath"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	MaxMarkupBytes    = 1 << 20
	MaxMarkupDepth    = 32
	MaxMarkupElements = 16384
	MaxMarkupFonts    = 4
)

// MarkupFonts explicitly maps styles to real font faces. Regular is required;
// nil variants fail when requested, including on empty elements. No synthetic
// styles or implicit font fallback are applied. Fonts may be shared read-only.
type MarkupFonts struct {
	Regular, Bold, Italic, BoldItalic *Font
}

// MarkupOptions uses the same scale, line step, color and missing-glyph policy
// as Options. A nil Fonts selects the four embedded Go faces; otherwise the
// pointed-to table is copied during the call and never retained. Do not write
// that table concurrently with a call.
type MarkupOptions struct {
	Options
	Fonts *MarkupFonts
}

func DefaultMarkupOptions() MarkupOptions { return MarkupOptions{Options: DefaultOptions()} }

// NewMarkup returns the editable outlines of LayoutMarkup, discarding metrics.
func NewMarkup(source string, opt MarkupOptions) (*gomath.Object, error) {
	o, _, err := LayoutMarkup(source, opt)
	return o, err
}

// LayoutMarkup accepts a bounded XML text fragment containing only b, i and
// span foreground="#RRGGBB". Nested b/i select the actual BoldItalic face.
// XML entities are decoded; explicit newlines and whitespace are retained
// (XML normalizes literal CR and CRLF to LF); tabs remain one space.
// Comments, CDATA, declarations, namespaces and all other markup are errors.
//
// Kerning continues across color/element boundaries if the chosen Font pointer
// stays the same; a font change or newline breaks adjacency. Missing characters
// fail unless AllowMissing explicitly selects that style font's .notdef.
// Ascent/descent are maxima of all configured faces, even unused ones. Baseline
// step is Size*LineHeight, without automatic expansion for tall glyphs.
// The returned group Name is decoded text. No shaping/bidi/Pango is performed.
func LayoutMarkup(source string, opt MarkupOptions) (*gomath.Object, Metrics, error) {
	if len(source) > MaxMarkupBytes {
		return nil, Metrics{}, fmt.Errorf("markup exceeds %d input bytes", MaxMarkupBytes)
	}
	if !utf8.ValidString(source) {
		return nil, Metrics{}, fmt.Errorf("markup is not valid UTF-8")
	}
	if strings.Contains(source, "<!") || strings.Contains(source, "<?") {
		return nil, Metrics{}, fmt.Errorf("markup declarations, comments, CDATA and processing instructions are unsupported")
	}
	if strings.Contains(source, "]]>") {
		return nil, Metrics{}, fmt.Errorf("markup text must escape the XML sequence ]]>")
	}
	if err := validateMarkupEntities(source); err != nil {
		return nil, Metrics{}, err
	}
	fonts, err := markupFontTable(opt.Fonts)
	if err != nil {
		return nil, Metrics{}, err
	}
	value, plan, err := parseMarkup(source, fonts, opt.Color)
	if err != nil {
		return nil, Metrics{}, err
	}
	return layoutRun(value, fonts[Regular], nil, opt.Options, plan)
}

// encoding/xml replaces numeric surrogate references with U+FFFD rather than
// reporting their invalid scalar value. Validate references before decoding so
// malformed input cannot become a successful, visibly substituted glyph.
func validateMarkupEntities(source string) error {
	for i := 0; i < len(source); i++ {
		if source[i] != '&' {
			continue
		}
		end := strings.IndexByte(source[i+1:], ';')
		if end < 0 {
			return fmt.Errorf("unterminated markup entity")
		}
		end += i + 1
		entity := source[i+1 : end]
		switch entity {
		case "amp", "lt", "gt", "apos", "quot":
		default:
			if len(entity) < 2 || entity[0] != '#' {
				return fmt.Errorf("unsupported markup entity")
			}
			digits, base := entity[1:], 10
			if digits[0] == 'x' {
				digits, base = digits[1:], 16
			}
			for _, c := range digits {
				if !(c >= '0' && c <= '9' || base == 16 && (c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F')) {
					return fmt.Errorf("invalid numeric markup entity")
				}
			}
			n, err := strconv.ParseUint(digits, base, 32)
			if err != nil || !(n == 9 || n == 10 || n == 13 || n >= 0x20 && n <= 0xd7ff || n >= 0xe000 && n <= 0xfffd || n >= 0x10000 && n <= 0x10ffff) {
				return fmt.Errorf("numeric markup entity is not an XML character")
			}
		}
		i = end
	}
	return nil
}

func markupFontTable(config *MarkupFonts) ([MaxMarkupFonts]*Font, error) {
	var fonts [MaxMarkupFonts]*Font
	if config == nil {
		for i := range fonts {
			f, err := DefaultFontForStyle(FontStyle(i))
			if err != nil {
				return fonts, err
			}
			fonts[i] = f
		}
	} else {
		snapshot := *config
		fonts = [MaxMarkupFonts]*Font{snapshot.Regular, snapshot.Bold, snapshot.Italic, snapshot.BoldItalic}
	}
	for i, f := range fonts {
		if f == nil && i != int(Regular) {
			continue
		}
		if err := f.Validate(); err != nil {
			return fonts, fmt.Errorf("markup font %s: %w", markupStyleName(FontStyle(i)), err)
		}
	}
	return fonts, nil
}

func markupStyleName(style FontStyle) string {
	return [...]string{"regular", "bold", "italic", "bold-italic"}[style]
}

type markupRune struct {
	r     rune
	font  *Font
	color gomath.Color
	index sfnt.GlyphIndex
}

type markupPlan struct {
	fonts []*Font
	runes []markupRune
}

func parseMarkup(source string, fonts [MaxMarkupFonts]*Font, paint gomath.Color) (string, *markupPlan, error) {
	type scope struct {
		name  string
		style FontStyle
		color gomath.Color
	}
	var (
		stack    []scope
		decoded  strings.Builder
		elements int
	)
	state := scope{color: paint}
	plan := &markupPlan{}
	for _, f := range fonts {
		if f != nil {
			plan.fonts = append(plan.fonts, f)
		}
	}
	decoder := xml.NewDecoder(strings.NewReader(source))
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", nil, fmt.Errorf("markup XML: %w", err)
		}
		switch token := token.(type) {
		case xml.CharData:
			for _, r := range string(token) {
				if len(plan.runes) == MaxRunes {
					return "", nil, fmt.Errorf("markup exceeds %d decoded runes", MaxRunes)
				}
				plan.runes = append(plan.runes, markupRune{r: r, font: fonts[state.style], color: state.color})
				decoded.WriteRune(r)
			}
		case xml.StartElement:
			elements++
			if elements > MaxMarkupElements {
				return "", nil, fmt.Errorf("markup exceeds %d elements", MaxMarkupElements)
			}
			if len(stack) == MaxMarkupDepth {
				return "", nil, fmt.Errorf("markup exceeds %d nesting levels", MaxMarkupDepth)
			}
			if token.Name.Space != "" {
				return "", nil, fmt.Errorf("markup namespaces are unsupported")
			}
			old := state
			switch token.Name.Local {
			case "b", "i":
				if len(token.Attr) != 0 {
					return "", nil, fmt.Errorf("markup %s accepts no attributes", token.Name.Local)
				}
				if token.Name.Local == "b" {
					state.style |= Bold
				} else {
					state.style |= Italic
				}
			case "span":
				if len(token.Attr) != 1 || token.Attr[0].Name.Space != "" || token.Attr[0].Name.Local != "foreground" {
					return "", nil, fmt.Errorf("markup span requires exactly foreground=\"#RRGGBB\"")
				}
				state.color, err = markupColor(token.Attr[0].Value)
				if err != nil {
					return "", nil, err
				}
			default:
				return "", nil, fmt.Errorf("unsupported markup tag %q", token.Name.Local)
			}
			if fonts[state.style] == nil {
				return "", nil, fmt.Errorf("markup font variant %s is not configured", markupStyleName(state.style))
			}
			stack = append(stack, old)
			state.name = token.Name.Local
		case xml.EndElement:
			if len(stack) == 0 || token.Name.Space != "" || state.name != token.Name.Local {
				return "", nil, fmt.Errorf("invalid markup nesting at %q", token.Name.Local)
			}
			state = stack[len(stack)-1]
			stack = stack[:len(stack)-1]
		default:
			return "", nil, fmt.Errorf("unsupported XML markup construct")
		}
	}
	if len(stack) != 0 {
		return "", nil, fmt.Errorf("unclosed markup element")
	}
	return decoded.String(), plan, nil
}

func markupColor(value string) (gomath.Color, error) {
	if len(value) != 7 || value[0] != '#' {
		return gomath.Color{}, fmt.Errorf("markup foreground must be exactly #RRGGBB")
	}
	for _, c := range value[1:] {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return gomath.Color{}, fmt.Errorf("markup foreground must be exactly #RRGGBB")
		}
	}
	return gomath.Hex(value)
}

func prepareMarkup(plan *markupPlan, allowMissing bool, buf *sfnt.Buffer, ppem fixed.Int26_6) error {
	type key struct {
		font *Font
		r    rune
	}
	cache := make(map[key]fallbackGlyph)
	total := 0
	for i := range plan.runes {
		current := &plan.runes[i]
		r := current.r
		if r == '\n' {
			continue
		}
		if r == '\t' {
			r = ' '
		}
		k := key{current.font, r}
		glyph, ok := cache[k]
		if !ok {
			index, err := current.font.font.GlyphIndex(buf, r)
			if err != nil {
				return fmt.Errorf("markup glyph %U: %w", r, err)
			}
			if index == 0 && !allowMissing {
				return fmt.Errorf("markup style font has no glyph for %U", r)
			}
			segments, err := current.font.font.LoadGlyph(buf, index, ppem, nil)
			if err != nil {
				return fmt.Errorf("load markup glyph %U: %w", r, err)
			}
			glyph = fallbackGlyph{index: index, segments: len(segments)}
			cache[k] = glyph
		}
		current.index = glyph.index
		total += glyph.segments
		if total > MaxGlyphSegments {
			return fmt.Errorf("markup exceeds %d glyph segments", MaxGlyphSegments)
		}
	}
	return nil
}
