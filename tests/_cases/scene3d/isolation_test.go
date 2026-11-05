package scene3d

import (
	"bytes"
	"gomath"
	"gomath/geom"
	"gomath/three"
	"image"
	"image/color"
	"math"
	"sync"
	"testing"
)

func TestInputAndFrameIsolationConcurrentOrder(t *testing.T) {
	c, k, m := basicFixture(t)
	c.Renderer.Supersampling = 2
	p := IdentityPose()
	p.Position.X = .4
	p.Angle = .7
	p.Scale = geom.Vec3{X: -1.1, Y: .9, Z: 1.2}
	m[0].Axis = geom.Vec3{X: 1, Y: 2, Z: 3}
	first := IdentityPose()
	first.Scale.X = -1
	m[0].Keys = []PoseKey{{Pose: first}, {Time: 2, Pose: p}}
	m[0].Visible = []Visibility{{Add: .1, Remove: 1.8}}
	m[0].Mesh.Normals = []geom.Vec3{{Z: 1}, {Z: 1}, {Z: 1}}
	end := k[0]
	end.Time = 2
	end.Camera.Position.X = 1
	end.Camera.FOVY = 1.1
	k = append(k, end)
	src := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	for y := range 2 {
		for x := range 2 {
			src.SetNRGBA(x, y, color.NRGBA{R: 231, G: uint8(53 + 101*x), B: uint8(31 + 113*y), A: uint8(127 + 128*((x+y)%2))})
		}
	}
	tex, err := three.NewTexture(src)
	if err != nil {
		t.Fatal(err)
	}
	m[0].Material.Texture = tex
	m[0].Material.Sampler.Filter = three.Bilinear
	s := mustNew(t, c, k, m)
	times := []float64{0, .25, .75, 1.5, 2}
	wants := make([][]byte, len(times))
	for i, time := range times {
		f, err := s.Sample(time)
		if err != nil {
			t.Fatal(err)
		}
		wants[i] = bitmapBytes(f.Objects[0].Bitmap)
	}
	// Returned paths, objects, cameras and even asset-pointer replacement belong
	// only to that frame. No new gomath IDs are allocated by Scene.Sample.
	f, err := s.Sample(.75)
	if err != nil {
		t.Fatal(err)
	}
	old, err := s.Sample(.75)
	if err != nil {
		t.Fatal(err)
	}
	f.Time = 99
	f.Camera.FrameWidth = 100
	f.Objects[0].ID = 1
	f.Objects[0].Style.Opacity = 0
	f.Objects[0].Path.Contours[0].Curves[0].P0.X = math.NaN()
	*f.Objects[0].Bitmap = gomath.Bitmap{}
	if !bytes.Equal(bitmapBytes(old.Objects[0].Bitmap), wants[2]) {
		t.Fatal("frames shared mutable path or bitmap storage")
	}
	// Mutate every source slice while 8 workers sample private snapshots. A
	// missing copy becomes both a race and visible invalid geometry/key state.
	var wg sync.WaitGroup
	start := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		for i := range 40 {
			m[0].Mesh.Vertices[0].X = math.NaN()
			m[0].Mesh.Normals[0].Z = math.NaN()
			m[0].Mesh.Triangles[0][0] = 999
			m[0].Mesh.UVs[0].U = math.NaN()
			m[0].Keys[0].Time = math.NaN()
			m[0].Keys[1].Pose.Position.X = float64(i)
			m[0].Visible[0].Add = math.NaN()
			m[0].Axis.X = math.NaN()
			m[0].ID = 999
			m[0].Material.Color.A = 0
			m[0].Material.Texture = nil
			k[0].Camera.Position = geom.Vec3{}
			k[1].Camera.FOVY = 0
			k[0].Time = math.NaN()
			c.Duration = 100
			c.FrameWidth = 100
			c.Renderer.Width = 0
			src.SetNRGBA(0, 0, color.NRGBA{R: uint8(i), A: 255})
		}
	}()
	for worker := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for iteration := range 15 {
				i := (worker + iteration*3) % len(times)
				frame, err := s.Sample(times[i])
				if err != nil {
					t.Error(err)
					return
				}
				if frame.Objects[0].ID != s.BitmapID() || !bytes.Equal(bitmapBytes(frame.Objects[0].Bitmap), wants[i]) {
					t.Error("unordered concurrent sample changed identity or pixels")
					return
				}
			}
		}()
	}
	close(start)
	wg.Wait()
	if s.Duration() != 2 {
		t.Fatal("config was retained")
	}
	pose, visible, err := s.PoseAt(7, .75)
	if err != nil || !visible || !pose.Position.Finite() {
		t.Fatal("track identity or keys changed", pose, visible, err)
	}
	camera, err := s.CameraAt(.75)
	if err != nil || camera.Position.Z != 5 {
		t.Fatal("camera input leaked", camera, err)
	}
}
