package gomath

import "fmt"

// ValueTracker is a non-rendering scene object whose scalar participates in
// ordinary immutable animation snapshots. Read sampled values from Frame.Value,
// not the final authoring object captured by an updater closure.
type ValueTracker struct{ *Object }

func NewValueTracker(value float64) (*ValueTracker, error) {
	if !finite(value) {
		return nil, fmt.Errorf("tracker value must be finite")
	}
	o := Group()
	o.Name, o.Scalar = "ValueTracker", value
	return &ValueTracker{Object: o}, nil
}

func (v *ValueTracker) Value() float64 { return v.Scalar }
func (v *ValueTracker) SetValue(value float64) error {
	if v == nil || v.Object == nil || !finite(value) {
		return fmt.Errorf("tracker and value must be valid and finite")
	}
	v.Scalar = value
	return nil
}

// ValueTo validates the numerical target when Play validates the animation.
// It preserves the same rate/window semantics as Transform.
func ValueTo(tracker *ValueTracker, value float64) Animation {
	if tracker == nil || tracker.Object == nil {
		return Transform(nil, nil)
	}
	target := tracker.Copy()
	target.Scalar = value
	return Transform(tracker.Object, target)
}

func (f *Frame) Value(id uint64) (float64, error) {
	if f == nil {
		return 0, fmt.Errorf("nil frame")
	}
	o := f.Find(id)
	if o == nil {
		return 0, fmt.Errorf("value object %d missing from frame", id)
	}
	if !finite(o.Scalar) {
		return 0, fmt.Errorf("value object %d is not finite", id)
	}
	return o.Scalar, nil
}

// RedrawFunc returns independent geometry for one sampled object. It must be
// deterministic and safe for simultaneous calls. The returned tree must have
// the same child topology as the original, so sampled child IDs remain stable.
type RedrawFunc func(frame *Frame, time float64) (*Object, error)

// AddRedraw updates a scene object's sampled geometry after animation tracks
// and earlier registered updaters. It never changes authoring geometry. Unlike
// general Manim always_redraw, this first implementation requires fixed topology.
func (s *Scene) AddRedraw(object *Object, draw RedrawFunc) error {
	if object == nil || draw == nil {
		return fmt.Errorf("redraw needs an object and callback")
	}
	if err := validateObjects(s.objects); err != nil {
		return err
	}
	found := false
	for _, root := range s.objects {
		root.Walk(func(o *Object) { found = found || o == object })
	}
	if !found {
		return fmt.Errorf("redraw object must belong to the scene")
	}
	id := object.ID
	return s.AddUpdater(func(frame *Frame, time float64) error {
		dst := frame.Find(id)
		if dst == nil {
			return nil // It may not yet be visible or may have been removed.
		}
		geometry, err := draw(frame, time)
		if err != nil {
			return err
		}
		if err := geometry.Validate(); err != nil {
			return fmt.Errorf("redraw geometry: %w", err)
		}
		var sameTopology func(*Object, *Object) bool
		sameTopology = func(a, b *Object) bool {
			if len(a.Children) != len(b.Children) {
				return false
			}
			for i := range a.Children {
				if !sameTopology(a.Children[i], b.Children[i]) {
					return false
				}
			}
			return true
		}
		if !sameTopology(dst, geometry) {
			return fmt.Errorf("redraw changed child topology; fixed topology is required")
		}
		assignTree(dst, geometry)
		return nil
	})
}
