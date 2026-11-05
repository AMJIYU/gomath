package scenes

import (
	"context"
	"fmt"
	"gomath"
	"gomath/field"
	"gomath/geom"
	"gomath/text"
	"math"
	"sort"
)

func adaptiveDemoLines() (*field.AdaptiveStreamLines, error) {
	options := field.DefaultAdaptiveStreamOptions()
	options.Integration.EndTime = 6
	options.Integration.InitialStep = .3
	options.Integration.AbsoluteTolerance = 1e-10
	options.Integration.RelativeTolerance = 1e-8
	options.ColorFunc = func(seed, _ geom.Vec3, _ float64) gomath.Color {
		return gomath.Blue.Lerp(gomath.Pink, (math.Atan2(seed.Y, seed.X)+math.Pi)/(2*math.Pi))
	}
	seeds := make([]geom.Vec3, 12)
	for i := range seeds {
		angle := float64(i) * 2 * math.Pi / float64(len(seeds))
		seeds[i] = geom.V(2.2*math.Cos(angle), 2.2*math.Sin(angle))
	}
	lines, err := field.NewAdaptiveStreamLines(context.Background(), func(p geom.Vec3) geom.Vec3 {
		return geom.V(-.12*p.X-p.Y, p.X-.12*p.Y)
	}, seeds, options)
	if err != nil {
		return nil, err
	}
	for i, line := range lines.Lines() {
		points, times := line.Result.Points(), line.Result.Times()
		if line.Result.Reason() != field.TimeComplete || len(points) != len(times) || len(points) < 2 || times[len(times)-1] != 6 {
			return nil, fmt.Errorf("adaptive example seed%d did not finish: %s", i, line.Result.Reason())
		}
		for j, point := range points {
			t := times[j]
			want := geom.RotationZ(t).Apply(seeds[i]).Mul(math.Exp(-.12 * t))
			if point.Sub(want).Norm() > 2e-7 {
				return nil, fmt.Errorf("adaptive example seed%d accepted state%d differs from analytic solution", i, j)
			}
		}
	}
	return lines, nil
}

// AdaptiveStreamLines shows actual RK45 accepted geometry and time-based dot
// interpolation. The numerical solver and callbacks run once during authoring.
func AdaptiveStreamLines() (*gomath.Scene, error) {
	lines, err := adaptiveDemoLines()
	if err != nil {
		return nil, err
	}
	heading, err := title("Adaptive RK45 / damped rotation / nonuniform accepted times")
	if err != nil {
		return nil, err
	}
	heading.Scale(.9)
	stats := lines.Stats()
	options := text.DefaultOptions()
	options.Size = .28
	caption, err := text.New(fmt.Sprintf("12 seeds / %d accepted / %d rejected / %d field calls\nx' = -0.12x - y,  y' = x - 0.12y", stats.AcceptedSteps, stats.RejectedSteps, stats.Evaluations), text.DefaultFont(), options)
	if err != nil {
		return nil, err
	}
	caption.MoveTo(geom.V(0, -2.8))
	s := gomath.NewScene()
	if err = s.Add(heading, caption, lines.Object,
		gomath.Line(geom.V(-3, 0), geom.V(3, 0)).SetStroke(gomath.RGB(.22, .25, .3), .02),
		gomath.Line(geom.V(0, -2.4), geom.V(0, 2.4)).SetStroke(gomath.RGB(.22, .25, .3), .02)); err != nil {
		return nil, err
	}
	for _, line := range lines.Lines() {
		times, points := line.Result.Times(), line.Result.Points()
		color := line.Object.Style.Stroke
		dot := gomath.Dot(points[0], .075).SetFill(color)
		if err = s.Add(dot); err != nil {
			return nil, err
		}
		if err = s.AddRedraw(dot, func(_ *gomath.Frame, time float64) (*gomath.Object, error) {
			physical := max(0, min(6, time*1.5))
			j := sort.SearchFloat64s(times, physical)
			point := points[len(points)-1]
			if j == 0 {
				point = points[0]
			} else if j < len(times) {
				point = points[j-1].Lerp(points[j], (physical-times[j-1])/(times[j]-times[j-1]))
			}
			return gomath.Dot(point, .075).SetFill(color), nil
		}); err != nil {
			return nil, err
		}
	}
	if err = s.Wait(4); err != nil {
		return nil, err
	}
	return s, nil
}
