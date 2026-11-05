package three

import (
	"bytes"
	"gomath/geom"
	"image/color"
	"math"
	"slices"
	"testing"
)

// This analytic footprint uses only plane coordinates and ray/AABB slabs;
// it does not know about either the triangle rasterizer or the ray-triangle
// reference. Exclude the texel-width uncertainty band of a hard shadow map.
func TestShadowAnalyticPlaneBoxFootprint(t *testing.T) {
	cfg, _, instances := shadowFixture(t)
	cfg.Width, cfg.Height = 111, 103
	cfg.Light = Light{Direction: geom.Vec3{X: .6, Y: -.2, Z: 1}, Ambient: .2, Diffuse: .8}
	cfg.Shadow.Resolution, cfg.Shadow.Bias = 512, .025
	camera := Camera{Position: geom.Vec3{Z: 5}, Up: geom.Vec3{Y: 1}, Projection: Orthographic, OrthoHeight: 5, Near: 3.9, Far: 6}
	box, err := Box(1.12, .94, .8)
	if err != nil {
		t.Fatal(err)
	}
	instances[1] = NewInstance(box)
	instances[1].Model = geom.Translation(geom.Vec3{X: .18, Y: .11, Z: 1.6}) // z in [1.2,2], entirely clipped by observer.
	got := mustShadowRender(t, cfg, camera, instances...)
	light := cfg.Light.Direction
	blocked := func(x, y float64) bool {
		enter, exit := 0.0, math.Inf(1)
		for _, a := range [][4]float64{{x, light.X, .18 - .56, .18 + .56}, {y, light.Y, .11 - .47, .11 + .47}, {0, light.Z, 1.2, 2}} {
			lo, hi := (a[2]-a[0])/a[1], (a[3]-a[0])/a[1]
			if lo > hi {
				lo, hi = hi, lo
			}
			enter, exit = math.Max(enter, lo), math.Min(exit, hi)
		}
		return exit >= enter
	}
	shadowed, lit := 0, 0
	for y := range cfg.Height {
		for x := range cfg.Width {
			wx := (2*(float64(x)+.5)/float64(cfg.Width) - 1) * float64(cfg.Width) / float64(cfg.Height) * camera.OrthoHeight / 2
			wy := (1 - 2*(float64(y)+.5)/float64(cfg.Height)) * camera.OrthoHeight / 2
			state := blocked(wx, wy)
			stable := true
			// A conservative 0.08 world-unit band covers this map's diagonal
			// texel footprint and constant-bias displacement at caster edges.
			for _, delta := range []geom.Vec3{{X: .08}, {X: -.08}, {Y: .08}, {Y: -.08}, {X: .08, Y: .08}, {X: -.08, Y: -.08}} {
				if blocked(wx+delta.X, wy+delta.Y) != state {
					stable = false
				}
			}
			if !stable {
				continue
			}
			intensity := .2 + .8/math.Sqrt(1+.6*.6+.2*.2)
			if state {
				intensity = .2
				shadowed++
			} else {
				lit++
			}
			base := instances[0].Material.Color
			want := color.RGBA{R: uint8(math.Round(float64(base.R) * intensity)), G: uint8(math.Round(float64(base.G) * intensity)), B: uint8(math.Round(float64(base.B) * intensity)), A: 255}
			if ch := got.RGBAAt(x, y); ch != want {
				t.Fatalf("analytic box footprint (%g,%g): %v want %v", wx, wy, ch, want)
			}
		}
	}
	if shadowed < 100 || lit < 1000 {
		t.Fatalf("insufficient analytic coverage: shadow %d lit %d", shadowed, lit)
	}
}

func TestShadowLightClipSharedEdgesAndTwoSidedCasters(t *testing.T) {
	cfg, _, instances := shadowFixture(t)
	cfg.Light.Direction = geom.Vec3{Z: 1}
	cfg.Shadow = ShadowConfig{Enabled: true, Size: 2, Depth: 4, Resolution: 33, Bias: .01}
	// Both triangles cross light-space sides. Their clipped shared diagonal
	// crosses exact texel centers; every texel must contain plane depth zero.
	mesh := *instances[0].Mesh
	mesh.Vertices = []geom.Vec3{{X: -2.7, Y: -2.7}, {X: 2.7, Y: -2.7}, {X: 2.7, Y: 2.7}, {X: -2.7, Y: 2.7}}
	in := NewInstance(&mesh)
	for _, backfaces := range []bool{false, true} {
		cfg.CullBackfaces = backfaces
		r, err := NewRenderer(cfg)
		if err != nil {
			t.Fatal(err)
		}
		s, err := r.buildShadowMap(shadowPrepared(t, []Instance{in}))
		if err != nil {
			t.Fatal(err)
		}
		for i, d := range s.depth {
			if math.Abs(d) > 1e-12 || math.IsInf(d, 0) {
				t.Fatalf("clipped light-space crack at texel %d: %g", i, d)
			}
		}
		mesh.Triangles = slices.Clone(mesh.Triangles)
		for i := range mesh.Triangles {
			mesh.Triangles[i][1], mesh.Triangles[i][2] = mesh.Triangles[i][2], mesh.Triangles[i][1]
		}
		in.Mesh = &mesh
		reversed, err := r.buildShadowMap(shadowPrepared(t, []Instance{in}))
		if err != nil {
			t.Fatal(err)
		}
		for i, d := range reversed.depth {
			if math.Abs(d-s.depth[i]) > 1e-12 {
				t.Fatalf("two-sided caster changed texel %d", i)
			}
		}
	}
}

func TestShadowStrictDepthAndClosedVolumeBoundaries(t *testing.T) {
	cfg, camera, instances := shadowFixture(t)
	cfg.Light.Direction = geom.Vec3{Z: 1}
	cfg.Shadow = ShadowConfig{Enabled: true, Size: 4, Depth: 8, Resolution: 1, Bias: .25, LookupLimit: 9}
	r, _ := NewRenderer(cfg)
	s, err := r.buildShadowMap(shadowPrepared(t, instances[:1]))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		point   geom.Vec3
		blocked bool
	}{
		{geom.Vec3{}, false}, {geom.Vec3{Z: -.25}, false}, {geom.Vec3{Z: -.250001}, true},
		{geom.Vec3{X: 2, Y: 2, Z: -1}, true}, {geom.Vec3{X: -2, Y: -2, Z: -1}, true},
		{geom.Vec3{X: 2.000001, Z: -1}, false}, {geom.Vec3{Z: -4}, true}, {geom.Vec3{Z: -4.000001}, false},
	} {
		got, err := s.occluded(tc.point)
		if err != nil || got != tc.blocked {
			t.Fatalf("boundary %v: %v %v want %v", tc.point, got, err, tc.blocked)
		}
	}
	if _, err := s.occluded(geom.Vec3{X: math.Inf(1)}); err == nil {
		t.Fatal("nonfinite receiver accepted")
	}
	if _, err := s.occluded(geom.Vec3{}); err == nil {
		t.Fatal("exact lookup limit exceeded without error")
	}
	// Ambient is unaffected. No lookup work is necessary with zero diffuse.
	cfg.Light.Ambient, cfg.Light.Diffuse = 1, 0
	cfg.Shadow.LookupLimit = 1
	got := mustShadowRender(t, cfg, camera, instances...)
	cfg.Shadow.Enabled = false
	want := mustShadowRender(t, cfg, camera, instances...)
	if !bytes.Equal(got.Pix, want.Pix) {
		t.Fatal("ambient-only lighting changed")
	}
}

func TestShadowedPlaneKeepsPointsUnlit(t *testing.T) {
	cfg, _, instances := shadowFixture(t)
	cfg.Width, cfg.Height = 101, 101
	cfg.Supersampling = 4
	cfg.Light.Direction = geom.Vec3{X: 1, Z: 1}
	cfg.Shadow.Bias = .08
	camera := Camera{Position: geom.Vec3{Z: 5}, Up: geom.Vec3{Y: 1}, Projection: Orthographic, OrthoHeight: 5, Near: 2, Far: 6}
	caster := NewInstance(&Mesh{Vertices: []geom.Vec3{{X: -1, Y: -1, Z: 2}, {X: 1, Y: -1, Z: 2}, {Y: 1.4, Z: 2}}, Triangles: []Triangle{{0, 1, 2}}})
	// Center world x=-1.8 lies well inside the triangle shadow. All point
	// center samples are well inside the screen circle. A rear point is hidden.
	for _, alpha := range []uint8{128, 255} {
		c := color.NRGBA{R: 253, G: 18, B: 91, A: alpha}
		cloud, err := NewPointCloud([]geom.Vec3{{X: -1.8, Z: .2}, {X: -1.8, Z: -.2}}, []color.NRGBA{c, {R: 11, G: 255, A: 255}}, []float64{5, 8})
		if err != nil {
			t.Fatal(err)
		}
		r, _ := NewRenderer(cfg)
		plane, err := r.Render(camera, instances[0], caster)
		if err != nil {
			t.Fatal(err)
		}
		got, err := r.RenderWithPoints(camera, []*PointCloud{cloud}, instances[0], caster)
		if err != nil {
			t.Fatal(err)
		}
		pixel := got.RGBAAt(14, 50)
		under := plane.RGBAAt(14, 50)
		if under.R != uint8(math.Round(float64(instances[0].Material.Color.R)*cfg.Light.Ambient)) {
			t.Fatalf("point fixture not shadowed: %v", under)
		}
		want := referenceTextureLayers(color.NRGBA{R: under.R, G: under.G, B: under.B, A: under.A}, texturedRayLayer{depth: 1, rgb: c, alpha: float64(alpha) / 255})
		if pixel != want {
			t.Fatalf("unlit point alpha %d: %v want %v", alpha, pixel, want)
		}
	}
}

func TestShadowClippedLightVolumeAndSmoothNormalsReference(t *testing.T) {
	for _, smooth := range []bool{false, true} {
		for _, projection := range []Projection{Perspective, Orthographic} {
			cfg, camera, instances := shadowFixture(t)
			cfg.Width, cfg.Height = 61, 47
			cfg.Supersampling = 2
			camera.Projection = projection
			if smooth {
				sphere, err := Sphere(.8, 16, 8)
				if err != nil {
					t.Fatal(err)
				}
				instances[1] = NewInstance(sphere)
				instances[1].Model = geom.Translation(geom.Vec3{X: -.6, Y: .2, Z: .9}).Mul(geom.Rotation(geom.Vec3{X: 1, Y: 1}, .27)).Mul(geom.Scaling(-1, .7, 1.3))
			} else {
				// The sloping receiver crosses the map's far and side planes.
				// Its out-of-volume parts remain lit by the explicit local policy.
				instances[0].Mesh = &Mesh{Vertices: []geom.Vec3{{X: -3, Y: -2, Z: -2.4}, {X: 3, Y: -2, Z: -2.4}, {X: 3, Y: 2}, {X: -3, Y: 2}}, Triangles: []Triangle{{0, 1, 2}, {0, 2, 3}}, UVs: []UV{{}, {U: 1}, {U: 1, V: 1}, {V: 1}}}
				instances[0].Material.Texture = primitiveTexture(t, "gradient")
				instances[0].Material.Sampler = Sampler{Filter: Bilinear}
				cfg.Light.Direction = geom.Vec3{Z: 1}
				cfg.Shadow = ShadowConfig{Enabled: true, Center: geom.Vec3{Z: .5}, Size: 4, Depth: 3, Resolution: 128, Bias: .06}
				camera.Target.Z = -.7
			}
			cfg.CullBackfaces = true
			got := mustShadowRender(t, cfg, camera, instances...)
			want := shadowRayReference(t, cfg, camera, instances...)
			for i, ch := range got.Pix {
				if math.Abs(float64(ch)-float64(want.Pix[i])) > 1 {
					t.Fatalf("smooth %t projection %d sample channel %d: %d want %d", smooth, projection, i, ch, want.Pix[i])
				}
			}
		}
	}
}
