package subtitle

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

type WriteOptions struct {
	Limits Limits
}

// WriteSRT preserves numeric cue IDs. Anonymous cues acquire 1-based positional
// indices; collisions with existing IDs fail. Opaque IDs require an explicit
// Track.Renumber call. All validation and byte/line bounds precede I/O. Output
// is UTF-8 with LF, canonical timestamps and a blank line after each cue.
// Markup-looking text and character references are unsupported in plain SRT.
func WriteSRT(w io.Writer, track *Track, options WriteOptions) error {
	return write(w, track, SRT, options)
}

// WriteWebVTT writes plain text, escaping &, < and > so literal text never turns
// into markup, inline timestamps or cue syntax. IDs, Unicode and all line breaks
// are retained. Unsupported/reserved IDs and escaped lines exceeding the encoded
// limits fail before I/O. A writer error may leave a partially written stream.
func WriteWebVTT(w io.Writer, track *Track, options WriteOptions) error {
	return write(w, track, WebVTT, options)
}

func write(w io.Writer, track *Track, format Format, options WriteOptions) error {
	if w == nil {
		return fmt.Errorf("%w: nil subtitle writer", ErrInvalidInput)
	}
	if err := track.validate(); err != nil {
		return err
	}
	l, err := options.Limits.normalized()
	if err != nil {
		return err
	}
	if _, err := validateCues(track.cues, l); err != nil {
		return err
	}
	ids := make(map[string]struct{}, len(track.cues))
	if format == WebVTT && len("WEBVTT") > l.MaxLineBytes {
		return fmt.Errorf("%w: WebVTT signature line", ErrLimit)
	}
	for i, cue := range track.cues {
		id := outputID(cue, i, format)
		if len(id) > l.MaxIDBytes || len(id) > l.MaxLineBytes {
			return fmt.Errorf("%w: encoded identifier", ErrLimit)
		}
		if format == SRT {
			if err := srtID(id); err != nil {
				return fmt.Errorf("%w: SRT needs numeric IDs; call Track.Renumber explicitly: %w", ErrUnsupported, err)
			}
			if _, exists := ids[id]; exists {
				return fmt.Errorf("%w: generated SRT index conflicts with explicit ID %q", ErrInvalidInput, id)
			}
			ids[id] = struct{}{}
			if err := srtText(cue.Text); err != nil {
				return err
			}
		} else if reservedBlock(id) {
			return fmt.Errorf("%w: reserved WebVTT identifier %q", ErrUnsupported, id)
		}
		start, _ := FormatTimestamp(cue.StartMS, format)
		end, _ := FormatTimestamp(cue.EndMS, format)
		if len(start)+5+len(end) > l.MaxLineBytes {
			return fmt.Errorf("%w: encoded timing line", ErrLimit)
		}
		if format == WebVTT {
			for line := range strings.SplitSeq(cue.Text, "\n") {
				if err := checkEscapedLine(line, l.MaxLineBytes); err != nil {
					return err
				}
			}
		}
	}
	// The same serializer runs against a counting sink before writing a byte.
	// No fully encoded subtitle file is materialized for size validation.
	if err := serialize(&byteCounter{remaining: l.MaxBytes}, track.cues, format); err != nil {
		return err
	}
	buffered := bufio.NewWriterSize(w, 4096)
	if err := serialize(buffered, track.cues, format); err != nil {
		return err
	}
	if err := buffered.Flush(); err != nil {
		return fmt.Errorf("subtitle: write: %w", err)
	}
	return nil
}

func outputID(cue Cue, index int, format Format) string {
	if format == SRT && cue.ID == "" {
		return strconv.Itoa(index + 1)
	}
	return cue.ID
}

func serialize(w io.Writer, cues []Cue, format Format) error {
	if format == WebVTT {
		if err := writeString(w, "WEBVTT\n\n"); err != nil {
			return err
		}
	}
	for i, cue := range cues {
		if id := outputID(cue, i, format); id != "" {
			if err := writeString(w, id+"\n"); err != nil {
				return err
			}
		}
		start, _ := FormatTimestamp(cue.StartMS, format)
		end, _ := FormatTimestamp(cue.EndMS, format)
		if err := writeString(w, start+" --> "+end+"\n"); err != nil {
			return err
		}
		if format == WebVTT {
			if err := writeEscaped(w, cue.Text); err != nil {
				return err
			}
		} else if err := writeString(w, cue.Text); err != nil {
			return err
		}
		if err := writeString(w, "\n\n"); err != nil {
			return err
		}
	}
	return nil
}

func escapedByte(b byte) string {
	switch b {
	case '&':
		return "&amp;"
	case '<':
		return "&lt;"
	case '>':
		return "&gt;"
	}
	return ""
}

func checkEscapedLine(line string, maxBytes int) error {
	remaining := maxBytes
	for i := 0; i < len(line); i++ {
		needed := 1
		if escape := escapedByte(line[i]); escape != "" {
			needed = len(escape)
		}
		if needed > remaining {
			return fmt.Errorf("%w: escaped WebVTT line", ErrLimit)
		}
		remaining -= needed
	}
	return nil
}

func writeEscaped(w io.Writer, text string) error {
	start := 0
	for i := 0; i < len(text); i++ {
		if escape := escapedByte(text[i]); escape != "" {
			if err := writeString(w, text[start:i]); err != nil {
				return err
			}
			if err := writeString(w, escape); err != nil {
				return err
			}
			start = i + 1
		}
	}
	return writeString(w, text[start:])
}

func writeString(w io.Writer, text string) error {
	n, err := io.WriteString(w, text)
	if err != nil {
		return fmt.Errorf("subtitle: write: %w", err)
	}
	if n != len(text) {
		return fmt.Errorf("subtitle: write: %w", io.ErrShortWrite)
	}
	return nil
}

type byteCounter struct{ remaining int64 }

func (w *byteCounter) Write(data []byte) (int, error)       { return w.count(len(data)) }
func (w *byteCounter) WriteString(text string) (int, error) { return w.count(len(text)) }

func (w *byteCounter) count(n int) (int, error) {
	if int64(n) > w.remaining {
		return 0, fmt.Errorf("%w: encoded output bytes", ErrLimit)
	}
	w.remaining -= int64(n)
	return n, nil
}
