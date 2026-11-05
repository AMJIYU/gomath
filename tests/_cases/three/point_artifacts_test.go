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

func pointWorkloadData(count int, mode string) ([]geom.Vec3, []color.NRGBA, []float64) {
	positions, colors, radii := make([]geom.Vec3, count), make([]color.NRGBA, count), make([]float64, count)
	for i := range count {
		theta := float64(i) * 2.399963229728653
		radius := 2 * math.Sqrt((float64(i)+.5)/float64(count))
		s, c := math.Sincos(theta)
		positions[i] = geom.Vec3{X: radius * c, Y: radius * s, Z: .55*math.Sin(4*theta) + .25*math.Cos(3*radius)}
		alpha := uint8(255)
		if mode != "opaque" {
			alpha = []uint8{0, 96, 255}[i%3]
		}
		colors[i] = color.NRGBA{R: uint8(math.Round(125 + 55*positions[i].X)), G: uint8(math.Round(125 + 55*positions[i].Y)), B: uint8(70 + i%150), A: alpha}
		radii[i] = .8 + float64(i%5)*.35
	}
	return positions, colors, radii
}

func pointWorkloadMesh(tb testing.TB, textured bool) Instance {
	tb.Helper()
	m, err := Cylinder(.75, 1.4, 48, 6)
	if err != nil {
		tb.Fatal(err)
	}
	in := NewInstance(m)
	in.Material.Color = color.NRGBA{R: 210, G: 200, B: 180, A: 255}
	if textured {
		in.Material.Color = DefaultMaterial().Color
		in.Material.Texture = primitiveTexture(tb, "mixed")
		in.Material.Sampler = Sampler{Filter: Bilinear, AddressU: Repeat, AddressV: Clamp}
	}
	return in
}

func pointWorkloadCamera() Camera {
	return Camera{Position: geom.Vec3{X: 3.6, Y: -4.5, Z: 3.2}, Up: geom.Vec3{Z: 1}, Projection: Perspective, FOVY: math.Pi / 4, Near: .1, Far: 20}
}

func TestPointCloudArtifact(t *testing.T) {
	output := os.Getenv("GOMATH_THREE_ARTIFACTS")
	if output == "" {
		output = t.TempDir()
	}
	if err := os.MkdirAll(output, 0755); err != nil {
		t.Fatal(err)
	}
	p, c, radii := pointWorkloadData(2048, "mixed")
	cloud := mustPointCloud(t, p, c, radii)
	cfg := DefaultConfig(320, 240)
	cfg.Background = color.NRGBA{R: 20, G: 25, B: 35, A: 255}
	contact := image.NewRGBA(image.Rect(0, 0, 960, 480))
	for row, scale := range []int{1, 4} {
		cfg.Supersampling = scale
		renderer, err := NewRenderer(cfg)
		if err != nil {
			t.Fatal(err)
		}
		for col := range 3 {
			var meshes []Instance
			if col > 0 {
				meshes = []Instance{pointWorkloadMesh(t, col == 2)}
			}
			img, err := renderer.RenderWithPoints(pointWorkloadCamera(), []*PointCloud{cloud}, meshes...)
			if err != nil {
				t.Fatal(err)
			}
			draw.Draw(contact, image.Rect(col*320, row*240, (col+1)*320, (row+1)*240), img, image.Point{}, draw.Src)
		}
	}
	path := filepath.Join(output, "points.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	encodeErr, closeErr := png.Encode(f, contact), f.Close()
	if encodeErr != nil || closeErr != nil {
		t.Fatalf("encode %v close %v", encodeErr, closeErr)
	}
	t.Logf("artifact %s; columns points / opaque cylinder / textured mixed-alpha cylinder; rows 1x / 4x SSAA", path)
}

var benchmarkCloud *PointCloud

func BenchmarkNewPointCloud(b *testing.B) {
	p, c, radii := pointWorkloadData(32768, "mixed")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cloud, err := NewPointCloud(p, c, radii)
		if err != nil {
			b.Fatal(err)
		}
		benchmarkCloud = cloud
	}
}

func BenchmarkRenderPointCloud(b *testing.B) {
	for _, mode := range []string{"opaque", "mixed", "textured"} {
		for _, scale := range []int{1, 2} {
			b.Run(fmt.Sprintf("%s/s%d", mode, scale), func(b *testing.B) {
				p, c, radii := pointWorkloadData(4096, mode)
				cloud := mustPointCloud(b, p, c, radii)
				meshes := []Instance{pointWorkloadMesh(b, mode == "textured")}
				cfg := DefaultConfig(320, 240)
				cfg.Supersampling = scale
				r, err := NewRenderer(cfg)
				if err != nil {
					b.Fatal(err)
				}
				camera := pointWorkloadCamera()
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					if _, err := r.RenderWithPoints(camera, []*PointCloud{cloud}, meshes...); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
