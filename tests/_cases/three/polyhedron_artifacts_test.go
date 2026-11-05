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

func TestPolyhedronArtifacts(t *testing.T) {
	controls := loadPolyhedronControls(t)
	sheet := image.NewRGBA(image.Rect(0, 0, 960, 540))
	for column, control := range controls {
		m, err := polyhedronConstructor(t, control.Name)(1.1)
		if err != nil {
			t.Fatal(err)
		}
		c := DefaultConfig(240, 180)
		c.Supersampling = 2
		c.Background = color.NRGBA{R: 17, G: 27, B: 43, A: 255}
		camera := primitiveCamera(Perspective)
		// Frame each different circumradius similarly, without rescaling the
		// source geometry or changing the common 1.1 world-unit edge length.
		radius := m.Vertices[0].Norm()
		camera.Position = camera.Position.Mul(2.7 * radius / camera.Position.Norm())
		pose := polyhedronPose{scale: geom.Vec3{X: 1, Y: 1, Z: 1}, axis: geom.Vec3{X: .3, Y: 1, Z: .7}}
		in := NewInstance(m)
		in.Material.Color = color.NRGBA{R: 239, G: 173, B: 67, A: 255}
		flat := mustShadowRender(t, c, camera, in)
		pose.angle = .63
		pose.scale = geom.Vec3{X: -.93, Y: 1.17, Z: .81}
		in.Model = polyhedronModel(pose)
		in.Material.Color = color.NRGBA{R: 255, G: 255, B: 255, A: 191}
		in.Material.Texture = primitiveTexture(t, "mixed")
		in.Material.Sampler.Filter = Bilinear
		got := mustShadowRender(t, c, camera, in)
		want := polyhedronPlaneReference(t, c, camera, control, 1.1, pose, in.Material)
		for i, ch := range got.Pix {
			if math.Abs(float64(ch)-float64(want.Pix[i])) > 1 {
				t.Fatal("artifact plane reference", control.Name, i, ch, want.Pix[i])
			}
		}
		for row, img := range []*image.RGBA{flat, got, want} {
			draw.Draw(sheet, image.Rect(column*240, row*180, (column+1)*240, (row+1)*180), img, image.Point{}, draw.Src)
		}
	}
	dir := t.TempDir()
	if requested := os.Getenv("GOMATH_THREE_ARTIFACTS"); requested != "" {
		dir = requested
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(dir, "platonic-solids.png")
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

var polyhedronBenchmarkMesh *Mesh
var polyhedronBenchmarkImage *image.RGBA

func BenchmarkBuildPlatonic(b *testing.B) {
	for _, name := range []string{"Tetrahedron", "Octahedron", "Icosahedron", "Dodecahedron"} {
		b.Run(name, func(b *testing.B) {
			build := polyhedronConstructor(b, name)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				var err error
				polyhedronBenchmarkMesh, err = build(1)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkBuildPolyhedron(b *testing.B) {
	for _, sides := range []int{32, 256, MaxPolyhedronVertices - 2} {
		b.Run(fmt.Sprintf("bipyramid%d", sides), func(b *testing.B) {
			v, f := polyhedronBipyramid(sides)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				var err error
				polyhedronBenchmarkMesh, err = Polyhedron(v, f)
				if err != nil {
					b.Fatal(err)
				}
			}
			b.StopTimer()
			b.ReportMetric(float64(len(v)), "logical-vertices/op")
			b.ReportMetric(float64(len(f)), "faces/op")
		})
	}
}

func BenchmarkRenderPolyhedra(b *testing.B) {
	for _, mode := range []string{"opaque", "textured-alpha-lights"} {
		for _, ssaa := range []int{1, 2} {
			b.Run(fmt.Sprintf("%s/ssaa%d", mode, ssaa), func(b *testing.B) {
				c := DefaultConfig(320, 240)
				c.Supersampling = ssaa
				c.CullBackfaces = mode == "opaque"
				if mode != "opaque" {
					c = multiLightConfig(c)
				}
				r, err := NewRenderer(c)
				if err != nil {
					b.Fatal(err)
				}
				var instances []Instance
				for i, name := range []string{"Tetrahedron", "Octahedron", "Icosahedron", "Dodecahedron"} {
					m, err := polyhedronConstructor(b, name)(1.1)
					if err != nil {
						b.Fatal(err)
					}
					in := NewInstance(m)
					in.Model = geom.Translation(geom.Vec3{X: float64(i%2)*2.3 - 1.15, Y: float64(i/2)*2.3 - 1.15}).Mul(geom.Rotation(geom.Vec3{X: .3, Y: 1, Z: .7}, .63))
					in.Material.Color = color.NRGBA{R: 239, G: 173, B: 67, A: 255}
					if mode != "opaque" {
						in.Material.Color.A = 191
						in.Material.Texture = primitiveTexture(b, "mixed")
						in.Material.Sampler.Filter = Bilinear
					}
					instances = append(instances, in)
				}
				camera := primitiveCamera(Perspective)
				camera.Position = camera.Position.Mul(1.4)
				b.ReportAllocs()
				b.ResetTimer()
				for range b.N {
					polyhedronBenchmarkImage, err = r.Render(camera, instances...)
					if err != nil {
						b.Fatal(err)
					}
				}
				b.StopTimer()
				b.ReportMetric(156, "vertices/frame")
				b.ReportMetric(68, "tris/frame")
			})
		}
	}
}
