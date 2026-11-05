package scene3d

import (
	"fmt"
	"gomath/geom"
	"gomath/three"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func rotationWorkload(t testing.TB, ssaa int) (Config, []CameraKey, []MeshTrack, []PointTrack) {
	t.Helper()
	_, keys := loadOrientation(t)
	c, cameras, meshes := meshTimelineWorkload(t, ssaa, true, true)
	_, _, _, points := pointWorkloadFixture(t, ssaa, true)
	meshes[0].Axis = geom.Vec3{}
	for i := range meshes[0].Keys {
		meshes[0].Keys[i].Pose.Angle = 0
	}
	meshes[0].RotationKeys = keys
	points[0].Axis = geom.Vec3{}
	for i := range points[0].Keys {
		points[0].Keys[i].Pose.Angle = 0
	}
	points[0].RotationKeys = append([]RotationKey(nil), keys...)
	for i := range points[0].RotationKeys {
		points[0].RotationKeys[i].Rotation = keys[len(keys)-1-i].Rotation
	}
	return c, cameras, meshes, points
}

func TestRotationContactSheet(t *testing.T) {
	c, cameras, meshes, points := rotationWorkload(t, 2)
	s := mustNewPoints(t, c, cameras, meshes, points)
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
	file, err := os.Create(filepath.Join(dir, "orientation-timeline-ssaa2.png"))
	if err != nil {
		t.Fatal(err)
	}
	err = png.Encode(file, sheet)
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		t.Fatal(err, closeErr)
	}
	t.Log("320x240 SSAA2 [0 .5 1 1.5 2], 2400 points and 4140 mesh triangles, independent shortest-arc mesh/point orientations, deforming torus normals/UV/material and point buffers, signed scales/camera motion")
}

func BenchmarkRotationTimeline(b *testing.B) {
	for _, ssaa := range []int{1, 2} {
		b.Run(fmt.Sprintf("Sample/ssaa%d", ssaa), func(b *testing.B) {
			c, cameras, meshes, points := rotationWorkload(b, ssaa)
			s := mustNewPoints(b, c, cameras, meshes, points)
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
			c, cameras, meshes, points := rotationWorkload(b, ssaa)
			s := mustNewPoints(b, c, cameras, meshes, points)
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
	b.Run("RotationAt", func(b *testing.B) {
		c, cameras, meshes, points := rotationWorkload(b, 1)
		s := mustNewPoints(b, c, cameras, meshes, points)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			q, err := s.RotationAt(1, .2+float64(i%5)*.35)
			if err != nil {
				b.Fatal(err)
			}
			orientationSink = q
		}
	})
	b.Run("Snapshot", func(b *testing.B) {
		c, cameras, meshes, points := rotationWorkload(b, 1)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			s, err := NewWithPoints(c, cameras, meshes, points)
			if err != nil {
				b.Fatal(err)
			}
			workloadScene = s
		}
	})
}

var orientationSink geom.Quat
