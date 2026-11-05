package gomath_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"gomath"
	"gomath/geom"
	"math"
	"os"
	"strconv"
	"testing"
)

type connectionReferenceState struct {
	Alpha  float64
	Points [][3]float64
}

func checkConnectionTransformStates(t *testing.T, object *gomath.Object, states []connectionReferenceState) {
	t.Helper()
	if len(states) != 7 {
		t.Fatal("expected seven pinned animation states")
	}
	target := object.Copy().ApplyMatrix(geom.Translation(geom.Vec3{X: .6, Y: -.4, Z: .2}).Mul(geom.RotationZ(-.4)).Mul(geom.Scaling(1.1, 1.1, 1.1)))
	s := gomath.NewScene()
	must(t, s.Add(object))
	must(t, s.Play(1, gomath.Transform(object, target).WithRate(gomath.Linear)))
	for _, state := range states {
		frame, err := s.Sample(state.Alpha)
		must(t, err)
		var got []geom.Vec3
		for _, contour := range frame.Objects[0].Path.Contours {
			for _, curve := range contour.Curves {
				got = append(got, curve.P0, curve.P1, curve.P2, curve.P3)
			}
		}
		if len(got) != len(state.Points) {
			t.Fatal("connection topology differs", len(got), len(state.Points))
		}
		for j, point := range state.Points {
			want := geom.Vec3{X: point[0], Y: point[1], Z: point[2]}
			if got[j].Sub(want).Norm() > 2e-11 {
				t.Fatalf("point%d, alpha%g: got%v, want%v", j, state.Alpha, got[j], want)
			}
		}
	}
}

func TestPinnedManimTangentialArcTransformStates(t *testing.T) {
	var fixture struct {
		Source struct {
			Version, Commit string
			Hash            string `json:"generator_sha256"`
		}
		Cases []struct {
			A, B, C, D [3]float64
			Radius     float64
			Corner     [2]int
			States     []connectionReferenceState
		} `json:"tangential_cases"`
	}
	data, err := os.ReadFile("tests/data/root/manim-arc-connections-0.22.1.json")
	must(t, err)
	must(t, json.Unmarshal(data, &fixture))
	generator, err := os.ReadFile("tests/reference/arc_connections.py")
	must(t, err)
	hash := sha256.Sum256(generator)
	if fixture.Source.Version != "0.22.1" || fixture.Source.Commit != "16ea443e2a5f1d79db0a721161411ce916d6e733" || fixture.Source.Hash != hex.EncodeToString(hash[:]) || len(fixture.Cases) != 16 {
		t.Fatal("tangential arc provenance or count differs")
	}
	vector := func(p [3]float64) geom.Vec3 { return geom.Vec3{X: p[0], Y: p[1], Z: p[2]} }
	for i, ref := range fixture.Cases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			arc, err := gomath.NewTangentialArc(gomath.Line(vector(ref.A), vector(ref.B)), gomath.Line(vector(ref.C), vector(ref.D)), gomath.TangentialArcOptions{Radius: ref.Radius, Corner: ref.Corner})
			must(t, err)
			checkConnectionTransformStates(t, arc.Object, ref.States)
		})
	}
}

func TestPinnedManimArcConnectionTransformStates(t *testing.T) {
	var fixture struct {
		Source struct {
			Version, Commit string
			Hash            string `json:"generator_sha256"`
		}
		Cases []struct {
			Start, End [3]float64
			Options    struct{ Angle, Radius float64 }
			Sweep      float64
			States     []struct {
				Alpha  float64
				Points [][3]float64
			}
		}
	}
	data, err := os.ReadFile("tests/data/root/manim-arc-connections-0.22.1.json")
	must(t, err)
	must(t, json.Unmarshal(data, &fixture))
	generator, err := os.ReadFile("tests/reference/arc_connections.py")
	must(t, err)
	hash := sha256.Sum256(generator)
	if fixture.Source.Version != "0.22.1" || fixture.Source.Commit != "16ea443e2a5f1d79db0a721161411ce916d6e733" || fixture.Source.Hash != hex.EncodeToString(hash[:]) || len(fixture.Cases) != 33 {
		t.Fatal("arc connection provenance or count differs")
	}
	vector := func(p [3]float64) geom.Vec3 { return geom.Vec3{X: p[0], Y: p[1], Z: p[2]} }
	for i, ref := range fixture.Cases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			arc, err := gomath.NewArcBetweenPoints(vector(ref.Start), vector(ref.End), gomath.ArcBetweenPointsOptions{Angle: ref.Options.Angle, Radius: ref.Options.Radius})
			must(t, err)
			if math.Abs(arc.Sweep-ref.Sweep) > 2e-12 {
				t.Fatal("arc sweep differs", arc.Sweep, ref.Sweep)
			}
			target := arc.Object.Copy().ApplyMatrix(geom.Translation(geom.Vec3{X: .6, Y: -.4, Z: .2}).Mul(geom.RotationZ(-.4)).Mul(geom.Scaling(1.1, 1.1, 1.1)))
			s := gomath.NewScene()
			must(t, s.Add(arc.Object))
			must(t, s.Play(1, gomath.Transform(arc.Object, target).WithRate(gomath.Linear)))
			for _, state := range ref.States {
				frame, err := s.Sample(state.Alpha)
				must(t, err)
				var got []geom.Vec3
				for _, c := range frame.Objects[0].Path.Contours {
					for _, k := range c.Curves {
						got = append(got, k.P0, k.P1, k.P2, k.P3)
					}
				}
				if len(got) != len(state.Points) {
					t.Fatal("arc connection topology differs", len(got), len(state.Points))
				}
				for j, point := range state.Points {
					if got[j].Sub(vector(point)).Norm() > 2e-11 {
						t.Fatalf("arc point%d, alpha%g: got%v, want%v", j, state.Alpha, got[j], point)
					}
				}
			}
		})
	}
}
