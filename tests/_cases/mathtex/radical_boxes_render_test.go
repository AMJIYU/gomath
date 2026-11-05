package mathtex

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"gomath"
	"gomath/export"
	"gomath/geom"
	"gomath/path"
	"gomath/render"
	"image"
	"io"
	"math"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func rbRasterizer(t *testing.T) (*render.Rasterizer, render.Config) {
	t.Helper()
	c := render.DefaultConfig()
	c.Width, c.Height, c.Workers, c.FrameWidth, c.Background = 640, 480, 1, 6, gomath.Black
	r, err := render.NewRasterizer(c)
	if err != nil {
		t.Fatal(err)
	}
	return r, c
}
func rbInk(img *image.RGBA) int {
	n := 0
	for y := range img.Bounds().Dy() {
		for x := range img.Bounds().Dx() {
			p := img.RGBAAt(x, y)
			if p.R != 0 || p.G != 0 || p.B != 0 {
				n++
			}
		}
	}
	return n
}

func TestRadicalBoxRealInkAndPhantomRendering(t *testing.T) {
	r, c := rbRasterizer(t)
	for _, source := range []string{`\sqrt[\hat{n}_i]{\dfrac{g}{n}}`, `\boxed{\sqrt[n_i]{g}\phantom{W}A}_j^n`, `\boxed{\phantom{\dfrac{g}{n}}}`} {
		b := rbLayout(t, source)
		b.object.MoveTo(geom.V(0, 0))
		full, err := r.Render(&gomath.Frame{Objects: []*gomath.Object{b.object}})
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"BoxedRule", "RadicalRule", "RootIndex"} {
			part := rbNamed(b.object, name)
			if part == nil {
				continue
			}
			if !part.Bounds().Valid {
				t.Fatal("expected real ink", name)
			}
			without := b.object.Copy()
			rbNamed(without, name).Walk(func(n *gomath.Object) { n.Path = path.Path{} })
			base, err := r.Render(&gomath.Frame{Objects: []*gomath.Object{without}})
			if err != nil {
				t.Fatal(err)
			}
			changed := 0
			bounds, stroke := part.Bounds(), 0.0
			part.Walk(func(n *gomath.Object) { stroke = math.Max(stroke, n.Style.StrokeWidth) })
			scale := float64(c.Width) / c.FrameWidth
			margin := stroke/2 + 2/scale
			for y := range c.Height {
				for x := range c.Width {
					p := full.RGBAAt(x, y)
					if p != base.RGBAAt(x, y) {
						changed++
						wx, wy := (float64(x)+.5-float64(c.Width)/2)/scale, (float64(c.Height)/2-float64(y)-.5)/scale
						if wx < bounds.Min.X-margin || wx > bounds.Max.X+margin || wy < bounds.Min.Y-margin || wy > bounds.Max.Y+margin {
							t.Fatal("ink escaped constituent bounds", name)
						}
					}
					if (p.R != 0 || p.G != 0 || p.B != 0) && (x < 2 || y < 2 || x >= c.Width-2 || y >= c.Height-2) {
						t.Fatal("clipped new path")
					}
				}
			}
			if changed < 40 {
				t.Fatal("constituent not visibly rendered", name, changed)
			}
		}
	}
	for _, command := range []string{"phantom", "hphantom", "vphantom"} {
		o, err := New(`\`+command+`{\boxed{\sqrt[n_i]{\dfrac{g}{n}}}}`, DefaultOptions())
		if err != nil {
			t.Fatal(err)
		}
		img, err := r.Render(&gomath.Frame{Objects: []*gomath.Object{o}})
		if err != nil {
			t.Fatal(err)
		}
		if rbInk(img) != 0 {
			t.Fatal("phantom renders hidden ink", command)
		}
	}
}

func TestRadicalBoxWriteTransformSnapshotsAndPreview(t *testing.T) {
	r, c := rbRasterizer(t)
	dir := t.TempDir()
	if preview := os.Getenv("GOMATH_RADICAL_BOXES_PREVIEW_DIR"); preview != "" {
		dir = preview
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	source := rbLayout(t, `\color{green}\boxed{\sqrt[\textcolor{red}{n_i}]{\dfrac{g}{n}}\hphantom{W}A}_j`)
	target := rbLayout(t, `\color{blue}\boxed{\sqrt[\textcolor{red}{m_i}]{\dfrac{W}{g}}\hphantom{W}B}_j`)
	source.object.MoveTo(geom.V(0, 0))
	target.object.MoveTo(geom.V(0, 0))
	scene := gomath.NewScene()
	scene.Add(source.object)
	saved := source.object.Copy()
	targetBorder := rbNamed(target.object, "BoxedRule").Path.Clone()
	border := rbNamed(source.object, "BoxedRule")
	borderID := border.ID
	if err := scene.Play(1, gomath.Write(source.object)); err != nil {
		t.Fatal(err)
	}
	if err := scene.Play(1, gomath.Transform(source.object, target.object).WithRate(gomath.Linear)); err != nil {
		t.Fatal(err)
	}
	if rbNamed(source.object, "BoxedRule") != border || border.ID != borderID {
		t.Fatal("authored transform lost existing identity")
	}
	// Scene owns endpoint snapshots; modifying the caller target cannot leak.
	target.object.Shift(geom.V(100, 100)).SetOpacity(0)
	hashes := map[float64]string{}
	for _, time := range []float64{0, .25, .5, .75, 1, 1.25, 1.5, 1.75, 2} {
		frame, err := scene.Sample(time)
		if err != nil {
			t.Fatal(err)
		}
		if len(frame.Objects) != 1 || frame.Objects[0].ID != source.object.ID {
			t.Fatal("sample identity changed")
		}
		if err := frame.Objects[0].Validate(); err != nil {
			t.Fatal(err)
		}
		img, err := r.Render(frame)
		if err != nil {
			t.Fatal(time, err)
		}
		if time == 0 && rbInk(img) != 0 {
			t.Fatal("Write start visible")
		}
		if time > 0 && rbInk(img) < 80 {
			t.Fatal("animation intermediate has no ink", time)
		}
		if time > 0 && time < 1 {
			frame.Objects[0].Walk(func(n *gomath.Object) { rbNear(t, n.Style.Reveal, time) })
			// The invisible advance box is never materialized during Write.
			p := rbNamed(frame.Objects[0], "Phantom hphantom")
			if p == nil || p.Bounds().Valid {
				t.Fatal("Write phantom acquired ink")
			}
		}
		if time > 1 && time < 2 {
			p := rbNamed(frame.Objects[0], "BoxedRule")
			if p == nil || p.Style.StrokeWidth <= 0 {
				t.Fatal("Transform lost border")
			}
			from := rbNamed(saved, "BoxedRule").Path
			for ci, contour := range p.Path.Contours {
				for i, curve := range contour.Curves {
					want := from.Contours[ci].Curves[i].Lerp(targetBorder.Contours[ci].Curves[i], time-1)
					for j, point := range []geom.Vec3{curve.P0, curve.P1, curve.P2, curve.P3} {
						v := []geom.Vec3{want.P0, want.P1, want.P2, want.P3}[j]
						rbNear(t, point.X, v.X)
						rbNear(t, point.Y, v.Y)
					}
				}
			}
		}
		hashes[time] = rbHash(img.Pix)
		if time == .5 || time == 1 || time == 1.5 || time == 2 {
			name := map[float64]string{.5: "write-half", 1: "write-complete", 1.5: "transform-half", 2: "transform-complete"}[time]
			if err := export.PNG(filepath.Join(dir, name+".png"), img); err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(dir, name+".svg")
			if err := export.SVG(file, frame, c); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(data, []byte("<text")) || bytes.Contains(data, []byte("<image")) {
				t.Fatal("expected actual vector paths")
			}
			decoder := xml.NewDecoder(bytes.NewReader(data))
			paths := 0
			for {
				token, err := decoder.Token()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatal(err)
				}
				if start, ok := token.(xml.StartElement); ok && start.Name.Local == "path" {
					paths++
				}
			}
			if paths < 5 {
				t.Fatal("preview SVG has too few real paths")
			}
		}
		// A returned frame's paths and color are independently owned.
		frame.Objects[0].Shift(geom.V(20, 20)).SetFill(gomath.Red)
	}
	for _, pair := range [][2]float64{{.25, .5}, {.5, .75}, {.75, 1}, {1, 1.25}, {1.25, 1.5}, {1.5, 1.75}, {1.75, 2}} {
		if hashes[pair[0]] == hashes[pair[1]] {
			t.Fatal("animation pixels static", pair)
		}
	}
	// Concurrent reads return the same scene snapshots and pixels.
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rr, err := render.NewRasterizer(c)
			if err != nil {
				t.Error(err)
				return
			}
			for _, time := range []float64{.5, 1, 1.5, 2} {
				f, err := scene.Sample(time)
				if err != nil {
					t.Error(err)
					return
				}
				im, err := rr.Render(f)
				if err != nil {
					t.Error(err)
					return
				}
				if rbHash(im.Pix) != hashes[time] {
					t.Error("concurrent animation pixels changed")
				}
			}
		}()
	}
	wg.Wait()
	if !saved.Bounds().Valid || saved.Bounds().Center().Norm() > 1e-12 {
		t.Fatal("original copy was mutated")
	}
}

func TestRadicalBoxLegacyRootPixelsAndPaths(t *testing.T) {
	var refs struct {
		Records []struct{ Source, GeometrySHA256, PixelSHA256 string }
	}
	if err := json.Unmarshal(rbRead(t, "tests", "data", "mathtex", "radical_boxes", "legacy.json"), &refs); err != nil {
		t.Fatal(err)
	}
	if len(refs.Records) != 5 {
		t.Fatal("incomplete legacy root compatibility capture")
	}
	c := render.DefaultConfig()
	c.Width, c.Height, c.Workers, c.FrameWidth, c.Background = 640, 320, 1, 8, gomath.Black
	r, err := render.NewRasterizer(c)
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range refs.Records {
		o, err := New(ref.Source, DefaultOptions())
		if err != nil {
			t.Fatal(err)
		}
		type drawable struct {
			Path   path.Path
			Style  gomath.Style
			ZIndex int
		}
		d := []drawable{}
		o.Walk(func(n *gomath.Object) {
			if len(n.Path.Contours) > 0 {
				d = append(d, drawable{n.Path, n.Style, n.ZIndex})
			}
		})
		raw, err := json.Marshal(d)
		if err != nil {
			t.Fatal(err)
		}
		if rbHash(raw) != ref.GeometrySHA256 {
			t.Fatal("old unindexed radical geometry/style/order changed", ref.Source)
		}
		im, err := r.Render(&gomath.Frame{Objects: []*gomath.Object{o}})
		if err != nil {
			t.Fatal(err)
		}
		if rbHash(im.Pix) != ref.PixelSHA256 {
			t.Fatal("old unindexed radical pixel bytes changed", ref.Source)
		}
	}
}
