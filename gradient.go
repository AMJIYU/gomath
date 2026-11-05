package gomath

import (
	"fmt"
	"gomath/geom"
	"math"
	"sort"
)

const MaxGradientStops = 256
const MaxGradientCoordinate = 1e9

type GradientKind uint8

const (
	LinearGradient GradientKind = iota + 1
	RadialGradient
)

// GradientStop uses straight sRGB and alpha. Offsets are in [0,1]. Equal
// offsets form a hard stop; at the boundary the last stop wins.
type GradientStop struct {
	Offset float64
	Color  Color
}

// Gradient is an immutable, pad-extended fill in world coordinates. The radial
// subset has coincident center/focus and zero inner radius. Its affine frame
// maps a unit x ramp or unit circle to world space. Construct with New*Gradient;
// the zero value is invalid. Objects and snapshots may safely share it.
type Gradient struct {
	kind    GradientKind
	matrix  [6]float64 // SVG order: a,b,c,d,e,f
	inverse [4]float64
	stops   []GradientStop
	ready   bool
	err     error
}

func NewLinearGradient(start, end geom.Vec3, stops []GradientStop) (*Gradient, error) {
	if err := gradientPoint(start); err != nil {
		return nil, err
	}
	if err := gradientPoint(end); err != nil {
		return nil, err
	}
	dx, dy := end.X-start.X, end.Y-start.Y
	return newGradient(LinearGradient, [6]float64{dx, dy, -dy, dx, start.X, start.Y}, stops)
}
func NewRadialGradient(center geom.Vec3, radius float64, stops []GradientStop) (*Gradient, error) {
	if err := gradientPoint(center); err != nil {
		return nil, err
	}
	if !finite(radius) || radius <= 0 || radius > MaxGradientCoordinate {
		return nil, fmt.Errorf("gradient radius must be finite and in (0,1e9]")
	}
	return newGradient(RadialGradient, [6]float64{radius, 0, 0, radius, center.X, center.Y}, stops)
}
func newGradient(kind GradientKind, matrix [6]float64, stops []GradientStop) (*Gradient, error) {
	if len(stops) < 2 || len(stops) > MaxGradientStops {
		return nil, fmt.Errorf("gradient requires 2..%d stops", MaxGradientStops)
	}
	previous := 0.0
	for _, s := range stops {
		if !finite(s.Offset) || s.Offset < previous || s.Offset > 1 || !s.Color.valid() {
			return nil, fmt.Errorf("gradient stops must have finite nondecreasing offsets in [0,1] and valid colors")
		}
		previous = s.Offset
	}
	if stops[0].Offset != 0 || stops[len(stops)-1].Offset != 1 {
		return nil, fmt.Errorf("gradient stops must include offsets 0 and 1")
	}
	g := &Gradient{kind: kind, matrix: matrix, stops: append([]GradientStop(nil), stops...), ready: true}
	g.err = g.prepareFrame()
	if g.err != nil {
		return nil, g.err
	}
	return g, nil
}
func gradientPoint(p geom.Vec3) error {
	if !p.Finite() || p.Z != 0 || math.Abs(p.X) > MaxGradientCoordinate || math.Abs(p.Y) > MaxGradientCoordinate {
		return fmt.Errorf("gradient points must be finite 2D coordinates within +/-1e9")
	}
	return nil
}
func (g *Gradient) prepareFrame() error {
	for _, v := range g.matrix {
		if !finite(v) || math.Abs(v) > MaxGradientCoordinate {
			return fmt.Errorf("gradient affine coefficients exceed +/-1e9")
		}
	}
	a, b, c, d := g.matrix[0], g.matrix[1], g.matrix[2], g.matrix[3]
	norm := math.Max(math.Max(math.Abs(a), math.Abs(b)), math.Max(math.Abs(c), math.Abs(d)))
	det := a*d - b*c
	if norm == 0 || math.Abs(det)/(norm*norm) < 1e-12 {
		return fmt.Errorf("gradient frame is singular or ill-conditioned")
	}
	g.inverse = [4]float64{d / det, -b / det, -c / det, a / det}
	for _, v := range g.inverse {
		if !finite(v) || math.Abs(v) > MaxGradientCoordinate {
			return fmt.Errorf("gradient inverse exceeds +/-1e9")
		}
	}
	return nil
}
func (g *Gradient) Validate() error {
	if g == nil {
		return nil
	}
	if !g.ready {
		return fmt.Errorf("uninitialized gradient; use NewLinearGradient or NewRadialGradient")
	}
	return g.err
}
func (g *Gradient) Kind() GradientKind {
	if g == nil {
		return 0
	}
	return g.kind
}
func (g *Gradient) Matrix() [6]float64 {
	if g == nil {
		return [6]float64{}
	}
	return g.matrix
}
func (g *Gradient) Stops() []GradientStop {
	if g == nil {
		return nil
	}
	return append([]GradientStop(nil), g.stops...)
}

// ColorAt evaluates at a world point. Alpha and encoded sRGB channels interpolate
// separately, before coverage/opacity compositing; this is not linear-light RGB.
func (g *Gradient) ColorAt(point geom.Vec3) (Color, error) {
	if g == nil {
		return Color{}, fmt.Errorf("nil gradient")
	}
	if err := g.Validate(); err != nil {
		return Color{}, err
	}
	if err := gradientPoint(point); err != nil {
		return Color{}, err
	}
	x, y := point.X-g.matrix[4], point.Y-g.matrix[5]
	u := g.inverse[0]*x + g.inverse[2]*y
	value := u
	if g.kind == RadialGradient {
		v := g.inverse[1]*x + g.inverse[3]*y
		value = math.Hypot(u, v)
	}
	if value < 0 {
		return g.stops[0].Color, nil
	}
	value = clamp(value)
	i := sort.Search(len(g.stops), func(i int) bool { return g.stops[i].Offset > value })
	if i == 0 {
		return g.stops[0].Color, nil
	}
	if i == len(g.stops) {
		return g.stops[i-1].Color, nil
	}
	a, b := g.stops[i-1], g.stops[i]
	return a.Color.Lerp(b.Color, (value-a.Offset)/(b.Offset-a.Offset)), nil
}

// Transform returns a new paint with its affine frame transformed alongside
// geometry. Perspective, z mixing, singular and excessive transforms error.
func (g *Gradient) Transform(m geom.Mat4) (*Gradient, error) {
	if g == nil {
		return nil, nil
	}
	if err := g.Validate(); err != nil {
		return nil, err
	}
	for _, v := range m {
		if !finite(v) {
			return nil, fmt.Errorf("gradient transform must be finite")
		}
	}
	if m[2] != 0 || m[6] != 0 || m[8] != 0 || m[9] != 0 || m[11] != 0 || m[12] != 0 || m[13] != 0 || m[14] != 0 || m[15] != 1 {
		return nil, fmt.Errorf("gradient transform requires a 2D affine matrix")
	}
	q := *g
	v := g.matrix
	q.matrix = [6]float64{m[0]*v[0] + m[1]*v[1], m[4]*v[0] + m[5]*v[1], m[0]*v[2] + m[1]*v[3], m[4]*v[2] + m[5]*v[3], m[0]*v[4] + m[1]*v[5] + m[3], m[4]*v[4] + m[5]*v[5] + m[7]}
	q.err = q.prepareFrame()
	if q.err != nil {
		return nil, q.err
	}
	return &q, nil
}
func (g *Gradient) transformed(m geom.Mat4) *Gradient {
	q, err := g.Transform(m)
	if err != nil {
		return &Gradient{ready: true, err: err}
	}
	return q
}

// SetFillGradient selects the paint for this node and descendants. Nil exposes
// the stored solid Fill again; SetFill and SetColor explicitly clear gradients.
// As with SetClip, malformed paints are rejected by scene/render validation.
func (o *Object) SetFillGradient(g *Gradient) *Object {
	o.Walk(func(v *Object) { v.Style.FillGradient = g })
	return o
}

func gradientPaintCompatible(a, b *Gradient) error {
	if a == b {
		return nil
	}
	if a == nil || b == nil {
		return fmt.Errorf("gradient presence changes require separate FadeOut/FadeIn layers")
	}
	if err := a.Validate(); err != nil {
		return err
	}
	if err := b.Validate(); err != nil {
		return err
	}
	if a.kind != b.kind || len(a.stops) != len(b.stops) {
		return fmt.Errorf("gradient type/stop changes are unsupported in Transform")
	}
	for i, s := range a.stops {
		if s != b.stops[i] {
			return fmt.Errorf("gradient stop changes are unsupported in Transform")
		}
	}
	return nil
}

// Check the determinant quadratic over the complete interpolation interval.
// The inverse bound uses a conservative maximum coefficient / minimum determinant.
func gradientFrameCompatible(a, b *Gradient) error {
	if a == nil || a == b || a.matrix == b.matrix {
		return nil
	}
	x, y := a.matrix, b.matrix
	var d [4]float64
	limit := 0.0
	for i := range 4 {
		d[i] = y[i] - x[i]
		limit = math.Max(limit, math.Max(math.Abs(x[i]), math.Abs(y[i])))
	}
	c0 := x[0]*x[3] - x[1]*x[2]
	c1 := x[0]*d[3] + d[0]*x[3] - x[1]*d[2] - d[1]*x[2]
	c2 := d[0]*d[3] - d[1]*d[2]
	lo, hi := c0, c0+c1+c2
	if lo > hi {
		lo, hi = hi, lo
	}
	if c2 != 0 {
		t := -c1 / (2 * c2)
		if t > 0 && t < 1 {
			v := c0 + c1*t + c2*t*t
			lo = math.Min(lo, v)
			hi = math.Max(hi, v)
		}
	}
	if lo <= 0 && hi >= 0 {
		return fmt.Errorf("gradient frame interpolation becomes singular; use Rotate or separate fades")
	}
	minimum := math.Min(math.Abs(lo), math.Abs(hi))
	if minimum/(limit*limit) < 1e-12 || limit/minimum > MaxGradientCoordinate {
		return fmt.Errorf("gradient frame interpolation exceeds conditioning/inverse budget")
	}
	return nil
}
func interpolateGradient(a, b *Gradient, t float64) *Gradient {
	if a == b || a == nil {
		return a
	}
	if t == 0 {
		return a
	}
	if t == 1 {
		return b
	}
	q := *a
	for i := range q.matrix {
		q.matrix[i] = a.matrix[i]*(1-t) + b.matrix[i]*t
	}
	q.err = q.prepareFrame()
	return &q
}
func validateGradientFrames(a, b *Object) error {
	if err := gradientFrameCompatible(a.Style.FillGradient, b.Style.FillGradient); err != nil {
		return err
	}
	for i := range a.Children {
		if err := validateGradientFrames(a.Children[i], b.Children[i]); err != nil {
			return err
		}
	}
	return nil
}

func (o *Object) validateGradientGeometry() error {
	if o.Style.FillGradient == nil {
		return nil
	}
	for _, c := range o.Path.Contours {
		if err := gradientPoint(c.Start); err != nil {
			return err
		}
		for _, b := range c.Curves {
			for _, p := range []geom.Vec3{b.P0, b.P1, b.P2, b.P3} {
				if err := gradientPoint(p); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
