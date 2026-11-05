package gomath_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"gomath"
	"gomath/geom"
	"gomath/path"
	"image"
	"math"
	"math/cmplx"
	"os"
	"reflect"
	"strconv"
	"sync"
	"testing"
)

func pointwiseMapper(kind string) func(geom.Vec3) (geom.Vec3, error) {
	return func(p geom.Vec3) (geom.Vec3, error) {
		switch kind {
		case "quadratic":
			return geom.Vec3{X: p.X + .15*p.Y*p.Y, Y: p.Y + .25*p.X*p.X, Z: p.Z + .1*p.X*p.Y}, nil
		case "affine":
			return geom.Vec3{X: 1.2*p.X + .3*p.Y + .4, Y: -.2*p.X + .7*p.Y - .1, Z: p.Z + .2}, nil
		}
		return p, nil
	}
}

func TestPinnedManimPointwiseFamilyTransformStates(t *testing.T) {
	var fixture struct {
		Source struct {
			Version, Commit string
			Hash            string `json:"generator_sha256"`
		}
		Cases []struct {
			Kind   string
			Handle float64
			Parts  [][][3]float64
			States []connectionReferenceState
		}
	}
	data, err := os.ReadFile("tests/data/root/pointwise/manim-0.22.1.json")
	must(t, err)
	must(t, json.Unmarshal(data, &fixture))
	generator, err := os.ReadFile("tests/reference/pointwise.py")
	must(t, err)
	hash := sha256.Sum256(generator)
	if fixture.Source.Version != "0.22.1" || fixture.Source.Commit != "16ea443e2a5f1d79db0a721161411ce916d6e733" || fixture.Source.Hash != hex.EncodeToString(hash[:]) || len(fixture.Cases) != 24 {
		t.Fatal("pointwise provenance/count differs")
	}
	for i, ref := range fixture.Cases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			vec := func(p [3]float64) geom.Vec3 { return geom.Vec3{X: p[0], Y: p[1], Z: p[2]} }
			var children []*gomath.Object
			for _, part := range ref.Parts {
				var b path.Builder
				b.MoveTo(vec(part[0]))
				for j := 0; j < len(part); j += 4 {
					b.CubicTo(vec(part[j+1]), vec(part[j+2]), vec(part[j+3]))
				}
				if part[0] == part[len(part)-1] {
					b.Close()
				}
				children = append(children, gomath.NewObject(b.Path()))
			}
			object := children[0]
			if len(children) == 2 {
				object = gomath.Group(children[0], gomath.Group(children[1]))
			}
			originalIDs := []uint64{}
			object.Walk(func(o *gomath.Object) { originalIDs = append(originalIDs, o.ID) })
			o := gomath.PointwiseOptions{HandleScale: ref.Handle}
			switch ref.Kind {
			case "square":
				err = object.ApplyComplex(func(z complex128) (complex128, error) { return z * z, nil }, o)
			case "exponential":
				err = object.ApplyComplex(func(z complex128) (complex128, error) { return cmplx.Exp(.4 * z), nil }, o)
			default:
				err = object.ApplyPointwise(pointwiseMapper(ref.Kind), o)
			}
			must(t, err)
			nextIDs := []uint64{}
			object.Walk(func(o *gomath.Object) { nextIDs = append(nextIDs, o.ID) })
			if !reflect.DeepEqual(originalIDs, nextIDs) {
				t.Fatal("pointwise mapping changed object IDs")
			}
			scene := gomath.NewScene()
			must(t, scene.Add(object))
			target := object.Copy().ApplyMatrix(geom.Translation(geom.Vec3{X: .6, Y: -.4, Z: .2}).Mul(geom.RotationZ(-.4)).Mul(geom.Scaling(1.1, 1.1, 1.1)))
			must(t, scene.Play(1, gomath.Transform(object, target).WithRate(gomath.Linear)))
			for _, state := range ref.States {
				frame, err := scene.Sample(state.Alpha)
				must(t, err)
				var points []geom.Vec3
				frame.Objects[0].Walk(func(v *gomath.Object) {
					for _, c := range v.Path.Contours {
						for _, b := range c.Curves {
							points = append(points, b.P0, b.P1, b.P2, b.P3)
						}
					}
				})
				if len(points) != len(state.Points) {
					t.Fatal("mapped family topology differs", len(points), len(state.Points))
				}
				for j, p := range points {
					if p.Sub(vec(state.Points[j])).Norm() > 2e-11 {
						t.Fatal("mapped point differs", state.Alpha, j, p, state.Points[j])
					}
				}
			}
		})
	}
}

type pointwiseHostilePanic struct{}

func (pointwiseHostilePanic) String() string { panic("panic payload must not be formatted") }

func TestPointwiseErrorsAreAtomicAndPreflightRejects(t *testing.T) {
	o := gomath.DefaultPointwiseOptions()
	object := gomath.Group(gomath.Square(1), gomath.Circle(.4).Shift(geom.V(2, 0)))
	before, err := json.Marshal(object)
	must(t, err)
	sentinel := errors.New("late point failure")
	for _, kind := range []string{"error", "panic", "nilpanic", "nonfinite", "range", "expanded-range"} {
		calls := 0
		err = object.ApplyPointwise(func(p geom.Vec3) (geom.Vec3, error) {
			calls++
			if calls >= 20 {
				switch kind {
				case "error":
					return geom.Vec3{}, sentinel
				case "panic":
					panic(pointwiseHostilePanic{})
				case "nilpanic":
					panic(nil)
				case "nonfinite":
					p.X = math.Inf(1)
				case "range":
					p.X = 1e12 + 1
				case "expanded-range":
					if calls%4 == 2 {
						p.X = 1e12
					}
				}
			}
			return p, nil
		}, o)
		if err == nil {
			t.Fatal("invalid pointwise mapping succeeded", kind)
		}
		if kind == "error" && !errors.Is(err, sentinel) {
			t.Fatal("mapping error identity lost", err)
		}
		after, err := json.Marshal(object)
		must(t, err)
		if string(before) != string(after) {
			t.Fatal("failed mapping partially committed", kind)
		}
	}
	for _, scale := range []float64{0, 1e-7, 1.1, math.NaN(), math.Inf(1)} {
		calls := 0
		if err := object.ApplyPointwise(func(p geom.Vec3) (geom.Vec3, error) { calls++; return p, nil }, gomath.PointwiseOptions{HandleScale: scale}); err == nil || calls != 0 {
			t.Fatal("bad handle option invoked mapping", scale, calls)
		}
	}
	if err := object.ApplyComplex(nil, o); err == nil {
		t.Fatal("nil complex mapper accepted")
	}
	clip, err := gomath.NewClip(gomath.Square(1).Path)
	must(t, err)
	gradient, err := gomath.NewLinearGradient(geom.V(0, 0), geom.V(1, 0), []gomath.GradientStop{{Color: gomath.Red}, {Offset: 1, Color: gomath.Blue}})
	must(t, err)
	bitmap, err := gomath.NewBitmap(image.NewRGBA(image.Rect(0, 0, 2, 2)))
	must(t, err)
	picture, err := gomath.NewImage(bitmap, 1)
	must(t, err)
	cyclic := gomath.Group()
	cyclic.Children = []*gomath.Object{cyclic}
	duplicate := gomath.Square(1)
	for _, bad := range []*gomath.Object{nil, cyclic, gomath.Group(duplicate, duplicate), gomath.Square(1).SetClip(clip), gomath.Square(1).SetFillGradient(gradient), picture, gomath.Group(), gomath.Square(1).Shift(geom.V(2e12, 0))} {
		calls := 0
		if err := bad.ApplyPointwise(func(p geom.Vec3) (geom.Vec3, error) { calls++; return p, nil }, o); err == nil || calls != 0 {
			t.Fatal("invalid/unsupported source invoked mapper", calls, err)
		}
	}
	deep := gomath.Square(1)
	for range gomath.MaxAnnotationDepth {
		deep = gomath.Group(deep)
	}
	calls := 0
	if err := deep.ApplyPointwise(func(p geom.Vec3) (geom.Vec3, error) { calls++; return p, nil }, o); err == nil || calls != 0 {
		t.Fatal("depth budget not checked before callback")
	}
}

func TestPointwiseCallbackOrderMetadataAndSnapshotOwnership(t *testing.T) {
	var builder path.Builder
	builder.MoveTo(geom.V(0, 0))
	builder.CubicTo(geom.V(1, 2), geom.V(3, 4), geom.V(5, 6))
	child := gomath.NewObject(builder.Path()).SetStroke(gomath.Green, .07)
	child.Scalar = 7
	child.ZIndex = 3
	child.Name = "source"
	object := gomath.Group(child)
	id := child.ID
	var actual []geom.Vec3
	must(t, object.ApplyPointwise(func(p geom.Vec3) (geom.Vec3, error) { actual = append(actual, p); return p.Add(geom.V(1, -2)), nil }, gomath.PointwiseOptions{HandleScale: .25}))
	want := []geom.Vec3{geom.V(0, 0), geom.V(.25, .5), geom.V(4.5, 5.5), geom.V(5, 6)}
	if !reflect.DeepEqual(actual, want) || object.Children[0] != child || child.ID != id || child.Scalar != 7 || child.ZIndex != 3 || child.Name != "source" || child.Style.Stroke != gomath.Green {
		t.Fatal("mapping order or metadata differs", actual)
	}
	scene := gomath.NewScene()
	must(t, scene.Add(object))
	must(t, scene.Play(1, gomath.Shift(object, geom.V(2, .3)).WithRate(gomath.Linear)))
	baseline, err := scene.Sample(.4)
	must(t, err)
	// Mapping authoring after scene capture must not change frozen samples.
	must(t, object.ApplyComplex(func(z complex128) (complex128, error) { return z * z, nil }, gomath.DefaultPointwiseOptions()))
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			frame, err := scene.Sample(.4)
			if err != nil || !reflect.DeepEqual(frame, baseline) {
				t.Error("pointwise scene snapshot differs", err)
				return
			}
			frame.Objects[0].Children[0].Path.Contours[0].Curves[0].P1.X += 100
		}()
	}
	wg.Wait()
	frame, err := scene.Sample(.4)
	must(t, err)
	if !reflect.DeepEqual(frame, baseline) {
		t.Fatal("pointwise snapshots alias")
	}
}
