package mathtex

import (
	"encoding/json"
	"gomath"
	"gomath/geom"
	"gomath/render"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
)

type alphabetFontReference struct {
	Command string     `json:"command"`
	Input   string     `json:"input"`
	Rune    string     `json:"rune"`
	Glyph   string     `json:"glyph"`
	Advance float64    `json:"advance_em"`
	BBox    [4]float64 `json:"bbox_em"`
	Raster  string     `json:"raster"`
	Font    struct {
		SHA256 string `json:"sha256"`
	} `json:"font"`
}

func alphabetReferences(t *testing.T) []alphabetFontReference {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "tests", "data", "mathtex", "alphabets", "reference.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Records []alphabetFontReference `json:"records"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest.Records) != 166 {
		t.Fatal("incomplete independent alphabet reference")
	}
	seen := map[string]bool{}
	counts := map[string]int{}
	for _, ref := range manifest.Records {
		key := ref.Command + "/" + ref.Input
		if seen[key] || ref.Glyph == "" || ref.Glyph == ".notdef" || len([]rune(ref.Rune)) != 1 || len(ref.Input) != 1 || ref.Font.SHA256 != "5add3f3f2bd7fd897d2fa5ccbe468607c52111dc44cdfaaf2d851a574f5357a7" {
			t.Fatal("invalid independent alphabet reference", key)
		}
		seen[key] = true
		counts[ref.Command]++
	}
	if counts["mathbb"] != 62 || counts["mathcal"] != 52 || counts["mathfrak"] != 52 {
		t.Fatal("incomplete source alphabet coverage", counts)
	}
	return manifest.Records
}

// Original-font hmtx/CFF extrema, not derived from our paths or layout.
func TestMathAlphabetIndependentFontMetrics(t *testing.T) {
	for _, ref := range alphabetReferences(t) {
		t.Run(ref.Command+"/"+ref.Input, func(t *testing.T) {
			opt := DefaultOptions()
			opt.Size = 1
			b, err := layout(`\`+ref.Command+`{`+ref.Input+`}`, opt)
			if err != nil {
				t.Fatal(err)
			}
			bounds := b.object.Bounds()
			left := math.Max(0, -ref.BBox[0])
			actual := []float64{b.width, b.height, b.depth, bounds.Min.X, bounds.Min.Y, bounds.Max.X, bounds.Max.Y}
			want := []float64{math.Max(ref.Advance, ref.BBox[2]) + left, math.Max(0, ref.BBox[3]), math.Max(0, -ref.BBox[1]), ref.BBox[0] + left, ref.BBox[1], ref.BBox[2] + left, ref.BBox[3]}
			for i := range actual {
				if !finite(actual[i]) || math.Abs(actual[i]-want[i]) > 2e-12 {
					t.Fatalf("metric %d got %.15g want %.15g", i, actual[i], want[i])
				}
			}
			node := named(b.object, ref.Rune)
			if node == nil || len(node.Path.Contours) == 0 {
				t.Fatal("missing actual mapped outline", ref.Rune)
			}
			for _, c := range node.Path.Contours {
				if !c.Closed || len(c.Curves) == 0 {
					t.Fatal("open/empty source font contour")
				}
			}
		})
	}
}

// FreeType loads the original source font at 256 ppem. Go tests use committed
// masks only. The silhouette check protects real counters and disconnected ink;
// every unmatched ink pixel must remain within two pixels of the other mask.
func TestMathAlphabetIndependentFreeTypeMasks(t *testing.T) {
	config := render.DefaultConfig()
	config.Width, config.Height, config.Workers = 384, 384, 1
	config.FrameWidth, config.Background = 1.5, gomath.Black
	renderer, err := render.NewRasterizer(config)
	if err != nil {
		t.Fatal(err)
	}
	worst := 1.0
	for _, ref := range alphabetReferences(t) {
		t.Run(ref.Command+"/"+ref.Input, func(t *testing.T) {
			file, err := os.Open(filepath.Join("..", "tests", "data", "mathtex", "alphabets", ref.Raster))
			if err != nil {
				t.Fatal(err)
			}
			reference, err := png.Decode(file)
			file.Close()
			if err != nil {
				t.Fatal(err)
			}
			if reference.Bounds().Dx() != 384 || reference.Bounds().Dy() != 384 {
				t.Fatal("unexpected FreeType raster size")
			}
			opt := DefaultOptions()
			opt.Size = 1
			b, err := layout(`\`+ref.Command+`{`+ref.Input+`}`, opt)
			if err != nil {
				t.Fatal(err)
			}
			left := math.Max(0, -ref.BBox[0])
			b.object.Shift(geom.V(-.5625-left, -.375)) // source origin (48,288)
			raster, err := renderer.Render(&gomath.Frame{Objects: []*gomath.Object{b.object}})
			if err != nil {
				t.Fatal(err)
			}
			var actual, expected [384 * 384]bool
			intersection, union := 0, 0
			for y := range 384 {
				for x := range 384 {
					i := y*384 + x
					red, _, _, _ := reference.At(x, y).RGBA()
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
				t.Fatal("empty font mask")
			}
			iou := float64(intersection) / float64(union)
			worst = math.Min(worst, iou)
			if iou < .95 {
				t.Errorf("silhouette IoU %.6f < .95", iou)
			}
			for y := range 384 {
				for x := range 384 {
					i := y*384 + x
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
							if xx >= 0 && xx < 384 && yy >= 0 && yy < 384 && opposite[yy*384+xx] {
								nearby = true
							}
						}
					}
					if !nearby {
						t.Fatalf("ink (%d,%d) is over two pixels away from independent mask", x, y)
					}
				}
			}
		})
	}
	t.Logf("166 independent source glyphs; minimum silhouette IoU %.6f", worst)
}
