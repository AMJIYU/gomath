package audio

import "fmt"

// Trim copies the half-open interval [start,end), in seconds. Both boundaries
// round to the nearest frame (half-frame ties round upward). Out-of-range time
// is an error; use TrimFrames for exact integer boundaries.
func (p *PCM) Trim(start, end float64) (*PCM, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	if !finite(start) || !finite(end) || start < 0 || end < start || end > p.Duration() {
		return nil, fmt.Errorf("%w: trim interval outside PCM", ErrInvalidInput)
	}
	first, err := secondsToFrame(start, p.SampleRate(), p.Frames())
	if err != nil {
		return nil, err
	}
	last, err := secondsToFrame(end, p.SampleRate(), p.Frames())
	if err != nil {
		return nil, err
	}
	return p.TrimFrames(first, last)
}

func (p *PCM) TrimFrames(start, end int) (*PCM, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	if start < 0 || end < start || end > p.Frames() {
		return nil, fmt.Errorf("%w: trim frame interval outside PCM", ErrInvalidInput)
	}
	return &PCM{format: p.format, samples: append([]float64(nil), p.samples[start*p.Channels():end*p.Channels()]...)}, nil
}

// Mix places other at a nonnegative time in seconds, rounded to the nearest
// frame. It extends the result with silence as needed and sums overlaps without
// clipping. Both formats (including speaker masks) must match exactly.
func (p *PCM) Mix(other *PCM, offset float64) (*PCM, error) {
	return p.MixWithLimits(other, offset, Limits{})
}

func (p *PCM) MixWithLimits(other *PCM, offset float64, limits Limits) (*PCM, error) {
	l, err := limits.normalized()
	if err != nil {
		return nil, err
	}
	if err := p.validate(); err != nil {
		return nil, err
	}
	frame, err := secondsToFrame(offset, p.SampleRate(), l.MaxFrames)
	if err != nil {
		return nil, err
	}
	return p.MixFrames(other, frame, l)
}

// MixFrames is Mix with an exact frame offset and explicit resource limits.
// An empty other clip contributes no duration, even with a nonzero offset.
func (p *PCM) MixFrames(other *PCM, offset int, limits Limits) (*PCM, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	if err := other.validate(); err != nil {
		return nil, err
	}
	if offset < 0 || p.format != other.format {
		return nil, fmt.Errorf("%w: nonnegative offset and matching PCM formats required", ErrInvalidInput)
	}
	l, err := limits.normalized()
	if err != nil {
		return nil, err
	}
	frames := uint64(p.Frames())
	if other.Frames() != 0 {
		frames = max(frames, uint64(offset)+uint64(other.Frames()))
	}
	if err := l.checkFrames(frames, p.Channels()); err != nil {
		return nil, err
	}
	result := &PCM{format: p.format, samples: make([]float64, int(frames)*p.Channels())}
	copy(result.samples, p.samples)
	if other.Frames() != 0 {
		base := offset * p.Channels()
		for i, v := range other.samples {
			sum := result.samples[base+i] + v
			if !finite(sum) {
				return nil, fmt.Errorf("%w: mixing overflows sample %d", ErrInvalidInput, base+i)
			}
			result.samples[base+i] = sum
		}
	}
	return result, nil
}

// Gain applies a linear multiplier to all channels; negative values invert
// polarity. Overflow is an error. Convert decibels explicitly with 10^(dB/20).
func (p *PCM) Gain(gain float64) (*PCM, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	if !finite(gain) {
		return nil, fmt.Errorf("%w: nonfinite gain", ErrInvalidInput)
	}
	result := &PCM{format: p.format, samples: make([]float64, len(p.samples))}
	for i, v := range p.samples {
		result.samples[i] = v * gain
		if !finite(result.samples[i]) {
			return nil, fmt.Errorf("%w: gain overflows sample %d", ErrInvalidInput, i)
		}
	}
	return result, nil
}

// GainPoint gives a linear gain at a time in seconds.
type GainPoint struct {
	Time float64
	Gain float64
}

// Envelope evaluates piecewise-linear gain at each frame's time, identically
// on all channels. End values are held outside the control-point interval.
// Supply 1..4096 finite points, with strictly increasing times in [0,Duration].
// No normalization, clipping, automatic fade, or channel remapping is applied.
func (p *PCM) Envelope(points []GainPoint) (*PCM, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	if len(points) == 0 {
		return nil, fmt.Errorf("%w: empty gain envelope", ErrInvalidInput)
	}
	if len(points) > 4096 {
		return nil, fmt.Errorf("%w: more than 4096 gain points", ErrLimit)
	}
	for i, point := range points {
		if !finite(point.Time) || !finite(point.Gain) || point.Time < 0 || point.Time > p.Duration() || (i != 0 && point.Time <= points[i-1].Time) {
			return nil, fmt.Errorf("%w: envelope point %d has invalid time/gain", ErrInvalidInput, i)
		}
	}
	result := &PCM{format: p.format, samples: make([]float64, len(p.samples))}
	segment := 0
	for frame := 0; frame < p.Frames(); frame++ {
		t := float64(frame) / float64(p.SampleRate())
		for segment+1 < len(points) && t >= points[segment+1].Time {
			segment++
		}
		gain := points[segment].Gain
		if t > points[segment].Time && segment+1 < len(points) {
			a, b := points[segment], points[segment+1]
			x := (t - a.Time) / (b.Time - a.Time)
			// Convex interpolation avoids overflowing b.Gain-a.Gain for
			// large, finite gains with opposite signs.
			gain = (1-x)*a.Gain + x*b.Gain
		}
		for channel := 0; channel < p.Channels(); channel++ {
			i := frame*p.Channels() + channel
			result.samples[i] = p.samples[i] * gain
			if !finite(result.samples[i]) {
				return nil, fmt.Errorf("%w: envelope overflows sample %d", ErrInvalidInput, i)
			}
		}
	}
	return result, nil
}
