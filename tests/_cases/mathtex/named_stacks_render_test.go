package mathtex_test

import (
	"gomath"
	"gomath/geom"
	"gomath/mathtex"
	"gomath/path"
	"gomath/render"
	"gomath/text"
	"math"
	"testing"
)

func TestNamedOperatorAndStacksRenderActualAnnotations(t *testing.T) {
	for _, source := range []string{
		`\operatorname{rank}\limits_g^n`, `\displaystyle\operatorname*{arg max}_g^n`,
		`\overset{n}{=}`, `\underset{g}{=}`, `\stackrel{\operatorname{def}}{=}`, `\overset{n}{\frac{}{}}`,
	} {
		t.Run(source, func(t *testing.T) {
			opt := mathtex.DefaultOptions()
			opt.Style, opt.Color = text.Italic, gomath.Blue // names stay upright
			o, err := mathtex.New(source, opt)
			if err != nil {
				t.Fatal(err)
			}
			o.MoveTo(geom.V(0, 0))
			stack := o.Children[0]
			body := stack.Children[0].Bounds()
			bounds := o.Bounds()
			c := render.DefaultConfig()
			c.Width, c.Height, c.Workers, c.Background = 640, 480, 1, gomath.Black
			c.FrameWidth = math.Max(2.8, math.Max(bounds.Size().X*1.5, bounds.Size().Y*float64(c.Width)/float64(c.Height)*1.5))
			r, err := render.NewRasterizer(c)
			if err != nil {
				t.Fatal(err)
			}
			full, err := r.Render(&gomath.Frame{Objects: []*gomath.Object{o}})
			if err != nil {
				t.Fatal(err)
			}
			for i, annotation := range stack.Children[1:] {
				label := annotation.Bounds()
				without := o.Copy()
				without.Children[0].Children[i+1].Walk(func(n *gomath.Object) { n.Path = path.Path{} })
				base, err := r.Render(&gomath.Frame{Objects: []*gomath.Object{without}})
				if err != nil {
					t.Fatal(err)
				}
				scale := float64(c.Width) / c.FrameWidth
				margin, pixels := 2/scale, 0
				for y := range c.Height {
					for x := range c.Width {
						pixel := full.RGBAAt(x, y)
						if (pixel.R != 0 || pixel.G != 0 || pixel.B != 0) && (x < 2 || y < 2 || x >= c.Width-2 || y >= c.Height-2) {
							t.Fatal("named/stack formula clipped")
						}
						if pixel == base.RGBAAt(x, y) {
							continue
						}
						pixels++
						wx, wy := (float64(x)+.5-float64(c.Width)/2)/scale, (float64(c.Height)/2-float64(y)-.5)/scale
						if wx < label.Min.X-margin || wx > label.Max.X+margin || wy < label.Min.Y-margin || wy > label.Max.Y+margin {
							t.Fatal("annotation pixels escaped the vector bbox")
						}
						if annotation.Name == "UpperLimit" && wy <= body.Max.Y || annotation.Name == "LowerLimit" && wy >= body.Min.Y {
							t.Fatal("annotation pixels overlap body")
						}
					}
				}
				if pixels < 20 {
					t.Fatal("annotation did not render actual ink", pixels)
				}
			}
		})
	}
}
