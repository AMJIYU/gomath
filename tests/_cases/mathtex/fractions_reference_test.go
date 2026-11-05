package mathtex

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gobolditalic"
	"golang.org/x/image/font/gofont/goitalic"
	"golang.org/x/image/font/gofont/goregular"
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

type fractionFontReference struct {
	Context string                        `json:"context"`
	Style   text.FontStyle                `json:"style"`
	Rune    string                        `json:"rune"`
	Ratio   float64                       `json:"ratio"`
	Raster  string                        `json:"raster"`
	BBox    [4]float64                    `json:"bbox_em"`
	Advance float64                       `json:"advance_em"`
	OriginX float64                       `json:"outline_origin_x_em"`
	Font    struct{ File, SHA256 string } `json:"font"`
}

func fractionReferences(t *testing.T) []fractionFontReference {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "tests", "data", "mathtex", "fractions", "reference.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Records []fractionFontReference `json:"records"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest.Records) != 48 {
		t.Fatal("incomplete independent fraction font references")
	}
	fonts := [][]byte{goregular.TTF, gobold.TTF, goitalic.TTF, gobolditalic.TTF}
	seen := map[string]bool{}
	for _, ref := range manifest.Records {
		key := ref.Context + "/" + string(rune('0'+ref.Style)) + "/" + ref.Rune
		if ref.Style > text.BoldItalic || len(ref.Rune) != 1 || seen[key] {
			t.Fatal("invalid/duplicate reference", key)
		}
		seen[key] = true
		hash := sha256.Sum256(fonts[ref.Style])
		if hex.EncodeToString(hash[:]) != ref.Font.SHA256 {
			t.Fatal("reference source font differs from embedded face", ref.Font.File)
		}
		wanted := map[string]float64{"dfrac": 1, "tfrac": .7, "substack": .65, "script-binom": .65 * .7}
		if math.Abs(ref.Ratio-wanted[ref.Context]) > 1e-15 || ref.Ratio == 0 {
			t.Fatal("reference scale mismatch", ref.Context, ref.Ratio)
		}
	}
	return manifest.Records
}

func fractionReferenceBox(t *testing.T, ref fractionFontReference) box {
	t.Helper()
	operand := "{" + ref.Rune + "}"
	source := `\` + ref.Context + operand + operand
	switch ref.Context {
	case "substack":
		source = `\substack{` + ref.Rune + `\\` + ref.Rune + `}`
	case "script-binom":
		source = `x^{\binom` + operand + operand + `}`
	}
	opt := DefaultOptions()
	opt.Size, opt.Style = 1, ref.Style
	b, err := layout(source, opt)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// Source hmtx/glyf extrema establish real operand dimensions at each documented
// scale. Ruled fraction advances/baselines are checked against those independent
// font measurements plus the public, explicit gap/axis policy.
func TestGeneralFractionIndependentFontMetrics(t *testing.T) {
	for _, ref := range fractionReferences(t) {
		b := fractionReferenceBox(t, ref)
		leaf := named(b.object, ref.Rune)
		if leaf == nil || !leaf.Bounds().Valid {
			t.Fatal("missing real operand glyph")
		}
		bounds := leaf.Bounds()
		for i, values := range [][2]float64{
			{bounds.Size().X, (ref.BBox[2] - ref.BBox[0]) * ref.Ratio},
			{bounds.Size().Y, (ref.BBox[3] - ref.BBox[1]) * ref.Ratio},
		} {
			if math.Abs(values[0]-values[1]) > 1.0/2048*ref.Ratio {
				t.Fatalf("%s/%d/%s source glyph dimension %d: got %g want %g", ref.Context, ref.Style, ref.Rune, i, values[0], values[1])
			}
		}
		if ref.Context != "dfrac" && ref.Context != "tfrac" {
			continue
		}
		x0, x1 := (ref.BBox[0]+ref.OriginX)*ref.Ratio, (ref.BBox[2]+ref.OriginX)*ref.Ratio
		y0, y1 := ref.BBox[1]*ref.Ratio, ref.BBox[3]*ref.Ratio
		left := math.Max(0, -x0)
		width := math.Max(ref.Advance*ref.Ratio, x1) + left + .25
		gap := .14
		if ref.Context == "dfrac" {
			gap = .24
		}
		clearance := gap + .045/2
		ny, dy := .16+clearance+math.Max(0, -y0), .16-clearance-math.Max(0, y1)
		num, den := named(b.object, "FractionNumerator").Bounds(), named(b.object, "FractionDenominator").Bounds()
		got := []float64{b.width, b.height, b.depth, num.Min.X, num.Min.Y, num.Max.Y, den.Min.Y, den.Max.Y}
		want := []float64{width, ny + math.Max(0, y1), math.Max(0, -dy+math.Max(0, -y0)), .125 + x0 + left, ny + y0, ny + y1, dy + y0, dy + y1}
		for i := range got {
			if math.Abs(got[i]-want[i]) > 2.0/2048 {
				t.Fatalf("%s/%d/%s source font / fraction metric %d got %g want %g", ref.Context, ref.Style, ref.Rune, i, got[i], want[i])
			}
		}
	}
}

// This compares operand shapes, normalized to the reference font origin;
// whole-formula placement is tested independently above and analytically in
// fractions_test.go. FreeType is not used by Go tests or runtime.
func TestGeneralFractionIndependentFreeTypeOperands(t *testing.T) {
	c := render.DefaultConfig()
	c.Width, c.Height, c.Workers, c.FrameWidth, c.Background = 384, 384, 1, 1.5, gomath.Black
	renderer, err := render.NewRasterizer(c)
	if err != nil {
		t.Fatal(err)
	}
	worst := 1.0
	for _, ref := range fractionReferences(t) {
		file, err := os.Open(filepath.Join("..", "tests", "data", "mathtex", "fractions", ref.Raster))
		if err != nil {
			t.Fatal(err)
		}
		reference, err := png.Decode(file)
		file.Close()
		if err != nil || reference.Bounds().Dx() != 384 || reference.Bounds().Dy() != 384 {
			t.Fatal("invalid independent mask", ref.Raster, err)
		}
		b := fractionReferenceBox(t, ref)
		glyph := named(b.object, ref.Rune).Copy()
		bounds := glyph.Bounds()
		glyph.Shift(geom.V(-.5625+ref.BBox[0]*ref.Ratio-bounds.Min.X, -.375+ref.BBox[1]*ref.Ratio-bounds.Min.Y))
		raster, err := renderer.Render(&gomath.Frame{Objects: []*gomath.Object{glyph}})
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
			t.Fatal("empty operand raster")
		}
		iou := float64(intersection) / float64(union)
		worst = math.Min(worst, iou)
		if iou < .95 {
			t.Errorf("%s silhouette IoU %.6f < .95", ref.Raster, iou)
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
					t.Fatalf("%s unmatched ink at (%d,%d) exceeds two pixels", ref.Raster, x, y)
				}
			}
		}
	}
	t.Logf("48 independent FreeType operand masks; minimum silhouette IoU %.6f", worst)
}
