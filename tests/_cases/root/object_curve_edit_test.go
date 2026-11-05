package gomath_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"gomath"
	"gomath/geom"
	"gomath/path"
	"math"
	"os"
	"reflect"
	"sync"
	"testing"
)

func TestPinnedManimObjectCurveEdits(t *testing.T) {
	var fixture struct {
		Source struct {
			Version, Commit string
			Hash            string `json:"generator_sha256"`
		}
		Parts [][][][3]float64
		Cases []struct {
			Form, Mode string
			States     []connectionReferenceState
		}
	}
	data, e := os.ReadFile("tests/data/root/object-curve-edit/manim-0.22.1-verified.json")
	must(t, e)
	must(t, json.Unmarshal(data, &fixture))
	generator, e := os.ReadFile("tests/reference/object_curve_edit.py")
	must(t, e)
	h := sha256.Sum256(generator)
	if fixture.Source.Version != "0.22.1" || fixture.Source.Commit != "16ea443e2a5f1d79db0a721161411ce916d6e733" || fixture.Source.Hash != hex.EncodeToString(h[:]) || len(fixture.Cases) != 12 {
		t.Fatal("curve-family provenance mismatch")
	}
	vec := func(p [3]float64) geom.Vec3 { return geom.Vec3{X: p[0], Y: p[1], Z: p[2]} }
	for _, c := range fixture.Cases {
		t.Run(c.Form+"/"+c.Mode, func(t *testing.T) {
			var children []*gomath.Object
			for _, contours := range fixture.Parts {
				var outline path.Path
				for _, points := range contours {
					q := path.Contour{Start: vec(points[0]), Closed: points[0] == points[len(points)-1]}
					for i := 0; i < len(points); i += 4 {
						q.Curves = append(q.Curves, path.Cubic{P0: vec(points[i]), P1: vec(points[i+1]), P2: vec(points[i+2]), P3: vec(points[i+3])})
					}
					outline.Contours = append(outline.Contours, q)
				}
				children = append(children, gomath.NewObject(outline))
			}
			o := children[1]
			if c.Form == "multi" {
				o = children[0]
			}
			if c.Form == "nested" {
				o = gomath.Group(children[0], gomath.Group(children[1]))
			}
			id := o.ID
			switch c.Mode {
			case "smooth":
				must(t, o.MakeSmooth())
			case "jagged":
				must(t, o.MakeJagged())
			case "prepare11":
				must(t, o.PrepareForNonlinearTransform(11))
			case "prepare3":
				must(t, o.PrepareForNonlinearTransform(3))
			}
			target := o.Copy().ApplyMatrix(geom.Translation(geom.Vec3{X: .4, Y: -.3, Z: .1}).Mul(geom.RotationZ(.27)).Mul(geom.Scaling(1.08, 1.08, 1.08)))
			s := gomath.NewScene()
			must(t, s.Add(o))
			must(t, s.Play(1, gomath.Transform(o, target).WithRate(gomath.Linear)))
			for _, state := range c.States {
				f := sample(t, s, state.Alpha)
				var got []geom.Vec3
				f.Objects[0].Walk(func(o *gomath.Object) {
					for _, contour := range o.Path.Contours {
						for _, v := range contour.Curves {
							got = append(got, v.P0, v.P1, v.P2, v.P3)
						}
					}
				})
				if len(got) != len(state.Points) {
					t.Fatal("curve count differs", len(got), len(state.Points))
				}
				for i, p := range state.Points {
					if got[i].Sub(vec(p)).Norm() > 1e-10 {
						t.Fatalf("alpha%g point%d got%v want%v", state.Alpha, i, got[i], vec(p))
					}
				}
				if f.Objects[0].ID != id {
					t.Fatal("identity changed")
				}
			}
		})
	}
}

func TestObjectCurveEditsAtomicityPointersAndBounds(t *testing.T) {
	line := gomath.Line(geom.V(0, 0), geom.V(2, 0))
	bad := gomath.Line(geom.V(4, 0), geom.V(5, 0))
	bad.Path.Contours[0].Curves[0].P0.X += 1e-10
	root := gomath.Group(line, bad)
	before := line.Path.Clone()
	if root.PrepareForNonlinearTransform(10) == nil || root.MakeSmooth() == nil || !reflect.DeepEqual(line.Path, before) {
		t.Fatal("later invalid join published partial edit")
	}
	must(t, line.PrepareForNonlinearTransform(8))
	if len(line.Path.Contours[0].Curves) != 8 {
		t.Fatal("preparation count")
	}
	near(t, line.Center().X, 1)
	must(t, line.PrepareForNonlinearTransform(3))
	if len(line.Path.Contours[0].Curves) != 8 {
		t.Fatal("preparation removed curves")
	}
	for _, n := range []int{-1, 65537} {
		if line.PrepareForNonlinearTransform(n) == nil {
			t.Fatal("invalid count accepted")
		}
	}
	if line.SetAnchorMode(path.AnchorMode("bad")) == nil {
		t.Fatal("invalid handle mode accepted")
	}
	closed := gomath.NewObject(path.Path{Contours: []path.Contour{{Start: geom.V(0, 0), Closed: true, Curves: []path.Cubic{path.Line(geom.V(0, 0), geom.V(1, 0))}}}})
	must(t, closed.PrepareForNonlinearTransform(3))
	if closed.MakeSmooth() == nil {
		t.Fatal("implicit smoothing seam accepted")
	}
	point := gomath.NewObject(path.Path{Contours: []path.Contour{{Start: geom.V(1, 2)}}})
	must(t, point.PrepareForNonlinearTransform(9))
	if len(point.Path.Contours[0].Curves) != 9 {
		t.Fatal("point expansion")
	}
	manyPoints := gomath.NewObject(path.Path{Contours: []path.Contour{{Start: geom.V(1, 0)}, {Start: geom.V(2, 0)}}})
	if manyPoints.PrepareForNonlinearTransform(2) == nil {
		t.Fatal("ambiguous point-only allocation accepted")
	}
	curve := gomath.Circle(1)
	original := curve.Path.Clone()
	curve.Path.Contours[0].Curves[0].P1.X = math.Inf(1)
	if curve.MakeJagged() == nil {
		t.Fatal("nonfinite input accepted")
	}
	curve.Path = original
	if curve.MakeSmooth() == nil {
		t.Fatal("circle with a floating-point closing gap accepted for smoothing")
	}
	root = gomath.Group(gomath.Square(2), line)
	first, second := root.Children[0], root.Children[1]
	id1, id2 := first.ID, second.ID
	must(t, root.MakeSmooth())
	if root.Children[0] != first || root.Children[1] != second || first.ID != id1 || second.ID != id2 {
		t.Fatal("tree identity changed")
	}
	enormous := gomath.Group()
	for range 100 {
		enormous.Children = append(enormous.Children, gomath.Line(geom.V(0, 0), geom.V(1, 0)))
	}
	saved := enormous.Children[0].Path.Clone()
	if enormous.PrepareForNonlinearTransform(1000) == nil || !reflect.DeepEqual(saved, enormous.Children[0].Path) {
		t.Fatal("aggregate output budget ignored or failed nonatomically")
	}
}

func TestObjectCurveEditsIndependentCopiesAndSampling(t *testing.T) {
	c, e := path.FromAnchors([]geom.Vec3{geom.V(-1, 0), geom.V(0, 1), geom.V(1, 0)}, path.AnchorJagged)
	must(t, e)
	original := gomath.NewObject(path.Path{Contours: []path.Contour{c}})
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			o := original.Copy()
			if e := o.PrepareForNonlinearTransform(24); e != nil {
				t.Error(e)
				return
			}
			if e := o.MakeSmooth(); e != nil {
				t.Error(e)
				return
			}
			s := gomath.NewScene()
			if e := s.Add(o); e != nil {
				t.Error(e)
				return
			}
			if e := s.Play(1, gomath.Shift(o, geom.V(1, 0))); e != nil {
				t.Error(e)
				return
			}
			if _, e := s.Sample(.35); e != nil {
				t.Error(e)
			}
		})
	}
	wg.Wait()
	if len(original.Path.Contours[0].Curves) != 2 {
		t.Fatal("independent editing changed source")
	}
}
