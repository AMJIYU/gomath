package subtitle

import (
	"errors"
	"math"
	"slices"
	"strings"
	"testing"
)

func mustTrack(t testing.TB, cues []Cue, options Options) *Track {
	t.Helper()
	track, err := New(cues, options)
	if err != nil {
		t.Fatal(err)
	}
	return track
}

func TestImmutableAppendSortAndOverlapPolicies(t *testing.T) {
	original := []Cue{{ID: "late", StartMS: 1000, EndMS: 2000, Text: "later"}}
	track := mustTrack(t, original, Options{Order: OrderStableSort})
	original[0].Text = "changed"
	copy := track.Cues()
	copy[0].Text = "changed"
	added := []Cue{{ID: "early", StartMS: 0, EndMS: 1000, Text: "earlier"}}
	result, err := track.Append(added...)
	if err != nil {
		t.Fatal(err)
	}
	added[0].Text = "changed"
	if track.Len() != 1 || result.Len() != 2 || track.Cues()[0].Text != "later" || result.Cues()[0].Text != "earlier" {
		t.Fatal("constructor, accessors or append alias caller storage")
	}
	result.cues[1].Text = "changed" // Verify separate result cue arrays.
	if track.cues[0].Text != "later" {
		t.Fatal("derived track aliases original cue array")
	}
	if _, err := track.Append(Cue{ID: "late", StartMS: 3000, EndMS: 4000, Text: "duplicate"}); !errors.Is(err, ErrInvalidInput) || track.Len() != 1 {
		t.Fatal("duplicate ID append mutated source", err)
	}
	strict := mustTrack(t, []Cue{{StartMS: 1000, EndMS: 2000, Text: "x"}}, Options{})
	if _, err := strict.Append(Cue{StartMS: 0, EndMS: 1000, Text: "y"}); !errors.Is(err, ErrOrder) {
		t.Fatal("strict order silently sorted", err)
	}
	if _, err := strict.Append(Cue{StartMS: 1500, EndMS: 2500, Text: "overlap"}); !errors.Is(err, ErrOverlap) {
		t.Fatal("overlap silently repaired", err)
	}
	// Adjacent intervals meet at exactly one boundary and do not overlap.
	if _, err := strict.Append(Cue{StartMS: 2000, EndMS: 2001, Text: "adjacent"}); err != nil {
		t.Fatal(err)
	}
	unsorted := []Cue{
		{ID: "c", StartMS: 20, EndMS: 30, Text: "c"},
		{ID: "a", StartMS: 10, EndMS: 100, Text: "a"},
		{ID: "b", StartMS: 10, EndMS: 12, Text: "b"},
	}
	allowed := mustTrack(t, unsorted, Options{Order: OrderStableSort, Overlap: OverlapAllow})
	if got := allowed.Cues(); got[0].ID != "a" || got[1].ID != "b" || got[2].ID != "c" || unsorted[0].ID != "c" {
		t.Fatal("sort is not stable or changed original order", got)
	}
	if _, err := New(unsorted, Options{Order: OrderStableSort}); !errors.Is(err, ErrOverlap) {
		t.Fatal("sorting lost overlapping cue", err)
	}
	numbered, err := allowed.Renumber()
	if err != nil || numbered.cues[0].ID != "1" || numbered.cues[2].ID != "3" || allowed.cues[0].ID != "a" {
		t.Fatal("explicit renumbering failed", err)
	}
	if got, err := allowed.Cue(1); err != nil || got != allowed.cues[1] {
		t.Fatal("indexed access", err)
	}
	for _, i := range []int{-1, 3} {
		if _, err := allowed.Cue(i); !errors.Is(err, ErrInvalidInput) {
			t.Fatal("invalid cue index", err)
		}
	}
	if allowed.Options().Order != OrderStableSort || allowed.Options().Limits.MaxCues == 0 {
		t.Fatal("stored policies unavailable")
	}
	clone, err := allowed.Append()
	if err != nil || !slices.Equal(clone.cues, allowed.cues) {
		t.Fatal("empty append", err)
	}
}

func TestCueAndCollectionValidation(t *testing.T) {
	for _, cue := range []Cue{
		{StartMS: -1, EndMS: 1, Text: "x"}, {StartMS: 1, EndMS: 1, Text: "x"},
		{StartMS: 2, EndMS: 1, Text: "x"}, {EndMS: 1}, {EndMS: 1, Text: "\xff"},
		{EndMS: 1, Text: "a\x00b"}, {EndMS: 1, Text: "a\rb"}, {EndMS: 1, Text: "a\x7fb"},
		{EndMS: 1, Text: "a\u0085b"}, {EndMS: 1, Text: "a\n\nb"}, {EndMS: 1, Text: "a\n \tb" + "\n \t"},
		{EndMS: 1, Text: "\nx"}, {EndMS: 1, Text: "x\n"}, {EndMS: 1, Text: " \t"},
		{ID: "\xff", EndMS: 1, Text: "x"}, {ID: "a\nb", EndMS: 1, Text: "x"},
		{ID: "a\tb", EndMS: 1, Text: "x"}, {ID: "-->", EndMS: 1, Text: "x"}, {ID: "  ", EndMS: 1, Text: "x"},
	} {
		if _, err := New([]Cue{cue}, Options{}); !errors.Is(err, ErrInvalidInput) {
			t.Fatal("invalid cue accepted", cue, err)
		}
	}
	valid := Cue{ID: "x", StartMS: 1, EndMS: 2, Text: "你好\n  preserved \t"}
	mustTrack(t, []Cue{valid}, Options{})
	for _, options := range []Options{
		{Order: 99}, {Overlap: 99}, {Limits: Limits{MaxCues: -1}}, {Limits: Limits{MaxTextBytes: -1}},
		{Limits: Limits{MaxTotalBytes: -1}}, {Limits: Limits{MaxLineBytes: -1}}, {Limits: Limits{MaxIDBytes: -1}},
		{Limits: Limits{MaxBytes: -1}}, {Limits: Limits{MaxBytes: math.MaxInt64}}, {Limits: Limits{MaxTimestampMS: -1}},
	} {
		if _, err := New(nil, options); !errors.Is(err, ErrInvalidInput) {
			t.Fatal("invalid options accepted", options, err)
		}
	}
	for _, limits := range []Limits{
		{MaxTextBytes: 1}, {MaxTotalBytes: 1}, {MaxLineBytes: 1}, {MaxTimestampMS: 1},
	} {
		if _, err := New([]Cue{valid}, Options{Limits: limits}); !errors.Is(err, ErrLimit) {
			t.Fatal("cue limit ignored", limits, err)
		}
	}
	if _, err := New([]Cue{{ID: "ab", EndMS: 1, Text: "x"}}, Options{Limits: Limits{MaxIDBytes: 1}}); !errors.Is(err, ErrLimit) {
		t.Fatal("ID size ignored", err)
	}
	if _, err := New([]Cue{valid, valid}, Options{Limits: Limits{MaxCues: 1}}); !errors.Is(err, ErrLimit) {
		t.Fatal("cue count ignored", err)
	}
	if _, err := New([]Cue{valid, valid}, Options{Overlap: OverlapAllow}); !errors.Is(err, ErrInvalidInput) {
		t.Fatal("duplicate IDs", err)
	}
	for _, track := range []*Track{nil, {}} {
		if _, err := track.Append(); !errors.Is(err, ErrInvalidInput) {
			t.Fatal("invalid append receiver", err)
		}
		if _, err := track.Renumber(); !errors.Is(err, ErrInvalidInput) {
			t.Fatal("invalid renumber receiver", err)
		}
		if _, err := track.Cue(0); !errors.Is(err, ErrInvalidInput) {
			t.Fatal("invalid cue receiver", err)
		}
	}
	empty := mustTrack(t, nil, Options{})
	if empty.Len() != 0 {
		t.Fatal("invalid empty track")
	}
	bounded := mustTrack(t, []Cue{{EndMS: 1, Text: "x"}}, Options{Limits: Limits{MaxTotalBytes: 2}})
	if _, err := bounded.Append(Cue{StartMS: 1, EndMS: 2, Text: "xx"}); !errors.Is(err, ErrLimit) {
		t.Fatal("append total byte budget ignored", err)
	}
	if _, err := empty.Append(Cue{EndMS: 1, Text: strings.Repeat("x", 1<<17)}); !errors.Is(err, ErrLimit) {
		t.Fatal("append cue size ignored", err)
	}
}
