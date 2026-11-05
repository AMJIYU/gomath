package render_test

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"gomath"
	"gomath/examples/scenes"
	"gomath/geom"
	"gomath/render"
	"gomath/svg"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
)

const ssReferenceDir = "../tests/data/render/supersampling"

func ssLoadPNG(t *testing.T, filename string) image.Image {
	t.Helper()
	f, err := os.Open(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	im, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	return im
}
func ssDifference(t *testing.T, got, want image.Image) (mae, rms float64, maximum int) {
	t.Helper()
	if got.Bounds() != want.Bounds() {
		t.Fatal("reference/output dimensions differ")
	}
	for y := got.Bounds().Min.Y; y < got.Bounds().Max.Y; y++ {
		for x := got.Bounds().Min.X; x < got.Bounds().Max.X; x++ {
			a := color.RGBAModel.Convert(got.At(x, y)).(color.RGBA)
			b := color.RGBAModel.Convert(want.At(x, y)).(color.RGBA)
			for _, d := range []int{int(a.R) - int(b.R), int(a.G) - int(b.G), int(a.B) - int(b.B), int(a.A) - int(b.A)} {
				mae += math.Abs(float64(d))
				rms += float64(d * d)
				maximum = max(maximum, int(math.Abs(float64(d))))
			}
		}
	}
	n := float64(got.Bounds().Dx() * got.Bounds().Dy() * 4)
	return mae / n, math.Sqrt(rms / n), maximum
}
func TestSupersamplingIndependentCairoOddSizeReferences(t *testing.T) {
	for _, name := range []string{"crossing", "holes-duplicate", "mixed"} {
		t.Run(name, func(t *testing.T) {
			f, err := os.Open(filepath.Join(ssReferenceDir, name+".svg"))
			if err != nil {
				t.Fatal(err)
			}
			o, err := svg.Read(f, svg.Options{Width: 95})
			f.Close()
			if err != nil {
				t.Fatal(err)
			}
			c := render.DefaultConfig()
			c.Width, c.Height, c.FrameWidth, c.Workers, c.Background = 95, 63, 95, 1, gomath.Transparent
			for _, scale := range []int{1, 2, 4, 8} {
				c.Supersampling = scale
				r, err := render.NewRasterizer(c)
				if err != nil {
					t.Fatal(err)
				}
				got, err := r.Render(&gomath.Frame{Objects: []*gomath.Object{o}})
				if err != nil {
					t.Fatal(err)
				}
				ref := ssLoadPNG(t, filepath.Join(ssReferenceDir, fmt.Sprintf("%s-cairo-ss%d.png", name, scale)))
				mae, rms, maximum := ssDifference(t, got, ref)
				t.Logf("scale=%d MAE=%.6f RMS=%.6f maximum=%d", scale, mae, rms, maximum)
				// These are new fixture-specific bounds; no legacy pixel
				// baseline or generic reference threshold is changed.
				limits := map[string]map[int][3]float64{
					"crossing":        {1: {.21, 1.7, 127}, 2: {.13, .95, 42}, 4: {.12, .85, 21}, 8: {.04, .2, 3}},
					"holes-duplicate": {1: {1.6, 7.2, 65}, 2: {1.1, 3.8, 35}, 4: {.65, 1.9, 15}, 8: {.5, 1.2, 11}},
					"mixed":           {1: {1.6, 5, 60}, 2: {1.6, 5, 60}, 4: {1.6, 5, 60}, 8: {1.6, 5, 60}},
				}[name][scale]
				if mae > limits[0] || rms > limits[1] || float64(maximum) > limits[2] {
					t.Fatal("independent odd-size reference bounds exceeded")
				}
				if name == "holes-duplicate" {
					if a := got.RGBAAt(45, 32).A; a != 0 {
						t.Fatal("hole filled", a)
					}
					if a := got.RGBAAt(12, 12).A; a < 127 || a > 128 {
						t.Fatal("duplicate nonzero contours applied opacity twice", a)
					}
					if got.RGBAAt(94, 62).A != 0 {
						t.Fatal("odd edge lost transparency")
					}
				}
			}
		})
	}
}
func TestSupersamplingPartialFractionsIndependentCairoReference(t *testing.T) {
	scene, err := scenes.Fractions()
	if err != nil {
		t.Fatal(err)
	}
	frame, err := scene.Sample(.35)
	if err != nil {
		t.Fatal(err)
	}
	c := render.DefaultConfig()
	c.Width, c.Height, c.Workers = 640, 360, 1
	nativeRef := ssLoadPNG(t, filepath.Join(ssReferenceDir, "fractions-035-cairo-ss1.png"))
	for _, scale := range []int{1, 2, 4, 8} {
		c.Supersampling = scale
		r, err := render.NewRasterizer(c)
		if err != nil {
			t.Fatal(err)
		}
		got, err := r.Render(frame)
		if err != nil {
			t.Fatal(err)
		}
		mae, rms, maximum := ssDifference(t, got, nativeRef)
		fine := ssLoadPNG(t, filepath.Join(ssReferenceDir, fmt.Sprintf("fractions-035-cairo-ss%d.png", scale)))
		fmae, frms, fmax := ssDifference(t, got, fine)
		t.Logf("scale=%d against Cairo1 MAE=%.6f RMS=%.6f max=%d; against matching Cairo+BOX MAE=%.6f RMS=%.6f max=%d", scale, mae, rms, maximum, fmae, frms, fmax)
		fineLimits := map[int][3]float64{2: {.025, .3, 34}, 4: {.015, .2, 10}, 8: {.01, .15, 9}}
		if scale > 1 {
			limit := fineLimits[scale]
			if fmae > limit[0] || frms > limit[1] || float64(fmax) > limit[2] {
				t.Fatal("matching supersampled independent reference bounds exceeded")
			}
		}
		if scale > 1 && (mae > 1 || maximum > 64) {
			t.Fatal("optional quality failed original independent pixel bounds")
		}
	}
}
func TestSupersamplingAnalyticalBowtieAndPartialGlyphArea(t *testing.T) {
	raw, err := os.ReadFile("../tests/data/render/partial-glyph-aa/cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name string  `json:"name"`
		D    string  `json:"d_pixel"`
		Area float64 `json:"skia_pixel_area"`
	}
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		p, err := svg.ParsePath(tc.D)
		if err != nil {
			t.Fatal(err)
		}
		p = p.Transform(geom.Translation(geom.V(-8, 8)).Mul(geom.Scaling(1, -1, 1)))
		o := gomath.NewObject(p).SetFill(gomath.White).SetStroke(gomath.Transparent, 0)
		for _, scale := range []int{2, 4, 8} {
			c := render.DefaultConfig()
			c.Width, c.Height, c.FrameWidth, c.Workers, c.Background, c.Supersampling = 16, 16, 16, 1, gomath.Transparent, scale
			r, err := render.NewRasterizer(c)
			if err != nil {
				t.Fatal(err)
			}
			got, err := r.Render(&gomath.Frame{Objects: []*gomath.Object{o}})
			if err != nil {
				t.Fatal(err)
			}
			alpha := float64(got.RGBAAt(4, 4).A)
			t.Logf("%s scale=%d alpha=%.0f independent area*255=%.6f error=%.6f", tc.Name, scale, alpha, tc.Area*255, math.Abs(alpha-tc.Area*255))
			if math.Abs(alpha-tc.Area*255) > map[int]float64{2: 33, 4: 10, 8: 3}[scale] {
				t.Fatal("finite supersampling glyph area bound exceeded")
			}
			if tc.Name == "one-pixel-bowtie" && math.Abs(alpha-127.5) > .5 {
				t.Fatal("analytical half-pixel nonzero area lost")
			}
		}
	}
}
func TestSupersamplingReferenceProvenance(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(ssReferenceDir, "provenance.json"))
	if err != nil {
		t.Fatal(err)
	}
	var p struct {
		Generator     string            `json:"generator"`
		GeneratorHash string            `json:"generator_sha256"`
		Files         map[string]string `json:"files_sha256"`
	}
	if err = json.Unmarshal(raw, &p); err != nil {
		t.Fatal(err)
	}
	for name, want := range p.Files {
		b, err := os.ReadFile(filepath.Join(ssReferenceDir, name))
		if err != nil {
			t.Fatal(err)
		}
		if fmt.Sprintf("%x", sha256.Sum256(b)) != want {
			t.Fatalf("fixture %s changed without provenance", name)
		}
	}
	b, err := os.ReadFile("../" + p.Generator)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprintf("%x", sha256.Sum256(b)) != p.GeneratorHash {
		t.Fatal("generator changed without provenance")
	}
}
