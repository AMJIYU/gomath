package audio

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"
)

// AIFFReadOptions configures bounded AIFF/AIFF-C integer PCM decoding.
// Zero Limits fields select DefaultLimits; MaxChunks counts all local chunks.
// MaxWork=0 selects 2^32 units. One unit is charged per declared FORM byte
// (including header/pads/unused SSND bytes) and per decoded scalar sample.
// DiscardMetadata permits losing only NAME, AUTH, (c) and ANNO ASCII text.
// Markers, loops, comments, channel layouts, private and unknown chunks are
// always unsupported, including when metadata discard is explicitly enabled.
type AIFFReadOptions struct {
	Limits          Limits
	MaxWork         uint64
	DiscardMetadata bool
}

type aiffFormat struct {
	format Format
	frames uint32
	depth  int
	little bool
}

// ReadAIFF reads one FORM AIFF/AIFC object, leaving following bytes unread.
// AIFF and AIFC/NONE support signed PCM8/16/24/32; AIFC/sowt supports PCM16.
// Other compression and reduced-width samples are explicit errors. Sample
// rates must be exact positive 80-bit integers in 1..MaxSampleRate. AIFC needs
// exactly one FVER=0xA2805140 and a correctly padded Pascal compression name.
//
// COMM and SSND may appear in either order. COMM-first streams through 32 KiB
// scratch. SSND-first temporarily buffers its post-offset payload, bounded by
// MaxBytes (growth may temporarily retain both old and new arrays). The single
// output float64 array is separately bounded by MaxFrames/MaxSamples. No whole
// file buffer, seeking or external decoder is required.
//
// COMM determines frame count. Offset and trailing unused SSND storage are
// consumed, not decoded; blockSize describes storage alignment, not sample
// gaps. Samples remain contiguous. All chunk/Pascal pad bytes must be zero.
// Output retains file channel order with ChannelMask=0 (indexed channels),
// normalized as signed integer/2^(depth-1), just like integer WAVE/FLAC input.
// No partial PCM is returned on error; underlying I/O errors remain wrapped.
func ReadAIFF(r io.Reader, options AIFFReadOptions) (*PCM, error) {
	if r == nil {
		return nil, aiffInvalid("nil reader")
	}
	limits, err := options.Limits.normalized()
	if err != nil {
		return nil, err
	}
	work := options.MaxWork
	if work == 0 {
		work = 1 << 32
	}
	if limits.MaxBytes < 12 || work < 12 {
		return nil, fmt.Errorf("%w: AIFF header budget", ErrLimit)
	}
	var header [12]byte
	if err := aiffReadFull(r, header[:]); err != nil {
		return nil, err
	}
	form := string(header[8:])
	if string(header[:4]) != "FORM" || (form != "AIFF" && form != "AIFC") {
		return nil, fmt.Errorf("%w: expected FORM AIFF/AIFC", ErrUnsupported)
	}
	size := int64(binary.BigEndian.Uint32(header[4:8]))
	if size < 4 || size > math.MaxInt32 || size&1 != 0 {
		return nil, aiffInvalid("invalid FORM size")
	}
	if size+8 > limits.MaxBytes || uint64(size+8) > work {
		return nil, fmt.Errorf("%w: AIFF FORM byte/work budget", ErrLimit)
	}
	work -= uint64(size + 8)
	reader := &io.LimitedReader{R: r, N: size - 4}
	var (
		format                       aiffFormat
		pcm                          *PCM
		seenCOMM, seenSSND, seenFVER bool
		seenText                     uint8
		deferred                     []byte
	)
	for count := 0; reader.N != 0; count++ {
		if count >= limits.MaxChunks {
			return nil, fmt.Errorf("%w: AIFF chunk count", ErrLimit)
		}
		if reader.N < 8 {
			return nil, aiffInvalid("partial chunk header")
		}
		var chunk [8]byte
		if err := aiffReadFull(reader, chunk[:]); err != nil {
			return nil, err
		}
		id, n := string(chunk[:4]), int64(binary.BigEndian.Uint32(chunk[4:]))
		if n > math.MaxInt32 || n+(n&1) > reader.N {
			return nil, aiffInvalid("chunk length/pad crosses FORM boundary")
		}
		switch id {
		case "COMM":
			if seenCOMM {
				return nil, aiffInvalid("duplicate COMM")
			}
			format, err = readAIFFCommon(reader, n, form == "AIFC")
			if err != nil {
				return nil, err
			}
			if err := limits.checkFrames(uint64(format.frames), format.format.Channels); err != nil {
				return nil, err
			}
			samples := uint64(format.frames) * uint64(format.format.Channels)
			if samples > work {
				return nil, fmt.Errorf("%w: AIFF sample work budget", ErrLimit)
			}
			work -= samples
			seenCOMM = true
			if seenSSND {
				pcm, err = readAIFFSound(bytes.NewReader(deferred), int64(len(deferred)), format)
				deferred = nil
			}
		case "SSND":
			if seenSSND || n < 8 {
				return nil, aiffInvalid("duplicate/short SSND")
			}
			var sound [8]byte
			if err := aiffReadFull(reader, sound[:]); err != nil {
				return nil, err
			}
			offset := int64(binary.BigEndian.Uint32(sound[:4]))
			// blockSize is an advisory storage-alignment size. PCM contains no
			// padding between frames regardless of its value (AIFF 1.3 p10).
			if offset > n-8 {
				return nil, aiffInvalid("SSND offset exceeds payload")
			}
			if err := discardAIFFBytes(reader, offset); err != nil {
				return nil, err
			}
			seenSSND = true
			if seenCOMM {
				pcm, err = readAIFFSound(reader, n-8-offset, format)
			} else {
				deferred, err = bufferAIFFSound(reader, n-8-offset)
			}
		case "FVER":
			if form != "AIFC" {
				return nil, fmt.Errorf("%w: FVER in AIFF", ErrUnsupported)
			}
			if seenFVER || n != 4 {
				return nil, aiffInvalid("duplicate/invalid FVER")
			}
			var version [4]byte
			if err := aiffReadFull(reader, version[:]); err != nil {
				return nil, err
			}
			if binary.BigEndian.Uint32(version[:]) != 0xa2805140 {
				return nil, fmt.Errorf("%w: AIFC version", ErrUnsupported)
			}
			seenFVER = true
		case "NAME", "AUTH", "(c) ", "ANNO":
			if !options.DiscardMetadata {
				return nil, fmt.Errorf("%w: AIFF %s requires DiscardMetadata", ErrUnsupported, id)
			}
			var flag uint8
			switch id {
			case "NAME":
				flag = 1
			case "AUTH":
				flag = 2
			case "(c) ":
				flag = 4
			}
			if seenText&flag != 0 {
				return nil, aiffInvalid("duplicate singleton text chunk")
			}
			seenText |= flag
			err = discardAIFFText(reader, n)
		default:
			return nil, fmt.Errorf("%w: AIFF chunk %q", ErrUnsupported, id)
		}
		if err != nil {
			return nil, err
		}
		if n&1 != 0 {
			var pad [1]byte
			if err := aiffReadFull(reader, pad[:]); err != nil {
				return nil, err
			}
			if pad[0] != 0 {
				return nil, aiffInvalid("nonzero chunk pad")
			}
		}
	}
	if !seenCOMM || (form == "AIFC" && !seenFVER) || (!seenSSND && format.frames != 0) {
		return nil, aiffInvalid("missing COMM, FVER or required SSND")
	}
	if pcm == nil { // COMM with zero frames can legally omit SSND.
		pcm = &PCM{format: format.format}
	}
	return pcm, nil
}

func aiffInvalid(reason string) error {
	return fmt.Errorf("%w: AIFF %s", ErrInvalidInput, reason)
}

func aiffReadFull(r io.Reader, p []byte) error {
	if _, err := io.ReadFull(r, p); err != nil {
		return fmt.Errorf("%w: AIFF data: %w", ErrInvalidInput, err)
	}
	return nil
}

func discardAIFFBytes(r io.Reader, n int64) error {
	var block [4096]byte
	for n > 0 {
		count := min(n, int64(len(block)))
		if err := aiffReadFull(r, block[:int(count)]); err != nil {
			return err
		}
		n -= count
	}
	return nil
}

func readAIFFCommon(r io.Reader, n int64, compressed bool) (aiffFormat, error) {
	var f aiffFormat
	if (!compressed && n != 18) || (compressed && (n < 24 || n > 278 || n&1 != 0)) {
		return f, aiffInvalid("COMM size")
	}
	var data [278]byte
	if err := aiffReadFull(r, data[:int(n)]); err != nil {
		return f, err
	}
	f.format.Channels = int(binary.BigEndian.Uint16(data[:2]))
	f.frames = binary.BigEndian.Uint32(data[2:6])
	f.depth = int(binary.BigEndian.Uint16(data[6:8]))
	rate, err := aiffSampleRate(data[8:18])
	if err != nil {
		return f, err
	}
	f.format.SampleRate = rate
	if err := f.format.validate(); err != nil {
		return f, err
	}
	if f.depth != 8 && f.depth != 16 && f.depth != 24 && f.depth != 32 {
		return f, fmt.Errorf("%w: AIFF sample depth %d", ErrUnsupported, f.depth)
	}
	if compressed {
		// The pstring includes its count byte and its own zero pad; the
		// compression type, not this localized display name, selects decoding.
		length := int(data[22]) + 1
		padded := length + length&1
		if int(n) != 22+padded || (length&1 != 0 && data[22+length] != 0) {
			return f, aiffInvalid("AIFC Pascal compression name length/pad")
		}
		switch string(data[18:22]) {
		case "NONE":
		case "sowt":
			if f.depth != 16 {
				return f, fmt.Errorf("%w: AIFC/sowt requires PCM16", ErrUnsupported)
			}
			f.little = true
		default:
			return f, fmt.Errorf("%w: AIFC compression %q", ErrUnsupported, data[18:22])
		}
	}
	return f, nil
}

// Inspect all 64 explicit significand bits; conversion through float64 would
// silently round away small fractional parts, pseudo encodings or overflow.
func aiffSampleRate(b []byte) (int, error) {
	exponent := binary.BigEndian.Uint16(b[:2])
	mantissa := binary.BigEndian.Uint64(b[2:])
	if exponent&0x8000 != 0 || exponent == 0x7fff || mantissa == 0 {
		return 0, aiffInvalid("nonpositive/nonfinite 80-bit sample rate")
	}
	if (exponent == 0) == (mantissa>>63 != 0) {
		return 0, aiffInvalid("nonnormal 80-bit sample rate")
	}
	e := int(exponent) - 16383
	if e < 0 || e > 19 {
		return 0, fmt.Errorf("%w: AIFF requires integer sample rate 1..%d", ErrUnsupported, MaxSampleRate)
	}
	shift := 63 - e
	if mantissa&((uint64(1)<<shift)-1) != 0 {
		return 0, fmt.Errorf("%w: fractional AIFF sample rate", ErrUnsupported)
	}
	rate := int(mantissa >> shift)
	if rate > MaxSampleRate {
		return 0, fmt.Errorf("%w: AIFF sample rate %d", ErrUnsupported, rate)
	}
	return rate, nil
}

func readAIFFSound(r io.Reader, n int64, f aiffFormat) (*PCM, error) {
	count := int(f.frames) * f.format.Channels // checked by Limits.checkFrames
	width := f.depth / 8
	needed := int64(count) * int64(width)
	if needed > n {
		return nil, aiffInvalid("COMM frame count exceeds SSND samples")
	}
	pcm := &PCM{format: f.format, samples: make([]float64, count)}
	var block [32768]byte
	scale := float64(uint64(1) << (f.depth - 1))
	for offset := 0; offset < count; {
		samples := min(count-offset, len(block)/width)
		data := block[:samples*width]
		if err := aiffReadFull(r, data); err != nil {
			return nil, err
		}
		for i := range samples {
			b := data[i*width : (i+1)*width]
			var value int32
			switch f.depth {
			case 8:
				value = int32(int8(b[0]))
			case 16:
				if f.little {
					value = int32(int16(binary.LittleEndian.Uint16(b)))
				} else {
					value = int32(int16(binary.BigEndian.Uint16(b)))
				}
			case 24:
				value = (int32(b[0])<<16 | int32(b[1])<<8 | int32(b[2])) << 8 >> 8
			case 32:
				value = int32(binary.BigEndian.Uint32(b))
			}
			pcm.samples[offset+i] = float64(value) / scale
		}
		offset += samples
	}
	if err := discardAIFFBytes(r, n-needed); err != nil {
		return nil, err
	}
	return pcm, nil
}

// Read before allocating/growing so a huge declared but truncated SSND cannot
// force a huge up-front allocation. Each capacity is bounded by its chunk size.
func bufferAIFFSound(r io.Reader, n int64) ([]byte, error) {
	if uint64(n) > uint64(int(^uint(0)>>1)) {
		return nil, fmt.Errorf("%w: AIFF deferred SSND storage", ErrLimit)
	}
	var data []byte
	var block [4096]byte
	for int64(len(data)) < n {
		count := min(n-int64(len(data)), int64(len(block)))
		if err := aiffReadFull(r, block[:int(count)]); err != nil {
			return nil, err
		}
		if cap(data)-len(data) < int(count) {
			capacity := cap(data) + min(max(cap(data), len(block)), int(n)-cap(data))
			grown := make([]byte, len(data), capacity)
			copy(grown, data)
			data = grown
		}
		data = append(data, block[:int(count)]...)
	}
	return data, nil
}

func discardAIFFText(r io.Reader, n int64) error {
	var block [4096]byte
	for n > 0 {
		count := min(n, int64(len(block)))
		if err := aiffReadFull(r, block[:int(count)]); err != nil {
			return err
		}
		for _, c := range block[:int(count)] {
			if c >= 128 {
				return fmt.Errorf("%w: non-ASCII AIFF text metadata", ErrUnsupported)
			}
		}
		n -= count
	}
	return nil
}
