package network_test

import (
	"fmt"
	"gomath/geom"
	"gomath/network"
	"math"
	"testing"
)

func FuzzGraphMutationTransactions(f *testing.F) {
	f.Add([]byte{0, 1, 2, 3, 4, 5}, float64(4), float64(3))
	f.Add([]byte{0, 0, 1, 1, 2, 3}, math.NaN(), math.Inf(1))
	f.Add([]byte{4, 3, 1, 0, 2}, float64(1e12), float64(1e-200))
	f.Fuzz(func(t *testing.T, ops []byte, x, y float64) {
		if len(ops) > 32 {
			ops = ops[:32]
		}
		g := mutationGraph(t, len(ops)%2 == 0)
		for i, op := range ops {
			before := captureMutation(g)
			ids := g.Vertices()
			var err error
			switch op % 6 {
			case 0:
				err = g.AddVertices([]network.VertexSpec{{ID: fmt.Sprintf("n%d", i), Position: geom.V(x, y)}})
			case 1:
				if len(ids) > 0 {
					err = g.AddEdges([]network.Edge{{From: ids[int(op)%len(ids)], To: ids[(int(op)+1)%len(ids)]}})
				} else {
					err = g.AddEdges([]network.Edge{{"missing", "other"}})
				}
			case 2:
				if len(ids) > 0 {
					err = g.RemoveVertices([]string{ids[int(op)%len(ids)]})
				} else {
					err = g.RemoveVertices([]string{"missing"})
				}
			case 3:
				edges := g.Edges()
				if len(edges) > 0 {
					err = g.RemoveEdges([]network.Edge{edges[int(op)%len(edges)]})
				} else {
					err = g.RemoveEdges([]network.Edge{{"a", "b"}})
				}
			case 4:
				err = g.AddVertices([]network.VertexSpec{{ID: "valid", Position: geom.V(5, 5)}, {ID: "late", Position: geom.V(x, y)}})
			case 5:
				err = g.RemoveVertices([]string{"a", "missing"})
			}
			if err != nil {
				unchangedMutation(t, g, before)
			} else {
				if g.Object != before.root || g.Object.ID != before.objects[before.root].ID {
					t.Fatal("root identity changed")
				}
				for _, id := range before.vertices {
					if v, ok := g.Vertex(id); ok {
						if _, existed := before.objects[v]; !existed {
							t.Fatal("survivor pointer replaced")
						}
					}
				}
			}
			assertMutationOrder(t, g)
		}
	})
}
