package path_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"gomath"
	"gomath/geom"
	"gomath/path"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func ceMust(t testing.TB, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func ceNear(t testing.TB, got, want geom.Vec3, tolerance float64) {
	t.Helper()
	if !got.Finite() || got.Sub(want).Norm() > tolerance {
		t.Fatalf("point %v, want %v (tolerance %g)", got, want, tolerance)
	}
}

func ceVec(p [3]float64) geom.Vec3 { return geom.Vec3{X: p[0], Y: p[1], Z: p[2]} }

func ceContour(points [][3]float64) path.Contour {
	c := path.Contour{Start: ceVec(points[0])}
	for i := 0; i+3 < len(points); i += 4 {
		c.Curves = append(c.Curves, path.Cubic{P0: ceVec(points[i]), P1: ceVec(points[i+1]), P2: ceVec(points[i+2]), P3: ceVec(points[i+3])})
	}
	c.Closed = len(c.Curves) > 0 && c.Start == c.Curves[len(c.Curves)-1].P3
	return c
}

func cePoints(t testing.TB, c path.Contour, points [][3]float64) {
	t.Helper()
	if len(points) != 4*len(c.Curves) {
		t.Fatalf("got %d curves, want %d points", len(c.Curves), len(points))
	}
	for i, v := range c.Curves {
		for j, p := range []geom.Vec3{v.P0, v.P1, v.P2, v.P3} {
			ceNear(t, p, ceVec(points[4*i+j]), 1e-10)
		}
	}
}

func TestCurveEditPinnedManimAndInteriorTransform(t *testing.T) {
	fixturePath := filepath.Join("..", "tests", "data", "path", "curve-edit", "reference.json")
	data, err := os.ReadFile(fixturePath)
	ceMust(t, err)
	fixtureHash := sha256.Sum256(data)
	if hex.EncodeToString(fixtureHash[:]) != "c67c1698aa8a931ca523bc441ff62e2e16579e58ec4d04798fcaf46e06093148" {
		t.Fatal("curve edit reference fixture hash changed")
	}
	var f struct {
		Source struct {
			Version, Commit string
			SHA256          map[string]string
			Generator       string `json:"generator_sha256"`
		}
		Anchors []struct {
			Name, Mode      string
			Anchors, Points [][3]float64
		}
		Insertions []struct {
			Name          string
			Input, Points [][3]float64
			Total         int
		}
		Transforms []struct {
			Name           string
			Source, Target [][3]float64
			Total          int
			States         []struct {
				Alpha  float64
				Points [][3]float64
			}
		}
	}
	ceMust(t, json.Unmarshal(data, &f))
	generator, err := os.ReadFile(filepath.Join("..", "tests", "reference", "curve_edit.py"))
	ceMust(t, err)
	h := sha256.Sum256(generator)
	if f.Source.Version != "0.22.1" || f.Source.Commit != "16ea443e2a5f1d79db0a721161411ce916d6e733" || f.Source.Generator != hex.EncodeToString(h[:]) || len(f.Source.SHA256) != 6 || len(f.Anchors) != 24 || len(f.Insertions) != 18 || len(f.Transforms) != 4 {
		t.Fatal("reference provenance or case counts changed")
	}
	for name, hash := range f.Source.SHA256 {
		// Verify local pinned source when available, without git or Python.
		blob, err := os.ReadFile(filepath.Join("..", "..", "manim", filepath.FromSlash(name)))
		if os.IsNotExist(err) {
			continue
		}
		ceMust(t, err)
		sum := sha256.Sum256(blob)
		if hex.EncodeToString(sum[:]) != hash {
			t.Fatalf("pinned source changed: %s", name)
		}
	}
	for _, tc := range f.Anchors {
		t.Run("anchors/"+tc.Name, func(t *testing.T) {
			a := make([]geom.Vec3, len(tc.Anchors))
			for i, v := range tc.Anchors {
				a[i] = ceVec(v)
			}
			c, err := path.FromAnchors(a, path.AnchorMode(tc.Mode))
			ceMust(t, err)
			cePoints(t, c, tc.Points)
			ceNear(t, c.Start, a[0], 0)
			j, err := path.FromAnchors(a, path.AnchorJagged)
			ceMust(t, err)
			p, err := (path.Path{Contours: []path.Contour{j}}).WithAnchorMode(path.AnchorMode(tc.Mode))
			ceMust(t, err)
			cePoints(t, p.Contours[0], tc.Points)
		})
	}
	for _, tc := range f.Insertions {
		t.Run("insert/"+tc.Name, func(t *testing.T) {
			c, err := ceContour(tc.Input).WithCurveCount(tc.Total)
			ceMust(t, err)
			cePoints(t, c, tc.Points)
		})
	}
	for _, tc := range f.Transforms {
		t.Run("transform/"+tc.Name, func(t *testing.T) {
			// Explicitly prepare both topologies with the new method. The existing
			// automatic path.Align policy is a different algorithm and is unchanged.
			a, err := ceContour(tc.Source).WithCurveCount(tc.Total)
			ceMust(t, err)
			b, err := ceContour(tc.Target).WithCurveCount(tc.Total)
			ceMust(t, err)
			s := gomath.NewScene()
			source := gomath.NewObject(path.Path{Contours: []path.Contour{a}})
			target := gomath.NewObject(path.Path{Contours: []path.Contour{b}})
			id := source.ID
			ceMust(t, s.Add(source))
			ceMust(t, s.Play(1, gomath.Transform(source, target).WithRate(gomath.Linear)))
			if source.ID != id {
				t.Fatal("Transform changed source identity")
			}
			for _, state := range tc.States {
				frame, err := s.Sample(state.Alpha)
				ceMust(t, err)
				o := frame.Find(id)
				if o == nil || len(o.Path.Contours) != 1 {
					t.Fatal("missing contour in frame")
				}
				cePoints(t, o.Path.Contours[0], state.Points)
			}
		})
	}
}

func ceSampleContour() path.Contour {
	a := geom.Vec3{X: -1, Y: 2, Z: 1}
	b := geom.Vec3{X: 3, Y: -2, Z: 2}
	c := geom.Vec3{X: 4, Y: 1, Z: -1}
	return path.Contour{Start: a, Curves: []path.Cubic{
		{P0: a, P1: geom.Vec3{X: 2, Y: 6, Z: -1}, P2: geom.Vec3{X: -3, Y: 1, Z: 2}, P3: b},
		{P0: b, P1: b.Lerp(c, .3), P2: b.Lerp(c, .7), P3: c},
	}}
}

func TestCurveEditEqualParameterShapeAllocationAndDeterminism(t *testing.T) {
	rng := rand.New(rand.NewPCG(17, 93))
	for trial := range 80 {
		n := 1 + rng.IntN(13)
		total := n + rng.IntN(99)
		c := path.Contour{Start: geom.Vec3{X: rng.Float64(), Y: rng.Float64(), Z: rng.Float64()}}
		prev := c.Start
		for range n {
			point := func() geom.Vec3 {
				return geom.Vec3{X: 20*rng.Float64() - 10, Y: 20*rng.Float64() - 10, Z: 20*rng.Float64() - 10}
			}
			v := path.Cubic{P0: prev, P1: point(), P2: point(), P3: point()}
			c.Curves = append(c.Curves, v)
			prev = v.P3
		}
		q, err := c.WithCurveCount(total)
		ceMust(t, err)
		again, err := c.WithCurveCount(total)
		ceMust(t, err)
		if !reflect.DeepEqual(q, again) || len(q.Curves) != total {
			t.Fatalf("trial %d: nondeterministic count", trial)
		}
		// Derive allocation by enumerating the source's floor mapping instead
		// of duplicating production's ceil-difference arithmetic.
		counts := make([]int, n)
		for j := range total {
			counts[j*n/total]++
		}
		index := 0
		for i, parts := range counts {
			for j := range parts {
				v := q.Curves[index]
				for k := range 17 {
					u := float64(k) / 16
					ceNear(t, v.At(u), c.Curves[i].At((float64(j)+u)/float64(parts)), 1e-11)
				}
				if index > 0 && q.Curves[index-1].P3 != v.P0 {
					t.Fatal("subdivision introduced a gap")
				}
				index++
			}
		}
	}
}

func TestCurveEditSmoothDerivativeConditionsAndAnchors(t *testing.T) {
	for _, closed := range []bool{false, true} {
		a := []geom.Vec3{geom.V(0, 0), geom.V(1, 3), {X: 3, Y: -2, Z: 1}, geom.V(5, 2)}
		if closed {
			a = append(a, a[0])
		}
		c, err := path.FromAnchors(a, path.AnchorSmooth)
		ceMust(t, err)
		for i, v := range c.Curves {
			ceNear(t, v.P0, a[i], 0)
			ceNear(t, v.P3, a[i+1], 0)
			if i+1 == len(c.Curves) && !closed {
				continue
			}
			next := c.Curves[(i+1)%len(c.Curves)]
			ceNear(t, v.P3.Sub(v.P2), next.P1.Sub(next.P0), 1e-12)
			ceNear(t, v.P3.Sub(v.P2.Mul(2)).Add(v.P1), next.P2.Sub(next.P1.Mul(2)).Add(next.P0), 1e-12)
		}
		if !closed {
			first, last := c.Curves[0], c.Curves[len(c.Curves)-1]
			ceNear(t, first.P0.Sub(first.P1.Mul(2)).Add(first.P2), geom.Vec3{}, 1e-12)
			ceNear(t, last.P3.Sub(last.P2.Mul(2)).Add(last.P1), geom.Vec3{}, 1e-12)
		}
		j, err := (path.Path{Contours: []path.Contour{c}}).WithAnchorMode(path.AnchorJagged)
		ceMust(t, err)
		for i, v := range j.Contours[0].Curves {
			if v != path.Line(a[i], a[i+1]) {
				t.Fatal("jagged handles not linear thirds")
			}
		}
	}
}

func TestCurveEditInvalidInputsAndAtomicity(t *testing.T) {
	c := ceSampleContour()
	for _, total := range []int{-1, 0, 1, 65537, math.MaxInt} {
		if q, err := c.WithCurveCount(total); err == nil || len(q.Curves) != 0 {
			t.Fatalf("accepted total %d", total)
		}
	}
	for _, mode := range []path.AnchorMode{"", "Smooth", "cubic", "jagged\x00"} {
		if _, err := path.FromAnchors([]geom.Vec3{{}}, mode); err == nil {
			t.Fatal("accepted mode")
		}
		if _, err := (path.Path{}).WithAnchorMode(mode); err == nil {
			t.Fatal("accepted mode on empty path")
		}
	}
	if _, err := path.FromAnchors(nil, path.AnchorSmooth); err == nil {
		t.Fatal("accepted no anchors")
	}
	for _, v := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), math.Nextafter(1e9, math.Inf(1)), -1e9 - 1} {
		for axis := range 3 {
			point := geom.Vec3{}
			switch axis {
			case 0:
				point.X = v
			case 1:
				point.Y = v
			case 2:
				point.Z = v
			}
			if _, err := path.FromAnchors([]geom.Vec3{{}, point}, path.AnchorJagged); err == nil {
				t.Fatal("accepted bad anchor")
			}
			for control := range 5 {
				bad := path.Path{Contours: []path.Contour{c}}.Clone().Contours[0]
				switch control {
				case 0:
					bad.Start = point
				case 1:
					bad.Curves[0].P0 = point
				case 2:
					bad.Curves[0].P1 = point
				case 3:
					bad.Curves[0].P2 = point
				case 4:
					bad.Curves[0].P3 = point
				}
				if _, err := bad.WithCurveCount(3); err == nil {
					t.Fatal("accepted bad control")
				}
				if _, err := (path.Path{Contours: []path.Contour{bad}}).WithAnchorMode(path.AnchorJagged); err == nil {
					t.Fatal("accepted bad contour")
				}
			}
		}
	}
	for _, where := range []int{0, 1} {
		bad := path.Path{Contours: []path.Contour{c}}.Clone().Contours[0]
		bad.Curves[where].P0.X += 1e-12
		if _, err := bad.WithCurveCount(3); err == nil {
			t.Fatal("accepted tiny disconnection")
		}
	}
	implicit := c
	implicit.Closed = true
	if _, err := implicit.WithCurveCount(5); err != nil {
		t.Fatal("insertion rejected legal implicit closure", err)
	}
	if _, err := (path.Path{Contours: []path.Contour{implicit}}).WithAnchorMode(path.AnchorSmooth); err == nil {
		t.Fatal("smoothed implicit closure")
	}
	// Valid input anchors can produce out-of-bound smooth handles.
	a := []geom.Vec3{geom.V(0, 0), geom.V(1e9, 0), geom.V(-1e9, 0), geom.V(0, 0)}
	if _, err := path.FromAnchors(a, path.AnchorSmooth); err == nil {
		t.Fatal("accepted handle overshoot beyond coordinate cap")
	}
	// An invalid later contour must not return partial output or mutate earlier input.
	bad := c
	bad.Start.X += 1
	p := path.Path{Contours: []path.Contour{c, bad}}
	before := p.Clone()
	q, err := p.WithAnchorMode(path.AnchorSmooth)
	if err == nil || len(q.Contours) != 0 || !reflect.DeepEqual(before, p) {
		t.Fatal("failed edit was not atomic")
	}
}

func TestCurveEditResourceBoundaries(t *testing.T) {
	point := path.Contour{Start: geom.Vec3{X: 1e9, Y: -1e9, Z: 1e9}}
	q, err := point.WithCurveCount(65536)
	ceMust(t, err)
	if len(q.Curves) != 65536 || q.Curves[65535].P3 != point.Start {
		t.Fatal("maximum point expansion failed")
	}
	if _, err := q.WithCurveCount(65536); err == nil || !strings.Contains(err.Error(), "storage") {
		t.Fatal("conservative input/output storage cap not enforced", err)
	}
	q.Curves = q.Curves[:65535]
	_, err = q.WithCurveCount(65535)
	ceMust(t, err)
	p := path.Path{Contours: make([]path.Contour, 4096)}
	_, err = p.WithAnchorMode(path.AnchorJagged)
	ceMust(t, err)
	p.Contours = append(p.Contours, path.Contour{})
	if _, err := p.WithAnchorMode(path.AnchorJagged); err == nil {
		t.Fatal("contour limit not enforced")
	}
	a := make([]geom.Vec3, 41944) // 25*41943+1 equals the work budget exactly.
	_, err = path.FromAnchors(a, path.AnchorSmooth)
	ceMust(t, err)
	a = append(a, geom.Vec3{})
	if _, err := path.FromAnchors(a, path.AnchorSmooth); err == nil || !strings.Contains(err.Error(), "work") {
		t.Fatal("smooth work cap not enforced", err)
	}
	a = make([]geom.Vec3, 65538)
	if _, err := path.FromAnchors(a, path.AnchorJagged); err == nil {
		t.Fatal("anchor limit not enforced")
	}
	p = path.Path{Contours: []path.Contour{{Curves: make([]path.Cubic, 65537)}}}
	if _, err := p.WithAnchorMode(path.AnchorJagged); err == nil {
		t.Fatal("input curve limit not enforced")
	}
	p = path.Path{Contours: []path.Contour{{Curves: make([]path.Cubic, 37450)}}}
	if _, err := p.WithAnchorMode(path.AnchorSmooth); err == nil || !strings.Contains(err.Error(), "work") {
		t.Fatal("aggregate work cap not enforced", err)
	}
	allocs := testing.AllocsPerRun(30, func() { _, _ = point.WithCurveCount(math.MaxInt) })
	if allocs > 8 {
		t.Fatalf("invalid target allocated unexpectedly: %g allocations", allocs)
	}
}

func TestCurveEditNoAliasingContoursPointsAndConcurrentCalls(t *testing.T) {
	c := ceSampleContour()
	original := path.Path{Contours: []path.Contour{c}}.Clone()
	for _, total := range []int{2, 7} {
		q, err := c.WithCurveCount(total)
		ceMust(t, err)
		q.Curves[0].P1.X = 999
		if !reflect.DeepEqual(c, original.Contours[0]) {
			t.Fatal("curve output aliases input")
		}
	}
	a := []geom.Vec3{geom.V(0, 0), geom.V(1, 2), geom.V(3, 1)}
	q, err := path.FromAnchors(a, path.AnchorSmooth)
	ceMust(t, err)
	saved := q.Curves[0]
	a[0].X = 88
	if q.Curves[0] != saved {
		t.Fatal("constructor retained anchors")
	}
	p := path.Path{Contours: []path.Contour{c, {Start: geom.V(30, 30)}}}
	before := p.Clone()
	for _, mode := range []path.AnchorMode{path.AnchorSmooth, path.AnchorJagged} {
		x, err := p.WithAnchorMode(mode)
		ceMust(t, err)
		if len(x.Contours) != 2 || x.Contours[1].Start != p.Contours[1].Start || len(x.Contours[1].Curves) != 0 {
			t.Fatal("point contour or gap lost")
		}
		x.Contours[0].Curves[0].P2.X = 91
		x.Contours[1].Start.X = 99
		if !reflect.DeepEqual(before, p) {
			t.Fatal("path output aliases input")
		}
	}
	shared, err := (path.Path{Contours: []path.Contour{c, c}}).WithAnchorMode(path.AnchorSmooth)
	ceMust(t, err)
	second := shared.Contours[1].Curves[0]
	shared.Contours[0].Curves[0].P1.X = 777
	if shared.Contours[1].Curves[0] != second {
		t.Fatal("output contours share writable curves")
	}
	want, err := c.WithCurveCount(17)
	ceMust(t, err)
	wantSmooth, err := p.WithAnchorMode(path.AnchorSmooth)
	ceMust(t, err)
	var wg sync.WaitGroup
	failures := make(chan error, 24)
	for worker := range 24 {
		// Different cyclic-system sizes exercise independent scratch rather
		// than merely simultaneous reads of one fixed-size open solution.
		closedAnchors := make([]geom.Vec3, worker+4)
		for i := 0; i < len(closedAnchors)-1; i++ {
			angle := 2 * math.Pi * float64(i) / float64(len(closedAnchors)-1)
			closedAnchors[i] = geom.Vec3{X: math.Cos(angle), Y: math.Sin(angle), Z: float64(i % 3)}
		}
		closedAnchors[len(closedAnchors)-1] = closedAnchors[0]
		closedWant, err := path.FromAnchors(closedAnchors, path.AnchorSmooth)
		ceMust(t, err)
		wg.Go(func() {
			for range 12 {
				x, e := c.WithCurveCount(17)
				if e != nil || !reflect.DeepEqual(x, want) {
					failures <- fmt.Errorf("concurrent insertion: %v", e)
					return
				}
				y, e := p.WithAnchorMode(path.AnchorSmooth)
				if e != nil || !reflect.DeepEqual(y, wantSmooth) {
					failures <- fmt.Errorf("concurrent smooth: %v", e)
					return
				}
				z, e := path.FromAnchors(closedAnchors, path.AnchorSmooth)
				if e != nil || !reflect.DeepEqual(z, closedWant) {
					failures <- fmt.Errorf("concurrent cyclic smooth: %v", e)
					return
				}
				x.Curves[0].P0.X = 200
				y.Contours[0].Curves[0].P1.X = 300
				z.Curves[0].P2.Z = 400
			}
		})
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	if !reflect.DeepEqual(before, p) {
		t.Fatal("concurrent edits mutated inputs")
	}
	empty, err := (path.Path{}).WithAnchorMode(path.AnchorSmooth)
	ceMust(t, err)
	if len(empty.Contours) != 0 {
		t.Fatal("empty path expanded")
	}
	point := path.Contour{Start: geom.V(1, 2), Closed: true}
	zero, err := point.WithCurveCount(0)
	ceMust(t, err)
	if zero.Start != point.Start || !zero.Closed || len(zero.Curves) != 0 {
		t.Fatal("point-only no-op changed metadata")
	}
}

func FuzzCurveEditSubdivision(f *testing.F) {
	f.Add(uint8(3), 2., -5., 1.)
	f.Add(uint8(15), 0., 0., 0.)
	f.Fuzz(func(t *testing.T, n uint8, x, y, z float64) {
		for _, v := range []float64{x, y, z} {
			if math.IsNaN(v) || math.IsInf(v, 0) || math.Abs(v) > 1e6 {
				t.Skip()
			}
		}
		c := path.Cubic{P1: geom.Vec3{X: x, Y: y, Z: z}, P2: geom.Vec3{X: -z, Y: x, Z: y}, P3: geom.Vec3{X: 1, Y: -2, Z: 3}}
		count := 1 + int(n%64)
		q, err := (path.Contour{Curves: []path.Cubic{c}}).WithCurveCount(count)
		ceMust(t, err)
		for i, v := range q.Curves {
			for _, u := range []float64{0, .125, .5, .875, 1} {
				ceNear(t, v.At(u), c.At((float64(i)+u)/float64(count)), 1e-8)
			}
		}
	})
}

func BenchmarkCurveEdit(b *testing.B) {
	for _, n := range []int{64, 1024, 16384} {
		anchors := make([]geom.Vec3, n+1)
		for i := range anchors {
			anchors[i] = geom.Vec3{X: float64(i) / float64(n), Y: math.Sin(float64(i) * .1), Z: float64(i%7) * .01}
		}
		c, err := path.FromAnchors(anchors, path.AnchorJagged)
		ceMust(b, err)
		b.Run(fmt.Sprintf("Insert_%d_to_%d", n, 2*n+1), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_, err := c.WithCurveCount(2*n + 1)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
		b.Run(fmt.Sprintf("SmoothOpen_%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_, err := path.FromAnchors(anchors, path.AnchorSmooth)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
		closed := append(append([]geom.Vec3(nil), anchors...), anchors[0])
		b.Run(fmt.Sprintf("SmoothClosed_%d", n+1), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_, err := path.FromAnchors(closed, path.AnchorSmooth)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
