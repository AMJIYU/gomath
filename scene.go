package gomath

import (
	"fmt"
	"gomath/subtitle"
	"sort"
)

type Frame struct {
	Time    float64
	Objects []*Object
	Camera  *Camera2D
}

func (f *Frame) Find(id uint64) *Object {
	for _, o := range f.Objects {
		var found *Object
		o.Walk(func(c *Object) {
			if c.ID == id {
				found = c
			}
		})
		if found != nil {
			return found
		}
	}
	return nil
}

// FrameUpdater is evaluated on a private frame snapshot. Use absolute time to
// make it deterministic. It may run concurrently; captured data must be safe.
type FrameUpdater func(frame *Frame, time float64) error
type segment struct {
	start, end float64
	before     []*Object
	tracks     []track
}
type Scene struct {
	objects        []*Object
	segments       []segment
	duration       float64
	final          []*Object
	updaters       []FrameUpdater
	objectUpdaters map[int]objectUpdaterState
	updaterEvents  int
	camera         cameraTimeline
	sounds         []soundEvent
	captions       *subtitle.Track
}

func NewScene() *Scene             { return &Scene{} }
func (s *Scene) Duration() float64 { return s.duration }
func (s *Scene) Add(objects ...*Object) error {
	candidate := append(append([]*Object(nil), s.objects...), objects...)
	if e := validateObjects(candidate); e != nil {
		return e
	}
	s.objects = candidate
	s.final = snapshots(candidate)
	return nil
}
func (s *Scene) Remove(objects ...*Object) {
	ids := map[uint64]bool{}
	for _, o := range objects {
		if o != nil {
			ids[o.ID] = true
		}
	}
	s.objects = rewriteTree(s.objects, ids, nil)
	s.final = snapshots(s.objects)
}
func (s *Scene) AddUpdater(fn FrameUpdater) error {
	if fn == nil {
		return fmt.Errorf("nil frame updater")
	}
	s.updaters = append(s.updaters, fn)
	return nil
}
func (s *Scene) Wait(duration float64) error { return s.Play(duration) }

// Play validates and captures inputs before mutating authoring state. Each
// object's state is independent of frame sampling order.
func (s *Scene) Play(duration float64, animations ...Animation) error {
	if !finite(duration) || duration <= 0 || !finite(s.duration+duration) || s.duration+duration <= s.duration {
		return fmt.Errorf("duration must be finite and positive")
	}
	if len(animations) == 1 && animations[0].group != nil && animations[0].group.sequence {
		return s.playSuccession(duration, animations[0])
	}
	var err error
	animations, err = flattenAnimations(animations)
	if err != nil {
		return err
	}
	for _, a := range animations {
		if a.matching != nil {
			if err := matchingInputs(a.object, a.target, a.matching, true); err != nil {
				return err
			}
		}
	}
	roots := append([]*Object(nil), s.objects...)
	if e := validateObjects(roots); e != nil {
		return e
	}
	objects := map[uint64]*Object{}
	for _, o := range roots {
		o.Walk(func(c *Object) { objects[c.ID] = c })
	}
	var tracks []track
	written := map[uint64]bool{}
	for _, a := range animations {
		if a.object == nil {
			return fmt.Errorf("animation has nil object")
		}
		if e := validateObjects([]*Object{a.object}); e != nil {
			return e
		}
		if a.target != nil {
			if e := validateObjects([]*Object{a.target}); e != nil {
				return e
			}
		}
		if objects[a.object.ID] == nil {
			if a.mode != fadeIn && a.mode != create && a.mode != growing && a.mode != passingFlash && a.mode != copyMorph && a.mode != crossFadeIn {
				return fmt.Errorf("object %d is not in scene", a.object.ID)
			}
			roots = append(roots, a.object)
			if e := validateObjects(roots); e != nil {
				return e
			}
			a.object.Walk(func(c *Object) { objects[c.ID] = c })
		} else if objects[a.object.ID] != a.object {
			return fmt.Errorf("animation object %d does not match its scene identity", a.object.ID)
		}
		if a.mode == replacement && a.target != nil {
			inUse := false
			a.target.Walk(func(c *Object) { inUse = inUse || objects[c.ID] != nil })
			if inUse {
				return fmt.Errorf("replacement target or its descendants already belong to the scene")
			}
		}
		overlap := false
		a.object.Walk(func(c *Object) {
			if written[c.ID] {
				overlap = true
			}
		})
		if overlap {
			return fmt.Errorf("overlapping animation targets")
		}
		// Detect a child track followed by a parent track, as well as the reverse.
		a.object.Walk(func(c *Object) { written[c.ID] = true })
		tr, e := prepare(a)
		if e != nil {
			return e
		}
		tracks = append(tracks, tr)
	}
	before := snapshots(roots)
	staged := snapshots(roots)
	indexTree := func(objects []*Object) map[uint64]*Object {
		index := map[uint64]*Object{}
		for _, o := range objects {
			o.Walk(func(c *Object) { index[c.ID] = c })
		}
		return index
	}
	beforeIndex, stagedIndex := indexTree(before), indexTree(staged)
	// Appearance animations must start invisible even before their window.
	for _, tr := range tracks {
		if e := tr.apply(beforeIndex[tr.id], 0); e != nil {
			return e
		}
		if e := tr.apply(stagedIndex[tr.id], 1); e != nil {
			return e
		}
	}
	if e := validateObjects(staged); e != nil {
		return e
	}
	removals := map[uint64]bool{}
	replacements, stagedReplacements := map[uint64]*Object{}, map[uint64]*Object{}
	for _, a := range animations {
		if a.mode == fadeOut || a.mode == passingFlash || a.mode == uncreate || a.mode == crossFadeOut {
			removals[a.object.ID] = true
		}
		if a.mode == replacement {
			replacements[a.object.ID] = a.target
			stagedReplacements[a.object.ID] = a.target.snapshot()
		}
	}
	// Validate the final topology before any authoring object is changed. This
	// catches a replacement target also introduced by a simultaneous FadeIn.
	if e := validateObjects(rewriteTree(staged, removals, stagedReplacements)); e != nil {
		return e
	}
	for i, tr := range tracks {
		if animations[i].matching != nil {
			// Matching cleanup restores its removed source for later reuse.
			continue
		}
		assignTree(objects[tr.id], stagedIndex[tr.id])
		if animations[i].mode == fadeOut || animations[i].mode == crossFadeOut || (tr.pose != nil && tr.pose.kind == passingFlash) {
			assignTree(objects[tr.id], tr.from)
		}
	}
	s.objects = rewriteTree(roots, removals, replacements)
	s.segments = append(s.segments, segment{start: s.duration, end: s.duration + duration, before: before, tracks: tracks})
	s.duration += duration
	s.final = snapshots(s.objects)
	return nil
}

// rewriteTree retains surviving object pointers and child order. Use snapshot
// inputs while validating a planned edit; call on authoring trees only to commit.
func rewriteTree(roots []*Object, removals map[uint64]bool, replacements map[uint64]*Object) []*Object {
	if len(removals) == 0 && len(replacements) == 0 {
		return roots
	}
	out := make([]*Object, 0, len(roots))
	for _, o := range roots {
		if target := replacements[o.ID]; target != nil {
			out = append(out, target)
			continue
		}
		if removals[o.ID] {
			continue
		}
		o.Children = rewriteTree(o.Children, removals, replacements)
		out = append(out, o)
	}
	return out
}
func (s *Scene) Sample(time float64) (*Frame, error) {
	f, err := s.sampleBase(time)
	if err != nil {
		return nil, err
	}
	if err := s.applyUpdaters(f, time); err != nil {
		return nil, err
	}
	return f, nil
}

// sampleBase evaluates immutable object/camera tracks without user updaters.
// Replay merges integrated state before applying those absolute-time callbacks.
func (s *Scene) sampleBase(time float64) (*Frame, error) {
	if !finite(time) || time < 0 || time > s.duration {
		return nil, fmt.Errorf("sample time %g outside [0,%g]", time, s.duration)
	}
	f := &Frame{Time: time}
	i := sort.Search(len(s.segments), func(i int) bool { return s.segments[i].end > time })
	if i == len(s.segments) {
		f.Objects = snapshots(s.final)
	} else {
		seg := s.segments[i]
		f.Objects = snapshots(seg.before)
		index := map[uint64]*Object{}
		for _, o := range f.Objects {
			o.Walk(func(c *Object) { index[c.ID] = c })
		}
		alpha := (time - seg.start) / (seg.end - seg.start)
		for _, tr := range seg.tracks {
			if e := tr.apply(index[tr.id], alpha); e != nil {
				return nil, fmt.Errorf("animation at %g: %w", time, e)
			}
		}
	}
	if err := s.applyCamera(f, time); err != nil {
		return nil, err
	}
	return f, nil
}

func (s *Scene) applyUpdaters(f *Frame, time float64) error {
	if len(s.objectUpdaters) == 0 {
		for _, fn := range s.updaters {
			if err := fn(f, time); err != nil {
				return fmt.Errorf("frame updater at %g: %w", time, err)
			}
		}
		return nil
	}
	context := new(objectUpdaterFrameState)
	for index, fn := range s.updaters {
		var err error
		if managed, ok := s.objectUpdaters[index]; ok {
			err = s.applyObjectUpdater(managed, f, time, context)
		} else {
			// Legacy callbacks can change display topology; validate the
			// preceding managed batch and rebuild its index afterwards.
			if err = context.finish(f); err == nil {
				err = fn(f, time)
			}
			context = new(objectUpdaterFrameState)
		}
		if err != nil {
			return fmt.Errorf("frame updater at %g: %w", time, err)
		}
	}
	if err := context.finish(f); err != nil {
		return fmt.Errorf("object updater output at %g: %w", time, err)
	}
	return nil
}
