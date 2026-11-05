package mathtex

import (
	"bytes"
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
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func rbRoot(t *testing.T) string {
	t.Helper()
	d, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			return d
		}
		next := filepath.Dir(d)
		if next == d {
			t.Fatal("cannot find test data root")
		}
		d = next
	}
}
func rbRead(t *testing.T, paths ...string) []byte {
	t.Helper()
	v, err := os.ReadFile(filepath.Join(append([]string{rbRoot(t)}, paths...)...))
	if err != nil {
		t.Fatal(err)
	}
	return v
}
func rbHash(v []byte) string    { h := sha256.Sum256(v); return hex.EncodeToString(h[:]) }
func rbByteDiff(a, b uint8) int { return int(math.Abs(float64(a) - float64(b))) }

func TestRadicalBoxIndependentSourceFontAndCairo(t *testing.T) {
	var refs struct {
		GeneratorSHA256 string `json:"generator_sha256"`
		Fonts           []struct{ SHA256 string }
		Records         []struct {
			Name, Kind, Source   string
			Style                text.FontStyle
			Width, Height, Depth float64
			SVGSHA256            string `json:"svg_sha256"`
			PNGSHA256            string `json:"png_sha256"`
			Placements           []struct {
				Role, Letter, Color string
				Ratio, X, Baseline  float64
				BBox                [4]float64
			}
		}
	}
	data := rbRead(t, "tests", "data", "mathtex", "radical_boxes", "reference.json")
	if err := json.Unmarshal(data, &refs); err != nil {
		t.Fatal(err)
	}
	if len(refs.Records) != 20 || len(refs.Fonts) != 4 {
		t.Fatal("incomplete independent reference")
	}
	if rbHash(rbRead(t, "tests", "reference", "mathtex_radical_boxes.py")) != refs.GeneratorSHA256 {
		t.Fatal("reference generator changed")
	}
	for i, bytes := range [][]byte{goregular.TTF, gobold.TTF, goitalic.TTF, gobolditalic.TTF} {
		if rbHash(bytes) != refs.Fonts[i].SHA256 {
			t.Fatal("reference real font mismatch", i)
		}
	}
	c := render.DefaultConfig()
	c.Width, c.Height, c.Workers, c.FrameWidth, c.Background = 512, 384, 1, 4, gomath.Black
	r, err := render.NewRasterizer(c)
	if err != nil {
		t.Fatal(err)
	}
	worst, maxMAE := 1.0, 0.0
	for _, ref := range refs.Records {
		opt := DefaultOptions()
		opt.Size, opt.Style = 1, ref.Style
		b, err := layout(ref.Source, opt)
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range [][2]float64{{b.width, ref.Width}, {b.height, ref.Height}, {b.depth, ref.Depth}} {
			if math.Abs(p[0]-p[1]) > 2.0/2048 {
				t.Fatal("source-font analytical box", ref.Name, p)
			}
		}
		colors := map[string]bool{}
		for _, p := range ref.Placements {
			o := rbNamed(b.object, p.Role)
			if o == nil {
				t.Fatal("independent operand missing", p.Role)
			}
			bounds := o.Bounds()
			if !bounds.Valid {
				t.Fatal("independent operand invisible")
			}
			for i, v := range []float64{bounds.Min.X, bounds.Min.Y, bounds.Max.X, bounds.Max.Y} {
				if math.Abs(v-p.BBox[i]) > 2.0/2048 {
					t.Fatal("source-font operand placement", ref.Name, p.Role, i, v, p.BBox[i])
				}
			}
			paint, err := gomath.Hex(p.Color)
			if err != nil {
				t.Fatal(err)
			}
			o.Walk(func(n *gomath.Object) {
				if len(n.Path.Contours) > 0 && n.Style.Fill != paint {
					t.Error("operand paint", ref.Name, p.Role)
				}
			})
			colors[p.Color] = true
		}
		if ref.Kind == "indexed" || ref.Kind == "boxed" {
			colors["00ff00"] = true
		}
		for _, file := range []struct{ name, hash string }{{ref.Name + ".svg", ref.SVGSHA256}, {ref.Name + ".png", ref.PNGSHA256}} {
			if rbHash(rbRead(t, "tests", "data", "mathtex", "radical_boxes", file.name)) != file.hash {
				t.Fatal("independent data hash", file.name)
			}
		}
		expected, err := png.Decode(bytes.NewReader(rbRead(t, "tests", "data", "mathtex", "radical_boxes", ref.Name+".png")))
		if err != nil {
			t.Fatal(err)
		}
		if expected.Bounds().Dx() != 512 || expected.Bounds().Dy() != 384 {
			t.Fatal("wrong reference canvas")
		}
		b.object.Shift(geom.V(-1.5, 0)) // independent source SVG origin (64,192),128px/em
		actual, err := r.Render(&gomath.Frame{Objects: []*gomath.Object{b.object}})
		if err != nil {
			t.Fatal(err)
		}
		diff := 0
		for y := range 384 {
			for x := range 512 {
				a := actual.RGBAAt(x, y)
				b := color.NRGBAModel.Convert(expected.At(x, y)).(color.NRGBA)
				diff += rbByteDiff(a.R, b.R) + rbByteDiff(a.G, b.G) + rbByteDiff(a.B, b.B)
			}
		}
		mae := float64(diff) / (512 * 384 * 3)
		maxMAE = math.Max(maxMAE, mae)
		if mae > 1 {
			t.Fatal("whole raster MAE > 1 RGB byte", ref.Name, mae)
		}
		for paint := range colors {
			rgb, _ := hex.DecodeString(paint)
			matches := func(r, g, b uint8) bool {
				return rbByteDiff(r, rgb[0]) <= 2 && rbByteDiff(g, rgb[1]) <= 2 && rbByteDiff(b, rgb[2]) <= 2
			}
			intersection, union := 0, 0
			for y := range 384 {
				for x := range 512 {
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
			if intersection < 40 || iou < .94 {
				t.Fatal("Cairo per-color silhouette", ref.Name, paint, intersection, iou)
			}
		}
	}
	t.Logf("20 fontTools/manual SVG/Cairo references: worst color IoU %.6f; maximum whole-image RGB MAE %.6f bytes", worst, maxMAE)
}
