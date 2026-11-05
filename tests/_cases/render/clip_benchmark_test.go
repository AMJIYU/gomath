package render_test

import (
	"gomath"
	"gomath/geom"
	"gomath/render"
	"image"
	"image/color"
	"testing"
)

func BenchmarkClippedRaster(b *testing.B) {
	c := render.DefaultConfig()
	c.Width, c.Height, c.Workers = 640, 360, 1
	pixels := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := range 64 {
		for x := range 64 {
			pixels.SetRGBA(x, y, color.RGBA{R: uint8(4 * x), G: uint8(4 * y), B: 128, A: 255})
		}
	}
	asset, err := gomath.NewBitmap(pixels)
	if err != nil {
		b.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		bitmap bool
		depth  int
	}{{"vector-baseline", false, 0}, {"vector-one-clip", false, 1}, {"vector-four-clips", false, 4}, {"bitmap-one-clip", true, 1}} {
		b.Run(tc.name, func(b *testing.B) {
			o := gomath.Rectangle(8, 5).SetFill(gomath.Blue).SetStroke(gomath.Transparent, 0)
			if tc.bitmap {
				o, err = gomath.NewImage(asset, 8)
				if err != nil {
					b.Fatal(err)
				}
			}
			for i := range tc.depth {
				clip, err := gomath.NewClip(gomath.Circle(2.2).Shift(geom.V(.1*float64(i), 0)).Path)
				if err != nil {
					b.Fatal(err)
				}
				o = gomath.Group(o).SetClip(clip)
			}
			frame := &gomath.Frame{Objects: []*gomath.Object{o}}
			r, err := render.NewRasterizer(c)
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if _, err = r.Render(frame); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
