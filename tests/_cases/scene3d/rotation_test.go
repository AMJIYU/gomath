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

type orientationFixture struct {
	Keys []struct {
		Time float64
		Q    [4]float64
	}
	Samples []struct {
		Time   float64
		Matrix [9]float64
	}
}

func loadOrientation(t testing.TB) (orientationFixture, []RotationKey) {
	t.Helper()
	data, err := os.ReadFile("../tests/data/scene3d/orientation_scipy.json")
	if err != nil {
		t.Fatal(err)
	}
	var f orientationFixture
	if err = json.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	if len(f.Keys) != 3 || len(f.Samples) != 35 {
		t.Fatal("incomplete orientation fixture")
	}
	keys := make([]RotationKey, len(f.Keys))
	for i, k := range f.Keys {
		keys[i] = RotationKey{k.Time, geom.Quat{W: k.Q[0], X: k.Q[1], Y: k.Q[2], Z: k.Q[3]}}
	}
	return f, keys
}

// SciPy matrices are applied explicitly; no Go quaternion/matrix transform or
// timeline lookup is used to produce reference world positions.
func referenceOrientation(v geom.Vec3, r [9]float64, p Pose) geom.Vec3 {
	x, y, z := v.X*p.Scale.X, v.Y*p.Scale.Y, v.Z*p.Scale.Z
	return geom.Vec3{X: r[0]*x + r[1]*y + r[2]*z + p.Position.X, Y: r[3]*x + r[4]*y + r[5]*z + p.Position.Y, Z: r[6]*x + r[7]*y + r[8]*z + p.Position.Z}
}

func referenceOrientationModel(r [9]float64, p Pose) geom.Mat4 {
	return geom.Mat4{r[0] * p.Scale.X, r[1] * p.Scale.Y, r[2] * p.Scale.Z, p.Position.X, r[3] * p.Scale.X, r[4] * p.Scale.Y, r[5] * p.Scale.Z, p.Position.Y, r[6] * p.Scale.X, r[7] * p.Scale.Y, r[8] * p.Scale.Z, p.Position.Z, 0, 0, 0, 1}
}

func TestRotationDenseSciPyAndPointMeshPixels(t *testing.T) {
	f, keys := loadOrientation(t)
	for _, projection := range []three.Projection{three.Perspective, three.Orthographic} {
		for _, ssaa := range []int{1, 2, 4} {
			for _, filter := range []three.TextureFilter{three.Nearest, three.Bilinear} {
				t.Run(fmt.Sprintf("projection%d/ssaa%d/filter%d", projection, ssaa, filter), func(t *testing.T) {
					c, cameras, tracks, a, b := meshKeyFixture(t)
					c.Renderer.Supersampling = ssaa
					c.Renderer.Background = color.NRGBA{R: 17, G: 31, B: 73, A: 107}
					cameras[0].Camera.Projection, cameras[0].Camera.OrthoHeight = projection, 4.7
					last := cameras[0]
					last.Time = 2
					last.Camera.Position, last.Camera.Target = geom.Vec3{X: .8, Y: .3, Z: 4.7}, geom.Vec3{X: .1, Y: -.1}
					last.Camera.FOVY, last.Camera.OrthoHeight = 1.1, 5.2
					cameras = append(cameras, last)
					start, end := IdentityPose(), IdentityPose()
					start.Scale, end.Scale = geom.Vec3{X: -.8, Y: 1.1, Z: .9}, geom.Vec3{X: -1.1, Y: .8, Z: 1.3}
					start.Position, end.Position = geom.Vec3{X: -.2, Y: .1}, geom.Vec3{X: .3, Z: -.1}
					tracks[0].Keys = []PoseKey{{Pose: start}, {Time: 2, Pose: end}}
					tracks[0].RotationKeys = keys
					ca, cb := color.NRGBA{R: 231, G: 93, B: 57, A: 237}, color.NRGBA{R: 71, G: 213, B: 173, A: 97}
					tracks[0].MaterialKeys = []MaterialKey{{Color: ca}, {Time: 2, Color: cb}}
					src := image.NewNRGBA(image.Rect(0, 0, 2, 2))
					for i, col := range []color.NRGBA{{R: 255, A: 255}, {G: 255, A: 131}, {B: 255}, {R: 173, G: 127, B: 231, A: 255}} {
						src.SetNRGBA(i%2, i/2, col)
					}
					texture, err := three.NewTexture(src)
					if err != nil {
						t.Fatal(err)
					}
					tracks[0].Material.Texture = texture
					tracks[0].Material.Sampler = three.Sampler{Filter: filter, AddressU: three.Repeat}
					cross := MeshTrack{ID: 8, Mesh: &three.Mesh{Vertices: []geom.Vec3{{X: -1.7, Y: -1.4, Z: -.9}, {X: 1.7, Y: -1.2, Z: .8}, {X: .2, Y: 1.6, Z: -.4}}, Triangles: []three.Triangle{{0, 1, 2}}}, Material: three.Material{Color: color.NRGBA{R: 21, G: 179, B: 109, A: 119}}}
					back := MeshTrack{ID: 9, Mesh: &three.Mesh{Vertices: []geom.Vec3{{X: -3, Y: -2.6, Z: -1.7}, {X: 3.2, Y: -2.6, Z: -1.5}, {Y: 2.9, Z: -1.9}}, Triangles: []three.Triangle{{0, 1, 2}}}, Material: three.Material{Color: color.NRGBA{R: 71, G: 93, B: 183, A: 255}}}
					tracks = append(tracks, cross, back)
					pa, pb := []geom.Vec3{{X: .2, Y: .1, Z: .5}, {X: -.5, Z: -.1}}, []geom.Vec3{{X: .37, Y: -.21, Z: -.4}, {X: -.17, Y: .3, Z: .7}}
					pca, pcb := []color.NRGBA{{R: 231, B: 173, A: 128}, {G: 211, A: 255}}, []color.NRGBA{{G: 241, A: 255}, {R: 237, B: 179, A: 113}}
					pra, prb := []float64{4, 2.6}, []float64{2.3, 3.7}
					pk := append([]RotationKey(nil), keys...)
					for i := range pk {
						pk[i].Rotation = keys[len(keys)-1-i].Rotation
					}
					ps, pe := IdentityPose(), IdentityPose()
					ps.Scale, pe.Scale = geom.Vec3{X: .8, Y: -1, Z: 1.2}, geom.Vec3{X: 1.1, Y: -.9, Z: .8}
					pe.Position = geom.Vec3{X: .19, Y: -.13, Z: .2}
					points := []PointTrack{{ID: 11, PointKeys: []PointKey{{Cloud: mustCloud(t, pa, pca, pra)}, {Time: 2, Cloud: mustCloud(t, pb, pcb, prb)}}, Keys: []PoseKey{{Pose: ps}, {Time: 2, Pose: pe}}, RotationKeys: pk}}
					s := mustNewPoints(t, c, cameras, tracks, points)
					for sampleIndex, sample := range f.Samples {
						time := sample.Time
						u := time / 2
						pose := Pose{Position: start.Position.Mul(1 - u).Add(end.Position.Mul(u)), Scale: start.Scale.Mul(1 - u).Add(end.Scale.Mul(u))}
						q, err := s.RotationAt(7, time)
						if err != nil {
							t.Fatal(err)
						}
						qm, err := q.Matrix()
						if err != nil {
							t.Fatal(err)
						}
						for i := range 3 {
							for j := range 3 {
								if math.Abs(qm[4*i+j]-sample.Matrix[3*i+j]) > 5e-14 {
									t.Fatal("orientation mismatch", time, qm, sample.Matrix)
								}
							}
						}
						local := referenceMeshAttributes(a, b, math.Max(0, math.Min(1, (time-.25)/1.5)))
						world := &three.Mesh{Vertices: make([]geom.Vec3, len(local.Vertices)), UVs: local.UVs, Triangles: local.Triangles}
						for i, v := range local.Vertices {
							world.Vertices[i] = referenceOrientation(v, sample.Matrix, pose)
						}
						pointPose := Pose{Position: ps.Position.Mul(1 - u).Add(pe.Position.Mul(u)), Scale: ps.Scale.Mul(1 - u).Add(pe.Scale.Mul(u))}
						var refPoints []referencePoint
						for i := range pa {
							v := pa[i].Mul(1 - u).Add(pb[i].Mul(u))
							v = referenceOrientation(v, f.Samples[len(f.Samples)-1-sampleIndex].Matrix, pointPose)
							refPoints = append(refPoints, referencePoint{v, referencePointColor(pca[i], pcb[i], u), pra[i]*(1-u) + prb[i]*u})
						}
						camera := cameras[0].Camera
						camera.Position, camera.Target = camera.Position.Mul(1-u).Add(last.Camera.Position.Mul(u)), camera.Target.Mul(1-u).Add(last.Camera.Target.Mul(u))
						camera.FOVY, camera.OrthoHeight = camera.FOVY*(1-u)+last.Camera.FOVY*u, camera.OrthoHeight*(1-u)+last.Camera.OrthoHeight*u
						mat := tracks[0].Material
						mat.Color = referencePointColor(ca, cb, u)
						want := referenceMixedPixels(c, camera, refPoints, referenceInstance{world, geom.Vec3{Z: 1}, IdentityPose(), mat, src}, referenceInstance{cross.Mesh, geom.Vec3{Z: 1}, IdentityPose(), cross.Material, nil}, referenceInstance{back.Mesh, geom.Vec3{Z: 1}, IdentityPose(), back.Material, nil})
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

func TestQuaternionNormalsLightingIntegration(t *testing.T) {
	f, keys := loadOrientation(t)
	for _, mode := range []string{"directional", "point", "shadow"} {
		t.Run(mode, func(t *testing.T) {
			c, cameras, tracks, a, b := meshKeyFixture(t)
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
			start, end := IdentityPose(), IdentityPose()
			start.Scale, end.Scale = geom.Vec3{X: -1, Y: .9, Z: 1.2}, geom.Vec3{X: -1.2, Y: 1.1, Z: .9}
			end.Position = geom.Vec3{X: .2, Y: .1, Z: -.1}
			tracks[0].RotationKeys = keys
			tracks[0].Keys = []PoseKey{{Pose: start}, {Time: 2, Pose: end}}
			s := mustNew(t, c, cameras, tracks)
			r, err := three.NewRenderer(c.Renderer)
			if err != nil {
				t.Fatal(err)
			}
			for _, sample := range f.Samples {
				u := sample.Time / 2
				pose := Pose{Position: start.Position.Mul(1 - u).Add(end.Position.Mul(u)), Scale: start.Scale.Mul(1 - u).Add(end.Scale.Mul(u))}
				mesh := referenceMeshAttributes(a, b, math.Max(0, math.Min(1, (sample.Time-.25)/1.5)))
				want, err := r.Render(cameras[0].Camera, three.Instance{Mesh: mesh, Material: tracks[0].Material, Model: referenceOrientationModel(sample.Matrix, pose)})
				if err != nil {
					t.Fatal(err)
				}
				frame, err := s.Sample(sample.Time)
				if err != nil {
					t.Fatal(err)
				}
				for i, got := range bitmapBytes(frame.Objects[0].Bitmap) {
					if d := int(got) - int(want.Pix[i]); d < -1 || d > 1 {
						t.Fatalf("t%g channel%d got%d want%d", sample.Time, i, got, want.Pix[i])
					}
				}
			}
		})
	}
}
