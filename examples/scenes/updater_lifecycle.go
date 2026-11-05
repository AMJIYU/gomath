package scenes

import (
	"gomath"
	"gomath/geom"
	"gomath/text"
	"math"
)

func UpdaterLifecycle() (*gomath.Scene, error) {
	scene := gomath.NewScene()
	heading, err := title("Object updater lifecycle / pause, animate, resume, remove")
	if err != nil {
		return nil, err
	}
	dot := gomath.Dot(geom.V(0, .3), .2).SetFill(gomath.Pink)
	axis := gomath.Line(geom.V(-5, -.8), geom.V(5, -.8)).SetStroke(gomath.RGB(.25, .3, .38), .02)
	phase := gomath.Rectangle(1.9, .06).SetFill(gomath.Yellow).MoveTo(geom.V(-4.4, -2.1))
	if err = scene.Add(heading, dot, axis, phase); err != nil {
		return nil, err
	}
	opt := text.DefaultOptions()
	opt.Size = .28
	for i, name := range []string{"registered", "suspended", "animation", "resumed", "removed"} {
		label, e := text.New(name, text.DefaultFont(), opt)
		if e != nil {
			return nil, e
		}
		label.MoveTo(geom.V(-4.4+2.2*float64(i), -1.7))
		if err = scene.Add(label); err != nil {
			return nil, err
		}
	}
	if _, err = scene.AddObjectRedraw(phase, func(_ *gomath.Frame, t float64) (*gomath.Object, error) {
		i := math.Min(4, math.Floor(t))
		return gomath.Rectangle(1.9, .06).SetFill(gomath.Yellow).MoveTo(geom.V(-4.4+2.2*i, -2.1)), nil
	}, gomath.DefaultObjectUpdaterOptions()); err != nil {
		return nil, err
	}
	handle, err := scene.AddObjectUpdater(dot, func(o *gomath.Object, _ *gomath.Frame, t float64) error {
		o.MoveTo(geom.V(3*math.Sin(.8*t), .5*math.Cos(1.5*t)+.3))
		return nil
	}, gomath.DefaultObjectUpdaterOptions())
	if err != nil {
		return nil, err
	}
	if err = scene.Wait(1); err != nil {
		return nil, err
	}
	if err = scene.SuspendUpdater(handle); err != nil {
		return nil, err
	}
	if err = scene.Wait(1); err != nil {
		return nil, err
	}
	if err = scene.ResumeUpdater(handle); err != nil {
		return nil, err
	}
	if err = scene.Play(1, gomath.Shift(dot, geom.V(3.5, 0))); err != nil {
		return nil, err
	}
	if err = scene.Wait(1); err != nil {
		return nil, err
	}
	if err = scene.RemoveUpdater(handle); err != nil {
		return nil, err
	}
	if err = scene.Wait(1); err != nil {
		return nil, err
	}
	return scene, nil
}
