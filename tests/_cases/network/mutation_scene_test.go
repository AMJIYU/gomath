package network_test

import (
	"bytes"
	"gomath"
	"gomath/geom"
	"gomath/network"
	"gomath/render"
	"reflect"
	"sync"
	"testing"
)

func TestGraphMutationCapturedSceneIndependentPixelsAndConcurrentAuthoring(t *testing.T) {
	g := mutationGraph(t, true)
	s := gomath.NewScene()
	if err := s.Add(g.Object); err != nil {
		t.Fatal(err)
	}
	if err := s.Play(2, gomath.Shift(g.Object, geom.V(0, -1))); err != nil {
		t.Fatal(err)
	}
	baseline, err := s.Sample(1)
	if err != nil {
		t.Fatal(err)
	}
	config := render.DefaultConfig()
	config.Width = 192
	config.Height = 144
	config.FrameWidth = 8
	draw := func(f *gomath.Frame) []byte {
		r, e := render.NewRasterizer(config)
		if e != nil {
			t.Error(e)
			return nil
		}
		im, e := r.Render(f)
		if e != nil {
			t.Error(e)
			return nil
		}
		return im.Pix
	}
	pixels := draw(baseline)
	a, _ := g.Vertex("a")
	if err = s.Add(a); err == nil {
		t.Fatal("borrowed vertex accepted as root")
	}
	var wg sync.WaitGroup
	for k := range 8 {
		wg.Go(func() {
			for j := range 12 {
				f, e := s.Sample(float64((k*3+j*7)%17) / 8)
				if e != nil {
					t.Error(e)
					return
				}
				f.Objects[0].Shift(geom.V(20, 20))
				again, e := s.Sample(1)
				if e != nil || !reflect.DeepEqual(again, baseline) {
					t.Error("captured scene changed", e)
					return
				}
			}
		})
	}
	// Authoring modifies only the original graph; already captured tracks and
	// snapshots contain no references to new topology or these metadata arrays.
	for range 12 {
		if err = g.AddVertices([]network.VertexSpec{{ID: "temporary", Position: geom.V(0, 3)}}); err != nil {
			t.Fatal(err)
		}
		if err = g.AddEdges([]network.Edge{{"c", "temporary"}}); err != nil {
			t.Fatal(err)
		}
		if err = g.RemoveVertices([]string{"temporary"}); err != nil {
			t.Fatal(err)
		}
	}
	wg.Wait()
	if err = g.RemoveVertices([]string{"b"}); err != nil {
		t.Fatal(err)
	}
	if err = g.AddVertices([]network.VertexSpec{{ID: "new", Position: geom.V(0, 2)}}); err != nil {
		t.Fatal(err)
	}
	f, err := s.Sample(1)
	if err != nil || !bytes.Equal(pixels, draw(f)) {
		t.Fatal("old timeline pixels changed", err)
	}
	fresh := gomath.NewScene()
	if err = fresh.Add(g.Object); err != nil {
		t.Fatal(err)
	}
	newFrame, err := fresh.Sample(0)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(pixels, draw(newFrame)) {
		t.Fatal("new capture did not see changed topology")
	}
	if baseline.Find(a.ID) == a {
		t.Fatal("borrowed snapshot identity")
	}
}

func TestGraphMutationConcurrentIndependentGraphsAndCopiedInputs(t *testing.T) {
	options := network.DefaultOptions()
	input := []string{"a", "b"}
	edges := []network.Edge{{"a", "b"}}
	g, err := network.NewGraph(input, edges, options)
	if err != nil {
		t.Fatal(err)
	}
	input[0] = "changed"
	edges[0].From = "changed"
	options.VertexColor = gomath.Red
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 8 {
				v := g.Vertices()
				e := g.Edges()
				v[0] = "no"
				e[0].From = "no"
				fresh, err := network.NewGraph(nil, nil, network.DefaultOptions())
				if err != nil {
					t.Error(err)
					return
				}
				if err = fresh.AddVertices([]network.VertexSpec{{ID: "a", Position: geom.V(-2, 0)}, {ID: "b", Position: geom.V(2, 0)}}); err != nil {
					t.Error(err)
					return
				}
				if err = fresh.AddEdges([]network.Edge{{"a", "b"}}); err != nil {
					t.Error(err)
					return
				}
				if err = fresh.RemoveEdges([]network.Edge{{"b", "a"}}); err != nil {
					t.Error(err)
					return
				}
				if err = fresh.Object.Validate(); err != nil {
					t.Error(err)
				}
			}
		})
	}
	wg.Wait()
	if g.Vertices()[0] != "a" || g.Edges()[0].From != "a" {
		t.Fatal("metadata aliased")
	}
}
