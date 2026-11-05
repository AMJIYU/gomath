package network

import (
	"errors"
	"fmt"
	"gomath"
	"gomath/geom"
	"gomath/path"
	"math"
	"strings"
)

var (
	ErrCurvedOptions  = errors.New("network: invalid curved graph input")
	ErrCurvedGeometry = errors.New("network: unexposed or degenerate curved geometry")
	ErrCurvedLimit    = errors.New("network: curved graph budget exceeded")
	ErrCurvedTopology = errors.New("network: curved graph topology changed")
)

type LoopSide string

const (
	LoopTop    LoopSide = "top"
	LoopRight  LoopSide = "right"
	LoopBottom LoopSide = "bottom"
	LoopLeft   LoopSide = "left"
)

// LoopOptions places a circular major arc outside the selected bbox face.
// Radius is explicit; tips are never shrunk to accommodate a small/flat loop.
type LoopOptions struct {
	Side      LoopSide
	Radius    float64
	Clockwise bool
}

func DefaultLoopOptions() LoopOptions { return LoopOptions{Side: LoopTop, Radius: .35} }

// CurvedEdge is one unique ordered pair. Nonloops use a signed Sweep in
// [-pi,pi]; zero is straight. Self-loops require Loop and zero Sweep.
// Positive sweep is counterclockwise about +Z; reciprocal positive sweeps
// therefore bow to opposite sides. There is no implicit reciprocal routing.
type CurvedEdge struct {
	From, To string
	Sweep    float64
	Loop     *LoopOptions
}

type CurvedLimits struct {
	MaxVertices, MaxEdges, MaxCubics, MaxContours int
	MaxWork                                       uint64
	MaxStorageBytes                               uint64
}

func DefaultCurvedLimits() CurvedLimits {
	return CurvedLimits{MaxVertices: 4096, MaxEdges: 65536, MaxCubics: 1000000, MaxContours: 1000000, MaxWork: 10_000_000_000_000, MaxStorageBytes: 512 << 20}
}

type CurvedOptions struct {
	Options  Options
	TipWidth float64 // Full triangular base width; Options.TipSize is physical trim length.
	Limits   CurvedLimits
}

func DefaultCurvedOptions() CurvedOptions {
	return CurvedOptions{Options: DefaultOptions(), TipWidth: .08, Limits: DefaultCurvedLimits()}
}

// CurvedDiGraph owns one tree; accessors return its borrowed vertices/edges.
// Mutations require exclusive authoring access. Scene sampling uses snapshots.
// Unlike embedding Graph, this wrapper cannot accidentally call straight-edge
// update methods. Routes/options are private immutable construction snapshots.
type CurvedDiGraph struct {
	Object                                *gomath.Object
	graph                                 *Graph
	routes                                []CurvedEdge
	options                               CurvedOptions
	shafts, heads                         []*gomath.Object
	root                                  *gomath.Object
	rootID                                uint64
	vertexIDs, edgeIDs, shaftIDs, headIDs []uint64
}

func (g *CurvedDiGraph) Directed() bool { return true }
func (g *CurvedDiGraph) Vertices() []string {
	if g == nil || g.graph == nil {
		return nil
	}
	return g.graph.Vertices()
}
func (g *CurvedDiGraph) Edges() []Edge {
	if g == nil || g.graph == nil {
		return nil
	}
	return g.graph.Edges()
}
func (g *CurvedDiGraph) Vertex(id string) (*gomath.Object, bool) {
	if g == nil || g.graph == nil {
		return nil, false
	}
	return g.graph.Vertex(id)
}
func (g *CurvedDiGraph) Edge(from, to string) (*gomath.Object, bool) {
	if g == nil || g.graph == nil {
		return nil, false
	}
	return g.graph.Edge(from, to)
}
func (g *CurvedDiGraph) Neighbors(id string) ([]string, error) {
	if g == nil || g.graph == nil {
		return nil, ErrCurvedOptions
	}
	return g.graph.Neighbors(id)
}
func (g *CurvedDiGraph) CurvedEdges() []CurvedEdge {
	if g == nil {
		return nil
	}
	return copyCurvedEdges(g.routes)
}
func copyCurvedEdges(in []CurvedEdge) []CurvedEdge {
	out := append([]CurvedEdge(nil), in...)
	for i := range out {
		out[i].From = strings.Clone(out[i].From)
		out[i].To = strings.Clone(out[i].To)
		if out[i].Loop != nil {
			l := *out[i].Loop
			out[i].Loop = &l
		}
	}
	return out
}

func curvedDimension(x float64) bool {
	return finite(x) && x >= 1e-9 && x <= gomath.MaxPrimitiveCoordinate
}
func curvedPoint(p geom.Vec3) bool {
	return planar(p) && math.Abs(p.X) <= gomath.MaxPrimitiveCoordinate && math.Abs(p.Y) <= gomath.MaxPrimitiveCoordinate
}

// Each edge reserves 100 million abstract work units, including the bounded
// parent arc/length/inverse kernels, cubic-face root isolation and certification.
// This is a conservative operation reserve, not a CPU-time/heap/RSS guarantee.
const curvedEdgeWork uint64 = 100_000_000
const curvedScratch uint64 = 8 << 20

func curvedCheck(o CurvedOptions, n, m, cubics, contours int, names uint64, layout *LayoutOptions) error {
	l := o.Limits
	if l.MaxVertices < 0 || l.MaxVertices > 4096 || l.MaxEdges < 0 || l.MaxEdges > 65536 || l.MaxCubics < 0 || l.MaxCubics > 1000000 || l.MaxContours < 0 || l.MaxContours > 1000000 || l.MaxWork > 10_000_000_000_000 || l.MaxStorageBytes < curvedScratch || l.MaxStorageBytes > 1<<30 {
		return ErrCurvedOptions
	}
	if n > l.MaxVertices || m > l.MaxEdges || cubics > l.MaxCubics || contours > l.MaxContours {
		return ErrCurvedLimit
	}
	bytes := curvedScratch + uint64(n)*2048 + uint64(m)*4096 + uint64(cubics+contours)*1024 + names
	work := 4*uint64(n+cubics+contours) + uint64(m)*curvedEdgeWork
	if layout != nil {
		if err := layout.validate(n, m); err != nil {
			return fmt.Errorf("%w: %v", ErrCurvedOptions, err)
		}
		if layout.Kind == Spring {
			work += (uint64(n)*uint64(n) + uint64(m)) * uint64(layout.Iterations)
		} else {
			work += uint64(n)
		}
	}
	if bytes > l.MaxStorageBytes || work > l.MaxWork {
		return ErrCurvedLimit
	}
	return nil
}

func curvedAddName(total *uint64, name string, limit uint64) bool {
	if *total > limit || uint64(len(name)) > limit-*total {
		return false
	}
	*total += uint64(len(name))
	return true
}

func NewCurvedDiGraph(vertices []string, edges []CurvedEdge, o CurvedOptions) (*CurvedDiGraph, error) {
	if !curvedDimension(o.Options.VertexRadius) || !curvedDimension(o.Options.EdgeWidth) || !curvedDimension(o.Options.TipSize) || !curvedDimension(o.TipWidth) {
		return nil, ErrCurvedOptions
	}
	style := gomath.DefaultStyle()
	style.Fill = o.Options.VertexColor
	style.Stroke = o.Options.EdgeColor
	if err := style.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCurvedOptions, err)
	}
	// 5 cubics per dot (including its explicit closing segment), <=8 shaft+3
	// head cubics per arrow. This reserve is invariant across future updates.
	if len(vertices) > 4096 || len(edges) > 65536 {
		return nil, ErrCurvedLimit
	}
	if err := curvedCheck(o, len(vertices), len(edges), 5*len(vertices)+11*len(edges), len(vertices)+2*len(edges), 0, &o.Options.Layout); err != nil {
		return nil, err
	}
	names := uint64(0)
	for _, v := range vertices {
		if !curvedAddName(&names, v, o.Limits.MaxStorageBytes) || !curvedAddName(&names, v, o.Limits.MaxStorageBytes) {
			return nil, ErrCurvedLimit
		}
	}
	for _, e := range edges {
		if !curvedAddName(&names, e.From, o.Limits.MaxStorageBytes) || !curvedAddName(&names, e.To, o.Limits.MaxStorageBytes) {
			return nil, ErrCurvedLimit
		}
	}
	if err := curvedCheck(o, len(vertices), len(edges), 5*len(vertices)+11*len(edges), len(vertices)+2*len(edges), names, &o.Options.Layout); err != nil {
		return nil, err
	}
	base := &Graph{index: make(map[string]int), lookup: make(map[Edge]int), directed: true, tipSize: o.Options.TipSize}
	for i, v := range vertices {
		if v == "" {
			return nil, ErrCurvedOptions
		}
		if _, ok := base.index[v]; ok {
			return nil, ErrCurvedOptions
		}
		id := strings.Clone(v)
		base.vertices = append(base.vertices, id)
		base.index[id] = i
	}
	routes := copyCurvedEdges(edges)
	indices := make([][2]int, 0, len(edges))
	for i, e := range routes {
		a, okA := base.index[e.From]
		b, okB := base.index[e.To]
		key := Edge{e.From, e.To}
		if _, dup := base.lookup[key]; dup || !okA || !okB {
			return nil, fmt.Errorf("%w: edge %d unknown or duplicate", ErrCurvedOptions, i)
		}
		if !finite(e.Sweep) || math.Abs(e.Sweep) > math.Pi {
			return nil, ErrCurvedOptions
		}
		if a == b {
			if e.Loop == nil || e.Sweep != 0 || !curvedDimension(e.Loop.Radius) {
				return nil, ErrCurvedOptions
			}
			switch e.Loop.Side {
			case LoopTop, LoopRight, LoopBottom, LoopLeft:
			default:
				return nil, ErrCurvedOptions
			}
		} else {
			if e.Loop != nil {
				return nil, ErrCurvedOptions
			}
			indices = append(indices, [2]int{a, b})
		}
		base.lookup[key] = i
		base.edges = append(base.edges, key)
	}
	// Self-loops exert no layout force. Cycles/reciprocal edges are allowed and
	// deterministic; reciprocal entries are separately enumerated spring forces.
	p, err := layoutPositions(len(vertices), indices, o.Options.Layout)
	if err != nil {
		return nil, err
	}
	g := &CurvedDiGraph{graph: base, routes: routes, options: o}
	for i, p := range p {
		if !curvedPoint(p) {
			return nil, ErrCurvedGeometry
		}
		node := gomath.Dot(p, o.Options.VertexRadius).SetFill(o.Options.VertexColor)
		node.Name = base.vertices[i]
		base.nodes = append(base.nodes, node)
		g.vertexIDs = append(g.vertexIDs, node.ID)
	}
	for range edges {
		shaft := gomath.NewObject(path.Path{}).SetStroke(o.Options.EdgeColor, o.Options.EdgeWidth)
		head := gomath.NewObject(path.Path{}).SetFill(o.Options.EdgeColor).SetStroke(gomath.Transparent, 0)
		link := gomath.Group(shaft, head)
		base.links = append(base.links, link)
		g.shafts = append(g.shafts, shaft)
		g.heads = append(g.heads, head)
		g.edgeIDs = append(g.edgeIDs, link.ID)
		g.shaftIDs = append(g.shaftIDs, shaft.ID)
		g.headIDs = append(g.headIDs, head.ID)
	}
	base.Object = gomath.Group(append(append([]*gomath.Object(nil), base.links...), base.nodes...)...)
	g.Object = base.Object
	g.root = base.Object
	g.rootID = g.root.ID
	if err := g.UpdateEdges(); err != nil {
		return nil, err
	}
	return g, nil
}

func (g *CurvedDiGraph) preflight(layout *LayoutOptions) error {
	if g == nil || g.graph == nil || g.Object == nil {
		return ErrCurvedOptions
	}
	b := g.graph
	if g.Object != g.root || g.Object.ID != g.rootID || len(g.Object.Path.Contours) != 0 || g.Object.Bitmap != nil || g.Object.Clip != nil || len(g.Object.Children) != len(b.links)+len(b.nodes) {
		return ErrCurvedTopology
	}
	count, contours, names := 0, 0, uint64(0)
	if !curvedAddName(&names, g.Object.Name, g.options.Limits.MaxStorageBytes) {
		return ErrCurvedLimit
	}
	for _, v := range b.vertices {
		if !curvedAddName(&names, v, g.options.Limits.MaxStorageBytes) {
			return ErrCurvedLimit
		}
	}
	for _, r := range g.routes {
		if !curvedAddName(&names, r.From, g.options.Limits.MaxStorageBytes) || !curvedAddName(&names, r.To, g.options.Limits.MaxStorageBytes) {
			return ErrCurvedLimit
		}
	}
	check := func(o *gomath.Object, id uint64) error {
		if o == nil || o.ID != id || len(o.Children) != 0 || o.Bitmap != nil || o.Clip != nil {
			return ErrCurvedTopology
		}
		if !curvedAddName(&names, o.Name, g.options.Limits.MaxStorageBytes) {
			return ErrCurvedLimit
		}
		if len(o.Path.Contours) > g.options.Limits.MaxContours-contours {
			return ErrCurvedLimit
		}
		contours += len(o.Path.Contours)
		for _, c := range o.Path.Contours {
			if len(c.Curves) > g.options.Limits.MaxCubics-count {
				return ErrCurvedLimit
			}
			count += len(c.Curves)
		}
		return nil
	}
	for i, v := range b.nodes {
		if g.Object.Children[len(b.links)+i] != v {
			return ErrCurvedTopology
		}
		if err := check(v, g.vertexIDs[i]); err != nil {
			return err
		}
	}
	vertexCurves, vertexContours := count, contours
	for i, e := range b.links {
		if g.Object.Children[i] != e || e.ID != g.edgeIDs[i] || len(e.Path.Contours) != 0 || e.Bitmap != nil || e.Clip != nil || len(e.Children) != 2 || e.Children[0] != g.shafts[i] || e.Children[1] != g.heads[i] {
			return ErrCurvedTopology
		}
		if !curvedAddName(&names, e.Name, g.options.Limits.MaxStorageBytes) {
			return ErrCurvedLimit
		}
		if err := check(g.shafts[i], g.shaftIDs[i]); err != nil {
			return err
		}
		if err := check(g.heads[i], g.headIDs[i]); err != nil {
			return err
		}
	}
	reserve := max(count, max(vertexCurves, 5*len(b.nodes))+11*len(b.links))
	if err := curvedCheck(g.options, len(b.nodes), len(b.links), reserve, max(contours, max(vertexContours, len(b.nodes))+2*len(b.links)), names, layout); err != nil {
		return err
	}
	if err := g.Object.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrCurvedGeometry, err)
	}
	for _, v := range b.nodes {
		if err := curvedValidatePath(v.Path); err != nil {
			return err
		}
	}
	for i := range b.links {
		if err := curvedValidatePath(g.shafts[i].Path); err != nil {
			return err
		}
		if err := curvedValidatePath(g.heads[i].Path); err != nil {
			return err
		}
	}
	return nil
}

func (g *CurvedDiGraph) SetPositions(positions map[string]geom.Vec3) error {
	if err := g.preflight(nil); err != nil {
		return err
	}
	b := g.graph
	if len(positions) != len(b.nodes) {
		return ErrCurvedOptions
	}
	paths := make([]path.Path, len(b.nodes))
	bounds := make([]geom.Bounds, len(b.nodes))
	for i, id := range b.vertices {
		p, ok := positions[id]
		if !ok || !curvedPoint(p) {
			return ErrCurvedOptions
		}
		delta := p.Sub(b.nodes[i].Center())
		if !curvedPoint(delta) {
			return ErrCurvedGeometry
		}
		paths[i] = b.nodes[i].Path.Transform(geom.Translation(delta))
		if err := curvedValidatePath(paths[i]); err != nil {
			return err
		}
		bounds[i] = paths[i].Bounds()
	}
	geometry, err := g.geometry(bounds)
	if err != nil {
		return err
	}
	for i, p := range paths {
		b.nodes[i].Path = p
	}
	g.apply(geometry)
	return nil
}
func (g *CurvedDiGraph) UpdateEdges() error {
	if err := g.preflight(nil); err != nil {
		return err
	}
	bounds := make([]geom.Bounds, len(g.graph.nodes))
	for i, v := range g.graph.nodes {
		bounds[i] = v.Bounds()
	}
	geometry, err := g.geometry(bounds)
	if err != nil {
		return err
	}
	g.apply(geometry)
	return nil
}
func (g *CurvedDiGraph) Relayout(o LayoutOptions) error {
	if err := g.preflight(&o); err != nil {
		return err
	}
	b := g.graph
	indices := make([][2]int, 0, len(b.edges))
	for _, e := range b.edges {
		if e.From != e.To {
			indices = append(indices, [2]int{b.index[e.From], b.index[e.To]})
		}
	}
	p, err := layoutPositions(len(b.nodes), indices, o)
	if err != nil {
		return err
	}
	positions := make(map[string]geom.Vec3, len(p))
	for i, v := range b.vertices {
		positions[v] = p[i]
	}
	return g.SetPositions(positions)
}
func (g *CurvedDiGraph) apply(p []edgePaths) {
	for i, e := range p {
		g.shafts[i].Path = e.shaft
		g.heads[i].Path = e.tip
	}
}

func curvedValidatePath(p path.Path) error {
	if err := p.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrCurvedGeometry, err)
	}
	for _, c := range p.Contours {
		if !curvedPoint(c.Start) {
			return ErrCurvedGeometry
		}
		for _, v := range c.Curves {
			for _, a := range []geom.Vec3{v.P0, v.P1, v.P2, v.P3} {
				if !curvedPoint(a) {
					return ErrCurvedGeometry
				}
			}
		}
	}
	return nil
}
