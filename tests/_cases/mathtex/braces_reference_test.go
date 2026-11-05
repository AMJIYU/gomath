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
	"gomath/text"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
)

type braceReference struct {
	GeneratorSHA256 string `json:"generator_sha256"`
	Manim           struct {
		Version, Commit string
		Executed        bool
	}
	Fonts       []struct{ File, SHA256 string }
	FilesSHA256 map[string]string `json:"files_sha256"`
	Records     []struct {
		Name, Source         string
		Style                text.FontStyle
		Width, Height, Depth float64
		BBox                 []float64
		MarkBBox             []float64 `json:"mark_bbox"`
	}
}

func braceRead(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("../tests/data/mathtex/braces", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func braceHash(b []byte) string { sum := sha256.Sum256(b); return hex.EncodeToString(sum[:]) }
func braceReadReference(t *testing.T) braceReference {
	t.Helper()
	var r braceReference
	if err := json.Unmarshal(braceRead(t, "reference.json"), &r); err != nil {
		t.Fatal(err)
	}
	g, err := os.ReadFile("../tests/reference/mathtex_braces.py")
	if err != nil {
		t.Fatal(err)
	}
	if braceHash(g) != r.GeneratorSHA256 || len(r.Fonts) != 4 || len(r.Records) != 8 || len(r.FilesSHA256) != 20 || r.Manim.Commit != "16ea443e2a5f1d79db0a721161411ce916d6e733" || r.Manim.Version != "0.22.1" || r.Manim.Executed {
		t.Fatal("incomplete/unpinned independent reference")
	}
	for i, b := range [][]byte{goregular.TTF, gobold.TTF, goitalic.TTF, gobolditalic.TTF} {
		if braceHash(b) != r.Fonts[i].SHA256 {
			t.Fatal("source font hash mismatch", i)
		}
	}
	for name, want := range r.FilesSHA256 {
		if braceHash(braceRead(t, name)) != want {
			t.Fatal("fixture hash mismatch", name)
		}
	}
	return r
}
func braceComparePNG(t *testing.T, name string, got *image.RGBA) {
	t.Helper()
	f, err := os.Open(filepath.Join("../tests/data/mathtex/braces", name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	want, err := png.Decode(f)
	if err != nil || want.Bounds() != got.Bounds() {
		t.Fatal("bad reference raster", err)
	}
	var sum, squares float64
	maximum, intersection, union := 0, 0, 0
	for y := range 768 {
		for x := range 768 {
			a, b := got.RGBAAt(x, y), color.NRGBAModel.Convert(want.At(x, y)).(color.NRGBA)
			for _, d := range []int{int(a.R) - int(b.R), int(a.G) - int(b.G), int(a.B) - int(b.B)} {
				sum += math.Abs(float64(d))
				squares += float64(d * d)
				maximum = max(maximum, int(math.Abs(float64(d))))
			}
			aa, bb := max(a.R, a.G, a.B) > 128, max(b.R, b.G, b.B) > 128
			if aa && bb {
				intersection++
			}
			if aa || bb {
				union++
			}
		}
	}
	mae, rms, iou := sum/(768*768*3), math.Sqrt(squares/(768*768*3)), float64(intersection)/float64(max(1, union))
	t.Logf("%s independent fontTools/manual/Cairo MAE=%.9g RMS=%.9g max=%d IoU=%.9g", name, mae, rms, maximum, iou)
	if mae > .08 || rms > 1 || maximum > 64 || iou < .97 || intersection < 30 {
		t.Fatal("independent pixel mismatch", name)
	}
}
func TestBraceIndependentFontGeometryAndCairo(t *testing.T) {
	r := braceReadReference(t)
	for _, ref := range r.Records {
		t.Run(ref.Name, func(t *testing.T) {
			opt := DefaultOptions()
			opt.Size, opt.Style = 1, ref.Style
			b, err := layout(ref.Source, opt)
			if err != nil {
				t.Fatal(err)
			}
			check := func(a, b float64) {
				t.Helper()
				if math.Abs(a-b) > 4.0/2048 {
					t.Fatal("source font/manual metric mismatch", a, b)
				}
			}
			check(b.width, ref.Width)
			check(b.height, ref.Height)
			check(b.depth, ref.Depth)
			ink := b.object.Bounds()
			for i, v := range []float64{ink.Min.X, ink.Min.Y, ink.Max.X, ink.Max.Y} {
				check(v, ref.BBox[i])
			}
			cmd := "overbrace"
			if ref.Name == "phantom" {
				cmd = "underbrace"
			}
			mb := named(b.object, "BraceMark "+cmd).Bounds()
			for i, v := range []float64{mb.Min.X, mb.Min.Y, mb.Max.X, mb.Max.Y} {
				check(v, ref.MarkBBox[i])
			}
			braceComparePNG(t, ref.Name, arrayReferenceRaster(t, b.object))
		})
	}
}
func TestBraceIndependentInteriorTransformAndIdentity(t *testing.T) {
	ref := braceReadReference(t).Records[0]
	opt := DefaultOptions()
	opt.Size = 1
	from, err := New(ref.Source, opt)
	if err != nil {
		t.Fatal(err)
	}
	// Capture actual child pointers as well as IDs before authoring.
	mark := named(from, "BraceMark overbrace")
	label := named(from, "BraceUpperLabel")
	markID, labelID := mark.ID, label.ID
	to := from.Copy().ApplyMatrix(geom.Translation(geom.V(.2, .3)).Mul(geom.Scaling(.8, 1.1, 1)))
	scene := gomath.NewScene()
	scene.Add(from)
	if err := scene.Play(1, gomath.Transform(from, to).WithRate(gomath.Linear)); err != nil {
		t.Fatal(err)
	}
	if named(from, "BraceMark overbrace") != mark || named(from, "BraceUpperLabel") != label {
		t.Fatal("authoring replaced child references")
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
		if got == nil || f.Find(markID) == nil || f.Find(labelID) == nil {
			t.Fatal("intermediate identity lost")
		}
		braceComparePNG(t, "transform-"+tc.label, arrayReferenceRaster(t, got))
	}
}
