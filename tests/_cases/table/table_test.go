package table_test

import (
	"gomath"
	"gomath/geom"
	"gomath/render"
	"gomath/table"
	"math"
	"testing"
)

func TestRulesAndHighlightPixels(t *testing.T) {
	tab, err := table.New([][]string{{"x", ""}, {"", "y"}}, table.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(tab.Lines().Children) != 6 {
		t.Fatal("rules missing")
	}
	if err := tab.HighlightCell(0, 1, gomath.Color{R: .2, G: .6, B: .8, A: .5}); err != nil {
		t.Fatal(err)
	}
	background, _ := tab.CellBackground(0, 1)
	center := background.Center()
	c := render.DefaultConfig()
	c.Width, c.Height, c.FrameWidth, c.Background = 256, 256, 8, gomath.Black
	r, err := render.NewRasterizer(c)
	if err != nil {
		t.Fatal(err)
	}
	img, err := r.Render(&gomath.Frame{Objects: []*gomath.Object{tab.Object}})
	if err != nil {
		t.Fatal(err)
	}
	pixel := img.RGBAAt(int(128+center.X*32), int(128-center.Y*32))
	if pixel.R < 24 || pixel.R > 27 || pixel.G < 75 || pixel.G > 78 || pixel.B < 100 || pixel.B > 104 {
		t.Fatal("cell background alpha wrong", pixel)
	}
	tab.Shift(geom.V(2, -1))
	if background.Center().Sub(center.Add(geom.V(2, -1))).Norm() > 1e-10 {
		t.Fatal("background did not follow table transform")
	}
	if err := tab.HighlightCell(0, 1, gomath.Color{A: 2}); err == nil || background.Style.Fill.A != .5 {
		t.Fatal("invalid highlight mutated cell")
	}
	if err := tab.HighlightCell(2, 0, gomath.Blue); err == nil {
		t.Fatal("invalid cell accepted")
	}
}

func TestRuleOptionsAndObjectOwnership(t *testing.T) {
	for _, inner := range []bool{true, false} {
		for _, outer := range []bool{true, false} {
			opt := table.DefaultOptions()
			opt.InnerLines, opt.OuterLines = inner, outer
			source := gomath.Circle(.3).MoveTo(geom.V(7, 8))
			tab, err := table.NewObjects([][]*gomath.Object{{source, source}, {source, source}}, opt)
			if err != nil {
				t.Fatal(err)
			}
			count := 0
			if inner {
				count += 2
			}
			if outer {
				count += 4
			}
			if len(tab.Lines().Children) != count {
				t.Fatal("incorrect rules", inner, outer)
			}
			if source.Center().Sub(geom.V(7, 8)).Norm() > 1e-10 {
				t.Fatal("source repositioned")
			}
			a, _ := tab.Entry(0, 0)
			b, _ := tab.Entry(0, 1)
			a.SetFill(gomath.Blue)
			if a == b || b.Style.Fill.A != 0 || source.Style.Fill.A != 0 {
				t.Fatal("shared cells")
			}
			scene := gomath.NewScene()
			if err := scene.Add(tab.Object); err != nil {
				t.Fatal(err)
			}
			if err := scene.Play(1, gomath.Rotate(tab.Object, math.Pi)); err != nil {
				t.Fatal(err)
			}
			if now, _ := tab.Entry(0, 0); now != a {
				t.Fatal("transform discarded selection")
			}
			if _, err := scene.Sample(.5); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestInvalidTableInputs(t *testing.T) {
	for _, input := range [][][]string{nil, {{}}, {{"1"}, {"2", "3"}}} {
		if _, err := table.New(input, table.DefaultOptions()); err == nil {
			t.Fatal("invalid grid accepted")
		}
	}
	opt := table.DefaultOptions()
	opt.PaddingX = math.NaN()
	if _, err := table.New([][]string{{"1"}}, opt); err == nil {
		t.Fatal("invalid spacing accepted")
	}
	opt = table.DefaultOptions()
	opt.LineWidth = 0
	if _, err := table.New([][]string{{"1"}}, opt); err == nil {
		t.Fatal("invisible rules accepted")
	}
}
