package three

import (
	"bytes"
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

func shadowFixture(tb testing.TB) (Config, Camera, []Instance) {
	tb.Helper()
	cfg := DefaultConfig(73, 55)
	cfg.Background = color.NRGBA{R: 17, G: 28, B: 41, A: 153}
	cfg.Light = Light{Direction: geom.Vec3{X: .65, Y: -.3, Z: 1}, Ambient: .23, Diffuse: .77}
	cfg.Shadow = ShadowConfig{Enabled: true, Center: geom.Vec3{Z: .5}, Size: 10, Depth: 12, Resolution: 192, Bias: .05}
	plane := &Mesh{Vertices: []geom.Vec3{{X: -3.1, Y: -2.7}, {X: 3.1, Y: -2.7}, {X: 3.1, Y: 2.7}, {X: -3.1, Y: 2.7}}, Triangles: []Triangle{{0, 1, 2}, {0, 2, 3}}, UVs: []UV{{}, {U: 1}, {U: 1, V: 1}, {V: 1}}}
	ground := NewInstance(plane)
	ground.Material.Color = color.NRGBA{R: 220, G: 235, B: 250, A: 255}
	box, err := Box(1.1, .9, 1.4)
	if err != nil {
		tb.Fatal(err)
	}
	caster := NewInstance(box)
	caster.Model = geom.Translation(geom.Vec3{X: .23, Y: .17, Z: .7}).Mul(geom.RotationZ(.27)).Mul(geom.Scaling(-1.1, .85, 1))
	caster.Material.Color = color.NRGBA{R: 210, G: 97, B: 42, A: 255}
	camera := Camera{Position: geom.Vec3{X: 4, Y: -6, Z: 5}, Target: geom.Vec3{Z: .4}, Up: geom.Vec3{Z: 1}, Projection: Perspective, FOVY: .7, OrthoHeight: 6.1, Near: .1, Far: 20}
	return cfg, camera, []Instance{ground, caster}
}

func mustShadowRender(t *testing.T, cfg Config, camera Camera, instances ...Instance) *image.RGBA {
	t.Helper()
	r, err := NewRenderer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	img, err := r.Render(camera, instances...)
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func TestShadowAgainstIndependentLightAndCameraRays(t *testing.T) {
	for _, projection := range []Projection{Perspective, Orthographic} {
		for _, scale := range []int{1, 2, 4} {
			for _, triangle := range []bool{false, true} {
				t.Run(fmt.Sprintf("projection%d/ssaa%d/triangle%t", projection, scale, triangle), func(t *testing.T) {
					cfg, camera, instances := shadowFixture(t)
					camera.Projection = projection
					cfg.Supersampling = scale
					if triangle {
						instances[1] = NewInstance(&Mesh{Vertices: []geom.Vec3{{X: -.93, Y: -.32, Z: 1.2}, {X: .83, Y: -.27, Z: 1.6}, {X: .23, Y: 1.17, Z: 1.4}}, Triangles: []Triangle{{0, 1, 2}}})
						// Partial observer near/far and side clipping. Light-space
						// casting still uses the entire original world triangle.
						camera.Near, camera.Far = 7.3, 9.6
						camera.FOVY, camera.OrthoHeight = .42, 3.1
					}
					if scale > 1 {
						instances[0].Material.Texture = primitiveTexture(t, "gradient")
						instances[0].Material.Sampler = Sampler{Filter: Bilinear, AddressU: Repeat, AddressV: Clamp}
					}
					got := mustShadowRender(t, cfg, camera, instances...)
					want := shadowRayReference(t, cfg, camera, instances...)
					for i, ch := range got.Pix {
						if math.Abs(float64(ch)-float64(want.Pix[i])) > 1 {
							t.Fatalf("pixel (%d,%d) channel %d: %d want %d", i/4%cfg.Width, i/4/cfg.Width, i%4, ch, want.Pix[i])
						}
					}
					cfg.Shadow.Enabled = false
					plain := mustShadowRender(t, cfg, camera, instances...)
					darkened := 0
					for i := 0; i < len(got.Pix); i += 4 {
						if int(got.Pix[i])+10 < int(plain.Pix[i]) {
							darkened++
						}
					}
					if darkened < 5 {
						t.Fatalf("insufficient real shadow coverage: %d pixels", darkened)
					}
				})
			}
		}
	}
}

func TestShadowConfigValidation(t *testing.T) {
	base, _, _ := shadowFixture(t)
	invalid := []func(*ShadowConfig){
		func(s *ShadowConfig) { s.Center.X = math.NaN() }, func(s *ShadowConfig) { s.Center.Y = math.Inf(1) }, func(s *ShadowConfig) { s.Center.Z = MaxCoordinate + 1 },
		func(s *ShadowConfig) { s.Size = 0 }, func(s *ShadowConfig) { s.Size = math.NaN() }, func(s *ShadowConfig) { s.Depth = 1e-7 }, func(s *ShadowConfig) { s.Depth = MaxCoordinate + 1 },
		func(s *ShadowConfig) { s.Resolution = 0 }, func(s *ShadowConfig) { s.Resolution = MaxShadowResolution + 1 }, func(s *ShadowConfig) { s.Resolution = int(^uint(0) >> 1) },
		func(s *ShadowConfig) { s.Bias = -1 }, func(s *ShadowConfig) { s.Bias = math.Inf(1) }, func(s *ShadowConfig) { s.Bias = s.Depth + 1 },
		func(s *ShadowConfig) { s.LookupLimit = -1 }, func(s *ShadowConfig) { s.LookupLimit = MaxShadowSamples + 1 }, func(s *ShadowConfig) { s.RasterSampleLimit = -1 }, func(s *ShadowConfig) { s.RasterSampleLimit = int(^uint(0) >> 1) },
	}
	for i, mutate := range invalid {
		cfg := base
		mutate(&cfg.Shadow)
		if _, err := NewRenderer(cfg); err == nil {
			t.Fatalf("invalid config %d accepted: %+v", i, cfg.Shadow)
		}
		cfg.Shadow.Enabled = false
		if _, err := NewRenderer(cfg); err != nil {
			t.Fatalf("disabled config %d affected defaults: %v", i, err)
		}
	}
	base.Light = Light{Ambient: 1}
	if _, err := NewRenderer(base); err == nil {
		t.Fatal("enabled map accepted zero light direction")
	}
	base.Light.Direction = geom.Vec3{Z: 1}
	base.Shadow.Resolution = MaxShadowResolution
	base.Shadow.Bias = base.Shadow.Depth
	base.Shadow.LookupLimit, base.Shadow.RasterSampleLimit = MaxShadowSamples, MaxShadowSamples
	if _, err := NewRenderer(base); err != nil {
		t.Fatal(err)
	} // Validate without allocating a maximum map.
}

func TestShadowRejectsUnsupportedVisibleAlpha(t *testing.T) {
	cfg, camera, instances := shadowFixture(t)
	r, err := NewRenderer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, alpha := range []uint8{1, 127, 254} {
		in := instances[0]
		in.Material.Color.A = alpha
		img, err := r.Render(camera, in)
		if img != nil || err == nil || !strings.Contains(err.Error(), "opaque") {
			t.Fatalf("alpha %d: image %v err %v", alpha, img != nil, err)
		}
	}
	for _, alpha := range []uint8{0, 128, 254} {
		in := instances[0]
		in.Material.Texture = testTexture(t, 2, 2, func(x, y int) color.NRGBA {
			a := uint8(255)
			if x == 1 && y == 1 {
				a = alpha
			}
			return color.NRGBA{R: 255, A: a}
		})
		img, err := r.Render(camera, in)
		if img != nil || err == nil || !strings.Contains(err.Error(), "alpha-textured") {
			t.Fatalf("texture alpha %d: %v", alpha, err)
		}
		in.Material.Color.A = 0
		if _, err := r.Render(camera, in); err != nil {
			t.Fatalf("invisible material: %v", err)
		}
		in.Model = geom.Scaling(0, 1, 1)
		if _, err := r.Render(camera, in); err == nil {
			t.Fatal("invisible invalid model accepted")
		}
	}
	instances[0].Material.Texture = primitiveTexture(t, "checker")
	if _, err := r.Render(camera, instances...); err != nil {
		t.Fatal(err)
	}
}

func TestShadowDisabledByteIdentityAndConfigCopy(t *testing.T) {
	cfg, camera, instances := shadowFixture(t)
	cfg.Shadow = ShadowConfig{}
	instances[0].Material.Texture = primitiveTexture(t, "mixed")
	instances[1].Material.Color.A = 151
	cfg.Supersampling = 2
	want := mustShadowRender(t, cfg, camera, instances...)
	cfg.Shadow = ShadowConfig{Center: geom.Vec3{X: math.NaN()}, Size: -1, Resolution: -1, Bias: math.Inf(1), LookupLimit: -1}
	got := mustShadowRender(t, cfg, camera, instances...)
	if !bytes.Equal(got.Pix, want.Pix) {
		t.Fatal("disabled shadow fields altered frame")
	}
	cfg, camera, instances = shadowFixture(t)
	r, err := NewRenderer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	want, err = r.Render(camera, instances...)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Shadow.Center.X = 900
	cfg.Shadow.Resolution = 1
	got, err = r.Render(camera, instances...)
	if err != nil || !bytes.Equal(want.Pix, got.Pix) {
		t.Fatalf("config was retained by reference: %v", err)
	}
}

func shadowPrepared(t *testing.T, instances []Instance) []preparedInstance {
	t.Helper()
	result := make([]preparedInstance, len(instances))
	for i, in := range instances {
		m, err := prepareModel(in.Model)
		if err != nil {
			t.Fatal(err)
		}
		result[i] = preparedInstance{in, m}
	}
	return result
}

func TestShadowBudgetsAndDepthBounds(t *testing.T) {
	cfg, camera, instances := shadowFixture(t)
	cfg.Width, cfg.Height = 31, 23
	cfg.Shadow.Resolution = 32
	r, err := NewRenderer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	s, err := r.buildShadowMap(shadowPrepared(t, instances))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.depth) != 32*32 || s.rasterSamples == 0 {
		t.Fatal("map size or accounting")
	}
	cfg.Shadow.RasterSampleLimit = int(s.rasterSamples) - 1
	r, _ = NewRenderer(cfg)
	img, err := r.Render(camera, instances...)
	if img != nil || err == nil || !strings.Contains(err.Error(), "raster sample budget") {
		t.Fatalf("raster budget: %v", err)
	}
	cfg.Shadow.RasterSampleLimit++
	if img := mustShadowRender(t, cfg, camera, instances...); img == nil {
		t.Fatal("exact raster budget failed")
	}
	// Obtain the exact receiving count with fresh depth so hidden samples and
	// two triangles on a shared edge exercise the same public render pass.
	r, _ = NewRenderer(cfg)
	s, err = r.buildShadowMap(shadowPrepared(t, instances))
	if err != nil {
		t.Fatal(err)
	}
	f, err := camera.frame(float64(cfg.Width) / float64(cfg.Height))
	if err != nil {
		t.Fatal(err)
	}
	scratch := image.NewRGBA(image.Rect(0, 0, cfg.Width, cfg.Height))
	depth := make([]float64, cfg.Width*cfg.Height)
	for i := range depth {
		depth[i] = math.Inf(1)
	}
	for i, in := range shadowPrepared(t, instances) {
		if err := r.renderWorldInstance(scratch, depth, camera, f, in, i, nil, s, nil); err != nil {
			t.Fatal(err)
		}
	}
	if s.lookups < 2 {
		t.Fatal("missing lookup accounting")
	}
	cfg.Shadow.LookupLimit = int(s.lookups) - 1
	r, _ = NewRenderer(cfg)
	img, err = r.Render(camera, instances...)
	if img != nil || err == nil || !strings.Contains(err.Error(), "lookup budget") {
		t.Fatalf("lookup budget: %v", err)
	}
	cfg.Shadow.LookupLimit++
	want := mustShadowRender(t, cfg, camera, instances...)
	if err == nil || want == nil {
		t.Fatal("budget failure did not recover")
	}
	// Explicitly reject upstream casters, even outside the observer frustum.
	outside := instances[1]
	outside.Model = geom.Translation(geom.Vec3{X: 100, Z: 100})
	r, _ = NewRenderer(cfg)
	img, err = r.Render(camera, outside)
	if img != nil || err == nil || !strings.Contains(err.Error(), "upstream") {
		t.Fatalf("uncovered upstream caster: %v", err)
	}
	// Downstream geometry is clipped and cannot shadow an in-box receiver.
	outside.Model = geom.Translation(geom.Vec3{X: 100, Z: -100})
	if _, err = r.Render(camera, outside); err != nil {
		t.Fatalf("downstream clipping: %v", err)
	}
	// Huge XY crossing polygons must clip before bounded integer raster loops.
	cfg.Light.Direction = geom.Vec3{Z: 1}
	cfg.Shadow = ShadowConfig{Enabled: true, Size: 2, Depth: 4, Resolution: 16, Bias: .01, RasterSampleLimit: 1024}
	large := NewInstance(&Mesh{Vertices: []geom.Vec3{{X: -MaxCoordinate, Y: -MaxCoordinate}, {X: MaxCoordinate, Y: -MaxCoordinate}, {Y: MaxCoordinate}}, Triangles: []Triangle{{0, 1, 2}}})
	mustShadowRender(t, cfg, camera, large)
}

func TestShadowSelfOcclusionBiasAndLocalVolume(t *testing.T) {
	cfg, camera, instances := shadowFixture(t)
	// A sloped light ray and a nearest-texel map need bias for the isolated
	// plane. The configured bias is above this fixture's within-texel slope.
	got := mustShadowRender(t, cfg, camera, instances[0])
	plain := cfg
	plain.Shadow.Enabled = false
	want := mustShadowRender(t, plain, camera, instances[0])
	if !bytes.Equal(got.Pix, want.Pix) {
		t.Fatal("isolated plane self-shadowed with adequate bias")
	}
	// Put the receiver and overhang in ONE indexed mesh. Suppressing all
	// same-object occlusion would incorrectly erase this shadow.
	combined := *instances[0].Mesh
	combined.Vertices = slices.Clone(combined.Vertices)
	combined.UVs = slices.Clone(combined.UVs)
	combined.Triangles = slices.Clone(combined.Triangles)
	combined.Vertices = append(combined.Vertices, geom.Vec3{X: -.8, Y: -.4, Z: 1.3}, geom.Vec3{X: .9, Y: -.4, Z: 1.3}, geom.Vec3{Y: 1, Z: 1.3})
	combined.UVs = append(combined.UVs, UV{}, UV{}, UV{})
	combined.Triangles = append(combined.Triangles, Triangle{4, 5, 6})
	in := NewInstance(&combined)
	got = mustShadowRender(t, cfg, camera, in)
	want = mustShadowRender(t, plain, camera, in)
	if bytes.Equal(got.Pix, want.Pix) {
		t.Fatal("same-mesh overhang failed to self-shadow")
	}
	cfg.Shadow.Bias = cfg.Shadow.Depth
	got = mustShadowRender(t, cfg, camera, in)
	if !bytes.Equal(got.Pix, want.Pix) {
		t.Fatal("maximum bias did not suppress all occlusion")
	}
	// Out-of-volume receiver queries are unshadowed, and are still charged.
	cfg.Shadow.Bias = .05
	cfg.Shadow.Size = .01
	cfg.Shadow.Resolution = 1
	r, _ := NewRenderer(cfg)
	s, err := r.buildShadowMap(shadowPrepared(t, []Instance{in}))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []geom.Vec3{{X: 1}, {Y: 1}, {Z: -100}} {
		blocked, err := s.occluded(p)
		if blocked || err != nil {
			t.Fatalf("outside local volume: %v %v", blocked, err)
		}
	}
	if s.lookups != 3 {
		t.Fatal("outside-volume lookups escaped budget")
	}
}

func TestShadowSubmissionConcurrentAndCameraIndependence(t *testing.T) {
	cfg, camera, instances := shadowFixture(t)
	cfg.Supersampling = 2
	instances[0].Material.Texture = primitiveTexture(t, "checker")
	r, err := NewRenderer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	want, err := r.Render(camera, instances...)
	if err != nil {
		t.Fatal(err)
	}
	instances[0], instances[1] = instances[1], instances[0]
	for i := range instances {
		m := *instances[i].Mesh
		m.Triangles = slices.Clone(m.Triangles)
		slices.Reverse(m.Triangles)
		instances[i].Mesh = &m
	}
	got, err := r.Render(camera, instances...)
	if err != nil || !bytes.Equal(got.Pix, want.Pix) {
		t.Fatalf("submission changed shadow: %v", err)
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			img, err := r.Render(camera, instances...)
			if err != nil || !bytes.Equal(img.Pix, want.Pix) {
				t.Errorf("concurrent render: %v", err)
			}
		}()
	}
	wg.Wait()
	// A caster outside an observer's Far plane still casts into the visible
	// plane. An observer-dependent light projection or caster list fails here.
	cfg.Light.Direction = geom.Vec3{X: 1, Z: 1}
	cfg.Shadow.Bias = .08
	camera = Camera{Position: geom.Vec3{Z: 5}, Up: geom.Vec3{Y: 1}, Projection: Orthographic, OrthoHeight: 5, Near: 4, Far: 6}
	ground := instances[1]
	ground.Material.Texture = nil
	caster := NewInstance(&Mesh{Vertices: []geom.Vec3{{X: -.4, Y: -.5, Z: 2}, {X: .6, Y: -.5, Z: 2}, {Y: .7, Z: 2}}, Triangles: []Triangle{{0, 1, 2}}})
	got = mustShadowRender(t, cfg, camera, ground, caster)
	plain := cfg
	plain.Shadow.Enabled = false
	want = mustShadowRender(t, plain, camera, ground, caster)
	if bytes.Equal(got.Pix, want.Pix) {
		t.Fatal("observer-clipped caster lost its shadow")
	}
	reference := shadowRayReference(t, cfg, camera, ground, caster)
	for i, ch := range got.Pix {
		if math.Abs(float64(ch)-float64(reference.Pix[i])) > 1 {
			t.Fatalf("off-camera caster reference mismatch at %d: %d want %d", i, ch, reference.Pix[i])
		}
	}
}

func TestShadowPointsRemainUnlitAndDoNotCast(t *testing.T) {
	cfg, camera, instances := shadowFixture(t)
	cfg.Supersampling = 4
	positions := []geom.Vec3{{Z: 1.8}, {X: 1, Z: .7}, {X: -1, Z: -.4}}
	colors := []color.NRGBA{{R: 255, G: 31, B: 63, A: 255}, {R: 14, G: 240, B: 120, A: 128}, {R: 3, G: 19, B: 255, A: 255}}
	cloud, err := NewPointCloud(positions, colors, []float64{3.7, 5.1, 7})
	if err != nil {
		t.Fatal(err)
	}
	r, _ := NewRenderer(cfg)
	got, err := r.RenderWithPoints(camera, []*PointCloud{cloud}, instances[0])
	if err != nil {
		t.Fatal(err)
	}
	cfg.Shadow.Enabled = false
	r, _ = NewRenderer(cfg)
	want, err := r.RenderWithPoints(camera, []*PointCloud{cloud}, instances[0])
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Pix, want.Pix) {
		t.Fatal("points lit/received shadows, cast on plane, or lost ordinary alpha/depth")
	}
	// A point-only scene also remains byte-identical, including SSAA edges.
	plain, err := r.RenderWithPoints(camera, []*PointCloud{cloud})
	if err != nil {
		t.Fatal(err)
	}
	cfg.Shadow.Enabled = true
	r, _ = NewRenderer(cfg)
	got, err = r.RenderWithPoints(camera, []*PointCloud{cloud})
	if err != nil || !bytes.Equal(got.Pix, plain.Pix) {
		t.Fatalf("point-only shadow config: %v", err)
	}
}
