package h264

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

// These deterministic inputs exercise flat colors, edges, smooth gradients,
// high frequency chroma, noise, nonzero origins and padded image strides.
func intraImage(width, height, frame int) *image.RGBA {
	parent := image.NewRGBA(image.Rect(-5, -7, width+11, height+9))
	im := parent.SubImage(image.Rect(2, 1, width+2, height+1)).(*image.RGBA)
	rng := rand.New(rand.NewPCG(123, uint64(frame+1)))
	for y := range height {
		for x := range width {
			r, g, b := byte((x*3+y+frame*9)%256), byte((y*5+frame*11)%256), byte((x+y*2+frame*17)%256)
			switch x * 4 / width {
			case 0:
				r, g, b = byte(frame*37), byte(y*255/max(1, height-1)), 190
			case 1:
				if (x/3+y/5+frame)%2 == 0 {
					r, g, b = 255, 30, 20
				} else {
					r, g, b = 10, 220, 255
				}
			case 3:
				r, g, b = byte(rng.Uint32()), byte(rng.Uint32()), byte(rng.Uint32())
			}
			im.SetRGBA(x+2, y+1, color.RGBA{r, g, b, 255})
		}
	}
	return im
}

func extremeImage(width, height, frame int) *image.RGBA {
	im := image.NewRGBA(image.Rect(0, 0, width, height))
	colors := [...]color.RGBA{{255, 255, 0, 255}, {0, 0, 255, 255}, {255, 0, 0, 255}, {0, 255, 255, 255}, {0, 0, 0, 255}, {255, 255, 255, 255}}
	for y := range height {
		for x := range width {
			index := 2*(frame%3) + (x/16+y/16)%2
			if y >= 32 {
				index = 2*(frame%3) + (x+y)%2
			}
			im.SetRGBA(x, y, colors[index])
		}
	}
	return im
}

func annexB(nals ...[]byte) []byte {
	var out []byte
	for _, n := range nals {
		out = append(out, 0, 0, 0, 1)
		out = append(out, n...)
	}
	return out
}

func cropped420(p picture420, width, height int) []byte {
	var out []byte
	for c := range 3 {
		w, h, stride := width, height, p.stride
		if c != 0 {
			w, h, stride = w/2, h/2, stride/2
		}
		for y := range h {
			out = append(out, p.planes[c][y*stride:y*stride+w]...)
		}
	}
	return out
}

func digest(p []byte) string {
	h := sha256.Sum256(p)
	return hex.EncodeToString(h[:])
}

type intraVector struct {
	Name                           string
	Kind                           string
	Width, Height, QP, Frames      int
	BitstreamSHA256, DecodedSHA256 string
}

// The optional dump writes development inputs/outputs only; this test never
// executes an external program. FFmpeg independently decodes the dumped H.264
// to establish the committed golden hashes. Ordinary test runs use hashes.
func TestIntraReferenceVectors(t *testing.T) {
	checkIntraReferenceVectors(t, false)
}

func checkIntraReferenceVectors(t *testing.T, modeSearch bool) {
	_, here, _, _ := runtime.Caller(0)
	// runtime.Caller may report the virtual production path under -overlay.
	root := filepath.Dir(here)
	for {
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(root)
		if parent == root {
			t.Fatal("cannot find repository fixtures")
		}
		root = parent
	}
	filename := "intra_vectors.json"
	if modeSearch {
		filename = "intra_modesearch_vectors.json"
	}
	fixture := filepath.Join(root, "tests/data/internal/h264", filename)
	var vectors []intraVector
	dump := os.Getenv("GOMATH_H264_DUMP_DIR")
	if dump != "" {
		for _, dim := range [][2]int{{2, 2}, {18, 22}, {48, 34}, {160, 96}, {320, 180}} {
			for _, qp := range []int{0, 12, 24, 36, 51} {
				vectors = append(vectors, intraVector{Name: fmt.Sprintf("%dx%d_qp%d", dim[0], dim[1], qp), Width: dim[0], Height: dim[1], QP: qp, Frames: 3})
			}
		}
		for qp := range 52 {
			vectors = append(vectors, intraVector{Name: fmt.Sprintf("extreme_qp%d", qp), Kind: "extreme", Width: 64, Height: 48, QP: qp, Frames: 3})
		}
		if modeSearch {
			for _, kind := range []string{"vertical", "horizontal", "gradient"} {
				for _, qp := range []int{0, 12, 24, 36, 51} {
					vectors = append(vectors, intraVector{Name: fmt.Sprintf("%s_qp%d", kind, qp), Kind: kind, Width: 320, Height: 180, QP: qp, Frames: 3})
				}
			}
		}
		if err := os.MkdirAll(dump, 0755); err != nil {
			t.Fatal(err)
		}
	} else {
		data, err := os.ReadFile(fixture)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, &vectors); err != nil || len(vectors) == 0 {
			t.Fatal("invalid vector fixtures", err)
		}
	}
	for i := range vectors {
		v := &vectors[i]
		t.Run(v.Name, func(t *testing.T) {
			e, err := NewIntra(v.Width, v.Height, 24, IntraOptions{QP: v.QP, ModeSearch: modeSearch})
			if err != nil {
				t.Fatal(err)
			}
			stream := annexB(e.SPS(), e.PPS())
			var source, rebuilt []byte
			for f := range v.Frames {
				im := intraImage(v.Width, v.Height, f)
				if v.Kind == "extreme" {
					im = extremeImage(v.Width, v.Height, f)
				} else if v.Kind != "" {
					im = modeSearchImage(v.Kind, v.Width, v.Height, f)
				}
				n, rec, err := e.encode(im)
				if err != nil {
					t.Fatal(err)
				}
				stream = append(stream, annexB(n)...)
				rebuilt = append(rebuilt, cropped420(rec, v.Width, v.Height)...)
				source = append(source, cropped420(makePicture420(im, v.Width, v.Height, rec.stride, (v.Height+15)/16*16), v.Width, v.Height)...)
			}
			if dump != "" {
				for ext, p := range map[string][]byte{".h264": stream, ".rec.yuv": rebuilt, ".src.yuv": source} {
					if err := os.WriteFile(filepath.Join(dump, v.Name+ext), p, 0644); err != nil {
						t.Fatal(err)
					}
				}
				v.BitstreamSHA256, v.DecodedSHA256 = digest(stream), digest(rebuilt)
			} else if digest(stream) != v.BitstreamSHA256 || digest(rebuilt) != v.DecodedSHA256 {
				t.Fatal("bitstream/reconstruction differs from independently decoded golden vector")
			}
		})
	}
	if dump != "" {
		data, _ := json.MarshalIndent(vectors, "", "  ")
		if err := os.WriteFile(filepath.Join(dump, filename), data, 0644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestIntraValidation(t *testing.T) {
	for _, args := range [][4]int{{0, 16, 24, 24}, {17, 16, 24, 24}, {16, 16, 0, 24}, {4098, 16, 24, 24}, {16, 16, 121, 24}, {16, 16, 24, -1}, {16, 16, 24, 52}} {
		if _, err := NewIntra(args[0], args[1], args[2], IntraOptions{QP: args[3]}); err == nil {
			t.Fatal("accepted invalid options", args)
		}
	}
	e, _ := NewIntra(16, 16, 24, IntraOptions{QP: 24})
	for _, im := range []*image.RGBA{nil, image.NewRGBA(image.Rect(0, 0, 18, 16)), {Rect: image.Rect(0, 0, 16, 16), Stride: 64, Pix: make([]byte, 4)}, {Rect: image.Rect(0, 0, 16, 16), Stride: -64, Pix: make([]byte, 1024)}} {
		if _, err := e.Encode(im); err == nil {
			t.Fatal("accepted invalid frame")
		}
	}
	bad := &image.RGBA{Rect: image.Rect(0, 0, 16, 16), Stride: int(^uint(0) >> 1), Pix: make([]byte, 1024)}
	if _, err := e.Encode(bad); err == nil {
		t.Fatal("accepted overflowing pixel stride")
	}
}

func TestIntraParameterSets(t *testing.T) {
	pcm, _ := New(16, 16, 24)
	e, _ := NewIntra(16, 16, 24, IntraOptions{QP: 24})
	if !bytes.Equal(e.SPS(), pcm.SPS()) || !bytes.Equal(e.PPS(), pcm.PPS()) {
		t.Fatal("ordinary intra parameter sets incompatible with PCM")
	}
	for _, test := range []struct{ w, h, fps, level int }{{1920, 1080, 120, 51}, {3840, 2160, 60, 52}, {4096, 4096, 24, 60}, {4096, 4096, 120, 61}} {
		e, err := NewIntra(test.w, test.h, test.fps, IntraOptions{QP: 24})
		if err != nil {
			t.Fatal(err)
		}
		if int(unescape(e.SPS())[2]) != test.level {
			t.Fatal("incorrect level for frame size / processing rate", test)
		}
	}
	for qp := range 52 {
		e, _ := NewIntra(16, 16, 24, IntraOptions{QP: qp})
		r := reader{p: unescape(e.PPS())}
		r.ue()
		r.ue()
		if r.bit() != 0 {
			t.Fatal("PPS requests CABAC")
		}
		r.bit()
		r.ue()
		r.ue()
		r.ue()
		r.bit()
		r.uint(2)
		r.ue() // initial QP delta is zero
		r.ue() // initial QS delta is zero
		code := int(r.ue())
		offset := (code + 1) / 2
		if code%2 == 0 {
			offset = -code / 2
		}
		if offset != max(0, 4-qp) {
			t.Fatal("chroma quantization offset", qp, offset)
		}
	}
}

func TestIntraCompressionAndQuality(t *testing.T) {
	im := image.NewRGBA(image.Rect(0, 0, 320, 180))
	for y := range 180 {
		for x := range 320 {
			im.SetRGBA(x, y, color.RGBA{byte(x * 255 / 319), byte(y * 255 / 179), byte((x + y) * 255 / 498), 255})
		}
	}
	pcm, _ := New(320, 180, 24)
	uncompressed, _ := pcm.Encode(im)
	previousBytes, previousMSE := len(uncompressed), 0.0
	for _, qp := range []int{12, 24, 36, 51} {
		e, _ := NewIntra(320, 180, 24, IntraOptions{QP: qp})
		n, rec, err := e.encode(im)
		if err != nil {
			t.Fatal(err)
		}
		if len(n) >= previousBytes {
			t.Fatalf("QP %d: compressed %d >= previous %d", qp, len(n), previousBytes)
		}
		src := cropped420(makePicture420(im, 320, 180, 320, 192), 320, 180)
		got := cropped420(rec, 320, 180)
		mse := 0.0
		for i, v := range src {
			d := float64(int(v) - int(got[i]))
			mse += d * d
		}
		mse /= float64(len(src))
		if mse <= previousMSE || qp == 24 && 10*math.Log10(255*255/mse) < 35 {
			t.Fatal("unexpected quantization quality", qp, mse)
		}
		t.Logf("QP=%d bytes=%d PCM=%d reduction=%.2fx YUV-PSNR=%.2f dB", qp, len(n), len(uncompressed), float64(len(uncompressed))/float64(len(n)), 10*math.Log10(255*255/mse))
		previousBytes, previousMSE = len(n), mse
	}
}

func TestIntraPixelsAndConcurrentCalls(t *testing.T) {
	e, _ := NewIntra(48, 34, 24, IntraOptions{QP: 24})
	im := intraImage(48, 34, 0)
	before := append([]byte(nil), im.Pix...)
	want, _ := e.Encode(im)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			got, err := e.Encode(im)
			if err != nil || !bytes.Equal(got, want) {
				t.Error("concurrent calls differ", err)
			}
		})
	}
	wg.Wait()
	if !bytes.Equal(im.Pix, before) {
		t.Fatal("encoder modified input")
	}
	other, _ := e.Encode(intraImage(48, 34, 1))
	if bytes.Equal(want, other) {
		t.Fatal("encoder discarded visible input changes")
	}
	for _, bad := range [][]byte{{0, 0, 0}, {0, 0, 1}, {0, 0, 2}} {
		if bytes.Contains(want[1:], bad) {
			t.Fatal("unescaped NAL", bad)
		}
	}
}

func TestIntraAlphaAndSubimage(t *testing.T) {
	im := intraImage(48, 34, 0)
	opaque := image.NewRGBA(image.Rect(0, 0, 48, 34))
	for y := range 34 {
		for x := range 48 {
			v := im.RGBAAt(x+im.Rect.Min.X, y+im.Rect.Min.Y)
			v.R, v.G, v.B, v.A = v.R/2, v.G/2, v.B/2, 128
			im.SetRGBA(x+im.Rect.Min.X, y+im.Rect.Min.Y, v)
			v.A = 255
			opaque.SetRGBA(x, y, v)
		}
	}
	e, _ := NewIntra(48, 34, 24, IntraOptions{QP: 24})
	a, err := e.Encode(im)
	if err != nil {
		t.Fatal(err)
	}
	b, err := e.Encode(opaque)
	if err != nil || !bytes.Equal(a, b) {
		t.Fatal("premultiplied transparency / subimage differs from black composite", err)
	}
}

func BenchmarkPCM(b *testing.B) {
	for _, dims := range [][2]int{{320, 180}, {1280, 720}} {
		b.Run(fmt.Sprintf("%dx%d", dims[0], dims[1]), func(b *testing.B) {
			e, _ := New(dims[0], dims[1], 24)
			im := intraImage(dims[0], dims[1], 0)
			b.ReportAllocs()
			b.SetBytes(int64(dims[0] * dims[1] * 4))
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

func BenchmarkIntra(b *testing.B) {
	for _, dims := range [][2]int{{320, 180}, {1280, 720}} {
		for _, qp := range []int{12, 24, 36} {
			b.Run(fmt.Sprintf("%dx%d/QP%d", dims[0], dims[1], qp), func(b *testing.B) {
				e, _ := NewIntra(dims[0], dims[1], 24, IntraOptions{QP: qp})
				im := intraImage(dims[0], dims[1], 0)
				b.ReportAllocs()
				b.SetBytes(int64(dims[0] * dims[1] * 4))
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
