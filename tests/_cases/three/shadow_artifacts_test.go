package three

import (
	"fmt"
	"gomath/geom"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// Columns: disabled, hard shadows, independent light/camera-ray reference.
// Rows: two observer positions sharing the same world shadow volume/light.
func TestShadowArtifacts(t *testing.T) {
	cfg, camera, instances := shadowFixture(t)
	cfg.Width, cfg.Height = 320, 240
	cfg.Supersampling = 2
	cfg.Shadow.Resolution, cfg.Shadow.Bias = 512, .08
	instances[0].Material.Texture = primitiveTexture(t, "checker")
	instances[0].Material.Sampler = Sampler{Filter: Bilinear, AddressU: Repeat, AddressV: Repeat}
	sheet := image.NewRGBA(image.Rect(0, 0, 960, 480))
	for row := range 2 {
		if row == 1 {
			camera.Position = geom.Vec3{X: -4, Y: -5, Z: 4.6}
		}
		enabled := mustShadowRender(t, cfg, camera, instances...)
		reference := shadowRayReference(t, cfg, camera, instances...)
		plain := cfg
		plain.Shadow.Enabled = false
		disabled := mustShadowRender(t, plain, camera, instances...)
		for col, img := range []*image.RGBA{disabled, enabled, reference} {
			draw.Draw(sheet, image.Rect(col*320, row*240, (col+1)*320, (row+1)*240), img, image.Point{}, draw.Src)
		}
	}
	dir := t.TempDir()
	if requested := os.Getenv("GOMATH_THREE_ARTIFACTS"); requested != "" {
		dir = requested
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(dir, "directional-shadows.png")
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

func BenchmarkRenderDirectionalShadow(b *testing.B) {
	for _, mode := range []string{"off", "map256", "map1024", "textured256"} {
		for _, scale := range []int{1, 2} {
			b.Run(fmt.Sprintf("%s/ssaa%d", mode, scale), func(b *testing.B) {
				cfg, camera, instances := shadowFixture(b)
				cfg.Width, cfg.Height = 320, 240
				cfg.Supersampling = scale
				cfg.Shadow.Enabled = mode != "off"
				cfg.Shadow.Resolution, cfg.Shadow.Bias = 256, .04
				if mode == "map1024" {
					cfg.Shadow.Resolution, cfg.Shadow.Bias = 1024, .01
				}
				if mode == "textured256" {
					instances[0].Material.Texture = primitiveTexture(b, "checker")
					instances[0].Material.Sampler = Sampler{Filter: Bilinear, AddressU: Repeat, AddressV: Clamp}
				}
				sphere, err := Sphere(.65, 64, 32)
				if err != nil {
					b.Fatal(err)
				}
				in := NewInstance(sphere)
				in.Model = geom.Translation(geom.Vec3{X: 1.6, Y: .5, Z: .65})
				instances = append(instances, in)
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
