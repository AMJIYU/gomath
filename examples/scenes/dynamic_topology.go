package scenes

import (
	"gomath"
	"gomath/geom"
	"math"
)

// DynamicTopology grows a display family from zero to sixteen actual vector
// bars, while authoring retains one empty root and a scalar animation track.
func DynamicTopology() (*gomath.Scene, error) {
	s := gomath.NewScene()
	heading, err := title("Dynamic topology / sampled values / deterministic children")
	if err != nil {
		return nil, err
	}
	tracker, err := gomath.NewValueTracker(0)
	if err != nil {
		return nil, err
	}
	root := gomath.Group()
	axis := gomath.Line(geom.V(-6, -1.5), geom.V(6, -1.5)).SetStroke(gomath.White, .025)
	if err = s.Add(heading, axis, root, tracker.Object); err != nil {
		return nil, err
	}
	id := tracker.ID
	if err = s.AddDynamicRedraw(root, func(f *gomath.Frame, _ float64) (*gomath.Object, error) {
		value, err := f.Value(id)
		if err != nil {
			return nil, err
		}
		group := gomath.Group()
		for i := 0; i < int(math.Floor(value)); i++ {
			height := 1.2 + 1.4*math.Sin((float64(i)+1)*.31)
			bar := gomath.Rectangle(.55, height).MoveTo(geom.V(-5.625+.75*float64(i), -1.5+height/2))
			bar.SetFill(gomath.Blue.Lerp(gomath.Pink, float64(i)/15)).SetStroke(gomath.Transparent, 0)
			group.Children = append(group.Children, bar)
		}
		return group, nil
	}, gomath.DynamicRedrawOptions{MaxObjects: 17, MaxCurves: 64, MaxContours: 16, MaxDepth: 2}); err != nil {
		return nil, err
	}
	if err = s.Play(4, gomath.ValueTo(tracker, 16).WithRate(gomath.Linear)); err != nil {
		return nil, err
	}
	if err = s.Wait(.5); err != nil {
		return nil, err
	}
	return s, nil
}
