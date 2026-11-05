package render

import (
	"encoding/json"
	"gomath"
	"gomath/geom"
	"gomath/svg"
	"image/png"
	"math"
	"os"
	"testing"
)

// This is an investigation and a high-resolution geometry regression, not a
// claim that the low-resolution nonzero fill-AA limitation has been fixed.
// Parent demo thresholds remain unchanged. The independent reference generator
// and the one-pixel linear bowtie reproduction are under tests/reference/data.
func TestPartialGlyphFillAAIndependentHighResolutionReference(t *testing.T) {
	data, err := os.ReadFile("../tests/data/render/partial-glyph-aa/cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name        string  `json:"name"`
		D           string  `json:"d_pixel"`
		StrokeWidth float64 `json:"stroke_width"`
		CairoAlpha  float64 `json:"cairo_alpha_x1"`
		CairoFine   float64 `json:"cairo_alpha_x32"`
		SkiaArea    float64 `json:"skia_pixel_area"`
		SignedArea  float64 `json:"signed_pixel_area"`
	}
	if err = json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) != 4 {
		t.Fatal("incomplete partial glyph evidence")
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			if tc.StrokeWidth != 0 {
				t.Fatal("fixture unexpectedly exercises stroke")
			}
			p, err := svg.ParsePath(tc.D)
			if err != nil {
				t.Fatal(err)
			}
			// Frozen pixel-space cubic controls are interpreted directly; no
			// font, formula, scene, exporter, or SVG importer is involved.
			p = p.Transform(geom.Translation(geom.V(-8, 8)).Mul(geom.Scaling(1, -1, 1)))
			o := gomath.NewObject(p)
			o.Style.Fill, o.Style.Stroke, o.Style.StrokeWidth = gomath.White, gomath.Transparent, 0
			for _, sample := range []struct {
				scale     int
				tolerance float64
			}{{1, 0}, {1, 1e-5}, {8, 0}, {32, 0}} {
				scale := sample.scale
				c := DefaultConfig()
				c.Width, c.Height, c.FrameWidth, c.Background = 16*scale, 16*scale, 16, gomath.Transparent
				c.CurveTolerance = sample.tolerance
				r, err := NewRasterizer(c)
				if err != nil {
					t.Fatal(err)
				}
				got, err := r.Render(&gomath.Frame{Objects: []*gomath.Object{o}})
				if err != nil {
					t.Fatal(err)
				}
				sum := 0
				for y := 4 * scale; y < 5*scale; y++ {
					for x := 4 * scale; x < 5*scale; x++ {
						sum += int(got.RGBAAt(x, y).A)
					}
				}
				alpha := float64(sum) / float64(scale*scale)
				t.Logf("scale=%d tolerance=%g native alpha=%.9g; Cairo native=%.9g Cairo 32x=%.9g; Skia nonzero area*255=%.9g abs(signed area)*255=%.9g", scale, sample.tolerance, alpha, tc.CairoAlpha, tc.CairoFine, tc.SkiaArea*255, math.Abs(tc.SignedArea)*255)
				if scale == 32 && (math.Abs(alpha-tc.CairoFine) > 1 || math.Abs(alpha-tc.SkiaArea*255) > 1) {
					t.Fatal("enlarged geometry differs from independent nonzero region coverage")
				}
				// Do not encode the erroneous coarse AA result as a desired
				// baseline. It is logged so a future fill-mask fix can close the
				// limitation without having to weaken this regression.
			}
			f, err := os.Open("../tests/data/render/partial-glyph-aa/" + tc.Name + ".png")
			if err != nil {
				t.Fatal(err)
			}
			ref, err := png.Decode(f)
			f.Close()
			if err != nil {
				t.Fatal(err)
			}
			_, _, _, a := ref.At(4, 4).RGBA()
			if float64(a>>8) != tc.CairoAlpha {
				t.Fatal("independent PNG alpha does not match provenance")
			}
		})
	}
}
