package mathtex

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"golang.org/x/image/font/gofont/goregular"
	"gomath"
	"gomath/geom"
	"gomath/render"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
)

type multilineReference struct {
	Font    struct{ SHA256 string }
	Records []struct {
		Name, Environment, Source string
		Rows, Columns             int
		Width, Height, Depth      float64
		SVGSHA256                 string `json:"svg_sha256"`
		PNGSHA256                 string `json:"png_sha256"`
		Cells                     []struct {
			Row, Column        int
			Glyph, Color       string
			X, Baseline, Width float64
			BBox               [4]float64
		}
	}
}

func TestMultilineIndependentFontPlacementAndCairo(t *testing.T) {
	dir := filepath.Join("..", "tests", "data", "mathtex", "multiline")
	data, err := os.ReadFile(filepath.Join(dir, "reference.json"))
	if err != nil {
		t.Fatal(err)
	}
	var refs multilineReference
	if err := json.Unmarshal(data, &refs); err != nil {
		t.Fatal(err)
	}
	fontHash := sha256.Sum256(goregular.TTF)
	if refs.Font.SHA256 != hex.EncodeToString(fontHash[:]) || len(refs.Records) != 2 {
		t.Fatal("source font / reference count mismatch")
	}
	c := render.DefaultConfig()
	c.Width, c.Height, c.Workers, c.FrameWidth, c.Background = 768, 768, 1, 6, gomath.Black
	r, err := render.NewRasterizer(c)
	if err != nil {
		t.Fatal(err)
	}
	worst, cellCount := 1.0, 0
	for _, ref := range refs.Records {
		for _, file := range []struct{ name, hash string }{{ref.Name + ".svg", ref.SVGSHA256}, {ref.Name + ".png", ref.PNGSHA256}} {
			data, err := os.ReadFile(filepath.Join(dir, file.name))
			if err != nil {
				t.Fatal(err)
			}
			hash := sha256.Sum256(data)
			if hex.EncodeToString(hash[:]) != file.hash {
				t.Fatal("independent reference hash mismatch", file.name)
			}
		}
		opt := DefaultOptions()
		opt.Size = 1
		b, err := layout(ref.Source, opt)
		if err != nil {
			t.Fatal(err)
		}
		if len(ref.Cells) != ref.Rows*ref.Columns {
			t.Fatal("incomplete cell records")
		}
		// Source-font quadratic midpoint rounding is at most one design unit;
		// row baseline sums may accumulate that error over the reference rows.
		tolerance := float64(ref.Rows+1) / 2048
		for _, pair := range [][2]float64{{b.width, ref.Width}, {b.height, ref.Height}, {b.depth, ref.Depth}} {
			if math.Abs(pair[0]-pair[1]) > tolerance {
				t.Fatal("independent baseline-box metric", ref.Name, pair)
			}
		}
		colors := map[string]bool{}
		for _, cell := range ref.Cells {
			object := named(b.object, fmt.Sprintf("%s cell[%d,%d]", ref.Environment, cell.Row, cell.Column))
			if object == nil {
				t.Fatal("missing reference operand")
			}
			bounds := object.Bounds()
			for i, value := range []float64{bounds.Min.X, bounds.Min.Y, bounds.Max.X, bounds.Max.Y} {
				if math.Abs(value-cell.BBox[i]) > tolerance {
					t.Fatal("independent operand placement", ref.Name, cell.Row, cell.Column, i, value, cell.BBox[i])
				}
			}
			paint, err := gomath.Hex(cell.Color)
			if err != nil {
				t.Fatal(err)
			}
			got := mathPaints(object)
			if len(got) != 1 || got[0] != paint {
				t.Fatal("independent operand color", ref.Name, cell.Row, cell.Column, got, paint)
			}
			colors[cell.Color] = true
			cellCount++
		}
		data, err := os.ReadFile(filepath.Join(dir, ref.Name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		expected, err := png.Decode(bytes.NewReader(data))
		if err != nil || expected.Bounds().Dx() != 768 || expected.Bounds().Dy() != 768 {
			t.Fatal("invalid independent Cairo raster", err)
		}
		// Independent SVG origin (64,384), 128 pixels/em, y up.
		b.object.Shift(geom.V(-2.5, 0))
		actual, err := r.Render(&gomath.Frame{Objects: []*gomath.Object{b.object}})
		if err != nil {
			t.Fatal(err)
		}
		difference := 0
		for y := range 768 {
			for x := range 768 {
				a := actual.RGBAAt(x, y)
				b := color.NRGBAModel.Convert(expected.At(x, y)).(color.NRGBA)
				difference += colorByteDiff(a.R, b.R) + colorByteDiff(a.G, b.G) + colorByteDiff(a.B, b.B)
			}
		}
		mae := float64(difference) / (768 * 768 * 3)
		if mae > 1 {
			t.Fatal("independent RGB MAE > 1 byte", ref.Name, mae)
		}
		for paint := range colors {
			rgb, _ := hex.DecodeString(paint)
			matches := func(r, g, b uint8) bool {
				return colorByteDiff(r, rgb[0]) <= 2 && colorByteDiff(g, rgb[1]) <= 2 && colorByteDiff(b, rgb[2]) <= 2
			}
			intersection, union := 0, 0
			for y := range 768 {
				for x := range 768 {
					a := actual.RGBAAt(x, y)
					b := color.NRGBAModel.Convert(expected.At(x, y)).(color.NRGBA)
					aa, bb := matches(a.R, a.G, a.B), matches(b.R, b.G, b.B)
					if aa && bb {
						intersection++
					}
					if aa || bb {
						union++
					}
				}
			}
			iou := float64(intersection) / float64(max(1, union))
			worst = math.Min(worst, iou)
			if intersection < 100 || iou < .95 {
				t.Fatal("independent colored operand silhouette", ref.Name, paint, intersection, iou)
			}
		}
		t.Logf("%s: source-font baseline/cell metrics; whole image RGB MAE %.6f bytes", ref.Name, mae)
	}
	if cellCount != 16 {
		t.Fatal("incomplete independent operand placements", cellCount)
	}
	t.Logf("16 independent operand placements; minimum per-color Cairo mask IoU %.6f", worst)
}
