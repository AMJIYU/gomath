package audio

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
)

// Microsoft Multimedia Standards Update (1994), Microsoft ADPCM. The first
// seven signed 8.8 coefficient pairs are compulsory even when not selected.
var wavMSCoefficients = [...][2]int16{{256, 0}, {512, -256}, {0, 0}, {192, 64}, {240, 0}, {460, -208}, {392, -232}}
var wavMSAdapt = [...]int64{230, 230, 230, 230, 307, 409, 512, 614, 768, 614, 512, 409, 307, 230, 230, 230}

const wavMSMaxDelta int64 = math.MaxInt64 / 768

// Standard fmt is 50 bytes. A byte-sized block predictor can address at most
// 256 coefficient pairs, bounding fmt to 22+4*256=1046 bytes. No input-sized
// buffer is needed to parse it. Other WAVE formats retain their existing parser.
func readWAVMSFormat(r io.Reader, n int64) (wavFormat, error) {
	var f wavFormat
	if n < 50 || n > 1046 || (n-22)%4 != 0 {
		return f, fmt.Errorf("%w: Microsoft ADPCM fmt length %d", ErrUnsupported, n)
	}
	var b [22]byte
	if _, err := io.ReadFull(r, b[:]); err != nil {
		return f, fmt.Errorf("audio: Microsoft ADPCM fmt: %w", err)
	}
	f.tag = binary.LittleEndian.Uint16(b[:2])
	f.bits = binary.LittleEndian.Uint16(b[14:16])
	f.format.Channels = int(binary.LittleEndian.Uint16(b[2:4]))
	rate := binary.LittleEndian.Uint32(b[4:8])
	if f.tag != 2 || f.bits != 4 || (f.format.Channels != 1 && f.format.Channels != 2) {
		return f, fmt.Errorf("%w: Microsoft ADPCM requires direct tag 2, 4-bit mono/stereo", ErrUnsupported)
	}
	if rate > MaxSampleRate {
		return f, fmt.Errorf("%w: Microsoft ADPCM sample rate %d", ErrUnsupported, rate)
	}
	f.format.SampleRate = int(rate)
	if err := f.format.validate(); err != nil {
		return f, err
	}
	count := int(binary.LittleEndian.Uint16(b[20:22]))
	if count < 7 || count > 256 || n != int64(22+4*count) || int(binary.LittleEndian.Uint16(b[16:18])) != 4+4*count {
		return f, fmt.Errorf("%w: Microsoft ADPCM coefficient count/fmt extension", ErrUnsupported)
	}
	f.align = int(binary.LittleEndian.Uint16(b[12:14]))
	f.msSamplesPerBlock = int(binary.LittleEndian.Uint16(b[18:20]))
	headerBytes := 7 * f.format.Channels
	if f.align < headerBytes || f.msSamplesPerBlock != 2+(f.align-headerBytes)*2/f.format.Channels {
		return f, fmt.Errorf("%w: Microsoft ADPCM samplesPerBlock/blockAlign", ErrInvalidInput)
	}
	// A compressed target/average byte rate is advisory, as for IMA. Decode
	// capacity and duration come exclusively from block layout and fact.
	if binary.LittleEndian.Uint32(b[8:12]) == 0 && uint64(rate)*uint64(f.align)/uint64(f.msSamplesPerBlock) != 0 {
		return f, fmt.Errorf("%w: zero Microsoft ADPCM average byte rate", ErrInvalidInput)
	}
	f.msCoefficients = make([][2]int16, count)
	var pair [4]byte
	for i := range f.msCoefficients {
		if _, err := io.ReadFull(r, pair[:]); err != nil {
			return f, fmt.Errorf("audio: Microsoft ADPCM coefficients: %w", err)
		}
		f.msCoefficients[i] = [2]int16{int16(binary.LittleEndian.Uint16(pair[:2])), int16(binary.LittleEndian.Uint16(pair[2:]))}
		if i < 7 && f.msCoefficients[i] != wavMSCoefficients[i] {
			return f, fmt.Errorf("%w: Microsoft ADPCM mandatory coefficient pair %d", ErrInvalidInput, i)
		}
	}
	return f, nil
}

type wavMSChannel struct {
	first, second int64
	delta         int64
	coeff         [2]int16
}

func (s *wavMSChannel) sample(code byte) (float64, error) {
	// All products use signed 64-bit arithmetic. In particular custom
	// coefficients can exceed a signed 32-bit intermediate at PCM extrema.
	predicted := (s.first*int64(s.coeff[0]) + s.second*int64(s.coeff[1])) / 256
	residual := int64(code)
	if code&8 != 0 {
		residual -= 16
	}
	value := max(int64(-32768), min(int64(32767), predicted+residual*s.delta))
	nextDelta := max(int64(16), s.delta*wavMSAdapt[code]/256)
	if nextDelta > wavMSMaxDelta {
		return 0, fmt.Errorf("%w: Microsoft ADPCM adaptive delta exceeds checked int64 bound", ErrLimit)
	}
	s.second, s.first, s.delta = s.first, value, nextDelta
	return float64(value) / 32768, nil
}

func readWAVMS(r io.Reader, n int64, f wavFormat, limits Limits) (*PCM, error) {
	if n%int64(f.align) != 0 {
		return nil, fmt.Errorf("%w: Microsoft ADPCM partial final blocks", ErrUnsupported)
	}
	frames := uint64(n/int64(f.align)) * uint64(f.msSamplesPerBlock)
	// Charge the full coded capacity, including padding later trimmed by fact.
	if err := limits.checkFrames(frames, f.format.Channels); err != nil {
		return nil, err
	}
	channels := f.format.Channels
	pcm := &PCM{format: f.format, samples: make([]float64, int(frames)*channels)}
	if n == 0 {
		return pcm, nil
	}
	block := make([]byte, f.align)
	for start := 0; start < len(pcm.samples); start += f.msSamplesPerBlock * channels {
		if _, err := io.ReadFull(r, block); err != nil {
			return nil, fmt.Errorf("audio: Microsoft ADPCM block: %w", err)
		}
		var state [2]wavMSChannel
		for ch := 0; ch < channels; ch++ {
			index := int(block[ch])
			if index >= len(f.msCoefficients) {
				return nil, fmt.Errorf("%w: Microsoft ADPCM predictor index", ErrInvalidInput)
			}
			delta := int16(binary.LittleEndian.Uint16(block[channels+2*ch:]))
			if delta < 0 {
				return nil, fmt.Errorf("%w: Microsoft ADPCM negative/unsigned-high header delta", ErrUnsupported)
			}
			state[ch] = wavMSChannel{
				first:  int64(int16(binary.LittleEndian.Uint16(block[3*channels+2*ch:]))),
				second: int64(int16(binary.LittleEndian.Uint16(block[5*channels+2*ch:]))),
				delta:  int64(delta), coeff: f.msCoefficients[index],
			}
			// Header stores sample1 then sample2; playback reverses that order.
			pcm.samples[start+ch] = float64(state[ch].second) / 32768
			pcm.samples[start+channels+ch] = float64(state[ch].first) / 32768
		}
		out := start + 2*channels
		for _, b := range block[7*channels:] {
			value, err := state[0].sample(b >> 4)
			if err != nil {
				return nil, err
			}
			pcm.samples[out] = value
			out++
			value, err = state[channels-1].sample(b & 15)
			if err != nil {
				return nil, err
			}
			pcm.samples[out] = value
			out++
		}
	}
	return pcm, nil
}
