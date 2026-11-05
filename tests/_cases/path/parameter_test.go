package path_test

import (
	"gomath/geom"
	"gomath/path"
	"math"
	"reflect"
	"testing"
)

func TestParameterTrimExactCubicsAndPenUpGaps(t *testing.T) {
	c := path.Cubic{P1: geom.V(2, 4), P2: geom.V(-3, 5), P3: geom.V(1, 1)}
	p := cubicPath(c)
	q, err := p.TrimByParameter(.2, .8)
	lengthMust(t, err)
	for i := range 33 {
		u := float64(i) / 32
		lengthPointNear(t, q.Contours[0].Curves[0].At(u), c.At(.2+.6*u), 1e-12)
	}
	var b path.Builder
	b.MoveTo(geom.V(0, 0))
	b.LineTo(geom.V(1, 0))
	b.MoveTo(geom.V(100, 0))
	b.LineTo(geom.V(110, 0))
	p = b.Path()
	q, err = p.TrimByParameter(.25, .75)
	lengthMust(t, err)
	if len(q.Contours) != 2 {
		t.Fatal("pen-up contours joined")
	}
	lengthPointNear(t, q.Contours[0].Start, geom.V(.5, 0), 1e-12)
	lengthPointNear(t, q.Contours[1].Curves[0].P3, geom.V(105, 0), 1e-12)
	full, err := p.TrimByParameter(0, 1)
	lengthMust(t, err)
	if !reflect.DeepEqual(full, p) {
		t.Fatal("full trim changed path")
	}
	full.Contours[0].Curves[0].P3.X = 9
	if p.Contours[0].Curves[0].P3.X != 1 {
		t.Fatal("trim retained writable input slices")
	}
	for _, v := range [][2]float64{{-.1, 1}, {.8, .2}, {0, 1.1}, {0, math.NaN()}} {
		if _, err := p.TrimByParameter(v[0], v[1]); err == nil {
			t.Fatal("invalid parameter interval accepted")
		}
	}
}
