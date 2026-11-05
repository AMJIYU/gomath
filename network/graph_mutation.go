package network

import (
	"errors"
	"fmt"
	"gomath"
	"gomath/geom"
	"gomath/path"
	"math"
	"strings"
	"unicode/utf8"
)

var (
	ErrGraphInput    = errors.New("network: invalid graph mutation input")
	ErrGraphLimit    = errors.New("network: graph resource limit exceeded")
	ErrGraphTopology = errors.New("network: graph tree identity or topology changed")
	ErrGraphGeometry = errors.New("network: graph geometry outside supported range or precision")
)

// GraphLimits applies to construction and each authoring transaction. Storage
// and work are conservative modeled reserves, not RSS or elapsed-time caps.
type GraphLimits struct {
	MaxVertices, MaxEdges, MaxCubics, MaxContours int
	MaxNameBytes, MaxStorageBytes, MaxWork        uint64
}

func DefaultGraphLimits() GraphLimits {
	return GraphLimits{4096, 65536, 1000000, 1000000, 16 << 20, 512 << 20, 1000000000}
}

// VertexSpec adds a default-radius dot at an explicit planar position. A nil
// Style uses the original constructor's vertex style; a non-nil complete Style
// is copied by value. Gradient and dash values are already immutable.
type VertexSpec struct {
	ID       string
	Position geom.Vec3
	Style    *gomath.Style
}

func NewGraphWithLimits(vertices []string, edges []Edge, o Options, limits GraphLimits) (*Graph, error) {
	return newGraphLimited(vertices, edges, o, false, limits)
}
func NewDiGraphWithLimits(vertices []string, edges []Edge, o Options, limits GraphLimits) (*DiGraph, error) {
	g, err := newGraphLimited(vertices, edges, o, true, limits)
	if err != nil {
		return nil, err
	}
	return &DiGraph{g}, nil
}

type graphLinkIdentity struct {
	id              uint64
	shaft, head     *gomath.Object
	shaftID, headID uint64
}
type graphAuthor struct {
	root      *gomath.Object
	rootID    uint64
	vertexIDs []uint64
	links     []graphLinkIdentity
	options   Options
	limits    GraphLimits
}
type graphStats struct {
	curves, contours int
	names, paints    uint64
}

func captureGraphAuthor(g *Graph, o Options, limits GraphLimits) *graphAuthor {
	a := &graphAuthor{root: g.Object, rootID: g.Object.ID, options: o, limits: limits, vertexIDs: make([]uint64, len(g.nodes)), links: make([]graphLinkIdentity, len(g.links))}
	// Layout is not retained: only original dot/edge authoring defaults matter.
	a.options.Layout = LayoutOptions{}
	for i, v := range g.nodes {
		a.vertexIDs[i] = v.ID
	}
	for i, e := range g.links {
		r := graphLinkIdentity{id: e.ID}
		if g.directed {
			r.shaft, r.head = e.Children[0], e.Children[1]
			r.shaftID, r.headID = r.shaft.ID, r.head.ID
		}
		a.links[i] = r
	}
	return a
}

func (l GraphLimits) validate() error {
	if l.MaxVertices < 0 || l.MaxVertices > 4096 || l.MaxEdges < 0 || l.MaxEdges > 65536 || l.MaxCubics < 0 || l.MaxCubics > 1000000 || l.MaxContours < 0 || l.MaxContours > 1000000 || l.MaxNameBytes > 64<<20 || l.MaxStorageBytes < 65536 || l.MaxStorageBytes > 1<<30 || l.MaxWork > 1000000000 {
		return ErrGraphInput
	}
	return nil
}

func graphName(s string) bool { return utf8.ValidString(s) && !strings.ContainsRune(s, 0) }
func graphAddName(s *graphStats, name string, l GraphLimits) error {
	if s.names > l.MaxNameBytes || uint64(len(name)) > l.MaxNameBytes-s.names || s.names > l.MaxWork || uint64(len(name)) > l.MaxWork-s.names {
		return ErrGraphLimit
	}
	s.names += uint64(len(name))
	if !graphName(name) {
		return ErrGraphInput
	}
	return nil
}
func graphPoint(p geom.Vec3) bool {
	return planar(p) && math.Abs(p.X) <= gomath.MaxPrimitiveCoordinate && math.Abs(p.Y) <= gomath.MaxPrimitiveCoordinate
}
func graphBounds(b geom.Bounds) bool {
	if !b.Valid || !graphPoint(b.Min) || !graphPoint(b.Max) || b.Size().X <= 0 || b.Size().Y <= 0 {
		return false
	}
	scale := max(math.Abs(b.Min.X), math.Abs(b.Min.Y), math.Abs(b.Max.X), math.Abs(b.Max.Y))
	eps := 16 * (math.Nextafter(scale, math.Inf(1)) - scale)
	return b.Size().X > eps && b.Size().Y > eps
}
func graphValidatePath(p path.Path) error {
	for _, c := range p.Contours {
		if !graphPoint(c.Start) {
			return ErrGraphGeometry
		}
		for _, v := range c.Curves {
			if !graphPoint(v.P0) || !graphPoint(v.P1) || !graphPoint(v.P2) || !graphPoint(v.P3) {
				return ErrGraphGeometry
			}
		}
	}
	if err := p.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrGraphGeometry, err)
	}
	return nil
}

func graphBudget(l GraphLimits, s graphStats, n, m int, directed bool, extraWork uint64) error {
	if err := l.validate(); err != nil {
		return err
	}
	if n < 0 || m < 0 || n > l.MaxVertices || m > l.MaxEdges || s.curves > l.MaxCubics || s.contours > l.MaxContours || s.names > l.MaxNameBytes {
		return ErrGraphLimit
	}
	objects := uint64(1 + n + m)
	if directed {
		objects += 2 * uint64(m)
	}
	// Includes two metadata generations, map slots, identity stamps, candidate
	// children, path clones/outputs and fixed scratch. Immutable paints are
	// conservatively charged separately at their public maximum dimensions.
	bytes := uint64(65536) + 4096*uint64(n+m) + 2048*objects + 1024*uint64(s.curves+s.contours) + 4*s.names + s.paints
	work := 128*uint64(1+n+m) + 16*uint64(s.curves+s.contours) + s.names + extraWork
	if bytes > l.MaxStorageBytes || work > l.MaxWork {
		return ErrGraphLimit
	}
	return nil
}
func graphLayoutBudget(l GraphLimits, s graphStats, n, m int, o LayoutOptions) error {
	if o.Kind != Circular && o.Kind != Grid && o.Kind != Spring {
		return ErrGraphInput
	}
	if err := o.validate(n, m); err != nil {
		return fmt.Errorf("%w: %v", ErrGraphInput, err)
	}
	work := uint64(n)
	if o.Kind == Spring {
		work = (uint64(n)*uint64(n) + uint64(m)) * uint64(o.Iterations)
	}
	return graphBudget(l, s, n, m, false, work)
}

func graphConstructionCheck(vertices []string, edges []Edge, o Options, l GraphLimits, directed bool) error {
	if err := l.validate(); err != nil {
		return err
	}
	if len(vertices) > l.MaxVertices || len(edges) > l.MaxEdges {
		return ErrGraphLimit
	}
	for _, x := range []float64{o.VertexRadius, o.EdgeWidth, o.TipSize} {
		if !finite(x) || x <= 0 || x > gomath.MaxPrimitiveCoordinate {
			return ErrGraphInput
		}
	}
	if err := graphBudget(l, graphStats{}, len(vertices), len(edges), directed, 0); err != nil {
		return err
	}
	s := graphStats{curves: 5*len(vertices) + len(edges), contours: len(vertices) + len(edges)}
	if directed {
		s.curves += 3 * len(edges)
		s.contours += len(edges)
	}
	for _, v := range vertices {
		if v == "" {
			return ErrGraphInput
		}
		if err := graphAddName(&s, v, l); err != nil {
			return err
		}
		if err := graphAddName(&s, v, l); err != nil {
			return err
		}
	}
	for _, e := range edges {
		if err := graphAddName(&s, e.From, l); err != nil {
			return err
		}
		if err := graphAddName(&s, e.To, l); err != nil {
			return err
		}
	}
	if err := graphBudget(l, s, len(vertices), len(edges), directed, 0); err != nil {
		return err
	}
	return graphLayoutBudget(l, s, len(vertices), len(edges), o.Layout)
}

// authorPreflight counts the bounded shallow tree before calling generic
// validation, so arbitrary nested/cyclic trees cannot cause unbounded traversal.
func (g *Graph) authorPreflight() (graphStats, error) {
	s := graphStats{}
	if g == nil || g.author == nil || g.Object == nil {
		return s, ErrGraphTopology
	}
	a, l := g.author, g.author.limits
	n, m := len(g.nodes), len(g.links)
	if err := graphBudget(l, s, n, m, g.directed, 0); err != nil {
		return s, err
	}
	if g.Object != a.root || g.Object.ID != a.rootID || len(g.vertices) != n || len(g.edges) != m || len(g.index) != n || len(g.lookup) != m || len(a.vertexIDs) != n || len(a.links) != m || len(g.Object.Children) != n+m {
		return s, ErrGraphTopology
	}
	if len(g.Object.Path.Contours) != 0 || g.Object.Bitmap != nil || g.Object.Clip != nil {
		return s, ErrGraphTopology
	}
	if err := graphAddName(&s, g.Object.Name, l); err != nil {
		return s, err
	}
	for i, v := range g.vertices {
		if v == "" {
			return s, ErrGraphTopology
		}
		if j, ok := g.index[v]; !ok || j != i {
			return s, ErrGraphTopology
		}
		if err := graphAddName(&s, v, l); err != nil {
			return s, err
		}
	}
	for i, e := range g.edges {
		_, from := g.index[e.From]
		_, to := g.index[e.To]
		if j, ok := g.lookup[edgeKey(e, g.directed)]; !ok || j != i || !from || !to || e.From == e.To {
			return s, ErrGraphTopology
		}
		if err := graphAddName(&s, e.From, l); err != nil {
			return s, err
		}
		if err := graphAddName(&s, e.To, l); err != nil {
			return s, err
		}
	}
	count := func(o *gomath.Object, id uint64, leaf bool) error {
		if o == nil || o.ID != id || o.Bitmap != nil || o.Clip != nil || leaf && len(o.Children) != 0 {
			return ErrGraphTopology
		}
		if err := graphAddName(&s, o.Name, l); err != nil {
			return err
		}
		if len(o.Path.Contours) > l.MaxContours-s.contours {
			return ErrGraphLimit
		}
		s.contours += len(o.Path.Contours)
		for _, c := range o.Path.Contours {
			if len(c.Curves) > l.MaxCubics-s.curves {
				return ErrGraphLimit
			}
			s.curves += len(c.Curves)
		}
		if err := graphStyle(o.Style); err != nil {
			return err
		}
		if o.Style.FillGradient != nil {
			s.paints += 16384
		}
		if o.Style.Dash != nil {
			s.paints += 8192
		}
		return nil
	}
	if err := graphStyle(g.Object.Style); err != nil {
		return s, err
	}
	if g.Object.Style.FillGradient != nil {
		s.paints += 16384
	}
	if g.Object.Style.Dash != nil {
		s.paints += 8192
	}
	for i, v := range g.nodes {
		if g.Object.Children[m+i] != v {
			return s, ErrGraphTopology
		}
		if err := count(v, a.vertexIDs[i], true); err != nil {
			return s, err
		}
	}
	vertexCurves, vertexContours := s.curves, s.contours
	for i, e := range g.links {
		if g.Object.Children[i] != e {
			return s, ErrGraphTopology
		}
		if err := count(e, a.links[i].id, !g.directed); err != nil {
			return s, err
		}
		if g.directed {
			r := a.links[i]
			if len(e.Path.Contours) != 0 || len(e.Children) != 2 || e.Children[0] != r.shaft || e.Children[1] != r.head {
				return s, ErrGraphTopology
			}
			if err := count(r.shaft, r.shaftID, true); err != nil {
				return s, err
			}
			if err := count(r.head, r.headID, true); err != nil {
				return s, err
			}
		}
	}
	// Updates can grow caller-authored empty arrow paths back to standard paths.
	edgeCurves, edgeContours := m, m
	if g.directed {
		edgeCurves *= 4
		edgeContours *= 2
	}
	s.curves = max(s.curves, max(vertexCurves, 5*n)+edgeCurves)
	s.contours = max(s.contours, max(vertexContours, n)+edgeContours)
	if err := graphBudget(l, s, n, m, g.directed, 0); err != nil {
		return s, err
	}
	if err := g.Object.Validate(); err != nil {
		return s, fmt.Errorf("%w: %v", ErrGraphInput, err)
	}
	for _, o := range g.nodes {
		if err := graphValidatePath(o.Path); err != nil {
			return s, err
		}
		if !graphBounds(o.Bounds()) {
			return s, ErrGraphGeometry
		}
	}
	for _, o := range g.links {
		if err := graphValidatePath(o.Path); err != nil {
			return s, err
		}
		for _, c := range o.Children {
			if err := graphValidatePath(c.Path); err != nil {
				return s, err
			}
		}
	}
	return s, nil
}

// AddVertices appends dots in batch order. Existing geometry/styles are not
// rerouted. Every item is validated before any child or metadata is published.
func (g *Graph) AddVertices(vertices []VertexSpec) error {
	s, err := g.authorPreflight()
	if err != nil {
		return err
	}
	if len(vertices) == 0 {
		return nil
	}
	l := g.author.limits
	if len(vertices) > l.MaxVertices-len(g.nodes) {
		return ErrGraphLimit
	}
	if err := graphBudget(l, s, len(g.nodes)+len(vertices), len(g.links), g.directed, 128*uint64(len(vertices))); err != nil {
		return err
	}
	seen := make(map[string]bool, len(vertices))
	for _, v := range vertices {
		if v.ID == "" || !graphPoint(v.Position) {
			return ErrGraphInput
		}
		if err := graphAddName(&s, v.ID, l); err != nil {
			return err
		}
		if err := graphAddName(&s, v.ID, l); err != nil {
			return err
		}
		if _, ok := g.index[v.ID]; ok || seen[v.ID] {
			return ErrGraphInput
		}
		seen[v.ID] = true
		if v.Style != nil {
			if err := graphStyle(*v.Style); err != nil {
				return fmt.Errorf("%w: %v", ErrGraphInput, err)
			}
			if v.Style.FillGradient != nil {
				s.paints += 16384
			}
			if v.Style.Dash != nil {
				s.paints += 8192
			}
		}
	}
	s.curves += 5 * len(vertices)
	s.contours += len(vertices)
	if err := graphBudget(l, s, len(g.nodes)+len(vertices), len(g.links), g.directed, 128*uint64(len(vertices))); err != nil {
		return err
	}
	names := append([]string(nil), g.vertices...)
	nodes := append([]*gomath.Object(nil), g.nodes...)
	for _, v := range vertices {
		id := strings.Clone(v.ID)
		node := gomath.Dot(v.Position, g.author.options.VertexRadius).SetFill(g.author.options.VertexColor)
		node.Name = id
		if v.Style != nil {
			node.Style = *v.Style
		}
		if err := node.ValidateVisual(); err != nil {
			return fmt.Errorf("%w: %v", ErrGraphInput, err)
		}
		if err := graphValidatePath(node.Path); err != nil {
			return err
		}
		if !graphBounds(node.Bounds()) {
			return ErrGraphGeometry
		}
		names = append(names, id)
		nodes = append(nodes, node)
	}
	g.publishGraph(names, g.edges, nodes, g.links)
	return nil
}

// AddEdges requires existing distinct endpoints, and appends unique keys in
// batch order. New links use the original edge color/width and directed tip
// policy. Surviving links, including authored paths and styles, are untouched.
func (g *Graph) AddEdges(edges []Edge) error {
	s, err := g.authorPreflight()
	if err != nil {
		return err
	}
	if len(edges) == 0 {
		return nil
	}
	l := g.author.limits
	if len(edges) > l.MaxEdges-len(g.links) {
		return ErrGraphLimit
	}
	if err := graphBudget(l, s, len(g.nodes), len(g.links)+len(edges), g.directed, 128*uint64(len(edges))); err != nil {
		return err
	}
	seen := make(map[Edge]bool, len(edges))
	for _, e := range edges {
		if err := graphAddName(&s, e.From, l); err != nil {
			return err
		}
		if err := graphAddName(&s, e.To, l); err != nil {
			return err
		}
		key := edgeKey(e, g.directed)
		_, a := g.index[e.From]
		_, b := g.index[e.To]
		_, exists := g.lookup[key]
		if !a || !b || e.From == e.To || exists || seen[key] {
			return ErrGraphInput
		}
		seen[key] = true
	}
	factor, contours := 1, 1
	if g.directed {
		factor, contours = 4, 2
	}
	s.curves += factor * len(edges)
	s.contours += contours * len(edges)
	if err := graphBudget(l, s, len(g.nodes), len(g.links)+len(edges), g.directed, 128*uint64(len(edges))); err != nil {
		return err
	}
	routes := make([]Edge, len(edges))
	links := make([]*gomath.Object, len(edges))
	o := g.author.options
	for i, e := range edges {
		routes[i] = Edge{strings.Clone(e.From), strings.Clone(e.To)}
		if g.directed {
			links[i] = gomath.Arrow(geom.V(0, 0), geom.V(1, 0), .1)
			links[i].Children[0].SetStroke(o.EdgeColor, o.EdgeWidth)
			links[i].Children[1].SetFill(o.EdgeColor)
		} else {
			links[i] = gomath.Line(geom.V(0, 0), geom.V(1, 0)).SetStroke(o.EdgeColor, o.EdgeWidth)
		}
	}
	// Temporary metadata only: reuse the legacy geometry kernel on new links.
	// No root or existing object tree is rebuilt, copied or replaced.
	batch := &Graph{vertices: g.vertices, edges: routes, index: g.index, links: links, directed: g.directed, tipSize: g.tipSize}
	bounds := make([]geom.Bounds, len(g.nodes))
	for i, v := range g.nodes {
		bounds[i] = v.Bounds()
	}
	geometry, err := batch.edgeGeometry(bounds)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrGraphGeometry, err)
	}
	batch.applyEdges(geometry)
	for _, e := range links {
		if err := e.Validate(); err != nil {
			return fmt.Errorf("%w: %v", ErrGraphGeometry, err)
		}
	}
	g.publishGraph(g.vertices, append(append([]Edge(nil), g.edges...), routes...), g.nodes, append(append([]*gomath.Object(nil), g.links...), links...))
	return nil
}

// RemoveVertices removes all requested vertices and their incident edges.
// Unknown/repeated IDs reject the entire batch; surviving input order is stable.
func (g *Graph) RemoveVertices(vertices []string) error {
	s, err := g.authorPreflight()
	if err != nil {
		return err
	}
	if len(vertices) == 0 {
		return nil
	}
	if len(vertices) > len(g.vertices) {
		return ErrGraphInput
	}
	request := graphStats{}
	for _, id := range vertices {
		if err := graphAddName(&request, id, g.author.limits); err != nil {
			return err
		}
	}
	if err := graphBudget(g.author.limits, s, len(g.nodes), len(g.links), g.directed, 128*uint64(len(vertices))+request.names); err != nil {
		return err
	}
	remove := make(map[string]bool, len(vertices))
	for _, id := range vertices {
		if _, ok := g.index[id]; !ok || remove[id] {
			return ErrGraphInput
		}
		remove[id] = true
	}
	var names []string
	var nodes []*gomath.Object
	var edges []Edge
	var links []*gomath.Object
	for i, id := range g.vertices {
		if !remove[id] {
			names = append(names, id)
			nodes = append(nodes, g.nodes[i])
		}
	}
	for i, e := range g.edges {
		if !remove[e.From] && !remove[e.To] {
			edges = append(edges, e)
			links = append(links, g.links[i])
		}
	}
	g.publishGraph(names, edges, nodes, links)
	return nil
}

// RemoveEdges normalizes undirected keys, rejects unknown/duplicate requests,
// and retains every surviving edge/node object in stable input order.
func (g *Graph) RemoveEdges(edges []Edge) error {
	s, err := g.authorPreflight()
	if err != nil {
		return err
	}
	if len(edges) == 0 {
		return nil
	}
	if len(edges) > len(g.edges) {
		return ErrGraphInput
	}
	request := graphStats{}
	for _, e := range edges {
		if err := graphAddName(&request, e.From, g.author.limits); err != nil {
			return err
		}
		if err := graphAddName(&request, e.To, g.author.limits); err != nil {
			return err
		}
	}
	if err := graphBudget(g.author.limits, s, len(g.nodes), len(g.links), g.directed, 128*uint64(len(edges))+request.names); err != nil {
		return err
	}
	remove := make(map[Edge]bool, len(edges))
	for _, e := range edges {
		key := edgeKey(e, g.directed)
		if _, ok := g.lookup[key]; !ok || remove[key] {
			return ErrGraphInput
		}
		remove[key] = true
	}
	var routes []Edge
	var links []*gomath.Object
	for i, e := range g.edges {
		if !remove[edgeKey(e, g.directed)] {
			routes = append(routes, e)
			links = append(links, g.links[i])
		}
	}
	g.publishGraph(g.vertices, routes, g.nodes, links)
	return nil
}

// All allocations and identity capture precede the single non-failing commit.
// Only metadata and the existing root's child slice change.
func (g *Graph) publishGraph(vertices []string, edges []Edge, nodes, links []*gomath.Object) {
	index := make(map[string]int, len(vertices))
	for i, v := range vertices {
		index[v] = i
	}
	lookup := make(map[Edge]int, len(edges))
	for i, e := range edges {
		lookup[edgeKey(e, g.directed)] = i
	}
	children := append(append(make([]*gomath.Object, 0, len(links)+len(nodes)), links...), nodes...)
	metadata := &Graph{Object: g.Object, nodes: nodes, links: links, directed: g.directed}
	author := captureGraphAuthor(metadata, g.author.options, g.author.limits)
	g.vertices, g.edges, g.nodes, g.links, g.index, g.lookup, g.author = vertices, edges, nodes, links, index, lookup, author
	g.Object.Children = children
}

func graphStyle(s gomath.Style) error {
	if err := s.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrGraphInput, err)
	}
	if s.StrokeWidth > gomath.MaxPrimitiveCoordinate || !finite(s.StrokeExtent()) || s.StrokeExtent() > gomath.MaxPrimitiveCoordinate {
		return ErrGraphGeometry
	}
	return nil
}
