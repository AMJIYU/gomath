package text

import (
	"fmt"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
	"gomath"
)

// MaxFallbackFonts bounds the entire ordered list, including the primary font.
const MaxFallbackFonts = 16

// NewWithFallback produces editable outlines using the first font containing
// each rune. See LayoutWithFallback for metrics, bounds, and missing-glyph policy.
func NewWithFallback(value string, fonts []*Font, opt Options) (*gomath.Object, error) {
	object, _, err := LayoutWithFallback(value, fonts, opt)
	return object, err
}

// LayoutWithFallback lays out a horizontal run with explicit, ordered fonts.
// Every font must be initialized; the list must contain 1..MaxFallbackFonts
// entries. The list is copied during the call and never retained. Font values
// are immutable; callers must not mutate the list concurrently with the call.
//
// A nonzero glyph index is actual coverage, including valid empty outlines such
// as spaces. If all fonts lack a rune, the call fails unless AllowMissing is set;
// that policy deliberately uses the primary font's glyph zero (.notdef).
// Font lookup/outline errors always propagate, without trying another font.
// Kerning applies only to adjacent glyphs chosen from the same Font pointer.
//
// All fonts use Size at the same baseline and normalized em. Ascent/Descent are
// the maxima of all configured fonts' line metrics, even for empty input or
// unused fallback fonts. Newlines move by Size*LineHeight, reset kerning, and
// include empty lines; tabs are one space. A small LineHeight can overlap ink.
// Width remains the widest line advance, including spaces. MaxRunes and the
// combined MaxGlyphSegments budget apply to the entire run. No shaping, bidi,
// grapheme-level selection, or system font discovery is performed.
func LayoutWithFallback(value string, fonts []*Font, opt Options) (*gomath.Object, Metrics, error) {
	if len(fonts) == 0 || len(fonts) > MaxFallbackFonts {
		return nil, Metrics{}, fmt.Errorf("fallback list must contain 1..%d fonts", MaxFallbackFonts)
	}
	snapshot := append([]*Font(nil), fonts...)
	for i, f := range snapshot {
		if err := f.Validate(); err != nil {
			return nil, Metrics{}, fmt.Errorf("fallback font %d: %w", i, err)
		}
	}
	return layout(value, snapshot[0], snapshot, opt)
}

type fallbackGlyph struct {
	font     *Font
	index    sfnt.GlyphIndex
	segments int
}

// Preflight repeated outlines against the combined work budget before creating
// a potentially large tree. The map is call-local and caches actual coverage;
// malformed glyph data is an error even if a later font could cover the rune.
func prepareFallback(value string, fonts []*Font, allowMissing bool, buf *sfnt.Buffer, ppem fixed.Int26_6) (map[rune]fallbackGlyph, error) {
	chosen := make(map[rune]fallbackGlyph)
	total := 0
	for _, r := range value {
		if r == '\n' {
			continue
		}
		if r == '\t' {
			r = ' '
		}
		glyph, ok := chosen[r]
		if !ok {
			glyph.font = fonts[0]
			for _, f := range fonts {
				idx, err := f.font.GlyphIndex(buf, r)
				if err != nil {
					return nil, fmt.Errorf("fallback glyph %U: %w", r, err)
				}
				if idx != 0 {
					glyph.font, glyph.index = f, idx
					break
				}
			}
			if glyph.index == 0 && !allowMissing {
				return nil, fmt.Errorf("fallback fonts have no glyph for %U", r)
			}
			segments, err := glyph.font.font.LoadGlyph(buf, glyph.index, ppem, nil)
			if err != nil {
				return nil, fmt.Errorf("load fallback glyph %U: %w", r, err)
			}
			glyph.segments = len(segments)
			chosen[r] = glyph
		}
		total += glyph.segments
		if total > MaxGlyphSegments {
			return nil, fmt.Errorf("text exceeds %d glyph segments", MaxGlyphSegments)
		}
	}
	return chosen, nil
}
