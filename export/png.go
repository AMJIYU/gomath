package export

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
)

func PNG(target string, frame image.Image) (err error) {
	f, e := newAtomicFile(target)
	if e != nil {
		return e
	}
	defer func() {
		if err != nil {
			_ = f.abort()
		}
	}()
	if e := png.Encode(f.file, frame); e != nil {
		return e
	}
	return f.commit()
}

// PNGSequence streams frames into a private directory and publishes it when
// rendering completes. The output path must not exist.
type PNGSequence struct {
	target, temp string
	next         int
	closed       bool
}

func NewPNGSequence(target string) (*PNGSequence, error) {
	if target == "" {
		return nil, fmt.Errorf("empty sequence path")
	}
	if _, e := os.Lstat(target); e == nil {
		return nil, fmt.Errorf("output exists: %s", target)
	} else if !os.IsNotExist(e) {
		return nil, e
	}
	if e := os.MkdirAll(filepath.Dir(target), 0755); e != nil {
		return nil, e
	}
	tmp, e := os.MkdirTemp(filepath.Dir(target), ".gomath-frames-*")
	if e != nil {
		return nil, e
	}
	return &PNGSequence{target: target, temp: tmp}, nil
}
func (s *PNGSequence) WriteFrame(index int, frame *image.RGBA) error {
	if s.closed || index != s.next {
		return fmt.Errorf("sequence expected frame %d", s.next)
	}
	if e := PNG(filepath.Join(s.temp, fmt.Sprintf("%06d.png", index)), frame); e != nil {
		return e
	}
	s.next++
	return nil
}
func (s *PNGSequence) Close() error {
	if s.closed {
		return fmt.Errorf("sequence already closed")
	}
	if _, e := os.Lstat(s.target); e == nil {
		return fmt.Errorf("output exists: %s", s.target)
	}
	if e := os.Rename(s.temp, s.target); e != nil {
		return e
	}
	s.closed = true
	return nil
}
func (s *PNGSequence) Abort() error {
	if s.closed {
		return nil
	}
	s.closed = true
	return os.RemoveAll(s.temp)
}
