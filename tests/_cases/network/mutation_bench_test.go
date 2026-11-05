package network_test

import (
	"fmt"
	"gomath/geom"
	"gomath/network"
	"testing"
)

// A bounded shared-host observation; excludes Scene/render/media/reference work.
func BenchmarkGraphMutation(b *testing.B) {
	ids := make([]string, 16)
	edges := make([]network.Edge, 16)
	for i := range ids {
		ids[i] = fmt.Sprintf("v%d", i)
	}
	for i := range edges {
		edges[i] = network.Edge{From: ids[i], To: ids[(i+1)%len(ids)]}
	}
	opts := network.DefaultOptions()
	opts.Layout.Scale = 3
	vertices := make([]network.VertexSpec, 8)
	extra := make([]network.Edge, 8)
	remove := make([]string, 8)
	for i := range vertices {
		id := fmt.Sprintf("added%d", i)
		vertices[i] = network.VertexSpec{ID: id, Position: geom.V(5, float64(i))}
		extra[i] = network.Edge{From: ids[i], To: id}
		remove[i] = id
	}
	b.Run("construct_16vertices_16directed", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err := network.NewDiGraph(ids, edges, opts); err != nil {
				b.Fatal(err)
			}
		}
	})
	g, err := network.NewDiGraph(ids, edges, opts)
	if err != nil {
		b.Fatal(err)
	}
	b.Run("four_batches_8vertices_8edges", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if err := g.AddVertices(vertices); err != nil {
				b.Fatal(err)
			}
			if err := g.AddEdges(extra); err != nil {
				b.Fatal(err)
			}
			if err := g.RemoveEdges(extra); err != nil {
				b.Fatal(err)
			}
			if err := g.RemoveVertices(remove); err != nil {
				b.Fatal(err)
			}
		}
	})
}
