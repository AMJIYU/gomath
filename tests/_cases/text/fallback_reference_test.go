package text_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"gomath"
	"gomath/export"
	"gomath/geom"
	"gomath/render"
	"gomath/text"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
)

type fallbackReference struct {
	Sources  map[string]string
	Fixtures []struct{ File, SHA256 string }
	Broken   struct{ File, SHA256 string }
	Records  []struct {
		Name, Text   string
		Order        []int
		AllowMissing bool `json:"allow_missing"`
		Metrics      text.Metrics
		SVGSHA256    string `json:"svg_sha256"`
		PNGSHA256    string `json:"png_sha256"`
		Glyphs       []struct {
			Rune, Font                 int
			X, Baseline, Advance, Kern float64
			BBox                       []float64
		}
	}
}

func verifiedReferenceData(t *testing.T, filename, want string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(fallbackDir(), filename))
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	if hex.EncodeToString(digest[:]) != want {
		t.Fatal("independent fixture digest changed", filename)
	}
	return data
}

func TestFallbackIndependentFontToolsAndCairo(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(fallbackDir(), "reference.json"))
	if err != nil {
		t.Fatal(err)
	}
	var reference fallbackReference
	if err := json.Unmarshal(data, &reference); err != nil {
		t.Fatal(err)
	}
	for name, source := range map[string][]byte{"Go-Regular.ttf": goregular.TTF, "Go-Bold.ttf": gobold.TTF} {
		h := sha256.Sum256(source)
		if hex.EncodeToString(h[:]) != reference.Sources[name] {
			t.Fatal("original source font changed", name)
		}
	}
	if len(reference.Fixtures) != 2 || len(reference.Records) != 4 {
		t.Fatal("incomplete independent references")
	}
	fonts := make([]*text.Font, 2)
	for i, fixture := range reference.Fixtures {
		fonts[i], err = text.Parse(verifiedReferenceData(t, fixture.File, fixture.SHA256))
		if err != nil {
			t.Fatal(err)
		}
	}
	verifiedReferenceData(t, reference.Broken.File, reference.Broken.SHA256)
	config := render.DefaultConfig()
	config.Width, config.Height, config.Workers, config.FrameWidth, config.Background = 768, 512, 1, 6, gomath.Black
	rasterizer, err := render.NewRasterizer(config)
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range reference.Records {
		t.Run(ref.Name, func(t *testing.T) {
			list := make([]*text.Font, len(ref.Order))
			for i, index := range ref.Order {
				list[i] = fonts[index]
			}
			opt := text.DefaultOptions()
			opt.Size, opt.AllowMissing = 1, ref.AllowMissing
			o, metrics, err := text.LayoutWithFallback(ref.Text, list, opt)
			if err != nil {
				t.Fatal(err)
			}
			for _, pair := range [][2]float64{{metrics.Width, ref.Metrics.Width}, {metrics.Height, ref.Metrics.Height}, {metrics.Ascent, ref.Metrics.Ascent}, {metrics.Descent, ref.Metrics.Descent}} {
				closeMetric(t, pair[0], pair[1])
			}
			if metrics.Lines != ref.Metrics.Lines {
				t.Fatal("independent line count", metrics)
			}
			index := 0
			for _, glyph := range ref.Glyphs {
				if glyph.BBox == nil {
					continue
				}
				if index >= len(o.Children) || o.Children[index].Name != string(rune(glyph.Rune)) {
					t.Fatal("independent glyph sequence differs", index)
				}
				bounds := o.Children[index].Bounds()
				for i, value := range []float64{bounds.Min.X, bounds.Min.Y, bounds.Max.X, bounds.Max.Y} {
					// sfnt rounds implied quadratic midpoints to design units;
					// fontTools evaluates the original exact quadratic extrema.
					if math.Abs(value-glyph.BBox[i]) > 1./2048 {
						t.Fatal("independent glyph bbox/baseline mismatch", index, i, value, glyph.BBox[i])
					}
				}
				index++
			}
			if index != len(o.Children) || index < 2 {
				t.Fatal("unexpected independent glyph count", index, len(o.Children))
			}
			verifiedReferenceData(t, ref.Name+".svg", ref.SVGSHA256)
			pngData := verifiedReferenceData(t, ref.Name+".png", ref.PNGSHA256)
			want, err := png.Decode(bytes.NewReader(pngData))
			if err != nil || want.Bounds().Dx() != 768 || want.Bounds().Dy() != 512 {
				t.Fatal("bad independent raster", err)
			}
			// Manual reference origin (64,192), 128 pixels per em.
			o.Shift(geom.V(-2.5, .5))
			frame := &gomath.Frame{Objects: []*gomath.Object{o}}
			got, err := rasterizer.Render(frame)
			if err != nil {
				t.Fatal(err)
			}
			intersection, union, difference := 0, 0, 0
			for y := range config.Height {
				for x := range config.Width {
					a := got.RGBAAt(x, y).R
					b, _, _, _ := want.At(x, y).RGBA()
					difference += int(math.Abs(float64(a) - float64(b>>8)))
					aa, bb := a > 127, b > 32767
					if aa && bb {
						intersection++
					}
					if aa || bb {
						union++
					}
				}
			}
			mae := float64(difference) / float64(config.Width*config.Height)
			iou := float64(intersection) / float64(max(1, union))
			if mae > 1 || iou < .97 || intersection < 100 {
				t.Fatal("independent Cairo mask mismatch", mae, iou, intersection)
			}
			// Export must contain real outline paths; no text/system-font lookup.
			target := filepath.Join(t.TempDir(), "fallback.svg")
			if err := export.SVG(target, frame, config); err != nil {
				t.Fatal(err)
			}
			svg, err := os.ReadFile(target)
			if err != nil || bytes.Contains(svg, []byte("<text")) || bytes.Count(svg, []byte(` d="M`)) != len(o.Children) {
				t.Fatal("SVG lost real fallback outlines", err)
			}
			t.Logf("%d analytical fontTools ink boxes; Cairo mask IoU %.6f, intensity MAE %.6f bytes", index, iou, mae)
		})
	}
}

func TestLegacySingleFontControlPointsMetricsAndPixels(t *testing.T) {
	dir := filepath.Join("..", "tests", "data", "text", "legacy")
	data, err := os.ReadFile(filepath.Join(dir, "reference.json"))
	if err != nil {
		t.Fatal(err)
	}
	var refs []struct {
		Source, GeometrySHA256, PixelSHA256, PNG string
		Style                                    text.FontStyle
		Options                                  text.Options
		Metrics                                  text.Metrics
	}
	if err := json.Unmarshal(data, &refs); err != nil || len(refs) != 4 {
		t.Fatal("invalid pre-extension compatibility fixtures", err)
	}
	c := render.DefaultConfig()
	c.Width, c.Height, c.Workers, c.FrameWidth, c.Background = 320, 240, 1, 6, gomath.Black
	r, err := render.NewRasterizer(c)
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range refs {
		f, err := text.DefaultFontForStyle(ref.Style)
		if err != nil {
			t.Fatal(err)
		}
		for _, fallback := range []bool{false, true} {
			var o *gomath.Object
			var m text.Metrics
			if fallback {
				o, m, err = text.LayoutWithFallback(ref.Source, []*text.Font{f}, ref.Options)
			} else {
				o, m, err = text.Layout(ref.Source, f, ref.Options)
			}
			if err != nil {
				t.Fatal(err)
			}
			h := sha256.Sum256(textGeometry(o))
			if hex.EncodeToString(h[:]) != ref.GeometrySHA256 || m != ref.Metrics {
				t.Fatal("single-font pre-extension paths/style/metrics changed", ref.Style, fallback)
			}
			o.Shift(geom.V(-1.5, .4))
			img, err := r.Render(&gomath.Frame{Objects: []*gomath.Object{o}})
			if err != nil {
				t.Fatal(err)
			}
			h = sha256.Sum256(img.Pix)
			if hex.EncodeToString(h[:]) != ref.PixelSHA256 {
				t.Fatal("single-font pre-extension pixels changed", ref.Style, fallback)
			}
			pngData, err := os.ReadFile(filepath.Join(dir, ref.PNG))
			if err != nil {
				t.Fatal(err)
			}
			want, err := png.Decode(bytes.NewReader(pngData))
			if err != nil || want.Bounds() != img.Bounds() {
				t.Fatal("invalid legacy PNG", err)
			}
			for y := range img.Bounds().Dy() {
				for x := range img.Bounds().Dx() {
					a := img.RGBAAt(x, y)
					red, green, blue, alpha := want.At(x, y).RGBA()
					if uint32(a.R)*257 != red || uint32(a.G)*257 != green || uint32(a.B)*257 != blue || uint32(a.A)*257 != alpha {
						t.Fatal("legacy archived PNG differs", x, y)
					}
				}
			}
		}
	}
}
