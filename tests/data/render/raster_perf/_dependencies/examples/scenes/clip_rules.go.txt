package scenes

import (
	"gomath"
	"gomath/geom"
	"gomath/path"
	"gomath/text"
)

func ClipRules() (*gomath.Scene, error) {
	compound := func(inner float64) path.Path {
		outer := gomath.Circle(1.7).Path.Clone()
		outer.Contours = append(outer.Contours, gomath.Circle(inner).Path.Contours...)
		return outer
	}
	leftClip, err := gomath.NewClipWithRule(compound(.7), path.NonZero)
	if err != nil {
		return nil, err
	}
	rightClip, err := gomath.NewClipWithRule(compound(.7), path.EvenOdd)
	if err != nil {
		return nil, err
	}
	bands := func() *gomath.Object {
		var objects []*gomath.Object
		for i := range 7 {
			color := gomath.RGB(.15+.1*float64(i), .65-.06*float64(i), .9-.08*float64(i))
			objects = append(objects, gomath.Rectangle(3.8, .54).SetFill(color).SetStroke(gomath.Transparent, 0).Shift(geom.V(0, -1.62+.54*float64(i))))
		}
		return gomath.Group(objects...)
	}
	left, right := bands().SetClip(leftClip), bands().SetClip(rightClip)
	left.Name, right.Name = "Nonzero clipped bands", "Evenodd clipped bands"
	left.Shift(geom.V(-3.1, 0))
	right.Shift(geom.V(3.1, 0))
	newLeft, err := gomath.NewClipWithRule(compound(1.1).Transform(geom.Translation(geom.V(-3.1, 0))), path.NonZero)
	if err != nil {
		return nil, err
	}
	newRight, err := gomath.NewClipWithRule(compound(1.1).Transform(geom.Translation(geom.V(3.1, 0))), path.EvenOdd)
	if err != nil {
		return nil, err
	}
	heading, err := title("Clip fill rules / same-direction contours / animated hole and camera")
	if err != nil {
		return nil, err
	}
	heading.Scale(.87)
	options := text.DefaultOptions()
	options.Size = .3
	lcaption, err := text.New("nonzero / the inner contour stays filled", text.DefaultFont(), options)
	if err != nil {
		return nil, err
	}
	rcaption, err := text.New("evenodd / the inner contour opens a hole", text.DefaultFont(), options)
	if err != nil {
		return nil, err
	}
	lcaption.MoveTo(geom.V(-3.1, -2.4))
	rcaption.MoveTo(geom.V(3.1, -2.4))
	s := gomath.NewScene()
	if err = s.SetCamera(gomath.Camera2D{FrameWidth: 14.2222222222}); err != nil {
		return nil, err
	}
	if err = s.Add(heading, lcaption, rcaption, left, right); err != nil {
		return nil, err
	}
	if err = s.Wait(.5); err != nil {
		return nil, err
	}
	if err = s.AnimateCameraWithRate(3, gomath.Camera2D{FrameWidth: 14.2222222222, Rotation: .05}, gomath.Linear,
		gomath.Transform(left, left.Copy().SetClip(newLeft)), gomath.Transform(right, right.Copy().SetClip(newRight))); err != nil {
		return nil, err
	}
	if err = s.Wait(.5); err != nil {
		return nil, err
	}
	return s, nil
}
