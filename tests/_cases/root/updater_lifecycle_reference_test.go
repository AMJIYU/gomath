package gomath_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"gomath"
	"gomath/geom"
	"gomath/path"
	"os"
	"testing"
)

func TestPinnedManimObjectUpdaterSuspensionStates(t *testing.T) {
	var fixture struct {
		Source struct {
			Version, Commit string
			Hash            string `json:"generator_sha256"`
		}
		Cases []struct {
			Kind      string
			Automatic bool
			States    []connectionReferenceState
		}
	}
	data, e := os.ReadFile("tests/data/root/updater-lifecycle/manim-0.22.1-verified.json")
	must(t, e)
	must(t, json.Unmarshal(data, &fixture))
	generator, e := os.ReadFile("tests/reference/updater_lifecycle.py")
	must(t, e)
	sum := sha256.Sum256(generator)
	if fixture.Source.Version != "0.22.1" || fixture.Source.Commit != "16ea443e2a5f1d79db0a721161411ce916d6e733" || fixture.Source.Hash != hex.EncodeToString(sum[:]) || len(fixture.Cases) != 13 {
		t.Fatal("updater source provenance/count mismatch")
	}
	rectangle := func(x float64) *gomath.Object {
		var b path.Builder
		b.MoveTo(geom.V(-.5, -.35))
		b.LineTo(geom.V(.5, -.35))
		b.LineTo(geom.V(.5, .35))
		b.LineTo(geom.V(-.5, .35))
		b.Close()
		return gomath.NewObject(b.Path()).Shift(geom.V(x, 0))
	}
	for _, c := range fixture.Cases {
		t.Run(c.Kind+map[bool]string{true: "/automatic", false: "/live"}[c.Automatic], func(t *testing.T) {
			s := gomath.NewScene()
			a, b := rectangle(-1), rectangle(1)
			parent := gomath.Group(a, b)
			must(t, s.Add(parent))
			target := a
			if c.Kind == "read-target" {
				target = b
			}
			if c.Kind == "parent-child" {
				target = parent
			}
			options := gomath.DefaultObjectUpdaterOptions()
			options.SuspendDuringAnimations = c.Automatic
			h := managedUpdater(t, s, target, func(o *gomath.Object, _ *gomath.Frame, time float64) error {
				if c.Kind == "parent-child" {
					o.Shift(geom.V(.2*time, .1*time))
				} else {
					o.MoveTo(geom.V(time, .2))
				}
				return nil
			}, options)
			if c.Kind == "ordered" {
				managedUpdater(t, s, target, func(o *gomath.Object, _ *gomath.Frame, time float64) error { o.Shift(geom.V(.3*time, .1)); return nil }, options)
			}
			switch c.Kind {
			case "manual-suspended":
				must(t, s.SuspendUpdater(h))
				must(t, s.Wait(1))
			case "manual-removed":
				must(t, s.RemoveUpdater(h))
				must(t, s.Wait(1))
			case "no-animation":
				must(t, s.Wait(1))
			default:
				subject := a
				if c.Kind == "child-parent" {
					subject = parent
				}
				destination := subject.Copy().Shift(geom.V(.8, .4))
				if c.Kind == "read-target" {
					destination = b
				}
				must(t, s.Play(1, gomath.Transform(subject, destination).WithRate(gomath.Linear)))
			}
			if len(c.States) != 7 {
				t.Fatal("missing interior states")
			}
			for _, state := range c.States {
				f := sample(t, s, state.Alpha)
				var got []geom.Vec3
				f.Objects[0].Walk(func(o *gomath.Object) {
					for _, contour := range o.Path.Contours {
						for _, curve := range contour.Curves {
							got = append(got, curve.P0, curve.P1, curve.P2, curve.P3)
						}
					}
				})
				if len(got) != len(state.Points) {
					t.Fatal("topology differs", len(got), len(state.Points))
				}
				for j, p := range state.Points {
					want := geom.Vec3{X: p[0], Y: p[1], Z: p[2]}
					if got[j].Sub(want).Norm() > 2e-11 {
						t.Fatalf("alpha%g point%d got%v want%v", state.Alpha, j, got[j], want)
					}
				}
			}
		})
	}
}
