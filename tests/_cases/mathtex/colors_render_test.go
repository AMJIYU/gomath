package mathtex

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"golang.org/x/image/font/gofont/goregular"
	"gomath"
	"gomath/export"
	"gomath/geom"
	"gomath/render"
	"image/color"
	"image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
)

type mathColorReference struct {
	Font    struct{ SHA256 string } `json:"font"`
	Records []struct {
		Name, Source string
		Colors       []string
		SVGSHA256    string `json:"svg_sha256"`
		PNGSHA256    string `json:"png_sha256"`
	}
}

func TestMathColorIndependentCairoSVG(t *testing.T) {
	dir := filepath.Join("..", "tests", "data", "mathtex", "colors")
	data, err := os.ReadFile(filepath.Join(dir, "reference.json"))
	if err != nil {
		t.Fatal(err)
	}
	var refs mathColorReference
	if err := json.Unmarshal(data, &refs); err != nil {
		t.Fatal(err)
	}
	fontHash := sha256.Sum256(goregular.TTF)
	if len(refs.Records) != 2 || refs.Font.SHA256 != hex.EncodeToString(fontHash[:]) {
		t.Fatal("incomplete references / source font mismatch")
	}
	c := render.DefaultConfig()
	c.Width, c.Height, c.Workers, c.FrameWidth, c.Background = 512, 512, 1, 2.5, gomath.Black
	r, err := render.NewRasterizer(c)
	if err != nil {
		t.Fatal(err)
	}
	worst := 1.0
	for _, ref := range refs.Records {
		for _, file := range []struct{ name, hash string }{{ref.Name + ".svg", ref.SVGSHA256}, {ref.Name + ".png", ref.PNGSHA256}} {
			data, err := os.ReadFile(filepath.Join(dir, file.name))
			if err != nil {
				t.Fatal(err)
			}
			hash := sha256.Sum256(data)
			if hex.EncodeToString(hash[:]) != file.hash {
				t.Fatal("independent fixture hash mismatch", file.name)
			}
		}
		data, err := os.ReadFile(filepath.Join(dir, ref.Name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		expected, err := png.Decode(bytes.NewReader(data))
		if err != nil || expected.Bounds().Dx() != 512 || expected.Bounds().Dy() != 512 {
			t.Fatal("invalid Cairo reference", err)
		}
		opt := DefaultOptions()
		opt.Size = 1
		o, err := New(ref.Source, opt)
		if err != nil {
			t.Fatal(err)
		}
		// Manual reference origin (160,256) at 204.8 px/em, y up.
		o.Shift(geom.V(-.46875, 0))
		actual, err := r.Render(&gomath.Frame{Objects: []*gomath.Object{o}})
		if err != nil {
			t.Fatal(err)
		}
		totalDifference := 0
		for y := range 512 {
			for x := range 512 {
				a := actual.RGBAAt(x, y)
				b := color.NRGBAModel.Convert(expected.At(x, y)).(color.NRGBA)
				totalDifference += colorByteDiff(a.R, b.R) + colorByteDiff(a.G, b.G) + colorByteDiff(a.B, b.B)
			}
		}
		mae := float64(totalDifference) / (512 * 512 * 3)
		if mae > 1 {
			t.Fatal("independent whole image RGB MAE > 1 byte", ref.Name, mae)
		}
		for _, paint := range ref.Colors {
			rgb, err := hex.DecodeString(paint)
			if err != nil || len(rgb) != 3 {
				t.Fatal("invalid reference paint", paint)
			}
			matches := func(r, g, b uint8) bool {
				return colorByteDiff(r, rgb[0]) <= 2 && colorByteDiff(g, rgb[1]) <= 2 && colorByteDiff(b, rgb[2]) <= 2
			}
			intersection, union := 0, 0
			for y := range 512 {
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
			if intersection < 100 || iou < .95 {
				t.Fatal("independent per-color interior mask", ref.Name, paint, intersection, iou)
			}
			t.Logf("%s #%s: interior IoU %.6f; whole image RGB MAE %.6f", ref.Name, paint, iou, mae)
		}
	}
	t.Logf("independent CairoSVG color mask minimum IoU %.6f", worst)
}

func colorByteDiff(a, b uint8) int {
	return int(math.Abs(float64(a) - float64(b)))
}

const mathColorPreview = `\begin{matrix}\color{red}\dfrac{\textcolor{blue}{\hat{n}}}{\textcolor[HTML]{008040}{g}}&\textcolor{violet}{\dbinom{\mathbb{N}}{\mathcal{A}}}\\\color{orange}\displaystyle\sum_{\substack{\color{cyan}i=1\\j\in\mathbb{N}}}^{\textcolor{pink}{n}}\frac{1}{i}&\color{teal}\left[\begin{matrix}\color{magenta}a&b\\c&\textcolor{yellow}{d}\end{matrix}\right]\end{matrix}`

func TestMathColorRasterAndSVGDescendants(t *testing.T) {
	o := mustLayout(t, mathColorPreview).object
	o.MoveTo(geom.V(0, 0))
	bounds := o.Bounds()
	c := render.DefaultConfig()
	c.Width, c.Height, c.Workers, c.Background = 960, 480, 1, gomath.Black
	c.FrameWidth = math.Max(bounds.Size().X*1.2, bounds.Size().Y*2*1.25)
	r, err := render.NewRasterizer(c)
	if err != nil {
		t.Fatal(err)
	}
	frame := &gomath.Frame{Objects: []*gomath.Object{o}}
	raster, err := r.Render(frame)
	if err != nil {
		t.Fatal(err)
	}
	paints := map[color.NRGBA]bool{}
	for _, paint := range mathPaints(o) {
		paints[paint.NRGBA(1)] = true
	}
	for paint := range paints {
		pixels := 0
		for y := range c.Height {
			for x := range c.Width {
				a := raster.RGBAAt(x, y)
				if colorByteDiff(a.R, paint.R) <= 1 && colorByteDiff(a.G, paint.G) <= 1 && colorByteDiff(a.B, paint.B) <= 1 {
					pixels++
				}
			}
		}
		if pixels < 5 {
			t.Fatal("visible descendant color missing from raster", paint, pixels)
		}
	}
	filename := filepath.Join(t.TempDir(), "colors.svg")
	if err := export.SVG(filename, frame, c); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	var styles []gomath.Style
	o.Walk(func(n *gomath.Object) {
		if len(n.Path.Contours) > 0 {
			styles = append(styles, n.Style)
		}
	})
	decoder := xml.NewDecoder(bytes.NewReader(data))
	index := 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		if start.Name.Local == "text" || start.Name.Local == "image" {
			t.Fatal("colored glyph delegated to external font/image")
		}
		attrs := map[string]string{}
		for _, attr := range start.Attr {
			attrs[attr.Name.Local] = attr.Value
		}
		if start.Name.Local != "path" || attrs["d"] == "" {
			continue
		}
		if index >= len(styles) {
			t.Fatal("unexpected drawable SVG path")
		}
		s := styles[index]
		for _, check := range []struct {
			attr  string
			paint gomath.Color
		}{{"fill", s.Fill}, {"stroke", s.Stroke}} {
			c := check.paint.NRGBA(1)
			want := fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
			alpha, err := strconv.ParseFloat(attrs[check.attr+"-opacity"], 64)
			if attrs[check.attr] != want || err != nil || math.Abs(alpha-check.paint.A) > 1e-6 {
				t.Fatal("SVG descendant paint lost", index, check.attr, attrs, want)
			}
		}
		index++
	}
	if index != len(styles) || len(paints) < 8 {
		t.Fatal("incomplete colored raster/SVG tree", index, len(styles), len(paints))
	}
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Rasterizers have their own scratch buffers; all read the same
			// frozen math tree, with no authoring mutation during rendering.
			reader, err := render.NewRasterizer(c)
			if err != nil {
				t.Error(err)
				return
			}
			got, err := reader.Render(frame)
			if err != nil {
				t.Error(err)
				return
			}
			if !bytes.Equal(got.Pix, raster.Pix) {
				t.Error("concurrent color render differs")
			}
		}()
	}
	wg.Wait()
	if preview := os.Getenv("GOMATH_TYPESET_PREVIEW_DIR"); preview != "" {
		if !filepath.IsAbs(preview) {
			t.Fatal("preview output directory must be absolute")
		}
		if err := os.MkdirAll(preview, 0755); err != nil {
			t.Fatal(err)
		}
		if err := export.PNG(filepath.Join(preview, "math-colors.png"), raster); err != nil {
			t.Fatal(err)
		}
		if err := export.SVG(filepath.Join(preview, "math-colors.svg"), frame, c); err != nil {
			t.Fatal(err)
		}
	}
}

func TestMathColorCloneMorphAndConcurrentSampling(t *testing.T) {
	const source = `\color{red}\dfrac{\textcolor{green}{\hat{\mathbb{R}}}}{\substack{n\\g}}`
	start := mustLayout(t, source).object
	target := mustLayout(t, strings.NewReplacer("red", "blue", "green", "yellow").Replace(source)).object
	before, after := mathPaints(start), mathPaints(target)
	copy := start.Copy()
	copy.SetColor(gomath.Black)
	if !reflect.DeepEqual(mathPaints(start), before) {
		t.Fatal("copy changed original colors")
	}
	scene := gomath.NewScene()
	if err := scene.Add(start); err != nil {
		t.Fatal(err)
	}
	if err := scene.Play(1, gomath.Transform(start, target).WithRate(gomath.Linear)); err != nil {
		t.Fatal(err)
	}
	check := func(time float64) {
		frame, err := scene.Sample(time)
		if err != nil {
			t.Error(err)
			return
		}
		o := frame.Find(start.ID)
		if o == nil {
			t.Error("morph lost identity")
			return
		}
		got := mathPaints(o)
		if len(got) != len(before) || len(got) != len(after) {
			t.Error("morph lost color descendants")
			return
		}
		for i, color := range got {
			if color != before[i].Lerp(after[i], time) {
				t.Error("morph failed actual descendant color interpolation", time, i, color)
			}
		}
		// Only the private sample changes; later reads retain authored colors.
		o.SetColor(gomath.Black)
	}
	for _, time := range []float64{0, .25, .5, 1} {
		check(time)
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 8 {
				check(float64(i) / 8)
			}
		}()
	}
	wg.Wait()
	if !reflect.DeepEqual(mathPaints(start), after) {
		t.Fatal("sampling mutated authored final paints")
	}
}
