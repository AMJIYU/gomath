package gomath_test

import (
	"errors"
	"gomath"
	"gomath/subtitle"
	"math"
	"reflect"
	"sync"
	"testing"
)

func TestSubcaptionTimelineRoundingPoliciesAndReplay(t *testing.T) {
	s := gomath.NewScene()
	must(t, s.Wait(.1255))
	must(t, s.AddSubcaption("后添加的尾段\n第二行", .7501, .5))
	must(t, s.AddSubcaptionMS("先开始", 500, -100))
	must(t, s.AddSubcaptionMS("相同起点保持顺序", 100, -100))
	near(t, s.Duration(), .1255)
	options := subtitle.Options{Order: subtitle.OrderStableSort, Overlap: subtitle.OverlapAllow}
	track, err := s.Subcaptions(options)
	must(t, err)
	want := []subtitle.Cue{
		{StartMS: 26, EndMS: 526, Text: "先开始"},
		{StartMS: 26, EndMS: 126, Text: "相同起点保持顺序"},
		{StartMS: 626, EndMS: 1376, Text: "后添加的尾段\n第二行"},
	}
	if !reflect.DeepEqual(track.Cues(), want) {
		t.Fatal("absolute boundaries/order/tails differ", track.Cues())
	}
	if _, err := s.Subcaptions(subtitle.Options{}); !errors.Is(err, subtitle.ErrOverlap) {
		t.Fatal("strict export accepted overlap", err)
	}
	replay, err := gomath.CompileReplay(s, gomath.DefaultReplayOptions())
	must(t, err)
	must(t, s.AddSubcaptionMS("之后的编排", 100, 1000))
	owned := track.Cues()
	owned[0].Text = "mutated"
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := replay.Subcaptions(options)
			if err != nil || !reflect.DeepEqual(got.Cues(), want) {
				t.Error("frozen replay captions changed", err)
			}
		}()
	}
	wg.Wait()
}

func TestSubcaptionValidationDoesNotChangeScene(t *testing.T) {
	s := gomath.NewScene()
	for _, c := range []struct {
		text             string
		duration, offset float64
	}{
		{"", 1, 0}, {"\xff", 1, 0}, {"blank\n\nline", 1, 0},
		{"x", 0, 0}, {"x", -1, 0}, {"x", .0001, 0},
		{"x", 1, -.1}, {"x", math.NaN(), 0},
		{"x", 1, math.Inf(1)}, {"x", 1e100, 0},
	} {
		if err := s.AddSubcaption(c.text, c.duration, c.offset); err == nil {
			t.Fatal("invalid caption accepted", c)
		}
	}
	for _, p := range [][2]int64{{0, 0}, {1, -1}, {math.MaxInt64, 0}, {1, math.MaxInt64}} {
		if err := s.AddSubcaptionMS("x", p[0], p[1]); err == nil {
			t.Fatal("invalid millisecond cue accepted", p)
		}
	}
	track, err := s.Subcaptions(subtitle.Options{})
	must(t, err)
	if track.Len() != 0 || s.Duration() != 0 {
		t.Fatal("failed caption changed scene")
	}
	var replay *gomath.Replay
	if _, err := replay.Subcaptions(subtitle.Options{}); err == nil {
		t.Fatal("nil replay accepted")
	}
}
