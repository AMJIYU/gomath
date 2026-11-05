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

func pointWorkloadFixture(t testing.TB, scale int, deform bool) (Config, []CameraKey, []MeshTrack, []PointTrack) {
	t.Helper()
	c, cameras, meshes := workloadFixture(t, scale)
	const count = 2400
	a, b := make([]geom.Vec3, count), make([]geom.Vec3, count)
	ca, cb := make([]color.NRGBA, count), make([]color.NRGBA, count)
	ra, rb := make([]float64, count), make([]float64, count)
	for i := range count {
		y := 1 - 2*(float64(i)+.5)/count
		r := math.Sqrt(1 - y*y)
		angle := float64(i) * math.Pi * (3 - math.Sqrt(5))
		a[i] = geom.Vec3{X: 1.6 * r * math.Cos(angle), Y: .9 * y, Z: 1.6 * r * math.Sin(angle)}
		b[i] = geom.Vec3{X: 1.3 * r * math.Cos(angle), Y: 1.2 * y, Z: 1.7*r*math.Sin(angle) + .15*math.Sin(3*angle)}
		ca[i] = color.NRGBA{R: uint8(91 + 127*(y+1)/2), G: 161, B: 243, A: uint8(113 + i%5*23)}
		cb[i] = color.NRGBA{R: 229, G: uint8(83 + 151*(y+1)/2), B: 127, A: uint8(97 + i%7*19)}
		ra[i], rb[i] = 1.2, 1.8
	}
	cloud := mustCloud(t, a, ca, ra)
	first, last := IdentityPose(), IdentityPose()
	first.Position, last.Position = geom.Vec3{Y: .1}, geom.Vec3{X: .15, Y: .2}
	last.Scale, last.Angle = geom.Vec3{X: 1.05, Y: .9, Z: 1.1}, 1.7
	track := PointTrack{ID: 20, Cloud: cloud, Axis: geom.Vec3{Y: 1}, Keys: []PoseKey{{Pose: first}, {Time: 2, Pose: last}}, Visible: []Visibility{{Add: .1}}}
	if deform {
		track.Cloud = nil
		track.PointKeys = []PointKey{{Cloud: cloud}, {Time: 2, Cloud: mustCloud(t, b, cb, rb)}}
	}
	return c, cameras, meshes, []PointTrack{track}
}

func TestPointTimelineContactSheet(t *testing.T) {
	c, cameras, meshes, points := pointWorkloadFixture(t, 2, true)
	s := mustNewPoints(t, c, cameras, meshes, points)
	times := []float64{0, .5, 1, 1.5, 2}
	sheet := image.NewRGBA(image.Rect(0, 0, c.Renderer.Width*len(times), c.Renderer.Height))
	for i, time := range times {
		frame, err := s.Sample(time)
		if err != nil {
			t.Fatal(err)
		}
		dst := image.Rect(i*c.Renderer.Width, 0, (i+1)*c.Renderer.Width, c.Renderer.Height)
		draw.Draw(sheet, dst, frame.Objects[0].Bitmap, image.Point{}, draw.Src)
	}
	dir := os.Getenv("GOMATH_SCENE3D_ARTIFACTS")
	if dir == "" {
		dir = t.TempDir()
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(filepath.Join(dir, "points-timeline-ssaa2.png"))
	if err != nil {
		t.Fatal(err)
	}
	err = png.Encode(file, sheet)
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		t.Fatal(err, closeErr)
	}
	t.Log("320x240 SSAA2 times[0 .5 1 1.5 2], 2400 indexed points, animated positions/colors/radii plus TRS, 4140 mesh triangles; points added .1, sphere removed 2")
}

func TestPointTimelineMediaFrames(t *testing.T) {
	c, cameras, meshes, points := pointWorkloadFixture(t, 2, true)
	c.Renderer.Width, c.Renderer.Height = 64, 48
	s := mustNewPoints(t, c, cameras, meshes, points)
	rc := render.DefaultConfig()
	rc.Width, rc.Height, rc.FPS, rc.Workers = 64, 48, 2, 3
	dir := filepath.Join(t.TempDir(), "points")
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
			t.Fatal("point/mesh media frame lost actual 3D pixels", i, err)
		}
	}
}

func BenchmarkPointTimeline(b *testing.B) {
	for _, scale := range []int{1, 2} {
		for _, deform := range []bool{false, true} {
			name := "TRS"
			if deform {
				name = "BuffersAndTRS"
			}
			b.Run(fmt.Sprintf("%s/ssaa%d", name, scale), func(b *testing.B) {
				c, cameras, meshes, points := pointWorkloadFixture(b, scale, deform)
				s := mustNewPoints(b, c, cameras, meshes, points)
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					// Five non-key times; the point track is always visible.
					frame, err := s.Sample(.2 + float64(i%5)*.35)
					if err != nil {
						b.Fatal(err)
					}
					workloadFrame = frame
				}
			})
		}
		b.Run(fmt.Sprintf("DirectThree/ssaa%d", scale), func(b *testing.B) {
			c, cameras, meshes, points := pointWorkloadFixture(b, scale, true)
			s := mustNewPoints(b, c, cameras, meshes, points)
			r, err := three.NewRenderer(c.Renderer)
			if err != nil {
				b.Fatal(err)
			}
			var views [5]three.Camera
			var clouds [5][]*three.PointCloud
			var batches [5][]three.Instance
			for i := range 5 {
				time := .2 + float64(i)*.35
				views[i], err = s.CameraAt(time)
				if err != nil {
					b.Fatal(err)
				}
				for _, track := range s.tracks {
					pose, visible := track.at(time)
					if visible {
						batches[i] = append(batches[i], three.Instance{Mesh: track.Mesh, Model: pose.matrix(track.Axis), Material: track.Material})
					}
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
				pixels, err := r.RenderWithPoints(views[i%5], clouds[i%5], batches[i%5]...)
				if err != nil {
					b.Fatal(err)
				}
				workloadPixels = pixels
			}
		})
	}
	b.Run("SnapshotBuffers", func(b *testing.B) {
		c, cameras, meshes, points := pointWorkloadFixture(b, 1, true)
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
