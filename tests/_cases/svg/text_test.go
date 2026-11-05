package svg_test

import (
	"fmt"
	"gomath"
	"gomath/geom"
	"gomath/path"
	"gomath/svg"
	"gomath/text"
	"math"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func textRead(t testing.TB, body string, resolver svg.FontResolver) *gomath.Object {
	t.Helper()
	o, err := svg.Read(strings.NewReader(`<svg viewBox="0 0 100 100">`+body+`</svg>`), svg.Options{Width: 100, FontResolver: resolver})
	if err != nil {
		t.Fatal(err)
	}
	if err := o.Validate(); err != nil {
		t.Fatal(err)
	}
	return o
}

func textRun(t testing.TB, value string, style text.FontStyle, size float64) (*gomath.Object, text.Metrics) {
	t.Helper()
	f, err := text.DefaultFontForStyle(style)
	if err != nil {
		t.Fatal(err)
	}
	o, m, err := text.Layout(value, f, text.Options{Size: size, LineHeight: 1, Color: gomath.Black})
	if err != nil {
		t.Fatal(err)
	}
	return o, m
}

func textWorld(x, y float64) geom.Mat4 {
	return geom.Translation(geom.V(-50, 50)).Mul(geom.Scaling(1, -1, 1)).Mul(geom.Translation(geom.V(x, y))).Mul(geom.Scaling(1, -1, 1))
}

func textPathEqual(t testing.TB, actual, expected path.Path) {
	t.Helper()
	if len(actual.Contours) != len(expected.Contours) {
		t.Fatalf("contours %d want %d", len(actual.Contours), len(expected.Contours))
	}
	for i, a := range actual.Contours {
		b := expected.Contours[i]
		if a.Closed != b.Closed || len(a.Curves) != len(b.Curves) || a.Start.Sub(b.Start).Norm() > 1e-8 {
			t.Fatalf("contour %d changed", i)
		}
		for j, c := range a.Curves {
			d := b.Curves[j]
			if c.P0.Sub(d.P0).Norm() > 1e-8 || c.P1.Sub(d.P1).Norm() > 1e-8 || c.P2.Sub(d.P2).Norm() > 1e-8 || c.P3.Sub(d.P3).Norm() > 1e-8 {
				t.Fatalf("contour %d curve %d changed", i, j)
			}
		}
	}
}

func TestTextRealStylesBaselineAndUnicode(t *testing.T) {
	for _, tc := range []struct {
		style text.FontStyle
		attrs string
	}{
		{text.Regular, `font-weight="400" font-style="normal"`},
		{text.Bold, `font-weight="bold"`},
		{text.Italic, `font-style="italic"`},
		{text.BoldItalic, `font-style="italic" font-weight="700"`},
	} {
		t.Run(tc.attrs, func(t *testing.T) {
			value := "AV éΩЖ∑"
			root := textRead(t, `<text x="20" y="50" dx="2pt" dy="-3" font-family="'Go'" font-size="12pt" `+tc.attrs+`>`+value+`</text>`, nil)
			expected, _ := textRun(t, value, tc.style, 16)
			got := instanceShapes(root)
			if len(got) != len(expected.Children) {
				t.Fatal("lost real glyphs", len(got))
			}
			for i, g := range got {
				textPathEqual(t, g.Path, expected.Children[i].Path.Transform(textWorld(20+8.0/3, 47)))
				if g.Style.Fill != gomath.Black || g.Style.Stroke.A != 0 {
					t.Fatal("SVG default paint lost")
				}
			}
		})
	}
}

func TestTextWhitespaceAndAnchors(t *testing.T) {
	for _, tc := range []struct{ attrs, source, normalized string }{
		{"", "  A\t  V\n  ", "A V"},
		{`xml:space="preserve"`, "  A\tV\n ", "  A V  "},
		{"", ` A<!--split--><![CDATA[V]]> `, "AV"},
		{"", ` A<tspan>  V </tspan> Z `, "A V Z"},
		{`xml:space="preserve"`, ` A<tspan xml:space="default">  V </tspan>Z `, " A V Z "},
	} {
		for _, anchor := range []string{"start", "middle", "end"} {
			root := textRead(t, `<text x="50" y="50" font-size="10" text-anchor="`+anchor+`" `+tc.attrs+`>`+tc.source+`</text>`, nil)
			expected, m := textRun(t, tc.normalized, text.Regular, 10)
			x := 50.0
			if anchor == "middle" {
				x -= m.Width / 2
			} else if anchor == "end" {
				x -= m.Width
			}
			got := instanceShapes(root)
			if len(got) != len(expected.Children) {
				t.Fatalf("%q: got %d glyphs", tc.source, len(got))
			}
			for i, g := range got {
				textPathEqual(t, g.Path, expected.Children[i].Path.Transform(textWorld(x, 50)))
			}
		}
	}
}

func TestTextSpanPositionsChunkAnchorsAndEmptySpans(t *testing.T) {
	root := textRead(t, `<text x="10" y="30" font-size="10">A<tspan dx="2" dy="3" fill="red">V</tspan>Z<tspan x="70" y="60" text-anchor="end" font-weight="bold">AB</tspan><tspan x="99"/><tspan>Q</tspan></text>`, nil)
	got := instanceShapes(root)
	a, am := textRun(t, "A", text.Regular, 10)
	vz, vm := textRun(t, "VZ", text.Regular, 10)
	ab, bm := textRun(t, "AB", text.Bold, 10)
	q, qm := textRun(t, "Q", text.Regular, 10)
	if len(got) != 6 {
		t.Fatal("span glyph count", len(got))
	}
	textPathEqual(t, got[0].Path, a.Children[0].Path.Transform(textWorld(10, 30)))
	for i := range 2 {
		textPathEqual(t, got[i+1].Path, vz.Children[i].Path.Transform(textWorld(12+am.Width, 33)))
		textPathEqual(t, got[i+3].Path, ab.Children[i].Path.Transform(textWorld(70-bm.Width-qm.Width, 60)))
	}
	textPathEqual(t, got[5].Path, q.Children[0].Path.Transform(textWorld(70-qm.Width, 60)))
	if got[1].Style.Fill != gomath.RGB(1, 0, 0) || got[2].Style.Fill != gomath.Black || vm.Width <= 0 {
		t.Fatal("span paint leaked")
	}
	// Inner scalar positions override the ancestor value on the same first
	// addressable character; an empty span consumes no positions.
	root = textRead(t, `<text x="2" y="30" dx="10" font-size="10"><tspan x="20" dx="3">A</tspan>B</text>`, nil)
	expected, _ := textRun(t, "AB", text.Regular, 10)
	for i, g := range instanceShapes(root) {
		textPathEqual(t, g.Path, expected.Children[i].Path.Transform(textWorld(23, 30)))
	}
}

func TestTextSpanKerningWithCallerFontAndRelativePositions(t *testing.T) {
	data, err := os.ReadFile("../tests/data/svg/text-kern.ttf")
	if err != nil {
		t.Fatal(err)
	}
	f, err := text.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	clear(data) // parsed fonts own their bytes, not the caller's source buffer
	resolver := func(family string, style text.FontStyle) (*text.Font, error) {
		if family != "Kern Fixture" || style != text.Regular {
			return nil, fmt.Errorf("unsupported exact fixture face")
		}
		return f, nil
	}
	expected, pair, err := text.Layout("AV", f, text.Options{Size: 16, LineHeight: 1, Color: gomath.Black})
	if err != nil {
		t.Fatal(err)
	}
	_, single, _ := text.Layout("A", f, text.Options{Size: 16, LineHeight: 1, Color: gomath.Black})
	_, second, _ := text.Layout("V", f, text.Options{Size: 16, LineHeight: 1, Color: gomath.Black})
	if math.Abs(pair.Width-single.Width-second.Width+1.5) > 1e-9 {
		t.Fatal("synthetic -192/2048 em kern fixture was not used")
	}
	for _, span := range []string{`<tspan fill="red">V</tspan>`, `<tspan dx="0" fill="red">V</tspan>`, `<tspan dx="3" dy="2" fill="red">V</tspan>`} {
		root := textRead(t, `<text x="20" y="50" font-size="16" font-family="Kern Fixture">A`+span+`</text>`, resolver)
		glyphs := instanceShapes(root)
		textPathEqual(t, glyphs[0].Path, expected.Children[0].Path.Transform(textWorld(20, 50)))
		matrix := textWorld(20, 50)
		if strings.Contains(span, `dx="3"`) {
			matrix = textWorld(23, 52)
		}
		textPathEqual(t, glyphs[1].Path, expected.Children[1].Path.Transform(matrix))
	}
}

func TestTextMixedSizesCSSWideKeywordsAndSymbol(t *testing.T) {
	root := textRead(t, `<style>text{font-size:10;font-weight:bold;font-style:italic;text-anchor:end} #small{font-size:5;font-style:normal;font-weight:initial} .inherit{font-size:inherit;font-weight:unset;font-style:initial}</style><text x="80" y="30">A<tspan id="small">V</tspan><tspan class="inherit">Z</tspan></text>`, nil)
	a, am := textRun(t, "A", text.BoldItalic, 10)
	v, vm := textRun(t, "V", text.Regular, 5)
	z, zm := textRun(t, "Z", text.Bold, 10)
	got := instanceShapes(root)
	x := 80 - am.Width - vm.Width - zm.Width
	textPathEqual(t, got[0].Path, a.Children[0].Path.Transform(textWorld(x, 30)))
	textPathEqual(t, got[1].Path, v.Children[0].Path.Transform(textWorld(x+am.Width, 30)))
	textPathEqual(t, got[2].Path, z.Children[0].Path.Transform(textWorld(x+am.Width+vm.Width, 30)))
	root = textRead(t, `<defs><symbol id="s" viewBox="0 0 20 20"><text x="2" y="12" font-size="8">A</text></symbol></defs><use href="#s" x="10" y="20" width="40" height="40"/>`, nil)
	a, _ = textRun(t, "A", text.Regular, 8)
	world := geom.Translation(geom.V(-50, 50)).Mul(geom.Scaling(1, -1, 1))
	m := world.Mul(geom.Translation(geom.V(10, 20))).Mul(geom.Scaling(2, 2, 1)).Mul(geom.Translation(geom.V(2, 12))).Mul(geom.Scaling(1, -1, 1))
	textPathEqual(t, instanceShapes(root)[0].Path, a.Children[0].Path.Transform(m))
}

func TestTextBackwardRelativeOffsetsUseAdvanceExtents(t *testing.T) {
	a, am := textRun(t, "A", text.Regular, 10)
	v, vm := textRun(t, "V", text.Regular, 10)
	second := 50 + am.Width - 30
	left, right := math.Min(50, second), math.Max(50+am.Width, second+vm.Width)
	for _, anchor := range []string{"start", "middle", "end"} {
		root := textRead(t, `<text x="50" y="30" dx="2" font-size="10" text-anchor="`+anchor+`">A<tspan dx="-30" dy="2">V</tspan></text>`, nil)
		shift := 50 - left
		if anchor == "middle" {
			shift = 50 - (left+right)/2
		} else if anchor == "end" {
			shift = 50 - right
		}
		glyphs := instanceShapes(root)
		textPathEqual(t, glyphs[0].Path, a.Children[0].Path.Transform(textWorld(52+shift, 30)))
		textPathEqual(t, glyphs[1].Path, v.Children[0].Path.Transform(textWorld(second+2+shift, 32)))
	}
}

func TestTextCSSUseTransformsPaintAndOwnership(t *testing.T) {
	source := `<style>.definition text{fill:yellow} use text{fill:cyan} text>tspan{font-style:italic;fill:currentColor}.host{font-weight:bold;fill:red;color:blue} #copy{font-size:12}</style><defs class="definition"><text id="label" y="15">A<tspan id="i" stroke="green" stroke-width=".5">Ω</tspan></text></defs><g class="host"><use href="#label" x="10" y="5" transform="translate(2,3) scale(2)" opacity=".5"/><use id="copy" href="#label" x="40" y="50" font-weight="normal"/></g>`
	root := textRead(t, source, nil)
	got := instanceShapes(root)
	if len(got) != 4 || got[0].Style.Fill != gomath.RGB(1, 0, 0) || got[1].Style.Fill != gomath.RGB(0, 0, 1) || got[1].Style.StrokeWidth != 1 {
		t.Fatal("use CSS inheritance or paint failed", got)
	}
	a, am := textRun(t, "A", text.Bold, 16)
	b, _ := textRun(t, "Ω", text.BoldItalic, 16)
	world := geom.Translation(geom.V(-50, 50)).Mul(geom.Scaling(1, -1, 1))
	instance := geom.Translation(geom.V(2, 3)).Mul(geom.Scaling(2, 2, 1)).Mul(geom.Translation(geom.V(10, 5)))
	textPathEqual(t, got[0].Path, a.Children[0].Path.Transform(world.Mul(instance).Mul(geom.Translation(geom.V(0, 15))).Mul(geom.Scaling(1, -1, 1))))
	textPathEqual(t, got[1].Path, b.Children[0].Path.Transform(world.Mul(instance).Mul(geom.Translation(geom.V(am.Width, 15))).Mul(geom.Scaling(1, -1, 1))))
	ids := map[uint64]bool{}
	root.Walk(func(o *gomath.Object) {
		if ids[o.ID] {
			t.Fatal("shared ID", o.ID)
		}
		ids[o.ID] = true
	})
	copyBefore := got[2].Path.Clone()
	got[0].Path.Contours[0].Curves[0].P0.X += 500
	if !reflect.DeepEqual(got[2].Path, copyBefore) {
		t.Fatal("use instances share mutable glyph geometry")
	}
	// Fill-only outlines permit the existing real affine transform path.
	root = textRead(t, `<text x="20" y="30" transform="matrix(1 .2 .4 1 3 5)" font-size="10">A</text>`, nil)
	expected, _ := textRun(t, "A", text.Regular, 10)
	matrix := geom.Identity()
	matrix[1], matrix[4], matrix[3], matrix[7] = .4, .2, 3, 5
	textPathEqual(t, instanceShapes(root)[0].Path, expected.Children[0].Path.Transform(world.Mul(matrix).Mul(geom.Translation(geom.V(20, 30))).Mul(geom.Scaling(1, -1, 1))))
}

func TestTextResolverCachingIsolationAndFailure(t *testing.T) {
	for i := range 8 {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			calls := map[text.FontStyle]int{}
			resolver := func(family string, style text.FontStyle) (*text.Font, error) {
				if family != "My Font" {
					return nil, fmt.Errorf("unexpected family %q", family)
				}
				calls[style]++
				return text.DefaultFontForStyle(style)
			}
			root := textRead(t, `<g font-family="My Font"><text>AB<tspan font-weight="bold">CD</tspan>EF</text><text font-weight="bold">GH</text></g>`, resolver)
			if len(instanceShapes(root)) != 8 || calls[text.Regular] != 1 || calls[text.Bold] != 1 {
				t.Fatal("resolver was not cached per import", calls)
			}
		})
	}
	for _, resolver := range []svg.FontResolver{
		func(string, text.FontStyle) (*text.Font, error) { return nil, fmt.Errorf("font denied") },
		func(string, text.FontStyle) (*text.Font, error) { return nil, nil },
		func(string, text.FontStyle) (*text.Font, error) { return &text.Font{}, nil },
	} {
		if o, err := svg.Read(strings.NewReader(`<svg><text>A</text></svg>`), svg.Options{Width: 100, FontResolver: resolver}); err == nil || o != nil {
			t.Fatal("resolver failure fell back to another font")
		}
	}
	// A shared resolver may safely serve concurrent imports with parsed fonts.
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resolver := func(_ string, s text.FontStyle) (*text.Font, error) { return text.DefaultFontForStyle(s) }
			_, err := svg.Read(strings.NewReader(`<svg><text font-family="Custom">éΩЖ</text></svg>`), svg.Options{Width: 4, FontResolver: resolver})
			if err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
}

func TestTextUnsupportedSemanticsAreErrors(t *testing.T) {
	for _, body := range []string{
		`<text x="1 2">A</text>`, `<text x="10%">A</text>`, `<text dx="1,2">A</text>`, `<text y="NaN">A</text>`,
		`<text rotate="0">A</text>`, `<text textLength="20">A</text>`, `<text lengthAdjust="spacing">A</text>`, `<text><textPath href="#x">A</textPath></text>`,
		`<text font-size="0">A</text>`, `<text font-size="1em">A</text>`, `<text font-size="1e7">A</text>`,
		`<text font-style="oblique">A</text>`, `<text font-weight="600">A</text>`, `<text font-family="sans-serif">A</text>`, `<text font-family="Go,serif">A</text>`,
		`<text text-anchor="left">A</text>`, `<text xml:space="bogus">A</text>`, `<text xml:lang="en">A</text>`,
		`<text><tspan transform="scale(2)">A</tspan></text>`, `<tspan>A</tspan>`, `<text><use href="#a"/></text>`,
		`<text letter-spacing="1">A</text>`, `<g writing-mode="vertical-rl"><text>A</text></g>`, `<g font-variant="small-caps"><text>A</text></g>`,
		`<text direction="rtl">A</text>`, `<text style="font-feature-settings:'liga'">A</text>`, `<text style="white-space:pre">A</text>`,
		`<text text-rendering="optimizeLegibility">A</text>`,
		`<text>Á</text>`, `<text>مرحبا</text>`, `<text>שלום</text>`, `<text>हिन्दी</text>`, `<text>中文</text>`, `<text>A&#x200d;B</text>`, `<text>🦄</text>`,
		`<text transform="scale(2,1)" stroke="red">A</text>`, `<defs><text rotate="2">A</text></defs>`, `<defs><text>Á</text></defs>`,
		`<defs><symbol id="s" viewBox="0 0 10 10"><text font-size="20">W</text></symbol></defs><use href="#s" width="10" height="10"/>`,
		`<text x="1e308" dx="1e308">A</text>`,
	} {
		t.Run(body, func(t *testing.T) {
			if o, err := svg.Read(strings.NewReader(`<svg>`+body+`</svg>`), svg.DefaultOptions()); err == nil || o != nil {
				t.Fatalf("unsupported semantics accepted: %s", body)
			}
		})
	}
}

func BenchmarkReadTextInstances(b *testing.B) {
	var doc strings.Builder
	doc.WriteString(`<svg viewBox="0 0 640 320"><defs><text id="label" font-size="12">Go<tspan font-weight="bold">Math</tspan> Ω</text></defs>`)
	for i := range 32 {
		fmt.Fprintf(&doc, `<use href="#label" x="%d" y="%d"/>`, (i%8)*80, 20+(i/8)*80)
	}
	doc.WriteString(`</svg>`)
	source := doc.String()
	b.ReportAllocs()
	b.SetBytes(int64(len(source)))
	b.ResetTimer()
	for range b.N {
		if _, err := svg.Read(strings.NewReader(source), svg.DefaultOptions()); err != nil {
			b.Fatal(err)
		}
	}
}

// Keep a tight tolerance in outline comparisons independent of visual raster
// tolerances, including whitespace-only chunks with finite empty bounds.
func TestTextEmptyAndVisibilityLayout(t *testing.T) {
	for _, body := range []string{`<text/>`, "<text> \n </text>", `<text xml:space="preserve">   </text>`} {
		if len(instanceShapes(textRead(t, body, nil))) != 0 {
			t.Fatal("blank text manufactured outlines")
		}
	}
	a, am := textRun(t, "A", text.Regular, 10)
	root := textRead(t, `<text y="30" font-size="10" visibility="hidden">A<tspan visibility="visible">A</tspan><tspan display="none">A</tspan>A</text>`, nil)
	got := instanceShapes(root)
	if len(got) != 3 || got[0].Style.Opacity != 0 || got[1].Style.Opacity != 1 || got[2].Style.Opacity != 0 {
		t.Fatal("visibility/display handling failed")
	}
	textPathEqual(t, got[1].Path, a.Children[0].Path.Transform(textWorld(am.Width, 30)))
	if math.Abs(got[2].Bounds().Min.X-got[0].Bounds().Min.X-2*am.Width) > 1e-8 {
		t.Fatal("display:none changed text advance")
	}
}
