package mathtex

import (
	"encoding/json"
	"fmt"
	"gomath"
	"gomath/geom"
	"gomath/render"
	"gomath/text"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
)

type setFontReference struct {
	OriginX float64        `json:"outline_origin_x_em"`
	Command string         `json:"command"`
	Rune    string         `json:"rune"`
	Style   text.FontStyle `json:"style"`
	Native  bool           `json:"native"`
	Advance float64        `json:"advance_em"`
	BBox    [4]float64     `json:"bbox_em"`
	Raster  string         `json:"raster"`
	Font    struct {
		File   string `json:"file"`
		SHA256 string `json:"sha256"`
	} `json:"font"`
	Glyph string `json:"glyph"`
}

func setFontReferences(t *testing.T) []setFontReference {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "tests", "data", "mathtex", "set-symbols", "reference.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Records []setFontReference `json:"records"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest.Records) != 60 {
		t.Fatal("incomplete independent font reference")
	}
	seen := make(map[string]bool)
	for _, ref := range manifest.Records {
		key := fmt.Sprintf("%s/%d", ref.Command, ref.Style)
		if seen[key] || ref.Glyph == "" || ref.Glyph == ".notdef" || len(ref.Font.SHA256) != 64 || ref.Style > text.BoldItalic || setSymbolCommands[ref.Command] == 0 || string(setSymbolCommands[ref.Command]) != ref.Rune {
			t.Fatal("invalid/duplicate source-font record", key)
		}
		seen[key] = true
	}
	return manifest.Records
}

// Source-font hmtx and exact Bezier extrema were computed by fontTools, not
// gomath. Native quadratic loading rounds implied midpoints to design units
// and uses 1024 ppem in 26.6 fixed point (within one 1/2048 em design unit);
// supplemental CFF contours retain source coordinates directly.
func TestSetSymbolIndependentFontMetrics(t *testing.T) {
	for _, ref := range setFontReferences(t) {
		t.Run(fmt.Sprintf("%s/%d", ref.Command, ref.Style), func(t *testing.T) {
			opt := DefaultOptions()
			opt.Style, opt.Color = ref.Style, gomath.Blue
			for _, size := range []float64{.01, 1, 1e6} {
				opt.Size = size
				b, err := layout(`\`+ref.Command, opt)
				if err != nil {
					t.Fatal(err)
				}
				checkLimitsBox(t, b)
				left := math.Max(0, -ref.BBox[0]-ref.OriginX)
				ink := b.object.Bounds()
				got := []float64{b.width, b.height, b.depth, ink.Min.X, ink.Min.Y, ink.Max.X, ink.Max.Y}
				want := []float64{math.Max(ref.Advance, ref.BBox[2]+ref.OriginX) + left, math.Max(0, ref.BBox[3]), math.Max(0, -ref.BBox[1]), ref.BBox[0] + ref.OriginX + left, ref.BBox[1], ref.BBox[2] + ref.OriginX + left, ref.BBox[3]}
				tolerance := 2e-12
				if ref.Native {
					tolerance = 1.0 / 2048
				}
				for i := range got {
					if math.Abs(got[i]/size-want[i]) > tolerance {
						t.Fatalf("source font %s metric %d: got %.12g want %.12g", ref.Font.File, i, got[i]/size, want[i])
					}
				}
				b.object.Walk(func(o *gomath.Object) {
					if len(o.Path.Contours) == 0 {
						return
					}
					if o.Style.Fill != gomath.Blue || o.Style.StrokeWidth != 0 || o.Style.Stroke.A != 0 {
						t.Error("symbol style/color lost")
					}
					for _, c := range o.Path.Contours {
						if !c.Closed || len(c.Curves) == 0 {
							t.Error("empty or open source contour")
						}
					}
				})
			}
		})
	}
}

// The committed masks come directly from Pillow/FreeType loading original font
// files at 256 ppem. Tests load only these PNGs: no system fonts or external tool.
// Binary IoU permits hinting/AA differences; unmatched ink must stay within two
// pixels of the opposite silhouette, so missing holes/slashes cannot pass.
func TestSetSymbolIndependentFreeTypeMasks(t *testing.T) {
	worst := 1.0
	for _, ref := range setFontReferences(t) {
		t.Run(fmt.Sprintf("%s/%d", ref.Command, ref.Style), func(t *testing.T) {
			filename := filepath.Join("..", "tests", "data", "mathtex", "set-symbols", ref.Raster)
			file, err := os.Open(filename)
			if err != nil {
				t.Fatal(err)
			}
			reference, err := png.Decode(file)
			file.Close()
			if err != nil {
				t.Fatal(err)
			}
			if reference.Bounds().Dx() != 320 || reference.Bounds().Dy() != 320 {
				t.Fatal("unexpected reference raster dimensions")
			}
			opt := DefaultOptions()
			opt.Size, opt.Style = 1, ref.Style
			b, err := layout(`\`+ref.Command, opt)
			if err != nil {
				t.Fatal(err)
			}
			left := math.Max(0, -ref.BBox[0]-ref.OriginX)
			b.object.Shift(geom.V(-.5-left-ref.OriginX, -.25)) // font origin (32,224), baseline y=0
			config := render.DefaultConfig()
			config.Width, config.Height, config.Workers = 320, 320, 1
			config.FrameWidth, config.Background = 1.25, gomath.Black
			renderer, err := render.NewRasterizer(config)
			if err != nil {
				t.Fatal(err)
			}
			raster, err := renderer.Render(&gomath.Frame{Objects: []*gomath.Object{b.object}})
			if err != nil {
				t.Fatal(err)
			}
			var actual, expected [320 * 320]bool
			intersection, union := 0, 0
			for y := range 320 {
				for x := range 320 {
					red, _, _, _ := reference.At(x, y).RGBA()
					i := y*320 + x
					actual[i], expected[i] = raster.RGBAAt(x, y).R >= 128, red >= 128*257
					if actual[i] && expected[i] {
						intersection++
					}
					if actual[i] || expected[i] {
						union++
					}
				}
			}
			if union == 0 {
				t.Fatal("empty independent mask")
			}
			iou := float64(intersection) / float64(union)
			worst = math.Min(worst, iou)
			if iou < .95 {
				t.Errorf("silhouette IoU %.6f < .95", iou)
			}
			for y := range 320 {
				for x := range 320 {
					i := y*320 + x
					if actual[i] == expected[i] {
						continue
					}
					opposite := &expected
					if expected[i] {
						opposite = &actual
					}
					nearby := false
					for dy := -2; dy <= 2; dy++ {
						for dx := -2; dx <= 2; dx++ {
							xx, yy := x+dx, y+dy
							if xx >= 0 && xx < 320 && yy >= 0 && yy < 320 && opposite[yy*320+xx] {
								nearby = true
							}
						}
					}
					if !nearby {
						t.Fatalf("ink at (%d,%d) differs by more than two pixels from independent mask", x, y)
					}
				}
			}
		})
	}
	t.Logf("60 independent font masks; minimum binary silhouette IoU %.6f", worst)
}
