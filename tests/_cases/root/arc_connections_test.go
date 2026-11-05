package gomath_test

import (
	"gomath"
	"gomath/geom"
	"math"
	"testing"
)

func TestArcConnectionsCirclePlaneAndTinyBulge(t *testing.T) {
	for _, sweep := range []float64{.001, -.001, math.Pi / 2, -math.Pi / 2, math.Pi, -math.Pi, 4.8, -4.8} {
		arc, err := gomath.NewArcBetweenPoints(geom.V(-1, .5), geom.V(2, -.3), gomath.ArcBetweenPointsOptions{Angle: sweep})
		must(t, err)
		must(t, arc.Object.Validate())
		contour := arc.Object.Path.Contours[0]
		if arc.Linear || contour.Start != arc.Start || contour.Curves[7].P3 != arc.End || len(contour.Curves) != 8 || contour.Closed {
			t.Fatal("arc endpoints/topology differ", sweep)
		}
		for _, curve := range contour.Curves {
			for _, point := range []geom.Vec3{curve.P0, curve.P3} {
				if math.Abs(point.Sub(arc.Center).Norm()-arc.Radius) > 2e-10*arc.Radius {
					t.Fatal("arc anchor not on specified circle", sweep)
				}
			}
		}
		near(t, arc.Sweep, sweep)
		near(t, arc.Normal.Z, 1)
	}
	for _, radius := range []float64{1, -1, 2, -2} {
		arc, err := gomath.NewArcBetweenPoints(geom.V(-1, 0), geom.V(1, 0), gomath.ArcBetweenPointsOptions{Radius: radius, Angle: 9})
		must(t, err) // Explicit radius overrides finite Angle, as in Manim.
		near(t, arc.Radius, math.Abs(radius))
		near(t, arc.Sweep, math.Copysign(2*math.Asin(1/math.Abs(radius)), radius))
	}
	arc, err := gomath.NewArcBetweenPoints(geom.V(0, 0), geom.V(1, 0), gomath.ArcBetweenPointsOptions{Angle: 1e-12})
	must(t, err)
	mid := arc.Object.Path.Contours[0].Curves[3].P3
	near(t, mid.X, .5)
	if math.Abs(mid.Y+1.25e-13) > 1e-28 {
		t.Fatal("small sagitta lost to circle-center cancellation", mid)
	}
	arc, err = gomath.NewArcBetweenPoints(geom.Vec3{}, geom.Vec3{Y: 2}, gomath.ArcBetweenPointsOptions{Angle: math.Pi / 2, Normal: geom.Vec3{X: 1e-300}})
	must(t, err)
	mid = arc.Object.Path.Contours[0].Curves[3].P3
	if mid.Sub(geom.Vec3{Y: 1, Z: 1 - math.Sqrt2}).Norm() > 1e-12 || arc.Normal.Sub(geom.Vec3{X: 1}).Norm() > 1e-12 {
		t.Fatal("explicit 3D arc plane", mid, arc.Normal)
	}
	line, err := gomath.NewArcBetweenPoints(geom.V(-1, 0), geom.V(1, 0), gomath.ArcBetweenPointsOptions{})
	must(t, err)
	if !line.Linear || line.Radius != 0 || line.Sweep != 0 || len(line.Object.Path.Contours[0].Curves) != 1 {
		t.Fatal("zero sweep must give an ordinary line")
	}
}

func TestCurvedArrowHeadsJoinTrimmedShaftAndKeepIdentity(t *testing.T) {
	for _, double := range []bool{false, true} {
		for _, sweep := range []float64{0, math.Pi / 2, -math.Pi / 2, 4.8} {
			opts := gomath.CurvedArrowOptions{Arc: gomath.ArcBetweenPointsOptions{Angle: sweep}, TipLength: .25, TipWidth: .2, Double: double}
			start, end := geom.V(-2, 0), geom.V(2, 0)
			arrow, err := gomath.NewCurvedArrow(start, end, opts)
			must(t, err)
			must(t, arrow.Validate())
			want := 2
			if double {
				want = 3
			}
			if len(arrow.Children) != want {
				t.Fatal("curved arrow child count")
			}
			shaft := arrow.Children[0]
			c := shaft.Path.Contours[0]
			for i, tip := range arrow.Children[1:] {
				contour := tip.Path.Contours[0]
				base1, base2 := contour.Curves[0].P3, contour.Curves[1].P3
				wantTip, wantBase := end, c.Curves[len(c.Curves)-1].P3
				if i == 1 {
					wantTip, wantBase = start, c.Start
				}
				if contour.Start != wantTip || base1.Lerp(base2, .5).Sub(wantBase).Norm() > 1e-10 || math.Abs(base1.Sub(base2).Norm()-.2) > 1e-12 || !contour.Closed {
					t.Fatal("curved tip does not join the actual trimmed shaft", double, sweep)
				}
				if tip.Style.Fill != gomath.White || tip.Style.Stroke.A != 0 {
					t.Fatal("curved tip paint")
				}
			}
			s := gomath.NewScene()
			must(t, s.Add(arrow))
			must(t, s.Play(1, gomath.Shift(arrow, geom.V(1, .5))))
			if arrow.Children[0] != shaft {
				t.Fatal("curved arrow animation replaced shaft pointer")
			}
			frame, err := s.Sample(.4)
			must(t, err)
			frame.Objects[0].Children[0].Path.Contours[0].Start.X = 99
			again, err := s.Sample(.4)
			must(t, err)
			if again.Objects[0].Children[0].Path.Contours[0].Start.X == 99 {
				t.Fatal("curved arrow frame storage shared")
			}
		}
	}
}

func TestArcConnectionsAndArrowRejectUnsupportedGeometry(t *testing.T) {
	for _, options := range []gomath.ArcBetweenPointsOptions{
		{Angle: math.NaN()}, {Angle: 2 * math.Pi}, {Angle: -2 * math.Pi}, {Angle: 1e-15},
		{Angle: 1, Radius: .4}, {Radius: math.Inf(1)}, {Radius: 1e13},
		{Angle: 1, Normal: geom.V(1, 0)}, {Angle: 1, Normal: geom.Vec3{Z: math.NaN()}},
		{Angle: 1, Normal: geom.Vec3{X: math.MaxFloat64, Y: math.MaxFloat64, Z: math.MaxFloat64}},
	} {
		if arc, err := gomath.NewArcBetweenPoints(geom.V(0, 0), geom.V(1, 0), options); arc != nil || err == nil {
			t.Fatal("invalid arc accepted", options)
		}
	}
	for _, pair := range [][2]geom.Vec3{{geom.V(0, 0), geom.V(0, 0)}, {geom.V(0, 0), geom.V(1e-12, 0)}, {geom.V(1e13, 0), geom.V(0, 0)}, {geom.V(0, 0), {Z: 1}}} {
		if arc, err := gomath.NewArcBetweenPoints(pair[0], pair[1], gomath.DefaultArcBetweenPointsOptions()); arc != nil || err == nil {
			t.Fatal("invalid endpoints or implicit arc plane accepted", pair)
		}
	}
	for _, options := range []gomath.CurvedArrowOptions{
		{TipLength: -1}, {TipWidth: math.NaN()}, {TipLength: 2}, {TipLength: .6, Double: true},
		{Arc: gomath.ArcBetweenPointsOptions{Normal: geom.V(1, 0)}},
	} {
		if arrow, err := gomath.NewCurvedArrow(geom.V(0, 0), geom.V(1, 0), options); arrow != nil || err == nil {
			t.Fatal("invalid arrow accepted", options)
		}
	}
}

func BenchmarkArcBetweenPoints(b *testing.B) {
	options := gomath.DefaultArcBetweenPointsOptions()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := gomath.NewArcBetweenPoints(geom.V(-2, 0), geom.V(2, .5), options); err != nil {
			b.Fatal(err)
		}
	}
}
