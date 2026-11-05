// Package audio implements immutable, interleaved PCM, bounded RIFF/WAVE and
// native FLAC I/O, sample-accurate editing, and band-limited resampling using
// Go's standard library. It never launches external programs.
package audio

import (
	"errors"
	"fmt"
	"math"
	"math/bits"
)

var (
	ErrInvalidInput = errors.New("audio: invalid input")
	ErrUnsupported  = errors.New("audio: unsupported format or content")
	ErrLimit        = errors.New("audio: resource limit exceeded")
)

const (
	MaxSampleRate = 768000
	MaxChannels   = 64
)

// Format describes interleaved samples: frame f, channel c is at f*Channels+c.
// ChannelMask is either zero (indexed channels without speaker assignments), or
// a WAVE speaker mask using bits 0..17. Nonzero masks order channels by ascending
// set bit and must contain exactly Channels bits. No channel conversion is implicit.
type Format struct {
	SampleRate  int
	Channels    int
	ChannelMask uint32
}

func (f Format) validate() error {
	if f.SampleRate < 1 || f.SampleRate > MaxSampleRate || f.Channels < 1 || f.Channels > MaxChannels {
		return fmt.Errorf("%w: rate must be 1..%d and channels 1..%d", ErrInvalidInput, MaxSampleRate, MaxChannels)
	}
	if f.ChannelMask != 0 && (f.ChannelMask&^uint32(0x3ffff) != 0 || bits.OnesCount32(f.ChannelMask) != f.Channels) {
		return fmt.Errorf("%w: speaker mask does not match channel layout", ErrInvalidInput)
	}
	return nil
}

// Limits bound decoded/output frames and float64 sample storage, encoded bytes,
// and chunk/metadata count. Zero fields select defaults; negatives are errors.
// MaxBytes includes file headers/padding, not PCM RAM. MaxChunks counts RIFF
// chunks for WAVE and metadata blocks for FLAC (the writer emits STREAMINFO only).
// Edits own one new PCM array; a growing decoder can temporarily own both old
// and new arrays, each bounded by these limits. Callers retain their inputs.
type Limits struct {
	MaxFrames  int
	MaxSamples int
	MaxBytes   int64
	MaxChunks  int
}

// DefaultLimits returns independent defaults: up to 16M frames AND 16M samples
// (128 MiB float64 storage per PCM), 256 MiB encoded bytes, and 4096 chunks.
func DefaultLimits() Limits {
	return Limits{MaxFrames: 16 << 20, MaxSamples: 16 << 20, MaxBytes: 256 << 20, MaxChunks: 4096}
}

func (l Limits) normalized() (Limits, error) {
	if l.MaxFrames < 0 || l.MaxSamples < 0 || l.MaxBytes < 0 || l.MaxChunks < 0 {
		return l, fmt.Errorf("%w: negative resource limit", ErrInvalidInput)
	}
	d := DefaultLimits()
	if l.MaxFrames == 0 {
		l.MaxFrames = d.MaxFrames
	}
	if l.MaxSamples == 0 {
		l.MaxSamples = d.MaxSamples
	}
	if l.MaxBytes == 0 {
		l.MaxBytes = d.MaxBytes
	}
	if l.MaxChunks == 0 {
		l.MaxChunks = d.MaxChunks
	}
	return l, nil
}

func (l Limits) checkFrames(frames uint64, channels int) error {
	// The final bound also prevents byte-count overflow on 32-bit targets.
	maxStorageSamples := uint64(int(^uint(0)>>1) / 8)
	if frames > uint64(l.MaxFrames) || frames > uint64(l.MaxSamples)/uint64(channels) || frames > maxStorageSamples/uint64(channels) {
		return fmt.Errorf("%w: %d frames, %d channels", ErrLimit, frames, channels)
	}
	return nil
}

// PCM owns its samples. NewPCM copies input; Samples returns a copy; every edit
// returns independent storage. Samples must be finite, but may exceed [-1,1]
// to retain mixing headroom. Integer WAVE/FLAC export rejects excess amplitude.
// A PCM returned by this package is safe for concurrent reading and editing.
// The zero value is invalid; empty clips created by NewPCM are valid.
type PCM struct {
	format  Format
	samples []float64
}

func NewPCM(format Format, interleaved []float64) (*PCM, error) {
	return NewPCMWithLimits(format, interleaved, Limits{})
}

func NewPCMWithLimits(format Format, interleaved []float64, limits Limits) (*PCM, error) {
	if err := format.validate(); err != nil {
		return nil, err
	}
	l, err := limits.normalized()
	if err != nil {
		return nil, err
	}
	if len(interleaved)%format.Channels != 0 {
		return nil, fmt.Errorf("%w: incomplete interleaved frame", ErrInvalidInput)
	}
	if err := l.checkFrames(uint64(len(interleaved)/format.Channels), format.Channels); err != nil {
		return nil, err
	}
	for i, v := range interleaved {
		if !finite(v) {
			return nil, fmt.Errorf("%w: nonfinite sample %d", ErrInvalidInput, i)
		}
	}
	return &PCM{format: format, samples: append([]float64(nil), interleaved...)}, nil
}

func (p *PCM) validate() error {
	if p == nil {
		return fmt.Errorf("%w: nil PCM", ErrInvalidInput)
	}
	return p.format.validate()
}

func (p *PCM) Format() Format  { return p.format }
func (p *PCM) SampleRate() int { return p.format.SampleRate }
func (p *PCM) Channels() int   { return p.format.Channels }
func (p *PCM) Frames() int     { return len(p.samples) / p.format.Channels }

// Duration is the half-open time interval [0, Frames/SampleRate), in seconds.
func (p *PCM) Duration() float64 { return float64(p.Frames()) / float64(p.SampleRate()) }

func (p *PCM) Samples() []float64 { return append([]float64(nil), p.samples...) }

func (p *PCM) Sample(frame, channel int) (float64, error) {
	if err := p.validate(); err != nil {
		return 0, err
	}
	if frame < 0 || frame >= p.Frames() || channel < 0 || channel >= p.Channels() {
		return 0, fmt.Errorf("%w: sample index outside PCM", ErrInvalidInput)
	}
	return p.samples[frame*p.Channels()+channel], nil
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

func secondsToFrame(seconds float64, rate int, maxFrame int) (int, error) {
	if !finite(seconds) || seconds < 0 {
		return 0, fmt.Errorf("%w: time must be finite and nonnegative", ErrInvalidInput)
	}
	f := math.Round(seconds * float64(rate))
	// A strict upper comparison avoids converting float64(MaxInt) to int,
	// which can overflow due to float rounding on 64-bit platforms.
	if !finite(f) || f >= float64(int(^uint(0)>>1)) || f > float64(maxFrame) {
		return 0, fmt.Errorf("%w: time exceeds frame limit", ErrLimit)
	}
	return int(f), nil
}
