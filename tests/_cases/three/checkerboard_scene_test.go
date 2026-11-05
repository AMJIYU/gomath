package three_test

import (
	"bytes"
	"gomath/geom"
	"gomath/scene3d"
	"gomath/three"
	"image"
	"image/color"
	"math"
	"sync"
	"testing"
)

func checkerTRS(p, s, axis geom.Vec3, angle float64) geom.Mat4 {
	a := checkerUnit(axis)
	c, v, w := math.Cos(angle), 1-math.Cos(angle), math.Sin(angle)
	return geom.Mat4{
		(c + a.X*a.X*v) * s.X, (a.X*a.Y*v - a.Z*w) * s.Y, (a.X*a.Z*v + a.Y*w) * s.Z, p.X,
		(a.Y*a.X*v + a.Z*w) * s.X, (c + a.Y*a.Y*v) * s.Y, (a.Y*a.Z*v - a.X*w) * s.Z, p.Y,
		(a.Z*a.X*v - a.Y*w) * s.X, (a.Z*a.Y*v + a.X*w) * s.Y, (c + a.Z*a.Z*v) * s.Z, p.Z,
		0, 0, 0, 1,
	}
}

func checkerImageBytes(img image.Image) []byte {
	var out []byte
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			c := color.RGBAModel.Convert(img.At(x, y)).(color.RGBA)
			out = append(out, c.R, c.G, c.B, c.A)
		}
	}
	return out
}

func TestCheckerboardSceneIntermediateFramesAndSnapshots(t *testing.T) {
	for _, projection := range []three.Projection{three.Perspective, three.Orthographic} {
		calls := 0
		groups, e := three.CheckerboardSurface(-1, 1, -.8, .9, 5, 4, func(u, v float64) (geom.Vec3, error) { calls++; return checkerPoint(u, v), nil })
		if e != nil {
			t.Fatal(e)
		}
		ref := checkerReferenceGrid(-1, 1, -.8, .9, 5, 4, checkerPoint)
		styles := checkerStyles(t, "uv-lights")
		cfg := scene3d.DefaultConfig(43, 31, 2)
		cfg.Renderer = checkerLights(cfg.Renderer)
		cfg.Renderer.Supersampling = 2
		cfg.Renderer.Background = color.NRGBA{R: 17, G: 31, B: 53, A: 255}
		axis := geom.Vec3{X: .3, Y: 1, Z: .7}
		poses := []scene3d.PoseKey{{Pose: scene3d.Pose{Position: geom.Vec3{X: -.2, Y: .1}, Scale: geom.Vec3{X: -.95, Y: 1.1, Z: .9}, Angle: -.27}}, {Time: 2, Pose: scene3d.Pose{Position: geom.Vec3{X: .17, Y: -.08, Z: .12}, Scale: geom.Vec3{X: -1.17, Y: .87, Z: 1.05}, Angle: 1.31}}}
		first, last := poses[0].Pose, poses[1].Pose
		camera0 := checkerCamera(projection)
		camera1 := camera0
		camera1.Position = geom.Vec3{X: 3.6, Y: -3.1, Z: 3.7}
		camera1.Target = geom.Vec3{X: .07, Y: -.1, Z: .12}
		camera1.FOVY = .71
		camera1.OrthoHeight = 3.1
		cameras := []scene3d.CameraKey{{Camera: camera0}, {Time: 2, Camera: camera1}}
		var tracks []scene3d.MeshTrack
		for group, mesh := range groups {
			if mesh != nil {
				tracks = append(tracks, scene3d.MeshTrack{ID: uint64(group + 1), Mesh: mesh, Material: styles[group].material, Axis: axis, Keys: poses, Visible: []scene3d.Visibility{{Add: .1, Remove: 1.9}}})
			}
		}
		scene, e := scene3d.New(cfg, cameras, tracks)
		if e != nil {
			t.Fatal(e)
		}
		times := []float64{0, .1, .37, 1, 1.73, 1.9, 2}
		wants := make([]*image.RGBA, len(times))
		actual := make([][]byte, len(times))
		for i, time := range times {
			mix := func(a, b float64) float64 { return a*(1-time/2) + b*time/2 }
			vec := func(a, b geom.Vec3) geom.Vec3 { return geom.Vec3{X: mix(a.X, b.X), Y: mix(a.Y, b.Y), Z: mix(a.Z, b.Z)} }
			camera := camera0
			camera.Position = vec(camera0.Position, camera1.Position)
			camera.Target = vec(camera0.Target, camera1.Target)
			camera.FOVY = mix(camera0.FOVY, camera1.FOVY)
			camera.OrthoHeight = mix(camera0.OrthoHeight, camera1.OrthoHeight)
			model := checkerTRS(vec(first.Position, last.Position), vec(first.Scale, last.Scale), axis, mix(first.Angle, last.Angle))
			var tris []checkerWorldTriangle
			if time >= .1 && time < 1.9 {
				tris = checkerWorld(ref, model, styles)
			}
			wants[i] = checkerRayReference(cfg.Renderer, camera, tris)
			frame, e := scene.Sample(time)
			if e != nil {
				t.Fatal(e)
			}
			if frame.Objects[0].ID != scene.BitmapID() || frame.Find(scene.BitmapID()) != frame.Objects[0] {
				t.Fatal("sample identity changed")
			}
			checkerPixels(t, frame.Objects[0].Bitmap, wants[i])
			actual[i] = checkerImageBytes(frame.Objects[0].Bitmap)
		}
		if bytes.Equal(actual[2], actual[3]) || bytes.Equal(actual[3], actual[4]) {
			t.Fatal("3D middle frames must change with TRS/camera")
		}
		// Corrupt all mutable authoring buffers and callback state after New.
		for _, mesh := range groups {
			for i := range mesh.Vertices {
				mesh.Vertices[i] = geom.Vec3{X: math.NaN()}
				mesh.Normals[i] = geom.Vec3{}
				mesh.UVs[i] = three.UV{U: math.NaN()}
			}
			for i := range mesh.Triangles {
				mesh.Triangles[i] = three.Triangle{}
			}
		}
		poses[0].Pose.Scale = geom.Vec3{}
		poses[1].Pose.Angle = math.NaN()
		cameras[0].Camera.Up = geom.Vec3{}
		tracks[0].Material.Color = color.NRGBA{}
		tracks[0].Visible[0].Add = 1.8
		cfg.Renderer.PointLights[0].RGB = [3]float64{}
		for _, style := range styles {
			for i := range style.source.Pix {
				style.source.Pix[i] = 0
			}
		}
		var wg sync.WaitGroup
		for worker := range 8 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for k := range times {
					i := (k + worker) % len(times)
					frame, e := scene.Sample(times[i])
					if e != nil {
						t.Errorf("concurrent sample: %v", e)
						return
					}
					if !bytes.Equal(checkerImageBytes(frame.Objects[0].Bitmap), actual[i]) || frame.Objects[0].ID != scene.BitmapID() {
						t.Error("snapshot/order/concurrent sample changed")
						return
					}
					frame.Objects[0].Bitmap = nil
				}
			}()
		}
		wg.Wait()
		if calls != 30 {
			t.Fatal("surface callback retained/re-evaluated by scene", calls)
		}
	}
}

func TestCheckerboardEmptyGroupSceneAndSnapshotBudget(t *testing.T) {
	groups := checkerBuild(t, 1, 1, checkerPoint)
	if groups[1] != nil {
		t.Fatal("1x1 odd group must be nil")
	}
	var tracks []scene3d.MeshTrack
	for i, mesh := range groups {
		if mesh != nil {
			tracks = append(tracks, scene3d.MeshTrack{ID: uint64(i + 1), Mesh: mesh, Material: three.DefaultMaterial()})
		}
	}
	cfg := scene3d.DefaultConfig(31, 23, 1)
	camera := []scene3d.CameraKey{{Camera: checkerCamera(three.Perspective)}}
	s, e := scene3d.New(cfg, camera, tracks)
	if e != nil {
		t.Fatal(e)
	}
	if _, e := s.Sample(.5); e != nil {
		t.Fatal(e)
	}
	maximal := checkerBuild(t, 256, 256, func(u, v float64) geom.Vec3 { return geom.Vec3{X: u, Y: v} })
	tracks = nil
	for i := range 3 {
		for group, mesh := range maximal {
			tracks = append(tracks, scene3d.MeshTrack{ID: uint64(i*2 + group + 1), Mesh: mesh, Material: three.DefaultMaterial()})
		}
	}
	if scene, e := scene3d.New(cfg, camera, tracks); scene != nil || e == nil {
		t.Fatal("aggregate checkerboard snapshot budget ignored")
	}
}
