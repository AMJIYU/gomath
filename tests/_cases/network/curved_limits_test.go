package network_test

import (
	"errors"
	"gomath"
	"gomath/geom"
	"gomath/network"
	"gomath/path"
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestCurvedStrictInputRangesAndLimits(t *testing.T) {
	o := network.DefaultCurvedOptions()
	pair := []network.CurvedEdge{{From: "a", To: "b", Sweep: .8}}
	for _, v := range [][]string{{"a", "a"}, {""}, {"a"}} {
		g, e := network.NewCurvedDiGraph(v, pair, o)
		if g != nil || !errors.Is(e, network.ErrCurvedOptions) {
			t.Fatal("vertices", g, e)
		}
	}
	loop := network.DefaultLoopOptions()
	for _, es := range [][]network.CurvedEdge{
		{{From: "a", To: "b"}, {From: "a", To: "b"}}, {{From: "a", To: "missing"}}, {{From: "a", To: "a"}}, {{From: "a", To: "b", Sweep: math.NaN()}}, {{From: "a", To: "b", Sweep: math.Pi + 1e-10}}, {{From: "a", To: "b", Loop: &loop}}, {{From: "a", To: "a", Sweep: .1, Loop: &loop}}, {{From: "a", To: "a", Loop: &network.LoopOptions{Side: "random", Radius: .3}}}, {{From: "a", To: "a", Loop: &network.LoopOptions{Side: network.LoopTop, Radius: 0}}},
	} {
		g, e := network.NewCurvedDiGraph([]string{"a", "b"}, es, o)
		if g != nil || !errors.Is(e, network.ErrCurvedOptions) {
			t.Fatal("route options", g, e)
		}
	}
	for _, mut := range []func(*network.CurvedOptions){
		func(o *network.CurvedOptions) { o.TipWidth = 0 }, func(o *network.CurvedOptions) { o.Options.TipSize = math.NaN() }, func(o *network.CurvedOptions) { o.Options.VertexRadius = 1e-12 }, func(o *network.CurvedOptions) { o.Options.EdgeColor.R = 2 }, func(o *network.CurvedOptions) { o.Limits.MaxContours = -1 }, func(o *network.CurvedOptions) { o.Limits.MaxWork = 10_000_000_000_001 }, func(o *network.CurvedOptions) { o.Limits.MaxStorageBytes = 0 }, func(o *network.CurvedOptions) { o.Options.Layout.Kind = "tree" },
	} {
		q := o
		mut(&q)
		g, e := network.NewCurvedDiGraph([]string{"a", "b"}, pair, q)
		if g != nil || !errors.Is(e, network.ErrCurvedOptions) {
			t.Fatal("options", g, e)
		}
	}
	for _, which := range []string{"vertices", "edges", "cubics", "contours", "work", "storage"} {
		q := o
		switch which {
		case "vertices":
			q.Limits.MaxVertices = 1
		case "edges":
			q.Limits.MaxEdges = 0
		case "cubics":
			q.Limits.MaxCubics = 20
		case "contours":
			q.Limits.MaxContours = 3
		case "work":
			q.Limits.MaxWork = 1
		case "storage":
			q.Limits.MaxStorageBytes = 8 << 20
		}
		g, e := network.NewCurvedDiGraph([]string{"a", "b"}, pair, q)
		if g != nil || !errors.Is(e, network.ErrCurvedLimit) {
			t.Fatal(which, g, e)
		}
	}
	// Successful exact reserved bounds: 2 dots, 1 arrow, 4 contours and 6 name bytes.
	q := o
	q.Limits.MaxVertices = 2
	q.Limits.MaxEdges = 1
	q.Limits.MaxCubics = 21
	q.Limits.MaxContours = 4
	q.Limits.MaxWork = 100_000_000 + 4*(2+21+4) + 2
	q.Limits.MaxStorageBytes = 8<<20 + 2*2048 + 4096 + (21+4)*1024 + 6
	g, e := network.NewCurvedDiGraph([]string{"a", "b"}, pair, q)
	if e != nil {
		t.Fatal("exact reserve", e)
	}
	if e = g.UpdateEdges(); e != nil {
		t.Fatal("exact update reserve", e)
	}
	q.Limits.MaxStorageBytes--
	if g, e := network.NewCurvedDiGraph([]string{"a", "b"}, pair, q); g != nil || !errors.Is(e, network.ErrCurvedLimit) {
		t.Fatal("reserve off by one", g, e)
	}
	for _, mut := range []func(*network.CurvedOptions){func(o *network.CurvedOptions) { o.Options.TipSize = 10 }, func(o *network.CurvedOptions) { o.TipWidth = 10 }, func(o *network.CurvedOptions) { o.Options.Layout.Center = geom.V(1e13, 0) }} {
		q := o
		mut(&q)
		g, e := network.NewCurvedDiGraph([]string{"a", "b"}, pair, q)
		if g != nil || e == nil {
			t.Fatal("invisible or unbounded geometry", g, e)
		}
	}
}
func TestCurvedTopologyAndLateFailureKeepPaths(t *testing.T) {
	mutations := []func(*network.CurvedDiGraph){
		func(g *network.CurvedDiGraph) { g.Object = g.Object.Copy() },
		func(g *network.CurvedDiGraph) { g.Object.ID++ },
		func(g *network.CurvedDiGraph) { g.Object.Path = gomath.Line(geom.V(0, 0), geom.V(1, 0)).Path },
		func(g *network.CurvedDiGraph) {
			v, _ := g.Vertex("a")
			v.Children = []*gomath.Object{gomath.Circle(.1)}
		},
		func(g *network.CurvedDiGraph) { a, _ := g.Edge("a", "b"); a.Children[1] = a.Children[1].Copy() },
		func(g *network.CurvedDiGraph) { a, _ := g.Edge("a", "b"); a.Children[1].ID++ },
		func(g *network.CurvedDiGraph) { a, _ := g.Edge("a", "b"); a.Bitmap = &gomath.Bitmap{} },
		func(g *network.CurvedDiGraph) { v, _ := g.Vertex("a"); v.Clip = &gomath.Clip{} },
	}
	for _, mut := range mutations {
		g := curvedPair(t, .7)
		mut(g)
		before := graphPaths(g)
		if e := g.UpdateEdges(); !errors.Is(e, network.ErrCurvedTopology) {
			t.Fatal("topology", e)
		}
		if !reflect.DeepEqual(before, graphPaths(g)) {
			t.Fatal("failed topology update wrote paths")
		}
	}
	o := network.DefaultCurvedOptions()
	es := []network.CurvedEdge{{From: "a", To: "b", Sweep: .5}, {From: "b", To: "c", Sweep: .5}}
	g, e := network.NewCurvedDiGraph([]string{"a", "b", "c"}, es, o)
	if e != nil {
		t.Fatal(e)
	}
	before := graphPaths(g)
	if e = g.SetPositions(map[string]geom.Vec3{"a": geom.V(-3, 1), "b": geom.V(0, 0), "c": geom.V(0, .1)}); e == nil || !reflect.DeepEqual(before, graphPaths(g)) {
		t.Fatal("late edge failure published earlier paths", e)
	}
	v, _ := g.Vertex("a")
	v.Path = path.Path{Contours: []path.Contour{{Start: geom.V(0, 0)}}}
	if e = g.UpdateEdges(); !errors.Is(e, network.ErrCurvedGeometry) {
		t.Fatal("collapsed bbox", e)
	}
	g = curvedPair(t, .7)
	v, _ = g.Vertex("a")
	v.Name = strings.Repeat("n", 1024)
	if e = g.UpdateEdges(); e != nil {
		t.Fatal("bounded renamed vertex", e)
	}
}
func TestCurvedEmptyNilSingletonAndLoopRange(t *testing.T) {
	var nilg *network.CurvedDiGraph
	if nilg.Vertices() != nil || nilg.Edges() != nil || nilg.CurvedEdges() != nil {
		t.Fatal("nil getters")
	}
	if _, ok := nilg.Vertex("a"); ok {
		t.Fatal("nil vertex")
	}
	if _, ok := nilg.Edge("a", "b"); ok {
		t.Fatal("nil edge")
	}
	if e := nilg.UpdateEdges(); !errors.Is(e, network.ErrCurvedOptions) {
		t.Fatal("nil update", e)
	}
	if e := nilg.SetPositions(nil); !errors.Is(e, network.ErrCurvedOptions) {
		t.Fatal("nil positions", e)
	}
	if e := nilg.Relayout(network.DefaultLayout()); !errors.Is(e, network.ErrCurvedOptions) {
		t.Fatal("nil relayout", e)
	}
	if _, e := nilg.Neighbors("a"); !errors.Is(e, network.ErrCurvedOptions) {
		t.Fatal("nil neighbors", e)
	}
	for _, v := range [][]string{nil, {"a"}} {
		g, e := network.NewCurvedDiGraph(v, nil, network.DefaultCurvedOptions())
		if e != nil {
			t.Fatal(e)
		}
		if e = g.UpdateEdges(); e != nil {
			t.Fatal(e)
		}
		if e = g.Relayout(network.DefaultLayout()); e != nil {
			t.Fatal(e)
		}
	}
	for _, r := range []float64{1e-9, 1e6} {
		loop := network.LoopOptions{Side: network.LoopTop, Radius: r}
		g, e := network.NewCurvedDiGraph([]string{"a"}, []network.CurvedEdge{{From: "a", To: "a", Loop: &loop}}, network.DefaultCurvedOptions())
		if g != nil || !errors.Is(e, network.ErrCurvedGeometry) {
			t.Fatal("unexposed loop", r, g, e)
		}
	}
}

func TestCurvedAuthoredGeometryAndFutureOutputAreBothBudgeted(t *testing.T) {
	o := network.DefaultCurvedOptions()
	o.Limits.MaxCubics = 25
	g, e := network.NewCurvedDiGraph([]string{"a", "b"}, []network.CurvedEdge{{From: "a", To: "b"}}, o)
	if e != nil {
		t.Fatal(e)
	}
	v, _ := g.Vertex("a")
	v.Path = gomath.RegularPolygon(11, .12).MoveTo(v.Center()).Path
	before := graphPaths(g)
	if e = g.UpdateEdges(); !errors.Is(e, network.ErrCurvedLimit) || !reflect.DeepEqual(before, graphPaths(g)) {
		t.Fatal("future edge geometry not reserved", e)
	}
	o = network.DefaultCurvedOptions()
	o.Limits.MaxContours = 4
	g, e = network.NewCurvedDiGraph([]string{"a", "b"}, []network.CurvedEdge{{From: "a", To: "b"}}, o)
	if e != nil {
		t.Fatal(e)
	}
	v, _ = g.Vertex("a")
	v.Path.Contours = append(v.Path.Contours, path.Contour{Start: v.Center()})
	if e = g.UpdateEdges(); !errors.Is(e, network.ErrCurvedLimit) {
		t.Fatal("point-only contour unbudgeted", e)
	}
	// Relayout must include its layout iterations in addition to edge geometry.
	o = network.DefaultCurvedOptions()
	o.Limits.MaxWork = 100_000_000 + 4*(2+21+4) + 2
	g, e = network.NewCurvedDiGraph([]string{"a", "b"}, []network.CurvedEdge{{From: "a", To: "b", Sweep: .5}}, o)
	if e != nil {
		t.Fatal(e)
	}
	before = graphPaths(g)
	layout := network.DefaultLayout()
	layout.Kind = network.Spring
	layout.Iterations = 100
	if e = g.Relayout(layout); !errors.Is(e, network.ErrCurvedLimit) || !reflect.DeepEqual(before, graphPaths(g)) {
		t.Fatal("layout work unbudgeted", e)
	}
}

func TestCurvedExtremeTrimCannotSilentlyLoseTipLength(t *testing.T) {
	g := curvedPair(t, .7)
	before := graphPaths(g)
	if e := g.SetPositions(map[string]geom.Vec3{"a": geom.V(-1e11, 0), "b": geom.V(1e11, 0)}); e == nil || !reflect.DeepEqual(before, graphPaths(g)) {
		t.Fatal("extreme length tolerance silently swallowed small tip", e)
	}
	o := network.DefaultCurvedOptions()
	o.TipWidth = 1e-6
	loop := network.LoopOptions{Side: network.LoopTop, Radius: 1000}
	if g, e := network.NewCurvedDiGraph([]string{"a"}, []network.CurvedEdge{{From: "a", To: "a", Loop: &loop}}, o); g != nil || !errors.Is(e, network.ErrCurvedGeometry) {
		t.Fatal("parent trim accuracy not preflighted", g, e)
	}
}
