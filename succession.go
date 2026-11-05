package gomath

import (
	"fmt"
	"reflect"
)

// Succession begins each child after the preceding child has finished and been
// cleaned up. The same object may be used by successive children. Child durations
// set their relative shares of Play's runtime; nested Succession is supported.
//
// The sequence must be the sole Play argument, with a full [0,1] window and a
// linear group clock. Parallel groups may be individual steps, with disjoint
// targets inside each step. A sequence inside a parallel group is rejected.
func Succession(animations ...Animation) Animation {
	return Animation{group: &animationGroup{children: append([]Animation(nil), animations...), lag: 1, sequence: true}, rate: Linear, end: 1}
}

type sequenceStep struct {
	animation Animation
	end       float64
}

func sequenceSteps(root *compiledAnimation) ([]sequenceStep, error) {
	var steps []sequenceStep
	var visit func(*compiledAnimation, float64, float64) error
	visit = func(node *compiledAnimation, lo, hi float64) error {
		a := node.animation
		if a.group == nil || !a.group.sequence {
			// Validate all parallel subtrees before any user rate is evaluated.
			if _, err := flattenAnimations([]Animation{a}); err != nil {
				return err
			}
			steps = append(steps, sequenceStep{animation: a, end: hi})
			return nil
		}
		if a.start != 0 || a.end != 1 || (a.rate != nil && reflect.ValueOf(a.rate).Pointer() != reflect.ValueOf(Linear).Pointer()) {
			return fmt.Errorf("Succession currently requires a full window and linear group clock; child rates/windows are supported")
		}
		elapsed, previous := 0.0, lo
		for i, child := range node.children {
			elapsed += child.duration
			end := lo + (hi-lo)*(elapsed/node.span)
			if i == len(node.children)-1 {
				end = hi
			}
			if !finite(end) || end <= previous || end > hi {
				return fmt.Errorf("unrepresentable Succession child boundary")
			}
			if err := visit(child, previous, end); err != nil {
				return err
			}
			previous = end
		}
		return nil
	}
	if err := visit(root, 0, 1); err != nil {
		return nil, err
	}
	return steps, nil
}

// playSuccession stages every child against a private identity-preserving copy.
// Failure in a late child leaves both the scene and every input object intact.
func (s *Scene) playSuccession(duration float64, animation Animation) error {
	roots, err := compileAnimations([]Animation{animation})
	if err != nil {
		return err
	}
	steps, err := sequenceSteps(roots[0])
	if err != nil {
		return err
	}
	if err := validateObjects(s.objects); err != nil {
		return err
	}
	originals := map[uint64]*Object{}
	const maxSequenceObjects = 100000
	capture := func(root *Object) error {
		if root == nil {
			return nil
		}
		if err := root.Validate(); err != nil {
			return err
		}
		var collision bool
		root.Walk(func(o *Object) {
			if old := originals[o.ID]; old != nil && old != o {
				collision = true
			}
			originals[o.ID] = o
		})
		if collision {
			return fmt.Errorf("Succession inputs reuse an ID with different object pointers")
		}
		if len(originals) > maxSequenceObjects {
			return fmt.Errorf("Succession exceeds %d captured objects", maxSequenceObjects)
		}
		return nil
	}
	for _, root := range s.objects {
		if err := capture(root); err != nil {
			return err
		}
	}
	var captureAnimation func(Animation) error
	captureAnimation = func(a Animation) error {
		for _, o := range []*Object{a.object, a.target, a.route} {
			if err := capture(o); err != nil {
				return err
			}
		}
		if a.group != nil {
			for _, child := range a.group.children {
				if err := captureAnimation(child); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := captureAnimation(animation); err != nil {
		return err
	}
	clones := make(map[uint64]*Object, len(originals))
	for id, original := range originals {
		clone := *original
		clone.Path = original.Path.Clone()
		clone.Children = nil
		clones[id] = &clone
	}
	for id, original := range originals {
		for _, child := range original.Children {
			clones[id].Children = append(clones[id].Children, clones[child.ID])
		}
	}
	cloneObject := func(o *Object) *Object {
		if o == nil {
			return nil
		}
		return clones[o.ID]
	}
	var cloneAnimation func(Animation) Animation
	cloneAnimation = func(a Animation) Animation {
		a.object, a.target, a.route = cloneObject(a.object), cloneObject(a.target), cloneObject(a.route)
		if a.group != nil {
			group := *a.group
			group.children = make([]Animation, len(a.group.children))
			for i, child := range a.group.children {
				group.children[i] = cloneAnimation(child)
			}
			a.group = &group
		}
		return a
	}
	staged := NewScene()
	staged.duration = s.duration
	for _, root := range s.objects {
		staged.objects = append(staged.objects, clones[root.ID])
	}
	for i, step := range steps {
		end := s.duration + duration*step.end
		if end <= staged.duration || !finite(end) {
			return fmt.Errorf("unrepresentable Succession step %d runtime", i)
		}
		if err := staged.Play(end-staged.duration, cloneAnimation(step.animation)); err != nil {
			return fmt.Errorf("Succession step %d: %w", i, err)
		}
		staged.duration = end
		staged.segments[len(staged.segments)-1].end = end
	}
	// Include generated children as well as inputs removed by earlier cleanup.
	// Two passes commit fields and topology by identity, preserving replacement
	// target pointers rather than keeping an unrelated old child at its index.
	states := map[uint64]*Object{}
	var gather func(*Object) error
	gather = func(o *Object) error {
		if old := states[o.ID]; old != nil {
			if old != o {
				return fmt.Errorf("staged identity collision %d", o.ID)
			}
			return nil
		}
		states[o.ID] = o
		if len(states) > maxSequenceObjects {
			return fmt.Errorf("Succession exceeds %d staged objects", maxSequenceObjects)
		}
		for _, child := range o.Children {
			if err := gather(child); err != nil {
				return err
			}
		}
		return nil
	}
	for _, clone := range clones {
		if err := gather(clone); err != nil {
			return err
		}
	}
	for id := range states {
		if originals[id] == nil {
			originals[id] = &Object{ID: id}
		}
	}
	for id, state := range states {
		dst := originals[id]
		// Path ownership is transferred from the private staging tree; tracks
		// and final frames already own independent snapshots.
		dst.Path, dst.Bitmap, dst.Scalar = state.Path, state.Bitmap, state.Scalar
		dst.Clip = state.Clip
		dst.Style, dst.ZIndex = state.Style, state.ZIndex
		dst.Name = state.Name
		dst.Children = make([]*Object, len(state.Children))
		for i, child := range state.Children {
			dst.Children[i] = originals[child.ID]
		}
	}
	s.objects = make([]*Object, len(staged.objects))
	for i, root := range staged.objects {
		s.objects[i] = originals[root.ID]
	}
	s.segments = append(s.segments, staged.segments...)
	s.duration, s.final = staged.duration, staged.final
	return nil
}
