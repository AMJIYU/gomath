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

func fractionPart(object *gomath.Object, name string) *gomath.Object {
	var found *gomath.Object
	object.Walk(func(n *gomath.Object) {
		if n.Name == name {
			found = n
		}
	})
	return found
}

func TestGeneralFractionRenderConstituentsAndBounds(t *testing.T) {
	for _, test := range []struct {
		source string
		parts  []string
	}{
		{`\dfrac{g}{n}`, []string{"FractionNumerator", "FractionDenominator", "FractionRule"}},
		{`\tfrac{g}{n}`, []string{"FractionNumerator", "FractionDenominator", "FractionRule"}},
		{`\dbinom{g}{n}`, []string{"FractionNumerator", "FractionDenominator", "Delimiter (", "Delimiter )"}},
		{`\tbinom{g}{n}`, []string{"Delimiter (", "Delimiter )"}},
		{`\sum\limits_{\substack{i=1\\j=2}}^n`, []string{"SubstackRow[0]", "SubstackRow[1]"}},
	} {
		t.Run(test.source, func(t *testing.T) {
			o, err := mathtex.New(test.source, mathtex.DefaultOptions())
			if err != nil {
				t.Fatal(err)
			}
			o.MoveTo(geom.V(0, 0))
			bounds := o.Bounds()
			c := render.DefaultConfig()
			c.Width, c.Height, c.Workers, c.Background = 480, 480, 1, gomath.Black
			c.FrameWidth = math.Max(2, math.Max(bounds.Size().X, bounds.Size().Y)*1.5)
			r, err := render.NewRasterizer(c)
			if err != nil {
				t.Fatal(err)
			}
			full, err := r.Render(&gomath.Frame{Objects: []*gomath.Object{o}})
			if err != nil {
				t.Fatal(err)
			}
			for _, name := range test.parts {
				part := fractionPart(o, name)
				if part == nil {
					t.Fatal("missing render constituent", name)
				}
				ink := part.Bounds()
				without := o.Copy()
				fractionPart(without, name).Walk(func(n *gomath.Object) { n.Path = path.Path{} })
				base, err := r.Render(&gomath.Frame{Objects: []*gomath.Object{without}})
				if err != nil {
					t.Fatal(err)
				}
				scale := float64(c.Width) / c.FrameWidth
				margin := part.Style.StrokeWidth/2 + 2/scale
				pixels := 0
				for y := range c.Height {
					for x := range c.Width {
						pixel := full.RGBAAt(x, y)
						if pixel != base.RGBAAt(x, y) {
							pixels++
							wx, wy := (float64(x)+.5-float64(c.Width)/2)/scale, (float64(c.Height)/2-float64(y)-.5)/scale
							if wx < ink.Min.X-margin || wx > ink.Max.X+margin || wy < ink.Min.Y-margin || wy > ink.Max.Y+margin {
								t.Fatal("rendered constituent escapes bounds", name)
							}
						}
						if pixel.R > 0 && (x < 2 || x >= c.Width-2 || y < 2 || y >= c.Height-2) {
							t.Fatal("fraction clipped by viewport")
						}
					}
				}
				if pixels < 20 {
					t.Fatal("constituent generated no visible pixels", name, pixels)
				}
			}
		})
	}
}
