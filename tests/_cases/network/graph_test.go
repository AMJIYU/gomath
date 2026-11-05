package network_test

import (
	"gomath"
	"gomath/geom"
	"gomath/network"
	"gomath/render"
	"math"
	"reflect"
	"testing"
)

func near(t *testing.T, a, b geom.Vec3) {
	t.Helper()
	if a.Sub(b).Norm() > 1e-9 {
		t.Fatalf("got %v, want %v", a, b)
	}
}

func square(t *testing.T, o network.Options) *network.Graph {
	t.Helper()
	g, err := network.NewGraph([]string{"a", "b", "c", "d"}, []network.Edge{{"a", "b"}, {"b", "c"}, {"c", "d"}, {"d", "a"}}, o)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func TestCircularAndGridGeometry(t *testing.T) {
	o := network.DefaultOptions()
	o.Layout.Center = geom.V(1, -1)
	g := square(t, o)
	for i, id := range g.Vertices() {
		node, _ := g.Vertex(id)
		a := math.Pi/2 + float64(i)*math.Pi/2
		near(t, node.Center(), o.Layout.Center.Add(geom.V(2*math.Cos(a), 2*math.Sin(a))))
	}
	o.Layout.Kind, o.Layout.Columns = network.Grid, 2
	ids := make(map[string]uint64)
	for _, id := range g.Vertices() {
		n, _ := g.Vertex(id)
		ids[id] = n.ID
	}
	if err := g.Relayout(o.Layout); err != nil {
		t.Fatal(err)
	}
	want := []geom.Vec3{geom.V(-1, 1), geom.V(3, 1), geom.V(-1, -3), geom.V(3, -3)}
	for i, id := range g.Vertices() {
		n, _ := g.Vertex(id)
		near(t, n.Center(), want[i])
		if n.ID != ids[id] {
			t.Fatal("relayout replaced vertex identity")
		}
	}
	if err := g.Object.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestSeededSpringDeterminismBoundsAndTopology(t *testing.T) {
	o := network.DefaultOptions()
	o.Layout.Kind, o.Layout.Seed, o.Layout.Iterations = network.Spring, 42, 160
	a, b := square(t, o), square(t, o)
	for _, id := range a.Vertices() {
		x, _ := a.Vertex(id)
		y, _ := b.Vertex(id)
		if x.Center() != y.Center() {
			t.Fatal("same seed is not deterministic")
		}
		p := x.Center()
		if !p.Finite() || math.Abs(p.X) > o.Layout.Scale || math.Abs(p.Y) > o.Layout.Scale {
			t.Fatal("spring escaped bounds", p)
		}
		if x.ID == y.ID || x == y {
			t.Fatal("separate graphs share vertices")
		}
	}
	o.Layout.Seed++
	if err := b.Relayout(o.Layout); err != nil {
		t.Fatal(err)
	}
	x, _ := a.Vertex("a")
	y, _ := b.Vertex("a")
	if x.Center() == y.Center() {
		t.Fatal("different seed had no effect")
	}
	neighbors, err := a.Neighbors("a")
	if err != nil || !reflect.DeepEqual(neighbors, []string{"b", "d"}) {
		t.Fatal(neighbors, err)
	}
	if a.Directed() || len(a.Edges()) != 4 {
		t.Fatal("wrong topology")
	}
}

func TestDirectedEndpointsIdentityAndStyle(t *testing.T) {
	g, err := network.NewDiGraph([]string{"a", "b"}, []network.Edge{{"a", "b"}}, network.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	a, _ := g.Vertex("a")
	b, _ := g.Vertex("b")
	e, _ := g.Edge("a", "b")
	shaft, tip := e.Children[0], e.Children[1]
	shaft.SetStroke(gomath.Yellow, .08)
	if err := g.SetPositions(map[string]geom.Vec3{"a": geom.V(-2, 0), "b": geom.V(2, 0)}); err != nil {
		t.Fatal(err)
	}
	if n, _ := g.Vertex("a"); n != a {
		t.Fatal("vertex reference changed")
	}
	if n, _ := g.Edge("a", "b"); n != e || n.Children[0] != shaft || n.Children[1] != tip {
		t.Fatal("edge/arrow reference changed")
	}
	if shaft.Style.Stroke != gomath.Yellow || shaft.Style.StrokeWidth != .08 {
		t.Fatal("style reset")
	}
	near(t, shaft.Path.Contours[0].Start, geom.V(-1.88, 0))
	near(t, tip.Path.Contours[0].Start, geom.V(1.88, 0))
	if !tip.Path.Contours[0].Closed || len(tip.Path.Contours[0].Curves) != 3 {
		t.Fatal("missing closed arrowhead")
	}
	base := shaft.Path.Contours[0].Curves[0].P3
	if base.X >= tip.Path.Contours[0].Start.X || base.X <= -1.88 {
		t.Fatal("arrowhead points backwards")
	}
	a.MoveTo(geom.V(-2, 1))
	b.MoveTo(geom.V(2, 1))
	if err := g.UpdateEdges(); err != nil {
		t.Fatal(err)
	}
	near(t, shaft.Path.Contours[0].Start, geom.V(-1.88, 1))
	near(t, tip.Path.Contours[0].Start, geom.V(1.88, 1))
	if _, ok := g.Edge("b", "a"); ok {
		t.Fatal("directed reverse edge exists")
	}
	neighbors, err := g.Neighbors("b")
	if err != nil || len(neighbors) != 0 || !g.Directed() {
		t.Fatal("wrong directed adjacency")
	}
}

func TestUpdatesAreTransactionalAndRejectTreeChanges(t *testing.T) {
	g := square(t, network.DefaultOptions())
	before := g.Object.Copy()
	positions := map[string]geom.Vec3{}
	for _, id := range g.Vertices() {
		positions[id] = geom.V(0, 0)
	}
	if err := g.SetPositions(positions); err == nil {
		t.Fatal("coincident endpoints accepted")
	}
	for i, child := range g.Object.Children {
		if !reflect.DeepEqual(child.Path, before.Children[i].Path) {
			t.Fatal("failed update changed geometry")
		}
	}
	positions["a"] = geom.V(math.NaN(), 0)
	if err := g.SetPositions(positions); err == nil {
		t.Fatal("NaN accepted")
	}
	delete(positions, "a")
	positions["unknown"] = geom.V(1, 1)
	if err := g.SetPositions(positions); err == nil {
		t.Fatal("unknown/missing position accepted")
	}
	o := network.DefaultLayout()
	o.Kind = "spectral"
	if err := g.Relayout(o); err == nil {
		t.Fatal("unsupported layout accepted")
	}
	e, _ := g.Edge("a", "b")
	a, _ := g.Vertex("a")
	a.MoveTo(geom.V(0, 0))
	b, _ := g.Vertex("b")
	b.MoveTo(geom.V(0, 0))
	p := e.Path.Clone()
	if err := g.UpdateEdges(); err == nil {
		t.Fatal("coincident update accepted")
	}
	if !reflect.DeepEqual(e.Path, p) {
		t.Fatal("failed edge update changed path")
	}
	g.Object.Children[0] = gomath.Circle(1)
	if err := g.UpdateEdges(); err == nil {
		t.Fatal("replaced tree child accepted")
	}
}

func TestInputValidationAndSimpleGraphRules(t *testing.T) {
	for _, edges := range [][]network.Edge{{{"a", "missing"}}, {{"a", "a"}}, {{"a", "b"}, {"b", "a"}}} {
		if _, err := network.NewGraph([]string{"a", "b"}, edges, network.DefaultOptions()); err == nil {
			t.Fatal("invalid edges accepted", edges)
		}
	}
	for _, vertices := range [][]string{{"a", "a"}, {""}} {
		if _, err := network.NewGraph(vertices, nil, network.DefaultOptions()); err == nil {
			t.Fatal("invalid vertices accepted")
		}
	}
	for _, kind := range []network.LayoutKind{network.Circular, network.Grid, network.Spring} {
		o := network.DefaultOptions()
		o.Layout.Kind = kind
		for _, vertices := range [][]string{nil, {"only"}} {
			g, err := network.NewGraph(vertices, nil, o)
			if err != nil {
				t.Fatal(err)
			}
			if err := g.Object.Validate(); err != nil {
				t.Fatal(err)
			}
			if len(vertices) == 1 {
				node, _ := g.Vertex("only")
				near(t, node.Center(), geom.V(0, 0))
			}
		}
	}
	for _, mutate := range []func(*network.Options){
		func(o *network.Options) { o.Layout.Kind = "unknown" },
		func(o *network.Options) { o.Layout.Scale = math.Inf(1) },
		func(o *network.Options) { o.Layout.Center.Z = 1 },
		func(o *network.Options) { o.VertexRadius = 0 },
		func(o *network.Options) { o.EdgeColor.A = math.NaN() },
		func(o *network.Options) { o.Layout.Kind = network.Spring; o.Layout.Iterations = 1001 },
		func(o *network.Options) { o.Layout.Kind = network.Grid; o.Layout.Columns = -1 },
	} {
		o := network.DefaultOptions()
		mutate(&o)
		if _, err := network.NewGraph([]string{"a"}, nil, o); err == nil {
			t.Fatal("invalid options accepted", o)
		}
	}
	o := network.DefaultOptions()
	o.Layout.Kind = network.Spring
	o.Layout.Iterations = 1000
	vertices := make([]string, 200)
	for i := range vertices {
		vertices[i] = string(rune(1000 + i))
	}
	if _, err := network.NewGraph(vertices, nil, o); err == nil {
		t.Fatal("unbounded spring work accepted")
	}
	g, err := network.NewDiGraph([]string{"a", "b"}, []network.Edge{{"a", "b"}, {"b", "a"}}, network.DefaultOptions())
	if err != nil || len(g.Edges()) != 2 {
		t.Fatal("reciprocal directed edges rejected", err)
	}
}

func TestNumericallyCollapsedVerticesAndTipsReturnErrors(t *testing.T) {
	o := network.DefaultOptions()
	o.Layout.Center = geom.V(1e20, 0)
	if _, err := network.NewGraph([]string{"a"}, nil, o); err == nil {
		t.Fatal("vertex lost to coordinate precision was accepted")
	}
	o = network.DefaultOptions()
	o.TipSize = 1e-200
	if _, err := network.NewDiGraph([]string{"a", "b"}, []network.Edge{{"a", "b"}}, o); err == nil {
		t.Fatal("numerically invisible arrowhead accepted")
	}
	g := square(t, network.DefaultOptions())
	node, _ := g.Vertex("a")
	node.Scale(0)
	if err := g.UpdateEdges(); err == nil {
		t.Fatal("collapsed vertex accepted")
	}
}

func TestSceneOwnershipRenderingAndSnapshots(t *testing.T) {
	g, err := network.NewDiGraph([]string{"a", "b"}, []network.Edge{{"a", "b"}}, network.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if err := g.SetPositions(map[string]geom.Vec3{"a": geom.V(-2, 0), "b": geom.V(2, 0)}); err != nil {
		t.Fatal(err)
	}
	scene := gomath.NewScene()
	if err := scene.Add(g.Object, g.Object.Copy().Shift(geom.V(0, 1))); err != nil {
		t.Fatal("independent copy rejected", err)
	}
	vertex, _ := g.Vertex("a")
	if err := scene.Add(vertex); err == nil {
		t.Fatal("shared child accepted as root")
	}
	if err := scene.Play(1, gomath.Shift(g.Object, geom.V(0, -1))); err != nil {
		t.Fatal(err)
	}
	f, err := scene.Sample(.5)
	if err != nil {
		t.Fatal(err)
	}
	c := render.DefaultConfig()
	c.Width, c.Height, c.FrameWidth, c.Background = 160, 100, 8, gomath.Black
	r, err := render.NewRasterizer(c)
	if err != nil {
		t.Fatal(err)
	}
	img, err := r.Render(f)
	if err != nil {
		t.Fatal(err)
	}
	// x=-1 is on the shaft at y=-.5; x=1.78 is inside the filled tip.
	if img.RGBAAt(60, 60).R < 60 || img.RGBAAt(115, 60).R < 160 {
		t.Fatal("rendered graph lacks shaft or tip")
	}
	f.Objects[0].Shift(geom.V(20, 20))
	f2, err := scene.Sample(.5)
	if err != nil {
		t.Fatal(err)
	}
	if f2.Objects[0].Center().X > 10 {
		t.Fatal("sampling shares mutable geometry")
	}
}

func BenchmarkSpringLayout(b *testing.B) {
	ids := make([]string, 40)
	edges := make([]network.Edge, 40)
	for i := range ids {
		ids[i] = string(rune('A' + i))
	}
	for i := range edges {
		edges[i] = network.Edge{From: ids[i], To: ids[(i+1)%len(ids)]}
	}
	o := network.DefaultOptions()
	o.Layout.Kind, o.Layout.Iterations, o.VertexRadius = network.Spring, 100, .005
	b.ReportAllocs()
	for b.Loop() {
		if _, err := network.NewGraph(ids, edges, o); err != nil {
			b.Fatal(err)
		}
	}
}
