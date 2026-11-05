package export_test

import (
	"gomath"
	"gomath/export"
	"gomath/svg"
	"math"
	"strconv"
	"strings"
	"testing"
)

func TestStrokeAttributesRoundTrip(t *testing.T) {
	for _, cap := range []gomath.StrokeCap{gomath.CapRound, gomath.CapButt, gomath.CapSquare} {
		for _, join := range []gomath.StrokeJoin{gomath.JoinMiter, gomath.JoinRound, gomath.JoinBevel} {
			for _, limit := range []float64{0, 1, 1.5, 9.123456789012345} {
				s := gomath.DefaultStyle()
				s.StrokeCap, s.StrokeJoin, s.MiterLimit = cap, join, limit
				attrs, err := export.StrokeAttributes(s)
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(attrs, `stroke-miterlimit="`+strconv.FormatFloat(s.EffectiveMiterLimit(), 'g', -1, 64)+`"`) {
					t.Fatal("miter precision or default lost", attrs)
				}
				// Import a real visible object, not just a standalone attribute
				// parser: these fields must reach the object used by rendering.
				doc := `<svg viewBox="0 0 128 128"><path id="stroke" d="M32 80L64 80L64 40" fill="none" stroke="white" stroke-width="24" ` + attrs + `/></svg>`
				root, err := svg.Read(strings.NewReader(doc), svg.Options{Width: 128})
				if err != nil {
					t.Fatal(err)
				}
				var got *gomath.Object
				root.Walk(func(o *gomath.Object) {
					if o.Name == "stroke" {
						got = o
					}
				})
				if got == nil || len(got.Path.Contours) != 1 || got.Style.Stroke.A != 1 || got.Style.StrokeWidth != 24 || got.Style.StrokeCap != cap || got.Style.StrokeJoin != join || got.Style.EffectiveMiterLimit() != s.EffectiveMiterLimit() {
					t.Fatalf("stroke round trip failed: cap=%v join=%v limit=%v object=%+v", cap, join, limit, got)
				}
			}
		}
	}
}

func TestStrokeAttributesRejectInvalid(t *testing.T) {
	for _, mutate := range []func(*gomath.Style){
		func(s *gomath.Style) { s.StrokeCap = 255 },
		func(s *gomath.Style) { s.StrokeJoin = 255 },
		func(s *gomath.Style) { s.MiterLimit = .5 },
		func(s *gomath.Style) { s.MiterLimit = math.NaN() },
		func(s *gomath.Style) { s.MiterLimit = math.Inf(1) },
		func(s *gomath.Style) { s.StrokeWidth = -1 },
	} {
		s := gomath.DefaultStyle()
		mutate(&s)
		attrs, err := export.StrokeAttributes(s)
		if err == nil || attrs != "" {
			t.Fatalf("invalid style exported: %+v -> %q, %v", s, attrs, err)
		}
	}
}
