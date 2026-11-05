package svg_test

import (
	"encoding/json"
	"fmt"
	"gomath"
	"gomath/geom"
	"gomath/render"
	"gomath/svg"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"strings"
	"testing"
)

func readInstances(t *testing.T, source string) *gomath.Object {
	t.Helper()
	root, err := svg.Read(strings.NewReader(source), svg.Options{Width: 100})
	if err != nil {
		t.Fatal(err)
	}
	if err := root.Validate(); err != nil {
		t.Fatal(err)
	}
	return root
}

func instanceShapes(root *gomath.Object) []*gomath.Object {
	var shapes []*gomath.Object
	root.Walk(func(o *gomath.Object) {
		if len(o.Path.Contours) != 0 {
			shapes = append(shapes, o)
		}
	})
	return shapes
}

// Most test documents use a 100x100 viewport and world width 100. Convert back
// to SVG coordinates so the independent expected values stay easy to audit.
func assertInstanceBox(t *testing.T, o *gomath.Object, want [4]float64) {
	t.Helper()
	bounds := o.Bounds()
	got := [4]float64{bounds.Min.X + 50, 50 - bounds.Max.Y, bounds.Size().X, bounds.Size().Y}
	for i := range got {
		if !bounds.Valid || math.Abs(got[i]-want[i]) > 1e-8 {
			t.Fatalf("SVG bounds %v, want %v", got, want)
		}
	}
}

func TestUseForwardReferencesNestedDefsAndIndependentInstances(t *testing.T) {
	root := readInstances(t, `<svg viewBox="0 0 100 100">
<use href="#tile" x="10" y="20" fill="red"/>
<use href="#tile" x="40" y="20" fill="blue"/>
<g fill="magenta" transform="translate(70,0)"><defs>
 <g id="tile" transform="translate(2,3)"><rect id="box" width="8" height="6"/>
  <defs><rect id="hidden" width="100" height="100"/></defs>
  <symbol id="inert"><rect width="100" height="100"/></symbol>
 </g>
</defs></g>
<use href="#hidden" x="2" y="3" transform="scale(.1)" fill="green"/>
</svg>`)
	shapes := instanceShapes(root)
	if len(shapes) != 3 {
		t.Fatalf("got %d shapes; nested definitions must remain unpainted", len(shapes))
	}
	assertInstanceBox(t, shapes[0], [4]float64{12, 23, 8, 6})
	assertInstanceBox(t, shapes[1], [4]float64{42, 23, 8, 6})
	assertInstanceBox(t, shapes[2], [4]float64{.2, .3, 10, 10})
	if shapes[0].Style.Fill != gomath.RGB(1, 0, 0) || shapes[1].Style.Fill != gomath.RGB(0, 0, 1) {
		t.Fatal("instances must inherit from use, not from the definition's ancestors")
	}
	ids := make(map[uint64]bool)
	root.Walk(func(o *gomath.Object) {
		if ids[o.ID] {
			t.Error("reused object ID", o.ID)
		}
		ids[o.ID] = true
	})
	before := shapes[1].Path.Contours[0].Curves[0].P0
	shapes[0].Path.Contours[0].Curves[0].P0 = geom.V(123, 456)
	if shapes[1].Path.Contours[0].Curves[0].P0 != before {
		t.Fatal("instances share curve storage")
	}
}

func TestUseHrefNamespaceAndPrecedence(t *testing.T) {
	for _, attrs := range []string{
		`href="#a"`, `xlink:href="#a"`, `legacy:href="#a"`,
		`xlink:href="#missing" href="#a"`, `href="#a" xlink:href="#missing"`,
	} {
		t.Run(attrs, func(t *testing.T) {
			root := readInstances(t, `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" xmlns:legacy="http://www.w3.org/1999/xlink" viewBox="0 0 100 100"><defs><rect id="a" width="3" height="4"/></defs><use `+attrs+` x="10" y="20"/></svg>`)
			shapes := instanceShapes(root)
			if len(shapes) != 1 {
				t.Fatal("missing reference instance")
			}
			assertInstanceBox(t, shapes[0], [4]float64{10, 20, 3, 4})
		})
	}
}

func TestUseNestedTransformsAndInlineStyle(t *testing.T) {
	root := readInstances(t, `<svg viewBox="0 0 100 100"><defs>
 <g id="tile" transform="translate(1,2)"><path id="line" d="M0 0 L4 0" fill="none"/>
  <rect id="explicit" width="2" height="2" fill="white"/>
 </g><use id="alias" href="#tile" x="3" y="4" transform="scale(2)"/>
</defs><g transform="translate(5,7)" stroke="green" stroke-width="3">
 <use id="instance" href="#alias" x="10" y="20" transform="rotate(90)" fill="red" stroke="red"
  style="stroke:blue;stroke-width:2;stroke-linecap:square;stroke-linejoin:bevel;stroke-miterlimit:7;fill-opacity:0.4" opacity="0.5"/>
</g></svg>`)
	shapes := instanceShapes(root)
	if len(shapes) != 2 {
		t.Fatal("wrong number of expanded shapes", len(shapes))
	}
	// translate(5,7) rotate(90) translate(10,20) scale(2)
	// translate(3,4) translate(1,2), applied to (0,0) and (4,0).
	assertInstanceBox(t, shapes[0], [4]float64{-27, 25, 0, 8})
	if shapes[0].Style.Stroke != gomath.RGB(0, 0, 1) || shapes[0].Style.StrokeWidth != 4 ||
		shapes[0].Style.StrokeCap != gomath.CapSquare || shapes[0].Style.StrokeJoin != gomath.JoinBevel || shapes[0].Style.MiterLimit != 7 {
		t.Fatal("use stroke inheritance or style precedence lost", shapes[0].Style)
	}
	if shapes[1].Style.Fill != (gomath.Color{R: 1, G: 1, B: 1, A: .4}) {
		t.Fatal("explicit definition fill must override use fill and inherit fill-opacity", shapes[1].Style)
	}
	root.Walk(func(o *gomath.Object) {
		if o.Name == "instance" && o.Style.Opacity != .5 {
			t.Error("use opacity lost")
		}
	})
}

func TestSymbolViewBoxDimensionsAndAspectRatio(t *testing.T) {
	cases := []struct {
		name, symbol, use, rect string
		want                    [4]float64
	}{
		{"default parent viewport", `viewBox="0 0 10 20"`, `x="5" y="7"`, `width="10" height="20"`, [4]float64{30, 7, 50, 100}},
		{"symbol dimensions", `viewBox="0 0 10 10" width="40" height="20"`, `x="5" y="7"`, `width="10" height="10"`, [4]float64{15, 7, 20, 20}},
		{"use overrides dimensions", `viewBox="0 0 10 10" width="40" height="20"`, `x="5" y="7" width="60" height="20"`, `width="10" height="10"`, [4]float64{25, 7, 20, 20}},
		{"no viewBox does not scale", `width="20" height="30"`, `x="5" y="7" width="40" height="60"`, `width="2" height="4"`, [4]float64{5, 7, 2, 4}},
		{"nonzero viewBox origin", `viewBox="10 20 10 10"`, `x="5" y="7" width="40" height="20"`, `x="10" y="20" width="10" height="10"`, [4]float64{15, 7, 20, 20}},
		{"none", `viewBox="0 0 10 10" preserveAspectRatio="none"`, `x="5" y="7" width="40" height="20"`, `width="10" height="10"`, [4]float64{5, 7, 40, 20}},
		{"max meet", `viewBox="0 0 10 10" preserveAspectRatio="xMaxYMax meet"`, `x="5" y="7" width="40" height="20"`, `width="10" height="10"`, [4]float64{25, 7, 20, 20}},
		{"min meet", `viewBox="0 0 10 10" preserveAspectRatio="xMinYMin"`, `x="5" y="7" width="20" height="40"`, `width="10" height="10"`, [4]float64{5, 7, 20, 20}},
		{"slice visible", `viewBox="0 0 10 10" preserveAspectRatio="xMidYMid slice" overflow="visible"`, `x="5" y="7" width="40" height="20"`, `width="10" height="10"`, [4]float64{5, -3, 40, 40}},
		{"percentage use", `viewBox="0 0 10 10"`, `x="10%" y="20%" width="50%" height="20%"`, `width="10" height="10"`, [4]float64{25, 20, 20, 20}},
		{"percentage symbol", `viewBox="0 0 10 10" width="50%" height="20%"`, `x="10%" y="20%"`, `width="10" height="10"`, [4]float64{25, 20, 20, 20}},
		{"physical units", `viewBox="0 0 10 10"`, `x="1pc" y="1pt" width=".5in" height="24px"`, `width="10" height="10"`, [4]float64{28, 96.0 / 72, 24, 24}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := readInstances(t, `<svg viewBox="0 0 100 100"><defs><symbol id="s" `+tc.symbol+`><rect `+tc.rect+`/></symbol></defs><use href="#s" `+tc.use+`/></svg>`)
			shapes := instanceShapes(root)
			if len(shapes) != 1 {
				t.Fatal("missing symbol instance")
			}
			assertInstanceBox(t, shapes[0], tc.want)
		})
	}
}

func TestSymbolZeroDimensionsAndOrdinaryUseDimensions(t *testing.T) {
	for _, dimension := range []string{`width="0"`, `height="0"`, `width="0%"`} {
		root := readInstances(t, `<svg viewBox="0 0 100 100"><symbol id="s" viewBox="0 0 10 10"><rect width="10" height="10"/></symbol><use href="#s" `+dimension+`/></svg>`)
		if len(instanceShapes(root)) != 0 {
			t.Error("zero symbol viewport must not paint", dimension)
		}
	}
	root := readInstances(t, `<svg viewBox="0 0 100 100"><defs><rect id="r" width="10" height="5"/></defs><use href="#r" width="0" height="50"/></svg>`)
	assertInstanceBox(t, instanceShapes(root)[0], [4]float64{0, 0, 10, 5})
}

func TestNestedSymbolPercentageViewportAndClipFrames(t *testing.T) {
	root := readInstances(t, `<svg viewBox="0 0 100 100"><defs>
 <symbol id="inner" viewBox="0 0 10 10"><rect x="2" y="2" width="6" height="6"/></symbol>
 <symbol id="outer" viewBox="0 0 20 20"><g transform="translate(1,1)"><use href="#inner" x="25%" y="25%" width="50%" height="50%"/></g></symbol>
</defs><use href="#outer" x="10" y="20" width="40" height="40" transform="rotate(0)"/></svg>`)
	shapes := instanceShapes(root)
	if len(shapes) != 1 {
		t.Fatal("nested symbol expansion failed")
	}
	assertInstanceBox(t, shapes[0], [4]float64{26, 36, 12, 12})
}

func TestUseRejectInvalidReferencesAndDocuments(t *testing.T) {
	cases := []struct{ name, source, message string }{
		{"missing href", `<svg><use/></svg>`, "local fragment"},
		{"empty fragment", `<svg><use href="#"/></svg>`, "local fragment"},
		{"unknown", `<svg><use href="#missing"/></svg>`, "unknown"},
		{"unknown in unused defs", `<svg><defs><use href="#missing"/></defs></svg>`, "unknown"},
		{"external file", `<svg><use href="other.svg#a"/></svg>`, "local fragment"},
		{"external URL", `<svg><use href="https://example.invalid/a.svg#a"/></svg>`, "local fragment"},
		{"data URI", `<svg><use href="data:image/svg+xml,test"/></svg>`, "local fragment"},
		{"external xlink", `<svg xmlns:xlink="http://www.w3.org/1999/xlink"><use xlink:href="file:///tmp/a.svg#a"/></svg>`, "local fragment"},
		{"empty href overrides xlink", `<svg xmlns:xlink="http://www.w3.org/1999/xlink"><defs><g id="a"/></defs><use href="" xlink:href="#a"/></svg>`, "local fragment"},
		{"foreign href namespace", `<svg xmlns:other="urn:other"><g id="a"/><use other:href="#a"/></svg>`, "local fragment"},
		{"duplicate unused IDs", `<svg><defs><g id="a"/><defs><g id="a"/></defs></defs></svg>`, "duplicate SVG id"},
		{"duplicate rendered IDs", `<svg><rect id="a"/><g id="a"/></svg>`, "duplicate SVG id"},
		{"self cycle", `<svg><use id="a" href="#a"/></svg>`, "cyclic"},
		{"indirect cycle", `<svg><defs><use id="a" href="#b"/><use id="b" href="#a"/></defs></svg>`, "cyclic"},
		{"ancestor cycle", `<svg><g id="a"><use href="#a"/></g></svg>`, "cyclic"},
		{"symbol cycle", `<svg><defs><symbol id="a"><use href="#b"/></symbol><symbol id="b"><use href="#a"/></symbol></defs></svg>`, "cyclic"},
		{"defs target", `<svg><defs id="a"/><use href="#a"/></svg>`, "unsupported SVG use target"},
		{"unsupported target", `<svg><defs><foreignObject id="a"/></defs><use href="#a"/></svg>`, "unsupported SVG use target"},
		{"unsupported stylesheet in defs", `<svg><defs><style>rect{filter:blur(2px)}</style><rect id="a"/></defs><use href="#a"/></svg>`, "filter"},
		{"multiple roots", `<svg/><svg/>`, "multiple"},
		{"unclosed", `<svg><defs>`, "decode"},
		{"duplicate attribute", `<svg><use href="#a" href="#b"/></svg>`, "duplicate SVG attribute"},
		{"text outside root", `<svg/>bad`, "outside"},
		{"use child", `<svg><g id="a"/><use href="#a"><rect width="1"/></use></svg>`, "unsupported child"},
		{"foreign defs", `<svg xmlns:other="urn:other"><other:defs><rect width="1"/></other:defs></svg>`, "namespace"},
		{"nonfinite transform", `<svg><g transform="scale(1e308) scale(1e308)"/></svg>`, "finite"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o, err := svg.Read(strings.NewReader(tc.source), svg.DefaultOptions())
			if err == nil || o != nil || !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("got object %v, error %v; want %q", o, err, tc.message)
			}
		})
	}
}

func TestPinnedManimUseGeometryAndPaint(t *testing.T) {
	source, err := os.ReadFile("../tests/data/svg/manim-use.svg")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("../tests/data/svg/manim-use.json")
	if err != nil {
		t.Fatal(err)
	}
	var reference struct {
		SourceCommit string `json:"source_commit"`
		Shapes       []struct {
			Bounds      [4]float64 `json:"bounds"`
			Fill        string     `json:"fill"`
			FillOpacity float64    `json:"fill_opacity"`
		} `json:"shapes"`
	}
	if err := json.Unmarshal(data, &reference); err != nil {
		t.Fatal(err)
	}
	if reference.SourceCommit != "16ea443e2a5f1d79db0a721161411ce916d6e733" {
		t.Fatal("reference source pin changed")
	}
	root := readInstances(t, string(source))
	shapes := instanceShapes(root)
	if len(shapes) != len(reference.Shapes) {
		t.Fatalf("got %d shapes, Manim has %d", len(shapes), len(reference.Shapes))
	}
	for i, shape := range shapes {
		assertInstanceBox(t, shape, reference.Shapes[i].Bounds)
		want, err := gomath.Hex(reference.Shapes[i].Fill)
		if err != nil {
			t.Fatal(err)
		}
		want.A *= reference.Shapes[i].FillOpacity
		if shape.Style.Fill != want {
			t.Errorf("shape %d fill = %v, Manim %v", i, shape.Style.Fill, want)
		}
	}
}

func TestSymbolRejectUnsupportedAndInvalidGeometry(t *testing.T) {
	for _, attrs := range []string{
		`viewBox="0 0 0 1"`, `viewBox="0 0 1 -1"`, `viewBox=""`, `viewBox="0 0 1"`,
		`viewBox="0 0 10 10" preserveAspectRatio="bad"`,
		`viewBox="0 0 10 10" preserveAspectRatio="xMidYMid stretch"`,
		`width="-1"`, `height="NaN"`, `width="1e308in"`,
		`x="1"`, `refX="center"`, `overflow="invalid"`,
	} {
		source := `<svg><defs><symbol id="s" ` + attrs + `><rect width="1" height="1"/></symbol></defs><use href="#s"/></svg>`
		if _, err := svg.Read(strings.NewReader(source), svg.DefaultOptions()); err == nil {
			t.Error("accepted symbol attributes", attrs)
		}
	}
	for _, attrs := range []string{`x="bad"`, `y="Inf"`, `width="-1"`, `height="-5%"`, `height="1em"`, `width="1e308in"`} {
		source := `<svg><defs><symbol id="s" viewBox="0 0 10 10"/></defs><use href="#s" ` + attrs + `/></svg>`
		if _, err := svg.Read(strings.NewReader(source), svg.DefaultOptions()); err == nil {
			t.Error("accepted use attributes", attrs)
		}
	}
	for _, attrs := range []string{``, `preserveAspectRatio="xMidYMid slice"`} {
		source := `<svg viewBox="0 0 100 100"><defs><symbol id="s" viewBox="0 0 10 10" ` + attrs + `><rect x="-1" y="-1" width="12" height="12"/></symbol></defs><use href="#s" width="20" height="10"/></svg>`
		if _, err := svg.Read(strings.NewReader(source), svg.DefaultOptions()); err == nil || !strings.Contains(err.Error(), "clipping") {
			t.Error("must explicitly reject symbol content requiring clipping", err)
		}
	}
	// Overflow visible is implemented, including inline style precedence.
	root := readInstances(t, `<svg viewBox="0 0 100 100"><symbol id="s" viewBox="0 0 10 10" overflow="hidden" style="overflow:visible"><rect x="-1" y="-1" width="12" height="12"/></symbol><use href="#s" width="20" height="20"/></svg>`)
	assertInstanceBox(t, instanceShapes(root)[0], [4]float64{-2, -2, 24, 24})
}

func TestSVGReferenceAndXMLLimits(t *testing.T) {
	const limit = 128
	accepted := `<svg>` + strings.Repeat(`<g>`, limit-1) + strings.Repeat(`</g>`, limit-1) + `</svg>`
	readInstances(t, accepted)
	tooDeep := `<svg>` + strings.Repeat(`<g>`, limit) + strings.Repeat(`</g>`, limit) + `</svg>`
	if _, err := svg.Read(strings.NewReader(tooDeep), svg.DefaultOptions()); err == nil || !strings.Contains(err.Error(), "XML depth") {
		t.Fatal("XML depth was not bounded", err)
	}
	var refs strings.Builder
	refs.WriteString(`<svg><defs><g id="n0"/>`)
	for i := 1; i <= limit; i++ {
		fmt.Fprintf(&refs, `<use id="n%d" href="#n%d"/>`, i, i-1)
	}
	refs.WriteString(`</defs></svg>`)
	if _, err := svg.Read(strings.NewReader(refs.String()), svg.DefaultOptions()); err == nil || !strings.Contains(err.Error(), "reference depth") {
		t.Fatal("reference graph depth was not bounded", err)
	}
	tooMany := `<svg><defs>` + strings.Repeat(`<g/>`, 100000) + `</defs></svg>`
	if _, err := svg.Read(strings.NewReader(tooMany), svg.DefaultOptions()); err == nil || !strings.Contains(err.Error(), "XML element count") {
		t.Fatal("XML element count was not bounded", err)
	}
	var fanout strings.Builder
	fanout.WriteString(`<svg><defs><rect id="n0" width="1" height="1"/>`)
	for i := 1; i <= 16; i++ {
		fmt.Fprintf(&fanout, `<g id="n%d"><use href="#n%d"/><use href="#n%d"/></g>`, i, i-1, i-1)
	}
	fanout.WriteString(`</defs><use href="#n16"/></svg>`)
	if _, err := svg.Read(strings.NewReader(fanout.String()), svg.DefaultOptions()); err == nil || !strings.Contains(err.Error(), "expanded element count") {
		t.Fatal("reference expansion count was not bounded", err)
	}
}

func renderInstances(t *testing.T, source string, width, height int) *image.RGBA {
	t.Helper()
	root, err := svg.Read(strings.NewReader(source), svg.Options{Width: 8})
	if err != nil {
		t.Fatal(err)
	}
	c := render.DefaultConfig()
	c.Width, c.Height, c.Workers, c.FrameWidth, c.Background = width, height, 1, 8, gomath.Transparent
	raster, err := render.NewRasterizer(c)
	if err != nil {
		t.Fatal(err)
	}
	img, err := raster.Render(&gomath.Frame{Objects: []*gomath.Object{root}})
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func TestLocalInstancesRasterReference(t *testing.T) {
	source, err := os.ReadFile("../tests/data/svg/local-instances.svg")
	if err != nil {
		t.Fatal(err)
	}
	referenceFile, err := os.Open("../tests/data/svg/local-instances.png")
	if err != nil {
		t.Fatal(err)
	}
	defer referenceFile.Close()
	reference, err := png.Decode(referenceFile)
	if err != nil {
		t.Fatal(err)
	}
	actual := renderInstances(t, string(source), 128, 96)
	if actual.Bounds() != reference.Bounds() {
		t.Fatal("raster dimensions differ")
	}
	var total float64
	maxDiff := 0
	for y := range 96 {
		for x := range 128 {
			a, b := actual.RGBAAt(x, y), color.RGBAModel.Convert(reference.At(x, y)).(color.RGBA)
			for _, v := range []int{int(a.R) - int(b.R), int(a.G) - int(b.G), int(a.B) - int(b.B), int(a.A) - int(b.A)} {
				d := int(math.Abs(float64(v)))
				maxDiff = max(maxDiff, d)
				total += float64(d)
			}
		}
	}
	mean := total / (128 * 96 * 4)
	t.Logf("CairoSVG 2.7.1, 128x96 RGBA: MAE=%g max=%d", mean, maxDiff)
	if mean > .05 || maxDiff > 2 {
		t.Fatalf("local reference raster differs: MAE=%g max=%d", mean, maxDiff)
	}
}

func TestUseOpacityAndIndependentRasterColors(t *testing.T) {
	actual := renderInstances(t, `<svg viewBox="0 0 100 100"><defs><rect id="r" width="20" height="20"/></defs><use href="#r" x="10" y="10" fill="red" opacity="0.5"/><use href="#r" x="50" y="10" style="fill:blue;fill-opacity:0.25"/></svg>`, 100, 100)
	for _, tc := range []struct {
		x, y int
		want color.RGBA
	}{
		{15, 15, color.RGBA{128, 0, 0, 128}},
		{55, 15, color.RGBA{0, 0, 64, 64}},
		{35, 15, color.RGBA{}},
	} {
		if got := actual.RGBAAt(tc.x, tc.y); got != tc.want {
			t.Errorf("pixel (%d,%d) = %v, want %v", tc.x, tc.y, got, tc.want)
		}
	}
}

func BenchmarkReadLocalInstances(b *testing.B) {
	var doc strings.Builder
	doc.WriteString(`<svg viewBox="0 0 256 256"><defs><g id="tile"><rect width="8" height="8"/><circle cx="4" cy="4" r="2" fill="white"/></g></defs>`)
	for i := range 256 {
		fmt.Fprintf(&doc, `<use href="#tile" x="%d" y="%d" fill="blue"/>`, (i%16)*16, (i/16)*16)
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
