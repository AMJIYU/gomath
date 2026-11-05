package scene3d

import (
	"fmt"
	"gomath/three"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func orientedCameraWorkload(t testing.TB, ssaa int) (Config, OrientedCameraTrack, []MeshTrack, []PointTrack) {
	t.Helper()
	c, _, meshes, points := rotationWorkload(t, ssaa)
	_, camera := loadCameraOrientation(t)
	camera.Far = 30
	return c, camera, meshes, points
}

func TestOrientedCameraContactSheet(t *testing.T) {
	c, camera, meshes, points := orientedCameraWorkload(t, 2)
	s := mustOriented(t, c, camera, meshes, points)
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
	f, err := os.Create(filepath.Join(dir, "oriented-camera-ssaa2.png"))
	if err != nil {
		t.Fatal(err)
	}
	err = png.Encode(f, sheet)
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		t.Fatal(err, closeErr)
	}
	t.Log("320x240 SSAA2 [0 .5 1 1.5 2], quaternion eye camera with roll/yaw/pitch, 2400 points/4140 triangles, animated geometry/normals/UV/color/alpha/object orientations; sphere removed at 2")
}

func BenchmarkOrientedCameraTimeline(b *testing.B) {
	for _, ssaa := range []int{1, 2} {
		b.Run(fmt.Sprintf("Sample/ssaa%d", ssaa), func(b *testing.B) {
			c, camera, meshes, points := orientedCameraWorkload(b, ssaa)
			s := mustOriented(b, c, camera, meshes, points)
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
		b.Run(fmt.Sprintf("DirectThree/ssaa%d", ssaa), func(b *testing.B) {
			c, camera, meshes, points := orientedCameraWorkload(b, ssaa)
			s := mustOriented(b, c, camera, meshes, points)
			r, err := three.NewRenderer(c.Renderer)
			if err != nil {
				b.Fatal(err)
			}
			var views [5]three.Camera
			var instances [5][]three.Instance
			var clouds [5][]*three.PointCloud
			for i := range 5 {
				time := .2 + float64(i)*.35
				views[i], err = s.CameraAt(time)
				if err != nil {
					b.Fatal(err)
				}
				for _, track := range s.tracks {
					pose, _ := track.at(time)
					model, err := trackModel(pose, track.Axis, track.RotationKeys, time)
					if err != nil {
						b.Fatal(err)
					}
					mesh, err := track.meshAt(time)
					if err != nil {
						b.Fatal(err)
					}
					instances[i] = append(instances[i], three.Instance{Mesh: mesh, Model: model, Material: track.materialAt(time)})
				}
				for _, track := range s.points {
					cloud, err := track.cloudAt(time, poseAt(track.keys, time))
					if err != nil {
						b.Fatal(err)
					}
					clouds[i] = append(clouds[i], cloud)
				}
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				pixels, err := r.RenderWithPoints(views[i%5], clouds[i%5], instances[i%5]...)
				if err != nil {
					b.Fatal(err)
				}
				workloadPixels = pixels
			}
		})
	}
	b.Run("CameraAt", func(b *testing.B) {
		c, camera, meshes, points := orientedCameraWorkload(b, 1)
		s := mustOriented(b, c, camera, meshes, points)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			view, err := s.CameraAt(.2 + float64(i%5)*.35)
			if err != nil {
				b.Fatal(err)
			}
			orientedViewSink = view
		}
	})
	b.Run("Snapshot", func(b *testing.B) {
		c, camera, meshes, points := orientedCameraWorkload(b, 1)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			s, err := NewWithOrientedCamera(c, camera, meshes, points)
			if err != nil {
				b.Fatal(err)
			}
			workloadScene = s
		}
	})
}

var orientedViewSink three.Camera
