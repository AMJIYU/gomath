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

// Columns: point light without attenuation, with attenuation, independent ray
// reference with attenuation. Rows move only the world-space light source.
func TestPointLightArtifacts(t *testing.T) {
	cfg, camera, instances := shadowFixture(t)
	cfg.Width, cfg.Height = 320, 240
	cfg.Supersampling = 2
	cfg.Shadow = ShadowConfig{}
	instances[0].Material.Texture = primitiveTexture(t, "checker")
	instances[0].Material.Sampler = Sampler{Filter: Bilinear, AddressU: Repeat, AddressV: Clamp}
	instances[1].Material.Color.A = 183
	// Lift the translucent box above the plane: exact coplanar opaque/alpha
	// depth ties depend on float projection rounding, not the light equation.
	instances[1].Model = geom.Translation(geom.Vec3{Z: .08}).Mul(instances[1].Model)
	sheet := image.NewRGBA(image.Rect(0, 0, 960, 480))
	for row, position := range []geom.Vec3{{X: 1.2, Y: -2, Z: 3}, {X: -2.3, Y: 1.2, Z: 1.5}} {
		cfg.Light = Light{Kind: PointLight, Position: position, Ambient: .15, Diffuse: .85, Attenuation: .12}
		attenuated := mustShadowRender(t, cfg, camera, instances...)
		reference := pointLightRayReference(t, cfg, camera, instances...)
		for i, ch := range attenuated.Pix {
			if math.Abs(float64(ch)-float64(reference.Pix[i])) > 1 {
				t.Fatalf("row%d reference pixel%d,%d channel%d: %d want%d", row, i/4%cfg.Width, i/4/cfg.Width, i%4, ch, reference.Pix[i])
			}
		}
		cfg.Light.Attenuation = 0
		constant := mustShadowRender(t, cfg, camera, instances...)
		for column, img := range []*image.RGBA{constant, attenuated, reference} {
			draw.Draw(sheet, image.Rect(column*320, row*240, (column+1)*320, (row+1)*240), img, image.Point{}, draw.Src)
		}
	}
	dir := t.TempDir()
	if requested := os.Getenv("GOMATH_THREE_ARTIFACTS"); requested != "" {
		dir = requested
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(dir, "world-point-light.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, sheet); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	t.Log(path)
}

func BenchmarkRenderPointLight(b *testing.B) {
	for _, mode := range []string{"flat", "smooth", "mixedUV"} {
		for _, scale := range []int{1, 2} {
			b.Run(fmt.Sprintf("%s/ssaa%d", mode, scale), func(b *testing.B) {
				cfg, camera, instances := shadowFixture(b)
				cfg.Width, cfg.Height = 320, 240
				cfg.Supersampling = scale
				cfg.Shadow = ShadowConfig{}
				cfg.Light = Light{Kind: PointLight, Position: geom.Vec3{X: 1, Y: -2, Z: 4}, Ambient: .15, Diffuse: .85, Attenuation: .12}
				sphere, err := Sphere(.65, 64, 32)
				if err != nil {
					b.Fatal(err)
				}
				if mode == "flat" {
					sphere.Normals = nil
				}
				in := NewInstance(sphere)
				in.Model = geom.Translation(geom.Vec3{X: 1.6, Y: .5, Z: .65})
				instances = append(instances, in)
				if mode == "mixedUV" {
					instances[0].Material.Texture = primitiveTexture(b, "mixed")
					instances[0].Material.Sampler = Sampler{Filter: Bilinear, AddressU: Repeat, AddressV: Clamp}
					instances[1].Material.Color = color.NRGBA{R: 210, G: 97, B: 42, A: 151}
				}
				r, err := NewRenderer(cfg)
				if err != nil {
					b.Fatal(err)
				}
				b.ReportAllocs()
				b.ResetTimer()
				for range b.N {
					if _, err := r.Render(camera, instances...); err != nil {
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
			})
		}
	}
}
