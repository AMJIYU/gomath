package network_test

import (
	"fmt"
	"gomath/geom"
	"gomath/network"
	"testing"
)

// A bounded shared-host observation. No claims about full-library throughput.
func BenchmarkCurvedNetwork(b *testing.B) {
	ids := make([]string, 12)
	edges := make([]network.CurvedEdge, 0, 15)
	for i := range ids {
		ids[i] = fmt.Sprintf("v%d", i)
	}
	for i := range ids {
		edges = append(edges, network.CurvedEdge{From: ids[i], To: ids[(i+1)%len(ids)], Sweep: .6})
	}
	for i := 0; i < 3; i++ {
		loop := network.DefaultLoopOptions()
		edges = append(edges, network.CurvedEdge{From: ids[i], To: ids[i], Loop: &loop})
	}
	o := network.DefaultCurvedOptions()
	o.Options.Layout.Scale = 3
	b.Run("build_12vertices_15arrows", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, e := network.NewCurvedDiGraph(ids, edges, o); e != nil {
				b.Fatal(e)
			}
		}
	})
	g, e := network.NewCurvedDiGraph(ids, edges, o)
	if e != nil {
		b.Fatal(e)
	}
	positions := make(map[string]geom.Vec3, len(ids))
	for _, id := range ids {
		n, _ := g.Vertex(id)
		positions[id] = n.Center().Add(geom.V(.1, .1))
	}
	b.Run("setpositions_12vertices_15arrows", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if e := g.SetPositions(positions); e != nil {
				b.Fatal(e)
			}
		}
	})
}
