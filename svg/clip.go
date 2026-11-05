package svg

import (
	"fmt"
	"gomath"
	"gomath/geom"
	"gomath/path"
	"strings"
	"unicode"
)

// These budgets cover validation plans and their materialized instances together.
// Source geometry is parsed once per element/rule pair, then reused immutably.
const (
	maxClipDefinitions   = 1024
	maxClipShapeBytes    = 1 << 20
	maxClipPaths         = 8192
	maxClipTotalCurves   = 1 << 20
	maxClipTotalContours = 65536
)

type clipBudget struct {
	paths, curves, contours int
	counts                  map[*gomath.Clip][2]int
}

func (b *clipBudget) consume(p *gomath.Clip) error {
	count, ok := b.counts[p]
	if !ok {
		outline := p.Path()
		count[1] = len(outline.Contours)
		for _, c := range outline.Contours {
			count[0] += len(c.Curves)
		}
		if b.counts == nil {
			b.counts = make(map[*gomath.Clip][2]int)
		}
		b.counts[p] = count
	}
	return b.add(count[0], count[1])
}

func (b *clipBudget) add(curves, contours int) error {
	if b.paths >= maxClipPaths || curves > maxClipTotalCurves-b.curves || contours > maxClipTotalContours-b.contours {
		return fmt.Errorf("SVG clip materialization budget exceeds %d paths, %d curves or %d contours", maxClipPaths, maxClipTotalCurves, maxClipTotalContours)
	}
	b.paths++
	b.curves += curves
	b.contours += contours
	return nil
}

// Deliberately no CSS escapes, external URLs, fallback values or geometry boxes.
func clipReference(value string) (string, error) {
	value = trimCSS(value)
	if strings.EqualFold(value, "none") {
		return "", nil
	}
	if len(value) < 6 || !strings.EqualFold(value[:4], "url(") || value[len(value)-1] != ')' {
		return "", fmt.Errorf("SVG clip-path requires none or url(#local-id), got %q", value)
	}
	ref := trimCSS(value[4 : len(value)-1])
	if len(ref) >= 2 && (ref[0] == '\'' || ref[0] == '"') && ref[len(ref)-1] == ref[0] {
		ref = ref[1 : len(ref)-1]
	}
	if !strings.HasPrefix(ref, "#") || len(ref) < 2 || strings.ContainsAny(ref[1:], "#%\\\"'();") || strings.IndexFunc(ref, unicode.IsSpace) >= 0 {
		return "", fmt.Errorf("SVG clip-path requires an unescaped local fragment reference, got %q", value)
	}
	return ref[1:], nil
}

func (doc *document) clipTarget(id string) (*element, error) {
	e := doc.ids[id]
	if e == nil {
		return nil, fmt.Errorf("unknown SVG clip-path reference #%s", id)
	}
	if e.name.Local != "clipPath" || e.name.Space != "" && e.name.Space != svgNamespace {
		return nil, fmt.Errorf("SVG clip-path #%s must reference a clipPath", id)
	}
	return e, nil
}

type clipShapeKey struct {
	element *element
	rule    path.FillRule
	bbox    bool
}

func clipFillRule(value string) path.FillRule {
	if value == "evenodd" {
		return path.EvenOdd
	}
	return path.NonZero
}

type clipCompiler struct {
	doc    *document
	active map[*element]bool
	source map[*element]inheritedStyle
	shapes map[clipShapeKey]*gomath.Clip
	union  *clipUnionBudget
}

// Clip definitions inherit from their light-tree ancestors, never from the
// referencing graphic. Ancestor transforms and ancestor clip effects do not
// transform/constrain a definition; only its own transform/clip-path does.
func (c *clipCompiler) sourceStyle(e *element) (inheritedStyle, []*element, error) {
	var chain []*element
	for p := e; p != nil; p = p.parent {
		chain = append(chain, p)
	}
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	style := initialSVGStyle()
	for i, p := range chain {
		if computed, ok := c.source[p]; ok {
			style = computed
			continue
		}
		attrs, err := c.doc.css.attributes(p, chain[:i+1])
		if err != nil {
			return style, nil, err
		}
		style, _, err = parseStyle(style, attrs)
		if err != nil {
			return style, nil, err
		}
		c.source[p] = style
	}
	return style, chain, nil
}

func (doc *document) prepareClips() error {
	definitions := 0
	check := func(value string) error {
		switch strings.ToLower(trimCSS(value)) {
		case "initial", "inherit", "unset":
			return nil
		}
		id, err := clipReference(value)
		if err != nil || id == "" {
			return err
		}
		_, err = doc.clipTarget(id)
		return err
	}
	// Validate references even in unused definitions, unmatched and overridden
	// author declarations, just as the CSS parser validates unsupported values.
	for _, e := range doc.elements {
		if value, ok := e.attrs["clip-path"]; ok {
			if err := check(value); err != nil {
				return err
			}
		}
		for _, d := range e.inline {
			if d.property == "clip-path" {
				if err := check(d.value); err != nil {
					return err
				}
			}
		}
		if e.name.Local == "clipPath" {
			definitions++
		}
	}
	for _, rule := range doc.css.rules {
		for _, d := range rule.declarations {
			if d.property == "clip-path" {
				if err := check(d.value); err != nil {
					return err
				}
			}
		}
	}
	if definitions > maxClipDefinitions {
		return fmt.Errorf("SVG clipPath definition count exceeds %d", maxClipDefinitions)
	}
	if definitions > 0 {
		for _, e := range doc.elements {
			for _, attr := range e.foreignAttrs {
				if attr.Space == xmlNamespace && attr.Local == "base" {
					return fmt.Errorf("SVG xml:base with clipPath is not supported; local references require the document base")
				}
			}
		}
	}
	doc.clipPlans = make(map[*element][]*gomath.Clip)
	c := clipCompiler{doc: doc, active: make(map[*element]bool), source: make(map[*element]inheritedStyle), shapes: make(map[clipShapeKey]*gomath.Clip)}
	for _, e := range doc.elements {
		if e.name.Local == "clipPath" {
			if _, err := c.compile(e, 1); err != nil {
				return fmt.Errorf("SVG clipPath %q: %w", e.attrs["id"], err)
			}
		}
	}
	return nil
}

func (c *clipCompiler) compile(e *element, depth int) ([]*gomath.Clip, error) {
	if c.active[e] {
		return nil, fmt.Errorf("cyclic SVG clip-path/use reference at id %q", e.attrs["id"])
	}
	if depth > gomath.MaxClipDepth {
		return nil, fmt.Errorf("SVG clip reference depth exceeds %d", gomath.MaxClipDepth)
	}
	if plan, ok := c.doc.clipPlans[e]; ok {
		if depth > 1 && c.union != nil && c.union.plans[e] {
			return nil, fmt.Errorf("SVG multi-child clip union nested references are unsupported")
		}
		return plan, nil
	}
	c.active[e] = true
	defer delete(c.active, e)
	if e.name.Space != "" && e.name.Space != svgNamespace {
		return nil, fmt.Errorf("unsupported SVG clipPath namespace %q", e.name.Space)
	}
	if len(e.foreignAttrs) != 0 {
		return nil, fmt.Errorf("unsupported SVG clipPath namespaced attribute %s", e.foreignAttrs[0].Local)
	}
	for key := range e.attrs {
		switch key {
		case "id", "class", "style", "xmlns", "transform", "clipPathUnits":
		default:
			if !cssProperty(key) {
				return nil, fmt.Errorf("unsupported SVG clipPath attribute %q", key)
			}
		}
	}
	if units, ok := e.attrs["clipPathUnits"]; ok && trimCSS(units) != "userSpaceOnUse" && trimCSS(units) != "objectBoundingBox" {
		return nil, fmt.Errorf("SVG clipPathUnits %q is not supported; use userSpaceOnUse or objectBoundingBox", units)
	}
	if e.hasText {
		return nil, fmt.Errorf("SVG clipPath cannot contain character data")
	}
	style, chain, err := c.sourceStyle(e)
	if err != nil {
		return nil, err
	}
	matrix, err := transform(e.attrs["transform"])
	if err != nil || !finiteMatrix(matrix) {
		return nil, fmt.Errorf("invalid SVG clipPath transform %q", e.attrs["transform"])
	}
	var child *element
	var children []*element
	for _, p := range e.children {
		switch p.name.Local {
		case "title", "desc", "metadata", "defs", "style":
			if p.name.Space == "" || p.name.Space == svgNamespace {
				continue
			}
		}
		if len(children) >= maxClipUnionChildren {
			return nil, fmt.Errorf("SVG clip union child count exceeds %d", maxClipUnionChildren)
		}
		children = append(children, p)
		child = p
	}
	if len(children) > 1 {
		plan, err := c.compileUnion(e, children, style, chain, matrix, depth)
		if err != nil {
			return nil, err
		}
		c.doc.clipPlans[e] = plan
		return plan, nil
	}
	var plan []*gomath.Clip
	if child == nil {
		if err := c.doc.clipBudget.add(0, 0); err != nil {
			return nil, err
		}
		empty, _ := gomath.NewClipWithRule(path.Path{}, clipFillRule(style.clipRule))
		plan = []*gomath.Clip{empty}
	} else {
		plan, err = c.graphic(child, style, chain, matrix, depth, bboxClip(e))
		if err != nil {
			return nil, err
		}
	}
	plan, err = c.intersect(plan, style.clipPath, matrix, depth, nil)
	if err != nil {
		return nil, err
	}
	c.doc.clipPlans[e] = plan
	return plan, nil
}

func (c *clipCompiler) intersect(plan []*gomath.Clip, id string, matrix geom.Mat4, depth int, bounds *geom.Bounds) ([]*gomath.Clip, error) {
	if id == "" {
		return plan, nil
	}
	target, err := c.doc.clipTarget(id)
	if err != nil {
		return nil, err
	}
	nested, err := c.compile(target, depth+1)
	if err != nil {
		return nil, err
	}
	matrix, ignore, err := clipPlacement(target, matrix, bounds)
	if err != nil {
		return nil, err
	}
	if ignore {
		return plan, nil
	}
	if len(plan)+len(nested) > gomath.MaxClipDepth {
		return nil, fmt.Errorf("SVG clip intersection exceeds %d paths", gomath.MaxClipDepth)
	}
	for _, p := range nested {
		if err := c.doc.clipBudget.consume(p); err != nil {
			return nil, err
		}
		q, err := p.Transform(matrix)
		if err != nil {
			return nil, fmt.Errorf("SVG nested clip transform: %w", err)
		}
		plan = append(plan, q)
	}
	return plan, nil
}

func clipShapeAttribute(name, key string) bool {
	switch name {
	case "path":
		return key == "d"
	case "rect":
		return key == "x" || key == "y" || key == "width" || key == "height" || key == "rx" || key == "ry"
	case "circle":
		return key == "cx" || key == "cy" || key == "r"
	case "ellipse":
		return key == "cx" || key == "cy" || key == "rx" || key == "ry"
	case "line":
		return key == "x1" || key == "y1" || key == "x2" || key == "y2"
	case "polygon", "polyline":
		return key == "points"
	case "use":
		return key == "href" || key == "x" || key == "y" || key == "width" || key == "height"
	}
	return false
}

func (c *clipCompiler) graphic(e *element, parent inheritedStyle, chain []*element, matrix geom.Mat4, depth int, bbox bool) ([]*gomath.Clip, error) {
	if depth > maxSVGDepth || c.active[e] {
		return nil, fmt.Errorf("cyclic or excessive SVG clip use reference at id %q", e.attrs["id"])
	}
	c.active[e] = true
	defer delete(c.active, e)
	if e.name.Space != "" && e.name.Space != svgNamespace {
		return nil, fmt.Errorf("unsupported SVG clipping graphic namespace %q", e.name.Space)
	}
	switch e.name.Local {
	case "path", "rect", "circle", "ellipse", "line", "polygon", "polyline", "use":
	default:
		return nil, fmt.Errorf("unsupported SVG clipPath child/target <%s>; requires a single basic/path shape or direct use", e.name.Local)
	}
	if len(e.foreignAttrs) != 0 {
		return nil, fmt.Errorf("unsupported SVG clipping graphic namespaced attribute %s", e.foreignAttrs[0].Local)
	}
	for key, value := range e.attrs {
		switch key {
		case "id", "class", "style", "transform", "xmlns":
		default:
			if !cssProperty(key) && !clipShapeAttribute(e.name.Local, key) {
				return nil, fmt.Errorf("unsupported SVG clipping graphic attribute %q", key)
			}
		}
		if len(value) > maxClipShapeBytes {
			return nil, fmt.Errorf("SVG clip geometry attribute exceeds %d bytes", maxClipShapeBytes)
		}
	}
	if bbox {
		if err := validateBBoxClipLengths(e); err != nil {
			return nil, err
		}
	}
	if e.hasText {
		return nil, fmt.Errorf("SVG clipping graphic cannot contain character data")
	}
	for _, child := range e.children {
		if !definitionOnly(child) {
			return nil, fmt.Errorf("unsupported SVG clipping graphic child <%s>", child.name.Local)
		}
	}
	chain = append(chain, e)
	attrs, err := c.doc.css.attributes(e, chain)
	if err != nil {
		return nil, err
	}
	style, _, err := parseStyle(parent, attrs)
	if err != nil {
		return nil, err
	}
	local, err := transform(e.attrs["transform"])
	matrix = matrix.Mul(local)
	if err != nil || !finiteMatrix(matrix) {
		return nil, fmt.Errorf("invalid SVG clipping graphic transform")
	}
	clipMatrix := matrix
	var bounds *geom.Bounds
	var plan []*gomath.Clip
	if e.name.Local == "use" {
		// SVG clipPath use is deliberately direct-to-shape; groups/symbols/text
		// need silhouette union/shaping semantics that this representation lacks.
		if e.target == nil || e.target.name.Local == "use" {
			return nil, fmt.Errorf("SVG clipPath use requires a direct basic/path target")
		}
		x, err := length(e.attrs, "x", 0)
		if err != nil {
			return nil, err
		}
		y, err := length(e.attrs, "y", 0)
		if err != nil {
			return nil, err
		}
		for _, key := range []string{"width", "height"} {
			v, err := length(e.attrs, key, 0)
			if err != nil || v < 0 {
				return nil, fmt.Errorf("invalid SVG clip use %s", key)
			}
		}
		plan, err = c.graphic(e.target, style, nil, matrix.Mul(geom.Translation(geom.V(x, y))), depth+1, bbox)
		if err != nil {
			return nil, err
		}
	} else {
		key := clipShapeKey{element: e, rule: clipFillRule(style.clipRule), bbox: bbox}
		source := c.shapes[key]
		if source == nil {
			p, err := shape(e.name.Local, e.attrs)
			if err != nil {
				return nil, err
			}
			curves := 0
			for _, contour := range p.Contours {
				curves += len(contour.Curves)
			}
			if err := c.doc.clipBudget.add(curves, len(p.Contours)); err != nil {
				return nil, err
			}
			source, err = gomath.NewClipWithRule(p, key.rule)
			if err != nil {
				return nil, fmt.Errorf("SVG clip outline: %w", err)
			}
			c.shapes[key] = source
		}
		if style.clipPath != "" && bboxClip(c.doc.ids[style.clipPath]) {
			if err := c.doc.clipBudget.consume(source); err != nil {
				return nil, err
			}
			box, err := bboxGeometryBounds(source.Path())
			if err != nil {
				return nil, err
			}
			bounds = &box
		}
		if err := c.doc.clipBudget.consume(source); err != nil {
			return nil, err
		}
		q, err := source.Transform(matrix)
		if err != nil {
			return nil, fmt.Errorf("SVG clip transform: %w", err)
		}
		plan = []*gomath.Clip{q}
	}
	// Paint, alpha and stroke never alter a clipping silhouette. display and
	// visibility on graphical children do. Still validate hidden references.
	if style.displayNone || style.visibilityHidden && e.name.Local != "use" {
		if err := c.doc.clipBudget.add(0, 0); err != nil {
			return nil, err
		}
		empty, _ := gomath.NewClipWithRule(path.Path{}, clipFillRule(style.clipRule))
		plan = []*gomath.Clip{empty}
	}
	return c.intersect(plan, style.clipPath, clipMatrix, depth, bounds)
}

func (s *expansion) applyClip(object *gomath.Object, id string, matrix geom.Mat4, shadowRoots []*element, bounds *geom.Bounds) (*gomath.Object, error) {
	if id == "" {
		return object, nil
	}
	target, err := s.document.clipTarget(id)
	if err != nil {
		return nil, err
	}
	for p := target; p != nil; p = p.parent {
		for _, root := range shadowRoots {
			if p == root {
				return nil, fmt.Errorf("SVG clone-local clipPath #%s inside a use target is not yet supported; place shared clip definitions outside the cloned subtree", id)
			}
		}
	}
	plan := s.document.clipPlans[target]
	if len(plan) == 0 {
		return nil, fmt.Errorf("SVG clip-path #%s has no validated plan", id)
	}
	matrix, ignore, err := clipPlacement(target, matrix, bounds)
	if err != nil {
		return nil, err
	}
	if ignore {
		return object, nil
	}
	for _, p := range plan {
		if err := s.document.clipBudget.consume(p); err != nil {
			return nil, err
		}
		q, err := p.Transform(matrix)
		if err != nil {
			return nil, fmt.Errorf("SVG clip instance transform: %w", err)
		}
		if object.Clip != nil {
			s.count++
			if s.count > maxSVGElements {
				return nil, fmt.Errorf("SVG expanded element count exceeds %d (including clip intersection groups)", maxSVGElements)
			}
			object = gomath.Group(object)
		}
		object.SetClip(q)
	}
	return object, nil
}

func validateClipDepth(o *gomath.Object, depth int) error {
	if o.Clip != nil {
		depth++
		if depth > gomath.MaxClipDepth {
			return fmt.Errorf("SVG clip ancestors exceed %d", gomath.MaxClipDepth)
		}
	}
	for _, child := range o.Children {
		if err := validateClipDepth(child, depth); err != nil {
			return err
		}
	}
	return nil
}
