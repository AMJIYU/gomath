package mathtex_test

import (
	"gomath"
	"gomath/geom"
	"gomath/mathtex"
	"gomath/path"
	"gomath/render"
	"math"
	"testing"
)

func TestAccentMarksRenderVisibleBoundedInk(t *testing.T) {
	for _, command := range []string{"overline", "underline", "hat", "widehat", "bar", "vec", "dot", "ddot"} {
		t.Run(command, func(t *testing.T) {
			opt := mathtex.DefaultOptions()
			opt.Size, opt.Color = .8, gomath.Blue
			object, err := mathtex.New(`\`+command+`{x}`, opt)
			if err != nil {
				t.Fatal(err)
			}
			object.MoveTo(geom.V(0, 0))
			bounds := object.Bounds()
			mark := object.Children[0].Children[1].Bounds()
			without := object.Copy()
			without.Children[0].Children[1].Walk(func(n *gomath.Object) { n.Path = path.Path{} })
			c := render.DefaultConfig()
			c.Width, c.Height, c.Workers, c.Background = 640, 384, 1, gomath.Black
			c.FrameWidth = math.Max(2.5, math.Max(bounds.Size().X*1.5, bounds.Size().Y*float64(c.Width)/float64(c.Height)*1.5))
			r, err := render.NewRasterizer(c)
			if err != nil {
				t.Fatal(err)
			}
			full, err := r.Render(&gomath.Frame{Objects: []*gomath.Object{object}})
			if err != nil {
				t.Fatal(err)
			}
			base, err := r.Render(&gomath.Frame{Objects: []*gomath.Object{without}})
			if err != nil {
				t.Fatal(err)
			}
			changed := 0
			scale := float64(c.Width) / c.FrameWidth
			margin := 2 / scale
			for y := range c.Height {
				for x := range c.Width {
					pixel := full.RGBAAt(x, y)
					if (pixel.R != 0 || pixel.G != 0 || pixel.B != 0) && (x < 2 || y < 2 || x >= c.Width-2 || y >= c.Height-2) {
						t.Fatal("accent formula clipped by viewport")
					}
					if pixel == base.RGBAAt(x, y) {
						continue
					}
					changed++
					wx, wy := (float64(x)+.5-float64(c.Width)/2)/scale, (float64(c.Height)/2-float64(y)-.5)/scale
					if wx < mark.Min.X-margin || wx > mark.Max.X+margin || wy < mark.Min.Y-margin || wy > mark.Max.Y+margin {
						t.Fatal("rendered mark escapes its geometric bbox", wx, wy, mark)
					}
				}
			}
			if changed < 20 {
				t.Fatal("accent command did not add visible mark pixels", changed)
			}
		})
	}
}
