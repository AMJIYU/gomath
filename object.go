package gomath

import (
	"fmt"
	"gomath/geom"
	"gomath/path"
	"math"
	"sync/atomic"
)

var objectSequence atomic.Uint64

// Object owns a path and optional children. Do not form cycles, share a child
// between roots, or mutate a scene while it is being sampled or rendered.
type Object struct {
	ID     uint64
	Name   string
	Path   path.Path
	Bitmap *Bitmap
	Clip   *Clip
	// Scalar is a finite animatable numerical value. ValueTracker uses this
	// slot; vector and image objects normally leave it at zero.
	Scalar   float64
	Style    Style
	ZIndex   int
	Children []*Object
}

func NewObject(p path.Path) *Object {
	return &Object{ID: objectSequence.Add(1), Path: p.Clone(), Style: DefaultStyle()}
}
func Group(objects ...*Object) *Object {
	o := NewObject(path.Path{})
	o.Children = append([]*Object(nil), objects...)
	return o
}

// Validate checks geometry, styles, identities and tree ownership without
// mutating the object. It detects cycles before any recursive layout or copy.
func (o *Object) Validate() error { return validateObjects([]*Object{o}) }
func (o *Object) snapshot() *Object {
	if o == nil {
		return nil
	}
	q := *o
	q.Path = o.Path.Clone()
	q.Children = make([]*Object, len(o.Children))
	for i, c := range o.Children {
		q.Children[i] = c.snapshot()
	}
	return &q
}

// Copy deep-copies geometry and assigns new identities, suitable for adding both
// the original and copy to the same scene.
func (o *Object) Copy() *Object {
	q := o.snapshot()
	q.Walk(func(c *Object) { c.ID = objectSequence.Add(1) })
	return q
}
func (o *Object) Walk(fn func(*Object)) {
	if o == nil {
		return
	}
	fn(o)
	for _, c := range o.Children {
		c.Walk(fn)
	}
}
func (o *Object) Bounds() geom.Bounds {
	var b geom.Bounds
	o.Walk(func(c *Object) {
		p := c.Path.Bounds()
		if p.Valid {
			b.Include(p.Min)
			b.Include(p.Max)
		}
	})
	return b
}
func (o *Object) Center() geom.Vec3 { return o.Bounds().Center() }
func (o *Object) ApplyMatrix(m geom.Mat4) *Object {
	o.Walk(func(c *Object) {
		c.Path = c.Path.Transform(m)
		if c.Style.FillGradient != nil {
			c.Style.FillGradient = c.Style.FillGradient.transformed(m)
		}
		if c.Clip != nil {
			c.Clip = c.Clip.transformed(m)
		}
	})
	return o
}
func (o *Object) Shift(v geom.Vec3) *Object  { return o.ApplyMatrix(geom.Translation(v)) }
func (o *Object) MoveTo(p geom.Vec3) *Object { return o.Shift(p.Sub(o.Center())) }
func (o *Object) Scale(factor float64) *Object {
	return o.ApplyMatrix(geom.About(o.Center(), geom.Scaling(factor, factor, factor)))
}
func (o *Object) Rotate(angle float64) *Object {
	return o.ApplyMatrix(geom.About(o.Center(), geom.RotationZ(angle)))
}
func (o *Object) SetFill(c Color) *Object {
	o.Walk(func(v *Object) { v.Style.Fill = c; v.Style.FillGradient = nil })
	return o
}
func (o *Object) SetStroke(c Color, width float64) *Object {
	o.Walk(func(v *Object) { v.Style.Stroke = c; v.Style.StrokeWidth = width })
	return o
}

// SetOpacity applies to this group as a whole; descendants retain their opacity.
func (o *Object) SetOpacity(a float64) *Object { o.Style.Opacity = a; return o }
func (o *Object) SetColor(c Color) *Object {
	o.Walk(func(v *Object) {
		if v.Style.Fill.A > 0 || v.Style.FillGradient != nil {
			v.Style.FillGradient = nil
			v.Style.Fill = c
		}
		if v.Style.Stroke.A > 0 {
			v.Style.Stroke = c
		}
	})
	return o
}
func (o *Object) NextTo(other *Object, direction geom.Vec3, buff float64) *Object {
	u := direction.Unit()
	a, b := o.Bounds(), other.Bounds()
	extent := func(v geom.Vec3) float64 { return math.Abs(u.X)*v.X/2 + math.Abs(u.Y)*v.Y/2 + math.Abs(u.Z)*v.Z/2 }
	return o.MoveTo(b.Center().Add(u.Mul(extent(a.Size()) + extent(b.Size()) + buff)))
}
func (o *Object) Arrange(direction geom.Vec3, buff float64) *Object {
	if len(o.Children) < 2 {
		return o
	}
	center := o.Center()
	for i := 1; i < len(o.Children); i++ {
		o.Children[i].NextTo(o.Children[i-1], direction, buff)
	}
	return o.MoveTo(center)
}
func validateObjects(roots []*Object) error {
	seen := map[*Object]bool{}
	ids := map[uint64]bool{}
	var visit func(*Object) error
	visit = func(o *Object) error {
		if o == nil {
			return fmt.Errorf("nil object")
		}
		if seen[o] {
			return fmt.Errorf("object %d appears more than once or forms a cycle", o.ID)
		}
		seen[o] = true
		if o.ID == 0 || ids[o.ID] {
			return fmt.Errorf("invalid or duplicate object ID %d", o.ID)
		}
		ids[o.ID] = true
		if e := o.ValidateVisual(); e != nil {
			return fmt.Errorf("object %d: %w", o.ID, e)
		}
		for _, c := range o.Children {
			if e := visit(c); e != nil {
				return e
			}
		}
		return nil
	}
	for _, o := range roots {
		if e := visit(o); e != nil {
			return e
		}
	}
	return nil
}
func finite(f float64) bool { return !math.IsNaN(f) && !math.IsInf(f, 0) }
func snapshots(objects []*Object) []*Object {
	out := make([]*Object, len(objects))
	for i, o := range objects {
		out[i] = o.snapshot()
	}
	return out
}
