package graph_test

import (
	"gomath/geom"
	"gomath/graph"
	"math"
	"reflect"
	"testing"
)

func TestPolarPlaneGridAndAffineCoordinates(t *testing.T) {
	options := graph.DefaultPolarPlaneOptions()
	options.Offset, options.Clockwise, options.Rays = .4, true, 12
	p, err := graph.NewPolarPlane(graph.Range{Min: 0, Max: 3, Step: 1}, 2.5, options)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Circles()) != 3 || len(p.Rays()) != 12 {
		t.Fatal("polar grid count")
	}
	for i, circle := range p.Circles() {
		if circle.Radius != float64(i+1) || len(circle.Object.Path.Contours[0].Curves) != 4 || math.Abs(circle.Object.Bounds().Size().X-2*2.5*circle.Radius/3) > 1e-12 {
			t.Fatal("polar circle geometry", circle.Radius)
		}
	}
	for i, ray := range p.Rays() {
		angle := options.Offset - float64(i)*2*math.Pi/12
		want := geom.V(2.5*math.Cos(angle), 2.5*math.Sin(angle))
		if ray.Path.Contours[0].Start != (geom.Vec3{}) || ray.Path.Contours[0].Curves[0].P3.Sub(want).Norm() > 1e-12 {
			t.Fatal("polar ray", i)
		}
	}
	// A nonorthogonal transform tests actual affine inversion, including
	// off-plane projection and azimuth orientation/offset restoration.
	matrix := geom.Mat4{2, .3, 0, .7, -.2, -.7, 0, -1, .3, .4, 1, .2, 0, 0, 0, 1}
	p.Object.ApplyMatrix(matrix)
	u := matrix.Apply(geom.V(1, 0)).Sub(matrix.Apply(geom.V(0, 0)))
	v := matrix.Apply(geom.V(0, 1)).Sub(matrix.Apply(geom.V(0, 0)))
	normal := u.Cross(v).Unit()
	center, err := p.Point(0, 2.8)
	if err != nil {
		t.Fatal(err)
	}
	if r, a, err := p.Coordinates(center); err != nil || r != 0 || a != 0 {
		t.Fatal("transformed origin convention", r, a, err)
	}
	for _, radius := range []float64{.1, 1, 3, 7} {
		for _, angle := range []float64{0, .8, math.Pi, 5.7} {
			point, err := p.Point(radius, angle)
			want := matrix.Apply(geom.V(radius*2.5/3*math.Cos(.4-angle), radius*2.5/3*math.Sin(.4-angle)))
			if err != nil || point.Sub(want).Norm() > 2e-12 {
				t.Fatal("polar affine forward", radius, angle, err)
			}
			backR, backA, err := p.Coordinates(point.Add(normal.Mul(2)))
			angleError := math.Abs(math.Remainder(backA-angle, 2*math.Pi))
			if err != nil || math.Abs(backR-radius) > 2e-12 || angleError > 2e-12 {
				t.Fatal("polar affine inverse", radius, angle, backR, backA, err)
			}
		}
	}
	selection := p.Rays()
	first := selection[0]
	selection[0] = nil
	circles := p.Circles()
	circles[0].Radius, circles[0].Object = 10, nil
	if p.Rays()[0] != first || p.Circles()[0].Radius != 1 || p.Circles()[0].Object == nil {
		t.Fatal("polar selection storage exposed")
	}
}

func TestPolarPlaneOriginBoundaryAndPlots(t *testing.T) {
	p, err := graph.NewPolarPlane(graph.Range{Min: 0, Max: 2.5, Step: 1}, 2.5, graph.DefaultPolarPlaneOptions())
	if err != nil {
		t.Fatal(err)
	}
	if got := p.Circles(); len(got) != 3 || got[2].Radius != 2.5 {
		t.Fatal("explicit outer circle missing")
	}
	r, a, err := p.Coordinates(geom.V(0, 0))
	if err != nil || r != 0 || a != 0 {
		t.Fatal("origin convention", r, a, err)
	}
	// Reduction before offset addition accepts huge finite azimuths.
	if _, err = p.Point(1, math.MaxFloat64); err != nil {
		t.Fatal("finite azimuth", err)
	}
	calls := 0
	curve, err := p.Plot(func(theta float64) float64 {
		calls++
		if theta == .5 {
			return math.Inf(1)
		}
		return 1 + theta
	}, 0, 1, 9)
	if err != nil || calls != 9 || len(curve.Path.Contours) != 2 {
		t.Fatal("polar plot gaps", calls, err)
	}
	for _, contour := range curve.Path.Contours {
		if len(contour.Curves) != 3 {
			t.Fatal("polar plot bridged gap")
		}
	}
	before := curve.Path.Clone()
	p.Object.Shift(geom.V(2, 1))
	if !reflect.DeepEqual(curve.Path, before) {
		t.Fatal("polar plot aliases coordinate geometry")
	}
	if curve, err := p.Plot(func(float64) float64 { return -1 }, 0, 1, 3); curve != nil || err == nil {
		t.Fatal("negative radial plot accepted")
	}
}

func TestPolarPlaneValidation(t *testing.T) {
	var missing *graph.PolarPlane
	if _, err := missing.Point(1, 0); err == nil {
		t.Fatal("nil polar plane")
	}
	if _, _, err := missing.Coordinates(geom.V(0, 0)); err == nil || missing.Rays() != nil || missing.Circles() != nil {
		t.Fatal("nil polar inverse/selection")
	}
	valid := graph.DefaultPolarPlaneOptions()
	for _, mutate := range []func(*graph.PolarPlaneOptions){
		func(o *graph.PolarPlaneOptions) { o.Rays = 0 },
		func(o *graph.PolarPlaneOptions) { o.Rays = 4097 },
		func(o *graph.PolarPlaneOptions) { o.Offset = math.NaN() },
		func(o *graph.PolarPlaneOptions) { o.GridWidth = 0 },
	} {
		options := valid
		mutate(&options)
		if p, err := graph.NewPolarPlane(graph.Range{Min: 0, Max: 3, Step: 1}, 3, options); p != nil || err == nil {
			t.Fatal("invalid polar options accepted")
		}
	}
	for _, radius := range []graph.Range{{Min: -1, Max: 2, Step: 1}, {Min: 1, Max: 2, Step: 1}, {Min: 0, Max: 3, Step: 0}, {Min: 0, Max: 30000, Step: 1}} {
		if p, err := graph.NewPolarPlane(radius, 3, valid); p != nil || err == nil {
			t.Fatal("invalid radial range accepted")
		}
	}
	for _, world := range []float64{0, -1, math.Inf(1), math.MaxFloat64, math.SmallestNonzeroFloat64} {
		if p, err := graph.NewPolarPlane(graph.Range{Min: 0, Max: 3, Step: 1}, world, valid); p != nil || err == nil {
			t.Fatal("invalid physical radius", world)
		}
	}
	p, _ := graph.NewPolarPlane(graph.Range{Min: 0, Max: 3, Step: 1}, 3, valid)
	for _, pair := range [][2]float64{{-1, 0}, {math.NaN(), 0}, {1, math.Inf(1)}} {
		if _, err := p.Point(pair[0], pair[1]); err == nil {
			t.Fatal("invalid polar point")
		}
	}
	p.Object.ApplyMatrix(geom.Scaling(0, 1, 1))
	if _, err := p.Point(1, 0); err == nil {
		t.Fatal("collapsed polar frame")
	}
}

func BenchmarkPolarPlaneMapping(b *testing.B) {
	p, err := graph.NewPolarPlane(graph.Range{Min: 0, Max: 3, Step: 1}, 3, graph.DefaultPolarPlaneOptions())
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		point, err := p.Point(2, .7)
		if err != nil {
			b.Fatal(err)
		}
		if _, _, err = p.Coordinates(point); err != nil {
			b.Fatal(err)
		}
	}
}
