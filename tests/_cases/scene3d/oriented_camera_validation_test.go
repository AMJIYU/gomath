package scene3d

import (
	"bytes"
	"gomath/geom"
	"gomath/three"
	"math"
	"strings"
	"sync"
	"testing"
)

func TestOrientedCameraTrackValidation(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*OrientedCameraTrack)
	}{
		{"empty", func(c *OrientedCameraTrack) { c.Keys = nil }},
		{"zero", func(c *OrientedCameraTrack) { c.Keys[1].Orientation = geom.Quat{} }},
		{"nan-q", func(c *OrientedCameraTrack) { c.Keys[1].Orientation.X = math.NaN() }},
		{"inf-q", func(c *OrientedCameraTrack) { c.Keys[1].Orientation.W = math.Inf(1) }},
		{"nan-eye", func(c *OrientedCameraTrack) { c.Keys[1].Position.X = math.NaN() }},
		{"eye-bound", func(c *OrientedCameraTrack) { c.Keys[1].Position.X = 1e12 + 1 }},
		{"duplicate", func(c *OrientedCameraTrack) { c.Keys[1].Time = c.Keys[0].Time }},
		{"reverse", func(c *OrientedCameraTrack) { c.Keys[1].Time = .1 }},
		{"negative", func(c *OrientedCameraTrack) { c.Keys[0].Time = -1 }},
		{"late", func(c *OrientedCameraTrack) { c.Keys[1].Time = 3 }},
		{"nan-time", func(c *OrientedCameraTrack) { c.Keys[1].Time = math.NaN() }},
		{"inf-time", func(c *OrientedCameraTrack) { c.Keys[1].Time = math.Inf(1) }},
		{"near", func(c *OrientedCameraTrack) { c.Near = 0 }},
		{"far", func(c *OrientedCameraTrack) { c.Far = c.Near }},
		{"far-inf", func(c *OrientedCameraTrack) { c.Far = math.Inf(1) }},
		{"projection", func(c *OrientedCameraTrack) { c.Projection = three.Projection(9) }},
		{"fov", func(c *OrientedCameraTrack) { c.Keys[1].FOVY = math.Pi }},
		{"unused-finite", func(c *OrientedCameraTrack) { c.Keys[1].OrthoHeight = math.NaN() }},
		{"ortho", func(c *OrientedCameraTrack) { c.Projection = three.Orthographic; c.Keys[1].OrthoHeight = 0 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _, meshes := basicFixture(t)
			_, track := loadCameraOrientation(t)
			tc.edit(&track)
			if s, err := NewWithOrientedCamera(c, track, meshes, nil); s != nil || err == nil {
				t.Fatal("invalid camera accepted", s, err)
			}
		})
	}
	c, cameras, meshes := basicFixture(t)
	cameras[0].Camera = three.Camera{Mode: three.OrientedCamera, Position: geom.Vec3{Z: 5}, Orientation: geom.IdentityQuat(), Near: .1, Far: 20, FOVY: .9}
	if _, err := New(c, cameras, meshes); err == nil || !strings.Contains(err.Error(), "NewWithOrientedCamera") {
		t.Fatal("mixed look-at constructor accepted oriented keys", err)
	}
	_, track := loadCameraOrientation(t)
	if _, err := newScene(c, cameras, &track, meshes, nil); err == nil {
		t.Fatal("internal mixed camera tracks accepted")
	}
	track.Keys = make([]OrientedCameraKey, MaxKeyframes+1)
	if _, err := NewWithOrientedCamera(c, track, meshes, nil); err == nil {
		t.Fatal("oversized camera keys accepted")
	}
}

func TestOrientedCameraPolesEndpointsAndBudgets(t *testing.T) {
	c, _, meshes := basicFixture(t)
	track := OrientedCameraTrack{Near: .1, Far: 20, Keys: []OrientedCameraKey{{Time: .25, Position: geom.Vec3{Z: 5}, Orientation: geom.Quat{W: 1e308}, FOVY: .9, OrthoHeight: 4}, {Time: 1.75, Position: geom.Vec3{Z: 5}, Orientation: geom.Quat{W: -math.SmallestNonzeroFloat64}, FOVY: .9, OrthoHeight: 4}}}
	s := mustOriented(t, c, track, meshes, nil)
	for _, time := range []float64{0, .25, 1, 1.75, 2} {
		camera, err := s.CameraAt(time)
		if err != nil {
			t.Fatal(err)
		}
		want := geom.IdentityQuat()
		if time >= 1.75 {
			want.W = -1
		}
		if camera.Orientation != want {
			t.Fatal("held/antipodal endpoint", time, camera)
		}
		if _, err = s.Sample(time); err != nil {
			t.Fatal(err)
		}
	}
	for _, time := range []float64{-1, 3, math.NaN(), math.Inf(1)} {
		if _, err := s.CameraAt(time); err == nil {
			t.Fatal("invalid camera query accepted")
		}
		if frame, err := s.Sample(time); frame != nil || err == nil {
			t.Fatal("invalid sample accepted", frame, err)
		}
	}
	// Crossing the look-at Up pole has a valid quaternion frame and visible
	// geometry. At t=1, local -Z has rotated to world +Y, local +Y to +Z.
	box, err := three.Box(1, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	pose := IdentityPose()
	pose.Position = geom.Vec3{Y: 3}
	meshes[0].Mesh = box
	meshes[0].Keys = []PoseKey{{Pose: pose}}
	track.Keys = []OrientedCameraKey{{Orientation: geom.IdentityQuat(), FOVY: .9, OrthoHeight: 4}, {Time: 2, Orientation: geom.Quat{X: 1}, FOVY: .9, OrthoHeight: 4}}
	for _, projection := range []three.Projection{three.Perspective, three.Orthographic} {
		track.Projection = projection
		s = mustOriented(t, c, track, meshes, nil)
		camera, err := s.CameraAt(1)
		if err != nil {
			t.Fatal(err)
		}
		v, err := camera.WorldToView(geom.Vec3{Y: 2})
		if err != nil || v.Sub(geom.Vec3{Z: 2}).Norm() > 1e-14 {
			t.Fatal(v, err)
		}
		frame, err := s.Sample(1)
		if err != nil || frame.Objects[0].Bitmap.RGBAAt(20, 15).A == 0 {
			t.Fatal("pole geometry missing", err)
		}
	}
	c.Duration = 0
	track.Keys = track.Keys[:1]
	s = mustOriented(t, c, track, meshes, nil)
	if _, err := s.Sample(0); err != nil {
		t.Fatal(err)
	}

	c, _, meshes = basicFixture(t)
	_, track = loadCameraOrientation(t)
	base := track.Keys[0]
	base.Time = 0
	track.Keys = make([]OrientedCameraKey, MaxKeyframes)
	for i := range track.Keys {
		track.Keys[i] = base
		track.Keys[i].Time = 2 * float64(i) / float64(len(track.Keys)-1)
	}
	mustOriented(t, c, track, meshes, nil) // Exactly 65536 camera keys with static mesh.
	meshes[0].Keys = []PoseKey{{Pose: IdentityPose()}}
	if _, err := NewWithOrientedCamera(c, track, meshes, nil); err == nil || !strings.Contains(err.Error(), "budget") {
		t.Fatal("combined camera/mesh keys exceeded", err)
	}
	track.Keys = track.Keys[:MaxKeyframes-2]
	points := []PointTrack{{ID: 11, Cloud: pointCountCloud(t, 1), RotationKeys: []RotationKey{{Rotation: geom.IdentityQuat()}}}}
	mustOriented(t, c, track, meshes, points)
	points[0].Keys = []PoseKey{{Pose: IdentityPose()}}
	if _, err := NewWithOrientedCamera(c, track, meshes, points); err == nil || !strings.Contains(err.Error(), "budget") {
		t.Fatal("combined camera/point keys exceeded", err)
	}
}

func TestOrientedCameraIsolationConcurrentOrder(t *testing.T) {
	f, track := loadCameraOrientation(t)
	c, _, meshes := basicFixture(t)
	c.Renderer.Supersampling = 2
	s := mustOriented(t, c, track, meshes, nil)
	wants := make([][]byte, len(f.Samples))
	views := make([]three.Camera, len(f.Samples))
	for i, sample := range f.Samples {
		frame, err := s.Sample(sample.Time)
		if err != nil {
			t.Fatal(err)
		}
		wants[i] = bitmapBytes(frame.Objects[0].Bitmap)
		views[i], err = s.CameraAt(sample.Time)
		if err != nil {
			t.Fatal(err)
		}
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		for range 100 {
			track.Keys[0].Time = -1
			track.Keys[1].Orientation = geom.Quat{}
			track.Keys[1].Position = geom.Vec3{X: math.NaN()}
			track.Keys[2].FOVY = math.NaN()
			track.Keys[2].OrthoHeight = math.Inf(1)
		}
		track = OrientedCameraTrack{}
	}()
	for worker := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for j := range 35 {
				i := (worker + 13*j) % len(f.Samples)
				time := f.Samples[i].Time
				view, err := s.CameraAt(time)
				if err != nil || view != views[i] {
					t.Error("camera snapshot/order", view, err)
					return
				}
				view.Position.X = math.NaN()
				view.Orientation = geom.Quat{}
				frame, err := s.Sample(time)
				if err != nil {
					t.Error(err)
					return
				}
				if frame.Objects[0].ID != s.BitmapID() || !bytes.Equal(wants[i], bitmapBytes(frame.Objects[0].Bitmap)) {
					t.Error("oriented camera authoring or returned values leaked")
					return
				}
			}
		}()
	}
	close(start)
	wg.Wait()
}
