package audio

import (
	"encoding/binary"
	"fmt"
	"io"
)

// WAVE DVI ADPCM, Microsoft Multimedia Standards Update (1994), pp. 28-34.
// These are format constants, not an encoder-dependent approximation.
var wavIMASteps = [...]int{
	7, 8, 9, 10, 11, 12, 13, 14, 16, 17, 19, 21, 23, 25, 28, 31,
	34, 37, 41, 45, 50, 55, 60, 66, 73, 80, 88, 97, 107, 118, 130,
	143, 157, 173, 190, 209, 230, 253, 279, 307, 337, 371, 408, 449,
	494, 544, 598, 658, 724, 796, 876, 963, 1060, 1166, 1282, 1411,
	1552, 1707, 1878, 2066, 2272, 2499, 2749, 3024, 3327, 3660,
	4026, 4428, 4871, 5358, 5894, 6484, 7132, 7845, 8630, 9493,
	10442, 11487, 12635, 13899, 15289, 16818, 18500, 20350,
	22385, 24623, 27086, 29794, 32767,
}

var wavIMAIndex = [...]int{-1, -1, -1, -1, 2, 4, 6, 8}

func validateWAVIMAFormat(f wavFormat, b []byte) (wavFormat, error) {
	if len(b) != 20 || binary.LittleEndian.Uint16(b[16:18]) != 2 {
		return f, fmt.Errorf("%w: IMA ADPCM requires a 20-byte fmt and 2-byte extension", ErrUnsupported)
	}
	if f.bits != 4 || (f.format.Channels != 1 && f.format.Channels != 2) {
		return f, fmt.Errorf("%w: IMA ADPCM supports only 4-bit mono/stereo", ErrUnsupported)
	}
	if err := f.format.validate(); err != nil {
		return f, err
	}
	// Each block has a four-byte header per channel, followed by groups of
	// four bytes per channel. Each group adds eight frames. Header-only
	// blocks are well-defined (one frame); arbitrary nibble tails are not.
	groupBytes := 4 * f.format.Channels
	if f.align < groupBytes || f.align%groupBytes != 0 {
		return f, fmt.Errorf("%w: IMA blockAlign must contain complete channel groups", ErrInvalidInput)
	}
	f.imaSamplesPerBlock = int(binary.LittleEndian.Uint16(b[18:20]))
	expected := 1 + (f.align-groupBytes)*2/f.format.Channels
	if f.imaSamplesPerBlock != expected {
		return f, fmt.Errorf("%w: IMA samplesPerBlock disagrees with blockAlign", ErrInvalidInput)
	}
	// Compressed nAvgBytesPerSec is an advisory average, not a frame count.
	// Encoders may write their target bitrate rather than a measured average.
	// Zero is allowed only when the format's byte rate rounds below 1 B/s.
	// Derive timing only from rate and fact.
	if binary.LittleEndian.Uint32(b[8:12]) == 0 && uint64(f.format.SampleRate)*uint64(f.align)/uint64(f.imaSamplesPerBlock) != 0 {
		return f, fmt.Errorf("%w: zero IMA average byte rate", ErrInvalidInput)
	}
	return f, nil
}

func readWAVIMA(r io.Reader, n int64, f wavFormat, limits Limits) (*PCM, error) {
	if n%int64(f.align) != 0 {
		return nil, fmt.Errorf("%w: IMA ADPCM partial final blocks are not supported", ErrUnsupported)
	}
	frames := uint64(n/int64(f.align)) * uint64(f.imaSamplesPerBlock)
	// Charge all coded frames, including the tail later removed by fact.
	// This bounds both output storage and nibble decoding work even when
	// fact follows data or attempts to hide whole blocks.
	if err := limits.checkFrames(frames, f.format.Channels); err != nil {
		return nil, err
	}
	channels := f.format.Channels
	pcm := &PCM{format: f.format, samples: make([]float64, int(frames)*channels)}
	if n == 0 {
		return pcm, nil
	}
	// At most 65535 bytes by WAVEFORMATEX, independent of encoded length.
	block := make([]byte, f.align)
	for offset := 0; offset < len(pcm.samples); offset += f.imaSamplesPerBlock * channels {
		if _, err := io.ReadFull(r, block); err != nil {
			return nil, fmt.Errorf("audio: IMA ADPCM block: %w", err)
		}
		var predictor, index [2]int
		for channel := 0; channel < channels; channel++ {
			header := block[4*channel : 4*channel+4]
			predictor[channel] = int(int16(binary.LittleEndian.Uint16(header)))
			index[channel] = int(header[2])
			if index[channel] > 88 || header[3] != 0 {
				return nil, fmt.Errorf("%w: IMA step index/reserved header byte", ErrInvalidInput)
			}
			pcm.samples[offset+channel] = float64(predictor[channel]) / 32768
		}
		for group := 4 * channels; group < len(block); group += 4 * channels {
			frame := 1 + (group-4*channels)*2/channels
			for channel := 0; channel < channels; channel++ {
				for j, b := range block[group+4*channel : group+4*channel+4] {
					for half := 0; half < 2; half++ {
						nibble := int(b>>(4*half)) & 15 // low nibble first
						step := wavIMASteps[index[channel]]
						// The specified separate shifts are intentional. Replacing
						// them with ((2*(nibble&7)+1)*step)>>3 changes PCM.
						diff := step >> 3
						if nibble&4 != 0 {
							diff += step
						}
						if nibble&2 != 0 {
							diff += step >> 1
						}
						if nibble&1 != 0 {
							diff += step >> 2
						}
						if nibble&8 != 0 {
							diff = -diff
						}
						predictor[channel] = max(-32768, min(32767, predictor[channel]+diff))
						index[channel] = max(0, min(88, index[channel]+wavIMAIndex[nibble&7]))
						pcm.samples[offset+(frame+2*j+half)*channels+channel] = float64(predictor[channel]) / 32768
					}
				}
			}
		}
	}
	return pcm, nil
}
