package gomath_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"gomath"
	"gomath/geom"
	"os"
	"strconv"
	"testing"
)

func TestPinnedManimAnnotationTransformControlSets(t *testing.T) {
	var fixture struct {
		Source struct {
			Version, Commit string
			Hash            string `json:"generator_sha256"`
		}
		Cases []struct {
			Kind  string
			Parts []struct {
				Points [][3]float64
				Closed bool
			}
			PaddingX      float64 `json:"padding_x"`
			PaddingY      float64 `json:"padding_y"`
			Buffer, Scale float64
			States        []connectionReferenceState
		}
	}
	data, err := os.ReadFile("tests/data/root/manim-annotations-0.22.1.json")
	must(t, err)
	must(t, json.Unmarshal(data, &fixture))
	generator, err := os.ReadFile("tests/reference/annotations.py")
	must(t, err)
	hash := sha256.Sum256(generator)
	if fixture.Source.Version != "0.22.1" || fixture.Source.Commit != "16ea443e2a5f1d79db0a721161411ce916d6e733" || fixture.Source.Hash != hex.EncodeToString(hash[:]) || len(fixture.Cases) != 48 {
		t.Fatal("annotation provenance or case count differs")
	}
	vector := func(v [3]float64) geom.Vec3 { return geom.Vec3{X: v[0], Y: v[1], Z: v[2]} }
	for i, ref := range fixture.Cases {
		t.Run(ref.Kind+strconv.Itoa(i), func(t *testing.T) {
			var parts []*gomath.Object
			for _, part := range ref.Parts {
				points := make([]geom.Vec3, len(part.Points))
				for j, p := range part.Points {
					points[j] = vector(p)
				}
				object := gomath.Polyline(points...)
				if part.Closed {
					object = gomath.Polygon(points...)
				}
				parts = append(parts, object)
			}
			target := gomath.Group(parts...)
			var object *gomath.Object
			options := gomath.SurroundingRectangleOptions{PaddingX: ref.PaddingX, PaddingY: ref.PaddingY}
			var err error
			switch ref.Kind {
			case "surrounding":
				object, err = gomath.NewSurroundingRectangle(target, options)
			case "background":
				object, err = gomath.NewBackgroundRectangle(target, gomath.BackgroundRectangleOptions{Rectangle: options, Fill: gomath.Color{A: .75}})
			case "underline":
				object, err = gomath.NewUnderline(target, gomath.UnderlineOptions{Buffer: ref.Buffer})
			case "cross":
				object, err = gomath.NewCross(target, gomath.CrossOptions{Scale: ref.Scale})
			default:
				t.Fatal("unknown reference kind")
			}
			must(t, err)
			destination := object.Copy().ApplyMatrix(geom.Translation(geom.Vec3{X: .6, Y: -.4, Z: .2}).Mul(geom.RotationZ(-.4)).Mul(geom.Scaling(1.1, 1.1, 1.1)))
			s := gomath.NewScene()
			must(t, s.Add(object))
			must(t, s.Play(1, gomath.Transform(object, destination).WithRate(gomath.Linear)))
			if len(ref.States) != 7 {
				t.Fatal("expected seven pinned animation states")
			}
			for _, state := range ref.States {
				frame, err := s.Sample(state.Alpha)
				must(t, err)
				var got []geom.Vec3
				frame.Objects[0].Walk(func(o *gomath.Object) {
					for _, c := range o.Path.Contours {
						for _, k := range c.Curves {
							got = append(got, k.P0, k.P1, k.P2, k.P3)
						}
					}
				})
				if len(got) != len(state.Points) {
					t.Fatal("annotation control count differs", len(got), len(state.Points))
				}
				used := make([]bool, len(got))
				for _, point := range state.Points {
					match := -1
					for j, p := range got {
						if !used[j] && p.Sub(vector(point)).Norm() < 2e-11 {
							match = j
							break
						}
					}
					if match < 0 {
						t.Fatal("annotation control multiset differs", state.Alpha, point)
					}
					used[match] = true
				}
			}
		})
	}
}
