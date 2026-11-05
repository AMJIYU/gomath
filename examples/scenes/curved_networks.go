package scenes

import (
	"gomath"
	"gomath/geom"
	"gomath/network"
	"gomath/text"
	"math"
)

// CurvedNetworks authors routing once, then animates the captured whole tree.
// Individual node animations do not install a mutable per-frame edge updater.
func CurvedNetworks() (*gomath.Scene, error) {
	options := network.DefaultCurvedOptions()
	options.Options.Layout.Scale = 2.5
	options.Options.VertexRadius = .2
	options.Options.TipSize = .16
	options.TipWidth = .08
	loop := network.LoopOptions{Side: network.LoopRight, Radius: .45}
	graph, err := network.NewCurvedDiGraph([]string{"a", "b", "c"}, []network.CurvedEdge{
		{From: "a", To: "b", Sweep: math.Pi / 3},
		{From: "b", To: "a", Sweep: math.Pi / 3},
		{From: "b", To: "c", Sweep: -.6},
		{From: "c", To: "a", Sweep: .6},
		{From: "b", To: "b", Loop: &loop},
	}, options)
	if err != nil {
		return nil, err
	}
	positions := map[string]geom.Vec3{"a": geom.V(-2.8, -.5), "b": geom.V(2.8, -.5), "c": geom.V(0, 1.4)}
	if err = graph.SetPositions(positions); err != nil {
		return nil, err
	}
	for i, edge := range graph.Edges() {
		object, _ := graph.Edge(edge.From, edge.To)
		object.SetColor([]gomath.Color{gomath.Blue, gomath.Pink, gomath.White, gomath.White, gomath.Yellow}[i])
	}
	content := gomath.Group(graph.Object)
	for _, name := range graph.Vertices() {
		labelOptions := text.DefaultOptions()
		labelOptions.Size = .32
		label, err := text.New(name, text.DefaultFont(), labelOptions)
		if err != nil {
			return nil, err
		}
		label.MoveTo(positions[name].Add(geom.V(0, .45)))
		content.Children = append(content.Children, label)
	}
	heading, err := title("Directed networks / reciprocal curves and explicit self-loops")
	if err != nil {
		return nil, err
	}
	heading.Scale(.9)
	captionOptions := text.DefaultOptions()
	captionOptions.Size = .3
	caption, err := text.New("Authored bbox endpoints / stable shaft and head identities", text.DefaultFont(), captionOptions)
	if err != nil {
		return nil, err
	}
	caption.MoveTo(geom.V(0, -2.4))
	s := gomath.NewScene()
	if err = s.Add(heading, caption); err != nil {
		return nil, err
	}
	if err = s.Play(1, gomath.FadeIn(content)); err != nil {
		return nil, err
	}
	if err = s.Play(2, gomath.Transform(content, content.Copy().Scale(1.08).Rotate(.08))); err != nil {
		return nil, err
	}
	if err = s.Wait(.5); err != nil {
		return nil, err
	}
	return s, nil
}
