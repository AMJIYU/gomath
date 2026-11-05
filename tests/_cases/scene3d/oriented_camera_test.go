package scene3d

import (
	"encoding/json"
	"fmt"
	"gomath/geom"
	"gomath/three"
	"image"
	"image/color"
	"math"
	"os"
	"testing"
)

type cameraOrientationFixture struct {
	Keys []struct {
		Time      float64
		Eye       [3]float64
		Q         [4]float64
		FOV, Span float64
	}
	Samples []struct {
		Time      float64
		Eye       [3]float64
		Matrix    [9]float64
		FOV, Span float64
	}
}

func loadCameraOrientation(t testing.TB) (cameraOrientationFixture, OrientedCameraTrack) {
	t.Helper()
	data, err := os.ReadFile("../tests/data/scene3d/camera_orientation_scipy.json")
	if err != nil {
		t.Fatal(err)
	}
	var f cameraOrientationFixture
	if err = json.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	if len(f.Keys) != 3 || len(f.Samples) != 35 {
		t.Fatal("incomplete camera reference")
	}
	track := OrientedCameraTrack{Projection: three.Perspective, Near: .1, Far: 20}
	for _, k := range f.Keys {
		track.Keys = append(track.Keys, OrientedCameraKey{Time: k.Time, Position: geom.Vec3{X: k.Eye[0], Y: k.Eye[1], Z: k.Eye[2]}, Orientation: geom.Quat{W: k.Q[0], X: k.Q[1], Y: k.Q[2], Z: k.Q[3]}, FOVY: k.FOV, OrthoHeight: k.Span})
	}
	return f, track
}

func mustOriented(t testing.TB, c Config, camera OrientedCameraTrack, meshes []MeshTrack, points []PointTrack) *Scene {
	t.Helper()
	s, err := NewWithOrientedCamera(c, camera, meshes, points)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// Reference camera rays are constructed from independently exported SciPy
// columns. No Go quaternion/frame/projection/timeline helper is used.
func referenceOrientedCamera(eye [3]float64, r [9]float64, projection three.Projection, near, far, fov, span float64) three.Camera {
	p := geom.Vec3{X: eye[0], Y: eye[1], Z: eye[2]}
	return three.Camera{Position: p, Target: p.Add(geom.Vec3{X: -r[2], Y: -r[5], Z: -r[8]}), Up: geom.Vec3{X: r[1], Y: r[4], Z: r[7]}, Projection: projection, Near: near, Far: far, FOVY: fov, OrthoHeight: span}
}

func TestOrientedCameraDenseMixedRayPixels(t *testing.T) {
	f, cameraTrack := loadCameraOrientation(t)
	objects, objectKeys := loadOrientation(t)
	for _, projection := range []three.Projection{three.Perspective, three.Orthographic} {
		for _, ssaa := range []int{1, 2, 4} {
			for _, filter := range []three.TextureFilter{three.Nearest, three.Bilinear} {
				t.Run(fmt.Sprintf("projection%d/ssaa%d/filter%d", projection, ssaa, filter), func(t *testing.T) {
					c, _, meshes, a, b := meshKeyFixture(t)
					c.Renderer.Supersampling = ssaa
					c.Renderer.Background = color.NRGBA{R: 17, G: 31, B: 73, A: 107}
					cameraTrack.Projection = projection
					meshes[0].RotationKeys = objectKeys
					p0, p1 := IdentityPose(), IdentityPose()
					p0.Scale, p1.Scale = geom.Vec3{X: -.8, Y: 1.1, Z: .9}, geom.Vec3{X: -1.1, Y: .8, Z: 1.3}
					p1.Position = geom.Vec3{X: .27, Y: -.11, Z: .13}
					meshes[0].Keys = []PoseKey{{Pose: p0}, {Time: 2, Pose: p1}}
					ca, cb := color.NRGBA{R: 231, G: 93, B: 57, A: 237}, color.NRGBA{R: 71, G: 213, B: 173, A: 97}
					meshes[0].MaterialKeys = []MaterialKey{{Color: ca}, {Time: 2, Color: cb}}
					src := image.NewNRGBA(image.Rect(0, 0, 2, 2))
					for i, col := range []color.NRGBA{{R: 255, A: 255}, {G: 255, A: 131}, {B: 255}, {R: 173, G: 127, B: 231, A: 255}} {
						src.SetNRGBA(i%2, i/2, col)
					}
					tex, err := three.NewTexture(src)
					if err != nil {
						t.Fatal(err)
					}
					meshes[0].Material.Texture = tex
					meshes[0].Material.Sampler = three.Sampler{Filter: filter, AddressU: three.Repeat}
					cross := MeshTrack{ID: 8, Mesh: &three.Mesh{Vertices: []geom.Vec3{{X: -1.7, Y: -1.4, Z: -.9}, {X: 1.7, Y: -1.2, Z: .8}, {X: .2, Y: 1.6, Z: -.4}}, Triangles: []three.Triangle{{0, 1, 2}}}, Material: three.Material{Color: color.NRGBA{R: 21, G: 179, B: 109, A: 119}}}
					back := MeshTrack{ID: 9, Mesh: &three.Mesh{Vertices: []geom.Vec3{{X: -3, Y: -2.6, Z: -1.7}, {X: 3.2, Y: -2.6, Z: -1.5}, {Y: 2.9, Z: -1.9}}, Triangles: []three.Triangle{{0, 1, 2}}}, Material: three.Material{Color: color.NRGBA{R: 71, G: 93, B: 183, A: 255}}}
					meshes = append(meshes, cross, back)
					positions := []geom.Vec3{{X: .2, Y: .1, Z: .5}, {X: -.5, Z: -.1}}
					colors := []color.NRGBA{{R: 231, B: 173, A: 128}, {G: 211, A: 255}}
					radii := []float64{4, 2.6}
					points := []PointTrack{{ID: 11, Cloud: mustCloud(t, positions, colors, radii), RotationKeys: objectKeys}}
					s := mustOriented(t, c, cameraTrack, meshes, points)
					for i, sample := range f.Samples {
						time := sample.Time
						u := time / 2
						camera, err := s.CameraAt(time)
						if err != nil {
							t.Fatal(err)
						}
						m, err := camera.Orientation.Matrix()
						if err != nil {
							t.Fatal(err)
						}
						for row := range 3 {
							for col := range 3 {
								if math.Abs(m[row*4+col]-sample.Matrix[row*3+col]) > 5e-14 {
									t.Fatal("SciPy camera rotation", time, m, sample.Matrix)
								}
							}
						}
						if camera.Position.Sub(geom.Vec3{X: sample.Eye[0], Y: sample.Eye[1], Z: sample.Eye[2]}).Norm() > 2e-14 || math.Abs(camera.FOVY-sample.FOV) > 2e-14 || math.Abs(camera.OrthoHeight-sample.Span) > 2e-14 {
							t.Fatal("eye/lens interpolation", camera, sample)
						}
						local := referenceMeshAttributes(a, b, math.Max(0, math.Min(1, (time-.25)/1.5)))
						pose := Pose{Position: p0.Position.Mul(1 - u).Add(p1.Position.Mul(u)), Scale: p0.Scale.Mul(1 - u).Add(p1.Scale.Mul(u))}
						world := &three.Mesh{Vertices: make([]geom.Vec3, len(local.Vertices)), UVs: local.UVs, Triangles: local.Triangles}
						for j, p := range local.Vertices {
							world.Vertices[j] = referenceOrientation(p, objects.Samples[i].Matrix, pose)
						}
						var rp []referencePoint
						for j, p := range positions {
							rp = append(rp, referencePoint{referenceOrientation(p, objects.Samples[i].Matrix, IdentityPose()), colors[j], radii[j]})
						}
						refCamera := referenceOrientedCamera(sample.Eye, sample.Matrix, projection, .1, 20, sample.FOV, sample.Span)
						material := meshes[0].Material
						material.Color = referencePointColor(ca, cb, u)
						want := referenceMixedPixels(c, refCamera, rp, referenceInstance{world, geom.Vec3{Z: 1}, IdentityPose(), material, src}, referenceInstance{cross.Mesh, geom.Vec3{Z: 1}, IdentityPose(), cross.Material, nil}, referenceInstance{back.Mesh, geom.Vec3{Z: 1}, IdentityPose(), back.Material, nil})
						frame, err := s.Sample(time)
						if err != nil {
							t.Fatal(err)
						}
						for j, got := range bitmapBytes(frame.Objects[0].Bitmap) {
							if d := int(got) - int(want.Pix[j]); d < -1 || d > 1 {
								t.Fatalf("t%g channel%d got%d want%d", time, j, got, want.Pix[j])
							}
						}
					}
				})
			}
		}
	}
}

func TestOrientedCameraNearFarRayPixels(t *testing.T) {
	f, track := loadCameraOrientation(t)
	for _, projection := range []three.Projection{three.Perspective, three.Orthographic} {
		for _, ssaa := range []int{1, 2, 4} {
			c, _, meshes := basicFixture(t)
			c.Renderer.Supersampling = ssaa
			track.Projection = projection
			track.Near = .5
			track.Far = 5
			meshes[0].Mesh = &three.Mesh{Vertices: []geom.Vec3{{X: -1.913, Y: -1.227, Z: 5.123}, {X: 1.719, Y: -1.219, Z: 3.037}, {X: .113, Y: 1.617, Z: -2.213}}, Triangles: []three.Triangle{{0, 1, 2}}}
			s := mustOriented(t, c, track, meshes, nil)
			for i := 0; i < len(f.Samples); i += 4 {
				sample := f.Samples[i]
				camera := referenceOrientedCamera(sample.Eye, sample.Matrix, projection, track.Near, track.Far, sample.FOV, sample.Span)
				want := referencePixels(c, camera, referenceInstance{meshes[0].Mesh, geom.Vec3{Z: 1}, IdentityPose(), meshes[0].Material, nil})
				frame, err := s.Sample(sample.Time)
				if err != nil {
					t.Fatal(err)
				}
				for j, got := range bitmapBytes(frame.Objects[0].Bitmap) {
					if d := int(got) - int(want.Pix[j]); d < -1 || d > 1 {
						t.Fatalf("proj%d ssaa%d t%g channel%d got%d want%d", projection, ssaa, sample.Time, j, got, want.Pix[j])
					}
				}
			}
		}
	}
}

// The light physics is independently tested in three. This comparison uses
// the existing look-at renderer with SciPy camera columns as an integration
// oracle for the point-light/world-varying and directional-shadow paths.
func TestOrientedCameraLightingIntegration(t *testing.T) {
	f, track := loadCameraOrientation(t)
	for _, mode := range []string{"directional", "point", "shadow"} {
		t.Run(mode, func(t *testing.T) {
			c, _, meshes, a, b := meshKeyFixture(t)
			c.Renderer.Supersampling = 2
			c.Renderer.Light = three.Light{Direction: geom.Vec3{X: .5, Y: 1, Z: 3}, Ambient: .15, Diffuse: .85}
			if mode == "point" {
				c.Renderer.Light.Kind = three.PointLight
				c.Renderer.Light.Position = geom.Vec3{X: .5, Y: 2, Z: 3}
				c.Renderer.Light.Attenuation = .025
			}
			if mode == "shadow" {
				c.Renderer.Shadow = three.ShadowConfig{Enabled: true, Size: 10, Depth: 20, Resolution: 64, Bias: .025}
			}
			pose := IdentityPose()
			pose.Scale = geom.Vec3{X: -.9, Y: .7, Z: 1.1}
			meshes[0].Keys = []PoseKey{{Pose: pose}}
			cloud := mustCloud(t, []geom.Vec3{{X: .3, Y: -.2, Z: .6}}, []color.NRGBA{{R: 177, G: 231, B: 71, A: 137}}, []float64{2.3})
			s := mustOriented(t, c, track, meshes, []PointTrack{{ID: 11, Cloud: cloud}})
			r, err := three.NewRenderer(c.Renderer)
			if err != nil {
				t.Fatal(err)
			}
			for _, sample := range f.Samples {
				view := referenceOrientedCamera(sample.Eye, sample.Matrix, track.Projection, track.Near, track.Far, sample.FOV, sample.Span)
				mesh := referenceMeshAttributes(a, b, math.Max(0, math.Min(1, (sample.Time-.25)/1.5)))
				want, err := r.RenderWithPoints(view, []*three.PointCloud{cloud}, three.Instance{Mesh: mesh, Material: meshes[0].Material, Model: referenceMatrix(geom.Vec3{Z: 1}, pose)})
				if err != nil {
					t.Fatal(err)
				}
				frame, err := s.Sample(sample.Time)
				if err != nil {
					t.Fatal(err)
				}
				for j, got := range bitmapBytes(frame.Objects[0].Bitmap) {
					if d := int(got) - int(want.Pix[j]); d < -1 || d > 1 {
						t.Fatalf("t%g channel%d got%d want%d", sample.Time, j, got, want.Pix[j])
					}
				}
			}
		})
	}
}
