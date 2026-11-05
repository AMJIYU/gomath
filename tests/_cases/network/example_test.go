package network_test

import (
	"fmt"
	"gomath"
	"gomath/geom"
	"gomath/network"
)

func ExampleNewDiGraph() {
	g, err := network.NewDiGraph([]string{"source", "sink"}, []network.Edge{{From: "source", To: "sink"}}, network.DefaultOptions())
	if err != nil {
		panic(err)
	}
	sink, _ := g.Vertex("sink")
	sink.MoveTo(geom.V(2, 0))
	if err := g.UpdateEdges(); err != nil {
		panic(err)
	}
	scene := gomath.NewScene()
	if err := scene.Add(g.Object); err != nil {
		panic(err)
	}
	fmt.Println(g.Directed(), len(g.Vertices()), len(g.Edges()))
	// Output: true 2 1
}
