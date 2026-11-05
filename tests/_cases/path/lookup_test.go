package path

import (
	"gomath/geom"
	"math"
	"testing"
)

func TestLookupBudgetAndIntervalOwnership(t *testing.T) {
	var b Builder
	b.MoveTo(geom.V(0, 0))
	for i := range 8 {
		x := float64(i)
		b.CubicTo(geom.V(x+12, 25), geom.V(x-18, -32), geom.V(x+1, 0))
	}
	p := b.Path()
	for _, limit := range []int{0, 1, 2, 3, 17, 31, 32, 33, 64, 65536} {
		m, err := p.MeasureWithOptions(1e-7, MeasureOptions{MaxLookupSamples: limit})
		if err != nil {
			t.Fatal(err)
		}
		if len(m.lookup) > limit || cap(m.lookup) > limit {
			t.Fatal("lookup storage exceeds configured budget", limit, len(m.lookup), cap(m.lookup))
		}
		end := 0
		for _, c := range m.curves {
			if c.lookupCount == 0 {
				continue
			}
			if c.lookupCount < 2 || c.lookupStart != end {
				t.Fatal("partial/shared lookup interval escaped")
			}
			knots := m.lookup[c.lookupStart : c.lookupStart+c.lookupCount]
			if knots[0] != (lengthKnot{}) || knots[len(knots)-1].parameter != 1 || knots[len(knots)-1].length != c.length {
				t.Fatal("invalid lookup endpoints")
			}
			for i := 1; i < len(knots); i++ {
				if knots[i].parameter <= knots[i-1].parameter || knots[i].length < knots[i-1].length {
					t.Fatal("lookup not monotone")
				}
			}
			end += c.lookupCount
		}
		if end != len(m.lookup) {
			t.Fatal("abandoned partial cache retained")
		}
		for i := range 101 {
			s, err := m.AtFraction(float64(i) / 100)
			if err != nil || !s.Point.Finite() {
				t.Fatal("budget changed query correctness", limit, i, err)
			}
		}
	}
	for _, limit := range []int{-1, MaxLengthLookupSamples + 1} {
		if m, err := p.MeasureWithOptions(1e-7, MeasureOptions{MaxLookupSamples: limit}); err == nil || m != nil {
			t.Fatal("invalid lookup budget accepted")
		}
	}
}

func TestLookupAnalyticStationaryCubicAndContourJumps(t *testing.T) {
	// x=8t³ has a stationary start and exact analytic inverse. Its curve is
	// followed by an isolated contour jump, which must still add no length.
	p := Path{Contours: []Contour{{Curves: []Cubic{{P3: geom.V(8, 0)}}}, {Start: geom.V(100, 0), Curves: []Cubic{Line(geom.V(100, 0), geom.V(101, 0))}}}}
	for _, limit := range []int{0, 1, 2, 3, 64} {
		m, err := p.MeasureWithOptions(1e-9, MeasureOptions{MaxLookupSamples: limit})
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range []float64{0, 1e-7, .1, 1, 3, 7.99, 8} {
			s, err := m.AtLength(d)
			if err != nil || math.Abs(s.Point.X-d) > 1e-9 || math.Abs(s.Parameter-math.Cbrt(d/8)) > 1e-6 || s.Contour != 0 {
				t.Fatal("analytic inverse or incoming boundary differs", limit, d, s, err)
			}
		}
		s, err := m.AtLength(8.5)
		if err != nil || s.Contour != 1 || math.Abs(s.Point.X-100.5) > 1e-9 {
			t.Fatal("contour gap acquired length", limit, s, err)
		}
		if allocations := testing.AllocsPerRun(100, func() {
			if _, err := m.AtLength(3.2); err != nil {
				panic(err)
			}
		}); allocations != 0 {
			t.Fatal("successful query allocated", limit, allocations)
		}
	}
}
