package audio

import "fmt"

// MaxConcatClips bounds the number of clips in one Concat, including its
// receiver and empty clips. It bounds validation work independently of duration.
const MaxConcatClips = 4096

// Concat copies p followed by others, in the supplied order, without gaps or
// overlaps. Every clip, including empty clips, must be nonnil, valid and have
// exactly the same Format (including ChannelMask). No conversion is implicit.
// At most MaxConcatClips clips, including p, are accepted. Limits bounds output
// frames/samples; encoded byte/chunk limits do not apply. Work is linear in clip
// count plus output samples. All validation precedes the single PCM allocation.
// The result owns independent samples, even when others is empty.
func (p *PCM) Concat(others []*PCM, limits Limits) (*PCM, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	if len(others) >= MaxConcatClips {
		return nil, fmt.Errorf("%w: concat exceeds %d clips", ErrLimit, MaxConcatClips)
	}
	l, err := limits.normalized()
	if err != nil {
		return nil, err
	}
	frames := uint64(p.Frames())
	if err := l.checkFrames(frames, p.Channels()); err != nil {
		return nil, err
	}
	for i, clip := range others {
		if err := clip.validate(); err != nil {
			return nil, fmt.Errorf("concat clip %d: %w", i+1, err)
		}
		if clip.format != p.format {
			return nil, fmt.Errorf("%w: concat clip %d format differs", ErrInvalidInput, i+1)
		}
		// Each valid clip's frame count is at most MaxInt. Incrementally
		// checking the storage bound keeps this addition below uint64 overflow.
		frames += uint64(clip.Frames())
		if err := l.checkFrames(frames, p.Channels()); err != nil {
			return nil, err
		}
	}
	result := &PCM{format: p.format, samples: make([]float64, int(frames)*p.Channels())}
	at := copy(result.samples, p.samples)
	for _, clip := range others {
		at += copy(result.samples[at:], clip.samples)
	}
	return result, nil
}

// Repeat copies count consecutive instances of p. Negative counts are invalid;
// count zero or an empty p returns an empty PCM with p's Format in constant
// work, even for count=MaxInt. Limits bounds output frames/samples; encoded
// byte/chunk limits do not apply. Multiplication is checked before allocation.
// Nonempty output uses linear copying work and one independent sample array;
// count=1 also returns a copy. No silence, overlap or boundary fade is added.
func (p *PCM) Repeat(count int, limits Limits) (*PCM, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	if count < 0 {
		return nil, fmt.Errorf("%w: negative repeat count", ErrInvalidInput)
	}
	l, err := limits.normalized()
	if err != nil {
		return nil, err
	}
	if count == 0 || p.Frames() == 0 {
		return &PCM{format: p.format}, nil
	}
	// Bound the product before forming it. checkFrames also enforces sample
	// count and architecture-specific float64 storage/byte-count bounds.
	if uint64(count) > uint64(l.MaxFrames)/uint64(p.Frames()) {
		return nil, fmt.Errorf("%w: repeated frame count exceeds limit", ErrLimit)
	}
	frames := uint64(count) * uint64(p.Frames())
	if err := l.checkFrames(frames, p.Channels()); err != nil {
		return nil, err
	}
	result := &PCM{format: p.format, samples: make([]float64, int(frames)*p.Channels())}
	// Doubling avoids a count-sized loop for single-frame clips. The last
	// copy may be partial, but always ends on the requested repeated pattern.
	for filled := copy(result.samples, p.samples); filled < len(result.samples); {
		filled += copy(result.samples[filled:], result.samples[:filled])
	}
	return result, nil
}
