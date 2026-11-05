package three_test

import (
	"fmt"
	"gomath/geom"
	"gomath/three"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestCheckerboardArtifacts(t *testing.T) {
	dir := os.Getenv("GOMATH_CHECKERBOARD_ARTIFACTS")
	if dir == "" {
		t.Skip("set GOMATH_CHECKERBOARD_ARTIFACTS to generate image")
	}
	if e := os.MkdirAll(dir, 0755); e != nil {
		t.Fatal(e)
	}
	sheet := image.NewRGBA(image.Rect(0, 0, 480, 540))
	for col, curved := range []bool{false, true} {
		point := checkerPoint
		if !curved {
			point = func(u, v float64) geom.Vec3 { return geom.Vec3{X: u, Y: v, Z: .13*u - .21*v} }
		}
		groups := checkerBuild(t, 7, 5, point)
		ref := checkerReferenceGrid(-1, 1, -.8, .9, 7, 5, point)
		cfg := three.DefaultConfig(240, 180)
		cfg.Supersampling = 2
		cfg.CullBackfaces = false
		cfg.Background = color.NRGBA{R: 17, G: 27, B: 43, A: 255}
		camera := checkerCamera(three.Perspective)
		camera.FOVY = .52
		styles := checkerStyles(t, "opaque")
		plain := checkerRender(t, cfg, camera, checkerInstances(groups, styles, checkerModel(0))...)
		styles = checkerStyles(t, "uv-lights")
		cfg = checkerLights(cfg)
		model := checkerModel(1)
		got := checkerRender(t, cfg, camera, checkerInstances(groups, styles, model)...)
		want := checkerRayReference(cfg, camera, checkerWorld(ref, model, styles))
		checkerPixels(t, got, want)
		for row, img := range []*image.RGBA{plain, got, want} {
			draw.Draw(sheet, image.Rect(col*240, row*180, (col+1)*240, (row+1)*180), img, image.Point{}, draw.Src)
		}
	}
	path := filepath.Join(dir, "checkerboard-surfaces.png")
	f, e := os.Create(path)
	if e != nil {
		t.Fatal(e)
	}
	encodeErr := png.Encode(f, sheet)
	closeErr := f.Close()
	if encodeErr != nil || closeErr != nil {
		t.Fatalf("encode%v close%v", encodeErr, closeErr)
	}
	t.Log(path)
}

var checkerBenchmarkGroups [2]*three.Mesh
var checkerBenchmarkImage *image.RGBA

func BenchmarkBuildCheckerboard(b *testing.B) {
	for _, n := range []int{32, 256} {
		b.Run(fmt.Sprintf("%dx%d", n, n), func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				var e error
				checkerBenchmarkGroups, e = three.CheckerboardSurface(-1, 1, -.8, .9, n, n, func(u, v float64) (geom.Vec3, error) { return checkerPoint(u, v), nil })
				if e != nil {
					b.Fatal(e)
				}
			}
			b.ReportMetric(float64((n+1)*(n+1)), "samples/op")
			b.ReportMetric(float64(2*n*n), "tris/op")
		})
	}
}

func BenchmarkRenderCheckerboard(b *testing.B) {
	groups := checkerBuild(b, 16, 12, checkerPoint)
	for _, style := range []string{"opaque", "uv-lights"} {
		for _, ssaa := range []int{1, 2} {
			b.Run(fmt.Sprintf("%s/s%d", style, ssaa), func(b *testing.B) {
				cfg := three.DefaultConfig(320, 240)
				cfg.Supersampling = ssaa
				cfg.CullBackfaces = style == "opaque"
				if style == "uv-lights" {
					cfg = checkerLights(cfg)
				}
				instances := checkerInstances(groups, checkerStyles(b, style), checkerModel(2))
				camera := checkerCamera(three.Perspective)
				r, e := three.NewRenderer(cfg)
				if e != nil {
					b.Fatal(e)
				}
				b.ReportAllocs()
				b.ResetTimer()
				for range b.N {
					checkerBenchmarkImage, e = r.Render(camera, instances...)
					if e != nil {
						b.Fatal(e)
					}
				}
				b.ReportMetric(float64(len(groups[0].Vertices)+len(groups[1].Vertices)), "vertices/frame")
				b.ReportMetric(float64(len(groups[0].Triangles)+len(groups[1].Triangles)), "tris/frame")
			})
		}
	}
}
