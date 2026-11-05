package subtitle

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

type Format uint8

const (
	SRT Format = iota + 1
	WebVTT
)

// ParseTimestamp parses exact milliseconds, with no floating-point arithmetic.
// SRT requires HH:MM:SS,mmm; WebVTT accepts HH:MM:SS.mmm or MM:SS.mmm.
// Hours have at least two decimal digits, minutes/seconds exactly two in 0..59,
// and milliseconds exactly three. Signs, whitespace, decimal rounding and
// int64 overflow are rejected. Stream readers additionally apply timeline limits.
func ParseTimestamp(text string, format Format) (int64, error) {
	if format != SRT && format != WebVTT {
		return 0, fmt.Errorf("%w: timestamp format", ErrUnsupported)
	}
	// An int64 millisecond timestamp never needs more than 13 hour digits.
	if len(text) > 23 {
		return 0, fmt.Errorf("%w: timestamp too long", ErrInvalidInput)
	}
	parts := strings.Split(text, ":")
	if len(parts) == 2 && format == WebVTT {
		parts = append([]string{"00"}, parts...)
	}
	if len(parts) != 3 || len(parts[0]) < 2 || len(parts[1]) != 2 || len(parts[2]) != 6 {
		return 0, fmt.Errorf("%w: invalid timestamp shape %q", ErrInvalidInput, text)
	}
	separator := byte(',')
	if format == WebVTT {
		separator = '.'
	}
	if parts[2][2] != separator {
		return 0, fmt.Errorf("%w: timestamp decimal separator", ErrInvalidInput)
	}
	hours, err := decimal(parts[0])
	if err != nil {
		return 0, err
	}
	minutes, err := decimal(parts[1])
	if err != nil {
		return 0, err
	}
	seconds, err := decimal(parts[2][:2])
	if err != nil {
		return 0, err
	}
	millis, err := decimal(parts[2][3:])
	if err != nil {
		return 0, err
	}
	if minutes > 59 || seconds > 59 {
		return 0, fmt.Errorf("%w: timestamp minute/second out of range", ErrInvalidInput)
	}
	tail := minutes*60000 + seconds*1000 + millis
	if hours > (math.MaxInt64-tail)/3600000 {
		return 0, fmt.Errorf("%w: timestamp arithmetic overflow", ErrInvalidInput)
	}
	return hours*3600000 + tail, nil
}

func decimal(text string) (int64, error) {
	if text == "" {
		return 0, fmt.Errorf("%w: empty decimal", ErrInvalidInput)
	}
	for _, b := range []byte(text) {
		if b < '0' || b > '9' {
			return 0, fmt.Errorf("%w: non-ASCII decimal", ErrInvalidInput)
		}
	}
	n, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: decimal overflow", ErrInvalidInput)
	}
	return n, nil
}

// FormatTimestamp emits at least two hour digits and exactly three fractional
// digits. Nonnegative int64 millisecond values round-trip exactly.
func FormatTimestamp(milliseconds int64, format Format) (string, error) {
	if milliseconds < 0 {
		return "", fmt.Errorf("%w: negative timestamp", ErrInvalidInput)
	}
	separator := ','
	if format == WebVTT {
		separator = '.'
	} else if format != SRT {
		return "", fmt.Errorf("%w: timestamp format", ErrUnsupported)
	}
	return fmt.Sprintf("%02d:%02d:%02d%c%03d", milliseconds/3600000, milliseconds/60000%60, milliseconds/1000%60, separator, milliseconds%1000), nil
}
