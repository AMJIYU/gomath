package mathtex

import (
	"fmt"
	"gomath"
	"gomath/geom"
	"math"
	"strings"
	"unicode/utf8"
)

const (
	// MaxParts bounds explicit fragment handles, including empty fragments.
	MaxParts = 256
	// These additional bounds cover the combined object tree, including handles.
	MaxPartsNodes      = 1 << 18
	MaxPartsGlyphs     = MaxSourceRunes // path-bearing nodes, including structural marks
	MaxPartsTreeDepth  = 8*MaxNesting + 8
	MaxPartsCoordinate = 1e9
)

// PartsMetrics are authoring extents about baseline y=0, including zero-ink
// advances. They do not change when the resulting objects are transformed.
type PartsMetrics struct{ Advance, Height, Depth float64 }

// PartInfo retains an exact source key and its original authoring placement.
// Index distinguishes repeated keys; ObjectID identifies its stable live root.
type PartInfo struct {
	Index                     int
	Source                    string
	ObjectID                  uint64
	X, Advance, Height, Depth float64
}

// Parts owns one scene root with an ordered child per explicit fragment.
// Authoring follows gomath.Object's ownership and concurrency rules. Do not
// replace/reorder its direct children or change their IDs. Geometry, styling,
// and ordinary Transform operations on these children preserve the handles.
type Parts struct {
	object  *gomath.Object
	parts   []*gomath.Object
	info    []PartInfo
	metrics PartsMetrics
}

// NewParts checks and lays out 1..MaxParts separately valid fragments. No TeX
// syntax, kerning, declarations, or implicit atom gap crosses a boundary.
// Fragments start at the preceding advance on the same baseline; explicit
// spacing (for example a separate `\quad`) retains its advance without ink.
// On failure no partial result escapes. All fragments share resource budgets.
func NewParts(sources []string, opt Options) (*Parts, error) {
	p, _, err := LayoutParts(sources, opt)
	return p, err
}

// LayoutParts is NewParts with semantic layout extents. Empty strings remain
// addressable fragments. This is explicit isolation, not source mapping into
// a single concatenated TeX expression.
func LayoutParts(sources []string, opt Options) (*Parts, PartsMetrics, error) {
	if len(sources) == 0 || len(sources) > MaxParts {
		return nil, PartsMetrics{}, fmt.Errorf("math parts require 1..%d fragments", MaxParts)
	}
	runes := 0
	for i, source := range sources {
		if !utf8.ValidString(source) {
			return nil, PartsMetrics{}, fmt.Errorf("math part %d: source is not valid UTF-8", i)
		}
		if len(source) > 4*MaxSourceRunes || utf8.RuneCountInString(source) > MaxSourceRunes-runes {
			return nil, PartsMetrics{}, fmt.Errorf("math parts exceed %d total source runes", MaxSourceRunes)
		}
		runes += utf8.RuneCountInString(source)
	}
	// Reuse the single-expression option/style checks, including empty input.
	if _, err := layout("", opt); err != nil {
		return nil, PartsMetrics{}, err
	}
	b := partsBuilder{parser: parser{options: opt, style: opt.Style}}
	b.parser.beginOperandAudit()
	for i, source := range sources {
		// A small caller substring can retain a very large backing string.
		if err := b.append(i, strings.Clone(source)); err != nil {
			return nil, PartsMetrics{}, fmt.Errorf("math part %d: %w", i, err)
		}
	}
	root := gomath.Group(b.objects...)
	root.Name = "MathParts"
	if err := b.budget.inspect(root, 1); err != nil {
		return nil, PartsMetrics{}, err
	}
	p := &Parts{object: root, parts: append([]*gomath.Object(nil), b.objects...), info: b.info, metrics: b.metrics}
	return p, p.metrics, nil
}

type partsBuilder struct {
	parser  parser
	budget  partsBudget
	objects []*gomath.Object
	info    []PartInfo
	metrics PartsMetrics
}

func (b *partsBuilder) append(index int, source string) error {
	p := &b.parser
	// Reset lexical/style state only. Cells, curves and charged hidden operands
	// survive every boundary. The parser's row machinery retains MaxNesting.
	p.source, p.position = []rune(source), 0
	p.style, p.alphabet = p.options.Style, ordinaryAlphabet
	p.display, p.fractionEm = false, 0
	fragment, _, err := p.row(topRow, p.options.Size)
	if err != nil {
		return err
	}
	// This audit also charges all legacy structural paths, exactly once, while
	// retaining charges for discarded phantom ink from previous fragments.
	if err := p.auditOperand(fragment); err != nil {
		return err
	}
	if err := b.budget.inspect(fragment.object, 2); err != nil {
		return err
	}
	for object := range p.charged {
		// Hidden path nodes are no longer reachable from fragment.object.
		if err := b.budget.inspect(object, 2); err != nil {
			return err
		}
	}
	next := b.metrics.Advance + fragment.width
	if !partsCoordinate(next) || !partsCoordinate(fragment.height) || !partsCoordinate(fragment.depth) || fragment.width > 0 && next <= b.metrics.Advance {
		return fmt.Errorf("math parts extents overflow or advance underflows")
	}
	fragment.object.Shift(geom.V(b.metrics.Advance, 0))
	fragment.object.Name = source
	// Check coordinates again after placing the fragment, without double charging.
	if err := partsCheckCoordinates(fragment.object); err != nil {
		return err
	}
	b.info = append(b.info, PartInfo{Index: index, Source: source, ObjectID: fragment.object.ID,
		X: b.metrics.Advance, Advance: fragment.width, Height: fragment.height, Depth: fragment.depth})
	b.objects = append(b.objects, fragment.object)
	b.metrics = PartsMetrics{Advance: next, Height: math.Max(b.metrics.Height, fragment.height), Depth: math.Max(b.metrics.Depth, fragment.depth)}
	return nil
}

// Shared with checked copies/snapshots. Iterative inspection rejects malicious
// authoring cycles/depth before recursive Object.Copy/Validate can be reached.
type partsBudget struct {
	nodes, glyphs, curves, contours int
	seen                            map[*gomath.Object]bool
	strict                          bool
}

func (b *partsBudget) inspect(root *gomath.Object, depth int) error {
	if b.seen == nil {
		b.seen = make(map[*gomath.Object]bool)
	}
	type entry struct {
		object *gomath.Object
		depth  int
	}
	stack := []entry{{root, depth}}
	for len(stack) > 0 {
		e := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if e.object == nil || e.depth > MaxPartsTreeDepth {
			return fmt.Errorf("math parts have a nil node or exceed %d tree depth", MaxPartsTreeDepth)
		}
		if b.seen[e.object] {
			if b.strict {
				return fmt.Errorf("math parts contain shared ownership or a cycle")
			}
			continue
		}
		b.seen[e.object] = true
		b.nodes++
		if len(e.object.Children) > MaxPartsNodes-b.nodes-len(stack) || len(e.object.Path.Contours) > MaxLayoutCurves-b.contours {
			return fmt.Errorf("math parts exceed aggregate child/contour budget")
		}
		b.contours += len(e.object.Path.Contours)
		if len(e.object.Path.Contours) > 0 {
			b.glyphs++
		}
		for _, contour := range e.object.Path.Contours {
			if len(contour.Curves) > MaxLayoutCurves-b.curves {
				return fmt.Errorf("math parts exceed aggregate layout curve budget")
			}
			b.curves += len(contour.Curves)
		}
		if b.nodes > MaxPartsNodes || b.glyphs > MaxPartsGlyphs {
			return fmt.Errorf("math parts exceed aggregate node/glyph budget (%d/%d)", MaxPartsNodes, MaxPartsGlyphs)
		}
		if b.curves > MaxLayoutCurves {
			return fmt.Errorf("math parts exceed aggregate layout curve budget")
		}
		if err := e.object.ValidateVisual(); err != nil {
			return fmt.Errorf("math parts: %w", err)
		}
		if err := partsNodeCoordinates(e.object); err != nil {
			return err
		}
		for _, child := range e.object.Children {
			stack = append(stack, entry{child, e.depth + 1})
		}
	}
	return nil
}

func partsCoordinate(v float64) bool { return finite(v) && math.Abs(v) <= MaxPartsCoordinate }
func partsNodeCoordinates(o *gomath.Object) error {
	check := func(p geom.Vec3) bool { return partsCoordinate(p.X) && partsCoordinate(p.Y) && partsCoordinate(p.Z) }
	for _, c := range o.Path.Contours {
		if !check(c.Start) {
			return fmt.Errorf("math parts coordinates exceed +/-1e9")
		}
		for _, curve := range c.Curves {
			if !check(curve.P0) || !check(curve.P1) || !check(curve.P2) || !check(curve.P3) {
				return fmt.Errorf("math parts coordinates exceed +/-1e9")
			}
		}
	}
	return nil
}

func partsCheckCoordinates(root *gomath.Object) error {
	var err error
	root.Walk(func(o *gomath.Object) {
		if err == nil {
			err = partsNodeCoordinates(o)
		}
	})
	return err
}

func (p *Parts) Object() *gomath.Object { return p.object }
func (p *Parts) Len() int               { return len(p.parts) }
func (p *Parts) Metrics() PartsMetrics  { return p.metrics }
func (p *Parts) Info(index int) (PartInfo, bool) {
	if index < 0 || index >= len(p.info) {
		return PartInfo{}, false
	}
	return p.info[index], true
}
func (p *Parts) Part(index int) (*gomath.Object, bool) {
	if index < 0 || index >= len(p.parts) {
		return nil, false
	}
	return p.parts[index], true
}
func partsKeys(info []PartInfo) []string {
	keys := make([]string, len(info))
	for i, v := range info {
		keys[i] = v.Source
	}
	return keys
}
func (p *Parts) Keys() []string { return partsKeys(p.info) }

// PartSelection is an ordered set of borrowed live handles, never another
// owning group. Objects returns an independent slice containing those handles.
// Do not add borrowed handles as separate scene roots; add Copy instead.
type PartSelection struct{ objects []*gomath.Object }

func selectParts(info []PartInfo, objects []*gomath.Object, key string) PartSelection {
	s := PartSelection{}
	for i, v := range info {
		if v.Source == key {
			s.objects = append(s.objects, objects[i])
		}
	}
	return s
}
func (p *Parts) Select(key string) PartSelection  { return selectParts(p.info, p.parts, key) }
func (s PartSelection) Len() int                  { return len(s.objects) }
func (s PartSelection) Objects() []*gomath.Object { return append([]*gomath.Object(nil), s.objects...) }

// Copy returns a standalone owning group with fresh IDs, including when empty.
func (s PartSelection) Copy() (*gomath.Object, error) {
	root := gomath.Group(s.objects...)
	if err := partsValidateTree(root); err != nil {
		return nil, err
	}
	return root.Copy(), nil
}

func partsValidateTree(root *gomath.Object) error {
	budget := partsBudget{strict: true}
	if err := budget.inspect(root, 1); err != nil {
		return err
	}
	return root.Validate()
}

func (p *Parts) validate() error {
	if p == nil || p.object == nil || len(p.parts) != len(p.info) || len(p.object.Children) != len(p.parts) {
		return fmt.Errorf("invalid math parts ownership")
	}
	for i, part := range p.parts {
		if part == nil || p.object.Children[i] != part || part.ID != p.info[i].ObjectID {
			return fmt.Errorf("math part %d ownership/identity changed", i)
		}
	}
	return partsValidateTree(p.object)
}

// Copy checks the authored tree and copies it with fresh identities. Keys and
// original extents are retained, and selections refer only to the new tree.
func (p *Parts) Copy() (*Parts, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	root := p.object.Copy()
	info := append([]PartInfo(nil), p.info...)
	for i := range info {
		info[i].ObjectID = root.Children[i].ID
	}
	return &Parts{object: root, parts: append([]*gomath.Object(nil), root.Children...), info: info, metrics: p.metrics}, nil
}

// PartsSnapshot owns immutable captured geometry, exact keys and IDs. Accessors
// return values or detached deep copies. It may be read concurrently after
// authoring has finished, and is suitable as input to a future matching planner.
// It does not implement TransformMatchingTex.
type PartsSnapshot struct {
	object  *gomath.Object
	info    []PartInfo
	metrics PartsMetrics
}

func partsCapture(root *gomath.Object) *gomath.Object {
	copy := root.Copy()
	var ids []uint64
	root.Walk(func(o *gomath.Object) { ids = append(ids, o.ID) })
	i := 0
	copy.Walk(func(o *gomath.Object) { o.ID = ids[i]; i++ })
	return copy
}

// Snapshot requires exclusive access while capturing the live authored tree.
func (p *Parts) Snapshot() (*PartsSnapshot, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	return &PartsSnapshot{object: partsCapture(p.object), info: append([]PartInfo(nil), p.info...), metrics: p.metrics}, nil
}

func (s *PartsSnapshot) Len() int              { return len(s.info) }
func (s *PartsSnapshot) Keys() []string        { return partsKeys(s.info) }
func (s *PartsSnapshot) Metrics() PartsMetrics { return s.metrics }
func (s *PartsSnapshot) Info(index int) (PartInfo, bool) {
	if index < 0 || index >= len(s.info) {
		return PartInfo{}, false
	}
	return s.info[index], true
}

// Object and Part preserve captured IDs for lookup; use Object().Copy() to add
// the captured geometry alongside its source in one scene.
func (s *PartsSnapshot) Object() *gomath.Object { return partsCapture(s.object) }
func (s *PartsSnapshot) Part(index int) (*gomath.Object, bool) {
	if index < 0 || index >= len(s.info) {
		return nil, false
	}
	return partsCapture(s.object.Children[index]), true
}

// Select returns detached handles; mutating them cannot change the snapshot.
func (s *PartsSnapshot) Select(key string) PartSelection {
	objects := make([]*gomath.Object, len(s.info))
	for i, info := range s.info {
		if info.Source == key {
			objects[i] = partsCapture(s.object.Children[i])
		}
	}
	return selectParts(s.info, objects, key)
}
