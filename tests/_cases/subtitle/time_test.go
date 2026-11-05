package subtitle

import (
	"errors"
	"math"
	"testing"
)

func TestExactMillisecondTimestamps(t *testing.T) {
	for _, test := range []struct {
		text   string
		format Format
		ms     int64
	}{
		{"00:00:00,000", SRT, 0}, {"00:00:00,001", SRT, 1},
		{"00:00:59,999", SRT, 59999}, {"00:01:00,000", SRT, 60000},
		{"01:02:03,004", SRT, 3723004}, {"100:00:00,001", SRT, 360000001},
		{"00:00.001", WebVTT, 1}, {"59:59.999", WebVTT, 3599999},
		{"01:02:03.004", WebVTT, 3723004}, {"100:00:00.001", WebVTT, 360000001},
		{"2562047788015:12:55,807", SRT, math.MaxInt64},
		{"2562047788015:12:55.807", WebVTT, math.MaxInt64},
	} {
		got, err := ParseTimestamp(test.text, test.format)
		if err != nil || got != test.ms {
			t.Fatal(test.text, got, test.ms, err)
		}
		canonical, err := FormatTimestamp(test.ms, test.format)
		if err != nil {
			t.Fatal(err)
		}
		back, err := ParseTimestamp(canonical, test.format)
		if err != nil || back != test.ms {
			t.Fatal("formatted timestamp did not round-trip", canonical, back, err)
		}
	}
	got, _ := FormatTimestamp(3723004, SRT)
	if got != "01:02:03,004" {
		t.Fatal("SRT format", got)
	}
	got, _ = FormatTimestamp(1, WebVTT)
	if got != "00:00:00.001" {
		t.Fatal("WebVTT format", got)
	}
}

func TestRejectMalformedTimestamps(t *testing.T) {
	for _, format := range []Format{SRT, WebVTT} {
		for _, text := range []string{
			"", "-01:00:00,000", "+01:00:00,000", "1:02:03,004", "001:02:03,0000",
			"00:60:00,000", "00:00:60,000", "00:00:00,00", "00:00:00,0000",
			"00:00:00,00x", "00:00:00,000 ", " 00:00:00,000", "00:00,000",
			"00:00:00,000 -->", "2562047788015:12:55,808", "999999999999999999999:00:00,000",
			"00:60.000", "60:00.000", "0:00.000", "00:00.00", "00:00.0000",
			"00:00:60.000", "00:60:00.000", "2562047788015:12:55.808",
			"٠٠:00:00,000", "00：00：00,000", "00:00:00\x00,000",
		} {
			if _, err := ParseTimestamp(text, format); err == nil {
				t.Fatal("malformed timestamp accepted", format, text)
			}
		}
	}
	if _, err := ParseTimestamp("00:00:00.000", SRT); !errors.Is(err, ErrInvalidInput) {
		t.Fatal("wrong SRT decimal separator", err)
	}
	if _, err := ParseTimestamp("00:00:00,000", WebVTT); !errors.Is(err, ErrInvalidInput) {
		t.Fatal("wrong VTT decimal separator", err)
	}
	if _, err := ParseTimestamp("00:00:00,000", 99); !errors.Is(err, ErrUnsupported) {
		t.Fatal("unknown timestamp format", err)
	}
	if _, err := FormatTimestamp(-1, SRT); !errors.Is(err, ErrInvalidInput) {
		t.Fatal("negative timestamp accepted", err)
	}
	if _, err := FormatTimestamp(0, 99); !errors.Is(err, ErrUnsupported) {
		t.Fatal("unknown format accepted", err)
	}
}

func TestCueAtOffsetDurationAndOverflow(t *testing.T) {
	for _, test := range []struct {
		now, duration, offset int64
		want                  Cue
	}{
		{1000, 1234, -999, Cue{StartMS: 1, EndMS: 1235, Text: "你好\nnext"}},
		{1000, 1, -1000, Cue{StartMS: 0, EndMS: 1, Text: "你好\nnext"}},
		{1000, 2, 3, Cue{StartMS: 1003, EndMS: 1005, Text: "你好\nnext"}},
		{math.MaxInt64, 1, -math.MaxInt64, Cue{StartMS: 0, EndMS: 1, Text: "你好\nnext"}},
	} {
		got, err := CueAt("你好\nnext", test.now, test.duration, test.offset)
		if err != nil || got != test.want {
			t.Fatal("offset arithmetic", got, err)
		}
	}
	for _, times := range [][3]int64{
		{-1, 1, 1}, {1, 0, 0}, {1, -1, 0}, {0, 1, -1},
		{math.MaxInt64, 1, 1}, {math.MaxInt64, 1, 0}, {math.MaxInt64 - 1, 2, 0},
		{math.MaxInt64, 1, math.MinInt64}, {0, 1, math.MinInt64},
	} {
		if _, err := CueAt("x", times[0], times[1], times[2]); !errors.Is(err, ErrInvalidInput) {
			t.Fatal("invalid or overflowing time accepted", times, err)
		}
	}
	if _, err := CueAt("x", DefaultLimits().MaxTimestampMS, 1, 0); !errors.Is(err, ErrLimit) {
		t.Fatal("CueAt ignored default timeline limit", err)
	}
	if _, err := CueAt("", 0, 1, 0); !errors.Is(err, ErrInvalidInput) {
		t.Fatal("empty cue text", err)
	}
}
