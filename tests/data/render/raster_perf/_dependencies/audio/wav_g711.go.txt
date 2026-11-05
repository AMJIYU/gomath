package audio

import (
	"encoding/binary"
	"fmt"
	"io"
)

// ITU-T G.711 (11/1988), Tables 1a/b and 2a/b: reconstruct the quantization
// interval midpoints in signed 16-bit scale. A-law inverts even bits on the
// wire; mu-law complements the sign/segment/mantissa representation.
func wavALawSample(code byte) int16 {
	a := code ^ 0x55
	segment := int(a>>4) & 7
	magnitude := int(a&15)*16 + 8
	if segment > 0 {
		magnitude = (magnitude + 256) << (segment - 1)
	}
	if a&128 == 0 {
		magnitude = -magnitude
	}
	return int16(magnitude)
}

func wavMuLawSample(code byte) int16 {
	u := ^code
	magnitude := ((int(u&15)*8 + 132) << ((u >> 4) & 7)) - 132
	if u&128 != 0 {
		magnitude = -magnitude
	}
	return int16(magnitude)
}

// Built once from the reconstruction equations, then only read. Integer zero
// becomes positive float64 zero for both mu-law zero codes (0x7f and 0xff).
var wavG711PCM = func() [2][256]float64 {
	var tables [2][256]float64
	for code := range 256 {
		tables[0][code] = float64(wavALawSample(byte(code))) / 32768
		tables[1][code] = float64(wavMuLawSample(byte(code))) / 32768
	}
	return tables
}()

func validateWAVG711Format(f wavFormat, b []byte) (wavFormat, error) {
	// The registered companded formats use WAVEFORMATEX with no extra data.
	// Legacy 16-byte fmt and extensible compressed GUIDs are not this subset.
	if len(b) != 18 || binary.LittleEndian.Uint16(b[16:18]) != 0 {
		return f, fmt.Errorf("%w: G.711 requires an 18-byte fmt and cbSize=0", ErrUnsupported)
	}
	if f.bits != 8 {
		return f, fmt.Errorf("%w: G.711 requires 8-bit codes", ErrUnsupported)
	}
	if err := f.format.validate(); err != nil {
		return f, err
	}
	if f.align != f.format.Channels || uint64(binary.LittleEndian.Uint32(b[8:12])) != uint64(f.format.SampleRate)*uint64(f.format.Channels) {
		return f, fmt.Errorf("%w: G.711 block alignment/byte rate inconsistent with format", ErrInvalidInput)
	}
	return f, nil
}

// Called only after ReadWAV checks frame alignment and all decoded size limits.
// No block headers, nibble order, predictor state or tail padding exist in G.711.
func readWAVG711(r io.Reader, frames int, f wavFormat) (*PCM, error) {
	pcm := &PCM{format: f.format, samples: make([]float64, frames*f.format.Channels)}
	table := &wavG711PCM[f.tag-6]
	var block [32768]byte
	for offset := 0; offset < len(pcm.samples); {
		count := min(len(pcm.samples)-offset, len(block))
		if _, err := io.ReadFull(r, block[:count]); err != nil {
			return nil, fmt.Errorf("audio: G.711 samples: %w", err)
		}
		for i, code := range block[:count] {
			pcm.samples[offset+i] = table[code]
		}
		offset += count
	}
	return pcm, nil
}
