package gomath

import "fmt"

// DynamicRedrawOptions bounds each returned display tree before it is copied.
// Zero fields select defaults (4096 objects, 262144 curves, 16384 contours,
// depth 64). Hard maxima are 65536 objects/contours, 1048576 curves and depth256.
type DynamicRedrawOptions struct {
	MaxObjects, MaxCurves, MaxContours, MaxDepth int
}

func (o DynamicRedrawOptions) normalized() (DynamicRedrawOptions, error) {
	if o.MaxObjects == 0 {
		o.MaxObjects = 4096
	}
	if o.MaxCurves == 0 {
		o.MaxCurves = 262144
	}
	if o.MaxContours == 0 {
		o.MaxContours = 16384
	}
	if o.MaxDepth == 0 {
		o.MaxDepth = 64
	}
	if o.MaxObjects < 1 || o.MaxObjects > 65536 || o.MaxCurves < 1 || o.MaxCurves > 1048576 || o.MaxContours < 1 || o.MaxContours > 65536 || o.MaxDepth < 1 || o.MaxDepth > 256 {
		return o, fmt.Errorf("invalid dynamic redraw budgets")
	}
	return o, nil
}

// AddDynamicRedraw replaces one private sampled display tree, allowing a
// different child topology at each absolute time. It preserves the target root
// ID and reserves deterministic preorder child-ID slots at registration. Child
// IDs identify ordinal positions, not semantic correspondence across topology
// changes; original authoring-child IDs are not retained in the generated tree.
//
// The callback has the same purity/concurrency contract as RedrawFunc. It runs
// after animation tracks and earlier registered updaters. It may read sampled
// trackers through frame.Value. It must not retain/mutate frames or concurrently
// mutate its returned tree. Output is copied before installation; immutable
// bitmap assets and styles may be shared. The authoring tree stays unchanged.
// This is an absolute-time display updater, not Manim's general DT scheduling.
func (s *Scene) AddDynamicRedraw(object *Object, draw RedrawFunc, options DynamicRedrawOptions) error {
	opt, err := options.normalized()
	if err != nil {
		return err
	}
	if object == nil || draw == nil {
		return fmt.Errorf("dynamic redraw needs an object and callback")
	}
	if err := validateObjects(s.objects); err != nil {
		return err
	}
	found := false
	var maxSceneID uint64
	for _, root := range s.objects {
		root.Walk(func(o *Object) {
			found = found || o == object
			maxSceneID = max(maxSceneID, o.ID)
		})
	}
	if !found {
		return fmt.Errorf("dynamic redraw object must belong to the scene")
	}
	reserve := uint64(opt.MaxObjects - 1)
	var first uint64
	if reserve != 0 {
		for {
			previous := objectSequence.Load()
			base := max(previous, maxSceneID)
			if base > ^uint64(0)-reserve {
				return fmt.Errorf("dynamic redraw identity space exhausted")
			}
			if objectSequence.CompareAndSwap(previous, base+reserve) {
				first = base + 1
				break
			}
		}
	}
	id := object.ID
	return s.AddUpdater(func(frame *Frame, time float64) error {
		dst := frame.Find(id)
		if dst == nil {
			return nil
		}
		geometry, err := draw(frame, time)
		if err != nil {
			return err
		}
		if err := validateDynamicTree(geometry, opt); err != nil {
			return fmt.Errorf("dynamic redraw geometry: %w", err)
		}
		state := geometry.snapshot()
		next := first
		state.Walk(func(o *Object) {
			if o == state {
				o.ID = id
			} else {
				o.ID = next
				next++
			}
		})
		// Replace topology directly. assignTree intentionally preserves old
		// child identities and would violate the reserved-slot policy here.
		*dst = *state
		// Public Object IDs can be reassigned by callers. Detect even a forged
		// collision against reserved slots elsewhere in this sampled frame.
		if err := validateObjects(frame.Objects); err != nil {
			return fmt.Errorf("dynamic redraw frame: %w", err)
		}
		return nil
	})
}

func validateDynamicTree(root *Object, opt DynamicRedrawOptions) error {
	type entry struct {
		object *Object
		depth  int
	}
	stack := []entry{{root, 1}}
	seen, ids := map[*Object]bool{}, map[uint64]bool{}
	curves, contours := 0, 0
	for len(stack) != 0 {
		item := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		o := item.object
		if o == nil || seen[o] {
			return fmt.Errorf("nil, shared or cyclic object")
		}
		if o.ID == 0 || ids[o.ID] {
			return fmt.Errorf("zero or duplicate generated identity")
		}
		seen[o], ids[o.ID] = true, true
		if len(seen) > opt.MaxObjects || item.depth > opt.MaxDepth || len(o.Children) > opt.MaxObjects-len(seen)-len(stack) {
			return fmt.Errorf("dynamic redraw object/depth budget exceeded")
		}
		if len(o.Path.Contours) > opt.MaxContours-contours {
			return fmt.Errorf("dynamic redraw contour budget exceeded")
		}
		contours += len(o.Path.Contours)
		for _, contour := range o.Path.Contours {
			if len(contour.Curves) > opt.MaxCurves-curves {
				return fmt.Errorf("dynamic redraw curve budget exceeded")
			}
			curves += len(contour.Curves)
		}
		if err := o.ValidateVisual(); err != nil {
			return err
		}
		for i := len(o.Children) - 1; i >= 0; i-- {
			stack = append(stack, entry{o.Children[i], item.depth + 1})
		}
	}
	return nil
}
