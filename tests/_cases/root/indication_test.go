package gomath_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"gomath"
	"gomath/geom"
	"math"
	"os"
	"reflect"
	"sync"
	"testing"
)

func TestPinnedManimGrowingAndIndication(t *testing.T) {
	data, err := os.ReadFile("tests/data/root/manim-indication-0.22.1.json")
	must(t, err)
	var fixture struct {
		Source struct {
			Version, Commit string
			Hash            string `json:"generator_sha256"`
		}
		Cases []struct {
			Kind   string
			Alpha  float64
			Points [][]float64
		}
	}
	must(t, json.Unmarshal(data, &fixture))
	generator, err := os.ReadFile("tests/reference/indication.py")
	must(t, err)
	hash := sha256.Sum256(generator)
	if fixture.Source.Version != "0.22.1" || fixture.Source.Commit != "16ea443e2a5f1d79db0a721161411ce916d6e733" || fixture.Source.Hash != hex.EncodeToString(hash[:]) || len(fixture.Cases) != 88 {
		t.Fatal("invalid indication provenance/count")
	}
	for _, c := range fixture.Cases {
		t.Run(fmt.Sprintf("%s/%g", c.Kind, c.Alpha), func(t *testing.T) {
			o := gomath.Rectangle(4, 2).MoveTo(geom.V(1, .5))
			s := gomath.NewScene()
			var a gomath.Animation
			switch c.Kind {
			case "point":
				a = gomath.GrowFromPoint(o, geom.V(-3, 2))
			case "center":
				a = gomath.GrowFromCenter(o)
			case "edge":
				a = gomath.GrowFromEdge(o, geom.V(1, 1))
			case "spin":
				a = gomath.SpinIn(o, 2*math.Pi)
			case "indicate":
				must(t, s.Add(o))
				a = gomath.Indicate(o)
			case "wiggle":
				must(t, s.Add(o))
				a = gomath.WiggleObject(o)
			case "wave":
				must(t, s.Add(o))
				a = gomath.ApplyWave(o)
			case "wave_ripples":
				must(t, s.Add(o))
				opt := gomath.DefaultWaveOptions()
				opt.Direction = geom.V(1, 2)
				opt.Amplitude = .4
				opt.TimeWidth = .5
				opt.Ripples = 3
				a = gomath.ApplyWaveWith(o, opt).WithRate(gomath.Linear)
			default:
				t.Fatal("unknown reference kind")
			}
			must(t, s.Play(1, a))
			actual := sample(t, s, c.Alpha).Find(o.ID)
			var points []geom.Vec3
			for _, contour := range actual.Path.Contours {
				for _, curve := range contour.Curves {
					points = append(points, curve.P0, curve.P1, curve.P2, curve.P3)
				}
			}
			if len(points) != len(c.Points) {
				t.Fatal("point count mismatch")
			}
			for i, p := range points {
				for axis, got := range []float64{p.X, p.Y, p.Z} {
					if math.Abs(got-c.Points[i][axis]) > 1e-9 {
						t.Fatalf("point %d axis %d: got %.17g want %.17g", i, axis, got, c.Points[i][axis])
					}
				}
			}
		})
	}
}

func TestGrowingIdentityColorAndConcurrentEffects(t *testing.T) {
	child := gomath.Square(1).SetFill(gomath.Blue)
	group := gomath.Group(child)
	s := gomath.NewScene()
	must(t, s.Play(1, gomath.GrowFromPoint(group, geom.V(-2, 3)).WithPointColor(gomath.Red).WithRate(gomath.Linear)))
	if group.Children[0] != child {
		t.Fatal("growing replaced existing child")
	}
	initial := sample(t, s, 0).Find(child.ID)
	near(t, initial.Center().X, -2)
	near(t, initial.Center().Y, 3)
	if initial.Style.Fill != gomath.Red || child.Style.Fill != gomath.Blue {
		t.Fatal("point/target color lost")
	}
	opt := gomath.DefaultWiggleOptions()
	point := geom.V(1, 1)
	opt.ScaleCenter, opt.RotationCenter = &point, &point
	a := gomath.WiggleObjectWith(group, opt)
	point = geom.V(999, 999)
	must(t, s.Play(1, a))
	must(t, s.Play(1, gomath.ApplyWave(group)))
	replay, err := gomath.CompileReplay(s, gomath.DefaultReplayOptions())
	must(t, err)
	for _, time := range []float64{.2, 1.4, 2.7} {
		want := sample(t, s, time)
		var wg sync.WaitGroup
		for range 8 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				got, err := replay.Sample(time)
				if err != nil || !reflect.DeepEqual(got, want) {
					t.Error("effect replay mismatch", err)
				}
			}()
		}
		wg.Wait()
	}
	arrow := gomath.Arrow(geom.V(-2, 0), geom.V(2, 0), .4)
	s = gomath.NewScene()
	must(t, s.Play(1, gomath.GrowArrow(arrow)))
	for _, o := range sample(t, s, 0).Objects[0].Children {
		near(t, o.Center().X, -2)
		near(t, o.Center().Y, 0)
	}
}

func TestIndicationValidationAndFailures(t *testing.T) {
	o := gomath.Square(1)
	wave := gomath.DefaultWaveOptions()
	wave.TimeWidth = 0
	wiggle := gomath.DefaultWiggleOptions()
	wiggle.Scale = -1
	invalid := []gomath.Animation{
		gomath.GrowFromPoint(nil, geom.V(0, 0)), gomath.GrowFromPoint(o, geom.V(math.NaN(), 0)),
		gomath.GrowFromEdge(o, geom.V(math.Inf(1), 0)), gomath.SpinIn(o, math.NaN()),
		gomath.GrowFromCenter(o).WithPointColor(gomath.RGB(math.NaN(), 0, 0)),
		gomath.Shift(o, geom.V(1, 0)).WithPointColor(gomath.Red),
		gomath.WiggleObjectWith(o, wiggle), gomath.ApplyWaveWith(o, wave),
		gomath.GrowFromCenter(gomath.Group()),
	}
	for i, a := range invalid {
		s := gomath.NewScene()
		must(t, s.Add(o))
		if s.Play(1, a) == nil {
			t.Errorf("invalid effect accepted %d", i)
		}
		near(t, s.Duration(), 0)
		near(t, o.Center().X, 0)
	}
	s := gomath.NewScene()
	must(t, s.Add(o))
	wave = gomath.DefaultWaveOptions()
	wave.WaveFunc = func(float64) float64 { return math.NaN() }
	must(t, s.Play(1, gomath.ApplyWaveWith(o, wave)))
	if _, err := s.Sample(.5); err == nil {
		t.Fatal("nonfinite wave displacement ignored")
	}
}
