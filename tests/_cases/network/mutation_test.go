package network_test

import (
	"errors"
	"gomath"
	"gomath/geom"
	"gomath/network"
	"math"
	"reflect"
	"testing"
)

type mutationCapture struct {
	root     *gomath.Object
	vertices []string
	edges    []network.Edge
	objects  map[*gomath.Object]gomath.Object
}

func captureMutation(g *network.Graph) mutationCapture {
	s := mutationCapture{g.Object, g.Vertices(), g.Edges(), map[*gomath.Object]gomath.Object{}}
	g.Object.Walk(func(o *gomath.Object) {
		v := *o
		v.Path = o.Path.Clone()
		v.Children = append([]*gomath.Object(nil), o.Children...)
		s.objects[o] = v
	})
	return s
}
func unchangedMutation(t *testing.T, g *network.Graph, s mutationCapture) {
	t.Helper()
	if g.Object != s.root || !reflect.DeepEqual(g.Vertices(), s.vertices) || !reflect.DeepEqual(g.Edges(), s.edges) {
		t.Fatal("partial metadata publication")
	}
	for p, v := range s.objects {
		// nil and zero-length Children are observationally equivalent.
		actual := *p
		actual.Path = p.Path.Clone()
		actual.Children = append([]*gomath.Object(nil), p.Children...)
		if !reflect.DeepEqual(actual, v) {
			t.Fatalf("partial object change: %d", p.ID)
		}
	}
}
func mutationGraph(t *testing.T, directed bool) *network.Graph {
	t.Helper()
	var g *network.Graph
	var err error
	if directed {
		var d *network.DiGraph
		d, err = network.NewDiGraph([]string{"a", "b", "c"}, []network.Edge{{"a", "b"}}, network.DefaultOptions())
		if d != nil {
			g = d.Graph
		}
	} else {
		g, err = network.NewGraph([]string{"a", "b", "c"}, []network.Edge{{"a", "b"}}, network.DefaultOptions())
	}
	if err != nil {
		t.Fatal(err)
	}
	if err = g.SetPositions(map[string]geom.Vec3{"a": geom.V(-2, 0), "b": geom.V(2, 0), "c": geom.V(2, 2)}); err != nil {
		t.Fatal(err)
	}
	return g
}
func assertMutationOrder(t *testing.T, g *network.Graph) {
	t.Helper()
	for i, e := range g.Edges() {
		p, ok := g.Edge(e.From, e.To)
		if !ok || g.Object.Children[i] != p {
			t.Fatal("edge order")
		}
	}
	for i, id := range g.Vertices() {
		p, ok := g.Vertex(id)
		if !ok || g.Object.Children[len(g.Edges())+i] != p {
			t.Fatal("vertex order")
		}
	}
	if err := g.Object.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestGraphMutationIdentityStylesOrderingAndGeometry(t *testing.T) {
	for _, directed := range []bool{false, true} {
		t.Run(map[bool]string{false: "Graph", true: "DiGraph"}[directed], func(t *testing.T) {
			g := mutationGraph(t, directed)
			root := g.Object
			a, _ := g.Vertex("a")
			b, _ := g.Vertex("b")
			old, _ := g.Edge("a", "b")
			root.SetOpacity(.75)
			old.SetStroke(gomath.Yellow, .08)
			old.Name = "authored edge"
			a.Scalar = 2
			a.ZIndex = 4
			dash, err := gomath.NewDashPattern([]float64{.1, .2}, .07)
			if err != nil {
				t.Fatal(err)
			}
			a.Style.Dash = dash
			before := captureMutation(g)
			style := gomath.DefaultStyle()
			style.Fill = gomath.Red
			style.Stroke = gomath.Transparent
			style.StrokeWidth = 0
			style.Opacity = .4
			style.Dash = dash
			items := []network.VertexSpec{{ID: "δ", Position: geom.V(-2, 2), Style: &style}, {ID: "e", Position: geom.V(0, 4)}}
			if err = g.AddVertices(items); err != nil {
				t.Fatal(err)
			}
			d, _ := g.Vertex("δ")
			e, _ := g.Vertex("e")
			near(t, d.Center(), geom.V(-2, 2))
			near(t, e.Center(), geom.V(0, 4))
			if d.Style != style || e.Style.Fill != network.DefaultOptions().VertexColor {
				t.Fatal("initial style/default")
			}
			items[0].ID = "changed"
			items[0].Position = geom.V(100, 100)
			style.Fill = gomath.Green
			if d.Name != "δ" || d.Style.Fill != gomath.Red {
				t.Fatal("input alias")
			}
			if err = g.AddEdges([]network.Edge{{"b", "c"}, {"c", "δ"}, {"δ", "a"}}); err != nil {
				t.Fatal(err)
			}
			for p, v := range before.objects {
				actual := *p
				actual.Children = append([]*gomath.Object(nil), p.Children...)
				if p == root {
					continue
				}
				if !reflect.DeepEqual(actual, v) {
					t.Fatal("existing authored object changed", p.ID)
				}
			}
			if g.Object != root || root.ID != before.objects[root].ID || root.Style != before.objects[root].Style {
				t.Fatal("root changed")
			}
			if got, _ := g.Vertex("a"); got != a {
				t.Fatal("vertex pointer")
			}
			if got, _ := g.Vertex("b"); got != b {
				t.Fatal("vertex pointer")
			}
			if got, _ := g.Edge("a", "b"); got != old {
				t.Fatal("edge pointer")
			}
			link, _ := g.Edge("b", "c")
			shaft := link
			if directed {
				shaft = link.Children[0]
			}
			near(t, shaft.Path.Contours[0].Start, geom.V(2, .12))
			if directed {
				near(t, link.Children[1].Path.Contours[0].Start, geom.V(2, 1.88))
				assertArrowJoin(t, link)
			} else {
				near(t, shaft.Path.Contours[0].Curves[0].P3, geom.V(2, 1.88))
			}
			if err = g.RemoveEdges([]network.Edge{{"c", "δ"}}); err != nil {
				t.Fatal(err)
			}
			if err = g.RemoveVertices([]string{"b", "e"}); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(g.Vertices(), []string{"a", "c", "δ"}) || !reflect.DeepEqual(g.Edges(), []network.Edge{{"δ", "a"}}) {
				t.Fatal("cascade or survivor ordering", g.Vertices(), g.Edges())
			}
			want := []string{"δ"}
			if directed {
				want = nil
			}
			got, err := g.Neighbors("a")
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatal("neighbors", got, err)
			}
			if _, ok := g.Vertex("b"); ok {
				t.Fatal("stale vertex index")
			}
			if _, ok := g.Edge("a", "b"); ok {
				t.Fatal("stale edge index")
			}
			if err = g.AddVertices([]network.VertexSpec{{ID: "b", Position: geom.V(2, 0)}}); err != nil {
				t.Fatal(err)
			}
			newB, _ := g.Vertex("b")
			if newB == b || newB.ID == b.ID {
				t.Fatal("deleted identity recycled")
			}
			if err = g.AddEdges([]network.Edge{{"a", "b"}}); err != nil {
				t.Fatal(err)
			}
			newEdge, _ := g.Edge("a", "b")
			if newEdge == old || newEdge.ID == old.ID {
				t.Fatal("deleted edge identity recycled")
			}
			assertMutationOrder(t, g)
		})
	}
}

func TestGraphMutationCanonicalKeysAndDirectedReverse(t *testing.T) {
	g := mutationGraph(t, false)
	if got, _ := g.Edge("b", "a"); got == nil {
		t.Fatal("undirected reverse lookup")
	}
	s := captureMutation(g)
	for _, batch := range [][]network.Edge{{{"b", "a"}}, {{"b", "c"}, {"c", "b"}}, {{"a", "a"}}, {{"c", "missing"}}} {
		if err := g.AddEdges(batch); !errors.Is(err, network.ErrGraphInput) {
			t.Fatal(err, batch)
		}
		unchangedMutation(t, g, s)
	}
	if err := g.RemoveEdges([]network.Edge{{"b", "a"}}); err != nil {
		t.Fatal(err)
	}
	if err := g.AddEdges([]network.Edge{{"c", "a"}}); err != nil {
		t.Fatal(err)
	}
	if g.Edges()[0] != (network.Edge{"c", "a"}) {
		t.Fatal("canonical lookup changed authored orientation")
	}
	d := mutationGraph(t, true)
	if err := d.AddEdges([]network.Edge{{"b", "a"}}); err != nil {
		t.Fatal(err)
	}
	if err := d.RemoveEdges([]network.Edge{{"a", "b"}}); err != nil {
		t.Fatal(err)
	}
	if _, ok := d.Edge("b", "a"); !ok {
		t.Fatal("directed reverse was removed")
	}
}

func TestGraphMutationLateFailuresAreAtomic(t *testing.T) {
	for _, directed := range []bool{false, true} {
		g := mutationGraph(t, directed)
		c, _ := g.Vertex("c")
		c.MoveTo(geom.V(-2, 0))
		before := captureMutation(g)
		if err := g.AddEdges([]network.Edge{{"b", "c"}, {"a", "c"}}); !errors.Is(err, network.ErrGraphGeometry) {
			t.Fatal("late geometry accepted", err)
		}
		unchangedMutation(t, g, before)
		if err := g.AddVertices([]network.VertexSpec{{ID: "good", Position: geom.V(0, 5)}, {ID: "late", Position: geom.V(1e12, 0)}}); !errors.Is(err, network.ErrGraphGeometry) {
			t.Fatal("late range accepted", err)
		}
		unchangedMutation(t, g, before)
		for _, remove := range [][]string{{"a", "missing"}, {"a", "a"}} {
			if err := g.RemoveVertices(remove); err == nil {
				t.Fatal("invalid remove")
			}
			unchangedMutation(t, g, before)
		}
		for _, remove := range [][]network.Edge{{{"a", "b"}, {"b", "c"}}, {{"a", "b"}, {"a", "b"}}} {
			if err := g.RemoveEdges(remove); err == nil {
				t.Fatal("invalid remove")
			}
			unchangedMutation(t, g, before)
		}
	}
}

func TestGraphMutationEmptyNoopNilAndInvalidVertices(t *testing.T) {
	g, err := network.NewGraph(nil, nil, network.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	before := captureMutation(g)
	for _, call := range []func() error{func() error { return g.AddVertices(nil) }, func() error { return g.AddEdges(nil) }, func() error { return g.RemoveVertices(nil) }, func() error { return g.RemoveEdges(nil) }} {
		if err = call(); err != nil {
			t.Fatal(err)
		}
		unchangedMutation(t, g, before)
	}
	if err = g.AddVertices([]network.VertexSpec{{ID: "a", Position: geom.V(0, 0)}}); err != nil {
		t.Fatal(err)
	}
	if err = g.RemoveVertices([]string{"a"}); err != nil {
		t.Fatal(err)
	}
	if len(g.Object.Children) != 0 {
		t.Fatal("remove all")
	}
	var nilGraph *network.Graph
	for _, call := range []func() error{func() error { return nilGraph.AddVertices(nil) }, func() error { return nilGraph.AddEdges(nil) }, func() error { return nilGraph.RemoveVertices(nil) }, func() error { return nilGraph.RemoveEdges(nil) }, nilGraph.UpdateEdges, func() error { return nilGraph.Relayout(network.DefaultLayout()) }, func() error { return nilGraph.SetPositions(nil) }} {
		if err = call(); err == nil {
			t.Fatal("nil accepted")
		}
	}
	badStyle := gomath.DefaultStyle()
	badStyle.Opacity = 2
	for _, v := range []network.VertexSpec{{ID: ""}, {ID: "a\x00b"}, {ID: string([]byte{255})}, {ID: "n", Position: geom.V(math.NaN(), 0)}, {ID: "n", Position: geom.V(1e13, 0)}, {ID: "n", Position: geom.Vec3{Z: 1}}, {ID: "n", Style: &badStyle}} {
		if err = g.AddVertices([]network.VertexSpec{v}); err == nil {
			t.Fatal("bad vertex accepted", v)
		}
	}
	if err = g.AddVertices([]network.VertexSpec{{ID: "n"}, {ID: "n"}}); err == nil {
		t.Fatal("duplicate new vertices")
	}
}

func TestGraphMutationTopologyIDsAndInvalidAuthoring(t *testing.T) {
	for _, mutate := range []func(*network.Graph){
		func(g *network.Graph) { g.Object = g.Object.Copy() },
		func(g *network.Graph) { g.Object.ID++ },
		func(g *network.Graph) { v, _ := g.Vertex("a"); v.ID++ },
		func(g *network.Graph) { e, _ := g.Edge("a", "b"); e.Children[0] = e.Children[0].Copy() },
		func(g *network.Graph) { e, _ := g.Edge("a", "b"); e.Children[0].ID++ },
		func(g *network.Graph) { v, _ := g.Vertex("a"); v.Children = []*gomath.Object{g.Object} },
		func(g *network.Graph) { g.Object.Path = gomath.Square(1).Path },
		func(g *network.Graph) {
			g.Object.Children[0], g.Object.Children[1] = g.Object.Children[1], g.Object.Children[0]
		},
	} {
		g := mutationGraph(t, true)
		mutate(g)
		for _, call := range []func() error{func() error { return g.AddVertices(nil) }, func() error { return g.AddEdges(nil) }, func() error { return g.RemoveVertices([]string{"a"}) }, func() error { return g.RemoveEdges([]network.Edge{{"a", "b"}}) }, g.UpdateEdges} {
			if err := call(); !errors.Is(err, network.ErrGraphTopology) {
				t.Fatal("topology accepted", err)
			}
		}
	}
	g := mutationGraph(t, true)
	a, _ := g.Vertex("a")
	a.Style.Opacity = math.NaN()
	if err := g.AddEdges(nil); err == nil {
		t.Fatal("invalid surviving style")
	}
	g = mutationGraph(t, true)
	g.Object.Name = string([]byte{255})
	if err := g.RemoveVertices(nil); !errors.Is(err, network.ErrGraphInput) {
		t.Fatal(err)
	}
}
