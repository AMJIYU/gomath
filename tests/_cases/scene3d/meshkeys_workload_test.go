package scene3d

import (
	"bytes"
	"context"
	"fmt"
	"gomath/export"
	"gomath/geom"
	"gomath/render"
	"gomath/three"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func meshTimelineWorkload(t testing.TB, ssaa int, geometry, material bool) (Config, []CameraKey, []MeshTrack) {
	t.Helper()
	c, cameras, tracks := workloadFixture(t, ssaa)
	if geometry {
		a := tracks[0].Mesh
		b := &three.Mesh{Vertices: make([]geom.Vec3, len(a.Vertices)), Normals: make([]geom.Vec3, len(a.Normals)), UVs: make([]three.UV, len(a.UVs)), Triangles: append([]three.Triangle(nil), a.Triangles...)}
		for i, p := range a.Vertices {
			// An invertible nonlinear shear, not a model transform. Apply its
			// analytic inverse-transpose Jacobian to authored endpoint normals.
			b.Vertices[i] = geom.Vec3{X: p.X, Y: p.Y, Z: p.Z + .45*math.Sin(2*p.X)*math.Cos(p.Y)}
			dx, dy := .9*math.Cos(2*p.X)*math.Cos(p.Y), -.45*math.Sin(2*p.X)*math.Sin(p.Y)
			n := a.Normals[i]
			n = geom.Vec3{X: n.X - dx*n.Z, Y: n.Y - dy*n.Z, Z: n.Z}
			b.Normals[i] = n.Mul(1 / math.Sqrt(n.Dot(n)))
			uv := a.UVs[i]
			b.UVs[i] = three.UV{U: uv.U + .27, V: uv.V - .13}
		}
		tracks[0].Mesh, tracks[0].MeshKeys = nil, []MeshKey{{Mesh: a}, {Time: 2, Mesh: b}}
	}
	if material {
		tracks[0].MaterialKeys = []MaterialKey{
			{Color: color.NRGBA{R: 255, G: 255, B: 255, A: 255}},
			{Time: 1, Color: color.NRGBA{R: 255, G: 131, B: 83, A: 183}},
			{Time: 2, Color: color.NRGBA{R: 101, G: 211, B: 255, A: 137}},
		}
	}
	return c, cameras, tracks
}

func TestMeshGeometryMaterialContactSheet(t *testing.T) {
	c, cameras, tracks := meshTimelineWorkload(t, 2, true, true)
	s := mustNew(t, c, cameras, tracks)
	times := []float64{0, .5, 1, 1.5, 2}
	sheet := image.NewRGBA(image.Rect(0, 0, c.Renderer.Width*len(times), c.Renderer.Height))
	for i, time := range times {
		frame, err := s.Sample(time)
		if err != nil {
			t.Fatal(err)
		}
		draw.Draw(sheet, image.Rect(i*c.Renderer.Width, 0, (i+1)*c.Renderer.Width, c.Renderer.Height), frame.Objects[0].Bitmap, image.Point{}, draw.Src)
	}
	dir := os.Getenv("GOMATH_SCENE3D_ARTIFACTS")
	if dir == "" {
		dir = t.TempDir()
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(filepath.Join(dir, "meshkeys-timeline-ssaa2.png"))
	if err != nil {
		t.Fatal(err)
	}
	err = png.Encode(file, sheet)
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		t.Fatal(err, closeErr)
	}
	t.Log("320x240 SSAA2 [0 .5 1 1.5 2], 2159 vertices/4140 triangles; torus nonlinear shear, changing normals/UV and material color/alpha, reflected nonuniform TRS, moving camera; sphere removed at 2")
}

func TestMeshGeometryMaterialMedia(t *testing.T) {
	c, cameras, tracks := meshTimelineWorkload(t, 2, true, true)
	c.Renderer.Width, c.Renderer.Height = 64, 48
	s := mustNew(t, c, cameras, tracks)
	rc := render.DefaultConfig()
	rc.Width, rc.Height, rc.FPS, rc.Workers = 64, 48, 2, 3
	dir := filepath.Join(t.TempDir(), "meshkeys")
	sink, err := export.NewPNGSequence(dir)
	if err != nil {
		t.Fatal(err)
	}
	stats, err := render.RenderScene(context.Background(), s, rc, sink)
	if err != nil || stats.Frames != 4 {
		t.Fatal(stats, err)
	}
	for i := range 4 {
		file, err := os.Open(filepath.Join(dir, fmt.Sprintf("%06d.png", i)))
		if err != nil {
			t.Fatal(err)
		}
		img, err := png.Decode(file)
		file.Close()
		if err != nil {
			t.Fatal(err)
		}
		frame, err := s.Sample(float64(i) / 2)
		if err != nil || !bytes.Equal(bitmapBytes(img), bitmapBytes(frame.Objects[0].Bitmap)) {
			t.Fatal("deforming geometry/material media frame changed CPU pixels", i, err)
		}
	}
}

func BenchmarkMeshTimeline(b *testing.B) {
	for _, ssaa := range []int{1, 2} {
		for _, mode := range []string{"GeometryOpaque", "ColorOnly", "GeometryAndColor"} {
			b.Run(fmt.Sprintf("%s/ssaa%d", mode, ssaa), func(b *testing.B) {
				c, cameras, tracks := meshTimelineWorkload(b, ssaa, mode != "ColorOnly", mode != "GeometryOpaque")
				s := mustNew(b, c, cameras, tracks)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					frame, err := s.Sample(.2 + float64(i%5)*.35)
					if err != nil {
						b.Fatal(err)
					}
					workloadFrame = frame
				}
			})
		}
		b.Run(fmt.Sprintf("DirectThree/ssaa%d", ssaa), func(b *testing.B) {
			c, cameras, tracks := meshTimelineWorkload(b, ssaa, true, true)
			s := mustNew(b, c, cameras, tracks)
			r, err := three.NewRenderer(c.Renderer)
			if err != nil {
				b.Fatal(err)
			}
			var views [5]three.Camera
			var batches [5][]three.Instance
			for i := range 5 {
				time := .2 + float64(i)*.35
				views[i], err = s.CameraAt(time)
				if err != nil {
					b.Fatal(err)
				}
				for _, track := range s.tracks {
					pose, _ := track.at(time)
					mesh, err := track.meshAt(time)
					if err != nil {
						b.Fatal(err)
					}
					batches[i] = append(batches[i], three.Instance{Mesh: mesh, Model: pose.matrix(track.Axis), Material: track.materialAt(time)})
				}
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				pixels, err := r.Render(views[i%5], batches[i%5]...)
				if err != nil {
					b.Fatal(err)
				}
				workloadPixels = pixels
			}
		})
	}
	b.Run("Snapshot", func(b *testing.B) {
		c, cameras, tracks := meshTimelineWorkload(b, 1, true, true)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			s, err := New(c, cameras, tracks)
			if err != nil {
				b.Fatal(err)
			}
			workloadScene = s
		}
	})
}
