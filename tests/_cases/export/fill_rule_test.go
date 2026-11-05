package export_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"gomath"
	"gomath/export"
	"gomath/geom"
	"gomath/path"
	"gomath/render"
	"gomath/svg"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func ordinaryExportFrame(t *testing.T) *gomath.Frame {
	t.Helper()
	p := gomath.Circle(25).Path
	p.Contours = append(p.Contours, gomath.Circle(10).Path.Contours...)
	left := gomath.NewObject(p).SetFill(gomath.RGB(48.0/255, 160.0/255, 224.0/255)).SetStroke(gomath.Transparent, 0).SetFillRule(path.EvenOdd).SetOpacity(.7)
	left.Style.Reveal = .72
	left.Shift(geom.V(-31, 0))
	g, e := gomath.NewLinearGradient(geom.V(-25, 0), geom.V(25, 0), []gomath.GradientStop{{Offset: 0, Color: gomath.Color{R: 1, A: .7}}, {Offset: 1, Color: gomath.Color{B: 1, A: .7}}})
	if e != nil {
		t.Fatal(e)
	}
	right := gomath.NewObject(p).SetFillGradient(g).SetStroke(gomath.Transparent, 0).SetFillRule(path.EvenOdd).Shift(geom.V(31, 0))
	clip, e := gomath.NewClip(gomath.Rectangle(120, 44).Path)
	if e != nil {
		t.Fatal(e)
	}
	return &gomath.Frame{Objects: []*gomath.Object{gomath.Group(left, right).SetClip(clip)}}
}
func ordinaryExportConfig() render.Config {
	c := render.DefaultConfig()
	c.Width, c.Height, c.FrameWidth, c.Workers, c.Background = 128, 96, 128, 1, gomath.Transparent
	return c
}
func ordinaryExportDifference(t *testing.T, a, b image.Image) (float64, float64, int) {
	t.Helper()
	if a.Bounds() != b.Bounds() {
		t.Fatal("dimensions")
	}
	sum, squared, mx := 0.0, 0.0, 0
	for y := range a.Bounds().Dy() {
		for x := range a.Bounds().Dx() {
			p, q := color.RGBAModel.Convert(a.At(x, y)).(color.RGBA), color.RGBAModel.Convert(b.At(x, y)).(color.RGBA)
			for _, d := range []int{int(p.R) - int(q.R), int(p.G) - int(q.G), int(p.B) - int(q.B), int(p.A) - int(q.A)} {
				sum += math.Abs(float64(d))
				squared += float64(d * d)
				mx = max(mx, int(math.Abs(float64(d))))
			}
		}
	}
	n := float64(a.Bounds().Dx() * a.Bounds().Dy() * 4)
	return sum / n, math.Sqrt(squared / n), mx
}
func TestSVGOrdinaryEvenOddActualExportReferenceAndRoundtrip(t *testing.T) {
	const directory = "../tests/data/export/fill-evenodd"
	frame := ordinaryExportFrame(t)
	c := ordinaryExportConfig()
	for _, camera := range []bool{false, true} {
		current := frame
		if camera {
			current = &gomath.Frame{Objects: []*gomath.Object{frame.Objects[0].Copy().ApplyMatrix(geom.Translation(geom.V(7, -3)).Mul(geom.RotationZ(.3)))}, Camera: &gomath.Camera2D{FrameWidth: 128, Center: geom.V(7, -3), Rotation: .3}}
		}
		filename := filepath.Join(t.TempDir(), "current.svg")
		if e := export.SVG(filename, current, c); e != nil {
			t.Fatal(e)
		}
		content, e := os.ReadFile(filename)
		if e != nil {
			t.Fatal(e)
		}
		if strings.Count(string(content), `fill-rule="evenodd"`) != 2 || strings.Contains(string(content), `fill-rule="nonzero"`) {
			t.Fatal("ordinary rules not explicitly exported only when opted in")
		}
		if !camera {
			frozen, e := os.ReadFile(filepath.Join(directory, "actual.svg"))
			if e != nil {
				t.Fatal(e)
			}
			if !bytes.Equal(content, frozen) {
				t.Fatal("actual export changed since independent interpretation")
			}
		}
		// Only the existing full-viewport background percentage is normalized;
		// ordinary paths, gradient paint servers and clip definitions are intact.
		data := strings.Replace(string(content), `width="100%" height="100%"`, `width="128" height="96"`, 1)
		o, e := svg.Read(strings.NewReader(data), svg.Options{Width: 128})
		if e != nil {
			t.Fatal(e)
		}
		for _, scale := range []int{1, 4} {
			c.Supersampling = scale
			r, e := render.NewRasterizer(c)
			if e != nil {
				t.Fatal(e)
			}
			got, e := r.Render(current)
			if e != nil {
				t.Fatal(e)
			}
			f, e := os.Open(filepath.Join(directory, fmt.Sprintf("cairo-ss%d.png", scale)))
			if e != nil {
				t.Fatal(e)
			}
			want, e := png.Decode(f)
			f.Close()
			if e != nil {
				t.Fatal(e)
			}
			mae, rms, mx := ordinaryExportDifference(t, got, want)
			t.Logf("camera=%t ss%d MAE=%.6f RMS=%.6f max=%d", camera, scale, mae, rms, mx)
			if mae > .4 || rms > 2 || mx > 32 {
				t.Fatal("actual export/reference mismatch")
			}
			imported, e := r.Render(&gomath.Frame{Objects: []*gomath.Object{o}})
			if e != nil {
				t.Fatal(e)
			}
			mae, rms, mx = ordinaryExportDifference(t, got, imported)
			if mae > .01 || rms > .1 || mx > 1 {
				t.Fatal("export/import changed ordinary fill", mae, rms, mx)
			}
		}
	}
	raw, e := os.ReadFile(filepath.Join(directory, "provenance.json"))
	if e != nil {
		t.Fatal(e)
	}
	var p struct {
		Generator string
		Hash      string            `json:"generator_sha256"`
		Go        string            `json:"go_generator"`
		GoHash    string            `json:"go_generator_sha256"`
		Files     map[string]string `json:"files_sha256"`
	}
	if e = json.Unmarshal(raw, &p); e != nil {
		t.Fatal(e)
	}
	for name, want := range p.Files {
		b, e := os.ReadFile(filepath.Join(directory, name))
		if e != nil {
			t.Fatal(e)
		}
		if fmt.Sprintf("%x", sha256.Sum256(b)) != want {
			t.Fatal("fixture hash", name)
		}
	}
	for name, want := range map[string]string{p.Generator: p.Hash, p.Go: p.GoHash} {
		b, e := os.ReadFile("../" + name)
		if e != nil {
			t.Fatal(e)
		}
		if fmt.Sprintf("%x", sha256.Sum256(b)) != want {
			t.Fatal("generator hash", name)
		}
	}
}
func TestSVGOrdinaryFillRuleDefaultAndInvalidAtomicExport(t *testing.T) {
	c := ordinaryExportConfig()
	o := gomath.Square(40).SetFill(gomath.Red)
	dir := t.TempDir()
	name := filepath.Join(dir, "output.svg")
	if e := export.SVG(name, &gomath.Frame{Objects: []*gomath.Object{o}}, c); e != nil {
		t.Fatal(e)
	}
	old, e := os.ReadFile(name)
	if e != nil {
		t.Fatal(e)
	}
	o.SetFillRule(path.NonZero)
	explicit := filepath.Join(dir, "explicit.svg")
	if e := export.SVG(explicit, &gomath.Frame{Objects: []*gomath.Object{o}}, c); e != nil {
		t.Fatal(e)
	}
	current, e := os.ReadFile(explicit)
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(old, current) || bytes.Contains(current, []byte("fill-rule")) {
		t.Fatal("default SVG bytes changed")
	}
	o.SetFillRule(255)
	invalid := filepath.Join(dir, "invalid.svg")
	if e := export.SVG(invalid, &gomath.Frame{Objects: []*gomath.Object{o}}, c); e == nil {
		t.Fatal("invalid rule export accepted")
	}
	if _, e := os.Stat(invalid); !os.IsNotExist(e) {
		t.Fatal("invalid rule published new output", e)
	}
	if e := export.SVG(name, &gomath.Frame{Objects: []*gomath.Object{o}}, c); e == nil {
		t.Fatal("invalid rule export accepted")
	}
	after, e := os.ReadFile(name)
	if e != nil || !bytes.Equal(after, old) {
		t.Fatal("invalid export published output", e)
	}
}
