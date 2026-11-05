package audio

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
)

// WAVEncoding selects signed integer PCM or IEEE floating-point samples.
type WAVEncoding uint8

const (
	PCM16 WAVEncoding = iota + 1
	PCM24
	PCM32
	Float32
	Float64
)

func (e WAVEncoding) parameters() (tag, bits uint16, err error) {
	switch e {
	case PCM16:
		return 1, 16, nil
	case PCM24:
		return 1, 24, nil
	case PCM32:
		return 1, 32, nil
	case Float32:
		return 3, 32, nil
	case Float64:
		return 3, 64, nil
	default:
		return 0, 0, fmt.Errorf("%w: WAVE encoding %d", ErrUnsupported, e)
	}
}

type WAVReadOptions struct {
	Limits Limits
	// DiscardMetadata explicitly permits discarding LIST/INFO, bext, iXML,
	// axml and ID3 chunks. Default behavior rejects these rather than losing
	// annotations silently. JUNK/PAD filler is always skipped. Unknown chunks,
	// playlists, loops, wavl and extra audio data chunks are always rejected.
	DiscardMetadata bool
}

type WAVWriteOptions struct {
	// Encoding defaults to PCM16. Integer encodings accept only [-1,1].
	Encoding WAVEncoding
	Limits   Limits
}

type wavFormat struct {
	format             Format
	tag                uint16
	bits               uint16
	align              int
	imaSamplesPerBlock int
	msSamplesPerBlock  int
	msCoefficients     [][2]int16
}

// ReadWAV reads exactly one little-endian RIFF/WAVE object from r, leaving any
// following stream bytes unread. The declared RIFF size is bounded before chunk
// processing. Chunk extents, padding presence, format fields and finite audio
// samples are checked. One fmt and one data chunk are supported, with fmt first.
// WAVE_FORMAT_EXTENSIBLE is supported for full-width PCM/IEEE with a valid mask;
// Microsoft IMA ADPCM (tag 0x11) supports 4-bit mono/stereo complete blocks and
// requires fact; only the last block may contain samples beyond fact's count.
// Microsoft ADPCM (tag 2) supports bounded 4-bit mono/stereo complete blocks,
// a validated coefficient table, final-block fact trimming and zero RIFF pads.
// G.711 A-law/mu-law (tags 6/7) supports 8-bit indexed channels, requires exact
// fact frame counts and zero RIFF pad bytes, and never trims or inserts samples.
// Other compression, reduced valid-bit depth, RIFX and RF64 are explicit errors.
// PCM/IEEE fact is optional and must match the decoded frame count when present.
func ReadWAV(r io.Reader, options WAVReadOptions) (*PCM, error) {
	if r == nil {
		return nil, fmt.Errorf("%w: nil WAVE reader", ErrInvalidInput)
	}
	l, err := options.Limits.normalized()
	if err != nil {
		return nil, err
	}
	if l.MaxBytes < 12 {
		return nil, fmt.Errorf("%w: WAVE header requires 12 bytes", ErrLimit)
	}
	var header [12]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, fmt.Errorf("audio: WAVE header: %w", err)
	}
	if string(header[:4]) != "RIFF" || string(header[8:]) != "WAVE" {
		return nil, fmt.Errorf("%w: expected little-endian RIFF/WAVE", ErrUnsupported)
	}
	size := int64(binary.LittleEndian.Uint32(header[4:8]))
	if size < 4 {
		return nil, fmt.Errorf("%w: RIFF size smaller than WAVE type", ErrInvalidInput)
	}
	if size+8 > l.MaxBytes {
		return nil, fmt.Errorf("%w: RIFF requires %d bytes", ErrLimit, size+8)
	}
	reader := &io.LimitedReader{R: r, N: size - 4}
	var (
		format     wavFormat
		pcm        *PCM
		seenFmt    bool
		seenFact   bool
		fact       uint32
		nonzeroPad bool
	)
	for count := 0; reader.N != 0; count++ {
		if count >= l.MaxChunks {
			return nil, fmt.Errorf("%w: too many RIFF chunks", ErrLimit)
		}
		var chunk [8]byte
		if reader.N < 8 {
			return nil, fmt.Errorf("%w: partial RIFF chunk header", ErrInvalidInput)
		}
		if _, err := io.ReadFull(reader, chunk[:]); err != nil {
			return nil, fmt.Errorf("audio: RIFF chunk header: %w", err)
		}
		id := string(chunk[:4])
		n := int64(binary.LittleEndian.Uint32(chunk[4:]))
		if n+(n&1) > reader.N {
			return nil, fmt.Errorf("%w: chunk %q crosses RIFF boundary (including pad)", ErrInvalidInput, id)
		}
		switch id {
		case "fmt ":
			if seenFmt {
				return nil, fmt.Errorf("%w: duplicate WAVE fmt chunk", ErrUnsupported)
			}
			format, err = readWAVFormat(reader, n)
			if err != nil {
				return nil, err
			}
			seenFmt = true
		case "data":
			if !seenFmt || pcm != nil {
				return nil, fmt.Errorf("%w: WAVE requires one data chunk after fmt", ErrUnsupported)
			}
			if format.imaSamplesPerBlock != 0 {
				pcm, err = readWAVIMA(reader, n, format, l)
				if err != nil {
					return nil, err
				}
				break
			}
			if format.msSamplesPerBlock != 0 {
				pcm, err = readWAVMS(reader, n, format, l)
				if err != nil {
					return nil, err
				}
				break
			}
			if n%int64(format.align) != 0 {
				return nil, fmt.Errorf("%w: incomplete WAVE frame", ErrInvalidInput)
			}
			frames := uint64(n / int64(format.align))
			if err := l.checkFrames(frames, format.format.Channels); err != nil {
				return nil, err
			}
			pcm, err = readWAVSamples(reader, int(frames), format)
			if err != nil {
				return nil, err
			}
		case "fact":
			if seenFact || n != 4 {
				return nil, fmt.Errorf("%w: unsupported fact chunk length or duplication", ErrUnsupported)
			}
			var data [4]byte
			if _, err := io.ReadFull(reader, data[:]); err != nil {
				return nil, fmt.Errorf("audio: WAVE fact: %w", err)
			}
			fact, seenFact = binary.LittleEndian.Uint32(data[:]), true
		case "JUNK", "PAD ":
			err = discardWAVChunk(reader, n)
		case "LIST":
			if n < 4 {
				return nil, fmt.Errorf("%w: LIST chunk lacks type", ErrInvalidInput)
			}
			var kind [4]byte
			if _, err := io.ReadFull(reader, kind[:]); err != nil {
				return nil, fmt.Errorf("audio: WAVE LIST type: %w", err)
			}
			if string(kind[:]) != "INFO" || !options.DiscardMetadata {
				return nil, fmt.Errorf("%w: LIST/%s; metadata discard requires explicit opt-in", ErrUnsupported, kind)
			}
			err = discardWAVChunk(reader, n-4)
		case "bext", "iXML", "axml", "ID3 ", "id3 ":
			if !options.DiscardMetadata {
				return nil, fmt.Errorf("%w: metadata %q requires explicit discard opt-in", ErrUnsupported, id)
			}
			err = discardWAVChunk(reader, n)
		default:
			return nil, fmt.Errorf("%w: RIFF chunk %q", ErrUnsupported, id)
		}
		if err != nil {
			return nil, err
		}
		if n&1 != 0 {
			var pad [1]byte
			if _, err := io.ReadFull(reader, pad[:]); err != nil {
				return nil, fmt.Errorf("audio: RIFF padding: %w", err)
			}
			nonzeroPad = nonzeroPad || pad[0] != 0
		}
	}
	if pcm == nil {
		return nil, fmt.Errorf("%w: WAVE lacks fmt/data", ErrInvalidInput)
	}
	if format.msSamplesPerBlock != 0 {
		if !seenFact || nonzeroPad {
			return nil, fmt.Errorf("%w: Microsoft ADPCM requires fact and zero RIFF pads", ErrInvalidInput)
		}
		capacity, frames := uint64(pcm.Frames()), uint64(fact)
		if frames > capacity || (capacity != 0 && frames <= capacity-uint64(format.msSamplesPerBlock)) {
			return nil, fmt.Errorf("%w: Microsoft ADPCM fact must end in the last block", ErrInvalidInput)
		}
		pcm.samples = pcm.samples[:int(frames)*format.format.Channels]
		return pcm, nil
	}
	if format.imaSamplesPerBlock != 0 {
		if !seenFact {
			return nil, fmt.Errorf("%w: IMA ADPCM requires fact frame count", ErrInvalidInput)
		}
		capacity := uint64(pcm.Frames())
		frames := uint64(fact)
		if frames > capacity || (capacity != 0 && frames <= capacity-uint64(format.imaSamplesPerBlock)) {
			return nil, fmt.Errorf("%w: IMA fact must end in the last block", ErrInvalidInput)
		}
		pcm.samples = pcm.samples[:int(frames)*format.format.Channels]
		return pcm, nil
	}
	if format.tag == 6 || format.tag == 7 {
		if !seenFact {
			return nil, fmt.Errorf("%w: G.711 requires fact frame count", ErrInvalidInput)
		}
		if nonzeroPad {
			return nil, fmt.Errorf("%w: G.711 WAVE requires zero RIFF pad bytes", ErrInvalidInput)
		}
	}
	if seenFact && uint64(fact) != uint64(pcm.Frames()) {
		return nil, fmt.Errorf("%w: fact frame count disagrees with data", ErrInvalidInput)
	}
	return pcm, nil
}

func discardWAVChunk(r io.Reader, n int64) error {
	if _, err := io.CopyN(io.Discard, r, n); err != nil {
		return fmt.Errorf("audio: RIFF chunk payload: %w", err)
	}
	return nil
}

func readWAVFormat(r io.Reader, n int64) (wavFormat, error) {
	var f wavFormat
	if n != 16 && n != 18 && n != 20 && n != 40 {
		return readWAVMSFormat(r, n)
	}
	var b [40]byte
	if _, err := io.ReadFull(r, b[:int(n)]); err != nil {
		return f, fmt.Errorf("audio: WAVE fmt: %w", err)
	}
	f.tag = binary.LittleEndian.Uint16(b[:2])
	f.format.Channels = int(binary.LittleEndian.Uint16(b[2:4]))
	rate := binary.LittleEndian.Uint32(b[4:8])
	if rate > MaxSampleRate {
		return f, fmt.Errorf("%w: WAVE sample rate %d", ErrUnsupported, rate)
	}
	f.format.SampleRate = int(rate)
	f.align = int(binary.LittleEndian.Uint16(b[12:14]))
	f.bits = binary.LittleEndian.Uint16(b[14:16])
	if f.tag == 0x11 {
		return validateWAVIMAFormat(f, b[:int(n)])
	}
	if f.tag == 6 || f.tag == 7 {
		return validateWAVG711Format(f, b[:int(n)])
	}
	if n == 20 {
		return f, fmt.Errorf("%w: unexpected WAVE format extension", ErrUnsupported)
	}
	if f.tag == 0xfffe {
		if n != 40 || binary.LittleEndian.Uint16(b[16:18]) != 22 || binary.LittleEndian.Uint16(b[18:20]) != f.bits {
			return f, fmt.Errorf("%w: extensible WAVE requires full-width valid bits and 22-byte extension", ErrUnsupported)
		}
		f.format.ChannelMask = binary.LittleEndian.Uint32(b[20:24])
		// KSDATAFORMAT_SUBTYPE_{PCM,IEEE_FLOAT}: tag + base GUID.
		base := [14]byte{0, 0, 0, 0, 0x10, 0, 0x80, 0, 0, 0xaa, 0, 0x38, 0x9b, 0x71}
		if string(b[26:40]) != string(base[:]) {
			return f, fmt.Errorf("%w: extensible WAVE subformat GUID", ErrUnsupported)
		}
		f.tag = binary.LittleEndian.Uint16(b[24:26])
	} else if (n == 18 && binary.LittleEndian.Uint16(b[16:18]) != 0) || n == 40 {
		return f, fmt.Errorf("%w: unexpected WAVE format extension", ErrUnsupported)
	}
	if !((f.tag == 1 && (f.bits == 16 || f.bits == 24 || f.bits == 32)) || (f.tag == 3 && (f.bits == 32 || f.bits == 64))) {
		return f, fmt.Errorf("%w: WAVE tag %d, %d bits", ErrUnsupported, f.tag, f.bits)
	}
	if err := f.format.validate(); err != nil {
		return f, err
	}
	expectedAlign := f.format.Channels * int(f.bits/8)
	expectedRate := uint64(rate) * uint64(expectedAlign)
	if f.align != expectedAlign || uint64(binary.LittleEndian.Uint32(b[8:12])) != expectedRate {
		return f, fmt.Errorf("%w: WAVE block alignment/byte rate inconsistent with format", ErrInvalidInput)
	}
	return f, nil
}

func readWAVSamples(r io.Reader, frames int, f wavFormat) (*PCM, error) {
	if f.tag == 6 || f.tag == 7 {
		return readWAVG711(r, frames, f)
	}
	pcm := &PCM{format: f.format, samples: make([]float64, frames*f.format.Channels)}
	var block [32768]byte
	bytesPerSample := int(f.bits / 8)
	for offset := 0; offset < len(pcm.samples); {
		count := min(len(pcm.samples)-offset, len(block)/bytesPerSample)
		data := block[:count*bytesPerSample]
		if _, err := io.ReadFull(r, data); err != nil {
			return nil, fmt.Errorf("audio: WAVE samples: %w", err)
		}
		for i := 0; i < count; i++ {
			b := data[i*bytesPerSample : (i+1)*bytesPerSample]
			var value float64
			switch {
			case f.tag == 1 && f.bits == 16:
				value = float64(int16(binary.LittleEndian.Uint16(b))) / 32768
			case f.tag == 1 && f.bits == 24:
				integer := int32(b[0]) | int32(b[1])<<8 | int32(b[2])<<16
				integer = integer << 8 >> 8
				value = float64(integer) / 8388608
			case f.tag == 1 && f.bits == 32:
				value = float64(int32(binary.LittleEndian.Uint32(b))) / 2147483648
			case f.tag == 3 && f.bits == 32:
				value = float64(math.Float32frombits(binary.LittleEndian.Uint32(b)))
			case f.tag == 3 && f.bits == 64:
				value = math.Float64frombits(binary.LittleEndian.Uint64(b))
			}
			if !finite(value) {
				return nil, fmt.Errorf("%w: nonfinite WAVE sample %d", ErrInvalidInput, offset+i)
			}
			pcm.samples[offset+i] = value
		}
		offset += count
	}
	return pcm, nil
}

// WriteWAV writes a bounded RIFF/WAVE, streaming samples through 32 KiB scratch
// storage. Formats with a speaker mask or more than two channels use extensible
// headers; floating-point encodings include fact. Validation precedes all I/O.
// Integers quantize by rounding to nearest (ties away from zero), with +1 mapped
// to the greatest positive code and -1 to the least negative code. Values outside
// [-1,1] are errors. Float32 export rejects overflow; Float64 retains all bits.
// I/O failures can leave a partial stream, as with any non-transactional writer.
func WriteWAV(w io.Writer, pcm *PCM, options WAVWriteOptions) error {
	if w == nil {
		return fmt.Errorf("%w: nil WAVE writer", ErrInvalidInput)
	}
	if err := pcm.validate(); err != nil {
		return err
	}
	l, err := options.Limits.normalized()
	if err != nil {
		return err
	}
	if err := l.checkFrames(uint64(pcm.Frames()), pcm.Channels()); err != nil {
		return err
	}
	encoding := options.Encoding
	if encoding == 0 {
		encoding = PCM16
	}
	tag, bits, err := encoding.parameters()
	if err != nil {
		return err
	}
	for i, v := range pcm.samples {
		if !finite(v) || (tag == 1 && (v < -1 || v > 1)) || (tag == 3 && bits == 32 && math.Abs(v) > math.MaxFloat32) {
			return fmt.Errorf("%w: sample %d outside WAVE encoding range", ErrInvalidInput, i)
		}
	}
	var (
		extensible  = pcm.format.ChannelMask != 0 || pcm.Channels() > 2
		formatBytes = 16
		chunkCount  = 2
	)
	if extensible {
		formatBytes = 40
	} else if tag == 3 {
		formatBytes = 18
	}
	headerBytes := 12 + 8 + formatBytes + 8
	if tag == 3 {
		headerBytes += 12
		chunkCount++
	}
	if chunkCount > l.MaxChunks {
		return fmt.Errorf("%w: WAVE writer chunk count", ErrLimit)
	}
	// Check against the 32-bit RIFF limit before multiplying sample counts.
	maxPayload := uint64(math.MaxUint32) - uint64(headerBytes-8) - 1
	bytesPerSample := int(bits / 8)
	if uint64(len(pcm.samples)) > maxPayload/uint64(bytesPerSample) {
		return fmt.Errorf("%w: data exceeds RIFF 32-bit size", ErrLimit)
	}
	payloadBytes := uint64(len(pcm.samples)) * uint64(bytesPerSample)
	totalBytes := uint64(headerBytes) + payloadBytes + payloadBytes&1
	if totalBytes > uint64(l.MaxBytes) {
		return fmt.Errorf("%w: WAVE requires %d bytes", ErrLimit, totalBytes)
	}
	var header [80]byte
	copy(header[:4], "RIFF")
	binary.LittleEndian.PutUint32(header[4:8], uint32(totalBytes-8))
	copy(header[8:16], "WAVEfmt ")
	binary.LittleEndian.PutUint32(header[16:20], uint32(formatBytes))
	fmtTag := tag
	if extensible {
		fmtTag = 0xfffe
	}
	binary.LittleEndian.PutUint16(header[20:22], fmtTag)
	binary.LittleEndian.PutUint16(header[22:24], uint16(pcm.Channels()))
	binary.LittleEndian.PutUint32(header[24:28], uint32(pcm.SampleRate()))
	align := pcm.Channels() * bytesPerSample
	binary.LittleEndian.PutUint32(header[28:32], uint32(pcm.SampleRate()*align))
	binary.LittleEndian.PutUint16(header[32:34], uint16(align))
	binary.LittleEndian.PutUint16(header[34:36], bits)
	if extensible {
		binary.LittleEndian.PutUint16(header[36:38], 22)
		binary.LittleEndian.PutUint16(header[38:40], bits)
		binary.LittleEndian.PutUint32(header[40:44], pcm.format.ChannelMask)
		binary.LittleEndian.PutUint16(header[44:46], tag)
		copy(header[46:60], []byte{0, 0, 0, 0, 0x10, 0, 0x80, 0, 0, 0xaa, 0, 0x38, 0x9b, 0x71})
	}
	next := 20 + formatBytes
	if tag == 3 {
		copy(header[next:next+4], "fact")
		binary.LittleEndian.PutUint32(header[next+4:next+8], 4)
		binary.LittleEndian.PutUint32(header[next+8:next+12], uint32(pcm.Frames()))
		next += 12
	}
	copy(header[next:next+4], "data")
	binary.LittleEndian.PutUint32(header[next+4:next+8], uint32(payloadBytes))
	if err := writeWAVBlock(w, header[:headerBytes]); err != nil {
		return err
	}
	var block [32768]byte
	for offset := 0; offset < len(pcm.samples); {
		count := min(len(pcm.samples)-offset, len(block)/bytesPerSample)
		data := block[:count*bytesPerSample]
		for i := 0; i < count; i++ {
			b := data[i*bytesPerSample : (i+1)*bytesPerSample]
			v := pcm.samples[offset+i]
			if tag == 1 {
				scale := math.Ldexp(1, int(bits)-1)
				quantized := int64(math.Round(v * scale))
				quantized = min(quantized, int64(scale)-1)
				switch bits {
				case 16:
					binary.LittleEndian.PutUint16(b, uint16(quantized))
				case 24:
					b[0], b[1], b[2] = byte(quantized), byte(quantized>>8), byte(quantized>>16)
				case 32:
					binary.LittleEndian.PutUint32(b, uint32(quantized))
				}
			} else if bits == 32 {
				binary.LittleEndian.PutUint32(b, math.Float32bits(float32(v)))
			} else {
				binary.LittleEndian.PutUint64(b, math.Float64bits(v))
			}
		}
		if err := writeWAVBlock(w, data); err != nil {
			return err
		}
		offset += count
	}
	if payloadBytes&1 != 0 {
		return writeWAVBlock(w, []byte{0})
	}
	return nil
}

func writeWAVBlock(w io.Writer, data []byte) error {
	n, err := w.Write(data)
	if err != nil {
		return fmt.Errorf("audio: writing WAVE: %w", err)
	}
	if n != len(data) {
		return fmt.Errorf("audio: writing WAVE: %w", io.ErrShortWrite)
	}
	return nil
}
