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

func TestMultilineRowsRenderVisibleAndBounded(t *testing.T) {
	for _, tc := range []struct {
		source string
		cells  []string
	}{
		{`\begin{aligned}\color{red}a&=\dfrac{n}{g}\\\mathbb{R}&=\hat{n}_i\end{aligned}`, []string{"aligned cell[0,0]", "aligned cell[0,1]", "aligned cell[1,0]", "aligned cell[1,1]"}},
		{`\begin{gathered}\color{blue}\hat{n}\\\sum_{\substack{i=1\\j=2}}^n\dfrac{n}{g}\\\mathbb{R}\end{gathered}`, []string{"gathered cell[0,0]", "gathered cell[1,0]", "gathered cell[2,0]"}},
	} {
		o, err := mathtex.New(tc.source, mathtex.DefaultOptions())
		if err != nil {
			t.Fatal(err)
		}
		o.MoveTo(geom.V(0, 0))
		bounds := o.Bounds()
		c := render.DefaultConfig()
		c.Width, c.Height, c.Workers, c.Background = 480, 480, 1, gomath.Black
		c.FrameWidth = math.Max(2, math.Max(bounds.Size().X, bounds.Size().Y)*1.3)
		r, err := render.NewRasterizer(c)
		if err != nil {
			t.Fatal(err)
		}
		full, err := r.Render(&gomath.Frame{Objects: []*gomath.Object{o}})
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range tc.cells {
			cell := fractionPart(o, name)
			if cell == nil {
				t.Fatal("missing cell", name)
			}
			ink, stroke := cell.Bounds(), 0.0
			cell.Walk(func(n *gomath.Object) { stroke = math.Max(stroke, n.Style.StrokeWidth) })
			without := o.Copy()
			fractionPart(without, name).Walk(func(n *gomath.Object) { n.Path = path.Path{} })
			base, err := r.Render(&gomath.Frame{Objects: []*gomath.Object{without}})
			if err != nil {
				t.Fatal(err)
			}
			scale, changed := float64(c.Width)/c.FrameWidth, 0
			margin := stroke/2 + 2/scale
			for y := range c.Height {
				for x := range c.Width {
					pixel := full.RGBAAt(x, y)
					if pixel != base.RGBAAt(x, y) {
						changed++
						wx, wy := (float64(x)+.5-float64(c.Width)/2)/scale, (float64(c.Height)/2-float64(y)-.5)/scale
						if wx < ink.Min.X-margin || wx > ink.Max.X+margin || wy < ink.Min.Y-margin || wy > ink.Max.Y+margin {
							t.Fatal("multiline cell pixels escape bounds", name)
						}
					}
					if (pixel.R > 0 || pixel.G > 0 || pixel.B > 0) && (x < 2 || y < 2 || x >= c.Width-2 || y >= c.Height-2) {
						t.Fatal("multiline clipped by viewport")
					}
				}
			}
			if changed < 20 {
				t.Fatal("cell has no actual visible raster", name, changed)
			}
		}
	}
}
