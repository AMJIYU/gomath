package audio

import "fmt"

// RemixOptions bounds output storage and matrix work. MaxWork counts one unit
// per coefficient validation plus one per multiply/add pair, including zero
// coefficients: inputChannels*outputChannels*(1+frames). Zero selects 512M.
// Limits bounds output frames/samples; encoded byte/chunk limits do not apply.
type RemixOptions struct {
	Limits  Limits
	MaxWork uint64
}

// Remix applies matrix[outputChannel][inputChannel] to every input frame.
// Both channel dimensions are bounded by MaxChannels. The sample rate must
// match; use Resample separately. Rows/columns follow each Format's channel
// order (ascending speaker bits when its mask is nonzero). The caller explicitly
// chooses the complete output Format, including its mask: no speaker routing,
// mask inference, normalization or clipping is implicit. A zero mask explicitly
// requests indexed output channels, even when the input has speaker assignments.
//
// Coefficients must be finite, including on empty inputs. Products and running
// sums are evaluated in increasing input-channel order, rounded separately to
// float64, and must remain finite, even if later terms would cancel overflow.
// Finite headroom outside [-1,1] is preserved. Results own their sample storage;
// the matrix is neither modified nor retained. Callers must not modify the
// matrix during this call; concurrent calls may share an unmodified matrix.
func (p *PCM) Remix(output Format, matrix [][]float64, options RemixOptions) (*PCM, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	if err := output.validate(); err != nil {
		return nil, err
	}
	if output.SampleRate != p.SampleRate() {
		return nil, fmt.Errorf("%w: remix requires equal sample rates", ErrInvalidInput)
	}
	if len(matrix) != output.Channels {
		return nil, fmt.Errorf("%w: matrix needs %d output rows", ErrInvalidInput, output.Channels)
	}
	limits, err := options.Limits.normalized()
	if err != nil {
		return nil, err
	}
	if err := limits.checkFrames(uint64(p.Frames()), output.Channels); err != nil {
		return nil, err
	}
	work := options.MaxWork
	if work == 0 {
		work = 512 << 20
	}
	coefficients := uint64(p.Channels()) * uint64(output.Channels)
	// Division avoids overflow even if callers raise sample limits to MaxInt.
	if uint64(p.Frames())+1 > work/coefficients {
		return nil, fmt.Errorf("%w: remix exceeds matrix work budget", ErrLimit)
	}
	for row, values := range matrix {
		if len(values) != p.Channels() {
			return nil, fmt.Errorf("%w: matrix row %d needs %d input columns", ErrInvalidInput, row, p.Channels())
		}
		for col, value := range values {
			if !finite(value) {
				return nil, fmt.Errorf("%w: nonfinite matrix coefficient [%d][%d]", ErrInvalidInput, row, col)
			}
		}
	}
	result := &PCM{format: output, samples: make([]float64, p.Frames()*output.Channels)}
	for frame := 0; frame < p.Frames(); frame++ {
		input := p.samples[frame*p.Channels() : (frame+1)*p.Channels()]
		for row, values := range matrix {
			sum := 0.0
			for col, coefficient := range values {
				// Explicit conversions force separate rounding rather than FMA.
				product := float64(input[col] * coefficient)
				sum = float64(sum + product)
				if !finite(sum) {
					return nil, fmt.Errorf("%w: remix overflows frame %d, output %d, input %d", ErrInvalidInput, frame, row, col)
				}
			}
			result.samples[frame*output.Channels+row] = sum
		}
	}
	return result, nil
}
