package scenes

import (
	"gomath"
	"gomath/geom"
	"gomath/network"
	"gomath/text"
)

func NetworkMutations() (*gomath.Scene, error) {
	options := network.DefaultOptions()
	options.VertexRadius = .12
	graph, err := network.NewDiGraph(nil, nil, options)
	if err != nil {
		return nil, err
	}
	if err = graph.AddVertices([]network.VertexSpec{{ID: "a", Position: geom.V(-1.2, -.1)}, {ID: "b", Position: geom.V(1.2, -.1)}, {ID: "c", Position: geom.V(0, 1.3)}}); err != nil {
		return nil, err
	}
	if err = graph.AddEdges([]network.Edge{{From: "a", To: "b"}, {From: "b", To: "c"}, {From: "c", To: "a"}}); err != nil {
		return nil, err
	}
	capture := func() (*gomath.Object, error) {
		group := gomath.Group(graph.Object.Copy())
		for _, name := range graph.Vertices() {
			vertex, _ := graph.Vertex(name)
			opt := text.DefaultOptions()
			opt.Size = .28
			label, err := text.New(name, text.DefaultFont(), opt)
			if err != nil {
				return nil, err
			}
			offset := map[string]geom.Vec3{"a": geom.V(-.25, -.25), "b": geom.V(.25, -.25), "c": geom.V(0, .3), "d": geom.V(.25, -.2)}[name]
			label.MoveTo(vertex.Center().Add(offset))
			group.Children = append(group.Children, label)
		}
		return group, nil
	}
	first, err := capture()
	if err != nil {
		return nil, err
	}
	if err = graph.AddVertices([]network.VertexSpec{{ID: "d", Position: geom.V(0, -1.3)}}); err != nil {
		return nil, err
	}
	if err = graph.AddEdges([]network.Edge{{From: "b", To: "d"}, {From: "c", To: "d"}}); err != nil {
		return nil, err
	}
	second, err := capture()
	if err != nil {
		return nil, err
	}
	if err = graph.RemoveVertices([]string{"b"}); err != nil {
		return nil, err
	}
	third, err := capture()
	if err != nil {
		return nil, err
	}
	heading, err := title("Graph authoring / atomic batches / independent captured states")
	if err != nil {
		return nil, err
	}
	heading.Scale(.87)
	scene := gomath.NewScene()
	if err = scene.Add(heading); err != nil {
		return nil, err
	}
	states := []*gomath.Object{first, second, third}
	for i, state := range states {
		state.Shift(geom.V(-4.3+4.3*float64(i), 0))
		opt := text.DefaultOptions()
		opt.Size = .28
		caption, err := text.New([]string{"3 vertices / 3 directed edges", "add d + two edges", "remove b + incident edges"}[i], text.DefaultFont(), opt)
		if err != nil {
			return nil, err
		}
		caption.MoveTo(geom.V(-4.3+4.3*float64(i), -2.2))
		if err = scene.Add(caption); err != nil {
			return nil, err
		}
		if err = scene.Play(.8, gomath.FadeIn(state)); err != nil {
			return nil, err
		}
	}
	if err = scene.Wait(1.1); err != nil {
		return nil, err
	}
	return scene, nil
}
