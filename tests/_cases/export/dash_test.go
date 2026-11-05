package export_test

import (
	"gomath"
	"gomath/export"
	"gomath/geom"
	"gomath/render"
	"gomath/svg"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestDashStrokeAttributesAndFullSVGExport(t *testing.T) {
	p, err := gomath.NewDashPattern([]float64{.125, .25, .5}, -.3)
	if err != nil {
		t.Fatal(err)
	}
	style := gomath.DefaultStyle()
	style.Dash = p
	attrs, err := export.StrokeAttributes(style)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(attrs, `stroke-dasharray="0.125 0.25 0.5 0.125 0.25 0.5"`) || !strings.Contains(attrs, `stroke-dashoffset="`+strconv.FormatFloat(p.Offset(), 'g', -1, 64)+`"`) {
		t.Fatal("dash export precision/normalization", attrs)
	}
	source := `<svg viewBox="0 0 8 8"><path d="M0 0L4 0" stroke="red" ` + attrs + `/></svg>`
	root, err := svg.Read(strings.NewReader(source), svg.Options{Width: 8})
	if err != nil {
		t.Fatal(err)
	}
	root.Walk(func(o *gomath.Object) {
		if len(o.Path.Contours) > 0 && !reflect.DeepEqual(o.Style.Dash.Lengths(), p.Lengths()) {
			t.Error("normalized dash did not round-trip")
		}
	})
	o := gomath.Line(geom.V(-2, 0), geom.V(2, 0)).SetStroke(gomath.RGB(.8, .1, .2), .1)
	o.Style.Dash = p
	o.Style.StrokeCap = gomath.CapSquare
	target := filepath.Join(t.TempDir(), "dash.svg")
	c := render.DefaultConfig()
	c.Width, c.Height, c.FrameWidth = 160, 120, 8
	if err := export.SVG(target, &gomath.Frame{Objects: []*gomath.Object{o}}, c); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `stroke-dasharray="0.125 0.25 0.5 0.125 0.25 0.5"`) {
		t.Fatalf("SVG lost native dash: %s", data)
	}
	// Export uses a uniform transform around world-unit geometry. The pattern
	// stays in the path's units; applying the pixel scale a second time is wrong.
	if !strings.Contains(string(data), `scale(20 -20)`) {
		t.Fatal("unexpected world/pixel transform")
	}
	style.Dash = &gomath.DashPattern{}
	if attrs, err := export.StrokeAttributes(style); err == nil || attrs != "" {
		t.Fatal("export accepted invalid immutable pattern")
	}
}
func TestDashExportRasterRoundTrip(t *testing.T) {
	p, err := gomath.NewDashPattern([]float64{.4, .2}, -.1)
	if err != nil {
		t.Fatal(err)
	}
	o := gomath.Polyline(geom.V(-2, 1), geom.V(0, 1), geom.V(0, -1), geom.V(2, -1)).SetStroke(gomath.RGB(.8, .2, .1), .15)
	o.Style.Dash = p
	o.Style.StrokeCap = gomath.CapRound
	o.Style.StrokeJoin = gomath.JoinRound
	c := render.DefaultConfig()
	c.Width, c.Height, c.FrameWidth, c.Workers, c.Background = 160, 120, 8, 1, gomath.Transparent
	frame := &gomath.Frame{Objects: []*gomath.Object{o}}
	r, err := render.NewRasterizer(c)
	if err != nil {
		t.Fatal(err)
	}
	before, err := r.Render(frame)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "dash.svg")
	if err := export.SVG(target, frame, c); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	// Remove the export's transparent background rect: SVG import's existing
	// shape length parser does not support percentage rectangle dimensions.
	text := string(data)
	start := strings.Index(text, "<rect ")
	end := strings.Index(text[start:], "/>") + start + 2
	text = text[:start] + text[end:]
	imported, err := svg.Read(strings.NewReader(text), svg.Options{Width: 8})
	if err != nil {
		t.Fatal(err)
	}
	after, err := r.Render(&gomath.Frame{Objects: []*gomath.Object{imported}})
	if err != nil {
		t.Fatal(err)
	}
	maxDelta := 0
	for i, v := range before.Pix {
		maxDelta = max(maxDelta, int(math.Abs(float64(int(v)-int(after.Pix[i])))))
	}
	if maxDelta > 1 {
		t.Fatalf("SVG dash export/import raster differs by %d", maxDelta)
	}
}
