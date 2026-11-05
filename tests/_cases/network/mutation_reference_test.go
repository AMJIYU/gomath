package network_test

import (
	"encoding/json"
	"gomath/geom"
	"gomath/network"
	"os"
	"reflect"
	"testing"
)

func TestGraphMutationPinnedManimOrderGeometryAndCleanup(t *testing.T) {
	data, err := os.ReadFile("mutation_manim.json")
	if err != nil {
		t.Fatal(err)
	}
	var ref struct {
		Commit string `json:"reference_commit"`
		Cases  []struct {
			Kind          string
			RemovedAbsent bool `json:"removed_absent"`
			Steps         []struct {
				Operation string
				Vertices  []string
				Edges     [][]string
				Positions map[string][]float64
				Neighbors map[string][]string
				Endpoints []struct {
					Edge       []string
					Start, End []float64
					HasTip     bool `json:"has_tip"`
				}
				RootIdentity bool       `json:"root_identity_preserved"`
				NodeIdentity bool       `json:"surviving_node_identity"`
				EdgeIdentity bool       `json:"surviving_edge_identity"`
				LayoutKeys   []string   `json:"layout_keys"`
				ConfigKeys   []string   `json:"vertex_config_keys"`
				EdgeKeys     [][]string `json:"edge_config_keys"`
			}
		}
	}
	if err = json.Unmarshal(data, &ref); err != nil {
		t.Fatal(err)
	}
	if ref.Commit != "16ea443e2a5f1d79db0a721161411ce916d6e733" || len(ref.Cases) != 2 {
		t.Fatal("reference provenance")
	}
	vec := func(p []float64) geom.Vec3 { return geom.Vec3{X: p[0], Y: p[1], Z: p[2]} }
	checked := 0
	for _, c := range ref.Cases {
		g := mutationGraph(t, c.Kind == "DiGraph")
		if len(c.Steps) != 5 || !c.RemovedAbsent {
			t.Fatal("reference missing cleanup")
		}
		for _, step := range c.Steps {
			switch step.Operation {
			case "initial":
			case "add_vertices":
				err = g.AddVertices([]network.VertexSpec{{ID: "d", Position: geom.V(-2, 2)}, {ID: "e", Position: geom.V(0, 4)}})
			case "add_edges":
				err = g.AddEdges([]network.Edge{{"b", "c"}, {"c", "d"}, {"d", "a"}})
			case "remove_edges":
				err = g.RemoveEdges([]network.Edge{{"c", "d"}})
			case "remove_vertices":
				err = g.RemoveVertices([]string{"b", "e"})
			default:
				t.Fatal("unsupported reference step")
			}
			if err != nil {
				t.Fatal(err)
			}
			if !step.RootIdentity || !step.NodeIdentity || !step.EdgeIdentity {
				t.Fatal("source survivor identity not preserved")
			}
			if !reflect.DeepEqual(g.Vertices(), step.Vertices) {
				t.Fatal("source vertex order", c.Kind, step.Operation, g.Vertices(), step.Vertices)
			}
			edges := g.Edges()
			if len(edges) != len(step.Edges) || !reflect.DeepEqual(step.LayoutKeys, step.Vertices) || !reflect.DeepEqual(step.ConfigKeys, step.Vertices) || !reflect.DeepEqual(step.EdgeKeys, step.Edges) {
				t.Fatal("source metadata cleanup")
			}
			for i, e := range edges {
				if e != (network.Edge{From: step.Edges[i][0], To: step.Edges[i][1]}) {
					t.Fatal("source edge order")
				}
			}
			for _, id := range g.Vertices() {
				v, _ := g.Vertex(id)
				near(t, v.Center(), vec(step.Positions[id]))
				ns, e := g.Neighbors(id)
				if e != nil || len(ns) != len(step.Neighbors[id]) {
					t.Fatal("neighbors", id, ns, e)
				}
				for i, w := range ns {
					if w != step.Neighbors[id][i] {
						t.Fatal("neighbor order")
					}
				}
			}
			for _, endpoint := range step.Endpoints {
				e, _ := g.Edge(endpoint.Edge[0], endpoint.Edge[1])
				shaft := e
				if c.Kind == "DiGraph" {
					shaft = e.Children[0]
					near(t, e.Children[1].Path.Contours[0].Start, vec(endpoint.End))
					assertArrowJoin(t, e)
				} else {
					near(t, e.Path.Contours[0].Curves[0].P3, vec(endpoint.End))
				}
				near(t, shaft.Path.Contours[0].Start, vec(endpoint.Start))
				checked++
			}
			assertMutationOrder(t, g)
		}
	}
	if checked < 16 {
		t.Fatal("insufficient reference samples", checked)
	}
}
