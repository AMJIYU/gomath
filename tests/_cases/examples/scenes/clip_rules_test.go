package scenes

import (
	"gomath"
	"gomath/path"
	"gomath/render"
	"image/color"
	"testing"
)

func TestClipRulesDemoKeepsHoleThroughMorphAndCamera(t *testing.T) {
	s, err := ClipRules()
	if err != nil {
		t.Fatal(err)
	}
	c := render.DefaultConfig()
	c.Width, c.Height, c.Workers, c.Supersampling = 320, 180, 1, 4
	raster, err := render.NewRasterizer(c)
	if err != nil {
		t.Fatal(err)
	}
	background := c.Background.NRGBA(1)
	bg := color.RGBA{R: background.R, G: background.G, B: background.B, A: background.A}
	for _, time := range []float64{0, .5, 1.25, 2.8, 4} {
		frame, err := s.Sample(time)
		if err != nil {
			t.Fatal(err)
		}
		pixels, err := raster.Render(frame)
		if err != nil {
			t.Fatal(err)
		}
		projected, effective, err := render.ProjectFrame(frame, c)
		if err != nil {
			t.Fatal(err)
		}
		found := 0
		for _, root := range projected.Objects {
			root.Walk(func(o *gomath.Object) {
				if o.Name != "Nonzero clipped bands" && o.Name != "Evenodd clipped bands" {
					return
				}
				found++
				if o.Clip == nil {
					t.Error("missing clip", time, o.Name)
					return
				}
				even := o.Name == "Evenodd clipped bands"
				if (o.Clip.FillRule() == path.EvenOdd) != even {
					t.Error("projected rule reset", time, o.Name, o.Clip.FillRule())
				}
				center := o.Clip.Bounds().Center()
				scale := float64(c.Width) / effective.FrameWidth
				x, y := int(float64(c.Width)/2+center.X*scale), int(float64(c.Height)/2-center.Y*scale)
				pixel := pixels.RGBAAt(x, y)
				if (pixel == bg) != even {
					t.Error("wrong compound center coverage", time, o.Name, pixel, bg)
				}
			})
		}
		if found != 2 {
			t.Fatal("demo coverage targets missing", time, found)
		}
	}
}
