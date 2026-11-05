package gomath_test

import (
	"gomath"
	"gomath/geom"
	"gomath/render"
	"math"
	"testing"
)

func TestAnnularFillHoleAndClockwiseSector(t *testing.T) {
	for _, sweep := range []float64{2 * math.Pi, -2 * math.Pi, math.Pi, -math.Pi} {
		o, err := gomath.AnnularSector(.5, 1, 0, sweep)
		must(t, err)
		must(t, o.Validate())
		o.SetFill(gomath.White).SetStroke(gomath.Transparent, 0)
		s := gomath.NewScene()
		must(t, s.Add(o))
		f, err := s.Sample(0)
		must(t, err)
		c := render.DefaultConfig()
		c.Width, c.Height, c.FrameWidth, c.Background = 128, 128, 4, gomath.Black
		r, err := render.NewRasterizer(c)
		must(t, err)
		im, err := r.Render(f)
		must(t, err)
		if im.RGBAAt(64, 64).R != 0 || im.RGBAAt(64, 15).R != 0 {
			t.Fatal("hole or exterior filled", sweep)
		}
		y := 45
		if sweep < 0 && math.Abs(sweep) < 2*math.Pi {
			y = 83
		}
		if im.RGBAAt(76, y).R != 255 {
			t.Fatal("expected ring ink missing", sweep)
		}
		if math.Abs(sweep) == 2*math.Pi && len(o.Path.Contours) != 2 {
			t.Fatal("full ring needs separate contours")
		}
	}
}

func TestPrimitiveBoundsAndDashLayout(t *testing.T) {
	e, err := gomath.Ellipse(4, 2)
	must(t, err)
	near(t, e.Bounds().Size().X, 4)
	near(t, e.Bounds().Size().Y, 2)
	for _, radius := range []float64{0, .25, 1} {
		r, err := gomath.RoundedRectangle(4, 2, radius)
		must(t, err)
		must(t, r.Validate())
		near(t, r.Bounds().Size().X, 4)
		near(t, r.Bounds().Size().Y, 2)
	}
	d, err := gomath.DashedLine(geom.V(0, 0), geom.V(2.3, 0), .4, .5)
	must(t, err)
	if len(d.Children) != 3 {
		t.Fatal("dash count", len(d.Children))
	}
	for i, child := range d.Children {
		near(t, child.Bounds().Min.X, .8*float64(i))
		near(t, child.Bounds().Max.X, .8*float64(i)+.4)
		if child.Style.StrokeCap != gomath.CapButt {
			t.Fatal("dash cap extends into gap")
		}
	}
	double, err := gomath.DoubleArrow(geom.V(-2, 1), geom.V(2, 1), .2)
	must(t, err)
	must(t, double.Validate())
	near(t, double.Bounds().Min.X, -2)
	near(t, double.Bounds().Max.X, 2)
	shaft := double.Children[0]
	s := gomath.NewScene()
	must(t, s.Add(double))
	must(t, s.Play(1, gomath.Shift(double, geom.V(1, 0))))
	if shaft != double.Children[0] {
		t.Fatal("primitive transform replaced child reference")
	}
}

func TestPrimitiveValidationBeforeAllocation(t *testing.T) {
	bad := []func() (*gomath.Object, error){
		func() (*gomath.Object, error) { return gomath.Ellipse(0, 1) },
		func() (*gomath.Object, error) { return gomath.Ellipse(1, math.NaN()) },
		func() (*gomath.Object, error) { return gomath.Annulus(2, 1) },
		func() (*gomath.Object, error) { return gomath.Annulus(-1, 1) },
		func() (*gomath.Object, error) { return gomath.Sector(1, 0, 0) },
		func() (*gomath.Object, error) { return gomath.Sector(1, 0, 4*math.Pi) },
		func() (*gomath.Object, error) { return gomath.Sector(1, math.Inf(1), 1) },
		func() (*gomath.Object, error) { return gomath.RoundedRectangle(2, 1, .6) },
		func() (*gomath.Object, error) { return gomath.RoundedRectangle(2, 1, -.1) },
		func() (*gomath.Object, error) { return gomath.DashedLine(geom.V(0, 0), geom.V(1, 0), 1e-9, 1) },
		func() (*gomath.Object, error) { return gomath.DashedLine(geom.V(0, 0), geom.V(1, 0), .1, 0) },
		func() (*gomath.Object, error) { return gomath.DashedLine(geom.V(0, 0), geom.V(0, 0), .1, .5) },
		func() (*gomath.Object, error) { return gomath.DoubleArrow(geom.V(0, 0), geom.V(1, 0), .5) },
		func() (*gomath.Object, error) { return gomath.DoubleArrow(geom.V(0, 0), geom.Vec3{X: 1, Z: 1}, .1) },
	}
	for i, f := range bad {
		if o, err := f(); err == nil || o != nil {
			t.Fatal("invalid primitive accepted", i)
		}
	}
}
