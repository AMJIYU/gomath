package scenes

import (
	"gomath"
	"gomath/geom"
	"gomath/text"
)

func Gradients() (*gomath.Scene, error) {
	s := gomath.NewScene()
	heading, err := title("Vector gradients / linear and radial / affine paint frames")
	if err != nil {
		return nil, err
	}
	linear, err := gomath.NewLinearGradient(geom.V(-5.4, 0), geom.V(-.6, 0), []gomath.GradientStop{
		{Offset: 0, Color: gomath.Blue}, {Offset: .45, Color: gomath.Pink}, {Offset: 1, Color: gomath.Yellow},
	})
	if err != nil {
		return nil, err
	}
	radial, err := gomath.NewRadialGradient(geom.V(3.1, .1), 1.5, []gomath.GradientStop{
		{Offset: 0, Color: gomath.RGB(1, .95, .65)}, {Offset: .4, Color: gomath.RGB(.9, .4, .2)}, {Offset: 1, Color: gomath.RGB(.1, .2, .55)},
	})
	if err != nil {
		return nil, err
	}
	left := gomath.Rectangle(4.8, 2.5).MoveTo(geom.V(-3, .1)).SetFillGradient(linear).SetStroke(gomath.White, .025)
	right := gomath.Circle(1.5).MoveTo(geom.V(3.1, .1)).SetFillGradient(radial).SetStroke(gomath.White, .025)
	clip, err := gomath.NewClip(gomath.Circle(1.4).MoveTo(geom.V(3.1, .1)).Path)
	if err != nil {
		return nil, err
	}
	right.SetClip(clip)
	opt := text.DefaultOptions()
	opt.Size = .32
	caption, err := text.New("Real vector coverage / immutable stops / nested clipping / SVG paint servers", text.DefaultFont(), opt)
	if err != nil {
		return nil, err
	}
	caption.MoveTo(geom.V(0, -2.6))
	if err = s.Add(heading, caption, left, right); err != nil {
		return nil, err
	}
	if err = s.Play(1.5, gomath.Rotate(left, .3), gomath.Scale(right, .85)); err != nil {
		return nil, err
	}
	if err = s.Play(1.5, gomath.Shift(left, geom.V(.3, 0)), gomath.Rotate(right, -.35)); err != nil {
		return nil, err
	}
	if err = s.Wait(1); err != nil {
		return nil, err
	}
	return s, nil
}
