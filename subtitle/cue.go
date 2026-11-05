// Package subtitle implements immutable millisecond subtitle tracks and bounded,
// strict UTF-8 SRT and plain-text WebVTT I/O using only the standard library.
package subtitle

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	ErrInvalidInput = errors.New("subtitle: invalid input")
	ErrUnsupported  = errors.New("subtitle: unsupported content")
	ErrLimit        = errors.New("subtitle: resource limit exceeded")
	ErrOrder        = errors.New("subtitle: cues are not chronological")
	ErrOverlap      = errors.New("subtitle: overlapping cues")
)

// Cue is a value. StartMS/EndMS define a nonempty half-open interval in integer
// milliseconds. Text is plain UTF-8, with LF-separated nonblank lines. ID is an
// optional opaque cue identifier; SRT identifiers must be positive decimal
// integers. A track copies cue values and retains no caller-owned cue slices.
type Cue struct {
	ID      string
	StartMS int64
	EndMS   int64
	Text    string
}

// CueAt adds an offset to a scene time and a positive duration, all in exact
// milliseconds. Signed offsets are allowed if the resulting start is nonnegative.
// Arithmetic overflow, invalid text and default resource limits are checked.
func CueAt(text string, nowMS, durationMS, offsetMS int64) (Cue, error) {
	if nowMS < 0 || durationMS <= 0 || offsetMS < -nowMS || (offsetMS > 0 && nowMS > math.MaxInt64-offsetMS) {
		return Cue{}, fmt.Errorf("%w: invalid scene time, duration or offset", ErrInvalidInput)
	}
	start := nowMS + offsetMS
	if start < 0 || start > math.MaxInt64-durationMS {
		return Cue{}, fmt.Errorf("%w: cue time overflow or negative start", ErrInvalidInput)
	}
	cue := Cue{StartMS: start, EndMS: start + durationMS, Text: text}
	if err := validateCue(cue, DefaultLimits()); err != nil {
		return Cue{}, err
	}
	return cue, nil
}

// Limits bound cue count, per-cue text bytes, total text+ID bytes, individual
// encoded lines, identifiers, encoded stream bytes, and timeline extent.
// Zero fields select defaults; negative fields and MaxBytes==MaxInt64 are errors.
type Limits struct {
	MaxCues        int
	MaxTextBytes   int
	MaxTotalBytes  int64
	MaxLineBytes   int
	MaxIDBytes     int
	MaxBytes       int64
	MaxTimestampMS int64
}

func DefaultLimits() Limits {
	return Limits{
		MaxCues: 10000, MaxTextBytes: 64 << 10, MaxTotalBytes: 16 << 20,
		MaxLineBytes: 16 << 10, MaxIDBytes: 1024, MaxBytes: 32 << 20,
		MaxTimestampMS: 24 * 60 * 60 * 1000,
	}
}

func (l Limits) normalized() (Limits, error) {
	if l.MaxCues < 0 || l.MaxTextBytes < 0 || l.MaxTotalBytes < 0 || l.MaxLineBytes < 0 || l.MaxIDBytes < 0 || l.MaxBytes < 0 || l.MaxBytes == math.MaxInt64 || l.MaxTimestampMS < 0 {
		return l, fmt.Errorf("%w: invalid resource limit", ErrInvalidInput)
	}
	d := DefaultLimits()
	if l.MaxCues == 0 {
		l.MaxCues = d.MaxCues
	}
	if l.MaxTextBytes == 0 {
		l.MaxTextBytes = d.MaxTextBytes
	}
	if l.MaxTotalBytes == 0 {
		l.MaxTotalBytes = d.MaxTotalBytes
	}
	if l.MaxLineBytes == 0 {
		l.MaxLineBytes = d.MaxLineBytes
	}
	if l.MaxIDBytes == 0 {
		l.MaxIDBytes = d.MaxIDBytes
	}
	if l.MaxBytes == 0 {
		l.MaxBytes = d.MaxBytes
	}
	if l.MaxTimestampMS == 0 {
		l.MaxTimestampMS = d.MaxTimestampMS
	}
	return l, nil
}

type OrderPolicy uint8

const (
	// OrderStrict rejects decreasing start times; equal start times are stable.
	OrderStrict OrderPolicy = iota
	// OrderStableSort stably sorts by start time, retaining order on equal starts.
	OrderStableSort
)

type OverlapPolicy uint8

const (
	// OverlapReject rejects any intersecting half-open intervals.
	OverlapReject OverlapPolicy = iota
	OverlapAllow
)

type Options struct {
	Order   OrderPolicy
	Overlap OverlapPolicy
	Limits  Limits
}

func (o Options) normalized() (Options, error) {
	if o.Order > OrderStableSort || o.Overlap > OverlapAllow {
		return o, fmt.Errorf("%w: unknown order/overlap policy", ErrInvalidInput)
	}
	var err error
	o.Limits, err = o.Limits.normalized()
	return o, err
}

// Track is an immutable, chronologically ordered cue collection. Construction,
// Append and Renumber return independent cue storage. Strings are immutable Go
// values; existing track strings can be shared by derived tracks. Concurrent
// reads, appends and writes are safe with independently managed readers/writers.
// The zero value is invalid; New(nil, Options{}) creates a valid empty track.
type Track struct {
	cues       []Cue
	options    Options
	totalBytes int64
}

func New(cues []Cue, options Options) (*Track, error) {
	o, err := options.normalized()
	if err != nil {
		return nil, err
	}
	total, err := validateCues(cues, o.Limits)
	if err != nil {
		return nil, err
	}
	owned := append([]Cue(nil), cues...)
	for i := range owned {
		owned[i].Text = strings.Clone(owned[i].Text)
		owned[i].ID = strings.Clone(owned[i].ID)
	}
	return finish(owned, o, total)
}

func finish(owned []Cue, options Options, total int64) (*Track, error) {
	if options.Order == OrderStableSort {
		slices.SortStableFunc(owned, func(a, b Cue) int {
			if a.StartMS < b.StartMS {
				return -1
			}
			if a.StartMS > b.StartMS {
				return 1
			}
			return 0
		})
	}
	var maxEnd int64
	for i, cue := range owned {
		if i != 0 && cue.StartMS < owned[i-1].StartMS {
			return nil, fmt.Errorf("%w: cue %d", ErrOrder, i)
		}
		if options.Overlap == OverlapReject && i != 0 && cue.StartMS < maxEnd {
			return nil, fmt.Errorf("%w: cue %d", ErrOverlap, i)
		}
		maxEnd = max(maxEnd, cue.EndMS)
	}
	return &Track{cues: owned, options: options, totalBytes: total}, nil
}

func (t *Track) validate() error {
	if t == nil || t.options.Limits.MaxCues == 0 {
		return fmt.Errorf("%w: nil or zero Track", ErrInvalidInput)
	}
	return nil
}

func (t *Track) Len() int         { return len(t.cues) }
func (t *Track) Cues() []Cue      { return append([]Cue(nil), t.cues...) }
func (t *Track) Options() Options { return t.options }

func (t *Track) Cue(index int) (Cue, error) {
	if err := t.validate(); err != nil {
		return Cue{}, err
	}
	if index < 0 || index >= len(t.cues) {
		return Cue{}, fmt.Errorf("%w: cue index out of bounds", ErrInvalidInput)
	}
	return t.cues[index], nil
}

// Append applies the original track's limits, ordering and overlap policies.
// No mutation is committed on errors, including duplicate nonempty IDs.
func (t *Track) Append(cues ...Cue) (*Track, error) {
	if err := t.validate(); err != nil {
		return nil, err
	}
	l := t.options.Limits
	if len(cues) > l.MaxCues-len(t.cues) {
		return nil, fmt.Errorf("%w: appended cue count", ErrLimit)
	}
	added, err := validateCues(cues, l)
	if err != nil {
		return nil, err
	}
	if added > l.MaxTotalBytes-t.totalBytes {
		return nil, fmt.Errorf("%w: appended text/identifier bytes", ErrLimit)
	}
	owned := make([]Cue, len(t.cues)+len(cues))
	copy(owned, t.cues)
	copy(owned[len(t.cues):], cues)
	// Check identifiers across both collections before cloning strings.
	if _, err := validateCues(owned, l); err != nil {
		return nil, err
	}
	for i := len(t.cues); i < len(owned); i++ {
		owned[i].Text = strings.Clone(owned[i].Text)
		owned[i].ID = strings.Clone(owned[i].ID)
	}
	return finish(owned, t.options, t.totalBytes+added)
}

// Renumber explicitly replaces all identifiers with 1-based decimal sequence
// IDs. Use this before SRT export when converting opaque WebVTT identifiers.
// Text/timing and order are unchanged; the original track keeps its IDs.
func (t *Track) Renumber() (*Track, error) {
	if err := t.validate(); err != nil {
		return nil, err
	}
	owned := t.Cues()
	for i := range owned {
		owned[i].ID = strconv.Itoa(i + 1)
	}
	total, err := validateCues(owned, t.options.Limits)
	if err != nil {
		return nil, err
	}
	return finish(owned, t.options, total)
}

func validateCues(cues []Cue, l Limits) (int64, error) {
	if len(cues) > l.MaxCues {
		return 0, fmt.Errorf("%w: cue count", ErrLimit)
	}
	ids := make(map[string]struct{}, len(cues))
	var total int64
	for i, cue := range cues {
		if err := validateCue(cue, l); err != nil {
			return 0, fmt.Errorf("subtitle: cue %d: %w", i, err)
		}
		for _, size := range []int{len(cue.Text), len(cue.ID)} {
			if int64(size) > l.MaxTotalBytes-total {
				return 0, fmt.Errorf("%w: total text/identifier bytes", ErrLimit)
			}
			total += int64(size)
		}
		if cue.ID != "" {
			if _, exists := ids[cue.ID]; exists {
				return 0, fmt.Errorf("%w: duplicate cue identifier %q", ErrInvalidInput, cue.ID)
			}
			ids[cue.ID] = struct{}{}
		}
	}
	return total, nil
}

func validateCue(cue Cue, l Limits) error {
	if cue.StartMS < 0 || cue.EndMS <= cue.StartMS {
		return fmt.Errorf("%w: cue requires 0 <= start < end", ErrInvalidInput)
	}
	if cue.EndMS > l.MaxTimestampMS {
		return fmt.Errorf("%w: cue exceeds timeline bound", ErrLimit)
	}
	if len(cue.Text) > l.MaxTextBytes || len(cue.ID) > l.MaxIDBytes || len(cue.ID) > l.MaxLineBytes {
		return fmt.Errorf("%w: cue text/identifier size", ErrLimit)
	}
	if !utf8.ValidString(cue.Text) || !utf8.ValidString(cue.ID) || cue.Text == "" {
		return fmt.Errorf("%w: empty text or invalid UTF-8", ErrInvalidInput)
	}
	if strings.Contains(cue.ID, "-->") || strings.Trim(cue.ID, " \t") == "" && cue.ID != "" {
		return fmt.Errorf("%w: invalid cue identifier", ErrInvalidInput)
	}
	for _, r := range cue.ID {
		if unicode.IsControl(r) {
			return fmt.Errorf("%w: control character in identifier", ErrInvalidInput)
		}
	}
	for _, r := range cue.Text {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return fmt.Errorf("%w: control character in text", ErrInvalidInput)
		}
	}
	for line := range strings.SplitSeq(cue.Text, "\n") {
		if strings.Trim(line, " \t") == "" {
			return fmt.Errorf("%w: blank cue text line", ErrInvalidInput)
		}
		if len(line) > l.MaxLineBytes {
			return fmt.Errorf("%w: cue text line size", ErrLimit)
		}
	}
	return nil
}
