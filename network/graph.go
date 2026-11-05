package network

import (
	"fmt"
	"gomath"
	"gomath/geom"
	"gomath/path"
	"math"
	"strings"
)

type Edge struct{ From, To string }

type Options struct {
	Layout                  LayoutOptions
	VertexRadius, EdgeWidth float64
	TipSize                 float64
	VertexColor, EdgeColor  gomath.Color
}

// DefaultOptions returns a fully specified configuration. Constructors validate
// all options; use this helper before overriding individual values.
func DefaultOptions() Options {
	return Options{Layout: DefaultLayout(), VertexRadius: .12, EdgeWidth: .035, TipSize: .16, VertexColor: gomath.Blue, EdgeColor: gomath.White}
}

// Graph owns exactly one object tree. Vertex and Edge return borrowed nodes of
// Object; do not add them as additional scene roots. Object.Copy() produces an
// independent renderable tree. Mutations require exclusive authoring access.
// Self-loops and parallel edges are unsupported and rejected. Input order is
// preserved and controls layout and rendering order.
type Graph struct {
	Object   *gomath.Object
	vertices []string
	edges    []Edge
	index    map[string]int
	lookup   map[Edge]int
	nodes    []*gomath.Object
	links    []*gomath.Object
	directed bool
	tipSize  float64
	author   *graphAuthor
}

// DiGraph has ordered edges, with a filled arrowhead at each destination.
// Reciprocal edges are supported but share a straight segment.
type DiGraph struct{ *Graph }

func NewGraph(vertices []string, edges []Edge, options Options) (*Graph, error) {
	return newGraph(vertices, edges, options, false)
}

func NewDiGraph(vertices []string, edges []Edge, options Options) (*DiGraph, error) {
	g, err := newGraph(vertices, edges, options, true)
	if err != nil {
		return nil, err
	}
	return &DiGraph{g}, nil
}

func edgeKey(e Edge, directed bool) Edge {
	if !directed && e.From > e.To {
		e.From, e.To = e.To, e.From
	}
	return e
}

func newGraph(vertices []string, edges []Edge, o Options, directed bool) (*Graph, error) {
	return newGraphLimited(vertices, edges, o, directed, DefaultGraphLimits())
}

func newGraphLimited(vertices []string, edges []Edge, o Options, directed bool, limits GraphLimits) (*Graph, error) {
	if err := graphConstructionCheck(vertices, edges, o, limits, directed); err != nil {
		return nil, err
	}
	if len(vertices) > 4096 || len(edges) > 65536 {
		return nil, fmt.Errorf("graph supports at most 4096 vertices and 65536 edges")
	}
	if !finite(o.VertexRadius) || o.VertexRadius <= 0 || !finite(o.EdgeWidth) || o.EdgeWidth <= 0 || !finite(o.TipSize) || o.TipSize <= 0 {
		return nil, fmt.Errorf("vertex radius, edge width and tip size must be finite and positive")
	}
	style := gomath.DefaultStyle()
	style.Fill, style.Stroke = o.VertexColor, o.EdgeColor
	if err := style.Validate(); err != nil {
		return nil, fmt.Errorf("graph colors: %w", err)
	}
	g := &Graph{vertices: append([]string(nil), vertices...), edges: append([]Edge(nil), edges...), index: make(map[string]int), lookup: make(map[Edge]int), directed: directed, tipSize: o.TipSize}
	for i, id := range vertices {
		id = strings.Clone(id)
		g.vertices[i] = id
		if _, exists := g.index[id]; exists || id == "" {
			return nil, fmt.Errorf("empty or duplicate vertex %q", id)
		}
		g.index[id] = i
	}
	indices := make([][2]int, len(edges))
	for i, e := range edges {
		e = Edge{strings.Clone(e.From), strings.Clone(e.To)}
		g.edges[i] = e
		from, okFrom := g.index[e.From]
		to, okTo := g.index[e.To]
		key := edgeKey(e, directed)
		_, duplicate := g.lookup[key]
		if !okFrom || !okTo || from == to || duplicate {
			return nil, fmt.Errorf("edge %d (%q,%q): unknown vertex, self-loop or duplicate edge", i, e.From, e.To)
		}
		g.lookup[key], indices[i] = i, [2]int{from, to}
	}
	positions, err := layoutPositions(len(vertices), indices, o.Layout)
	if err != nil {
		return nil, err
	}
	for i, p := range positions {
		node := gomath.Dot(p, o.VertexRadius).SetFill(o.VertexColor)
		node.Name = g.vertices[i]
		g.nodes = append(g.nodes, node)
	}
	for range edges {
		var link *gomath.Object
		if directed {
			link = gomath.Arrow(geom.V(0, 0), geom.V(1, 0), .1)
			link.Children[0].SetStroke(o.EdgeColor, o.EdgeWidth)
			link.Children[1].SetFill(o.EdgeColor)
		} else {
			link = gomath.Line(geom.V(0, 0), geom.V(1, 0)).SetStroke(o.EdgeColor, o.EdgeWidth)
		}
		g.links = append(g.links, link)
	}
	children := append(append([]*gomath.Object(nil), g.links...), g.nodes...)
	g.Object = gomath.Group(children...)
	g.author = captureGraphAuthor(g, o, limits)
	if err := g.UpdateEdges(); err != nil {
		return nil, err
	}
	return g, nil
}

func (g *Graph) Directed() bool     { return g.directed }
func (g *Graph) Vertices() []string { return append([]string(nil), g.vertices...) }
func (g *Graph) Edges() []Edge      { return append([]Edge(nil), g.edges...) }
func (g *Graph) Vertex(id string) (*gomath.Object, bool) {
	i, ok := g.index[id]
	if !ok {
		return nil, false
	}
	return g.nodes[i], true
}
func (g *Graph) Edge(from, to string) (*gomath.Object, bool) {
	i, ok := g.lookup[edgeKey(Edge{from, to}, g.directed)]
	if !ok {
		return nil, false
	}
	return g.links[i], true
}

// Neighbors returns outgoing neighbors for a DiGraph, in input edge order.
func (g *Graph) Neighbors(id string) ([]string, error) {
	if _, ok := g.index[id]; !ok {
		return nil, fmt.Errorf("unknown vertex %q", id)
	}
	var out []string
	for _, e := range g.edges {
		if e.From == id {
			out = append(out, e.To)
		} else if !g.directed && e.To == id {
			out = append(out, e.From)
		}
	}
	return out, nil
}

// Relayout changes vertex positions and edge geometry atomically. It preserves
// all object IDs, child references and styles. It does not animate the change.
func (g *Graph) Relayout(options LayoutOptions) error {
	stats, err := g.authorPreflight()
	if err != nil {
		return err
	}
	if err := graphLayoutBudget(g.author.limits, stats, len(g.nodes), len(g.links), options); err != nil {
		return err
	}
	indices := make([][2]int, len(g.edges))
	for i, e := range g.edges {
		indices[i] = [2]int{g.index[e.From], g.index[e.To]}
	}
	p, err := layoutPositions(len(g.vertices), indices, options)
	if err != nil {
		return err
	}
	positions := make(map[string]geom.Vec3, len(p))
	for i, id := range g.vertices {
		positions[id] = p[i]
	}
	return g.SetPositions(positions)
}

// SetPositions requires exactly one finite XY position per vertex. Edges are
// clipped to the vertices' axis-aligned bounding boxes. Overlapping endpoints
// with no exposed edge segment return an error without changing any paths.
func (g *Graph) SetPositions(positions map[string]geom.Vec3) error {
	if err := g.validateTree(); err != nil {
		return err
	}
	if len(positions) != len(g.nodes) {
		return fmt.Errorf("positions must contain exactly all vertices")
	}

	paths := make([]path.Path, len(g.nodes))
	bounds := make([]geom.Bounds, len(g.nodes))
	for i, id := range g.vertices {
		p, ok := positions[id]
		if !ok || !graphPoint(p) {
			return fmt.Errorf("missing or invalid planar position for %q", id)
		}
		if len(g.nodes[i].Children) != 0 {
			return fmt.Errorf("vertex %q must remain a leaf", id)
		}
		delta := p.Sub(g.nodes[i].Center())
		if !graphPoint(delta) {
			return ErrGraphGeometry
		}
		paths[i] = g.nodes[i].Path.Transform(geom.Translation(delta))
		if err := graphValidatePath(paths[i]); err != nil {
			return err
		}
		bounds[i] = paths[i].Bounds()
	}
	geometry, err := g.edgeGeometry(bounds)
	if err != nil {
		return err
	}
	for i := range paths {
		g.nodes[i].Path = paths[i]
	}
	g.applyEdges(geometry)
	return nil
}

type edgePaths struct{ shaft, tip path.Path }

// UpdateEdges follows direct vertex MoveTo/Shift/Scale changes. Call it after
// moving vertices and before Scene.Add/Play captures authoring state. It never
// replaces an edge, arrow child, or vertex object. This is not a frame callback.
func (g *Graph) UpdateEdges() error {
	if err := g.validateTree(); err != nil {
		return err
	}
	bounds := make([]geom.Bounds, len(g.nodes))
	for i, node := range g.nodes {
		if len(node.Children) != 0 {
			return fmt.Errorf("vertex %q must remain a leaf", g.vertices[i])
		}
		bounds[i] = node.Bounds()
	}
	geometry, err := g.edgeGeometry(bounds)
	if err != nil {
		return err
	}
	g.applyEdges(geometry)
	return nil
}

func (g *Graph) validateTree() error {
	_, err := g.authorPreflight()
	return err
}

func (g *Graph) edgeGeometry(bounds []geom.Bounds) ([]edgePaths, error) {
	for i, b := range bounds {
		if !graphBounds(b) {
			return nil, fmt.Errorf("vertex %q has invalid planar bounds", g.vertices[i])
		}
	}
	out := make([]edgePaths, len(g.edges))
	for i, e := range g.edges {
		link := g.links[i]
		if g.directed && (len(link.Children) != 2 || len(link.Children[0].Children) != 0 || len(link.Children[1].Children) != 0) || !g.directed && len(link.Children) != 0 {
			return nil, fmt.Errorf("edge %d topology changed", i)
		}
		a, b := bounds[g.index[e.From]], bounds[g.index[e.To]]
		delta := b.Center().Sub(a.Center())
		d := math.Hypot(delta.X, delta.Y)
		if !finite(d) || d == 0 {
			return nil, fmt.Errorf("edge %d has coincident or invalid centers", i)
		}
		u := geom.V(delta.X/d, delta.Y/d)
		extent := func(b geom.Bounds) float64 {
			x, y := math.Inf(1), math.Inf(1)
			if u.X != 0 {
				x = (b.Size().X / 2) / math.Abs(u.X)
			}
			if u.Y != 0 {
				y = (b.Size().Y / 2) / math.Abs(u.Y)
			}
			return min(x, y)
		}
		ra, rb := extent(a), extent(b)
		if ra+rb >= d {
			return nil, fmt.Errorf("edge %d endpoints overlap", i)
		}
		start, end := a.Center().Add(u.Mul(ra)), b.Center().Sub(u.Mul(rb))
		if start == end {
			return nil, fmt.Errorf("edge %d loses endpoint precision", i)
		}
		base := end
		if g.directed {
			tip := min(g.tipSize, (d-ra-rb)*.4)
			base = end.Sub(u.Mul(tip))
			v := geom.V(-u.Y, u.X).Mul(tip * .4)
			left, right := base.Add(v), base.Sub(v)
			if base == end || base == start || left == right || left == end || right == end {
				return nil, fmt.Errorf("edge %d arrowhead loses precision", i)
			}
			var pb path.Builder
			pb.MoveTo(end)
			pb.LineTo(left)
			pb.LineTo(right)
			pb.Close()
			out[i].tip = pb.Path()
		}
		out[i].shaft = path.Path{Contours: []path.Contour{{Start: start, Curves: []path.Cubic{path.Line(start, base)}}}}
		if err := graphValidatePath(out[i].shaft); err != nil {
			return nil, err
		}
		if err := graphValidatePath(out[i].tip); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (g *Graph) applyEdges(paths []edgePaths) {
	for i, p := range paths {
		if g.directed {
			g.links[i].Children[0].Path, g.links[i].Children[1].Path = p.shaft, p.tip
		} else {
			g.links[i].Path = p.shaft
		}
	}
}
