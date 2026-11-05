package svg_test

import (
	"fmt"
	"gomath"
	"gomath/geom"
	"gomath/svg"
	"image/color"
	"reflect"
	"strings"
	"testing"
)

func clipBox(t *testing.T, o *gomath.Object, want [4]float64) {
	t.Helper()
	if o.Clip == nil {
		t.Fatal("missing native clip", o.Name)
	}
	assertInstanceBox(t, gomath.NewObject(o.Clip.Path()), want)
}

func TestSVGClipTransformUseCSSAndIndependentOwnership(t *testing.T) {
	source := `<svg viewBox="0 0 100 100" xmlns:xlink="http://www.w3.org/1999/xlink">
 <style>.definition > clipPath {clip-rule:nonzero} .host .cut {clip-path:url(#wrong)}
 .tile > .body {clip-path:url('#window')} #inline {clip-path:url(#wrong)}</style>
 <defs class="definition" transform="translate(900 900)">
 <rect id="silhouette" x="1" y="2" width="6" height="4" transform="scale(2)" fill="none" stroke="red" stroke-width="30" opacity="0"/>
 <clipPath id="window" transform="translate(2 3)"><use xlink:href="#silhouette" x="3" y="4" transform="scale(2 1)"/></clipPath>
 <clipPath id="wrong"><rect width="1" height="1"/></clipPath>
 <g id="tile" class="tile"><rect class="body cut" width="60" height="30"/></g></defs>
 <g class="host"><use href="#tile" x="10" y="20"/><use href="#tile" x="40" y="20"/></g>
 <rect id="inline" width="100" height="100" style="clip-path:URL( '#window' )"/>
 </svg>`
	root := readInstances(t, source)
	shapes := instanceShapes(root)
	if len(shapes) != 3 {
		t.Fatal("clip definitions painted or use geometry lost", len(shapes))
	}
	clipBox(t, shapes[0], [4]float64{22, 31, 24, 8})
	clipBox(t, shapes[1], [4]float64{52, 31, 24, 8})
	clipBox(t, shapes[2], [4]float64{12, 11, 24, 8})
	before := shapes[1].Clip.Path()
	returned := shapes[0].Clip.Path()
	returned.Contours[0].Curves[0].P0 = geom.V(999, 999)
	if !reflect.DeepEqual(before, shapes[1].Clip.Path()) {
		t.Fatal("Clip.Path aliases immutable geometry")
	}
	shapes[0].Shift(geom.V(7, 9))
	if !reflect.DeepEqual(before, shapes[1].Clip.Path()) {
		t.Fatal("transformed clip aliases sibling")
	}
	second := readInstances(t, source)
	clipBox(t, instanceShapes(second)[0], [4]float64{22, 31, 24, 8})
	ids := map[uint64]bool{}
	root.Walk(func(o *gomath.Object) {
		if ids[o.ID] {
			t.Fatal("shared expanded object ID")
		}
		ids[o.ID] = true
	})
}

func TestSVGClipUseHostCoordinatesAndNestedTransform(t *testing.T) {
	root := readInstances(t, `<svg viewBox="0 0 100 100"><defs>
 <clipPath id="c"><rect x="1" y="2" width="8" height="6"/></clipPath>
 <rect id="tile" width="20" height="20" clip-path="url(#c)"/>
 </defs><use id="host" href="#tile" x="4" y="5" transform="translate(10 20) scale(2)" clip-path="url(#c)"/>
 </svg>`)
	clipBox(t, cssObject(t, root, "host"), [4]float64{12, 24, 16, 12})
	clipBox(t, cssObject(t, root, "tile"), [4]float64{20, 34, 16, 12})
}

func TestSVGClipCSSWideValuesAndDefinitionInheritance(t *testing.T) {
	root := readInstances(t, `<svg viewBox="0 0 100 100"><style>
 .inherit {clip-path:inherit} .unset {clip-path:unset} .initial {clip-path:initial}
 .definition {visibility:hidden} .definition > clipPath > .show {visibility:visible}
 </style><defs class="definition"><clipPath id="c"><rect class="show" x="10" y="20" width="30" height="40"/></clipPath></defs>
 <g clip-path="url(#c)"><rect id="normal" width="100" height="100"/><rect id="inherit" class="inherit" width="100" height="100"/>
 <rect id="unset" class="unset" width="100" height="100"/><rect id="initial" class="initial" width="100" height="100"/></g></svg>`)
	clipBox(t, cssObject(t, root, "inherit"), [4]float64{10, 20, 30, 40})
	for _, id := range []string{"normal", "unset", "initial"} {
		if cssObject(t, root, id).Clip != nil {
			t.Fatal("non-inherited clip-path leaked into child", id)
		}
	}
}

func TestSVGClipSilhouetteNonzeroHoleAndPaintIndependence(t *testing.T) {
	source := `<svg viewBox="0 0 100 100"><defs><clipPath id="c" display="none" opacity="0" fill="none" stroke="blue" stroke-width="30">
 <path d="M10 10H90V90H10Z M30 30V70H70V30Z" clip-rule="nonzero" opacity="0"/>
 </clipPath></defs><rect width="100" height="100" fill="red" clip-path="url(#c)"/></svg>`
	img := renderInstances(t, source, 100, 100)
	for _, tc := range []struct {
		x, y int
		want color.RGBA
	}{{20, 20, color.RGBA{R: 255, A: 255}}, {50, 50, color.RGBA{}}, {5, 50, color.RGBA{}}} {
		if got := img.RGBAAt(tc.x, tc.y); got != tc.want {
			t.Errorf("clip pixel %d,%d = %v, want %v", tc.x, tc.y, got, tc.want)
		}
	}
}

func TestSVGClipVisibilityAndDisplay(t *testing.T) {
	for _, body := range []string{
		`<rect width="100" height="100" display="none"/>`,
		`<rect width="100" height="100" visibility="hidden"/>`,
		`<use href="#r" display="none"/>`,
		``,
	} {
		source := `<svg viewBox="0 0 100 100"><defs><rect id="r" width="100" height="100" visibility="visible"/>
 <clipPath id="c">` + body + `</clipPath></defs><rect width="100" height="100" fill="red" clip-path="url(#c)"/></svg>`
		if got := renderInstances(t, source, 100, 100).RGBAAt(50, 50); got != (color.RGBA{}) {
			t.Fatal("empty/hidden clip did not hide content", body, got)
		}
	}
	visible := `<svg viewBox="0 0 100 100"><defs><rect id="r" width="100" height="100" visibility="visible"/>
 <clipPath id="c"><use href="#r" visibility="hidden"/></clipPath></defs><rect width="100" height="100" fill="red" clip-path="url(#c)"/></svg>`
	if got := renderInstances(t, visible, 100, 100).RGBAAt(50, 50); got != (color.RGBA{R: 255, A: 255}) {
		t.Fatal("target could not restore inherited visibility", got)
	}
}

func TestSVGClipPathAndChildIntersection(t *testing.T) {
	source := `<svg viewBox="0 0 100 100"><defs>
 <clipPath id="left"><rect width="40" height="100"/></clipPath>
 <clipPath id="top"><rect width="100" height="40"/></clipPath>
 <clipPath id="c" transform="translate(10 20)" clip-path="url(#left)">
 <rect width="80" height="80" clip-path="url(#top)" transform="translate(5 5)"/>
 </clipPath></defs><rect width="100" height="100" fill="blue" clip-path="url(#c)"/></svg>`
	img := renderInstances(t, source, 100, 100)
	for _, tc := range []struct {
		x, y    int
		visible bool
	}{{20, 30, true}, {45, 60, true}, {51, 35, false}, {20, 66, false}, {14, 30, false}} {
		if got := img.RGBAAt(tc.x, tc.y); (got.A != 0) != tc.visible {
			t.Error("nested silhouette intersection", tc, got)
		}
	}
}

func TestSVGClipTextTspanAndSymbol(t *testing.T) {
	root := readInstances(t, `<svg viewBox="0 0 100 100"><defs><clipPath id="c"><rect width="12" height="8"/></clipPath>
 <symbol id="s" viewBox="0 0 10 10" overflow="visible" clip-path="url(#c)"><rect width="10" height="10"/></symbol></defs>
 <text id="text" x="4" y="20" clip-path="url(#c)">Go<tspan id="span" clip-path="inherit">Math</tspan></text>
 <use href="#s" x="30" y="40" width="20" height="20"/></svg>`)
	clipBox(t, cssObject(t, root, "text"), [4]float64{0, 0, 12, 8})
	clipBox(t, cssObject(t, root, "span"), [4]float64{0, 0, 12, 8})
	clipBox(t, cssObject(t, root, "s"), [4]float64{30, 40, 24, 16})
}

func TestSVGClipAffineReflectionAndBitmapOpacity(t *testing.T) {
	root := readInstances(t, `<svg viewBox="0 0 100 100"><defs><clipPath id="c" transform="matrix(-2 0 1 3 20 10)">
 <rect x="1" y="2" width="4" height="5"/></clipPath></defs><rect width="100" height="100" clip-path="url(#c)"/></svg>`)
	clipBox(t, instanceShapes(root)[0], [4]float64{12, 16, 13, 15})
	uri := imageURI(t, "embedded.png", "png")
	for _, attrs := range []string{`opacity=".5"`, `style="opacity:.5"`} {
		source := `<svg viewBox="0 0 100 100"><defs><clipPath id="c"><rect x="20" y="20" width="60" height="60"/></clipPath>
 <image id="im" href="` + uri + `" width="100" height="100" preserveAspectRatio="none"/></defs>
 <use href="#im" clip-path="url(#c)" ` + attrs + `/></svg>`
		got := renderInstances(t, source, 100, 100)
		plain := renderInstances(t, imageDoc(uri, `width="100" height="100" preserveAspectRatio="none"`), 100, 100)
		for _, pt := range [][2]int{{35, 35}, {65, 65}} {
			a, b := got.RGBAAt(pt[0], pt[1]), plain.RGBAAt(pt[0], pt[1])
			for _, pair := range [][2]uint8{{a.R, b.R}, {a.G, b.G}, {a.B, b.B}, {a.A, b.A}} {
				d := int(pair[0])*2 - int(pair[1])
				if d < -2 || d > 2 {
					t.Fatal("clipped use bitmap opacity was lost", a, b)
				}
			}
		}
		if got.RGBAAt(10, 50).A != 0 {
			t.Fatal("bitmap escaped clip")
		}
	}
}

func TestSVGClipCloneLocalDefinitionsExplicitBoundary(t *testing.T) {
	// A host's reference is in the light tree. A reference in its cloned target
	// would need instance-local definition resolution and is explicitly rejected.
	root := readInstances(t, `<svg viewBox="0 0 100 100"><defs><g id="tile"><defs>
 <clipPath id="c"><rect width="5" height="6"/></clipPath></defs><rect width="10" height="10"/></g></defs>
 <use id="host" href="#tile" clip-path="url(#c)" x="20"/></svg>`)
	clipBox(t, cssObject(t, root, "host"), [4]float64{0, 0, 5, 6})
	source := `<svg><defs><g id="tile"><defs><clipPath id="c"><rect width="5" height="6"/></clipPath></defs>
 <rect width="10" height="10" clip-path="url(#c)"/></g></defs><use href="#tile"/></svg>`
	object, err := svg.Read(strings.NewReader(source), svg.DefaultOptions())
	if object != nil || err == nil || !strings.Contains(err.Error(), "clone-local") {
		t.Fatal("clone-local clip definitions silently used source-tree CSS", err)
	}
}

func TestSVGClipRejectUnsupportedAndInvalidInputs(t *testing.T) {
	for _, tc := range []struct{ name, source, want string }{
		{"external", `<svg><rect clip-path="url(https://example.invalid/a.svg#c)"/></svg>`, "local fragment"},
		{"file", `<svg><rect clip-path="url(a.svg#c)"/></svg>`, "local fragment"},
		{"missing", `<svg><rect clip-path="url(#missing)"/></svg>`, "unknown"},
		{"unmatched missing", `<svg><style>.unused{clip-path:url(#missing)}</style></svg>`, "unknown"},
		{"unused missing", `<svg><defs><rect clip-path="url(#missing)"/></defs></svg>`, "unknown"},
		{"override missing", `<svg><style>rect{clip-path:none}</style><rect clip-path="url(#missing)"/></svg>`, "unknown"},
		{"wrong target", `<svg><rect id="r"/><rect clip-path="url(#r)"/></svg>`, "must reference"},
		{"duplicate", `<svg><clipPath id="c"/><clipPath id="c"/></svg>`, "duplicate SVG id"},
		{"self cycle", `<svg><clipPath id="c" clip-path="url(#c)"/></svg>`, "cyclic"},
		{"child cycle", `<svg><clipPath id="c"><rect clip-path="url(#c)"/></clipPath></svg>`, "cyclic"},
		{"indirect cycle", `<svg><clipPath id="a" clip-path="url(#b)"/><clipPath id="b" clip-path="url(#a)"/></svg>`, "cyclic"},
		{"use cycle", `<svg><defs><rect id="r" clip-path="url(#c)"/><clipPath id="c"><use href="#r"/></clipPath></defs></svg>`, "cyclic"},
		{"invalid units", `<svg><clipPath clipPathUnits="invalidUnits"><rect width="1" height="1"/></clipPath></svg>`, "userSpaceOnUse"},
		{"invalid rule", `<svg><clipPath clip-rule="winding"><path d="M0 0H1V1Z"/></clipPath></svg>`, "clip-rule"},
		{"curved union", `<svg><clipPath><circle r="5"/><path d="M0 0V5H5Z"/></clipPath></svg>`, "union"},
		{"group", `<svg><clipPath><g><rect width="5" height="5"/></g></clipPath></svg>`, "single basic"},
		{"text", `<svg><clipPath><text>Go</text></clipPath></svg>`, "single basic"},
		{"use group", `<svg><g id="g"/><clipPath><use href="#g"/></clipPath></svg>`, "single basic"},
		{"use chain", `<svg><rect id="r"/><use id="u" href="#r"/><clipPath><use href="#u"/></clipPath></svg>`, "direct basic"},
		{"percent", `<svg><clipPath><rect width="50%" height="10"/></clipPath></svg>`, "length"},
		{"unknown child attr", `<svg><clipPath><rect width="4" height="4" magic="1"/></clipPath></svg>`, "attribute"},
		{"unknown def attr", `<svg><clipPath viewBox="0 0 4 4"/></svg>`, "attribute"},
		{"empty uri", `<svg><rect clip-path="url(#)"/></svg>`, "local fragment"},
		{"fallback", `<svg><rect clip-path="url(#c) none"/></svg>`, "requires"},
		{"basic CSS shape", `<svg><rect style="clip-path:circle(50%)"/></svg>`, "requires"},
		{"escape", `<svg><rect clip-path="url(#%63)"/></svg>`, "local fragment"},
		{"space", `<svg><rect clip-path="url('#c d')"/></svg>`, "local fragment"},
		{"nonfinite", `<svg><clipPath transform="scale(1e308) scale(1e308)"/></svg>`, "transform"},
		{"namespaced attribute", `<svg xmlns:f="urn:foreign"><clipPath f:transform="scale(2)"/></svg>`, "namespaced attribute"},
		{"namespaced geometry", `<svg xmlns:f="urn:foreign"><clipPath><rect f:width="10"/></clipPath></svg>`, "namespaced attribute"},
		{"xml base", `<svg xml:base="https://example.invalid/"><clipPath id="c"/><rect clip-path="url(#c)"/></svg>`, "xml:base"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			object, err := svg.Read(strings.NewReader(tc.source), svg.DefaultOptions())
			if object != nil || err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("object=%v err=%v; want %s", object, err, tc.want)
			}
		})
	}
}

func TestSVGClipConcurrentImports(t *testing.T) {
	for i := range 12 {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			root := readInstances(t, `<svg viewBox="0 0 100 100"><defs><clipPath id="c"><rect width="5" height="6"/></clipPath></defs><rect width="10" height="10" clip-path="url(#c)"/></svg>`)
			clipBox(t, instanceShapes(root)[0], [4]float64{0, 0, 5, 6})
		})
	}
}

func BenchmarkReadClipInstances(b *testing.B) {
	var source strings.Builder
	source.WriteString(`<svg viewBox="0 0 256 256"><defs><clipPath id="c"><path d="M0 0H14V14H0Z M4 4V10H10V4Z"/></clipPath><rect id="tile" width="16" height="16" fill="red" clip-path="url(#c)"/></defs>`)
	for i := range 256 {
		fmt.Fprintf(&source, `<use href="#tile" x="%d" y="%d"/>`, i%16*16, i/16*16)
	}
	source.WriteString(`</svg>`)
	input := source.String()
	b.ReportAllocs()
	b.SetBytes(int64(len(input)))
	b.ResetTimer()
	for range b.N {
		if _, err := svg.Read(strings.NewReader(input), svg.DefaultOptions()); err != nil {
			b.Fatal(err)
		}
	}
}
