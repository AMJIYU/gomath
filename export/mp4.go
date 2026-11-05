package export

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"gomath/internal/h264"
	"image"
	"io"
	"math"
)

// MP4Codec selects a pure-Go full-range YUV420 video encoder. The predictive
// subset uses bounded single-reference IDR/P pictures; other codecs use all IDR.
type MP4Codec uint8

const (
	H264Intra      MP4Codec = iota // Integer transform, quantization and CAVLC.
	H264PCM                        // Uncompressed samples in I_PCM macroblocks.
	H264Predictive                 // IDR/P: exact reconstructed-YUV zero-motion skip, otherwise I4x4.
)

const MaxMotionSearchRadius = h264.MaxMotionSearchRadius

type MP4Options struct {
	Codec MP4Codec
	// QP is the spatial quantization parameter, 0..51, for compressed codecs.
	// Zero is valid and still lossy. Use DefaultMP4Options for QP 24.
	QP int
	// ModeSearch compares legal DC/vertical/horizontal luma predictions. It can
	// improve compression at additional CPU cost; valid for compressed codecs.
	ModeSearch bool
	// InterResidual enables zero-motion P_L0_16x16 transform/quantized residuals.
	// It applies only to H264Predictive; false preserves the original skip/intra
	// encoding. There is no B picture or bitrate/quality guarantee.
	InterResidual bool
	// MotionSearchRadius enables exhaustive integer-luma P_L0_16x16 search,
	// 1..MaxMotionSearchRadius, requiring H264Predictive and InterResidual.
	// Zero retains existing bytes. Full RDO evaluates up to (2R+1)^2 inter
	// candidates plus intra per P macroblock, at substantial additional CPU cost.
	MotionSearchRadius int
	// GOP is used only by H264Predictive, 1..65536; zero selects 60. No B frames
	// are used. Repeated lossy inputs may not exactly skip.
	GOP int
	// MaxFrames bounds per-frame sample metadata. Zero selects 1,000,000.
	MaxFrames int
}

func DefaultMP4Options() MP4Options { return MP4Options{Codec: H264Intra, QP: 24, MaxFrames: 1000000} }

type videoEncoder interface {
	SPS() []byte
	PPS() []byte
	Encode(*image.RGBA) ([]byte, error)
}

// MP4 writes streaming video samples and publishes metadata/output on Close.
// Frame pixels are bounded; the sample-size table grows to MaxFrames entries.
type MP4 struct {
	out                *atomicFile
	encoder            videoEncoder
	width, height, fps int
	maxFrames          int
	predictive         bool
	keyframes          []uint32
	audio              *movAudio
	mdat               int64
	offset             uint64
	sizes              []uint32
}

// NewMP4 preserves the original I_PCM encoding. For smaller files use
// NewMP4WithOptions with DefaultMP4Options or explicit H264Intra settings.
func NewMP4(target string, width, height, fps int) (*MP4, error) {
	return NewMP4WithOptions(target, width, height, fps, MP4Options{Codec: H264PCM})
}

func NewMP4WithOptions(target string, width, height, fps int, opt MP4Options) (*MP4, error) {
	return newVideoFile(target, width, height, fps, opt, false)
}

func newVideoFile(target string, width, height, fps int, opt MP4Options, quickTime bool) (*MP4, error) {
	if opt.MaxFrames == 0 {
		opt.MaxFrames = 1000000
	}
	if opt.MaxFrames < 1 || opt.MaxFrames > 1<<26 {
		return nil, fmt.Errorf("MP4 frame limit must be in 1..67108864")
	}
	var encoder videoEncoder
	var e error
	if opt.Codec != H264Predictive && opt.GOP != 0 {
		return nil, fmt.Errorf("MP4 GOP applies only to H264Predictive")
	}
	if opt.Codec != H264Predictive && opt.InterResidual {
		return nil, fmt.Errorf("MP4 inter residuals apply only to H264Predictive")
	}
	if opt.MotionSearchRadius < 0 || opt.MotionSearchRadius > MaxMotionSearchRadius || opt.MotionSearchRadius != 0 && (opt.Codec != H264Predictive || !opt.InterResidual) {
		return nil, fmt.Errorf("MP4 motion search radius must be 0..%d; nonzero requires predictive inter residuals", MaxMotionSearchRadius)
	}
	switch opt.Codec {
	case H264Intra:
		encoder, e = h264.NewIntra(width, height, fps, h264.IntraOptions{QP: opt.QP, ModeSearch: opt.ModeSearch})
	case H264PCM:
		if opt.QP != 0 || opt.ModeSearch {
			return nil, fmt.Errorf("MP4 QP and mode search apply only to compressed codecs")
		}
		encoder, e = h264.New(width, height, fps)
	case H264Predictive:
		gop := opt.GOP
		if gop == 0 {
			gop = 60
		}
		var predictive *h264.PredictiveEncoder
		predictive, e = h264.NewPredictive(width, height, fps, h264.PredictiveOptions{QP: opt.QP, ModeSearch: opt.ModeSearch, GOP: gop, InterResidual: opt.InterResidual, MotionSearchRadius: opt.MotionSearchRadius})
		if e == nil {
			encoder = &predictiveVideoEncoder{PredictiveEncoder: predictive}
		}
	default:
		return nil, fmt.Errorf("unsupported MP4 codec %d", opt.Codec)
	}
	if e != nil {
		return nil, e
	}
	out, e := newAtomicFile(target)
	if e != nil {
		return nil, e
	}
	ftyp := box("ftyp", []byte("isom"), be32(0x200), []byte("isomiso2avc1mp41"))
	if quickTime {
		ftyp = box("ftyp", []byte("qt  "), be32(0x200), []byte("qt  "))
	}
	if _, e = out.file.Write(ftyp); e != nil {
		_ = out.abort()
		return nil, e
	}
	position := int64(len(ftyp))
	if _, e = out.file.Write(join(be32(1), []byte("mdat"), be64(0))); e != nil {
		_ = out.abort()
		return nil, e
	}
	return &MP4{out: out, encoder: encoder, width: width, height: height, fps: fps, maxFrames: opt.MaxFrames, predictive: opt.Codec == H264Predictive, mdat: position, offset: uint64(position + 16)}, nil
}

type predictiveVideoEncoder struct {
	*h264.PredictiveEncoder
	keyframe bool
}

func (e *predictiveVideoEncoder) Encode(img *image.RGBA) ([]byte, error) {
	f, err := e.PredictiveEncoder.Encode(img)
	if err != nil {
		return nil, err
	}
	e.keyframe = f.Keyframe
	return f.NAL, nil
}
func (m *MP4) WriteFrame(index int, img *image.RGBA) error {
	if m.out.closed || index != len(m.sizes) {
		return fmt.Errorf("MP4 expected frame %d", len(m.sizes))
	}
	if len(m.sizes) >= m.maxFrames {
		return fmt.Errorf("MP4 frame count limit exceeded")
	}
	sample, e := m.encoder.Encode(img)
	if e != nil {
		return e
	}
	size := uint64(len(sample)) + 4
	if size > math.MaxUint32 {
		_ = m.out.abort()
		return fmt.Errorf("MP4 sample too large")
	}
	if _, e := m.out.file.Write(be32(uint32(len(sample)))); e != nil {
		_ = m.out.abort()
		return e
	}
	if _, e := m.out.file.Write(sample); e != nil {
		_ = m.out.abort()
		return e
	}
	m.sizes = append(m.sizes, uint32(size))
	if m.predictive && m.encoder.(*predictiveVideoEncoder).keyframe {
		m.keyframes = append(m.keyframes, uint32(len(m.sizes)))
	}
	return nil
}
func (m *MP4) Close() error {
	if m.out.closed {
		return fmt.Errorf("MP4 already closed")
	}
	if len(m.sizes) == 0 {
		return fmt.Errorf("cannot close empty MP4")
	}
	end, e := m.out.file.Seek(0, io.SeekCurrent)
	if e != nil {
		return e
	}
	if _, e = m.out.file.Seek(m.mdat+8, io.SeekStart); e != nil {
		return e
	}
	if _, e = m.out.file.Write(be64(uint64(end - m.mdat))); e != nil {
		return e
	}
	if _, e = m.out.file.Seek(end, io.SeekStart); e != nil {
		return e
	}
	if _, e = m.out.file.Write(m.moov()); e != nil {
		return e
	}
	return m.out.commit()
}
func (m *MP4) Abort() error       { return m.out.abort() }
func be16(n uint16) []byte        { p := make([]byte, 2); binary.BigEndian.PutUint16(p, n); return p }
func be32(n uint32) []byte        { p := make([]byte, 4); binary.BigEndian.PutUint32(p, n); return p }
func be64(n uint64) []byte        { p := make([]byte, 8); binary.BigEndian.PutUint64(p, n); return p }
func join(parts ...[]byte) []byte { return bytes.Join(parts, nil) }
func box(kind string, parts ...[]byte) []byte {
	p := join(parts...)
	return join(be32(uint32(len(p)+8)), []byte(kind), p)
}
func full(kind string, flags uint32, parts ...[]byte) []byte {
	return box(kind, be32(flags), join(parts...))
}
func matrix() []byte {
	return join(be32(0x10000), be32(0), be32(0), be32(0), be32(0x10000), be32(0), be32(0), be32(0), be32(0x40000000))
}
func (m *MP4) moov() []byte {
	e := m.encoder
	count := uint32(len(m.sizes))
	fps := uint32(m.fps)
	mvhd := full("mvhd", 0, be32(0), be32(0), be32(fps), be32(count), be32(0x10000), be16(0x100), make([]byte, 10), matrix(), make([]byte, 24), be32(2))
	tkhd := full("tkhd", 3, be32(0), be32(0), be32(1), be32(0), be32(count), make([]byte, 8), be16(0), be16(0), be16(0), be16(0), matrix(), be32(uint32(m.width)<<16), be32(uint32(m.height)<<16))
	if m.audio != nil {
		scale := m.audio.movieTimescale(m.fps)
		videoDuration := uint64(count) * uint64(scale) / uint64(m.fps)
		duration := max(videoDuration, m.audio.movieDuration(scale))
		mvhd = full("mvhd", 1<<24, be64(0), be64(0), be32(scale), be64(duration), be32(0x10000), be16(0x100), make([]byte, 10), matrix(), make([]byte, 24), be32(3))
		tkhd = full("tkhd", 1<<24|3, be64(0), be64(0), be32(1), be32(0), be64(videoDuration), make([]byte, 8), be16(0), be16(0), be16(0), be16(0), matrix(), be32(uint32(m.width)<<16), be32(uint32(m.height)<<16))
	}
	mdhd := full("mdhd", 0, be32(0), be32(0), be32(fps), be32(count), be16(0x55c4), be16(0))
	hdlr := full("hdlr", 0, be32(0), []byte("vide"), make([]byte, 12), []byte("GoMath Video\x00"))
	vmhd := full("vmhd", 1, make([]byte, 8))
	dinf := box("dinf", full("dref", 0, be32(1), full("url ", 1)))
	sps, pps := e.SPS(), e.PPS()
	avcc := box("avcC", []byte{1, sps[1], sps[2], sps[3], 0xff, 0xe1}, be16(uint16(len(sps))), sps, []byte{1}, be16(uint16(len(pps))), pps)
	entry := box("avc1", make([]byte, 6), be16(1), make([]byte, 16), be16(uint16(m.width)), be16(uint16(m.height)), be32(0x480000), be32(0x480000), be32(0), be16(1), make([]byte, 32), be16(0x18), be16(0xffff), avcc)
	stsd := full("stsd", 0, be32(1), entry)
	stts := full("stts", 0, be32(1), be32(count), be32(1))
	stsc := full("stsc", 0, be32(1), be32(1), be32(count), be32(1))
	sizes := make([]byte, 4*len(m.sizes))
	for i, size := range m.sizes {
		binary.BigEndian.PutUint32(sizes[i*4:], size)
	}
	stsz := full("stsz", 0, be32(0), be32(count), sizes)
	co64 := full("co64", 0, be32(1), be64(m.offset))
	var stss []byte
	if m.predictive {
		indices := make([]byte, 4*len(m.keyframes))
		for i, index := range m.keyframes {
			binary.BigEndian.PutUint32(indices[4*i:], index)
		}
		stss = full("stss", 0, be32(uint32(len(m.keyframes))), indices)
	}
	stbl := box("stbl", stsd, stts, stss, stsc, stsz, co64)
	minf := box("minf", vmhd, dinf, stbl)
	mdia := box("mdia", mdhd, hdlr, minf)
	trak := box("trak", tkhd, mdia)
	if m.audio != nil {
		return box("moov", mvhd, trak, m.audio.trak(m.audio.movieTimescale(m.fps)))
	}
	return box("moov", mvhd, trak)
}
