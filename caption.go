package gomath

import (
	"fmt"
	"gomath/subtitle"
	"math"
)

// AddSubcaption adds plain UTF-8 text at the current cursor plus offset, in
// seconds. Both absolute boundaries round to nearest milliseconds (ties up).
// A nonempty interval is required after rounding. It does not advance the scene.
// Overlapping and out-of-order additions are retained and stably time-sorted;
// Subcaptions lets callers choose stricter ordering/overlap acceptance for export.
// This explicit millisecond policy does not claim Manim's submillisecond timing.
func (s *Scene) AddSubcaption(text string, duration, offset float64) error {
	start := s.duration + offset
	end := start + duration
	limit := float64(subtitle.DefaultLimits().MaxTimestampMS)
	if !finite(duration) || duration <= 0 || !finite(offset) || !finite(start) || !finite(end) || start < 0 || end*1000 > limit {
		return fmt.Errorf("caption requires finite positive duration and boundaries inside the default subtitle timeline")
	}
	first, last := int64(math.Round(start*1000)), int64(math.Round(end*1000))
	cue, err := subtitle.CueAt(text, first, last-first, 0)
	if err != nil {
		return err
	}
	return s.addCaption(cue)
}

// AddSubcaptionMS uses exact integer duration/offset after rounding the current
// authoring cursor once to milliseconds. It shares AddSubcaption's cue limits.
func (s *Scene) AddSubcaptionMS(text string, durationMS, offsetMS int64) error {
	x := math.Round(s.duration * 1000)
	if !finite(x) || x < 0 || x > float64(subtitle.DefaultLimits().MaxTimestampMS) {
		return fmt.Errorf("caption cursor outside default subtitle timeline")
	}
	cue, err := subtitle.CueAt(text, int64(x), durationMS, offsetMS)
	if err != nil {
		return err
	}
	return s.addCaption(cue)
}

func (s *Scene) addCaption(cue subtitle.Cue) error {
	var track *subtitle.Track
	var err error
	if s.captions == nil {
		track, err = subtitle.New([]subtitle.Cue{cue}, subtitle.Options{Order: subtitle.OrderStableSort, Overlap: subtitle.OverlapAllow})
	} else {
		track, err = s.captions.Append(cue)
	}
	if err != nil {
		return err
	}
	s.captions = track
	return nil
}

// Subcaptions returns an independent immutable track with the requested policy.
// Cue tails are retained even if they extend past scene duration.
func (s *Scene) Subcaptions(options subtitle.Options) (*subtitle.Track, error) {
	var cues []subtitle.Cue
	if s.captions != nil {
		cues = s.captions.Cues()
	}
	return subtitle.New(cues, options)
}

func (r *Replay) Subcaptions(options subtitle.Options) (*subtitle.Track, error) {
	if r == nil || r.scene == nil {
		return nil, fmt.Errorf("uninitialized replay")
	}
	return r.scene.Subcaptions(options)
}
