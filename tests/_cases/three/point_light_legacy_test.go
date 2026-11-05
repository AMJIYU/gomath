package three

import (
	"crypto/sha256"
	"fmt"
	"gomath/geom"
	"image"
	"image/color"
	"testing"
)

// These raw-RGBA SHA256 values are captured BEFORE adding Light.Kind or the
// point-light shader. They pin the existing directional paths, including
// smooth normals, alpha/UV filtering, points and opt-in directional shadows.
func TestDirectionalLightLegacyHashes(t *testing.T) {
	for _, name := range []string{"opaque1", "opaque2", "opaque4", "alphaUV", "shadow", "points"} {
		t.Run(name, func(t *testing.T) {
			cfg := DefaultConfig(47, 35)
			cfg.Background = color.NRGBA{R: 19, G: 37, B: 53, A: 107}
			camera := primitiveCamera(Perspective)
			sphere, err := Sphere(.8, 12, 6)
			if err != nil {
				t.Fatal(err)
			}
			in := NewInstance(sphere)
			in.Model = geom.Scaling(-1, .85, 1.1)
			in.Material.Color = color.NRGBA{R: 213, G: 151, B: 79, A: 255}
			instances := []Instance{in}
			var clouds []*PointCloud
			switch name {
			case "opaque2":
				cfg.Supersampling = 2
			case "opaque4":
				cfg.Supersampling = 4
			case "alphaUV":
				cfg.Supersampling = 2
				camera = testCamera(Perspective)
				in = NewInstance(uvQuad(1.7, 1.3, 2.6))
				in.Material.Texture = primitiveTexture(t, "mixed")
				in.Material.Sampler = Sampler{Filter: Bilinear, AddressU: Repeat, AddressV: Clamp}
				in.Material.Color = color.NRGBA{R: 193, G: 157, B: 231, A: 177}
				instances = []Instance{in}
				front := NewInstance(triangleMesh())
				front.Material.Color = color.NRGBA{R: 29, G: 173, B: 209, A: 103}
				instances = append(instances, front)
			case "shadow":
				cfg, camera, instances = shadowFixture(t)
				cfg.Width, cfg.Height = 47, 35
				cfg.Supersampling = 2
			case "points":
				cfg.Supersampling = 2
				p, err := NewPointCloud([]geom.Vec3{{X: .3, Z: 1.2}, {X: -.7, Y: -.5}, {X: .3, Z: -1}}, []color.NRGBA{{R: 237, B: 17, A: 255}, {G: 219, B: 59, A: 119}, {R: 81, G: 153, A: 255}}, []float64{3.7, 5.3, 4.1})
				if err != nil {
					t.Fatal(err)
				}
				clouds = []*PointCloud{p}
			}
			r, err := NewRenderer(cfg)
			if err != nil {
				t.Fatal(err)
			}
			var img *image.RGBA
			if clouds != nil {
				img, err = r.RenderWithPoints(camera, clouds, instances...)
			} else {
				img, err = r.Render(camera, instances...)
			}
			if err != nil {
				t.Fatal(err)
			}
			got := fmt.Sprintf("%x", sha256.Sum256(img.Pix))
			want := map[string]string{
				"opaque1": "70a14229520bce0fe10916d6f68bea2f7eec141fddad961da6e649ebb529be92",
				"opaque2": "8c13e4f09ff39a2e5387167e303a557616a754e64a886b4dddc4a0333029620e",
				"opaque4": "ce055db57e80561a9c14aea68c79bb92f3353756e82258c9c6a72281010b3c8b",
				"alphaUV": "51498b78018cae9371d24d35f913c2066a8be56c7b03a8b7a1af0f6c86ce73cd",
				"shadow":  "575e239a4d2ad1ca71a4b64dcc386fa90fff2f8ff7bba1535875c4ec1cf2e183",
				"points":  "a9688887e7ff534b93271942dffea6c9262a67e656e286ee897a866e75b2fe58",
			}
			if got != want[name] {
				t.Fatalf("directional frame changed: %s want %s", got, want[name])
			}
		})
	}
}
