package text_test

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
	"gomath/export"
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

func TestMarkupIndependentStyledFontToolsAndCairo(t *testing.T) {
	dir := filepath.Join("..", "tests", "data", "text", "markup")
	data, err := os.ReadFile(filepath.Join(dir, "reference.json"))
	if err != nil {
		t.Fatal(err)
	}
	var refs struct {
		Sources map[string]string
		Records []struct {
			Name, Source, Text string
			CustomFonts        bool `json:"custom_fonts"`
			Metrics            text.Metrics
			SVGSHA256          string `json:"svg_sha256"`
			PNGSHA256          string `json:"png_sha256"`
			Glyphs             []struct {
				Rune, Font        int
				Color             string
				X, Baseline, Kern float64
				BBox              []float64
			}
		}
	}
	if err := json.Unmarshal(data, &refs); err != nil || len(refs.Records) != 4 {
		t.Fatal("incomplete styled reference", err)
	}
	for name, data := range map[string][]byte{
		"Go-Regular.ttf": goregular.TTF, "Go-Bold.ttf": gobold.TTF,
		"Go-Italic.ttf": goitalic.TTF, "Go-Bold-Italic.ttf": gobolditalic.TTF,
	} {
		h := sha256.Sum256(data)
		if hex.EncodeToString(h[:]) != refs.Sources[name] {
			t.Fatal("independent original face hash differs", name)
		}
	}
	for _, name := range []string{"primary.ttf", "secondary.ttf"} {
		verifiedReferenceData(t, name, refs.Sources[name])
	}
	verified := func(t *testing.T, name, hash string) []byte {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		h := sha256.Sum256(data)
		if hex.EncodeToString(h[:]) != hash {
			t.Fatal("independent markup image hash differs", name)
		}
		return data
	}
	fonts := fallbackFonts(t)
	c := render.DefaultConfig()
	c.Width, c.Height, c.Workers, c.FrameWidth, c.Background = 1024, 640, 1, 8, gomath.Black
	r, err := render.NewRasterizer(c)
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range refs.Records {
		t.Run(ref.Name, func(t *testing.T) {
			opt := text.DefaultMarkupOptions()
			opt.Size = 1
			if ref.CustomFonts {
				opt.Fonts = &text.MarkupFonts{Regular: fonts[0], Bold: fonts[1]}
			}
			o, m, err := text.LayoutMarkup(ref.Source, opt)
			if err != nil {
				t.Fatal(err)
			}
			if m.Lines != ref.Metrics.Lines || o.Name != ref.Text {
				t.Fatal("independent decoded text/line count", o.Name, m)
			}
			for _, pair := range [][2]float64{{m.Width, ref.Metrics.Width}, {m.Height, ref.Metrics.Height}, {m.Ascent, ref.Metrics.Ascent}, {m.Descent, ref.Metrics.Descent}} {
				closeMetric(t, pair[0], pair[1])
			}
			index := 0
			colors := map[string]bool{}
			for _, glyph := range ref.Glyphs {
				if glyph.BBox == nil {
					continue
				}
				if index >= len(o.Children) || o.Children[index].Name != string(rune(glyph.Rune)) {
					t.Fatal("independent styled glyph sequence", index)
				}
				child := o.Children[index]
				bounds := child.Bounds()
				for i, value := range []float64{bounds.Min.X, bounds.Min.Y, bounds.Max.X, bounds.Max.Y} {
					if math.Abs(value-glyph.BBox[i]) > 1./2048 {
						t.Fatal("independent fontTools glyph placement/baseline", index, i, value, glyph.BBox[i])
					}
				}
				paint, err := gomath.Hex(glyph.Color)
				if err != nil || child.Style.Fill != paint {
					t.Fatal("independent foreground scope differs", index, child.Style.Fill, paint, err)
				}
				colors[glyph.Color] = true
				index++
			}
			if index != len(o.Children) || index < 2 {
				t.Fatal("independent ink glyph count", index, len(o.Children))
			}
			verified(t, ref.Name+".svg", ref.SVGSHA256)
			expected, err := png.Decode(bytes.NewReader(verified(t, ref.Name+".png", ref.PNGSHA256)))
			if err != nil || expected.Bounds().Dx() != c.Width || expected.Bounds().Dy() != c.Height {
				t.Fatal("invalid independent Cairo markup raster", err)
			}
			o.Shift(geom.V(-3.5, 1)) // manual SVG origin (64,192), 128 pixels/em
			frame := &gomath.Frame{Objects: []*gomath.Object{o}}
			actual, err := r.Render(frame)
			if err != nil {
				t.Fatal(err)
			}
			difference := 0
			for y := range c.Height {
				for x := range c.Width {
					a := actual.RGBAAt(x, y)
					b := color.NRGBAModel.Convert(expected.At(x, y)).(color.NRGBA)
					difference += int(math.Abs(float64(a.R)-float64(b.R)) + math.Abs(float64(a.G)-float64(b.G)) + math.Abs(float64(a.B)-float64(b.B)))
				}
			}
			mae := float64(difference) / float64(c.Width*c.Height*3)
			if mae > 1 {
				t.Fatal("independent RGB MAE exceeds one byte", mae)
			}
			worst := 1.
			for paint := range colors {
				rgb, _ := hex.DecodeString(paint)
				matches := func(r, g, b uint8) bool {
					return math.Abs(float64(r)-float64(rgb[0])) <= 2 && math.Abs(float64(g)-float64(rgb[1])) <= 2 && math.Abs(float64(b)-float64(rgb[2])) <= 2
				}
				intersection, union := 0, 0
				for y := range c.Height {
					for x := range c.Width {
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
				if iou < .97 || intersection < 100 {
					t.Fatal("independent foreground silhouette mismatch", paint, iou, intersection)
				}
			}
			target := filepath.Join(t.TempDir(), "markup.svg")
			if err := export.SVG(target, frame, c); err != nil {
				t.Fatal(err)
			}
			svg, err := os.ReadFile(target)
			if err != nil || bytes.Contains(svg, []byte("<text")) || bytes.Count(svg, []byte(` d="M`)) != len(o.Children) {
				t.Fatal("markup SVG did not preserve real outlines", err)
			}
			t.Logf("%d independent styled boxes, minimum foreground Cairo IoU %.6f, RGB MAE %.6f bytes", index, worst, mae)
		})
	}
}
