package render

import (
	"bytes"
	"fmt"
	"gomath"
	"gomath/geom"
	"gomath/path"
	"math"
	"reflect"
	"strings"
	"testing"
)

func dashPattern(t testing.TB, lengths []float64, offset float64) *gomath.DashPattern {
	t.Helper()
	p, err := gomath.NewDashPattern(lengths, offset)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func dashLine(coords ...float64) path.Polyline {
	line := path.Polyline{}
	for i := 0; i < len(coords); i += 2 {
		line.Points = append(line.Points, geom.V(coords[i], coords[i+1]))
	}
	return line
}
func assertDashEndpoints(t *testing.T, parts []dashPiece, want [][4]float64) {
	t.Helper()
	if len(parts) != len(want) {
		t.Fatalf("pieces=%d want %d: %+v", len(parts), len(want), parts)
	}
	for i, part := range parts {
		a, b := part.Points[0], part.Points[len(part.Points)-1]
		got := [4]float64{a.X, a.Y, b.X, b.Y}
		for j := range got {
			if math.Abs(got[j]-want[i][j]) > 1e-9 {
				t.Fatalf("piece %d endpoints %v want %v", i, got, want[i])
			}
		}
	}
}
func TestDashPolylineLengthsOffsetAndContourReset(t *testing.T) {
	line := dashLine(0, 0, 4, 0, 4, 8)
	for _, tc := range []struct {
		offset float64
		want   [][4]float64
	}{
		{0, [][4]float64{{0, 0, 4, 1}, {4, 4, 4, 8}}},
		{2, [][4]float64{{0, 0, 3, 0}, {4, 2, 4, 7}}},
		{-2, [][4]float64{{2, 0, 4, 3}, {4, 6, 4, 8}}},
		{10, [][4]float64{{0, 0, 3, 0}, {4, 2, 4, 7}}},
	} {
		t.Run(fmt.Sprint(tc.offset), func(t *testing.T) {
			parts, err := dashLines([]path.Polyline{line}, dashPattern(t, []float64{5, 3}, tc.offset))
			if err != nil {
				t.Fatal(err)
			}
			assertDashEndpoints(t, parts, tc.want)
			if tc.offset == 0 && len(parts[0].Points) != 3 {
				t.Fatal("lost original corner inside dash")
			}
		})
	}
	parts, err := dashLines([]path.Polyline{dashLine(0, 0, 7, 0), dashLine(0, 10, 7, 10)}, dashPattern(t, []float64{3, 2}, 0))
	if err != nil {
		t.Fatal(err)
	}
	assertDashEndpoints(t, parts, [][4]float64{{0, 0, 3, 0}, {5, 0, 7, 0}, {0, 10, 3, 10}, {5, 10, 7, 10}})
	diagonal, err := dashLines([]path.Polyline{dashLine(0, 0, 6, 8)}, dashPattern(t, []float64{5, 5}, 0))
	if err != nil {
		t.Fatal(err)
	}
	assertDashEndpoints(t, diagonal, [][4]float64{{0, 0, 3, 4}})
}
func TestDashClosedSeamAndZeroEntries(t *testing.T) {
	ring := dashLine(0, 0, 10, 0, 10, 10, 0, 10)
	ring.Closed = true
	parts, err := dashLines([]path.Polyline{ring}, dashPattern(t, []float64{12, 6}, 0))
	if err != nil {
		t.Fatal(err)
	}
	assertDashEndpoints(t, parts, [][4]float64{{0, 4, 10, 2}, {10, 8, 0, 10}})
	if len(parts[0].Points) != 4 || parts[0].Points[1] != (geom.V(0, 0)) || parts[0].Closed {
		t.Fatalf("seam did not join: %+v", parts[0])
	}
	closed, err := dashLines([]path.Polyline{ring}, dashPattern(t, []float64{50, 2}, 0))
	if err != nil || len(closed) != 1 || !closed[0].Closed {
		t.Fatalf("full ring must retain joins: %+v %v", closed, err)
	}
	dots, err := dashLines([]path.Polyline{dashLine(0, 0, 10, 0)}, dashPattern(t, []float64{0, 5}, 0))
	if err != nil {
		t.Fatal(err)
	}
	assertDashEndpoints(t, dots, [][4]float64{{0, 0, 0, 0}, {5, 0, 5, 0}, {10, 0, 10, 0}})
	zeroGaps, err := dashLines([]path.Polyline{dashLine(0, 0, 10, 0)}, dashPattern(t, []float64{5, 0}, 0))
	if err != nil {
		t.Fatal(err)
	}
	assertDashEndpoints(t, zeroGaps, [][4]float64{{0, 0, 10, 0}})
	point, err := dashLines([]path.Polyline{dashLine(1, 1, 1, 1)}, dashPattern(t, []float64{5, 2}, 0))
	if err != nil || len(point) != 1 {
		t.Fatalf("zero length on point %v %v", point, err)
	}
	off, err := dashLines([]path.Polyline{dashLine(1, 1)}, dashPattern(t, []float64{5, 2}, 6))
	if err != nil || len(off) != 0 {
		t.Fatalf("zero length off point %v %v", off, err)
	}
}
func TestDashCapsReuseSolidOutlines(t *testing.T) {
	for _, cap := range []gomath.StrokeCap{gomath.CapButt, gomath.CapRound, gomath.CapSquare} {
		s := gomath.DefaultStyle()
		s.StrokeCap = cap
		s.Dash = dashPattern(t, []float64{16, 16}, 0)
		m := strokeMask(t, [][2]float64{{32, 64}, {112, 64}}, false, 4, s)
		for _, tc := range []struct {
			x     int
			paint bool
		}{{34, true}, {50, cap != gomath.CapButt}, {58, false}, {66, true}} {
			got := m.AlphaAt(tc.x, 64).A
			if (got > 0) != tc.paint {
				t.Fatalf("cap %v at %d coverage=%d", cap, tc.x, got)
			}
		}
		s.Dash = dashPattern(t, []float64{0, 16}, 0)
		dots := strokeMask(t, [][2]float64{{32, 64}, {112, 64}}, false, 4, s)
		if got := dots.AlphaAt(32, 64).A; (got > 0) != (cap != gomath.CapButt) {
			t.Fatalf("dot cap %v coverage %d", cap, got)
		}
	}
	// A dash straddles the original L vertex: its join must match a solid stroke.
	for _, join := range []gomath.StrokeJoin{gomath.JoinMiter, gomath.JoinRound, gomath.JoinBevel} {
		s := gomath.DefaultStyle()
		s.StrokeCap = gomath.CapButt
		s.StrokeJoin = join
		ref := strokeMask(t, [][2]float64{{32, 80}, {64, 80}, {64, 40}}, false, 6, s)
		s.Dash = dashPattern(t, []float64{100, 10}, 0)
		actual := strokeMask(t, [][2]float64{{32, 80}, {64, 80}, {64, 40}}, false, 6, s)
		if !bytes.Equal(actual.Pix, ref.Pix) {
			t.Fatal("dash lost cap/join continuity")
		}
	}
}
func TestDashBudgetsAndRenderingError(t *testing.T) {
	tiny := dashPattern(t, []float64{1e-8, 1e-8}, 0)
	if _, err := dashLines([]path.Polyline{dashLine(0, 0, 1, 0)}, tiny); err == nil || !strings.Contains(err.Error(), "dash fragment budget") {
		t.Fatalf("tiny pattern budget %v", err)
	}
	// The cap is per object, not per contour.
	lines := make([]path.Polyline, maxDashFragments+1)
	for i := range lines {
		lines[i] = dashLine(0, 0, 1, 0)
	}
	if _, err := dashLines(lines, dashPattern(t, []float64{2, 2}, 0)); err == nil || !strings.Contains(err.Error(), "fragment budget") {
		t.Fatalf("aggregate contour budget %v", err)
	}
	manyZero := make([]float64, gomath.MaxDashEntries)
	manyZero[1] = 1
	if _, err := dashLines([]path.Polyline{dashLine(0, 0, 1000, 0)}, dashPattern(t, manyZero, 0)); err == nil {
		t.Fatal("zero-entry work not bounded")
	}
	c := DefaultConfig()
	c.Width, c.Height, c.FrameWidth, c.Background = 64, 64, 4, gomath.Transparent
	r, err := NewRasterizer(c)
	if err != nil {
		t.Fatal(err)
	}
	o := gomath.Line(geom.V(-1, 0), geom.V(1, 0))
	o.Style.Dash = tiny
	if out, err := r.Render(&gomath.Frame{Objects: []*gomath.Object{o}}); err == nil || out != nil {
		t.Fatal("renderer silently accepted excessive dashes")
	}
	unchanged := dashLine(0, 0, 4, 0)
	before := append([]geom.Vec3(nil), unchanged.Points...)
	_, err = dashLines([]path.Polyline{unchanged}, dashPattern(t, []float64{1, 1}, 0))
	if err != nil || !reflect.DeepEqual(unchanged.Points, before) {
		t.Fatal("dashing mutates flattened path")
	}
}
func BenchmarkDashStrokeRender(b *testing.B) {
	c := DefaultConfig()
	c.Width, c.Height, c.FrameWidth, c.Workers, c.Background = 512, 256, 8, 1, gomath.Transparent
	r, err := NewRasterizer(c)
	if err != nil {
		b.Fatal(err)
	}
	o := gomath.Circle(1.5).SetStroke(gomath.RGB(.9, .2, .1), .035)
	o.Style.Dash = dashPattern(b, []float64{.1, .05}, .03)
	frame := &gomath.Frame{Objects: []*gomath.Object{o}}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := r.Render(frame); err != nil {
			b.Fatal(err)
		}
	}
}

func TestDashSquareDotUsesPathTangent(t *testing.T) {
	s := gomath.DefaultStyle()
	s.StrokeCap = gomath.CapSquare
	s.Dash = dashPattern(t, []float64{0, 50}, 0)
	actual := strokeMask(t, [][2]float64{{40, 40}, {80, 80}}, false, 6, s)
	if actual.AlphaAt(47, 40).A == 0 || actual.AlphaAt(45, 45).A != 0 {
		t.Fatal("square dot was not rotated to diagonal tangent")
	}
	// The point/output budget is checked across contours, including original
	// vertices contained in a single long dash, not only fragment endpoints.
	budget := dashBudget{points: maxDashPoints - 1}
	if _, err := dashContour(dashLine(0, 0, 1, 0), []float64{2, 2}, 0, &budget); err == nil || !strings.Contains(err.Error(), "point budget") {
		t.Fatalf("dash point budget: %v", err)
	}
}

func TestDashNativeColorOpacityAndReveal(t *testing.T) {
	c := DefaultConfig()
	c.Width, c.Height, c.FrameWidth, c.Workers, c.Background = 128, 64, 128, 1, gomath.Transparent
	r, err := NewRasterizer(c)
	if err != nil {
		t.Fatal(err)
	}
	red := gomath.RGB(1, 0, 0)
	red.A = .5
	line := gomath.Line(geom.V(-50, 0), geom.V(50, 0)).SetStroke(red, 4)
	line.Style.StrokeCap = gomath.CapButt
	line.Style.Dash = dashPattern(t, []float64{10, 10}, 0)
	line.SetOpacity(.5)
	frame := &gomath.Frame{Objects: []*gomath.Object{line}}
	for _, reveal := range []float64{1, .5} {
		line.Style.Reveal = reveal
		img, err := r.Render(frame)
		if err != nil {
			t.Fatal(err)
		}
		p := img.RGBAAt(18, 32)
		if p.R != 64 || p.G != 0 || p.B != 0 || p.A != 64 {
			t.Fatalf("premultiplied dash color/opacity: %v", p)
		}
		if img.RGBAAt(28, 32).A != 0 {
			t.Fatal("dash gap was painted")
		}
		if reveal == .5 && img.RGBAAt(98, 32).A != 0 {
			t.Fatal("Reveal failed to trim dashed path")
		}
	}
}

func TestDashNativeCircleRoundoffSeam(t *testing.T) {
	o := gomath.Circle(1.5)
	lines := o.Path.Flatten(.02 / 64)
	pieces, err := dashLines(lines, dashPattern(t, []float64{.1, .05}, .03))
	if err != nil || len(pieces) < 60 {
		t.Fatalf("native circle seam: %d pieces %v", len(pieces), err)
	}
}

func TestDashConsecutiveZeroGapsPreserveJoins(t *testing.T) {
	s := gomath.DefaultStyle()
	s.StrokeCap = gomath.CapRound
	s.StrokeJoin = gomath.JoinMiter
	coords := [][2]float64{{32, 80}, {64, 80}, {64, 40}}
	reference := strokeMask(t, coords, false, 6, s)
	s.Dash = dashPattern(t, []float64{8, 0, 0, 0}, 0)
	actual := strokeMask(t, coords, false, 6, s)
	if !bytes.Equal(actual.Pix, reference.Pix) {
		t.Fatal("consecutive zero entries broke a continuous join")
	}
	ring := dashLine(0, 0, 10, 0, 10, 10, 0, 10)
	ring.Closed = true
	pieces, err := dashLines([]path.Polyline{ring}, dashPattern(t, []float64{10, 0, 0, 0}, 0))
	if err != nil || len(pieces) != 1 || !pieces[0].Closed {
		t.Fatalf("zero events at closed seam: %+v %v", pieces, err)
	}
}
