package svg

import (
	"gomath"
	"gomath/geom"
	"gomath/path"
	"gomath/text"
	"strings"
	"testing"
)

func textGradientExpansion(t *testing.T) (*expansion, expansionContext) {
	t.Helper()
	doc, err := readDocument(strings.NewReader(`<svg><defs><linearGradient id="g"><stop stop-color="red"/><stop offset="1" stop-color="blue"/></linearGradient></defs></svg>`))
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.prepareCSS(); err != nil {
		t.Fatal(err)
	}
	if err := doc.prepareGradients(); err != nil {
		t.Fatal(err)
	}
	style := initialSVGStyle()
	style.fillServer = "g"
	return &expansion{document: doc}, expansionContext{style: style, matrix: geom.Identity(), inText: true, viewport: [2]float64{300, 150}}
}

func TestSVGTextBBoxGradientSharedScopeAndOwnedSlices(t *testing.T) {
	s, ctx := textGradientExpansion(t)
	var outer, span geom.Bounds
	backing := make([]bboxScope, 1, 4)
	backing[0] = bboxScope{&outer, geom.Identity()}
	parent := &textNode{ctx: ctx}
	parent.ctx.bboxes = backing
	child := &textNode{ctx: ctx}
	child.ctx.bboxes = append(backing[:1:1], bboxScope{&span, geom.Identity()})
	solid := &textNode{ctx: ctx}
	solid.ctx.style.fillServer = ""
	chars := []textCharacter{{r: 'A', node: parent}, {r: 'V', node: child}, {r: ' ', node: solid}, {r: 'g', node: parent}}
	scope, err := s.textGradientTarget(chars, ctx)
	if err != nil {
		t.Fatal(err)
	}
	if scope == nil || len(parent.ctx.bboxes) != 2 || len(child.ctx.bboxes) != 3 || len(solid.ctx.bboxes) != 1 || len(scope.paints) != 0 {
		t.Fatal("wrong scope ownership")
	}
	if backing[:2][1].box != nil {
		t.Fatal("inherited backing storage was mutated")
	}
	if parent.ctx.bboxes[1].box != scope.bounds || child.ctx.bboxes[2].box != scope.bounds || solid.ctx.bboxes[0].box != scope.bounds {
		t.Fatal("not one full text ledger")
	}
	if s.document.clipBudget.paths != 1 {
		t.Fatal("scope not charged exactly once")
	}
	font := text.DefaultFont()
	cells, err := s.bboxTextCells(chars, font, 24, 10, 50)
	if err != nil {
		t.Fatal(err)
	}
	for _, cell := range cells {
		if err := s.collectBBox(cell.p, cell.character.node.ctx); err != nil {
			t.Fatal(err)
		}
	}
	if !scope.bounds.Valid || scope.bounds.Size().X == 0 || scope.bounds.Size().Y == 0 {
		t.Fatal("glyph cells not measured")
	}
}

func TestSVGTextBBoxGradientScopeAndMetricsBudgets(t *testing.T) {
	for _, kind := range []string{"scope", "layouts", "glyphs", "segments", "expanded", "metric-paths"} {
		t.Run(kind, func(t *testing.T) {
			s, ctx := textGradientExpansion(t)
			switch kind {
			case "scope":
				s.document.clipBudget.paths = maxClipPaths
			case "layouts":
				s.text.layouts = maxTextLayouts
			case "glyphs":
				s.text.glyphs = maxTextGlyphs
			case "segments":
				s.text.segments = maxTextSegments
			case "expanded":
				s.count = maxSVGElements
			case "metric-paths":
				s.document.clipBudget.paths = maxClipPaths - 1
			}
			node := &textNode{ctx: ctx}
			chars := []textCharacter{{r: 'A', node: node}}
			scope, err := s.textGradientTarget(chars, ctx)
			if kind == "scope" {
				if err == nil || scope != nil {
					t.Fatal("scope budget bypass")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.bboxTextCells(chars, text.DefaultFont(), 24, 0, 0); err == nil {
				t.Fatal("metric work budget bypass", kind)
			}
		})
	}
}

func TestSVGTextBBoxGradientQueueBudgetsAndError(t *testing.T) {
	for _, kind := range []string{"instances", "stops", "missing-scope"} {
		t.Run(kind, func(t *testing.T) {
			s, ctx := textGradientExpansion(t)
			ctx.textPaint = &textGradientScope{bounds: &geom.Bounds{Valid: true, Max: geom.V(10, 20)}}
			switch kind {
			case "instances":
				s.document.gradientBudget.instances = maxGradientInstances
			case "stops":
				s.document.gradientBudget.stops = maxGradientInstanceStops - 1
			case "missing-scope":
				ctx.textPaint = nil
			}
			object := gomath.NewObject(path.Path{})
			if err := s.applyGradient(object, path.Path{}, ctx); err == nil {
				t.Fatal("queue/error budget bypass", kind)
			}
			if ctx.textPaint != nil && len(ctx.textPaint.paints) != 0 {
				t.Fatal("rejected paint retained")
			}
		})
	}
	s, ctx := textGradientExpansion(t)
	ctx.textPaint = &textGradientScope{bounds: &geom.Bounds{Valid: true, Max: geom.V(10, 20)}}
	s.document.gradientBudget = gradientBudget{instances: maxGradientInstances - 1, stops: maxGradientInstanceStops - 2}
	object := gomath.NewObject(path.Path{})
	if err := s.applyGradient(object, path.Path{}, ctx); err != nil {
		t.Fatal("last available paint rejected", err)
	}
	if len(ctx.textPaint.paints) != 1 || s.document.gradientBudget.instances != maxGradientInstances || s.document.gradientBudget.stops != maxGradientInstanceStops {
		t.Fatal("paint not charged before queue")
	}
	if err := ctx.textPaint.finish(); err != nil || object.Style.FillGradient == nil {
		t.Fatal("boundary paint not resolved", err)
	}
	ctx.textPaint.paints[0].matrix = geom.Scaling(0, 1, 1)
	if err := ctx.textPaint.finish(); err == nil {
		t.Fatal("deferred frame failure discarded")
	}
}

func TestSVGTextBBoxGradientNoExtraScopeForUserSpaceOrSolids(t *testing.T) {
	for _, kind := range []string{"solid", "user-space", "empty"} {
		s, ctx := textGradientExpansion(t)
		s.document.clipBudget.paths = maxClipPaths
		if kind == "solid" {
			ctx.style.fillServer = ""
		}
		if kind == "user-space" {
			s.document.gradients[s.document.ids["g"]].attrs["gradientUnits"] = "userSpaceOnUse"
		}
		chars := []textCharacter{{r: 'A', node: &textNode{ctx: ctx}}}
		if kind == "empty" {
			chars = nil
		}
		scope, err := s.textGradientTarget(chars, ctx)
		if err != nil || scope != nil {
			t.Fatal("unneeded geometry scope", kind, err)
		}
	}
}

func TestSVGTextBBoxGradientZeroBoundsTransparentAndAtomicBudget(t *testing.T) {
	for _, box := range []geom.Bounds{{}, {Valid: true, Min: geom.V(5, 6), Max: geom.V(5, 20)}, {Valid: true, Max: geom.V(20, 0)}} {
		s, ctx := textGradientExpansion(t)
		ctx.textPaint = &textGradientScope{bounds: &box}
		object := gomath.NewObject(path.Path{}).SetStroke(gomath.Black, 2)
		if err := s.applyGradient(object, path.Path{}, ctx); err != nil {
			t.Fatal(err)
		}
		if err := ctx.textPaint.finish(); err != nil {
			t.Fatal(err)
		}
		if object.Style.FillGradient != nil || object.Style.Fill != gomath.Transparent || object.Style.Stroke != gomath.Black || object.Style.StrokeWidth != 2 {
			t.Fatal("zero bbox affected stroke or acquired fill")
		}
	}
	defs := `<defs><linearGradient id="g"><stop/><stop offset="1"/></linearGradient></defs>`
	object, err := Read(strings.NewReader(`<svg>`+defs+`<text fill="url(#g)">`+strings.Repeat("A", maxGradientInstances+1)+`</text></svg>`), DefaultOptions())
	if object != nil || err == nil || !strings.Contains(err.Error(), "gradient instance budget") {
		t.Fatal("unbounded deferred glyph paints", err)
	}
}
