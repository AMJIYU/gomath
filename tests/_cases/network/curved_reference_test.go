package network_test

import (
	"encoding/json"
	"gomath/geom"
	"gomath/network"
	"os"
	"testing"
)

func TestCurvedIndependentNumPySciPyGeometry(t *testing.T) {
	data, e := os.ReadFile("curved_math.json")
	if e != nil {
		t.Fatal(e)
	}
	var ref struct {
		TipLength    float64 `json:"tip_length"`
		TipWidth     float64 `json:"tip_width"`
		VertexRadius float64 `json:"vertex_radius"`
		Cases        []struct {
			Kind      string
			A, B      []float64
			Sweep     float64
			Side      network.LoopSide
			Radius    float64
			Clockwise bool
			Shaft     [][][]float64
			Head      [][]float64
		}
	}
	if e = json.Unmarshal(data, &ref); e != nil {
		t.Fatal(e)
	}
	vec := func(a []float64) geom.Vec3 { return geom.V(a[0], a[1]) }
	checks := 0
	for _, c := range ref.Cases {
		o := network.DefaultCurvedOptions()
		o.Options.TipSize = ref.TipLength
		o.TipWidth = ref.TipWidth
		o.Options.VertexRadius = ref.VertexRadius
		names := []string{"a", "b"}
		route := network.CurvedEdge{From: "a", To: "b", Sweep: c.Sweep}
		if c.Kind == "loop" {
			names = []string{"a"}
			route.To = "a"
			route.Loop = &network.LoopOptions{Side: c.Side, Radius: c.Radius, Clockwise: c.Clockwise}
		}
		g, e := network.NewCurvedDiGraph(names, []network.CurvedEdge{route}, o)
		if e != nil {
			t.Fatal(e)
		}
		positions := map[string]geom.Vec3{"a": vec(c.A)}
		if c.Kind != "loop" {
			positions["b"] = vec(c.B)
		}
		if e = g.SetPositions(positions); e != nil {
			t.Fatal(e)
		}
		ar, _ := g.Edge(route.From, route.To)
		curves := ar.Children[0].Path.Contours[0].Curves
		if len(curves) != len(c.Shaft) {
			t.Fatal("reference topology differs", c.Kind, c.Sweep, len(curves), len(c.Shaft))
		}
		for i, v := range curves {
			for j, w := range c.Shaft[i] {
				p := v.At(float64(j) / 4)
				if p.Sub(vec(w)).Norm() > 2e-8 {
					t.Fatalf("independent shaft %s sweep %g curve %d sample %d: %v want %v", c.Kind, c.Sweep, i, j, p, w)
				}
				checks++
			}
		}
		head := ar.Children[1].Path.Contours[0]
		for i, w := range c.Head {
			if head.Curves[i].P0.Sub(vec(w)).Norm() > 2e-8 {
				t.Fatal("independent head mismatch", c.Kind, c.Sweep, i, head.Curves[i].P0, w)
			}
			checks++
		}
	}
	if len(ref.Cases) != 14 || checks < 450 {
		t.Fatal("incomplete reference", len(ref.Cases), checks)
	}
}
