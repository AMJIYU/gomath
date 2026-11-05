package text_test

import (
	"bytes"
	"gomath"
	"gomath/geom"
	"gomath/render"
	"gomath/text"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestMarkupActualFacesAndComposition(t *testing.T) {
	for _, tc := range []struct {
		source string
		style  text.FontStyle
	}{
		{"AVg", text.Regular}, {"<b>AVg</b>", text.Bold}, {"<i>AVg</i>", text.Italic},
		{"<b><i>AVg</i></b>", text.BoldItalic}, {"<i><b>AVg</b></i>", text.BoldItalic},
		{"<b><b>AVg</b></b>", text.Bold},
	} {
		f, err := text.DefaultFontForStyle(tc.style)
		if err != nil {
			t.Fatal(err)
		}
		opt := text.DefaultMarkupOptions()
		got, gm, err := text.LayoutMarkup(tc.source, opt)
		if err != nil {
			t.Fatal(err)
		}
		want, wm, err := text.Layout("AVg", f, opt.Options)
		if err != nil || gm != wm || !bytes.Equal(textGeometry(got), textGeometry(want)) {
			t.Fatal("markup did not select real face outlines", tc.source, err)
		}
	}
	// Closing tags restore both style bits, independently of nesting order.
	got, err := text.NewMarkup("A<b>V<i>g</i>V</b><i>x<b>A</b>x</i>A", text.DefaultMarkupOptions())
	if err != nil {
		t.Fatal(err)
	}
	styles := []text.FontStyle{text.Regular, text.Bold, text.BoldItalic, text.Bold, text.Italic, text.BoldItalic, text.Italic, text.Regular}
	x := 0.
	for i, r := range "AVgVxAxA" {
		f, _ := text.DefaultFontForStyle(styles[i])
		one, m, err := text.Layout(string(r), f, text.DefaultOptions())
		if err != nil {
			t.Fatal(err)
		}
		one.Shift(geom.V(x, 0))
		a, b := got.Children[i].Bounds(), one.Children[0].Bounds()
		if a.Min.Sub(b.Min).Norm()+a.Max.Sub(b.Max).Norm() > 1e-12 {
			t.Fatal("nested font scope/advance differs", i)
		}
		x += m.Width
	}
}

func TestMarkupColorScopeAndContinuousKerning(t *testing.T) {
	fonts := fallbackFonts(t)
	opt := text.DefaultMarkupOptions()
	opt.Size = 1
	opt.Fonts = &text.MarkupFonts{Regular: fonts[0]}
	source := `<span foreground="#ff0000">A<span foreground="#00FF00">V</span>A</span>V`
	got, gm, err := text.LayoutMarkup(source, opt)
	if err != nil {
		t.Fatal(err)
	}
	want, wm, err := text.Layout("AVAV", fonts[0], opt.Options)
	if err != nil || gm != wm {
		t.Fatal("color boundaries lost same-font kerning", gm, wm, err)
	}
	for i, paint := range []gomath.Color{gomath.RGB(1, 0, 0), gomath.RGB(0, 1, 0), gomath.RGB(1, 0, 0), opt.Color} {
		want.Children[i].Style.Fill = paint
	}
	if !bytes.Equal(textGeometry(got), textGeometry(want)) {
		t.Fatal("color scope or kerning changed outlines")
	}
	// An empty tag does not invent a glyph or a font boundary between A and V.
	for _, source := range []string{`A<span foreground="#000000"/>V`, `A<b/>V`} {
		opt.Fonts.Bold = fonts[1]
		_, m, err := text.LayoutMarkup(source, opt)
		if err != nil {
			t.Fatal(err)
		}
		_, wm, _ := text.Layout("AV", fonts[0], opt.Options)
		closeMetric(t, m.Width, wm.Width)
	}
	opt.Fonts = &text.MarkupFonts{Regular: fonts[0], Bold: fonts[1]}
	_, m, err := text.LayoutMarkup("AV<b>AV</b>AV", opt)
	if err != nil {
		t.Fatal(err)
	}
	_, regular, _ := text.Layout("AV", fonts[0], opt.Options)
	_, bold, _ := text.Layout("AV", fonts[1], opt.Options)
	closeMetric(t, m.Width, 2*regular.Width+bold.Width)
	// Explicit aliases use the same real face, so same-pointer kerning survives.
	opt.Fonts.Bold = fonts[0]
	_, m, err = text.LayoutMarkup("A<b>V</b>", opt)
	if err != nil {
		t.Fatal(err)
	}
	closeMetric(t, m.Width, regular.Width)
}

func TestMarkupXMLTextWhitespaceAndBaseline(t *testing.T) {
	opt := text.DefaultMarkupOptions()
	for _, source := range []string{
		`&lt;&amp;&gt;&quot;&apos; &#65;&#x56;`,
		" A\tV \n\n g \n",
		"A\r\nV\rg", "A&#13;V", "&lt;b&gt;AV&lt;/b&gt;", "αβ&#x3b3;", "&amp;nbsp;", "]]&gt;",
	} {
		plain := source
		switch source {
		case `&lt;&amp;&gt;&quot;&apos; &#65;&#x56;`:
			plain = "<&>\"' AV"
		case "A\r\nV\rg":
			plain = "A\nV\ng"
		case "A&#13;V":
			// XML preserves numeric carriage-return entities as characters. It
			// has no special newline behavior in ordinary text, either.
			plain = "A\rV"
			opt.AllowMissing = true
		case "&lt;b&gt;AV&lt;/b&gt;":
			plain = "<b>AV</b>"
		case "αβ&#x3b3;":
			plain = "αβγ"
		case "&amp;nbsp;":
			plain = "&nbsp;"
		case "]]&gt;":
			plain = "]]>"
		}
		got, gm, err := text.LayoutMarkup(source, opt)
		if err != nil {
			t.Fatal(err)
		}
		want, wm, err := text.Layout(plain, text.DefaultFont(), opt.Options)
		if err != nil || gm != wm || !bytes.Equal(textGeometry(got), textGeometry(want)) || got.Name != plain {
			t.Fatal("decoded text/baseline/whitespace changed", source, err)
		}
	}
	fonts := fallbackFonts(t)
	opt.Fonts = &text.MarkupFonts{Regular: fonts[0], Bold: fonts[1]}
	for _, value := range []string{"", "A", "<b/>", "\n\n", "A\n<b>g</b>\n"} {
		_, m, err := text.LayoutMarkup(value, opt)
		if err != nil {
			t.Fatal(err)
		}
		closeMetric(t, m.Ascent, 2300./2048*opt.Size)
		closeMetric(t, m.Descent, 700./2048*opt.Size)
		closeMetric(t, m.Height, m.Ascent+m.Descent+float64(m.Lines-1)*opt.Size*opt.LineHeight)
	}
	// A line break moves the baseline and resets adjacency, but does not close
	// font or color scopes. Closing tags after the break still restores them.
	opt = text.DefaultMarkupOptions()
	got, gm, err := text.LayoutMarkup("<span foreground=\"#20c080\"><b>A\nV</b></span>g", opt)
	if err != nil {
		t.Fatal(err)
	}
	bold, _ := text.DefaultFontForStyle(text.Bold)
	want, wm, err := text.Layout("A\nV", bold, opt.Options)
	if err != nil {
		t.Fatal(err)
	}
	for i := range want.Children {
		paint, _ := gomath.Hex("20c080")
		want.Children[i].Style.Fill = paint
		if got.Children[i].Bounds() != want.Children[i].Bounds() || got.Children[i].Style.Fill != paint {
			t.Fatal("newline incorrectly ended a markup scope", i)
		}
	}
	g, _, err := text.Layout("g", text.DefaultFont(), opt.Options)
	if err != nil {
		t.Fatal(err)
	}
	_, v, err := text.Layout("V", bold, opt.Options)
	if err != nil {
		t.Fatal(err)
	}
	g.Shift(geom.V(v.Width, -opt.Size*opt.LineHeight))
	a, b := got.Children[2].Bounds(), g.Children[0].Bounds()
	if a.Min.Sub(b.Min).Norm()+a.Max.Sub(b.Max).Norm() > 1e-12 || got.Children[2].Style.Fill != opt.Color || gm.Lines != wm.Lines {
		t.Fatal("scope failed to restore on a later line")
	}
}

func TestMarkupCustomFontsMissingAndMalformedGlyphs(t *testing.T) {
	fonts := fallbackFonts(t)
	opt := text.DefaultMarkupOptions()
	opt.Fonts = &text.MarkupFonts{Regular: fonts[0]}
	for _, source := range []string{"<b>A</b>", "<i/>", "<b><i>A</i></b>"} {
		if _, _, err := text.LayoutMarkup(source, opt); err == nil || !strings.Contains(err.Error(), "variant") {
			t.Fatal("missing real style face accepted", source, err)
		}
	}
	opt.Fonts = &text.MarkupFonts{Regular: fonts[0], Bold: fonts[1], Italic: fonts[0]}
	if _, err := text.NewMarkup("<b><i/></b>", opt); err == nil || !strings.Contains(err.Error(), "bold-italic") {
		t.Fatal("missing combined style accepted", err)
	}
	if _, err := text.NewMarkup("β", opt); err == nil || !strings.Contains(err.Error(), "U+03B2") {
		t.Fatal("unrequested style silently used as font fallback", err)
	}
	if _, err := text.NewMarkup("<b>β</b>", opt); err != nil {
		t.Fatal("configured style glyph rejected", err)
	}
	opt.AllowMissing = true
	got, err := text.NewMarkup("β", opt)
	if err != nil {
		t.Fatal(err)
	}
	want, err := text.New("β", fonts[0], opt.Options)
	if err != nil || !bytes.Equal(textGeometry(got), textGeometry(want)) {
		t.Fatal("AllowMissing did not use the selected style's .notdef", err)
	}
	data, err := os.ReadFile(filepath.Join(fallbackDir(), "broken.ttf"))
	if err != nil {
		t.Fatal(err)
	}
	broken, err := text.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	opt.Fonts.Bold = broken
	if _, err := text.NewMarkup("<b>A</b>", opt); err == nil || !strings.Contains(err.Error(), "load markup glyph") {
		t.Fatal("broken style outline silently replaced", err)
	}
}

func TestMarkupRejectsUnknownOrMalformedXML(t *testing.T) {
	for _, source := range []string{
		"<u>A</u>", "<strong>A</strong>", "<br/>", "<markup>A</markup>", "<B>A</B>",
		`<b color="#ff0000">A</b>`, `<i weight="bold">A</i>`, `<span/>`,
		`<span foreground="#ff0000" foreground="#00ff00">A</span>`,
		`<span foreground="#ff0000" size="20">A</span>`, `<span color="#ff0000">A</span>`,
		`<span foreground="red">A</span>`, `<span foreground="#fff">A</span>`,
		`<span foreground="#ff000080">A</span>`, `<span foreground=" #ff0000">A</span>`,
		`<span foreground="#gg0000">A</span>`, `<span foreground="＃ff0000">A</span>`,
		`<span foreground=#ff0000>A</span>`, `<b xmlns="urn:test">A</b>`,
		`<x:b>A</x:b>`, `<span x:foreground="#ff0000">A</span>`,
		"<b><i>A</b></i>", "<b>A", "A</b>", "<b>A</i>", "<b / >", "<", "A < B",
		"&unknown;", "&amp", "&", "&#0;", "&#xD800;", "&#x110000;", "&#-1;", "&#xZZ;", "&#xFFFF;", "&#xFFFE;", "&#;", "&#x;", "&#x+41;", "&#999999999999999999999999999999;", "]]>", "\uFFFE", "\uFFFF",
		"\x00", "\x01", string([]byte{0xff}), "<!--comment-->", "<![CDATA[A]]>",
		"<!DOCTYPE x>", `<!DOCTYPE x [<!ENTITY y "A">]>&y;`, `<?xml version="1.0"?>A`, "<?x?>",
	} {
		if o, m, err := text.LayoutMarkup(source, text.DefaultMarkupOptions()); err == nil || o != nil || m != (text.Metrics{}) {
			t.Fatalf("unsupported markup accepted: %q: %v", source, err)
		}
	}
}

func TestMarkupGlobalBudgetsAndValidation(t *testing.T) {
	opt := text.DefaultMarkupOptions()
	// Inclusive byte boundary with legal whitespace inside an empty XML element.
	maxBytes := "<b" + strings.Repeat(" ", text.MaxMarkupBytes-len("<b></b>")) + "></b>"
	for _, source := range []string{
		maxBytes, strings.Repeat("<b>", text.MaxMarkupDepth) + "A" + strings.Repeat("</b>", text.MaxMarkupDepth),
		strings.Repeat("<b/>", text.MaxMarkupElements), strings.Repeat("&#32;", text.MaxRunes),
	} {
		if _, err := text.NewMarkup(source, opt); err != nil {
			t.Fatal("inclusive markup bound rejected", len(source), err)
		}
	}
	for _, tc := range []struct{ source, message string }{
		{maxBytes + " ", "input bytes"},
		{strings.Repeat("<b>", text.MaxMarkupDepth+1) + "A" + strings.Repeat("</b>", text.MaxMarkupDepth+1), "nesting"},
		{strings.Repeat("<b/>", text.MaxMarkupElements+1), "elements"},
		{strings.Repeat("&#32;", text.MaxRunes+1), "decoded runes"},
		{strings.Repeat("g", text.MaxRunes), "glyph segments"},
	} {
		if o, m, err := text.LayoutMarkup(tc.source, opt); err == nil || o != nil || m != (text.Metrics{}) || !strings.Contains(err.Error(), tc.message) {
			t.Fatal("markup budget not enforced atomically", tc.message, err)
		}
	}
	// Each style's contribution is under the segment cap, but the total is over.
	fonts := fallbackFonts(t)
	opt.Fonts = &text.MarkupFonts{Regular: fonts[0], Bold: fonts[1]}
	const primarySegments, secondarySegments = 13, 24 // pinned sfnt fixture operations
	first := (3 * text.MaxGlyphSegments / 10) / primarySegments
	second := (8 * text.MaxGlyphSegments / 10) / secondarySegments
	combined := strings.Repeat("A", first) + "<b>" + strings.Repeat("g", second) + "</b>"
	if _, err := text.NewMarkup(combined, opt); err == nil || !strings.Contains(err.Error(), "glyph segments") {
		t.Fatal("segment budget was reset across style runs", err)
	}
	for _, config := range []*text.MarkupFonts{{}, {Regular: &text.Font{}}, {Regular: fonts[0], Italic: &text.Font{}}} {
		bad := text.DefaultMarkupOptions()
		bad.Fonts = config
		if _, err := text.NewMarkup("", bad); err == nil {
			t.Fatal("invalid font configuration accepted")
		}
	}
	for _, n := range []float64{0, -1, math.NaN(), math.Inf(1), 1e7, math.SmallestNonzeroFloat64} {
		bad := text.DefaultMarkupOptions()
		bad.Size = n
		if _, err := text.NewMarkup("", bad); err == nil {
			t.Fatal("invalid size accepted", n)
		}
		bad = text.DefaultMarkupOptions()
		bad.Size, bad.LineHeight = .01, n
		if _, err := text.NewMarkup("", bad); err == nil {
			t.Fatal("invalid line height accepted", n)
		}
	}
	bad := text.DefaultMarkupOptions()
	bad.Color.R = math.NaN()
	if _, err := text.NewMarkup(`<span foreground="#ffffff">A</span>`, bad); err == nil {
		t.Fatal("invalid base color silently hidden by span")
	}
}

func TestMarkupConcurrentLayoutRenderingAndOwnership(t *testing.T) {
	opt := text.DefaultMarkupOptions()
	opt.Fonts = &text.MarkupFonts{}
	opt.Fonts.Regular, _ = text.DefaultFontForStyle(text.Regular)
	opt.Fonts.Bold, _ = text.DefaultFontForStyle(text.Bold)
	opt.Fonts.Italic, _ = text.DefaultFontForStyle(text.Italic)
	opt.Fonts.BoldItalic, _ = text.DefaultFontForStyle(text.BoldItalic)
	const source = `A<b>V<i>g</i></b><span foreground="#00ff00">β</span>` + "\nx &amp; g"
	base, err := text.NewMarkup(source, opt)
	if err != nil {
		t.Fatal(err)
	}
	base.Shift(geom.V(-1, 0))
	before := textGeometry(base)
	c := render.DefaultConfig()
	c.Width, c.Height, c.Workers, c.FrameWidth, c.Background = 240, 160, 1, 4, gomath.Black
	r, _ := render.NewRasterizer(c)
	want, err := r.Render(&gomath.Frame{Objects: []*gomath.Object{base}})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := render.NewRasterizer(c)
			if err != nil {
				t.Error(err)
				return
			}
			for range 4 {
				o, err := text.NewMarkup(source, opt)
				if err != nil {
					t.Error(err)
					return
				}
				o.Shift(geom.V(-1, 0))
				if !bytes.Equal(before, textGeometry(o)) {
					t.Error("concurrent markup geometry differs")
				}
				for _, object := range []*gomath.Object{base, o.Copy()} {
					got, err := r.Render(&gomath.Frame{Objects: []*gomath.Object{object}})
					if err != nil || got == nil || !bytes.Equal(want.Pix, got.Pix) {
						t.Error("concurrent markup pixels differ", err)
					}
				}
				o.Children[0].Path.Contours[0].Curves[0].P0.X += 10
			}
		}()
	}
	wg.Wait()
	opt.Fonts.Regular = nil
	if !bytes.Equal(before, textGeometry(base)) {
		t.Fatal("output retained mutable configuration or glyph paths")
	}
}

func BenchmarkMarkupLayout(b *testing.B) {
	opt := text.DefaultMarkupOptions()
	const source = `A<b>V<i>g</i></b><span foreground="#20c080">β&amp;x</span>` + "\n" + `<i>g</i> AV`
	if _, err := text.NewMarkup(source, opt); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, _, err := text.LayoutMarkup(source, opt); err != nil {
			b.Fatal(err)
		}
	}
}
