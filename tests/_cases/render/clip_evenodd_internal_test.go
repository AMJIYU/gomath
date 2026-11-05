package render

import (
	"gomath"
	"gomath/geom"
	"gomath/path"
	"image"
	"math"
	"strings"
	"testing"
)

func parityPolygon(points ...geom.Vec3) path.Path {
	var b path.Builder
	b.MoveTo(points[0])
	for _, p := range points[1:] {
		b.LineTo(p)
	}
	b.Close()
	return b.Path()
}
func parityMask(t *testing.T, p path.Path) *image.Alpha {
	t.Helper()
	c := ssConfig()
	c.Width, c.Height, c.FrameWidth = 8, 8, 8
	r, err := NewRasterizer(c)
	if err != nil {
		t.Fatal(err)
	}
	p = p.Transform(geom.Translation(geom.V(-4, 4)).Mul(geom.Scaling(1, -1, 1)))
	mask := image.NewAlpha(image.Rect(0, 0, 8, 8))
	if err := r.paintEvenOddClip(mask, p); err != nil {
		t.Fatal(err)
	}
	return mask
}
func polygonArea(p path.Path) float64 {
	sum := 0.0
	for _, c := range p.Contours {
		previous := c.Start
		for _, curve := range c.Curves {
			v := curve.P3
			sum += previous.X*v.Y - previous.Y*v.X
			previous = v
		}
		sum += previous.X*c.Start.Y - previous.Y*c.Start.X
	}
	return math.Abs(sum) / 2
}
func TestEvenOddClipAnalyticalAreaAndRationalPolygonOracle(t *testing.T) {
	bowtie := parityPolygon(geom.V(2, 2), geom.V(3, 3), geom.V(2, 3), geom.V(3, 2))
	a := parityPolygon(geom.V(.25, .5), geom.V(6.5, 1.25), geom.V(1.5, 6.75), geom.V(6.25, 6.5), geom.V(.75, 2.75))
	b := parityPolygon(geom.V(2, 1), geom.V(6, 1), geom.V(6, 6), geom.V(2, 6))
	compound := a.Clone()
	compound.Contours = append(compound.Contours, b.Contours...)
	touch := parityPolygon(geom.V(1, 1), geom.V(4, 1), geom.V(4, 5), geom.V(1, 5))
	other := parityPolygon(geom.V(4, 1), geom.V(7, 1), geom.V(7, 5), geom.V(4, 5))
	touch.Contours = append(touch.Contours, other.Contours...)
	reverse := parityPolygon(geom.V(1, 5), geom.V(4, 5), geom.V(4, 1), geom.V(1, 1))
	cancel := touch.Clone()
	cancel.Contours = append(cancel.Contours, reverse.Contours...)
	for i, p := range []path.Path{bowtie, a, compound, touch, cancel} {
		mask := parityMask(t, p)
		for y := range 8 {
			for x := range 8 {
				cell := parityPolygon(geom.V(float64(x), float64(y)), geom.V(float64(x+1), float64(y)), geom.V(float64(x+1), float64(y+1)), geom.V(float64(x), float64(y+1)))
				// Independent arrangement/classification with exact rational intersections
				// supplies area; no scan-band/coverage implementation is reused here.
				region, err := path.Boolean(p, cell, path.BooleanIntersection, path.BooleanOptions{RuleA: path.EvenOdd})
				if err != nil {
					t.Fatalf("oracle case %d pixel %d,%d: %v", i, x, y, err)
				}
				expected := uint8(math.Round(math.Min(1, polygonArea(region)) * 255))
				if got := mask.AlphaAt(x, y).A; math.Abs(float64(got)-float64(expected)) > 1 {
					t.Fatalf("case %d pixel %d,%d alpha=%d exact-arrangement area=%d", i, x, y, got, expected)
				}
			}
		}
	}
	if got := parityMask(t, bowtie).AlphaAt(2, 2).A; got != 128 {
		t.Fatal("half-pixel analytic parity area", got)
	}
	thin := parityPolygon(geom.V(1, 2.00390625), geom.V(7, 2.00390625), geom.V(7, 2.03515625), geom.V(1, 2.03515625))
	if got := parityMask(t, thin).AlphaAt(3, 2).A; got != 8 {
		t.Fatal("unsampled thin band lost", got)
	}
}
func TestEvenOddClipIntegralAndExplicitBudgets(t *testing.T) {
	for _, tc := range []struct{ a, b, want float64 }{{-2, -1, 0}, {2, 4, 1}, {.2, .8, .5}, {-1, 1, .25}, {0, 2, .75}, {-1, 2, .5}, {1, 1, 1}, {0, 0, 0}} {
		for _, order := range [][2]float64{{tc.a, tc.b}, {tc.b, tc.a}} {
			if got := clipLinearIntegral(order[0], order[1]); math.Abs(got-tc.want) > 1e-14 {
				t.Fatal("linear clamp area", tc, got)
			}
		}
	}
	c := ssConfig()
	c.Width, c.Height, c.FrameWidth = 8, 8, 8
	r, _ := NewRasterizer(c)
	p := gomath.Square(4).Path
	mask := image.NewAlpha(image.Rect(0, 0, 8, 8))
	r.clipWork = MaxClipRasterWork
	if err := r.paintEvenOddClip(mask, p); err == nil || !strings.Contains(err.Error(), "work budget") {
		t.Fatal("scan work bound ignored", err)
	}
	r.clipWork = 0
	r.clipPoints = MaxClipRasterPoints
	if err := r.paintEvenOddClip(mask, p); err == nil || !strings.Contains(err.Error(), "flattened points") {
		t.Fatal("point budget ignored", err)
	}
	r.clipPoints = 0
	if err := r.paintEvenOddClip(mask, gomath.Square(3e9).Path); err == nil || !strings.Contains(err.Error(), "coordinate") {
		t.Fatal("coordinate bound ignored", err)
	}
	// The event bound is checked before allocating per-event storage. A compact
	// line input can imply a tall direct mask without allocating its pixels.
	r.clipPoints = 0
	oversized := &image.Alpha{Stride: 1, Rect: image.Rect(0, 0, 1, MaxEvenOddClipEvents)}
	if err := r.paintEvenOddClip(oversized, p); err == nil || !strings.Contains(err.Error(), "events") {
		t.Fatal("event storage bound ignored", err)
	}
}
func TestEvenOddClipWorkAbortReturnsNoPartialOutput(t *testing.T) {
	// Many individually valid coincident edges force the bounded pair sweep to
	// reject before completing; duplicate contours must never become nonzero.
	p := gomath.Square(4).Path
	source := p.Contours[0]
	for len(p.Contours) < gomath.MaxClipContours {
		p.Contours = append(p.Contours, source)
	}
	clip, err := gomath.NewClipWithRule(p, path.EvenOdd)
	if err != nil {
		t.Fatal(err)
	}
	object := gomath.Square(6).SetClip(clip)
	c := ssConfig()
	c.Width, c.Height, c.FrameWidth = 8, 8, 8
	r, _ := NewRasterizer(c)
	// Reduce remaining budget through prior ancestor work rather than changing
	// package constants or allowing a slow benchmark-sized regression.
	r.clipWork = MaxClipRasterWork - 4
	mask := image.NewAlpha(image.Rect(0, 0, 8, 8))
	if err := r.paintEvenOddClip(mask, p); err == nil {
		t.Fatal("failed to charge edge collection")
	}
	// Force rejection in normal public rendering by adding enough duplicate
	// diagonal edges for the pair/event/order work to exceed the frame limit.
	// All source controls remain within native curve/contour budgets.
	noisy := gomath.Polygon(geom.V(-3, -3), geom.V(3, 3), geom.V(-3, 3), geom.V(3, -3)).Path
	for len(noisy.Contours) < gomath.MaxClipContours {
		noisy.Contours = append(noisy.Contours, noisy.Contours[0])
	}
	noisyClip, err := gomath.NewClipWithRule(noisy, path.EvenOdd)
	if err != nil {
		t.Fatal(err)
	}
	object.SetClip(noisyClip)
	if got, err := r.Render(&gomath.Frame{Objects: []*gomath.Object{object}}); err == nil || got != nil {
		t.Fatal("bounded pathological parity sweep exposed partial output", err)
	}
	if _, err := r.Render(&gomath.Frame{}); err != nil {
		t.Fatal("failed sweep poisoned next render", err)
	}
}
