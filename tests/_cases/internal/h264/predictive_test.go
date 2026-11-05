package h264

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// This small syntax decoder intentionally consumes the emitted modes and CAVLC
// coefficients, including skip runs and cross-MB contexts. Independent FFmpeg
// fixtures below additionally validate transforms, reference management and POC.
func decodePredictive(t *testing.T, out PredictiveFrame, ref picture420, width, height, qp int) (picture420, []bool) {
	t.Helper()
	return decodePredictiveWithStats(t, out, ref, width, height, qp, nil)
}

func decodePredictiveWithStats(t *testing.T, out PredictiveFrame, ref picture420, width, height, qp int, stats *interDecodeStats) (picture420, []bool) {
	t.Helper()
	r := reader{p: unescape(out.NAL)}
	kind := uint32(0)
	if out.Keyframe {
		kind = 2
	}
	if out.NAL[0] != 0x41 && !out.Keyframe || out.Keyframe && out.NAL[0] != 0x65 || r.ue() != 0 || r.ue() != kind || r.ue() != 0 || r.uint(4) != uint32(out.FrameNum) {
		t.Fatal("invalid predictive header", out)
	}
	if out.Keyframe {
		if r.ue() > 1 || r.bit() != 0 || r.bit() != 0 {
			t.Fatal("invalid IDR marking")
		}
	} else if r.bit() != 0 || r.bit() != 0 || r.bit() != 0 {
		t.Fatal("unexpected reference list/marking")
	}
	code := int(r.ue())
	delta := (code + 1) / 2
	if code%2 == 0 {
		delta = -code / 2
	}
	if delta+26 != qp || r.ue() != 1 {
		t.Fatal("QP/deblocking mismatch")
	}
	pw, ph := (width+15)/16*16, (height+15)/16*16
	rec := newPicture420(pw, ph)
	counts := [3][]uint8{make([]uint8, pw*ph/16), make([]uint8, pw*ph/64), make([]uint8, pw*ph/64)}
	modes := make([]uint8, pw*ph/16)
	for i := range modes {
		modes[i] = 2
	}
	skips := make([]bool, pw*ph/256)
	motion := make([]decodedMotion, len(skips))
	prediction := newPicture420(pw, ph)
	for mb := 0; mb < len(skips); {
		if !out.Keyframe {
			run := int(r.ue())
			if run > len(skips)-mb {
				t.Fatal("skip run beyond picture")
			}
			for range run {
				if ref.planes[0] == nil {
					t.Fatal("skip without reference")
				}
				mv := predictDecodedMotion(motion, pw/16, mb, true)
				motion[mb] = mv
				compensateDecodedMotion(t, rec, ref, mb%(pw/16)*16, mb/(pw/16)*16, mv)
				if stats != nil && (mv.x != 0 || mv.y != 0) {
					stats.MovingSkip++
				}
				skips[mb] = true
				mb++
			}
			if mb == len(skips) {
				break
			}
		}
		mx, my := mb%(pw/16)*16, mb/(pw/16)*16
		wantType := uint32(5)
		if out.Keyframe {
			wantType = 0
		}
		mbType := r.ue()
		if !out.Keyframe && mbType == 0 {
			mv := predictDecodedMotion(motion, pw/16, mb, false)
			dx, dy := signedMotion(&r), signedMotion(&r)
			mv.x, mv.y = mv.x+dx, mv.y+dy
			motion[mb] = mv
			compensateDecodedMotion(t, prediction, ref, mx, my, mv)
			cbp := decodeInterResidual(t, &r, prediction, rec, counts, mx, my, qp)
			if stats != nil {
				stats.Inter++
				stats.CBP[cbp]++
				if mv.x != 0 || mv.y != 0 {
					stats.Moved++
				}
				if mv.x&7 != 0 || mv.y&7 != 0 {
					stats.OddChroma++
				}
				if dx != 0 || dy != 0 {
					stats.MVD++
				}
			}
			mb++
			continue
		}
		if mbType != wantType {
			t.Fatal("expected supported macroblock type", mbType)
		}
		if stats != nil {
			stats.Intra++
		}
		for b := range 16 {
			x, y := mx+(b/4%2)*8+b%2*4, my+b/8*8+(b/2%2)*4
			bx, by := x/4, y/4
			mode := uint8(2)
			if bx > 0 && by > 0 {
				mode = min(modes[by*(pw/4)+bx-1], modes[(by-1)*(pw/4)+bx])
			}
			if r.bit() == 0 {
				rem := uint8(r.uint(3))
				if rem >= mode {
					rem++
				}
				mode = rem
			}
			if mode > 2 || mode == 0 && y == 0 || mode == 1 && x == 0 {
				t.Fatal("invalid predictor", mode, x, y)
			}
			modes[by*(pw/4)+bx] = mode
		}
		if r.ue() != 0 || r.ue() != 0 || r.ue() != 0 {
			t.Fatal("unexpected chroma/CBP/delta")
		}
		for b := range 16 {
			x, y := mx+(b/4%2)*8+b%2*4, my+b/8*8+(b/2%2)*4
			scan := readCAVLC(&r, 16, neighborCount(counts[0], pw/4, x/4, y/4))
			var q [16]int
			for i, pos := range frameScan {
				q[pos] = scan[i]
				if scan[i] != 0 {
					counts[0][y/4*(pw/4)+x/4]++
				}
			}
			residual := inverse4(dequant4(q, qp))
			mode, dc := modes[y/4*(pw/4)+x/4], lumaDC(rec.planes[0], pw, x, y)
			for j := range 4 {
				for i := range 4 {
					pred := dc
					if mode == 0 {
						pred = int(rec.planes[0][(y-1)*pw+x+i])
					} else if mode == 1 {
						pred = int(rec.planes[0][(y+j)*pw+x-1])
					}
					rec.planes[0][(y+j)*pw+x+i] = byte(max(0, min(255, pred+residual[j*4+i])))
				}
			}
		}
		var dc [2][4]int
		for c := range 2 {
			copy(dc[c][:], readCAVLC(&r, 4, -1))
			dc[c] = hadamard2(dc[c])
		}
		cq, stride := chromaQP(max(4, qp)), pw/2
		for c := 1; c < 3; c++ {
			pred := chromaDC(rec.planes[c], stride, mx/2, my/2)
			for b := range 4 {
				x, y := mx/2+b%2*4, my/2+b/2*4
				scan := readCAVLC(&r, 15, neighborCount(counts[c], pw/8, x/4, y/4))
				var q [16]int
				for i, pos := range frameScan[1:] {
					q[pos] = scan[i]
					if scan[i] != 0 {
						counts[c][y/4*(pw/8)+x/4]++
					}
				}
				v := dequant4(q, cq)
				v[0] = dc[c-1][b] * (inverseScale[cq%6][0] << (cq / 6)) >> 1
				writeBlock(rec.planes[c], stride, x, y, pred[b], inverse4(v))
			}
		}
		mb++
	}
	if r.bit() != 1 {
		t.Fatal("missing RBSP stop bit")
	}
	for r.pos < len(r.p)*8 {
		if r.bit() != 0 {
			t.Fatal("trailing nonzero syntax")
		}
	}
	return rec, skips
}

func predictiveImage(kind string, width, height, f int) *image.RGBA {
	if kind == "mixed" || kind == "static" {
		if kind == "static" {
			f = 0
		}
		return intraImage(width, height, f)
	}
	if kind == "extreme" {
		return extremeImage(width, height, f)
	}
	im := image.NewRGBA(image.Rect(-2, 3, width-2, height+3))
	mbw, mbh := (width+15)/16, (height+15)/16
	for y := range height {
		for x := range width {
			v := byte(128)
			if kind != "flat" && f > 0 && (y/16*mbw+x/16) == f%(mbw*mbh) {
				v = byte((x*37 + y*13 + f*17) % 256)
			}
			if kind == "checker" {
				v = 128
				if f%2 == 1 && (x/16+y/16)%2 == 0 {
					v = byte((x*37 + y*13 + f*17) % 256)
				}
			}
			c := color.RGBA{v, v, v, 255}
			if kind == "chroma" {
				c = color.RGBA{128, 128, 128, 255}
				if f%2 != 0 && x/16 == 1 {
					c = color.RGBA{130, 127, 128, 255} // same Y=128, different Cr
				}
			}
			im.SetRGBA(x-2, y+3, c)
		}
	}
	return im
}

type predictiveVector struct {
	Name, Kind                     string
	Width, Height, QP, GOP, Frames int
	ModeSearch                     bool
	BitstreamSHA256, DecodedSHA256 string
	Bytes, Skipped                 int
}

func predictiveVectors() []predictiveVector {
	var out []predictiveVector
	for _, search := range []bool{false, true} {
		for _, dim := range [][2]int{{2, 2}, {18, 22}, {34, 38}, {50, 34}, {320, 180}} {
			for _, qp := range []int{0, 24, 51} {
				out = append(out, predictiveVector{Kind: "local", Width: dim[0], Height: dim[1], QP: qp, GOP: 7, Frames: 23, ModeSearch: search})
			}
		}
		for qp := range 52 {
			out = append(out, predictiveVector{Kind: "extreme", Width: 34, Height: 38, QP: qp, GOP: 4, Frames: 7, ModeSearch: search})
		}
		for _, kind := range []string{"flat", "static", "mixed", "chroma", "local", "recovery", "checker"} {
			for _, qp := range []int{0, 24, 51} {
				out = append(out, predictiveVector{Kind: kind, Width: 50, Height: 34, QP: qp, GOP: 33, Frames: 40, ModeSearch: search})
			}
		}
		out = append(out, predictiveVector{Kind: "local", Width: 34, Height: 38, QP: 24, GOP: 1, Frames: 6, ModeSearch: search}, predictiveVector{Kind: "flat", Width: 2, Height: 2, QP: 24, GOP: 65536, Frames: 70, ModeSearch: search})
	}
	// Integration default: two complete 60-picture GOPs plus a third IDR.
	// Include three frame_num wraps before every periodic IDR, odd MB grids,
	// changing detail, exact-background skips, and QP endpoint chroma behavior.
	for _, search := range []bool{false, true} {
		for _, dim := range [][2]int{{2, 2}, {34, 38}, {320, 180}} {
			for _, kind := range []string{"flat", "local", "mixed"} {
				out = append(out, predictiveVector{Kind: kind, Width: dim[0], Height: dim[1], QP: 24, GOP: 60, Frames: 125, ModeSearch: search})
			}
		}
		for _, qp := range []int{0, 51} {
			for _, kind := range []string{"local", "chroma"} {
				out = append(out, predictiveVector{Kind: kind, Width: 34, Height: 38, QP: qp, GOP: 60, Frames: 125, ModeSearch: search})
			}
		}
	}
	for i := range out {
		v := &out[i]
		v.Name = fmt.Sprintf("%s_%dx%d_q%d_g%d_n%d_s%t", v.Kind, v.Width, v.Height, v.QP, v.GOP, v.Frames, v.ModeSearch)
	}
	return out
}

// Dump mode only writes reference artifacts. External decoder execution is a
// separate development action; normal tests need no decoder or subprocess.
func TestPredictiveReferenceVectors(t *testing.T) {
	dump := os.Getenv("GOMATH_H264_PREDICTIVE_DUMP_DIR")
	vectors := predictiveVectors()
	fixture := filepath.Join("../../tests/data/internal/h264", "predictive_vectors.json")
	if dump == "" {
		data, err := os.ReadFile(fixture)
		if err != nil {
			t.Fatal(err)
		}
		if err = json.Unmarshal(data, &vectors); err != nil || len(vectors) != 206 {
			t.Fatal("invalid verified fixtures", err, len(vectors))
		}
	} else if err := os.MkdirAll(dump, 0755); err != nil {
		t.Fatal(err)
	}
	for i := range vectors {
		v := &vectors[i]
		t.Run(v.Name, func(t *testing.T) {
			e, err := NewPredictive(v.Width, v.Height, 24, PredictiveOptions{QP: v.QP, ModeSearch: v.ModeSearch, GOP: v.GOP})
			if err != nil {
				t.Fatal(err)
			}
			stream := annexB(e.SPS(), e.PPS())
			var rebuilt, source []byte
			var decoded picture420
			position, totalSkipped := 0, 0
			for f := range v.Frames {
				if v.Kind == "recovery" {
					if f == 4 || f == 5 || f == 17 {
						e.ForceIDR()
						position = 0
					}
					if f == 12 {
						e.Reset()
						position = 0
					}
					if f == 4 || f == 12 {
						if _, err = e.Encode(nil); err == nil {
							t.Fatal("invalid frame consumed")
						}
					}
				}
				im := predictiveImage(v.Kind, v.Width, v.Height, f)
				out, err := e.Encode(im)
				if err != nil {
					t.Fatal(err)
				}
				key := position == 0 || position == v.GOP
				if key {
					position = 0
				}
				if out.Keyframe != key || out.FrameNum != uint8(position%16) || out.POC != position*2 {
					t.Fatal("sequence metadata", f, position, out)
				}
				position++
				rec, skips := decodePredictive(t, out, decoded, v.Width, v.Height, v.QP)
				src := makePicture420(im, v.Width, v.Height, rec.stride, (v.Height+15)/16*16)
				skipped := 0
				for mb, skip := range skips {
					if skip {
						skipped++
					}
					if !out.Keyframe && skip != equalMacroblock(src, decoded, mb%(rec.stride/16)*16, mb/(rec.stride/16)*16) {
						t.Fatal("skip decision did not compare reference reconstruction")
					}
				}
				if skipped != out.SkippedMacroblocks {
					t.Fatal("skip metadata")
				}
				for c := range 3 {
					if !bytes.Equal(rec.planes[c], e.reference.planes[c]) {
						t.Fatal("syntax/reference mismatch", f, c)
					}
				}
				decoded = rec
				totalSkipped += skipped
				stream = append(stream, annexB(out.NAL)...)
				rebuilt = append(rebuilt, cropped420(rec, v.Width, v.Height)...)
				source = append(source, cropped420(src, v.Width, v.Height)...)
			}
			if dump != "" {
				for ext, data := range map[string][]byte{".h264": stream, ".rec.yuv": rebuilt, ".src.yuv": source} {
					if err := os.WriteFile(filepath.Join(dump, v.Name+ext), data, 0644); err != nil {
						t.Fatal(err)
					}
				}
				v.BitstreamSHA256, v.DecodedSHA256 = digest(stream), digest(rebuilt)
				v.Bytes, v.Skipped = len(stream), totalSkipped
			} else if digest(stream) != v.BitstreamSHA256 || digest(rebuilt) != v.DecodedSHA256 || len(stream) != v.Bytes || totalSkipped != v.Skipped {
				t.Fatal("independently decoded vector mismatch")
			}
		})
	}
	if dump != "" {
		data, _ := json.MarshalIndent(vectors, "", "  ")
		if err := os.WriteFile(filepath.Join(dump, "predictive_vectors.json"), data, 0644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPredictiveValidationAndRecovery(t *testing.T) {
	for _, o := range []PredictiveOptions{{QP: -1, GOP: 3}, {QP: 52, GOP: 3}, {QP: 24}, {QP: 24, GOP: -1}, {QP: 24, GOP: 65537}} {
		if _, err := NewPredictive(16, 16, 24, o); err == nil {
			t.Fatal("invalid options", o)
		}
	}
	if _, err := NewPredictive(17, 16, 24, PredictiveOptions{GOP: 3}); err == nil {
		t.Fatal("invalid size")
	}
	e, _ := NewPredictive(50, 34, 24, PredictiveOptions{QP: 24, GOP: 65536, ModeSearch: true})
	control, _ := NewPredictive(50, 34, 24, PredictiveOptions{QP: 24, GOP: 65536, ModeSearch: true})
	for f := range 35 {
		if f == 3 {
			e.ForceIDR()
			control.ForceIDR()
		}
		if f == 18 {
			e.Reset()
			control.Reset()
		}
		for _, bad := range []*image.RGBA{nil, image.NewRGBA(image.Rect(0, 0, 16, 16)), {Rect: image.Rect(0, 0, 50, 34), Stride: 199, Pix: make([]byte, 6800)}, {Rect: image.Rect(0, 0, 50, 34), Stride: int(^uint(0) >> 1), Pix: make([]byte, 6800)}} {
			if got, err := e.Encode(bad); err == nil || len(got.NAL) != 0 {
				t.Fatal("invalid image changed state")
			}
		}
		im := predictiveImage("local", 50, 34, f)
		a, err := e.Encode(im)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := control.Encode(im)
		if !bytes.Equal(a.NAL, b.NAL) || a.Keyframe != b.Keyframe || a.POC != b.POC {
			t.Fatal("error consumed state")
		}
	}
	r := reader{p: unescape(e.SPS())}
	r.uint(24)
	r.ue()
	if r.ue() != 0 || r.ue() != 2 || r.ue() != 1 || r.bit() != 0 {
		t.Fatal("single-reference SPS")
	}
	old, _ := NewIntra(50, 34, 24, IntraOptions{QP: 24, ModeSearch: true})
	if !bytes.Equal(e.PPS(), old.PPS()) {
		t.Fatal("intra PPS changed")
	}
}

// Failed frames immediately before/at/after GOP60 boundaries must not shift
// the advertised sync sample positions or the reference-picture sequence.
func TestPredictiveGOP60ErrorBoundary(t *testing.T) {
	for _, search := range []bool{false, true} {
		opts := PredictiveOptions{QP: 24, ModeSearch: search, GOP: 60}
		e, _ := NewPredictive(34, 38, 24, opts)
		control, _ := NewPredictive(34, 38, 24, opts)
		for f := range 125 {
			if f == 59 || f == 60 || f == 61 || f == 119 || f == 120 || f == 121 {
				if out, err := e.Encode(nil); err == nil || len(out.NAL) != 0 {
					t.Fatal("failed picture consumed")
				}
			}
			im := predictiveImage("local", 34, 38, f)
			got, err := e.Encode(im)
			if err != nil {
				t.Fatal(err)
			}
			want, err := control.Encode(im)
			if err != nil {
				t.Fatal(err)
			}
			if got.Keyframe != (f%60 == 0) || got.FrameNum != uint8(f%60%16) || got.POC != 2*(f%60) || !bytes.Equal(got.NAL, want.NAL) {
				t.Fatal("GOP60 position shifted", search, f, got.Keyframe, got.FrameNum, got.POC)
			}
		}
	}
}

func TestPredictiveConcurrentSerialization(t *testing.T) {
	e, _ := NewPredictive(16, 16, 24, PredictiveOptions{QP: 24, GOP: 100})
	im := predictiveImage("flat", 16, 16, 0)
	var wg sync.WaitGroup
	out := make([]PredictiveFrame, 40)
	for i := range out {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var err error
			out[i], err = e.Encode(im)
			if err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	seen := make(map[int]bool)
	for _, f := range out {
		if seen[f.POC] || f.FrameNum != uint8(f.POC/2%16) || f.Keyframe != (f.POC == 0) {
			t.Fatal("lost/out-of-sequence state", f.POC)
		}
		seen[f.POC] = true
	}
}

func TestPredictiveSkipBoundaries(t *testing.T) {
	for _, search := range []bool{false, true} {
		e, _ := NewPredictive(50, 34, 24, PredictiveOptions{QP: 24, GOP: 100, ModeSearch: search})
		im := predictiveImage("flat", 50, 34, 0)
		if _, err := e.Encode(im); err != nil {
			t.Fatal(err)
		}
		out, _ := e.Encode(im)
		if out.SkippedMacroblocks != 12 || len(out.NAL) > 8 {
			t.Fatal("flat not truly skipped", out)
		}
		// All 12 input/reference macroblocks are exact. Change each raster index
		// in turn; this covers leading, interior, trailing, and zero skip runs.
		for mb := range 12 {
			e.Reset()
			e.Encode(im)
			changed := predictiveImage("flat", 50, 34, 0)
			x, y := mb%4*16, mb/4*16
			changed.SetRGBA(x-2, y+3, color.RGBA{0, 0, 0, 255})
			out, _ = e.Encode(changed)
			_, skips := decodePredictive(t, out, equalReference128(64, 48), 50, 34, 24)
			if skips[mb] || out.SkippedMacroblocks != 11 {
				t.Fatal("wrong skip map", mb, out.SkippedMacroblocks)
			}
		}
		// A lossy repeated input is not a license to skip its changed samples.
		e.Reset()
		im = intraImage(50, 34, 0)
		e.Encode(im)
		out, _ = e.Encode(im)
		if out.SkippedMacroblocks == 12 {
			t.Fatal("skipped original instead of reconstruction")
		}
	}
	// Chroma equality and padded (not only visible) samples are mandatory.
	a, b := equalReference128(32, 32), equalReference128(32, 32)
	for c := range 3 {
		b.planes[c][len(b.planes[c])-1]++
		if equalMacroblock(a, b, 16, 16) {
			t.Fatal("ignored padded/chroma sample", c)
		}
		b.planes[c][len(b.planes[c])-1]--
	}
}

func equalReference128(w, h int) picture420 {
	p := newPicture420(w, h)
	for c := range 3 {
		for i := range p.planes[c] {
			p.planes[c][i] = 128
		}
	}
	return p
}

func TestPredictiveMeasurements(t *testing.T) {
	for _, kind := range []string{"flat", "local", "static", "mixed"} {
		const width, height, frames = 320, 180, 33
		for _, search := range []bool{false, true} {
			var size [2]int
			var squared [2]float64
			var totalSamples int
			skipped := 0
			for variant, gop := range []int{1, 33} {
				e, _ := NewPredictive(width, height, 24, PredictiveOptions{QP: 24, GOP: gop, ModeSearch: search})
				size[variant] = len(annexB(e.SPS(), e.PPS()))
				for f := range frames {
					im := predictiveImage(kind, width, height, f)
					out, _ := e.Encode(im)
					size[variant] += len(out.NAL) + 4
					if variant == 1 {
						skipped += out.SkippedMacroblocks
					}
					src := cropped420(makePicture420(im, width, height, 320, 192), width, height)
					rec := cropped420(e.reference, width, height)
					for i, v := range src {
						d := float64(v) - float64(rec[i])
						squared[variant] += d * d
					}
					totalSamples = len(src) * frames
				}
			}
			psnr := func(s float64) float64 { return 10 * math.Log10(255*255*float64(totalSamples)/s) }
			t.Logf("%s search=%t QP24 %dframes: allIDR=%dB %.3fdB predictive=%dB %.3fdB skip=%d/%d", kind, search, frames, size[0], psnr(squared[0]), size[1], psnr(squared[1]), skipped, 32*240)
		}
	}
}

func BenchmarkPredictive(b *testing.B) {
	for _, kind := range []string{"flat", "local", "static", "mixed"} {
		for _, search := range []bool{false, true} {
			for _, gop := range []int{1, 33} {
				b.Run(fmt.Sprintf("%s/search%t/gop%d", kind, search, gop), func(b *testing.B) {
					images := make([]*image.RGBA, 33)
					for f := range images {
						images[f] = predictiveImage(kind, 320, 180, f)
					}
					e, _ := NewPredictive(320, 180, 24, PredictiveOptions{QP: 24, GOP: gop, ModeSearch: search})
					b.ReportAllocs()
					b.ResetTimer()
					for range b.N {
						for _, im := range images {
							if _, err := e.Encode(im); err != nil {
								b.Fatal(err)
							}
						}
					}
					b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*33), "ns/frame")
				})
			}
		}
	}
}
