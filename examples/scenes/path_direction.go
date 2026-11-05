package scenes

import (
	"gomath"
	"gomath/geom"
	"gomath/path"
	"gomath/text"
)

func PathDirection() (*gomath.Scene, error) {
	var b path.Builder
	b.MoveTo(geom.V(-4, 1))
	b.CubicTo(geom.V(-3, 2.6), geom.V(-1.5, -.6), geom.V(-.5, 1))
	b.CubicTo(geom.V(.5, 2.2), geom.V(1.5, -.2), geom.V(2.3, 1))
	b.CubicTo(geom.V(3, 2), geom.V(3.6, .2), geom.V(4, 1))
	forward := gomath.NewObject(b.Path()).SetStroke(gomath.Blue, .06)
	backward := forward.Copy().Shift(geom.V(0, -2)).SetStroke(gomath.Pink, .06)
	if err := backward.ReverseDirection(); err != nil {
		return nil, err
	}
	first := gomath.Dot(forward.Path.Contours[0].Start, .1).SetFill(gomath.Yellow)
	second := gomath.Dot(backward.Path.Contours[0].Start, .1).SetFill(gomath.Yellow)
	heading, err := title("Path reversal / same curves, opposite reveal and traversal")
	if err != nil {
		return nil, err
	}
	style := text.DefaultOptions()
	style.Size = .3
	caption, err := text.New("Exact control reversal / no resampling or geometry fitting", text.DefaultFont(), style)
	if err != nil {
		return nil, err
	}
	caption.MoveTo(geom.V(0, -2.6))
	s := gomath.NewScene()
	if err = s.Add(heading, caption); err != nil {
		return nil, err
	}
	if err = s.Play(2, gomath.Create(forward), gomath.Create(backward)); err != nil {
		return nil, err
	}
	if err = s.Add(first, second); err != nil {
		return nil, err
	}
	if err = s.Play(2, gomath.MoveAlongPath(first, forward).WithRate(gomath.Linear), gomath.MoveAlongPath(second, backward).WithRate(gomath.Linear)); err != nil {
		return nil, err
	}
	if err = s.Wait(.6); err != nil {
		return nil, err
	}
	return s, nil
}
