package grid

import (
	"gomath"
	"gomath/geom"
	"math"
	"testing"
)

func TestAdaptiveGridSpacing(t *testing.T) {
	g, err := New([][]*gomath.Object{
		{gomath.Rectangle(1, 2), gomath.Rectangle(3, 1)},
		{gomath.Group(), gomath.Rectangle(1, .5)},
	}, Options{HorizontalGap: .4, VerticalGap: .2, MinCellWidth: .2, MinCellHeight: .3})
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(g.Width-4.8) > 1e-10 || math.Abs(g.Height-2.9) > 1e-10 {
		t.Fatal(g.Width, g.Height)
	}
	a, _ := g.Entry(0, 1)
	b, _ := g.Entry(1, 1)
	if math.Abs(a.Center().X-b.Center().X) > 1e-10 {
		t.Fatal("column centers differ")
	}
	if math.Abs(g.XEdges[0]+g.XEdges[len(g.XEdges)-1]) > 1e-10 || math.Abs(g.YEdges[0]+g.YEdges[len(g.YEdges)-1]) > 1e-10 {
		t.Fatal("layout is not centered")
	}
}

func TestInvalidCellsRejectBeforeCopy(t *testing.T) {
	cycle := gomath.Group()
	cycle.Children = []*gomath.Object{cycle}
	for _, object := range []*gomath.Object{nil, cycle, gomath.Square(1).Shift(geom.V(math.NaN(), 0))} {
		if _, err := New([][]*gomath.Object{{object}}, Options{}); err == nil {
			t.Fatal("invalid cell accepted")
		}
	}
	if _, err := New([][]*gomath.Object{{gomath.Group()}}, Options{}); err == nil {
		t.Fatal("zero-sized grid accepted")
	}
	if _, err := New([][]*gomath.Object{{gomath.Square(1)}}, Options{HorizontalGap: math.Inf(1)}); err == nil {
		t.Fatal("nonfinite spacing accepted")
	}
}
