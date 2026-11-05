package svg_test

import (
	"gomath"
	"gomath/svg"
	"strings"
	"testing"
)

func TestImportStrokeInheritanceAndCSSPrecedence(t *testing.T) {
	doc := `<svg viewBox="0 0 100 100" stroke="white" stroke-width="10" stroke-linecap="square" stroke-linejoin="round" stroke-miterlimit="7">
	<g stroke-linecap="round" stroke-linejoin="bevel">
	<path id="inherited" d="M10 10L50 10L50 50"/>
	<path id="override" d="M10 20L50 20" stroke-linecap="butt" stroke-linejoin="round" style="stroke-linecap: square; stroke-linejoin: miter; stroke-miterlimit: 2.5"/>
	<path id="reset" d="M10 30L50 30" style="stroke-linecap:initial;stroke-linejoin:initial;stroke-miterlimit:initial"/>
	<path id="explicit-inherit" d="M10 40L50 40" style="stroke-linecap:inherit;stroke-linejoin:unset;stroke-miterlimit:inherit"/>
	</g></svg>`
	root, err := svg.Read(strings.NewReader(doc), svg.Options{Width: 10})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]struct {
		cap   gomath.StrokeCap
		join  gomath.StrokeJoin
		limit float64
	}{
		"inherited":        {gomath.CapRound, gomath.JoinBevel, 7},
		"override":         {gomath.CapSquare, gomath.JoinMiter, 2.5},
		"reset":            {gomath.CapButt, gomath.JoinMiter, 4},
		"explicit-inherit": {gomath.CapRound, gomath.JoinBevel, 7},
	}
	seen := 0
	root.Walk(func(o *gomath.Object) {
		w, ok := want[o.Name]
		if !ok {
			return
		}
		seen++
		if o.Style.StrokeCap != w.cap || o.Style.StrokeJoin != w.join || o.Style.EffectiveMiterLimit() != w.limit || o.Style.StrokeWidth != 1 {
			t.Errorf("%s: style %+v, want %+v with world width 1", o.Name, o.Style, w)
		}
	})
	if seen != len(want) {
		t.Fatal("imported stroke objects missing")
	}
}

func TestSVGStrokeDefaultsAndUnsupportedStyles(t *testing.T) {
	root, err := svg.Read(strings.NewReader(`<svg><path d="M0 0L100 0" stroke="white"/></svg>`), svg.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	root.Walk(func(o *gomath.Object) {
		if len(o.Path.Contours) != 0 && (o.Style.StrokeCap != gomath.CapButt || o.Style.StrokeJoin != gomath.JoinMiter || o.Style.EffectiveMiterLimit() != 4) {
			t.Error("SVG initial stroke values lost", o.Style)
		}
	})
	for _, attrs := range []string{
		`stroke-linecap="triangle"`, `stroke-linecap=""`,
		`stroke-linejoin="arcs"`, `stroke-linejoin="miter-clip"`,
		`stroke-miterlimit="0"`, `stroke-miterlimit="0.5"`, `stroke-miterlimit="-1"`,
		`stroke-miterlimit="NaN"`, `stroke-miterlimit="+Inf"`, `stroke-miterlimit="4px"`,
		`style="stroke-linejoin:arcs"`, `style="stroke-linecap:triangle"`,
		`style="stroke-miterlimit:0"`, `style="stroke-linecap:round !important"`,
		`stroke-dasharray="-2 3"`, `stroke-dashoffset="NaN"`,
		`stroke-alignment="inner"`, `stroke-dashcorner="2"`,
	} {
		for _, group := range []bool{false, true} {
			doc := `<svg><path d="M0 0L100 0" stroke="white" ` + attrs + `/></svg>`
			if group {
				doc = `<svg><g ` + attrs + `><path d="M0 0L100 0" stroke="white"/></g></svg>`
			}
			if _, err := svg.Read(strings.NewReader(doc), svg.DefaultOptions()); err == nil {
				t.Errorf("unsupported SVG style accepted: %s", doc)
			}
		}
	}
}
