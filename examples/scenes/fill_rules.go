package scenes

import (
	"gomath"
	"gomath/geom"
	"gomath/path"
	"gomath/text"
)

func FillRules() (*gomath.Scene, error) {
	compound := func(inner float64) path.Path {
		outer := gomath.Circle(1.7).Path.Clone()
		outer.Contours = append(outer.Contours, gomath.Circle(inner).Path.Contours...)
		return outer
	}
	gradient, err := gomath.NewLinearGradient(geom.V(-1.7, 0), geom.V(1.7, 0), []gomath.GradientStop{
		{Offset: 0, Color: gomath.Blue}, {Offset: .5, Color: gomath.Pink}, {Offset: 1, Color: gomath.Yellow},
	})
	if err != nil {
		return nil, err
	}
	left := gomath.NewObject(compound(.7)).SetFillGradient(gradient).SetStroke(gomath.White, .035).Shift(geom.V(-3.1, 0))
	right := gomath.NewObject(compound(.7)).SetFillGradient(gradient).SetFillRule(path.EvenOdd).SetStroke(gomath.White, .035).Shift(geom.V(3.1, 0))
	left.Name, right.Name = "Nonzero ordinary fill", "Evenodd ordinary fill"
	leftTarget, rightTarget := left.Copy(), right.Copy()
	leftTarget.Path = compound(1.1).Transform(geom.Translation(geom.V(-3.1, 0)))
	rightTarget.Path = compound(1.1).Transform(geom.Translation(geom.V(3.1, 0)))
	heading, err := title("Ordinary fill rules / same-direction contours / gradient interiors")
	if err != nil {
		return nil, err
	}
	heading.Scale(.9)
	s := gomath.NewScene()
	if err = s.SetCamera(gomath.Camera2D{FrameWidth: 14.2222222222}); err != nil {
		return nil, err
	}
	if err = s.Add(heading, left, right); err != nil {
		return nil, err
	}
	for i, value := range []string{"nonzero / filled center", "evenodd / actual hole"} {
		options := text.DefaultOptions()
		options.Size = .32
		caption, err := text.New(value, text.DefaultFont(), options)
		if err != nil {
			return nil, err
		}
		caption.MoveTo(geom.V(-3.1+6.2*float64(i), -2.4))
		if err = s.Add(caption); err != nil {
			return nil, err
		}
	}
	if err = s.Wait(.5); err != nil {
		return nil, err
	}
	if err = s.AnimateCameraWithRate(3, gomath.Camera2D{FrameWidth: 14.2222222222, Rotation: .05}, gomath.Linear,
		gomath.Transform(left, leftTarget), gomath.Transform(right, rightTarget)); err != nil {
		return nil, err
	}
	if err = s.Wait(.5); err != nil {
		return nil, err
	}
	return s, nil
}
