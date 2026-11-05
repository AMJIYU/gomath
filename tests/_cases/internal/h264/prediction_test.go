package h264

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"math"
	"sync"
	"testing"
)

func modeSearchImage(kind string, width, height, frame int) *image.RGBA {
	if kind == "mixed" {
		return intraImage(width, height, frame)
	}
	im := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			v := byte((x*37 + frame*13) % 256)
			if kind == "horizontal" {
				v = byte((y*37 + frame*13) % 256)
			}
			c := color.RGBA{v, v, v, 255}
			if kind == "gradient" {
				c = color.RGBA{byte(x * 255 / max(1, width-1)), byte(y * 255 / max(1, height-1)), byte((x + y + frame) * 255 / max(1, width+height-2)), 255}
			}
			im.SetRGBA(x, y, c)
		}
	}
	return im
}

func TestIntraModeSearchReferenceVectors(t *testing.T) {
	checkIntraReferenceVectors(t, true)
}

func TestIntraModePredictionAndSignaling(t *testing.T) {
	modes := []uint8{0, 1, 2, 1, 0, 2, 2, 2, 2}
	for _, c := range []struct {
		x, y      int
		predicted uint8
	}{{0, 0, 2}, {1, 0, 2}, {0, 1, 2}, {1, 1, 1}, {2, 1, 0}, {1, 2, 0}} {
		if got := predictedIntra4x4(modes, 3, c.x, c.y); got != c.predicted {
			t.Fatal("incorrect neighbor mode", c, got)
		}
	}
	for predicted := uint8(0); predicted < 9; predicted++ {
		for mode := uint8(0); mode < 9; mode++ {
			var w bits
			writeIntra4x4Mode(&w, predicted, mode)
			r := reader{p: w.data}
			got := predicted
			if r.bit() == 0 {
				got = uint8(r.uint(3))
				if got >= predicted {
					got++
				}
			}
			if got != mode || predicted == mode && r.pos != 1 || predicted != mode && r.pos != 4 {
				t.Fatal("mode syntax does not round trip", predicted, mode, got, r.pos)
			}
		}
	}
}

func TestIntraDirectionalChoices(t *testing.T) {
	const stride = 16
	for _, want := range []uint8{intraVertical, intraHorizontal, intraDC} {
		src, rec := make([]byte, stride*stride), make([]byte, stride*stride)
		for i, v := range []byte{10, 80, 170, 240} {
			rec[3*stride+4+i] = v
			rec[(4+i)*stride+3] = 128
			if want == intraHorizontal {
				rec[(4+i)*stride+3] = v
			}
			if want == intraDC {
				rec[3*stride+4+i] = 128
			}
		}
		for j := range 4 {
			for i := range 4 {
				v := rec[3*stride+4+i]
				if want == intraHorizontal {
					v = rec[(4+j)*stride+3]
				}
				src[(4+j)*stride+4+i] = v
			}
		}
		choice := chooseLuma4x4(src, rec, stride, 4, 4, 24, 0, intraDC, make([]byte, 128))
		if choice.mode != want || choice.total != 0 {
			t.Fatal("directional predictor not selected", want, choice.mode, choice.total)
		}
	}
	// At the top-left, neither directional predictor is legal.
	choice := chooseLuma4x4(make([]byte, 256), make([]byte, 256), 16, 0, 0, 24, 0, intraDC, make([]byte, 128))
	if choice.mode != intraDC {
		t.Fatal("unavailable reference was used")
	}
}

// Parse the actual slice header, mode flags/rem values and all CAVLC blocks.
// Reconstruct luma from the decoded modes rather than encoder-side choices.
// This checks cross-macroblock mode state, context staging and syntax order.
func parseIntraLuma(t *testing.T, nal []byte, width, height, qp int) ([]byte, [3]int, [2]int) {
	t.Helper()
	r := reader{p: unescape(nal)}
	if r.ue() != 0 || r.ue() != 2 || r.ue() != 0 {
		t.Fatal("invalid slice prefix")
	}
	r.uint(4)
	r.ue()
	r.bit()
	r.bit()
	r.ue() // slice QP delta (already supplied by test)
	if r.ue() != 1 {
		t.Fatal("deblocking unexpectedly enabled")
	}
	pw, ph := (width+15)/16*16, (height+15)/16*16
	rec := make([]byte, pw*ph)
	modes := make([]uint8, pw*ph/16)
	counts := [3][]uint8{make([]uint8, pw*ph/16), make([]uint8, pw*ph/64), make([]uint8, pw*ph/64)}
	var used [3]int
	var flags [2]int
	for my := 0; my < ph; my += 16 {
		for mx := 0; mx < pw; mx += 16 {
			if r.ue() != 0 {
				t.Fatal("macroblock is not I4x4")
			}
			for b := range 16 {
				x, y := mx+(b/4%2)*8+(b%2)*4, my+(b/8)*8+(b/2%2)*4
				bx, by := x/4, y/4
				mode := uint8(2)
				if x != 0 && y != 0 {
					a, b := modes[by*(pw/4)+bx-1], modes[(by-1)*(pw/4)+bx]
					if a < b {
						mode = a
					} else {
						mode = b
					}
				}
				flag := r.bit()
				flags[flag]++
				if flag == 0 {
					rem := uint8(r.uint(3))
					if rem >= mode {
						rem++
					}
					mode = rem
				}
				if mode > 2 || mode == 0 && y == 0 || mode == 1 && x == 0 {
					t.Fatal("illegal/unavailable prediction mode", x, y, mode)
				}
				modes[by*(pw/4)+bx] = mode
				used[mode]++
			}
			if r.ue() != 0 || r.ue() != 0 || r.ue() != 0 {
				t.Fatal("unexpected chroma mode/CBP/QP delta")
			}
			for b := range 16 {
				x, y := mx+(b/4%2)*8+(b%2)*4, my+(b/8)*8+(b/2%2)*4
				bx, by := x/4, y/4
				nc := neighborCount(counts[0], pw/4, bx, by)
				scan := readCAVLC(&r, 16, nc)
				var q [16]int
				for i, pos := range frameScan {
					q[pos] = scan[i]
					if scan[i] != 0 {
						counts[0][by*(pw/4)+bx]++
					}
				}
				residual := inverse4(dequant4(q, qp))
				mode := modes[by*(pw/4)+bx]
				dc := lumaDC(rec, pw, x, y)
				for j := range 4 {
					for i := range 4 {
						pred := dc
						if mode == 0 {
							pred = int(rec[(y-1)*pw+x+i])
						} else if mode == 1 {
							pred = int(rec[(y+j)*pw+x-1])
						}
						rec[(y+j)*pw+x+i] = byte(max(0, min(255, pred+residual[j*4+i])))
					}
				}
			}
			readCAVLC(&r, 4, -1)
			readCAVLC(&r, 4, -1)
			for c := 1; c < 3; c++ {
				for b := range 4 {
					x, y := mx/8+b%2, my/8+b/2
					scan := readCAVLC(&r, 15, neighborCount(counts[c], pw/8, x, y))
					for _, v := range scan {
						if v != 0 {
							counts[c][y*(pw/8)+x]++
						}
					}
				}
			}
		}
	}
	if r.bit() != 1 {
		t.Fatal("missing rbsp_stop_one_bit")
	}
	for r.pos < len(r.p)*8 {
		if r.bit() != 0 {
			t.Fatal("unexpected data after slice")
		}
	}
	return rec, used, flags
}

func TestIntraModeSearchSliceSyntax(t *testing.T) {
	for _, kind := range []string{"mixed", "vertical", "horizontal", "gradient"} {
		for _, qp := range []int{0, 3, 12, 24, 36, 51} {
			t.Run(fmt.Sprintf("%s/QP%d", kind, qp), func(t *testing.T) {
				const width, height = 50, 38
				e, _ := NewIntra(width, height, 24, IntraOptions{QP: qp, ModeSearch: true})
				n, rec, err := e.encode(modeSearchImage(kind, width, height, 0))
				if err != nil {
					t.Fatal(err)
				}
				got, modes, flags := parseIntraLuma(t, n, width, height, qp)
				if !bytes.Equal(got, rec.planes[0]) {
					t.Fatal("decoded mode syntax does not reproduce luma")
				}
				if modes[2] == 0 || flags[1] == 0 {
					t.Fatal("mode flags did not exercise DC and predicted paths", modes, flags)
				}
				if kind == "vertical" && (modes[0] == 0 || flags[0] == 0) || kind == "horizontal" && (modes[1] == 0 || flags[0] == 0) {
					t.Fatal("directional modes were never signalled", modes)
				}
			})
		}
	}
}

func TestIntraModeSearchConcurrentAndDefault(t *testing.T) {
	im := intraImage(48, 34, 0)
	a, _ := NewIntra(48, 34, 24, IntraOptions{QP: 24})
	b, _ := NewIntra(48, 34, 24, IntraOptions{QP: 24, ModeSearch: false})
	x, _ := a.Encode(im)
	y, _ := b.Encode(im)
	if !bytes.Equal(x, y) {
		t.Fatal("explicit false differs from default")
	}
	e, _ := NewIntra(48, 34, 24, IntraOptions{QP: 24, ModeSearch: true})
	if !bytes.Equal(e.SPS(), a.SPS()) || !bytes.Equal(e.PPS(), a.PPS()) {
		t.Fatal("mode search changes parameter sets")
	}
	want, _ := e.Encode(im)
	before := append([]byte(nil), im.Pix...)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			got, err := e.Encode(im)
			if err != nil || !bytes.Equal(got, want) {
				t.Error("search has mutable shared state", err)
			}
		})
	}
	wg.Wait()
	if !bytes.Equal(im.Pix, before) {
		t.Fatal("search modified input pixels")
	}
}

func TestIntraModeSearchComparison(t *testing.T) {
	for _, kind := range []string{"mixed", "vertical", "horizontal", "gradient"} {
		for _, qp := range []int{12, 24, 36} {
			im := modeSearchImage(kind, 320, 180, 0)
			var sizes [2]int
			var psnr [2]float64
			for i, search := range []bool{false, true} {
				e, _ := NewIntra(320, 180, 24, IntraOptions{QP: qp, ModeSearch: search})
				n, rec, err := e.encode(im)
				if err != nil {
					t.Fatal(err)
				}
				src := cropped420(makePicture420(im, 320, 180, rec.stride, 192), 320, 180)
				got := cropped420(rec, 320, 180)
				var sse float64
				for j, v := range got {
					d := float64(int(v) - int(src[j]))
					sse += d * d
				}
				sizes[i] = len(n)
				psnr[i] = 10 * math.Log10(255*255*float64(len(got))/sse)
			}
			t.Logf("%s QP%d bytes=%d/%d PSNR=%.3f/%.3f dB (DC/search)", kind, qp, sizes[0], sizes[1], psnr[0], psnr[1])
			if (kind == "vertical" || kind == "horizontal") && (sizes[1] >= sizes[0] || psnr[1] < psnr[0]-0.1) {
				t.Fatal("repeated-row/column input did not benefit from directional prediction")
			}
		}
	}
}

func BenchmarkIntraModeSearch(b *testing.B) {
	for _, c := range []struct {
		kind          string
		width, height int
	}{{"mixed", 320, 180}, {"vertical", 320, 180}, {"horizontal", 320, 180}, {"gradient", 320, 180}, {"mixed", 1280, 720}} {
		for _, search := range []bool{false, true} {
			b.Run(fmt.Sprintf("%s/%dx%d/search=%t", c.kind, c.width, c.height, search), func(b *testing.B) {
				e, _ := NewIntra(c.width, c.height, 24, IntraOptions{QP: 24, ModeSearch: search})
				im := modeSearchImage(c.kind, c.width, c.height, 0)
				b.ReportAllocs()
				b.ResetTimer()
				for range b.N {
					n, err := e.Encode(im)
					if err != nil {
						b.Fatal(err)
					}
					b.ReportMetric(float64(len(n)), "bytes/frame")
				}
			})
		}
	}
}
