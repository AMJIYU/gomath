package scene3d

import (
	"bytes"
	"fmt"
	"gomath/geom"
	"gomath/three"
	"image"
	"image/color"
	"math"
	"testing"
)

func bitmapBytes(img image.Image) []byte {
	b := img.Bounds()
	out := make([]byte, 0, b.Dx()*b.Dy()*4)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := color.RGBAModel.Convert(img.At(x, y)).(color.RGBA)
			out = append(out, c.R, c.G, c.B, c.A)
		}
	}
	return out
}

func TestPoseGeometryAndEndpoints(t *testing.T) {
	cfg, cameras, tracks := basicFixture(t)
	a, b := IdentityPose(), IdentityPose()
	a.Position, b.Position = geom.Vec3{X: -2, Y: .3}, geom.Vec3{X: 3, Y: -.7, Z: 1}
	a.Scale, b.Scale = geom.Vec3{X: -1, Y: 2, Z: .5}, geom.Vec3{X: -2, Y: 1, Z: 1.5}
	a.Angle, b.Angle = -.2, 4*math.Pi+.4
	tracks[0].Axis = geom.Vec3{X: 1, Y: 2, Z: -3}
	tracks[0].Keys = []PoseKey{{Time: .25, Pose: a}, {Time: 1.75, Pose: b}}
	s := mustNew(t, cfg, cameras, tracks)
	for _, time := range []float64{0, .25, .625, 1, 1.375, 1.75, 2} {
		pose, visible, err := s.PoseAt(7, time)
		if err != nil || !visible {
			t.Fatal(pose, visible, err)
		}
		u := math.Max(0, math.Min(1, (time-.25)/1.5))
		want := Pose{Position: a.Position.Mul(1 - u).Add(b.Position.Mul(u)), Scale: a.Scale.Mul(1 - u).Add(b.Scale.Mul(u)), Angle: a.Angle*(1-u) + b.Angle*u}
		exact := time <= .25 || time >= 1.75
		if (exact && pose != want) || pose.Position.Sub(want.Position).Norm() > 2e-14 || pose.Scale.Sub(want.Scale).Norm() > 2e-14 || math.Abs(pose.Angle-want.Angle) > 2e-14 {
			t.Fatalf("time%g pose%+v want%+v", time, pose, want)
		}
		matrix := pose.matrix(s.tracks[0].Axis)
		for _, p := range []geom.Vec3{{X: 1}, {Y: 1}, {Z: 1}, {X: .3, Y: -.7, Z: 1.2}} {
			got, want := matrix.Apply(p), referenceTransform(p, tracks[0].Axis, want)
			if got.Sub(want).Norm() > 2e-14 {
				t.Fatalf("time%g geometry%+v want%+v", time, got, want)
			}
		}
	}
	// A whole turn must not collapse to identity at all interior times.
	tracks[0].Axis = geom.Vec3{Z: 1}
	a, b = IdentityPose(), IdentityPose()
	b.Angle = 2 * math.Pi
	tracks[0].Keys = []PoseKey{{Pose: a}, {Time: 2, Pose: b}}
	s = mustNew(t, cfg, cameras, tracks)
	pose, _, _ := s.PoseAt(7, 1)
	got := pose.matrix(geom.Vec3{Z: 1}).Apply(geom.Vec3{X: 1})
	if math.Abs(got.X+1) > 1e-14 || math.Abs(got.Y) > 1e-14 || math.Abs(got.Norm()-1) > 1e-14 {
		t.Fatalf("half-turn geometry collapsed or blended matrix: %v", got)
	}
	if p, _, _ := s.PoseAt(7, 2); p != b {
		t.Fatal("endpoint angle was wrapped")
	}
}

func TestCameraKeysAndInvalidInterior(t *testing.T) {
	cfg, cameras, tracks := basicFixture(t)
	a := cameras[0].Camera
	b := a
	b.Position = geom.Vec3{X: 2, Y: 1, Z: 4}
	b.Target = geom.Vec3{X: .5, Y: -.2, Z: -.7}
	b.FOVY, b.OrthoHeight = 1.1, 6
	cameras = []CameraKey{{Time: .25, Camera: a}, {Time: 1.75, Camera: b}}
	for _, projection := range []three.Projection{three.Perspective, three.Orthographic} {
		cameras[0].Camera.Projection, cameras[1].Camera.Projection = projection, projection
		s := mustNew(t, cfg, cameras, tracks)
		for _, time := range []float64{0, .25, 1, 1.75, 2} {
			got, err := s.CameraAt(time)
			if err != nil {
				t.Fatal(err)
			}
			u := math.Max(0, math.Min(1, (time-.25)/1.5))
			want := a
			want.Projection = projection
			want.Position = a.Position.Mul(1 - u).Add(b.Position.Mul(u))
			want.Target = a.Target.Mul(1 - u).Add(b.Target.Mul(u))
			want.FOVY, want.OrthoHeight = a.FOVY*(1-u)+b.FOVY*u, a.OrthoHeight*(1-u)+b.OrthoHeight*u
			if got != want {
				t.Fatalf("time%g camera%+v want%+v", time, got, want)
			}
		}
	}
	// Both endpoint look-at cameras are valid; at t=1 Position equals Target.
	a.Position, b.Position = geom.Vec3{Z: 2}, geom.Vec3{Z: -2}
	a.Target, b.Target = geom.Vec3{}, geom.Vec3{}
	s := mustNew(t, cfg, []CameraKey{{Camera: a}, {Time: 2, Camera: b}}, tracks)
	if _, err := s.CameraAt(1); err == nil {
		t.Fatal("singular interior camera accepted")
	}
	if frame, err := s.Sample(1); frame != nil || err == nil {
		t.Fatal("invalid camera returned partial frame")
	}
	if _, err := s.Sample(0); err != nil {
		t.Fatal("failure poisoned valid endpoint", err)
	}
	// Another valid endpoint pair passes through a view parallel to Up.
	a.Position, b.Position = geom.Vec3{X: -1, Y: 2}, geom.Vec3{X: 1, Y: 2}
	s = mustNew(t, cfg, []CameraKey{{Camera: a}, {Time: 2, Camera: b}}, tracks)
	if _, err := s.Sample(1); err == nil {
		t.Fatal("parallel-up interior accepted")
	}
}

func TestVisibilityAddRemoveBoundaries(t *testing.T) {
	cfg, cameras, tracks := basicFixture(t)
	tracks[0].Visible = []Visibility{{Add: .5, Remove: 1}, {Add: 1.5}}
	a, b := IdentityPose(), IdentityPose()
	b.Angle = math.Pi
	tracks[0].Keys = []PoseKey{{Pose: a}, {Time: 2, Pose: b}}
	s := mustNew(t, cfg, cameras, tracks)
	bg := color.RGBAModel.Convert(cfg.Renderer.Background).(color.RGBA)
	for _, time := range []float64{0, math.Nextafter(.5, 0), .5, math.Nextafter(.5, 1), math.Nextafter(1, 0), 1, math.Nextafter(1, 2), math.Nextafter(1.5, 0), 1.5, 2} {
		want := time >= .5 && time < 1 || time >= 1.5
		pose, visible, err := s.PoseAt(7, time)
		if err != nil || visible != want || pose.Angle != math.Pi*(time/2) {
			t.Fatalf("time%.18g pose%+v visible%t want%t %v", time, pose, visible, want, err)
		}
		frame, err := s.Sample(time)
		if err != nil {
			t.Fatal(err)
		}
		if center := frame.Objects[0].Bitmap.RGBAAt(20, 15); (center != bg) != want {
			t.Fatalf("time%.18g rendered visibility mismatch %v", time, center)
		}
	}
	tracks[0].Visible = []Visibility{{Remove: 2}}
	s = mustNew(t, cfg, cameras, tracks)
	if _, visible, _ := s.PoseAt(7, 2); visible {
		t.Fatal("remove at Duration was not exclusive")
	}
	tracks[0].Visible = []Visibility{{Add: 2}}
	s = mustNew(t, cfg, cameras, tracks)
	if _, v, _ := s.PoseAt(7, math.Nextafter(2, 0)); v {
		t.Fatal("endpoint-only Add appeared early")
	}
	if _, v, _ := s.PoseAt(7, 2); !v {
		t.Fatal("endpoint Add was excluded")
	}
	tracks[0].Visible = []Visibility{{Remove: 1}, {Add: 1}}
	s = mustNew(t, cfg, cameras, tracks)
	if _, v, _ := s.PoseAt(7, 1); !v {
		t.Fatal("touching visibility spans opened a gap")
	}
}

func TestTimelinePixelsAgainstIndependentRays(t *testing.T) {
	for _, projection := range []three.Projection{three.Perspective, three.Orthographic} {
		for _, scale := range []int{1, 2, 4} {
			for _, filter := range []three.TextureFilter{three.Nearest, three.Bilinear} {
				t.Run(fmt.Sprintf("projection%d/ssaa%d/filter%d", projection, scale, filter), func(t *testing.T) {
					cfg, cameras, tracks := basicFixture(t)
					cfg.Renderer.Supersampling = scale
					cfg.Renderer.Background = color.NRGBA{R: 17, G: 31, B: 73, A: 107}
					aCam := cameras[0].Camera
					aCam.Position, aCam.Target = geom.Vec3{X: .2, Y: -.1, Z: 5.5}, geom.Vec3{X: -.15}
					aCam.Projection, aCam.FOVY, aCam.OrthoHeight = projection, .83, 4.7
					bCam := aCam
					bCam.Position, bCam.Target = geom.Vec3{X: 1.1, Y: .4, Z: 4.7}, geom.Vec3{X: .2, Y: .17, Z: -.3}
					bCam.FOVY, bCam.OrthoHeight = 1.02, 5.4
					cameras = []CameraKey{{Camera: aCam}, {Time: 2, Camera: bCam}}
					first, last := IdentityPose(), IdentityPose()
					first.Position, last.Position = geom.Vec3{X: -.7, Y: .1, Z: .2}, geom.Vec3{X: .8, Y: -.2, Z: .1}
					first.Scale, last.Scale = geom.Vec3{X: -.8, Y: 1.1, Z: .9}, geom.Vec3{X: -1.3, Y: .7, Z: 1.2}
					first.Angle, last.Angle = -.4, 2*math.Pi+.7
					tracks[0].Axis = geom.Vec3{X: 1, Y: 2, Z: 3}
					tracks[0].Keys = []PoseKey{{Time: .25, Pose: first}, {Time: 1.75, Pose: last}}
					tracks[0].Material.Color.A = 173
					tracks[0].Mesh.UVs = []three.UV{{U: -.27, V: 1.17}, {U: 1.41, V: .17}, {U: .23, V: -.31}}
					src := image.NewNRGBA(image.Rect(0, 0, 4, 4))
					for y := range 4 {
						for x := range 4 {
							src.SetNRGBA(x, y, color.NRGBA{R: uint8(53 + x*61), G: uint8(47 + y*63), B: 137, A: []uint8{0, 128, 255}[(x+y)%3]})
						}
					}
					tex, err := three.NewTexture(src)
					if err != nil {
						t.Fatal(err)
					}
					tracks[0].Material.Texture = tex
					tracks[0].Material.Sampler = three.Sampler{Filter: filter, AddressU: three.Repeat, AddressV: three.Clamp}
					cross := MeshTrack{ID: 8, Mesh: &three.Mesh{Vertices: []geom.Vec3{{X: -1.37, Y: -1.03, Z: -.7}, {X: 1.53, Y: -.93, Z: .73}, {X: .17, Y: 1.41, Z: -.23}}, Triangles: []three.Triangle{{0, 1, 2}}}, Material: three.Material{Color: color.NRGBA{R: 31, G: 197, B: 89, A: 119}}}
					back := MeshTrack{ID: 9, Mesh: &three.Mesh{Vertices: []geom.Vec3{{X: -3.17, Y: -2.43, Z: -1.41}, {X: 3.53, Y: -2.13, Z: -1.79}, {X: .13, Y: 2.81, Z: -1.51}}, Triangles: []three.Triangle{{0, 1, 2}}}, Material: three.Material{Color: color.NRGBA{R: 83, G: 103, B: 211, A: 255}}}
					tracks = append(tracks, cross, back)
					s := mustNew(t, cfg, cameras, tracks)
					for _, time := range []float64{0, .25, .625, 1, 1.375, 1.75, 2} {
						u := math.Max(0, math.Min(1, (time-.25)/1.5))
						pose := Pose{Position: first.Position.Mul(1 - u).Add(last.Position.Mul(u)), Scale: first.Scale.Mul(1 - u).Add(last.Scale.Mul(u)), Angle: first.Angle*(1-u) + last.Angle*u}
						v := time / 2
						camera := aCam
						camera.Position, camera.Target = aCam.Position.Mul(1-v).Add(bCam.Position.Mul(v)), aCam.Target.Mul(1-v).Add(bCam.Target.Mul(v))
						camera.FOVY, camera.OrthoHeight = aCam.FOVY*(1-v)+bCam.FOVY*v, aCam.OrthoHeight*(1-v)+bCam.OrthoHeight*v
						want := referencePixels(cfg, camera, referenceInstance{tracks[0].Mesh, tracks[0].Axis, pose, tracks[0].Material, src}, referenceInstance{cross.Mesh, geom.Vec3{Z: 1}, IdentityPose(), cross.Material, nil}, referenceInstance{back.Mesh, geom.Vec3{Z: 1}, IdentityPose(), back.Material, nil})
						frame, err := s.Sample(time)
						if err != nil {
							t.Fatal(err)
						}
						got := bitmapBytes(frame.Objects[0].Bitmap)
						for i, ch := range got {
							if math.Abs(float64(ch)-float64(want.Pix[i])) > 1 {
								t.Fatalf("time%g pixel%d,%d channel%d: %d want%d", time, i/4%cfg.Renderer.Width, i/4/cfg.Renderer.Width, i%4, ch, want.Pix[i])
							}
						}
					}
				})
			}
		}
	}
}

func TestZeroDurationAndBackground(t *testing.T) {
	cfg, cameras, _ := basicFixture(t)
	cfg.Duration = 0
	s := mustNew(t, cfg, cameras, nil)
	f, err := s.Sample(0)
	if err != nil {
		t.Fatal(err)
	}
	bg := make([]byte, cfg.Renderer.Width*cfg.Renderer.Height*4)
	c := color.RGBAModel.Convert(cfg.Renderer.Background).(color.RGBA)
	for i := 0; i < len(bg); i += 4 {
		copy(bg[i:], []byte{c.R, c.G, c.B, c.A})
	}
	if !bytes.Equal(bg, bitmapBytes(f.Objects[0].Bitmap)) {
		t.Fatal("empty scene did not render background")
	}
	if _, err := s.Sample(math.SmallestNonzeroFloat64); err == nil {
		t.Fatal("zero-duration sample exceeded endpoint")
	}
}
