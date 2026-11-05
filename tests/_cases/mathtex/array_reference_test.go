package mathtex

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gobolditalic"
	"golang.org/x/image/font/gofont/goitalic"
	"golang.org/x/image/font/gofont/goregular"
	"gomath"
	"gomath/geom"
	"gomath/render"
	"gomath/text"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
)

type arrayReference struct {
	GeneratorSHA256 string `json:"generator_sha256"`
	Fonts           []struct{ File, SHA256 string }
	FilesSHA256     map[string]string `json:"files_sha256"`
	Records         []struct {
		Name, Source         string
		Style, Rows, Columns int
		Width, Height, Depth float64
		Cells                []struct {
			Row, Column                       int
			X, Baseline, Width, Height, Depth float64
			BBox                              []float64
		}
		Rules []struct {
			Axis       string
			Index      int
			Start, End [2]float64
		}
	}
}

func arrayReadReference(t *testing.T) arrayReference {
	t.Helper()
	data, err := os.ReadFile("../tests/data/mathtex/arrays/reference.json")
	if err != nil {
		t.Fatal(err)
	}
	var refs arrayReference
	if err := json.Unmarshal(data, &refs); err != nil {
		t.Fatal(err)
	}
	generator, err := os.ReadFile("../tests/reference/mathtex_arrays.py")
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(generator)
	if hex.EncodeToString(hash[:]) != refs.GeneratorSHA256 {
		t.Fatal("reference generator hash mismatch")
	}
	fonts := [][]byte{goregular.TTF, gobold.TTF, goitalic.TTF, gobolditalic.TTF}
	if len(refs.Fonts) != 4 || len(refs.Records) != 7 {
		t.Fatal("missing reference faces/records")
	}
	for i, f := range refs.Fonts {
		hash := sha256.Sum256(fonts[i])
		if f.SHA256 != hex.EncodeToString(hash[:]) {
			t.Fatal("source font hash mismatch", f.File)
		}
	}
	for name, want := range refs.FilesSHA256 {
		data, err := os.ReadFile(filepath.Join("../tests/data/mathtex/arrays", name))
		if err != nil {
			t.Fatal(err)
		}
		hash := sha256.Sum256(data)
		if hex.EncodeToString(hash[:]) != want {
			t.Fatal("reference fixture hash mismatch", name)
		}
	}
	return refs
}
func arrayReferenceRaster(t *testing.T, o *gomath.Object) *image.RGBA {
	t.Helper()
	c := render.DefaultConfig()
	c.Width, c.Height, c.Workers, c.FrameWidth, c.Background = 768, 768, 1, 6, gomath.Black
	r, err := render.NewRasterizer(c)
	if err != nil {
		t.Fatal(err)
	}
	copy := o.Copy().Shift(geom.V(-2.5, 0))
	im, err := r.Render(&gomath.Frame{Objects: []*gomath.Object{copy}})
	if err != nil {
		t.Fatal(err)
	}
	return im
}
func arrayCompareReferencePNG(t *testing.T, name string, got *image.RGBA, paints map[string]bool) (float64, float64, int) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("../tests/data/mathtex/arrays", name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := png.Decode(bytes.NewReader(data))
	if err != nil || want.Bounds() != got.Bounds() {
		t.Fatal("invalid reference raster", err)
	}
	var sum, squares float64
	maximum := 0
	for y := range 768 {
		for x := range 768 {
			a, b := got.RGBAAt(x, y), color.NRGBAModel.Convert(want.At(x, y)).(color.NRGBA)
			for _, d := range []int{int(a.R) - int(b.R), int(a.G) - int(b.G), int(a.B) - int(b.B)} {
				sum += math.Abs(float64(d))
				squares += float64(d * d)
				maximum = max(maximum, int(math.Abs(float64(d))))
			}
		}
	}
	mae, rms := sum/(768*768*3), math.Sqrt(squares/(768*768*3))
	worst := 1.0
	for paint := range paints {
		rgb, err := hex.DecodeString(paint)
		if err != nil || len(rgb) != 3 {
			t.Fatal("bad reference paint")
		}
		matches := func(r, g, b uint8) bool {
			return colorByteDiff(r, rgb[0]) <= 2 && colorByteDiff(g, rgb[1]) <= 2 && colorByteDiff(b, rgb[2]) <= 2
		}
		intersection, union := 0, 0
		for y := range 768 {
			for x := range 768 {
				a, b := got.RGBAAt(x, y), color.NRGBAModel.Convert(want.At(x, y)).(color.NRGBA)
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
		if intersection < 30 || iou < .97 {
			t.Fatal("independent colored geometry mismatch", name, paint, intersection, iou)
		}
	}
	t.Logf("%s fontTools/manual/Cairo MAE=%.9g RMS=%.9g max=%d worst-color-IoU=%.9g", name, mae, rms, maximum, worst)
	if mae > .08 || rms > 1 || maximum > 64 {
		t.Fatal("independent array raster bounds exceeded", name, mae, rms, maximum)
	}
	return mae, worst, maximum
}
func TestArrayIndependentFontMetricsRulesAndCairo(t *testing.T) {
	refs := arrayReadReference(t)
	for _, ref := range refs.Records {
		t.Run(ref.Name, func(t *testing.T) {
			o := DefaultOptions()
			o.Size, o.Style = 1, text.FontStyle(ref.Style)
			b, err := layout(ref.Source, o)
			if err != nil {
				t.Fatal(err)
			}
			// Original quadratic font bounds vs sfnt's design-unit rounding. Layout
			// policy is exact in separate tests; oracle tolerates only accumulated font units.
			tolerance := float64(ref.Rows+1) / 2048
			for _, pair := range [][2]float64{{b.width, ref.Width}, {b.height, ref.Height}, {b.depth, ref.Depth}} {
				if math.Abs(pair[0]-pair[1]) > tolerance {
					t.Fatal("independent array metric", pair)
				}
			}
			for _, cell := range ref.Cells {
				object := arrayCell(t, b, cell.Row, cell.Column)
				bounds := object.Bounds()
				if len(cell.BBox) == 0 {
					if bounds.Valid {
						t.Fatal("phantom reference cell leaked ink")
					}
					continue
				}
				if !bounds.Valid {
					t.Fatal("visible operand disappeared")
				}
				for i, value := range []float64{bounds.Min.X, bounds.Min.Y, bounds.Max.X, bounds.Max.Y} {
					if math.Abs(value-cell.BBox[i]) > tolerance {
						t.Fatal("independent operand placement", cell.Row, cell.Column, i, value, cell.BBox[i])
					}
				}
			}
			for _, rule := range ref.Rules {
				object := arrayRule(t, b, rule.Axis, rule.Index)
				curve := object.Path.Contours[0].Curves[0]
				for _, pair := range [][2]float64{{curve.P0.X, rule.Start[0]}, {curve.P0.Y, rule.Start[1]}, {curve.P3.X, rule.End[0]}, {curve.P3.Y, rule.End[1]}} {
					if math.Abs(pair[0]-pair[1]) > tolerance {
						t.Fatal("independent rule placement", rule.Axis, rule.Index, pair)
					}
				}
				near(t, object.Style.StrokeWidth, .045)
				if object.Style.StrokeCap != gomath.CapButt || object.Style.Stroke != gomath.RGB(0, 1, 0) {
					t.Fatal("rule color/cap changed")
				}
			}
			paints := map[string]bool{}
			b.object.Walk(func(n *gomath.Object) {
				if len(n.Path.Contours) == 0 {
					return
				}
				if n.Style.Fill.A > 0 {
					r, g, bb := n.Style.Fill.R, n.Style.Fill.G, n.Style.Fill.B
					paints[fmt.Sprintf("%02x%02x%02x", int(math.Round(r*255)), int(math.Round(g*255)), int(math.Round(bb*255)))] = true
				}
				if n.Style.StrokeWidth > 0 && n.Style.Stroke.A > 0 {
					paints["00ff00"] = true
				}
			})
			arrayCompareReferencePNG(t, ref.Name, arrayReferenceRaster(t, b.object), paints)
		})
	}
}
func TestArrayIndependentIntermediateTransform(t *testing.T) {
	refs := arrayReadReference(t)
	ref := refs.Records[0]
	o := DefaultOptions()
	o.Size = 1
	b, err := layout(ref.Source, o)
	if err != nil {
		t.Fatal(err)
	}
	from := b.object
	to := from.Copy().ApplyMatrix(geom.Translation(geom.V(.2, .3)).Mul(geom.Scaling(.8, 1.1, 1)))
	scene := gomath.NewScene()
	scene.Add(from)
	if err := scene.Play(1, gomath.Transform(from, to).WithRate(gomath.Linear)); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		time  float64
		label string
	}{{.35, "035"}, {.7, "070"}} {
		f, err := scene.Sample(tc.time)
		if err != nil {
			t.Fatal(err)
		}
		got := f.Find(from.ID)
		if got == nil || named(got, "ArrayVRule[0]") == nil {
			t.Fatal("morph lost rules/identity")
		}
		arrayCompareReferencePNG(t, "morph-"+tc.label, arrayReferenceRaster(t, got), map[string]bool{"00ff00": true, "ff0000": true, "0000ff": true, "8040c0": true, "ff8000": true})
	}
}
