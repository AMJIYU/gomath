package gomath

import (
	"reflect"
	"testing"
)

func TestObjectUpdaterEventCapacityAndClearAtomicity(t *testing.T) {
	s := NewScene()
	child := Group()
	parent := Group(child)
	if e := s.Add(parent); e != nil {
		t.Fatal(e)
	}
	fn := func(*Object, *Frame, float64) error { return nil }
	a, e := s.AddObjectUpdater(parent, fn, ObjectUpdaterOptions{})
	if e != nil {
		t.Fatal(e)
	}
	b, e := s.AddObjectUpdater(child, fn, ObjectUpdaterOptions{})
	if e != nil {
		t.Fatal(e)
	}
	// Exercise the final event slot without authoring 65k unrelated wait segments.
	s.updaterEvents = MaxObjectUpdaterEvents - 1
	s.duration = 1
	beforeA := append([]objectUpdaterEvent(nil), s.objectUpdaters[a.ordinal-1].events...)
	beforeB := append([]objectUpdaterEvent(nil), s.objectUpdaters[b.ordinal-1].events...)
	if s.ClearObjectUpdaters(parent, true) == nil {
		t.Fatal("partial clear accepted over capacity")
	}
	if !reflect.DeepEqual(beforeA, s.objectUpdaters[a.ordinal-1].events) || !reflect.DeepEqual(beforeB, s.objectUpdaters[b.ordinal-1].events) || s.updaterEvents != MaxObjectUpdaterEvents-1 {
		t.Fatal("failed clear published partial schedule")
	}
	if e := s.SuspendUpdater(a); e != nil {
		t.Fatal(e)
	}
	if s.updaterEvents != MaxObjectUpdaterEvents {
		t.Fatal("did not consume final event")
	}
	s.duration = 2
	if s.ResumeUpdater(a) == nil || s.RemoveUpdater(b) == nil {
		t.Fatal("event cap ignored")
	}
	if s.objectUpdaters[a.ordinal-1].events[1].active {
		t.Fatal("failed change modified schedule")
	}
	s.duration = 1
	if e := s.ResumeUpdater(a); e != nil {
		t.Fatal("same-cursor coalescing needs no event capacity", e)
	}
	if s.updaterEvents != MaxObjectUpdaterEvents {
		t.Fatal("same-cursor edit changed event count")
	}
	if h, e := s.AddObjectUpdater(child, fn, ObjectUpdaterOptions{}); e == nil || h != nil {
		t.Fatal("registration ignored full event capacity")
	}
}

func TestObjectUpdaterFrameAndSuspensionTraversalBudgets(t *testing.T) {
	for _, roots := range [][]*Object{{nil}, {Group(), nil}, make([]*Object, 65537)} {
		if _, e := boundedUpdaterIndex(roots); e == nil {
			t.Fatal("invalid frame accepted")
		}
	}
	root := Group()
	root.Children = make([]*Object, 65536)
	if _, e := boundedUpdaterIndex([]*Object{root}); e == nil {
		t.Fatal("frame node budget ignored")
	}
	root = Group()
	root.Children = []*Object{root}
	if _, e := boundedUpdaterIndex([]*Object{root}); e == nil {
		t.Fatal("frame cycle accepted")
	}
	root = Group()
	for range 256 {
		root = Group(root)
	}
	if _, e := boundedUpdaterIndex([]*Object{root}); e == nil {
		t.Fatal("frame depth cap ignored")
	}
	// Oversized immutable track metadata must reject before allocating a search
	// stack or calling an updater. Normal Play validates its own input tree.
	trackRoot := Group()
	trackRoot.Children = make([]*Object, 1<<20)
	s := &Scene{duration: 1, segments: []segment{{start: 0, end: 1, tracks: []track{{from: trackRoot}}}}}
	if _, e := s.updaterSuspendedIDs(.5); e == nil {
		t.Fatal("suspension traversal cap ignored")
	}
}
