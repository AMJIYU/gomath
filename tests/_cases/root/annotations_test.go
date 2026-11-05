package gomath_test

import (
	"encoding/json"
	"gomath"
	"gomath/geom"
	"gomath/path"
	"math"
	"reflect"
	"testing"
)

func TestAnnotationsUseCompleteGeometricBoundsAndIndependentGeometry(t *testing.T) {
	target := gomath.Group(gomath.Rectangle(2, 1).Shift(geom.Vec3{X: -1, Y: .4, Z: .2}), gomath.Line(geom.Vec3{X: 1, Y: -1, Z: -.2}, geom.Vec3{X: 2, Y: 1, Z: .4}))
	target.SetStroke(gomath.Blue, 5).SetOpacity(0)
	target.ZIndex = 7
	before, err := json.Marshal(target)
	must(t, err)
	b := target.Bounds()
	box, err := gomath.NewSurroundingRectangle(target, gomath.SurroundingRectangleOptions{PaddingX: .2, PaddingY: .3, CornerRadius: .15})
	must(t, err)
	background, err := gomath.NewBackgroundRectangle(target, gomath.DefaultBackgroundRectangleOptions())
	must(t, err)
	underline, err := gomath.NewUnderline(target, gomath.UnderlineOptions{Buffer: .25})
	must(t, err)
	cross, err := gomath.NewCross(target, gomath.CrossOptions{Scale: 1.2})
	must(t, err)
	after, err := json.Marshal(target)
	must(t, err)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("annotation constructor mutated target")
	}
	near(t, box.Bounds().Size().X, b.Size().X+.4)
	near(t, box.Bounds().Size().Y, b.Size().Y+.6)
	if box.Center().Sub(b.Center()).Norm() > 1e-12 {
		t.Fatal("box center differs")
	}
	if background.Style.Fill != (gomath.Color{A: .75}) || background.Style.Stroke.A != 0 || background.Style.StrokeWidth != 0 || background.Style.Opacity != 1 {
		t.Fatal("background style differs")
	}
	near(t, underline.Bounds().Min.X, b.Min.X)
	near(t, underline.Bounds().Max.X, b.Max.X)
	near(t, underline.Center().Y, b.Min.Y-.25)
	near(t, underline.Center().Z, b.Center().Z)
	near(t, cross.Bounds().Size().X, b.Size().X*1.2)
	near(t, cross.Bounds().Size().Y, b.Size().Y*1.2)
	if len(cross.Children) != 2 || cross.Children[0].Path.Contours[0].Start.X >= cross.Children[1].Path.Contours[0].Start.X {
		t.Fatal("cross child order differs")
	}
	for _, object := range []*gomath.Object{box, background, underline, cross} {
		must(t, object.Validate())
		if object.ZIndex != 7 {
			t.Fatal("annotation lost explicit target layer")
		}
	}
	boxBefore := box.Path.Clone()
	target.Shift(geom.V(4, 3)).SetColor(gomath.Pink)
	if !reflect.DeepEqual(boxBefore, box.Path) {
		t.Fatal("target edits affected constructed annotation")
	}
	s := gomath.NewScene()
	must(t, s.Add(cross))
	child := cross.Children[0]
	id := child.ID
	must(t, s.Play(1, gomath.Transform(cross, cross.Copy().Rotate(.4).Scale(.8))))
	if cross.Children[0] != child || child.ID != id {
		t.Fatal("annotation transform replaced stable child")
	}
	frame, err := s.Sample(.4)
	must(t, err)
	frame.Objects[0].Children[0].Path.Contours[0].Curves[0].P1.X = 456
	fresh, err := s.Sample(.4)
	must(t, err)
	if reflect.DeepEqual(frame, fresh) {
		t.Fatal("annotation frames share geometry")
	}
}

func TestAnnotationsUseCurveExtremaRatherThanControlHull(t *testing.T) {
	target := gomath.CubicBezier(geom.V(0, 0), geom.V(0, 2), geom.V(2, 2), geom.V(2, 0))
	box, err := gomath.NewSurroundingRectangle(target, gomath.SurroundingRectangleOptions{})
	must(t, err)
	near(t, box.Bounds().Max.Y, 1.5)
	clip, err := gomath.NewClip(gomath.Rectangle(.1, .1).Path)
	must(t, err)
	target.SetClip(clip)
	again, err := gomath.NewSurroundingRectangle(target, gomath.SurroundingRectangleOptions{})
	must(t, err)
	if !reflect.DeepEqual(box.Path, again.Path) {
		t.Fatal("target clip incorrectly shrank bounds")
	}
}

func TestAnnotationsRejectInvalidGeometryOptionsAndExhaustedLimits(t *testing.T) {
	target := gomath.Rectangle(2, 1)
	for _, o := range []gomath.SurroundingRectangleOptions{{PaddingX: -1}, {PaddingY: math.NaN()}, {PaddingX: math.Inf(1)}, {PaddingX: 1e12}, {CornerRadius: -1}, {CornerRadius: 1}} {
		if got, err := gomath.NewSurroundingRectangle(target, o); got != nil || err == nil {
			t.Fatal("invalid surrounding options accepted", o)
		}
	}
	for _, o := range []gomath.UnderlineOptions{{Buffer: -1}, {Buffer: math.NaN()}, {Buffer: math.Inf(1)}, {Buffer: 1e12}} {
		if got, err := gomath.NewUnderline(target, o); got != nil || err == nil {
			t.Fatal("invalid underline options accepted", o)
		}
	}
	for _, scale := range []float64{0, -1, math.NaN(), math.Inf(1), 1e12} {
		if got, err := gomath.NewCross(target, gomath.CrossOptions{Scale: scale}); got != nil || err == nil {
			t.Fatal("invalid cross scale accepted", scale)
		}
	}
	if got, err := gomath.NewBackgroundRectangle(target, gomath.BackgroundRectangleOptions{Fill: gomath.Color{A: 2}}); got != nil || err == nil {
		t.Fatal("invalid background color accepted")
	}
	cycle := gomath.Group()
	cycle.Children = []*gomath.Object{cycle}
	shared := gomath.Square(1)
	oversize := gomath.Line(geom.V(0, 0), geom.V(1e12+1, 1))
	badID := gomath.Square(1)
	badID.ID = 0
	tooDeep := gomath.Square(1)
	for range gomath.MaxAnnotationDepth {
		tooDeep = gomath.Group(tooDeep)
	}
	for _, bad := range []*gomath.Object{nil, gomath.Group(), cycle, gomath.Group(shared, shared), oversize, badID, tooDeep} {
		if got, err := gomath.NewSurroundingRectangle(bad, gomath.DefaultSurroundingRectangleOptions()); got != nil || err == nil {
			t.Fatal("invalid annotation target accepted")
		}
	}
	for _, line := range []*gomath.Object{gomath.Line(geom.V(0, -1), geom.V(0, 1)), gomath.Line(geom.V(-1, 0), geom.V(1, 0))} {
		if got, err := gomath.NewCross(line, gomath.DefaultCrossOptions()); got != nil || err == nil {
			t.Fatal("degenerate cross accepted")
		}
	}
	if got, err := gomath.NewUnderline(gomath.Line(geom.V(0, -1), geom.V(0, 1)), gomath.DefaultUnderlineOptions()); got != nil || err == nil {
		t.Fatal("zero width underline accepted")
	}
	// A positive-padding box around a point is valid; no padding is not.
	point := gomath.Polyline(geom.V(0, 0))
	_, err := gomath.NewSurroundingRectangle(point, gomath.DefaultSurroundingRectangleOptions())
	must(t, err)
	if got, err := gomath.NewSurroundingRectangle(point, gomath.SurroundingRectangleOptions{}); got != nil || err == nil {
		t.Fatal("zero dimensional box accepted")
	}
}

func TestAnnotationInclusiveTreeContourAndCurveBudgets(t *testing.T) {
	deep := gomath.Square(1)
	for range gomath.MaxAnnotationDepth - 1 {
		deep = gomath.Group(deep)
	}
	_, err := gomath.NewSurroundingRectangle(deep, gomath.DefaultSurroundingRectangleOptions())
	must(t, err)
	target := gomath.Square(1)
	for range gomath.MaxAnnotationObjects - 1 {
		target.Children = append(target.Children, gomath.Group())
	}
	_, err = gomath.NewSurroundingRectangle(target, gomath.DefaultSurroundingRectangleOptions())
	must(t, err)
	target.Children = append(target.Children, gomath.Group())
	if got, err := gomath.NewSurroundingRectangle(target, gomath.DefaultSurroundingRectangleOptions()); got != nil || err == nil {
		t.Fatal("annotation object budget bypassed")
	}
	contours := make([]path.Contour, gomath.MaxAnnotationContours)
	for i := range contours {
		contours[i].Start = geom.V(float64(i%2), float64(i%3))
	}
	target = gomath.NewObject(path.Path{Contours: contours})
	_, err = gomath.NewSurroundingRectangle(target, gomath.DefaultSurroundingRectangleOptions())
	must(t, err)
	target.Path.Contours = append(target.Path.Contours, path.Contour{})
	if got, err := gomath.NewSurroundingRectangle(target, gomath.DefaultSurroundingRectangleOptions()); got != nil || err == nil {
		t.Fatal("annotation contour budget bypassed")
	}
	base := gomath.Square(1).Path.Contours[0]
	curves := make([]path.Cubic, 0, gomath.MaxAnnotationCurves+4)
	for range gomath.MaxAnnotationCurves / 4 {
		curves = append(curves, base.Curves...)
	}
	target = gomath.NewObject(path.Path{Contours: []path.Contour{{Start: base.Start, Curves: curves, Closed: true}}})
	_, err = gomath.NewSurroundingRectangle(target, gomath.DefaultSurroundingRectangleOptions())
	must(t, err)
	target.Path.Contours[0].Curves = append(target.Path.Contours[0].Curves, base.Curves...)
	if got, err := gomath.NewSurroundingRectangle(target, gomath.DefaultSurroundingRectangleOptions()); got != nil || err == nil {
		t.Fatal("annotation curve budget bypassed")
	}
}

func BenchmarkSurroundingRectangleAnnotation(b *testing.B) {
	target := gomath.Group(gomath.Rectangle(2, 1), gomath.Circle(.6).Shift(geom.V(2, 0)))
	options := gomath.DefaultSurroundingRectangleOptions()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := gomath.NewSurroundingRectangle(target, options); err != nil {
			b.Fatal(err)
		}
	}
}
