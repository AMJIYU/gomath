package three

import (
	"bytes"
	"cmp"
	"fmt"
	"gomath/geom"
	"image"
	"image/color"
	"math"
	"slices"
	"sync"
	"testing"
)

type referencePoint struct {
	x, y, z, radius float64
	color           color.NRGBA
}
type referencePointLayer struct {
	texturedRayLayer
	kind, order int
	rgba        uint32
}

// Point centers are independently projected with an authored camera basis in
// output-pixel coordinates. Circle membership uses Euclidean distance, without
// the production sample-grid bounds or squared-distance implementation. Mesh
// depth comes from independent world-space ray/triangle intersections.
func pointReferenceFrame(t *testing.T, cfg Config, camera Camera, clouds []*PointCloud, instances ...Instance) *image.RGBA {
	t.Helper()
	normalize := func(v geom.Vec3) geom.Vec3 { return v.Mul(1 / v.Norm()) }
	forward := normalize(camera.Target.Sub(camera.Position))
	right := normalize(forward.Cross(camera.Up))
	up := right.Cross(forward)
	aspect := float64(cfg.Width) / float64(cfg.Height)
	var points []referencePoint
	for _, cloud := range clouds {
		for i := range cloud.Len() {
			p, c, radius, err := cloud.Point(i)
			if err != nil {
				t.Fatal(err)
			}
			p = p.Sub(camera.Position)
			z := p.Dot(forward)
			if z < camera.Near || z > camera.Far {
				continue
			}
			span := camera.OrthoHeight / 2
			if camera.Projection == Perspective {
				span = z * math.Tan(camera.FOVY/2)
			}
			points = append(points, referencePoint{float64(cfg.Width) / 2 * (1 + p.Dot(right)/(span*aspect)), float64(cfg.Height) / 2 * (1 - p.Dot(up)/span), z, radius, c})
		}
	}
	scale := max(1, cfg.Supersampling)
	img := image.NewRGBA(image.Rect(0, 0, cfg.Width, cfg.Height))
	for y := range cfg.Height {
		for x := range cfg.Width {
			var sum [4]int
			for sy := range scale {
				for sx := range scale {
					px, py := float64(x)+(float64(sx)+.5)/float64(scale), float64(y)+(float64(sy)+.5)/float64(scale)
					rx, ry := (2*px/float64(cfg.Width)-1)*aspect, 1-2*py/float64(cfg.Height)
					origin, direction := camera.Position, forward
					if camera.Projection == Perspective {
						direction = direction.Add(right.Mul(rx * math.Tan(camera.FOVY/2))).Add(up.Mul(ry * math.Tan(camera.FOVY/2)))
					} else {
						origin = origin.Add(right.Mul(rx * camera.OrthoHeight / 2)).Add(up.Mul(ry * camera.OrthoHeight / 2))
					}
					var layers []referencePointLayer
					order := 0
					add := func(depth float64, c color.NRGBA, alpha float64, kind int) {
						if alpha == 0 {
							return
						}
						layers = append(layers, referencePointLayer{texturedRayLayer{depth, c, alpha}, kind, order, uint32(c.R)<<24 | uint32(c.G)<<16 | uint32(c.B)<<8 | uint32(c.A)})
						order++
					}
					for _, instance := range instances {
						for _, indices := range instance.Mesh.Triangles {
							a, b, c := instance.Mesh.Vertices[indices[0]], instance.Mesh.Vertices[indices[1]], instance.Mesh.Vertices[indices[2]]
							e1, e2 := b.Sub(a), c.Sub(a)
							p := direction.Cross(e2)
							det := e1.Dot(p)
							if math.Abs(det) < 1e-14 || cfg.CullBackfaces && det <= 0 {
								continue
							}
							s := origin.Sub(a)
							u := s.Dot(p) / det
							q := s.Cross(e1)
							v, z := direction.Dot(q)/det, e2.Dot(q)/det
							if u < 0 || v < 0 || u+v > 1 || z < camera.Near || z > camera.Far {
								continue
							}
							base := instance.Material.Color
							alpha := float64(base.A) / 255
							if texture := instance.Material.Texture; texture != nil {
								var uv UV
								for i, weight := range []float64{1 - u - v, u, v} {
									uv.U += weight * instance.Mesh.UVs[indices[i]].U
									uv.V += weight * instance.Mesh.UVs[indices[i]].V
								}
								texel, err := texture.Sample(uv, instance.Material.Sampler)
								if err != nil {
									t.Fatal(err)
								}
								base.R = uint8(math.Round(float64(base.R) * float64(texel.R) / 255))
								base.G = uint8(math.Round(float64(base.G) * float64(texel.G) / 255))
								base.B = uint8(math.Round(float64(base.B) * float64(texel.B) / 255))
								alpha *= float64(texel.A) / 255
							}
							add(z, base, alpha, 0)
						}
					}
					for _, p := range points {
						if math.Hypot(px-p.x, py-p.y) <= p.radius {
							add(p.z, p.color, float64(p.color.A)/255, 1)
						}
					}
					slices.SortFunc(layers, func(a, b referencePointLayer) int {
						if c := cmp.Compare(a.depth, b.depth); c != 0 {
							return c
						}
						if (a.alpha == 1) != (b.alpha == 1) {
							if a.alpha == 1 {
								return -1
							}
							return 1
						}
						if a.alpha == 1 {
							if c := cmp.Compare(a.kind, b.kind); c != 0 {
								return c
							}
							if a.kind == 0 {
								return cmp.Compare(a.order, b.order)
							}
							return cmp.Compare(a.rgba, b.rgba)
						}
						return cmp.Compare(b.rgba, a.rgba) // inverse of far-to-near RGBA tie order
					})
					nearest := make([]texturedRayLayer, len(layers))
					for i := range layers {
						nearest[i] = layers[i].texturedRayLayer
					}
					c := referenceTextureLayers(cfg.Background, nearest...)
					for i, channel := range [4]uint8{c.R, c.G, c.B, c.A} {
						sum[i] += int(channel)
					}
				}
			}
			n := scale * scale
			img.SetRGBA(x, y, color.RGBA{R: uint8((sum[0] + n/2) / n), G: uint8((sum[1] + n/2) / n), B: uint8((sum[2] + n/2) / n), A: uint8((sum[3] + n/2) / n)})
		}
	}
	return img
}

func pointFixture(t *testing.T, rotated bool) ([]*PointCloud, []Instance, Camera) {
	positions := []geom.Vec3{{X: .07, Y: .13, Z: -2}, {X: .5, Y: -.4, Z: -3}, {X: -.8, Y: .25, Z: -1}, {X: .2, Y: .8, Z: -10}, {Z: -.999}, {Z: -10.001}, {Z: 1}, {X: 3, Y: .13, Z: -2}, {X: -3, Y: -.27, Z: -2}, {Y: 2.2, Z: -2}, {X: MaxCoordinate, Z: -2}}
	colors := []color.NRGBA{{R: 230, G: 30, B: 60, A: 128}, {R: 20, G: 210, B: 80, A: 255}, {R: 40, G: 80, B: 240, A: 64}, {R: 220, G: 40, B: 210, A: 254}, {A: 255}, {A: 255}, {A: 255}, {R: 255, G: 190, B: 30, A: 173}, {B: 255, A: 1}, {R: 255, G: 255, A: 128}, {A: 255}}
	radii := []float64{2.4, 3.2, 1.1, 2.6, 64, 64, 64, 6, 5, 3.6, 64}
	camera := testCamera(Perspective)
	mesh := &Mesh{Vertices: []geom.Vec3{{X: -2, Y: -1.3, Z: -1.8}, {X: 2.5, Y: -1, Z: -5.4}, {X: .25, Y: 1.7, Z: -3.2}}, Triangles: []Triangle{{0, 1, 2}}, UVs: []UV{{}, {U: 1}, {V: 1}}}
	back := &Mesh{Vertices: []geom.Vec3{{X: -2.2, Y: -1.5, Z: -4}, {X: 2.7, Y: -1.2, Z: -2.2}, {X: .5, Y: 2, Z: -3.7}}, Triangles: []Triangle{{0, 1, 2}}, UVs: []UV{{}, {U: 1}, {V: 1}}}
	if rotated {
		camera.Position, camera.Target = geom.Vec3{X: -3, Y: 2, Z: 4}, geom.Vec3{Y: 2, Z: 4}
		convert := func(p geom.Vec3) geom.Vec3 { return geom.Vec3{X: -3 - p.Z, Y: 2 + p.Y, Z: 4 + p.X} }
		for i := range positions {
			positions[i] = convert(positions[i])
		}
		// Avoid transforming the deliberately extreme offscreen X over bounds.
		positions[len(positions)-1] = geom.Vec3{X: -1, Y: MaxCoordinate, Z: 4}
		for _, m := range []*Mesh{mesh, back} {
			for i, p := range m.Vertices {
				m.Vertices[i] = convert(p)
			}
		}
	}
	cloud := mustPointCloud(t, positions, colors, radii)
	frontInstance, backInstance := NewInstance(mesh), NewInstance(back)
	frontInstance.Material.Color = color.NRGBA{R: 180, G: 100, B: 20, A: 255}
	backInstance.Material.Color = color.NRGBA{R: 60, G: 120, B: 245, A: 128}
	return []*PointCloud{cloud}, []Instance{frontInstance, backInstance}, camera
}

func TestPointsAndMeshesAgainstIndependentCoverage(t *testing.T) {
	for _, projection := range []Projection{Perspective, Orthographic} {
		for _, scale := range []int{1, 2, 4} {
			for _, rotated := range []bool{false, true} {
				for _, mode := range []string{"points", "meshes", "textured"} {
					t.Run(fmt.Sprintf("p%d/s%d/rot%t/%s", projection, scale, rotated, mode), func(t *testing.T) {
						clouds, meshes, camera := pointFixture(t, rotated)
						camera.Projection = projection
						cfg := DefaultConfig(31, 23)
						cfg.Supersampling, cfg.Light = scale, Light{Ambient: 1}
						cfg.Background = color.NRGBA{R: 73, G: 119, B: 181, A: 97}
						if mode == "points" {
							meshes = nil
						}
						if mode == "textured" {
							meshes[1].Material.Texture = testTexture(t, 1, 1, func(_, _ int) color.NRGBA { return color.NRGBA{R: 255, G: 255, B: 255, A: 173} })
							meshes[1].Material.Color.A = 193
						}
						r, err := NewRenderer(cfg)
						if err != nil {
							t.Fatal(err)
						}
						got, err := r.RenderWithPoints(camera, clouds, meshes...)
						if err != nil {
							t.Fatal(err)
						}
						want := pointReferenceFrame(t, cfg, camera, clouds, meshes...)
						for i, ch := range got.Pix {
							if math.Abs(float64(ch)-float64(want.Pix[i])) > 1 {
								t.Fatalf("pixel %d channel %d got %d want %d", i/4, i%4, ch, want.Pix[i])
							}
						}
						// Also reverse mesh submission; the fixture has distinct depths.
						slices.Reverse(meshes)
						reordered, err := r.RenderWithPoints(camera, reversePointClouds(t, clouds), meshes...)
						if err != nil || !bytes.Equal(got.Pix, reordered.Pix) {
							t.Fatalf("submission order changed output: %v", err)
						}
					})
				}
			}
		}
	}
}

func reversePointClouds(t *testing.T, clouds []*PointCloud) []*PointCloud {
	t.Helper()
	var result []*PointCloud
	for _, c := range clouds {
		for i := range c.Len() {
			p, color, radius, err := c.Point(i)
			if err != nil {
				t.Fatal(err)
			}
			result = append(result, onePoint(t, p, color, radius))
		}
	}
	slices.Reverse(result)
	return result
}

func TestPointCircleBoundaryFixedRadiusAndTies(t *testing.T) {
	cfg := DefaultConfig(9, 9)
	cfg.Light = Light{} // Points remain unlit even with zero ambient.
	camera := testCamera(Orthographic)
	camera.OrthoHeight = 9
	r, _ := NewRenderer(cfg)
	red := color.NRGBA{R: 255, A: 255}
	for _, projection := range []Projection{Perspective, Orthographic} {
		camera.Projection = projection
		for _, z := range []float64{-1, -2, -10} {
			cloud := onePoint(t, geom.Vec3{Z: z}, red, 1)
			img, err := r.RenderWithPoints(camera, []*PointCloud{cloud})
			if err != nil {
				t.Fatal(err)
			}
			for y := range 9 {
				for x := range 9 {
					want := color.RGBA{}
					if (x-4)*(x-4)+(y-4)*(y-4) <= 1 {
						want = color.RGBA{R: 255, A: 255}
					}
					if img.RGBAAt(x, y) != want {
						t.Fatalf("disk/depth/projection boundary at %d,%d", x, y)
					}
				}
			}
		}
	}
	camera.Projection = Orthographic
	colors := []color.NRGBA{{R: 255, A: 255}, {B: 255, A: 255}, {G: 255, A: 255}}
	positions := slices.Repeat([]geom.Vec3{{Z: -2}}, 3)
	cloud := mustPointCloud(t, positions, colors, nil)
	img, err := r.RenderWithPoints(camera, []*PointCloud{cloud})
	if err != nil || img.RGBAAt(4, 4) != (color.RGBA{B: 255, A: 255}) {
		t.Fatalf("opaque point tie: %v", err)
	}
	reordered, err := r.RenderWithPoints(camera, reversePointClouds(t, []*PointCloud{cloud}))
	if err != nil || !bytes.Equal(img.Pix, reordered.Pix) {
		t.Fatal("opaque point ties depend on order")
	}
	// Mesh wins exact ties, and transparent point/mesh ties contribute no layer
	// in front of that opaque depth. Ortho avoids reciprocal rounding ambiguity.
	mesh := NewInstance(uvQuad(3, 3, 2))
	mesh.Material.Color = color.NRGBA{R: 23, G: 45, B: 67, A: 255}
	img, err = r.RenderWithPoints(camera, []*PointCloud{cloud}, mesh)
	if err != nil || img.RGBAAt(4, 4) != (color.RGBA{A: 255}) {
		t.Fatalf("mesh opaque tie / zero-light: %v", err)
	}
	for i := range colors {
		colors[i].A = 128
	}
	cloud = mustPointCloud(t, positions, colors, nil)
	img, err = r.RenderWithPoints(camera, []*PointCloud{cloud})
	if err != nil {
		t.Fatal(err)
	}
	reordered, err = r.RenderWithPoints(camera, reversePointClouds(t, []*PointCloud{cloud}))
	if err != nil || !bytes.Equal(img.Pix, reordered.Pix) {
		t.Fatal("transparent point ties depend on order")
	}
}

func TestPointsPreserveRenderAndConcurrentOwnership(t *testing.T) {
	clouds, meshes, camera := pointFixture(t, false)
	cfg := DefaultConfig(31, 23)
	cfg.Supersampling = 2
	r, _ := NewRenderer(cfg)
	for _, alpha := range []uint8{0, 128, 255} {
		meshes[0].Material.Color.A = alpha
		want, err := r.Render(camera, meshes...)
		if err != nil {
			t.Fatal(err)
		}
		for _, list := range [][]*PointCloud{nil, {}, {onePoint(t, geom.Vec3{Z: -2}, color.NRGBA{}, 64)}} {
			got, err := r.RenderWithPoints(camera, list, meshes...)
			if err != nil || !bytes.Equal(got.Pix, want.Pix) {
				t.Fatalf("point API changed mesh-only output: %v", err)
			}
		}
	}
	sourcePositions := []geom.Vec3{{X: .2, Y: -.1, Z: -1.5}}
	sourceColors := []color.NRGBA{{R: 235, G: 41, B: 91, A: 173}}
	sourceRadii := []float64{2.5}
	clouds = append(clouds, mustPointCloud(t, sourcePositions, sourceColors, sourceRadii))
	want, err := r.RenderWithPoints(camera, clouds, meshes...)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := range 10000 {
			sourcePositions[0].X = float64(i)
			sourceColors[0].R = uint8(i)
			sourceRadii[0] = float64(i)
		}
	}()
	for i := range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			active := r
			if i%2 == 0 {
				active, _ = NewRenderer(cfg)
			}
			got, err := active.RenderWithPoints(camera, clouds, meshes...)
			if err != nil || !bytes.Equal(got.Pix, want.Pix) {
				t.Errorf("concurrent output: %v", err)
				return
			}
			got.Pix[0] ^= 255
		}()
	}
	wg.Wait()
	got, err := r.RenderWithPoints(camera, clouds, meshes...)
	if err != nil || !bytes.Equal(got.Pix, want.Pix) {
		t.Fatal("source or output mutation changed cloud")
	}
}

func TestPointSSAAEdgesAndExtremeProjection(t *testing.T) {
	for _, scale := range []int{1, 2, 4} {
		for _, alpha := range []uint8{128, 255} {
			for _, background := range []color.NRGBA{{}, {R: 63, G: 145, B: 211, A: 79}} {
				for _, radius := range []float64{MinPointRadius, .83, 1} {
					cfg := DefaultConfig(3, 3)
					cfg.Supersampling, cfg.Background = scale, background
					camera := testCamera(Orthographic)
					camera.OrthoHeight = 3
					clouds := []*PointCloud{onePoint(t, geom.Vec3{Z: -2}, color.NRGBA{R: 245, G: 53, B: 118, A: alpha}, radius)}
					r, _ := NewRenderer(cfg)
					got, err := r.RenderWithPoints(camera, clouds)
					if err != nil {
						t.Fatal(err)
					}
					want := pointReferenceFrame(t, cfg, camera, clouds)
					requireTextureFrame(t, got, want)
					if scale > 1 && radius > .5 && alpha == 255 && background.A == 0 {
						partial := false
						for i := 3; i < len(got.Pix); i += 4 {
							partial = partial || got.Pix[i] > 0 && got.Pix[i] < 255
						}
						if !partial {
							t.Fatal("SSAA circle edge has no partial coverage")
						}
					}
				}
			}
		}
	}
	// Projected centers exceed int64, despite valid finite world coordinates.
	// Clamping in floating point must happen before any integer conversion.
	cfg := DefaultConfig(8, 8)
	cfg.Supersampling = 4
	camera := testCamera(Perspective)
	camera.Near, camera.FOVY = 1e-6, 1e-4
	r, _ := NewRenderer(cfg)
	positions := []geom.Vec3{{X: MaxCoordinate, Z: -1e-6}, {X: -MaxCoordinate, Z: -1e-6}, {Y: MaxCoordinate, Z: -1e-6}, {Y: -MaxCoordinate, Z: -1e-6}}
	cloud := mustPointCloud(t, positions, slices.Repeat([]color.NRGBA{{A: 255}}, len(positions)), slices.Repeat([]float64{MaxPointRadius}, len(positions)))
	got, err := r.RenderWithPoints(camera, []*PointCloud{cloud})
	if err != nil || !bytes.Equal(got.Pix, make([]byte, 8*8*4)) {
		t.Fatalf("extreme offscreen center: %v", err)
	}
}

func TestPointManyLayersAcrossFragmentBlocks(t *testing.T) {
	const count = 5000
	positions, colors := make([]geom.Vec3, count), make([]color.NRGBA, count)
	for i := range count {
		positions[i].Z = -1.1 - 8.8*float64(i*541%count)/count
		colors[i] = color.NRGBA{R: uint8(i * 37), G: uint8(i * 71), B: uint8(i * 13), A: 1}
	}
	cloud := mustPointCloud(t, positions, colors, nil)
	cfg := DefaultConfig(1, 1)
	cfg.Light = Light{Ambient: 1}
	cfg.PointSampleLimit, cfg.TransparentFragmentLimit = count, count+1
	camera := testCamera(Orthographic)
	for _, precise := range []bool{false, true} {
		var meshes []Instance
		if precise {
			m := triangleMesh()
			for i := range m.Vertices {
				m.Vertices[i].Z = -9.99
			}
			m.UVs = []UV{{}, {U: 1}, {V: 1}}
			in := NewInstance(m)
			in.Material.Color.A = 173
			in.Material.Texture = testTexture(t, 1, 1, func(_, _ int) color.NRGBA { return color.NRGBA{R: 230, G: 80, B: 130, A: 128} })
			meshes = []Instance{in}
		}
		r, _ := NewRenderer(cfg)
		got, err := r.RenderWithPoints(camera, []*PointCloud{cloud}, meshes...)
		if err != nil {
			t.Fatal(err)
		}
		want := pointReferenceFrame(t, cfg, camera, []*PointCloud{cloud}, meshes...)
		requireTextureFrame(t, got, want)
		reversedPositions, reversedColors := slices.Clone(positions), slices.Clone(colors)
		slices.Reverse(reversedPositions)
		slices.Reverse(reversedColors)
		reversed := mustPointCloud(t, reversedPositions, reversedColors, nil)
		other, err := r.RenderWithPoints(camera, []*PointCloud{reversed}, meshes...)
		if err != nil || !bytes.Equal(got.Pix, other.Pix) {
			t.Fatalf("cross-block point order: %v", err)
		}
	}
}

func TestPointCoplanarTransparentMeshTie(t *testing.T) {
	cfg := DefaultConfig(1, 1)
	cfg.Light = Light{Ambient: 1}
	camera := testCamera(Orthographic)
	clouds := []*PointCloud{
		onePoint(t, geom.Vec3{Z: -2}, color.NRGBA{R: 200, B: 50, A: 128}, 1),
		onePoint(t, geom.Vec3{Z: -2}, color.NRGBA{R: 40, G: 220, A: 1}, 1),
	}
	for _, textured := range []bool{false, true} {
		m := triangleMesh()
		m.UVs = []UV{{}, {U: 1}, {V: 1}}
		mesh := NewInstance(m)
		mesh.Material.Color = color.NRGBA{G: 190, B: 20, A: 173}
		if textured {
			mesh.Material.Texture = testTexture(t, 1, 1, func(_, _ int) color.NRGBA { return color.NRGBA{R: 255, G: 255, B: 255, A: 128} })
		}
		r, _ := NewRenderer(cfg)
		got, err := r.RenderWithPoints(camera, clouds, mesh)
		if err != nil {
			t.Fatal(err)
		}
		requireTextureFrame(t, got, pointReferenceFrame(t, cfg, camera, clouds, mesh))
		reordered, err := r.RenderWithPoints(camera, reversePointClouds(t, clouds), mesh)
		if err != nil || !bytes.Equal(got.Pix, reordered.Pix) {
			t.Fatalf("mesh/point coplanar tie changed: %v", err)
		}
	}
}
