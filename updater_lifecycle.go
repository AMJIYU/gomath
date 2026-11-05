package gomath

import (
	"fmt"
	"sort"
)

const (
	MaxObjectUpdaters      = 4096
	MaxObjectUpdaterEvents = 65536
)

// ObjectUpdater edits one private sampled family. It must use absolute time,
// preserve its pointers/IDs/topology and mutate only the supplied object tree.
// Frame is for reading sampled state. No callback runs during authoring and no
// sampled update feeds back into subsequent animation starting geometry.
type ObjectUpdater func(object *Object, frame *Frame, time float64) error

type ObjectUpdaterOptions struct {
	// A Play owns each animated family for its full segment, including windows
	// and grouped delays. Animating an ancestor also suspends this updater;
	// animating a descendant does not suspend an updater on its parent.
	SuspendDuringAnimations bool
	// Zero limits use the same bounded family budgets as DynamicRedrawOptions.
	Limits DynamicRedrawOptions
}

func DefaultObjectUpdaterOptions() ObjectUpdaterOptions {
	return ObjectUpdaterOptions{SuspendDuringAnimations: true}
}

// UpdaterHandle identifies one registration. Its fields are private; handles
// belong to their registering Scene and cannot be moved to another Scene.
type UpdaterHandle struct{ ordinal int }

type objectUpdaterEvent struct {
	time            float64
	active, removed bool
}

type objectUpdaterState struct {
	handle  *UpdaterHandle
	id      uint64
	update  ObjectUpdater
	options ObjectUpdaterOptions
	events  []objectUpdaterEvent
}

// AddObjectUpdater takes effect at the current authoring cursor. It shares
// registration order with legacy AddUpdater/AddRedraw callbacks. Suspended,
// removed and not-yet-registered entries are skipped; a missing target is also
// skipped. Object IDs bind the updater, so replacement does not transfer it.
func (s *Scene) AddObjectUpdater(object *Object, update ObjectUpdater, options ObjectUpdaterOptions) (*UpdaterHandle, error) {
	if s == nil || object == nil || update == nil {
		return nil, fmt.Errorf("object updater needs a scene, object and callback")
	}
	limits, err := options.Limits.normalized()
	if err != nil {
		return nil, err
	}
	options.Limits = limits
	if len(s.objectUpdaters) == MaxObjectUpdaters || s.updaterEvents == MaxObjectUpdaterEvents {
		return nil, fmt.Errorf("object updater registration/event budget exceeded")
	}
	if err := validateDynamicTree(object, limits); err != nil {
		return nil, fmt.Errorf("object updater target: %w", err)
	}
	found, err := boundedUpdaterTarget(s.objects, object.ID)
	if err != nil {
		return nil, fmt.Errorf("object updater scene: %w", err)
	}
	if found != object {
		return nil, fmt.Errorf("object updater target must be owned by this scene")
	}
	if s.objectUpdaters == nil {
		s.objectUpdaters = make(map[int]objectUpdaterState)
	}
	index := len(s.updaters)
	handle := &UpdaterHandle{ordinal: index + 1}
	s.objectUpdaters[index] = objectUpdaterState{handle: handle, id: object.ID, update: update, options: options,
		events: []objectUpdaterEvent{{time: s.duration, active: true}}}
	s.updaters = append(s.updaters, nil)
	s.updaterEvents++
	return handle, nil
}

// AddObjectRedraw is the managed fixed-topology variant of AddRedraw. It uses
// the same cursor/suspension/removal semantics as AddObjectUpdater, validates
// independent generated geometry before copying it, and keeps sampled IDs.
func (s *Scene) AddObjectRedraw(object *Object, draw RedrawFunc, options ObjectUpdaterOptions) (*UpdaterHandle, error) {
	if draw == nil {
		return nil, fmt.Errorf("object redraw requires a callback")
	}
	limits, err := options.Limits.normalized()
	if err != nil {
		return nil, err
	}
	return s.AddObjectUpdater(object, func(dst *Object, frame *Frame, time float64) error {
		geometry, err := draw(frame, time)
		if err != nil {
			return err
		}
		if err := validateDynamicTree(geometry, limits); err != nil {
			return fmt.Errorf("managed redraw geometry: %w", err)
		}
		type pair struct{ a, b *Object }
		stack := []pair{{dst, geometry}}
		for len(stack) > 0 {
			p := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if len(p.a.Children) != len(p.b.Children) {
				return fmt.Errorf("managed redraw changed child topology")
			}
			for i, a := range p.a.Children {
				stack = append(stack, pair{a, p.b.Children[i]})
			}
		}
		assignTree(dst, geometry)
		return nil
	}, options)
}

// SuspendUpdater and ResumeUpdater record a half-open change at the current
// cursor. Same-cursor edits coalesce; earlier samples retain their old state.
func (s *Scene) SuspendUpdater(handle *UpdaterHandle) error {
	return s.changeObjectUpdater(handle, false, false)
}
func (s *Scene) ResumeUpdater(handle *UpdaterHandle) error {
	return s.changeObjectUpdater(handle, true, false)
}

// RemoveUpdater permanently ends this registration at the current cursor.
// Removing again is an error. The historical callback remains available for
// earlier samples; callback closures must remain immutable/concurrency-safe.
func (s *Scene) RemoveUpdater(handle *UpdaterHandle) error {
	return s.changeObjectUpdater(handle, false, true)
}

// ClearObjectUpdaters permanently removes managed registrations on an owned
// target, and optionally its descendants, at the current cursor. Legacy global
// frame callbacks are independent. The batch either records every change or
// leaves the schedule unchanged if its event budget would be exceeded.
func (s *Scene) ClearObjectUpdaters(object *Object, recursive bool) error {
	if s == nil || object == nil {
		return fmt.Errorf("clear object updaters needs a scene-owned object")
	}
	found, err := boundedUpdaterTarget(s.objects, object.ID)
	if err != nil {
		return err
	}
	if found != object {
		return fmt.Errorf("clear object updater target must be owned by this scene")
	}
	ids := map[uint64]bool{object.ID: true}
	if recursive {
		object.Walk(func(o *Object) { ids[o.ID] = true })
	}
	var handles []*UpdaterHandle
	additional := 0
	for _, entry := range s.objectUpdaters {
		last := entry.events[len(entry.events)-1]
		if ids[entry.id] && !last.removed {
			handles = append(handles, entry.handle)
			if last.time != s.duration {
				additional++
			}
		}
	}
	if additional > MaxObjectUpdaterEvents-s.updaterEvents {
		return fmt.Errorf("clear object updaters exceeds event budget")
	}
	for _, handle := range handles {
		// All handles and capacity were preflighted above; no callbacks run.
		if err := s.RemoveUpdater(handle); err != nil {
			return err
		}
	}
	return nil
}

func (s *Scene) changeObjectUpdater(handle *UpdaterHandle, active, removed bool) error {
	if s == nil || handle == nil {
		return fmt.Errorf("unknown object updater handle")
	}
	index := handle.ordinal - 1
	entry, ok := s.objectUpdaters[index]
	if !ok || entry.handle != handle {
		return fmt.Errorf("object updater handle belongs to another scene or is uninitialized")
	}
	last := entry.events[len(entry.events)-1]
	if last.removed {
		return fmt.Errorf("object updater was permanently removed")
	}
	if last.active == active && !removed {
		return nil
	}
	next := objectUpdaterEvent{time: s.duration, active: active, removed: removed}
	if last.time == s.duration {
		entry.events[len(entry.events)-1] = next
	} else {
		if s.updaterEvents == MaxObjectUpdaterEvents {
			return fmt.Errorf("object updater event budget exceeded")
		}
		entry.events = append(entry.events, next)
		s.updaterEvents++
	}
	s.objectUpdaters[index] = entry
	return nil
}

// boundedUpdaterTarget also protects registration from malformed ownership.
func boundedUpdaterTarget(roots []*Object, id uint64) (*Object, error) {
	index, err := boundedUpdaterIndex(roots)
	if err != nil {
		return nil, err
	}
	return index[id], nil
}

// Hard frame-wide traversal caps are separate from each target's usually
// smaller family limits. Managed callback batches share one immutable index.
func boundedUpdaterIndex(roots []*Object) (map[uint64]*Object, error) {
	const maxNodes = 65536
	type item struct {
		o     *Object
		depth int
	}
	if len(roots) > maxNodes {
		return nil, fmt.Errorf("object updater frame object budget exceeded")
	}
	stack := make([]item, 0, len(roots))
	for _, root := range roots {
		stack = append(stack, item{root, 1})
	}
	index := make(map[uint64]*Object)
	curves, contours := 0, 0
	for len(stack) > 0 {
		v := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		o := v.o
		if o == nil || o.ID == 0 || index[o.ID] != nil || v.depth > 256 || len(index) == maxNodes {
			return nil, fmt.Errorf("object updater frame has invalid ownership or excessive depth/objects")
		}
		index[o.ID] = o
		if len(o.Children) > maxNodes-len(index)-len(stack) || len(o.Path.Contours) > maxNodes-contours {
			return nil, fmt.Errorf("object updater frame object/contour budget exceeded")
		}
		contours += len(o.Path.Contours)
		for _, contour := range o.Path.Contours {
			if len(contour.Curves) > (1<<20)-curves {
				return nil, fmt.Errorf("object updater frame curve budget exceeded")
			}
			curves += len(contour.Curves)
		}
		if err := o.ValidateVisual(); err != nil {
			return nil, err
		}
		for _, child := range o.Children {
			stack = append(stack, item{child, v.depth + 1})
		}
	}
	return index, nil
}

type updaterOwnedNode struct {
	object   *Object
	id       uint64
	children []*Object
}

func captureUpdaterNodes(root *Object) []updaterOwnedNode {
	var nodes []updaterOwnedNode
	root.Walk(func(o *Object) {
		nodes = append(nodes, updaterOwnedNode{o, o.ID, append([]*Object(nil), o.Children...)})
	})
	return nodes
}
func checkUpdaterNodes(nodes []updaterOwnedNode) error {
	for _, n := range nodes {
		if n.object.ID != n.id || len(n.object.Children) != len(n.children) {
			return fmt.Errorf("object updater changed identity or child topology")
		}
		for i, child := range n.children {
			if n.object.Children[i] != child {
				return fmt.Errorf("object updater replaced or reordered a child")
			}
		}
	}
	return nil
}

type objectUpdaterFrameState struct {
	index     map[uint64]*Object
	roots     []*Object
	nodes     []updaterOwnedNode
	suspended map[uint64]bool
}

func (c *objectUpdaterFrameState) prepare(frame *Frame) error {
	if c.index != nil {
		return nil
	}
	index, err := boundedUpdaterIndex(frame.Objects)
	if err != nil {
		return err
	}
	c.index = index
	c.roots = append([]*Object(nil), frame.Objects...)
	for _, root := range frame.Objects {
		c.nodes = append(c.nodes, captureUpdaterNodes(root)...)
	}
	return nil
}
func (c *objectUpdaterFrameState) finish(frame *Frame) error {
	if c == nil || c.index == nil {
		return nil
	}
	if len(c.roots) != len(frame.Objects) {
		return fmt.Errorf("object updater changed frame roots")
	}
	for i, root := range c.roots {
		if root != frame.Objects[i] {
			return fmt.Errorf("object updater replaced or reordered a frame root")
		}
	}
	if err := checkUpdaterNodes(c.nodes); err != nil {
		return err
	}
	_, err := boundedUpdaterIndex(frame.Objects)
	return err
}
func (s *Scene) updaterSuspendedIDs(time float64) (map[uint64]bool, error) {
	result := make(map[uint64]bool)
	segIndex := sort.Search(len(s.segments), func(i int) bool { return s.segments[i].end > time })
	if segIndex == len(s.segments) {
		return result, nil
	}
	work := 0
	for _, tr := range s.segments[segIndex].tracks {
		// Only the animated source family is suspended. A separately owned
		// Transform destination remains read-only scene data.
		stack := []*Object{tr.from}
		for len(stack) > 0 {
			o := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			work++
			if work > 1<<20 || o == nil || len(o.Children) > (1<<20)-work-len(stack) {
				return nil, fmt.Errorf("object updater animation search budget exceeded")
			}
			result[o.ID] = true
			stack = append(stack, o.Children...)
		}
	}
	return result, nil
}

func (s *Scene) applyObjectUpdater(entry objectUpdaterState, frame *Frame, time float64, context *objectUpdaterFrameState) error {
	i := sort.Search(len(entry.events), func(i int) bool { return entry.events[i].time > time })
	if i == 0 || !entry.events[i-1].active {
		return nil
	}
	if entry.options.SuspendDuringAnimations {
		if context.suspended == nil {
			var err error
			context.suspended, err = s.updaterSuspendedIDs(time)
			if err != nil {
				return err
			}
		}
		if context.suspended[entry.id] {
			return nil
		}
	}
	if err := context.prepare(frame); err != nil {
		return err
	}
	dst := context.index[entry.id]
	if dst == nil {
		return nil
	}
	if err := validateDynamicTree(dst, entry.options.Limits); err != nil {
		return err
	}
	nodes := captureUpdaterNodes(dst)
	if err := callObjectUpdater(entry.update, dst, frame, time); err != nil {
		return err
	}
	if err := checkUpdaterNodes(nodes); err != nil {
		return err
	}
	return validateDynamicTree(dst, entry.options.Limits)
}

func callObjectUpdater(update ObjectUpdater, dst *Object, frame *Frame, time float64) (err error) {
	completed := false
	defer func() {
		if !completed {
			_ = recover()
			err = fmt.Errorf("object updater callback panicked")
		}
	}()
	err = update(dst, frame, time)
	completed = true
	return err
}
