package gomath_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"gomath"
	"gomath/geom"
	"math"
	"os"
	"reflect"
	"strconv"
	"sync"
	"testing"
)

func TestPinnedManimArcBraceTransformStates(t *testing.T) {
	var fixture struct {
		Source struct {
			Version, Commit string
			Hash            string `json:"generator_sha256"`
		}
		Cases []struct {
			Center, Tip                                        [3]float64
			DirectionResult                                    [3]float64 `json:"direction_result"`
			Radius, Start, Sweep, Direction, Buffer, Sharpness float64
			States                                             []connectionReferenceState
		}
	}
	data, err := os.ReadFile("tests/data/root/arc-braces/manim-planar-0.22.1.json")
	must(t, err)
	must(t, json.Unmarshal(data, &fixture))
	generator, err := os.ReadFile("tests/reference/arc_braces.py")
	must(t, err)
	hash := sha256.Sum256(generator)
	if fixture.Source.Version != "0.22.1" || fixture.Source.Commit != "16ea443e2a5f1d79db0a721161411ce916d6e733" || fixture.Source.Hash != hex.EncodeToString(hash[:]) || len(fixture.Cases) != 36 {
		t.Fatal("arc brace provenance/count differs")
	}
	vec := func(p [3]float64) geom.Vec3 { return geom.Vec3{X: p[0], Y: p[1], Z: p[2]} }
	for i, ref := range fixture.Cases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			o := gomath.BraceOptions{Direction: geom.V(ref.Direction, 0), Buffer: ref.Buffer, Sharpness: ref.Sharpness}
			brace, err := gomath.NewArcBrace(vec(ref.Center), ref.Radius, ref.Start, ref.Sweep, o)
			must(t, err)
			tip, err := brace.Tip()
			must(t, err)
			if tip.Sub(vec(ref.Tip)).Norm() > 2e-11 {
				t.Fatal("arc brace tip differs", tip, ref.Tip)
			}
			direction, err := brace.Direction()
			must(t, err)
			if direction.Sub(vec(ref.DirectionResult)).Norm() > 2e-8 {
				t.Fatal("arc brace direction differs", direction, ref.DirectionResult)
			}
			checkConnectionTransformStates(t, brace.Object, ref.States)
		})
	}
}

func TestArcBraceRadiusFloorScalingAndInterval(t *testing.T) {
	o := gomath.DefaultArcBraceOptions()
	center := geom.Vec3{X: .4, Y: -.3, Z: .7}
	for _, radius := range []float64{.1, .2, .3, .5, 1, 2, 10} {
		brace, err := gomath.NewArcBrace(center, radius, -.8, 1.6, o)
		must(t, err)
		reverse, err := gomath.NewArcBrace(center, radius, .8, -1.6, o)
		must(t, err)
		if !reflect.DeepEqual(brace.Object.Path, reverse.Object.Path) {
			t.Fatal("reversing sweep changed fitted interval")
		}
		// Check the independent anchor radii. Before exp the right-facing
		// outline's closest anchor is at buff, farthest at buff+.273-ish.
		rmin, rmax := math.Inf(1), 0.
		for _, c := range brace.Object.Path.Contours[0].Curves {
			for _, p := range []geom.Vec3{c.P0, c.P3} {
				r := math.Hypot(p.X-center.X, p.Y-center.Y)
				rmin, rmax = math.Min(rmin, r), math.Max(rmax, r)
				if p.Z != center.Z {
					t.Fatal("wrong output Z plane")
				}
			}
		}
		scale := math.Max(1, radius)
		if math.Abs(rmin-math.Max(.3, radius)*math.Exp(o.Buffer/scale)) > 1e-10 || rmax <= rmin {
			t.Fatal("wrong radial floor or buffer scaling", radius, rmin, rmax)
		}
	}
	a, err := gomath.NewArcBrace(center, .1, -1, 2, o)
	must(t, err)
	b, err := gomath.NewArcBrace(center, .3, -1, 2, o)
	must(t, err)
	if !reflect.DeepEqual(a.Object.Path, b.Object.Path) {
		t.Fatal("documented radius floor differs")
	}
	// An affine center translation is applied after exp, including nonzero Z.
	base, err := gomath.NewArcBrace(geom.Vec3{}, 2, -1, 2, o)
	must(t, err)
	translated, err := gomath.NewArcBrace(center, 2, -1, 2, o)
	must(t, err)
	if !reflect.DeepEqual(base.Object.Copy().Shift(center).Path, translated.Object.Path) {
		t.Fatal("explicit center translation differs")
	}
}

func TestArcBraceInvalidInputsAndIndependentSnapshots(t *testing.T) {
	o := gomath.DefaultArcBraceOptions()
	for _, mutate := range []func(*gomath.BraceOptions){
		func(o *gomath.BraceOptions) { o.Direction = geom.Vec3{} },
		func(o *gomath.BraceOptions) { o.Direction.Y = 1 },
		func(o *gomath.BraceOptions) { o.Direction.Z = 1 },
		func(o *gomath.BraceOptions) { o.Buffer = math.Inf(1) },
		func(o *gomath.BraceOptions) { o.Buffer = 100 },
		func(o *gomath.BraceOptions) { o.Buffer = -1 },
		func(o *gomath.BraceOptions) { o.Sharpness = 0 },
	} {
		bad := o
		mutate(&bad)
		if b, err := gomath.NewArcBrace(geom.Vec3{}, 1, -1, 2, bad); b != nil || err == nil {
			t.Fatal("bad arc brace options accepted", bad)
		}
	}
	for _, args := range [][3]float64{{0, -1, 2}, {1e6 + 1, -1, 2}, {math.NaN(), -1, 2}, {1, math.Inf(1), 2}, {1, 1e6, 1}, {1, -1, 0}, {1, -1, 2*math.Pi + 1e-6}, {1, -1, math.NaN()}} {
		if b, err := gomath.NewArcBrace(geom.Vec3{}, args[0], args[1], args[2], o); b != nil || err == nil {
			t.Fatal("bad circle inputs accepted", args)
		}
	}
	for _, center := range []geom.Vec3{{X: math.NaN()}, {X: 1e12}, {Z: 1e12 + 1}} {
		if b, err := gomath.NewArcBrace(center, 1, -1, 2, o); b != nil || err == nil {
			t.Fatal("bad/unrepresentable center accepted", center)
		}
	}
	brace, err := gomath.NewArcBrace(geom.Vec3{}, 1, -1, 2, o)
	must(t, err)
	tip, err := brace.Tip()
	must(t, err)
	scene := gomath.NewScene()
	must(t, scene.Add(brace.Object))
	must(t, scene.Play(1, gomath.Transform(brace.Object, brace.Object.Copy().Shift(geom.V(1, .3))).WithRate(gomath.Linear)))
	finalTip, err := brace.Tip()
	must(t, err)
	if finalTip.Sub(tip.Add(geom.V(1, .3))).Norm() > 1e-12 {
		t.Fatal("arc brace tip lost identity")
	}
	baseline, err := scene.Sample(.37)
	must(t, err)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for _, at := range []float64{.9, .1, .37, .6} {
				frame, err := scene.Sample(at)
				if err != nil {
					t.Error(err)
					return
				}
				if at == .37 && !reflect.DeepEqual(frame, baseline) {
					t.Error("concurrent arc brace frame differs")
				}
				frame.Objects[0].Path.Contours[0].Curves[0].P1.X += 50
			}
		}()
	}
	wg.Wait()
	frame, err := scene.Sample(.37)
	must(t, err)
	if !reflect.DeepEqual(frame, baseline) {
		t.Fatal("arc brace frame aliases")
	}
}
