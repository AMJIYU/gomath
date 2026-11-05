package svg_test

import (
	"bytes"
	"gomath"
	"gomath/geom"
	"gomath/render"
	"gomath/svg"
	"image/color"
	"image/png"
	"math"
	"os"
	"strings"
	"testing"
)

func TestSVGClipIndependentCairoReferences(t *testing.T) {
	for _, name := range []string{"transform", "nested", "bitmap", "use", "use-positions"} {
		t.Run(name, func(t *testing.T) {
			prefix := "../tests/data/svg/clip-import-" + name
			source, err := os.ReadFile(prefix + ".svg")
			if err != nil {
				t.Fatal(err)
			}
			root, err := svg.Read(strings.NewReader(string(source)), svg.Options{Width: 8})
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(prefix + ".png")
			if err != nil {
				t.Fatal(err)
			}
			want, err := png.Decode(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			config := render.DefaultConfig()
			config.Width, config.Height, config.Workers, config.FrameWidth, config.Background = 128, 96, 1, 8, gomath.Transparent
			raster, err := render.NewRasterizer(config)
			if err != nil {
				t.Fatal(err)
			}
			got, err := raster.Render(&gomath.Frame{Objects: []*gomath.Object{root}})
			if err != nil {
				t.Fatal(err)
			}
			if got.Bounds() != want.Bounds() {
				t.Fatal("reference dimensions differ")
			}
			total, maximum, changed := 0, 0, 0
			for y := range 96 {
				for x := range 128 {
					a, b := got.RGBAAt(x, y), color.RGBAModel.Convert(want.At(x, y)).(color.RGBA)
					pixelChanged := false
					for _, pair := range [][2]uint8{{a.R, b.R}, {a.G, b.G}, {a.B, b.B}, {a.A, b.A}} {
						d := int(math.Abs(float64(int(pair[0]) - int(pair[1]))))
						total += d
						maximum = max(maximum, d)
						pixelChanged = pixelChanged || d > 8
					}
					if pixelChanged {
						changed++
					}
				}
			}
			mean, ratio := float64(total)/(128*96*4), float64(changed)/(128*96)
			t.Logf("CairoSVG clip %s 128x96 premultiplied RGBA: MAE=%.9g max=%d fraction>8=%.9g", name, mean, maximum, ratio)
			if mean > .35 || maximum > 45 || ratio > .02 {
				t.Fatal("clip reference tolerance exceeded")
			}
			// Transform the imported geometry and clips, then independently invert
			// that rigid transform via camera projection. Source remains immutable.
			original := root.Copy()
			root.ApplyMatrix(geom.Translation(geom.V(10, 20)).Mul(geom.RotationZ(math.Pi / 6)))
			camera := gomath.Camera2D{Center: geom.V(10, 20), Rotation: math.Pi / 6, FrameWidth: 8}
			moved, err := raster.Render(&gomath.Frame{Objects: []*gomath.Object{root}, Camera: &camera})
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got.Pix, moved.Pix) {
				t.Fatal("camera transform changed imported clipping pixels")
			}
			again, err := raster.Render(&gomath.Frame{Objects: []*gomath.Object{original}})
			if err != nil || !bytes.Equal(got.Pix, again.Pix) {
				t.Fatal("mutating an imported copy changed its sibling snapshot", err)
			}
		})
	}
}
