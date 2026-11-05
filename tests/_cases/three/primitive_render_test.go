package three

import (
	"bytes"
	"fmt"
	"gomath/geom"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
)

type primitiveCase struct {
	name string
	mesh *Mesh
}

func newPrimitiveCases(tb testing.TB, detailed bool) []primitiveCase {
	tb.Helper()
	n, h, k := 9, 2, 7
	if detailed {
		n, h, k = 64, 8, 32
	}
	cylinder, err := Cylinder(.9, 2, n, h)
	if err != nil {
		tb.Fatal(err)
	}
	cone, err := Cone(1, 2, n, h)
	if err != nil {
		tb.Fatal(err)
	}
	torus, err := Torus(1.05, .4, n, k)
	if err != nil {
		tb.Fatal(err)
	}
	return []primitiveCase{{"cylinder", cylinder}, {"cone", cone}, {"torus", torus}}
}

func primitiveCamera(projection Projection) Camera {
	return Camera{Position: geom.Vec3{X: 3.7, Y: -4.1, Z: 2.8}, Up: geom.Vec3{Z: 1}, Projection: projection, FOVY: math.Pi / 3, OrthoHeight: 3.6, Near: .1, Far: 20}
}

func primitiveTexture(tb testing.TB, mode string) *Texture {
	tb.Helper()
	src := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for y := range 64 {
		for x := range 64 {
			c := color.NRGBA{R: uint8(4 * x), G: uint8(4 * y), B: 83, A: 255}
			if mode == "checker" || mode == "mixed" {
				c = color.NRGBA{R: 245, G: 192, B: 64, A: 255}
				if (x/8+y/8)%2 == 0 {
					c = color.NRGBA{R: 35, G: 110, B: 210, A: 255}
				}
			}
			if mode == "mixed" || mode == "tinted" {
				c.A = []uint8{0, 128, 255}[(x/8+y/8)%3]
			}
			src.SetNRGBA(x, y, c)
		}
	}
	tex, err := NewTexture(src)
	if err != nil {
		tb.Fatal(err)
	}
	return tex
}

// Independent world-space Moller-Trumbore intersections, with no renderer
// projection, clipping, edge equations, depth interpolation or raster helpers.
type primitiveRayTriangle struct {
	a, e1, e2 geom.Vec3
	uv        [3]UV
}

func primitiveRayTriangles(m *Mesh) []primitiveRayTriangle {
	triangles := make([]primitiveRayTriangle, len(m.Triangles))
	for i, indices := range m.Triangles {
		a, b, c := m.Vertices[indices[0]], m.Vertices[indices[1]], m.Vertices[indices[2]]
		triangles[i] = primitiveRayTriangle{a, b.Sub(a), c.Sub(a), [3]UV{m.UVs[indices[0]], m.UVs[indices[1]], m.UVs[indices[2]]}}
	}
	return triangles
}

func primitiveRayReference(t *testing.T, cfg Config, camera Camera, instance Instance) *image.RGBA {
	t.Helper()
	triangles := primitiveRayTriangles(instance.Mesh)
	normalize := func(p geom.Vec3) geom.Vec3 { return p.Mul(1 / p.Norm()) }
	forward := normalize(camera.Target.Sub(camera.Position))
	right := normalize(forward.Cross(camera.Up))
	up := right.Cross(forward)
	scale := max(1, cfg.Supersampling)
	img := image.NewRGBA(image.Rect(0, 0, cfg.Width, cfg.Height))
	hits := 0
	for y := range cfg.Height {
		for x := range cfg.Width {
			var sum [4]int
			for sy := range scale {
				for sx := range scale {
					px, py := float64(x)+(float64(sx)+.5)/float64(scale), float64(y)+(float64(sy)+.5)/float64(scale)
					rx, ry := (2*px/float64(cfg.Width)-1)*float64(cfg.Width)/float64(cfg.Height), 1-2*py/float64(cfg.Height)
					origin, direction := camera.Position, forward
					if camera.Projection == Perspective {
						direction = direction.Add(right.Mul(rx * math.Tan(camera.FOVY/2))).Add(up.Mul(ry * math.Tan(camera.FOVY/2)))
					} else {
						origin = origin.Add(right.Mul(rx * camera.OrthoHeight / 2)).Add(up.Mul(ry * camera.OrthoHeight / 2))
					}
					var layers []texturedRayLayer
					for _, tri := range triangles {
						p := direction.Cross(tri.e2)
						det := tri.e1.Dot(p)
						if math.Abs(det) < 1e-14 || cfg.CullBackfaces && det <= 0 {
							continue
						}
						s := origin.Sub(tri.a)
						u := s.Dot(p) / det
						q := s.Cross(tri.e1)
						v := direction.Dot(q) / det
						depth := tri.e2.Dot(q) / det
						if u < 0 || v < 0 || u+v > 1 || depth < camera.Near || depth > camera.Far {
							continue
						}
						uv := UV{U: (1-u-v)*tri.uv[0].U + u*tri.uv[1].U + v*tri.uv[2].U, V: (1-u-v)*tri.uv[0].V + u*tri.uv[1].V + v*tri.uv[2].V}
						texel, err := instance.Material.Texture.Sample(uv, instance.Material.Sampler)
						if err != nil {
							t.Fatal(err)
						}
						layers = append(layers, texturedRayLayer{depth, texel, float64(texel.A) / 255 * float64(instance.Material.Color.A) / 255})
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
					if len(layers) > 0 {
						hits++
					}
					c := referenceTextureLayers(cfg.Background, layers...)
					for i, ch := range [4]uint8{c.R, c.G, c.B, c.A} {
						sum[i] += int(ch)
					}
				}
			}
			n := scale * scale
			img.SetRGBA(x, y, color.RGBA{R: uint8((sum[0] + n/2) / n), G: uint8((sum[1] + n/2) / n), B: uint8((sum[2] + n/2) / n), A: uint8((sum[3] + n/2) / n)})
		}
	}
	if hits < 30 {
		t.Fatalf("insufficient primitive ray coverage: %d", hits)
	}
	return img
}

func TestNewPrimitivesTexturedAgainstRays(t *testing.T) {
	for _, primitive := range newPrimitiveCases(t, false) {
		for _, projection := range []Projection{Perspective, Orthographic} {
			for _, mode := range []string{"checker", "gradient", "mixed", "tinted"} {
				for _, scale := range []int{1, 2, 4} {
					for _, clipped := range []bool{false, true} {
						t.Run(fmt.Sprintf("%s/p%d/%s/s%d/clip%t", primitive.name, projection, mode, scale, clipped), func(t *testing.T) {
							cfg := DefaultConfig(41, 31)
							cfg.Light = Light{Ambient: 1}
							cfg.Supersampling = scale
							cfg.CullBackfaces = mode == "checker" || mode == "gradient"
							cfg.Background = color.NRGBA{R: 17, G: 31, B: 53, A: 91}
							camera := primitiveCamera(projection)
							if projection == Orthographic {
								camera.Position.Z = -camera.Position.Z // Include bottom cap UVs.
							}
							if clipped {
								camera.Near, camera.Far = 5.3, 6.5
								camera.FOVY, camera.OrthoHeight = .35, 1.4
							}
							in := NewInstance(primitive.mesh)
							in.Material.Texture = primitiveTexture(t, mode)
							in.Material.Sampler = Sampler{Filter: Bilinear, AddressU: Repeat, AddressV: Clamp}
							if mode == "tinted" {
								in.Material.Color.A = 173
							}
							renderer, err := NewRenderer(cfg)
							if err != nil {
								t.Fatal(err)
							}
							got, err := renderer.Render(camera, in)
							if err != nil {
								t.Fatal(err)
							}
							want := primitiveRayReference(t, cfg, camera, in)
							// Independent barycentric evaluation may put exact channel
							// rounding ties on different sides of a half integer.
							for i, ch := range got.Pix {
								if math.Abs(float64(ch)-float64(want.Pix[i])) > 1 {
									t.Fatalf("pixel (%d,%d) channel %d: %d want %d", i/4%cfg.Width, i/4/cfg.Width, i%4, ch, want.Pix[i])
								}
							}
							// Triangle submission order cannot change the new topology's
							// opaque coverage or independently depth-sorted alpha layers.
							copyMesh := *primitive.mesh
							copyMesh.Triangles = slices.Clone(copyMesh.Triangles)
							slices.Reverse(copyMesh.Triangles)
							in.Mesh = &copyMesh
							reversed, err := renderer.Render(camera, in)
							if err != nil || !bytes.Equal(got.Pix, reversed.Pix) {
								t.Fatalf("triangle order changed primitive rendering: %v", err)
							}
						})
					}
				}
			}
		}
	}
}

func TestNewPrimitivesConcurrentRendering(t *testing.T) {
	cfg := DefaultConfig(71, 53)
	cfg.Supersampling = 2
	renderer, err := NewRenderer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	tex := primitiveTexture(t, "mixed")
	for _, primitive := range newPrimitiveCases(t, false) {
		in := NewInstance(primitive.mesh)
		in.Material.Texture = tex
		in.Material.Sampler = Sampler{Filter: Bilinear, AddressU: Repeat, AddressV: Repeat}
		camera := primitiveCamera(Perspective)
		want, err := renderer.Render(camera, in)
		if err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		for range 8 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				got, err := renderer.Render(camera, in)
				if err != nil || !bytes.Equal(got.Pix, want.Pix) {
					t.Errorf("%s concurrent frame: %v", primitive.name, err)
				}
			}()
		}
		wg.Wait()
	}
}

// Opt-in artifacts are generated by the central runner. Normal tests render
// and encode into temporary storage without rewriting repository fixtures.
func TestNewPrimitiveArtifacts(t *testing.T) {
	output := os.Getenv("GOMATH_THREE_ARTIFACTS")
	if output == "" {
		output = t.TempDir()
	}
	if err := os.MkdirAll(output, 0755); err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig(320, 240)
	cfg.Supersampling, cfg.CullBackfaces = 2, true
	cfg.Background = color.NRGBA{R: 20, G: 25, B: 35, A: 255}
	contact := image.NewRGBA(image.Rect(0, 0, 3*cfg.Width, 3*cfg.Height))
	for row, mode := range []string{"checker", "gradient", "mixed"} {
		// Transparent closed surfaces display both front and rear layers.
		cfg.CullBackfaces = mode != "mixed"
		renderer, err := NewRenderer(cfg)
		if err != nil {
			t.Fatal(err)
		}
		for col, primitive := range newPrimitiveCases(t, true) {
			in := NewInstance(primitive.mesh)
			in.Material.Texture = primitiveTexture(t, mode)
			in.Material.Sampler = Sampler{Filter: Bilinear, AddressU: Repeat, AddressV: Clamp}
			camera := primitiveCamera(Perspective)
			camera.FOVY = math.Pi / 5
			img, err := renderer.Render(camera, in)
			if err != nil {
				t.Fatal(err)
			}
			draw.Draw(contact, image.Rect(col*cfg.Width, row*cfg.Height, (col+1)*cfg.Width, (row+1)*cfg.Height), img, image.Point{}, draw.Src)
		}
	}
	path := filepath.Join(output, "primitives.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	encodeErr := png.Encode(f, contact)
	closeErr := f.Close()
	if encodeErr != nil || closeErr != nil {
		t.Fatalf("encode %v close %v", encodeErr, closeErr)
	}
	t.Logf("artifact: %s; columns cylinder/cone/torus; rows checker/UV-gradient/mixed-alpha", path)
}

var benchmarkPrimitive *Mesh

func BenchmarkNewPrimitives(b *testing.B) {
	for _, entry := range []struct {
		name string
		new  func(float64, float64, int, int) (*Mesh, error)
	}{{"Cylinder", Cylinder}, {"Cone", Cone}, {"Torus", Torus}} {
		b.Run(entry.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				mesh, err := entry.new(1, .5, 64, 32)
				if err != nil {
					b.Fatal(err)
				}
				benchmarkPrimitive = mesh
			}
		})
	}
}

func BenchmarkRenderTexturedPrimitives(b *testing.B) {
	for _, primitive := range newPrimitiveCases(b, true) {
		for _, mode := range []string{"checker", "mixed"} {
			b.Run(primitive.name+"/"+mode, func(b *testing.B) {
				cfg := DefaultConfig(320, 240)
				cfg.Supersampling = 2
				cfg.CullBackfaces = mode != "mixed"
				renderer, err := NewRenderer(cfg)
				if err != nil {
					b.Fatal(err)
				}
				in := NewInstance(primitive.mesh)
				in.Material.Texture = primitiveTexture(b, mode)
				in.Material.Sampler = Sampler{Filter: Bilinear, AddressU: Repeat, AddressV: Clamp}
				camera := primitiveCamera(Perspective)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if _, err := renderer.Render(camera, in); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
