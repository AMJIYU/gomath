package h264

import (
	"bytes"
	"fmt"
	"image"
	"sync"
)

// MaxMotionSearchRadius bounds each motion-enabled P macroblock to at most
// (2*4+1)^2=81 complete inter transform/quantization/CAVLC trials plus one intra.
const MaxMotionSearchRadius = 4

// PredictiveOptions configures a bounded single-reference encoder. QP and
// ModeSearch have the same meaning as IntraOptions. GOP is required, in
// [1,65536]: it counts pictures including the initial IDR; GOP 1 is all-IDR.
// There is no B picture or rate control.
type PredictiveOptions struct {
	QP         int
	ModeSearch bool
	GOP        int
	// InterResidual enables P_L0_16x16 transform/quantized CAVLC
	// residuals, competing with intra by a bounded local rate/distortion cost.
	// Prediction is zero-motion unless MotionSearchRadius is nonzero.
	// It also permits skip when all inter coefficients quantize to zero.
	// False preserves the original exact-skip/intra bitstream byte for byte.
	InterResidual bool
	// MotionSearchRadius opts into exhaustive integer-luma motion search in
	// [-R,R] on each axis, relative to the co-located block. Zero preserves
	// existing bytes. Values 1..4 require InterResidual; every P macroblock
	// evaluates (2R+1)^2 full transform/quant/CAVLC candidates plus one intra
	// trial. No fractional-luma, multi-reference or subpartition search exists.
	MotionSearchRadius int
}

// PredictiveFrame is one complete reference-picture NAL (header and emulation
// prevention included, no start code or length). Keyframe identifies an IDR.
// FrameNum is the four-bit H.264 frame_num; POC is the derived type-2 picture
// order count, restarting at zero on IDR. DTS equals PTS: there is no reordering.
// SkippedMacroblocks counts P_SKIP MBs, including padding. With InterResidual,
// this also includes non-exact inputs whose inter residual quantizes to zero.
type PredictiveFrame struct {
	NAL                []byte
	Keyframe           bool
	FrameNum           uint8
	POC                int
	SkippedMacroblocks int
}

// PredictiveEncoder emits IDR and P slices. Each P macroblock is either a
// P_SKIP referencing the immediately previous reconstructed picture,
// or I4x4/CAVLC, with optional P_L0_16x16 residuals. By default skip requires
// exact converted-YUV equality with the reference reconstruction. InterResidual
// additionally allows skip if all inter residual coefficients quantize to zero.
// MotionSearchRadius optionally searches bounded integer motion; skip then uses
// its normative inferred vector, which can differ from the coded-MB predictor.
// Comparing input RGBA equality alone is never sufficient.
//
// Calls are serialized, but callers must submit pictures in display order.
// Do not mutate the input during Encode. Errors consume no picture and change
// no reference or sequence state. After dropping any successful output, call
// ForceIDR or Reset before encoding more pictures for that decoder.
// Use this encoder's SPS/PPS: NewIntra's SPS has no reference-picture storage.
type PredictiveEncoder struct {
	interResidual bool
	motionRadius  int
	workspace     *interWorkspace
	intra         *IntraEncoder
	gop           int
	mu            sync.Mutex
	reference     picture420
	position      int
	forceIDR      bool
	nextIDR       uint32
}

// NewPredictive accepts NewIntra's image geometry/rate limits and an explicit
// GOP. The first successful Encode always produces an IDR.
func NewPredictive(width, height, fps int, options PredictiveOptions) (*PredictiveEncoder, error) {
	intra, err := NewIntra(width, height, fps, IntraOptions{QP: options.QP, ModeSearch: options.ModeSearch})
	if err != nil {
		return nil, err
	}
	if options.GOP < 1 || options.GOP > 65536 {
		return nil, fmt.Errorf("H.264 predictive GOP must be in 1..65536")
	}
	if options.MotionSearchRadius < 0 || options.MotionSearchRadius > MaxMotionSearchRadius {
		return nil, fmt.Errorf("H.264 motion search radius must be in 0..%d", MaxMotionSearchRadius)
	}
	if options.MotionSearchRadius != 0 && !options.InterResidual {
		return nil, fmt.Errorf("H.264 motion search requires InterResidual")
	}
	return &PredictiveEncoder{intra: intra, gop: options.GOP, interResidual: options.InterResidual, motionRadius: options.MotionSearchRadius}, nil
}

// SPS returns the escaped NAL for one short-term reference and type-2 POC.
func (e *PredictiveEncoder) SPS() []byte { return e.intra.geometry.spsReferences(e.intra.level, 1) }

// PPS returns the escaped NAL, using the configured QP-dependent chroma offset.
func (e *PredictiveEncoder) PPS() []byte { return e.intra.PPS() }

// ForceIDR makes the next successful Encode start an independently decodable
// GOP. A failed Encode leaves the request pending. Send SPS/PPS at a new stream
// entry point. This does not retroactively repair already dropped pictures.
func (e *PredictiveEncoder) ForceIDR() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.forceIDR = true
}

// Reset also releases the retained reference and optional reusable workspace.
// The next successful output is an IDR. Parameter sets and options remain unchanged.
func (e *PredictiveEncoder) Reset() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.reference = picture420{}
	e.workspace = nil
	e.position = 0
	e.forceIDR = true
}

// Encode converts and encodes one picture, committing its reference and sequence
// state only on success. The returned NAL owns its bytes.
func (e *PredictiveEncoder) Encode(img *image.RGBA) (PredictiveFrame, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	width, height := e.intra.geometry.Width, e.intra.geometry.Height
	if err := validateRGBA(img, width, height); err != nil {
		return PredictiveFrame{}, err
	}
	key := e.reference.planes[0] == nil || e.forceIDR || e.position >= e.gop
	var (
		out PredictiveFrame
		rec picture420
	)
	if key {
		var err error
		out.NAL, rec, err = e.intra.encodeIDR(img, e.nextIDR)
		if err != nil {
			return PredictiveFrame{}, err
		}
		out.Keyframe = true
	} else {
		out.FrameNum = uint8(e.position % 16)
		out.POC = 2 * e.position
		out.NAL, rec, out.SkippedMacroblocks = e.encodeP(img, out.FrameNum)
	}
	// Commit only after complete encoding. IDR ids alternate even across Reset
	// so consecutive IDRs are distinct pictures (7.4.1.2.4, 7.4.3).
	if key {
		e.position = 0
		e.nextIDR ^= 1
	}
	e.position++
	e.reference, e.forceIDR = rec, false
	return out, nil
}

func (e *PredictiveEncoder) encodeP(img *image.RGBA, frameNum uint8) ([]byte, picture420, int) {
	if e.motionRadius != 0 {
		return e.encodePMotion(img, frameNum)
	}
	if e.interResidual {
		return e.encodePResidual(img, frameNum)
	}
	width, height := e.intra.geometry.Width, e.intra.geometry.Height
	pw, ph := (width+15)/16*16, (height+15)/16*16
	src, rec := makePicture420(img, width, height, pw, ph), newPicture420(pw, ph)
	counts := [3][]uint8{make([]uint8, pw*ph/16), make([]uint8, pw*ph/64), make([]uint8, pw*ph/64)}
	var modes, scratch []byte
	if e.intra.modeSearch {
		modes, scratch = make([]byte, pw*ph/16), make([]byte, 128)
		for i := range modes {
			modes[i] = intraDC // inter neighbour mode inferred as DC (8.3.1.1)
		}
	}
	var w bits
	w.data = make([]byte, 0, pw*ph/2)
	w.ue(0) // first_mb_in_slice
	w.ue(0) // P slice
	w.ue(0) // pic_parameter_set_id
	w.uint(uint32(frameNum), 4)
	w.bit(0) // num_ref_idx_active_override_flag: one L0 reference
	w.bit(0) // ref_pic_list_modification_flag_l0: latest short-term
	w.bit(0) // adaptive_ref_pic_marking_mode_flag: sliding window
	w.se(int32(e.intra.qp - 26))
	w.ue(1) // deblocking disabled, also on reference pictures
	run, skipped := 0, 0
	for my := 0; my < ph; my += 16 {
		for mx := 0; mx < pw; mx += 16 {
			if equalMacroblock(src, e.reference, mx, my) {
				// All inter neighbours in this subset have mv=(0,0), refIdx=0;
				// intra neighbours supply mv=(0,0), refIdx=-1. 8.4.1.1 and
				// 8.4.1.3 therefore infer zero motion even when A/B are intra.
				copyMacroblock(rec, e.reference, mx, my)
				run++
				skipped++
				// Counts stay zero (9.2.1); modes stay inferred DC.
				continue
			}
			w.ue(uint32(run)) // mb_skip_run, including zero before a coded MB
			run = 0
			w.ue(5) // P-slice I_NxN (Table 7-13), followed by intra CBP mapping
			e.intra.writeIntraMB(&w, src, rec, counts, modes, scratch, mx, my)
		}
	}
	if run > 0 {
		w.ue(uint32(run))
	}
	w.trailing()
	return nal(0x41, w.data), rec, skipped
}

func equalMacroblock(a, b picture420, x, y int) bool {
	for c := range 3 {
		size, stride, bx, by := 16, a.stride, x, y
		if c != 0 {
			size, stride, bx, by = 8, stride/2, x/2, y/2
		}
		for j := range size {
			i := (by+j)*stride + bx
			if !bytes.Equal(a.planes[c][i:i+size], b.planes[c][i:i+size]) {
				return false
			}
		}
	}
	return true
}

func copyMacroblock(dst, src picture420, x, y int) {
	for c := range 3 {
		size, stride, bx, by := 16, src.stride, x, y
		if c != 0 {
			size, stride, bx, by = 8, stride/2, x/2, y/2
		}
		for j := range size {
			i := (by+j)*stride + bx
			copy(dst.planes[c][i:i+size], src.planes[c][i:i+size])
		}
	}
}
