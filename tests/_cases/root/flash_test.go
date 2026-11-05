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
	"testing"
)

func TestPinnedManimFlashAndUncreateEndpoints(t *testing.T) {
	data, err := os.ReadFile("tests/data/root/manim-flash-0.22.1.json")
	must(t, err)
	var fixture struct {
		Source struct {
			Version, Commit string
			Hash            string `json:"generator_sha256"`
		}
		Cases []struct {
			Kind       string
			Alpha      float64
			Start, End []float64
			Active     bool
		}
		Cleanup float64 `json:"fade_cleanup_opacity"`
	}
	must(t, json.Unmarshal(data, &fixture))
	generator, err := os.ReadFile("tests/reference/flash.py")
	must(t, err)
	hash := sha256.Sum256(generator)
	if fixture.Source.Version != "0.22.1" || fixture.Source.Commit != "16ea443e2a5f1d79db0a721161411ce916d6e733" || fixture.Source.Hash != hex.EncodeToString(hash[:]) || len(fixture.Cases) != 27 {
		t.Fatal("invalid partial-animation provenance/count")
	}
	for _, c := range fixture.Cases {
		t.Run(fmt.Sprintf("%s/%g", c.Kind, c.Alpha), func(t *testing.T) {
			o := gomath.Rectangle(4, 2)
			s := gomath.NewScene()
			var a gomath.Animation
			if c.Kind == "uncreate" {
				must(t, s.Add(o))
				a = gomath.Uncreate(o)
			} else if c.Kind == "flash_linear" {
				a = gomath.PassingFlashWithWidth(o, .2).WithRate(gomath.Linear)
			} else {
				a = gomath.PassingFlashWithWidth(o, .4)
			}
			must(t, s.Play(1, a))
			f, err := s.Sample(c.Alpha)
			must(t, err)
			if c.Alpha == 1 {
				if f.Find(o.ID) != nil {
					t.Fatal("remover retained target")
				}
				return
			}
			visible := f.Find(o.ID)
			p := visible.Path.Partial(visible.Style.Reveal)
			if !c.Active {
				if length, err := p.Length(1e-8); err != nil || length > 1e-9 {
					t.Fatal("inactive flash visible", length, err)
				}
				return
			}
			if len(p.Contours) != 1 || len(p.Contours[0].Curves) == 0 {
				t.Fatal("active partial path missing")
			}
			contour := p.Contours[0]
			for j, v := range []geom.Vec3{contour.Start, contour.Curves[len(contour.Curves)-1].P3} {
				want := [][]float64{c.Start, c.End}[j]
				if len(want) != 3 || v.Sub(geom.Vec3{X: want[0], Y: want[1], Z: want[2]}).Norm() > 1e-9 {
					t.Fatal("partial endpoint differs", v, want)
				}
			}
		})
	}
	o := gomath.Rectangle(4, 2).SetOpacity(.4)
	s := gomath.NewScene()
	must(t, s.Add(o))
	must(t, s.Play(1, gomath.FadeOut(o)))
	near(t, o.Style.Opacity, fixture.Cleanup)
	must(t, s.Play(1, gomath.FadeIn(o)))
	near(t, sample(t, s, 2).Find(o.ID).Style.Opacity, .4)
}

func TestFlashCleanupIdentityReplayAndValidation(t *testing.T) {
	child := gomath.Line(geom.V(-2, 0), geom.V(2, 0))
	o := gomath.Group(child)
	before := o.Copy().Path
	childPath := child.Path.Clone()
	s := gomath.NewScene()
	must(t, s.Play(1, gomath.ShowPassingFlash(o)))
	if !reflect.DeepEqual(child.Path, childPath) || !reflect.DeepEqual(o.Path, before) || o.Children[0] != child {
		t.Fatal("flash cleanup did not restore original geometry/refs")
	}
	replay, err := gomath.CompileReplay(s, gomath.DefaultReplayOptions())
	must(t, err)
	o.Shift(geom.V(10, 0))
	f, err := replay.Sample(.5)
	must(t, err)
	if f.Find(child.ID).Bounds().Max.X > 2.01 {
		t.Fatal("later authoring leaked into replay")
	}
	for _, width := range []float64{0, -1, math.NaN(), math.Inf(1), 1e7} {
		fresh := gomath.NewScene()
		if fresh.Play(1, gomath.PassingFlashWithWidth(child, width)) == nil || fresh.Duration() != 0 {
			t.Fatal("invalid flash accepted or changed timeline", width)
		}
	}
	if gomath.NewScene().Play(1, gomath.ShowPassingFlash(nil)) == nil || gomath.NewScene().Play(1, gomath.Uncreate(child)) == nil {
		t.Fatal("invalid target accepted")
	}
}
