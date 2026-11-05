package path_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"gomath/geom"
	"gomath/path"
	"math"
	"math/rand"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
)

type pdContour struct {
	Points [][3]float64
	Closed bool
}

func pdVec(p [3]float64) geom.Vec3 { return geom.Vec3{X: p[0], Y: p[1], Z: p[2]} }

func pdPath(contours []pdContour) path.Path {
	var p path.Path
	for _, c := range contours {
		v := path.Contour{Start: pdVec(c.Points[0]), Closed: c.Closed}
		for i := 0; i+3 < len(c.Points); i += 4 {
			v.Curves = append(v.Curves, path.Cubic{P0: pdVec(c.Points[i]), P1: pdVec(c.Points[i+1]), P2: pdVec(c.Points[i+2]), P3: pdVec(c.Points[i+3])})
		}
		p.Contours = append(p.Contours, v)
	}
	return p
}

func pdPoints(p path.Path) []geom.Vec3 {
	var result []geom.Vec3
	for _, c := range p.Contours {
		if len(c.Curves) == 0 {
			result = append(result, c.Start)
		}
		for _, v := range c.Curves {
			result = append(result, v.P0, v.P1, v.P2, v.P3)
		}
	}
	return result
}

func pdMust(t testing.TB, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func pdCompare(t testing.TB, p path.Path, expected [][3]float64, tolerance float64) {
	t.Helper()
	points := pdPoints(p)
	if len(points) != len(expected) {
		t.Fatalf("points=%d want=%d", len(points), len(expected))
	}
	for i, v := range points {
		if !v.Finite() || v.Sub(pdVec(expected[i])).Norm() > tolerance {
			t.Fatalf("point %d got=%v want=%v", i, v, expected[i])
		}
	}
}

func TestDirectionPinnedManimAndTransformControls(t *testing.T) {
	var fixture struct {
		Source struct {
			Version, Commit string
			Hash            string            `json:"generator_sha256"`
			Files           map[string]string `json:"files_sha256"`
			Verified        bool              `json:"installed_equals_local_equals_git"`
		}
		Paths []struct {
			Name     string
			Contours []pdContour
			Reversed [][3]float64
			States   []struct {
				Alpha  float64
				Points [][3]float64
			}
		}
		Force []struct {
			Name, Target, Before, After, Error string
		} `json:"force_audit"`
	}
	data, err := os.ReadFile("../tests/data/path/direction/reference.json")
	pdMust(t, err)
	pdMust(t, json.Unmarshal(data, &fixture))
	generator, err := os.ReadFile("../tests/reference/path_direction.py")
	pdMust(t, err)
	hash := sha256.Sum256(generator)
	if fixture.Source.Version != "0.22.1" || fixture.Source.Commit != "16ea443e2a5f1d79db0a721161411ce916d6e733" || fixture.Source.Hash != hex.EncodeToString(hash[:]) || !fixture.Source.Verified || len(fixture.Source.Files) < 7 || len(fixture.Paths) != 8 || len(fixture.Force) != 18 {
		t.Fatal("direction reference provenance mismatch")
	}
	interior := 0
	for _, tc := range fixture.Paths {
		t.Run(tc.Name, func(t *testing.T) {
			p := pdPath(tc.Contours)
			before := p.Clone()
			q, err := p.Reversed()
			pdMust(t, err)
			pdCompare(t, q, tc.Reversed, 0)
			if len(p.Contours) != 0 && !reflect.DeepEqual(p, before) {
				t.Fatal("source mutated")
			}
			target := q.Transform(geom.Translation(geom.Vec3{X: .4, Y: -.3, Z: .1}).Mul(geom.RotationZ(.27)).Mul(geom.Scaling(1.08, 1.08, 1.08)))
			for _, state := range tc.States {
				pdCompare(t, path.InterpolateAligned(p, target, state.Alpha), state.Points, 1e-10)
				if state.Alpha > 0 && state.Alpha < 1 {
					interior++
				}
			}
		})
	}
	if interior != 20 {
		t.Fatal("missing actual Transform interior states", interior)
	}
	foundNonOrientable := false
	for _, audit := range fixture.Force {
		if audit.Name == "line" && audit.Target == "CW" && audit.After == "CCW" && audit.Error == "" {
			foundNonOrientable = true
		}
	}
	if !foundNonOrientable {
		t.Fatal("force_direction audit lost the zero-area limitation")
	}
}

func pdSample(n int) path.Contour {
	c := path.Contour{Start: geom.Vec3{X: -3, Y: 2, Z: -1}}
	a := c.Start
	for i := range n {
		b := geom.Vec3{X: float64(i) + .5, Y: float64(i%3) - 1, Z: float64(i%7) - 2}
		c.Curves = append(c.Curves, path.Cubic{P0: a, P1: a.Add(geom.Vec3{X: 2, Y: -4, Z: 6}), P2: b.Add(geom.Vec3{X: -1, Y: 3, Z: -2}), P3: b})
		a = b
	}
	return c
}

func TestDirectionExactReversalShapeEndpointsAndMetadata(t *testing.T) {
	open := pdSample(7)
	closed := pdSample(3)
	closed.Curves = append(closed.Curves, path.Line(closed.Curves[2].P3, closed.Start))
	closed.Closed = true
	cases := []path.Path{
		{}, {Contours: []path.Contour{}},
		{Contours: []path.Contour{{Start: geom.Vec3{X: 2, Y: -3, Z: 4}, Closed: true}}},
		{Contours: []path.Contour{{Curves: []path.Cubic{}}}},
		{Contours: []path.Contour{open, {Start: geom.V(20, 3)}, closed, open}},
	}
	for _, p := range cases {
		q, err := p.Reversed()
		pdMust(t, err)
		r, err := q.Reversed()
		pdMust(t, err)
		if !reflect.DeepEqual(r, p) {
			t.Fatal("double reversal changed exact controls, order, or nil/empty storage", r, p)
		}
		for i, c := range p.Contours {
			v := q.Contours[len(p.Contours)-1-i]
			one, err := c.Reversed()
			pdMust(t, err)
			if !reflect.DeepEqual(v, one) || v.Closed != c.Closed {
				t.Fatal("path/contour reversal or closed metadata differs")
			}
			wantStart := c.Start
			if len(c.Curves) > 0 {
				wantStart = c.Curves[len(c.Curves)-1].P3
			}
			if v.Start != wantStart {
				t.Fatal("start did not use true endpoint")
			}
			for j, curve := range c.Curves {
				reversed := v.Curves[len(c.Curves)-1-j]
				if reversed != (path.Cubic{P0: curve.P3, P1: curve.P2, P2: curve.P1, P3: curve.P0}) {
					t.Fatal("explicit control values changed")
				}
				for _, u := range []float64{0, .03125, .125, .375, .5, .875, 1} {
					if reversed.At(u).Sub(curve.At(1-u)).Norm() > 1e-12 {
						t.Fatal("Bezier shape was not preserved")
					}
				}
			}
		}
	}
}

func TestDirectionStrictInputsAndImplicitClosureAtomicity(t *testing.T) {
	base := pdSample(2)
	valid := path.Path{Contours: []path.Contour{pdSample(1)}}
	for slot := range 9 {
		for _, invalid := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), math.Nextafter(1e9, math.Inf(1)), -1e9 - 1} {
			c := path.Path{Contours: []path.Contour{base}}.Clone().Contours[0]
			v := &c.Start
			if slot > 0 {
				x := &c.Curves[(slot-1)/4]
				v = []*geom.Vec3{&x.P0, &x.P1, &x.P2, &x.P3}[(slot-1)%4]
			}
			v.Z = invalid
			p := path.Path{Contours: []path.Contour{valid.Contours[0], c}}
			if q, err := p.Reversed(); err == nil || !reflect.DeepEqual(q, path.Path{}) || !reflect.DeepEqual(p.Contours[0], valid.Contours[0]) {
				t.Fatal("invalid coordinate accepted, partial result, or input changed")
			}
			if _, err := c.Reversed(); err == nil {
				t.Fatal("contour accepted invalid coordinate")
			}
		}
	}
	for _, slot := range []int{0, 1} {
		c := path.Path{Contours: []path.Contour{base}}.Clone().Contours[0]
		c.Curves[slot].P0.X += 1e-12
		if _, err := c.Reversed(); err == nil {
			t.Fatal("tolerant connection accepted")
		}
	}
	for _, gap := range []float64{1, 1e-14} {
		c := path.Contour{Closed: true, Curves: []path.Cubic{path.Line(geom.V(0, 0), geom.V(gap, 0))}}
		if _, err := c.Reversed(); err == nil || !strings.Contains(err.Error(), "implicit") {
			t.Fatal("implicit closing edge accepted", err)
		}
		c.Closed = false
		q, err := c.Reversed()
		pdMust(t, err)
		if q.Start != c.Curves[0].P3 || q.Curves[0].P3 != c.Start || q.Closed {
			t.Fatal("open near-closed endpoints snapped or closed")
		}
	}
	boundary := path.Contour{Start: geom.Vec3{X: 1e9, Y: -1e9, Z: 1e9}}
	_, err := boundary.Reversed()
	pdMust(t, err)
}

func TestDirectionSharedBudgetsAndEarlyRejection(t *testing.T) {
	points := path.Path{Contours: make([]path.Contour, path.MaxDirectionContours)}
	_, err := points.Reversed()
	pdMust(t, err)
	points.Contours = append(points.Contours, path.Contour{})
	if _, err := points.Reversed(); err == nil || !strings.Contains(err.Error(), "contour") {
		t.Fatal("contour limit ignored", err)
	}
	c := path.Contour{Curves: make([]path.Cubic, 65535)}
	_, err = c.Reversed()
	pdMust(t, err)
	c.Curves = append(c.Curves, path.Cubic{})
	if _, err := c.Reversed(); err == nil || !strings.Contains(err.Error(), "storage") {
		t.Fatal("borrowed input + output storage limit ignored", err)
	}
	c.Curves = append(c.Curves, path.Cubic{})
	if _, err := c.Reversed(); err == nil || !strings.Contains(err.Error(), "curve limit") {
		t.Fatal("curve count cap ignored", err)
	}
	split := path.Path{Contours: []path.Contour{{Curves: make([]path.Cubic, 32768)}, {Curves: make([]path.Cubic, 32768)}}}
	if _, err := split.Reversed(); err == nil || !strings.Contains(err.Error(), "storage") {
		t.Fatal("per-contour budget incorrectly replaced shared budget", err)
	}
	// All count/budget failures precede scanning or cloning these controls.
	split.Contours[0].Curves[0].P1.X = math.NaN()
	allocations := testing.AllocsPerRun(20, func() {
		if _, err := split.Reversed(); err == nil || !strings.Contains(err.Error(), "storage") {
			t.Fatal("preflight did not precede validation", err)
		}
	})
	if allocations > 8 {
		t.Fatal("rejected oversized input allocated geometry", allocations)
	}
}

func TestDirectionIndependentAliasedContoursAndConcurrentCalls(t *testing.T) {
	c := pdSample(4)
	p := path.Path{Contours: []path.Contour{c, {Start: geom.V(20, 3)}, c}}
	before := p.Clone()
	want, err := p.Reversed()
	pdMust(t, err)
	second := want.Contours[2].Curves[0]
	want.Contours[0].Curves[0].P1.X = 900
	want.Contours[1].Start.Y = 800
	if want.Contours[2].Curves[0] != second || !reflect.DeepEqual(before, p) {
		t.Fatal("output aliases input or another output contour")
	}
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			for range 25 {
				q, err := p.Reversed()
				if err != nil {
					t.Error(err)
					return
				}
				r, err := q.Reversed()
				if err != nil || !reflect.DeepEqual(r, before) {
					t.Error("concurrent reversal differed", err)
					return
				}
				q.Contours[0].Curves[0].P2.Z = 123
			}
		})
	}
	wg.Wait()
	if !reflect.DeepEqual(p, before) {
		t.Fatal("concurrent calls mutated source")
	}
	// Reverse outputs also remain unchanged when a separate source is authored.
	q, err := p.Reversed()
	pdMust(t, err)
	saved := q.Clone()
	p.Contours[0].Curves[0].P3.Z = -700
	if !reflect.DeepEqual(q, saved) {
		t.Fatal("returned output retained input storage")
	}
}

func TestDirectionRandomCurves(t *testing.T) {
	rng := rand.New(rand.NewSource(20261105))
	for range 200 {
		c := path.Contour{}
		point := func() geom.Vec3 {
			return geom.Vec3{X: 10*rng.Float64() - 5, Y: 10*rng.Float64() - 5, Z: 10*rng.Float64() - 5}
		}
		c.Start = point()
		a := c.Start
		for range 1 + rng.Intn(30) {
			v := path.Cubic{P0: a, P1: point(), P2: point(), P3: point()}
			c.Curves = append(c.Curves, v)
			a = v.P3
		}
		q, err := c.Reversed()
		pdMust(t, err)
		r, err := q.Reversed()
		pdMust(t, err)
		if !reflect.DeepEqual(c, r) {
			t.Fatal("random double reversal is not exact")
		}
		for i, v := range c.Curves {
			u := rng.Float64()
			if v.At(u).Sub(q.Curves[len(q.Curves)-1-i].At(1-u)).Norm() > 1e-12 {
				t.Fatal("random cubic changed shape")
			}
		}
	}
}

var pdBenchmarkResult path.Path

func BenchmarkDirectionReversed(b *testing.B) {
	for _, n := range []int{64, 1024, 16384} {
		for _, count := range []int{1, 16} {
			p := path.Path{}
			for range count {
				p.Contours = append(p.Contours, pdSample(n/count))
			}
			b.Run(fmt.Sprintf("Curves%d_Contours%d", n, count), func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(192*n + 112*count))
				for b.Loop() {
					q, err := p.Reversed()
					if err != nil {
						b.Fatal(err)
					}
					pdBenchmarkResult = q
				}
			})
		}
	}
}
