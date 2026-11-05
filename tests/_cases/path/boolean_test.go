package path_test

import (
	"fmt"
	"gomath/geom"
	"gomath/path"
	"math"
	"math/rand"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func boolPolygon(points ...geom.Vec3) path.Path {
	var b path.Builder
	b.MoveTo(points[0])
	for _, v := range points[1:] {
		b.LineTo(v)
	}
	b.Close()
	return b.Path()
}
func boolRect(x0, y0, x1, y1 float64) path.Path {
	return boolPolygon(geom.V(x0, y0), geom.V(x1, y0), geom.V(x1, y1), geom.V(x0, y1))
}
func boolReverse(p path.Path) path.Path {
	q := p.Clone()
	for i, c := range p.Contours {
		last := c.Start
		if len(c.Curves) > 0 {
			last = c.Curves[len(c.Curves)-1].P3
		}
		q.Contours[i].Start = last
		for j, v := range c.Curves {
			q.Contours[i].Curves[len(c.Curves)-1-j] = path.Cubic{P0: v.P3, P1: v.P2, P2: v.P1, P3: v.P0}
		}
	}
	return q
}
func boolJoin(ps ...path.Path) path.Path {
	var p path.Path
	for _, q := range ps {
		p.Contours = append(p.Contours, q.Clone().Contours...)
	}
	return p
}
func boolArea(p path.Path) float64 {
	area := 0.0
	for _, c := range p.Contours {
		for _, s := range c.Curves {
			area += s.P0.X*s.P3.Y - s.P0.Y*s.P3.X
		}
	}
	return area / 2
}
func boolMembership(p path.Path, v geom.Vec3, rule path.FillRule) bool {
	w := 0
	for _, c := range p.Contours {
		prev := c.Start
		points := make([]geom.Vec3, 0, len(c.Curves)+1)
		for _, s := range c.Curves {
			points = append(points, s.P3)
		}
		if points[len(points)-1] != c.Start {
			points = append(points, c.Start)
		}
		for _, q := range points {
			cross := (q.X-prev.X)*(v.Y-prev.Y) - (q.Y-prev.Y)*(v.X-prev.X)
			if prev.Y <= v.Y && q.Y > v.Y && cross > 0 {
				w++
			}
			if prev.Y > v.Y && q.Y <= v.Y && cross < 0 {
				w--
			}
			prev = q
		}
	}
	if rule == path.EvenOdd {
		return w%2 != 0
	}
	return w != 0
}
func boolCombine(a, b bool, op path.BooleanOp) bool {
	switch op {
	case path.BooleanUnion:
		return a || b
	case path.BooleanIntersection:
		return a && b
	case path.BooleanDifference:
		return a && !b
	default:
		return a != b
	}
}
func boolCheckGrid(t *testing.T, a, b, got path.Path, op path.BooleanOp, ra, rb path.FillRule) {
	t.Helper()
	for y := -2; y < 13; y++ {
		for x := -2; x < 13; x++ {
			p := geom.V(float64(x)+.137, float64(y)+.331)
			want := boolCombine(boolMembership(a, p, ra), boolMembership(b, p, rb), op)
			if actual := boolMembership(got, p, path.NonZero); actual != want {
				t.Fatalf("op %d at %v got %v want %v", op, p, actual, want)
			}
			if boolMembership(got, p, path.EvenOdd) != want {
				t.Fatal("canonical output differs under EvenOdd", p)
			}
		}
	}
}

func TestBooleanAreaContactsCollinearHolesAndEmpty(t *testing.T) {
	a := boolRect(0, 0, 4, 3)
	for _, tc := range []struct {
		name  string
		a, b  path.Path
		areas [4]float64
	}{
		{"overlap", a, boolRect(2, 1, 5, 4), [4]float64{17, 4, 8, 13}},
		{"disjoint", a, boolRect(6, 1, 9, 4), [4]float64{21, 0, 12, 21}},
		{"identical", a, a, [4]float64{12, 12, 0, 0}},
		{"opposite orientation", a, boolReverse(a), [4]float64{12, 12, 0, 0}},
		{"edge contact", a, boolRect(4, 0, 6, 3), [4]float64{18, 0, 12, 18}},
		{"point contact", a, boolRect(4, 3, 6, 5), [4]float64{16, 0, 12, 16}},
		{"contained", a, boolRect(1, 1, 3, 2), [4]float64{12, 2, 10, 10}},
		{"partial collinear", a, boolRect(2, 0, 6, 2), [4]float64{16, 4, 8, 12}},
		{"hole", boolJoin(boolRect(0, 0, 10, 10), boolReverse(boolRect(2, 2, 8, 8))), boolRect(3, 3, 7, 7), [4]float64{80, 0, 64, 80}},
		{"empty left", path.Path{}, a, [4]float64{12, 0, 0, 12}},
		{"empty right", a, path.Path{}, [4]float64{12, 0, 12, 12}},
		{"both empty", path.Path{}, path.Path{}, [4]float64{}},
	} {
		for op := path.BooleanUnion; op <= path.BooleanXor; op++ {
			t.Run(tc.name+string(rune('0'+op)), func(t *testing.T) {
				beforeA, beforeB := tc.a.Clone(), tc.b.Clone()
				got, err := path.Boolean(tc.a, tc.b, op, path.BooleanOptions{})
				if err != nil {
					t.Fatal(err)
				}
				if math.Abs(boolArea(got)-tc.areas[op]) > 1e-9 {
					t.Fatalf("area=%g want %g: %+v", boolArea(got), tc.areas[op], got)
				}
				boolCheckGrid(t, tc.a, tc.b, got, op, path.NonZero, path.NonZero)
				if !reflect.DeepEqual(beforeA, tc.a.Clone()) || !reflect.DeepEqual(beforeB, tc.b.Clone()) {
					t.Fatal("mutated operands")
				}
				for _, c := range got.Contours {
					if !c.Closed || c.Curves[len(c.Curves)-1].P3 != c.Start {
						t.Fatal("open output")
					}
				}
			})
		}
	}
}

func TestBooleanFillRulesSelfCrossingAndCancellation(t *testing.T) {
	r := boolRect(0, 0, 8, 8)
	for _, a := range []path.Path{boolJoin(r, r), boolJoin(r, boolReverse(r)), boolJoin(r, boolRect(2, 2, 6, 6)), boolPolygon(geom.V(0, 0), geom.V(4, 4), geom.V(0, 4), geom.V(4, 0))} {
		b := boolRect(1, 1, 7, 7)
		for _, ra := range []path.FillRule{path.NonZero, path.EvenOdd} {
			for _, rb := range []path.FillRule{path.NonZero, path.EvenOdd} {
				for op := path.BooleanUnion; op <= path.BooleanXor; op++ {
					got, err := path.Boolean(a, b, op, path.BooleanOptions{RuleA: ra, RuleB: rb})
					if err != nil {
						t.Fatal(err)
					}
					boolCheckGrid(t, a, b, got, op, ra, rb)
				}
			}
		}
	}
	// An entirely doubled winding has area under NonZero and none under EvenOdd.
	for _, tc := range []struct {
		rule path.FillRule
		area float64
	}{{path.NonZero, 64}, {path.EvenOdd, 0}} {
		p, err := path.Boolean(boolJoin(r, r), path.Path{}, path.BooleanUnion, path.BooleanOptions{RuleA: tc.rule})
		if err != nil || boolArea(p) != tc.area {
			t.Fatal(p, err)
		}
	}
}

func TestBooleanDeterminismIndependentOwnershipAndConcurrentReads(t *testing.T) {
	a, b := boolRect(0, 0, 6, 6), boolRect(2, 2, 8, 8)
	want, err := path.Boolean(a, b, path.BooleanUnion, path.BooleanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for range 12 {
		got, err := path.Boolean(b, a, path.BooleanUnion, path.BooleanOptions{})
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatal("nondeterministic or asymmetric result", err)
		}
	}
	copyWant := want.Clone()
	want.Contours[0].Curves[0].P0 = geom.V(999, 999)
	got, err := path.Boolean(a, b, path.BooleanUnion, path.BooleanOptions{})
	if err != nil || !reflect.DeepEqual(got, copyWant) {
		t.Fatal("output aliases input or another result")
	}
	var wg sync.WaitGroup
	for range 6 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 4 {
				p, err := path.Boolean(a, b, path.BooleanUnion, path.BooleanOptions{})
				if err != nil || !reflect.DeepEqual(p, copyWant) {
					t.Error("concurrent Boolean", err)
				}
			}
		}()
	}
	wg.Wait()
}

func TestBooleanRejectsInvalidGeometryAndCubic(t *testing.T) {
	r := boolRect(0, 0, 2, 2)
	open := r.Clone()
	open.Contours[0].Closed = false
	disconnected := r.Clone()
	disconnected.Contours[0].Curves[1].P0.X += 1e-12
	zero := r.Clone()
	zero.Contours[0].Curves = append([]path.Cubic{path.Line(geom.V(0, 0), geom.V(0, 0))}, zero.Contours[0].Curves...)
	curved := r.Clone()
	curved.Contours[0].Curves[0].P1.Y = .5
	nonplane := r.Clone()
	nonplane.Contours[0].Curves[0].P1.Z = .01
	nan := r.Clone()
	nan.Contours[0].Curves[0].P1.X = math.NaN()
	for _, tc := range []struct {
		name string
		p    path.Path
		want string
	}{{"open", open, "closed"}, {"move only", path.Path{Contours: []path.Contour{{Closed: true}}}, "nonzero"}, {"disconnected", disconnected, "exactly"}, {"zero edge", zero, "zero-length"}, {"curved", curved, "nonlinear"}, {"nonplane", nonplane, "XY"}, {"nan", nan, "finite"}, {"collinear", boolPolygon(geom.V(0, 0), geom.V(1, 0), geom.V(2, 0)), "collinear"}} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := path.Boolean(tc.p, r, path.BooleanUnion, path.BooleanOptions{})
			if err == nil || len(got.Contours) != 0 || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("%v %v", got, err)
			}
		})
	}
}

func TestBooleanRandomRectangleTruthTables(t *testing.T) {
	rng := rand.New(rand.NewSource(80191))
	for range 60 {
		makeP := func() path.Path {
			var ps []path.Path
			for range 3 {
				x, y := float64(rng.Intn(9)), float64(rng.Intn(9))
				p := boolRect(x, y, x+float64(rng.Intn(4)+1), y+float64(rng.Intn(4)+1))
				if rng.Intn(2) == 0 {
					p = boolReverse(p)
				}
				ps = append(ps, p)
			}
			return boolJoin(ps...)
		}
		a, b := makeP(), makeP()
		ra, rb := path.FillRule(rng.Intn(2)), path.FillRule(rng.Intn(2))
		for op := path.BooleanUnion; op <= path.BooleanXor; op++ {
			got, err := path.Boolean(a, b, op, path.BooleanOptions{RuleA: ra, RuleB: rb})
			if err != nil {
				t.Fatal(err)
			}
			boolCheckGrid(t, a, b, got, op, ra, rb)
		}
	}
}

func TestBooleanRandomNonAxisAlignedPolygons(t *testing.T) {
	rng := rand.New(rand.NewSource(94213))
	for i := range 40 {
		triangle := func() path.Path {
			for {
				p := boolPolygon(geom.V(float64(rng.Intn(12)), float64(rng.Intn(12))), geom.V(float64(rng.Intn(12)), float64(rng.Intn(12))), geom.V(float64(rng.Intn(12)), float64(rng.Intn(12))))
				if math.Abs(boolArea(p)) > .5 {
					return p
				}
			}
		}
		a, b := triangle(), triangle()
		for op := path.BooleanUnion; op <= path.BooleanXor; op++ {
			p, err := path.Boolean(a, b, op, path.BooleanOptions{})
			if err != nil {
				t.Fatalf("case %d op%d: %v\na=%+v\nb=%+v", i, op, err, a, b)
			}
			boolCheckGrid(t, a, b, p, op, path.NonZero, path.NonZero)
		}
	}
}

func TestBooleanRandomSelfCrossingPolygons(t *testing.T) {
	rng := rand.New(rand.NewSource(1357911))
	for i := range 50 {
		makeP := func() path.Path {
			points := make([]geom.Vec3, 0, 5)
			for len(points) < 5 {
				v := geom.V(float64(rng.Intn(12)), float64(rng.Intn(12)))
				unique := true
				for _, q := range points {
					unique = unique && q != v
				}
				if unique {
					points = append(points, v)
				}
			}
			return boolPolygon(points...)
		}
		a, b := makeP(), makeP()
		for _, rule := range []path.FillRule{path.NonZero, path.EvenOdd} {
			for op := path.BooleanUnion; op <= path.BooleanXor; op++ {
				got, err := path.Boolean(a, b, op, path.BooleanOptions{RuleA: rule, RuleB: rule})
				if err != nil {
					t.Fatalf("case %d rule%d op%d: %v", i, rule, op, err)
				}
				boolCheckGrid(t, a, b, got, op, rule, rule)
			}
		}
	}
}

func BenchmarkBooleanPolygonIntersection(b *testing.B) {
	for _, n := range []int{8, 24} {
		b.Run(fmt.Sprintf("vertices%d", n), func(b *testing.B) {
			polygon := func(dx float64) path.Path {
				points := make([]geom.Vec3, n)
				for i := range points {
					a := 2 * math.Pi * float64(i) / float64(n)
					points[i] = geom.V(dx+5*math.Cos(a), 5*math.Sin(a))
				}
				return boolPolygon(points...)
			}
			a, c := polygon(0), polygon(2)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if _, err := path.Boolean(a, c, path.BooleanIntersection, path.BooleanOptions{}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
