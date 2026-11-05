package audio

import (
	"bufio"
	"bytes"
	"crypto/md5"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"mime"
	"strings"
	"unicode/utf8"
)

// FLACReadOptions configures strict native FLAC decoding to independent PCM.
// Zero Limits fields select DefaultLimits. MaxChunks counts metadata blocks.
// MaxWork defaults to 2^32 units: one per encoded bit, reconstructed sample,
// and predictor multiply/add pair. Exhaustion always returns ErrLimit.
// DiscardMetadata explicitly permits losing application, seek-table, Vorbis
// comment and picture blocks. Padding is always permitted. Cue sheets, reserved
// metadata and channel-mask overrides remain unsupported, even with opt-in.
type FLACReadOptions struct {
	Limits          Limits
	MaxWork         uint64
	DiscardMetadata bool
}

// ReadFLAC decodes a complete, finite native FLAC stream through EOF. Trailing
// bytes, concatenated streams, unknown/reserved content, changing audio format,
// absent MD5, and unsupported depths/channel layouts are errors. Byte-aligned
// PCM8/16/24/32 with 1..8 indexed channels is accepted. Constant, verbatim,
// fixed 0..4, LPC 1..32,
// partitioned Rice/Rice2, escape, wasted bits and all stereo assignments work.
// CRC8/16, frame numbering, STREAMINFO bounds, total (when known) and PCM MD5
// are verified before returning any PCM. A zero total is counted through EOF;
// a zero MD5 is rejected because there is no checksum to verify.
// Decoded samples are exact integers divided by 2^(depth-1), with a conventional
// indexed Format (ChannelMask zero). Channel order is preserved; stereo
// decorrelation is used only for two channels. No external decoder runs.
// r must reach EOF: native FLAC has no stream byte length. MaxBytes allows an
// additional one-byte probe solely to distinguish exact-length EOF from excess.
func ReadFLAC(r io.Reader, options FLACReadOptions) (*PCM, error) {
	if r == nil {
		return nil, fmt.Errorf("%w: nil FLAC reader", ErrInvalidInput)
	}
	l, err := options.Limits.normalized()
	if err != nil {
		return nil, err
	}
	work := options.MaxWork
	if work == 0 {
		work = 1 << 32
	}
	d := flacDecoder{limits: l, work: work}
	d.r = bufio.NewReaderSize(&flacBoundReader{r: r, left: l.MaxBytes}, 4096)
	header := d.metadataBytes(4)
	if d.err != nil {
		return nil, d.err
	}
	if string(header) != "fLaC" {
		return nil, fmt.Errorf("%w: expected native FLAC", ErrUnsupported)
	}
	var seenMetadata byte
	for index := 0; ; index++ {
		if index >= l.MaxChunks {
			return nil, fmt.Errorf("%w: FLAC metadata count", ErrLimit)
		}
		h := d.metadataBytes(4)
		if d.err != nil {
			return nil, d.err
		}
		kind, n := h[0]&127, int(h[1])<<16|int(h[2])<<8|int(h[3])
		if index == 0 && kind != 0 {
			return nil, flacBad("STREAMINFO must be first")
		}
		if kind == 127 || (kind == 0 && (index != 0 || n != 34)) {
			return nil, flacBad("invalid/duplicate STREAMINFO or metadata type")
		}
		if kind >= 7 {
			return nil, fmt.Errorf("%w: reserved FLAC metadata %d", ErrUnsupported, kind)
		}
		if (kind == 3 || kind == 4) && seenMetadata&(1<<kind) != 0 {
			return nil, flacBad("duplicate SEEKTABLE/VORBIS_COMMENT")
		}
		seenMetadata |= 1 << kind
		if kind >= 2 && !options.DiscardMetadata {
			return nil, fmt.Errorf("%w: FLAC metadata %d requires DiscardMetadata", ErrUnsupported, kind)
		}
		b := d.metadataBytes(n)
		if d.err != nil {
			return nil, d.err
		}
		if kind == 0 {
			err = d.streamInfo(b)
		} else {
			err = d.metadata(kind, b)
		}
		if err != nil {
			return nil, err
		}
		if h[0]&128 != 0 {
			break
		}
	}
	if bytes.Equal(d.md5[:], make([]byte, 16)) {
		return nil, fmt.Errorf("%w: FLAC has no PCM MD5", ErrUnsupported)
	}
	digest := md5.New()
	if _, err := digest.Write(nil); err != nil {
		return nil, fmt.Errorf("%w: FLAC PCM MD5 unavailable: %w", ErrUnsupported, err)
	}
	var out []float64
	var decoded, frameNumber uint64
	var previousBlock, strategy = 0, -1
	var minFrame, maxFrame int64
	var channel [8][]int64
	var raw []byte
	for {
		if _, err := d.r.Peek(1); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("audio: FLAC frame boundary: %w", err)
		}
		if previousBlock != 0 && (previousBlock < d.minBlock || (strategy == 0 && previousBlock != d.maxBlock)) {
			return nil, flacBad("short block before final frame")
		}
		start := d.used
		n, assignment, blocking, number := d.frameHeader()
		if d.err != nil {
			return nil, d.err
		}
		if strategy != -1 && strategy != blocking {
			return nil, flacBad("blocking strategy changed")
		}
		strategy = blocking
		wantNumber := frameNumber
		if strategy == 1 {
			wantNumber = decoded
		}
		if number != wantNumber || (strategy == 0 && d.minBlock != d.maxBlock) {
			return nil, flacBad("frame/sample number or fixed block bounds mismatch")
		}
		next := decoded + uint64(n)
		if next >= 1<<36 {
			return nil, fmt.Errorf("%w: FLAC total samples field", ErrLimit)
		}
		if err := l.checkFrames(next, d.channels); err != nil {
			return nil, err
		}
		if d.total != 0 && next > d.total {
			return nil, flacBad("more samples than STREAMINFO total")
		}
		d.spend(uint64(n * d.channels))
		if d.err != nil {
			return nil, d.err
		}
		for c := 0; c < d.channels; c++ {
			if cap(channel[c]) < n {
				channel[c] = make([]int64, n)
			}
			channel[c] = channel[c][:n]
			depth := d.depth
			if (assignment == 8 || assignment == 10) && c == 1 || assignment == 9 && c == 0 {
				depth++
			}
			d.subframe(channel[c], depth)
			if d.err != nil {
				return nil, d.err
			}
		}
		if d.avail != 0 && d.get(int(d.avail)) != 0 {
			return nil, flacBad("nonzero frame alignment padding")
		}
		crc := d.crc16
		d.frameCRC = false
		if footer := d.get(16); d.err == nil && footer != uint64(crc) {
			return nil, flacBad("frame CRC16 mismatch")
		}
		if d.err != nil {
			return nil, d.err
		}
		frameSize := d.used - start
		if d.minFrame != 0 && frameSize < d.minFrame || d.maxFrame != 0 && frameSize > d.maxFrame {
			return nil, flacBad("encoded frame size outside STREAMINFO bounds")
		}
		if frameNumber == 0 || frameSize < minFrame {
			minFrame = frameSize
		}
		maxFrame = max(maxFrame, frameSize)
		needed := int(next) * d.channels
		if cap(out) < needed {
			// Divide before multiplying: user-supplied MaxFrames*channels may
			// overflow uint64 with more than two channels.
			maxFrames := min(uint64(l.MaxFrames), uint64(l.MaxSamples)/uint64(d.channels), uint64(int(^uint(0)>>1)/8)/uint64(d.channels))
			maxSamples := maxFrames * uint64(d.channels)
			// l.checkFrames has already proved needed and its byte size fit int.
			capacity := int(min(maxSamples, uint64(max(needed, cap(out)+cap(out)/2))))
			grown := make([]float64, len(out), capacity)
			copy(grown, out)
			out = grown
		}
		out = out[:needed]
		bytesNeeded := n * d.channels * (d.depth / 8)
		if cap(raw) < bytesNeeded {
			raw = make([]byte, bytesNeeded)
		}
		raw = raw[:bytesNeeded]
		scale := float64(int64(1) << (d.depth - 1))
		for f := 0; f < n; f++ {
			switch assignment {
			case 8:
				channel[1][f] = channel[0][f] - channel[1][f]
			case 9:
				channel[0][f] += channel[1][f]
			case 10:
				a, b := channel[0][f], channel[1][f]
				mid := a<<1 | b&1
				channel[0][f], channel[1][f] = (mid+b)>>1, (mid-b)>>1
			}
			for c := 0; c < d.channels; c++ {
				v := channel[c][f]
				if !flacSignedFits(v, d.depth) {
					return nil, flacBad("restored sample overflow")
				}
				out[(int(decoded)+f)*d.channels+c] = float64(v) / scale
				for j := 0; j < d.depth/8; j++ {
					raw[(f*d.channels+c)*(d.depth/8)+j] = byte(v >> uint(j*8))
				}
			}
		}
		if _, err := digest.Write(raw); err != nil {
			return nil, fmt.Errorf("%w: FLAC PCM MD5 unavailable: %w", ErrUnsupported, err)
		}
		decoded, previousBlock = next, n
		frameNumber++
	}
	if frameNumber == 0 || d.total != 0 && decoded != d.total {
		return nil, flacBad("empty/truncated stream or total samples mismatch")
	}
	if d.minFrame != 0 && minFrame != d.minFrame || d.maxFrame != 0 && maxFrame != d.maxFrame {
		return nil, flacBad("STREAMINFO frame size extrema mismatch")
	}
	if !bytes.Equal(digest.Sum(nil), d.md5[:]) {
		return nil, flacBad("PCM MD5 mismatch")
	}
	return &PCM{format: Format{SampleRate: d.rate, Channels: d.channels}, samples: out}, nil
}

// The bounded source never prefetches beyond the budget. Its one-byte probe
// distinguishes a file exactly at the limit from a longer file.
type flacBoundReader struct {
	r    io.Reader
	left int64
}

func (r *flacBoundReader) Read(b []byte) (int, error) {
	if len(b) == 0 {
		return 0, nil
	}
	if r.left == 0 {
		var probe [1]byte
		n, err := r.r.Read(probe[:])
		if n > 0 {
			return 0, fmt.Errorf("%w: FLAC encoded bytes", ErrLimit)
		}
		return 0, err
	}
	n, err := r.r.Read(b[:min(int64(len(b)), r.left)])
	r.left -= int64(n)
	return n, err
}

type flacDecoder struct {
	r                     *bufio.Reader
	limits                Limits
	work                  uint64
	used                  int64
	err                   error
	rate, channels, depth int
	minBlock, maxBlock    int
	minFrame, maxFrame    int64
	total                 uint64
	md5                   [16]byte
	byte                  byte
	avail                 uint8
	crc8                  byte
	crc16                 uint16
	headerCRC, frameCRC   bool
}

func flacBad(message string) error { return fmt.Errorf("%w: FLAC %s", ErrInvalidInput, message) }

func (d *flacDecoder) spend(n uint64) {
	if d.err != nil {
		return
	}
	if n > d.work {
		d.err = fmt.Errorf("%w: FLAC decode work", ErrLimit)
	} else {
		d.work -= n
	}
}

func (d *flacDecoder) metadataBytes(n int) []byte {
	if d.err != nil {
		return nil
	}
	if int64(n) > d.limits.MaxBytes-d.used {
		d.err = fmt.Errorf("%w: FLAC metadata bytes", ErrLimit)
		return nil
	}
	d.spend(uint64(n) * 8)
	if d.err != nil {
		return nil
	}
	b := make([]byte, n)
	got, err := io.ReadFull(d.r, b)
	d.used += int64(got)
	if err != nil {
		d.err = fmt.Errorf("%w: FLAC metadata: %w", ErrInvalidInput, err)
	}
	return b
}

// flacReadCRC16 contains the remainder for each possible leading byte under
// FLAC's unreflected x^16+x^15+x^2+1 polynomial. Initialized once, then only
// read by decoders: each byte update is equivalent to eight long-division steps.
var flacReadCRC16 = func() [256]uint16 {
	var table [256]uint16
	for i := range table {
		crc := uint16(i) << 8
		for range 8 {
			if crc&0x8000 != 0 {
				crc = crc<<1 ^ 0x8005
			} else {
				crc <<= 1
			}
		}
		table[i] = crc
	}
	return table
}()

func (d *flacDecoder) get(n int) uint64 {
	if d.err != nil {
		return 0
	}
	d.spend(uint64(n))
	var v uint64
	for n > 0 && d.err == nil {
		if d.avail == 0 {
			b, err := d.r.ReadByte()
			if err != nil {
				if errors.Is(err, io.EOF) {
					err = io.ErrUnexpectedEOF
				}
				d.err = fmt.Errorf("%w: FLAC frame: %w", ErrInvalidInput, err)
				return 0
			}
			d.used++
			d.byte, d.avail = b, 8
			if d.headerCRC {
				d.crc8 ^= b
				for range 8 {
					if d.crc8&128 != 0 {
						d.crc8 = d.crc8<<1 ^ 7
					} else {
						d.crc8 <<= 1
					}
				}
			}
			if d.frameCRC {
				d.crc16 = d.crc16<<8 ^ flacReadCRC16[byte(d.crc16>>8)^b]
			}
		}
		take := min(n, int(d.avail))
		v = v<<uint(take) | uint64(d.byte>>uint(d.avail-uint8(take)))&((1<<uint(take))-1)
		d.avail -= uint8(take)
		n -= take
	}
	return v
}

func (d *flacDecoder) signed(n int) int64 {
	v := d.get(n)
	if n == 0 {
		return 0
	}
	return int64(v<<uint(64-n)) >> uint(64-n)
}

func (d *flacDecoder) unary(maximum uint64) uint64 {
	var n uint64
	for d.err == nil && d.get(1) == 0 {
		n++
		if n > maximum {
			d.err = flacBad("unary code exceeds numeric range")
		}
	}
	return n
}

func (d *flacDecoder) streamInfo(b []byte) error {
	d.minBlock, d.maxBlock = int(binary.BigEndian.Uint16(b[:2])), int(binary.BigEndian.Uint16(b[2:4]))
	d.minFrame = int64(b[4])<<16 | int64(b[5])<<8 | int64(b[6])
	d.maxFrame = int64(b[7])<<16 | int64(b[8])<<8 | int64(b[9])
	packed := binary.BigEndian.Uint64(b[10:18])
	d.rate, d.channels, d.depth = int(packed>>44), int(packed>>41&7)+1, int(packed>>36&31)+1
	d.total = packed & ((1 << 36) - 1)
	copy(d.md5[:], b[18:])
	if d.minBlock < 16 || d.maxBlock < d.minBlock || d.minFrame != 0 && d.maxFrame != 0 && d.minFrame > d.maxFrame || d.rate == 0 || d.depth < 4 {
		return flacBad("invalid STREAMINFO")
	}
	if d.rate > MaxSampleRate || (d.depth != 8 && d.depth != 16 && d.depth != 24 && d.depth != 32) {
		return fmt.Errorf("%w: FLAC requires byte-aligned PCM8/16/24/32 within PCM sample rate range", ErrUnsupported)
	}
	return d.limits.checkFrames(d.total, d.channels)
}

func (d *flacDecoder) codedNumber() uint64 {
	first := byte(d.get(8))
	if first < 128 {
		return uint64(first)
	}
	count, mask := 0, byte(128)
	for first&mask != 0 && mask != 0 {
		count++
		mask >>= 1
	}
	if count < 2 || count > 7 {
		d.err = flacBad("invalid coded number prefix")
		return 0
	}
	n := uint64(first & (mask - 1))
	for i := 1; i < count; i++ {
		b := d.get(8)
		if d.err == nil && b&192 != 128 {
			d.err = flacBad("invalid coded number continuation")
		}
		n = n<<6 | b&63
	}
	minimum := uint64(1) << uint(5*count-4)
	if count == 2 {
		minimum = 128
	}
	if d.err == nil && n < minimum {
		d.err = flacBad("overlong coded number")
	}
	return n
}

func (d *flacDecoder) frameHeader() (n, assignment, strategy int, number uint64) {
	d.crc8, d.crc16, d.headerCRC, d.frameCRC = 0, 0, true, true
	sync := d.get(16)
	if d.err == nil && sync&0xfffe != 0xfff8 {
		d.err = flacBad("frame sync/reserved bit or trailing data")
		return
	}
	strategy = int(sync & 1)
	blockCode, rateCode := int(d.get(4)), int(d.get(4))
	assignment, depthCode := int(d.get(4)), int(d.get(3))
	reserved := d.get(1)
	if d.err == nil && (blockCode == 0 || rateCode == 15 || assignment >= 11 || depthCode == 3 || reserved != 0) {
		d.err = flacBad("reserved frame header code")
		return
	}
	number = d.codedNumber()
	if d.err == nil && strategy == 0 && number >= 1<<31 {
		d.err = flacBad("fixed frame number exceeds 31 bits")
	}
	switch blockCode {
	case 1:
		n = 192
	case 2, 3, 4, 5:
		n = 144 << blockCode
	case 6:
		n = int(d.get(8)) + 1
	case 7:
		n = int(d.get(16)) + 1
	default:
		n = 1 << blockCode
	}
	rates := [...]int{d.rate, 88200, 176400, 192000, 8000, 16000, 22050, 24000, 32000, 44100, 48000, 96000}
	rate := 0
	if rateCode < len(rates) {
		rate = rates[rateCode]
	} else {
		switch rateCode {
		case 12:
			rate = int(d.get(8)) * 1000
		case 13:
			rate = int(d.get(16))
		case 14:
			rate = int(d.get(16)) * 10
		}
	}
	depths := [...]int{d.depth, 8, 12, 0, 16, 20, 24, 32}
	channels := assignment + 1
	if assignment >= 8 {
		channels = 2
	}
	if d.err == nil && (n > d.maxBlock || n == 65536 || rate == 0) {
		d.err = flacBad("frame block/rate outside STREAMINFO")
	}
	if d.err == nil && (channels != d.channels || rate != d.rate || depths[depthCode] != d.depth) {
		d.err = fmt.Errorf("%w: FLAC audio properties change within stream", ErrUnsupported)
	}
	crc := d.crc8
	d.headerCRC = false
	if got := d.get(8); d.err == nil && got != uint64(crc) {
		d.err = flacBad("header CRC8 mismatch")
	}
	return
}

func flacSignedFits(v int64, depth int) bool {
	return v >= -(int64(1)<<(depth-1)) && v < int64(1)<<(depth-1)
}

func (d *flacDecoder) subframe(x []int64, depth int) {
	pad, kind, wasted := d.get(1), int(d.get(6)), int(d.get(1))
	if pad != 0 && d.err == nil {
		d.err = flacBad("nonzero subframe padding bit")
	}
	if wasted != 0 {
		wasted = int(d.unary(uint64(depth-2))) + 1
	}
	if d.err != nil {
		return
	}
	effective := depth - wasted
	if effective < 1 {
		d.err = flacBad("invalid wasted bit depth")
		return
	}
	switch {
	case kind == 0:
		v := d.signed(effective)
		for i := range x {
			x[i] = v
		}
	case kind == 1:
		for i := range x {
			x[i] = d.signed(effective)
		}
	case kind >= 8 && kind <= 12 || kind >= 32:
		order := kind - 8
		if kind >= 32 {
			order = kind - 31
		}
		if order >= len(x) {
			d.err = flacBad("predictor order must be below block size")
			return
		}
		for i := 0; i < order; i++ {
			x[i] = d.signed(effective)
		}
		var coefficients [32]int64
		shift := int64(0)
		if kind >= 32 {
			precision := int(d.get(4)) + 1
			shift = d.signed(5)
			if d.err == nil && (precision == 16 || shift < 0) {
				d.err = flacBad("LPC reserved precision or negative shift")
				return
			}
			for i := 0; i < order; i++ {
				coefficients[i] = d.signed(precision)
			}
		} else {
			fixed := [5][4]int64{{}, {1}, {2, -1}, {3, -3, 1}, {4, -6, 4, -1}}
			copy(coefficients[:], fixed[order][:])
		}
		d.spend(uint64((len(x) - order) * order))
		d.residual(x, order)
		for i := order; i < len(x) && d.err == nil; i++ {
			// Previous samples are checked at every step. At <=33 bits
			// (including a 32-bit stereo side), 32 coefficients of <=15
			// signed bits sum in magnitude to <=2^51. Adding a signed
			// residual of magnitude <2^31 is safe in int64 before checking.
			var prediction int64
			for j := 0; j < order; j++ {
				prediction += coefficients[j] * x[i-j-1]
			}
			x[i] += prediction >> uint(shift)
			if !flacSignedFits(x[i], effective) {
				d.err = flacBad("predictor sample overflow")
			}
		}
	default:
		d.err = fmt.Errorf("%w: reserved FLAC subframe type %d", ErrUnsupported, kind)
	}
	if d.err == nil && wasted != 0 {
		for i := range x {
			x[i] <<= uint(wasted)
		}
	}
}

func (d *flacDecoder) residual(x []int64, order int) {
	method, partitionOrder := int(d.get(2)), int(d.get(4))
	if d.err != nil {
		return
	}
	if method > 1 {
		d.err = fmt.Errorf("%w: reserved FLAC residual method", ErrUnsupported)
		return
	}
	partitions := 1 << partitionOrder
	if len(x)%partitions != 0 || len(x)/partitions <= order {
		d.err = flacBad("invalid Rice partition order")
		return
	}
	parameterBits := 4 + method
	for partition, index := 0, order; partition < partitions && d.err == nil; partition++ {
		parameter := int(d.get(parameterBits))
		end := (partition + 1) * (len(x) / partitions)
		if parameter == (1<<parameterBits)-1 {
			width := int(d.get(5))
			for ; index < end && d.err == nil; index++ {
				x[index] = d.signed(width)
			}
		} else {
			for ; index < end && d.err == nil; index++ {
				quotient := d.unary(uint64(0xfffffffe) >> uint(parameter))
				u := quotient<<uint(parameter) | d.get(parameter)
				if d.err == nil && u > 0xfffffffe {
					d.err = flacBad("residual exceeds signed 32-bit magnitude")
				}
				x[index] = int64(u>>1) ^ -int64(u&1)
			}
		}
	}
}

// Defined metadata is structurally checked even when its loss is authorized.
func (d *flacDecoder) metadata(kind byte, b []byte) error {
	switch kind {
	case 1:
		for _, v := range b {
			if v != 0 {
				return flacBad("nonzero PADDING payload")
			}
		}
	case 2:
		if len(b) < 4 {
			return flacBad("short APPLICATION ID")
		}
	case 3:
		if len(b)%18 != 0 {
			return flacBad("SEEKTABLE length")
		}
		var previous uint64
		placeholder := false
		for i := 0; i < len(b); i += 18 {
			n := binary.BigEndian.Uint64(b[i : i+8])
			if n == math.MaxUint64 {
				placeholder = true
				continue
			}
			if placeholder || i != 0 && n <= previous || binary.BigEndian.Uint16(b[i+16:i+18]) == 0 || d.total != 0 && n >= d.total {
				return flacBad("invalid seek point ordering/extent")
			}
			previous = n
		}
	case 4:
		return flacComments(b)
	case 5:
		// Cue sheets have nested timing/index semantics not implemented here.
		return fmt.Errorf("%w: FLAC CUESHEET metadata", ErrUnsupported)
	case 6:
		return flacPicture(b)
	}
	return nil
}

func flacComments(b []byte) error {
	pos := 0
	read32 := func() (uint32, bool) {
		if len(b)-pos < 4 {
			return 0, false
		}
		v := binary.LittleEndian.Uint32(b[pos : pos+4])
		pos += 4
		return v, true
	}
	readString := func() (string, bool) {
		n, ok := read32()
		if !ok || uint64(n) > uint64(len(b)-pos) {
			return "", false
		}
		s := b[pos : pos+int(n)]
		pos += int(n)
		return string(s), utf8.Valid(s)
	}
	if _, ok := readString(); !ok {
		return flacBad("invalid UTF-8 vendor comment")
	}
	count, ok := read32()
	if !ok || uint64(count) > uint64((len(b)-pos)/4) {
		return flacBad("invalid Vorbis comment count")
	}
	for range count {
		s, ok := readString()
		equal := strings.IndexByte(s, '=')
		if !ok || equal <= 0 {
			return flacBad("invalid Vorbis comment")
		}
		for _, c := range []byte(s[:equal]) {
			if c < 0x20 || c > 0x7d || c == '=' {
				return flacBad("invalid Vorbis field name")
			}
		}
		if strings.EqualFold(s[:equal], "WAVEFORMATEXTENSIBLE_CHANNEL_MASK") {
			return fmt.Errorf("%w: FLAC channel-mask metadata override", ErrUnsupported)
		}
	}
	if pos != len(b) {
		return flacBad("Vorbis comment trailing bytes")
	}
	return nil
}

func flacPicture(b []byte) error {
	if len(b) < 4 || binary.BigEndian.Uint32(b[:4]) > 20 {
		return flacBad("PICTURE type")
	}
	pos := 4
	for index := 0; index < 2; index++ {
		if len(b)-pos < 4 {
			return flacBad("PICTURE string length")
		}
		n := uint64(binary.BigEndian.Uint32(b[pos : pos+4]))
		pos += 4
		if n > uint64(len(b)-pos) {
			return flacBad("PICTURE string extent")
		}
		s := b[pos : pos+int(n)]
		if index == 0 {
			for _, c := range s {
				if c < 0x20 || c > 0x7e {
					return flacBad("PICTURE media type must be printable ASCII")
				}
			}
			if string(s) != "-->" {
				mediaType, _, err := mime.ParseMediaType(string(s))
				if err != nil || !strings.Contains(mediaType, "/") {
					return flacBad("PICTURE media type syntax")
				}
			}
		} else if !utf8.Valid(s) {
			return flacBad("PICTURE description UTF-8")
		}
		pos += int(n)
	}
	if len(b)-pos < 20 {
		return flacBad("PICTURE dimensions/data extent")
	}
	n := uint64(binary.BigEndian.Uint32(b[pos+16 : pos+20]))
	if n != uint64(len(b)-pos-20) {
		return flacBad("PICTURE data length")
	}
	return nil
}
