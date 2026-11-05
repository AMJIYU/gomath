package gomath

import (
	"fmt"
	"gomath/audio"
	"math"
)

const MaxSoundEvents = 1024

type soundEvent struct {
	clip        *audio.PCM
	start, gain float64
}

type SoundOptions struct {
	// Offset is relative to the current scene authoring cursor. Negative values
	// are valid when the resulting absolute start remains nonnegative.
	Offset float64
	// GainDB is an amplitude gain in decibels. Zero preserves input amplitude.
	GainDB float64
}

// AddSound schedules immutable PCM at the current cursor plus offset. It does
// not advance the scene. Soundtrack preserves sound tails beyond video duration.
func (s *Scene) AddSound(clip *audio.PCM, offset float64) error {
	return s.AddSoundWithOptions(clip, SoundOptions{Offset: offset})
}

func (s *Scene) AddSoundWithOptions(clip *audio.PCM, opt SoundOptions) error {
	if clip == nil {
		return fmt.Errorf("nil sound clip")
	}
	if _, err := audio.NewPCM(clip.Format(), nil); err != nil {
		return err
	}
	start := s.duration + opt.Offset
	gain := math.Pow(10, opt.GainDB/20)
	if !finite(opt.Offset) || !finite(start) || start < 0 || !finite(opt.GainDB) || !finite(gain) || !finite(start+clip.Duration()) {
		return fmt.Errorf("sound start/gain must be finite and start nonnegative")
	}
	if len(s.sounds) >= MaxSoundEvents {
		return fmt.Errorf("sound event limit %d exceeded", MaxSoundEvents)
	}
	s.sounds = append(s.sounds, soundEvent{clip: clip, start: start, gain: gain})
	return nil
}

type SoundtrackOptions struct {
	// Resample selects the sinc filter. Its Limits apply to clips, output and
	// temporary sample storage. Zero limits select audio's bounded defaults.
	Resample audio.ResampleOptions
	// MaxMixSamples bounds output initialization plus summed clip samples.
	// Zero selects 512M; resampling also has its own MaxWork budget per clip.
	MaxMixSamples uint64
}

// Soundtrack mixes scheduled sounds into independent PCM. Starts round to the
// nearest output frame (half-frame ties upward), resampling aligns time zero.
// Output covers ceil(scene duration * rate) and all sound tails; no clip is
// silently truncated. Channels are mixed indexwise, without conversion; known
// conflicting speaker masks are errors. Unspecified masks keep channel indices.
// Mixing retains headroom beyond [-1,1]; integer file writers may reject it.
//
// Sampling is read-only and concurrency safe after authoring. One output buffer,
// its final owned copy, and at most one resampled clip coexist during this call;
// concurrent calls multiply those storage costs. There is no implicit cache.
func (s *Scene) Soundtrack(format audio.Format, opt SoundtrackOptions) (*audio.PCM, error) {
	empty, err := audio.NewPCMWithLimits(format, nil, opt.Resample.Limits)
	if err != nil {
		return nil, err
	}
	// Even an empty soundtrack validates filter options, rather than accepting
	// a configuration that would fail only when a later clip is introduced.
	if _, err = empty.Resample(format.SampleRate, opt.Resample); err != nil {
		return nil, err
	}
	limits := opt.Resample.Limits
	defaults := audio.DefaultLimits()
	if limits.MaxFrames == 0 {
		limits.MaxFrames = defaults.MaxFrames
	}
	if limits.MaxSamples == 0 {
		limits.MaxSamples = defaults.MaxSamples
	}
	maxFrames := min(limits.MaxFrames, limits.MaxSamples/format.Channels, int(^uint(0)>>1)/8/format.Channels)
	toFrame := func(seconds float64, ceil bool) (int, error) {
		x := seconds * float64(format.SampleRate)
		if ceil {
			x = math.Ceil(x)
		} else {
			x = math.Round(x)
		}
		if !finite(x) || x < 0 || x > float64(maxFrames) {
			return 0, fmt.Errorf("%w: soundtrack frame range", audio.ErrLimit)
		}
		frame := int(x)
		if frame < 0 || frame > maxFrames {
			return 0, fmt.Errorf("%w: soundtrack frame range", audio.ErrLimit)
		}
		return frame, nil
	}
	frames, err := toFrame(s.duration, true)
	if err != nil {
		return nil, err
	}
	starts := make([]int, len(s.sounds))
	lengths := make([]int, len(s.sounds))
	work := uint64(0)
	maxWork := opt.MaxMixSamples
	if maxWork == 0 {
		maxWork = 512 << 20
	}
	for i, event := range s.sounds {
		f := event.clip.Format()
		if f.Channels != format.Channels || f.ChannelMask != 0 && format.ChannelMask != 0 && f.ChannelMask != format.ChannelMask {
			return nil, fmt.Errorf("sound %d has incompatible channels/speaker mask", i)
		}
		if event.clip.Frames() == 0 {
			continue
		}
		start, err := toFrame(event.start, false)
		if err != nil {
			return nil, err
		}
		// Integer quotient/remainder matches audio.Resample's exact ceil length.
		n, old, rate := uint64(event.clip.Frames()), uint64(f.SampleRate), uint64(format.SampleRate)
		if n/old > uint64(maxFrames)/rate {
			return nil, fmt.Errorf("%w: sound %d exceeds soundtrack frame budget", audio.ErrLimit, i)
		}
		length := n/old*rate + (n%old*rate+old-1)/old
		if length > uint64(maxFrames-start) {
			return nil, fmt.Errorf("%w: sound %d exceeds soundtrack frame budget", audio.ErrLimit, i)
		}
		starts[i], lengths[i] = start, int(length)
		if length != 0 {
			frames = max(frames, start+int(length))
		}
		cost := length * uint64(format.Channels)
		if cost > maxWork-work {
			return nil, fmt.Errorf("%w: soundtrack mixing work budget exceeded", audio.ErrLimit)
		}
		work += cost
	}
	if uint64(frames)*uint64(format.Channels) > maxWork || work > maxWork-uint64(frames)*uint64(format.Channels) {
		return nil, fmt.Errorf("%w: soundtrack mixing work budget exceeded", audio.ErrLimit)
	}
	buffer := make([]float64, frames*format.Channels)
	for i, event := range s.sounds {
		clip := event.clip
		if clip.SampleRate() != format.SampleRate {
			clip, err = clip.Resample(format.SampleRate, opt.Resample)
			if err != nil {
				return nil, fmt.Errorf("resample sound %d: %w", i, err)
			}
		}
		for j := range lengths[i] {
			for c := range format.Channels {
				x, err := clip.Sample(j, c)
				if err != nil {
					return nil, err
				}
				at := (starts[i]+j)*format.Channels + c
				buffer[at] += x * event.gain
				if !finite(buffer[at]) {
					return nil, fmt.Errorf("sound %d mixing overflow", i)
				}
			}
		}
	}
	return audio.NewPCMWithLimits(format, buffer, opt.Resample.Limits)
}

// Soundtrack uses the frozen authored sound events captured by CompileReplay.
func (r *Replay) Soundtrack(format audio.Format, opt SoundtrackOptions) (*audio.PCM, error) {
	if r == nil || r.scene == nil {
		return nil, fmt.Errorf("uninitialized replay")
	}
	return r.scene.Soundtrack(format, opt)
}
