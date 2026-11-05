package scenes

import (
	"gomath"
	"gomath/audio"
	"gomath/geom"
	"gomath/text"
	"math"
)

// Composition exercises nested, duration-weighted group clocks, geometry
// overshoot, immutable sound scheduling and actual PCM synthesis.
func Composition() (*gomath.Scene, error) {
	s := gomath.NewScene()
	if err := s.AddSubcaption("组合动画与音频：纯 Go", .5, 0); err != nil {
		return nil, err
	}
	heading, err := title("Group clocks / easing / pure Go audio")
	if err != nil {
		return nil, err
	}
	if err = s.Add(heading); err != nil {
		return nil, err
	}
	var dots []*gomath.Object
	names := []string{"Smooth", "Back", "Elastic"}
	colors := []gomath.Color{gomath.Blue, gomath.Yellow, gomath.Pink}
	for i := range 3 {
		y := 1.2 - float64(i)*1.3
		opt := text.DefaultOptions()
		opt.Size = .35
		label, err := text.New(names[i], text.DefaultFont(), opt)
		if err != nil {
			return nil, err
		}
		label.MoveTo(geom.V(-5, y))
		line := gomath.Line(geom.V(-3, y), geom.V(3, y)).SetStroke(gomath.RGB(.3, .36, .46), .025)
		dot := gomath.Dot(geom.V(-3, y), .16).SetFill(colors[i]).SetStroke(gomath.Transparent, 0)
		if err = s.Add(label, line, dot); err != nil {
			return nil, err
		}
		dots = append(dots, dot)
	}
	opt := text.DefaultOptions()
	opt.Size = .27
	note, err := text.New("Unequal child durations, nested sigmoid clock, preserved overshoot", text.DefaultFont(), opt)
	if err != nil {
		return nil, err
	}
	note.MoveTo(geom.V(0, -2.7))
	if err = s.Add(note); err != nil {
		return nil, err
	}
	if err = s.Wait(.5); err != nil {
		return nil, err
	}
	if err = s.AddSubcaption("A shared group clock schedules three easing curves.", 4, 0); err != nil {
		return nil, err
	}
	for i, frequency := range []float64{261.625565, 329.627557, 391.995436} {
		const rate = 48000
		samples := make([]float64, rate*3/5*2)
		for j := 0; j < len(samples)/2; j++ {
			t := float64(j) / rate
			v := .12 * math.Sin(2*math.Pi*frequency*t) * math.Min(1, t/.01) * math.Exp(-5*t)
			samples[2*j], samples[2*j+1] = v, v
		}
		clip, err := audio.NewPCM(audio.Format{SampleRate: rate, Channels: 2, ChannelMask: 3}, samples)
		if err != nil {
			return nil, err
		}
		if err = s.AddSound(clip, []float64{0, .8, 1.4}[i]); err != nil {
			return nil, err
		}
	}
	group := gomath.LaggedGroup(.2,
		gomath.Shift(dots[0], geom.V(6, 0)).WithRate(gomath.Smooth).WithDuration(2),
		gomath.Shift(dots[1], geom.V(6, 0)).WithRate(gomath.EaseOutBack).WithDuration(1.5),
		gomath.Shift(dots[2], geom.V(6, 0)).WithRate(gomath.EaseOutElastic).WithDuration(1),
	)
	if err = s.Play(4, gomath.AnimationGroup(group.WithRate(gomath.Smooth))); err != nil {
		return nil, err
	}
	if err = s.Wait(.5); err != nil {
		return nil, err
	}
	return s, nil
}
