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
	"strconv"
	"sync"
	"testing"
)

func TestPinnedManimBraceOrderedTransformStates(t *testing.T) {
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
			Start, End, Direction [3]float64
			Buffer, Sharpness     float64
			Tip                   [3]float64
			DirectionResult       [3]float64 `json:"direction_result"`
			States                []connectionReferenceState
		}
	}
	data, err := os.ReadFile("tests/data/root/manim-braces-0.22.1.json")
	must(t, err)
	must(t, json.Unmarshal(data, &fixture))
	generator, err := os.ReadFile("tests/reference/braces.py")
	must(t, err)
	hash := sha256.Sum256(generator)
	if fixture.Source.Version != "0.22.1" || fixture.Source.Commit != "16ea443e2a5f1d79db0a721161411ce916d6e733" || fixture.Source.Hash != hex.EncodeToString(hash[:]) || len(fixture.Cases) != 32 {
		t.Fatal("brace provenance or count differs")
	}
	vec := func(v [3]float64) geom.Vec3 { return geom.Vec3{X: v[0], Y: v[1], Z: v[2]} }
	for i, ref := range fixture.Cases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			o := gomath.BraceOptions{Direction: vec(ref.Direction), Buffer: ref.Buffer, Sharpness: ref.Sharpness}
			var brace *gomath.Brace
			if ref.Kind == "points" {
				brace, err = gomath.NewBraceBetweenPoints(vec(ref.Start), vec(ref.End), o)
			} else {
				var parts []*gomath.Object
				for _, part := range ref.Parts {
					points := make([]geom.Vec3, len(part.Points))
					for j, p := range part.Points {
						points[j] = vec(p)
					}
					object := gomath.Polyline(points...)
					if part.Closed {
						object = gomath.Polygon(points...)
					}
					parts = append(parts, object)
				}
				brace, err = gomath.NewBrace(gomath.Group(parts...), o)
			}
			must(t, err)
			tip, err := brace.Tip()
			must(t, err)
			if tip.Sub(vec(ref.Tip)).Norm() > 2e-11 {
				t.Fatalf("tip differs: %v want %v", tip, ref.Tip)
			}
			direction, err := brace.Direction()
			must(t, err)
			if direction.Sub(vec(ref.DirectionResult)).Norm() > 2e-8 {
				t.Fatalf("direction differs: %v want %v", direction, ref.DirectionResult)
			}
			checkConnectionTransformStates(t, brace.Object, ref.States)
		})
	}
}

func TestBraceProjectedGeometryBufferAndOwnership(t *testing.T) {
	for _, direction := range []geom.Vec3{geom.V(0, -1), geom.V(1, 0), geom.V(-.6, .8)} {
		target := gomath.Rectangle(2, 1).Rotate(.31).Shift(geom.V(.6, -.4))
		target.ZIndex = 7
		before, err := json.Marshal(target)
		must(t, err)
		o := gomath.DefaultBraceOptions()
		o.Direction = direction
		brace, err := gomath.NewBrace(target, o)
		must(t, err)
		after, err := json.Marshal(target)
		must(t, err)
		if string(before) != string(after) || brace.Object.ZIndex != 7 || brace.Object.Style.Fill != gomath.White || brace.Object.Style.StrokeWidth != 0 {
			t.Fatal("source mutated or output style differs")
		}
		normal := direction.Unit()
		angle := math.Atan2(normal.X, -normal.Y)
		rotation := geom.RotationZ(-angle)
		projected := target.Copy().ApplyMatrix(rotation).Bounds()
		outline := brace.Object.Copy().ApplyMatrix(rotation).Bounds()
		if math.Abs(outline.Size().X-projected.Size().X) > 2e-10 || math.Abs(outline.Max.Y-(projected.Min.Y-o.Buffer)) > 2e-10 {
			t.Fatal("wrong projected width/gap", outline, projected)
		}
		bounds := brace.Object.Bounds()
		target.Shift(geom.V(10, 20))
		if brace.Object.Bounds() != bounds {
			t.Fatal("brace retained its target")
		}
		brace.Object.Shift(geom.V(-4, 2))
		if target.Bounds().Center().Sub(geom.V(10.6, 19.6)).Norm() > 1e-12 {
			t.Fatal("brace changed source")
		}
	}
	// This cubic's endpoints are at y=0, but its actual upper extremum is y=3.
	var builder path.Builder
	builder.MoveTo(geom.V(-1, 0))
	builder.CubicTo(geom.V(-1, 4), geom.V(1, 4), geom.V(1, 0))
	o := gomath.DefaultBraceOptions()
	o.Direction = geom.V(0, 1)
	brace, err := gomath.NewBrace(gomath.NewObject(builder.Path()), o)
	must(t, err)
	if math.Abs(brace.Object.Bounds().Min.Y-3.2) > 2e-10 {
		t.Fatal("brace did not measure actual cubic extrema")
	}
}

func TestBraceTipFollowsTransformAndSnapshotsAreIndependent(t *testing.T) {
	brace, err := gomath.NewBraceBetweenPoints(geom.V(-1, 0), geom.V(1, .7), gomath.DefaultBraceBetweenPointsOptions())
	must(t, err)
	tip, err := brace.Tip()
	must(t, err)
	scene := gomath.NewScene()
	must(t, scene.Add(brace.Object))
	matrix := geom.Translation(geom.V(.6, -.4)).Mul(geom.RotationZ(.4)).Mul(geom.Scaling(1.2, .8, 1))
	id := brace.Object.ID
	must(t, scene.Play(1, gomath.Transform(brace.Object, brace.Object.Copy().ApplyMatrix(matrix)).WithRate(gomath.Linear)))
	actual, err := brace.Tip()
	must(t, err)
	if brace.Object.ID != id || actual.Sub(matrix.Apply(tip)).Norm() > 1e-12 {
		t.Fatal("tip lost authoring identity")
	}
	baseline, err := scene.Sample(.4)
	must(t, err)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			frame, err := scene.Sample(.4)
			if err != nil || !reflect.DeepEqual(frame, baseline) {
				t.Error("different concurrent snapshot", err)
				return
			}
			frame.Objects[0].Path.Contours[0].Curves[0].P1.X += 10
		}()
	}
	wg.Wait()
	frame, err := scene.Sample(.4)
	must(t, err)
	if !reflect.DeepEqual(frame, baseline) {
		t.Fatal("sampled geometry leaked")
	}
	brace.Object.Path.Contours[0].Curves = nil
	if _, err = brace.Tip(); err == nil {
		t.Fatal("changed topology accepted")
	}
	var empty *gomath.Brace
	if _, err = empty.Direction(); err == nil {
		t.Fatal("nil brace accepted")
	}
}

func TestBraceErrorsAndPrecisionPolicy(t *testing.T) {
	target := gomath.Rectangle(2, 1)
	for _, mutate := range []func(*gomath.BraceOptions){
		func(o *gomath.BraceOptions) { o.Direction = geom.Vec3{} },
		func(o *gomath.BraceOptions) { o.Direction.Z = 1 },
		func(o *gomath.BraceOptions) { o.Direction.X = math.NaN() },
		func(o *gomath.BraceOptions) { o.Buffer = -1 },
		func(o *gomath.BraceOptions) { o.Buffer = math.Inf(1) },
		func(o *gomath.BraceOptions) { o.Sharpness = 0 },
		func(o *gomath.BraceOptions) { o.Sharpness = 1e6 + 1 },
	} {
		o := gomath.DefaultBraceOptions()
		mutate(&o)
		if result, err := gomath.NewBrace(target, o); result != nil || err == nil {
			t.Fatal("invalid options accepted")
		}
	}
	for _, object := range []*gomath.Object{nil, gomath.Group(), gomath.Line(geom.V(0, -1), geom.V(0, 1)), target.Copy().Shift(geom.V(1e12-2, 0))} {
		if result, err := gomath.NewBrace(object, gomath.DefaultBraceOptions()); result != nil || err == nil {
			t.Fatal("invalid or unrepresentable source accepted")
		}
	}
	cyclic := gomath.Group()
	cyclic.Children = []*gomath.Object{cyclic}
	if result, err := gomath.NewBrace(cyclic, gomath.DefaultBraceOptions()); result != nil || err == nil {
		t.Fatal("cyclic source accepted")
	}
	for _, points := range [][2]geom.Vec3{{{}, {}}, {{}, {Z: 1}}, {{}, {X: 2e12}}, {{X: math.NaN()}, {X: 1}}} {
		if result, err := gomath.NewBraceBetweenPoints(points[0], points[1], gomath.DefaultBraceBetweenPointsOptions()); result != nil || err == nil {
			t.Fatal("invalid points accepted")
		}
	}
	o := gomath.DefaultBraceOptions()
	o.Direction = geom.V(0, -math.SmallestNonzeroFloat64)
	if _, err := gomath.NewBrace(target, o); err != nil {
		t.Fatal("finite subnormal direction could not normalize", err)
	}
}

func TestPinnedManimBraceLabelPlacementAndCopies(t *testing.T) {
	var ref struct {
		Source struct {
			Version, Commit string
			Hash            string `json:"generator_sha256"`
		}
		Cases []struct {
			Direction [3]float64
			Buffer    float64
			Min       [3]float64 `json:"label_min"`
			Max       [3]float64 `json:"label_max"`
		}
	}
	data, err := os.ReadFile("tests/data/root/manim-brace-labels-0.22.1.json")
	must(t, err)
	must(t, json.Unmarshal(data, &ref))
	generator, err := os.ReadFile("tests/reference/brace_labels.py")
	must(t, err)
	hash := sha256.Sum256(generator)
	if ref.Source.Version != "0.22.1" || ref.Source.Commit != "16ea443e2a5f1d79db0a721161411ce916d6e733" || ref.Source.Hash != hex.EncodeToString(hash[:]) || len(ref.Cases) != 12 {
		t.Fatal("brace label provenance/count differs")
	}
	vec := func(v [3]float64) geom.Vec3 { return geom.Vec3{X: v[0], Y: v[1], Z: v[2]} }
	for _, c := range ref.Cases {
		o := gomath.DefaultBraceOptions()
		o.Direction = vec(c.Direction)
		brace, err := gomath.NewBrace(gomath.Rectangle(2, 1), o)
		must(t, err)
		label := gomath.Group(gomath.Rectangle(.8, .3)).Shift(geom.Vec3{X: 3, Y: -1, Z: .4})
		before, err := json.Marshal(label)
		must(t, err)
		placed, err := brace.LabelAtTip(label, c.Buffer)
		must(t, err)
		bounds := placed.Bounds()
		if bounds.Min.Sub(vec(c.Min)).Norm() > 2e-11 || bounds.Max.Sub(vec(c.Max)).Norm() > 2e-11 || placed.ID == label.ID || placed.Children[0].ID == label.Children[0].ID {
			t.Fatal("label position/identity differs", bounds, c)
		}
		placed.Shift(geom.V(10, 10))
		after, err := json.Marshal(label)
		must(t, err)
		if string(before) != string(after) {
			t.Fatal("source label mutated")
		}
		if result, err := brace.LabelAtTip(label, -1); result != nil || err == nil {
			t.Fatal("negative label gap accepted")
		}
		if result, err := brace.LabelAtTip(gomath.Group(), .1); result != nil || err == nil {
			t.Fatal("empty label accepted")
		}
	}
}

func BenchmarkBraceConstruction(b *testing.B) {
	target := gomath.Group(gomath.Rectangle(2, 1), gomath.Circle(.5).Shift(geom.V(1, .4)))
	o := gomath.DefaultBraceOptions()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := gomath.NewBrace(target, o); err != nil {
			b.Fatal(err)
		}
	}
}
