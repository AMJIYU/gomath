package gomath

import (
	"fmt"
	"gomath/geom"
)

// FadeTransformOptions chooses ghost fitting for the two independent layers.
// Stretch independently matches all nonzero source extents; otherwise Dimension
// (0=X,1=Y,2=Z) selects one uniform scale. Zero source extents stay unscaled.
type FadeTransformOptions struct {
	Stretch   bool
	Dimension int
}

func DefaultFadeTransformOptions() FadeTransformOptions {
	return FadeTransformOptions{Stretch: true, Dimension: 1}
}

// FadeTransform crossfades two independently fitted layers. It supports distinct
// bitmap assets and image/vector transitions without morphing pixel assets.
// Source must be in the scene; target is introduced at the step's start. Source
// is removed and restored for reuse at the step's end; the real target remains.
func FadeTransform(source, target *Object) Animation {
	return FadeTransformWithOptions(source, target, DefaultFadeTransformOptions())
}

func FadeTransformWithOptions(source, target *Object, options FadeTransformOptions) Animation {
	out := Animation{object: source, target: target, mode: crossFadeOut, rate: Linear, end: 1, fit: &options}
	in := Animation{object: target, target: source, mode: crossFadeIn, rate: Linear, end: 1, fit: &options}
	return AnimationGroup(out, in).WithRate(Smooth)
}

func fitGhost(object, target *Object, opt *FadeTransformOptions) error {
	if opt == nil || opt.Dimension < 0 || opt.Dimension > 2 {
		return fmt.Errorf("FadeTransform dimension must be 0, 1 or 2")
	}
	if target == nil {
		return fmt.Errorf("FadeTransform has nil peer")
	}
	a, b := object.Bounds(), target.Bounds()
	if !a.Valid || !b.Valid {
		return nil
	}
	x, y := a.Size(), b.Size()
	from, to := [3]float64{x.X, x.Y, x.Z}, [3]float64{y.X, y.Y, y.Z}
	factors := [3]float64{1, 1, 1}
	if opt.Stretch {
		for i := range 3 {
			if from[i] != 0 {
				factors[i] = to[i] / from[i]
			}
		}
	} else if from[opt.Dimension] != 0 {
		scale := to[opt.Dimension] / from[opt.Dimension]
		factors = [3]float64{scale, scale, scale}
	}
	for _, v := range factors {
		if !finite(v) {
			return fmt.Errorf("FadeTransform fit scale overflows")
		}
	}
	object.ApplyMatrix(geom.Translation(a.Center().Mul(-1))).ApplyMatrix(geom.Scaling(factors[0], factors[1], factors[2])).Shift(b.Center())
	return object.Validate()
}
