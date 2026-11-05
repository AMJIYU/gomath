// Development-only actual-export input for the independent Cairo XML oracle.
// Go's package walker ignores this underscored reference-tool directory.
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
	directory := "tests/data/export/clip-evenodd"
	check(os.MkdirAll(directory, 0755))
	outer := gomath.Circle(25).Path
	inner := gomath.Circle(10).Path
	outer.Contours = append(outer.Contours, inner.Contours...)
	even, err := gomath.NewClipWithRule(outer, path.EvenOdd)
	check(err)
	nonzero, err := gomath.NewClip(outer)
	check(err)
	left := gomath.Square(60).SetFill(gomath.RGB(48.0/255, 160.0/255, 224.0/255)).SetStroke(gomath.Transparent, 0).SetOpacity(.7).SetClip(even).Shift(geom.V(-31, 0))
	right := gomath.Square(60).SetFill(gomath.RGB(224.0/255, 80.0/255, 48.0/255)).SetStroke(gomath.Transparent, 0).SetClip(nonzero).Shift(geom.V(31, 0))
	mask, err := gomath.NewClip(gomath.Rectangle(120, 44).Path)
	check(err)
	root := gomath.Group(left, right).SetClip(mask)
	frame := &gomath.Frame{Objects: []*gomath.Object{root}}
	config := render.DefaultConfig()
	config.Width, config.Height, config.FrameWidth, config.Workers, config.Background = 128, 96, 128, 1, gomath.Transparent
	check(export.SVG(filepath.Join(directory, "actual.svg"), frame, config))
	r, err := render.NewRasterizer(config)
	check(err)
	pixels, err := r.Render(frame)
	check(err)
	output, err := os.Create(filepath.Join(directory, "native.png"))
	check(err)
	check(png.Encode(output, pixels))
	check(output.Close())
	metadata, err := json.MarshalIndent(map[string]any{"width": 128, "height": 96, "config": config, "scope": "Actual Go SVG export and native raster, independently interpreted by tests/reference/clip_evenodd_export.py; two same-winding compound circles, mixed clip rules, nonzero ancestor and alpha fill."}, "", "  ")
	check(err)
	check(os.WriteFile(filepath.Join(directory, "input.json"), append(metadata, '\n'), 0644))
}
