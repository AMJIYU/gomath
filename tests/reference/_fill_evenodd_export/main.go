// Development-only fixture producer. The underscore directory is intentionally
// excluded from Go package discovery; no runtime subprocess/reference backend.
package main

import (
	"encoding/json"
	"gomath"
	"gomath/export"
	"gomath/geom"
	"gomath/path"
	"gomath/render"
	"image/png"
	"os"
	"path/filepath"
)

func check(err error) {
	if err != nil {
		panic(err)
	}
}
func main() {
	dir := "tests/data/export/fill-evenodd"
	check(os.MkdirAll(dir, 0755))
	p := gomath.Circle(25).Path
	p.Contours = append(p.Contours, gomath.Circle(10).Path.Contours...)
	left := gomath.NewObject(p).SetFill(gomath.RGB(48.0/255, 160.0/255, 224.0/255)).SetStroke(gomath.Transparent, 0).SetFillRule(path.EvenOdd).SetOpacity(.7)
	left.Style.Reveal = .72
	left.Shift(geom.V(-31, 0))
	g, e := gomath.NewLinearGradient(geom.V(-25, 0), geom.V(25, 0), []gomath.GradientStop{{Offset: 0, Color: gomath.Color{R: 1, A: .7}}, {Offset: 1, Color: gomath.Color{B: 1, A: .7}}})
	check(e)
	right := gomath.NewObject(p).SetFillGradient(g).SetStroke(gomath.Transparent, 0).SetFillRule(path.EvenOdd).Shift(geom.V(31, 0))
	clip, e := gomath.NewClip(gomath.Rectangle(120, 44).Path)
	check(e)
	f := &gomath.Frame{Objects: []*gomath.Object{gomath.Group(left, right).SetClip(clip)}}
	c := render.DefaultConfig()
	c.Width, c.Height, c.FrameWidth, c.Workers, c.Background = 128, 96, 128, 1, gomath.Transparent
	target := filepath.Join(dir, "actual.svg")
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		check(err)
	}
	check(export.SVG(target, f, c))
	r, e := render.NewRasterizer(c)
	check(e)
	im, e := r.Render(f)
	check(e)
	file, e := os.Create(filepath.Join(dir, "native.png"))
	check(e)
	check(png.Encode(file, im))
	check(file.Close())
	data, e := json.MarshalIndent(map[string]any{"width": 128, "height": 96, "frame_width": 128, "radius": 25, "inner_radius": 10, "left_reveal": .72, "left_opacity": .7, "fill_rule": "evenodd", "clip_rule": "nonzero", "gradient_alpha": .7}, "", "  ")
	check(e)
	check(os.WriteFile(filepath.Join(dir, "input.json"), append(data, '\n'), 0644))
}
