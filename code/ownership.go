package code

import (
	"fmt"
	"math"

	"gomath"
	"gomath/geom"
)

// validate checks the fixed shallow tree before any recursive library copy.
// This also bounds author-added geometry, rejects cycles/reparenting, and
// keeps copies from silently losing the meaning of their selections.
func (c *Code) validate() error {
	if c == nil || len(c.nodes) == 0 || c.Object != c.nodes[0].object {
		return fmt.Errorf("%w: nil, uninitialized or replaced root", ErrObject)
	}
	ids := make(map[uint64]bool, len(c.nodes))
	curves := 0
	contours := 0
	for _, node := range c.nodes {
		o := node.object
		if o == nil || o.ID == 0 || ids[o.ID] {
			return fmt.Errorf("%w: invalid or duplicate identity", ErrObject)
		}
		ids[o.ID] = true
		if len(o.Children) != len(node.children) {
			return fmt.Errorf("%w: child topology changed", ErrObject)
		}
		for i, child := range node.children {
			if child != o.Children[i] {
				return fmt.Errorf("%w: child topology changed", ErrObject)
			}
		}
		if o.Bitmap != nil || o.Clip != nil {
			return fmt.Errorf("%w: code copies require unclipped vector objects", ErrObject)
		}
		if len(o.Path.Contours) > c.config.limits.Curves-contours {
			return fmt.Errorf("%w: excessive contours", ErrLimit)
		}
		contours += len(o.Path.Contours)
		for _, contour := range o.Path.Contours {
			if len(contour.Curves) > c.config.limits.Curves-curves {
				return fmt.Errorf("%w: excessive curves", ErrLimit)
			}
			curves += len(contour.Curves)
			check := func(v geom.Vec3) bool {
				return v.Finite() && math.Abs(v.X) <= MaxCoordinate && math.Abs(v.Y) <= MaxCoordinate && math.Abs(v.Z) <= MaxCoordinate
			}
			if !check(contour.Start) {
				return fmt.Errorf("%w: coordinate outside bounds", ErrLimit)
			}
			for _, curve := range contour.Curves {
				if !check(curve.P0) || !check(curve.P1) || !check(curve.P2) || !check(curve.P3) {
					return fmt.Errorf("%w: coordinate outside bounds", ErrLimit)
				}
			}
		}
		if err := o.ValidateVisual(); err != nil {
			return fmt.Errorf("%w: %v", ErrObject, err)
		}
	}
	return nil
}

// Copy validates authored geometry and deep-copies paths, selection metadata
// and the object tree with fresh identities. No mutable path or slice is shared.
func (c *Code) Copy() (*Code, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}
	return c.clone(), nil
}

func (c *Code) clone() *Code {
	q := *c
	q.Object = c.Object.Copy()
	byOld := make(map[*gomath.Object]*gomath.Object, len(c.nodes))
	i := 0
	q.nodes = nil
	q.Object.Walk(func(o *gomath.Object) {
		byOld[c.nodes[i].object] = o
		i++
		q.nodes = append(q.nodes, ownedNode{o, append([]*gomath.Object(nil), o.Children...)})
	})
	q.lines = c.Lines()
	q.tokens = c.Tokens()
	q.glyphs = c.Glyphs()
	for i := range q.glyphs {
		q.glyphs[i].Object = byOld[c.glyphs[i].Object]
	}
	remap := func(objects []*gomath.Object) []*gomath.Object {
		out := make([]*gomath.Object, len(objects))
		for i, o := range objects {
			out[i] = byOld[o]
		}
		return out
	}
	q.lineObjects = remap(c.lineObjects)
	q.numberObjects = remap(c.numberObjects)
	q.tokenObjects = make([][]*gomath.Object, len(c.tokenObjects))
	for i, objects := range c.tokenObjects {
		q.tokenObjects[i] = remap(objects)
	}
	q.background = byOld[c.background]
	return &q
}

// Snapshot has no exported mutable state. Copy is safe for concurrent calls
// after Snapshot returns, including while the original Code is being edited.
type Snapshot struct{ code *Code }

func (c *Code) Snapshot() (*Snapshot, error) {
	q, err := c.Copy()
	if err != nil {
		return nil, err
	}
	return &Snapshot{q}, nil
}

func (s *Snapshot) Copy() (*Code, error) {
	if s == nil || s.code == nil {
		return nil, fmt.Errorf("%w: nil or uninitialized snapshot", ErrObject)
	}
	return s.code.clone(), nil
}
