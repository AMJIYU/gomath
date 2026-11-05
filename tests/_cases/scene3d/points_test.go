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

func mustCloud(t testing.TB, p []geom.Vec3, c []color.NRGBA, r []float64) *three.PointCloud {
	t.Helper()
	cloud, err := three.NewPointCloud(p, c, r)
	if err != nil {
		t.Fatal(err)
	}
	return cloud
}

func mustNewPoints(t testing.TB, c Config, cameras []CameraKey, meshes []MeshTrack, points []PointTrack) *Scene {
	t.Helper()
	s, err := NewWithPoints(c, cameras, meshes, points)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// Independently weighted premultiplied endpoint color, rounded to NRGBA bytes.
func referencePointColor(a, b color.NRGBA, u float64) color.NRGBA {
	if u == 0 {
		return a
	}
	if u == 1 {
		return b
	}
	weights := [2]float64{(1 - u) * float64(a.A) / 255, u * float64(b.A) / 255}
	alpha := weights[0] + weights[1]
	if alpha == 0 {
		return color.NRGBA{}
	}
	channel := func(x, y uint8) uint8 {
		return uint8(math.Round((weights[0]*float64(x) + weights[1]*float64(y)) / alpha))
	}
	return color.NRGBA{R: channel(a.R, b.R), G: channel(a.G, b.G), B: channel(a.B, b.B), A: uint8(math.Round(255 * alpha))}
}

func TestPointKeysGeometryAndRadiusPolicy(t *testing.T) {
	c, cameras, _ := basicFixture(t)
	a := mustCloud(t, []geom.Vec3{{X: 1, Y: .2, Z: -.1}, {Y: 1}}, []color.NRGBA{{B: 255}, {R: 23, G: 179, A: 128}}, []float64{1, 3})
	b := mustCloud(t, []geom.Vec3{{X: 2, Y: .3, Z: .1}, {X: -.3, Y: .8}}, []color.NRGBA{{R: 255, A: 255}, {G: 37, B: 231, A: 64}}, []float64{5, 2})
	first, last := IdentityPose(), IdentityPose()
	first.Scale, last.Scale = geom.Vec3{X: -1, Y: 2, Z: .5}, geom.Vec3{X: -2, Y: 1, Z: 1.5}
	first.Position, last.Position = geom.Vec3{Y: -.2}, geom.Vec3{X: .7, Y: .3, Z: -.4}
	last.Angle = 2 * math.Pi
	axis := geom.Vec3{X: 1, Y: 2, Z: 3}
	track := PointTrack{ID: 11, Axis: axis, Keys: []PoseKey{{Pose: first}, {Time: 2, Pose: last}}, PointKeys: []PointKey{{Time: .25, Cloud: a}, {Time: 1.75, Cloud: b}}}
	s := mustNewPoints(t, c, cameras, nil, []PointTrack{track})
	for _, time := range []float64{0, .25, .625, 1, 1.375, 1.75, 2} {
		v := time / 2
		pose := Pose{Position: first.Position.Mul(1 - v).Add(last.Position.Mul(v)), Scale: first.Scale.Mul(1 - v).Add(last.Scale.Mul(v)), Angle: 2 * math.Pi * v}
		gotPose, visible, err := s.PoseAt(11, time)
		if err != nil || !visible || gotPose.Position.Sub(pose.Position).Norm() > 2e-14 || gotPose.Scale.Sub(pose.Scale).Norm() > 2e-14 || math.Abs(gotPose.Angle-pose.Angle) > 2e-14 {
			t.Fatal("point PoseAt does not use shared ID/motion semantics", gotPose, pose, err)
		}
		cloud, err := s.points[0].cloudAt(time, gotPose)
		if err != nil {
			t.Fatal(err)
		}
		u := math.Max(0, math.Min(1, (time-.25)/1.5))
		for i := range 2 {
			pa, ca, ra, _ := a.Point(i)
			pb, cb, rb, _ := b.Point(i)
			position := pa.Mul(1 - u).Add(pb.Mul(u))
			want := referenceTransform(position, axis, pose)
			p, color, radius, err := cloud.Point(i)
			if err != nil || p.Sub(want).Norm() > 2e-14 || color != referencePointColor(ca, cb, u) || radius != ra*(1-u)+rb*u {
				t.Fatalf("t%g point%d: %v/%v color%v radius%g: %v", time, i, p, want, color, radius, err)
			}
			if i == 0 && u > 0 && color.B != 0 {
				t.Fatal("hidden blue RGB bled into visible red")
			}
		}
	}
	// A point centered at the local origin changes neither its center nor its
	// pixel radius under any scale/rotation. This compares actual raster bytes.
	cloud := mustCloud(t, []geom.Vec3{{}}, []color.NRGBA{{R: 217, G: 83, A: 173}}, []float64{4.3})
	start, end := IdentityPose(), IdentityPose()
	end.Scale, end.Angle = geom.Vec3{X: 3, Y: 2, Z: .5}, math.Pi
	track = PointTrack{ID: 1, Cloud: cloud, Keys: []PoseKey{{Pose: start}, {Time: 2, Pose: end}}}
	s = mustNewPoints(t, c, cameras, nil, []PointTrack{track})
	f0, _ := s.Sample(0)
	for _, time := range []float64{.5, 1, 2} {
		frame, err := s.Sample(time)
		if err != nil || !bytes.Equal(bitmapBytes(f0.Objects[0].Bitmap), bitmapBytes(frame.Objects[0].Bitmap)) {
			t.Fatal("model scale changed pixel radius", time, err)
		}
	}
}

func TestMixedPointTimelineIndependentPixels(t *testing.T) {
	for _, projection := range []three.Projection{three.Perspective, three.Orthographic} {
		for _, ssaa := range []int{1, 2, 4} {
			for _, filter := range []three.TextureFilter{three.Nearest, three.Bilinear} {
				t.Run(fmt.Sprintf("projection%d/ssaa%d/filter%d", projection, ssaa, filter), func(t *testing.T) {
					c, cameras, meshes := basicFixture(t)
					c.Renderer.Supersampling = ssaa
					c.Renderer.Background = color.NRGBA{R: 17, G: 31, B: 73, A: 107}
					cameras[0].Camera.Projection = projection
					cameras[0].Camera.OrthoHeight = 4.7
					lastCamera := cameras[0]
					lastCamera.Time = 2
					lastCamera.Camera.Position, lastCamera.Camera.Target = geom.Vec3{X: .8, Y: .3, Z: 4.7}, geom.Vec3{X: .1, Y: -.1}
					lastCamera.Camera.FOVY, lastCamera.Camera.OrthoHeight = 1.1, 5.2
					cameras = append(cameras, lastCamera)
					src := image.NewNRGBA(image.Rect(0, 0, 2, 2))
					for i, color := range []color.NRGBA{{R: 255, A: 255}, {G: 255, A: 131}, {B: 255}, {R: 173, G: 127, B: 231, A: 255}} {
						src.SetNRGBA(i%2, i/2, color)
					}
					texture, err := three.NewTexture(src)
					if err != nil {
						t.Fatal(err)
					}
					meshes[0].Material.Texture = texture
					meshes[0].Material.Sampler.Filter = filter
					// Avoid an exact texel-boundary hit, whose float tie can differ
					// between independent ray and raster evaluation equations.
					meshes[0].Mesh.UVs = []three.UV{{U: -.27, V: 1.17}, {U: 1.41, V: .17}, {U: .23, V: -.31}}
					cross := MeshTrack{ID: 8, Mesh: &three.Mesh{Vertices: []geom.Vec3{{X: -1.7, Y: -1.4, Z: -.9}, {X: 1.7, Y: -1.2, Z: .8}, {X: .2, Y: 1.6, Z: -.4}}, Triangles: []three.Triangle{{0, 1, 2}}}, Material: three.Material{Color: color.NRGBA{R: 21, G: 179, B: 109, A: 119}}}
					back := MeshTrack{ID: 9, Mesh: &three.Mesh{Vertices: []geom.Vec3{{X: -3, Y: -2.6, Z: -1.7}, {X: 3.2, Y: -2.6, Z: -1.5}, {Y: 2.9, Z: -1.9}}, Triangles: []three.Triangle{{0, 1, 2}}}, Material: three.Material{Color: color.NRGBA{R: 71, G: 93, B: 183, A: 255}}}
					meshes = append(meshes, cross, back)
					pa := []geom.Vec3{{X: -1, Y: -.4, Z: .6}, {X: .3, Y: .2, Z: -.4}, {X: -.3, Y: -.1, Z: .7}, {X: 1.2, Y: .4, Z: -.7}, {X: .7, Y: -.6, Z: 1.2}}
					pb := []geom.Vec3{{X: -.7, Y: -.2, Z: -.5}, {X: .1, Y: .4, Z: .3}, {X: .4, Y: -.2, Z: -.3}, {X: .8, Y: .1, Z: -1.9}, {X: .2, Y: -.4, Z: .8}}
					ca := []color.NRGBA{{R: 247, G: 197, A: 255}, {G: 241, B: 137, A: 128}, {R: 253, B: 113, A: 73}, {B: 231, A: 255}, {B: 255}}
					cb := []color.NRGBA{{G: 191, B: 229, A: 255}, {R: 191, B: 151, A: 219}, {R: 211, G: 137, A: 37}, {R: 91, G: 173, A: 255}, {R: 255, A: 255}}
					ra, rb := []float64{2.3, 4.1, 5.2, 3.7, 3}, []float64{4.7, 2.9, 3.1, 2.4, 5}
					a, b := mustCloud(t, pa, ca, ra), mustCloud(t, pb, cb, rb)
					start, end := IdentityPose(), IdentityPose()
					start.Scale, end.Scale = geom.Vec3{X: -1, Y: 1.1, Z: .9}, geom.Vec3{X: -.7, Y: .8, Z: 1.2}
					start.Position, end.Position = geom.Vec3{X: -.2, Y: .1}, geom.Vec3{X: .3, Z: -.1}
					start.Angle, end.Angle = -.2, 1.3
					axis := geom.Vec3{X: 1, Y: 2, Z: 3}
					points := []PointTrack{{ID: 11, Axis: axis, Keys: []PoseKey{{Pose: start}, {Time: 2, Pose: end}}, PointKeys: []PointKey{{Time: .25, Cloud: a}, {Time: 1.75, Cloud: b}}}}
					s := mustNewPoints(t, c, cameras, meshes, points)
					for _, time := range []float64{0, .25, .625, 1, 1.375, 1.75, 2} {
						v, u := time/2, math.Max(0, math.Min(1, (time-.25)/1.5))
						pose := Pose{Position: start.Position.Mul(1 - v).Add(end.Position.Mul(v)), Scale: start.Scale.Mul(1 - v).Add(end.Scale.Mul(v)), Angle: start.Angle*(1-v) + end.Angle*v}
						camera := cameras[0].Camera
						camera.Position = camera.Position.Mul(1 - v).Add(lastCamera.Camera.Position.Mul(v))
						camera.Target = camera.Target.Mul(1 - v).Add(lastCamera.Camera.Target.Mul(v))
						camera.FOVY = camera.FOVY*(1-v) + lastCamera.Camera.FOVY*v
						camera.OrthoHeight = camera.OrthoHeight*(1-v) + lastCamera.Camera.OrthoHeight*v
						refPoints := make([]referencePoint, len(pa))
						for i := range pa {
							position := referenceTransform(pa[i].Mul(1-u).Add(pb[i].Mul(u)), axis, pose)
							refPoints[i] = referencePoint{position, referencePointColor(ca[i], cb[i], u), ra[i]*(1-u) + rb[i]*u}
						}
						want := referenceMixedPixels(c, camera, refPoints, referenceInstance{meshes[0].Mesh, geom.Vec3{Z: 1}, IdentityPose(), meshes[0].Material, src}, referenceInstance{cross.Mesh, geom.Vec3{Z: 1}, IdentityPose(), cross.Material, nil}, referenceInstance{back.Mesh, geom.Vec3{Z: 1}, IdentityPose(), back.Material, nil})
						frame, err := s.Sample(time)
						if err != nil {
							t.Fatal(err)
						}
						for i, got := range bitmapBytes(frame.Objects[0].Bitmap) {
							if d := int(got) - int(want.Pix[i]); d < -1 || d > 1 {
								t.Fatalf("t%g channel%d got%d want%d", time, i, got, want.Pix[i])
							}
						}
					}
				})
			}
		}
	}
}

func TestPointClippingVisibilityAndDepth(t *testing.T) {
	c, cameras, meshes := basicFixture(t)
	cameras[0].Camera.Position, cameras[0].Camera.Target = geom.Vec3{}, geom.Vec3{Z: 1}
	cameras[0].Camera.Near, cameras[0].Camera.Far = .125, 8
	cameras[0].Camera.OrthoHeight = 4
	red := color.NRGBA{R: 241, G: 67, A: 255}
	for _, projection := range []three.Projection{three.Perspective, three.Orthographic} {
		cameras[0].Camera.Projection = projection
		for _, z := range []float64{math.Nextafter(.125, 0), .125, 8, math.Nextafter(8, 9)} {
			cloud := mustCloud(t, []geom.Vec3{{Z: z}}, []color.NRGBA{red}, nil)
			s := mustNewPoints(t, c, cameras, nil, []PointTrack{{ID: 11, Cloud: cloud}})
			frame, err := s.Sample(0)
			if err != nil {
				t.Fatal(err)
			}
			want := color.RGBA{}
			if z >= .125 && z <= 8 {
				want = color.RGBA{R: red.R, G: red.G, A: 255}
			}
			if got := frame.Objects[0].Bitmap.RGBAAt(20, 15); got != want {
				t.Fatal("near/far center clipping", projection, z, got, want)
			}
		}
		// Offscreen centers retain the visible portion of their screen disk.
		span := 2.0
		if projection == three.Perspective {
			span = 2 * math.Tan(cameras[0].Camera.FOVY/2)
		}
		cloud := mustCloud(t, []geom.Vec3{{X: span * (41.0 / 31) * 1.05, Z: 2}}, []color.NRGBA{red}, []float64{5})
		s := mustNewPoints(t, c, cameras, nil, []PointTrack{{ID: 11, Cloud: cloud, Visible: []Visibility{{Add: .5, Remove: 1}, {Add: 1.5}}}})
		for _, time := range []float64{math.Nextafter(.5, 0), .5, math.Nextafter(1, 0), 1, 1.5, 2} {
			frame, err := s.Sample(time)
			if err != nil {
				t.Fatal(err)
			}
			_, visible, err := s.PoseAt(11, time)
			want := time >= .5 && time < 1 || time >= 1.5
			if err != nil || visible != want || (frame.Objects[0].Bitmap.RGBAAt(0, 15).A != 0) != want {
				t.Fatal("offscreen disk or point visibility boundary", projection, time, visible, err)
			}
		}
	}
	// A real Z translation crosses the opaque mesh. Screen XY stays identical.
	c, cameras, meshes = basicFixture(t)
	cloud := mustCloud(t, []geom.Vec3{{}}, []color.NRGBA{{G: 255, A: 255}}, []float64{3})
	a, b := IdentityPose(), IdentityPose()
	a.Position.Z, b.Position.Z = -1, 1
	s := mustNewPoints(t, c, cameras, meshes, []PointTrack{{ID: 11, Cloud: cloud, Keys: []PoseKey{{Pose: a}, {Time: 2, Pose: b}}}})
	for _, time := range []float64{0, 1, 2} {
		frame, err := s.Sample(time)
		if err != nil {
			t.Fatal(err)
		}
		want := color.RGBA{R: 231, G: 83, B: 47, A: 255}
		if time > 1 {
			want = color.RGBA{G: 255, A: 255}
		}
		if got := frame.Objects[0].Bitmap.RGBAAt(20, 15); got != want {
			t.Fatal("world depth or exact mesh/point tie", time, got, want)
		}
	}
}
