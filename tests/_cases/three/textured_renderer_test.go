package three

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"gomath/geom"
	"image"
	"image/color"
	"math"
	"slices"
	"strings"
	"sync"
	"testing"
)

func uvQuad(x, y, depth float64) *Mesh {
	return &Mesh{
		Vertices:  []geom.Vec3{{X: -x, Y: -y, Z: -depth}, {X: x, Y: -y, Z: -depth}, {X: x, Y: y, Z: -depth}, {X: -x, Y: y, Z: -depth}},
		Triangles: []Triangle{{0, 1, 2}, {0, 2, 3}},
		UVs:       []UV{{U: 0, V: 1}, {U: 1, V: 1}, {U: 1, V: 0}, {}},
	}
}

func textureReferenceFrame(cfg Config, camera Camera, sample func(origin, direction geom.Vec3) color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, cfg.Width, cfg.Height))
	scale := max(1, cfg.Supersampling)
	for y := range cfg.Height {
		for x := range cfg.Width {
			var sum [4]int
			for sy := range scale {
				for sx := range scale {
					px := float64(x) + (float64(sx)+.5)/float64(scale)
					py := float64(y) + (float64(sy)+.5)/float64(scale)
					rx, ry := (2*px/float64(cfg.Width)-1)*float64(cfg.Width)/float64(cfg.Height), 1-2*py/float64(cfg.Height)
					origin, direction := geom.Vec3{}, geom.Vec3{X: rx * math.Tan(camera.FOVY/2), Y: ry * math.Tan(camera.FOVY/2), Z: -1}
					if camera.Projection == Orthographic {
						origin, direction = geom.Vec3{X: rx * camera.OrthoHeight / 2, Y: ry * camera.OrthoHeight / 2}, geom.Vec3{Z: -1}
					}
					c := sample(origin, direction)
					for i, ch := range [4]uint8{c.R, c.G, c.B, c.A} {
						sum[i] += int(ch)
					}
				}
			}
			n := scale * scale
			img.SetRGBA(x, y, color.RGBA{R: uint8((sum[0] + n/2) / n), G: uint8((sum[1] + n/2) / n), B: uint8((sum[2] + n/2) / n), A: uint8((sum[3] + n/2) / n)})
		}
	}
	return img
}

func requireTextureFrame(t *testing.T, got, want *image.RGBA) {
	t.Helper()
	if got.Bounds() != want.Bounds() {
		t.Fatalf("bounds %v want %v", got.Bounds(), want.Bounds())
	}
	for y := range want.Bounds().Dy() {
		for x := range want.Bounds().Dx() {
			if a, b := got.RGBAAt(x, y), want.RGBAAt(x, y); a != b {
				t.Fatalf("pixel (%d,%d): %v want %v", x, y, a, b)
			}
		}
	}
}

func TestTexturedSlantedUVAndClippingAgainstRays(t *testing.T) {
	for _, pattern := range []string{"checker", "UV-gradient"} {
		t.Run(pattern, func(t *testing.T) {
			texture := testTexture(t, 16, 16, func(x, y int) color.NRGBA {
				if pattern == "checker" {
					if (x/2+y/2)%2 == 0 {
						return color.NRGBA{R: 240, G: 220, B: 80, A: 255}
					}
					return color.NRGBA{R: 20, G: 40, B: 170, A: 255}
				}
				return color.NRGBA{R: uint8(17 * x), G: uint8(17 * y), B: 53, A: 255}
			})
			for _, projection := range []Projection{Perspective, Orthographic} {
				for _, scale := range []int{1, 2, 4} {
					for _, filter := range []TextureFilter{Nearest, Bilinear} {
						for _, address := range []TextureAddress{Clamp, Repeat} {
							for _, firstZ := range []float64{-1, -.25, .25} {
								t.Run(fmt.Sprintf("p%d/s%d/f%d/a%d/z%g", projection, scale, filter, address, firstZ), func(t *testing.T) {
									cfg := DefaultConfig(31, 23)
									cfg.Supersampling = scale
									cfg.Light = Light{Ambient: 1}
									cfg.Background = color.NRGBA{R: 43, G: 165, B: 233, A: 73}
									// An all-opaque texture must never consume a transparent budget.
									cfg.TransparentFragmentLimit = 1
									r, err := NewRenderer(cfg)
									if err != nil {
										t.Fatal(err)
									}
									m := perspectiveTriangle()
									if firstZ != -1 {
										m.Vertices[0] = geom.Vec3{X: -.2, Y: -.2, Z: firstZ}
									}
									m.UVs = []UV{{U: -.13, V: .17}, {U: 3.71, V: .43}, {U: .67, V: 2.61}}
									in := NewInstance(m)
									in.Material.Texture = texture
									in.Material.Sampler = Sampler{Filter: filter, AddressU: address, AddressV: address}
									camera := testCamera(projection)
									if firstZ != -1 {
										camera.Far = 6
									}
									img, err := r.Render(camera, in)
									if err != nil {
										t.Fatal(err)
									}
									hits := 0
									want := textureReferenceFrame(cfg, camera, func(origin, direction geom.Vec3) color.RGBA {
										depth, weights, hit := rayHit(origin, direction, m)
										if !hit || depth < camera.Near || depth > camera.Far {
											return color.RGBAModel.Convert(cfg.Background).(color.RGBA)
										}
										// World-space ray barycentrics supply the independent UV
										// reference. No projected/screen weights are reused here.
										var uv UV
										for i, weight := range weights {
											uv.U += weight * m.UVs[i].U
											uv.V += weight * m.UVs[i].V
										}
										texel, err := texture.Sample(uv, in.Material.Sampler)
										if err != nil {
											t.Fatal(err)
										}
										hits++
										return color.RGBA{R: texel.R, G: texel.G, B: texel.B, A: 255}
									})
									requireTextureFrame(t, img, want)
									if hits < 20 {
										t.Fatalf("insufficient slanted/clipped coverage: %d", hits)
									}
									// Untextured authored UVs must not alter existing rendering.
									in.Material.Texture = nil
									withUV, err := r.Render(camera, in)
									if err != nil {
										t.Fatal(err)
									}
									m.UVs = nil
									withoutUV, err := r.Render(camera, in)
									if err != nil || !bytes.Equal(withUV.Pix, withoutUV.Pix) {
										t.Fatalf("UVs changed untextured fast path: %v", err)
									}
								})
							}
						}
					}
				}
			}
		})
	}
}

type texturedRayLayer struct {
	depth float64
	rgb   color.NRGBA
	alpha float64
}

// Reference retains product alpha and accumulates front-to-back transmittance;
// renderer sorts far-to-near and uses source-over instead.
func referenceTextureLayers(bg color.NRGBA, nearest ...texturedRayLayer) color.RGBA {
	var sum [4]float64
	remaining := 1.0
	for _, layer := range nearest {
		weight := remaining * layer.alpha
		sum[0] += float64(layer.rgb.R) * weight
		sum[1] += float64(layer.rgb.G) * weight
		sum[2] += float64(layer.rgb.B) * weight
		sum[3] += 255 * weight
		remaining *= 1 - layer.alpha
	}
	base := color.RGBAModel.Convert(bg).(color.RGBA)
	for i, ch := range [4]uint8{base.R, base.G, base.B, base.A} {
		sum[i] += float64(ch) * remaining
	}
	return color.RGBA{R: uint8(math.Round(sum[0])), G: uint8(math.Round(sum[1])), B: uint8(math.Round(sum[2])), A: uint8(math.Round(sum[3]))}
}

func TestTexturedMixedAlphaOpaqueMaskingAndSSAA(t *testing.T) {
	texture := testTexture(t, 3, 1, func(x, y int) color.NRGBA { return color.NRGBA{R: 255, A: []uint8{255, 128, 0}[x]} })
	for _, scale := range []int{1, 2, 4} {
		for _, materialAlpha := range []uint8{255, 128} {
			cfg := DefaultConfig(3, 1)
			cfg.Light = Light{Ambient: 1}
			cfg.Supersampling = scale
			if materialAlpha == 255 {
				// One partial red cell and two green cells survive the opaque mask.
				cfg.TransparentFragmentLimit = 3 * scale * scale
			}
			r, err := NewRenderer(cfg)
			if err != nil {
				t.Fatal(err)
			}
			front := colored(uvQuad(3, 1, 2), color.NRGBA{R: 255, G: 255, B: 255, A: materialAlpha})
			front.Material.Texture = texture
			middle := colored(coverageQuad(50, 20, 2.5), color.NRGBA{G: 255, A: 128})
			back := colored(coverageQuad(50, 20, 3), color.NRGBA{B: 255, A: 255})
			camera := testCamera(Orthographic)
			camera.OrthoHeight = 2
			instances := []Instance{front, middle, back}
			for _, order := range [][]int{{0, 1, 2}, {0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0}} {
				img, err := r.Render(camera, instances[order[0]], instances[order[1]], instances[order[2]])
				if err != nil {
					t.Fatal(err)
				}
				for x, alpha := range []uint8{255, 128, 0} {
					want := referenceTextureLayers(cfg.Background,
						texturedRayLayer{rgb: color.NRGBA{R: 255}, alpha: float64(alpha) / 255 * float64(materialAlpha) / 255},
						texturedRayLayer{rgb: color.NRGBA{G: 255}, alpha: 128.0 / 255},
						texturedRayLayer{rgb: color.NRGBA{B: 255}, alpha: 1})
					if got := img.RGBAAt(x, 0); got != want {
						t.Fatalf("scale=%d material-alpha=%d order=%v x=%d: %v want %v", scale, materialAlpha, order, x, got, want)
					}
				}
			}
		}
	}
}

func TestTexturedCrossingTransparencyAgainstRays(t *testing.T) {
	texture := testTexture(t, 4, 4, func(x, y int) color.NRGBA {
		return color.NRGBA{R: uint8(50 + 60*x), G: uint8(40 + 50*y), B: 233, A: []uint8{0, 90, 171, 255}[(x+y)%4]}
	})
	for _, projection := range []Projection{Perspective, Orthographic} {
		for _, scale := range []int{1, 2, 4} {
			cfg := DefaultConfig(31, 23)
			cfg.Light = Light{Ambient: 1}
			cfg.Supersampling = scale
			cfg.Background = color.NRGBA{R: 41, G: 163, B: 233, A: 73}
			r, err := NewRenderer(cfg)
			if err != nil {
				t.Fatal(err)
			}
			a := perspectiveTriangle()
			b := &Mesh{Vertices: []geom.Vec3{{X: -2.4, Y: -2.4, Z: -3}, {X: 2.4, Y: -2.4, Z: -3}, {Y: 2.4, Z: -3}}, Triangles: []Triangle{{0, 1, 2}}}
			if projection == Orthographic {
				a.Vertices = []geom.Vec3{{X: -.8, Y: -.8, Z: -1}, {X: .8, Y: -.8, Z: -8}, {Y: .8, Z: -8}}
				b.Vertices = []geom.Vec3{{X: -.8, Y: -.8, Z: -3}, {X: .8, Y: -.8, Z: -3}, {Y: .8, Z: -3}}
			}
			a.UVs = []UV{{U: -.13, V: .17}, {U: 1.71, V: .43}, {U: .67, V: 1.61}}
			b.UVs = []UV{{U: .17, V: .21}, {U: .93, V: .34}, {U: .41, V: .87}}
			instances := []Instance{NewInstance(a), NewInstance(b)}
			for i := range instances {
				instances[i].Material.Texture = texture
				instances[i].Material.Color.A = []uint8{193, 255}[i]
				instances[i].Material.Sampler = Sampler{AddressU: Repeat, AddressV: Repeat}
			}
			camera := testCamera(projection)
			camera.OrthoHeight = 2
			img, err := r.Render(camera, instances...)
			if err != nil {
				t.Fatal(err)
			}
			reverse, err := r.Render(camera, instances[1], instances[0])
			if err != nil || !bytes.Equal(img.Pix, reverse.Pix) {
				t.Fatalf("textured submission order changed frame: %v", err)
			}
			want := textureReferenceFrame(cfg, camera, func(origin, direction geom.Vec3) color.RGBA {
				var layers []texturedRayLayer
				for _, in := range instances {
					depth, weights, hit := rayHit(origin, direction, in.Mesh)
					if !hit || depth < camera.Near || depth > camera.Far {
						continue
					}
					var uv UV
					for i, weight := range weights {
						uv.U += weight * in.Mesh.UVs[i].U
						uv.V += weight * in.Mesh.UVs[i].V
					}
					texel, err := texture.Sample(uv, in.Material.Sampler)
					if err != nil {
						t.Fatal(err)
					}
					layers = append(layers, texturedRayLayer{depth, texel, float64(texel.A) / 255 * float64(in.Material.Color.A) / 255})
				}
				slices.SortFunc(layers, func(a, b texturedRayLayer) int {
					if a.depth < b.depth {
						return -1
					}
					if a.depth > b.depth {
						return 1
					}
					return 0
				})
				return referenceTextureLayers(cfg.Background, layers...)
			})
			requireTextureFrame(t, img, want)
		}
	}
}

func TestTexturedAlphaPrecisionAndBudgetRecovery(t *testing.T) {
	cfg := DefaultConfig(1, 1)
	cfg.Light = Light{Ambient: 1}
	cfg.TransparentFragmentLimit = 512
	r, err := NewRenderer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	texture := testTexture(t, 1, 1, func(x, y int) color.NRGBA { return color.NRGBA{R: 255, A: 1} })
	in := colored(uvQuad(2, 2, 2), color.NRGBA{R: 255, G: 255, B: 255, A: 1})
	in.Material.Texture = texture
	instances := make([]Instance, 512)
	for i := range instances {
		instances[i] = in
	}
	img, err := r.Render(testCamera(Orthographic), instances...)
	if err != nil {
		t.Fatal(err)
	}
	want := uint8(math.Round(255 * (1 - math.Pow(1-1.0/(255*255), 512))))
	if got := img.RGBAAt(0, 0); want == 0 || got != (color.RGBA{R: want, A: want}) {
		t.Fatalf("texture*material alpha rounded before composition: %v want R=A=%d", got, want)
	}
	// Almost-opaque bilinear alpha must not round to opaque before depth writes.
	texture = testTexture(t, 2, 1, func(x, y int) color.NRGBA { return color.NRGBA{R: 255, A: []uint8{255, 0}[x]} })
	in = NewInstance(uvQuad(2, 2, 2))
	for i := range in.Mesh.UVs {
		in.Mesh.UVs[i] = UV{U: .25000001, V: .5}
	}
	in.Material.Texture, in.Material.Sampler = texture, Sampler{Filter: Bilinear}
	cfg.TransparentFragmentLimit = 1
	r, err = NewRenderer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if bad, err := r.Render(testCamera(Orthographic), in, in); bad != nil || err == nil || !strings.Contains(err.Error(), "budget exceeded") {
			t.Fatalf("nearly opaque texels bypassed transparent budget: image=%v err=%v", bad, err)
		}
		if good, err := r.Render(testCamera(Orthographic), in); err != nil || good.RGBAAt(0, 0) != (color.RGBA{R: 255, A: 255}) {
			t.Fatalf("render failed to recover after textured budget error: image=%v err=%v", good, err)
		}
	}
}

func TestTexturedBilinearAlphaMaterialTintAndLighting(t *testing.T) {
	texture := testTexture(t, 2, 1, func(x, y int) color.NRGBA {
		if x == 0 {
			return color.NRGBA{R: 255, B: 100, A: 255}
		}
		return color.NRGBA{G: 255}
	})
	for _, scale := range []int{1, 2, 4} {
		cfg := DefaultConfig(1, 1)
		cfg.Light = Light{Ambient: .5}
		cfg.Supersampling = scale
		cfg.Background = color.NRGBA{R: 43, G: 165, B: 233, A: 73}
		r, err := NewRenderer(cfg)
		if err != nil {
			t.Fatal(err)
		}
		in := colored(uvQuad(2, 2, 2), color.NRGBA{R: 201, G: 101, B: 53, A: 128})
		for i := range in.Mesh.UVs {
			in.Mesh.UVs[i] = UV{U: .5, V: .5}
		}
		in.Material.Texture, in.Material.Sampler = texture, Sampler{Filter: Bilinear}
		img, err := r.Render(testCamera(Orthographic), in)
		if err != nil {
			t.Fatal(err)
		}
		lit := color.NRGBA{R: 101, B: uint8(math.Round(100 * 53.0 / 255 * .5))}
		want := referenceTextureLayers(cfg.Background, texturedRayLayer{rgb: lit, alpha: .5 * 128 / 255})
		if got := img.RGBAAt(0, 0); got != want {
			t.Fatalf("filtered alpha, tint, light scale=%d: %v want %v", scale, got, want)
		}
	}
}

func TestTexturedBilinearAlphaPerSampleOcclusion(t *testing.T) {
	texture := testTexture(t, 2, 1, func(x, y int) color.NRGBA {
		if x == 0 {
			return color.NRGBA{R: 255, A: 255}
		}
		return color.NRGBA{G: 255}
	})
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
		front := NewInstance(uvQuad(1, 1, 2))
		front.Material.Texture, front.Material.Sampler = texture, Sampler{Filter: Bilinear}
		middle := colored(coverageQuad(0, 2, 3), color.NRGBA{B: 255, A: 255})
		back := colored(coverageQuad(8, 4, 4), color.NRGBA{G: 255, A: 128})
		img, err := r.Render(camera, back, front, middle)
		if err != nil {
			t.Fatal(err)
		}
		want := textureReferenceFrame(cfg, camera, func(origin, direction geom.Vec3) color.RGBA {
			u := (origin.X + 1) / 2
			alpha := max(0, min(1, 1.5-2*u))
			layers := []texturedRayLayer{{rgb: color.NRGBA{R: 255}, alpha: alpha}}
			if origin.X < 0 {
				layers = append(layers, texturedRayLayer{rgb: color.NRGBA{B: 255}, alpha: 1})
			}
			layers = append(layers, texturedRayLayer{rgb: color.NRGBA{G: 255}, alpha: 128.0 / 255})
			return referenceTextureLayers(cfg.Background, layers...)
		})
		requireTextureFrame(t, img, want)
	}
}

func TestTexturedClippedSharedEdgesAndBudget(t *testing.T) {
	texture := testTexture(t, 1, 1, func(x, y int) color.NRGBA { return color.NRGBA{R: 255, G: 255, B: 255, A: 128} })
	for _, projection := range []Projection{Perspective, Orthographic} {
		for _, scale := range []int{1, 2, 4} {
			cfg := DefaultConfig(7, 5)
			cfg.Supersampling = scale
			cfg.Light = Light{Ambient: 1}
			cfg.TransparentFragmentLimit = 7 * 5 * scale * scale
			r, err := NewRenderer(cfg)
			if err != nil {
				t.Fatal(err)
			}
			m := coverageQuad(80, 20, 2)
			m.UVs = make([]UV, len(m.Vertices))
			for i := range m.UVs {
				m.UVs[i] = UV{U: MaxUVCoordinate, V: -MaxUVCoordinate}
			}
			in := colored(m, color.NRGBA{R: 255, G: 255, B: 255, A: 64})
			in.Material.Texture = texture
			in.Material.Sampler = Sampler{Filter: Bilinear, AddressU: Repeat, AddressV: Repeat}
			in.Model = geom.Translation(geom.Vec3{X: .37, Y: -.23}).Mul(geom.Rotation(geom.Vec3{Z: 1}, .31))
			img, err := r.Render(testCamera(projection), in)
			if err != nil {
				t.Fatal(err)
			}
			want := referenceTextureLayers(cfg.Background, texturedRayLayer{rgb: in.Material.Color, alpha: 128.0 / 255 * 64 / 255})
			for y := range cfg.Height {
				for x := range cfg.Width {
					if got := img.RGBAAt(x, y); got != want {
						t.Fatalf("clipped shared edge p=%d scale=%d at (%d,%d): %v want %v", projection, scale, x, y, got, want)
					}
				}
			}
			slices.Reverse(m.Triangles)
			reverse, err := r.Render(testCamera(projection), in)
			if err != nil || !bytes.Equal(img.Pix, reverse.Pix) {
				t.Fatalf("clipped texture triangle permutation changed frame: %v", err)
			}
			// Constants and textures consume the same per-frame budget.
			plain := colored(coverageQuad(80, 20, 1.5), color.NRGBA{G: 255, A: 128})
			if bad, err := r.Render(testCamera(projection), in, plain); bad != nil || err == nil || !strings.Contains(err.Error(), "budget exceeded") {
				t.Fatalf("mixed constants/textures escaped shared fragment limit: %v", err)
			}
		}
	}
}

func TestTexturedReflectionCullingAndInverseTransposeNormals(t *testing.T) {
	texture := testTexture(t, 8, 8, func(x, y int) color.NRGBA { return color.NRGBA{R: uint8(x * 30), G: uint8(y * 30), A: 255} })
	cfg := DefaultConfig(8, 8)
	cfg.Light, cfg.CullBackfaces = Light{Ambient: 1}, true
	r, err := NewRenderer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, alpha := range []uint8{255, 128} {
		in := colored(uvQuad(2, 2, 2), color.NRGBA{R: 255, G: 255, B: 255, A: alpha})
		in.Material.Texture = texture
		normal, err := r.Render(testCamera(Orthographic), in)
		if err != nil {
			t.Fatal(err)
		}
		in.Model = geom.Scaling(-1, 1, 1)
		reflected, err := r.Render(testCamera(Orthographic), in)
		if err != nil {
			t.Fatal(err)
		}
		for y := range 8 {
			for x := range 8 {
				if got, want := reflected.RGBAAt(x, y), normal.RGBAAt(7-x, y); got != want || got.A != alpha {
					t.Fatalf("reflected UV or winding at (%d,%d): %v want %v", x, y, got, want)
				}
			}
		}
	}
	cfg = DefaultConfig(64, 64)
	cfg.Light, cfg.CullBackfaces = Light{Direction: geom.Vec3{Z: 1}, Diffuse: 1}, true
	r, err = NewRenderer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	white := testTexture(t, 1, 1, func(x, y int) color.NRGBA { return color.NRGBA{R: 255, A: 128} })
	m := triangleMesh()
	m.UVs = make([]UV, len(m.Vertices))
	m.Normals = []geom.Vec3{{X: 1, Z: 1}, {X: 1, Z: 1}, {X: 1, Z: 1}}
	for _, sx := range []float64{2, -2} {
		in := NewInstance(m)
		in.Material.Texture = white
		in.Model = geom.Translation(geom.Vec3{Z: -2}).Mul(geom.Scaling(sx, 1, .5))
		img, err := r.Render(testCamera(Orthographic), in)
		if err != nil {
			t.Fatal(err)
		}
		shade := uint8(math.Round(255 * 2 / math.Hypot(.5, 2)))
		if got, want := img.RGBAAt(32, 32), referenceTransparentLayers(cfg.Background, color.NRGBA{R: shade, A: 128}); got != want {
			t.Fatalf("textured inverse transpose sx=%g: %v want %v", sx, got, want)
		}
	}
	m.Triangles[0] = Triangle{0, 2, 1}
	in := NewInstance(m)
	in.Material.Texture = white
	if img, err := r.Render(testCamera(Orthographic), in); err != nil || litPixels(img) != 0 {
		t.Fatalf("textured backface survived culling: %v", err)
	}
}

func TestTexturedSplitVertexSeamAndRepeat(t *testing.T) {
	texture := testTexture(t, 2, 1, func(x, y int) color.NRGBA {
		if x == 0 {
			return color.NRGBA{R: 255, A: 255}
		}
		return color.NRGBA{B: 255, A: 255}
	})
	m := uvQuad(2, 2, 2)
	// Split the shared diagonal's two vertices to select independent UVs on
	// either side; a shared position index alone cannot represent this seam.
	m.Vertices = append(m.Vertices, m.Vertices[0], m.Vertices[2])
	m.Triangles = []Triangle{{0, 1, 2}, {4, 5, 3}}
	m.UVs = []UV{{U: .25, V: .5}, {U: .25, V: .5}, {U: .25, V: .5}, {U: .75, V: .5}, {U: .75, V: .5}, {U: .75, V: .5}}
	in := colored(m, color.NRGBA{R: 255, G: 255, B: 255, A: 128})
	in.Material.Texture = texture
	for _, scale := range []int{1, 2, 4} {
		cfg := DefaultConfig(8, 8)
		cfg.Supersampling = scale
		cfg.Light = Light{Ambient: 1}
		r, err := NewRenderer(cfg)
		if err != nil {
			t.Fatal(err)
		}
		img, err := r.Render(testCamera(Orthographic), in)
		if err != nil {
			t.Fatal(err)
		}
		for y := range 8 {
			for x := range 8 {
				if got := img.RGBAAt(x, y); got.A != 128 || got.G != 0 || int(got.R)+int(got.B) < 128 || int(got.R)+int(got.B) > 129 {
					t.Fatalf("split seam gap or double blend scale=%d (%d,%d): %v", scale, x, y, got)
				}
			}
		}
		if img.RGBAAt(0, 0).B != 128 || img.RGBAAt(7, 7).R != 128 {
			t.Fatal("split vertex UVs did not select opposite sides of seam")
		}
	}
	// Repeat operates after interpolating unwrapped UVs, across the 1.0 seam.
	m = uvQuad(8, 2, 2)
	m.UVs = []UV{{U: .75, V: 1}, {U: 1.25, V: 1}, {U: 1.25}, {U: .75}}
	in = NewInstance(m)
	in.Material.Texture, in.Material.Sampler = texture, Sampler{AddressU: Repeat}
	cfg := DefaultConfig(4, 1)
	cfg.Light = Light{Ambient: 1}
	r, err := NewRenderer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	img, err := r.Render(testCamera(Orthographic), in)
	if err != nil {
		t.Fatal(err)
	}
	for x := range 4 {
		want := color.RGBA{B: 255, A: 255}
		if x >= 2 {
			want = color.RGBA{R: 255, A: 255}
		}
		if got := img.RGBAAt(x, 0); got != want {
			t.Fatalf("repeat seam x=%d: %v want %v", x, got, want)
		}
	}
}

func TestTexturedSharedTextureConcurrentDeterminism(t *testing.T) {
	src := image.NewNRGBA(image.Rect(7, 11, 23, 27))
	for y := 11; y < 27; y++ {
		for x := 7; x < 23; x++ {
			src.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 11), G: uint8(y * 7), B: 170, A: []uint8{0, 128, 255}[(x+y)%3]})
		}
	}
	texture, err := NewTexture(src)
	if err != nil {
		t.Fatal(err)
	}
	m := perspectiveTriangle()
	m.UVs = []UV{{U: -.13, V: .17}, {U: 1.71, V: .43}, {U: .67, V: 1.61}}
	for _, scale := range []int{1, 2, 4} {
		cfg := DefaultConfig(31, 19)
		cfg.Supersampling = scale
		cfg.Background = color.NRGBA{R: 30, G: 90, B: 170, A: 75}
		shared, err := NewRenderer(cfg)
		if err != nil {
			t.Fatal(err)
		}
		frame := func(r *Renderer, i int) (*image.RGBA, error) {
			in := colored(m, color.NRGBA{R: 201, G: 101, B: 53, A: uint8(100 + i*70)})
			in.Model = geom.Rotation(geom.Vec3{Z: 1}, float64(i)*.13)
			in.Material.Texture = texture
			in.Material.Sampler = Sampler{Filter: TextureFilter(i % 2), AddressU: Repeat, AddressV: Clamp}
			return r.Render(testCamera(Perspective), in)
		}
		var references [3][32]byte
		for i := range references {
			img, err := frame(shared, i)
			if err != nil {
				t.Fatal(err)
			}
			references[i] = sha256.Sum256(img.Pix)
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
					index := (worker + i) % len(references)
					if worker == 0 {
						// Only this worker mutates src, whose storage must be
						// independent of every concurrent texture sample/render.
						for j := range src.Pix {
							src.Pix[j] ^= 255
						}
					}
					img, err := frame(r, index)
					if err != nil {
						errors <- err
						return
					}
					if sha256.Sum256(img.Pix) != references[index] {
						errors <- fmt.Errorf("texture scale=%d worker=%d frame=%d changed", scale, worker, index)
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

func BenchmarkRenderTexturedTriangle(b *testing.B) {
	src := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for y := range 64 {
		for x := range 64 {
			c := color.NRGBA{R: 40, G: 90, B: 170, A: 128}
			if (x/8+y/8)%2 == 0 {
				c = color.NRGBA{R: 240, G: 220, B: 80, A: 255}
			}
			src.SetNRGBA(x, y, c)
		}
	}
	texture, err := NewTexture(src)
	if err != nil {
		b.Fatal(err)
	}
	m := perspectiveTriangle()
	m.UVs = []UV{{U: -.13, V: .17}, {U: 3.71, V: .43}, {U: .67, V: 2.61}}
	for _, filter := range []TextureFilter{Nearest, Bilinear} {
		for _, scale := range []int{1, 2, 4} {
			b.Run(fmt.Sprintf("filter%d/%dx", filter, scale), func(b *testing.B) {
				cfg := DefaultConfig(320, 180)
				cfg.Supersampling = scale
				r, err := NewRenderer(cfg)
				if err != nil {
					b.Fatal(err)
				}
				in := NewInstance(m)
				in.Material.Texture = texture
				in.Material.Sampler = Sampler{Filter: filter, AddressU: Repeat, AddressV: Repeat}
				b.ReportAllocs()
				b.ResetTimer()
				for range b.N {
					if _, err := r.Render(testCamera(Perspective), in); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
