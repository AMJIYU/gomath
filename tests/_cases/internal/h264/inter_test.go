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

type interDecodeStats struct {
	Inter, Intra                      int
	CBP                               [48]int
	Moved, OddChroma, MVD, MovingSkip int
}

// Independent Table 9-4(a) decode mapping; do not use the encoder's inverse.
var normativeInterCBP = [48]uint8{0, 16, 1, 2, 4, 8, 32, 3, 5, 10, 12, 15, 47, 7, 11, 13, 14, 6, 9, 31, 35, 37, 42, 44, 33, 34, 36, 40, 39, 43, 45, 46, 17, 18, 20, 24, 19, 21, 26, 28, 23, 27, 29, 30, 22, 25, 38, 41}

func decodeInterMB(t *testing.T, r *reader, ref, rec picture420, counts [3][]uint8, mx, my, qp int) uint8 {
	t.Helper()
	if ref.planes[0] == nil || r.ue() != 0 || r.ue() != 0 {
		t.Fatal("missing reference/nonzero MVD")
	}
	return decodeInterResidual(t, r, ref, rec, counts, mx, my, qp)
}

func decodeInterResidual(t *testing.T, r *reader, ref, rec picture420, counts [3][]uint8, mx, my, qp int) uint8 {
	t.Helper()
	code := r.ue()
	if code >= 48 {
		t.Fatal("invalid inter CBP", code)
	}
	cbp := normativeInterCBP[code]
	if cbp != 0 && r.ue() != 0 {
		t.Fatal("inter QP delta")
	}
	copyMacroblock(rec, ref, mx, my)
	pw := rec.stride
	for b := range 16 {
		if cbp&(1<<(b/4)) == 0 {
			continue
		}
		x, y := mx+(b/4%2)*8+b%2*4, my+b/8*8+(b/2%2)*4
		scan := readCAVLC(r, 16, neighborCount(counts[0], pw/4, x/4, y/4))
		var q [16]int
		for i, pos := range frameScan {
			q[pos] = scan[i]
			if scan[i] != 0 {
				counts[0][y/4*(pw/4)+x/4]++
			}
		}
		v := inverse4(dequant4(q, qp))
		for j := range 4 {
			for i := range 4 {
				p := (y+j)*pw + x + i
				rec.planes[0][p] = byte(max(0, min(255, int(ref.planes[0][p])+v[j*4+i])))
			}
		}
	}
	var dc [2][4]int
	if cbp/16 != 0 {
		for c := range 2 {
			copy(dc[c][:], readCAVLC(r, 4, -1))
			dc[c] = hadamard2(dc[c])
		}
	}
	cq, stride := chromaQP(max(4, qp)), pw/2
	for c := 1; c < 3; c++ {
		for b := range 4 {
			x, y := mx/2+b%2*4, my/2+b/2*4
			var q [16]int
			if cbp/16 == 2 {
				scan := readCAVLC(r, 15, neighborCount(counts[c], pw/8, x/4, y/4))
				for i, pos := range frameScan[1:] {
					q[pos] = scan[i]
					if scan[i] != 0 {
						counts[c][y/4*(pw/8)+x/4]++
					}
				}
			}
			v := dequant4(q, cq)
			v[0] = dc[c-1][b] * (inverseScale[cq%6][0] << (cq / 6)) >> 1
			v = inverse4(v)
			for j := range 4 {
				for i := range 4 {
					p := (y+j)*stride + x + i
					rec.planes[c][p] = byte(max(0, min(255, int(ref.planes[c][p])+v[j*4+i])))
				}
			}
		}
	}
	return cbp
}

func interImage(kind string, w, h, f int) *image.RGBA {
	if kind == "drift" || kind == "noisechange" {
		im := intraImage(w, h, 0)
		if kind == "noisechange" {
			im = intraImage(w, h, f)
		}
		if kind == "drift" && f%5 != 0 {
			for y := range h {
				for x := range w {
					p := im.PixOffset(im.Rect.Min.X+x, im.Rect.Min.Y+y)
					for c := range 3 {
						im.Pix[p+c] = byte(min(255, int(im.Pix[p+c])+f%5))
					}
				}
			}
		}
		return im
	}
	if kind == "chromaedge" {
		im := image.NewRGBA(image.Rect(0, 0, w, h))
		colors := [...]color.RGBA{{255, 0, 0, 255}, {0, 130, 0, 255}, {0, 0, 255, 255}, {0, 50, 0, 255}, {255, 255, 0, 255}, {0, 255, 255, 255}}
		for y := range h {
			for x := range w {
				i := (x/4 + y/4) % len(colors)
				if x/16 == f%max(1, (w+15)/16) {
					i = (i + f) % len(colors)
				}
				im.SetRGBA(x, y, colors[i])
			}
		}
		return im
	}
	return predictiveImage(kind, w, h, f)
}

func interVectors() []predictiveVector {
	var out []predictiveVector
	for _, search := range []bool{false, true} {
		for qp := range 52 {
			for _, kind := range []string{"static", "extreme", "chromaedge"} {
				out = append(out, predictiveVector{Kind: kind, Width: 34, Height: 38, QP: qp, GOP: 4, Frames: 9, ModeSearch: search})
			}
		}
		for _, dim := range [][2]int{{2, 2}, {18, 22}, {50, 34}, {320, 180}} {
			for _, qp := range []int{0, 24, 51} {
				for _, kind := range []string{"local", "drift", "checker", "chroma"} {
					out = append(out, predictiveVector{Kind: kind, Width: dim[0], Height: dim[1], QP: qp, GOP: 17, Frames: 20, ModeSearch: search})
				}
			}
		}
		for _, kind := range []string{"static", "drift", "chromaedge", "noisechange", "recovery", "flat"} {
			out = append(out, predictiveVector{Kind: kind, Width: 50, Height: 34, QP: 24, GOP: 60, Frames: 125, ModeSearch: search})
		}
	}
	// The exact fixed-size measurement groups also receive independent decode.
	for _, search := range []bool{false, true} {
		for _, kind := range []string{"static", "drift", "local", "noisechange"} {
			out = append(out, predictiveVector{Kind: kind, Width: 320, Height: 180, QP: 24, GOP: 60, Frames: 60, ModeSearch: search})
		}
	}
	for i := range out {
		v := &out[i]
		v.Name = fmt.Sprintf("%s_%dx%d_q%d_g%d_n%d_s%t", v.Kind, v.Width, v.Height, v.QP, v.GOP, v.Frames, v.ModeSearch)
	}
	return out
}

func TestInterReferenceVectors(t *testing.T) {
	dump := os.Getenv("GOMATH_H264_INTER_DUMP_DIR")
	vectors := interVectors()
	if dump == "" {
		data, err := os.ReadFile("../../tests/data/internal/h264/inter_vectors.json")
		if err != nil {
			t.Fatal(err)
		}
		if err = json.Unmarshal(data, &vectors); err != nil || len(vectors) != 428 {
			t.Fatal("invalid inter fixtures", err, len(vectors))
		}
	} else if err := os.MkdirAll(dump, 0755); err != nil {
		t.Fatal(err)
	}
	var all interDecodeStats
	var nonExactSkips int
	for i := range vectors {
		v := &vectors[i]
		t.Run(v.Name, func(t *testing.T) {
			e, err := NewPredictive(v.Width, v.Height, 24, PredictiveOptions{QP: v.QP, ModeSearch: v.ModeSearch, GOP: v.GOP, InterResidual: true})
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
					if f == 4 || f == 12 || f == 59 || f == 60 {
						if _, err = e.Encode(nil); err == nil {
							t.Fatal("invalid picture consumed")
						}
					}
				}
				im := interImage(v.Kind, v.Width, v.Height, f)
				out, err := e.Encode(im)
				if err != nil {
					t.Fatal(err)
				}
				key := position == 0 || position == v.GOP
				if key {
					position = 0
				}
				if out.Keyframe != key || out.FrameNum != uint8(position%16) || out.POC != 2*position {
					t.Fatal("inter sequence state")
				}
				position++
				rec, skips := decodePredictiveWithStats(t, out, decoded, v.Width, v.Height, v.QP, &all)
				src := makePicture420(im, v.Width, v.Height, rec.stride, (v.Height+15)/16*16)
				for c := range 3 {
					if !bytes.Equal(rec.planes[c], e.reference.planes[c]) {
						t.Fatal("inter decoded reconstruction mismatch", f, c)
					}
				}
				skipped := 0
				for mb, skip := range skips {
					if !skip {
						continue
					}
					skipped++
					x, y := mb%(rec.stride/16)*16, mb/(rec.stride/16)*16
					if !equalMacroblock(src, decoded, x, y) {
						nonExactSkips++
						if q := makeInterMB(src, decoded, x, y, v.QP); q.cbp != 0 {
							t.Fatal("nonzero residual was discarded for skip")
						}
					}
				}
				if skipped != out.SkippedMacroblocks {
					t.Fatal("inter skip metadata")
				}
				totalSkipped += skipped
				decoded = rec
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
				t.Fatal("independently decoded inter vector mismatch")
			}
		})
	}
	if all.Inter == 0 || all.Intra == 0 || nonExactSkips == 0 {
		t.Fatal("inter/intra/quantized-skip path not exercised", all, nonExactSkips)
	}
	for cbp := 1; cbp < 48; cbp++ {
		if all.CBP[cbp] == 0 {
			t.Fatal("independent matrix did not exercise inter CBP", cbp)
		}
	}
	t.Logf("inter=%d intra=%d non-exact-skips=%d CBP=%v", all.Inter, all.Intra, nonExactSkips, all.CBP)
	if dump != "" {
		data, _ := json.MarshalIndent(vectors, "", "  ")
		if err := os.WriteFile(filepath.Join(dump, "inter_vectors.json"), data, 0644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestInterResidualCBPAndContexts(t *testing.T) {
	// Every legal CBP, including zero, sparse 8x8 luma, DC-only chroma and
	// chroma AC. Follow inter with intra in the full vector tests above.
	for cbp := range 48 {
		var mb interMB
		mb.cbp = uint8(cbp)
		for b := range 16 {
			if cbp&(1<<(b/4)) != 0 {
				mb.luma[b][b%16] = 1
			}
		}
		if cbp/16 > 0 {
			mb.dc[0][0], mb.dc[1][3] = 2, -1
		}
		if cbp/16 == 2 {
			mb.ac[0][0][1], mb.ac[1][3][15] = 3, -2
		}
		counts := [3][]uint8{make([]uint8, 128), make([]uint8, 32), make([]uint8, 32)}
		var w bits
		writeInterMB(&w, &mb, counts, 64, 16, 16)
		r := reader{p: w.data}
		if r.ue() != 0 {
			t.Fatal("not P_L0_16x16")
		}
		decodedCounts := [3][]uint8{make([]uint8, 128), make([]uint8, 32), make([]uint8, 32)}
		got := decodeInterMB(t, &r, equalReference128(64, 32), newPicture420(64, 32), decodedCounts, 16, 16, 24)
		if got != uint8(cbp) || r.pos != bitLength(&w) {
			t.Fatal("CBP syntax length", cbp, got, r.pos, bitLength(&w))
		}
		for c := range 3 {
			if !bytes.Equal(counts[c], decodedCounts[c]) {
				t.Fatal("CBP counts", cbp, c)
			}
		}
	}
	// An aligned and every unaligned concatenation must preserve exact syntax.
	for prefix := range 8 {
		for n := range 65 {
			var a, b bits
			for i := range prefix {
				a.bit(uint32(i))
			}
			for i := range n {
				b.bit(uint32(i / 3))
			}
			appendBits(&a, &b)
			if bitLength(&a) != prefix+n {
				t.Fatal("append length")
			}
			r := reader{p: a.data}
			r.uint(prefix)
			for i := range n {
				if r.bit() != uint32(i/3)&1 {
					t.Fatal("append content")
				}
			}
		}
	}
	for n := range 1000 {
		var w bits
		w.ue(uint32(n))
		if ueLength(n) != bitLength(&w) {
			t.Fatal("skip run cost")
		}
	}
}

func TestInterStateAndParameterPolicy(t *testing.T) {
	for _, search := range []bool{false, true} {
		for qp := range 52 {
			opts := PredictiveOptions{QP: qp, ModeSearch: search, GOP: 3, InterResidual: true}
			e, _ := NewPredictive(18, 22, 24, opts)
			control, _ := NewPredictive(18, 22, 24, opts)
			old, _ := NewPredictive(18, 22, 24, PredictiveOptions{QP: qp, ModeSearch: search, GOP: 3})
			if !bytes.Equal(e.SPS(), old.SPS()) || !bytes.Equal(e.PPS(), old.PPS()) {
				t.Fatal("InterResidual changed parameter policy")
			}
			for f := range 8 {
				if f == 2 {
					e.ForceIDR()
					control.ForceIDR()
				}
				if f == 5 {
					e.Reset()
					control.Reset()
				}
				if _, err := e.Encode(nil); err == nil {
					t.Fatal("bad frame consumed")
				}
				im := interImage("drift", 18, 22, f)
				a, err := e.Encode(im)
				if err != nil {
					t.Fatal(err)
				}
				b, _ := control.Encode(im)
				if !bytes.Equal(a.NAL, b.NAL) || a.Keyframe != b.Keyframe || a.POC != b.POC {
					t.Fatal("inter error advanced state")
				}
			}
		}
	}
}

// Concurrent identical inputs must serialize to the same sequence as ordered
// calls, including after frame_num wrap; trial buffers must never be shared.
func TestInterConcurrentSerialization(t *testing.T) {
	opts := PredictiveOptions{QP: 24, GOP: 60, ModeSearch: true, InterResidual: true}
	e, _ := NewPredictive(18, 22, 24, opts)
	control, _ := NewPredictive(18, 22, 24, opts)
	im := interImage("static", 18, 22, 0)
	outputs := make([]PredictiveFrame, 40)
	var wg sync.WaitGroup
	for i := range outputs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var err error
			outputs[i], err = e.Encode(im)
			if err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	ordered := make([]PredictiveFrame, len(outputs))
	seen := make(map[int]bool)
	for _, out := range outputs {
		i := out.POC / 2
		if i >= len(outputs) || seen[i] {
			t.Fatal("concurrent sequence state", i)
		}
		seen[i] = true
		ordered[i] = out
	}
	for i, out := range ordered {
		want, err := control.Encode(im)
		if err != nil || !bytes.Equal(out.NAL, want.NAL) || out.Keyframe != want.Keyframe || out.FrameNum != want.FrameNum {
			t.Fatal("concurrent buffers/state", i, err)
		}
	}
}

func TestInterMeasurements(t *testing.T) {
	for _, kind := range []string{"static", "drift", "local", "noisechange"} {
		for _, search := range []bool{false, true} {
			const w, h, frames = 320, 180, 60
			for _, enabled := range []bool{false, true} {
				e, _ := NewPredictive(w, h, 24, PredictiveOptions{QP: 24, ModeSearch: search, GOP: 60, InterResidual: enabled})
				size, skipped, inter := len(annexB(e.SPS(), e.PPS())), 0, 0
				var sse float64
				var decoded picture420
				for f := range frames {
					im := interImage(kind, w, h, f)
					out, err := e.Encode(im)
					if err != nil {
						t.Fatal(err)
					}
					size += len(out.NAL) + 4
					skipped += out.SkippedMacroblocks
					var stats interDecodeStats
					decoded, _ = decodePredictiveWithStats(t, out, decoded, w, h, 24, &stats)
					inter += stats.Inter
					src := cropped420(makePicture420(im, w, h, 320, 192), w, h)
					rec := cropped420(e.reference, w, h)
					for i, v := range src {
						d := float64(v) - float64(rec[i])
						sse += d * d
					}
				}
				psnr := 10 * math.Log10(255*255*float64(w*h*3/2*frames)/sse)
				t.Logf("%s search=%t inter=%t: %dB %.3fdB skipped=%d interMB=%d", kind, search, enabled, size, psnr, skipped, inter)
			}
		}
	}
}

func BenchmarkInterResidual(b *testing.B) {
	for _, kind := range []string{"static", "drift", "local", "noisechange"} {
		for _, search := range []bool{false, true} {
			for _, enabled := range []bool{false, true} {
				b.Run(fmt.Sprintf("%s/search%t/inter%t", kind, search, enabled), func(b *testing.B) {
					images := make([]*image.RGBA, 60)
					for f := range images {
						images[f] = interImage(kind, 320, 180, f)
					}
					e, _ := NewPredictive(320, 180, 24, PredictiveOptions{QP: 24, ModeSearch: search, GOP: 60, InterResidual: enabled})
					b.ReportAllocs()
					b.ResetTimer()
					for range b.N {
						for _, im := range images {
							if _, err := e.Encode(im); err != nil {
								b.Fatal(err)
							}
						}
					}
					b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*60), "ns/frame")
				})
			}
		}
	}
}
