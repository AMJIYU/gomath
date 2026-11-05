package network_test

import (
	"bytes"
	"gomath"
	"gomath/geom"
	"gomath/network"
	"gomath/render"
	"math"
	"reflect"
	"sync"
	"testing"
)

func TestCurvedIntermediateFramesPixelsAndConcurrentSnapshots(t *testing.T) {
	o := network.DefaultCurvedOptions()
	loop := network.DefaultLoopOptions()
	routes := []network.CurvedEdge{{From: "a", To: "b", Sweep: math.Pi / 2}, {From: "b", To: "a", Sweep: math.Pi / 2}, {From: "a", To: "a", Loop: &loop}}
	g, e := network.NewCurvedDiGraph([]string{"a", "b"}, routes, o)
	if e != nil {
		t.Fatal(e)
	}
	if e = g.SetPositions(map[string]geom.Vec3{"a": geom.V(-1.5, 0), "b": geom.V(1.5, 0)}); e != nil {
		t.Fatal(e)
	}
	scene := gomath.NewScene()
	if e = scene.Add(g.Object); e != nil {
		t.Fatal(e)
	}
	a, _ := g.Vertex("a")
	if e = scene.Add(a); e == nil {
		t.Fatal("borrowed child accepted as root")
	}
	if e = scene.Play(2, gomath.Shift(g.Object, geom.V(0, 1))); e != nil {
		t.Fatal(e)
	}
	cfg := render.DefaultConfig()
	cfg.Width = 192
	cfg.Height = 144
	cfg.FrameWidth = 6
	draw := func(frame *gomath.Frame) []byte {
		r, e := render.NewRasterizer(cfg)
		if e != nil {
			t.Error(e)
			return nil
		}
		img, e := r.Render(frame)
		if e != nil {
			t.Error(e)
			return nil
		}
		return img.Pix
	}
	midpoint, e := scene.Sample(1)
	if e != nil {
		t.Fatal(e)
	}
	pixels := draw(midpoint)
	// Independently author the same halfway pose. No interpolation formula from
	// the scene internals is used in the expected render.
	expected, e := network.NewCurvedDiGraph([]string{"a", "b"}, routes, o)
	if e != nil {
		t.Fatal(e)
	}
	if e = expected.SetPositions(map[string]geom.Vec3{"a": geom.V(-1.5, .5), "b": geom.V(1.5, .5)}); e != nil {
		t.Fatal(e)
	}
	// Rebuilding trig/trim geometry after translation can differ by a few ulps;
	// the rasterized interior frame must still be pixel exact.
	if !bytes.Equal(pixels, draw(&gomath.Frame{Objects: []*gomath.Object{expected.Object}})) {
		t.Fatal("interior animation pixels differ from separately authored pose")
	}
	start, _ := scene.Sample(0)
	end, _ := scene.Sample(2)
	if bytes.Equal(pixels, draw(start)) || bytes.Equal(pixels, draw(end)) {
		t.Fatal("animation lacks visible intermediate movement")
	}
	lit := 0
	for i := 0; i < len(pixels); i += 4 {
		if pixels[i] > 40 || pixels[i+1] > 40 {
			lit++
		}
	}
	if lit < 150 {
		t.Fatal("no visible curved edges/loop")
	}
	// The snapshot's shaft/head joins and authored identities survive animation.
	for _, edge := range g.Edges() {
		ar, _ := g.Edge(edge.From, edge.To)
		snap := midpoint.Find(ar.ID)
		if snap == nil || snap == ar || snap.Children[0].ID != ar.Children[0].ID {
			t.Fatal("snapshot identity/reference contract")
		}
		assertArrowJoin(t, snap)
	}
	var wg sync.WaitGroup
	for k := range 8 {
		wg.Go(func() {
			for j := range 8 {
				at := float64((k*3+j*5)%17) / 8
				f, e := scene.Sample(at)
				if e != nil {
					t.Error(e)
					return
				}
				if e = f.Objects[0].Validate(); e != nil {
					t.Error(e)
				}
				f.Objects[0].Shift(geom.V(20, 20))
				f, e = scene.Sample(1)
				if e != nil {
					t.Error(e)
					return
				}
				if !reflect.DeepEqual(f, midpoint) || !bytes.Equal(pixels, draw(f)) {
					t.Error("concurrent out-of-order snapshot changed")
				}
			}
		})
	}
	wg.Wait()
}

func TestCurvedConcurrentIndependentConstructionAndReadCopies(t *testing.T) {
	o := network.DefaultCurvedOptions()
	edges := []network.CurvedEdge{{From: "a", To: "b", Sweep: .8}, {From: "b", To: "a", Sweep: .8}}
	original := curvedPair(t, .8)
	want := graphPaths(original)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			g := curvedPair(t, .8)
			if !reflect.DeepEqual(want, graphPaths(g)) {
				t.Error("nondeterministic geometry")
			}
			for range 5 {
				vs := original.Vertices()
				es := original.Edges()
				cs := original.CurvedEdges()
				vs[0] = "changed"
				es[0].From = "changed"
				cs[0].Sweep = 0
				if original.Vertices()[0] != "a" || original.Edges()[0].From != "a" || original.CurvedEdges()[0].Sweep != .8 {
					t.Error("aliased metadata")
				}
			}
			g, e := network.NewCurvedDiGraph([]string{"a", "b"}, edges, o)
			if e != nil {
				t.Error(e)
				return
			}
			layout := network.DefaultLayout()
			layout.Kind = network.Spring
			layout.Seed = 42
			if e = g.Relayout(layout); e != nil {
				t.Error(e)
			}
		})
	}
	wg.Wait()
}
