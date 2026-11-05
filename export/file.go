// Package export writes images and animation files using pure Go codecs.
package export

import (
	"fmt"
	"os"
	"path/filepath"
)

// atomicFile publishes a complete output only after a successful Close.
// Existing files are rejected rather than overwritten.
type atomicFile struct {
	file         *os.File
	target, temp string
	closed       bool
}

func newAtomicFile(target string) (*atomicFile, error) {
	if target == "" {
		return nil, fmt.Errorf("empty output path")
	}
	if _, e := os.Lstat(target); e == nil {
		return nil, fmt.Errorf("output already exists: %s", target)
	} else if !os.IsNotExist(e) {
		return nil, e
	}
	if e := os.MkdirAll(filepath.Dir(target), 0755); e != nil {
		return nil, e
	}
	f, e := os.CreateTemp(filepath.Dir(target), ".gomath-*")
	if e != nil {
		return nil, e
	}
	return &atomicFile{file: f, target: target, temp: f.Name()}, nil
}
func (f *atomicFile) commit() error {
	if f.closed {
		return fmt.Errorf("output is already closed")
	}
	if e := f.file.Close(); e != nil {
		return e
	} // Link is an atomic no-replace publication on the same filesystem.
	if e := os.Link(f.temp, f.target); e != nil {
		return fmt.Errorf("publish output: %w", e)
	}
	f.closed = true
	return os.Remove(f.temp)
}
func (f *atomicFile) abort() error {
	if f.closed {
		return nil
	}
	f.closed = true
	_ = f.file.Close()
	return os.Remove(f.temp)
}
