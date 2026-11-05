package three

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"gomath/geom"
	"image"
	"image/color"
	"image/draw"
	"math"
	"strings"
	"sync"
	"testing"
)

func TestSupersamplingLimitsBeforeAllocation(t *testing.T) {
	for _, test := range []struct {
		w, h, scale, rw, rh int
		bytes               int64
	}{
		{2, 3, 0, 2, 3, 72}, {2, 3, 1, 2, 3, 72}, {2, 3, 2, 4, 6, 312}, {2, 3, 4, 8, 12, 1176},
		{4096, 1024, 1, 4096, 1024, 48 << 20},
		{4096, 1024, 2, 8192, 2048, 208 << 20},
		{2048, 512, 4, 8192, 2048, 196 << 20},
	} {
		c := DefaultConfig(test.w, test.h)
		c.Supersampling = test.scale
		r, err := NewRenderer(c)
		if err != nil {
			t.Fatalf("valid layout %+v: %v", test, err)
		}
		if got := r.sampling; got.width != test.rw || got.height != test.rh || got.rasterBytes != test.bytes || got.rasterBytes > MaxRasterBufferBytes {
			t.Fatalf("layout %+v: %+v", test, got)
		}
	}
	maxInt := int(^uint(0) >> 1)
	for _, test := range []struct {
		w, h, scale int
		message     string
	}{
		{1, 1, -1, "supersampling"}, {1, 1, 3, "supersampling"}, {1, 1, 8, "supersampling"}, {1, 1, maxInt, "supersampling"},
		{maxInt, maxInt, 4, "dimensions"}, {-1, 2, 4, "dimensions"}, {4096, 2048, 1, "dimensions"},
		{4096, 1024, 4, "internal dimensions"}, {2048, 1024, 4, "internal pixels"},
	} {
		c := DefaultConfig(test.w, test.h)
		c.Supersampling = test.scale
		r, err := NewRenderer(c)
		if r != nil || err == nil || !strings.Contains(err.Error(), test.message) {
			t.Fatalf("invalid layout %+v: renderer=%v err=%v", test, r, err)
		}
	}
}

func TestSupersamplingDefaultMatchesOne(t *testing.T) {
	cfg := DefaultConfig(37, 23)
	cfg.Background = color.NRGBA{R: 60, G: 100, B: 170, A: 89}
	base, err := NewRenderer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Supersampling = 1
	one, err := NewRenderer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, projection := range []Projection{Perspective, Orthographic} {
		a, err := base.Render(testCamera(projection), colored(triangleMesh(), red))
		if err != nil {
			t.Fatal(err)
		}
		b, err := one.Render(testCamera(projection), colored(triangleMesh(), red))
		if err != nil {
			t.Fatal(err)
		}
		if a.Bounds() != b.Bounds() || !bytes.Equal(a.Pix, b.Pix) {
			t.Fatal("zero/default changed scale-1 output")
		}
	}
}

// The quad extends beyond the viewport. Its vertical right edge gives a known
// coverage fraction without depending on a triangle barycentric calculation.
func coverageQuad(right, extent, depth float64) *Mesh {
	return &Mesh{Vertices: []geom.Vec3{{X: -4 * extent, Y: -4 * extent, Z: -depth}, {X: right, Y: -4 * extent, Z: -depth}, {X: right, Y: 4 * extent, Z: -depth}, {X: -4 * extent, Y: 4 * extent, Z: -depth}}, Triangles: []Triangle{{0, 1, 2}, {0, 2, 3}}}
}

func TestSupersamplingTransparentCoverageAndPremultiplication(t *testing.T) {
	for _, projection := range []Projection{Perspective, Orthographic} {
		for _, test := range []struct {
			scale      int
			fraction   float64
			background color.NRGBA
			want       color.RGBA
		}{
			{2, .5, color.NRGBA{R: 255, G: 255, B: 255}, color.RGBA{R: 100, G: 50, B: 25, A: 128}},
			{4, .5, color.NRGBA{}, color.RGBA{R: 100, G: 50, B: 25, A: 128}},
			{4, .25, color.NRGBA{}, color.RGBA{R: 50, G: 25, B: 13, A: 64}},
			{4, .75, color.NRGBA{}, color.RGBA{R: 150, G: 75, B: 38, A: 191}},
			{4, .5, color.NRGBA{B: 200, A: 128}, color.RGBA{R: 100, G: 50, B: 75, A: 192}},
		} {
			cfg := DefaultConfig(1, 1)
			cfg.Supersampling = test.scale
			cfg.Light = Light{Ambient: 1}
			cfg.Background = test.background
			r, err := NewRenderer(cfg)
			if err != nil {
				t.Fatal(err)
			}
			camera := testCamera(projection)
			camera.OrthoHeight = 2
			extent := 1.0
			if projection == Perspective {
				extent = 2
			}
			m := coverageQuad(extent*(2*test.fraction-1), extent, 2)
			img, err := r.Render(camera, colored(m, color.NRGBA{R: 200, G: 100, B: 50, A: 255}))
			if err != nil {
				t.Fatal(err)
			}
			if got := img.RGBAAt(0, 0); got != test.want {
				t.Fatalf("projection %d scale %d coverage %g bg %v: %v want %v", projection, test.scale, test.fraction, test.background, got, test.want)
			}
			if test.fraction == .5 && test.background.A == 0 {
				over := image.NewRGBA(image.Rect(0, 0, 1, 1))
				draw.Draw(over, over.Bounds(), image.White, image.Point{}, draw.Src)
				draw.Draw(over, over.Bounds(), img, image.Point{}, draw.Over)
				if got := over.RGBAAt(0, 0); got != (color.RGBA{R: 227, G: 177, B: 152, A: 255}) {
					t.Fatalf("incorrect premultiplied edge over white: %v", got)
				}
			}
		}
	}
}

func TestSupersamplingBackgroundAndViewportCoverage(t *testing.T) {
	for _, scale := range []int{1, 2, 4} {
		for _, alpha := range []uint8{0, 128, 255} {
			cfg := DefaultConfig(7, 3)
			cfg.Supersampling = scale
			cfg.Background = color.NRGBA{R: 200, G: 101, B: 50, A: alpha}
			cfg.Light = Light{Ambient: 1}
			r, err := NewRenderer(cfg)
			if err != nil {
				t.Fatal(err)
			}
			camera := testCamera(Orthographic)
			empty, err := r.Render(camera)
			if err != nil {
				t.Fatal(err)
			}
			if empty.Bounds() != image.Rect(0, 0, 7, 3) {
				t.Fatalf("incorrect resolved bounds: %v", empty.Bounds())
			}
			want := color.RGBAModel.Convert(cfg.Background).(color.RGBA)
			for y := 0; y < 3; y++ {
				for x := 0; x < 7; x++ {
					if got := empty.RGBAAt(x, y); got != want {
						t.Fatalf("background scale=%d alpha=%d: %v want %v", scale, alpha, got, want)
					}
				}
			}
			full, err := r.Render(camera, colored(coverageQuad(20, 5, 2), red))
			if err != nil {
				t.Fatal(err)
			}
			for y := 0; y < 3; y++ {
				for x := 0; x < 7; x++ {
					if got := full.RGBAAt(x, y); got != (color.RGBA{R: 255, A: 255}) {
						t.Fatalf("clipped viewport/shared-edge coverage at %d,%d scale=%d: %v", x, y, scale, got)
					}
				}
			}
		}
	}
}

func TestSupersamplingClippedEdgesAgainstRayReference(t *testing.T) {
	const width, height = 13, 9
	m := &Mesh{Vertices: []geom.Vec3{{X: -3, Y: -1.6, Z: -.25}, {X: 3.5, Y: -1.6, Z: -3}, {X: .1, Y: 3.2, Z: -9}}, Triangles: []Triangle{{0, 1, 2}}}
	base := color.NRGBA{R: 201, G: 111, B: 53, A: 255}
	for _, projection := range []Projection{Perspective, Orthographic} {
		for _, scale := range []int{2, 4} {
			for _, bgAlpha := range []uint8{0, 96} {
				cfg := DefaultConfig(width, height)
				cfg.Supersampling = scale
				cfg.Light = Light{Ambient: 1}
				cfg.Background = color.NRGBA{R: 41, G: 163, B: 233, A: bgAlpha}
				r, err := NewRenderer(cfg)
				if err != nil {
					t.Fatal(err)
				}
				camera := testCamera(projection)
				camera.Far = 6
				img, err := r.Render(camera, colored(m, base))
				if err != nil {
					t.Fatal(err)
				}
				bg := color.RGBAModel.Convert(cfg.Background).(color.RGBA)
				partials, clippedPartials := 0, 0
				for y := 0; y < height; y++ {
					for x := 0; x < width; x++ {
						covered, depthClipped := 0, false
						for sy := 0; sy < scale; sy++ {
							for sx := 0; sx < scale; sx++ {
								px := float64(x) + (float64(sx)+.5)/float64(scale)
								py := float64(y) + (float64(sy)+.5)/float64(scale)
								rx, ry := (2*px/width-1)*width/height, 1-2*py/height
								origin, direction := geom.Vec3{}, geom.Vec3{X: rx, Y: ry, Z: -1}
								if projection == Orthographic {
									origin = geom.Vec3{X: rx * 2, Y: ry * 2}
									direction = geom.Vec3{Z: -1}
								}
								depth, _, hit := rayHit(origin, direction, m)
								if hit {
									if depth >= camera.Near && depth <= camera.Far {
										covered++
									} else {
										depthClipped = true
									}
								}
							}
						}
						n := scale * scale
						mix := func(fore, back uint8) uint8 {
							return uint8(math.Round((float64(covered)*float64(fore) + float64(n-covered)*float64(back)) / float64(n)))
						}
						want := color.RGBA{R: mix(base.R, bg.R), G: mix(base.G, bg.G), B: mix(base.B, bg.B), A: mix(255, bg.A)}
						if got := img.RGBAAt(x, y); got != want {
							t.Fatalf("projection %d scale %d bg alpha %d pixel (%d,%d): %v want %v (%d/%d hits)", projection, scale, bgAlpha, x, y, got, want, covered, n)
						}
						if covered > 0 && covered < n {
							partials++
							if depthClipped {
								clippedPartials++
							}
						}
					}
				}
				if partials == 0 || clippedPartials == 0 {
					t.Fatalf("insufficient edge/near-far clipping coverage for projection %d scale %d: %d/%d", projection, scale, partials, clippedPartials)
				}
			}
		}
	}
}

func TestSupersamplingOpaqueOcclusionPerSample(t *testing.T) {
	for _, scale := range []int{2, 4} {
		cfg := DefaultConfig(1, 1)
		cfg.Supersampling = scale
		cfg.Light = Light{Ambient: 1}
		r, err := NewRenderer(cfg)
		if err != nil {
			t.Fatal(err)
		}
		front := colored(coverageQuad(0, 1, 2), red)
		back := colored(coverageQuad(4, 1, 4), green)
		camera := testCamera(Orthographic)
		camera.OrthoHeight = 2
		a, err := r.Render(camera, front, back)
		if err != nil {
			t.Fatal(err)
		}
		b, err := r.Render(camera, back, front)
		if err != nil {
			t.Fatal(err)
		}
		if got := a.RGBAAt(0, 0); got != (color.RGBA{R: 128, G: 128, A: 255}) || !bytes.Equal(a.Pix, b.Pix) {
			t.Fatalf("scale %d per-sample opaque occlusion: %v; reverse %v", scale, got, b.RGBAAt(0, 0))
		}
	}
}

func TestSupersamplingConcurrentDeterminism(t *testing.T) {
	m, err := Sphere(1, 16, 8)
	if err != nil {
		t.Fatal(err)
	}
	camera := testCamera(Perspective)
	camera.Position = geom.Vec3{Z: 3}
	camera.Target = geom.Vec3{}
	for _, scale := range []int{2, 4} {
		cfg := DefaultConfig(31, 19)
		cfg.Supersampling = scale
		cfg.Background = color.NRGBA{R: 30, G: 90, B: 170, A: 75}
		shared, err := NewRenderer(cfg)
		if err != nil {
			t.Fatal(err)
		}
		var references [3][32]byte
		for frame := range references {
			in := NewInstance(m)
			in.Model = geom.Rotation(geom.Vec3{Y: 1}, float64(frame)*.37)
			img, err := shared.Render(camera, in)
			if err != nil {
				t.Fatal(err)
			}
			references[frame] = sha256.Sum256(img.Pix)
		}
		var wg sync.WaitGroup
		errors := make(chan error, 8)
		for worker := 0; worker < 8; worker++ {
			wg.Add(1)
			go func(worker int) {
				defer wg.Done()
				r := shared
				if worker%2 == 0 {
					var err error
					r, err = NewRenderer(cfg)
					if err != nil {
						errors <- err
						return
					}
				}
				for i := 0; i < 6; i++ {
					frame := (i + worker) % len(references)
					in := NewInstance(m)
					in.Model = geom.Rotation(geom.Vec3{Y: 1}, float64(frame)*.37)
					img, err := r.Render(camera, in)
					if err != nil {
						errors <- err
						return
					}
					if sha256.Sum256(img.Pix) != references[frame] {
						errors <- fmt.Errorf("scale %d worker %d frame %d differs", scale, worker, frame)
						return
					}
					// Mutation must not reach any sample or another returned frame.
					img.Pix[0] ^= 255
				}
			}(worker)
		}
		wg.Wait()
		close(errors)
		for err := range errors {
			t.Error(err)
		}
	}
}

func BenchmarkRenderSphereSupersampling(b *testing.B) {
	m, err := Sphere(1, 64, 32)
	if err != nil {
		b.Fatal(err)
	}
	camera := testCamera(Perspective)
	camera.Position = geom.Vec3{Z: 3}
	camera.Target = geom.Vec3{}
	in := NewInstance(m)
	for _, scale := range []int{1, 2, 4} {
		b.Run(fmt.Sprintf("%dx", scale), func(b *testing.B) {
			cfg := DefaultConfig(320, 180)
			cfg.Supersampling = scale
			r, err := NewRenderer(cfg)
			if err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if _, err := r.Render(camera, in); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
