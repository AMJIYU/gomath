package three

import (
	"fmt"
	"gomath/geom"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// Opt-in independent reference contact sheet; ordinary suites keep the smaller
// dense ray matrix in TestSegmentIndependentRays and do not rewrite artifacts.
func TestSegmentArtifacts(t *testing.T) {
	dir := os.Getenv("GOMATH_SEGMENT_ARTIFACTS")
	if dir == "" {
		t.Skip("set GOMATH_SEGMENT_ARTIFACTS to generate contact sheet")
	}
	if e := os.MkdirAll(dir, 0755); e != nil {
		t.Fatal(e)
	}
	meshes, refs := segmentRenderMeshes(t, false)
	sheet := image.NewRGBA(image.Rect(0, 0, 480, 540))
	for col, m := range meshes {
		c := DefaultConfig(240, 180)
		c.Supersampling = 2
		c.CullBackfaces = true
		c.Background = color.NRGBA{R: 17, G: 27, B: 43, A: 255}
		camera := primitiveCamera(Perspective)
		camera.FOVY = .58
		in := NewInstance(m)
		in.Material.Color = color.NRGBA{R: 239, G: 173, B: 67, A: 255}
		plain := mustShadowRender(t, c, camera, in)
		pose := polyhedronPose{scale: geom.Vec3{X: -.87, Y: 1.17, Z: .91}, axis: geom.Vec3{X: .3, Y: 1, Z: .7}, angle: .63}
		in.Model = polyhedronModel(pose)
		in.Material.Texture = primitiveTexture(t, "mixed")
		in.Material.Sampler.Filter = Bilinear
		in.Material.Color = color.NRGBA{R: 255, G: 255, B: 255, A: 191}
		c = multiLightConfig(c)
		c.CullBackfaces = false
		got := mustShadowRender(t, c, camera, in)
		ref := NewInstance(segmentReferencePose(refs[col], pose))
		ref.Material = in.Material
		want := pointLightRayReferenceWithLighting(t, c, camera, segmentLighting(c), ref)
		segmentPixelCheck(t, got, want)
		for row, img := range []*image.RGBA{plain, got, want} {
			draw.Draw(sheet, image.Rect(col*240, row*180, (col+1)*240, (row+1)*180), img, image.Point{}, draw.Src)
		}
	}
	path := filepath.Join(dir, "segments.png")
	f, e := os.Create(path)
	if e != nil {
		t.Fatal(e)
	}
	encodeErr := png.Encode(f, sheet)
	closeErr := f.Close()
	if encodeErr != nil || closeErr != nil {
		t.Fatalf("encode:%v close:%v", encodeErr, closeErr)
	}
	t.Log(path)
}

var segmentBenchmarkMesh *Mesh
var segmentBenchmarkFrame *image.RGBA

func BenchmarkBuildSegments(b *testing.B) {
	for _, radial := range []int{24, 256} {
		axial := 4
		if radial == 256 {
			axial = 256
		}
		for _, arrow := range []bool{false, true} {
			b.Run(fmt.Sprintf("arrow%t/r%d/a%d", arrow, radial, axial), func(b *testing.B) {
				start, end := geom.Vec3{X: -.7, Y: -.37, Z: -1.1}, geom.Vec3{X: .8, Y: .23, Z: 1.2}
				o := ArrowOptions{ShaftRadius: .22, HeadRadius: .52, HeadLength: .8, RadialSegments: radial, ShaftSegments: axial, HeadSegments: axial}
				b.ReportAllocs()
				b.ResetTimer()
				for range b.N {
					var e error
					if arrow {
						segmentBenchmarkMesh, e = ArrowBetween(start, end, o)
					} else {
						segmentBenchmarkMesh, e = CylinderBetween(start, end, .32, radial, axial)
					}
					if e != nil {
						b.Fatal(e)
					}
				}
			})
		}
	}
}

func BenchmarkRenderSegments(b *testing.B) {
	meshes, _ := segmentRenderMeshes(b, true)
	for _, ssaa := range []int{1, 2} {
		for _, style := range []string{"opaque", "uv-alpha-lights"} {
			b.Run(fmt.Sprintf("%s/s%d", style, ssaa), func(b *testing.B) {
				c := DefaultConfig(320, 240)
				c.Supersampling = ssaa
				c.CullBackfaces = style == "opaque"
				if style != "opaque" {
					c = multiLightConfig(c)
				}
				var instances []Instance
				vertices, triangles := 0, 0
				for i, m := range meshes {
					in := NewInstance(m)
					in.Model = geom.Translation(geom.Vec3{X: float64(i)*1.4 - .7}).Mul(geom.Rotation(geom.Vec3{X: .3, Y: 1, Z: .7}, .63))
					if style != "opaque" {
						in.Material.Texture = primitiveTexture(b, "mixed")
						in.Material.Color.A = 191
						in.Material.Sampler.Filter = Bilinear
					}
					instances = append(instances, in)
					vertices += len(m.Vertices)
					triangles += len(m.Triangles)
				}
				r, e := NewRenderer(c)
				if e != nil {
					b.Fatal(e)
				}
				camera := primitiveCamera(Perspective)
				b.ReportAllocs()
				b.ResetTimer()
				for range b.N {
					segmentBenchmarkFrame, e = r.Render(camera, instances...)
					if e != nil {
						b.Fatal(e)
					}
				}
				b.ReportMetric(float64(vertices), "vertices/frame")
				b.ReportMetric(float64(triangles), "tris/frame")
			})
		}
	}
}
