package scenes

import (
	"gomath"
	"gomath/geom"
	"gomath/path"
	"gomath/text"
)

func CurveEdit() (*gomath.Scene, error) {
	anchors := []geom.Vec3{geom.V(-1.8, -.5), geom.V(-1, .9), geom.V(-.2, -.4), geom.V(.8, 1), geom.V(1.8, -.2)}
	contour, err := path.FromAnchors(anchors, path.AnchorJagged)
	if err != nil {
		return nil, err
	}
	line := gomath.NewObject(path.Path{Contours: []path.Contour{contour}}).SetStroke(gomath.Blue, .045).Shift(geom.V(-3.3, -.1))
	smooth := line.Copy()
	if err = smooth.MakeSmooth(); err != nil {
		return nil, err
	}
	smooth.SetStroke(gomath.Pink, .045)
	grid := gomath.Group()
	for i := -3; i <= 3; i++ {
		v := float64(i) * .5
		grid.Children = append(grid.Children, gomath.Line(geom.V(-1.5, v), geom.V(1.5, v)), gomath.Line(geom.V(v, -1.5), geom.V(v, 1.5)))
	}
	grid.SetStroke(gomath.Blue, .018)
	if err = grid.PrepareForNonlinearTransform(32); err != nil {
		return nil, err
	}
	warped := grid.Copy()
	if err = warped.ApplyComplex(func(z complex128) (complex128, error) { return z + .18*z*z, nil }, gomath.DefaultPointwiseOptions()); err != nil {
		return nil, err
	}
	grid.Shift(geom.V(3.2, 0))
	warped.Shift(geom.V(3.2, 0)).SetStroke(gomath.Green, .018)
	heading, err := title("Curve editing / smooth handles and nonlinear preparation")
	if err != nil {
		return nil, err
	}
	options := text.DefaultOptions()
	options.Size = .28
	left, err := text.New("Jagged anchors -> smooth cubic spline", text.DefaultFont(), options)
	if err != nil {
		return nil, err
	}
	left.MoveTo(geom.V(-3.3, -2.3))
	right, err := text.New("32 exact subdivisions per line -> complex map", text.DefaultFont(), options)
	if err != nil {
		return nil, err
	}
	right.MoveTo(geom.V(3.1, -2.3))
	scene := gomath.NewScene()
	if err = scene.Add(heading, left, right, line, grid); err != nil {
		return nil, err
	}
	if err = scene.Play(2.5, gomath.Transform(line, smooth), gomath.Transform(grid, warped)); err != nil {
		return nil, err
	}
	if err = scene.Wait(1); err != nil {
		return nil, err
	}
	return scene, nil
}
