package network_test

import (
	"errors"
	"gomath"
	"gomath/geom"
	"gomath/network"
	"gomath/path"
	"math"
	"reflect"
	"testing"
)

func curvedPair(t testing.TB, sweep float64) *network.CurvedDiGraph {
	t.Helper()
	g, e := network.NewCurvedDiGraph([]string{"a", "b"}, []network.CurvedEdge{{From: "a", To: "b", Sweep: sweep}}, network.DefaultCurvedOptions())
	if e != nil {
		t.Fatal(e)
	}
	if e = g.SetPositions(map[string]geom.Vec3{"a": geom.V(-2, 0), "b": geom.V(2, 0)}); e != nil {
		t.Fatal(e)
	}
	return g
}
func assertArrowJoin(t testing.TB, arrow *gomath.Object) {
	t.Helper()
	shaft, head := arrow.Children[0].Path.Contours[0], arrow.Children[1].Path.Contours[0]
	base := shaft.Curves[len(shaft.Curves)-1].P3
	left, right := head.Curves[0].P3, head.Curves[1].P3
	if base.Sub(left.Lerp(right, .5)).Norm() > coordinateTolerance(base, 1e-10) {
		t.Fatal("head disconnected from shaft", base, left, right)
	}
	if len(head.Curves) != 3 || !head.Closed {
		t.Fatal("invalid triangular head")
	}
}
func coordinateTolerance(p geom.Vec3, minimum float64) float64 {
	scale := max(1, math.Abs(p.X), math.Abs(p.Y))
	return max(minimum, 32*(math.Nextafter(scale, math.Inf(1))-scale))
}
func boundary(p geom.Vec3, b geom.Bounds) bool {
	tol := max(coordinateTolerance(b.Min, 1e-9), coordinateTolerance(b.Max, 1e-9))
	return p.X >= b.Min.X-tol && p.X <= b.Max.X+tol && p.Y >= b.Min.Y-tol && p.Y <= b.Max.Y+tol && min(math.Abs(p.X-b.Min.X), math.Abs(p.X-b.Max.X), math.Abs(p.Y-b.Min.Y), math.Abs(p.Y-b.Max.Y)) <= tol
}

func assertEndpointGeometry(t testing.TB, g *network.CurvedDiGraph) {
	t.Helper()
	for _, e := range g.Edges() {
		a, _ := g.Vertex(e.From)
		b, _ := g.Vertex(e.To)
		arrow, _ := g.Edge(e.From, e.To)
		shaft := arrow.Children[0].Path.Contours[0]
		if !boundary(shaft.Start, a.Bounds()) || !boundary(arrow.Children[1].Path.Contours[0].Start, b.Bounds()) {
			t.Fatal("endpoints not on node bboxes", e, shaft.Start, arrow.Children[1].Path.Contours[0].Start, a.Bounds(), b.Bounds())
		}
		assertArrowJoin(t, arrow)
		for _, o := range arrow.Children {
			for _, c := range o.Path.Contours {
				for _, v := range c.Curves {
					for j := range 201 {
						p := v.At(float64(j) / 200)
						for _, bb := range []geom.Bounds{a.Bounds(), b.Bounds()} {
							tol := max(coordinateTolerance(bb.Min, 1e-8), coordinateTolerance(bb.Max, 1e-8))
							if p.X > bb.Min.X+tol && p.X < bb.Max.X-tol && p.Y > bb.Min.Y+tol && p.Y < bb.Max.Y-tol {
								t.Fatal("visible path enters endpoint bbox", e, p)
							}
						}
					}
				}
			}
		}
	}
}
func TestCurvedSignedSweepReciprocalAndBboxEndpoints(t *testing.T) {
	for _, s := range []float64{-math.Pi, -math.Pi / 2, -.2, 0, .2, math.Pi / 2, math.Pi} {
		g := curvedPair(t, s)
		assertEndpointGeometry(t, g)
		ar, _ := g.Edge("a", "b")
		p := ar.Children[0].Path
		if s == 0 {
			if len(p.Contours[0].Curves) != 1 {
				t.Fatal("straight route not straight")
			}
		} else {
			mid := p.Contours[0].Curves[len(p.Contours[0].Curves)/2].P0
			if math.Abs(mid.Y) < .01 || mid.Y*s >= 0 {
				t.Fatal("missing or wrong signed curvature", s, mid)
			}
		}
	}
	edges := []network.CurvedEdge{{From: "a", To: "b", Sweep: math.Pi / 2}, {From: "b", To: "a", Sweep: math.Pi / 2}}
	g, e := network.NewCurvedDiGraph([]string{"a", "b"}, edges, network.DefaultCurvedOptions())
	if e != nil {
		t.Fatal(e)
	}
	if e = g.SetPositions(map[string]geom.Vec3{"a": geom.V(-2, 0), "b": geom.V(2, 0)}); e != nil {
		t.Fatal(e)
	}
	assertEndpointGeometry(t, g)
	a, _ := g.Edge("a", "b")
	b, _ := g.Edge("b", "a")
	if a.Bounds().Center().Y*b.Bounds().Center().Y >= 0 {
		t.Fatal("reciprocal routes not separated")
	}
	// Cropping follows actual square bboxes, not the original radius assumption.
	n, _ := g.Vertex("a")
	n.Path = gomath.Rectangle(.8, .5).Shift(geom.V(-2, 0)).Path
	if e = g.UpdateEdges(); e != nil {
		t.Fatal(e)
	}
	assertEndpointGeometry(t, g)
}
func TestCurvedLoopsAllSidesDirectionsAndCyclicLayouts(t *testing.T) {
	for _, side := range []network.LoopSide{network.LoopTop, network.LoopRight, network.LoopBottom, network.LoopLeft} {
		for _, cw := range []bool{false, true} {
			loop := network.DefaultLoopOptions()
			loop.Side = side
			loop.Clockwise = cw
			g, e := network.NewCurvedDiGraph([]string{"a"}, []network.CurvedEdge{{From: "a", To: "a", Loop: &loop}}, network.DefaultCurvedOptions())
			if e != nil {
				t.Fatalf("loop %s cw=%v %v", side, cw, e)
			}
			assertEndpointGeometry(t, g)
			n, _ := g.Vertex("a")
			ar, _ := g.Edge("a", "a")
			delta := ar.Bounds().Center().Sub(n.Center())
			switch side {
			case network.LoopTop:
				if delta.Y <= .15 {
					t.Fatal("loop misplaced")
				}
			case network.LoopRight:
				if delta.X <= .15 {
					t.Fatal("loop misplaced")
				}
			case network.LoopBottom:
				if delta.Y >= -.15 {
					t.Fatal("loop misplaced")
				}
			case network.LoopLeft:
				if delta.X >= -.15 {
					t.Fatal("loop misplaced")
				}
			}
			tip := ar.Children[1].Path.Contours[0].Start
			start := ar.Children[0].Path.Contours[0].Start
			if start == tip {
				t.Fatal("collapsed loop anchors")
			}
			adj, e := g.Neighbors("a")
			if e != nil || !reflect.DeepEqual(adj, []string{"a"}) {
				t.Fatal("loop adjacency", adj, e)
			}
		}
	}
	for _, kind := range []network.LayoutKind{network.Circular, network.Grid, network.Spring} {
		o := network.DefaultCurvedOptions()
		o.Options.Layout.Kind = kind
		o.Options.VertexRadius = .04
		o.Options.TipSize = .025
		o.TipWidth = .01
		loop := network.LoopOptions{Side: network.LoopTop, Radius: .1}
		es := []network.CurvedEdge{{From: "a", To: "b", Sweep: .5}, {From: "b", To: "c", Sweep: .5}, {From: "c", To: "a", Sweep: .5}, {From: "b", To: "b", Loop: &loop}}
		g, e := network.NewCurvedDiGraph([]string{"a", "b", "c"}, es, o)
		if e != nil {
			t.Fatal(kind, e)
		}
		if e = g.Relayout(o.Options.Layout); e != nil {
			t.Fatal(kind, e)
		}
		assertEndpointGeometry(t, g)
	}
}
func graphPaths(g *network.CurvedDiGraph) []path.Path {
	var out []path.Path
	g.Object.Walk(func(o *gomath.Object) { out = append(out, o.Path.Clone()) })
	return out
}
func TestCurvedAtomicUpdatesReferencesStylesAndInputCopies(t *testing.T) {
	o := network.DefaultCurvedOptions()
	loop := network.DefaultLoopOptions()
	vertices := []string{"a", "b"}
	edges := []network.CurvedEdge{{From: "a", To: "b", Sweep: .7}, {From: "a", To: "a", Loop: &loop}}
	g, e := network.NewCurvedDiGraph(vertices, edges, o)
	if e != nil {
		t.Fatal(e)
	}
	var refs []*gomath.Object
	var ids []uint64
	var styles []gomath.Style
	g.Object.Walk(func(n *gomath.Object) {
		refs = append(refs, n)
		ids = append(ids, n.ID)
		n.Style.Opacity = .7
		styles = append(styles, n.Style)
	})
	loop.Radius = math.NaN()
	vertices[0] = "bad"
	edges[0].Sweep = math.NaN()
	o.Options.TipSize = 999
	got := g.CurvedEdges()
	got[0].Sweep = math.Inf(1)
	got[1].Loop.Radius = 0
	if e = g.SetPositions(map[string]geom.Vec3{"a": geom.V(-2, .5), "b": geom.V(2, -.5)}); e != nil {
		t.Fatal(e)
	}
	v, _ := g.Vertex("b")
	v.Shift(geom.V(0, .25))
	if e = g.UpdateEdges(); e != nil {
		t.Fatal(e)
	}
	layout := network.DefaultLayout()
	layout.Scale = 3
	if e = g.Relayout(layout); e != nil {
		t.Fatal(e)
	}
	at := 0
	g.Object.Walk(func(n *gomath.Object) {
		if refs[at] != n || ids[at] != n.ID || styles[at] != n.Style {
			t.Error("update replaced identity/reference/style")
		}
		at++
	})
	before := graphPaths(g)
	for _, positions := range []map[string]geom.Vec3{{"a": geom.V(0, 0), "b": geom.V(0, 0)}, {"a": geom.V(math.NaN(), 0), "b": geom.V(2, 0)}, {"a": geom.V(0, 0)}, {"a": geom.V(0, 0), "unknown": geom.V(2, 0)}} {
		if e = g.SetPositions(positions); e == nil {
			t.Fatal("invalid positions accepted")
		}
		if !reflect.DeepEqual(before, graphPaths(g)) {
			t.Fatal("failed positions changed any path")
		}
	}
	layout.Kind = "unknown"
	if e = g.Relayout(layout); e == nil || !reflect.DeepEqual(before, graphPaths(g)) {
		t.Fatal("failed relayout changed paths")
	}
	a, _ := g.Vertex("a")
	b, _ := g.Vertex("b")
	b.MoveTo(a.Center())
	before = graphPaths(g)
	if e = g.UpdateEdges(); e == nil || !reflect.DeepEqual(before, graphPaths(g)) {
		t.Fatal("failed UpdateEdges changed caller state")
	}
	// A replaced shaft/head may have valid topology, but cannot replace borrowed references.
	ar, _ := g.Edge("a", "b")
	ar.Children[0] = ar.Children[0].Copy()
	if e = g.UpdateEdges(); !errors.Is(e, network.ErrCurvedTopology) {
		t.Fatal("replaced shaft accepted", e)
	}
}
