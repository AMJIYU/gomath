package scene3d

import (
	"fmt"
	"gomath"
	"gomath/geom"
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

// A real three-mesh scene; texture authoring and primitive tessellation are
// outside timed sampling. The sphere disappears in the last contact-sheet cell.
func workloadFixture(t testing.TB, scale int) (Config, []CameraKey, []MeshTrack) {
	t.Helper()
	c := DefaultConfig(320, 240, 2)
	c.Renderer.Supersampling = scale
	c.Renderer.Background = color.NRGBA{R: 14, G: 23, B: 39, A: 255}
	c.Renderer.Light = three.Light{Direction: geom.Vec3{X: -1, Y: 3, Z: 4}, Ambient: .3, Diffuse: .7}
	camera := three.Camera{Position: geom.Vec3{X: 4, Y: 2.8, Z: 6}, Target: geom.Vec3{}, Up: geom.Vec3{Y: 1}, Projection: three.Perspective, Near: .1, Far: 30, FOVY: .75}
	last := camera
	last.Position, last.Target, last.FOVY = geom.Vec3{X: -3.5, Y: 2.3, Z: 6}, geom.Vec3{X: .15, Y: .15}, .67
	mesh, err := three.Torus(.8, .27, 48, 20)
	if err != nil {
		t.Fatal(err)
	}
	sphere, err := three.Sphere(.52, 48, 24)
	if err != nil {
		t.Fatal(err)
	}
	floor, err := three.Box(5, .12, 3.5)
	if err != nil {
		t.Fatal(err)
	}
	checker := image.NewNRGBA(image.Rect(0, 0, 16, 8))
	for y := range 8 {
		for x := range 16 {
			c := color.NRGBA{R: 48, G: 207, B: 227, A: 255}
			if (x/2+y/2)%2 == 0 {
				c = color.NRGBA{R: 231, G: 243, B: 255, A: 255}
			}
			checker.SetNRGBA(x, y, c)
		}
	}
	texture, err := three.NewTexture(checker)
	if err != nil {
		t.Fatal(err)
	}
	start, end, ball, ballEnd, base := IdentityPose(), IdentityPose(), IdentityPose(), IdentityPose(), IdentityPose()
	start.Position, end.Position = geom.Vec3{X: -.8}, geom.Vec3{X: .1, Y: .3}
	start.Scale, end.Scale = geom.Vec3{X: -1, Y: 1, Z: 1}, geom.Vec3{X: -1.2, Y: .8, Z: 1.1}
	end.Angle = 2 * math.Pi
	ball.Position, ballEnd.Position = geom.Vec3{X: 1.2, Y: -.2}, geom.Vec3{X: 1.1, Y: .5, Z: -.3}
	base.Position = geom.Vec3{Y: -1.25}
	tracks := []MeshTrack{
		{ID: 1, Mesh: mesh, Material: three.Material{Color: color.NRGBA{R: 255, G: 255, B: 255, A: 255}, Texture: texture, Sampler: three.Sampler{Filter: three.Bilinear, AddressU: three.Repeat, AddressV: three.Repeat}}, Axis: geom.Vec3{X: 1, Y: 2, Z: .5}, Keys: []PoseKey{{Pose: start}, {Time: 2, Pose: end}}},
		{ID: 2, Mesh: sphere, Material: three.Material{Color: color.NRGBA{R: 252, G: 158, B: 65, A: 255}}, Keys: []PoseKey{{Pose: ball}, {Time: 2, Pose: ballEnd}}, Visible: []Visibility{{Remove: 2}}},
		{ID: 3, Mesh: floor, Material: three.Material{Color: color.NRGBA{R: 82, G: 102, B: 126, A: 255}}, Keys: []PoseKey{{Pose: base}}},
	}
	return c, []CameraKey{{Camera: camera}, {Time: 2, Camera: last}}, tracks
}

func TestTimelineContactSheet(t *testing.T) {
	c, cameras, tracks := workloadFixture(t, 2)
	s := mustNew(t, c, cameras, tracks)
	times := []float64{0, .5, 1, 1.5, 2}
	sheet := image.NewRGBA(image.Rect(0, 0, c.Renderer.Width*len(times), c.Renderer.Height))
	for i, time := range times {
		frame, err := s.Sample(time)
		if err != nil {
			t.Fatal(err)
		}
		if frame.Objects[0].ID != s.BitmapID() {
			t.Fatal("bitmap identity changed")
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
	file, err := os.Create(filepath.Join(dir, "timeline-ssaa2.png"))
	if err != nil {
		t.Fatal(err)
	}
	err = png.Encode(file, sheet)
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		t.Fatal(err, closeErr)
	}
	vertices, triangles := 0, 0
	for _, track := range tracks {
		vertices += len(track.Mesh.Vertices)
		triangles += len(track.Mesh.Triangles)
	}
	t.Logf("320x240 SSAA2 times%v, %d vertices/%d triangles before final sphere removal", times, vertices, triangles)
}

var workloadFrame *gomath.Frame
var workloadScene *Scene
var workloadPixels *image.RGBA

func BenchmarkTimeline(b *testing.B) {
	for _, scale := range []int{1, 2} {
		b.Run(fmt.Sprintf("Sample/ssaa%d", scale), func(b *testing.B) {
			c, cameras, tracks := workloadFixture(b, scale)
			s := mustNew(b, c, cameras, tracks)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				frame, err := s.Sample(float64(i%5) * .375)
				if err != nil {
					b.Fatal(err)
				}
				workloadFrame = frame
			}
		})
		b.Run(fmt.Sprintf("DirectThree/ssaa%d", scale), func(b *testing.B) {
			c, cameras, tracks := workloadFixture(b, scale)
			r, err := three.NewRenderer(c.Renderer)
			if err != nil {
				b.Fatal(err)
			}
			// Precompute the same five poses/cameras for the control. This
			// excludes timeline lookup/TRS and immutable bitmap/frame creation.
			var views [5]three.Camera
			var batches [5][]three.Instance
			for i := range 5 {
				time := float64(i) * .375
				views[i], err = cameraAt(cameras, time)
				if err != nil {
					b.Fatal(err)
				}
				for _, track := range tracks {
					pose, _ := track.at(time)
					axis, err := rotationAxis(track.Axis)
					if err != nil {
						b.Fatal(err)
					}
					batches[i] = append(batches[i], three.Instance{Mesh: track.Mesh, Model: pose.matrix(axis), Material: track.Material})
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
		c, cameras, tracks := workloadFixture(b, 1)
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
