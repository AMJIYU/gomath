package path_test

import (
	"errors"
	"gomath"
	"gomath/geom"
	"gomath/path"
	"math"
	"reflect"
	"sync"
	"testing"
)

func lengthNear(t *testing.T, got, want, tolerance float64) {
	t.Helper()
	if math.IsNaN(got) || math.Abs(got-want) > tolerance {
		t.Fatalf("got %.17g want %.17g (tolerance %g)", got, want, tolerance)
	}
}
func lengthPointNear(t *testing.T, got, want geom.Vec3, tolerance float64) {
	t.Helper()
	if !got.Finite() || math.Hypot(math.Hypot(got.X-want.X, got.Y-want.Y), got.Z-want.Z) > tolerance {
		t.Fatalf("point %+v want %+v (tolerance %g)", got, want, tolerance)
	}
}
func lengthMust(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func cubicPath(c path.Cubic) path.Path {
	return path.Path{Contours: []path.Contour{{Start: c.P0, Curves: []path.Cubic{c}}}}
}

func TestCubicLengthStraightDegenerateReversed(t *testing.T) {
	a, b := geom.Vec3{X: 1, Y: -2, Z: 3}, geom.Vec3{X: 3, Y: 1, Z: 9}
	c := path.Line(a, b)
	want := 7.0
	for _, curve := range []path.Cubic{c, {P0: c.P3, P1: c.P2, P2: c.P1, P3: c.P0}} {
		length, err := curve.Length(1e-10)
		lengthMust(t, err)
		lengthNear(t, length, want, 1e-12)
		for _, fraction := range []float64{0, .01, .25, .5, .9, 1} {
			sample, err := curve.AtLength(fraction*length, 1e-10)
			lengthMust(t, err)
			lengthPointNear(t, sample.Point, curve.P0.Lerp(curve.P3, fraction), 1e-10)
			lengthPointNear(t, sample.Tangent, curve.P3.Sub(curve.P0).Mul(1/want), 1e-12)
			lengthNear(t, sample.Parameter, fraction, 1e-10)
		}
	}
	constant := path.Cubic{P0: a, P1: a, P2: a, P3: a}
	length, err := constant.Length(1e-10)
	lengthMust(t, err)
	lengthNear(t, length, 0, 0)
	sample, err := constant.AtLength(0, 1e-10)
	lengthMust(t, err)
	if sample.Point != a || sample.Tangent != (geom.Vec3{}) || sample.Parameter != 0 {
		t.Fatal("constant curve sample", sample)
	}
	if _, err := constant.AtLength(.01, 1e-10); err == nil {
		t.Fatal("distance beyond constant curve accepted")
	}
}

func TestCubicInverseNonuniformParameterAndEndpointTangents(t *testing.T) {
	// x=10*t^3: equal parameter steps are very unequal distances.
	c := path.Cubic{P3: geom.V(10, 0)}
	for _, distance := range []float64{0, .00001, .1, 1, 2, 5, 9.9, 10} {
		sample, err := c.AtLength(distance, 1e-9)
		lengthMust(t, err)
		lengthNear(t, sample.Parameter, math.Cbrt(distance/10), 2e-6)
		lengthPointNear(t, sample.Point, geom.V(distance, 0), 1e-9)
		lengthPointNear(t, sample.Tangent, geom.V(1, 0), 1e-14)
		u, err := c.ParameterAtLength(distance, 1e-9)
		lengthMust(t, err)
		lengthNear(t, u, sample.Parameter, 0)
	}
	// Both endpoint derivatives vanish; the limiting tangents still exist.
	c = path.Cubic{P2: geom.V(10, 0), P3: geom.V(10, 0)}
	for _, distance := range []float64{0, 10} {
		sample, err := c.AtLength(distance, 1e-9)
		lengthMust(t, err)
		lengthPointNear(t, sample.Tangent, geom.V(1, 0), 0)
	}
}

func TestCubicLengthDoubleBackAndStationaryInterior(t *testing.T) {
	c := path.Cubic{P1: geom.V(1, 0), P2: geom.V(-1, 0)}
	length, err := c.Length(1e-10)
	lengthMust(t, err)
	lengthNear(t, length, 2/math.Sqrt(3), 1e-10)
	m, err := cubicPath(c).Measure(1e-9)
	lengthMust(t, err)
	previous := 0.0
	for i := range 17 {
		sample, err := m.AtFraction(float64(i) / 16)
		if err != nil {
			t.Fatalf("sample %d: %v", i, err)
		}
		if sample.Parameter < previous {
			t.Fatal("inverse parameter reversed on a retracing curve")
		}
		previous = sample.Parameter
		prefix, err := c.LengthBetween(0, sample.Parameter, 1e-10)
		lengthMust(t, err)
		lengthNear(t, prefix, sample.Distance, 1e-9)
	}
	// x=(t-.5)^2 has a reversal at t=.5 and no unique tangent there.
	c = path.Cubic{P0: geom.V(.25, 0), P1: geom.V(-1.0/12, 0), P2: geom.V(-1.0/12, 0), P3: geom.V(.25, 0)}
	sample, err := c.AtLength(.25, 1e-10)
	lengthMust(t, err)
	lengthPointNear(t, sample.Point, geom.V(0, 0), 1e-12)
	lengthPointNear(t, sample.Tangent, geom.V(0, 0), 0)
}

// referenceLength is an independent fixed-grid composite Simpson integral.
// It differentiates in Bernstein form directly, rather than using the
// production de Casteljau derivative or embedded Gauss/Kronrod rules.
func referenceLength(c path.Cubic, end float64) float64 {
	const intervals = 1 << 18
	h := end / intervals
	var sum, correction float64
	for i := 0; i <= intervals; i++ {
		u, t := 1-float64(i)*h, float64(i)*h
		d0, d1, d2 := c.P1.Sub(c.P0), c.P2.Sub(c.P1), c.P3.Sub(c.P2)
		d := geom.Vec3{
			X: 3 * (u*u*d0.X + 2*u*t*d1.X + t*t*d2.X),
			Y: 3 * (u*u*d0.Y + 2*u*t*d1.Y + t*t*d2.Y),
			Z: 3 * (u*u*d0.Z + 2*u*t*d1.Z + t*t*d2.Z),
		}
		weight := 2.0
		if i == 0 || i == intervals {
			weight = 1
		} else if i%2 == 1 {
			weight = 4
		}
		value := weight*math.Hypot(math.Hypot(d.X, d.Y), d.Z) - correction
		next := sum + value
		correction, sum = (next-sum)-value, next
	}
	return sum * h / 3
}

func TestCubicLengthAgainstIndependentIntegral(t *testing.T) {
	curves := []path.Cubic{
		{P0: geom.V(0, 0), P1: geom.V(12, 25), P2: geom.V(-18, -32), P3: geom.V(2, 1)},
		{P0: geom.V(0, 0), P1: geom.V(20, 0), P2: geom.V(-20, .00001), P3: geom.V(.000001, 0)},
		{P0: geom.V(1, 1), P1: geom.V(-8, 20), P2: geom.V(20, 20), P3: geom.V(1, 1)},
		{P0: geom.Vec3{X: 1, Y: -2, Z: 4}, P1: geom.Vec3{X: -10, Y: 25, Z: 8}, P2: geom.Vec3{X: 20, Y: -16, Z: -12}, P3: geom.Vec3{X: 7, Y: 3, Z: -1}},
	}
	for _, c := range curves {
		got, err := c.Length(1e-8)
		lengthMust(t, err)
		lengthNear(t, got, referenceLength(c, 1), 1e-8)
		reversed := path.Cubic{P0: c.P3, P1: c.P2, P2: c.P1, P3: c.P0}
		reverseLength, err := reversed.Length(1e-8)
		lengthMust(t, err)
		lengthNear(t, reverseLength, got, 1e-8)
		m, err := cubicPath(c).Measure(1e-8)
		lengthMust(t, err)
		for _, fraction := range []float64{.03, .2, .5, .83, .99} {
			sample, err := m.AtFraction(fraction)
			lengthMust(t, err)
			lengthNear(t, referenceLength(c, sample.Parameter), sample.Distance, 1e-8)
			lengthPointNear(t, sample.Point, c.At(sample.Parameter), 1e-12)
			lengthNear(t, sample.Tangent.Norm(), 1, 1e-12)
		}
	}
}

func TestCircleCubicLength(t *testing.T) {
	// These four cubics approximate a circle. Their actual length differs
	// from 2*pi*radius by about 0.000881485*radius; do not mistake modeling error
	// for a quadrature error or assert an exact-circle capability.
	for _, radius := range []float64{.1, 1, 10} {
		p := gomath.Circle(radius).Path
		length, err := p.Length(1e-9)
		lengthMust(t, err)
		lengthNear(t, length, 6.284066792295423*radius, 1e-9)
		lengthNear(t, length, 2*math.Pi*radius, .000882*radius)
		contourLength, err := p.Contours[0].Length(1e-9)
		lengthMust(t, err)
		lengthNear(t, contourLength, length, 0)
	}
}

func TestUniformLengthSamplingUnequalSegmentsAndContourBoundaries(t *testing.T) {
	var b path.Builder
	b.MoveTo(geom.V(0, 0))
	b.LineTo(geom.V(1, 0))
	b.LineTo(geom.V(10, 0))
	p := b.Path()
	samples, err := p.Resample(11, 1e-9)
	lengthMust(t, err)
	for i, sample := range samples {
		lengthPointNear(t, sample.Point, geom.V(float64(i), 0), 1e-9)
		lengthNear(t, sample.Distance, float64(i), 1e-12)
	}
	// The old parameter-based Partial intentionally reaches x=1 halfway.
	if p.Partial(.5).Contours[0].Curves[0].P3.X != 1 {
		t.Fatal("existing Partial semantics changed")
	}
	q, err := p.PartialByLength(.5, 1e-9)
	lengthMust(t, err)
	lengthNear(t, q.Contours[0].Curves[1].P3.X, 5, 1e-9)

	b.MoveTo(geom.V(100, 20))
	b.LineTo(geom.V(102, 20))
	m, err := b.Path().Measure(1e-9)
	lengthMust(t, err)
	lengthNear(t, m.Length(), 12, 1e-12)
	boundary, err := m.AtLength(10)
	lengthMust(t, err)
	if boundary.Contour != 0 || boundary.Curve != 1 || boundary.Parameter != 1 || boundary.Point != geom.V(10, 0) {
		t.Fatal("boundary did not choose incoming curve", boundary)
	}
	next, err := m.AtLength(10.5)
	lengthMust(t, err)
	if next.Contour != 1 {
		t.Fatal("contour jump lost", next)
	}
	lengthPointNear(t, next.Point, geom.V(100.5, 20), 1e-9)
	end, err := m.AtFraction(1)
	lengthMust(t, err)
	lengthPointNear(t, end.Point, geom.V(102, 20), 0)
}

func TestLengthClosedImplicitEdgeAndStationaryContours(t *testing.T) {
	c := path.Contour{Start: geom.V(0, 0), Curves: []path.Cubic{path.Line(geom.V(0, 0), geom.V(3, 0)), path.Line(geom.V(3, 0), geom.V(3, 4))}, Closed: true}
	length, err := c.Length(1e-9)
	lengthMust(t, err)
	lengthNear(t, length, 12, 1e-12)
	sample, err := c.AtLength(9.5, 1e-9)
	lengthMust(t, err)
	if sample.Curve != len(c.Curves) {
		t.Fatal("implicit closing edge index missing", sample)
	}
	lengthPointNear(t, sample.Point, geom.V(1.5, 2), 1e-9)
	lengthPointNear(t, sample.Tangent, geom.V(-.6, -.8), 1e-12)
	samples, err := c.Resample(5, 1e-9)
	lengthMust(t, err)
	lengthPointNear(t, samples[0].Point, samples[4].Point, 0)
	if len(c.Curves) != 2 {
		t.Fatal("measuring materialized closure in input")
	}

	p := path.Path{Contours: []path.Contour{{Start: geom.V(-100, -100)}, {Start: geom.V(3, 4), Curves: []path.Cubic{path.Line(geom.V(3, 4), geom.V(3, 4)), path.Line(geom.V(3, 4), geom.V(6, 4))}}, {Start: geom.V(100, 100)}}}
	m, err := p.Measure(1e-9)
	lengthMust(t, err)
	start, err := m.AtLength(0)
	lengthMust(t, err)
	if start.Contour != 1 || start.Curve != 1 || start.Point != geom.V(3, 4) {
		t.Fatal("zero-length prefixes were not skipped", start)
	}
	end, err := m.AtFraction(1)
	lengthMust(t, err)
	if end.Contour != 1 || end.Point != geom.V(6, 4) {
		t.Fatal("zero-length suffix changed endpoint", end)
	}
	stationary := path.Path{Contours: []path.Contour{{Start: geom.V(7, 8)}, {Start: geom.V(20, 30)}}}
	m, err = stationary.Measure(1e-9)
	lengthMust(t, err)
	samples, err = m.Resample(3)
	lengthMust(t, err)
	for _, s := range samples {
		if s.Point != geom.V(7, 8) || s.Curve != -1 || s.Tangent != (geom.Vec3{}) {
			t.Fatal("stationary path contract broken", s)
		}
	}
}

func TestMeasuredPathSnapshotAndConcurrentQueries(t *testing.T) {
	p := gomath.Circle(2).Path
	m, err := p.Measure(1e-8)
	lengthMust(t, err)
	want, err := m.Resample(17)
	lengthMust(t, err)
	p.Contours[0].Curves[0].P0 = geom.V(100, 100)
	p.Contours[0].Start = geom.V(200, 200)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := m.Resample(17)
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Error("measured snapshot mutated or concurrent queries differ", err)
			}
		}()
	}
	wg.Wait()
}

func TestLengthRejectsInvalidAndUnachievableInputs(t *testing.T) {
	c := path.Cubic{P1: geom.V(2, 4), P2: geom.V(-1, -3), P3: geom.V(1, 0)}
	p := cubicPath(c)
	for _, tolerance := range []float64{0, -1, math.NaN(), math.Inf(1), math.Inf(-1)} {
		if _, err := c.Length(tolerance); err == nil {
			t.Error("invalid cubic tolerance accepted", tolerance)
		}
		if _, err := c.AtLength(0, tolerance); err == nil {
			t.Error("invalid inverse tolerance accepted", tolerance)
		}
		if _, err := p.Length(tolerance); err == nil {
			t.Error("invalid path tolerance accepted", tolerance)
		}
		if _, err := p.Measure(tolerance); err == nil {
			t.Error("invalid measure tolerance accepted", tolerance)
		}
		if _, err := p.PartialByLength(.5, tolerance); err == nil {
			t.Error("invalid partial tolerance accepted", tolerance)
		}
		if _, err := p.TrimByLength(0, 1, tolerance); err == nil {
			t.Error("invalid trim tolerance accepted", tolerance)
		}
	}
	for _, controls := range []path.Cubic{
		{P1: geom.V(math.NaN(), 0)}, {P3: geom.V(math.Inf(1), 0)},
		{P0: geom.V(-math.MaxFloat64, 0), P1: geom.V(math.MaxFloat64, 0)},
	} {
		if _, err := controls.Length(1e-8); err == nil {
			t.Error("invalid cubic accepted", controls)
		}
		if _, err := cubicPath(controls).Measure(1e-8); err == nil {
			t.Error("invalid path accepted", controls)
		}
	}
	disconnected := cubicPath(c)
	disconnected.Contours[0].Start = geom.V(10, 10)
	if _, err := disconnected.Measure(1e-8); err == nil {
		t.Error("disconnected contour accepted")
	}
	if _, err := c.Length(1e-30); !errors.Is(err, path.ErrLengthConvergence) {
		t.Fatal("unachievable tolerance did not return convergence error", err)
	}
	m, err := p.Measure(1e-8)
	lengthMust(t, err)
	for _, distance := range []float64{-1, m.Length() + 1, math.NaN(), math.Inf(1)} {
		if _, err := m.AtLength(distance); err == nil {
			t.Error("invalid distance accepted", distance)
		}
		if _, err := c.AtLength(distance, 1e-8); err == nil {
			t.Error("invalid cubic distance accepted", distance)
		}
	}
	for _, fraction := range []float64{-.01, 1.01, math.NaN(), math.Inf(1)} {
		if _, err := m.AtFraction(fraction); err == nil {
			t.Error("invalid fraction accepted", fraction)
		}
		if _, err := m.PartialByLength(fraction); err == nil {
			t.Error("invalid partial fraction accepted", fraction)
		}
	}
	for _, count := range []int{-1, 0, 1, path.MaxLengthSamples + 1} {
		if _, err := m.Resample(count); err == nil {
			t.Error("invalid sample count accepted", count)
		}
	}
	for _, interval := range [][2]float64{{-.1, 1}, {1, .5}, {0, m.Length() + 1}, {math.NaN(), 1}, {0, math.Inf(1)}} {
		if _, err := m.TrimByLength(interval[0], interval[1]); err == nil {
			t.Error("invalid trim interval accepted", interval)
		}
	}
	for _, interval := range [][2]float64{{-.1, 1}, {1, .5}, {0, 1.1}, {math.NaN(), 1}} {
		if _, err := c.LengthBetween(interval[0], interval[1], 1e-8); err == nil {
			t.Error("invalid parameter interval accepted", interval)
		}
	}
	empty, err := (path.Path{}).Measure(1e-8)
	lengthMust(t, err)
	lengthNear(t, empty.Length(), 0, 0)
	if _, err := empty.AtLength(0); !errors.Is(err, path.ErrEmptyPath) {
		t.Fatal("empty path sample did not fail", err)
	}
	if _, err := empty.Resample(2); !errors.Is(err, path.ErrEmptyPath) {
		t.Fatal("empty resample did not fail", err)
	}
	var zero path.ArcLength
	if _, err := zero.AtFraction(0); !errors.Is(err, path.ErrEmptyPath) {
		t.Fatal("zero snapshot did not fail", err)
	}
}
