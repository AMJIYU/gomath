package export

import (
	"bytes"
	"encoding/xml"
	"errors"
	"gomath"
	"gomath/geom"
	"gomath/path"
	"gomath/render"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func exportGradient(t *testing.T, radial bool, stops []gomath.GradientStop) *gomath.Gradient {
	t.Helper()
	var g *gomath.Gradient
	var e error
	if radial {
		g, e = gomath.NewRadialGradient(geom.V(0, 0), 1, stops)
	} else {
		g, e = gomath.NewLinearGradient(geom.V(-60, 0), geom.V(60, 0), stops)
	}
	if e != nil {
		t.Fatal(e)
	}
	return g
}
func exportGradientFrame(t *testing.T, name string) *gomath.Frame {
	t.Helper()
	red, blue := gomath.RGB(1, 0, 0), gomath.RGB(0, 0, 1)
	var o *gomath.Object
	switch name {
	case "linear-hard-hole":
		stops := []gomath.GradientStop{{Offset: 0, Color: red}, {Offset: .4, Color: gomath.RGB(0, 1, 0)}, {Offset: .4, Color: blue}, {Offset: 1, Color: gomath.White}}
		var p path.Builder
		for _, vertices := range [][]geom.Vec3{{geom.V(-72, 52), geom.V(72, 52), geom.V(72, -52), geom.V(-72, -52)}, {geom.V(-20, 20), geom.V(-20, -20), geom.V(20, -20), geom.V(20, 20)}} {
			p.MoveTo(vertices[0])
			for _, v := range vertices[1:] {
				p.LineTo(v)
			}
			p.Close()
		}
		o = gomath.NewObject(p.Path()).SetStroke(gomath.Transparent, 0).SetFillGradient(exportGradient(t, false, stops))
	case "radial-affine-clip":
		g := exportGradient(t, true, []gomath.GradientStop{{Offset: 0, Color: gomath.White}, {Offset: .5, Color: red}, {Offset: 1, Color: blue}})
		m := geom.Identity()
		m[0], m[1], m[4], m[5] = 60, 10, -15, -35
		g, e := g.Transform(m)
		if e != nil {
			t.Fatal(e)
		}
		clip, e := gomath.NewClip(gomath.Rectangle(120, 100).Path)
		if e != nil {
			t.Fatal(e)
		}
		o = gomath.Rectangle(150, 110).SetStroke(gomath.Transparent, 0).SetFillGradient(g).SetClip(clip).SetOpacity(.6)
	case "linear-alpha":
		g := exportGradient(t, false, []gomath.GradientStop{{Offset: 0, Color: gomath.Color{R: 1}}, {Offset: 1, Color: blue}})
		o = gomath.Rectangle(144, 104).SetStroke(gomath.Transparent, 0).SetFillGradient(g)
	default:
		t.Fatal("unknown reference")
	}
	return &gomath.Frame{Objects: []*gomath.Object{o}}
}
func exportGradientConfig() render.Config {
	c := render.DefaultConfig()
	c.Width, c.Height, c.FrameWidth, c.Workers, c.Background = 160, 120, 160, 1, gomath.Transparent
	return c
}

func TestSVGGradientPaintServersAndIndependentInputs(t *testing.T) {
	for _, name := range []string{"linear-hard-hole", "radial-affine-clip", "linear-alpha"} {
		t.Run(name, func(t *testing.T) {
			frame, c := exportGradientFrame(t, name), exportGradientConfig()
			var data bytes.Buffer
			if e := writeSVG(&data, frame, c); e != nil {
				t.Fatal(e)
			}
			content := data.String()
			if strings.Contains(content, "<image") || strings.Contains(content, "data:image/") {
				t.Fatal("gradient was rasterized")
			}
			decoder := xml.NewDecoder(strings.NewReader(content))
			servers, stops, painted := 0, 0, 0
			for {
				token, e := decoder.Token()
				if e == io.EOF {
					break
				}
				if e != nil {
					t.Fatal(e)
				}
				start, ok := token.(xml.StartElement)
				if !ok {
					continue
				}
				attrs := map[string]string{}
				for _, a := range start.Attr {
					attrs[a.Name.Local] = a.Value
				}
				switch start.Name.Local {
				case "linearGradient", "radialGradient":
					servers++
					if attrs["gradientUnits"] != "userSpaceOnUse" || attrs["spreadMethod"] != "pad" || attrs["color-interpolation"] != "sRGB" || attrs["gradientTransform"] == "" {
						t.Fatal("missing paint semantics", attrs)
					}
					if name == "radial-affine-clip" && (start.Name.Local != "radialGradient" || attrs["fx"] != "0" || attrs["fy"] != "0" || attrs["r"] != "1" || attrs["gradientTransform"] != "matrix(60 -15 10 -35 0 0)") {
						t.Fatal("radial affine/focal semantics", attrs)
					}
					if name != "radial-affine-clip" && (attrs["x1"] != "0" || attrs["x2"] != "1" || attrs["gradientTransform"] != "matrix(120 0 -0 120 -60 0)") {
						t.Fatal("linear affine semantics", attrs)
					}
				case "stop":
					stops++
					if attrs["offset"] == "" || attrs["stop-color"] == "" || attrs["stop-opacity"] == "" {
						t.Fatal("missing stop content")
					}
				case "path":
					if attrs["fill"] == "url(#gomath-gradient-0)" {
						painted++
						if name == "radial-affine-clip" && attrs["fill-opacity"] != "0.59999999999999998" {
							t.Fatal("object opacity lost", attrs)
						}
					}
				}
			}
			expectedStops := 2
			if name == "linear-hard-hole" {
				expectedStops = 4
			}
			if name == "radial-affine-clip" {
				expectedStops = 3
			}
			if servers != 1 || painted != 1 || stops != expectedStops {
				t.Fatal("missing paint server/stops", servers, painted, stops)
			}
			// Optional development artifacts are independently consumed by CairoSVG,
			// never used as source-scene goldens by native rendering tests.
			if dir := os.Getenv("GOMATH_GRADIENT_PREVIEW_DIR"); dir != "" {
				if e := os.MkdirAll(dir, 0755); e != nil {
					t.Fatal(e)
				}
				if e := SVG(filepath.Join(dir, name+"-export.svg"), frame, c); e != nil {
					t.Fatal(e)
				}
			}
		})
	}
}
func TestSVGGradientSharedPaintCameraAndClip(t *testing.T) {
	frame := exportGradientFrame(t, "radial-affine-clip")
	o := frame.Objects[0]
	copy := o.Copy()
	frame.Objects = append(frame.Objects, copy)
	var data bytes.Buffer
	if e := writeSVG(&data, frame, exportGradientConfig()); e != nil {
		t.Fatal(e)
	}
	if strings.Count(data.String(), "<radialGradient ") != 1 {
		t.Fatal("immutable shared paints not reused")
	}
	// Public SVG export projects camera along with the vector field and masks.
	frame.Objects = frame.Objects[:1]
	o.Rotate(.6).Shift(geom.V(7, 9))
	camera := gomath.Camera2D{Center: geom.V(7, 9), Rotation: .6, FrameWidth: 160}
	frame.Camera = &camera
	file := filepath.Join(t.TempDir(), "camera.svg")
	if e := SVG(file, frame, exportGradientConfig()); e != nil {
		t.Fatal(e)
	}
	dataBytes, e := os.ReadFile(file)
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(string(dataBytes), "<clipPath ") || !strings.Contains(string(dataBytes), "gradientTransform=") {
		t.Fatal("paint or clip lost in camera export")
	}
	if frame.Camera == nil {
		t.Fatal("camera source mutated")
	}
}

func TestSVGGradientEmptyGroupReferencesResolve(t *testing.T) {
	stops := []gomath.GradientStop{{Offset: 0, Color: gomath.Red}, {Offset: 1, Color: gomath.Blue}}
	child := gomath.Square(1)
	group := gomath.Group(child).SetFillGradient(exportGradient(t, false, stops))
	child.SetFillGradient(exportGradient(t, true, stops))
	var data bytes.Buffer
	if err := writeSVG(&data, &gomath.Frame{Objects: []*gomath.Object{group}}, exportGradientConfig()); err != nil {
		t.Fatal(err)
	}
	decoder := xml.NewDecoder(&data)
	definitions := map[string]bool{}
	var references []string
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
		for _, attr := range start.Attr {
			if (start.Name.Local == "linearGradient" || start.Name.Local == "radialGradient") && attr.Name.Local == "id" {
				definitions[attr.Value] = true
			}
			if start.Name.Local == "path" && attr.Name.Local == "fill" && strings.HasPrefix(attr.Value, "url(#") {
				references = append(references, strings.TrimSuffix(strings.TrimPrefix(attr.Value, "url(#"), ")"))
			}
		}
	}
	if len(definitions) != 2 || len(references) != 2 {
		t.Fatal("missing group/child paint servers or references", definitions, references)
	}
	for _, ref := range references {
		if !definitions[ref] {
			t.Fatal("unresolved paint reference", ref)
		}
	}
}

func TestSVGGradientStopBudgetInvalidAndAtomic(t *testing.T) {
	stops := make([]gomath.GradientStop, gomath.MaxGradientStops)
	for i := range stops {
		stops[i] = gomath.GradientStop{Offset: float64(i) / float64(len(stops)-1), Color: gomath.White}
	}
	g, e := gomath.NewRadialGradient(geom.V(0, 0), 1, stops)
	if e != nil {
		t.Fatal(e)
	}
	frame := &gomath.Frame{}
	for i := 0; i <= MaxSVGGradientStops/gomath.MaxGradientStops; i++ {
		q, e := g.Transform(geom.Translation(geom.V(float64(i), 0)))
		if e != nil {
			t.Fatal(e)
		}
		frame.Objects = append(frame.Objects, gomath.Square(1).SetFillGradient(q))
	}
	file := filepath.Join(t.TempDir(), "budget.svg")
	if e := SVG(file, frame, exportGradientConfig()); e == nil {
		t.Fatal("stop budget bypass")
	}
	if _, e := os.Stat(file); !os.IsNotExist(e) {
		t.Fatal("failed SVG published output")
	}
	frame.Objects = frame.Objects[:len(frame.Objects)-1]
	if e := SVG(file, frame, exportGradientConfig()); e != nil {
		t.Fatal("exact stop boundary rejected", e)
	}
	invalid := gomath.Square(1).SetFillGradient(&gomath.Gradient{})
	file = filepath.Join(t.TempDir(), "invalid.svg")
	if e := SVG(file, &gomath.Frame{Objects: []*gomath.Object{invalid}}, exportGradientConfig()); e == nil {
		t.Fatal("invalid paint exported")
	}
	sentinel := errors.New("gradient writer failed")
	if e := writeSVG(gradientFailWriter{sentinel}, exportGradientFrame(t, "linear-alpha"), exportGradientConfig()); !errors.Is(e, sentinel) {
		t.Fatal("writer error lost", e)
	}
}

type gradientFailWriter struct{ err error }

func (w gradientFailWriter) Write([]byte) (int, error) { return 0, w.err }

func TestSVGNilGradientUsesLegacySolidBytes(t *testing.T) {
	o := gomath.Square(2).SetFill(gomath.Blue)
	// Rendering the same scene after explicitly clearing the optional paint must
	// preserve the complete pre-extension serializer branch, including clip defs.
	clip, e := gomath.NewClip(gomath.Circle(.8).Path)
	if e != nil {
		t.Fatal(e)
	}
	o.SetClip(clip)
	var before, after bytes.Buffer
	f := &gomath.Frame{Objects: []*gomath.Object{o}}
	c := exportGradientConfig()
	if e := writeSVG(&before, f, c); e != nil {
		t.Fatal(e)
	}
	o.SetFillGradient(exportGradient(t, false, []gomath.GradientStop{{Offset: 0, Color: gomath.White}, {Offset: 1, Color: gomath.Black}})).SetFill(gomath.Blue)
	if e := writeSVG(&after, f, c); e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(before.Bytes(), after.Bytes()) || strings.Contains(after.String(), "Gradient") || strings.Contains(after.String(), "gomath-gradient-") {
		t.Fatal("nil gradient changed old solid output")
	}
}
