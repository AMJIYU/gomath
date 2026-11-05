package svg

import (
	"encoding/xml"
	"gomath/geom"
	"gomath/path"
	"strings"
	"testing"
)

func unionTestCompiler() *clipCompiler {
	return &clipCompiler{doc: &document{}, union: &clipUnionBudget{plans: make(map[*element]bool)}}
}
func unionTestRect() *element {
	return &element{name: xml.Name{Local: "rect"}, attrs: map[string]string{"width": "10", "height": "10"}}
}
func TestClipUnionSharedBudgetsPreflight(t *testing.T) {
	for _, which := range []string{"inputs", "segments", "contours", "calls"} {
		t.Run(which, func(t *testing.T) {
			c := unionTestCompiler()
			switch which {
			case "inputs":
				c.union.inputs = maxClipUnionInputs - 1
			case "segments":
				c.union.segments = maxClipUnionSourceSegments - 5
			case "contours":
				c.union.contours = maxClipUnionSourceContours - 1
			case "calls":
				c.union.calls = maxClipUnionCalls - 2
			}
			e := &element{attrs: map[string]string{}}
			p, err := c.compileUnion(e, []*element{unionTestRect(), unionTestRect()}, initialSVGStyle(), nil, geom.Identity(), 1)
			if err == nil || p != nil || !strings.Contains(err.Error(), "budget") {
				t.Fatal("shared preflight bypass", p, err)
			}
			if which != "calls" && c.union.calls != 0 {
				t.Fatal("Boolean ran before all source checks")
			}
		})
	}
	c := unionTestCompiler()
	c.doc.clipBudget.curves = maxClipTotalCurves - 3
	if _, err := c.unionChild(unionTestRect(), initialSVGStyle(), nil, geom.Identity(), false, false); err == nil {
		t.Fatal("union bypassed shared old clip curve budget")
	}
	c = unionTestCompiler()
	c.doc.clipBudget.contours = maxClipTotalContours
	if _, err := c.unionChild(unionTestRect(), initialSVGStyle(), nil, geom.Identity(), false, false); err == nil {
		t.Fatal("union bypassed shared old clip contour budget")
	}
	c = unionTestCompiler()
	c.union.calls = maxClipUnionCalls
	if p, err := c.unionBoolean(path.Path{}, path.Path{}, path.NonZero, path.NonZero); err == nil || len(p.Contours) != 0 {
		t.Fatal("unbounded Boolean reservations")
	}
}
func TestClipUnionBooleanOperandPreflight(t *testing.T) {
	c := unionTestCompiler()
	var b path.Builder
	b.MoveTo(geom.V(0, 0))
	for i := range maxClipUnionOperandSegments + 1 {
		b.LineTo(geom.V(float64(i+1), float64(i%2)))
	}
	b.Close()
	if p, err := c.unionBoolean(b.Path(), path.Path{}, path.NonZero, path.NonZero); err == nil || len(p.Contours) != 0 || c.union.calls != 0 {
		t.Fatal("operand bound bypass", err)
	}
	limits := clipUnionLimits()
	if limits.Contours <= 0 || limits.Segments <= 0 || limits.Pairs <= 0 || limits.Vertices <= 0 || limits.Edges <= 0 || limits.SplitPoints <= 0 || limits.Work <= 0 || limits.RationalBits <= 0 || limits.AllocationBytes <= 0 {
		t.Fatal("implicit backend default budget")
	}
}
