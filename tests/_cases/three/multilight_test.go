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

func multiLightSources() []PointLightSource {
	return []PointLightSource{
		{Position: geom.Vec3{X: 1.3, Y: -.6, Z: .7}, RGB: [3]float64{1, .17, .03}, Diffuse: .89, Attenuation: .17},
		{Position: geom.Vec3{X: -2.1, Y: 1.3, Z: -1.7}, RGB: [3]float64{.02, .9, 1}, Diffuse: .73, Attenuation: .023},
		{Position: geom.Vec3{X: .1, Y: -3.7, Z: -8}, RGB: [3]float64{.19, 1, .07}, Diffuse: .67, Attenuation: .003},
	}
}

func multiLightConfig(c Config) Config {
	c.Light = Light{Kind: PointLightList, Ambient: .13}
	c.PointLights = multiLightSources()
	return c
}

// Independent scalar math: direct squared-distance/dot products, without any
// production normalization, pointDiffuse or pointIntensities helper.
func multiLightReferenceIntensity(c Config, p, n geom.Vec3) [3]float64 {
	rgb := [3]float64{c.Light.Ambient, c.Light.Ambient, c.Light.Ambient}
	for _, source := range c.PointLights {
		d := source.Position.Sub(p)
		d2 := d.Dot(d)
		if d2 == 0 {
			continue
		}
		energy := source.Diffuse * math.Max(0, n.Dot(d)/math.Sqrt(d2)) / (1 + source.Attenuation*d2)
		for i := range rgb {
			rgb[i] += source.RGB[i] * energy
		}
	}
	for i := range rgb {
		rgb[i] = math.Min(1, rgb[i])
	}
	return rgb
}

func TestMultiLightValidation(t *testing.T) {
	base := multiLightConfig(DefaultConfig(7, 5))
	mutations := []func(*Config){
		func(c *Config) { c.Light.Kind = PointLight },
		func(c *Config) { c.Light.Kind = DirectionalLight },
		func(c *Config) { c.Light.Direction.X = 1 },
		func(c *Config) { c.Light.Direction.X = math.NaN() },
		func(c *Config) { c.Light.Position.Z = 1 },
		func(c *Config) { c.Light.Attenuation = .1 },
		func(c *Config) { c.Light.Diffuse = .1 },
		func(c *Config) { c.Light.Ambient = math.NaN() },
		func(c *Config) { c.Light.Ambient = 1.01 },
		func(c *Config) { c.Shadow.Enabled = true },
		func(c *Config) { c.PointLights = make([]PointLightSource, MaxPointLights+1) },
		func(c *Config) { c.PointLights[1].Position.X = math.NaN() },
		func(c *Config) { c.PointLights[1].Position.Y = math.Inf(-1) },
		func(c *Config) { c.PointLights[1].Position.Z = MaxCoordinate + 1 },
		func(c *Config) { c.PointLights[1].Attenuation = -1 },
		func(c *Config) { c.PointLights[1].Attenuation = math.NaN() },
		func(c *Config) { c.PointLights[1].Attenuation = math.Inf(1) },
		func(c *Config) { c.PointLights[1].Attenuation = MaxLightAttenuation + 1 },
		func(c *Config) { c.PointLights[1].Diffuse = -1 },
		func(c *Config) { c.PointLights[1].Diffuse = 1.01 },
		func(c *Config) { c.PointLights[1].Diffuse = math.NaN() },
		func(c *Config) { c.PointLights[1].RGB[0] = -math.SmallestNonzeroFloat64 },
		func(c *Config) { c.PointLights[1].RGB[1] = 1.01 },
		func(c *Config) { c.PointLights[1].RGB[2] = math.NaN() },
		func(c *Config) { c.PointLights[1].RGB[0] = math.Inf(1) },
		func(c *Config) { c.PointLightEvaluationLimit = -1 },
		func(c *Config) { c.PointLightEvaluationLimit = MaxPointLightEvaluations + 1 },
	}
	for i, mutate := range mutations {
		c := base
		c.PointLights = slices.Clone(base.PointLights)
		mutate(&c)
		if r, err := NewRenderer(c); r != nil || err == nil {
			t.Fatalf("invalid config %d accepted", i)
		}
	}
	for _, c := range []Config{
		{Width: 1, Height: 1, PointLightEvaluationLimit: 1},
		{Width: 1, Height: 1, Light: Light{Kind: PointLight}, PointLightEvaluationLimit: 1},
	} {
		if _, err := NewRenderer(c); err == nil {
			t.Fatal("legacy mode silently ignored list budget")
		}
	}
	// Even invisible sources must validate all fields; count limit is inclusive.
	c := base
	c.PointLights = make([]PointLightSource, MaxPointLights)
	for i := range c.PointLights {
		c.PointLights[i] = PointLightSource{Position: geom.Vec3{X: MaxCoordinate, Y: -MaxCoordinate}, Attenuation: MaxLightAttenuation}
	}
	c.PointLightEvaluationLimit = MaxPointLightEvaluations
	if _, err := NewRenderer(c); err != nil {
		t.Fatal(err)
	}
	c.PointLights[0].Position.X = math.Inf(1)
	if _, err := NewRenderer(c); err == nil {
		t.Fatal("zero-strength source skipped validation")
	}
}

func TestMultiLightAgainstIndependentRays(t *testing.T) {
	for _, mode := range []string{"constant", "opaque-nearest", "opaque-bilinear", "mixed-nearest", "mixed-bilinear", "tinted-bilinear"} {
		for _, smooth := range []bool{false, true} {
			for _, projection := range []Projection{Perspective, Orthographic} {
				for _, scale := range []int{1, 2, 4} {
					for _, clipped := range []bool{false, true} {
						t.Run(fmt.Sprintf("%s/smooth%t/p%d/s%d/clip%t", mode, smooth, projection, scale, clipped), func(t *testing.T) {
							c, camera, instances := pointLightRayFixture(t, mode, smooth)
							c = multiLightConfig(c)
							c.Supersampling, camera.Projection = scale, projection
							if clipped {
								camera.Position = geom.Vec3{X: 1.7, Y: .8, Z: 1.2}
								camera.Target = geom.Vec3{Z: -3}
								camera.Near, camera.Far = 2.1, 7.2
								camera.FOVY, camera.OrthoHeight = .61, 3.17
								c.CullBackfaces = true
							}
							got := mustShadowRender(t, c, camera, instances...)
							want := pointLightRayReferenceWithLighting(t, c, camera, func(p, n geom.Vec3) [3]float64 { return multiLightReferenceIntensity(c, p, n) }, instances...)
							for i, ch := range got.Pix {
								if math.Abs(float64(ch)-float64(want.Pix[i])) > 1 {
									t.Fatalf("pixel%d,%d channel%d: %d want%d", i/4%c.Width, i/4/c.Width, i%4, ch, want.Pix[i])
								}
							}
						})
					}
				}
			}
		}
	}
}

func TestMultiLightAnalyticLimitsAndCancellation(t *testing.T) {
	c := multiLightConfig(DefaultConfig(1, 1))
	c.Light.Ambient = .2
	c.PointLights = []PointLightSource{
		{Position: geom.Vec3{Z: 2}, RGB: [3]float64{1, 0, 0}, Diffuse: .7},
		{Position: geom.Vec3{Z: -2}, RGB: [3]float64{0, 1, 0}, Diffuse: .9},
	}
	for _, tc := range []struct {
		normal geom.Vec3
		want   [3]float64
	}{
		{geom.Vec3{Z: 1}, [3]float64{.9, .2, .2}},
		{geom.Vec3{Z: -1}, [3]float64{.2, 1, .2}},
		{geom.Vec3{}, [3]float64{.2, .2, .2}},
	} {
		r, _ := NewRenderer(c)
		got, err := r.pointIntensities(geom.Vec3{}, tc.normal, &pointLightWork{remaining: 2})
		for i := range got {
			if err != nil || math.Abs(got[i]-tc.want[i]) > 1e-15 {
				t.Fatal("opposing sources canceled or ambient repeated", got, tc.want, err)
			}
		}
	}
	for _, tc := range []struct {
		source PointLightSource
		world  geom.Vec3
		want   float64
	}{
		{PointLightSource{Position: geom.Vec3{X: math.SmallestNonzeroFloat64}, RGB: [3]float64{1, 1, 1}, Diffuse: 1}, geom.Vec3{}, 1},
		{PointLightSource{Position: geom.Vec3{X: MaxCoordinate}, RGB: [3]float64{1, 1, 1}, Diffuse: 1, Attenuation: MaxLightAttenuation}, geom.Vec3{X: -MaxCoordinate}, 1 / (1 + 4e36)},
		{PointLightSource{RGB: [3]float64{1, 1, 1}, Diffuse: 1}, geom.Vec3{}, 0},
	} {
		c.Light.Ambient = 0
		c.PointLights = []PointLightSource{tc.source}
		r, _ := NewRenderer(c)
		got, err := r.pointIntensities(tc.world, geom.Vec3{X: 1}, &pointLightWork{remaining: 1})
		for _, ch := range got {
			if err != nil || !finite(ch) || math.Abs(ch-tc.want) > math.Abs(tc.want)*1e-14 {
				t.Fatal("extreme source", got, tc.want, err)
			}
		}
	}
	c.PointLights = make([]PointLightSource, MaxPointLights)
	for i := range c.PointLights {
		c.PointLights[i] = PointLightSource{Position: geom.Vec3{Z: 1}, RGB: [3]float64{1, .01, 0}, Diffuse: 1}
	}
	r, _ := NewRenderer(c)
	got, err := r.pointIntensities(geom.Vec3{}, geom.Vec3{Z: 1}, &pointLightWork{remaining: MaxPointLights})
	if err != nil || got[0] != 1 || math.Abs(got[1]-.16) > 1e-15 || got[2] != 0 {
		t.Fatal("channel saturation", got, err)
	}
	if _, err := r.pointIntensities(geom.Vec3{X: math.NaN()}, geom.Vec3{Z: 1}, &pointLightWork{remaining: MaxPointLights}); err == nil {
		t.Fatal("nonfinite interpolated world position accepted")
	}
	// Actual interpolation of opposite authored normals cancels at this ray.
	c.Light.Ambient = .2
	m := &Mesh{Vertices: []geom.Vec3{{X: -2, Y: -1, Z: -3}, {X: 2, Y: -1, Z: -3}, {Y: 1, Z: -3}}, Triangles: []Triangle{{0, 1, 2}}, Normals: []geom.Vec3{{Z: 1}, {Z: 1}, {Z: -1}}}
	in := NewInstance(m)
	in.Material.Color = color.NRGBA{R: 200, G: 100, B: 50, A: 255}
	if got := mustShadowRender(t, c, testCamera(Orthographic), in).RGBAAt(0, 0); got != (color.RGBA{R: 40, G: 20, B: 10, A: 255}) {
		t.Fatal("canceling interpolated normals", got)
	}
}

func TestMultiLightBudgetAcrossPassesAndOcclusion(t *testing.T) {
	for _, scale := range []int{1, 2, 4} {
		for _, textured := range []bool{false, true} {
			c := multiLightConfig(DefaultConfig(7, 5))
			c.Supersampling = scale
			back := NewInstance(uvQuad(20, 20, 4))
			front := NewInstance(uvQuad(20, 20, 2))
			front.Material.Color.A = 137
			if textured {
				front.Material.Texture = testTexture(t, 1, 1, func(int, int) color.NRGBA { return color.NRGBA{R: 255, G: 71, B: 97, A: 151} })
			}
			hidden := front
			hidden.Mesh = uvQuad(20, 20, 5)
			c.PointLightEvaluationLimit = c.Width * c.Height * scale * scale * len(c.PointLights) * 2
			camera := testCamera(Perspective)
			want := mustShadowRender(t, c, camera, front, back, hidden)
			c.PointLightEvaluationLimit--
			r, err := NewRenderer(c)
			if err != nil {
				t.Fatal(err)
			}
			if img, err := r.Render(camera, back, front, hidden); img != nil || err == nil || !strings.Contains(err.Error(), "light evaluation budget") {
				t.Fatalf("s%d tex%t budget overflow: %v", scale, textured, err)
			}
			if img, err := r.Render(camera, back); err != nil || img == nil {
				t.Fatal("overflow poisoned later frame", err)
			}
			c.PointLightEvaluationLimit++
			if got := mustShadowRender(t, c, camera, back, hidden, front); !bytes.Equal(got.Pix, want.Pix) {
				t.Fatal("transparent permutation changed image")
			}
			c.PointLightEvaluationLimit = c.Width * c.Height * scale * scale * len(c.PointLights)
			// Opaque overdraw is charged when it survives the current depth;
			// nearest-first submission avoids shading the hidden opaque surface.
			near := NewInstance(uvQuad(20, 20, 1))
			r, _ = NewRenderer(c)
			if _, err := r.Render(camera, near, back); err != nil {
				t.Fatal("occluded sample charged", err)
			}
			if img, err := r.Render(camera, back, near); img != nil || err == nil {
				t.Fatal("opaque overdraw not charged")
			}
		}
	}
}

func TestMultiLightLegacyEquivalenceAndEmpty(t *testing.T) {
	for _, mode := range []string{"constant", "mixed-bilinear", "opaque-nearest"} {
		for _, scale := range []int{1, 2, 4} {
			c, camera, instances := pointLightRayFixture(t, mode, true)
			c.Supersampling = scale
			want := mustShadowRender(t, c, camera, instances...)
			old := c.Light
			c.Light = Light{Kind: PointLightList, Ambient: old.Ambient}
			c.PointLights = []PointLightSource{{Position: old.Position, RGB: [3]float64{1, 1, 1}, Diffuse: old.Diffuse, Attenuation: old.Attenuation}}
			if got := mustShadowRender(t, c, camera, instances...); !bytes.Equal(got.Pix, want.Pix) {
				t.Fatal("one white list source differs from legacy point")
			}
			c.PointLights = nil
			got := mustShadowRender(t, c, camera, instances...)
			c.Light.Kind = PointLight
			want = mustShadowRender(t, c, camera, instances...)
			if !bytes.Equal(got.Pix, want.Pix) {
				t.Fatal("empty list ambient counted incorrectly")
			}
		}
	}
}

func TestMultiLightSkippedAndZeroStrengthBudget(t *testing.T) {
	c := multiLightConfig(DefaultConfig(1, 1))
	c.PointLightEvaluationLimit = 1 // Fewer than the three sources.
	in := NewInstance(uvQuad(20, 20, 3))
	camera := testCamera(Orthographic)
	for _, skip := range []string{"alpha-zero", "empty-texel", "clipped"} {
		hidden := in
		switch skip {
		case "alpha-zero":
			hidden.Material.Color.A = 0
		case "empty-texel":
			hidden.Material.Texture = testTexture(t, 1, 1, func(int, int) color.NRGBA { return color.NRGBA{R: 255} })
		case "clipped":
			hidden.Model = geom.Translation(geom.Vec3{X: 100})
		}
		if got := mustShadowRender(t, c, camera, hidden); got.RGBAAt(0, 0) != (color.RGBA{}) {
			t.Fatal("skipped sample changed image", skip)
		}
	}
	// A disabled or black source still reserves its list slot/evaluation.
	for _, disabled := range []bool{false, true} {
		for i := range c.PointLights {
			if disabled {
				c.PointLights[i].Diffuse = 0
			} else {
				c.PointLights[i].RGB = [3]float64{}
			}
		}
		r, err := NewRenderer(c)
		if err != nil {
			t.Fatal(err)
		}
		if img, err := r.Render(camera, in); img != nil || err == nil || !strings.Contains(err.Error(), "evaluation budget") {
			t.Fatal("zero energy bypassed budget", err)
		}
		if _, err := r.pointIntensities(geom.Vec3{}, geom.Vec3{}, &pointLightWork{remaining: 1}); err == nil {
			t.Fatal("canceled normal bypassed evaluation budget")
		}
	}
	c.PointLights = nil
	c.Light.Ambient = .2
	if got := mustShadowRender(t, c, camera, in).RGBAAt(0, 0); got != (color.RGBA{R: 51, G: 51, B: 51, A: 255}) {
		t.Fatal("empty list spent evaluation budget", got)
	}
	// The independent transparent-fragment quota still fails the whole image.
	c = multiLightConfig(DefaultConfig(3, 3))
	c.TransparentFragmentLimit = 1
	in.Material.Color.A = 137
	r, err := NewRenderer(c)
	if err != nil {
		t.Fatal(err)
	}
	if img, err := r.Render(camera, in); img != nil || err == nil || !strings.Contains(err.Error(), "fragment budget") {
		t.Fatal("point list bypassed fragment budget", err)
	}
}

func TestMultiLightInputCopyConcurrentAndPermutation(t *testing.T) {
	c, camera, instances := pointLightRayFixture(t, "mixed-bilinear", true)
	c = multiLightConfig(c)
	c.Supersampling = 2
	original := slices.Clone(c.PointLights)
	r, err := NewRenderer(c)
	if err != nil {
		t.Fatal(err)
	}
	want, err := r.Render(camera, instances...)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(c.PointLights, original) {
		t.Fatal("constructor sorted caller list")
	}
	var wg sync.WaitGroup
	start := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		for range 100 {
			for i := range c.PointLights {
				c.PointLights[i].Position.X = math.NaN()
				c.PointLights[i].RGB = [3]float64{math.Inf(1)}
			}
		}
	}()
	for worker := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			order := slices.Clone(instances)
			if worker%2 == 1 {
				slices.Reverse(order)
			}
			for range 4 {
				img, err := r.Render(camera, order...)
				if err != nil || !bytes.Equal(img.Pix, want.Pix) {
					t.Errorf("concurrent owned config: %v", err)
					return
				}
			}
		}()
	}
	close(start)
	wg.Wait()
	c.PointLights = original
	for shift := range len(original) {
		c.PointLights = append(slices.Clone(original[shift:]), original[:shift]...)
		if got := mustShadowRender(t, c, camera, instances...); !bytes.Equal(got.Pix, want.Pix) {
			t.Fatal("light permutation changed pixels")
		}
	}
	offset := geom.Vec3{X: 7, Y: -11, Z: 13}
	for i := range c.PointLights {
		c.PointLights[i].Position = c.PointLights[i].Position.Add(offset)
	}
	camera.Position, camera.Target = camera.Position.Add(offset), camera.Target.Add(offset)
	for i := range instances {
		instances[i].Model = geom.Translation(offset).Mul(instances[i].Model)
	}
	got := mustShadowRender(t, c, camera, instances...)
	for i, ch := range got.Pix {
		if math.Abs(float64(ch)-float64(want.Pix[i])) > 1 {
			t.Fatal("common world translation changed lighting", i, ch, want.Pix[i])
		}
	}
}

func TestMultiLightMixedUnlitPoints(t *testing.T) {
	for _, scale := range []int{1, 2, 4} {
		c := multiLightConfig(DefaultConfig(1, 1))
		c.Supersampling = scale
		front := NewInstance(uvQuad(20, 20, 2))
		front.Material.Color = color.NRGBA{R: 211, G: 51, B: 77, A: 131}
		back := NewInstance(uvQuad(20, 20, 4))
		back.Material.Color = color.NRGBA{R: 31, G: 171, B: 239, A: 255}
		point := color.NRGBA{R: 171, G: 213, B: 23, A: 137}
		cloud, err := NewPointCloud([]geom.Vec3{{Z: -3}, {Z: -5}}, []color.NRGBA{point, {B: 255, A: 255}}, []float64{3, 3})
		if err != nil {
			t.Fatal(err)
		}
		r, _ := NewRenderer(c)
		img, err := r.RenderWithPoints(testCamera(Orthographic), []*PointCloud{cloud}, front, back)
		if err != nil {
			t.Fatal(err)
		}
		var sum [4]int
		for y := range scale {
			for x := range scale {
				wx, wy := 4*(float64(x)+.5)/float64(scale)-2, 2-4*(float64(y)+.5)/float64(scale)
				shade := func(in Instance, z float64) color.NRGBA {
					i := multiLightReferenceIntensity(c, geom.Vec3{X: wx, Y: wy, Z: -z}, geom.Vec3{Z: 1})
					b := in.Material.Color
					b.R = uint8(math.Round(float64(b.R) * i[0]))
					b.G = uint8(math.Round(float64(b.G) * i[1]))
					b.B = uint8(math.Round(float64(b.B) * i[2]))
					return b
				}
				out := referenceTransparentLayers(c.Background, shade(front, 2), point, shade(back, 4))
				for i, ch := range [4]uint8{out.R, out.G, out.B, out.A} {
					sum[i] += int(ch)
				}
			}
		}
		for i, ch := range img.Pix {
			if int(ch) != (sum[i]+scale*scale/2)/(scale*scale) {
				t.Fatal("lit meshes/unlit disks independent composition", scale, img.Pix, sum)
			}
		}
		c.PointLightEvaluationLimit = 1
		r, _ = NewRenderer(c)
		if img, err := r.RenderWithPoints(testCamera(Orthographic), []*PointCloud{cloud}); err != nil || img == nil {
			t.Fatal("unlit points consumed light budget", err)
		}
		near, err := NewPointCloud([]geom.Vec3{{Z: -1}}, []color.NRGBA{{R: point.R, G: point.G, B: point.B, A: 255}}, []float64{3})
		if err != nil {
			t.Fatal(err)
		}
		c.PointLightEvaluationLimit = 0
		r, _ = NewRenderer(c)
		img, err = r.RenderWithPoints(testCamera(Orthographic), []*PointCloud{near, cloud}, front, back)
		if err != nil || img.RGBAAt(0, 0) != (color.RGBA{R: point.R, G: point.G, B: point.B, A: 255}) {
			t.Fatal("opaque point depth", err)
		}
	}
}
