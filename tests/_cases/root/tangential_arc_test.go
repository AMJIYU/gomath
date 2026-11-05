package gomath_test

import (
	"gomath"
	"gomath/geom"
	"math"
	"testing"
)

func TestTangentialArcTouchesBothSelectedInfiniteLines(t *testing.T) {
	first := gomath.Line(geom.V(-2, -.4), geom.V(2, .4))
	second := gomath.Line(geom.V(-.4, -2), geom.V(.4, 2))
	u, v := geom.V(4, .8).Unit(), geom.V(.8, 4).Unit()
	for _, corner := range [][2]int{{1, 1}, {1, -1}, {-1, 1}, {-1, -1}} {
		for _, radius := range []float64{.1, .7, 2} {
			arc, err := gomath.NewTangentialArc(first, second, gomath.TangentialArcOptions{Radius: radius, Corner: corner})
			must(t, err)
			must(t, arc.Object.Validate())
			near(t, arc.Radius, radius)
			if arc.Sweep <= 0 || arc.Sweep >= math.Pi {
				t.Fatal("fillet must use the positive minor sweep", arc.Sweep)
			}
			center := arc.Center
			if math.Abs(math.Abs(center.X*u.Y-center.Y*u.X)-radius) > 1e-12 || math.Abs(math.Abs(center.X*v.Y-center.Y*v.X)-radius) > 1e-12 {
				t.Fatal("fillet center not one radius from each line", center, radius)
			}
			var touches [2]bool
			contour := arc.Object.Path.Contours[0]
			for i, endpoint := range []geom.Vec3{arc.Start, arc.End} {
				tangent := contour.Curves[0].P1.Sub(arc.Start).Unit()
				if i == 1 {
					tangent = arc.End.Sub(contour.Curves[7].P2).Unit()
				}
				for j, ray := range []geom.Vec3{u.Mul(float64(corner[0])), v.Mul(float64(corner[1]))} {
					if math.Abs(endpoint.X*ray.Y-endpoint.Y*ray.X) < 1e-12 && endpoint.Dot(ray) > 0 {
						if math.Abs(tangent.X*ray.Y-tangent.Y*ray.X) > 1e-12 {
							t.Fatal("fillet endpoint is not tangent to its selected line")
						}
						touches[j] = true
					}
				}
			}
			if !touches[0] || !touches[1] {
				t.Fatal("fillet did not touch both requested rays", corner)
			}
		}
	}
}

func TestTangentialArcRejectsInvalidOrParallelInputs(t *testing.T) {
	first, second := gomath.Line(geom.V(-1, 0), geom.V(1, 0)), gomath.Line(geom.V(0, -1), geom.V(0, 1))
	for _, options := range []gomath.TangentialArcOptions{{}, {Radius: -1}, {Radius: math.NaN()}, {Radius: math.Inf(1)}, {Radius: 1, Corner: [2]int{0, 1}}, {Radius: 1, Corner: [2]int{1, 2}}} {
		if arc, err := gomath.NewTangentialArc(first, second, options); arc != nil || err == nil {
			t.Fatal("invalid fillet options accepted", options)
		}
	}
	for _, object := range []*gomath.Object{nil, first, gomath.Circle(1), gomath.Line(geom.V(0, 1), geom.V(1, 1+1e-13))} {
		if arc, err := gomath.NewTangentialArc(first, object, gomath.TangentialArcOptions{Radius: .4}); arc != nil || err == nil {
			t.Fatal("invalid/parallel fillet lines accepted")
		}
	}
}
