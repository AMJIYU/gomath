package path_test

import (
	"gomath/geom"
	"gomath/path"
	"reflect"
	"testing"
)

func TestTrimByLengthAcrossSegmentsAndContours(t *testing.T) {
	var b path.Builder
	b.MoveTo(geom.V(0, 0))
	b.LineTo(geom.V(1, 0))
	b.LineTo(geom.V(10, 0))
	b.MoveTo(geom.V(100, 20))
	b.LineTo(geom.V(104, 20))
	p := b.Path()
	m, err := p.Measure(1e-9)
	lengthMust(t, err)
	for _, interval := range [][2]float64{{0, 1}, {1, 10}, {.5, 8}, {5, 12}, {10, 14}, {0, 14}} {
		q, err := m.TrimByLength(interval[0], interval[1])
		lengthMust(t, err)
		lengthMust(t, q.Validate())
		gotLength, err := q.Length(1e-9)
		lengthMust(t, err)
		lengthNear(t, gotLength, interval[1]-interval[0], 1e-9)
		if interval[0] < 10 && interval[1] > 10 && len(q.Contours) != 2 {
			t.Fatal("trim connected disjoint contours")
		}
		first, err := q.AtLength(0, 1e-9)
		lengthMust(t, err)
		wantStart := geom.V(interval[0], 0)
		if interval[0] >= 10 {
			wantStart = geom.V(100+interval[0]-10, 20)
		}
		lengthPointNear(t, first.Point, wantStart, 1e-9)
		last, err := q.AtLength(gotLength, 1e-9)
		lengthMust(t, err)
		wantEnd := geom.V(interval[1], 0)
		if interval[1] > 10 {
			wantEnd = geom.V(100+interval[1]-10, 20)
		}
		lengthPointNear(t, last.Point, wantEnd, 1e-9)
	}
	q, err := p.TrimByLength(5, 12, 1e-9)
	lengthMust(t, err)
	q.Contours[0].Curves[0].P3 = geom.V(999, 999)
	if !reflect.DeepEqual(p, b.Path()) {
		t.Fatal("trim mutated authoring path")
	}
	again, err := m.TrimByLength(5, 12)
	lengthMust(t, err)
	if again.Contours[0].Curves[0].P3 == geom.V(999, 999) {
		t.Fatal("trim result aliases snapshot")
	}
	empty, err := m.TrimByLength(7, 7)
	lengthMust(t, err)
	if len(empty.Contours) != 0 {
		t.Fatal("zero-width trim is not empty")
	}
}

func TestTrimByLengthPreservesCurvedGeometry(t *testing.T) {
	c := path.Cubic{P0: geom.V(-2, 1), P1: geom.V(8, 12), P2: geom.V(-6, -10), P3: geom.V(4, 2)}
	m, err := cubicPath(c).Measure(1e-9)
	lengthMust(t, err)
	a, err := m.AtFraction(.13)
	lengthMust(t, err)
	b, err := m.AtFraction(.87)
	lengthMust(t, err)
	q, err := m.TrimByLength(a.Distance, b.Distance)
	lengthMust(t, err)
	lengthMust(t, q.Validate())
	if len(q.Contours) != 1 || len(q.Contours[0].Curves) != 1 || q.Contours[0].Closed {
		t.Fatal("curved trim structure wrong")
	}
	trimmed := q.Contours[0].Curves[0]
	for i := range 33 {
		u := float64(i) / 32
		lengthPointNear(t, trimmed.At(u), c.At(a.Parameter+(b.Parameter-a.Parameter)*u), 1e-12)
	}
	length, err := q.Length(1e-9)
	lengthMust(t, err)
	lengthNear(t, length, b.Distance-a.Distance, 1e-9)
}

func TestPartialByLengthClosureAndEndpoints(t *testing.T) {
	var b path.Builder
	b.MoveTo(geom.V(0, 0))
	b.LineTo(geom.V(3, 0))
	b.LineTo(geom.V(3, 4))
	b.Close()
	b.MoveTo(geom.V(100, 0))
	b.LineTo(geom.V(104, 0))
	p := b.Path()
	m, err := p.Measure(1e-9)
	lengthMust(t, err)
	for _, alpha := range []float64{0, .25, .5, .75, .9, 1} {
		q, err := m.PartialByLength(alpha)
		lengthMust(t, err)
		lengthMust(t, q.Validate())
		length, err := q.Length(1e-9)
		lengthMust(t, err)
		lengthNear(t, length, alpha*16, 1e-9)
		if len(q.Contours) > 0 && q.Contours[0].Closed != (alpha >= .75) {
			t.Fatal("partial closure wrong", alpha)
		}
		if alpha == 1 && !reflect.DeepEqual(q, p) {
			t.Fatal("full prefix changed authored contours")
		}
	}
	// A closed contour may rely on an implicit edge rather than Builder.Close.
	c := p.Contours[0]
	c.Curves = c.Curves[:2]
	implicit := path.Path{Contours: []path.Contour{c}}
	q, err := implicit.PartialByLength(9.5/12, 1e-9)
	lengthMust(t, err)
	if q.Contours[0].Closed || len(q.Contours[0].Curves) != 3 {
		t.Fatal("partial implicit closure not materialized as an open cubic")
	}
	lengthPointNear(t, q.Contours[0].Curves[2].P3, geom.V(1.5, 2), 1e-9)
	full, err := implicit.PartialByLength(1, 1e-9)
	lengthMust(t, err)
	if !reflect.DeepEqual(full, implicit) {
		t.Fatal("full implicit contour was rewritten")
	}
	// A proper subinterval stays open even if endpoints happen to coincide.
	cut, err := m.TrimByLength(1, 12)
	lengthMust(t, err)
	if cut.Contours[0].Closed {
		t.Fatal("trimmed contour incorrectly retained Closed")
	}
}

func TestLengthScaleAndSnapshotQueryAllocations(t *testing.T) {
	for _, scale := range []float64{1e-150, 1, 1e150} {
		c := path.Cubic{P1: geom.V(2*scale, 4*scale), P2: geom.V(-scale, -3*scale), P3: geom.V(scale, 0)}
		base := path.Cubic{P1: geom.V(2, 4), P2: geom.V(-1, -3), P3: geom.V(1, 0)}
		length, err := c.Length(1e-8 * scale)
		lengthMust(t, err)
		want, err := base.Length(1e-8)
		lengthMust(t, err)
		lengthNear(t, length/scale, want, 1e-8)
		m, err := cubicPath(c).Measure(1e-8 * scale)
		lengthMust(t, err)
		sample, err := m.AtFraction(.37)
		lengthMust(t, err)
		lengthNear(t, sample.Tangent.Norm(), 1, 1e-12)
	}
	c := path.Cubic{P1: geom.V(2, 4), P2: geom.V(-1, -3), P3: geom.V(1, 0)}
	m, err := cubicPath(c).Measure(1e-8)
	lengthMust(t, err)
	var queryErr error
	allocations := testing.AllocsPerRun(100, func() {
		_, queryErr = m.AtFraction(.37)
	})
	lengthMust(t, queryErr)
	if allocations != 0 {
		t.Fatalf("measured query allocated %g times", allocations)
	}
	allocations = testing.AllocsPerRun(100, func() {
		_, queryErr = c.Length(1e-8)
	})
	lengthMust(t, queryErr)
	if allocations != 0 {
		t.Fatalf("cubic length allocated %g times", allocations)
	}
}

func BenchmarkCubicLength(b *testing.B) {
	c := path.Cubic{P1: geom.V(12, 25), P2: geom.V(-18, -32), P3: geom.V(2, 1)}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := c.Length(1e-8); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPathMeasure(b *testing.B) {
	var builder path.Builder
	builder.MoveTo(geom.V(0, 0))
	for i := range 32 {
		x := float64(i)
		builder.CubicTo(geom.V(x+.2, 2), geom.V(x+.8, -2), geom.V(x+1, 0))
	}
	p := builder.Path()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := p.Measure(1e-6); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMeasuredPathAtFraction(b *testing.B) {
	c := path.Cubic{P1: geom.V(12, 25), P2: geom.V(-18, -32), P3: geom.V(2, 1)}
	m, err := cubicPath(c).Measure(1e-8)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		if _, err := m.AtFraction(float64(i%997) / 996); err != nil {
			b.Fatal(err)
		}
		i++
	}
}

func BenchmarkMeasuredPathResample(b *testing.B) {
	c := path.Cubic{P1: geom.V(12, 25), P2: geom.V(-18, -32), P3: geom.V(2, 1)}
	m, err := cubicPath(c).Measure(1e-8)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := m.Resample(101); err != nil {
			b.Fatal(err)
		}
	}
}
