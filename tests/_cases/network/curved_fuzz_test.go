package network_test

import (
	"gomath/geom"
	"gomath/network"
	"math"
	"reflect"
	"testing"
)

func FuzzCurvedAtomicPositions(f *testing.F) {
	f.Add(-1.0, 0.0, 1.0, 0.0, .7)
	f.Add(0.0, 0.0, 0.0, 0.0, -.7)
	f.Fuzz(func(t *testing.T, ax, ay, bx, by, sweep float64) {
		if !math.IsNaN(sweep) && !math.IsInf(sweep, 0) {
			sweep = math.Remainder(sweep, 2*math.Pi)
		}
		g, e := network.NewCurvedDiGraph([]string{"a", "b"}, []network.CurvedEdge{{From: "a", To: "b", Sweep: sweep}}, network.DefaultCurvedOptions())
		if e != nil {
			if g != nil {
				t.Fatal("partial constructor")
			}
			return
		}
		before := graphPaths(g)
		e = g.SetPositions(map[string]geom.Vec3{"a": geom.V(ax, ay), "b": geom.V(bx, by)})
		if e != nil {
			if !reflect.DeepEqual(before, graphPaths(g)) {
				t.Fatal("failed update changed paths")
			}
			return
		}
		if e = g.Object.Validate(); e != nil {
			t.Fatal(e)
		}
		assertEndpointGeometry(t, g)
	})
}
