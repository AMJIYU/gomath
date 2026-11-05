package three

import (
	"bytes"
	"fmt"
	"gomath/geom"
	"image/color"
	"math"
	"slices"
	"strings"
	"sync"
	"testing"
)

func TestPointLightValidation(t *testing.T) {
	base := DefaultConfig(7, 5)
	base.Light = Light{Kind: PointLight, Ambient: .2, Diffuse: .8}
	invalid := []func(*Light){
		func(l *Light) { l.Kind = LightKind(255) }, func(l *Light) { l.Position.X = math.NaN() }, func(l *Light) { l.Position.Y = math.Inf(-1) }, func(l *Light) { l.Position.Z = MaxCoordinate + 1 },
		func(l *Light) { l.Attenuation = -1 }, func(l *Light) { l.Attenuation = math.NaN() }, func(l *Light) { l.Attenuation = math.Inf(1) }, func(l *Light) { l.Attenuation = MaxLightAttenuation + 1 },
		func(l *Light) { l.Ambient = -1 }, func(l *Light) { l.Diffuse = math.Inf(1) }, func(l *Light) { l.Diffuse = 1.01 },
	}
	for i, mutate := range invalid {
		cfg := base
		mutate(&cfg.Light)
		if _, err := NewRenderer(cfg); err == nil {
			t.Fatalf("invalid point light %d accepted: %+v", i, cfg.Light)
		}
	}
	for _, position := range []geom.Vec3{{}, {X: MaxCoordinate, Y: -MaxCoordinate, Z: MaxCoordinate}, {X: math.SmallestNonzeroFloat64}} {
		cfg := base
		cfg.Light.Position = position
		cfg.Light.Attenuation = MaxLightAttenuation
		cfg.Light.Direction = geom.Vec3{X: math.NaN()} // Unused by PointLight.
		mustShadowRender(t, cfg, testCamera(Perspective), NewInstance(triangleMesh()))
	}
	for _, diffuse := range []float64{0, 1} {
		cfg := base
		cfg.Light.Diffuse = diffuse
		cfg.Light.Direction = geom.Vec3{Z: 1}
		cfg.Shadow = ShadowConfig{Enabled: true, Size: 4, Depth: 10, Resolution: 32}
		if _, err := NewRenderer(cfg); err == nil || !strings.Contains(err.Error(), "DirectionalLight") {
			t.Fatalf("point light used directional shadow map: %v", err)
		}
	}
	// An unused point position/coefficient cannot affect directional output.
	cfg := DefaultConfig(7, 5)
	want := mustShadowRender(t, cfg, testCamera(Perspective), NewInstance(triangleMesh()))
	cfg.Light.Position.X = math.NaN()
	cfg.Light.Attenuation = -1
	got := mustShadowRender(t, cfg, testCamera(Perspective), NewInstance(triangleMesh()))
	if !bytes.Equal(want.Pix, got.Pix) {
		t.Fatal("unused point fields affected directional rendering")
	}
}

func TestPointLightAnalyticPlane(t *testing.T) {
	for _, scale := range []int{1, 2, 4} {
		for _, attenuation := range []float64{0, .125, MaxLightAttenuation} {
			for _, alpha := range []uint8{128, 255} {
				cfg := DefaultConfig(7, 5)
				cfg.Supersampling = scale
				cfg.Background = color.NRGBA{R: 17, G: 31, B: 79, A: 119}
				cfg.Light = Light{Kind: PointLight, Position: geom.Vec3{Z: -1}, Ambient: .19, Diffuse: .93, Attenuation: attenuation}
				in := NewInstance(uvQuad(10, 10, 3))
				in.Material.Color = color.NRGBA{R: 217, G: 113, B: 53, A: alpha}
				got := mustShadowRender(t, cfg, testCamera(Orthographic), in)
				for y := range cfg.Height {
					for x := range cfg.Width {
						var sum [4]int
						for sy := range scale {
							for sx := range scale {
								wx := (2*(float64(x)+(float64(sx)+.5)/float64(scale))/float64(cfg.Width) - 1) * float64(cfg.Width) / float64(cfg.Height) * 2
								wy := (1 - 2*(float64(y)+(float64(sy)+.5)/float64(scale))/float64(cfg.Height)) * 2
								d2 := wx*wx + wy*wy + 4
								intensity := math.Min(1, .19+.93*2/math.Sqrt(d2)/(1+attenuation*d2))
								c := in.Material.Color
								c.R = uint8(math.Round(float64(c.R) * intensity))
								c.G = uint8(math.Round(float64(c.G) * intensity))
								c.B = uint8(math.Round(float64(c.B) * intensity))
								out := referenceTransparentLayers(cfg.Background, c)
								for i, ch := range [4]uint8{out.R, out.G, out.B, out.A} {
									sum[i] += int(ch)
								}
							}
						}
						n := scale * scale
						want := color.RGBA{R: uint8((sum[0] + n/2) / n), G: uint8((sum[1] + n/2) / n), B: uint8((sum[2] + n/2) / n), A: uint8((sum[3] + n/2) / n)}
						if ch := got.RGBAAt(x, y); ch != want {
							t.Fatalf("s%d attenuation %g alpha%d pixel%d,%d: %v want %v", scale, attenuation, alpha, x, y, ch, want)
						}
					}
				}
			}
		}
	}
}

func pointLightRayFixture(tb testing.TB, mode string, smooth bool) (Config, Camera, []Instance) {
	tb.Helper()
	cfg := DefaultConfig(43, 31)
	cfg.Background = color.NRGBA{R: 17, G: 39, B: 61, A: 107}
	cfg.Light = Light{Kind: PointLight, Position: geom.Vec3{X: 1.3, Y: -.6, Z: .7}, Attenuation: .17, Ambient: .13, Diffuse: .89}
	m := &Mesh{Vertices: []geom.Vec3{{X: -2.1, Y: -1.13, Z: -1.23}, {X: 2.47, Y: -1.37, Z: -6.71}, {X: .23, Y: 1.73, Z: -3.17}}, Triangles: []Triangle{{0, 1, 2}}, UVs: []UV{{U: -.31, V: 1.27}, {U: 1.43, V: .19}, {U: .37, V: -.23}}}
	if smooth {
		m.Normals = []geom.Vec3{{Y: .2, Z: 1}, {X: .3, Y: -.1, Z: .8}, {X: -.4, Y: .5, Z: 1}}
	}
	in := NewInstance(m)
	in.Model = geom.Translation(geom.Vec3{X: .17, Y: .09, Z: -.14}).Mul(geom.Rotation(geom.Vec3{Y: 1}, .13)).Mul(geom.Scaling(-1.1, .73, 1.07))
	in.Material.Color = color.NRGBA{R: 193, G: 157, B: 231, A: 173}
	if mode != "constant" {
		in.Material.Color.A = 255
		textureMode := "gradient"
		if strings.HasPrefix(mode, "mixed") || strings.HasPrefix(mode, "tinted") {
			textureMode = "mixed"
			if strings.HasPrefix(mode, "tinted") {
				in.Material.Color.A = 173
			}
		}
		in.Material.Texture = primitiveTexture(tb, textureMode)
		in.Material.Sampler = Sampler{AddressU: Repeat, AddressV: Clamp}
		if strings.HasSuffix(mode, "bilinear") {
			in.Material.Sampler.Filter = Bilinear
		}
	}
	cross := NewInstance(&Mesh{Vertices: []geom.Vec3{{X: -1.97, Y: -1.08, Z: -6.47}, {X: 2.13, Y: -1.31, Z: -1.37}, {X: .17, Y: 1.59, Z: -4.27}}, Triangles: []Triangle{{0, 1, 2}}})
	cross.Material.Color = color.NRGBA{R: 43, G: 217, B: 137, A: 107}
	back := NewInstance(uvQuad(4.73, 3.07, 7.83))
	back.Material.Color = color.NRGBA{R: 231, G: 201, B: 71, A: 255}
	return cfg, testCamera(Perspective), []Instance{in, back, cross}
}

func TestPointLightAgainstIndependentRays(t *testing.T) {
	for _, mode := range []string{"constant", "opaque-nearest", "opaque-bilinear", "mixed-nearest", "mixed-bilinear", "tinted-bilinear"} {
		for _, smooth := range []bool{false, true} {
			for _, projection := range []Projection{Perspective, Orthographic} {
				for _, scale := range []int{1, 2, 4} {
					for _, clipped := range []bool{false, true} {
						t.Run(fmt.Sprintf("%s/smooth%t/p%d/ssaa%d/clip%t", mode, smooth, projection, scale, clipped), func(t *testing.T) {
							cfg, camera, instances := pointLightRayFixture(t, mode, smooth)
							cfg.Supersampling = scale
							camera.Projection = projection
							if clipped {
								camera.Position = geom.Vec3{X: 1.7, Y: .8, Z: 1.2}
								camera.Target = geom.Vec3{Z: -3}
								camera.Near, camera.Far = 2.1, 7.2
								camera.FOVY, camera.OrthoHeight = .61, 3.17
								cfg.CullBackfaces = true
							}
							got := mustShadowRender(t, cfg, camera, instances...)
							want := pointLightRayReference(t, cfg, camera, instances...)
							for i, ch := range got.Pix {
								if math.Abs(float64(ch)-float64(want.Pix[i])) > 1 {
									t.Fatalf("pixel%d,%d channel%d: %d want%d", i/4%cfg.Width, i/4/cfg.Width, i%4, ch, want.Pix[i])
								}
							}
						})
					}
				}
			}
		}
	}
}

func TestPointLightCoincidenceBackfacesAndScaleExtremes(t *testing.T) {
	cfg := DefaultConfig(1, 1)
	cfg.Light = Light{Kind: PointLight, Position: geom.Vec3{Z: -3}, Ambient: .2, Diffuse: .8, Attenuation: .7}
	in := NewInstance(uvQuad(4, 4, 3))
	in.Material.Color = color.NRGBA{R: 200, G: 100, B: 50, A: 255}
	want := color.RGBA{R: 40, G: 20, B: 10, A: 255}
	if got := mustShadowRender(t, cfg, testCamera(Orthographic), in).RGBAAt(0, 0); got != want {
		t.Fatalf("coincident source: %v", got)
	}
	cfg.Light.Position.Z = -5
	if got := mustShadowRender(t, cfg, testCamera(Orthographic), in).RGBAAt(0, 0); got != want {
		t.Fatalf("light behind normal: %v", got)
	}
	cfg.Light.Position.Z = 0
	// Opposing authored normals cancel exactly on this one-pixel center ray.
	m := &Mesh{Vertices: []geom.Vec3{{X: -2, Y: -1, Z: -3}, {X: 2, Y: -1, Z: -3}, {Y: 2, Z: -3}}, Triangles: []Triangle{{0, 1, 2}}, Normals: []geom.Vec3{{Z: 1}, {Z: 1}, {Z: -2}}}
	// Normals normalize before interpolation: choose barycentrics (1/4,1/4,1/2).
	m.Vertices[2].Y = 1
	in.Mesh = m
	if got := mustShadowRender(t, cfg, testCamera(Orthographic), in).RGBAAt(0, 0); got != want {
		t.Fatalf("zero interpolated normal: %v", got)
	}
	// Direct analytic scalar checks probe separations below normal float range
	// and maximal bounded world deltas/attenuation without reciprocal overflow.
	for _, tc := range []struct {
		light         Light
		world, normal geom.Vec3
		want          float64
	}{
		{Light{Position: geom.Vec3{X: math.SmallestNonzeroFloat64}}, geom.Vec3{}, geom.Vec3{X: 1}, 1},
		{Light{Position: geom.Vec3{X: MaxCoordinate}, Attenuation: MaxLightAttenuation}, geom.Vec3{X: -MaxCoordinate}, geom.Vec3{X: 1}, 1 / (1 + 4e36)},
	} {
		got, err := tc.light.pointDiffuse(tc.world, tc.normal)
		if err != nil || !finite(got) || math.Abs(got-tc.want) > math.Abs(tc.want)*1e-14 {
			t.Fatalf("extreme point light: %g want %g err%v", got, tc.want, err)
		}
	}
}

func TestPointLightFragmentBudgetAndOpaqueOcclusion(t *testing.T) {
	for _, textured := range []bool{false, true} {
		cfg := DefaultConfig(7, 5)
		cfg.Supersampling = 2
		cfg.Light = Light{Kind: PointLight, Position: geom.Vec3{X: .3, Z: 1}, Ambient: .1, Diffuse: .9, Attenuation: .04}
		count := cfg.Width * cfg.Height * 4
		cfg.TransparentFragmentLimit = 2 * count
		front := NewInstance(uvQuad(20, 20, 2))
		front.Material.Color = color.NRGBA{R: 211, G: 17, B: 53, A: 137}
		second := front
		second.Mesh = uvQuad(20, 20, 3)
		second.Material.Color = color.NRGBA{R: 19, G: 233, B: 73, A: 193}
		if textured {
			front.Material.Texture = testTexture(t, 1, 1, func(x, y int) color.NRGBA { return color.NRGBA{R: 91, G: 71, B: 239, A: 127} })
		}
		opaque := NewInstance(uvQuad(20, 20, 4))
		hidden := front
		hidden.Mesh = uvQuad(20, 20, 5)
		camera := testCamera(Perspective)
		want := mustShadowRender(t, cfg, camera, hidden, second, opaque, front)
		cfg.TransparentFragmentLimit--
		r, err := NewRenderer(cfg)
		if err != nil {
			t.Fatal(err)
		}
		img, err := r.Render(camera, hidden, second, opaque, front)
		if img != nil || err == nil || !strings.Contains(err.Error(), "fragment budget") {
			t.Fatalf("budget overflow textured%t: %v", textured, err)
		}
		img, err = r.Render(camera, front, opaque, hidden)
		if err != nil || img == nil {
			t.Fatalf("failure leaked frame state: %v", err)
		}
		cfg.TransparentFragmentLimit++
		got := mustShadowRender(t, cfg, camera, front, opaque, hidden, second)
		if !bytes.Equal(got.Pix, want.Pix) {
			t.Fatal("permutation or opaque-before-alpha budget changed output")
		}
	}
}

func TestPointLightConcurrentConfigCopyAndWorldInvariance(t *testing.T) {
	cfg, camera, instances := pointLightRayFixture(t, "mixed-bilinear", true)
	cfg.Supersampling = 2
	r, err := NewRenderer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	want, err := r.Render(camera, instances...)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Light.Position = geom.Vec3{X: 100}
	cfg.Light.Attenuation = MaxLightAttenuation
	var wg sync.WaitGroup
	for worker := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			order := slices.Clone(instances)
			if worker%2 != 0 {
				slices.Reverse(order)
			}
			for range 3 {
				img, err := r.Render(camera, order...)
				if err != nil || !bytes.Equal(img.Pix, want.Pix) {
					t.Errorf("concurrent frame changed: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()
	// Moving scene, camera and light together leaves all world-space relative
	// vectors, attenuation and projected output unchanged (within rounding).
	cfg = r.config
	offset := geom.Vec3{X: 7, Y: -11, Z: 13}
	cfg.Light.Position = cfg.Light.Position.Add(offset)
	camera.Position = camera.Position.Add(offset)
	camera.Target = camera.Target.Add(offset)
	for i := range instances {
		instances[i].Model = geom.Translation(offset).Mul(instances[i].Model)
	}
	got := mustShadowRender(t, cfg, camera, instances...)
	for i, ch := range got.Pix {
		if math.Abs(float64(ch)-float64(want.Pix[i])) > 1 {
			t.Fatalf("translated light/camera/scene changed channel%d: %d want%d", i, ch, want.Pix[i])
		}
	}
}

func TestPointLightLeavesPointCloudsUnlit(t *testing.T) {
	cfg := DefaultConfig(31, 23)
	cfg.Supersampling = 4
	cloud, err := NewPointCloud([]geom.Vec3{{X: .3, Z: -2}, {X: -.4, Y: .1, Z: -3}}, []color.NRGBA{{R: 239, B: 73, A: 255}, {G: 193, B: 113, A: 137}}, []float64{4.1, 5.3})
	if err != nil {
		t.Fatal(err)
	}
	camera := testCamera(Perspective)
	r, _ := NewRenderer(cfg)
	want, err := r.RenderWithPoints(camera, []*PointCloud{cloud})
	if err != nil {
		t.Fatal(err)
	}
	cfg.Light = Light{Kind: PointLight, Position: geom.Vec3{X: 1, Z: 1}, Diffuse: 1, Attenuation: MaxLightAttenuation}
	r, _ = NewRenderer(cfg)
	got, err := r.RenderWithPoints(camera, []*PointCloud{cloud})
	if err != nil || !bytes.Equal(got.Pix, want.Pix) {
		t.Fatalf("point cloud was lit: %v", err)
	}
}

func TestPointLightMixedPointsDepthAndAlpha(t *testing.T) {
	for _, scale := range []int{1, 2, 4} {
		cfg := DefaultConfig(1, 1)
		cfg.Supersampling = scale
		cfg.Light = Light{Kind: PointLight, Ambient: .1, Diffuse: .9, Attenuation: .2}
		front := NewInstance(uvQuad(20, 20, 2))
		front.Material.Color = color.NRGBA{R: 239, G: 53, B: 71, A: 128}
		back := NewInstance(uvQuad(20, 20, 4))
		back.Material.Color = color.NRGBA{R: 73, G: 119, B: 227, A: 255}
		pointColor := color.NRGBA{R: 41, G: 211, B: 89, A: 137}
		cloud, err := NewPointCloud([]geom.Vec3{{Z: -3}, {Z: -5}}, []color.NRGBA{pointColor, {B: 255, A: 255}}, []float64{3, 3})
		if err != nil {
			t.Fatal(err)
		}
		r, err := NewRenderer(cfg)
		if err != nil {
			t.Fatal(err)
		}
		got, err := r.RenderWithPoints(testCamera(Orthographic), []*PointCloud{cloud}, front, back)
		if err != nil {
			t.Fatal(err)
		}
		var sum [4]int
		for sy := range scale {
			for sx := range scale {
				x, y := 4*(float64(sx)+.5)/float64(scale)-2, 2-4*(float64(sy)+.5)/float64(scale)
				shade := func(c color.NRGBA, depth float64) color.NRGBA {
					d2 := x*x + y*y + depth*depth
					intensity := .1 + .9*depth/math.Sqrt(d2)/(1+.2*d2)
					c.R, c.G, c.B = uint8(math.Round(float64(c.R)*intensity)), uint8(math.Round(float64(c.G)*intensity)), uint8(math.Round(float64(c.B)*intensity))
					return c
				}
				out := referenceTransparentLayers(cfg.Background, shade(front.Material.Color, 2), pointColor, shade(back.Material.Color, 4))
				for i, ch := range [4]uint8{out.R, out.G, out.B, out.A} {
					sum[i] += int(ch)
				}
			}
		}
		n := scale * scale
		want := color.RGBA{R: uint8((sum[0] + n/2) / n), G: uint8((sum[1] + n/2) / n), B: uint8((sum[2] + n/2) / n), A: uint8((sum[3] + n/2) / n)}
		if got.RGBAAt(0, 0) != want {
			t.Fatalf("ssaa%d mixed point/mesh composition: %v want%v", scale, got.RGBAAt(0, 0), want)
		}
		// A nearer opaque, unlit point replaces every lit mesh/point layer.
		pointColor.A = 255
		near, err := NewPointCloud([]geom.Vec3{{Z: -1}}, []color.NRGBA{pointColor}, []float64{3})
		if err != nil {
			t.Fatal(err)
		}
		got, err = r.RenderWithPoints(testCamera(Orthographic), []*PointCloud{cloud, near}, back, front)
		if err != nil || got.RGBAAt(0, 0) != (color.RGBA{R: pointColor.R, G: pointColor.G, B: pointColor.B, A: 255}) {
			t.Fatalf("ssaa%d opaque point depth: %v %v", scale, got, err)
		}
	}
}
