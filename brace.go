package gomath

import (
	"fmt"
	"gomath/geom"
	"gomath/path"
	"math"
)

// BraceOptions gives a planar outward direction, physical gap and dimensionless
// sharpness. Target bounds are measured in that direction's orthogonal frame.
// Sharpness changes the horizontal stretch of the fixed MIT Manim outline,
// whose unscaled height is about .273 world units. Nothing retains the target.
type BraceOptions struct {
	Direction         geom.Vec3
	Buffer, Sharpness float64
}

func DefaultBraceOptions() BraceOptions {
	return BraceOptions{Direction: geom.V(0, -1), Buffer: .2, Sharpness: 2}
}

// DefaultBraceBetweenPointsOptions uses the segment's right-hand planar normal.
// A nonzero Direction explicitly overrides this automatic direction.
func DefaultBraceBetweenPointsOptions() BraceOptions {
	o := DefaultBraceOptions()
	o.Direction = geom.Vec3{}
	return o
}

// Brace owns an ordinary filled vector object. Tip/Direction follow edits to
// that same fixed-topology path, including ordinary Scene Transform results.
// They return errors if the object/path identity or topology is replaced.
type Brace struct {
	Object   *Object
	owner    *Object
	id       uint64
	tipCurve int
	tipPoint int
}

// NewBrace reads a bounded tree once and places a filled brace on its projected
// geometric XY bounds, excluding stroke, clip, visibility and Reveal. The output
// lies at the source bounds' center Z. Direction must be finite, nonzero and XY.
func NewBrace(target *Object, o BraceOptions) (*Brace, error) {
	if !o.Direction.Finite() || o.Direction.Z != 0 || !boundedPrimitivePoint(o.Direction) || !finite(o.Buffer) || o.Buffer < 0 || o.Buffer > MaxPrimitiveCoordinate || !finite(o.Sharpness) || o.Sharpness < 1e-9 || o.Sharpness > 1e6 {
		return nil, fmt.Errorf("brace requires a bounded nonzero XY direction, nonnegative buffer and sharpness [1e-9,1e6]")
	}
	n := math.Hypot(o.Direction.X, o.Direction.Y)
	if n == 0 {
		return nil, fmt.Errorf("brace direction is zero")
	}
	d := geom.V(o.Direction.X/n, o.Direction.Y/n)
	t := geom.V(-d.Y, d.X)
	b, err := annotationBounds(target)
	if err != nil {
		return nil, err
	}
	// Project complete cubic geometry before taking extrema. Rotating the world
	// AABB would incorrectly enlarge diagonal or curved targets.
	project := geom.Mat4{t.X, t.Y, 0, 0, -d.X, -d.Y, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}
	var local geom.Bounds
	target.Walk(func(object *Object) {
		p := object.Path.Transform(project).Bounds()
		if p.Valid {
			local.Include(p.Min)
			local.Include(p.Max)
		}
	})
	width := local.Size().X
	if !primitiveDimension(width) {
		return nil, fmt.Errorf("brace projected width must be in [1e-9,%g]", MaxPrimitiveCoordinate)
	}
	length := math.Max(0, (width*o.Sharpness-.90552)/2)
	stretch := width / (.90552 + 2*length)
	// Fixed outline cap details must survive placement rounding. This deliberately
	// conservative check does not claim all finite translated shapes are usable.
	magnitude := math.Max(math.Max(math.Abs(b.Min.X), math.Abs(b.Max.X)), math.Max(math.Abs(b.Min.Y), math.Abs(b.Max.Y))) + o.Buffer + 1
	ulp := math.Nextafter(magnitude, math.Inf(1)) - magnitude
	if .0001*math.Min(1, stretch) < 32*ulp {
		return nil, fmt.Errorf("brace detail is unrepresentable at target/buffer scale (requires 32 ULPs)")
	}
	curves := make([]path.Cubic, len(braceTemplate))
	var rawBounds geom.Bounds
	for _, raw := range braceTemplate {
		for _, i := range []int{0, 3} {
			v := raw[i]
			rawBounds.Include(geom.V(v[0]+v[2]*length, -v[1]))
		}
	}
	bottom := geom.V((rawBounds.Min.X+rawBounds.Max.X)/2, rawBounds.Min.Y)
	best := math.Inf(1)
	tipCurve, tipPoint := 0, 0
	for i, raw := range braceTemplate {
		var controls [4]geom.Vec3
		for j, v := range raw {
			x, y := v[0]+v[2]*length, -v[1]
			if distance := math.Hypot(x-bottom.X, y-bottom.Y); distance < best {
				best, tipCurve, tipPoint = distance, i, j
			}
			x = (x-rawBounds.Min.X)*stretch + local.Min.X
			y += local.Min.Y - o.Buffer - rawBounds.Max.Y
			controls[j] = geom.Vec3{X: t.X*x - d.X*y, Y: t.Y*x - d.Y*y, Z: b.Center().Z}
		}
		// The shared endpoint must be bit-identical, including when intermediate
		// affine-in-length arithmetic rounds equivalent source expressions apart.
		if i > 0 {
			controls[0] = curves[i-1].P3
		}
		curves[i] = path.Cubic{P0: controls[0], P1: controls[1], P2: controls[2], P3: controls[3]}
	}
	curves[len(curves)-1].P3 = curves[0].P0
	object := NewObject(path.Path{Contours: []path.Contour{{Start: curves[0].P0, Curves: curves, Closed: true}}})
	object.SetFill(White).SetStroke(Transparent, 0)
	object.ZIndex = target.ZIndex
	if _, err = annotationBounds(object); err != nil {
		return nil, err
	}
	return &Brace{Object: object, owner: object, id: object.ID, tipCurve: tipCurve, tipPoint: tipPoint}, nil
}

// NewBraceBetweenPoints fits the XY segment at its common Z. A zero direction
// selects (dy,-dx), so reversing endpoints places the brace on the other side.
func NewBraceBetweenPoints(start, end geom.Vec3, o BraceOptions) (*Brace, error) {
	if !boundedPrimitivePoint(start) || !boundedPrimitivePoint(end) || start.Z != end.Z {
		return nil, fmt.Errorf("brace endpoints must be bounded and share one Z plane")
	}
	if !primitiveDimension(math.Hypot(end.X-start.X, end.Y-start.Y)) {
		return nil, fmt.Errorf("brace endpoint distance must be in [1e-9,%g]", MaxPrimitiveCoordinate)
	}
	if o.Direction == (geom.Vec3{}) {
		delta := end.Sub(start)
		length := math.Hypot(delta.X, delta.Y)
		o.Direction = geom.V(delta.Y/length, -delta.X/length)
	}
	return NewBrace(Line(start, end), o)
}

func (b *Brace) validatePath() error {
	if b == nil || b.Object == nil || b.Object != b.owner || b.Object.ID != b.id || len(b.Object.Children) != 0 || b.Object.Bitmap != nil || len(b.Object.Path.Contours) != 1 || len(b.Object.Path.Contours[0].Curves) != len(braceTemplate) || !b.Object.Path.Contours[0].Closed {
		return fmt.Errorf("brace object/path topology changed")
	}
	return b.Object.Validate()
}

func (b *Brace) Tip() (geom.Vec3, error) {
	if err := b.validatePath(); err != nil {
		return geom.Vec3{}, err
	}
	c := b.Object.Path.Contours[0].Curves[b.tipCurve]
	return [4]geom.Vec3{c.P0, c.P1, c.P2, c.P3}[b.tipPoint], nil
}

// Direction follows Manim's anchor-box accessor: it returns the unit vector
// from the current cubic endpoint bounds' center to the stored outline tip.
// For oblique or skewed braces this can differ from the original outward vector
// and from using Object.Bounds(), which includes interior cubic extrema.
func (b *Brace) Direction() (geom.Vec3, error) {
	tip, err := b.Tip()
	if err != nil {
		return geom.Vec3{}, err
	}
	var anchors geom.Bounds
	for _, c := range b.Object.Path.Contours[0].Curves {
		anchors.Include(c.P0)
		anchors.Include(c.P3)
	}
	d := tip.Sub(anchors.Center())
	length := math.Hypot(math.Hypot(d.X, d.Y), d.Z)
	if !finite(length) || length == 0 {
		return geom.Vec3{}, fmt.Errorf("brace tip direction is degenerate")
	}
	return geom.Vec3{X: d.X / length, Y: d.Y / length, Z: d.Z / length}, nil
}

// LabelAtTip returns an independently owned copy of label placed next to the
// current tip. As in Manim's default put_at_tip, each direction component is
// rounded to even before selecting the label's facing bbox corner and gap.
// A diagonal therefore applies buffer on both selected axes. Label geometry
// uses full cubic extrema, not anchor-only bounds. No live attachment is kept.
func (b *Brace) LabelAtTip(label *Object, buffer float64) (*Object, error) {
	if !finite(buffer) || buffer < 0 || buffer > MaxPrimitiveCoordinate {
		return nil, fmt.Errorf("brace label requires bounded nonnegative buffer")
	}
	tip, err := b.Tip()
	if err != nil {
		return nil, err
	}
	direction, err := b.Direction()
	if err != nil {
		return nil, err
	}
	bounds, err := annotationBounds(label)
	if err != nil {
		return nil, err
	}
	critical := func(minimum, maximum, direction float64) float64 {
		if direction > 0 {
			return minimum
		}
		if direction < 0 {
			return maximum
		}
		return minimum + (maximum-minimum)/2
	}
	d := geom.Vec3{X: math.RoundToEven(direction.X), Y: math.RoundToEven(direction.Y), Z: math.RoundToEven(direction.Z)}
	anchor := geom.Vec3{X: critical(bounds.Min.X, bounds.Max.X, d.X), Y: critical(bounds.Min.Y, bounds.Max.Y, d.Y), Z: critical(bounds.Min.Z, bounds.Max.Z, d.Z)}
	delta := tip.Add(d.Mul(buffer)).Sub(anchor)
	copy := label.Copy().Shift(delta)
	if _, err = annotationBounds(copy); err != nil {
		return nil, err
	}
	return copy, nil
}
