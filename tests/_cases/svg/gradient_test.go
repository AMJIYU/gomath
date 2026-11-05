package svg_test

import (
	"fmt"
	"gomath"
	"gomath/geom"
	"gomath/svg"
	"math"
	"reflect"
	"strings"
	"sync"
	"testing"
)

const gradientRedBlue = `<stop stop-color="red"/><stop offset="1" stop-color="blue"/>`

func gradientAt(t *testing.T, o *gomath.Object, x, y float64, want gomath.Color) {
	t.Helper()
	if o.Style.FillGradient == nil {
		t.Fatal("missing native gradient", o.Name)
	}
	got, err := o.Style.FillGradient.ColorAt(geom.V(x-50, 50-y))
	if err != nil {
		t.Fatal(err)
	}
	for i, pair := range [][2]float64{{got.R, want.R}, {got.G, want.G}, {got.B, want.B}, {got.A, want.A}} {
		if math.Abs(pair[0]-pair[1]) > 1e-8 {
			t.Fatalf("%s at (%g,%g) channel %d: %v want %v", o.Name, x, y, i, got, want)
		}
	}
}

func TestSVGGradientBoundingBoxAffineAndUserSpace(t *testing.T) {
	root := readInstances(t, `<svg viewBox="0 0 100 100"><defs transform="translate(900 900)">
 <linearGradient id="box" gradientTransform="matrix(1 .25 .5 1 0 0)">`+gradientRedBlue+`</linearGradient>
 <linearGradient id="user" gradientUnits="userSpaceOnUse" x1="10%" y1="20%" x2="90%" y2="20%" gradientTransform="translate(2 3)">`+gradientRedBlue+`</linearGradient>
 <radialGradient id="rad" cx="50%" cy="50%" r="50%" fx=".5" fy=".5" fr="0" gradientTransform="translate(.1 0)">`+gradientRedBlue+`</radialGradient>
 </defs><g transform="translate(3 4) matrix(1 .2 .3 1 0 0)">
 <rect id="a" x="10" y="20" width="40" height="60" fill="url(#box)"/>
 <rect id="b" width="90" height="90" fill="url(#user)"/>
 <rect id="r" x="10" y="20" width="40" height="60" fill="url(#rad)"/>
 </g></svg>`)
	local := geom.Translation(geom.V(3, 4)).Mul(geom.Mat4{1, .3, 0, 0, .2, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1})
	// Independent explicit matrix products: local * bbox * gradientTransform.
	for _, tc := range []struct {
		name  string
		point geom.Vec3
		want  gomath.Color
	}{
		{"a", geom.V(10+40*.5, 20+60*.125), gomath.RGB(.5, 0, .5)},
		{"b", geom.V(52, 23), gomath.RGB(.5, 0, .5)},
		{"r", geom.V(10+40*.6, 20+60*.5), gomath.RGB(1, 0, 0)},
		{"r", geom.V(10+40*.85, 20+60*.5), gomath.RGB(.5, 0, .5)},
	} {
		point := local.Apply(tc.point)
		gradientAt(t, cssObject(t, root, tc.name), point.X, point.Y, tc.want)
	}
}

func TestSVGGradientStopCascadeTemplateCurrentColorAndOpacity(t *testing.T) {
	root := readInstances(t, `<svg viewBox="0 0 100 100" color="blue" xmlns:xlink="http://www.w3.org/1999/xlink">
 <style>.defs > linearGradient {color:green} .defs > linearGradient > stop {stop-color:yellow}
 .s {stop-color:magenta} #chosen {stop-color:red} .host stop {stop-color:white}
 rect.paint {fill:url(#template);fill-opacity:.5}</style>
 <defs class="defs" stop-opacity=".1">
 <linearGradient id="base" color="red" x1="0" x2="1"><stop class="s" stop-color="blue"/>
 <stop id="chosen" offset=".5" stop-color="blue" style="stop-color:currentColor;stop-opacity:.4"/>
 <stop offset="1" stop-color="red" style="stop-color:blue"/></linearGradient>
 <linearGradient id="template" xlink:href="#base" color="cyan" stop-opacity=".2"/>
 <linearGradient id="own" href="#base" stop-color="red" stop-opacity=".3">
 <stop stop-color="inherit" stop-opacity="inherit"/><stop offset="1"/></linearGradient>
 </defs><g class="host" color="white" opacity=".6"><rect id="paint" class="paint" width="100" height="100"/></g>
 <rect id="original" width="10" height="10" fill="url(#base)"/>
 <rect id="explicit" width="10" height="10" fill="url(#own)"/>
 </svg>`)
	g := cssObject(t, root, "paint").Style.FillGradient
	want := []gomath.GradientStop{{Offset: 0, Color: gradientAlpha(gomath.RGB(1, 0, 1), .5)}, {Offset: .5, Color: gradientAlpha(gomath.RGB(0, .5, 0), .2)}, {Offset: 1, Color: gradientAlpha(gomath.RGB(0, 0, 1), .5)}}
	// The template's author selector beats presentation color; stop-color and
	// stop-opacity themselves do not inherit unless explicitly requested.
	if !reflect.DeepEqual(g.Stops(), want) {
		t.Fatalf("template stops %v want %v", g.Stops(), want)
	}
	original := cssObject(t, root, "original").Style.FillGradient.Stops()
	if original[0].Color != gomath.RGB(1, 1, 0) || original[1].Color != gradientAlpha(gomath.RGB(0, .5, 0), .4) {
		t.Fatal("own stops lost definition-tree selectors", original)
	}
	explicit := cssObject(t, root, "explicit").Style.FillGradient.Stops()
	// Author rule overrides stop presentation inherit; opacity inherit remains .3.
	if explicit[0].Color != gradientAlpha(gomath.RGB(1, 1, 0), .3) || explicit[1].Color != gomath.RGB(1, 1, 0) {
		t.Fatal("stop property inheritance", explicit)
	}
}

func TestSVGGradientUseSymbolTextAndOwnership(t *testing.T) {
	root := readInstances(t, `<svg viewBox="0 0 100 100"><defs>
 <linearGradient id="g" gradientUnits="userSpaceOnUse" x2="100%">`+gradientRedBlue+`</linearGradient>
 <rect id="tile" width="20" height="10"/>
 <symbol id="s" viewBox="0 0 10 10" overflow="visible"><rect id="symrect" width="10" height="10"/></symbol>
 </defs><g fill="url(#g)"><use href="#tile" x="5"/><use href="#tile" x="40"/>
 <use href="#s" x="60" y="20" width="20" height="20"/>
 <text id="text" x="0" y="90" font-size="10">Go<tspan fill-opacity=".2">A</tspan></text></g></svg>`)
	shapes := instanceShapes(root)
	gradientAt(t, shapes[0], 15, 5, gomath.RGB(.9, 0, .1))
	gradientAt(t, shapes[1], 50, 5, gomath.RGB(.9, 0, .1))
	gradientAt(t, cssObject(t, root, "symrect"), 70, 30, gomath.RGB(.5, 0, .5))
	if shapes[0].ID == shapes[1].ID {
		t.Fatal("shared instance IDs")
	}
	before := shapes[1].Style.FillGradient.Matrix()
	copied := root.Copy()
	stops := shapes[0].Style.FillGradient.Stops()
	stops[0].Color = gomath.White
	shapes[0].Shift(geom.V(4, 3)).SetFill(gomath.White)
	if shapes[1].Style.FillGradient.Matrix() != before || shapes[1].Style.FillGradient.Stops()[0].Color != gomath.RGB(1, 0, 0) {
		t.Fatal("gradient instances alias mutable state")
	}
	if instanceShapes(copied)[0].Style.FillGradient == nil {
		t.Fatal("snapshot paint cleared")
	}
	textShapes := instanceShapes(cssObject(t, root, "text"))
	if len(textShapes) != 3 {
		t.Fatal("text outlines lost", len(textShapes))
	}
	for i, o := range textShapes {
		if o.Style.FillGradient == nil {
			t.Fatal("text gradient absent")
		}
		alpha := 1.0
		if i == 2 {
			alpha = .2
		}
		gradientAt(t, o, 50, 90, gradientAlpha(gomath.RGB(.5, 0, .5), alpha))
	}
	// Sharing immutable stops across concurrent snapshot reads must preserve
	// independent frames. All goroutines only read the authored snapshot.
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 8 {
				_, err := instanceShapes(copied)[0].Style.FillGradient.ColorAt(geom.V(0, 0))
				if err != nil {
					t.Error(err)
				}
			}
		}()
	}
	wg.Wait()
}

func TestSVGGradientStopNormalizationAndDegenerates(t *testing.T) {
	for _, tc := range []struct {
		name, definition string
		solid            *gomath.Color
		offsets          []float64
	}{
		{"offset clamp", `<linearGradient id="g"><stop offset="-1" stop-color="red"/><stop offset=".7" stop-color="blue"/><stop offset=".2" stop-color="green"/><stop offset="120%"/></linearGradient>`, nil, []float64{0, .7, .7, 1}},
		{"endpoints", `<linearGradient id="g"><stop offset=".2"/><stop offset=".8"/></linearGradient>`, nil, []float64{0, .2, .8, 1}},
		{"empty", `<linearGradient id="g"/>`, colorPtr(gomath.Transparent), nil},
		{"single", `<linearGradient id="g"><stop stop-color="red" stop-opacity=".6"/></linearGradient>`, colorPtr(gradientAlpha(gomath.RGB(1, 0, 0), .6)), nil},
		{"zero linear", `<linearGradient id="g" x2="0">` + gradientRedBlue + `</linearGradient>`, colorPtr(gomath.RGB(0, 0, 1)), nil},
		{"zero radial", `<radialGradient id="g" r="0">` + gradientRedBlue + `</radialGradient>`, colorPtr(gomath.RGB(0, 0, 1)), nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := readInstances(t, `<svg viewBox="0 0 100 100"><defs>`+tc.definition+`</defs><rect id="r" width="100" height="100" fill="url(#g)" fill-opacity=".5"/></svg>`)
			o := cssObject(t, root, "r")
			if tc.solid != nil {
				want := *tc.solid
				want.A *= .5
				if o.Style.FillGradient != nil || o.Style.Fill != want {
					t.Fatal("degenerate solid mismatch", o.Style)
				}
			} else {
				stops := o.Style.FillGradient.Stops()
				if len(stops) != len(tc.offsets) {
					t.Fatal(stops)
				}
				for i, v := range tc.offsets {
					if stops[i].Offset != v {
						t.Fatal(stops)
					}
				}
			}
		})
	}
	// Radial user-space percentages use the normalized viewport diagonal.
	root := readInstances(t, `<svg viewBox="0 0 100 50"><radialGradient id="g" gradientUnits="userSpaceOnUse" cx="0" cy="0" r="100%">`+gradientRedBlue+`</radialGradient><rect id="r" width="100" height="50" fill="url(#g)"/></svg>`)
	radius := math.Hypot(100, 50) / math.Sqrt2
	got, err := cssObject(t, root, "r").Style.FillGradient.ColorAt(geom.V(radius/2-50, 25))
	if err != nil || math.Abs(got.R-.5) > 1e-8 {
		t.Fatal(got, err)
	}
}
func colorPtr(c gomath.Color) *gomath.Color { return &c }

func TestSVGGradientRejectsUnsupportedAndInvalidInputs(t *testing.T) {
	defs := `<linearGradient id="g">` + gradientRedBlue + `</linearGradient>`
	for _, tc := range []struct{ name, body, want string }{
		{"unknown", `<rect fill="url(#no)"/>`, "unknown"},
		{"unmatched", `<style>.absent {fill:url(#no)}</style>`, "unknown"},
		{"overridden", `<rect fill="url(#no)" style="fill:red"/>`, "unknown"},
		{"wrong type", `<rect id="g"/><rect fill="url(#g)"/>`, "must reference"},
		{"external", `<rect fill="url(file.svg#g)"/>`, "local"},
		{"fallback", defs + `<rect fill="url(#g) red"/>`, "requires"},
		{"stroke", defs + `<rect stroke="url(#g)"/>`, "stroke"},
		{"cycle", `<linearGradient id="a" href="#b"/><linearGradient id="b" href="#a"/>`, "cyclic"},
		{"external href", `<linearGradient href="file.svg#g"/>`, "href"},
		{"mixed href", `<linearGradient id="a"/><radialGradient href="#a"/>`, "mixed-type"},
		{"missing href", `<linearGradient href="#a"/>`, "unknown"},
		{"duplicate", defs + `<linearGradient id="g"/>`, "duplicate"},
		{"repeat", `<linearGradient spreadMethod="repeat"/>`, "spreadMethod"},
		{"reflect", `<radialGradient spreadMethod="reflect"/>`, "spreadMethod"},
		{"focus", `<radialGradient fx=".7"/>`, "concentric"},
		{"focus radius", `<radialGradient fr=".2"/>`, "concentric"},
		{"negative r", `<radialGradient r="-1"/>`, "nonnegative"},
		{"nan", `<radialGradient r="NaN"/>`, "finite"},
		{"bbox units", `<linearGradient x2="10px"/>`, "fractions"},
		{"too large", `<linearGradient x1="1e10"/>`, "coordinate"},
		{"empty coordinate", `<linearGradient x1=""/>`, "empty"},
		{"bad units", `<linearGradient gradientUnits="x"/>`, "gradientUnits"},
		{"singular", `<linearGradient gradientTransform="scale(0)"/>`, "singular"},
		{"unsupported attr", `<linearGradient transform="scale(2)"/>`, "attribute"},
		{"hex offset", `<linearGradient><stop offset="0x1p0"/></linearGradient>`, "offset"},
		{"quoted href", `<linearGradient id="a"/><linearGradient href="'#a'"/>`, "local fragment"},
		{"bad stop offset", `<linearGradient><stop offset="2px"/></linearGradient>`, "offset"},
		{"none stop", `<linearGradient><stop stop-color="none"/></linearGradient>`, "stop-color"},
		{"bad alpha", `<linearGradient><stop stop-opacity="2"/></linearGradient>`, "stop-opacity"},
		{"unknown CSS", `<style>stop {color-interpolation:linearRGB}</style>`, "sRGB"},
		{"important", `<style>stop {stop-color:red!important}</style>`, "important"},
		{"wrong child", `<linearGradient><rect/></linearGradient>`, "child"},
		{"stop child", `<linearGradient><stop><rect/></stop></linearGradient>`, "empty"},
		{"orphan stop", `<stop/>`, "direct"},
		{"clone local", `<g id="tile"><defs>` + defs + `</defs><rect width="2" height="2" fill="url(#g)"/></g><use href="#tile"/>`, "clone-local"},
		{"xml base", defs + `<g xml:base="file:///"/>`, "xml:base"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, err := svg.Read(strings.NewReader(`<svg>`+tc.body+`</svg>`), svg.DefaultOptions())
			if root != nil || err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("root=%v err=%v want %q", root, err, tc.want)
			}
		})
	}
}

func BenchmarkReadGradientInstances(b *testing.B) {
	var source strings.Builder
	source.WriteString(`<svg viewBox="0 0 256 256"><defs><radialGradient id="g">` + gradientRedBlue + `</radialGradient><rect id="tile" width="12" height="8"/></defs><g fill="url(#g)">`)
	for i := range 256 {
		fmt.Fprintf(&source, `<use href="#tile" x="%d" y="%d"/>`, (i%16)*16, (i/16)*16)
	}
	source.WriteString(`</g></svg>`)
	data := source.String()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := svg.Read(strings.NewReader(data), svg.DefaultOptions()); err != nil {
			b.Fatal(err)
		}
	}
}

func gradientAlpha(c gomath.Color, a float64) gomath.Color { c.A = a; return c }

func TestSVGGradientCSSPaintResetAlphaAndCurveBounds(t *testing.T) {
	root := readInstances(t, `<svg viewBox="0 0 100 100"><defs>
 <linearGradient id="g" x1="0" y1="0" x2="0" y2="1"><stop stop-color="#ff000080" stop-opacity=".5"/><stop offset="1" stop-color="blue"/></linearGradient>
 <linearGradient id="override" href="#g" x1="0" y1="0" x2="1" y2="0"><stop stop-color="white"/><stop offset="1" stop-color="black"/></linearGradient>
 </defs><g fill="url(#g)">
 <path id="curve" d="M0 0C0 100 100 100 100 0Z" fill-opacity=".5"/>
 <rect id="solid" width="1" height="1" fill="currentColor" color="red"/>
 <rect id="initial" width="1" height="1" fill="initial"/>
 <rect id="none" width="1" height="1" fill="none"/>
 <rect id="unset" width="1" height="1" fill="unset"/>
 <rect id="ownstops" width="100" height="100" fill="url(#override)"/>
 <line id="zero" x1="3" x2="3" y2="10" stroke="blue"/>
 </g></svg>`)
	// Exact cubic y maximum is 75, not the control polygon's 100.
	gradientAt(t, cssObject(t, root, "curve"), 50, 37.5, gradientAlpha(gomath.RGB(.5, 0, .5), (.5*128/255+1)*.25))
	gradientAt(t, cssObject(t, root, "ownstops"), 50, 25, gomath.RGB(.5, .5, .5))
	for _, tc := range []struct {
		name string
		fill gomath.Color
	}{{"solid", gomath.RGB(1, 0, 0)}, {"initial", gomath.Black}, {"none", gomath.Transparent}, {"zero", gomath.Transparent}} {
		o := cssObject(t, root, tc.name)
		if o.Style.FillGradient != nil || o.Style.Fill != tc.fill {
			t.Fatalf("%s failed to clear paintserver", tc.name)
		}
	}
	if cssObject(t, root, "unset").Style.FillGradient == nil {
		t.Fatal("unset failed to inherit fill paintserver")
	}
	// Standard href wins over xlink:href, including an invalid overridden value.
	r := readInstances(t, `<svg xmlns:xlink="http://www.w3.org/1999/xlink" viewBox="0 0 100 100"><linearGradient id="a">`+gradientRedBlue+`</linearGradient><linearGradient id="b" href="#a" xlink:href="external.svg#x"/><rect width="100" height="100" fill="url(#b)"/></svg>`)
	gradientAt(t, instanceShapes(r)[0], 25, 25, gomath.RGB(.75, 0, .25))
}

func TestSVGGradientRepeatedTemplateClippedGroupsAndBitmap(t *testing.T) {
	uri := imageURI(t, "embedded.png", "png")
	root := readInstances(t, `<svg viewBox="0 0 100 100"><defs>
 <linearGradient id="base">`+gradientRedBlue+`</linearGradient><linearGradient id="g" href="#base"/>
 <clipPath id="clip"><rect width="20" height="20"/></clipPath>
 <g id="tile" clip-path="url(#clip)"><rect width="30" height="30"/></g>
 </defs><g fill="url(#g)"><use href="#tile" x="3"/><use href="#tile" x="40"/>
 <image id="bitmap" href="`+uri+`" width="10" height="10"/></g></svg>`)
	shapes := instanceShapes(root)
	gradientAt(t, shapes[0], 18, 15, gomath.RGB(.5, 0, .5))
	gradientAt(t, shapes[1], 55, 15, gomath.RGB(.5, 0, .5))
	image := cssObject(t, root, "bitmap")
	if image.Bitmap == nil || image.Style.FillGradient != nil {
		t.Fatal("image was tinted or lost immutable asset")
	}
	before := image.Bitmap.RGBAAt(0, 0)
	image.SetFill(gomath.White)
	if image.Bitmap.RGBAAt(0, 0) != before {
		t.Fatal("paint mutated asset")
	}
}
