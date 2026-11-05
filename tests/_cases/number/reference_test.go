package number

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
	"strconv"
	"strings"
	"testing"
)

func hash(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func decode(t *testing.T, b []byte, v any) {
	t.Helper()
	if e := json.Unmarshal(b, v); e != nil {
		t.Fatal(e)
	}
}
func fromBits(t *testing.T, s string) float64 {
	t.Helper()
	b, e := strconv.ParseUint(s, 16, 64)
	if e != nil {
		t.Fatal(e)
	}
	return math.Float64frombits(b)
}
func generatorHash(t *testing.T) string {
	t.Helper()
	b, e := os.ReadFile(filepath.Join(root(t), "tests/reference/number.py"))
	if e != nil {
		t.Fatal(e)
	}
	return hash(b)
}

func TestNumberIndependentPythonAndPinnedManimFormats(t *testing.T) {
	var refs struct {
		Commit      string `json:"manim_commit"`
		Source      string `json:"manim_numbers_sha256"`
		Generator   string `json:"generator_sha256"`
		Differences int    `json:"differing_pinned_records"`
		Records     []struct {
			Bits      string
			Precision int
			Sign      bool `json:"include_sign"`
			Group     bool `json:"group_thousands"`
			Expected  string
			Manim     string `json:"manim_format"`
			Agrees    bool   `json:"agrees_with_pinned"`
		}
		Integers []struct {
			Value    string
			Sign     bool `json:"include_sign"`
			Group    bool `json:"group_thousands"`
			Expected string
		}
	}
	decode(t, data(t, "formats.json"), &refs)
	if refs.Commit != "16ea443e2a5f1d79db0a721161411ce916d6e733" || refs.Source != "d2d0b1bf1505a14129fec5b72b5612818a9c1d3c6ce3b21fe2f9f1a16452deaa" || refs.Generator != generatorHash(t) || len(refs.Records) != 1376 || len(refs.Integers) != 48 {
		t.Fatal("pinned source/generator/reference mismatch")
	}
	differences := 0
	for i, r := range refs.Records {
		v := fromBits(t, r.Bits)
		o := DefaultOptions()
		o.DecimalPlaces = r.Precision
		o.IncludeSign = r.Sign
		o.GroupThousands = r.Group
		got, e := formatDecimal(v, o)
		if e != nil || got != r.Expected {
			t.Fatal("independent Python fixed format", r.Bits, r.Precision, got, r.Expected, e)
		}
		if r.Agrees != (r.Expected == r.Manim) {
			t.Fatal("inconsistent pinned comparison")
		}
		if !r.Agrees {
			differences++
		}
		if i%73 == 0 || !r.Agrees {
			n := mustNew(t, v, o)
			if n.Text() != r.Expected || math.Float64bits(n.Value()) != math.Float64bits(v) {
				t.Fatal("public binary64 author contract")
			}
		}
	}
	if differences != 4 || differences != refs.Differences {
		t.Fatal("undeclared pinned rounding policy differences", differences)
	}
	for _, r := range refs.Integers {
		v, e := strconv.ParseInt(r.Value, 10, 64)
		if e != nil {
			t.Fatal(e)
		}
		o := DefaultIntegerOptions()
		o.IncludeSign = r.Sign
		o.GroupThousands = r.Group
		n, e := NewInteger(v, o)
		if e != nil || n.Text() != r.Expected || n.Value() != v {
			t.Fatal("independent exact Python int64 format", r.Value, r.Expected, e)
		}
	}
	t.Logf("1376 binary64 fixed formats / 48 exact int64 formats; %d documented pinned numpy.round zero-sign deviations", differences)
}

func TestNumberIndependentFontToolsAndCairo(t *testing.T) {
	var refs struct {
		Generator string `json:"generator_sha256"`
		Fonts     []struct{ SHA256 string }
		Fixtures  []struct{ File, SHA256 string }
		Records   []struct {
			Name, Kind, Value, Bits, Text, Unit string
			Precision                           int
			Sign                                bool `json:"include_sign"`
			Group                               bool `json:"group_thousands"`
			Anchor                              Alignment
			Size, Advance                       float64
			Style                               text.FontStyle
			Matrix                              [6]float64
			Glyphs                              []struct {
				Rune      rune
				X, Origin float64
				BBox      []float64
			}
			SVGHash string `json:"svg_sha256"`
			PNGHash string `json:"png_sha256"`
		}
	}
	decode(t, data(t, "layout.json"), &refs)
	if refs.Generator != generatorHash(t) || len(refs.Fonts) != 4 || len(refs.Records) != 20 || len(refs.Fixtures) != 1 {
		t.Fatal("independent layout manifest mismatch")
	}
	for i, b := range [][]byte{goregular.TTF, gobold.TTF, goitalic.TTF, gobolditalic.TTF} {
		if hash(b) != refs.Fonts[i].SHA256 {
			t.Fatal("font source hash", i)
		}
	}
	for _, r := range refs.Fixtures {
		if hash(data(t, r.File)) != r.SHA256 {
			t.Fatal("fixture font changed")
		}
	}
	c := render.DefaultConfig()
	c.Width, c.Height, c.Workers, c.FrameWidth, c.Background = 960, 384, 1, 10, gomath.Black
	renderer, e := render.NewRasterizer(c)
	if e != nil {
		t.Fatal(e)
	}
	worst, maxMAE := 1.0, 0.0
	for _, r := range refs.Records {
		o := DefaultOptions()
		o.DecimalPlaces = r.Precision
		o.IncludeSign = r.Sign
		o.GroupThousands = r.Group
		o.Unit = r.Unit
		o.Anchor = r.Anchor
		o.Size = r.Size
		o.Style = r.Style
		o.Color, _ = gomath.Hex("32a0ff")
		m := geom.Identity()
		m[0], m[4], m[1], m[5], m[3], m[7] = r.Matrix[0], r.Matrix[1], r.Matrix[2], r.Matrix[3], r.Matrix[4], r.Matrix[5]
		var object *gomath.Object
		if r.Kind == "integer" {
			v, e := strconv.ParseInt(r.Value, 10, 64)
			if e != nil {
				t.Fatal(e)
			}
			n, e := NewInteger(v, o)
			if e != nil || n.Text() != r.Text {
				t.Fatal("integer layout source text", e)
			}
			object = n.Object
			object.ApplyMatrix(m)
		} else {
			v := fromBits(t, r.Bits)
			n := mustNew(t, v, o)
			if strings.HasPrefix(r.Name, "affine") {
				n = mustNew(t, 0, o)
				n.Object.ApplyMatrix(m)
				if e := n.SetValue(v); e != nil {
					t.Fatal(e)
				}
			} else {
				n.Object.ApplyMatrix(m)
			}
			if n.Text() != r.Text {
				t.Fatal("decimal layout source text")
			}
			p, e := n.AnchorPoint()
			if e != nil {
				t.Fatal(e)
			}
			samePoint(t, p, m.Apply(geom.V(0, 0)))
			object = n.Object
		}
		if len(object.Children) != len(r.Glyphs) {
			t.Fatal("missing ordinal whitespace/glyph slot")
		}
		// Compare in the independent font coordinate frame. An affine bbox's
		// corners are not the bbox of its transformed actual curved outline.
		det := r.Matrix[0]*r.Matrix[3] - r.Matrix[1]*r.Matrix[2]
		inv := geom.Identity()
		inv[0], inv[1], inv[4], inv[5] = r.Matrix[3]/det, -r.Matrix[2]/det, -r.Matrix[1]/det, r.Matrix[0]/det
		inv[3] = -(inv[0]*r.Matrix[4] + inv[1]*r.Matrix[5])
		inv[7] = -(inv[4]*r.Matrix[4] + inv[5]*r.Matrix[5])
		for i, g := range r.Glyphs {
			actual := object.Children[i].Copy().ApplyMatrix(inv).Bounds()
			if len(g.BBox) == 0 {
				if actual.Valid {
					t.Fatal("source empty glyph acquired ink")
				}
				continue
			}
			wanted := []float64{g.X + g.Origin + g.BBox[0], g.BBox[1], g.X + g.Origin + g.BBox[2], g.BBox[3]}
			for j, v := range []float64{actual.Min.X, actual.Min.Y, actual.Max.X, actual.Max.Y} {
				if math.Abs(v-wanted[j]) > 2.0/2048*r.Size {
					t.Fatal("source hmtx/glyf placement", r.Name, i, j, v, wanted[j])
				}
			}
		}
		if hash(data(t, r.Name+".svg")) != r.SVGHash || hash(data(t, r.Name+".png")) != r.PNGHash {
			t.Fatal("independent image changed")
		}
		expected, e := png.Decode(bytes.NewReader(data(t, r.Name+".png")))
		if e != nil {
			t.Fatal(e)
		}
		if expected.Bounds().Dx() != 960 || expected.Bounds().Dy() != 384 {
			t.Fatal("bad independent canvas")
		}
		actual, e := renderer.Render(&gomath.Frame{Objects: []*gomath.Object{object}})
		if e != nil {
			t.Fatal(e)
		}
		diff, intersection, union := 0, 0, 0
		for y := range 384 {
			for x := range 960 {
				a := actual.RGBAAt(x, y)
				b := color.NRGBAModel.Convert(expected.At(x, y)).(color.NRGBA)
				diff += absByte(a.R, b.R) + absByte(a.G, b.G) + absByte(a.B, b.B)
				aa, bb := a.B >= 128, b.B >= 128
				if aa && bb {
					intersection++
				}
				if aa || bb {
					union++
				}
			}
		}
		iou := float64(intersection) / float64(max(1, union))
		mae := float64(diff) / (960 * 384 * 3)
		worst = math.Min(worst, iou)
		maxMAE = math.Max(maxMAE, mae)
		if intersection < 100 || iou < .95 || mae > 1 {
			t.Fatal("source-font Cairo raster", r.Name, intersection, iou, mae)
		}
	}
	t.Logf("20 independent real-font/Cairo layouts; minimum mask IoU %.6f, maximum RGB MAE %.6f bytes", worst, maxMAE)
}
func absByte(a, b uint8) int { return int(math.Abs(float64(a) - float64(b))) }
