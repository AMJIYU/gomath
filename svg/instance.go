package svg

import (
	"fmt"
	"gomath"
	"gomath/geom"
	"gomath/path"
	"gomath/text"
	"math"
	"strconv"
	"strings"
)

type viewportClip struct {
	matrix        geom.Mat4 // descendant coordinates to this viewport
	width, height float64
}

type expansionContext struct {
	matrix      geom.Mat4
	style       inheritedStyle
	viewport    [2]float64
	clips       []viewportClip
	cssChain    []*element // ancestors in this light/shadow tree, never through use
	inText      bool
	textPaint   *textGradientScope // entire text glyph-cell bbox and deferred paints
	shadowRoots []*element         // clone-local clip definitions currently require explicit rejection
	bboxes      []bboxScope        // active target-local geometry ledgers
}

func (c expansionContext) transformed(local geom.Mat4) expansionContext {
	c.matrix = c.matrix.Mul(local)
	if len(c.clips) != 0 {
		clips := make([]viewportClip, len(c.clips))
		copy(clips, c.clips)
		for i := range clips {
			clips[i].matrix = clips[i].matrix.Mul(local)
		}
		c.clips = clips
	}
	if len(c.bboxes) != 0 {
		scopes := make([]bboxScope, len(c.bboxes))
		copy(scopes, c.bboxes)
		for i := range scopes {
			scopes[i].matrix = scopes[i].matrix.Mul(local)
		}
		c.bboxes = scopes
	}
	return c
}

type expansion struct {
	document     *document
	count        int
	active       map[*element]bool
	fontResolver FontResolver
	text         textBudget
	bboxText     map[bboxTextMetricKey]text.Metrics
}

func (s *expansion) instantiate(e *element, ctx expansionContext, depth int, referenced bool, useAttrs map[string]string) (*gomath.Object, error) {
	if definitionOnly(e) && !(referenced && e.name.Local == "symbol") {
		return nil, nil
	}
	if depth > maxSVGDepth {
		return nil, fmt.Errorf("SVG expanded depth exceeds %d", maxSVGDepth)
	}
	s.count++
	if s.count > maxSVGElements {
		return nil, fmt.Errorf("SVG expanded element count exceeds %d", maxSVGElements)
	}
	if s.active[e] {
		return nil, fmt.Errorf("cyclic SVG use reference at id %q", e.attrs["id"])
	}
	s.active[e] = true
	defer delete(s.active, e)
	if e.name.Space != "" && e.name.Space != svgNamespace {
		return nil, fmt.Errorf("unsupported SVG element namespace %q", e.name.Space)
	}
	if e.hasText && !textElement(e) {
		return nil, fmt.Errorf("unsupported SVG text in <%s>", e.name.Local)
	}
	ctx.cssChain = append(ctx.cssChain, e)
	attrs, err := s.document.css.attributes(e, ctx.cssChain)
	if err != nil {
		return nil, err
	}
	style, opacity, err := parseStyle(ctx.style, attrs)
	if err != nil {
		return nil, err
	}
	local, err := transform(e.attrs["transform"])
	if err != nil {
		return nil, err
	}
	ctx = ctx.transformed(local)
	if !finiteMatrix(ctx.matrix) {
		return nil, fmt.Errorf("SVG transform exceeds finite coordinate range")
	}
	for _, scope := range ctx.bboxes {
		if !finiteMatrix(scope.matrix) {
			return nil, fmt.Errorf("SVG bbox transform exceeds finite coordinate range")
		}
	}
	ctx.style = style
	// display:none removes this subtree from ancestor bboxes. visibility,
	// fill:none and opacity:0 still contribute their original geometry.
	if style.displayNone {
		ctx.bboxes = nil
	}
	if len(ctx.bboxes) != 0 && (e.name.Local == "symbol" || e.name.Local == "svg") {
		return nil, fmt.Errorf("SVG objectBoundingBox container with <%s> viewport is not yet supported", e.name.Local)
	}
	clipMatrix := ctx.matrix
	clipShadows := ctx.shadowRoots
	var clipBounds *geom.Bounds
	needsBBox := style.clipPath != "" && bboxClip(s.document.ids[style.clipPath])
	if needsBBox && (e.name.Local == "g" || e.name.Local == "text" || e.name.Local == "use" && !directBBoxUse(e)) {
		ctx, clipBounds, err = ctx.bboxTarget(s)
		if err != nil {
			return nil, err
		}
	}
	finish := func(object *gomath.Object) (*gomath.Object, error) {
		return s.applyClip(object, style.clipPath, clipMatrix, clipShadows, clipBounds)
	}
	var object *gomath.Object
	switch e.name.Local {
	case "text":
		object, err = s.textInstance(e, ctx, depth)
		if err != nil {
			return nil, err
		}
	case "svg", "g", "symbol", "use":
		object = gomath.Group()
	case "image":
		if needsBBox {
			box, boxErr := imageViewport(e, ctx.viewport)
			if boxErr != nil {
				return nil, boxErr
			}
			clipBounds = &box
		}
		if len(ctx.bboxes) != 0 {
			box, boxErr := imageViewport(e, ctx.viewport)
			if boxErr != nil {
				return nil, boxErr
			}
			if err := s.collectBBox(bboxRectangle(box.Min.X, box.Min.Y, box.Size().X, box.Size().Y), ctx); err != nil {
				return nil, err
			}
		}
		object, err = imageInstance(e, ctx)
		if err != nil {
			return nil, err
		}
	default:
		if _, ok := e.attrs["pathLength"]; ok && style.dash != nil {
			return nil, fmt.Errorf("SVG dashed pathLength calibration is not yet supported")
		}
		p, err := shape(e.name.Local, e.attrs)
		if err != nil {
			return nil, err
		}
		if needsBBox {
			if err := s.document.clipBudget.geometry(p); err != nil {
				return nil, err
			}
			box, err := bboxGeometryBounds(p)
			if err != nil {
				return nil, err
			}
			clipBounds = &box
		}
		object, err = s.styledShape(p, ctx)
		if err != nil {
			return nil, err
		}
	}
	object.Style.FillRule = style.fillRule
	object.Name = e.attrs["id"]
	object.SetOpacity(opacity)
	if style.displayNone || (style.visibilityHidden && len(object.Path.Contours) != 0) {
		object.SetOpacity(0)
	}
	if e.name.Local == "text" {
		return finish(object)
	}
	if e.name.Local == "svg" && e != s.document.root {
		return nil, fmt.Errorf("nested SVG viewports are not yet supported")
	}
	if e.name.Local == "svg" && style.overflow != "visible" {
		return nil, fmt.Errorf("SVG root viewport overflow clipping is not yet supported")
	}
	if e.name.Local == "use" {
		x, err := viewportLength(e.attrs, "x", 0, ctx.viewport[0])
		if err != nil {
			return nil, err
		}
		y, err := viewportLength(e.attrs, "y", 0, ctx.viewport[1])
		if err != nil {
			return nil, err
		}
		for i, key := range []string{"width", "height"} {
			v, err := viewportLength(e.attrs, key, ctx.viewport[i], ctx.viewport[i])
			if err != nil || v < 0 {
				return nil, fmt.Errorf("invalid SVG use %s=%q", key, e.attrs[key])
			}
		}
		for _, child := range e.children {
			if child.name.Local != "title" && child.name.Local != "desc" && child.name.Local != "metadata" {
				return nil, fmt.Errorf("unsupported child <%s> in SVG use", child.name.Local)
			}
		}
		if needsBBox && directBBoxUse(e) {
			clipBounds, err = s.clipUseBounds(e, geom.Translation(geom.V(x, y)), ctx.viewport)
			if err != nil {
				return nil, err
			}
		}
		ctx = ctx.transformed(geom.Translation(geom.V(x, y)))
		// Selectors cannot cross the use shadow boundary. Inherited values
		// still arrive through ctx.style; declarations are rematched per clone.
		ctx.cssChain = nil
		ctx.shadowRoots = append(append([]*element(nil), ctx.shadowRoots...), e.target)
		child, err := s.instantiate(e.target, ctx, depth+1, true, e.attrs)
		if err != nil {
			return nil, fmt.Errorf("SVG use %q: %w", e.attrs["href"], err)
		}
		object.Children = append(object.Children, child)
		return finish(object)
	}
	if e.name.Local == "symbol" {
		attrs["overflow"] = style.overflow
		mapping, viewport, size, clip, err := symbolViewport(attrs, useAttrs, ctx.viewport)
		if err != nil {
			return nil, err
		}
		if size[0] == 0 || size[1] == 0 {
			return finish(object)
		}
		ctx = ctx.transformed(mapping)
		if !finiteMatrix(ctx.matrix) {
			return nil, fmt.Errorf("SVG symbol viewport exceeds finite coordinate range")
		}
		clipMatrix = ctx.matrix
		ctx.viewport = viewport
		if clip {
			ctx.clips = append(ctx.clips, viewportClip{matrix: mapping, width: size[0], height: size[1]})
		}
	}
	for _, child := range e.children {
		if e.name.Local != "svg" && e.name.Local != "g" && e.name.Local != "symbol" && !definitionOnly(child) {
			return nil, fmt.Errorf("unsupported child <%s> in SVG shape <%s>", child.name.Local, e.name.Local)
		}
		instance, err := s.instantiate(child, ctx, depth+1, false, nil)
		if err != nil {
			return nil, err
		}
		if instance != nil {
			object.Children = append(object.Children, instance)
		}
	}
	return finish(object)
}

func (s *expansion) styledShape(p path.Path, ctx expansionContext) (*gomath.Object, error) {
	if !ctx.inText {
		if err := s.collectBBox(p, ctx); err != nil {
			return nil, err
		}
	}
	style := ctx.style
	fill, stroke := style.fill, style.stroke
	fill.A *= style.fillOpacity
	stroke.A *= style.strokeOpacity
	sx := math.Hypot(ctx.matrix[0], ctx.matrix[4])
	sy := math.Hypot(ctx.matrix[1], ctx.matrix[5])
	dot := ctx.matrix[0]*ctx.matrix[1] + ctx.matrix[4]*ctx.matrix[5]
	if stroke.A > 0 && (math.Abs(sx-sy) > 1e-8 || math.Abs(dot) > 1e-8) {
		return nil, fmt.Errorf("non-uniform transformed SVG strokes are not yet supported")
	}
	for _, clip := range ctx.clips {
		bounds := p.Transform(clip.matrix).Bounds()
		pad := 0.0
		if stroke.A > 0 && style.width > 0 {
			// A conservative stroke extent also covers square caps and miters.
			limit := style.miterLimit
			if limit == 0 {
				limit = 4
			}
			factor := math.Max(math.Hypot(clip.matrix[0], clip.matrix[4]), math.Hypot(clip.matrix[1], clip.matrix[5]))
			pad = style.width * factor * math.Max(math.Sqrt2, limit) / 2
		}
		if bounds.Valid && (fill.A > 0 || stroke.A > 0) &&
			(bounds.Min.X-pad < -1e-8 || bounds.Min.Y-pad < -1e-8 || bounds.Max.X+pad > clip.width+1e-8 || bounds.Max.Y+pad > clip.height+1e-8) {
			return nil, fmt.Errorf("SVG symbol content requires viewport clipping; clipping is not yet supported (overflow=visible permits overflow)")
		}
	}
	object := gomath.NewObject(p.Transform(ctx.matrix))
	object.SetFill(fill).SetStroke(stroke, style.width*sx)
	object.Style.FillRule = style.fillRule
	object.Style.StrokeCap = style.cap
	object.Style.StrokeJoin = style.join
	object.Style.MiterLimit = style.miterLimit
	if style.dash != nil {
		dash, err := gomath.NewDashPattern(style.dash.Lengths(), style.dashOffset)
		if err != nil {
			return nil, err
		}
		object.Style.Dash, err = dash.Scaled(sx)
		if err != nil {
			return nil, fmt.Errorf("SVG transformed dash: %w", err)
		}
	}
	if err := s.applyGradient(object, p, ctx); err != nil {
		return nil, err
	}
	if err := object.ValidateVisual(); err != nil {
		return nil, fmt.Errorf("invalid SVG shape: %w", err)
	}
	return object, nil
}

func finiteMatrix(matrix geom.Mat4) bool {
	for _, value := range matrix {
		if !valid(value) {
			return false
		}
	}
	return true
}

func documentView(attrs map[string]string) ([4]float64, error) {
	if value, exists := attrs["viewBox"]; exists {
		return parseViewBox(value)
	}
	w, err := length(attrs, "width", 300)
	if err != nil {
		return [4]float64{}, err
	}
	h, err := length(attrs, "height", 150)
	if err != nil {
		return [4]float64{}, err
	}
	if w <= 0 || h <= 0 {
		return [4]float64{}, fmt.Errorf("invalid SVG viewport")
	}
	return [4]float64{0, 0, w, h}, nil
}

func parseViewBox(value string) ([4]float64, error) {
	values, err := numbers(value)
	if err != nil || len(values) != 4 || values[2] <= 0 || values[3] <= 0 {
		return [4]float64{}, fmt.Errorf("invalid SVG viewBox %q", value)
	}
	return [4]float64{values[0], values[1], values[2], values[3]}, nil
}

func viewportLength(attrs map[string]string, key string, fallback, extent float64) (float64, error) {
	value, exists := attrs[key]
	if !exists || strings.TrimSpace(value) == "auto" && (key == "width" || key == "height") {
		return fallback, nil
	}
	value = strings.TrimSpace(value)
	if strings.HasSuffix(value, "%") {
		v, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(value, "%")), 64)
		v = v * extent / 100
		if err != nil || !valid(v) {
			return 0, fmt.Errorf("invalid SVG percentage %s=%q", key, value)
		}
		return v, nil
	}
	return length(attrs, key, fallback)
}

func symbolViewport(attrs, useAttrs map[string]string, parent [2]float64) (geom.Mat4, [2]float64, [2]float64, bool, error) {
	var size [2]float64
	for i, key := range []string{"width", "height"} {
		v, err := viewportLength(attrs, key, parent[i], parent[i])
		if err != nil || v < 0 {
			return geom.Mat4{}, size, size, false, fmt.Errorf("invalid SVG symbol %s=%q", key, attrs[key])
		}
		v, err = viewportLength(useAttrs, key, v, parent[i])
		if err != nil || v < 0 {
			return geom.Mat4{}, size, size, false, fmt.Errorf("invalid SVG use %s=%q", key, useAttrs[key])
		}
		size[i] = v
	}
	// SVG 2 symbol reference-point offsets need a separate placement model.
	for _, key := range []string{"x", "y", "refX", "refY"} {
		if value, exists := attrs[key]; exists && strings.TrimSpace(value) != "0" {
			return geom.Mat4{}, size, size, false, fmt.Errorf("SVG symbol %s is not yet supported", key)
		}
	}
	overflow := attrs["overflow"]
	for _, declaration := range strings.Split(attrs["style"], ";") {
		key, value, _ := strings.Cut(declaration, ":")
		if strings.TrimSpace(key) == "overflow" {
			overflow = strings.TrimSpace(value)
		}
	}
	clip := true
	switch strings.TrimSpace(overflow) {
	case "", "hidden", "auto", "scroll":
	case "visible":
		clip = false
	default:
		return geom.Mat4{}, size, size, false, fmt.Errorf("unsupported SVG symbol overflow %q", overflow)
	}
	view, exists := attrs["viewBox"]
	if !exists {
		return geom.Identity(), size, size, clip, nil
	}
	box, err := parseViewBox(view)
	if err != nil {
		return geom.Mat4{}, size, size, clip, err
	}
	viewport := [2]float64{box[2], box[3]}
	sx, sy := size[0]/box[2], size[1]/box[3]
	x, y := 0.0, 0.0
	aspect := strings.Fields(attrs["preserveAspectRatio"])
	if len(aspect) == 0 {
		aspect = []string{"xMidYMid", "meet"}
	}
	if len(aspect) > 2 || (len(aspect) == 2 && aspect[1] != "meet" && aspect[1] != "slice") {
		return geom.Mat4{}, size, size, clip, fmt.Errorf("invalid SVG preserveAspectRatio")
	}
	if aspect[0] != "none" {
		alignments := map[string][2]float64{
			"xMinYMin": {0, 0}, "xMidYMin": {.5, 0}, "xMaxYMin": {1, 0},
			"xMinYMid": {0, .5}, "xMidYMid": {.5, .5}, "xMaxYMid": {1, .5},
			"xMinYMax": {0, 1}, "xMidYMax": {.5, 1}, "xMaxYMax": {1, 1},
		}
		alignment, ok := alignments[aspect[0]]
		if !ok {
			return geom.Mat4{}, size, size, clip, fmt.Errorf("invalid SVG preserveAspectRatio alignment %q", aspect[0])
		}
		scale := math.Min(sx, sy)
		if len(aspect) == 2 && aspect[1] == "slice" {
			scale = math.Max(sx, sy)
		}
		sx, sy = scale, scale
		x, y = (size[0]-box[2]*scale)*alignment[0], (size[1]-box[3]*scale)*alignment[1]
	}
	matrix := geom.Translation(geom.V(x, y)).Mul(geom.Scaling(sx, sy, 1)).Mul(geom.Translation(geom.V(-box[0], -box[1])))
	return matrix, viewport, size, clip, nil
}
