package scenes

import (
	"gomath"
	"gomath/geom"
	"math"
)

func Primitives() (*gomath.Scene, error) {
	s := gomath.NewScene()
	label, err := title("Geometry / real holes, rounded corners and fixed dashes")
	if err != nil {
		return nil, err
	}
	ellipse, err := gomath.Ellipse(2.4, 1.4)
	if err != nil {
		return nil, err
	}
	ring, err := gomath.Annulus(.42, .9)
	if err != nil {
		return nil, err
	}
	sector, err := gomath.AnnularSector(.3, .9, .2, -1.5*math.Pi)
	if err != nil {
		return nil, err
	}
	rounded, err := gomath.RoundedRectangle(2.4, 1.3, .3)
	if err != nil {
		return nil, err
	}
	dashes, err := gomath.DashedLine(geom.V(-1.2, 0), geom.V(1.2, 0), .2, .55)
	if err != nil {
		return nil, err
	}
	double, err := gomath.DoubleArrow(geom.V(-1.2, 0), geom.V(1.2, 0), .22)
	if err != nil {
		return nil, err
	}
	ellipse.SetStroke(gomath.Blue, .05).MoveTo(geom.V(-4, 1))
	ring.SetFill(gomath.Pink).SetStroke(gomath.Transparent, 0).MoveTo(geom.V(0, 1))
	// Shift preserves the mathematical origin of an asymmetric sector.
	sector.SetFill(gomath.Green).SetStroke(gomath.Transparent, 0).Shift(geom.V(4, 1))
	fill := gomath.Blue
	fill.A = .25
	rounded.SetFill(fill).SetStroke(gomath.Blue, .05).MoveTo(geom.V(-4, -1.2))
	dashes.SetStroke(gomath.Yellow, .055).MoveTo(geom.V(0, -1.2))
	double.SetColor(gomath.Pink).MoveTo(geom.V(4, -1.2))
	if err = s.Add(label); err != nil {
		return nil, err
	}
	if err = s.Play(2, gomath.LaggedGroup(.2,
		gomath.GrowFromCenter(ellipse), gomath.GrowFromCenter(ring), gomath.GrowFromCenter(sector),
		gomath.GrowFromCenter(rounded), gomath.Create(dashes), gomath.GrowFromCenter(double),
	)); err != nil {
		return nil, err
	}
	if err = s.Play(1, gomath.Indicate(ring)); err != nil {
		return nil, err
	}
	if err = s.Wait(.5); err != nil {
		return nil, err
	}
	return s, nil
}
