package export_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"gomath"
	"gomath/export"
	"gomath/geom"
	"gomath/path"
	"gomath/render"
	"gomath/svg"
	"image/color"
	"image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSVGClipRulesDefinitionsAndRoundTrip(t *testing.T) {
	outline := gomath.Square(60).Path
	inner := gomath.Square(24).Path
	outline.Contours = append(outline.Contours, inner.Contours...)
	even, err := gomath.NewClipWithRule(outline, path.EvenOdd)
	if err != nil {
		t.Fatal(err)
	}
	nonzero, err := gomath.NewClip(outline)
	if err != nil {
		t.Fatal(err)
	}
	left := gomath.Square(64).SetFill(gomath.Red).SetStroke(gomath.Transparent, 0).SetClip(even).Shift(geom.V(-34, 0))
	right := gomath.Square(64).SetFill(gomath.Blue).SetStroke(gomath.Transparent, 0).SetClip(nonzero).Shift(geom.V(34, 0))
	// A mixed-rule ancestor intersection must export independent rule-bearing defs.
	ancestor, err := gomath.NewClip(gomath.Rectangle(128, 56).Path)
	if err != nil {
		t.Fatal(err)
	}
	frame := &gomath.Frame{Objects: []*gomath.Object{gomath.Group(left, right).SetClip(ancestor)}}
	c := render.DefaultConfig()
	c.Width, c.Height, c.FrameWidth, c.Workers, c.Background = 128, 96, 128, 1, gomath.Transparent
	for _, camera := range []bool{false, true} {
		current := frame
		if camera {
			root := frame.Objects[0].Copy().ApplyMatrix(geom.Translation(geom.V(5, 6)).Mul(geom.RotationZ(.2)))
			current = &gomath.Frame{Objects: []*gomath.Object{root}, Camera: &gomath.Camera2D{FrameWidth: 128, Center: geom.V(5, 6), Rotation: .2}}
		}
		filename := filepath.Join(t.TempDir(), "rules.svg")
		if err := export.SVG(filename, current, c); err != nil {
			t.Fatal(err)
		}
		content, err := os.ReadFile(filename)
		if err != nil {
			t.Fatal(err)
		}
		decoder := xml.NewDecoder(bytes.NewReader(content))
		inClip := false
		rules := map[string]int{}
		for {
			token, err := decoder.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			switch token := token.(type) {
			case xml.StartElement:
				if token.Name.Local == "clipPath" {
					inClip = true
				}
				if token.Name.Local == "path" {
					for _, a := range token.Attr {
						if a.Name.Local == "fill-rule" {
							t.Fatal("added general path fill-rule")
						}
						if a.Name.Local == "clip-rule" {
							if !inClip {
								t.Fatal("rule attached outside clipping definition")
							}
							rules[a.Value]++
						}
					}
				}
			case xml.EndElement:
				if token.Name.Local == "clipPath" {
					inClip = false
				}
			}
		}
		if rules["evenodd"] != 1 || rules["nonzero"] != 2 {
			t.Fatal("lost rules or mixed ancestor", camera, rules)
		}
		// The existing importer rejects percentage primitive dimensions; turn
		// only the exporter's full-viewport background into equivalent absolute
		// dimensions. Clip definitions and all painted paths remain unmodified.
		normalized := strings.Replace(string(content), `width="100%" height="100%"`, `width="128" height="96"`, 1)
		imported, err := svg.Read(strings.NewReader(normalized), svg.Options{Width: 128})
		if err != nil {
			t.Fatal(err)
		}
		r, err := render.NewRasterizer(c)
		if err != nil {
			t.Fatal(err)
		}
		got, err := r.Render(&gomath.Frame{Objects: []*gomath.Object{imported}})
		if err != nil {
			t.Fatal(err)
		}
		want, err := r.Render(frame)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got.Pix, want.Pix) {
			t.Fatal("export/import changed clip pixels", camera)
		}
		if got.RGBAAt(30, 48).A != 0 || got.RGBAAt(98, 48).A != 255 {
			t.Fatal("evenodd hole/nonzero interior lost")
		}
	}
}
func TestSVGClipNonzeroDefaultExportBytesUnchanged(t *testing.T) {
	c := render.DefaultConfig()
	c.Width, c.Height = 96, 64
	p := gomath.Circle(2).Path
	old, err := gomath.NewClip(p)
	if err != nil {
		t.Fatal(err)
	}
	explicit, err := gomath.NewClipWithRule(p, path.NonZero)
	if err != nil {
		t.Fatal(err)
	}
	o := gomath.Square(5).SetClip(old)
	directory := t.TempDir()
	a, b := filepath.Join(directory, "a.svg"), filepath.Join(directory, "b.svg")
	if err := export.SVG(a, &gomath.Frame{Objects: []*gomath.Object{o}}, c); err != nil {
		t.Fatal(err)
	}
	o.SetClip(explicit)
	if err := export.SVG(b, &gomath.Frame{Objects: []*gomath.Object{o}}, c); err != nil {
		t.Fatal(err)
	}
	aa, err := os.ReadFile(a)
	if err != nil {
		t.Fatal(err)
	}
	bb, err := os.ReadFile(b)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(aa, bb) || !strings.Contains(string(aa), `clip-rule="nonzero"`) {
		t.Fatal("default export changed")
	}
}

func TestSVGActualEvenOddClipIndependentCairoExport(t *testing.T) {
	const directory = "../tests/data/export/clip-evenodd"
	outer, inner := gomath.Circle(25).Path, gomath.Circle(10).Path
	outer.Contours = append(outer.Contours, inner.Contours...)
	even, err := gomath.NewClipWithRule(outer, path.EvenOdd)
	if err != nil {
		t.Fatal(err)
	}
	nonzero, err := gomath.NewClip(outer)
	if err != nil {
		t.Fatal(err)
	}
	left := gomath.Square(60).SetFill(gomath.RGB(48.0/255, 160.0/255, 224.0/255)).SetStroke(gomath.Transparent, 0).SetOpacity(.7).SetClip(even).Shift(geom.V(-31, 0))
	right := gomath.Square(60).SetFill(gomath.RGB(224.0/255, 80.0/255, 48.0/255)).SetStroke(gomath.Transparent, 0).SetClip(nonzero).Shift(geom.V(31, 0))
	ancestor, err := gomath.NewClip(gomath.Rectangle(120, 44).Path)
	if err != nil {
		t.Fatal(err)
	}
	frame := &gomath.Frame{Objects: []*gomath.Object{gomath.Group(left, right).SetClip(ancestor)}}
	c := render.DefaultConfig()
	c.Width, c.Height, c.FrameWidth, c.Workers, c.Background = 128, 96, 128, 1, gomath.Transparent
	filename := filepath.Join(t.TempDir(), "current.svg")
	if err = export.SVG(filename, frame, c); err != nil {
		t.Fatal(err)
	}
	current, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	frozen, err := os.ReadFile(filepath.Join(directory, "actual.svg"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(current, frozen) {
		t.Fatal("actual export changed since independent Cairo interpretation; regenerate/review its provenance")
	}
	r, err := render.NewRasterizer(c)
	if err != nil {
		t.Fatal(err)
	}
	got, err := r.Render(frame)
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(filepath.Join(directory, "cairo.png"))
	if err != nil {
		t.Fatal(err)
	}
	ref, err := png.Decode(file)
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
	sum, squares, maximum := 0.0, 0.0, 0
	for y := range 96 {
		for x := range 128 {
			a, b := got.RGBAAt(x, y), color.RGBAModel.Convert(ref.At(x, y)).(color.RGBA)
			for _, d := range []int{int(a.R) - int(b.R), int(a.G) - int(b.G), int(a.B) - int(b.B), int(a.A) - int(b.A)} {
				sum += math.Abs(float64(d))
				squares += float64(d * d)
				maximum = max(maximum, int(math.Abs(float64(d))))
			}
		}
	}
	mae, rms := sum/(128*96*4), math.Sqrt(squares/(128*96*4))
	t.Logf("actual SVG independently interpreted with Cairo: MAE=%.6f RMS=%.6f max=%d", mae, rms, maximum)
	if mae > .1 || rms > .6 || maximum > 24 {
		t.Fatal("actual clip export/reference bounds exceeded")
	}
	raw, err := os.ReadFile(filepath.Join(directory, "provenance.json"))
	if err != nil {
		t.Fatal(err)
	}
	var p struct {
		Generator   string
		Hash        string            `json:"generator_sha256"`
		GoGenerator string            `json:"go_generator"`
		GoHash      string            `json:"go_generator_sha256"`
		Files       map[string]string `json:"files_sha256"`
	}
	if err = json.Unmarshal(raw, &p); err != nil {
		t.Fatal(err)
	}
	for name, want := range p.Files {
		data, err := os.ReadFile(filepath.Join(directory, name))
		if err != nil {
			t.Fatal(err)
		}
		if fmt.Sprintf("%x", sha256.Sum256(data)) != want {
			t.Fatal("export fixture hash", name)
		}
	}
	for name, want := range map[string]string{p.Generator: p.Hash, p.GoGenerator: p.GoHash} {
		data, err := os.ReadFile("../" + name)
		if err != nil {
			t.Fatal(err)
		}
		if fmt.Sprintf("%x", sha256.Sum256(data)) != want {
			t.Fatal("export generator hash", name)
		}
	}
}
