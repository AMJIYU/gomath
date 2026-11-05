package scenes

import (
	"gomath"
	"gomath/geom"
	"gomath/text"
	"math"
)

func PointwiseMappings() (*gomath.Scene, error) {
	scene := gomath.NewScene()
	heading, err := title("Pointwise maps / private target geometry / preserved object identity")
	if err != nil {
		return nil, err
	}
	heading.Scale(.85)
	var lines []*gomath.Object
	for i := 0; i <= 10; i++ {
		x := -2 + .4*float64(i)
		var horizontal, vertical []geom.Vec3
		for j := 0; j <= 20; j++ {
			y := -2 + .2*float64(j)
			horizontal = append(horizontal, geom.V(y, x))
			vertical = append(vertical, geom.V(x, y))
		}
		lines = append(lines, gomath.Polyline(horizontal...).SetStroke(gomath.Blue, .025), gomath.Polyline(vertical...).SetStroke(gomath.Green, .025))
	}
	grid := gomath.Group(lines...)
	left, right := grid.Copy().Shift(geom.V(-3.5, 0)), grid.Copy().Shift(geom.V(3.5, 0))
	target := grid.Copy()
	if err = target.ApplyPointwise(func(p geom.Vec3) (geom.Vec3, error) {
		return geom.Vec3{X: p.X + .18*math.Sin(1.7*p.Y), Y: p.Y + .18*math.Sin(1.5*p.X), Z: p.Z}, nil
	}, gomath.DefaultPointwiseOptions()); err != nil {
		return nil, err
	}
	target.Shift(geom.V(3.5, 0))
	options := text.DefaultOptions()
	options.Size = .28
	caption, err := text.New("Segmented source grid / map evaluated once during authoring / no callback during sampling", text.DefaultFont(), options)
	if err != nil {
		return nil, err
	}
	caption.MoveTo(geom.V(0, -2.65))
	if err = scene.Add(heading, caption, left, right); err != nil {
		return nil, err
	}
	if err = scene.Play(2.5, gomath.Transform(right, target)); err != nil {
		return nil, err
	}
	if err = scene.Wait(1); err != nil {
		return nil, err
	}
	return scene, nil
}
