package gomath

import (
	"fmt"
	"math"
	"reflect"
	"sort"
)

// Integration owns incremental state for an object subtree. Update receives a
// private state, elapsed dt and the new absolute time. It may change geometry,
// style and scalars, but must preserve child topology and IDs. Captured data must
// be read-only and safe for concurrent calls; all persistent state belongs here.
type Integration struct {
	Object *Object
	Update func(state *Object, dt, time float64) error
}

type ReplayOptions struct {
	Step            float64
	CheckpointEvery int
	MaxSteps        int
}

func DefaultReplayOptions() ReplayOptions {
	return ReplayOptions{Step: 1.0 / 120, CheckpointEvery: 30, MaxSteps: 1_000_000}
}

type replayCheckpoint struct {
	index  int
	time   float64
	states []*Object
}

// Replay freezes authoring tracks and preintegrates fixed time steps. Samples
// replay from the preceding immutable checkpoint, then perform a partial final
// step. Sampling is deterministic and concurrent without a shared mutable clock.
// Checkpoints store object states, not raster frames.
//
// This initial integration model reserves each target subtree for DT updates:
// it must exist unchanged throughout the authored timeline and cannot overlap
// an animation target. Animate separate objects and use AddRedraw to visualize
// integrated scalar state. Absolute-time updaters run after integration and do
// not feed display changes back into persistent state.
type Replay struct {
	scene       *Scene
	options     ReplayOptions
	updates     []Integration
	initial     []*Object
	checkpoints []replayCheckpoint
}

func CompileReplay(scene *Scene, opt ReplayOptions, updates ...Integration) (*Replay, error) {
	if scene == nil || !finite(opt.Step) || opt.Step <= 0 || opt.CheckpointEvery < 1 || opt.MaxSteps < 1 || opt.MaxSteps > 1_000_000 {
		return nil, fmt.Errorf("replay needs a scene, positive finite step, positive checkpoint interval and 1..1000000 maximum steps")
	}
	if err := validateObjects(scene.objects); err != nil {
		return nil, err
	}
	steps := math.Ceil(scene.Duration() / opt.Step)
	if !finite(steps) || steps > float64(opt.MaxSteps) {
		return nil, fmt.Errorf("replay exceeds %d integration steps", opt.MaxSteps)
	}
	frozen := freezeScene(scene)
	base, err := frozen.sampleBase(0)
	if err != nil {
		return nil, err
	}
	r := &Replay{scene: frozen, options: opt, updates: append([]Integration(nil), updates...)}
	owned := map[uint64]bool{}
	for i, update := range r.updates {
		if update.Object == nil || update.Update == nil {
			return nil, fmt.Errorf("integration %d needs a target and function", i)
		}
		if err := update.Object.Validate(); err != nil {
			return nil, err
		}
		state := base.Find(update.Object.ID)
		if state == nil {
			return nil, fmt.Errorf("integration %d target must exist from time zero", i)
		}
		identityMatches := false
		for _, root := range scene.objects {
			root.Walk(func(o *Object) { identityMatches = identityMatches || o == update.Object })
		}
		if !identityMatches {
			return nil, fmt.Errorf("integration %d target is not its scene object", i)
		}
		overlap := false
		state.Walk(func(o *Object) { overlap = overlap || owned[o.ID]; owned[o.ID] = true })
		if overlap {
			return nil, fmt.Errorf("overlapping integration targets")
		}
		r.initial = append(r.initial, state.snapshot())
		// Drop authoring pointers from the immutable replay's update records.
		r.updates[i].Object = nil
	}
	for _, seg := range frozen.segments {
		for _, track := range seg.tracks {
			overlap := false
			track.from.Walk(func(o *Object) { overlap = overlap || owned[o.ID] })
			track.to.Walk(func(o *Object) { overlap = overlap || owned[o.ID] })
			if overlap {
				return nil, fmt.Errorf("integration target overlaps an authored animation")
			}
		}
		if err := r.validateAuthoredState(seg.before); err != nil {
			return nil, err
		}
	}
	if err := r.validateAuthoredState(frozen.final); err != nil {
		return nil, err
	}
	states := snapshots(r.initial)
	r.checkpoints = append(r.checkpoints, replayCheckpoint{states: snapshots(states)})
	previous := 0.0
	for index := 1; index <= int(steps); index++ {
		time := math.Min(float64(index)*opt.Step, frozen.duration)
		if time <= previous {
			return nil, fmt.Errorf("integration step no longer advances time")
		}
		if err := r.advance(states, time-previous, time); err != nil {
			return nil, err
		}
		if index%opt.CheckpointEvery == 0 || index == int(steps) {
			r.checkpoints = append(r.checkpoints, replayCheckpoint{index: index, time: time, states: snapshots(states)})
		}
		previous = time
	}
	return r, nil
}

func (r *Replay) validateAuthoredState(roots []*Object) error {
	f := &Frame{Objects: roots}
	for _, initial := range r.initial {
		state := f.Find(initial.ID)
		if state == nil || !reflect.DeepEqual(initial, state) {
			return fmt.Errorf("integration target %d is removed, replaced or changed in the authored timeline", initial.ID)
		}
	}
	return nil
}

func (r *Replay) advance(states []*Object, dt, time float64) error {
	for i, state := range states {
		if err := r.updates[i].Update(state, dt, time); err != nil {
			return fmt.Errorf("integration %d at %g: %w", i, time, err)
		}
		if err := state.Validate(); err != nil {
			return fmt.Errorf("integration %d at %g: %w", i, time, err)
		}
		if !sameIdentityTree(state, r.initial[i]) {
			return fmt.Errorf("integration %d changed object identity or topology", i)
		}
	}
	return nil
}

func sameIdentityTree(a, b *Object) bool {
	if a.ID != b.ID || len(a.Children) != len(b.Children) {
		return false
	}
	for i := range a.Children {
		if !sameIdentityTree(a.Children[i], b.Children[i]) {
			return false
		}
	}
	return true
}

func (r *Replay) Duration() float64 {
	if r == nil || r.scene == nil {
		return 0
	}
	return r.scene.Duration()
}

func (r *Replay) Sample(time float64) (*Frame, error) {
	if r == nil || r.scene == nil || len(r.checkpoints) == 0 || !finite(time) || time < 0 || time > r.Duration() {
		return nil, fmt.Errorf("invalid replay sample time")
	}
	i := sort.Search(len(r.checkpoints), func(i int) bool { return r.checkpoints[i].time > time }) - 1
	checkpoint := r.checkpoints[i]
	states := snapshots(checkpoint.states)
	previous := checkpoint.time
	for index := checkpoint.index + 1; ; index++ {
		next := math.Min(float64(index)*r.options.Step, r.Duration())
		if next > time || next <= previous {
			break
		}
		if err := r.advance(states, next-previous, next); err != nil {
			return nil, err
		}
		previous = next
	}
	if time > previous {
		if err := r.advance(states, time-previous, time); err != nil {
			return nil, err
		}
	}
	f, err := r.scene.sampleBase(time)
	if err != nil {
		return nil, err
	}
	for _, state := range states {
		assignTree(f.Find(state.ID), state)
	}
	if err := r.scene.applyUpdaters(f, time); err != nil {
		return nil, err
	}
	return f, nil
}

func freezeScene(s *Scene) *Scene {
	frozen := *s
	frozen.objects, frozen.final = snapshots(s.objects), snapshots(s.final)
	frozen.updaters = append([]FrameUpdater(nil), s.updaters...)
	frozen.objectUpdaters = make(map[int]objectUpdaterState, len(s.objectUpdaters))
	for index, entry := range s.objectUpdaters {
		entry.events = append([]objectUpdaterEvent(nil), entry.events...)
		frozen.objectUpdaters[index] = entry
	}
	frozen.sounds = append([]soundEvent(nil), s.sounds...)
	frozen.camera.segments = append([]cameraSegment(nil), s.camera.segments...)
	frozen.segments = make([]segment, len(s.segments))
	for i, seg := range s.segments {
		copy := seg
		copy.before = snapshots(seg.before)
		copy.tracks = make([]track, len(seg.tracks))
		for j, tr := range seg.tracks {
			copy.tracks[j] = tr
			copy.tracks[j].from, copy.tracks[j].to = tr.from.snapshot(), tr.to.snapshot()
			if tr.rotation != nil {
				rotation := *tr.rotation
				copy.tracks[j].rotation = &rotation
			}
			if tr.pose != nil {
				pose := *tr.pose
				copy.tracks[j].pose = &pose
			}
		}
		frozen.segments[i] = copy
	}
	return &frozen
}
