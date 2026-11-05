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

// Independent decoder state uses quarter-luma units, unlike encoder vectors.
type decodedMotion struct {
	x, y  int
	inter bool
}

func signedMotion(r *reader) int {
	u := int(r.ue())
	if u&1 == 0 {
		return -u / 2
	}
	return (u + 1) / 2
}

func predictDecodedMotion(grid []decodedMotion, w, address int, skip bool) decodedMotion {
	x, y := address%w, address/w
	get := func(nx, ny int) (decodedMotion, bool) {
		if nx < 0 || nx >= w || ny < 0 {
			return decodedMotion{}, false
		}
		return grid[ny*w+nx], true
	}
	a, av := get(x-1, y)
	b, bv := get(x, y-1)
	c, cv := get(x+1, y-1)
	if !cv {
		c, cv = get(x-1, y-1)
	}
	if skip && (!av || !bv || a.inter && a.x == 0 && a.y == 0 || b.inter && b.x == 0 && b.y == 0) {
		return decodedMotion{inter: true}
	}
	if !bv && !cv {
		b, c = a, a
	}
	match := 0
	var found decodedMotion
	for _, n := range []decodedMotion{a, b, c} {
		if n.inter {
			match++
			found = n
		}
	}
	if match == 1 {
		return found
	}
	// Independent sorting network instead of sum/min/max.
	median := func(v [3]int) int {
		if v[0] > v[1] {
			v[0], v[1] = v[1], v[0]
		}
		if v[1] > v[2] {
			v[1], v[2] = v[2], v[1]
		}
		if v[0] > v[1] {
			v[0], v[1] = v[1], v[0]
		}
		return v[1]
	}
	return decodedMotion{median([3]int{a.x, b.x, c.x}), median([3]int{a.y, b.y, c.y}), true}
}

// Independent normative denominator64 chroma formula, with each tap clipped.
// Reads actual emitted MVDs and the independently predicted quarter-luma MV.
func compensateDecodedMotion(t *testing.T, dst, ref picture420, mx, my int, mv decodedMotion) {
	t.Helper()
	if !mv.inter || mv.x%4 != 0 || mv.y%4 != 0 {
		t.Fatal("unsupported decoded motion", mv)
	}
	for c := 0; c < 3; c++ {
		stride, n, x, y := ref.stride, 16, mx, my
		if c > 0 {
			stride, n, x, y = stride/2, 8, x/2, y/2
		}
		h := len(ref.planes[c]) / stride
		sample := func(a, b int) int { return int(ref.planes[c][max(0, min(h-1, b))*stride+max(0, min(stride-1, a))]) }
		for j := 0; j < n; j++ {
			for i := 0; i < n; i++ {
				v := 0
				if c == 0 {
					v = sample(x+i+mv.x/4, y+j+mv.y/4)
				} else {
					sx, sy := (x+i)*8+mv.x, (y+j)*8+mv.y
					ix, iy, fx, fy := sx>>3, sy>>3, sx&7, sy&7
					v = ((8-fx)*(8-fy)*sample(ix, iy) + fx*(8-fy)*sample(ix+1, iy) + (8-fx)*fy*sample(ix, iy+1) + fx*fy*sample(ix+1, iy+1) + 32) / 64
				}
				dst.planes[c][(y+j)*(dst.stride>>(min(c, 1)))+x+i] = byte(v)
			}
		}
	}
}

func motionImage(kind string, w, h, f int) *image.RGBA {
	if kind == "static" || kind == "noisechange" {
		return interImage(kind, w, h, f)
	}
	base := intraImage(w+32, h+32, 0)
	im := image.NewRGBA(image.Rect(-3, 2, w-3, h+2))
	for y := range h {
		for x := range w {
			dx, dy := f%11, f%7
			if kind == "reverse" || kind == "edge" {
				dx = f % 8
				if dx > 4 {
					dx = 8 - dx
				}
				dy = -dx
			}
			if kind == "tiles" && (x/16+y/16)%3 == 0 {
				dx, dy = -dx, -dy
			}
			if kind == "edge" {
				sx, sy := max(0, min(w-1, x-dx)), max(0, min(h-1, y-dy))
				palette := []color.RGBA{{255, 0, 0, 255}, {0, 0, 255, 255}, {0, 255, 0, 255}, {255, 255, 0, 255}}
				im.SetRGBA(x-3, y+2, palette[(sx/3+sy/5)%4])
			} else {
				im.SetRGBA(x-3, y+2, base.RGBAAt(x+16+dx, y+16+dy))
			}
		}
	}
	return im
}

type motionFixture struct {
	predictiveVector
	Radius int
}

func motionVectors() []motionFixture {
	var out []motionFixture
	add := func(kind string, w, h, qp, gop, n, r int, s bool) {
		out = append(out, motionFixture{predictiveVector: predictiveVector{Kind: kind, Width: w, Height: h, QP: qp, GOP: gop, Frames: n, ModeSearch: s}, Radius: r})
	}
	for _, search := range []bool{false, true} {
		for qp := range 52 {
			add("pan", 34, 38, qp, 4, 9, 1, search)
			add("edge", 34, 38, qp, 4, 9, 2, search)
		}
		for r := 1; r <= 4; r++ {
			for _, dim := range [][2]int{{2, 2}, {18, 22}, {50, 34}} {
				for _, qp := range []int{0, 24, 51} {
					add("reverse", dim[0], dim[1], qp, 17, 20, r, search)
				}
			}
		}
		for _, kind := range []string{"pan", "tiles", "edge", "recovery", "static"} {
			add(kind, 34, 38, 24, 60, 125, 2, search)
		}
		add("pan", 18, 22, 24, 1, 5, 4, search)
		add("edge", 2, 2, 24, 65536, 35, 4, search)
	}
	// Measurement groups are also independently decoded, with identical inputs
	// and option settings to the timing benchmarks (except radius0 controls).
	for _, kind := range []string{"pan", "static", "noisechange"} {
		for _, r := range []int{0, 1, 2} {
			add(kind, 320, 180, 24, 60, 60, r, false)
		}
	}
	for i := range out {
		v := &out[i]
		v.Name = fmt.Sprintf("%s_%dx%d_q%d_g%d_n%d_s%t_r%d", v.Kind, v.Width, v.Height, v.QP, v.GOP, v.Frames, v.ModeSearch, v.Radius)
	}
	return out
}

func TestMotionReferenceVectors(t *testing.T) {
	dump := os.Getenv("GOMATH_H264_MOTION_DUMP_DIR")
	vectors := motionVectors()
	if dump == "" {
		b, err := os.ReadFile("../../tests/data/internal/h264/motion_vectors.json")
		if err != nil {
			t.Fatal(err)
		}
		var golden []motionFixture
		if err = json.Unmarshal(b, &golden); err != nil || len(golden) != len(vectors) {
			t.Fatal("motion golden", err, len(golden), len(vectors))
		}
		for i := range vectors {
			if golden[i].Name != vectors[i].Name {
				t.Fatal("motion fixture definition changed", i)
			}
		}
		vectors = golden
	} else if err := os.MkdirAll(dump, 0755); err != nil {
		t.Fatal(err)
	}
	var stats interDecodeStats
	for k := range vectors {
		v := &vectors[k]
		t.Run(v.Name, func(t *testing.T) {
			e, err := NewPredictive(v.Width, v.Height, 24, PredictiveOptions{QP: v.QP, ModeSearch: v.ModeSearch, GOP: v.GOP, InterResidual: true, MotionSearchRadius: v.Radius})
			if err != nil {
				t.Fatal(err)
			}
			stream := annexB(e.SPS(), e.PPS())
			var rebuilt, source []byte
			var decoded picture420
			pos, skipped := 0, 0
			for f := range v.Frames {
				if v.Kind == "recovery" {
					if f == 4 || f == 5 || f == 17 {
						e.ForceIDR()
						pos = 0
					}
					if f == 12 {
						e.Reset()
						pos = 0
					}
					if f == 4 || f == 12 || f == 59 || f == 60 {
						if _, err := e.Encode(nil); err == nil {
							t.Fatal("failure consumed input")
						}
					}
				}
				im := motionImage(v.Kind, v.Width, v.Height, f)
				out, err := e.Encode(im)
				if err != nil {
					t.Fatal(err)
				}
				key := pos == 0 || pos == v.GOP
				if key {
					pos = 0
				}
				if out.Keyframe != key || out.FrameNum != uint8(pos%16) || out.POC != 2*pos {
					t.Fatal("sequence state", f, out)
				}
				pos++
				var skips []bool
				decoded, skips = decodePredictiveWithStats(t, out, decoded, v.Width, v.Height, v.QP, &stats)
				gotSkips := 0
				for _, b := range skips {
					if b {
						gotSkips++
					}
				}
				if gotSkips != out.SkippedMacroblocks {
					t.Fatal("skip metadata")
				}
				skipped += gotSkips
				for c := range 3 {
					if !bytes.Equal(decoded.planes[c], e.reference.planes[c]) {
						t.Fatal("motion reconstruction", f, c)
					}
				}
				if !key && v.Radius > 0 {
					for _, mv := range e.workspace.motion {
						if abs(mv.mv.x) > v.Radius || abs(mv.mv.y) > v.Radius {
							t.Fatal("search limit", mv)
						}
					}
				}
				stream = append(stream, annexB(out.NAL)...)
				rebuilt = append(rebuilt, cropped420(e.reference, v.Width, v.Height)...)
				src := makePicture420(im, v.Width, v.Height, decoded.stride, (v.Height+15)/16*16)
				source = append(source, cropped420(src, v.Width, v.Height)...)
			}
			if dump != "" {
				for ext, data := range map[string][]byte{".h264": stream, ".rec.yuv": rebuilt, ".src.yuv": source} {
					if err := os.WriteFile(filepath.Join(dump, v.Name+ext), data, 0644); err != nil {
						t.Fatal(err)
					}
				}
				v.BitstreamSHA256, v.DecodedSHA256, v.Bytes, v.Skipped = digest(stream), digest(rebuilt), len(stream), skipped
			} else if digest(stream) != v.BitstreamSHA256 || digest(rebuilt) != v.DecodedSHA256 || len(stream) != v.Bytes || skipped != v.Skipped {
				t.Fatal("motion golden mismatch")
			}
		})
	}
	if stats.Moved == 0 || stats.OddChroma == 0 || stats.MVD == 0 || stats.MovingSkip == 0 {
		t.Fatal("motion syntax not exercised", stats)
	}
	t.Logf("inter=%d intra=%d moved=%d odd-chroma=%d nonzero-MVD=%d moving-skip=%d", stats.Inter, stats.Intra, stats.Moved, stats.OddChroma, stats.MVD, stats.MovingSkip)
	if dump != "" {
		b, _ := json.MarshalIndent(vectors, "", "  ")
		if err := os.WriteFile(filepath.Join(dump, "motion_vectors.json"), b, 0644); err != nil {
			t.Fatal(err)
		}
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func TestMotionPredictorsAndSampling(t *testing.T) {
	// Includes intra versus unavailable, A-only, C-to-D fallback, a unique
	// reference match, mixed-sign medians and the separate skip zero rule.
	for w := 1; w <= 4; w++ {
		for h := 1; h <= 3; h++ {
			for seed := range 128 {
				grid := make([]motionInfo, w*h)
				decoded := make([]decodedMotion, w*h)
				for i := range grid {
					mv := motionVector{(seed+i*7)%9 - 4, (seed*3+i*11)%9 - 4}
					inter := (seed>>uint(i%7))&1 != 0
					if !inter {
						mv = motionVector{}
					}
					grid[i] = motionInfo{mv, inter}
					decoded[i] = decodedMotion{4 * mv.x, 4 * mv.y, inter}
				}
				for i := range grid {
					for _, skip := range []bool{false, true} {
						got := predictMotion(grid, w, i%w, i/w, skip)
						want := predictDecodedMotion(decoded, w, i, skip)
						if 4*got.x != want.x || 4*got.y != want.y {
							t.Fatal("motion predictor", w, h, seed, i, skip, got, want)
						}
					}
				}
			}
		}
	}
	// First row: general prediction propagates A, while skip must be zero.
	g := []motionInfo{{motionVector{3, -2}, true}, {}}
	if predictMotion(g, 2, 1, 0, false) != (motionVector{3, -2}) || predictMotion(g, 2, 1, 0, true) != (motionVector{}) {
		t.Fatal("skip differs from general prediction")
	}
	for _, dim := range [][2]int{{16, 16}, {32, 48}, {48, 32}} {
		ref := newPicture420(dim[0], dim[1])
		for c := range 3 {
			for i := range ref.planes[c] {
				ref.planes[c][i] = byte(i*73 + c*17)
			}
		}
		var compact [384]byte
		p := motionBlock(&compact)
		full := newPicture420(dim[0], dim[1])
		for my := 0; my < dim[1]; my += 16 {
			for mx := 0; mx < dim[0]; mx += 16 {
				for dy := -4; dy <= 4; dy++ {
					for dx := -4; dx <= 4; dx++ {
						compensateMotion(p, ref, mx, my, motionVector{dx, dy})
						compensateDecodedMotion(t, full, ref, mx, my, decodedMotion{4 * dx, 4 * dy, true})
						for c := range 3 {
							stride, n, x, y := full.stride, 16, mx, my
							if c > 0 {
								stride, n, x, y = stride/2, 8, x/2, y/2
							}
							for j := range n {
								if !bytes.Equal(p.planes[c][j*n:(j+1)*n], full.planes[c][(y+j)*stride+x:(y+j)*stride+x+n]) {
									t.Fatal("clipped chroma/luma", dim, mx, my, dx, dy, c, j)
								}
							}
						}
					}
				}
			}
		}
	}
}

func TestMotionConcurrentAndNALOwnership(t *testing.T) {
	o := PredictiveOptions{QP: 24, GOP: 7, ModeSearch: true, InterResidual: true, MotionSearchRadius: 2}
	e, _ := NewPredictive(34, 38, 24, o)
	control, _ := NewPredictive(34, 38, 24, o)
	im := motionImage("static", 34, 38, 0)
	out := make(chan PredictiveFrame, 24)
	var wg sync.WaitGroup
	for range 24 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			f, err := e.Encode(im)
			if err != nil {
				t.Error(err)
			}
			out <- f
		}()
	}
	wg.Wait()
	close(out)
	// Identical input makes the serialized output multiset deterministic even
	// when goroutines return to the channel in a different order.
	want := map[string]int{}
	for range 24 {
		f, err := control.Encode(im)
		if err != nil {
			t.Fatal(err)
		}
		want[string(f.NAL)]++
	}
	var nals, saved [][]byte
	for f := range out {
		want[string(f.NAL)]--
		nals = append(nals, f.NAL)
		saved = append(saved, bytes.Clone(f.NAL))
	}
	for _, n := range want {
		if n != 0 {
			t.Fatal("concurrent output/state")
		}
	}
	for f := range 12 {
		if _, err := e.Encode(motionImage("pan", 34, 38, f)); err != nil {
			t.Fatal(err)
		}
	}
	e.Reset()
	if e.workspace != nil {
		t.Fatal("retained motion storage")
	}
	for i, nal := range nals {
		if !bytes.Equal(nal, saved[i]) {
			t.Fatal("NAL mutated", i)
		}
	}
}

func TestMotionLimitsAndState(t *testing.T) {
	for _, o := range []PredictiveOptions{{QP: 24, GOP: 60, InterResidual: true, MotionSearchRadius: -1}, {QP: 24, GOP: 60, InterResidual: true, MotionSearchRadius: 5}, {QP: 24, GOP: 60, MotionSearchRadius: 1}} {
		if _, err := NewPredictive(34, 38, 24, o); err == nil {
			t.Fatal("unsupported option accepted", o)
		}
	}
	for _, d := range [][2]int{{33, 38}, {34, 37}, {1, 2}, {4098, 2}} {
		if _, err := NewPredictive(d[0], d[1], 24, PredictiveOptions{QP: 24, GOP: 60, InterResidual: true, MotionSearchRadius: 1}); err == nil {
			t.Fatal("unsupported geometry", d)
		}
	}
	for qp := range 52 {
		for _, search := range []bool{false, true} {
			o := PredictiveOptions{QP: qp, GOP: 7, ModeSearch: search, InterResidual: true, MotionSearchRadius: 1}
			e, _ := NewPredictive(18, 22, 24, o)
			control, _ := NewPredictive(18, 22, 24, o)
			for f := range 19 {
				if f == 4 || f == 5 {
					e.ForceIDR()
					control.ForceIDR()
				}
				if f == 12 {
					e.Reset()
					control.Reset()
					if e.workspace != nil {
						t.Fatal("Reset retained motion")
					}
				}
				if _, err := e.Encode(nil); err == nil {
					t.Fatal("nil input")
				}
				bad := image.NewRGBA(image.Rect(0, 0, 18, 22))
				bad.Stride = 1
				if _, err := e.Encode(bad); err == nil {
					t.Fatal("bad stride")
				}
				im := motionImage("pan", 18, 22, f)
				a, err := e.Encode(im)
				b, berr := control.Encode(im)
				if err != nil || berr != nil || !bytes.Equal(a.NAL, b.NAL) || a.Keyframe != b.Keyframe || a.POC != b.POC {
					t.Fatal("failed input changed state", qp, f, err, berr)
				}
				if f == 0 {
					base, _ := NewPredictive(18, 22, 24, PredictiveOptions{QP: qp, GOP: 7, ModeSearch: search, InterResidual: true})
					c, _ := base.Encode(im)
					if !bytes.Equal(a.NAL, c.NAL) || !bytes.Equal(e.SPS(), base.SPS()) || !bytes.Equal(e.PPS(), base.PPS()) {
						t.Fatal("IDR/parameter sets changed")
					}
				}
			}
		}
	}
}

func BenchmarkMotionSearch(b *testing.B) {
	for _, kind := range []string{"pan", "static", "noisechange"} {
		for _, r := range []int{0, 1, 2} {
			b.Run(fmt.Sprintf("%s/r%d", kind, r), func(b *testing.B) {
				images := make([]*image.RGBA, 60)
				for f := range images {
					images[f] = motionImage(kind, 320, 180, f)
				}
				e, _ := NewPredictive(320, 180, 24, PredictiveOptions{QP: 24, GOP: 60, InterResidual: true, MotionSearchRadius: r})
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

func TestMotionMeasurements(t *testing.T) {
	if os.Getenv("GOMATH_H264_MEASURE") != "1" {
		t.Skip("development measurement; opt in with GOMATH_H264_MEASURE=1")
	}
	for _, kind := range []string{"pan", "static", "noisechange"} {
		for _, r := range []int{0, 1, 2} {
			e, _ := NewPredictive(320, 180, 24, PredictiveOptions{QP: 24, GOP: 60, InterResidual: true, MotionSearchRadius: r})
			size := len(annexB(e.SPS(), e.PPS()))
			var sse float64
			for f := range 60 {
				im := motionImage(kind, 320, 180, f)
				out, err := e.Encode(im)
				if err != nil {
					t.Fatal(err)
				}
				size += 4 + len(out.NAL)
				src := cropped420(makePicture420(im, 320, 180, 320, 192), 320, 180)
				rec := cropped420(e.reference, 320, 180)
				for i, v := range src {
					d := float64(v) - float64(rec[i])
					sse += d * d
				}
			}
			t.Logf("%s/r%d: bytes=%d PSNR=%.6f", kind, r, size, 10*math.Log10(255*255*float64(320*180*3/2*60)/sse))
		}
	}
}
