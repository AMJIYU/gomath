package mathtex

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gobolditalic"
	"golang.org/x/image/font/gofont/goitalic"
	"golang.org/x/image/font/gofont/goregular"
)

type partsReference struct {
	GeneratorSHA256 string `json:"generator_sha256"`
	Manim           struct{ Version, Commit string }
	Fonts           []struct{ File, SHA256 string }
	FilesSHA256     map[string]string `json:"files_sha256"`
	Records         []struct {
		Name                   string
		Style                  text.FontStyle
		Sources                []string
		Advance, Height, Depth float64
		Parts                  []struct {
			Index                               int
			Source                              string
			X, Baseline, Advance, Height, Depth float64
			BBox                                []float64
		}
	}
}

func partsReadFile(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("../tests/data/mathtex/parts", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}
func partsHash(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func partsReadReference(t *testing.T) partsReference {
	t.Helper()
	var ref partsReference
	if err := json.Unmarshal(partsReadFile(t, "reference.json"), &ref); err != nil {
		t.Fatal(err)
	}
	g, err := os.ReadFile("../tests/reference/mathtex_parts.py")
	if err != nil {
		t.Fatal(err)
	}
	if partsHash(g) != ref.GeneratorSHA256 || len(ref.Records) < 8 || len(ref.Fonts) != 4 || ref.Manim.Commit != "16ea443e2a5f1d79db0a721161411ce916d6e733" || ref.Manim.Version != "0.22.1" {
		t.Fatal("incomplete/source-unpinned independent reference")
	}
	for i, data := range [][]byte{goregular.TTF, gobold.TTF, goitalic.TTF, gobolditalic.TTF} {
		if partsHash(data) != ref.Fonts[i].SHA256 {
			t.Fatal("independent source-font hash mismatch", i)
		}
	}
	for name, want := range ref.FilesSHA256 {
		if partsHash(partsReadFile(t, name)) != want {
			t.Fatal("independent fixture hash mismatch", name)
		}
	}
	return ref
}

func partsRaster(t *testing.T, object *gomath.Object) *image.RGBA {
	t.Helper()
	c := render.DefaultConfig()
	c.Width, c.Height, c.Workers, c.FrameWidth, c.Background = 768, 768, 1, 6, gomath.Black
	r, err := render.NewRasterizer(c)
	if err != nil {
		t.Fatal(err)
	}
	copy := object.Copy().Shift(geom.V(-2.5, 0))
	im, err := r.Render(&gomath.Frame{Objects: []*gomath.Object{copy}})
	if err != nil {
		t.Fatal(err)
	}
	return im
}

func partsComparePNG(t *testing.T, name string, got *image.RGBA) {
	t.Helper()
	file, err := os.Open(filepath.Join("../tests/data/mathtex/parts", name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	want, err := png.Decode(file)
	if err != nil || want.Bounds() != got.Bounds() {
		t.Fatal("reference image dimensions", err)
	}
	var sum, squares float64
	maximum, intersection, union := 0, 0, 0
	for y := range got.Bounds().Dy() {
		for x := range got.Bounds().Dx() {
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
	den := float64(got.Bounds().Dx() * got.Bounds().Dy() * 3)
	mae, rms, iou := sum/den, math.Sqrt(squares/den), float64(intersection)/float64(max(1, union))
	t.Logf("%s independent fontTools/manual/Cairo MAE=%.9g RMS=%.9g max=%d IoU=%.9g", name, mae, rms, maximum, iou)
	if mae > .08 || rms > 1 || maximum > 64 || iou < .97 || intersection < 30 {
		t.Fatal("independent geometry/pixel mismatch", name, mae, rms, maximum, iou)
	}
}

func TestPartsIndependentFontGeometryAndCairo(t *testing.T) {
	refs := partsReadReference(t)
	for _, ref := range refs.Records {
		t.Run(ref.Name, func(t *testing.T) {
			o := DefaultOptions()
			o.Size, o.Style = 1, ref.Style
			p, m, err := LayoutParts(ref.Sources, o)
			if err != nil {
				t.Fatal(err)
			}
			tolerance := float64(len(ref.Sources)+1) / 2048
			check := func(a, b float64) {
				t.Helper()
				if math.Abs(a-b) > tolerance {
					t.Fatal("independent font/layout metric", a, b, tolerance)
				}
			}
			check(m.Advance, ref.Advance)
			check(m.Height, ref.Height)
			check(m.Depth, ref.Depth)
			for _, part := range ref.Parts {
				info, ok := p.Info(part.Index)
				if !ok || info.Source != part.Source {
					t.Fatal("exact fragment record mismatch")
				}
				check(info.X, part.X)
				check(info.Advance, part.Advance)
				check(info.Height, part.Height)
				check(info.Depth, part.Depth)
				object, _ := p.Part(part.Index)
				bounds := object.Bounds()
				if len(part.BBox) == 0 {
					if bounds.Valid {
						t.Fatal("independent zero-ink fragment leaked geometry")
					}
					continue
				}
				if !bounds.Valid {
					t.Fatal("independent visible glyphs lost")
				}
				for i, value := range []float64{bounds.Min.X, bounds.Min.Y, bounds.Max.X, bounds.Max.Y} {
					check(value, part.BBox[i])
				}
			}
			partsComparePNG(t, ref.Name, partsRaster(t, p.Object()))
		})
	}
}

func TestPartsIndependentInteriorWholeAndFragmentTransforms(t *testing.T) {
	ref := partsReadReference(t).Records[0]
	for _, kind := range []string{"whole", "part"} {
		o := DefaultOptions()
		o.Size = 1
		p, err := NewParts(ref.Sources, o)
		if err != nil {
			t.Fatal(err)
		}
		from := p.Object()
		if kind == "part" {
			from, _ = p.Part(1)
		}
		id := from.ID
		frozen, err := p.Snapshot()
		if err != nil {
			t.Fatal(err)
		}
		before := frozen.Object()
		target := from.Copy().ApplyMatrix(geom.Translation(geom.V(.2, .3)).Mul(geom.Scaling(.8, 1.1, 1)))
		scene := gomath.NewScene()
		scene.Add(p.Object())
		if err := scene.Play(1, gomath.Transform(from, target).WithRate(gomath.Linear)); err != nil {
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
			if f.Find(id) == nil {
				t.Fatal("interior Transform lost selected ID")
			}
			for i := range p.Len() {
				info, _ := p.Info(i)
				if f.Find(info.ObjectID) == nil {
					t.Fatal("interior Transform lost fragment root")
				}
			}
			partsComparePNG(t, "transform-"+kind+"-"+tc.label, partsRaster(t, f.Find(p.Object().ID)))
			partsSameGeometry(t, frozen.Object(), before)
		}
	}
}
