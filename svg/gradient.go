package svg

import (
	"fmt"
	"gomath"
	"gomath/geom"
	"gomath/path"
	"math"
	"strings"
)

const (
	maxGradientAttributeBytes = 4096
	maxGradientDefinitions    = 1024
	maxGradientDepth          = 64
	maxGradientSourceStops    = 8192
	maxGradientResolvedStops  = 65536
	maxGradientInstances      = 8192
	maxGradientInstanceStops  = 1 << 20
)

type gradientBudget struct{ resolved, instances, stops int }

type gradientPlan struct {
	attrs        map[string]string
	stopElements []*element
	stops        []gomath.GradientStop // normalized endpoints; immutable after compilation
	dependencies []*element            // templates, for clone-local reference rejection
	height       int
}

func gradientElement(e *element) bool {
	return e.name.Local == "linearGradient" || e.name.Local == "radialGradient"
}

func gradientReference(value string) (string, error) {
	id, err := clipReference(value)
	if err != nil || id == "" {
		return "", fmt.Errorf("SVG gradient fill requires url(#local-id) without fallback, got %q", value)
	}
	return id, nil
}

func (doc *document) gradientTarget(id string) (*element, error) {
	e := doc.ids[id]
	if e == nil {
		return nil, fmt.Errorf("unknown SVG gradient reference #%s", id)
	}
	if !gradientElement(e) || e.name.Space != "" && e.name.Space != svgNamespace {
		return nil, fmt.Errorf("SVG fill #%s must reference a linearGradient or radialGradient", id)
	}
	return e, nil
}

func (doc *document) prepareGradients() error {
	definitions, stops := 0, 0
	check := func(property, value string) error {
		if (property != "fill" && property != "stroke") || !strings.HasPrefix(strings.ToLower(trimCSS(value)), "url(") {
			return nil
		}
		if property == "stroke" {
			return fmt.Errorf("SVG stroke paintservers are not supported")
		}
		id, err := gradientReference(value)
		if err != nil {
			return err
		}
		_, err = doc.gradientTarget(id)
		return err
	}
	for _, e := range doc.elements {
		if gradientElement(e) {
			definitions++
		}
		if e.name.Local == "stop" {
			stops++
			if e.parent == nil || !gradientElement(e.parent) {
				return fmt.Errorf("SVG stop must be a direct gradient child")
			}
		}
		for k, v := range e.attrs {
			if err := check(k, v); err != nil {
				return err
			}
		}
		for _, d := range e.inline {
			if err := check(d.property, d.value); err != nil {
				return err
			}
		}
	}
	for _, r := range doc.css.rules {
		for _, d := range r.declarations {
			if err := check(d.property, d.value); err != nil {
				return err
			}
		}
	}
	if definitions > maxGradientDefinitions || stops > maxGradientSourceStops {
		return fmt.Errorf("SVG gradient source budget exceeds %d definitions or %d stops", maxGradientDefinitions, maxGradientSourceStops)
	}
	if definitions == 0 {
		return nil
	}
	for _, e := range doc.elements {
		for _, a := range e.foreignAttrs {
			if a.Space == xmlNamespace && a.Local == "base" {
				return fmt.Errorf("SVG xml:base with gradients is not supported")
			}
		}
	}
	doc.gradients = make(map[*element]*gradientPlan)
	// Reuse the definition-tree cascade evaluator used by clips. Its cache is
	// private to this compiler and never changes the elements or authored styles.
	c := gradientCompiler{doc: doc, active: make(map[*element]bool), source: clipCompiler{doc: doc, source: make(map[*element]inheritedStyle)}}
	for _, e := range doc.elements {
		if gradientElement(e) {
			if _, err := c.compile(e, 1); err != nil {
				return fmt.Errorf("SVG gradient %q: %w", e.attrs["id"], err)
			}
		}
	}
	return nil
}

type gradientCompiler struct {
	doc    *document
	active map[*element]bool
	source clipCompiler
}

func gradientAttribute(kind, key string) bool {
	switch key {
	case "id", "class", "style", "xmlns", "href", "gradientUnits", "gradientTransform", "spreadMethod":
		return true
	}
	if cssProperty(key) {
		return true
	}
	if kind == "linearGradient" {
		return key == "x1" || key == "y1" || key == "x2" || key == "y2"
	}
	return key == "cx" || key == "cy" || key == "r" || key == "fx" || key == "fy" || key == "fr"
}

func (c *gradientCompiler) compile(e *element, depth int) (*gradientPlan, error) {
	if c.active[e] {
		return nil, fmt.Errorf("cyclic SVG gradient href at #%s", e.attrs["id"])
	}
	if depth > maxGradientDepth {
		return nil, fmt.Errorf("SVG gradient reference depth exceeds %d", maxGradientDepth)
	}
	if p := c.doc.gradients[e]; p != nil {
		if depth+p.height-1 > maxGradientDepth {
			return nil, fmt.Errorf("SVG gradient reference depth exceeds %d", maxGradientDepth)
		}
		return p, nil
	}
	c.active[e] = true
	defer delete(c.active, e)
	if e.name.Space != "" && e.name.Space != svgNamespace || len(e.foreignAttrs) != 0 {
		return nil, fmt.Errorf("unsupported SVG gradient namespace/attribute")
	}
	if e.hasText {
		return nil, fmt.Errorf("SVG gradient cannot contain character data")
	}
	for k, value := range e.attrs {
		if len(value) > maxGradientAttributeBytes {
			return nil, fmt.Errorf("SVG gradient attribute exceeds %d bytes", maxGradientAttributeBytes)
		}
		if trimCSS(value) == "" && k != "id" && k != "class" && k != "style" && k != "gradientTransform" {
			return nil, fmt.Errorf("empty SVG gradient attribute %q", k)
		}
		if !gradientAttribute(e.name.Local, k) {
			return nil, fmt.Errorf("unsupported SVG gradient attribute %q", k)
		}
	}
	p := &gradientPlan{attrs: make(map[string]string), height: 1, dependencies: []*element{e}}
	if href, ok := e.attrs["href"]; ok {
		if !strings.HasPrefix(trimCSS(href), "#") {
			return nil, fmt.Errorf("SVG gradient href requires a local fragment, got %q", href)
		}
		id, err := gradientReference("url(" + trimCSS(href) + ")")
		if err != nil {
			return nil, fmt.Errorf("SVG gradient href: %w", err)
		}
		target, err := c.doc.gradientTarget(id)
		if err != nil {
			return nil, err
		}
		if target.name.Local != e.name.Local {
			return nil, fmt.Errorf("SVG gradient mixed-type href templates are not supported")
		}
		parent, err := c.compile(target, depth+1)
		if err != nil {
			return nil, err
		}
		for k, v := range parent.attrs {
			p.attrs[k] = v
		}
		p.stopElements = parent.stopElements
		p.height = parent.height + 1
		p.dependencies = append(p.dependencies, parent.dependencies...)
	}
	for k, v := range e.attrs {
		switch k {
		case "gradientUnits", "gradientTransform", "spreadMethod", "x1", "y1", "x2", "y2", "cx", "cy", "r", "fx", "fy", "fr":
			p.attrs[k] = v
		}
	}
	var own []*element
	for _, child := range e.children {
		switch child.name.Local {
		case "title", "desc", "metadata":
			if child.name.Space != "" && child.name.Space != svgNamespace {
				return nil, fmt.Errorf("unsupported SVG gradient child namespace")
			}
		case "stop":
			own = append(own, child)
		default:
			return nil, fmt.Errorf("unsupported SVG gradient child <%s>", child.name.Local)
		}
	}
	if len(own) > 0 {
		p.stopElements = own
	}
	if len(p.stopElements) > gomath.MaxGradientStops {
		return nil, fmt.Errorf("SVG gradient exceeds %d stops", gomath.MaxGradientStops)
	}
	style, chain, err := c.source.sourceStyle(e)
	if err != nil {
		return nil, err
	}
	for _, stop := range p.stopElements {
		if stop.name.Space != "" && stop.name.Space != svgNamespace || len(stop.foreignAttrs) != 0 {
			return nil, fmt.Errorf("unsupported SVG stop namespace/attribute")
		}
		if stop.hasText || len(stop.children) != 0 {
			return nil, fmt.Errorf("SVG stop must be empty")
		}
		for k, value := range stop.attrs {
			if len(value) > maxGradientAttributeBytes {
				return nil, fmt.Errorf("SVG stop attribute exceeds %d bytes", maxGradientAttributeBytes)
			}
			if k != "offset" && k != "id" && k != "class" && k != "style" && k != "xmlns" && !cssProperty(k) {
				return nil, fmt.Errorf("unsupported SVG stop attribute %q", k)
			}
		}
		// Own stops use their light tree; inherited template children live in a
		// shadow tree. Selectors cannot cross its boundary, but inherited CSS values
		// (not template attributes) come from the referencing gradient.
		stopChain := append(append([]*element(nil), chain...), stop)
		if len(own) == 0 {
			stopChain = []*element{stop}
		}
		attrs, err := c.doc.css.attributes(stop, stopChain)
		if err != nil {
			return nil, err
		}
		computed, _, err := parseStyle(style, attrs)
		if err != nil {
			return nil, err
		}
		if v, ok := stop.attrs["offset"]; ok && trimCSS(v) == "" {
			return nil, fmt.Errorf("empty SVG stop offset")
		}
		offset, err := gradientScalar(stop.attrs["offset"], 0, 1, true)
		if err != nil {
			return nil, fmt.Errorf("SVG stop offset: %w", err)
		}
		offset = math.Max(0, math.Min(1, offset))
		if len(p.stops) > 0 {
			offset = math.Max(offset, p.stops[len(p.stops)-1].Offset)
		}
		color := computed.stopColor
		color.A *= computed.stopOpacity
		p.stops = append(p.stops, gomath.GradientStop{Offset: offset, Color: color})
	}
	if len(p.stops) >= 2 {
		if p.stops[0].Offset > 0 {
			p.stops = append([]gomath.GradientStop{{Offset: 0, Color: p.stops[0].Color}}, p.stops...)
		}
		if p.stops[len(p.stops)-1].Offset < 1 {
			p.stops = append(p.stops, gomath.GradientStop{Offset: 1, Color: p.stops[len(p.stops)-1].Color})
		}
	}
	if len(p.stops) > gomath.MaxGradientStops {
		return nil, fmt.Errorf("SVG gradient normalized endpoints exceed %d stops", gomath.MaxGradientStops)
	}
	if len(p.stops) > maxGradientResolvedStops-c.doc.gradientBudget.resolved {
		return nil, fmt.Errorf("SVG gradient resolved stop budget exceeds %d", maxGradientResolvedStops)
	}
	c.doc.gradientBudget.resolved += len(p.stops)
	view, err := documentView(c.doc.root.attrs)
	if err != nil {
		return nil, err
	}
	// Validate unused templates too, using the document viewport. Instances resolve
	// percentages again in their current (possibly symbol) viewport.
	if _, _, err := p.paint(e.name.Local, geom.Bounds{Valid: true, Min: geom.V(0, 0), Max: geom.V(1, 1)}, [2]float64{view[2], view[3]}, geom.Identity(), 1); err != nil {
		return nil, err
	}
	c.doc.gradients[e] = p
	return p, nil
}

func gradientNumber(value string) (float64, error) {
	s := scanner{data: value}
	v, err := s.number()
	if err != nil || s.pos != len(value) || strings.Contains(value, ",") {
		return 0, fmt.Errorf("invalid finite SVG gradient number %q", value)
	}
	return v, nil
}

// Bounding-box units are unitless fractions or percentages; user-space lengths
// reuse the existing absolute SVG length parser. No em/ex/calc/viewport units.
func gradientScalar(value string, fallback, extent float64, bbox bool) (float64, error) {
	value = trimCSS(value)
	if value == "" {
		return fallback, nil
	}
	if strings.HasSuffix(value, "%") {
		v, err := gradientNumber(trimCSS(value[:len(value)-1]))
		v = v * extent / 100
		if err != nil || !valid(v) {
			return 0, fmt.Errorf("invalid gradient percentage %q", value)
		}
		return v, nil
	}
	if bbox {
		v, err := gradientNumber(value)
		if err != nil || !valid(v) {
			return 0, fmt.Errorf("SVG objectBoundingBox gradient requires finite fractions or percentages, got %q", value)
		}
		return v, nil
	}
	return length(map[string]string{"gradient": value}, "gradient", fallback)
}

// Returns either a native gradient or an exact solid for SVG's empty, one-stop
// and degenerate-gradient cases. Matrix order preserves the SVG gradient frame,
// including nonuniform object bounding boxes and skew/reflection.
func (p *gradientPlan) paint(kind string, bounds geom.Bounds, viewport [2]float64, matrix geom.Mat4, opacity float64) (*gomath.Gradient, gomath.Color, error) {
	units := p.attrs["gradientUnits"]
	bbox := units == "" || units == "objectBoundingBox"
	if !bbox && units != "userSpaceOnUse" {
		return nil, gomath.Color{}, fmt.Errorf("unsupported SVG gradientUnits %q", units)
	}
	spread := p.attrs["spreadMethod"]
	if spread != "" && spread != "pad" {
		return nil, gomath.Color{}, fmt.Errorf("SVG gradient spreadMethod %q is not supported; requires pad", spread)
	}
	local, err := transform(p.attrs["gradientTransform"])
	if err != nil {
		return nil, gomath.Color{}, err
	}
	// Validate even empty/solid paints: an unsupported transform must not quietly
	// become accepted merely because stops happen to be transparent.
	probe, _ := gomath.NewLinearGradient(geom.V(0, 0), geom.V(1, 0), []gomath.GradientStop{{Offset: 0, Color: gomath.Black}, {Offset: 1, Color: gomath.Black}})
	if _, err = probe.Transform(local); err != nil {
		return nil, gomath.Color{}, fmt.Errorf("SVG gradientTransform: %w", err)
	}
	extent := viewport
	if bbox {
		extent = [2]float64{1, 1}
	}
	coord := func(key string, fallback, axis float64) (float64, error) {
		return gradientScalar(p.attrs[key], fallback, axis, bbox)
	}
	var start, end geom.Vec3
	radius := 0.0
	degenerate := false
	if kind == "linearGradient" {
		x1, e := coord("x1", 0, extent[0])
		if e != nil {
			return nil, gomath.Color{}, e
		}
		y1, e := coord("y1", 0, extent[1])
		if e != nil {
			return nil, gomath.Color{}, e
		}
		x2, e := coord("x2", extent[0], extent[0])
		if e != nil {
			return nil, gomath.Color{}, e
		}
		y2, e := coord("y2", 0, extent[1])
		if e != nil {
			return nil, gomath.Color{}, e
		}
		start, end = geom.V(x1, y1), geom.V(x2, y2)
		degenerate = start == end
	} else {
		cx, e := coord("cx", extent[0]/2, extent[0])
		if e != nil {
			return nil, gomath.Color{}, e
		}
		cy, e := coord("cy", extent[1]/2, extent[1])
		if e != nil {
			return nil, gomath.Color{}, e
		}
		diagonal := math.Hypot(extent[0], extent[1]) / math.Sqrt2
		radius, e = coord("r", diagonal/2, diagonal)
		if e != nil {
			return nil, gomath.Color{}, e
		}
		fx, e := coord("fx", cx, extent[0])
		if e != nil {
			return nil, gomath.Color{}, e
		}
		fy, e := coord("fy", cy, extent[1])
		if e != nil {
			return nil, gomath.Color{}, e
		}
		fr, e := coord("fr", 0, diagonal)
		if e != nil {
			return nil, gomath.Color{}, e
		}
		if fx != cx || fy != cy || fr != 0 {
			return nil, gomath.Color{}, fmt.Errorf("SVG radial gradient requires concentric focus (fx=cx, fy=cy, fr=0)")
		}
		if radius < 0 {
			return nil, gomath.Color{}, fmt.Errorf("SVG radial gradient radius must be nonnegative")
		}
		start = geom.V(cx, cy)
		degenerate = radius == 0
	}
	for _, value := range []float64{start.X, start.Y, end.X, end.Y, radius} {
		if !valid(value) || math.Abs(value) > gomath.MaxGradientCoordinate {
			return nil, gomath.Color{}, fmt.Errorf("SVG gradient coordinate exceeds +/-1e9")
		}
	}
	if bbox {
		if !bounds.Valid || bounds.Size().X == 0 || bounds.Size().Y == 0 {
			return nil, gomath.Transparent, nil
		}
		matrix = matrix.Mul(geom.Translation(bounds.Min)).Mul(geom.Scaling(bounds.Size().X, bounds.Size().Y, 1))
	}
	matrix = matrix.Mul(local)
	if _, err = probe.Transform(matrix); err != nil {
		return nil, gomath.Color{}, fmt.Errorf("SVG gradient instance frame: %w", err)
	}
	if len(p.stops) == 0 {
		return nil, gomath.Transparent, nil
	}
	if len(p.stops) == 1 || degenerate {
		color := p.stops[len(p.stops)-1].Color
		color.A *= opacity
		return nil, color, nil
	}
	stops := append([]gomath.GradientStop(nil), p.stops...)
	for i := range stops {
		stops[i].Color.A *= opacity
	}
	var g *gomath.Gradient
	if kind == "linearGradient" {
		g, err = gomath.NewLinearGradient(start, end, stops)
	} else {
		g, err = gomath.NewRadialGradient(start, radius, stops)
	}
	if err != nil {
		return nil, gomath.Color{}, err
	}
	g, err = g.Transform(matrix)
	return g, gomath.Color{}, err
}

func (s *expansion) applyGradient(object *gomath.Object, p path.Path, ctx expansionContext) error {
	id := ctx.style.fillServer
	if id == "" {
		return nil
	}
	target, err := s.document.gradientTarget(id)
	if err != nil {
		return err
	}
	plan := s.document.gradients[target]
	if plan == nil {
		return fmt.Errorf("SVG gradient #%s has no validated plan", id)
	}
	shadowSet := make(map[*element]bool, len(ctx.shadowRoots))
	for _, root := range ctx.shadowRoots {
		shadowSet[root] = true
	}
	for _, dependency := range plan.dependencies {
		for ancestor := dependency; ancestor != nil; ancestor = ancestor.parent {
			if shadowSet[ancestor] {
				return fmt.Errorf("SVG clone-local gradient #%s inside a use target is not supported; place shared definitions outside the cloned subtree", id)
			}
		}
	}

	b := &s.document.gradientBudget
	if b.instances >= maxGradientInstances || len(plan.stops) > maxGradientInstanceStops-b.stops {
		return fmt.Errorf("SVG gradient instance budget exceeds %d paints or %d stops", maxGradientInstances, maxGradientInstanceStops)
	}
	b.instances++
	b.stops += len(plan.stops)
	if ctx.inText && (plan.attrs["gradientUnits"] == "" || plan.attrs["gradientUnits"] == "objectBoundingBox") {
		if ctx.textPaint == nil {
			return fmt.Errorf("SVG text gradient #%s has no glyph-cell scope", id)
		}
		// Budget before appending. Layout must complete every anchored chunk
		// before the entire text bbox is available, even for a tspan fill.
		ctx.textPaint.paints = append(ctx.textPaint.paints, textGradientPaint{
			object: object, target: target, plan: plan,
			viewport: ctx.viewport, matrix: ctx.matrix, opacity: ctx.style.fillOpacity,
		})
		return nil
	}
	g, color, err := plan.paint(target.name.Local, p.Bounds(), ctx.viewport, ctx.matrix, ctx.style.fillOpacity)
	if err != nil {
		return fmt.Errorf("SVG gradient #%s: %w", id, err)
	}
	if g == nil {
		object.SetFill(color)
	} else {
		object.SetFillGradient(g)
	}
	return nil
}
