package export

import (
	"encoding/binary"
	"fmt"
	"gomath/audio"
	"math"
)

type movAudio struct {
	frames, rate, channels int
	offset                 uint64
}

func (a *movAudio) movieTimescale(fps int) uint32 {
	x, y := fps, a.rate
	for y != 0 {
		x, y = y, x%y
	}
	return uint32(fps / x * a.rate)
}
func (a *movAudio) movieDuration(scale uint32) uint64 {
	return uint64(a.frames) * uint64(scale) / uint64(a.rate)
}

// NewMOVWithAudio writes a QuickTime MOV with the selected pure-Go H.264 video
// and a sample-accurate PCM16 little-endian audio track. The soundtrack begins at
// zero and is neither resampled nor trimmed. Video and audio can have different
// lengths; the movie duration covers both. There is no compressed audio, channel
// conversion, edit list, streaming interleaving or fast-start metadata.
//
// Accepted audio is nonempty mono or stereo, 1..65535 Hz, within the default
// audio sample limit and [-1,1] amplitude. A zero/unspecified channel mask or the
// conventional mono-center/stereo-LR mask is accepted. Quantization rounds to
// nearest PCM16, saturating +1 to 32767; no normalization or dither is applied.
// Audio is written once in 32 KiB blocks before the video; pixel/sample metadata
// remains subject to MP4Options. Output is published only by a successful Close.
func NewMOVWithAudio(target string, width, height, fps int, pcm *audio.PCM, opt MP4Options) (*MP4, error) {
	if pcm == nil {
		return nil, fmt.Errorf("MOV requires PCM audio")
	}
	f := pcm.Format()
	if f.SampleRate < 1 || f.SampleRate > 65535 || (f.Channels != 1 && f.Channels != 2) {
		return nil, fmt.Errorf("MOV PCM16 requires mono/stereo audio at 1..65535 Hz")
	}
	if f.ChannelMask != 0 && !(f.Channels == 1 && f.ChannelMask == 4 || f.Channels == 2 && f.ChannelMask == 3) {
		return nil, fmt.Errorf("MOV PCM16 does not support this speaker layout")
	}
	frames := pcm.Frames()
	if frames == 0 || frames > audio.DefaultLimits().MaxSamples/f.Channels {
		return nil, fmt.Errorf("MOV audio is empty or exceeds the default sample budget")
	}
	// Validate before creating any file; PCM is immutable so this also covers
	// the subsequent write without retaining a second copy of all samples.
	for i := range frames {
		for c := range f.Channels {
			x, err := pcm.Sample(i, c)
			if err != nil {
				return nil, err
			}
			if math.IsNaN(x) || math.IsInf(x, 0) || x < -1 || x > 1 {
				return nil, fmt.Errorf("MOV audio sample %d/%d exceeds PCM16 range", i, c)
			}
		}
	}
	m, err := newVideoFile(target, width, height, fps, opt, true)
	if err != nil {
		return nil, err
	}
	m.audio = &movAudio{frames: frames, rate: f.SampleRate, channels: f.Channels, offset: m.offset}
	var block [32768]byte
	for start := 0; start < frames; {
		count := min(frames-start, len(block)/(2*f.Channels))
		for i := range count {
			for c := range f.Channels {
				x, e := pcm.Sample(start+i, c)
				if e != nil {
					_ = m.Abort()
					return nil, e
				}
				v := int32(math.Round(x * 32768))
				v = min(v, 32767)
				binary.LittleEndian.PutUint16(block[(i*f.Channels+c)*2:], uint16(int16(v)))
			}
		}
		n := count * f.Channels * 2
		if _, err = m.out.file.Write(block[:n]); err != nil {
			_ = m.Abort()
			return nil, err
		}
		m.offset += uint64(n)
		start += count
	}
	return m, nil
}

func (a *movAudio) trak(scale uint32) []byte {
	tkhd := full("tkhd", 1<<24|3, be64(0), be64(0), be32(2), be32(0), be64(a.movieDuration(scale)), make([]byte, 8), be16(0), be16(0), be16(0x100), be16(0), matrix(), be32(0), be32(0))
	mdhd := full("mdhd", 0, be32(0), be32(0), be32(uint32(a.rate)), be32(uint32(a.frames)), be16(0x55c4), be16(0))
	hdlr := full("hdlr", 0, be32(0), []byte("soun"), make([]byte, 12), []byte("GoMath Audio\x00"))
	smhd := full("smhd", 0, make([]byte, 4))
	dinf := box("dinf", full("dref", 0, be32(1), full("url ", 1)))
	entry := box("sowt", make([]byte, 6), be16(1), make([]byte, 8), be16(uint16(a.channels)), be16(16), be16(0), be16(0), be32(uint32(a.rate)<<16))
	stsd := full("stsd", 0, be32(1), entry)
	stts := full("stts", 0, be32(1), be32(uint32(a.frames)), be32(1))
	stsc := full("stsc", 0, be32(1), be32(1), be32(uint32(a.frames)), be32(1))
	stsz := full("stsz", 0, be32(uint32(2*a.channels)), be32(uint32(a.frames)))
	co64 := full("co64", 0, be32(1), be64(a.offset))
	stbl := box("stbl", stsd, stts, stsc, stsz, co64)
	minf := box("minf", smhd, dinf, stbl)
	return box("trak", tkhd, box("mdia", mdhd, hdlr, minf))
}
