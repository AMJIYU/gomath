package svg_test

import (
	"fmt"
	"gomath"
	"image/color"
	"image/png"
	"math"
	"os"
	"strings"
	"testing"

	"gomath/svg"
)

func cssObject(t *testing.T, root *gomath.Object, name string) *gomath.Object {
	t.Helper()
	var found *gomath.Object
	root.Walk(func(o *gomath.Object) {
		if o.Name == name {
			found = o
		}
	})
	if found == nil {
		t.Fatalf("missing object %q", name)
	}
	return found
}

func cssFill(t *testing.T, root *gomath.Object, name string, want gomath.Color) {
	t.Helper()
	if got := cssObject(t, root, name).Style.Fill; got != want {
		t.Errorf("%s fill = %v, want %v", name, got, want)
	}
}

func TestCSSCascadeSpecificityOrderAndPresentation(t *testing.T) {
	root := readInstances(t, `<svg viewBox="0 0 100 100">
<style><![CDATA[
 /* universal is an author rule and beats a presentation attribute */
 * {fill:gray}
 rect {fill:blue}
 .tone {fill:red}
 #identified {fill:green}
 rect.tone {fill:yellow}
 .tone {fill:orange}
 #list, .list {fill:cyan}
 .list {fill:magenta}
 .repeat {fill:red; fill:blue}
 #many {fill:white}
 .a.a.a.a.a.a.a.a.a.a.a {fill:red}
]]></style>
<rect id="type" fill="white" width="1" height="1"/>
<rect id="class" class="tone" width="1" height="1"/>
<rect id="identified" class="tone" width="1" height="1"/>
<rect id="inline" class="tone" style="fill:white" width="1" height="1"/>
<rect id="list" class="list" width="1" height="1"/>
<rect id="list-class" class="list" width="1" height="1"/>
<rect id="repeated" class="repeat" width="1" height="1"/>
<circle id="universal" r="1" fill="white"/>
<rect id="many" class="a" width="1" height="1"/>
<style>.repeat {fill:green}</style>
</svg>`)
	for name, want := range map[string]gomath.Color{
		"type": gomath.RGB(0, 0, 1), "class": gomath.RGB(1, 1, 0),
		"identified": gomath.RGB(0, .5, 0), "inline": gomath.White,
		"list": gomath.RGB(0, 1, 1), "list-class": gomath.RGB(1, 0, 1),
		"repeated": gomath.RGB(0, .5, 0), "universal": gomath.RGB(.5, .5, .5), "many": gomath.White,
	} {
		cssFill(t, root, name, want)
	}
}

func TestCSSDescendantChildCompoundsAndComments(t *testing.T) {
	root := readInstances(t, `<svg viewBox="0 0 100 100"><style>
  .outer rect {fill:blue}
  .outer > rect {fill:red}
  g.outer > g.inner rect.hot.cold {fill:yellow}
  #direct.hot {fill:green}
  rect/**/.commented { fill:rgb(0, 0, 255); }
  g g > .deep {fill:magenta}
</style><g class="outer"><rect id="direct" class="hot" width="1" height="1"/>
 <g class="inner"><rect id="descendant" width="1" height="1"/>
 <rect id="compound" class="hot cold" width="1" height="1"/>
 <g><rect id="deep" class="deep" width="1" height="1"/></g></g>
</g><rect id="commented" class="commented" width="1" height="1"/></svg>`)
	cssFill(t, root, "direct", gomath.RGB(0, .5, 0))
	cssFill(t, root, "descendant", gomath.RGB(0, 0, 1))
	cssFill(t, root, "compound", gomath.RGB(1, 1, 0))
	cssFill(t, root, "deep", gomath.RGB(1, 0, 1))
	cssFill(t, root, "commented", gomath.RGB(0, 0, 1))
}

func TestCSSClassWhitespaceAndCase(t *testing.T) {
	root := readInstances(t, `<svg viewBox="0 0 100 100"><style>
 .a {fill:red} .b {fill:blue} .case {FILL:NONE; STROKE:RGB(0, 255, 0)}
 g &gt; .entity {fill:blue}
 </style><rect id="nonspace" class="a&#160;b" width="1" height="1"/>
 <rect id="ascii-space" class="a&#9;b" width="1" height="1"/>
 <rect id="case" class="case" width="1" height="1"/>
 <g><rect id="entity" class="entity" width="1" height="1"/></g></svg>`)
	cssFill(t, root, "nonspace", gomath.Black)
	cssFill(t, root, "ascii-space", gomath.RGB(0, 0, 1))
	cssFill(t, root, "entity", gomath.RGB(0, 0, 1))
	if style := cssObject(t, root, "case").Style; style.Fill.A != 0 || style.Stroke != gomath.RGB(0, 1, 0) {
		t.Fatal("CSS property/value case lost", style)
	}
}

func TestCSSInheritanceKeywordsAndCurrentColor(t *testing.T) {
	root := readInstances(t, `<svg viewBox="0 0 100 100"><style>
 .parent {fill:red;stroke:blue;stroke-width:3;fill-opacity:.4;stroke-opacity:.6;stroke-linecap:square;stroke-linejoin:bevel;stroke-miterlimit:7;opacity:.5}
 .inherit {fill:inherit;stroke:inherit;stroke-width:inherit;fill-opacity:inherit;opacity:inherit}
 .initial {fill:initial;stroke:initial;stroke-width:initial;fill-opacity:initial;stroke-linecap:initial;stroke-linejoin:initial;stroke-miterlimit:initial;opacity:initial}
 .unset {fill:unset;opacity:unset}
 .current {fill:currentColor;stroke:currentColor;color:blue}
 .colored {color:red}
 .inherit-color {color:inherit;fill:currentColor}
</style><g class="parent">
 <rect id="implicit" width="1" height="1"/>
 <rect id="inherit" class="inherit" width="1" height="1"/>
 <rect id="initial" class="initial" width="1" height="1"/>
 <rect id="unset" class="unset" width="1" height="1"/>
</g><g class="current"><rect id="current" class="colored" width="1" height="1"/>
 <rect id="inherit-color" class="inherit-color" width="1" height="1"/>
</g></svg>`)
	for _, name := range []string{"implicit", "inherit", "unset"} {
		style := cssObject(t, root, name).Style
		if style.Fill != (gomath.Color{R: 1, A: .4}) || style.Stroke != (gomath.Color{B: 1, A: .6}) || style.StrokeWidth != 3 || style.StrokeCap != gomath.CapSquare || style.StrokeJoin != gomath.JoinBevel || style.MiterLimit != 7 {
			t.Error(name, "inherited style lost", style)
		}
		wantOpacity := 1.0
		if name == "inherit" {
			wantOpacity = .5
		}
		if style.Opacity != wantOpacity {
			t.Error(name, "opacity must only inherit explicitly", style.Opacity)
		}
	}
	initial := cssObject(t, root, "initial").Style
	if initial.Fill != gomath.Black || initial.Stroke.A != 0 || initial.StrokeWidth != 1 || initial.StrokeCap != gomath.CapButt || initial.StrokeJoin != gomath.JoinMiter || initial.EffectiveMiterLimit() != 4 || initial.Opacity != 1 {
		t.Error("initial values lost", initial)
	}
	cssFill(t, root, "current", gomath.RGB(1, 0, 0))
	cssFill(t, root, "inherit-color", gomath.RGB(0, 0, 1))
	if cssObject(t, root, "current").Style.Stroke != gomath.RGB(1, 0, 0) {
		t.Fatal("inherited currentColor stroke did not follow descendant color")
	}
}

func TestCSSUseShadowBoundaryAndStyleOwnership(t *testing.T) {
	source := `<svg viewBox="0 0 100 100"><defs class="definition">
 <style>
 .definition {fill:magenta}
 .definition .body {fill:yellow}
 .host .body, use > .body {fill:cyan}
 .tile > .body {stroke:blue;stroke-width:2}
 #original {fill:red}
 .left {color:red;fill:currentColor}
 .right {color:blue;fill:currentColor}
 </style>
 <g id="tile" class="tile"><rect class="body" width="8" height="6"/></g>
 <rect id="original" class="body" width="8" height="6"/>
</defs><g class="host"><use class="left" href="#tile" x="10"/>
 <use class="right" href="#tile" x="30"/>
 <use href="#original" x="50" fill="green"/></g></svg>`
	root := readInstances(t, source)
	shapes := instanceShapes(root)
	if len(shapes) != 3 {
		t.Fatal("definitions painted or instances missing")
	}
	if shapes[0].Style.Fill != gomath.RGB(1, 0, 0) || shapes[1].Style.Fill != gomath.RGB(0, 0, 1) || shapes[2].Style.Fill != gomath.RGB(1, 0, 0) {
		t.Fatal("selectors crossed use boundary or definitions supplied inherited fill", shapes[0].Style, shapes[1].Style, shapes[2].Style)
	}
	if shapes[0].Style.Stroke != gomath.RGB(0, 0, 1) || shapes[1].Style.StrokeWidth != 2 {
		t.Fatal("descendant selectors inside cloned subtree must still apply")
	}
	before := shapes[1].Style
	shapes[0].Style.Fill = gomath.White
	if shapes[1].Style != before {
		t.Fatal("instance style aliases sibling")
	}
	second := readInstances(t, source)
	if instanceShapes(second)[0].Style.Fill != gomath.RGB(1, 0, 0) || second.ID == root.ID {
		t.Fatal("separate imports share mutable styles or identity")
	}
}

func TestCSSVisibilityAndDisplayPixelSemantics(t *testing.T) {
	img := renderInstances(t, `<svg viewBox="0 0 100 100"><style>
 .hidden {visibility:hidden} .visible {visibility:visible;fill:red}
 .none {display:none} .inline {display:inline;visibility:visible;fill:blue}
 </style><g class="hidden"><rect x="10" y="10" width="10" height="10"/>
 <rect class="visible" x="30" y="10" width="10" height="10"/></g>
 <g class="none"><rect class="inline" x="50" y="10" width="10" height="10"/></g>
 <rect class="inline" x="70" y="10" width="10" height="10"/></svg>`, 100, 100)
	for _, tc := range []struct {
		x    int
		want color.RGBA
	}{
		{15, color.RGBA{}}, {35, color.RGBA{R: 255, A: 255}}, {55, color.RGBA{}}, {75, color.RGBA{B: 255, A: 255}},
	} {
		if got := img.RGBAAt(tc.x, 15); got != tc.want {
			t.Errorf("pixel x=%d = %v, want %v", tc.x, got, tc.want)
		}
	}
}

func TestCSSNestedUseBoundariesAndOriginalElement(t *testing.T) {
	root := readInstances(t, `<svg viewBox="0 0 100 100"><style>
 .source .body {fill:red}
 use {fill:blue}
 .outer > .alias {color:blue;fill:currentColor}
 .outer .body, use.alias .body, .host .body {fill:yellow}
 .leaf > .body {stroke:red;stroke-width:2}
 </style><g class="source"><rect id="original" class="body" width="4" height="4"/></g>
 <use href="#original" x="10"/>
 <defs><g id="leaf" class="leaf"><rect class="body" width="4" height="4"/></g>
 <g id="outer" class="outer"><use class="alias" href="#leaf"/></g></defs>
 <g class="host"><use href="#outer" x="20"/><use href="#outer" x="30"/></g></svg>`)
	shapes := instanceShapes(root)
	if len(shapes) != 4 {
		t.Fatal("nested CSS instances missing", len(shapes))
	}
	if shapes[0].Style.Fill != gomath.RGB(1, 0, 0) || shapes[1].Style.Fill != gomath.RGB(0, 0, 1) {
		t.Fatal("original and cloned element must match their own ancestor trees")
	}
	for _, shape := range shapes[2:] {
		if shape.Style.Fill != gomath.RGB(0, 0, 1) || shape.Style.Stroke != gomath.RGB(1, 0, 0) || shape.Style.StrokeWidth != 2 {
			t.Fatal("nested use boundary or inherited host style lost", shape.Style)
		}
	}
}

func TestCSSSymbolOverflowAndInlinePrecedence(t *testing.T) {
	source := `<svg viewBox="0 0 100 100"><style>symbol {overflow:visible;fill:blue} #s {fill:red}</style><defs><symbol id="s" viewBox="0 0 10 10" style="fill:white"><rect x="-1" y="-1" width="12" height="12"/></symbol></defs><use href="#s" width="20" height="20" fill="green"/></svg>`
	root := readInstances(t, source)
	shapes := instanceShapes(root)
	assertInstanceBox(t, shapes[0], [4]float64{-2, -2, 24, 24})
	if shapes[0].Style.Fill != gomath.White {
		t.Fatal("symbol CSS/inline precedence lost")
	}
}

func TestCSSRejectUnsupportedSyntaxAndProperties(t *testing.T) {
	for _, sheet := range []string{
		`rect:hover{fill:red}`, `[id=x]{fill:red}`, `g + rect{fill:red}`, `g ~ rect{fill:red}`, `svg|rect{fill:red}`,
		`rect::before{fill:red}`, `:is(rect){fill:red}`, `.\78{fill:red}`, `.中文{fill:red}`,
		`@import "https://example.invalid/a.css";`, `@media all {rect{fill:red}}`, `@keyframes x{}`,
		`rect{fill:red!important}`, `rect{fill:red ! IMPORTANT}`, `rect{fill:red!}`, `.unmatched{unknown-visible:1}`,
		`rect{filter:blur(2px)}`, `rect{transform:scale(2)}`, `rect{fill:var(--x)}`, `rect{width:10px}`,
		`rect{mix-blend-mode:screen}`, `rect{stroke-dasharray:0 0}`, `rect{fill-rule:winding}`, `rect{color:none}`,
		`rect{fill:red`, `rect fill:red}`, `rect{fill:red}bad`, `rect{fill}`, `rect{fill:}`, `rect{fill:blue} }`,
		`rect, {fill:red}`, `>rect{fill:red}`, `rect > {fill:red}`, `rect{.x{fill:red}}`, `/*unclosed`, `re/**/ct{fill:red}`,
	} {
		t.Run(sheet, func(t *testing.T) {
			if root, err := svg.Read(strings.NewReader(`<svg><style>`+sheet+`</style><rect width="1" height="1"/></svg>`), svg.DefaultOptions()); err == nil || root != nil {
				t.Fatalf("accepted unsupported stylesheet %q", sheet)
			}
		})
	}
	for _, style := range []string{`foo:bar`, `fill:red!important`, `stroke-width:NaN`, `transform:scale(2)`, `--x:red`, `fill:inherit!important`, `fill:`} {
		if _, err := svg.Read(strings.NewReader(`<svg><rect style="`+style+`"/></svg>`), svg.DefaultOptions()); err == nil {
			t.Error("unsupported inline CSS accepted", style)
		}
	}
	for _, source := range []string{
		`<svg><style type="text/less">rect{fill:red}</style></svg>`,
		`<svg><style media="screen">rect{fill:red}</style></svg>`,
		`<svg><style scoped="scoped">rect{fill:red}</style></svg>`,
		`<svg><style><g/></style></svg>`,
		`<?xml-stylesheet href="https://example.invalid/a.css"?><svg/>`,
		`<svg><style>svg{overflow:hidden}</style></svg>`,
	} {
		if _, err := svg.Read(strings.NewReader(source), svg.DefaultOptions()); err == nil {
			t.Error("unsupported style scope accepted", source)
		}
	}
}

func TestCSSLimits(t *testing.T) {
	cases := []struct{ name, body, message string }{
		{"bytes", `<style>` + strings.Repeat(" ", 256*1024+1) + `</style>`, "CSS text"},
		{"rules", `<style>` + strings.Repeat(`rect{}`, 513) + `</style>`, "rule count"},
		{"selectors", `<style>` + strings.Repeat(`rect,`, 1024) + `rect{}</style>`, "selector count"},
		{"declarations", `<style>rect{` + strings.Repeat(`fill:red;`, 4097) + `}</style>`, "declaration count"},
		{"inline declarations", strings.Repeat(`<g style="fill:red"/>`, 4097), "declaration count"},
		{"parts", `<style>` + strings.Repeat(`g `, 16) + `rect{}</style>`, "compound parts"},
		{"terms", `<style>.a` + strings.Repeat(`.a`, 16) + `{}</style>`, "terms"},
		{"matching work", `<style>` + strings.Repeat(`rect{fill:red}`, 512) + `</style>` + strings.Repeat(`<rect width="1" height="1"/>`, 2000), "work exceeds"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if root, err := svg.Read(strings.NewReader(`<svg>`+tc.body+`</svg>`), svg.DefaultOptions()); err == nil || root != nil || !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("object=%v error=%v; want %s", root, err, tc.message)
			}
		})
	}
}

func TestCSSRasterReference(t *testing.T) {
	source, err := os.ReadFile("../tests/data/svg/css-cascade.svg")
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Open("../tests/data/svg/css-cascade.png")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	reference, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	actual := renderInstances(t, string(source), 128, 96)
	if actual.Bounds() != reference.Bounds() {
		t.Fatal("CSS reference dimensions differ")
	}
	total, maxDiff, pixels := 0.0, 0, 0
	for y := range 96 {
		for x := range 128 {
			a, b := actual.RGBAAt(x, y), color.RGBAModel.Convert(reference.At(x, y)).(color.RGBA)
			mismatch := false
			for _, delta := range []int{int(a.R) - int(b.R), int(a.G) - int(b.G), int(a.B) - int(b.B), int(a.A) - int(b.A)} {
				d := int(math.Abs(float64(delta)))
				total += float64(d)
				maxDiff = max(maxDiff, d)
				mismatch = mismatch || d > 2
			}
			if mismatch {
				pixels++
			}
		}
	}
	mean, ratio := total/(128*96*4), float64(pixels)/(128*96)
	t.Logf("CairoSVG CSS 128x96 RGBA: MAE=%g max=%d changed-pixel-ratio=%g", mean, maxDiff, ratio)
	if mean > .1 || maxDiff > 16 || ratio > .02 {
		t.Fatalf("CSS reference raster differs: MAE=%g max=%d ratio=%g", mean, maxDiff, ratio)
	}
}

func TestCSSUseShadowReferencePixels(t *testing.T) {
	img := renderInstances(t, `<svg viewBox="0 0 100 100"><style>
 .definition .body {fill:yellow}
 .host .body, use .body {fill:cyan}
 .left {fill:red} .right {fill:blue}
 .tile>.mark {fill:white}
 </style><defs class="definition"><g id="tile" class="tile">
 <rect class="body" width="20" height="20"/>
 <rect class="mark" x="5" y="5" width="5" height="5"/>
 </g></defs><g class="host"><use class="left" href="#tile" x="10" y="10"/>
 <use class="right" href="#tile" x="50" y="10"/></g></svg>`, 100, 100)
	for _, tc := range []struct {
		x, y int
		want color.RGBA
	}{
		{12, 12, color.RGBA{R: 255, A: 255}}, {52, 12, color.RGBA{B: 255, A: 255}},
		{17, 17, color.RGBA{R: 255, G: 255, B: 255, A: 255}}, {57, 17, color.RGBA{R: 255, G: 255, B: 255, A: 255}},
		{35, 15, color.RGBA{}},
	} {
		if got := img.RGBAAt(tc.x, tc.y); got != tc.want {
			t.Errorf("shadow pixel (%d,%d) = %v want %v", tc.x, tc.y, got, tc.want)
		}
	}
}

func TestCSSReadIsolationAcrossConcurrentImports(t *testing.T) {
	for i := range 12 {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			name := fmt.Sprintf("c%d", i)
			root := readInstances(t, `<svg viewBox="0 0 100 100"><style>.`+name+`{fill:red}</style><rect id="a" class="`+name+`" width="1" height="1"/></svg>`)
			cssFill(t, root, "a", gomath.RGB(1, 0, 0))
		})
	}
}

func BenchmarkReadCSSInstances(b *testing.B) {
	var doc strings.Builder
	doc.WriteString(`<svg viewBox="0 0 256 256"><style>use{fill:blue}.tile>.body{stroke:red;stroke-width:1}.white{fill:white}#unused{fill:green}</style><defs><g id="tile" class="tile"><rect class="body" x="1" y="1" width="6" height="6"/><circle class="white" cx="4" cy="4" r="2"/></g></defs>`)
	for i := range 256 {
		fmt.Fprintf(&doc, `<use href="#tile" x="%d" y="%d"/>`, (i%16)*16, (i/16)*16)
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
