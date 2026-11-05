package scene3d

import (
	"gomath/geom"
	"gomath/render"
	"gomath/three"
	"image/color"
	"math"
	"testing"
)

var _ render.FrameSource = (*Scene)(nil)

func basicFixture(t testing.TB) (Config, []CameraKey, []MeshTrack) {
	t.Helper()
	cfg := DefaultConfig(41, 31, 2)
	cfg.Renderer.Light = three.Light{Ambient: 1}
	cam := three.Camera{Position: geom.Vec3{Z: 5}, Up: geom.Vec3{Y: 1}, Projection: three.Perspective, Near: .1, Far: 20, FOVY: .9, OrthoHeight: 4}
	m := &three.Mesh{Vertices: []geom.Vec3{{X: -1, Y: -.8}, {X: 1.2, Y: -.7}, {X: .1, Y: 1}}, Triangles: []three.Triangle{{0, 1, 2}}, UVs: []three.UV{{}, {U: 1}, {U: .5, V: 1}}}
	return cfg, []CameraKey{{Camera: cam}}, []MeshTrack{{ID: 7, Mesh: m, Material: three.Material{Color: color.NRGBA{R: 231, G: 83, B: 47, A: 255}}}}
}

func mustNew(t testing.TB, cfg Config, cameras []CameraKey, tracks []MeshTrack) *Scene {
	t.Helper()
	s, err := New(cfg, cameras, tracks)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestFrameSourceAndIdentity(t *testing.T) {
	cfg, cameras, tracks := basicFixture(t)
	s := mustNew(t, cfg, cameras, tracks)
	if s.Duration() != 2 || s.BitmapID() == 0 {
		t.Fatal("invalid duration/identity")
	}
	for _, time := range []float64{0, 2, .5, 0, 1} {
		f, err := s.Sample(time)
		if err != nil {
			t.Fatal(err)
		}
		if f.Time != time || len(f.Objects) != 1 || f.Camera == nil || f.Camera.FrameWidth != cfg.FrameWidth || f.Find(s.BitmapID()) != f.Objects[0] {
			t.Fatalf("invalid frame at%g: %+v", time, f)
		}
		o := f.Objects[0]
		if err := o.Validate(); err != nil {
			t.Fatal(err)
		}
		if got := o.Bitmap.RGBAAt(20, 15); got != (color.RGBA{R: 231, G: 83, B: 47, A: 255}) {
			t.Fatalf("real triangle missing: %v", got)
		}
		pose, visible, err := s.PoseAt(7, time)
		if err != nil || !visible || pose != IdentityPose() {
			t.Fatalf("identity pose: %+v %t %v", pose, visible, err)
		}
	}
	other := mustNew(t, cfg, cameras, tracks)
	if other.BitmapID() == s.BitmapID() {
		t.Fatal("different scenes share object identity")
	}
}

func TestInvalidSamples(t *testing.T) {
	cfg, cameras, tracks := basicFixture(t)
	s := mustNew(t, cfg, cameras, tracks)
	for _, time := range []float64{-1, 3, math.NaN(), math.Inf(1)} {
		if f, err := s.Sample(time); f != nil || err == nil {
			t.Fatalf("invalid time accepted: %v %v", f, err)
		}
		if _, err := s.CameraAt(time); err == nil {
			t.Fatal("CameraAt accepted invalid time")
		}
		if _, _, err := s.PoseAt(7, time); err == nil {
			t.Fatal("PoseAt accepted invalid time")
		}
	}
	if _, _, err := s.PoseAt(9, 0); err == nil {
		t.Fatal("unknown track accepted")
	}
	for _, zero := range []*Scene{nil, {}} {
		if f, err := zero.Sample(0); f != nil || err == nil {
			t.Fatal("nil/uninitialized scene accepted")
		}
		if zero.BitmapID() != 0 || zero.Duration() != 0 {
			t.Fatal("invalid scene metadata")
		}
	}
}
