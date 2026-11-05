package three

import (
	"fmt"
	"gomath/geom"
	"math"
	"reflect"
	"sync"
	"testing"
)

func segmentOptions() ArrowOptions {
	return ArrowOptions{ShaftRadius: .14, HeadRadius: .39, HeadLength: .65, RadialSegments: 11, ShaftSegments: 3, HeadSegments: 2}
}

func segmentClose(a, b, tolerance float64) bool { return math.Abs(a-b) <= tolerance }

func TestSegmentAnalyticGeometry(t *testing.T) {
	directions := []geom.Vec3{{Z: 1}, {Z: -1}, {X: 1}, {Y: -1}, {X: 1, Y: 2, Z: 3}, {X: -2, Y: 1, Z: -.0001}, {X: 1e-12, Z: 1}, {X: 1e-12, Z: -1}}
	for k, d := range directions {
		for _, size := range []float64{1e-5, 1, 1e10} {
			for _, arrow := range []bool{false, true} {
				t.Run(fmt.Sprintf("d%d/size%g/arrow%t", k, size, arrow), func(t *testing.T) {
					start := geom.Vec3{X: -1.1 * size, Y: .7 * size, Z: -.3 * size}
					end := start.Add(d.Mul(2.4 * size / d.Norm()))
					o := segmentOptions()
					o.ShaftRadius *= size
					o.HeadRadius *= size
					o.HeadLength *= size
					var got, want *Mesh
					var err error
					if arrow {
						got, err = ArrowBetween(start, end, o)
						want = segmentReference(start, end, 0, o.RadialSegments, 0, &o)
					} else {
						got, err = CylinderBetween(start, end, .39*size, 11, 3)
						want = segmentReference(start, end, .39*size, 11, 3, nil)
					}
					if err != nil {
						t.Fatal(err)
					}
					if len(got.Triangles) != len(want.Triangles) {
						t.Fatal("triangle counts", len(got.Triangles), len(want.Triangles))
					}
					for ti, tri := range got.Triangles {
						for j, index := range tri {
							ref := want.Triangles[ti][j]
							if got.Vertices[index].Sub(want.Vertices[ref]).Norm() > 3e-14*size || got.Normals[index].Sub(want.Normals[ref]).Norm() > 2e-14 || math.Abs(got.UVs[index].U-want.UVs[ref].U) > 2e-15 || math.Abs(got.UVs[index].V-want.UVs[ref].V) > 2e-15 {
								t.Fatalf("analytic triangle %d corner%d: pos%v/%v normal%v/%v UV%v/%v", ti, j, got.Vertices[index], want.Vertices[ref], got.Normals[index], want.Normals[ref], got.UVs[index], want.UVs[ref])
							}
						}
						a, b, c := got.Vertices[tri[0]], got.Vertices[tri[1]], got.Vertices[tri[2]]
						n := got.Normals[tri[0]].Add(got.Normals[tri[1]]).Add(got.Normals[tri[2]])
						if b.Sub(a).Cross(c.Sub(a)).Dot(n) <= 0 {
							t.Fatal("inward winding", ti)
						}
					}
					var volume float64
					for _, tri := range got.Triangles {
						a, b, c := got.Vertices[tri[0]].Sub(start), got.Vertices[tri[1]].Sub(start), got.Vertices[tri[2]].Sub(start)
						volume += a.Dot(b.Cross(c)) / 6
					}
					area := func(r float64) float64 {
						return float64(o.RadialSegments) / 2 * r * r * math.Sin(2*math.Pi/float64(o.RadialSegments))
					}
					expected := area(.39*size) * end.Sub(start).Norm()
					if arrow {
						expected = area(o.ShaftRadius)*(end.Sub(start).Norm()-o.HeadLength) + area(o.HeadRadius)*o.HeadLength/3
					}
					if math.Abs(volume/expected-1) > 2e-13 {
						t.Fatal("polygon volume", volume, expected)
					}
					segmentTopology(t, got, size)
				})
			}
		}
	}
}

func segmentTopology(t *testing.T, m *Mesh, size float64) {
	t.Helper()
	// Quantize only for test topology welding; production seam positions are
	// checked separately for exact equality below. This tolerates equivalent
	// analytic cap/join arithmetic at small and huge authored world scales.
	type key [3]int64
	vertices := map[key]int{}
	ids := make([]int, len(m.Vertices))
	for i, p := range m.Vertices {
		k := key{int64(math.Round(p.X / size * 1e12)), int64(math.Round(p.Y / size * 1e12)), int64(math.Round(p.Z / size * 1e12))}
		id, ok := vertices[k]
		if !ok {
			id = len(vertices)
			vertices[k] = id
		}
		ids[i] = id
	}
	type edge struct{ a, b int }
	type use struct{ count, balance int }
	edges := map[edge]use{}
	for _, tri := range m.Triangles {
		for j := range 3 {
			a, b := ids[tri[j]], ids[tri[(j+1)%3]]
			sign := 1
			if a > b {
				a, b = b, a
				sign = -1
			}
			e := edge{a, b}
			v := edges[e]
			v.count++
			v.balance += sign
			edges[e] = v
		}
	}
	for e, v := range edges {
		if e.a == e.b || v.count != 2 || v.balance != 0 {
			t.Fatal("not a closed outward manifold", e, v)
		}
	}
	if len(vertices)-len(edges)+len(m.Triangles) != 2 {
		t.Fatal("Euler", len(vertices), len(edges), len(m.Triangles))
	}
}

func TestSegmentBoundsAndBudgets(t *testing.T) {
	start, end := geom.Vec3{}, geom.Vec3{Z: 2}
	for _, bad := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), 0, -1, math.SmallestNonzeroFloat64, math.Nextafter(1e-6, 0), MaxCoordinate + 1} {
		if m, e := CylinderBetween(start, end, bad, 12, 2); m != nil || e == nil {
			t.Fatal("radius accepted", bad)
		}
		for field := range 3 {
			o := segmentOptions()
			switch field {
			case 0:
				o.ShaftRadius = bad
			case 1:
				o.HeadRadius = bad
			case 2:
				o.HeadLength = bad
			}
			if m, e := ArrowBetween(start, end, o); m != nil || e == nil {
				t.Fatal("arrow dimension accepted", field, bad)
			}
		}
	}
	for _, bad := range []int{-1, 0, 1, 2, 257, int(^uint(0) >> 1)} {
		if m, e := CylinderBetween(start, end, .2, bad, 1); m != nil || e == nil {
			t.Fatal("radial accepted", bad)
		}
	}
	for _, bad := range []int{-1, 0, 257, int(^uint(0) >> 1)} {
		if m, e := CylinderBetween(start, end, .2, 12, bad); m != nil || e == nil {
			t.Fatal("axial accepted", bad)
		}
		for field := range 3 {
			o := segmentOptions()
			switch field {
			case 0:
				o.RadialSegments = bad
			case 1:
				o.ShaftSegments = bad
			case 2:
				o.HeadSegments = bad
			}
			if m, e := ArrowBetween(start, end, o); m != nil || e == nil {
				t.Fatal("arrow segments accepted", field, bad)
			}
		}
	}
	for _, bad := range []geom.Vec3{{X: math.NaN()}, {Y: math.Inf(1)}, {Z: MaxCoordinate + 1}, start, {Z: math.Nextafter(1e-6, 0)}} {
		if m, e := CylinderBetween(start, bad, .2, 12, 1); m != nil || e == nil {
			t.Fatal("end accepted", bad)
		}
		if m, e := ArrowBetween(start, bad, segmentOptions()); m != nil || e == nil {
			t.Fatal("arrow end accepted", bad)
		}
		if !bad.Finite() || !bounded(bad) {
			if m, e := CylinderBetween(bad, end, .2, 12, 1); m != nil || e == nil {
				t.Fatal("start accepted", bad)
			}
		}
	}
	for _, change := range []func(*ArrowOptions){func(o *ArrowOptions) { o.HeadRadius = o.ShaftRadius }, func(o *ArrowOptions) { o.HeadRadius = o.ShaftRadius + 1e-7 }, func(o *ArrowOptions) { o.HeadLength = 2 }, func(o *ArrowOptions) { o.HeadLength = 3 }, func(o *ArrowOptions) { o.HeadLength = 2 - 1e-7 }} {
		o := segmentOptions()
		change(&o)
		if m, e := ArrowBetween(start, end, o); m != nil || e == nil {
			t.Fatal("invalid join accepted", o)
		}
	}
	for _, pair := range [][2]geom.Vec3{{{X: MaxCoordinate}, {X: MaxCoordinate, Z: 2}}, {{X: 1e10}, {X: 1e10, Z: 2}}} {
		if m, e := CylinderBetween(pair[0], pair[1], 1e-6, 256, 1); m != nil || e == nil {
			t.Fatal("unrepresentable radial detail accepted")
		}
	}
	thinCone := ArrowOptions{ShaftRadius: .002, HeadRadius: .01, HeadLength: .8, RadialSegments: 256, ShaftSegments: 1, HeadSegments: 256}
	if m, e := ArrowBetween(geom.Vec3{X: 1e8}, geom.Vec3{X: 1e8, Z: 2}, thinCone); m != nil || e == nil {
		t.Fatal("unrepresentable final cone ring accepted")
	}
	thinJoin := ArrowOptions{ShaftRadius: .1, HeadRadius: .10001, HeadLength: .8, RadialSegments: 12, ShaftSegments: 1, HeadSegments: 1}
	if m, e := ArrowBetween(geom.Vec3{X: 1e10}, geom.Vec3{X: 1e10, Z: 2}, thinJoin); m != nil || e == nil {
		t.Fatal("unrepresentable annular join accepted")
	}
	if m, e := CylinderBetween(geom.Vec3{X: MaxCoordinate}, geom.Vec3{X: MaxCoordinate, Z: 2}, 1e8, 12, 1); m != nil || e == nil {
		t.Fatal("generated out-of-bounds positions accepted")
	}
	if _, e := CylinderBetween(start, geom.Vec3{Z: 1e-6}, 1e-6, 3, 1); e != nil {
		t.Fatal("minimum cylinder", e)
	}
	if _, e := CylinderBetween(geom.Vec3{Z: -MaxCoordinate}, geom.Vec3{Z: MaxCoordinate}, 1e9, 3, 1); e != nil {
		t.Fatal("long diagonal must not inherit centered Cylinder height restriction", e)
	}
	if _, e := ArrowBetween(start, geom.Vec3{Z: 2e-6}, ArrowOptions{ShaftRadius: 1e-6, HeadRadius: 2e-6, HeadLength: 1e-6, RadialSegments: 3, ShaftSegments: 1, HeadSegments: 1}); e != nil {
		t.Fatal("minimum arrow", e)
	}
	m, e := CylinderBetween(start, end, .2, 256, 256)
	if e != nil {
		t.Fatal(e)
	}
	if len(m.Vertices) != 66563 || len(m.Triangles) != 131584 {
		t.Fatal("max cylinder counts", len(m.Vertices), len(m.Triangles))
	}
	o := segmentOptions()
	o.RadialSegments = 256
	o.ShaftSegments = 256
	o.HeadSegments = 256
	m, e = ArrowBetween(start, end, o)
	if e != nil {
		t.Fatal(e)
	}
	if len(m.Vertices) != 132868 || len(m.Triangles) != 262656 {
		t.Fatal("max arrow counts", len(m.Vertices), len(m.Triangles))
	}
}

func TestSegmentSeamsAndOwnership(t *testing.T) {
	start, end := geom.Vec3{X: -.3, Y: .2, Z: -1}, geom.Vec3{X: .7, Y: -.4, Z: 1.2}
	o := segmentOptions()
	want, e := ArrowBetween(start, end, o)
	if e != nil {
		t.Fatal(e)
	}
	shaftVertices := (o.ShaftSegments + 1) * (o.RadialSegments + 1)
	tipBase := shaftVertices + o.HeadSegments*(o.RadialSegments+1)
	for i := range o.RadialSegments {
		if want.Vertices[tipBase+i] != end {
			t.Fatal("tip endpoint must be exact")
		}
	}
	tailBase := tipBase + o.RadialSegments
	if want.Vertices[tailBase] != start {
		t.Fatal("tail cap center must be exact")
	}
	shoulderBase := tailBase + o.RadialSegments + 1
	shaftEnd := o.ShaftSegments * (o.RadialSegments + 1)
	for i := range o.RadialSegments + 1 {
		inner, outer := shoulderBase+i, shoulderBase+o.RadialSegments+1+i
		if want.Vertices[inner] != want.Vertices[shaftEnd+i] || want.Vertices[outer] != want.Vertices[shaftVertices+i] {
			t.Fatalf("join must be exactly shared in position %d inner=%#v shaft=%#v outer=%#v cone=%#v", i, want.Vertices[inner], want.Vertices[shaftEnd+i], want.Vertices[outer], want.Vertices[shaftVertices+i])
		}
		if want.Normals[inner] == want.Normals[shaftEnd+i] || want.Normals[outer] == want.Normals[shaftVertices+i] {
			t.Fatal("join must retain hard normal split")
		}
	}
	cylinder, err := CylinderBetween(start, end, .2, 11, 3)
	if err != nil {
		t.Fatal(err)
	}
	if cylinder.Vertices[4*12] != start || cylinder.Vertices[4*12+12] != end {
		t.Fatal("cylinder cap centers must be exact endpoints")
	}
	for i := range 11 {
		if cylinder.Vertices[4*12+1+i] != cylinder.Vertices[i] || cylinder.Vertices[4*12+12+1+i] != cylinder.Vertices[3*12+i] || want.Vertices[tailBase+1+i] != want.Vertices[i] {
			t.Fatal("hard caps must copy exact side boundary positions")
		}
	}
	for _, ring := range []struct{ base, rings int }{{0, o.ShaftSegments + 1}, {(o.ShaftSegments + 1) * (o.RadialSegments + 1), o.HeadSegments}} {
		for j := 0; j < ring.rings; j++ {
			a := ring.base + j*(o.RadialSegments+1)
			b := a + o.RadialSegments
			if want.Vertices[a] != want.Vertices[b] || want.Normals[a] != want.Normals[b] || want.UVs[a].U != 0 || want.UVs[b].U != 1 {
				t.Fatal("seam not exact", a, b)
			}
		}
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 4 {
				m, e := ArrowBetween(start, end, o)
				if e != nil || !reflect.DeepEqual(want, m) {
					t.Errorf("concurrent constructor mismatch: %v", e)
					return
				}
				m.Vertices[0] = geom.Vec3{}
				m.Normals[0] = geom.Vec3{}
				m.UVs[0] = UV{}
				m.Triangles[0] = Triangle{}
			}
		}()
	}
	wg.Wait()
	for _, n := range want.Normals {
		if !segmentClose(n.Norm(), 1, 2e-14) {
			t.Fatal("nonunit normal", n)
		}
	}
}
