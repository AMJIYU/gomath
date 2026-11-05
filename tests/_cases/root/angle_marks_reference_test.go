package gomath_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"gomath"
	"gomath/geom"
	"math"
	"os"
	"testing"
)

func TestPinnedManimAngleMarkTransformStates(t *testing.T) {
	var fixture struct {
		Source struct {
			Version, Commit string
			Hash            string `json:"generator_sha256"`
		}
		Cases []struct {
			A, B, C, D [3]float64
			Radius     float64
			Quadrant   [2]int
			Kind       string
			Sweep      float64
			States     []struct {
				Alpha    float64
				Points   [][3]float64
				Children [][][3]float64
			}
		}
	}
	data, err := os.ReadFile("tests/data/root/manim-angle-marks-0.22.1.json")
	must(t, err)
	must(t, json.Unmarshal(data, &fixture))
	generator, err := os.ReadFile("tests/reference/angle_marks.py")
	must(t, err)
	hash := sha256.Sum256(generator)
	if fixture.Source.Version != "0.22.1" || fixture.Source.Commit != "16ea443e2a5f1d79db0a721161411ce916d6e733" || fixture.Source.Hash != hex.EncodeToString(hash[:]) || len(fixture.Cases) != 64 {
		t.Fatal("angle reference provenance or case count differs")
	}
	vector := func(p [3]float64) geom.Vec3 { return geom.Vec3{X: p[0], Y: p[1], Z: p[2]} }
	check := func(t *testing.T, object *gomath.Object, points [][3]float64) {
		t.Helper()
		var got []geom.Vec3
		for _, c := range object.Path.Contours {
			for _, k := range c.Curves {
				got = append(got, k.P0, k.P1, k.P2, k.P3)
			}
		}
		if len(got) != len(points) {
			t.Fatalf("angle control-point count: got %d, want %d", len(got), len(points))
		}
		for i, p := range points {
			if got[i].Sub(vector(p)).Norm() > 2e-12 {
				t.Fatalf("angle control point %d: got %v, want %v", i, got[i], p)
			}
		}
	}
	for i, ref := range fixture.Cases {
		t.Run(ref.Kind+"/"+string(rune('A'+i)), func(t *testing.T) {
			opts := gomath.DefaultAngleOptions()
			opts.Radius, opts.Quadrant = ref.Radius, ref.Quadrant
			opts.OtherAngle, opts.Dot, opts.Elbow = ref.Kind == "cw", ref.Kind == "dot", ref.Kind == "elbow"
			angle, err := gomath.NewAngle(gomath.Line(vector(ref.A), vector(ref.B)), gomath.Line(vector(ref.C), vector(ref.D)), opts)
			must(t, err)
			if ref.Kind != "elbow" && math.Abs(angle.Sweep-ref.Sweep) > 2e-12 {
				t.Fatalf("angle sweep %g != %g", angle.Sweep, ref.Sweep)
			}
			target := angle.Object.Copy().ApplyMatrix(geom.Translation(geom.V(1.2, -.7)).Mul(geom.RotationZ(.65)).Mul(geom.Scaling(1.3, 1.3, 1.3)))
			s := gomath.NewScene()
			must(t, s.Add(angle.Object))
			must(t, s.Play(1, gomath.Transform(angle.Object, target).WithRate(gomath.Linear)))
			for _, state := range ref.States {
				frame, err := s.Sample(state.Alpha)
				must(t, err)
				if len(frame.Objects) != 1 || len(frame.Objects[0].Children) != len(state.Children) {
					t.Fatal("angle/dot frame topology differs")
				}
				check(t, frame.Objects[0], state.Points)
				for j, points := range state.Children {
					check(t, frame.Objects[0].Children[j], points)
				}
			}
		})
	}
}
