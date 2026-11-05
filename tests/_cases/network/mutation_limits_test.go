package network_test

import (
	"errors"
	"gomath"
	"gomath/geom"
	"gomath/network"
	"gomath/path"
	"math"
	"strings"
	"testing"
)

func TestGraphMutationExactEmptyAndSingletonBudgets(t *testing.T) {
	l := network.DefaultGraphLimits()
	l.MaxVertices = 0
	l.MaxEdges = 0
	l.MaxCubics = 0
	l.MaxContours = 0
	l.MaxNameBytes = 0
	l.MaxStorageBytes = 67584
	l.MaxWork = 128
	g, err := network.NewGraphWithLimits(nil, nil, network.DefaultOptions(), l)
	if err != nil {
		t.Fatal(err)
	}
	if err = g.AddVertices(nil); err != nil {
		t.Fatal(err)
	}
	l.MaxStorageBytes--
	if g, err = network.NewGraphWithLimits(nil, nil, network.DefaultOptions(), l); g != nil || !errors.Is(err, network.ErrGraphLimit) {
		t.Fatal("byte off-by-one", g, err)
	}
	l.MaxStorageBytes++
	l.MaxWork--
	if g, err = network.NewGraphWithLimits(nil, nil, network.DefaultOptions(), l); g != nil || !errors.Is(err, network.ErrGraphLimit) {
		t.Fatal("work off-by-one", g, err)
	}
	l = network.DefaultGraphLimits()
	l.MaxVertices = 1
	l.MaxEdges = 0
	l.MaxCubics = 5
	l.MaxContours = 1
	l.MaxNameBytes = 2
	l.MaxStorageBytes = 79880
	l.MaxWork = 355
	g, err = network.NewGraphWithLimits([]string{"a"}, nil, network.DefaultOptions(), l)
	if err != nil {
		t.Fatal("exact singleton", err)
	}
	if err = g.Relayout(network.DefaultLayout()); err != nil {
		t.Fatal(err)
	}
	before := captureMutation(g)
	if err = g.RemoveVertices([]string{"a"}); !errors.Is(err, network.ErrGraphLimit) {
		t.Fatal("removal work must also fit", err)
	}
	unchangedMutation(t, g, before)
	for _, change := range []func(*network.GraphLimits){func(l *network.GraphLimits) { l.MaxCubics-- }, func(l *network.GraphLimits) { l.MaxContours-- }, func(l *network.GraphLimits) { l.MaxNameBytes-- }, func(l *network.GraphLimits) { l.MaxStorageBytes-- }, func(l *network.GraphLimits) { l.MaxWork-- }} {
		bad := l
		change(&bad)
		if g, err = network.NewGraphWithLimits([]string{"a"}, nil, network.DefaultOptions(), bad); g != nil || !errors.Is(err, network.ErrGraphLimit) {
			t.Fatal("singleton boundary", g, err, bad)
		}
	}
}

func TestGraphMutationCountNameAndWorkCaps(t *testing.T) {
	for _, change := range []func(*network.GraphLimits){
		func(l *network.GraphLimits) { l.MaxVertices = 2 }, func(l *network.GraphLimits) { l.MaxCubics = 10 }, func(l *network.GraphLimits) { l.MaxContours = 2 }, func(l *network.GraphLimits) { l.MaxNameBytes = 4 },
	} {
		l := network.DefaultGraphLimits()
		change(&l)
		g, err := network.NewGraphWithLimits([]string{"a", "b"}, nil, network.DefaultOptions(), l)
		if err != nil {
			t.Fatal(err)
		}
		before := captureMutation(g)
		if err = g.AddVertices([]network.VertexSpec{{ID: "c", Position: geom.V(5, 0)}}); !errors.Is(err, network.ErrGraphLimit) {
			t.Fatal("new vertex budget", err)
		}
		unchangedMutation(t, g, before)
	}
	for _, directed := range []bool{false, true} {
		l := network.DefaultGraphLimits()
		l.MaxEdges = 0
		var g *network.Graph
		var err error
		if directed {
			var d *network.DiGraph
			d, err = network.NewDiGraphWithLimits([]string{"a", "b"}, nil, network.DefaultOptions(), l)
			if d != nil {
				g = d.Graph
			}
		} else {
			g, err = network.NewGraphWithLimits([]string{"a", "b"}, nil, network.DefaultOptions(), l)
		}
		if err != nil {
			t.Fatal(err)
		}
		before := captureMutation(g)
		if err = g.AddEdges([]network.Edge{{"a", "b"}}); !errors.Is(err, network.ErrGraphLimit) {
			t.Fatal(err)
		}
		unchangedMutation(t, g, before)
	}
	l := network.DefaultGraphLimits()
	l.MaxNameBytes = 12
	g, err := network.NewGraphWithLimits([]string{"a", "b"}, []network.Edge{{"a", "b"}}, network.DefaultOptions(), l)
	if err != nil {
		t.Fatal(err)
	}
	before := captureMutation(g)
	long := strings.Repeat("x", 13)
	for _, call := range []func() error{func() error { return g.AddVertices([]network.VertexSpec{{ID: long}}) }, func() error { return g.AddEdges([]network.Edge{{long, "a"}}) }, func() error { return g.RemoveVertices([]string{long}) }, func() error { return g.RemoveEdges([]network.Edge{{"a", long}}) }} {
		if err = call(); !errors.Is(err, network.ErrGraphLimit) {
			t.Fatal("request names must be bounded before lookup", err)
		}
		unchangedMutation(t, g, before)
	}
	g.Object.Name = long
	if err = g.UpdateEdges(); !errors.Is(err, network.ErrGraphLimit) {
		t.Fatal("authored root name", err)
	}
}

func TestGraphMutationGeometryAndPaintBudgetsBeforeTraversal(t *testing.T) {
	l := network.DefaultGraphLimits()
	l.MaxCubics = 15
	l.MaxContours = 4
	g, err := network.NewDiGraphWithLimits([]string{"a", "b"}, []network.Edge{{"a", "b"}}, network.DefaultOptions(), l)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := g.Vertex("a")
	edge, _ := g.Edge("a", "b")
	// Clearing current arrows cannot hide the four future output curves.
	edge.Children[0].Path = path.Path{}
	edge.Children[1].Path = path.Path{}
	for range 4 {
		c := a.Path.Contours[0].Curves
		end := c[len(c)-1].P3
		a.Path.Contours[0].Curves = append(c, path.Line(end, end))
	}
	before := captureMutation(g.Graph)
	if err = g.UpdateEdges(); !errors.Is(err, network.ErrGraphLimit) {
		t.Fatal("future curves not reserved", err)
	}
	unchangedMutation(t, g.Graph, before)
	g, err = network.NewDiGraphWithLimits([]string{"a", "b"}, []network.Edge{{"a", "b"}}, network.DefaultOptions(), l)
	if err != nil {
		t.Fatal(err)
	}
	a, _ = g.Vertex("a")
	a.Path.Contours = make([]path.Contour, 5)
	if err = g.RemoveVertices(nil); !errors.Is(err, network.ErrGraphLimit) {
		t.Fatal("point-only contours not counted", err)
	}
	l = network.DefaultGraphLimits()
	l.MaxStorageBytes = 110000
	u, err := network.NewGraphWithLimits([]string{"a"}, nil, network.DefaultOptions(), l)
	if err != nil {
		t.Fatal(err)
	}
	st := gomath.DefaultStyle()
	st.Dash, err = gomath.NewDashPattern([]float64{.1, .2}, 0)
	if err != nil {
		t.Fatal(err)
	}
	st.FillGradient, err = gomath.NewLinearGradient(geom.V(-1, 0), geom.V(1, 0), []gomath.GradientStop{{Offset: 0, Color: gomath.Red}, {Offset: 1, Color: gomath.Blue}})
	if err != nil {
		t.Fatal(err)
	}
	if err = u.AddVertices([]network.VertexSpec{{ID: "b", Position: geom.V(5, 0), Style: &st}}); !errors.Is(err, network.ErrGraphLimit) {
		t.Fatal("immutable paint payload reserve", err)
	}
}

func TestGraphMutationMalformedLimitsAndFiniteExtremePolicy(t *testing.T) {
	for _, change := range []func(*network.GraphLimits){func(l *network.GraphLimits) { l.MaxVertices = -1 }, func(l *network.GraphLimits) { l.MaxEdges = 65537 }, func(l *network.GraphLimits) { l.MaxCubics = 1000001 }, func(l *network.GraphLimits) { l.MaxContours = -1 }, func(l *network.GraphLimits) { l.MaxNameBytes = 1 << 40 }, func(l *network.GraphLimits) { l.MaxStorageBytes = 0 }, func(l *network.GraphLimits) { l.MaxStorageBytes = 1 << 40 }, func(l *network.GraphLimits) { l.MaxWork = 1 << 40 }} {
		l := network.DefaultGraphLimits()
		change(&l)
		if g, err := network.NewGraphWithLimits(nil, nil, network.DefaultOptions(), l); g != nil || !errors.Is(err, network.ErrGraphInput) {
			t.Fatal("limits", g, err)
		}
	}
	for _, name := range []string{"a\x00b", string([]byte{255})} {
		if g, err := network.NewGraph([]string{name}, nil, network.DefaultOptions()); g != nil || !errors.Is(err, network.ErrGraphInput) {
			t.Fatal("bad constructor name", g, err)
		}
	}
	g := mutationGraph(t, true)
	st := gomath.DefaultStyle()
	st.StrokeWidth = math.MaxFloat64
	if err := g.AddVertices([]network.VertexSpec{{ID: "d", Position: geom.V(0, 6), Style: &st}}); err == nil {
		t.Fatal("finite unusable stroke")
	}
	before := captureMutation(g)
	if err := g.SetPositions(map[string]geom.Vec3{"a": geom.V(-1e12, 0), "b": geom.V(1e12, 0), "c": geom.V(0, 1e12)}); err == nil {
		t.Fatal("controls outside coordinate bounds accepted")
	}
	unchangedMutation(t, g, before)
	// A normal well-separated graph can be scaled down without an arbitrary
	// scene-unit epsilon or a silent layout fallback.
	o := network.DefaultOptions()
	o.VertexRadius = 1e-30
	o.EdgeWidth = 1e-32
	o.TipSize = 1e-30
	o.Layout.Scale = 1e-28
	if _, err := network.NewDiGraph([]string{"a", "b"}, []network.Edge{{"a", "b"}}, o); err != nil {
		t.Fatal("representable tiny graph", err)
	}
}

func TestGraphMutationLayoutBudgetAtomicAndCurvedIsolation(t *testing.T) {
	l := network.DefaultGraphLimits()
	l.MaxWork = 10000
	ids := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"}
	g, err := network.NewGraphWithLimits(ids, nil, network.DefaultOptions(), l)
	if err != nil {
		t.Fatal(err)
	}
	before := captureMutation(g)
	o := network.DefaultLayout()
	o.Kind = network.Spring
	if err = g.Relayout(o); !errors.Is(err, network.ErrGraphLimit) {
		t.Fatal("layout work reserve", err)
	}
	unchangedMutation(t, g, before)
	c, err := network.NewCurvedDiGraph([]string{"a", "b"}, []network.CurvedEdge{{From: "a", To: "b", Sweep: .5}}, network.DefaultCurvedOptions())
	if err != nil {
		t.Fatal(err)
	}
	if _, exposes := any(c).(interface {
		AddVertices([]network.VertexSpec) error
	}); exposes {
		t.Fatal("curved graph exposes legacy topology mutation")
	}
	if err = c.UpdateEdges(); err != nil {
		t.Fatal("curved preflight changed", err)
	}
	root := c.Object
	root.Children[0] = root.Children[0].Copy()
	if err = c.UpdateEdges(); !errors.Is(err, network.ErrCurvedTopology) {
		t.Fatal("curved topology guard weakened", err)
	}
}
