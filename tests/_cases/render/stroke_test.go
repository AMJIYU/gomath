package render

import (
	"bytes"
	"golang.org/x/image/vector"
	"gomath"
	"gomath/geom"
	"gomath/path"
	"gomath/svg"
	"image"
	"image/color"
	"math"
	"strings"
	"testing"
)

// Rasterize actual outlines in pixel space, without the parent's ROI/style
// integration. This keeps cap/join coverage tests useful during that integration.
func strokeMask(t *testing.T, coords [][2]float64, closed bool, radius float64, style gomath.Style) *image.Alpha {
	t.Helper()
	c := DefaultConfig()
	c.Width, c.Height, c.FrameWidth = 128, 128, 128
	r, err := NewRasterizer(c)
	if err != nil {
		t.Fatal(err)
	}
	line := path.Polyline{Closed: closed}
	for _, p := range coords {
		line.Points = append(line.Points, geom.V(p[0]-64, 64-p[1]))
	}
	z := vector.NewRasterizer(128, 128)
	if err := r.strokeStyledLine(z, line, radius, style); err != nil {
		t.Fatal(err)
	}
	mask := image.NewAlpha(image.Rect(0, 0, 128, 128))
	z.Draw(mask, mask.Bounds(), image.NewUniform(color.Alpha{A: 255}), image.Point{})
	return mask
}

func TestStrokeCapEndpointCoverage(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		coords := [][2]float64{{32, 64}, {80, 64}}
		if reverse {
			coords[0], coords[1] = coords[1], coords[0]
		}
		for _, cap := range []gomath.StrokeCap{gomath.CapButt, gomath.CapRound, gomath.CapSquare} {
			s := gomath.DefaultStyle()
			s.StrokeCap = cap
			m := strokeMask(t, coords, false, 8, s)
			if m.AlphaAt(33, 64).A < 250 || m.AlphaAt(79, 64).A < 250 {
				t.Fatalf("cap=%v reversed=%v lost segment interior", cap, reverse)
			}
			for _, p := range []image.Point{{28, 64}, {83, 64}} {
				want := uint8(255)
				if cap == gomath.CapButt {
					want = 0
				}
				if got := m.AlphaAt(p.X, p.Y).A; got != want {
					t.Fatalf("cap=%v reversed=%v at %v: coverage=%d want=%d", cap, reverse, p, got, want)
				}
			}
			for _, p := range []image.Point{{25, 57}, {86, 57}} {
				want := uint8(0)
				if cap == gomath.CapSquare {
					want = 255
				}
				if got := m.AlphaAt(p.X, p.Y).A; got != want {
					t.Fatalf("cap=%v at corner %v: coverage=%d want=%d", cap, p, got, want)
				}
			}
		}
	}
}

func TestStrokeDiagonalSquareCap(t *testing.T) {
	for _, cap := range []gomath.StrokeCap{gomath.CapButt, gomath.CapRound, gomath.CapSquare} {
		s := gomath.DefaultStyle()
		s.StrokeCap = cap
		m := strokeMask(t, [][2]float64{{40, 40}, {80, 80}}, false, 8, s)
		for _, p := range []image.Point{{31, 40}, {88, 80}} {
			want := uint8(0)
			if cap == gomath.CapSquare {
				want = 255
			}
			if got := m.AlphaAt(p.X, p.Y).A; got != want {
				t.Fatalf("diagonal cap=%v at %v: coverage=%d want=%d", cap, p, got, want)
			}
		}
	}
}

func TestStrokeJoinCornerCoverage(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		for _, mirror := range []bool{false, true} {
			coords := [][2]float64{{32, 80}, {64, 80}, {64, 40}}
			if mirror {
				for i := range coords {
					coords[i][0] = 128 - coords[i][0]
				}
			}
			if reverse {
				coords[0], coords[2] = coords[2], coords[0]
			}
			for _, join := range []gomath.StrokeJoin{gomath.JoinMiter, gomath.JoinRound, gomath.JoinBevel} {
				s := gomath.DefaultStyle()
				s.StrokeJoin = join
				m := strokeMask(t, coords, false, 12, s)
				for _, sample := range []struct {
					x, y                int
					miter, round, bevel uint8
				}{
					{74, 90, 255, 0, 0},     // square miter tip, outside the radius
					{72, 87, 255, 255, 0},   // round arc beyond the bevel chord
					{67, 83, 255, 255, 255}, // all joins contain the corner interior
					{48, 60, 0, 0, 0},       // no accidental fill inside the elbow
				} {
					x := sample.x
					if mirror {
						x = 127 - x
					}
					want := map[gomath.StrokeJoin]uint8{gomath.JoinMiter: sample.miter, gomath.JoinRound: sample.round, gomath.JoinBevel: sample.bevel}[join]
					got := m.AlphaAt(x, sample.y).A
					if math.Abs(float64(got)-float64(want)) > 8 {
						t.Fatalf("join=%v reverse=%v mirror=%v at (%d,%d): coverage=%d want=%d", join, reverse, mirror, x, sample.y, got, want)
					}
				}
			}
		}
	}
}

func TestStrokeMiterLimitAndClosedJoins(t *testing.T) {
	coords := [][2]float64{{32, 80}, {64, 80}, {64, 40}}
	s := gomath.DefaultStyle()
	s.MiterLimit = 1.4 // Below sqrt(2): a right-angle miter must become a bevel.
	limited := strokeMask(t, coords, false, 12, s)
	s.StrokeJoin = gomath.JoinBevel
	bevel := strokeMask(t, coords, false, 12, s)
	if limited.AlphaAt(74, 90).A != 0 || limited.AlphaAt(67, 83).A < 250 {
		t.Fatal("miter limit did not clip the corner")
	}
	if !bytes.Equal(limited.Pix, bevel.Pix) {
		t.Fatal("limited miter differs from bevel, including inner coverage")
	}
	s.StrokeJoin, s.MiterLimit = gomath.JoinMiter, 1.5
	if strokeMask(t, coords, false, 12, s).AlphaAt(74, 90).A < 250 {
		t.Fatal("miter below limit was beveled")
	}
	closed := [][2]float64{{32, 96}, {96, 96}, {96, 32}, {32, 32}, {32, 96}}
	for _, join := range []gomath.StrokeJoin{gomath.JoinMiter, gomath.JoinRound, gomath.JoinBevel} {
		s.StrokeJoin = join
		var prior []byte
		for _, cap := range []gomath.StrokeCap{gomath.CapRound, gomath.CapButt, gomath.CapSquare} {
			s.StrokeCap = cap
			m := strokeMask(t, closed, true, 12, s)
			if m.AlphaAt(64, 64).A != 0 || m.AlphaAt(95, 64).A < 250 {
				t.Fatal("closed stroke lost hole or edge")
			}
			if prior != nil && !bytes.Equal(prior, m.Pix) {
				t.Fatal("closed contour affected by cap style")
			}
			prior = append([]byte(nil), m.Pix...)
		}
	}
}

func TestStrokeDuplicateAndZeroLengthPoints(t *testing.T) {
	for _, cap := range []gomath.StrokeCap{gomath.CapButt, gomath.CapRound, gomath.CapSquare} {
		s := gomath.DefaultStyle()
		s.StrokeCap = cap
		plain := strokeMask(t, [][2]float64{{32, 64}, {80, 64}}, false, 8, s)
		duplicate := strokeMask(t, [][2]float64{{32, 64}, {32, 64}, {80, 64}, {80, 64}}, false, 8, s)
		if !bytes.Equal(plain.Pix, duplicate.Pix) {
			t.Fatal("duplicate point changed coverage")
		}
		zero := strokeMask(t, [][2]float64{{64, 64}, {64, 64}}, false, 8, s)
		want := uint8(255)
		if cap == gomath.CapButt {
			want = 0
		}
		if zero.AlphaAt(64, 64).A != want {
			t.Fatalf("zero-length cap %v coverage wrong", cap)
		}
	}
}

func TestStrokeRejectsInvalidStyle(t *testing.T) {
	r, _ := NewRasterizer(DefaultConfig())
	z := vector.NewRasterizer(128, 128)
	line := path.Polyline{Points: []geom.Vec3{geom.V(0, 0), geom.V(1, 0)}}
	s := gomath.DefaultStyle()
	s.StrokeCap = 200
	if r.strokeStyledLine(z, line, 2, s) == nil {
		t.Fatal("unknown cap accepted")
	}
	s = gomath.DefaultStyle()
	s.StrokeJoin = 200
	if r.strokeStyledLine(z, line, 2, s) == nil {
		t.Fatal("unknown join accepted")
	}
	s = gomath.DefaultStyle()
	for _, radius := range []float64{-1, math.NaN(), math.Inf(1)} {
		if r.strokeStyledLine(z, line, radius, s) == nil {
			t.Fatal("invalid radius accepted", radius)
		}
	}
}

func TestStrokeSharpMiterUsesConfiguredLimit(t *testing.T) {
	// This tip has a miter ratio sqrt(26), greater than the old fixed limit.
	coords := [][2]float64{{48, 104}, {64, 24}, {80, 104}}
	for _, limit := range []float64{0, 4, 5, 5.2, 8} {
		s := gomath.DefaultStyle()
		s.MiterLimit = limit
		m := strokeMask(t, coords, false, 4, s)
		got := m.AlphaAt(63, 15).A
		if limit < math.Sqrt(26) && got != 0 {
			t.Fatalf("miter limit %g did not bevel the sharp tip: %d", limit, got)
		}
		if limit > math.Sqrt(26) && got < 250 {
			t.Fatalf("configured miter limit %g did not extend the tip: %d", limit, got)
		}
	}
	// An extremely sharp but nonzero angle must not hit an implicit fixed
	// numerical limit. Use outline coordinates to inspect its faraway tip.
	s := gomath.DefaultStyle()
	s.MiterLimit = 100000
	points := [][2]float64{{63.999, 104}, {64, 24}, {64.001, 104}}
	outline := offsetStyledSide(points, 1, -1, false, s)
	if len(outline) != 3 || outline[1][1] > -79000 {
		t.Fatal("large configured miter limit silently clipped", outline)
	}
}

func TestStrokeShortTurnHasNoUnboundedInnerSpike(t *testing.T) {
	for _, join := range []gomath.StrokeJoin{gomath.JoinRound, gomath.JoinBevel} {
		s := gomath.DefaultStyle()
		s.StrokeCap, s.StrokeJoin = gomath.CapButt, join
		m := strokeMask(t, [][2]float64{{60, 64}, {64, 64}, {60, 65}}, false, 10, s)
		for y := range 128 {
			for x := range 128 {
				if (x < 49 || x > 75 || y < 53 || y > 76) && m.AlphaAt(x, y).A != 0 {
					t.Fatalf("join %v made an inner spike at (%d,%d)", join, x, y)
				}
			}
		}
	}
}

func TestRoundStrokeMatchesDistanceToSegments(t *testing.T) {
	// A polyline with round caps and joins is the union of radius-wide
	// capsules. Check against that independent geometric definition, away
	// from the one-pixel antialias fringe, including overlapping short turns.
	for _, coords := range [][][2]float64{
		{{24, 80}, {64, 80}, {64, 32}},
		{{24, 96}, {64, 24}, {104, 96}},
		{{60, 64}, {64, 64}, {60, 65}},
		{{24, 64}, {96, 64}, {24, 64}},
		{{24, 24}, {104, 104}, {104, 24}, {24, 104}},
	} {
		s := gomath.DefaultStyle()
		s.StrokeJoin = gomath.JoinRound
		m := strokeMask(t, coords, false, 8, s)
		for y := range 128 {
			for x := range 128 {
				distance := math.Inf(1)
				px, py := float64(x)+.5, float64(y)+.5
				for i := 1; i < len(coords); i++ {
					a, b := coords[i-1], coords[i]
					dx, dy := b[0]-a[0], b[1]-a[1]
					u := math.Max(0, math.Min(1, ((px-a[0])*dx+(py-a[1])*dy)/(dx*dx+dy*dy)))
					distance = math.Min(distance, math.Hypot(px-a[0]-u*dx, py-a[1]-u*dy))
				}
				coverage := m.AlphaAt(x, y).A
				if (distance < 7 && coverage < 250) || (distance > 9 && coverage != 0) {
					t.Fatalf("round stroke %v at (%d,%d): distance=%g coverage=%d", coords, x, y, distance, coverage)
				}
			}
		}
	}
}

func TestImportedSVGStrokeAffectsRenderedCoverage(t *testing.T) {
	for _, cap := range []gomath.StrokeCap{gomath.CapButt, gomath.CapRound, gomath.CapSquare} {
		for _, join := range []gomath.StrokeJoin{gomath.JoinMiter, gomath.JoinRound, gomath.JoinBevel} {
			capName := map[gomath.StrokeCap]string{gomath.CapButt: "butt", gomath.CapRound: "round", gomath.CapSquare: "square"}[cap]
			joinName := map[gomath.StrokeJoin]string{gomath.JoinMiter: "miter", gomath.JoinRound: "round", gomath.JoinBevel: "bevel"}[join]
			doc := `<svg viewBox="0 0 128 128"><g fill="none" stroke="white" stroke-width="24" stroke-linecap="` + capName + `" stroke-linejoin="` + joinName + `" stroke-miterlimit="6"><path id="stroke" d="M32 80L64 80L64 40"/></g></svg>`
			root, err := svg.Read(strings.NewReader(doc), svg.Options{Width: 128})
			if err != nil {
				t.Fatal(err)
			}
			var o *gomath.Object
			root.Walk(func(candidate *gomath.Object) {
				if candidate.Name == "stroke" {
					o = candidate
				}
			})
			if o == nil {
				t.Fatal("SVG stroke object missing")
			}
			c := DefaultConfig()
			c.Width, c.Height, c.FrameWidth = 128, 128, 128
			r, _ := NewRasterizer(c)
			z := vector.NewRasterizer(128, 128)
			for _, line := range o.Path.Flatten(.02) {
				if err := r.strokeStyledLine(z, line, o.Style.StrokeWidth/2, o.Style); err != nil {
					t.Fatal(err)
				}
			}
			actual := image.NewAlpha(image.Rect(0, 0, 128, 128))
			z.Draw(actual, actual.Bounds(), image.NewUniform(o.Style.Stroke.NRGBA(1)), image.Point{})
			s := gomath.DefaultStyle()
			s.StrokeCap, s.StrokeJoin, s.MiterLimit = cap, join, 6
			expected := strokeMask(t, [][2]float64{{32, 80}, {64, 80}, {64, 40}}, false, 12, s)
			if !bytes.Equal(actual.Pix, expected.Pix) {
				t.Fatalf("SVG import changed rendered stroke: cap=%v join=%v", cap, join)
			}
		}
	}
}
