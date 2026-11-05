package scenes

import (
	"gomath"
	"gomath/geom"
	"gomath/number"
	"gomath/text"
)

func Numbers() (*gomath.Scene, error) {
	options := number.DefaultOptions()
	options.Size, options.Anchor = .7, number.Center
	options.IncludeSign, options.Unit = true, "kg"
	options.Color, options.MaxGlyphs = gomath.Blue, 64
	decimal, err := number.New(999.5, options)
	if err != nil {
		return nil, err
	}
	decimal.Object.Shift(geom.V(0, .5))
	source, err := decimal.Source()
	if err != nil {
		return nil, err
	}
	integerOptions := number.DefaultIntegerOptions()
	integerOptions.Size, integerOptions.Anchor, integerOptions.Color = .5, number.Center, gomath.Green
	integer, err := number.NewInteger(9007199254740993, integerOptions)
	if err != nil {
		return nil, err
	}
	integer.Object.Shift(geom.V(0, -1.2))
	tracker, err := gomath.NewValueTracker(999.5)
	if err != nil {
		return nil, err
	}
	scene := gomath.NewScene()
	heading, err := title("Numbers / sampled fixed decimals and exact int64 vector glyphs")
	if err != nil {
		return nil, err
	}
	heading.Scale(.87)
	if err = scene.Add(heading, decimal.Object, integer.Object, tracker.Object); err != nil {
		return nil, err
	}
	for i, value := range []string{"Sampled tracker / signs, grouping and two decimals", "Exact int64 / no float conversion"} {
		labelOptions := text.DefaultOptions()
		labelOptions.Size = .29
		label, err := text.New(value, text.DefaultFont(), labelOptions)
		if err != nil {
			return nil, err
		}
		label.MoveTo(geom.V(0, []float64{1.6, -2}[i]))
		if err = scene.Add(label); err != nil {
			return nil, err
		}
	}
	if err = scene.AddDynamicRedraw(decimal.Object, func(frame *gomath.Frame, _ float64) (*gomath.Object, error) {
		value, err := frame.Value(tracker.ID)
		if err != nil {
			return nil, err
		}
		return source.At(value)
	}, gomath.DynamicRedrawOptions{MaxObjects: 65}); err != nil {
		return nil, err
	}
	if err = scene.Play(1.5, gomath.ValueTo(tracker, 1200.5).WithRate(gomath.Linear)); err != nil {
		return nil, err
	}
	if err = scene.Play(1.5, gomath.ValueTo(tracker, -12.5).WithRate(gomath.Linear)); err != nil {
		return nil, err
	}
	if err = scene.Wait(.5); err != nil {
		return nil, err
	}
	return scene, nil
}
