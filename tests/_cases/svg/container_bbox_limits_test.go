package svg

import (
	"fmt"
	"gomath/geom"
	"gomath/text"
	"strings"
	"testing"
)

func TestSVGContainerBBoxSharedImportPathBudgetBoundary(t *testing.T) {
	for _, tc := range []struct {
		name, defs, body string
	}{
		{"group", "", `<g clip-path="url(#c)"><rect width="4" height="6"/></g>`},
		{"use-group", `<g id="g"><rect width="4" height="6"/></g>`, `<use href="#g" clip-path="url(#c)"/>`},
		{"use-chain", `<rect id="r" width="4" height="6"/><use id="u" href="#r"/>`, `<use href="#u" clip-path="url(#c)"/>`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prefix := `<svg><defs><clipPath id="c" clipPathUnits="objectBoundingBox"><rect width=".5" height="1"/></clipPath>` + tc.defs + `</defs>`
			// Two source clip charges, then scope + outline scan + transformed
			// outline + native clip materialization for each target instance.
			accepted := (maxClipPaths - 2) / 4
			if _, err := Read(strings.NewReader(prefix+strings.Repeat(tc.body, accepted)+`</svg>`), DefaultOptions()); err != nil {
				t.Fatal("exact accepted boundary rejected", err)
			}
			object, err := Read(strings.NewReader(prefix+strings.Repeat(tc.body, accepted+1)+`</svg>`), DefaultOptions())
			if object != nil || err == nil || !strings.Contains(err.Error(), "materialization budget") {
				t.Fatal("container bbox work bypassed shared budget", err)
			}
		})
	}
}

func TestSVGContainerBBoxEvenEmptyAndDegenerateScopesSpendBudget(t *testing.T) {
	for _, body := range []string{`<g clip-path="url(#c)"/>`, `<g clip-path="url(#c)"><line x2="4"/></g>`} {
		source := `<svg><defs><clipPath id="c" clipPathUnits="objectBoundingBox"/></defs>` + strings.Repeat(body, maxClipPaths) + `</svg>`
		object, err := Read(strings.NewReader(source), DefaultOptions())
		if object != nil || err == nil || !strings.Contains(err.Error(), "materialization budget") {
			t.Fatal("empty/degenerate container bypassed work budget", err)
		}
	}
}

func TestSVGContainerBBoxEveryActiveScopeSpendsGeometryBudget(t *testing.T) {
	for _, tc := range []struct {
		name   string
		budget clipBudget
	}{
		{"paths", clipBudget{paths: maxClipPaths - 2}},
		{"curves", clipBudget{curves: maxClipTotalCurves - 8}},
		{"contours", clipBudget{contours: maxClipTotalContours - 2}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := expansion{document: &document{clipBudget: tc.budget}}
			var a, b geom.Bounds
			ctx := expansionContext{bboxes: []bboxScope{{&a, geom.Identity()}}}
			p := bboxRectangle(1, 2, 3, 4)
			if err := s.collectBBox(p, ctx); err != nil {
				t.Fatal("exact remaining geometry budget rejected", err)
			}
			if !a.Valid || a.Min != geom.V(1, 2) || a.Max != geom.V(4, 6) {
				t.Fatal("geometry missing at boundary", a)
			}
			ctx.bboxes = append(ctx.bboxes, bboxScope{&b, geom.Identity()})
			if err := s.collectBBox(p, ctx); err == nil {
				t.Fatal("nested target reused another scope's budget")
			}
		})
	}
}

func TestSVGContainerBBoxMetricWorkSharesTextAndExpansionBudgets(t *testing.T) {
	font := text.DefaultFont()
	for _, tc := range []struct {
		name string
		s    expansion
	}{
		{"layouts", expansion{text: textBudget{layouts: maxTextLayouts}}},
		{"glyphs", expansion{text: textBudget{glyphs: maxTextGlyphs}}},
		{"segments", expansion{text: textBudget{segments: maxTextSegments}}},
		{"expanded", expansion{count: maxSVGElements}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := tc.s
			s.document = &document{}
			if _, err := s.bboxTextMetrics('A', 0, font, 24); err == nil {
				t.Fatal("bbox metric outlines bypassed", tc.name)
			}
		})
	}
	// A genuine cache hit needs no extra outlines or budget. Changing the font,
	// size, rune or pair cannot reuse a different advance/kerning record.
	s := expansion{document: &document{}}
	m, err := s.bboxTextMetrics('A', 0, font, 24)
	if err != nil {
		t.Fatal(err)
	}
	s.text.layouts = maxTextLayouts
	if cached, err := s.bboxTextMetrics('A', 0, font, 24); err != nil || cached != m {
		t.Fatal("immutable metrics cache changed", err)
	}
	for _, key := range [][3]int{{'A', 0, 25}, {'V', 0, 24}, {'A', 'V', 24}} {
		if _, err := s.bboxTextMetrics(rune(key[0]), rune(key[1]), font, float64(key[2])); err == nil {
			t.Fatal("metrics key collision", key)
		}
	}
}

func TestSVGContainerBBoxExpandedGraphStillBounded(t *testing.T) {
	var source strings.Builder
	source.WriteString(`<svg><defs><g id="a0"><path/></g>`)
	for i := 1; i < 17; i++ {
		fmt.Fprintf(&source, `<g id="a%d"><use href="#a%d"/><use href="#a%d"/></g>`, i, i-1, i-1)
	}
	source.WriteString(`</defs><use href="#a16"/></svg>`)
	object, err := Read(strings.NewReader(source.String()), DefaultOptions())
	if object != nil || err == nil || !strings.Contains(err.Error(), "expanded element count") {
		t.Fatal("expansion limit changed", err)
	}
}
