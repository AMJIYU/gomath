package gomath

// Shape-matcher behavior is compared with Manim CE's shape_matchers.py (MIT).
// Upstream license notices are preserved in LICENSES/manim-LICENSE.community.txt.

import (
	"fmt"
	"gomath/geom"
)

const (
	MaxAnnotationObjects  = 4096
	MaxAnnotationDepth    = 64
	MaxAnnotationContours = 16384
	MaxAnnotationCurves   = 65536
)

// SurroundingRectangleOptions uses world-unit padding on each side of the
// target's geometric XY bounds. Zero padding is meaningful. Bounds exclude
// stroke and clipping and include the complete authored paths, even if hidden.
type SurroundingRectangleOptions struct {
	PaddingX, PaddingY, CornerRadius float64
}

func DefaultSurroundingRectangleOptions() SurroundingRectangleOptions {
	return SurroundingRectangleOptions{PaddingX: .1, PaddingY: .1}
}

// NewSurroundingRectangle returns independent XY geometry at the bounds' center
// Z. It reads a bounded valid object tree once; no updater or target reference
// is retained. The initial stroke is pure yellow; ordinary styling is allowed.
func NewSurroundingRectangle(target *Object, o SurroundingRectangleOptions) (*Object, error) {
	if !finite(o.PaddingX) || !finite(o.PaddingY) || o.PaddingX < 0 || o.PaddingY < 0 || o.PaddingX > MaxPrimitiveCoordinate || o.PaddingY > MaxPrimitiveCoordinate {
		return nil, fmt.Errorf("surrounding rectangle requires bounded nonnegative padding")
	}
	b, err := annotationBounds(target)
	if err != nil {
		return nil, err
	}
	size := b.Size()
	width, height := size.X+2*o.PaddingX, size.Y+2*o.PaddingY
	object, err := RoundedRectangle(width, height, o.CornerRadius)
	if err != nil {
		return nil, err
	}
	object.MoveTo(b.Center()).SetStroke(RGB(1, 1, 0), DefaultStyle().StrokeWidth)
	object.ZIndex = target.ZIndex
	if err = annotationOutput(object, true); err != nil {
		return nil, err
	}
	return object, nil
}

type BackgroundRectangleOptions struct {
	Rectangle SurroundingRectangleOptions
	Fill      Color
}

func DefaultBackgroundRectangleOptions() BackgroundRectangleOptions {
	return BackgroundRectangleOptions{Fill: Color{A: .75}}
}

// NewBackgroundRectangle owns an ordinary filled rectangle. Add it before the
// target at the same ZIndex, or explicitly change its ZIndex. It does not alter
// scene order, force black during later styling, or replace Create's normal
// Reveal semantics with Manim BackgroundRectangle's special opacity behavior.
func NewBackgroundRectangle(target *Object, o BackgroundRectangleOptions) (*Object, error) {
	if !o.Fill.valid() {
		return nil, fmt.Errorf("background rectangle requires a valid fill color")
	}
	object, err := NewSurroundingRectangle(target, o.Rectangle)
	if err != nil {
		return nil, err
	}
	return object.SetFill(o.Fill).SetStroke(Transparent, 0), nil
}

type UnderlineOptions struct{ Buffer float64 }

func DefaultUnderlineOptions() UnderlineOptions { return UnderlineOptions{Buffer: .1} }

// NewUnderline spans the full geometric XY width at minY-Buffer. Target stroke,
// clip, visibility and Reveal do not shrink the authored geometric bounds.
func NewUnderline(target *Object, o UnderlineOptions) (*Object, error) {
	if !finite(o.Buffer) || o.Buffer < 0 || o.Buffer > MaxPrimitiveCoordinate {
		return nil, fmt.Errorf("underline requires bounded nonnegative buffer")
	}
	b, err := annotationBounds(target)
	if err != nil {
		return nil, err
	}
	if !primitiveDimension(b.Size().X) {
		return nil, fmt.Errorf("underline requires a bounded positive target width")
	}
	y, z := b.Min.Y-o.Buffer, b.Center().Z
	object := Line(geom.Vec3{X: b.Min.X, Y: y, Z: z}, geom.Vec3{X: b.Max.X, Y: y, Z: z})
	object.ZIndex = target.ZIndex
	if err = annotationOutput(object, false); err != nil {
		return nil, err
	}
	return object, nil
}

type CrossOptions struct{ Scale float64 }

func DefaultCrossOptions() CrossOptions { return CrossOptions{Scale: 1} }

// NewCross fits two diagonal lines to the target XY bounds, then scales both
// about its geometric center. Scale is explicit and positive; defaults use 1.
// The returned group owns both editable lines, initially red with width .06.
func NewCross(target *Object, o CrossOptions) (*Object, error) {
	if !primitiveDimension(o.Scale) {
		return nil, fmt.Errorf("cross requires a bounded positive scale")
	}
	b, err := annotationBounds(target)
	if err != nil {
		return nil, err
	}
	width, height := b.Size().X*o.Scale, b.Size().Y*o.Scale
	if !primitiveDimension(width) || !primitiveDimension(height) {
		return nil, fmt.Errorf("cross dimensions exceed bounds or are degenerate")
	}
	c := b.Center()
	point := func(x, y float64) geom.Vec3 { return c.Add(geom.V(x*width/2, y*height/2)) }
	object := Group(Line(point(-1, 1), point(1, -1)), Line(point(1, 1), point(-1, -1))).SetStroke(Red, .06)
	object.ZIndex = target.ZIndex
	if err = annotationOutput(object, true); err != nil {
		return nil, err
	}
	return object, nil
}

// Preflight precedes recursive validation/bounds. Resource limits cover the
// complete tree, including empty groups/contours, before large scratch growth.
func annotationBounds(target *Object) (geom.Bounds, error) {
	type node struct {
		object *Object
		depth  int
	}
	stack := []node{{target, 1}}
	var objects, contours, curves int
	seen := make(map[*Object]bool)
	for len(stack) > 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		o := current.object
		if o == nil || seen[o] {
			return geom.Bounds{}, fmt.Errorf("annotation target is nil, shared or cyclic")
		}
		objects++
		if objects > MaxAnnotationObjects || current.depth > MaxAnnotationDepth || len(o.Children) > MaxAnnotationObjects-objects-len(stack) {
			return geom.Bounds{}, fmt.Errorf("annotation object/depth budget exceeded")
		}
		seen[o] = true
		if len(o.Path.Contours) > MaxAnnotationContours-contours {
			return geom.Bounds{}, fmt.Errorf("annotation contour budget exceeded")
		}
		contours += len(o.Path.Contours)
		for _, contour := range o.Path.Contours {
			if len(contour.Curves) > MaxAnnotationCurves-curves {
				return geom.Bounds{}, fmt.Errorf("annotation curve budget exceeded")
			}
			curves += len(contour.Curves)
			if !boundedPrimitivePoint(contour.Start) {
				return geom.Bounds{}, fmt.Errorf("annotation source coordinates exceed bounds")
			}
			for _, curve := range contour.Curves {
				for _, point := range [4]geom.Vec3{curve.P0, curve.P1, curve.P2, curve.P3} {
					if !boundedPrimitivePoint(point) {
						return geom.Bounds{}, fmt.Errorf("annotation source controls exceed bounds")
					}
				}
			}
		}
		for _, child := range o.Children {
			stack = append(stack, node{child, current.depth + 1})
		}
	}
	if err := target.Validate(); err != nil {
		return geom.Bounds{}, err
	}
	b := target.Bounds()
	if !b.Valid || !boundedPrimitivePoint(b.Min) || !boundedPrimitivePoint(b.Max) {
		return geom.Bounds{}, fmt.Errorf("annotation target has no bounded geometry")
	}
	return b, nil
}

func annotationOutput(object *Object, twoDimensions bool) error {
	b, err := annotationBounds(object)
	if err != nil {
		return err
	}
	size := b.Size()
	if !primitiveDimension(size.X) || twoDimensions && !primitiveDimension(size.Y) {
		return fmt.Errorf("annotation geometry collapsed or exceeds dimension bounds")
	}
	return nil
}
