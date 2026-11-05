package audio

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
)

// AUReadOptions selects strict, bounded Sun/NeXT AU decoding. Limits use the
// existing defaults; MaxBytes includes the fixed header, annotation and data.
// AU has no chunks, so MaxChunks has no further effect after validation.
// DiscardMetadata permits discarding arbitrary annotation bytes. Without it,
// only an absent or all-zero annotation is accepted.
// AllowUnknownSize permits dataSize=0xffffffff and consumes data through EOF.
// The default rejects that unframed representation rather than consuming a
// following object. Use a separately bounded reader to delimit such a stream.
type AUReadOptions struct {
	Limits           Limits
	DiscardMetadata  bool
	AllowUnknownSize bool
}

// ReadAU reads big-endian .snd PCM8/16/24/32 or G.711 mu-law(1)/A-law(27).
// Output keeps interleaved file channel indices, ChannelMask=0, with integer
// PCM normalized by 2^(bits-1) and G.711 by 32768. Float, ADPCM, fragmented,
// host/little-endian variants and other encodings are explicitly unsupported.
//
// Offsets are byte offsets from the initial magic: 24 for no annotation, or
// >=28 for the at-least-four-byte info field. No alignment or terminator is
// inferred. The annotation is exactly offset-24 bytes. Known
// data sizes must contain complete channel frames; only those bytes are read,
// leaving all following stream bytes unread. Short input is an error.
// Unknown-size data requires AllowUnknownSize and a clean EOF at a complete
// frame. At a byte/frame/sample budget boundary a single-byte probe is needed
// to distinguish exact EOF from excess data; that byte is consumed on failure.
// No data is silently truncated to a budget and no partial PCM is returned.
//
// Known sizes are checked before annotation reads and allocation. Unknown
// input grows decoded storage only after reading complete frames, with each
// old/new array individually bounded by Limits and native int storage limits.
// Growth can temporarily retain both arrays. No encoded file buffer, seeking,
// native library or subprocess is used. Readers returning no progress 100
// consecutive times fail with io.ErrNoProgress; I/O errors retain their chain.
func ReadAU(r io.Reader, options AUReadOptions) (*PCM, error) {
	if r == nil {
		return nil, fmt.Errorf("%w: nil AU reader", ErrInvalidInput)
	}
	limits, err := options.Limits.normalized()
	if err != nil {
		return nil, err
	}
	if limits.MaxBytes < 24 {
		return nil, fmt.Errorf("%w: AU header requires 24 bytes", ErrLimit)
	}
	var header [24]byte
	if err = readAUFull(r, header[:]); err != nil {
		return nil, err
	}
	if string(header[:4]) != ".snd" {
		return nil, fmt.Errorf("%w: expected big-endian AU .snd", ErrUnsupported)
	}
	offset := int64(binary.BigEndian.Uint32(header[4:8]))
	size := binary.BigEndian.Uint32(header[8:12])
	encoding := binary.BigEndian.Uint32(header[12:16])
	rate, channels := binary.BigEndian.Uint32(header[16:20]), binary.BigEndian.Uint32(header[20:24])
	if offset < 24 {
		return nil, fmt.Errorf("%w: AU data offset precedes header", ErrInvalidInput)
	}
	if offset > 24 && offset < 28 {
		return nil, fmt.Errorf("%w: AU info field shorter than four bytes", ErrUnsupported)
	}
	if offset > limits.MaxBytes {
		return nil, fmt.Errorf("%w: AU annotation/offset byte budget", ErrLimit)
	}
	if rate == 0 || channels == 0 {
		return nil, fmt.Errorf("%w: AU zero sample rate/channels", ErrInvalidInput)
	}
	if rate > MaxSampleRate || channels > MaxChannels {
		return nil, fmt.Errorf("%w: AU rate/channels outside supported range", ErrUnsupported)
	}
	format := Format{SampleRate: int(rate), Channels: int(channels)}
	width := 0
	switch encoding {
	case 1, 2, 27:
		width = 1
	case 3:
		width = 2
	case 4:
		width = 3
	case 5:
		width = 4
	default:
		return nil, fmt.Errorf("%w: AU encoding %d", ErrUnsupported, encoding)
	}
	align := width * format.Channels
	unknown := size == math.MaxUint32
	if unknown && !options.AllowUnknownSize {
		return nil, fmt.Errorf("%w: AU unknown data size requires AllowUnknownSize", ErrUnsupported)
	}
	if !unknown {
		if int64(size) > limits.MaxBytes-offset {
			return nil, fmt.Errorf("%w: AU declared byte budget", ErrLimit)
		}
		if uint64(size)%uint64(align) != 0 {
			return nil, fmt.Errorf("%w: AU data size contains partial channel frame", ErrInvalidInput)
		}
		if err = limits.checkFrames(uint64(size)/uint64(align), format.Channels); err != nil {
			return nil, err
		}
	}
	var annotation [4096]byte
	for left := offset - 24; left > 0; {
		count := min(left, int64(len(annotation)))
		if err = readAUFull(r, annotation[:int(count)]); err != nil {
			return nil, err
		}
		if !options.DiscardMetadata {
			for _, b := range annotation[:int(count)] {
				if b != 0 {
					return nil, fmt.Errorf("%w: AU annotation requires DiscardMetadata", ErrUnsupported)
				}
			}
		}
		left -= count
	}
	if unknown {
		return readAUUnknown(r, format, encoding, width, limits, limits.MaxBytes-offset)
	}
	frames := int(uint64(size) / uint64(align))
	pcm := &PCM{format: format, samples: make([]float64, frames*format.Channels)}
	var block [32768]byte
	for start := 0; start < len(pcm.samples); {
		count := min(len(pcm.samples)-start, len(block)/width)
		data := block[:count*width]
		if err = readAUFull(r, data); err != nil {
			return nil, err
		}
		decodeAUSamples(pcm.samples[start:start+count], data, encoding, width)
		start += count
	}
	return pcm, nil
}

// Unlike io.ReadFull this keeps a non-EOF error returned alongside a full
// buffer. Unknown-size input must not turn a transport failure into clean EOF.
func readAUBuffer(r io.Reader, p []byte) (int, error) {
	total, empty := 0, 0
	for total < len(p) {
		n, err := r.Read(p[total:])
		if n < 0 || n > len(p)-total {
			return total, fmt.Errorf("%w: AU reader returned invalid count", ErrInvalidInput)
		}
		total += n
		if err != nil {
			return total, err
		}
		if n == 0 {
			empty++
			if empty >= 100 {
				return total, io.ErrNoProgress
			}
		} else {
			empty = 0
		}
	}
	return total, nil
}
func readAUFull(r io.Reader, p []byte) error {
	n, err := readAUBuffer(r, p)
	if n == len(p) && (err == nil || err == io.EOF) {
		return nil
	}
	if err == io.EOF {
		err = io.ErrUnexpectedEOF
	}
	return fmt.Errorf("audio: AU bytes: %w", err)
}
func readAUUnknown(r io.Reader, f Format, encoding uint32, width int, limits Limits, byteBudget int64) (*PCM, error) {
	align := width * f.Channels
	maxFrames := min(uint64(limits.MaxFrames), uint64(limits.MaxSamples)/uint64(f.Channels), uint64(int(^uint(0)>>1)/8)/uint64(f.Channels), uint64(byteBudget)/uint64(align))
	maxSamples := int(maxFrames) * f.Channels
	pcm := &PCM{format: f}
	var block [32768]byte
	for {
		remaining := maxFrames - uint64(pcm.Frames())
		if remaining == 0 {
			n, err := readAUBuffer(r, block[:1])
			if n == 0 && err == io.EOF {
				return pcm, nil
			}
			if err != nil && err != io.EOF {
				return nil, fmt.Errorf("audio: AU EOF probe: %w", err)
			}
			return nil, fmt.Errorf("%w: AU unknown-size data exceeds byte/frame/sample budget", ErrLimit)
		}
		count := int(min(remaining, uint64(len(block)/align))) * align
		n, err := readAUBuffer(r, block[:count])
		if err != nil && err != io.EOF {
			return nil, fmt.Errorf("audio: AU unknown-size samples: %w", err)
		}
		if n%align != 0 {
			return nil, fmt.Errorf("%w: AU EOF in channel frame: %w", ErrInvalidInput, io.ErrUnexpectedEOF)
		}
		samples := n / width
		needed := len(pcm.samples) + samples
		if needed > cap(pcm.samples) {
			capacity := min(maxSamples, max(needed, 2*cap(pcm.samples)))
			grown := make([]float64, needed, capacity)
			copy(grown, pcm.samples)
			pcm.samples = grown
		} else {
			pcm.samples = pcm.samples[:needed]
		}
		decodeAUSamples(pcm.samples[needed-samples:], block[:n], encoding, width)
		if err == io.EOF {
			return pcm, nil
		}
	}
}
func decodeAUSamples(dst []float64, data []byte, encoding uint32, width int) {
	if encoding == 1 || encoding == 27 {
		index := 0
		if encoding == 1 {
			index = 1
		}
		table := &wavG711PCM[index]
		for i, code := range data {
			dst[i] = table[code]
		}
		return
	}
	scale := float64(uint64(1) << uint(width*8-1))
	for i := range dst {
		b := data[i*width:]
		var value int32
		switch width {
		case 1:
			value = int32(int8(b[0]))
		case 2:
			value = int32(int16(binary.BigEndian.Uint16(b)))
		case 3:
			value = (int32(b[0])<<24 | int32(b[1])<<16 | int32(b[2])<<8) >> 8
		case 4:
			value = int32(binary.BigEndian.Uint32(b))
		}
		dst[i] = float64(value) / scale
	}
}
