package svg_test

import (
	"bytes"
	"encoding/json"
	"gomath"
	"gomath/geom"
	"gomath/render"
	"gomath/svg"
	"image/color"
	"image/png"
	"math"
	"os"
	"strings"
	"testing"
)

func TestTextFontToolsIndependentGeometry(t *testing.T) {
	var reference struct {
		Cases []struct {
			Style, Value  string
			Size, Advance float64
			Glyphs        []struct {
				Rune   string
				Bounds [4]float64
			}
		}
	}
	data, err := os.ReadFile("../tests/data/svg/text-fonttools.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &reference); err != nil {
		t.Fatal(err)
	}
	for _, tc := range reference.Cases {
		t.Run(tc.Style, func(t *testing.T) {
			maxError := 0.0
			attrs := ""
			if strings.Contains(tc.Style, "bold") {
				attrs += ` font-weight="bold"`
			}
			if strings.Contains(tc.Style, "italic") {
				attrs += ` font-style="italic"`
			}
			root := textRead(t, `<text x="50" y="50" font-size="24" text-anchor="end"`+attrs+`>`+tc.Value+`</text>`, nil)
			glyphs := instanceShapes(root)
			if len(glyphs) != len(tc.Glyphs) || tc.Size != 24 {
				t.Fatal("independent glyph count/size mismatch")
			}
			for i, glyph := range glyphs {
				want := tc.Glyphs[i]
				b := glyph.Bounds()
				got := [4]float64{b.Min.X + tc.Advance, b.Min.Y, b.Max.X + tc.Advance, b.Max.Y}
				if glyph.Name != want.Rune || !b.Valid {
					t.Fatal("independent glyph missing", want.Rune)
				}
				for j := range got {
					maxError = math.Max(maxError, math.Abs(got[j]-want.Bounds[j]))
					// sfnt uses 26.6 fixed coordinates at 1024 ppem; composed
					// bold-italic accents round slightly versus design units.
					if math.Abs(got[j]-want.Bounds[j]) > .001 {
						t.Fatalf("%s bounds %v want %v (advance %g)", want.Rune, got, want.Bounds, tc.Advance)
					}
				}
			}
			t.Logf("fontTools independent bounds/advance max error=%g SVG units", maxError)
		})
	}
}

func TestTextCairoSVGMatchingFonts(t *testing.T) {
	source, err := os.ReadFile("../tests/data/svg/text-outlines.svg")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("../tests/data/svg/text-outlines.png")
	if err != nil {
		t.Fatal(err)
	}
	reference, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	root, err := svg.Read(bytes.NewReader(source), svg.Options{Width: 320})
	if err != nil {
		t.Fatal(err)
	}
	c := render.DefaultConfig()
	c.Width, c.Height, c.FrameWidth, c.Workers, c.Background = 320, 200, 320, 1, gomath.Transparent
	raster, err := render.NewRasterizer(c)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := raster.Render(&gomath.Frame{Objects: []*gomath.Object{root}})
	if err != nil {
		t.Fatal(err)
	}
	if actual.Bounds() != reference.Bounds() {
		t.Fatal("independent raster size mismatch")
	}
	total, maxDiff, pixels := 0.0, 0, 0
	for y := range c.Height {
		for x := range c.Width {
			a, b := actual.RGBAAt(x, y), color.RGBAModel.Convert(reference.At(x, y)).(color.RGBA)
			changed := false
			for _, delta := range []int{int(a.R) - int(b.R), int(a.G) - int(b.G), int(a.B) - int(b.B), int(a.A) - int(b.A)} {
				d := int(math.Abs(float64(delta)))
				total += float64(d)
				maxDiff = max(maxDiff, d)
				changed = changed || d > 8
			}
			if changed {
				pixels++
			}
		}
	}
	mean, fraction := total/float64(c.Width*c.Height*4), float64(pixels)/float64(c.Width*c.Height)
	t.Logf("CairoSVG matched Go fonts: MAE=%g max=%d fraction_over_8=%g", mean, maxDiff, fraction)
	if mean > .12 || maxDiff > 40 || fraction > .005 {
		t.Fatalf("text reference differs: MAE=%g max=%d fraction=%g", mean, maxDiff, fraction)
	}
	// The main fixture compares independent outline rasterization with scalar
	// absolute positions. Anchor/advance, whitespace and span continuations are
	// tested against fontTools metrics and exact native outlines separately.
}

func TestTextDrawOrderOpacityPixels(t *testing.T) {
	// Overlay identical glyphs in the order root text, span, root text. Geometry
	// ownership must not reorder span glyphs before the parent's character data.
	root := textRead(t, `<text x="20" y="50" font-size="30" fill="red">H<tspan x="20" fill="blue">H</tspan><tspan x="20" fill="green">H</tspan></text>`, nil)
	c := render.DefaultConfig()
	c.Width, c.Height, c.FrameWidth, c.Background = 100, 100, 100, gomath.Transparent
	r, err := render.NewRasterizer(c)
	if err != nil {
		t.Fatal(err)
	}
	img, err := r.Render(&gomath.Frame{Objects: []*gomath.Object{root}})
	if err != nil {
		t.Fatal(err)
	}
	glyphs := instanceShapes(root)
	if len(glyphs) != 3 {
		t.Fatal("overlay glyph count")
	}
	for _, p := range []geom.Vec3{geom.V(24, 35), geom.V(24, 45)} {
		if got := img.RGBAAt(int(p.X), int(p.Y)); got.A != 255 || got.G != 128 || got.R != 0 || got.B != 0 {
			t.Fatal("mixed content draw order changed", p, got)
		}
	}
	root = textRead(t, `<g opacity=".5"><text x="20" y="50" font-size="30" opacity=".5" fill="red" fill-opacity=".5">H</text></g>`, nil)
	img, err = r.Render(&gomath.Frame{Objects: []*gomath.Object{root}})
	if err != nil {
		t.Fatal(err)
	}
	if got := img.RGBAAt(24, 35); got.R != 32 || got.A != 32 {
		t.Fatal("text opacity/fill opacity changed", got)
	}
}
