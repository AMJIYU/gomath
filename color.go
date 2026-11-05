package gomath

import (
	"fmt"
	"gomath/path"
	"image/color"
	"math"
	"strconv"
	"strings"
)

// Color is straight (not premultiplied) sRGB in [0,1].
type Color struct{ R, G, B, A float64 }

var (
	White       = RGB(1, 1, 1)
	Black       = RGB(0, 0, 0)
	Blue        = RGB(.345, .769, .867)
	Yellow      = RGB(1, .863, .294)
	Pink        = RGB(.82, .278, .741)
	Green       = RGB(.514, .757, .404)
	Red         = RGB(.988, .384, .333)
	Transparent = Color{}
)

func RGB(r, g, b float64) Color { return Color{r, g, b, 1} }
func clamp(v float64) float64   { return math.Max(0, math.Min(1, v)) }
func (c Color) NRGBA(opacity float64) color.NRGBA {
	return color.NRGBA{uint8(math.Round(clamp(c.R) * 255)), uint8(math.Round(clamp(c.G) * 255)), uint8(math.Round(clamp(c.B) * 255)), uint8(math.Round(clamp(c.A*opacity) * 255))}
}
func (c Color) Lerp(d Color, t float64) Color {
	return Color{c.R + (d.R-c.R)*t, c.G + (d.G-c.G)*t, c.B + (d.B-c.B)*t, c.A + (d.A-c.A)*t}
}
func (c Color) valid() bool {
	for _, v := range []float64{c.R, c.G, c.B, c.A} {
		if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 1 {
			return false
		}
	}
	return true
}
func Hex(value string) (Color, error) {
	s := strings.TrimPrefix(value, "#")
	if len(s) != 6 && len(s) != 8 {
		return Color{}, fmt.Errorf("color must have 6 or 8 hex digits")
	}
	n, e := strconv.ParseUint(s, 16, 32)
	if e != nil {
		return Color{}, fmt.Errorf("parse color: %w", e)
	}
	if len(s) == 6 {
		n = n<<8 | 255
	}
	return Color{float64((n>>24)&255) / 255, float64((n>>16)&255) / 255, float64((n>>8)&255) / 255, float64(n&255) / 255}, nil
}

// StrokeCap controls the endpoints of an open stroked contour.
type StrokeCap uint8

const (
	CapRound StrokeCap = iota // The zero value preserves GoMath's round caps.
	CapButt
	CapSquare
)

// StrokeJoin controls the outside of a corner between stroke segments.
type StrokeJoin uint8

const (
	JoinMiter StrokeJoin = iota
	JoinRound
	JoinBevel
)

type Style struct {
	Fill, Stroke Color
	// FillRule controls vector interiors only. Zero is NonZero; stroke outlines
	// and bitmap planes keep their existing semantics. This is a value, not shared mutable state.
	FillRule path.FillRule `json:",omitempty"`
	// FillGradient replaces Fill when non-nil. Immutable; assign a constructor result.
	FillGradient                 *Gradient `json:",omitempty"`
	StrokeWidth, Opacity, Reveal float64
	StrokeCap                    StrokeCap
	StrokeJoin                   StrokeJoin
	// MiterLimit is the maximum miter length in stroke radii. Zero means 4;
	// explicit values must be finite and at least 1. Longer joins are beveled.
	MiterLimit float64
	// Dash is immutable; nil means solid. Assign a NewDashPattern result.
	Dash *DashPattern
}

func DefaultStyle() Style {
	return Style{Fill: Transparent, Stroke: White, StrokeWidth: .035, Opacity: 1, Reveal: 1}
}

// Validate rejects styles that cannot be represented by the renderer.
func (s Style) Validate() error {
	if s.FillRule != path.NonZero && s.FillRule != path.EvenOdd {
		return fmt.Errorf("unsupported fill rule %d", s.FillRule)
	}
	if !s.Fill.valid() || !s.Stroke.valid() || !finite(s.StrokeWidth) || s.StrokeWidth < 0 || !finite(s.Opacity) || s.Opacity < 0 || s.Opacity > 1 || !finite(s.Reveal) || s.Reveal < 0 || s.Reveal > 1 {
		return fmt.Errorf("invalid color, stroke width, opacity or reveal")
	}
	if s.StrokeCap > CapSquare {
		return fmt.Errorf("unsupported stroke cap %d", s.StrokeCap)
	}
	if s.StrokeJoin > JoinBevel {
		return fmt.Errorf("unsupported stroke join %d", s.StrokeJoin)
	}
	if !finite(s.MiterLimit) || (s.MiterLimit != 0 && s.MiterLimit < 1) {
		return fmt.Errorf("miter limit must be zero (default 4) or finite and at least 1")
	}
	if err := s.FillGradient.Validate(); err != nil {
		return err
	}
	return s.Dash.Validate()
}

// EffectiveMiterLimit resolves the zero-value default. Call Validate first for
// untrusted styles, as this method does not replace validation.
func (s Style) EffectiveMiterLimit() float64 {
	if s.MiterLimit == 0 {
		return 4
	}
	return s.MiterLimit
}

// StrokeExtent is a conservative world-space distance from the path to any
// stroked pixel, excluding the rasterizer's antialias fringe. It is suitable for
// ROI padding after multiplying by the world-to-pixel scale. Validate first.
func (s Style) StrokeExtent() float64 {
	factor := 1.0
	if s.StrokeJoin == JoinMiter {
		factor = s.EffectiveMiterLimit()
	}
	if s.StrokeCap == CapSquare {
		factor = math.Max(factor, math.Sqrt2)
	}
	return s.StrokeWidth / 2 * factor
}

// Cap, join and dash patterns are discrete: keep the source until the target endpoint, as for
// ZIndex. Miter limits interpolate numerically using their effective values.
func (s Style) lerp(t Style, a float64) Style {
	if a <= 0 {
		return s
	}
	if a >= 1 {
		return t
	}
	limit := s.MiterLimit
	if s.MiterLimit != t.MiterLimit {
		limit = (1-a)*s.EffectiveMiterLimit() + a*t.EffectiveMiterLimit()
	}
	return Style{
		Fill: s.Fill.Lerp(t.Fill, a), Stroke: s.Stroke.Lerp(t.Stroke, a),
		FillRule:     s.FillRule, // Morph preparation requires identical rules.
		StrokeWidth:  s.StrokeWidth + (t.StrokeWidth-s.StrokeWidth)*a,
		Opacity:      s.Opacity + (t.Opacity-s.Opacity)*a,
		Reveal:       s.Reveal + (t.Reveal-s.Reveal)*a,
		FillGradient: interpolateGradient(s.FillGradient, t.FillGradient, a),
		StrokeCap:    s.StrokeCap, StrokeJoin: s.StrokeJoin, MiterLimit: limit, Dash: s.Dash,
	}
}
