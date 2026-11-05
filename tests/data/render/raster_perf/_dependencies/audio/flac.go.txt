package audio

import (
	"crypto/md5" // FLAC's required integrity field, not a security primitive.
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"math/bits"
)

// FLACWriteOptions configures native FLAC output (RFC 9639), without a container.
// Encoding reuses the WAVE integer-format constants: only PCM16 and PCM24 are
// supported; zero selects PCM16. Independent mono/stereo channels are supported.
// BlockSize is 16..65535, or zero for 4096. The final frame may be shorter.
// MaxWork defaults to 2^32 budget units. Preflight charges 256 per scalar sample
// plus 128 per frame for the two prediction searches and bounded output work.
// Limits bounds input frames/samples and exact encoded bytes, including metadata.
// MaxChunks counts metadata blocks (one STREAMINFO block is emitted).
type FLACWriteOptions struct {
	Encoding  WAVEncoding
	BlockSize int
	MaxWork   uint64
	Limits    Limits
}

// WriteFLAC writes lossless PCM16/24 mono/stereo to a non-seekable writer.
// Samples must be finite and in [-1,1]. Quantization matches WriteWAV: multiply
// by 2^(bits-1), round nearest with ties away from zero, and map the positive
// endpoint to 2^(bits-1)-1. There is no dither, gain, or out-of-range clipping.
// Losslessness is relative to these quantized integers, not arbitrary float64.
// A zero channel mask selects conventional mono or left/right; explicit masks
// must be front-center (mono) or front-left/front-right (stereo).
// Empty PCM is rejected: STREAMINFO's zero total denotes unknown length.
// All input, size and work checks finish before the first write. I/O errors can
// leave partial output. Auxiliary memory is O(BlockSize*Channels), not O(clip).
// This encoder provides no metadata import, LPC or stereo decorrelation.
// Native FLAC decoding is provided separately by ReadFLAC.
func WriteFLAC(w io.Writer, pcm *PCM, options FLACWriteOptions) error {
	if w == nil {
		return fmt.Errorf("%w: nil FLAC writer", ErrInvalidInput)
	}
	if err := pcm.validate(); err != nil {
		return err
	}
	if pcm.Channels() > 2 {
		return fmt.Errorf("%w: FLAC writer requires mono or stereo", ErrUnsupported)
	}
	wantMask := uint32(4)
	if pcm.Channels() == 2 {
		wantMask = 3
	}
	if pcm.format.ChannelMask != 0 && pcm.format.ChannelMask != wantMask {
		return fmt.Errorf("%w: FLAC writer cannot preserve speaker mask", ErrUnsupported)
	}
	encoding := options.Encoding
	if encoding == 0 {
		encoding = PCM16
	}
	depth := 16
	switch encoding {
	case PCM16:
	case PCM24:
		depth = 24
	default:
		return fmt.Errorf("%w: FLAC encoding must be PCM16 or PCM24", ErrUnsupported)
	}
	block := options.BlockSize
	if block == 0 {
		block = 4096
	}
	if block < 16 || block > 65535 {
		return fmt.Errorf("%w: FLAC block size must be 16..65535", ErrInvalidInput)
	}
	l, err := options.Limits.normalized()
	if err != nil {
		return err
	}
	frames := uint64(pcm.Frames())
	if frames == 0 {
		return fmt.Errorf("%w: FLAC requires nonempty PCM", ErrInvalidInput)
	}
	if err := l.checkFrames(frames, pcm.Channels()); err != nil {
		return err
	}
	count := (frames-1)/uint64(block) + 1
	if frames >= 1<<36 || count > 1<<31 {
		return fmt.Errorf("%w: FLAC sample/frame number field", ErrLimit)
	}
	maxWork := options.MaxWork
	if maxWork == 0 {
		maxWork = 1 << 32
	}
	// Counts have already been bounded by FLAC fields, so these products fit.
	work := uint64(len(pcm.samples))*256 + count*128
	if work > maxWork || l.MaxBytes < 42 {
		return fmt.Errorf("%w: FLAC encoding work or byte budget", ErrLimit)
	}
	for i, v := range pcm.samples {
		if !finite(v) || v < -1 || v > 1 {
			return fmt.Errorf("%w: sample %d outside FLAC integer range", ErrInvalidInput, i)
		}
	}
	// Two searches avoid retaining a plan or complete encoding for every frame.
	// Preflight computes exact frame lengths and MD5; the second pass emits frames.
	channels := pcm.Channels()
	quantized := make([]int64, min(block, pcm.Frames())*channels)
	md5Bytes := make([]byte, len(quantized)*(depth/8))
	digest := md5.New()
	totalBytes := uint64(42)
	minFrame, maxFrame := uint64(math.MaxUint32), uint64(0)
	for start, number := 0, uint32(0); start < pcm.Frames(); number++ {
		n := min(block, pcm.Frames()-start)
		x := quantized[:n*channels]
		flacQuantize(x, pcm.samples[start*channels:(start+n)*channels], depth)
		raw := md5Bytes[:len(x)*(depth/8)]
		for i, v := range x {
			for b := 0; b < depth/8; b++ {
				raw[i*(depth/8)+b] = byte(v >> (8 * b))
			}
		}
		if _, err := digest.Write(raw); err != nil {
			return fmt.Errorf("%w: FLAC PCM MD5 unavailable: %w", ErrUnsupported, err)
		}
		plans := flacPlans(x, channels, depth)
		header := flacHeader(number, n, channels, depth)
		size := flacFrameSize(len(header), plans[:channels])
		if size > uint64(l.MaxBytes)-totalBytes {
			return fmt.Errorf("%w: FLAC encoded bytes", ErrLimit)
		}
		totalBytes += size
		minFrame, maxFrame = min(minFrame, size), max(maxFrame, size)
		start += n
	}
	var stream [42]byte
	copy(stream[:], "fLaC")
	stream[4], stream[7] = 0x80, 34 // Last metadata block, type STREAMINFO.
	binary.BigEndian.PutUint16(stream[8:10], uint16(block))
	binary.BigEndian.PutUint16(stream[10:12], uint16(block))
	flacPut24(stream[12:15], minFrame)
	flacPut24(stream[15:18], maxFrame)
	packed := uint64(pcm.SampleRate())<<44 | uint64(channels-1)<<41 | uint64(depth-1)<<36 | frames
	binary.BigEndian.PutUint64(stream[18:26], packed)
	copy(stream[26:], digest.Sum(nil))
	if err := flacWrite(w, stream[:]); err != nil {
		return err
	}
	// Verbatim always bounds a frame, including the two subframe headers.
	buffer := make([]byte, 0, len(quantized)*(depth/8)+32)
	for start, number := 0, uint32(0); start < pcm.Frames(); number++ {
		n := min(block, pcm.Frames()-start)
		x := quantized[:n*channels]
		flacQuantize(x, pcm.samples[start*channels:(start+n)*channels], depth)
		plans := flacPlans(x, channels, depth)
		bw := flacBits{data: buffer[:0]}
		bw.data = append(bw.data, flacHeader(number, n, channels, depth)...)
		for c := 0; c < channels; c++ {
			flacSubframe(&bw, x[c:], channels, n, depth, plans[c])
		}
		bw.align()
		crc := flacCRC16(bw.data)
		bw.data = append(bw.data, byte(crc>>8), byte(crc))
		if err := flacWrite(w, bw.data); err != nil {
			return err
		}
		buffer = bw.data
		start += n
	}
	return nil
}

func flacWrite(w io.Writer, b []byte) error {
	n, err := w.Write(b)
	if err != nil {
		return fmt.Errorf("audio: write FLAC: %w", err)
	}
	if n != len(b) {
		return fmt.Errorf("audio: write FLAC: %w", io.ErrShortWrite)
	}
	return nil
}

func flacQuantize(dst []int64, src []float64, depth int) {
	scale := int64(1) << (depth - 1)
	for i, v := range src {
		dst[i] = min(int64(math.Round(v*float64(scale))), scale-1)
	}
}

func flacPut24(dst []byte, v uint64) {
	dst[0], dst[1], dst[2] = byte(v>>16), byte(v>>8), byte(v)
}

// Fixed blocking, explicit 16-bit block length, rate inherited from STREAMINFO.
func flacHeader(number uint32, n, channels, depth int) []byte {
	depthCode := byte(4)
	if depth == 24 {
		depthCode = 6
	}
	h := make([]byte, 4, 13)
	h[0], h[1], h[2], h[3] = 0xff, 0xf8, 0x70, byte(channels-1)<<4|depthCode<<1
	h = flacAppendNumber(h, number)
	h = append(h, byte((n-1)>>8), byte(n-1))
	return append(h, flacCRC8(h))
}

func flacAppendNumber(dst []byte, n uint32) []byte {
	if n < 128 {
		return append(dst, byte(n))
	}
	length := 2
	for n >= 1<<uint(5*length+1) {
		length++
	}
	dst = append(dst, byte(0xff<<uint(8-length))|byte(n>>uint(6*(length-1))))
	for shift := 6 * (length - 2); shift >= 0; shift -= 6 {
		dst = append(dst, 0x80|byte(n>>uint(shift))&0x3f)
	}
	return dst
}

type flacPlan struct {
	kind   int // 0 constant; 1 verbatim; 8..12 fixed predictor order 0..4.
	rice   int // 0..14 Rice parameter, 15 escape.
	width  int // Signed residual bit width for escape; zero is allowed.
	bitLen uint64
}

func flacPlans(x []int64, channels, depth int) [2]flacPlan {
	var plans [2]flacPlan
	for c := 0; c < channels; c++ {
		plans[c] = flacChoose(x[c:], channels, len(x)/channels, depth)
	}
	return plans
}

func flacChoose(x []int64, stride, n, depth int) flacPlan {
	best := flacPlan{kind: 1, bitLen: uint64(8 + n*depth)}
	constant := true
	for i := 1; i < n; i++ {
		if x[i*stride] != x[0] {
			constant = false
			break
		}
	}
	if constant {
		return flacPlan{kind: 0, bitLen: uint64(8 + depth)}
	}
	for order := 0; order <= 4 && order < n; order++ {
		var costs [15]uint64
		width := 0
		for i := order; i < n; i++ {
			r := x[i*stride] - flacPredict(x, stride, i, order)
			u := flacFold(r)
			// For 24-bit samples and fixed order <=4, |r| < 2^28.
			// Folding gives exactly the signed two's-complement bit width,
			// with zero requiring no bits in an escaped partition.
			if u != 0 {
				width = max(width, bits.Len64(u))
			}
			for k := range costs {
				costs[k] += (u >> k) + 1 + uint64(k)
			}
		}
		overhead := uint64(8 + order*depth + 2 + 4 + 4)
		for k, cost := range costs {
			if overhead+cost < best.bitLen {
				best = flacPlan{kind: 8 + order, rice: k, bitLen: overhead + cost}
			}
		}
		escapeCost := overhead + 5 + uint64((n-order)*width)
		if width <= 31 && escapeCost < best.bitLen {
			best = flacPlan{kind: 8 + order, rice: 15, width: width, bitLen: escapeCost}
		}
	}
	return best
}

func flacPredict(x []int64, s, i, order int) int64 {
	switch order {
	case 1:
		return x[(i-1)*s]
	case 2:
		return 2*x[(i-1)*s] - x[(i-2)*s]
	case 3:
		return 3*x[(i-1)*s] - 3*x[(i-2)*s] + x[(i-3)*s]
	case 4:
		return 4*x[(i-1)*s] - 6*x[(i-2)*s] + 4*x[(i-3)*s] - x[(i-4)*s]
	}
	return 0
}

func flacFold(r int64) uint64 { return uint64((r << 1) ^ (r >> 63)) }

func flacSubframe(b *flacBits, x []int64, stride, n, depth int, p flacPlan) {
	b.put(uint64(p.kind<<1), 8) // zero padding, subframe type, no wasted bits.
	switch p.kind {
	case 0:
		b.put(uint64(x[0]), depth)
	case 1:
		for i := 0; i < n; i++ {
			b.put(uint64(x[i*stride]), depth)
		}
	default:
		order := p.kind - 8
		for i := 0; i < order; i++ {
			b.put(uint64(x[i*stride]), depth)
		}
		b.put(0, 6) // Rice method 0, partition order 0 (one adaptive partition).
		b.put(uint64(p.rice), 4)
		if p.rice == 15 {
			b.put(uint64(p.width), 5)
		}
		for i := order; i < n; i++ {
			r := x[i*stride] - flacPredict(x, stride, i, order)
			if p.rice == 15 {
				b.put(uint64(r), p.width)
			} else {
				u := flacFold(r)
				b.zeros(u >> p.rice)
				b.put(1, 1)
				b.put(u, p.rice)
			}
		}
	}
}

func flacFrameSize(header int, plans []flacPlan) uint64 {
	n := uint64(0)
	for _, p := range plans {
		n += p.bitLen
	}
	return uint64(header) + (n+7)/8 + 2
}

type flacBits struct {
	data []byte
	used uint8 // Used bits in the last byte; zero at byte boundaries.
}

func (b *flacBits) put(v uint64, n int) {
	for n > 0 {
		if b.used == 0 {
			b.data = append(b.data, 0)
		}
		take := min(n, 8-int(b.used))
		b.data[len(b.data)-1] |= byte((v>>uint(n-take))&((1<<take)-1)) << (8 - b.used - uint8(take))
		b.used = (b.used + uint8(take)) & 7
		n -= take
	}
}

func (b *flacBits) zeros(n uint64) {
	if b.used != 0 && n > 0 {
		take := min(n, uint64(8-b.used))
		b.put(0, int(take))
		n -= take
	}
	b.data = append(b.data, make([]byte, int(n/8))...)
	b.put(0, int(n%8))
}

func (b *flacBits) align() { b.used = 0 }

func flacCRC8(b []byte) byte {
	var crc byte
	for _, v := range b {
		crc ^= v
		for range 8 {
			if crc&0x80 != 0 {
				crc = crc<<1 ^ 0x07
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}

func flacCRC16(b []byte) uint16 {
	var crc uint16
	for _, v := range b {
		crc ^= uint16(v) << 8
		for range 8 {
			if crc&0x8000 != 0 {
				crc = crc<<1 ^ 0x8005
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}
