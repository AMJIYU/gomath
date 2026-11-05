package subtitle

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"
)

// ReadSRT reads a bounded UTF-8 SubRip stream. Indices are canonical positive
// decimal uint64 IDs, retained independently of sorting. Timing extensions,
// HTML-like markup/character references, malformed text and duplicates fail.
// LF and CRLF line endings and an initial UTF-8 BOM are accepted; output text
// uses LF. Empty files are valid. No malformed cue is skipped or repaired.
func ReadSRT(r io.Reader, options Options) (*Track, error) {
	return read(r, SRT, options)
}

// ReadWebVTT reads the plain-text subset of WebVTT: WEBVTT + blank header line,
// optional identifiers, cue timings and multi-line UTF-8 text. The six named
// entities amp/lt/gt/nbsp/lrm/rlm are decoded as literal text. Tags (including
// inline timestamps), cue settings, STYLE/REGION/NOTE blocks, header metadata
// and unsupported references return explicit errors; nothing is stripped.
func ReadWebVTT(r io.Reader, options Options) (*Track, error) {
	return read(r, WebVTT, options)
}

type lineReader struct {
	r       *bufio.Reader
	limited *io.LimitedReader
	limits  Limits
	first   bool
	line    int
}

func newLineReader(r io.Reader, l Limits) *lineReader {
	limited := &io.LimitedReader{R: r, N: l.MaxBytes + 1}
	// Fixed scratch space, independent of attacker-controlled line sizes.
	return &lineReader{r: bufio.NewReaderSize(limited, 4096), limited: limited, limits: l, first: true}
}

func (r *lineReader) next() (string, error) {
	var data []byte
	for {
		part, err := r.r.ReadSlice('\n')
		if r.limited.N == 0 {
			return "", fmt.Errorf("%w: encoded stream bytes", ErrLimit)
		}
		// Allow CRLF and a first-line BOM beyond the logical line-byte limit.
		extra := 2
		if r.first {
			extra += 3
		}
		if uint64(len(data))+uint64(len(part)) > uint64(r.limits.MaxLineBytes)+uint64(extra) {
			return "", fmt.Errorf("%w: encoded line %d", ErrLimit, r.line+1)
		}
		data = append(data, part...)
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		if err != nil && !errors.Is(err, io.EOF) {
			return "", fmt.Errorf("subtitle: line %d: %w", r.line+1, err)
		}
		if len(data) == 0 && errors.Is(err, io.EOF) {
			return "", io.EOF
		}
		break
	}
	if data[len(data)-1] == '\n' {
		data = data[:len(data)-1]
		if len(data) > 0 && data[len(data)-1] == '\r' {
			data = data[:len(data)-1]
		}
	}
	if r.first {
		data = []byte(strings.TrimPrefix(string(data), "\ufeff"))
		r.first = false
	}
	r.line++
	if len(data) > r.limits.MaxLineBytes {
		return "", fmt.Errorf("%w: encoded line %d", ErrLimit, r.line)
	}
	if !utf8.Valid(data) || strings.ContainsRune(string(data), '\r') {
		return "", fmt.Errorf("%w: line %d has invalid UTF-8 or bare CR", ErrInvalidInput, r.line)
	}
	return string(data), nil
}

func (r *lineReader) nonempty() (string, error) {
	for {
		line, err := r.next()
		if err != nil || line != "" {
			return line, err
		}
	}
}

func read(r io.Reader, format Format, options Options) (*Track, error) {
	if r == nil {
		return nil, fmt.Errorf("%w: nil subtitle reader", ErrInvalidInput)
	}
	o, err := options.normalized()
	if err != nil {
		return nil, err
	}
	lines := newLineReader(r, o.Limits)
	if format == WebVTT {
		header, err := lines.next()
		if err != nil {
			return nil, fmt.Errorf("subtitle: WebVTT signature: %w", err)
		}
		if header != "WEBVTT" {
			if strings.HasPrefix(header, "WEBVTT ") || strings.HasPrefix(header, "WEBVTT\t") {
				return nil, fmt.Errorf("%w: WebVTT header annotation", ErrUnsupported)
			}
			return nil, fmt.Errorf("%w: missing WEBVTT signature", ErrInvalidInput)
		}
		blank, err := lines.next()
		if err != nil {
			return nil, fmt.Errorf("%w: missing WebVTT header terminator: %w", ErrInvalidInput, err)
		}
		if blank != "" {
			return nil, fmt.Errorf("%w: WebVTT header metadata", ErrUnsupported)
		}
	}
	var (
		cues  []Cue
		total int64
	)
	for {
		first, err := lines.nonempty()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if len(cues) >= o.Limits.MaxCues {
			return nil, fmt.Errorf("%w: cue count", ErrLimit)
		}
		cue := Cue{}
		timing := first
		if format == SRT {
			if err := srtID(first); err != nil {
				return nil, err
			}
			cue.ID = first
		} else {
			if reservedBlock(first) {
				return nil, fmt.Errorf("%w: WebVTT %s block", ErrUnsupported, first)
			}
			if !strings.Contains(first, "-->") {
				cue.ID = first
			}
		}
		if cue.ID != "" {
			if len(cue.ID) > o.Limits.MaxIDBytes {
				return nil, fmt.Errorf("%w: identifier bytes", ErrLimit)
			}
			timing, err = lines.next()
			if err != nil {
				return nil, fmt.Errorf("subtitle: missing cue timing: %w", err)
			}
		}
		cue.StartMS, cue.EndMS, err = parseTimingLine(timing, format)
		if err != nil {
			return nil, err
		}
		if cue.StartMS < 0 || cue.EndMS <= cue.StartMS {
			return nil, fmt.Errorf("%w: cue requires start < end", ErrInvalidInput)
		}
		if cue.EndMS > o.Limits.MaxTimestampMS {
			return nil, fmt.Errorf("%w: cue timeline extent", ErrLimit)
		}
		remaining := o.Limits.MaxTotalBytes - total
		if int64(len(cue.ID)) > remaining {
			return nil, fmt.Errorf("%w: total identifier bytes", ErrLimit)
		}
		cue.Text, err = readText(lines, format, min(int64(o.Limits.MaxTextBytes), remaining-int64(len(cue.ID))))
		if err != nil {
			return nil, err
		}
		if err := validateCue(cue, o.Limits); err != nil {
			return nil, err
		}
		total += int64(len(cue.ID)) + int64(len(cue.Text))
		cues = append(cues, cue)
	}
	if _, err := validateCues(cues, o.Limits); err != nil {
		return nil, err
	}
	return finish(cues, o, total)
}

func parseTimingLine(line string, format Format) (int64, int64, error) {
	fields := strings.FieldsFunc(line, func(r rune) bool { return r == ' ' || r == '\t' })
	if len(fields) < 3 || fields[1] != "-->" {
		return 0, 0, fmt.Errorf("%w: malformed timing line", ErrInvalidInput)
	}
	if len(fields) != 3 {
		return 0, 0, fmt.Errorf("%w: cue settings or timing extensions", ErrUnsupported)
	}
	start, err := ParseTimestamp(fields[0], format)
	if err != nil {
		return 0, 0, err
	}
	end, err := ParseTimestamp(fields[2], format)
	return start, end, err
}

func readText(lines *lineReader, format Format, budget int64) (string, error) {
	var text strings.Builder
	for {
		line, err := lines.next()
		if err != nil && !errors.Is(err, io.EOF) {
			return "", err
		}
		if errors.Is(err, io.EOF) || line == "" {
			break
		}
		if format == WebVTT {
			line, err = decodeVTTText(line)
		} else {
			err = srtText(line)
		}
		if err != nil {
			return "", err
		}
		needed := int64(len(line))
		if text.Len() != 0 {
			needed++
		}
		if needed > budget-int64(text.Len()) {
			return "", fmt.Errorf("%w: cue/total text bytes", ErrLimit)
		}
		if text.Len() != 0 {
			text.WriteByte('\n')
		}
		text.WriteString(line)
	}
	if text.Len() == 0 {
		return "", fmt.Errorf("%w: cue has no text", ErrInvalidInput)
	}
	return text.String(), nil
}

func srtID(id string) error {
	n, err := strconv.ParseUint(id, 10, 64)
	if err != nil || n == 0 || strconv.FormatUint(n, 10) != id {
		return fmt.Errorf("%w: SRT index must be a canonical positive decimal uint64", ErrInvalidInput)
	}
	return nil
}

func reservedBlock(id string) bool {
	for _, word := range []string{"NOTE", "STYLE", "REGION"} {
		if id == word || strings.HasPrefix(id, word+" ") || strings.HasPrefix(id, word+"\t") {
			return true
		}
	}
	return false
}

func srtText(text string) error {
	if strings.ContainsRune(text, '<') {
		return fmt.Errorf("%w: SRT angle-bracket markup is outside the plain-text subset", ErrUnsupported)
	}
	// Entity-looking text is ambiguous across SubRip players. Preserve it by
	// refusing implicit interpretation; use WebVTT for escaped literal text.
	for i := 0; i < len(text); i++ {
		if text[i] != '&' {
			continue
		}
		for j := i + 1; j < len(text); j++ {
			b := text[j]
			if b == ';' && j > i+1 {
				return fmt.Errorf("%w: SRT character reference", ErrUnsupported)
			}
			if !(b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '#') {
				break
			}
		}
	}
	return nil
}

func decodeVTTText(text string) (string, error) {
	if strings.ContainsRune(text, '<') {
		return "", fmt.Errorf("%w: WebVTT markup or inline timestamp", ErrUnsupported)
	}
	if strings.Contains(text, "-->") {
		return "", fmt.Errorf("%w: unescaped WebVTT cue arrow", ErrInvalidInput)
	}
	var decoded strings.Builder
	for i := 0; i < len(text); {
		if text[i] != '&' {
			decoded.WriteByte(text[i])
			i++
			continue
		}
		matched := false
		for _, entity := range vttEntities {
			if strings.HasPrefix(text[i:], entity.encoded) {
				decoded.WriteString(entity.decoded)
				i += len(entity.encoded)
				matched = true
				break
			}
		}
		if !matched {
			return "", fmt.Errorf("%w: WebVTT unknown, numeric or unescaped character reference", ErrUnsupported)
		}
	}
	return decoded.String(), nil
}

var vttEntities = [...]struct{ encoded, decoded string }{
	{"&amp;", "&"}, {"&lt;", "<"}, {"&gt;", ">"},
	{"&nbsp;", "\u00a0"}, {"&lrm;", "\u200e"}, {"&rlm;", "\u200f"},
}
