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
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
)

// Independent front-to-back transmittance reference. Inputs are unassociated
// shaded colors, nearest first; accumulation stays premultiplied until the
// single quantization at the end. The implementation instead uses far-to-near
// source-over, so this checks both ordering and alpha arithmetic.
func referenceTransparentLayers(background color.NRGBA, nearest ...color.NRGBA) color.RGBA {
	var sum [4]float64
	transmittance := 1.0
	for _, c := range nearest {
		weight := transmittance * float64(c.A) / 255
		sum[0] += float64(c.R) * weight
		sum[1] += float64(c.G) * weight
		sum[2] += float64(c.B) * weight
		sum[3] += 255 * weight
		transmittance *= 1 - float64(c.A)/255
	}
	bg := color.RGBAModel.Convert(background).(color.RGBA)
	for i, channel := range [4]uint8{bg.R, bg.G, bg.B, bg.A} {
		sum[i] += float64(channel) * transmittance
	}
	return color.RGBA{R: uint8(math.Round(sum[0])), G: uint8(math.Round(sum[1])), B: uint8(math.Round(sum[2])), A: uint8(math.Round(sum[3]))}
}

func TestTransparentPremultipliedSourceOver(t *testing.T) {
	for _, projection := range []Projection{Perspective, Orthographic} {
		for _, scale := range []int{1, 2, 4} {
			for _, bgAlpha := range []uint8{0, 93, 255} {
				for _, alpha := range []uint8{0, 1, 64, 128, 254} {
					cfg := DefaultConfig(7, 5)
					cfg.Supersampling = scale
					cfg.Background = color.NRGBA{R: 43, G: 165, B: 233, A: bgAlpha}
					cfg.Light = Light{Ambient: 1}
					r, err := NewRenderer(cfg)
					if err != nil {
						t.Fatal(err)
					}
					base := color.NRGBA{R: 201, G: 101, B: 53, A: alpha}
					img, err := r.Render(testCamera(projection), colored(coverageQuad(50, 20, 2), base))
					if err != nil {
						t.Fatal(err)
					}
					want := referenceTransparentLayers(cfg.Background, base)
					for y := 0; y < cfg.Height; y++ {
						for x := 0; x < cfg.Width; x++ {
							got := img.RGBAAt(x, y)
							if got != want || got.R > got.A || got.G > got.A || got.B > got.A {
								t.Fatalf("projection=%d scale=%d alpha=%d bg=%d pixel (%d,%d): %v want %v", projection, scale, alpha, bgAlpha, x, y, got, want)
							}
						}
					}
				}
			}
		}
	}
	// The returned image must also composite correctly in Go's bitmap pipeline.
	cfg := DefaultConfig(1, 1)
	cfg.Light = Light{Ambient: 1}
	r, err := NewRenderer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	img, err := r.Render(testCamera(Orthographic), colored(coverageQuad(8, 4, 2), color.NRGBA{R: 255, A: 128}))
	if err != nil {
		t.Fatal(err)
	}
	canvas := image.NewRGBA(img.Bounds())
	draw.Draw(canvas, canvas.Bounds(), image.White, image.Point{}, draw.Src)
	draw.Draw(canvas, canvas.Bounds(), img, image.Point{}, draw.Over)
	if got := canvas.RGBAAt(0, 0); got != (color.RGBA{R: 255, G: 127, B: 127, A: 255}) {
		t.Fatalf("transparent red over white: %v", got)
	}
}

func TestTransparentOpaqueOcclusionAndDepthTies(t *testing.T) {
	for _, projection := range []Projection{Perspective, Orthographic} {
		for _, scale := range []int{1, 2, 4} {
			cfg := DefaultConfig(7, 5)
			cfg.Supersampling = scale
			cfg.Light = Light{Ambient: 1}
			// Exactly enough for the front transparent layer. Hidden and
			// coplanar fragments must be rejected before consuming any budget.
			cfg.TransparentFragmentLimit = 7 * 5 * scale * scale
			r, err := NewRenderer(cfg)
			if err != nil {
				t.Fatal(err)
			}
			middle := coverageQuad(50, 20, 3)
			instances := []Instance{
				colored(coverageQuad(50, 20, 2), color.NRGBA{R: 255, A: 128}),
				colored(middle, green),
				colored(coverageQuad(50, 20, 4), color.NRGBA{B: 255, A: 200}),
				colored(middle, color.NRGBA{B: 255, A: 128}),
				colored(coverageQuad(50, 20, 1.5), color.NRGBA{R: 255, B: 255}),
			}
			for shift := range instances {
				ordered := append(slices.Clone(instances[shift:]), instances[:shift]...)
				for _, reverse := range []bool{false, true} {
					if reverse {
						slices.Reverse(ordered)
					}
					img, err := r.Render(testCamera(projection), ordered...)
					if err != nil {
						t.Fatal(err)
					}
					for y := 0; y < cfg.Height; y++ {
						for x := 0; x < cfg.Width; x++ {
							if got := img.RGBAAt(x, y); got != (color.RGBA{R: 128, G: 127, A: 255}) {
								t.Fatalf("projection=%d scale=%d shift=%d reverse=%t at (%d,%d): %v", projection, scale, shift, reverse, x, y, got)
							}
						}
					}
				}
			}
		}
	}
}

type transparentRayLayer struct {
	depth float64
	color color.NRGBA
}

func TestTransparentCrossingTrianglesAgainstRays(t *testing.T) {
	const width, height = 31, 23
	for _, projection := range []Projection{Perspective, Orthographic} {
		a := perspectiveTriangle()
		b := &Mesh{Vertices: []geom.Vec3{{X: -2.4, Y: -2.4, Z: -3}, {X: 2.4, Y: -2.4, Z: -3}, {Y: 2.4, Z: -3}}, Triangles: []Triangle{{0, 1, 2}}}
		if projection == Orthographic {
			a.Vertices = []geom.Vec3{{X: -.8, Y: -.8, Z: -1}, {X: .8, Y: -.8, Z: -8}, {Y: .8, Z: -8}}
			b.Vertices = []geom.Vec3{{X: -.8, Y: -.8, Z: -3}, {X: .8, Y: -.8, Z: -3}, {Y: .8, Z: -3}}
		}
		colors := []color.NRGBA{{R: 255, G: 31, A: 128}, {G: 43, B: 255, A: 96}}
		for _, scale := range []int{1, 2, 4} {
			cfg := DefaultConfig(width, height)
			cfg.Supersampling = scale
			cfg.Background = color.NRGBA{R: 41, G: 163, B: 233, A: 73}
			cfg.Light = Light{Ambient: 1}
			r, err := NewRenderer(cfg)
			if err != nil {
				t.Fatal(err)
			}
			camera := testCamera(projection)
			camera.OrthoHeight = 2
			img, err := r.Render(camera, colored(a, colors[0]), colored(b, colors[1]))
			if err != nil {
				t.Fatal(err)
			}
			reverse, err := r.Render(camera, colored(b, colors[1]), colored(a, colors[0]))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(img.Pix, reverse.Pix) {
				t.Fatalf("projection=%d scale=%d: submission order changed image", projection, scale)
			}
			frontA, frontB := 0, 0
			for y := 0; y < height; y++ {
				for x := 0; x < width; x++ {
					var sum [4]int
					for sy := range scale {
						for sx := range scale {
							px := float64(x) + (float64(sx)+.5)/float64(scale)
							py := float64(y) + (float64(sy)+.5)/float64(scale)
							rx, ry := (2*px/width-1)*width/height, 1-2*py/height
							origin, direction := geom.Vec3{}, geom.Vec3{X: rx, Y: ry, Z: -1}
							if projection == Orthographic {
								origin, direction = geom.Vec3{X: rx, Y: ry}, geom.Vec3{Z: -1}
							}
							var layers []transparentRayLayer
							for i, m := range []*Mesh{a, b} {
								depth, _, hit := rayHit(origin, direction, m)
								if hit && depth >= camera.Near && depth <= camera.Far {
									layers = append(layers, transparentRayLayer{depth, colors[i]})
								}
							}
							slices.SortFunc(layers, func(a, b transparentRayLayer) int {
								if a.depth < b.depth {
									return -1
								}
								return 1
							})
							if len(layers) == 2 {
								if layers[0].color == colors[0] {
									frontA++
								} else {
									frontB++
								}
							}
							var nearest []color.NRGBA
							for _, layer := range layers {
								nearest = append(nearest, layer.color)
							}
							ref := referenceTransparentLayers(cfg.Background, nearest...)
							for c, channel := range [4]uint8{ref.R, ref.G, ref.B, ref.A} {
								sum[c] += int(channel)
							}
						}
					}
					n := scale * scale
					want := color.RGBA{R: uint8((sum[0] + n/2) / n), G: uint8((sum[1] + n/2) / n), B: uint8((sum[2] + n/2) / n), A: uint8((sum[3] + n/2) / n)}
					if got := img.RGBAAt(x, y); got != want {
						t.Fatalf("projection=%d scale=%d pixel (%d,%d): %v want %v", projection, scale, x, y, got, want)
					}
				}
			}
			if frontA < 10 || frontB < 10 {
				t.Fatalf("insufficient crossing overlap: %d A-front, %d B-front", frontA, frontB)
			}
		}
	}
}

func TestTransparentCoplanarColorTieAndTrianglePermutation(t *testing.T) {
	cfg := DefaultConfig(9, 7)
	cfg.Light = Light{Ambient: 1}
	r, err := NewRenderer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	m := coverageQuad(50, 20, 2)
	colors := []color.NRGBA{{R: 255, A: 128}, {G: 255, A: 96}, {B: 255, A: 160}}
	want := referenceTransparentLayers(cfg.Background, colors...)
	for _, order := range [][]int{{0, 1, 2}, {0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0}} {
		img, err := r.Render(testCamera(Orthographic), colored(m, colors[order[0]]), colored(m, colors[order[1]]), colored(m, colors[order[2]]))
		if err != nil {
			t.Fatal(err)
		}
		for y := 0; y < cfg.Height; y++ {
			for x := 0; x < cfg.Width; x++ {
				if got := img.RGBAAt(x, y); got != want {
					t.Fatalf("coplanar permutation %v at (%d,%d): %v want %v", order, x, y, got, want)
				}
			}
		}
	}
	// Different normals give overlapping triangles in the SAME material an
	// observable order. Reversing mesh triangle indices must not change output.
	a, b := perspectiveTriangle(), triangleMesh()
	m = &Mesh{Vertices: append(slices.Clone(a.Vertices), b.Vertices...), Triangles: []Triangle{{0, 1, 2}, {3, 4, 5}}, Normals: []geom.Vec3{{Z: 1}, {Z: 1}, {Z: 1}, {X: 1}, {X: 1}, {X: 1}}}
	cfg.Light = Light{Direction: geom.Vec3{Z: 1}, Diffuse: 1}
	r, err = NewRenderer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	base := color.NRGBA{R: 201, G: 101, B: 53, A: 128}
	first, err := r.Render(testCamera(Perspective), colored(m, base))
	if err != nil {
		t.Fatal(err)
	}
	slices.Reverse(m.Triangles)
	second, err := r.Render(testCamera(Perspective), colored(m, base))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.Pix, second.Pix) || litPixels(first) == 0 {
		t.Fatal("transparent mesh triangle permutation changed output or lost coverage")
	}
}

func TestTransparentFragmentLimitsAndRecovery(t *testing.T) {
	for _, limit := range []int{-1, MaxTransparentFragments + 1, int(^uint(0) >> 1)} {
		cfg := DefaultConfig(1, 1)
		cfg.TransparentFragmentLimit = limit
		if r, err := NewRenderer(cfg); r != nil || err == nil || !strings.Contains(err.Error(), "fragment limit") {
			t.Fatalf("invalid limit %d: renderer=%v error=%v", limit, r, err)
		}
	}
	for _, limit := range []int{0, 1, MaxTransparentFragments} {
		cfg := DefaultConfig(1, 1)
		cfg.TransparentFragmentLimit = limit
		r, err := NewRenderer(cfg)
		if err != nil {
			t.Fatal(err)
		}
		want := limit
		if want == 0 {
			want = DefaultTransparentFragmentLimit
		}
		if r.config.TransparentFragmentLimit != want {
			t.Fatalf("limit %d normalized to %d want %d", limit, r.config.TransparentFragmentLimit, want)
		}
	}
	for _, scale := range []int{1, 2, 4} {
		cfg := DefaultConfig(1, 1)
		cfg.Supersampling = scale
		cfg.Light = Light{Ambient: 1}
		cfg.TransparentFragmentLimit = scale * scale
		r, err := NewRenderer(cfg)
		if err != nil {
			t.Fatal(err)
		}
		in := colored(coverageQuad(50, 20, 2), color.NRGBA{R: 255, A: 128})
		for range 2 {
			good, err := r.Render(testCamera(Orthographic), in)
			if err != nil || good.RGBAAt(0, 0) != (color.RGBA{R: 128, A: 128}) {
				t.Fatalf("exact sample budget at scale %d: image=%v err=%v", scale, good, err)
			}
			bad, err := r.Render(testCamera(Orthographic), in, in)
			if bad != nil || err == nil || !strings.Contains(err.Error(), fmt.Sprintf("budget exceeded (limit %d,", cfg.TransparentFragmentLimit)) {
				t.Fatalf("budget overflow scale=%d: image=%v err=%v", scale, bad, err)
			}
		}
	}
	// Exercise sorting and storage across block boundaries, with a partial final
	// block. No capacity doubling or discarded old fragment allocations is allowed.
	const width, height = 67, 63
	cfg := DefaultConfig(width, height)
	cfg.Light = Light{Ambient: 1}
	cfg.TransparentFragmentLimit = 2 * width * height
	r, err := NewRenderer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	front, back := color.NRGBA{R: 255, A: 128}, color.NRGBA{B: 255, A: 96}
	img, err := r.Render(testCamera(Orthographic), colored(coverageQuad(100, 20, 2), front), colored(coverageQuad(100, 20, 3), back))
	if err != nil {
		t.Fatal(err)
	}
	want := referenceTransparentLayers(cfg.Background, front, back)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			if got := img.RGBAAt(x, y); got != want {
				t.Fatalf("cross-block composition at (%d,%d): %v want %v", x, y, got, want)
			}
		}
	}
	b := &transparentBuffer{limit: transparentBlockSize + 3}
	if reflect.TypeFor[transparentFragment]().Size() != 16 {
		t.Fatal("fragment payload exceeds documented 16-byte record")
	}
	for i := 0; i < b.limit; i++ {
		if err := b.add(0, float64(i), 0xFF000080); err != nil {
			t.Fatal(err)
		}
	}
	allocated := 0
	for _, block := range b.blocks {
		allocated += cap(block)
	}
	if allocated != b.limit || b.add(0, 0, 0) == nil || b.count != b.limit {
		t.Fatalf("fragment memory/count limit: allocated=%d count=%d limit=%d", allocated, b.count, b.limit)
	}
}

func TestTransparentLayersQuantizeOnce(t *testing.T) {
	cfg := DefaultConfig(1, 1)
	cfg.Light = Light{Ambient: 1}
	r, err := NewRenderer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	const count = 512
	in := colored(coverageQuad(50, 20, 2), color.NRGBA{R: 201, G: 101, B: 53, A: 1})
	instances := make([]Instance, count)
	colors := make([]color.NRGBA, count)
	for i := range instances {
		instances[i], colors[i] = in, in.Material.Color
	}
	img, err := r.Render(testCamera(Orthographic), instances...)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := img.RGBAAt(0, 0), referenceTransparentLayers(cfg.Background, colors...); got != want {
		t.Fatalf("512 low-alpha layers quantized between layers: %v want %v", got, want)
	}
}

func TestTransparentClippedSharedEdgesDoNotDoubleBlend(t *testing.T) {
	for _, projection := range []Projection{Perspective, Orthographic} {
		for _, scale := range []int{1, 2, 4} {
			for _, size := range [][2]int{{1, 1}, {7, 5}, {13, 9}, {31, 19}} {
				for _, extent := range []float64{20, 127, 1000} {
					cfg := DefaultConfig(size[0], size[1])
					cfg.Supersampling = scale
					cfg.Light = Light{Ambient: 1}
					cfg.TransparentFragmentLimit = size[0] * size[1] * scale * scale
					r, err := NewRenderer(cfg)
					if err != nil {
						t.Fatal(err)
					}
					in := colored(coverageQuad(4*extent, extent, 2), color.NRGBA{R: 201, G: 101, B: 53, A: 64})
					// Rotation and off-center translation force independently
					// clipped shared edges as well as clipped polygon fan edges.
					in.Model = geom.Translation(geom.Vec3{X: .37, Y: -.23}).Mul(geom.Rotation(geom.Vec3{Z: 1}, .31))
					img, err := r.Render(testCamera(projection), in)
					if err != nil {
						t.Fatalf("projection=%d scale=%d size=%v extent=%g: %v", projection, scale, size, extent, err)
					}
					want := referenceTransparentLayers(cfg.Background, in.Material.Color)
					for y := range size[1] {
						for x := range size[0] {
							if got := img.RGBAAt(x, y); got != want {
								t.Fatalf("projection=%d scale=%d size=%v extent=%g pixel (%d,%d): %v want %v", projection, scale, size, extent, x, y, got, want)
							}
						}
					}
					// Both triangle submission order and winding are immaterial
					// when culling is disabled and vertex normals are supplied.
					in.Mesh.Normals = []geom.Vec3{{Z: 1}, {Z: 1}, {Z: 1}, {Z: 1}}
					in.Mesh.Triangles = []Triangle{{0, 3, 2}, {0, 2, 1}}
					reverse, err := r.Render(testCamera(projection), in)
					if err != nil || !bytes.Equal(img.Pix, reverse.Pix) {
						t.Fatalf("shared-edge winding/order changed image: %v", err)
					}
				}
			}
		}
	}
}

func TestTransparentClippedSmoothLightingAgainstRays(t *testing.T) {
	const width, height = 17, 13
	m := perspectiveTriangle()
	m.Vertices[0] = geom.Vec3{X: -.2, Y: -.2, Z: -.25}
	m.Normals = []geom.Vec3{{Z: 1}, {X: 1}, {Y: 1}}
	base := color.NRGBA{R: 201, G: 111, B: 53, A: 193}
	for _, projection := range []Projection{Perspective, Orthographic} {
		for _, scale := range []int{1, 2, 4} {
			cfg := DefaultConfig(width, height)
			cfg.Supersampling = scale
			cfg.Light = Light{Direction: geom.Vec3{Z: 1}, Ambient: .2, Diffuse: .8}
			cfg.Background = color.NRGBA{R: 41, G: 163, B: 233, A: 73}
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
			hits := 0
			for y := range height {
				for x := range width {
					var sum [4]int
					for sy := range scale {
						for sx := range scale {
							px := float64(x) + (float64(sx)+.5)/float64(scale)
							py := float64(y) + (float64(sy)+.5)/float64(scale)
							rx, ry := (2*px/width-1)*width/height, 1-2*py/height
							origin, direction := geom.Vec3{}, geom.Vec3{X: rx, Y: ry, Z: -1}
							if projection == Orthographic {
								origin, direction = geom.Vec3{X: 2 * rx, Y: 2 * ry}, geom.Vec3{Z: -1}
							}
							depth, weights, hit := rayHit(origin, direction, m)
							ref := color.RGBAModel.Convert(cfg.Background).(color.RGBA)
							if hit && depth >= camera.Near && depth <= camera.Far {
								n := m.Normals[0].Mul(weights[0]).Add(m.Normals[1].Mul(weights[1])).Add(m.Normals[2].Mul(weights[2]))
								n, _ = unit(n)
								intensity := .2 + .8*math.Max(0, n.Z)
								lit := color.NRGBA{R: uint8(math.Round(float64(base.R) * intensity)), G: uint8(math.Round(float64(base.G) * intensity)), B: uint8(math.Round(float64(base.B) * intensity)), A: base.A}
								ref = referenceTransparentLayers(cfg.Background, lit)
								hits++
							}
							for c, channel := range [4]uint8{ref.R, ref.G, ref.B, ref.A} {
								sum[c] += int(channel)
							}
						}
					}
					n := scale * scale
					want := color.RGBA{R: uint8((sum[0] + n/2) / n), G: uint8((sum[1] + n/2) / n), B: uint8((sum[2] + n/2) / n), A: uint8((sum[3] + n/2) / n)}
					if got := img.RGBAAt(x, y); got != want {
						t.Fatalf("clipped smooth normals projection=%d scale=%d pixel (%d,%d): %v want %v", projection, scale, x, y, got, want)
					}
				}
			}
			if hits < 10 {
				t.Fatalf("insufficient clipped smooth-normal coverage: %d", hits)
			}
		}
	}
}

func TestTransparentZeroAlphaStillValidates(t *testing.T) {
	r := ambientRenderer(t)
	in := colored(triangleMesh(), color.NRGBA{})
	in.Model = geom.Translation(geom.Vec3{X: MaxCoordinate}).Mul(geom.Scaling(4, 4, 4))
	if img, err := r.Render(testCamera(Orthographic), in); err == nil || img != nil {
		t.Fatalf("invisible out-of-bounds transformed geometry accepted: %v", err)
	}
}

func TestTransparentReflectionsNormalsAndCulling(t *testing.T) {
	cfg := DefaultConfig(64, 64)
	cfg.Light = Light{Direction: geom.Vec3{Z: 1}, Diffuse: 1}
	cfg.CullBackfaces = true
	r, err := NewRenderer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	m := triangleMesh()
	m.Normals = []geom.Vec3{{X: 1, Z: 1}, {X: 1, Z: 1}, {X: 1, Z: 1}}
	for _, sx := range []float64{2, -2} {
		in := colored(m, color.NRGBA{R: 255, A: 128})
		in.Model = geom.Translation(geom.Vec3{Z: -2}).Mul(geom.Scaling(sx, 1, .5))
		img, err := r.Render(testCamera(Orthographic), in)
		if err != nil {
			t.Fatal(err)
		}
		shade := uint8(math.Round(255 * 2 / math.Hypot(.5, 2)))
		if got, want := img.RGBAAt(32, 32), referenceTransparentLayers(cfg.Background, color.NRGBA{R: shade, A: 128}); got != want {
			t.Fatalf("transparent inverse transpose/reflection sx=%g: %v want %v", sx, got, want)
		}
	}
	m.Triangles[0] = Triangle{0, 2, 1}
	img, err := r.Render(testCamera(Orthographic), colored(m, color.NRGBA{R: 255, A: 128}))
	if err != nil || litPixels(img) != 0 {
		t.Fatalf("transparent backface survived culling: %v", err)
	}
}

func TestTransparentSSAAPerSampleOcclusion(t *testing.T) {
	for _, scale := range []int{2, 4} {
		cfg := DefaultConfig(1, 1)
		cfg.Supersampling = scale
		cfg.Light = Light{Ambient: 1}
		r, err := NewRenderer(cfg)
		if err != nil {
			t.Fatal(err)
		}
		camera := testCamera(Orthographic)
		camera.OrthoHeight = 2
		redGlass := colored(coverageQuad(8, 4, 4), color.NRGBA{R: 255, A: 128})
		greenHalf := colored(coverageQuad(0, 1, 3), green)
		blueGlass := colored(coverageQuad(8, 4, 2), color.NRGBA{B: 255, A: 64})
		img, err := r.Render(camera, redGlass, greenHalf, blueGlass)
		if err != nil {
			t.Fatal(err)
		}
		left := referenceTransparentLayers(cfg.Background, blueGlass.Material.Color, green)
		right := referenceTransparentLayers(cfg.Background, blueGlass.Material.Color, redGlass.Material.Color)
		avg := func(a, b uint8) uint8 { return uint8((int(a) + int(b) + 1) / 2) }
		want := color.RGBA{R: avg(left.R, right.R), G: avg(left.G, right.G), B: avg(left.B, right.B), A: avg(left.A, right.A)}
		if got := img.RGBAAt(0, 0); got != want {
			t.Fatalf("SSAA scale=%d: %v want %v; compositing must precede resolve", scale, got, want)
		}
	}
}

func TestTransparentConcurrentDeterminism(t *testing.T) {
	m, err := Sphere(1, 16, 8)
	if err != nil {
		t.Fatal(err)
	}
	camera := testCamera(Perspective)
	camera.Position, camera.Target = geom.Vec3{Z: 3}, geom.Vec3{}
	for _, scale := range []int{1, 2, 4} {
		cfg := DefaultConfig(31, 19)
		cfg.Supersampling = scale
		cfg.Background = color.NRGBA{R: 30, G: 90, B: 170, A: 75}
		shared, err := NewRenderer(cfg)
		if err != nil {
			t.Fatal(err)
		}
		makeFrame := func(r *Renderer, frame int) (*image.RGBA, error) {
			in := colored(m, color.NRGBA{R: 201, G: 101, B: 53, A: uint8(70 + frame*43)})
			in.Model = geom.Rotation(geom.Vec3{Y: 1}, float64(frame)*.37)
			return r.Render(camera, in)
		}
		var references [3][32]byte
		for frame := range references {
			img, err := makeFrame(shared, frame)
			if err != nil {
				t.Fatal(err)
			}
			references[frame] = sha256.Sum256(img.Pix)
		}
		var wg sync.WaitGroup
		errors := make(chan error, 8)
		for worker := range 8 {
			wg.Add(1)
			go func() {
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
				for i := range 6 {
					frame := (i + worker) % len(references)
					img, err := makeFrame(r, frame)
					if err != nil {
						errors <- err
						return
					}
					if sha256.Sum256(img.Pix) != references[frame] {
						errors <- fmt.Errorf("transparent scale=%d worker=%d frame=%d differs", scale, worker, frame)
						return
					}
					img.Pix[0] ^= 255
				}
			}()
		}
		wg.Wait()
		close(errors)
		for err := range errors {
			t.Error(err)
		}
	}
}

func BenchmarkRenderTransparentSphere(b *testing.B) {
	m, err := Sphere(1, 64, 32)
	if err != nil {
		b.Fatal(err)
	}
	camera := testCamera(Perspective)
	camera.Position, camera.Target = geom.Vec3{Z: 3}, geom.Vec3{}
	in := colored(m, color.NRGBA{R: 201, G: 101, B: 53, A: 128})
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
