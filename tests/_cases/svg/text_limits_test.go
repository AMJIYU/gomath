package svg

import (
	"fmt"
	"gomath/geom"
	"gomath/text"
	"strings"
	"testing"
)

func TestTextSourceExpansionAndNestingBudgets(t *testing.T) {
	for _, tc := range []struct{ body, message string }{
		{`<text>` + strings.Repeat(" ", maxTextRunes+1) + `</text>`, "source budget"},
		{`<text>` + strings.Repeat("🦄", maxTextBytes/4+1) + `</text>`, "source budget"},
		{`<text>` + strings.Repeat(`<tspan>`, maxTextDepth) + `A` + strings.Repeat(`</tspan>`, maxTextDepth) + `</text>`, "text nesting"},
		{`<defs><text id="t">` + strings.Repeat(" ", maxTextRunes/2+1) + `</text></defs><use href="#t"/><use href="#t"/>`, "expanded text rune budget"},
	} {
		if o, err := Read(strings.NewReader(`<svg>`+tc.body+`</svg>`), DefaultOptions()); err == nil || o != nil || !strings.Contains(err.Error(), tc.message) {
			t.Fatalf("expected %q, got %v", tc.message, err)
		}
	}
	for _, body := range []string{
		`<text>` + strings.Repeat(" ", maxTextRunes) + `</text>`,
		`<text>` + strings.Repeat(`<tspan>`, maxTextDepth-1) + `A` + strings.Repeat(`</tspan>`, maxTextDepth-1) + `</text>`,
	} {
		if _, err := Read(strings.NewReader(`<svg>`+body+`</svg>`), DefaultOptions()); err != nil {
			t.Fatal("exact source/nesting limit rejected", err)
		}
	}
}

func TestTextAggregateGlyphSegmentAndElementBudgets(t *testing.T) {
	for _, tc := range []struct {
		name, message string
		budget        textBudget
		count         int
	}{
		{"glyph", "glyph budget", textBudget{glyphs: maxTextGlyphs}, 0},
		{"segment", "segment budget", textBudget{segments: maxTextSegments}, 0},
		{"element", "element count", textBudget{}, maxSVGElements - 1},
		{"layout work", "layout work budget", textBudget{layouts: maxTextLayouts}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := readDocument(strings.NewReader(`<svg><text>A</text></svg>`))
			if err != nil {
				t.Fatal(err)
			}
			state := expansion{document: doc, active: map[*element]bool{}, text: tc.budget, count: tc.count}
			_, err = state.instantiate(doc.root, expansionContext{matrix: geom.Identity(), style: initialSVGStyle(), viewport: [2]float64{100, 100}}, 1, false, nil)
			if err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("aggregate %s budget: %v", tc.name, err)
			}
		})
	}
	// An exact glyph bound accepts one more real glyph; the subsequent call
	// shares the per-import budget and fails before returning a partial tree.
	doc, _ := readDocument(strings.NewReader(`<svg><text>A</text></svg>`))
	state := expansion{document: doc, active: map[*element]bool{}, text: textBudget{glyphs: maxTextGlyphs - 1}}
	ctx := expansionContext{matrix: geom.Identity(), style: initialSVGStyle(), viewport: [2]float64{100, 100}}
	if _, err := state.instantiate(doc.root, ctx, 1, false, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := state.instantiate(doc.root, ctx, 1, false, nil); err == nil || !strings.Contains(err.Error(), "glyph budget") {
		t.Fatal("glyph budget reset between text elements", err)
	}
}

func TestTextFontResolutionBudget(t *testing.T) {
	for _, count := range []int{maxTextFonts, maxTextFonts + 1} {
		var source strings.Builder
		source.WriteString(`<svg>`)
		for i := range count {
			fmt.Fprintf(&source, `<text font-family="F%d">A</text>`, i)
		}
		source.WriteString(`</svg>`)
		calls := 0
		resolver := func(_ string, style text.FontStyle) (*text.Font, error) {
			calls++
			return text.DefaultFontForStyle(style)
		}
		o, err := Read(strings.NewReader(source.String()), Options{Width: 4, FontResolver: resolver})
		if calls != maxTextFonts || (count == maxTextFonts && (err != nil || o == nil)) || (count > maxTextFonts && (err == nil || o != nil || !strings.Contains(err.Error(), "font budget"))) {
			t.Fatalf("font budget count=%d calls=%d err=%v", count, calls, err)
		}
	}
}
