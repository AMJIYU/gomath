package gomath_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"gomath"
	"gomath/geom"
	"gomath/path"
	"math"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
)

type odContour struct {
	Points [][3]float64
	Closed bool
}

func odVec(v [3]float64) geom.Vec3 { return geom.Vec3{X: v[0], Y: v[1], Z: v[2]} }

func odPath(contours []odContour) path.Path {
	var p path.Path
	for _, c := range contours {
		v := path.Contour{Start: odVec(c.Points[0]), Closed: c.Closed}
		for i := 0; i+3 < len(c.Points); i += 4 {
			v.Curves = append(v.Curves, path.Cubic{P0: odVec(c.Points[i]), P1: odVec(c.Points[i+1]), P2: odVec(c.Points[i+2]), P3: odVec(c.Points[i+3])})
		}
		p.Contours = append(p.Contours, v)
	}
	return p
}

func odPoints(p path.Path) []geom.Vec3 {
	var points []geom.Vec3
	for _, c := range p.Contours {
		if len(c.Curves) == 0 {
			points = append(points, c.Start)
		}
		for _, v := range c.Curves {
			points = append(points, v.P0, v.P1, v.P2, v.P3)
		}
	}
	return points
}

func odCompare(t testing.TB, p path.Path, want [][3]float64, tolerance float64) {
	t.Helper()
	points := odPoints(p)
	if len(points) != len(want) {
		t.Fatalf("got %d points want %d", len(points), len(want))
	}
	for i, v := range points {
		if !v.Finite() || v.Sub(odVec(want[i])).Norm() > tolerance {
			t.Fatalf("point%d got%v want%v", i, v, want[i])
		}
	}
}

func odObjects(root *gomath.Object) []*gomath.Object {
	var nodes []*gomath.Object
	root.Walk(func(v *gomath.Object) { nodes = append(nodes, v) })
	return nodes
}

func TestObjectDirectionPinnedManimAndActualTransform(t *testing.T) {
	var fixture struct {
		Source struct {
			Version, Commit string
			Hash            string `json:"generator_sha256"`
			Verified        bool   `json:"installed_equals_local_equals_git"`
		}
		Trees []struct {
			Name, Operation string
			Nodes           []struct {
				Parent   int
				Contours []odContour
			}
			Reversed [][][3]float64
			States   []struct {
				Alpha float64
				Nodes [][][3]float64
			}
		}
	}
	data, err := os.ReadFile("tests/data/path/direction/reference.json")
	must(t, err)
	must(t, json.Unmarshal(data, &fixture))
	generator, err := os.ReadFile("tests/reference/path_direction.py")
	must(t, err)
	hash := sha256.Sum256(generator)
	if fixture.Source.Version != "0.22.1" || fixture.Source.Commit != "16ea443e2a5f1d79db0a721161411ce916d6e733" || fixture.Source.Hash != hex.EncodeToString(hash[:]) || !fixture.Source.Verified || len(fixture.Trees) != 6 {
		t.Fatal("object direction provenance mismatch")
	}
	interiors := 0
	for _, tc := range fixture.Trees {
		t.Run(tc.Name, func(t *testing.T) {
			makeTree := func() []*gomath.Object {
				nodes := make([]*gomath.Object, len(tc.Nodes))
				for i, n := range tc.Nodes {
					nodes[i] = gomath.NewObject(odPath(n.Contours))
					if i > 0 {
						nodes[n.Parent].Children = append(nodes[n.Parent].Children, nodes[i])
					}
				}
				return nodes
			}
			targets := makeTree()
			ids := make([]uint64, len(targets))
			styles := make([]gomath.Style, len(targets))
			for i, v := range targets {
				ids[i], styles[i] = v.ID, v.Style
			}
			if tc.Operation == "reverse_points" {
				must(t, targets[0].ReversePoints())
			} else {
				must(t, targets[0].ReverseDirection())
			}
			for i, v := range odObjects(targets[0]) {
				if v != targets[i] || v.ID != ids[i] || v.Style != styles[i] {
					t.Fatal("tree order, pointer, ID or style changed")
				}
				odCompare(t, v.Path, tc.Reversed[i], 0)
			}
			if len(tc.States) == 0 {
				return
			}
			current := makeTree()
			rootID := current[0].ID
			targets[0].ApplyMatrix(geom.Translation(geom.Vec3{X: .4, Y: -.3, Z: .1}).Mul(geom.RotationZ(.27)).Mul(geom.Scaling(1.08, 1.08, 1.08)))
			scene := gomath.NewScene()
			must(t, scene.Add(current[0]))
			must(t, scene.Play(1, gomath.Transform(current[0], targets[0]).WithRate(gomath.Linear)))
			for _, state := range tc.States {
				frame := sample(t, scene, state.Alpha)
				nodes := odObjects(frame.Objects[0])
				if len(nodes) != len(state.Nodes) || nodes[0].ID != rootID {
					t.Fatal("Transform tree or identity changed")
				}
				for i, v := range nodes {
					odCompare(t, v.Path, state.Nodes[i], 1e-10)
				}
				if state.Alpha > 0 && state.Alpha < 1 {
					interiors++
				}
			}
		})
	}
	if interiors != 16 {
		t.Fatal("missing actual Transform interior frames", interiors)
	}
}

// Retain headers and independent numerical controls: rollback must leave
// existing slices and their capacities in place, not merely equal values.
type odSnapshot struct {
	value       gomath.Object
	geometry    path.Path
	contourBase *path.Contour
	curveBases  []*path.Cubic
	curveCaps   []int
	childBase   **gomath.Object
	children    []*gomath.Object
}

func odCapture(o *gomath.Object) odSnapshot {
	s := odSnapshot{value: *o, geometry: o.Path.Clone(), children: append([]*gomath.Object(nil), o.Children...)}
	if len(o.Path.Contours) > 0 {
		s.contourBase = &o.Path.Contours[0]
	}
	for _, c := range o.Path.Contours {
		var base *path.Cubic
		if len(c.Curves) > 0 {
			base = &c.Curves[0]
		}
		s.curveBases = append(s.curveBases, base)
		s.curveCaps = append(s.curveCaps, cap(c.Curves))
	}
	if len(o.Children) > 0 {
		s.childBase = &o.Children[0]
	}
	return s
}

func (s odSnapshot) check(t *testing.T, o *gomath.Object) {
	t.Helper()
	if o.ID != s.value.ID || o.Name != s.value.Name || o.Style != s.value.Style || o.Scalar != s.value.Scalar || o.ZIndex != s.value.ZIndex || o.Bitmap != s.value.Bitmap || o.Clip != s.value.Clip || len(o.Children) != len(s.children) || cap(o.Children) != cap(s.value.Children) || len(o.Path.Contours) != len(s.value.Path.Contours) || cap(o.Path.Contours) != cap(s.value.Path.Contours) {
		t.Fatal("rollback changed object metadata or slice header")
	}
	if s.contourBase != nil && &o.Path.Contours[0] != s.contourBase || s.childBase != nil && &o.Children[0] != s.childBase {
		t.Fatal("rollback replaced original storage")
	}
	for i, c := range o.Path.Contours {
		if cap(c.Curves) != s.curveCaps[i] || s.curveBases[i] != nil && &c.Curves[0] != s.curveBases[i] {
			t.Fatal("rollback replaced original curve storage")
		}
	}
	for i, child := range o.Children {
		if child != s.children[i] {
			t.Fatal("rollback replaced original child")
		}
	}
	// Clone's nil normalization is applied equally to both sides here.
	if !reflect.DeepEqual(o.Path.Clone(), s.geometry) {
		t.Fatal("rollback changed numerical controls")
	}
}

func TestObjectDirectionAtomicLateFailureAndAliasing(t *testing.T) {
	for _, cause := range []string{"join", "implicit", "coordinate", "bitmap", "style", "scalar", "clip"} {
		t.Run(cause, func(t *testing.T) {
			good := gomath.Line(geom.V(0, 0), geom.V(3, 2))
			bad := gomath.Line(geom.V(10, 0), geom.V(13, 2))
			root := gomath.Group(good, bad)
			// The iterative LIFO traversal stages the last child first.
			root.Children = []*gomath.Object{bad, good}
			switch cause {
			case "join":
				bad.Path.Contours[0].Curves[0].P0.X += 1e-12
			case "implicit":
				bad.Path.Contours[0].Closed = true
			case "coordinate":
				bad.Path.Contours[0].Curves[0].P2.Z = 1e9 + 1
			case "bitmap":
				bad.Bitmap = &gomath.Bitmap{}
			case "style":
				bad.Style.StrokeWidth = -1
			case "scalar":
				bad.Scalar = math.Inf(1)
			case "clip":
				bad.Clip = &gomath.Clip{}
			}
			before := []odSnapshot{odCapture(root), odCapture(good), odCapture(bad)}
			if err := root.ReversePoints(); err == nil {
				t.Fatal("invalid late node accepted")
			}
			for i, o := range []*gomath.Object{root, good, bad} {
				before[i].check(t, o)
			}
		})
	}
	left := gomath.Line(geom.V(0, 0), geom.V(2, 1))
	right := gomath.Line(geom.V(5, 0), geom.V(8, 1))
	// Input curve slices may alias. Each published path must own its output.
	right.Path = left.Path
	alias := left.Path
	saved := alias.Clone()
	root := gomath.Group(left, right)
	gradient, err := gomath.NewLinearGradient(geom.V(0, 0), geom.V(1, 0), []gomath.GradientStop{{Offset: 0, Color: gomath.Red}, {Offset: 1, Color: gomath.Blue}})
	must(t, err)
	dash, err := gomath.NewDashPattern([]float64{.2, .1}, .05)
	must(t, err)
	clip, err := gomath.NewClip(left.Path)
	must(t, err)
	root.Style.FillGradient, left.Style.Dash, right.Clip = gradient, dash, clip
	root.Name, left.Name, right.Name = "root", "left", "right"
	root.Scalar, left.ZIndex = 7, -4
	styles := []gomath.Style{root.Style, left.Style, right.Style}
	ids := []uint64{root.ID, left.ID, right.ID}
	childrenBase := &root.Children[0]
	must(t, root.ReversePoints())
	if root.Children[0] != left || root.Children[1] != right || &root.Children[0] != childrenBase || root.Name != "root" || left.Name != "left" || right.Name != "right" || root.Scalar != 7 || left.ZIndex != -4 || right.Clip != clip {
		t.Fatal("published edit replaced object metadata, child storage or pointers")
	}
	for i, v := range []*gomath.Object{root, left, right} {
		if v.ID != ids[i] || v.Style != styles[i] {
			t.Fatal("published edit changed identity or style pointers")
		}
	}
	if !reflect.DeepEqual(alias, saved) || &left.Path.Contours[0].Curves[0] == &right.Path.Contours[0].Curves[0] {
		t.Fatal("edit mutated borrowed input or published shared output")
	}
	second := right.Path.Clone()
	left.Path.Contours[0].Curves[0].P1.Z = 700
	if !reflect.DeepEqual(second, right.Path) || !reflect.DeepEqual(alias, saved) {
		t.Fatal("output retains writable aliases")
	}
}

func TestObjectDirectionReceiverOnlyAndEmptyFamilies(t *testing.T) {
	var nilObject *gomath.Object
	if nilObject.ReversePoints() == nil || nilObject.ReverseDirection() == nil {
		t.Fatal("nil receiver accepted")
	}
	root := gomath.Group(gomath.Group())
	must(t, root.ReversePoints())
	must(t, root.ReverseDirection())
	child := gomath.Line(geom.V(4, 0), geom.V(9, 1))
	child.Path.Contours[0].Closed = true
	root = gomath.Line(geom.V(0, 0), geom.V(2, 3))
	root.Children = []*gomath.Object{child}
	before := odCapture(child)
	must(t, root.ReverseDirection())
	before.check(t, child)
	if root.Path.Contours[0].Start != geom.V(2, 3) {
		t.Fatal("receiver path was not reversed")
	}
	if root.ReversePoints() == nil {
		t.Fatal("recursive reversal accepted implicit descendant closing edge")
	}
	child.Path.Contours[0].Closed = false
	child.Bitmap = &gomath.Bitmap{}
	must(t, root.ReverseDirection())
	if root.ReversePoints() == nil {
		t.Fatal("family reversal accepted bitmap")
	}
	point := gomath.NewObject(path.Path{Contours: []path.Contour{{Start: geom.Vec3{X: 3, Y: -1, Z: 5}, Closed: true}}})
	must(t, point.ReversePoints())
	if len(point.Path.Contours) != 1 || len(point.Path.Contours[0].Curves) != 0 || !point.Path.Contours[0].Closed || point.Path.Contours[0].Start.Z != 5 {
		t.Fatal("point-only contour was lost")
	}
}

func TestObjectDirectionTreeOwnershipAndExactBudgetBounds(t *testing.T) {
	for _, cause := range []string{"selfcycle", "cycle", "shared", "nilchild", "zeroID", "duplicateID"} {
		t.Run(cause, func(t *testing.T) {
			root, a, b := gomath.Group(), gomath.Group(), gomath.Group()
			root.Children = []*gomath.Object{a, b}
			switch cause {
			case "selfcycle":
				root.Children = append(root.Children, root)
			case "cycle":
				b.Children = []*gomath.Object{root}
			case "shared":
				a.Children = []*gomath.Object{b}
			case "nilchild":
				a.Children = []*gomath.Object{nil}
			case "zeroID":
				b.ID = 0
			case "duplicateID":
				b.ID = a.ID
			}
			before := odCapture(root)
			if root.ReversePoints() == nil || root.ReverseDirection() == nil {
				t.Fatal("invalid tree ownership accepted")
			}
			before.check(t, root)
		})
	}
	root := gomath.Group()
	for range gomath.MaxAnnotationObjects - 1 {
		root.Children = append(root.Children, gomath.Group())
	}
	must(t, root.ReversePoints())
	root.Children = append(root.Children, gomath.Group())
	if err := root.ReversePoints(); err == nil || !strings.Contains(err.Error(), "object/depth") {
		t.Fatal("object budget ignored", err)
	}
	root = gomath.Group()
	current := root
	for range gomath.MaxAnnotationDepth - 1 {
		child := gomath.Group()
		current.Children = []*gomath.Object{child}
		current = child
	}
	must(t, root.ReversePoints())
	current.Children = []*gomath.Object{gomath.Group()}
	if root.ReversePoints() == nil || root.ReverseDirection() == nil {
		t.Fatal("depth budget ignored")
	}
	// Every node fits individually. The complete tree must share both caps.
	for _, cause := range []string{"contours", "curves", "storage"} {
		t.Run(cause, func(t *testing.T) {
			var p path.Path
			switch cause {
			case "contours":
				p.Contours = make([]path.Contour, 2048)
			case "curves":
				p.Contours = []path.Contour{{Curves: make([]path.Cubic, 32769)}}
			case "storage":
				p.Contours = []path.Contour{{Curves: make([]path.Cubic, 32768)}}
			}
			a, b := gomath.NewObject(p), gomath.NewObject(p)
			root := gomath.Group(a, b)
			if cause == "contours" {
				must(t, root.ReversePoints()) // exactly 4096 total
				b.Path.Contours = append(b.Path.Contours, path.Contour{})
			}
			before := odCapture(a)
			if err := root.ReversePoints(); err == nil || !strings.Contains(err.Error(), map[string]string{"contours": "contour", "curves": "curve budget", "storage": "storage"}[cause]) {
				t.Fatal("shared budget ignored", err)
			}
			before.check(t, a)
			if root.ReverseDirection() == nil {
				t.Fatal("receiver-only wrapper bypasses full-tree accounting")
			}
		})
	}
	// 65535 cubics + one contour + one node leave 16 accounted bytes.
	fit := gomath.NewObject(path.Path{Contours: []path.Contour{{Curves: make([]path.Cubic, 65535)}}})
	must(t, fit.ReversePoints())
	// With no extra curves, one empty descendant crosses the storage boundary.
	fit.Children = []*gomath.Object{gomath.Group()}
	before := odCapture(fit)
	if err := fit.ReversePoints(); err == nil || !strings.Contains(err.Error(), "storage") {
		t.Fatal("tree staging was omitted from storage accounting", err)
	}
	before.check(t, fit)
}

func TestObjectDirectionConcurrentIndependentEditsAndSceneSampling(t *testing.T) {
	base := gomath.Group(gomath.Line(geom.V(-2, 0), geom.V(2, 1)), gomath.Group(gomath.Line(geom.V(0, 3), geom.V(2, 5))))
	before := odCapture(base.Children[0])
	var wg sync.WaitGroup
	for range 12 {
		wg.Go(func() {
			o := base.Copy()
			for range 8 {
				if err := o.ReversePoints(); err != nil {
					t.Error(err)
					return
				}
			}
			s := gomath.NewScene()
			if err := s.Add(o); err != nil {
				t.Error(err)
				return
			}
			target := o.Copy()
			if err := target.ReversePoints(); err != nil {
				t.Error(err)
				return
			}
			if err := s.Play(1, gomath.Transform(o, target).WithRate(gomath.Linear)); err != nil {
				t.Error(err)
				return
			}
			var samples sync.WaitGroup
			for _, alpha := range []float64{.125, .375, .5, .875} {
				samples.Go(func() {
					frame, err := s.Sample(alpha)
					if err != nil {
						t.Error(err)
						return
					}
					frame.Objects[0].Children[0].Path.Contours[0].Curves[0].P1.Z = 99
				})
			}
			samples.Wait()
		})
	}
	wg.Wait()
	before.check(t, base.Children[0])
}

func BenchmarkObjectDirectionFamily(b *testing.B) {
	for _, count := range []int{1, 64, 1024} {
		root := gomath.Group()
		for i := range count {
			p := path.Path{Contours: []path.Contour{{Start: geom.V(float64(i), 0)}}}
			a := p.Contours[0].Start
			for j := range 16 {
				z := geom.Vec3{X: float64(i) + float64(j+1)/16, Y: float64(j % 3), Z: float64(j % 5)}
				p.Contours[0].Curves = append(p.Contours[0].Curves, path.Cubic{P0: a, P1: a.Add(geom.V(0, 2)), P2: z.Add(geom.V(0, -1)), P3: z})
				a = z
			}
			root.Children = append(root.Children, gomath.NewObject(p))
		}
		b.Run(fmt.Sprintf("Nodes%d_Curves%d", count+1, 16*count), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if err := root.ReversePoints(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
