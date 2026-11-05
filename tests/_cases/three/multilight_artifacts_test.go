package three

import (
	"fmt"
	"gomath/geom"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// Same world/materials viewed from two eyes. Columns: legacy single white
// source, three colored sources, independent colored-light ray reference.
func TestMultiLightArtifacts(t *testing.T) {
	c, camera, instances := shadowFixture(t)
	c.Width, c.Height, c.Supersampling = 240, 180, 2
	c.Shadow = ShadowConfig{}
	instances[0].Material.Texture = primitiveTexture(t, "checker")
	instances[0].Material.Sampler = Sampler{Filter: Bilinear, AddressU: Repeat}
	instances[1].Material.Color = color.NRGBA{R: 231, G: 213, B: 193, A: 183}
	instances[1].Model = geom.Translation(geom.Vec3{Z: .08}).Mul(instances[1].Model)
	c = multiLightConfig(c)
	c.PointLights = []PointLightSource{
		{Position: geom.Vec3{X: 1.2, Y: -2, Z: 3}, RGB: [3]float64{1, .1, .03}, Diffuse: 1, Attenuation: .06},
		{Position: geom.Vec3{X: -2.3, Y: 1.2, Z: 2.5}, RGB: [3]float64{.04, .31, 1}, Diffuse: .9, Attenuation: .04},
		{Position: geom.Vec3{Y: 2.5, Z: 3}, RGB: [3]float64{.1, .9, .17}, Diffuse: .43, Attenuation: .08},
	}
	sheet := image.NewRGBA(image.Rect(0, 0, 720, 360))
	for row := range 2 {
		if row == 1 {
			camera.Position = geom.Vec3{X: -3.8, Y: -4.5, Z: 3.1}
		}
		got := mustShadowRender(t, c, camera, instances...)
		want := pointLightRayReferenceWithLighting(t, c, camera, func(p, n geom.Vec3) [3]float64 { return multiLightReferenceIntensity(c, p, n) }, instances...)
		for i, ch := range got.Pix {
			if math.Abs(float64(ch)-float64(want.Pix[i])) > 1 {
				t.Fatalf("row%d channel%d: %d want%d", row, i, ch, want.Pix[i])
			}
		}
		legacy := c
		legacy.PointLights = nil
		legacy.Light = Light{Kind: PointLight, Position: c.PointLights[0].Position, Ambient: c.Light.Ambient, Diffuse: 1, Attenuation: .06}
		single := mustShadowRender(t, legacy, camera, instances...)
		for col, img := range []*image.RGBA{single, got, want} {
			draw.Draw(sheet, image.Rect(col*240, row*180, (col+1)*240, (row+1)*180), img, image.Point{}, draw.Src)
		}
	}
	dir := t.TempDir()
	if requested := os.Getenv("GOMATH_THREE_ARTIFACTS"); requested != "" {
		dir = requested
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(dir, "multiple-point-lights.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = png.Encode(f, sheet); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	t.Log(path)
}

func BenchmarkRenderMultiLight(b *testing.B) {
	for _, count := range []int{0, 1, 3, 16} {
		for _, scale := range []int{1, 2} {
			b.Run(fmt.Sprintf("sources%d/ssaa%d", count, scale), func(b *testing.B) {
				c, camera, instances := shadowFixture(b)
				c.Width, c.Height, c.Supersampling = 320, 240, scale
				c.Shadow = ShadowConfig{}
				c.Light = Light{Kind: PointLightList, Ambient: .13}
				base := multiLightSources()
				for i := range count {
					p := base[i%len(base)]
					p.Position.Z += 2
					p.Position.X += float64(i) * .13
					p.Diffuse /= float64(max(1, count))
					c.PointLights = append(c.PointLights, p)
				}
				sphere, err := Sphere(.65, 64, 32)
				if err != nil {
					b.Fatal(err)
				}
				in := NewInstance(sphere)
				in.Model = geom.Translation(geom.Vec3{X: 1.6, Y: .5, Z: .65})
				instances = append(instances, in)
				instances[0].Material.Texture = primitiveTexture(b, "mixed")
				instances[0].Material.Sampler = Sampler{Filter: Bilinear, AddressU: Repeat}
				instances[1].Material.Color = color.NRGBA{R: 210, G: 97, B: 42, A: 151}
				cloud, err := NewPointCloud([]geom.Vec3{{X: .1, Y: -.3, Z: .8}, {X: .3, Y: .1, Z: .5}}, []color.NRGBA{{R: 211, B: 93, A: 137}, {G: 193, B: 57, A: 255}}, []float64{4.1, 3.7})
				if err != nil {
					b.Fatal(err)
				}
				r, err := NewRenderer(c)
				if err != nil {
					b.Fatal(err)
				}
				b.ReportAllocs()
				b.ResetTimer()
				for range b.N {
					if _, err := r.RenderWithPoints(camera, []*PointCloud{cloud}, instances...); err != nil {
						b.Fatal(err)
					}
				}
				b.StopTimer()
				vertices, triangles := 0, 0
				for _, in := range instances {
					vertices += len(in.Mesh.Vertices)
					triangles += len(in.Mesh.Triangles)
				}
				b.ReportMetric(float64(vertices), "vertices/frame")
				b.ReportMetric(float64(triangles), "tris/frame")
				b.ReportMetric(float64(count), "lights/frame")
			})
		}
	}
}
