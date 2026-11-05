package gomath_test

import (
	"encoding/json"
	"fmt"
	"gomath"
	"gomath/geom"
	"math"
	"os"
	"testing"
)

// This fixture was produced independently by the pinned Manim source. It tests
// a deliberately narrow set of intermediate states, not whole-API equivalence.
func TestPinnedManimIntermediateBehavior(t *testing.T) {
	data, err := os.ReadFile("tests/data/root/manim-animation-0.22.1.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Source struct{ Version, Commit string }
		Cases  []struct {
			Kind                string
			Alpha, Value, Width float64
			Center              []float64
			Points              [][]float64
			SourcePresent       bool `json:"source_present"`
			TargetPresent       bool `json:"target_present"`
		}
	}
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Source.Commit != "16ea443e2a5f1d79db0a721161411ce916d6e733" || len(fixture.Cases) != 35 {
		t.Fatal("incorrect reference provenance/case count")
	}
	near := func(t *testing.T, got, want float64) {
		t.Helper()
		if math.IsNaN(got) || math.Abs(got-want) > 1e-9 {
			t.Fatalf("got %.15g want %.15g", got, want)
		}
	}
	for _, c := range fixture.Cases {
		t.Run(fmt.Sprintf("%s/%.3f", c.Kind, c.Alpha), func(t *testing.T) {
			s := gomath.NewScene()
			o := gomath.Square(2).MoveTo(geom.V(-2, 0))
			var target *gomath.Object
			var animation gomath.Animation
			switch c.Kind {
			case "shift":
				animation = gomath.Shift(o, geom.V(4, 1)).WithRate(gomath.Linear)
			case "rotate":
				animation = gomath.Rotate(o, 2*math.Pi).WithRate(gomath.Linear)
			case "replacement":
				target = gomath.Square(1).MoveTo(geom.V(1, -1))
				animation = gomath.ReplacementTransform(o, target).WithRate(gomath.Linear)
			case "tracker":
				tracker, err := gomath.NewValueTracker(-2)
				if err != nil {
					t.Fatal(err)
				}
				o = tracker.Object
				animation = gomath.ValueTo(tracker, 6).WithRate(gomath.Linear)
			case "camera":
				if err = s.SetCamera(gomath.Camera2D{FrameWidth: 12, Center: geom.V(-1, .5)}); err != nil {
					t.Fatal(err)
				}
				if err = s.AnimateCameraWithRate(1, gomath.Camera2D{FrameWidth: 6, Center: geom.V(2, -1)}, gomath.Linear); err != nil {
					t.Fatal(err)
				}
				f, err := s.Sample(c.Alpha)
				if err != nil {
					t.Fatal(err)
				}
				near(t, f.Camera.FrameWidth, c.Width)
				near(t, f.Camera.Center.X, c.Center[0])
				near(t, f.Camera.Center.Y, c.Center[1])
				return
			default:
				t.Fatal("unexpected reference kind")
			}
			if err = s.Add(o); err != nil {
				t.Fatal(err)
			}
			if err = s.Play(1, animation); err != nil {
				t.Fatal(err)
			}
			f, err := s.Sample(c.Alpha)
			if err != nil {
				t.Fatal(err)
			}
			actual := f.Find(o.ID)
			if c.Kind == "tracker" {
				near(t, actual.Scalar, c.Value)
				return
			}
			if c.Kind == "replacement" {
				if (actual != nil) != c.SourcePresent || (f.Find(target.ID) != nil) != c.TargetPresent {
					t.Fatal("replacement identity differs from Manim")
				}
				if actual == nil {
					actual = f.Find(target.ID)
				}
			}
			var points []geom.Vec3
			for _, contour := range actual.Path.Contours {
				for _, curve := range contour.Curves {
					points = append(points, curve.P0, curve.P1, curve.P2, curve.P3)
				}
			}
			if len(points) != len(c.Points) {
				t.Fatalf("point count %d want %d", len(points), len(c.Points))
			}
			for i, p := range points {
				near(t, p.X, c.Points[i][0])
				near(t, p.Y, c.Points[i][1])
				near(t, p.Z, c.Points[i][2])
			}
		})
	}
}
