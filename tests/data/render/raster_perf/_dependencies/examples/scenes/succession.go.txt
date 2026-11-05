package scenes

import (
	"gomath"
	"gomath/geom"
	"gomath/mathtex"
	"math"
)

// Succession exercises delayed relative transforms and step-boundary cleanup.
func Succession() (*gomath.Scene, error) {
	s := gomath.NewScene()
	heading, err := title("Succession / shared objects / delayed begin state")
	if err != nil {
		return nil, err
	}
	formula, err := mathtex.New(`\displaystyle\sum_{k=0}^{n} k=\frac{n(n+1)}{2}`, mathtex.DefaultOptions())
	if err != nil {
		return nil, err
	}
	formula.Scale(.8).MoveTo(geom.V(0, -2))
	line := gomath.Line(geom.V(-4.5, 0), geom.V(4.5, 0)).SetStroke(gomath.RGB(.3, .4, .5), .025)
	pattern, err := gomath.NewDashPattern([]float64{.15, .1, .025, .1}, 0)
	if err != nil {
		return nil, err
	}
	line.Style.Dash = pattern
	o := gomath.Rectangle(1.2, .7).SetFill(gomath.Blue).SetStroke(gomath.White, .035).Shift(geom.V(-4, 0))
	if err = s.Add(heading, formula, line); err != nil {
		return nil, err
	}
	if err = s.Play(6, gomath.Succession(
		gomath.FadeIn(o).WithDuration(.5),
		gomath.Shift(o, geom.V(2, 0)),
		gomath.Succession(
			gomath.Shift(o, geom.V(2, 0)),
			gomath.Rotate(o, math.Pi),
			gomath.Shift(o, geom.V(2, 0)),
		).WithDuration(2),
		gomath.FadeOut(o).WithDuration(.5),
		gomath.FadeIn(o).WithDuration(.5),
		gomath.Shift(o, geom.V(2, 0)),
	)); err != nil {
		return nil, err
	}
	if err = s.Wait(.5); err != nil {
		return nil, err
	}
	return s, nil
}
