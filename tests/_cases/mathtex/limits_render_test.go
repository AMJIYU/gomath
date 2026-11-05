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

func TestLimitsRenderDistinctUpperAndLowerInk(t *testing.T) {
	for _, command := range []string{"sum", "prod", "lim", "int"} {
		for _, prefix := range []string{"", `\displaystyle`} {
			t.Run(prefix+command, func(t *testing.T) {
				opt := mathtex.DefaultOptions()
				opt.Color = gomath.Blue
				o, err := mathtex.New(prefix+`\`+command+`\limits_{g}^{n}`, opt)
				if err != nil {
					t.Fatal(err)
				}
				o.MoveTo(geom.V(0, 0))
				operator := o.Children[0]
				body := operator.Children[0].Bounds()
				c := render.DefaultConfig()
				c.Width, c.Height, c.Workers, c.Background = 480, 480, 1, gomath.Black
				bounds := o.Bounds()
				c.FrameWidth = math.Max(2, math.Max(bounds.Size().X, bounds.Size().Y)*1.5)
				r, err := render.NewRasterizer(c)
				if err != nil {
					t.Fatal(err)
				}
				full, err := r.Render(&gomath.Frame{Objects: []*gomath.Object{o}})
				if err != nil {
					t.Fatal(err)
				}
				for childIndex := 1; childIndex <= 2; childIndex++ {
					limit := operator.Children[childIndex].Bounds()
					without := o.Copy()
					without.Children[0].Children[childIndex].Walk(func(n *gomath.Object) { n.Path = path.Path{} })
					base, err := r.Render(&gomath.Frame{Objects: []*gomath.Object{without}})
					if err != nil {
						t.Fatal(err)
					}
					pixels := 0
					scale := float64(c.Width) / c.FrameWidth
					margin := 2 / scale
					for y := range c.Height {
						for x := range c.Width {
							pixel := full.RGBAAt(x, y)
							if (pixel.R != 0 || pixel.G != 0 || pixel.B != 0) && (x < 2 || y < 2 || x >= c.Width-2 || y >= c.Height-2) {
								t.Fatal("limits clipped by viewport")
							}
							if pixel == base.RGBAAt(x, y) {
								continue
							}
							pixels++
							wx, wy := (float64(x)+.5-float64(c.Width)/2)/scale, (float64(c.Height)/2-float64(y)-.5)/scale
							if wx < limit.Min.X-margin || wx > limit.Max.X+margin || wy < limit.Min.Y-margin || wy > limit.Max.Y+margin {
								t.Fatal("rendered limit escapes bbox")
							}
							if childIndex == 1 && wy <= body.Max.Y || childIndex == 2 && wy >= body.Min.Y {
								t.Fatal("limit pixels overlap the operator body")
							}
						}
					}
					if pixels < 20 {
						t.Fatal("limit did not render visible glyphs", pixels)
					}
				}
			})
		}
	}
}
