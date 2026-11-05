package svg_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"gomath"
	"gomath/geom"
	"gomath/render"
	"gomath/svg"
	"gomath/text"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"
)

const textBBoxGradientData = "../tests/data/svg/text-bbox-gradients/"

func textBBoxGradientFile(t testing.TB, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(textBBoxGradientData + name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func textBBoxGradientRead(t testing.TB, source []byte, width float64, font string) *gomath.Object {
	t.Helper()
	opt := svg.Options{Width: width}
	if font == "kern" || font == "zero" {
		name := "../tests/data/svg/text-kern.ttf"
		if font == "zero" {
			name = textBBoxGradientData + "zero-advance.ttf"
		}
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		f, err := text.Parse(data)
		if err != nil {
			t.Fatal(err)
		}
		opt.FontResolver = func(string, text.FontStyle) (*text.Font, error) { return f, nil }
	}
	object, err := svg.Read(bytes.NewReader(source), opt)
	if err != nil {
		t.Fatal(err)
	}
	if err := object.Validate(); err != nil {
		t.Fatal(err)
	}
	return object
}

func TestSVGTextBBoxGradientsFontToolsOracle(t *testing.T) {
	var oracle struct {
		Cases []struct {
			Name, Font string
			Glyphs     []struct {
				Rune, Kind string
				Matrix     [6]float64
				StopAlpha  float64 `json:"stop_alpha"`
				Colors     [2][3]float64
				Solid      []float64
			}
		}
	}
	if err := json.Unmarshal(textBBoxGradientFile(t, "oracle.json"), &oracle); err != nil {
		t.Fatal(err)
	}
	for _, tc := range oracle.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			root := textBBoxGradientRead(t, textBBoxGradientFile(t, tc.Name+".svg"), 128, tc.Font)
			glyphs := instanceShapes(root)
			if len(glyphs) != len(tc.Glyphs) {
				t.Fatalf("glyph count %d want %d", len(glyphs), len(tc.Glyphs))
			}
			for i, want := range tc.Glyphs {
				glyph := glyphs[i]
				if glyph.Name != want.Rune {
					t.Fatalf("glyph %d %q want %q", i, glyph.Name, want.Rune)
				}
				gradient := glyph.Style.FillGradient
				if want.Kind == "" {
					if gradient != nil {
						t.Fatal("solid/none/zero-width fill acquired gradient", i)
					}
					expected := gomath.Transparent
					if len(want.Solid) == 3 {
						expected = gomath.RGB(want.Solid[0], want.Solid[1], want.Solid[2])
					}
					if glyph.Style.Fill != expected {
						t.Fatal("solid/currentColor override", i, glyph.Style.Fill, expected)
					}
					continue
				}
				if gradient == nil {
					t.Fatal("missing gradient", i)
				}
				if (gradient.Kind() == gomath.RadialGradient) != (want.Kind == "radial") {
					t.Fatal("gradient kind", i)
				}
				matrix := gradient.Matrix()
				for j, v := range matrix {
					if math.Abs(v-want.Matrix[j]) > 1e-8 {
						t.Fatalf("glyph %d matrix %v fontTools oracle %v", i, matrix, want.Matrix)
					}
				}
				for j, stop := range gradient.Stops() {
					c := want.Colors[j]
					expected := gomath.Color{R: c[0], G: c[1], B: c[2], A: want.StopAlpha}
					if stop.Color != expected {
						t.Fatalf("glyph %d stop %d %v want %v", i, j, stop.Color, expected)
					}
				}
			}
		})
	}
}

func textBBoxGradientCompare(t *testing.T, actual *image.RGBA, filename string) {
	t.Helper()
	reference, err := png.Decode(bytes.NewReader(textBBoxGradientFile(t, filename)))
	if err != nil {
		t.Fatal(err)
	}
	if actual.Bounds() != reference.Bounds() {
		t.Fatal("raster size mismatch")
	}
	var total, maximum, over, covered int
	for y := range 96 {
		for x := range 128 {
			a, b := actual.RGBAAt(x, y), color.RGBAModel.Convert(reference.At(x, y)).(color.RGBA)
			covered += int(b.A)
			pixelMax := 0
			for _, d := range []int{int(a.R) - int(b.R), int(a.G) - int(b.G), int(a.B) - int(b.B), int(a.A) - int(b.A)} {
				d = int(math.Abs(float64(d)))
				total += d
				maximum = max(maximum, d)
				pixelMax = max(pixelMax, d)
			}
			if pixelMax > 8 {
				over++
			}
		}
	}
	mean, fraction := float64(total)/(128*96*4), float64(over)/(128*96)
	t.Logf("fontTools+Cairo %s MAE=%.9g max=%d fraction>8=%.9g", filename, mean, maximum, fraction)
	// Existing fixture tests/thresholds are unchanged. These independent text
	// paths constrain both color and original glyph coverage at SS1 and SS4.
	limit := 24
	if strings.Contains(filename, "ss1") {
		limit = 40
	}
	if mean > .12 || maximum > limit || fraction > .005 || covered < 1000 {
		t.Fatal("independent text bbox gradient reference mismatch")
	}
}

func TestSVGTextBBoxGradientsIndependentCairoCamera(t *testing.T) {
	for _, name := range []string{"cells-middle", "tspan-whole-kern", "relative-kern", "chunks-fonts", "visibility-overrides", "affine-local", "radial-reflection", "use-chain", "nested-clips", "template-alpha", "zero-width"} {
		for _, ss := range []int{1, 4} {
			t.Run(fmt.Sprintf("%s/ss%d", name, ss), func(t *testing.T) {
				font := "regular"
				if name == "tspan-whole-kern" || name == "relative-kern" {
					font = "kern"
				}
				if name == "zero-width" {
					font = "zero"
				}
				root := textBBoxGradientRead(t, textBBoxGradientFile(t, name+".svg"), 8, font)
				got := renderBBoxFrame(t, root, ss, nil)
				textBBoxGradientCompare(t, got, fmt.Sprintf("%s-cairo-ss%d.png", name, ss))
				before := root.Copy()
				root.ApplyMatrix(geom.Translation(geom.V(10, 20)).Mul(geom.RotationZ(.3)))
				camera := gomath.Camera2D{Center: geom.V(10, 20), Rotation: .3, FrameWidth: 8}
				moved := renderBBoxFrame(t, root, ss, &camera)
				for i, v := range got.Pix {
					if d := int(v) - int(moved.Pix[i]); d < -1 || d > 1 {
						t.Fatalf("camera delta %d at %d", d, i)
					}
				}
				if !bytes.Equal(got.Pix, renderBBoxFrame(t, before, ss, nil).Pix) {
					t.Fatal("transform changed saved import")
				}
			})
		}
	}
}

func TestSVGTextBBoxGradientsAnimationInteriorsOwnership(t *testing.T) {
	source := textBBoxGradientFile(t, "affine-local.svg")
	root := textBBoxGradientRead(t, source, 8, "regular")
	before := root.Copy()
	children := instanceShapes(root)
	target := root.Copy()
	normalize := geom.Translation(geom.V(-4, 3)).Mul(geom.Scaling(1.0/16, -1.0/16, 1))
	inverse := geom.Scaling(16, -16, 1).Mul(geom.Translation(geom.V(4, -3)))
	affine := geom.Identity()
	affine[0], affine[1], affine[3] = .82, -.12, 10
	affine[4], affine[5], affine[7] = .10, 1.05, -3
	target.ApplyMatrix(normalize.Mul(affine).Mul(inverse))
	scene := gomath.NewScene()
	scene.Add(root)
	if err := scene.Play(1, gomath.Transform(root, target).WithRate(gomath.Linear)); err != nil {
		t.Fatal(err)
	}
	for i, child := range instanceShapes(root) {
		if child != children[i] {
			t.Fatal("animation replaced child identity")
		}
	}
	for _, tick := range []int{0, 20, 50, 80, 100} {
		t.Run(fmt.Sprint(tick), func(t *testing.T) {
			t.Parallel()
			frame, err := scene.Sample(float64(tick) / 100)
			if err != nil {
				t.Fatal(err)
			}
			object := frame.Find(root.ID)
			for _, ss := range []int{1, 4} {
				textBBoxGradientCompare(t, renderBBoxFrame(t, object, ss, nil), fmt.Sprintf("animated-%03d-cairo-ss%d.png", tick, ss))
			}
			saved := instanceShapes(object)[0]
			old := saved.Style.FillGradient.Matrix()
			saved.Shift(geom.V(1, 2))
			next, err := scene.Sample(float64(tick) / 100)
			if err != nil {
				t.Fatal(err)
			}
			if instanceShapes(next.Find(root.ID))[0].Style.FillGradient.Matrix() != old {
				t.Fatal("sample snapshot mutated another sample")
			}
		})
	}
	fresh := textBBoxGradientRead(t, source, 8, "regular")
	a, b := instanceShapes(before), instanceShapes(fresh)
	for i := range a {
		if !reflect.DeepEqual(a[i].Path, b[i].Path) || a[i].Style.FillGradient.Matrix() != b[i].Style.FillGradient.Matrix() {
			t.Fatal("authored transform changed original import")
		}
	}
}

func TestSVGTextBBoxGradientsConcurrentImportsUseOwnership(t *testing.T) {
	source := textBBoxGradientFile(t, "use-chain.svg")
	for n := range 12 {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			t.Parallel()
			root := textBBoxGradientRead(t, source, 128, "regular")
			glyphs := instanceShapes(root)
			// Each clone owns glyph paths and paints in its own font/placement scope.
			if glyphs[0].ID == glyphs[3].ID || glyphs[0] == glyphs[3] || glyphs[0].Style.FillGradient == glyphs[3].Style.FillGradient {
				t.Fatal("use clones share mutable geometry/paint frame")
			}
			other := glyphs[3].Copy()
			glyphs[0].Shift(geom.V(20, 30))
			if !reflect.DeepEqual(glyphs[3].Path, other.Path) || glyphs[3].Style.FillGradient.Matrix() != other.Style.FillGradient.Matrix() {
				t.Fatal("use scope leaked")
			}
			stops := glyphs[3].Style.FillGradient.Stops()
			stops[0].Color = gomath.White
			if glyphs[3].Style.FillGradient.Stops()[0].Color == gomath.White {
				t.Fatal("gradient exposes stop ownership")
			}
		})
	}
}

func TestSVGTextBBoxGradientsUnsupportedAtomic(t *testing.T) {
	defs := `<defs><linearGradient id="g"><stop stop-color="red"/><stop offset="1" stop-color="blue"/></linearGradient><clipPath id="c" clipPathUnits="objectBoundingBox"><rect width="1" height="1"/></clipPath></defs>`
	for _, body := range []string{
		`<text fill="url(#g)">A<tspan transform="scale(2)">V</tspan></text>`,
		`<g clip-path="url(#c)"><svg><text fill="url(#g)">AV</text></svg></g>`,
		`<defs><symbol id="s" overflow="visible"><text fill="url(#g)">AV</text></symbol></defs><use href="#s" clip-path="url(#c)"/>`,
		`<defs><symbol id="s" overflow="visible"><text>AV</text></symbol><g id="tile"><use href="#s"/></g></defs><use href="#tile" fill="url(#g)" clip-path="url(#c)"/>`,
		`<defs><g id="tile"><linearGradient id="local"><stop/><stop offset="1"/></linearGradient><text fill="url(#local)">A</text></g></defs><use href="#tile"/>`,
		`<text fill="url(#missing)">AV</text>`,
		`<text transform="scale(0 1)" fill="url(#g)">AV</text>`,
		`<text x="1e10" fill="url(#g)">AV</text>`,
		`<text font-family="Missing" fill="url(#g)">AV</text>`,
		`<text fill="url(#g)" stroke="url(#g)">AV</text>`,
	} {
		object, err := svg.Read(strings.NewReader(`<svg viewBox="0 0 128 96">`+defs+`<rect width="10" height="10"/>`+body+`</svg>`), svg.Options{Width: 128})
		if err == nil || object != nil {
			t.Fatalf("unsupported branch produced partial output: %s: %v", body, err)
		}
	}
	for _, value := range []string{"", "   "} {
		root := textBBoxGradientRead(t, []byte(`<svg viewBox="0 0 128 96">`+defs+`<text xml:space="preserve" fill="url(#g)">`+value+`</text></svg>`), 128, "regular")
		if len(instanceShapes(root)) != 0 {
			t.Fatal("empty/whitespace text invented painted geometry")
		}
	}
}

func BenchmarkSVGTextBBoxGradientsImportRender(b *testing.B) {
	for _, name := range []string{"cells-middle", "nested-clips", "use-chain"} {
		source := textBBoxGradientFile(b, name+".svg")
		for _, ss := range []int{1, 4} {
			b.Run(fmt.Sprintf("%s_128x96_SS%d", name, ss), func(b *testing.B) {
				config := render.DefaultConfig()
				config.Width, config.Height, config.FrameWidth, config.Workers, config.Supersampling = 128, 96, 8, 1, ss
				raster, err := render.NewRasterizer(config)
				if err != nil {
					b.Fatal(err)
				}
				b.ReportAllocs()
				b.SetBytes(int64(len(source)))
				b.ResetTimer()
				for range b.N {
					root, err := svg.Read(bytes.NewReader(source), svg.Options{Width: 8})
					if err != nil {
						b.Fatal(err)
					}
					if _, err := raster.Render(&gomath.Frame{Objects: []*gomath.Object{root}}); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

func TestSVGTextBBoxGradientsInheritedFillKeywordsAndUserSpace(t *testing.T) {
	source := `<svg viewBox="0 0 100 100"><defs><linearGradient id="g"><stop stop-color="red"/><stop offset="1" stop-color="blue"/></linearGradient><linearGradient id="u" gradientUnits="userSpaceOnUse" x2="100"><stop stop-color="red"/><stop offset="1" stop-color="blue"/></linearGradient></defs><g fill="url(#g)" color="red"><text x="10" y="50" font-size="20" fill="inherit">A<tspan fill="initial">V</tspan><tspan fill="unset">g</tspan><tspan fill="none">j</tspan><tspan fill="url(#u)">A</tspan></text><text x="10" y="75" fill="currentColor" color="blue">AV</text></g></svg>`
	root := readInstances(t, source)
	glyphs := instanceShapes(root)
	if len(glyphs) != 7 {
		t.Fatal("missing mixed-paint glyphs")
	}
	if glyphs[0].Style.FillGradient == nil || glyphs[2].Style.FillGradient == nil || glyphs[0].Style.FillGradient.Matrix() != glyphs[2].Style.FillGradient.Matrix() {
		t.Fatal("inherited/unset fills lost shared text frame")
	}
	if glyphs[1].Style.FillGradient != nil || glyphs[1].Style.Fill != gomath.Black || glyphs[3].Style.FillGradient != nil || glyphs[3].Style.Fill != gomath.Transparent {
		t.Fatal("initial/none failed to clear paintserver")
	}
	// The user-space span keeps viewport units; the enclosing bbox paint still
	// includes this span's cells and each solid/none override.
	gradientAt(t, glyphs[4], 25, 50, gomath.RGB(.75, 0, .25))
	for _, g := range glyphs[5:] {
		if g.Style.FillGradient != nil || g.Style.Fill != gomath.RGB(0, 0, 1) {
			t.Fatal("inherited currentColor was frozen before descendant color")
		}
	}
	_, metrics := textRun(t, "AVgjA", text.Regular, 20)
	matrix := glyphs[0].Style.FillGradient.Matrix()
	if math.Abs(matrix[0]-metrics.Width) > 1e-8 || math.Abs(matrix[4]-(-40)) > 1e-8 {
		t.Fatal("paint overrides changed text bbox", matrix, metrics)
	}
}

func TestSVGTextBBoxGradientsUseGroupAndExistingSymbolMapping(t *testing.T) {
	defs := `<defs><linearGradient id="g"><stop stop-color="red"/><stop offset="1" stop-color="blue"/></linearGradient><g id="tile"><text x="4" y="30">AV</text><text x="54" y="60">AV</text></g><symbol id="symbol" viewBox="0 0 100 50" overflow="visible"><text x="10" y="30">AV</text></symbol></defs>`
	source := func(body string) []byte { return []byte(`<svg viewBox="0 0 128 96">` + defs + body + `</svg>`) }
	root := textBBoxGradientRead(t, source(`<use href="#tile" x="6" y="8" font-size="20" fill="url(#g)"/>`), 128, "regular")
	glyphs := instanceShapes(root)
	if len(glyphs) != 4 {
		t.Fatal("use group text glyphs missing")
	}
	a, b := glyphs[0].Style.FillGradient.Matrix(), glyphs[2].Style.FillGradient.Matrix()
	if math.Abs(a[0]-b[0]) > 1e-8 || math.Abs(b[4]-a[4]-50) > 1e-8 || math.Abs(b[5]-a[5]+30) > 1e-8 {
		t.Fatal("use fill became an aggregate group bbox", a, b)
	}
	symbol := textBBoxGradientRead(t, source(`<use href="#symbol" x="6" y="8" width="80" height="40" font-size="20" fill="url(#g)"/>`), 128, "regular")
	explicit := textBBoxGradientRead(t, source(`<g transform="translate(6 8) scale(.8)" font-size="20" fill="url(#g)"><text x="10" y="30">AV</text></g>`), 128, "regular")
	aa, bb := instanceShapes(symbol), instanceShapes(explicit)
	for i := range aa {
		compareBBoxPaths(t, aa[i].Path, bb[i].Path)
		if aa[i].Style.FillGradient.Matrix() != bb[i].Style.FillGradient.Matrix() {
			t.Fatal("existing symbol viewport mapping changed text's local bbox")
		}
	}
}
