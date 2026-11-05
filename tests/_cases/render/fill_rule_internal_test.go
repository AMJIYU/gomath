package render

import (
	"gomath"
	"gomath/geom"
	"gomath/path"
	"image"
	"strings"
	"testing"
)

func TestOrdinaryParitySharesMaskBudgetsAndRetainsNoPartialFrame(t *testing.T) {
	c := ssConfig()
	c.Width, c.Height, c.FrameWidth = 8, 8, 8
	r, _ := NewRasterizer(c)
	p := gomath.Square(4).Path
	s := gomath.DefaultStyle()
	s.Fill = gomath.White
	s.FillRule = path.EvenOdd
	dst := image.NewRGBA(image.Rect(0, 0, 8, 8))
	if e := r.paintParityFill(dst, p, dst.Bounds(), s, 1); e != nil {
		t.Fatal(e)
	}
	fillWork, fillPoints := r.clipWork, r.clipPoints
	if fillWork < 64 || fillPoints == 0 {
		t.Fatal("parity fill bypassed shared counters")
	}
	if e := r.paintEvenOddClip(image.NewAlpha(dst.Bounds()), p); e != nil {
		t.Fatal(e)
	}
	if r.clipWork <= fillWork || r.clipPoints != 2*fillPoints {
		t.Fatal("clip/fill counters were reset per mask")
	}
	r.clipWork = MaxClipRasterWork - 63
	if e := r.paintParityFill(dst, p, dst.Bounds(), s, 1); e == nil {
		t.Fatal("fill pixel work uncharged")
	}
	r.clipWork = 0
	r.clipPoints = MaxClipRasterPoints
	if e := r.paintParityFill(dst, p, dst.Bounds(), s, 1); e == nil || !strings.Contains(e.Error(), "flattened points") {
		t.Fatal("fill point limit", e)
	}
	r.clipPoints = 0
	if e := r.paintParityFill(dst, p, image.Rect(0, 0, MaxClipRasterPixels+1, 1), s, 1); e == nil || !strings.Contains(e.Error(), "pixel budget") {
		t.Fatal("mask allocation bound", e)
	}
	if e := r.paintParityFill(dst, gomath.Square(3e9).Path, dst.Bounds(), s, 1); e == nil || !strings.Contains(e.Error(), "coordinate") {
		t.Fatal("fill coordinate limit", e)
	}
	// A genuine public pathological scene exhausts the sweep's bounded work.
	noisy := gomath.Polygon(geom.V(-3, -3), geom.V(3, 3), geom.V(-3, 3), geom.V(3, -3)).Path
	for len(noisy.Contours) < 4096 {
		noisy.Contours = append(noisy.Contours, noisy.Contours[0])
	}
	o := gomath.NewObject(noisy).SetFill(gomath.White).SetStroke(gomath.Transparent, 0).SetFillRule(path.EvenOdd)
	if got, e := r.Render(&gomath.Frame{Objects: []*gomath.Object{o}}); e == nil || got != nil {
		t.Fatal("work error exposed partial frame", e)
	}
	got, e := r.Render(&gomath.Frame{Objects: []*gomath.Object{gomath.Square(2).SetFill(gomath.White).SetFillRule(path.EvenOdd)}})
	if e != nil || got.RGBAAt(4, 4).A != 255 {
		t.Fatal("failure poisoned mask/counters", e)
	}
}
