package audio

import (
	"fmt"
	"math"
)

// EdgeMode specifies samples beyond the original clip during filtering.
type EdgeMode uint8

const (
	// EdgeReflect uses half-sample symmetric reflection: -1 maps to 0,
	// -2 to 1, N to N-1. This preserves DC even for single-frame clips.
	EdgeReflect EdgeMode = iota
	// EdgeZero treats missing samples as zero (including their filter weights
	// in normalization), producing the expected boundary transient for silence.
	EdgeZero
	// EdgeHold repeats the nearest endpoint.
	EdgeHold
)

// ResampleOptions controls the Blackman-windowed sinc low-pass interpolator.
// Radius is the kernel half-width in cutoff-scaled input samples (4..256;
// default 32). Rolloff is a fraction of the lower Nyquist frequency (0..1,
// exclusive of 0; zero selects 0.94). Lower rolloff reserves transition band.
// Downsampling expands kernel support by 1/cutoff to prevent aliasing.
// MaxTaps bounds kernel storage (default 8193, maximum 65537). MaxWork bounds
// outputFrames * taps * (channels+1), default 512M scalar filter operations.
// Limits bounds input/output sample counts. Negative limits are errors.
type ResampleOptions struct {
	Radius  int
	Rolloff float64
	Edge    EdgeMode
	MaxTaps int
	MaxWork uint64
	Limits  Limits
}

func (o ResampleOptions) normalized() (ResampleOptions, error) {
	if o.Radius == 0 {
		o.Radius = 32
	}
	if o.Rolloff == 0 {
		o.Rolloff = .94
	}
	if o.MaxTaps == 0 {
		o.MaxTaps = 8193
	}
	if o.MaxWork == 0 {
		o.MaxWork = 512 << 20
	}
	if o.Radius < 4 || o.Radius > 256 || !finite(o.Rolloff) || o.Rolloff <= 0 || o.Rolloff > 1 || o.Edge > EdgeHold || o.MaxTaps < 1 || o.MaxTaps > 65537 {
		return o, fmt.Errorf("%w: invalid sinc resampler options", ErrInvalidInput)
	}
	var err error
	o.Limits, err = o.Limits.normalized()
	return o, err
}

// Resample returns independent PCM with ceil(Frames*newRate/oldRate) frames.
// Frame j samples input time j/newRate: zero time is aligned and there is no
// added filter latency. DC gain is normalized per phase. Duration may grow by
// less than one output frame. There is no dither, clipping or automatic gain.
// A request at the original rate returns an exact copy, without low-pass filtering.
// Excess taps, work, storage or nonfinite output are errors, never fallbacks.
func (p *PCM) Resample(newRate int, options ResampleOptions) (*PCM, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	format := p.format
	format.SampleRate = newRate
	if err := format.validate(); err != nil {
		return nil, err
	}
	o, err := options.normalized()
	if err != nil {
		return nil, err
	}
	if err := o.Limits.checkFrames(uint64(p.Frames()), p.Channels()); err != nil {
		return nil, err
	}
	// Quotient/remainder arithmetic avoids overflow of inputFrames*newRate.
	n, oldRate, rate := uint64(p.Frames()), uint64(p.SampleRate()), uint64(newRate)
	whole, tail := n/oldRate, (n%oldRate*rate+oldRate-1)/oldRate
	if whole > (^uint64(0)-tail)/rate {
		return nil, fmt.Errorf("%w: resampled frame count overflow", ErrLimit)
	}
	frames := whole*rate + tail
	if err := o.Limits.checkFrames(frames, p.Channels()); err != nil {
		return nil, err
	}
	if newRate == p.SampleRate() || n == 0 {
		if frames > o.MaxWork/uint64(p.Channels()) {
			return nil, fmt.Errorf("%w: copy exceeds resampling work budget", ErrLimit)
		}
		return &PCM{format: format, samples: append([]float64(nil), p.samples...)}, nil
	}
	cutoff := math.Min(1, float64(newRate)/float64(p.SampleRate())) * o.Rolloff
	halfWidth := math.Ceil(float64(o.Radius) / cutoff)
	if !finite(halfWidth) || halfWidth > float64((o.MaxTaps-1)/2) {
		return nil, fmt.Errorf("%w: sinc support exceeds %d taps", ErrLimit, o.MaxTaps)
	}
	radius := int(halfWidth)
	taps := 2*radius + 1
	workPerFrame := uint64(taps) * uint64(p.Channels()+1)
	if frames > o.MaxWork/workPerFrame {
		return nil, fmt.Errorf("%w: %d frames x %d taps x %d channels/work", ErrLimit, frames, taps, p.Channels()+1)
	}
	result := &PCM{format: format, samples: make([]float64, int(frames)*p.Channels())}
	weights := make([]float64, taps)
	lastPhase := -1
	for frame := 0; frame < int(frames); frame++ {
		// Integer time arithmetic avoids accumulated fractional phase drift.
		// The checked input/output bounds keep these products representable.
		q, r := frame/newRate, frame%newRate
		center := q*p.SampleRate() + int(uint64(r)*oldRate/rate)
		phase := int(uint64(r) * oldRate % rate)
		if phase != lastPhase {
			fraction := float64(phase) / float64(newRate)
			total := 0.0
			for tap := range weights {
				x := (float64(tap-radius) - fraction) * cutoff
				w := 0.0
				if math.Abs(x) < float64(o.Radius) {
					sinc := 1.0
					if x != 0 {
						sinc = math.Sin(math.Pi*x) / (math.Pi * x)
					}
					angle := math.Pi * x / float64(o.Radius)
					w = cutoff * sinc * (.42 + .5*math.Cos(angle) + .08*math.Cos(2*angle))
				}
				weights[tap] = w
				total += w
			}
			if !finite(total) || math.Abs(total) < 1e-12 {
				return nil, fmt.Errorf("%w: degenerate sinc normalization", ErrInvalidInput)
			}
			for tap := range weights {
				weights[tap] /= total
			}
			lastPhase = phase
		}
		out := result.samples[frame*p.Channels() : (frame+1)*p.Channels()]
		for tap, weight := range weights {
			if weight == 0 {
				continue
			}
			source := center - radius + tap
			if source < 0 || source >= p.Frames() {
				switch o.Edge {
				case EdgeZero:
					continue
				case EdgeHold:
					source = max(0, min(source, p.Frames()-1))
				case EdgeReflect:
					// Wide period arithmetic also works on 32-bit Go.
					period := int64(p.Frames()) * 2
					reflected := int64(source) % period
					if reflected < 0 {
						reflected += period
					}
					if reflected >= int64(p.Frames()) {
						reflected = period - 1 - reflected
					}
					source = int(reflected)
				}
			}
			in := p.samples[source*p.Channels() : (source+1)*p.Channels()]
			for channel, value := range in {
				out[channel] += value * weight
			}
		}
		for _, value := range out {
			if !finite(value) {
				return nil, fmt.Errorf("%w: resampling overflows frame %d", ErrInvalidInput, frame)
			}
		}
	}
	return result, nil
}
