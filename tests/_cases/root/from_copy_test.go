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
	"testing"
)

func TestTransformFromCopyPreservesSourceAndTargetIdentity(t *testing.T) {
	s := gomath.NewScene()
	source := gomath.Rectangle(4, 2).MoveTo(geom.V(-2, 0))
	target := gomath.Rectangle(2, 4).MoveTo(geom.V(2, 1))
	originalSource, originalTarget := source.Copy().Path, target.Copy().Path
	must(t, s.Add(source))
	must(t, s.Play(2, gomath.TransformFromCopy(source, target).WithRate(gomath.Linear)))
	if !reflect.DeepEqual(source.Path, originalSource) || !reflect.DeepEqual(target.Path, originalTarget) {
		t.Fatal("copy transform changed original source or final target geometry")
	}
	f := sample(t, s, 1)
	near(t, f.Find(source.ID).Center().X, -2)
	near(t, f.Find(target.ID).Center().X, 0)
	near(t, f.Find(target.ID).Center().Y, .5)
	must(t, s.Play(1, gomath.Shift(target, geom.V(1, 0))))
	near(t, target.Center().X, 3)
	// A copy transform can read a source moved by an earlier sequence step.
	two := gomath.Square(1).MoveTo(geom.V(5, 0))
	must(t, s.Play(2, gomath.Succession(gomath.Shift(source, geom.V(2, 0)), gomath.TransformFromCopy(source, two))))
	near(t, sample(t, s, 4).Find(two.ID).Center().X, 0)
	near(t, sample(t, s, 4.5).Find(two.ID).Center().X, 2.5)
	near(t, two.Center().X, 5)
	near(t, source.Center().X, 0)
}

func TestTransformFromCopyNestedPointersAndInvalidInput(t *testing.T) {
	source := gomath.Group(gomath.Square(1).Shift(geom.V(-2, 0)), gomath.Square(1))
	child := gomath.Square(1).Shift(geom.V(2, 0))
	target := gomath.Group(child)
	s := gomath.NewScene()
	must(t, s.Add(source))
	must(t, s.Play(1, gomath.TransformFromCopy(source, target)))
	if target.Children[0] != child || len(target.Children) != 2 {
		t.Fatal("copy transform lost existing child identity or alignment placeholders")
	}
	if target.Children[1].ID == source.Children[1].ID {
		t.Fatal("copied target reused source identity")
	}
	if err := s.Wait(.1); err != nil {
		t.Fatal(err)
	}
	for _, a := range []gomath.Animation{gomath.TransformFromCopy(nil, target), gomath.TransformFromCopy(source, nil)} {
		before := sample(t, s, s.Duration())
		if s.Play(1, a) == nil {
			t.Fatal("nil copy source/target accepted")
		}
		if !reflect.DeepEqual(before, sample(t, s, s.Duration())) {
			t.Fatal("invalid copy transform committed")
		}
	}
}

func TestPinnedManimTransformFromCopy(t *testing.T) {
	data, err := os.ReadFile("tests/data/root/manim-from-copy-0.22.1.json")
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
	generator, err := os.ReadFile("tests/reference/from_copy.py")
	must(t, err)
	hash := sha256.Sum256(generator)
	if fixture.Source.Version != "0.22.1" || fixture.Source.Commit != "16ea443e2a5f1d79db0a721161411ce916d6e733" || fixture.Source.Hash != hex.EncodeToString(hash[:]) || len(fixture.Cases) != 44 {
		t.Fatal("copy transform provenance/count differs")
	}
	for _, c := range fixture.Cases {
		s := gomath.NewScene()
		source := gomath.Rectangle(4, 2).MoveTo(geom.V(-2, 0))
		target := gomath.Rectangle(2, 4).MoveTo(geom.V(2, 1))
		must(t, s.Add(source))
		rate := gomath.Linear
		if c.Kind == "quad" || c.Kind == "grouped" {
			rate = gomath.EaseInQuad
		}
		if c.Kind == "back" {
			rate = gomath.EaseOutBack
		}
		animation := gomath.TransformFromCopy(source, target).WithRate(rate)
		if c.Kind == "grouped" {
			animation = gomath.AnimationGroup(animation.WithWindow(.2, .8)).WithRate(gomath.EaseInQuad)
		}
		must(t, s.Play(1, animation))
		got := sample(t, s, c.Alpha).Find(target.ID)
		var points []geom.Vec3
		for _, b := range got.Path.Contours[0].Curves {
			points = append(points, b.P0, b.P1, b.P2, b.P3)
		}
		if len(points) != len(c.Points) {
			t.Fatal("copy transform reference topology differs")
		}
		for i, p := range points {
			for j, v := range []float64{p.X, p.Y, p.Z} {
				if math.Abs(v-c.Points[i][j]) > 1e-10 {
					t.Fatalf("%s at %g point%d axis%d got%g want%g", c.Kind, c.Alpha, i, j, v, c.Points[i][j])
				}
			}
		}
		if !reflect.DeepEqual(source.Path, sample(t, s, c.Alpha).Find(source.ID).Path) {
			t.Fatal("source did not survive copy transform unchanged")
		}
	}
}
