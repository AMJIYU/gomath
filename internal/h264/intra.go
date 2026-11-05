package h264

import (
	"fmt"
	"image"
	"image/color"
)

// IntraOptions configures spatial quantization. QP is the luma quantization
// parameter in [0,51]; smaller values preserve more detail and cost more bits.
// Chroma QP follows H.264's mapping, with a minimum of 4 signalled by the PPS:
// this keeps extreme chroma DC residuals within Baseline's CAVLC level limit.
// QP 0 is still lossy. There is no bitrate target or temporal prediction.
// QP 24 is a useful starting point.
type IntraOptions struct {
	QP int
	// ModeSearch compares legal DC, vertical and horizontal I4x4 predictors.
	// It trades additional encoding work for a local rate/distortion choice.
	// False preserves the original DC-only bitstream exactly.
	ModeSearch bool
}

// IntraEncoder encodes 8-bit full-range YCbCr 4:2:0 Constrained Baseline H.264.
// Each call produces one IDR slice, using I_4x4 prediction, integer transform,
// quantization and CAVLC. Optional ModeSearch considers DC/vertical/horizontal;
// chroma always uses DC prediction and a 2x2 DC transform. Deblocking is disabled.
// There is no inter-frame prediction, rate control or PCM fallback. It is safe
// for concurrent calls.
type IntraEncoder struct {
	geometry   Encoder
	qp         int
	level      uint32
	modeSearch bool
}

// NewIntra accepts the same even dimensions and frame rates as New, plus an
// explicit QP. The existing New/Encoder API continues to encode I_PCM.
func NewIntra(width, height, fps int, options IntraOptions) (*IntraEncoder, error) {
	geometry, err := New(width, height, fps)
	if err != nil {
		return nil, err
	}
	if options.QP < 0 || options.QP > 51 {
		return nil, fmt.Errorf("H.264 intra QP must be in 0..51")
	}
	// Preserve level 5.1 for ordinary dimensions. Higher levels are necessary
	// for large pictures or frame rates (Annex A, Table A-1). This bounds frame
	// size/processing rate; no HRD/VBV bitrate guarantee is made.
	mbs := ((width + 15) / 16) * ((height + 15) / 16)
	level := uint32(61)
	for _, limit := range []struct{ level, frame, rate int }{{51, 36864, 983040}, {52, 36864, 2073600}, {60, 139264, 4177920}} {
		if mbs <= limit.frame && mbs*fps <= limit.rate {
			level = uint32(limit.level)
			break
		}
	}
	return &IntraEncoder{geometry: *geometry, qp: options.QP, level: level, modeSearch: options.ModeSearch}, nil
}

// SPS returns the sequence parameter set NAL, including its one-byte header
// and emulation prevention bytes, without an Annex B start code or length.
func (e *IntraEncoder) SPS() []byte { return e.geometry.sps(e.level) }

// PPS returns the picture parameter set NAL with the same framing as SPS.
// The PPS initial QP is 26; Encode signals the configured luma QP in the slice.
// QP 0..3 requires this encoder's PPS, which signals a positive chroma offset.
func (e *IntraEncoder) PPS() []byte { return e.geometry.pps(int32(max(0, 4-e.qp))) }

// Encode returns one complete IDR NAL with the same framing as SPS. Dimensions
// must match the constructor. Nonzero image origins and RGBA subimages work;
// premultiplied RGB composites alpha onto black, as with the PCM encoder.
func (e *IntraEncoder) Encode(img *image.RGBA) ([]byte, error) {
	n, _, err := e.encode(img)
	return n, err
}

type picture420 struct {
	planes [3][]byte
	stride int
}

func newPicture420(pw, ph int) picture420 {
	return picture420{planes: [3][]byte{make([]byte, pw*ph), make([]byte, pw*ph/4), make([]byte, pw*ph/4)}, stride: pw}
}

func makePicture420(img *image.RGBA, width, height, pw, ph int) picture420 {
	p := newPicture420(pw, ph)
	fillPicture420(p, img, width, height, pw, ph)
	return p
}

// fillPicture420 overwrites every padded sample, allowing serialized encoders
// to reuse conversion storage without sharing their reconstructed reference.
func fillPicture420(p picture420, img *image.RGBA, width, height, pw, ph int) {
	for y := 0; y < ph; y += 2 {
		for x := 0; x < pw; x += 2 {
			var cb, cr int
			for dy := range 2 {
				for dx := range 2 {
					i := img.PixOffset(img.Rect.Min.X+min(x+dx, width-1), img.Rect.Min.Y+min(y+dy, height-1))
					yv, u, v := color.RGBToYCbCr(img.Pix[i], img.Pix[i+1], img.Pix[i+2])
					p.planes[0][(y+dy)*pw+x+dx] = yv
					cb, cr = cb+int(u), cr+int(v)
				}
			}
			p.planes[1][y/2*(pw/2)+x/2] = byte((cb + 2) / 4)
			p.planes[2][y/2*(pw/2)+x/2] = byte((cr + 2) / 4)
		}
	}
}

func (e *IntraEncoder) encode(img *image.RGBA) ([]byte, picture420, error) {
	return e.encodeIDR(img, 0)
}

func (e *IntraEncoder) encodeIDR(img *image.RGBA, id uint32) ([]byte, picture420, error) {
	width, height := e.geometry.Width, e.geometry.Height
	if err := validateRGBA(img, width, height); err != nil {
		return nil, picture420{}, err
	}
	pw, ph := (width+15)/16*16, (height+15)/16*16
	src, rec := makePicture420(img, width, height, pw, ph), newPicture420(pw, ph)
	counts := [3][]uint8{make([]uint8, pw*ph/16), make([]uint8, pw*ph/64), make([]uint8, pw*ph/64)}
	var modes []uint8
	var searchScratch []byte
	if e.modeSearch {
		modes = make([]uint8, pw*ph/16)
		searchScratch = make([]byte, 128)
	}
	var w bits
	w.data = make([]byte, 0, pw*ph/2)
	w.ue(0)                // first_mb_in_slice
	w.ue(2)                // I slice
	w.ue(0)                // pic_parameter_set_id
	w.uint(0, 4)           // frame_num
	w.ue(id)               // idr_pic_id
	w.bit(0)               // no_output_of_prior_pics_flag
	w.bit(0)               // long_term_reference_flag
	w.se(int32(e.qp - 26)) // slice_qp_delta
	w.ue(1)                // disable_deblocking_filter_idc
	for my := 0; my < ph; my += 16 {
		for mx := 0; mx < pw; mx += 16 {
			w.ue(0) // I_NxN (4x4)
			e.writeIntraMB(&w, src, rec, counts, modes, searchScratch, mx, my)
		}
	}
	w.trailing()
	return nal(0x65, w.data), rec, nil
}

// validateRGBA completes all fallible work before a stateful encoder commits.
func validateRGBA(img *image.RGBA, width, height int) error {
	if img == nil || img.Rect.Dx() != width || img.Rect.Dy() != height || width < 2 || height < 2 {
		return fmt.Errorf("H.264 frame dimensions do not match encoder")
	}
	// image.RGBA is public and may be malformed. Check before indexing its Pix.
	row := width * 4
	if img.Stride < row || len(img.Pix) < row || img.Stride > (len(img.Pix)-row)/(height-1) {
		return fmt.Errorf("H.264 invalid RGBA pixel storage")
	}
	return nil
}

// writeIntraMB writes syntax after mb_type. Context arrays are picture-local;
// inter neighbours have zero residual counts and inferred DC intra modes.
func (e *IntraEncoder) writeIntraMB(w *bits, src, rec picture420, counts [3][]uint8, modes []uint8, searchScratch []byte, mx, my int) {
	pw := rec.stride
	var searched searchedLumaMB
	if e.modeSearch {
		searched = searchLumaMB(w, src.planes[0], rec.planes[0], counts[0], modes, pw, mx, my, e.qp, searchScratch)
	} else {
		for range 16 {
			w.bit(1) // every mode is DC (2), including its predicted mode
		}
	}
	w.ue(0) // intra_chroma_pred_mode: DC
	w.ue(0) // intra CBP mapping: 47 (all luma and chroma DC+AC)
	w.se(0) // mb_qp_delta
	for block := range 16 {
		if e.modeSearch {
			// Modes precede residual syntax in a macroblock. Search has
			// already reconstructed these blocks in decoding order.
			cavlc(w, searched.coeff[block][:], searched.context[block])
			continue
		}
		x := mx + (block/4%2)*8 + (block%2)*4
		y := my + (block/8)*8 + (block/2%2)*4
		pred := lumaDC(rec.planes[0], pw, x, y)
		var residual [16]int
		for j := range 4 {
			for i := range 4 {
				residual[j*4+i] = int(src.planes[0][(y+j)*pw+x+i]) - pred
			}
		}
		q := quant4(forward4(residual), e.qp)
		var scan [16]int
		for i, pos := range frameScan {
			scan[i] = q[pos]
		}
		nc := neighborCount(counts[0], pw/4, x/4, y/4)
		counts[0][y/4*(pw/4)+x/4] = uint8(cavlc(w, scan[:], nc))
		writeBlock(rec.planes[0], pw, x, y, pred, inverse4(dequant4(q, e.qp)))
	}
	// Chroma syntax orders BOTH DC blocks before either component's AC.
	var ac [2][4][16]int
	for c := range 2 {
		ac[c] = encodeChroma(w, src.planes[c+1], rec.planes[c+1], pw/2, mx/2, my/2, chromaQP(max(4, e.qp)))
	}
	for c := range 2 {
		for b := range 4 {
			x, y := mx/2+(b%2)*4, my/2+(b/2)*4
			var scan [15]int
			for i, pos := range frameScan[1:] {
				scan[i] = ac[c][b][pos]
			}
			nc := neighborCount(counts[c+1], pw/8, x/4, y/4)
			counts[c+1][y/4*(pw/8)+x/4] = uint8(cavlc(w, scan[:], nc))
		}
	}
}

func neighborCount(count []uint8, stride, x, y int) int {
	var a, b int
	if x > 0 {
		a = int(count[y*stride+x-1])
	}
	if y > 0 {
		b = int(count[(y-1)*stride+x])
	}
	if x > 0 && y > 0 {
		return (a + b + 1) >> 1
	}
	return a + b
}

func lumaDC(rec []byte, stride, x, y int) int {
	var sum, n int
	if x > 0 {
		for j := range 4 {
			sum += int(rec[(y+j)*stride+x-1])
		}
		n += 4
	}
	if y > 0 {
		for i := range 4 {
			sum += int(rec[(y-1)*stride+x+i])
		}
		n += 4
	}
	if n == 0 {
		return 128
	}
	return (sum + n/2) / n
}

func chromaDC(rec []byte, stride, x, y int) [4]int {
	var top, left [2]int
	if y > 0 {
		for i := range 8 {
			top[i/4] += int(rec[(y-1)*stride+x+i])
		}
	}
	if x > 0 {
		for j := range 8 {
			left[j/4] += int(rec[(y+j)*stride+x-1])
		}
	}
	if x > 0 && y > 0 {
		return [4]int{(top[0] + left[0] + 4) >> 3, (top[1] + 2) >> 2,
			(left[1] + 2) >> 2, (top[1] + left[1] + 4) >> 3}
	}
	if y > 0 {
		return [4]int{(top[0] + 2) >> 2, (top[1] + 2) >> 2, (top[0] + 2) >> 2, (top[1] + 2) >> 2}
	}
	if x > 0 {
		return [4]int{(left[0] + 2) >> 2, (left[0] + 2) >> 2, (left[1] + 2) >> 2, (left[1] + 2) >> 2}
	}
	return [4]int{128, 128, 128, 128}
}

func encodeChroma(w *bits, src, rec []byte, stride, x, y, qp int) [4][16]int {
	pred := chromaDC(rec, stride, x, y)
	var q [4][16]int
	var dc [4]int
	for b := range 4 {
		bx, by := x+(b%2)*4, y+(b/2)*4
		var v [16]int
		for j := range 4 {
			for i := range 4 {
				v[j*4+i] = int(src[(by+j)*stride+bx+i]) - pred[b]
			}
		}
		v = forward4(v)
		dc[b], v[0] = v[0], 0
		q[b] = quant4(v, qp)
	}
	dc = hadamard2(dc)
	for i := range dc {
		dc[i] = quantize(dc[i], forwardScale[qp%6][0], 16+qp/6)
	}
	cavlc(w, dc[:], -1) // 2x2 scan is row-major
	dc = hadamard2(dc)
	for b := range 4 {
		v := dequant4(q[b], qp)
		// 8.5.11.2: LevelScale is 16*inverseScale; division by 32.
		v[0] = (dc[b] * (inverseScale[qp%6][0] << (qp / 6))) >> 1
		writeBlock(rec, stride, x+(b%2)*4, y+(b/2)*4, pred[b], inverse4(v))
	}
	return q
}

func writeBlock(rec []byte, stride, x, y, pred int, residual [16]int) {
	for j := range 4 {
		for i := range 4 {
			rec[(y+j)*stride+x+i] = byte(max(0, min(255, pred+residual[j*4+i])))
		}
	}
}
