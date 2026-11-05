package scenes

import (
	"gomath"
	"gomath/geom"
	"gomath/path"
	"gomath/text"
)

// Boolean shows actual polygon arrangements. Both operands contain straight
// edges; the example does not approximate circles or claim curved booleans.
func Boolean() (*gomath.Scene, error) {
	s := gomath.NewScene()
	heading, err := title("Polygon Boolean / exact intersections / checked topology")
	if err != nil {
		return nil, err
	}
	if err = s.Add(heading); err != nil {
		return nil, err
	}
	var results []*gomath.Object
	for i, name := range []string{"union", "intersection", "A minus B", "exclusive or"} {
		center := geom.V(float64(i%2)*6-3, 1.35-float64(i/2)*2.75)
		a := gomath.Rectangle(2.2, 1.55).MoveTo(geom.V(-.35, 0))
		b := gomath.Polygon(geom.V(-.4, -.9), geom.V(1.25, -.9), geom.V(1.25, .85), geom.V(.3, 1.05))
		combined, err := path.Boolean(a.Path, b.Path, path.BooleanOp(i), path.DefaultBooleanOptions())
		if err != nil {
			return nil, err
		}
		result := gomath.NewObject(combined).SetFill(gomath.Blue.Lerp(gomath.Pink, float64(i)/3)).SetStroke(gomath.White, .018)
		group := gomath.Group(a.SetFill(gomath.Transparent).SetStroke(gomath.RGB(.28, .36, .48), .018),
			b.SetFill(gomath.Transparent).SetStroke(gomath.RGB(.5, .4, .28), .018), result).Shift(center)
		opt := text.DefaultOptions()
		opt.Size = .32
		label, err := text.New(name, text.DefaultFont(), opt)
		if err != nil {
			return nil, err
		}
		label.MoveTo(center.Add(geom.V(0, -1.22)))
		if err = s.Add(group, label); err != nil {
			return nil, err
		}
		results = append(results, group)
	}
	animations := make([]gomath.Animation, len(results))
	for i, result := range results {
		angle := .12
		if i%2 != 0 {
			angle = -angle
		}
		animations[i] = gomath.Rotate(result, angle)
	}
	if err = s.Play(1.5, animations...); err != nil {
		return nil, err
	}
	if err = s.Wait(1); err != nil {
		return nil, err
	}
	return s, nil
}
