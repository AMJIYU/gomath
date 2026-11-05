package code

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sync"
	"testing"

	"golang.org/x/image/font/gofont/gomono"
	"gomath"
	"gomath/export"
	"gomath/geom"
	"gomath/path"
	"gomath/render"
)

type refContour struct {
	Start  []float64
	Curves [][][]float64
	Closed bool
}
type refGlyph struct {
	Rune, Offset, Line, Column, Token int
	Color                             string
	BBox                              []float64
	Contours                          []refContour
}
type refRecord struct {
	Name, Source        string
	Numbers, Background bool
	First, Tab          int
	Size                float64
	Lines               []Line
	Tokens              []Token
	Metrics             map[string]float64
	Glyphs              []refGlyph
	NumberGlyphs        []refGlyph `json:"number_glyphs"`
	PNGSHA256           string     `json:"png_sha256"`
	SVGSHA256           string     `json:"svg_sha256"`
}
type refFrame struct {
	Name, Mode string
	Alpha      float64
	Glyphs     []refGlyph
	PNGSHA256  string `json:"png_sha256"`
	SVGSHA256  string `json:"svg_sha256"`
}
type codeRefs struct {
	FontSHA256      string `json:"font_sha256"`
	GeneratorSHA256 string `json:"generator_sha256"`
	Records         []refRecord
	Frames          []refFrame
}

func references(t *testing.T) codeRefs {
	t.Helper()
	var r codeRefs
	if err := json.Unmarshal(fixture(t, "reference.json"), &r); err != nil {
		t.Fatal(err)
	}
	if len(r.Records) != 6 || len(r.Frames) != 10 {
		t.Fatal("incomplete independent fixtures")
	}
	h := sha256.Sum256(gomono.TTF)
	if hex.EncodeToString(h[:]) != r.FontSHA256 {
		t.Fatal("Go Mono source hash differs")
	}
	return r
}

func refOptions(ref refRecord) Options {
	o := DefaultOptions()
	o.Size = ref.Size
	o.TabWidth = ref.Tab
	o.FirstLineNumber = ref.First
	o.Background = ref.Background
	o.LineNumbers = ref.Numbers
	return o
}

func comparePath(t *testing.T, actual path.Path, expected []refContour) {
	t.Helper()
	if len(actual.Contours) != len(expected) {
		t.Fatalf("contours: got %d, want %d", len(actual.Contours), len(expected))
	}
	point := func(got geom.Vec3, want []float64) {
		t.Helper()
		// sfnt rounds implied half-design-unit TrueType points; fontTools
		// retains the fraction. One design unit is < .04px in this raster.
		if math.Abs(got.X-want[0]) > 1./2048 || math.Abs(got.Y-want[1]) > 1./2048 {
			t.Fatalf("independent cubic point %v differs from %v", got, want)
		}
		closeValue(t, got.Z, 0)
	}
	for i, want := range expected {
		got := actual.Contours[i]
		point(got.Start, want.Start)
		if got.Closed != want.Closed || len(got.Curves) != len(want.Curves) {
			t.Fatalf("contour %d topology: got %d/%v, want %d/%v", i, len(got.Curves), got.Closed, len(want.Curves), want.Closed)
		}
		for j, curve := range got.Curves {
			for k, p := range []geom.Vec3{curve.P0, curve.P1, curve.P2, curve.P3} {
				point(p, want.Curves[j][k])
			}
		}
	}
}

func referenceRaster(t *testing.T) (*render.Rasterizer, render.Config) {
	t.Helper()
	cfg := render.DefaultConfig()
	cfg.Width = 1200
	cfg.Height = 720
	cfg.FrameWidth = 15
	cfg.Workers = 1
	cfg.Background = gomath.Black
	r, e := render.NewRasterizer(cfg)
	if e != nil {
		t.Fatal(e)
	}
	return r, cfg
}

func verifyFixture(t *testing.T, name, hash string) []byte {
	t.Helper()
	b := fixture(t, name)
	h := sha256.Sum256(b)
	if hex.EncodeToString(h[:]) != hash {
		t.Fatal("independent fixture hash differs", name)
	}
	return b
}

func comparePixels(t *testing.T, r *render.Rasterizer, frame *gomath.Frame, name, pngHash, svgHash string) {
	t.Helper()
	verifyFixture(t, name+".svg", svgHash)
	want, e := png.Decode(bytes.NewReader(verifyFixture(t, name+".png", pngHash)))
	if e != nil {
		t.Fatal(e)
	}
	got, e := r.Render(frame)
	if e != nil {
		t.Fatal(e)
	}
	if got.Bounds() != want.Bounds() {
		t.Fatal("reference raster dimensions differ")
	}
	diff, intersection, union := 0., 0, 0
	for y := range got.Bounds().Dy() {
		for x := range got.Bounds().Dx() {
			a, b := got.RGBAAt(x, y), color.NRGBAModel.Convert(want.At(x, y)).(color.NRGBA)
			diff += math.Abs(float64(a.R)-float64(b.R)) + math.Abs(float64(a.G)-float64(b.G)) + math.Abs(float64(a.B)-float64(b.B))
			aa, bb := int(a.R)+int(a.G)+int(a.B) > 32, int(b.R)+int(b.G)+int(b.B) > 32
			if aa && bb {
				intersection++
			}
			if aa || bb {
				union++
			}
		}
	}
	mae := diff / float64(got.Bounds().Dx()*got.Bounds().Dy()*3)
	iou := float64(intersection) / float64(max(1, union))
	if union == 0 {
		iou = 1
	}
	if mae > .6 || iou < .965 {
		t.Fatalf("%s independent raster RGB MAE %.6f, nonblack IoU %.6f, union %d", name, mae, iou, union)
	}
	// Check foreground colors individually so a large background cannot hide
	// an absent/miscolored keyword, number, comment or other category.
	for _, paint := range []gomath.Color{DefaultPalette().Keyword, DefaultPalette().Identifier, DefaultPalette().Number, DefaultPalette().String, DefaultPalette().Operator, DefaultPalette().Punctuation, DefaultPalette().Comment} {
		p := paint.NRGBA(1)
		in, un, referenceCount := 0, 0, 0
		match := func(a color.NRGBA) bool {
			return math.Abs(float64(a.R)-float64(p.R)) <= 2 && math.Abs(float64(a.G)-float64(p.G)) <= 2 && math.Abs(float64(a.B)-float64(p.B)) <= 2
		}
		for y := range got.Bounds().Dy() {
			for x := range got.Bounds().Dx() {
				a := got.RGBAAt(x, y)
				b := color.NRGBAModel.Convert(want.At(x, y)).(color.NRGBA)
				aa, bb := match(color.NRGBA{a.R, a.G, a.B, a.A}), match(b)
				if bb {
					referenceCount++
				}
				if aa && bb {
					in++
				}
				if aa || bb {
					un++
				}
			}
		}
		threshold := .94
		if referenceCount < 100 {
			threshold = .90
		}
		if referenceCount >= 20 && float64(in)/float64(max(1, un)) < threshold {
			t.Fatalf("%s foreground %v IoU %.6f", name, p, float64(in)/float64(max(1, un)))
		}
	}
	t.Logf("%s RGB MAE %.6f bytes; nonblack IoU %.6f", name, mae, iou)
}

func place(c *Code) { c.Object.Shift(geom.V(-5.25, 2.875)) }

func TestCodeIndependentTokenizerGridAndFontToolsGeometry(t *testing.T) {
	refs := references(t)
	for _, ref := range refs.Records {
		t.Run(ref.Name, func(t *testing.T) {
			c := mustCode(t, ref.Source, refOptions(ref))
			if !reflect.DeepEqual(c.Lines(), ref.Lines) {
				t.Fatal("independent physical lines differ", c.Lines(), ref.Lines)
			}
			if !slices.Equal(c.Tokens(), ref.Tokens) {
				t.Fatal("independent tokenizer differs", c.Tokens(), ref.Tokens)
			}
			m := c.Metrics()
			actual := map[string]float64{"cell_width": m.CellWidth, "line_advance": m.LineAdvance, "ascent": m.Ascent, "descent": m.Descent, "content_width": m.ContentWidth, "left": m.Left, "right": m.Right, "top": m.Top, "bottom": m.Bottom, "lines": float64(m.Lines), "columns": float64(m.Columns), "glyphs": float64(m.Glyphs), "curves": float64(m.Curves)}
			for key, want := range ref.Metrics {
				t.Run(key, func(t *testing.T) { closeValue(t, actual[key], want) })
			}
			if len(c.Glyphs()) != len(ref.Glyphs) {
				t.Fatal("ink-bearing source glyph count")
			}
			for i, g := range c.Glyphs() {
				want := ref.Glyphs[i]
				if int(g.Rune) != want.Rune || g.Offset != want.Offset || g.Line != want.Line || g.Column != want.Column || g.Token != want.Token {
					t.Fatal("independent cell metadata", i, g, want)
				}
				color, e := gomath.Hex(want.Color)
				if e != nil || g.Object.Style.Fill != color {
					t.Fatal("lexical palette", i, e)
				}
				bounds := g.Object.Bounds()
				for j, a := range []float64{bounds.Min.X, bounds.Min.Y, bounds.Max.X, bounds.Max.Y} {
					closeValue(t, a, want.BBox[j])
				}
				t.Run(fmt.Sprintf("glyph-%d", i), func(t *testing.T) { comparePath(t, g.Object.Path, want.Contours) })
			}
			numberIndex := 0
			for i := range c.numberObjects {
				number, _ := c.LineNumber(i)
				for _, g := range number.Children {
					want := ref.NumberGlyphs[numberIndex]
					numberIndex++
					comparePath(t, g.Path, want.Contours)
				}
			}
			if numberIndex != len(ref.NumberGlyphs) {
				t.Fatal("line-number glyph count")
			}
			place(c)
			r, cfg := referenceRaster(t)
			frame := &gomath.Frame{Objects: []*gomath.Object{c.Object}}
			comparePixels(t, r, frame, ref.Name, ref.PNGSHA256, ref.SVGSHA256)
			target := filepath.Join(t.TempDir(), "code.svg")
			if e := export.SVG(target, frame, cfg); e != nil {
				t.Fatal(e)
			}
			svg, e := os.ReadFile(target)
			if e != nil || bytes.Contains(svg, []byte("<text")) || bytes.Contains(svg, []byte("<image")) {
				t.Fatal("Code export lost vector outlines", e)
			}
		})
	}
}

func TestCodeIndependentWriteTransformInteriorFrames(t *testing.T) {
	refs := references(t)
	source, target := refs.Records[4], refs.Records[5]
	for _, mode := range []string{"write", "transform"} {
		t.Run(mode, func(t *testing.T) {
			c := mustCode(t, source.Source, refOptions(source))
			goal := mustCode(t, target.Source, refOptions(target))
			place(c)
			place(goal)
			original := append([]Glyph(nil), c.Glyphs()...)
			scene := gomath.NewScene()
			if mode == "write" {
				if e := scene.Play(1, gomath.Write(c.Object)); e != nil {
					t.Fatal(e)
				}
			} else {
				if e := scene.Add(c.Object); e != nil {
					t.Fatal(e)
				}
				if e := scene.Play(1, gomath.Transform(c.Object, goal.Object).WithRate(gomath.Linear)); e != nil {
					t.Fatal(e)
				}
			}
			for i, g := range original {
				if c.Glyphs()[i].Object != g.Object {
					t.Fatal("animation replaced authoring glyph reference")
				}
			}
			r, _ := referenceRaster(t)
			for _, ref := range refs.Frames {
				if ref.Mode != mode {
					continue
				}
				t.Run(ref.Name, func(t *testing.T) {
					frame, e := scene.Sample(ref.Alpha)
					if e != nil {
						t.Fatal(e)
					}
					for i, g := range original {
						actual := frame.Find(g.Object.ID)
						if actual == nil {
							t.Fatal("source glyph ID lost")
						}
						p := actual.Path
						if mode == "write" {
							closeValue(t, actual.Style.Reveal, ref.Alpha)
							p = p.Partial(actual.Style.Reveal)
						}
						p = p.Transform(geom.Translation(geom.V(5.25, -2.875)))
						t.Run(fmt.Sprintf("glyph-%d", i), func(t *testing.T) { comparePath(t, p, ref.Glyphs[i].Contours) })
					}
					comparePixels(t, r, frame, ref.Name, ref.PNGSHA256, ref.SVGSHA256)
				})
			}
			// Scene owns its captured tracks even after subsequent source/target
			// edits. Every returned frame owns independent mutable geometry.
			before, e := scene.Sample(.5)
			if e != nil {
				t.Fatal(e)
			}
			baseline := before.Find(original[0].Object.ID).Path.Clone()
			c.Object.Shift(geom.V(100, 100))
			goal.Object.SetFill(gomath.Red)
			var wg sync.WaitGroup
			for range 8 {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for range 12 {
						f, e := scene.Sample(.5)
						if e != nil {
							t.Error(e)
							return
						}
						g := f.Find(original[0].Object.ID)
						if !reflect.DeepEqual(g.Path, baseline) {
							t.Error("sample changed after authoring edit")
							return
						}
						g.Path.Contours[0].Curves[0].P1.X += 10
					}
				}()
			}
			wg.Wait()
		})
	}
}

func BenchmarkCodeRender100Lines(b *testing.B) {
	c := mustCode(b, "package p\n"+string(bytes.Repeat([]byte("var n = 42 // grid\n"), 100)), DefaultOptions())
	c.Object.Scale(.12).MoveTo(geom.V(0, 0))
	cfg := render.DefaultConfig()
	cfg.Width = 1200
	cfg.Height = 720
	cfg.FrameWidth = 15
	cfg.Workers = 1
	r, e := render.NewRasterizer(cfg)
	if e != nil {
		b.Fatal(e)
	}
	frame := &gomath.Frame{Objects: []*gomath.Object{c.Object}}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, e := r.Render(frame); e != nil {
			b.Fatal(e)
		}
	}
}
