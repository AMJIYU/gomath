package scene3d

import (
	"fmt"
	"gomath/geom"
	"gomath/three"
	"image"
	"image/color"
	"math"
	"reflect"
	"testing"
)

func meshKeyFixture(t testing.TB) (Config, []CameraKey, []MeshTrack, *three.Mesh, *three.Mesh) {
	t.Helper()
	c, cameras, tracks := basicFixture(t)
	a := tracks[0].Mesh
	a.Normals = []geom.Vec3{{X: .1, Z: 2}, {Y: .2, Z: 1}, {X: -.2, Z: 1.5}}
	a.UVs = []three.UV{{U: -.27, V: 1.17}, {U: 1.41, V: .17}, {U: .23, V: -.31}}
	b := &three.Mesh{
		Vertices: []geom.Vec3{{X: -1.15, Y: -.9, Z: .4}, {X: 1.4, Y: -.5, Z: -.35}, {X: -.15, Y: 1.15, Z: .2}},
		Normals:  []geom.Vec3{{X: -.2, Y: .1, Z: 1}, {X: .3, Z: 1.7}, {Y: -.2, Z: .9}},
		UVs:      []three.UV{{U: -.13, V: .91}, {U: 1.23, V: -.07}, {U: .41, V: -.17}}, Triangles: []three.Triangle{{0, 1, 2}},
	}
	tracks[0].Mesh = nil
	tracks[0].MeshKeys = []MeshKey{{Time: .25, Mesh: a}, {Time: 1.75, Mesh: b}}
	return c, cameras, tracks, a, b
}

// Independent attribute reference: no production meshAt, clone, lerp or normal
// helper. Original endpoint values are kept; interior normals use explicit
// weighted components and Euclidean normalization before the ray/renderer check.
func referenceMeshAttributes(a, b *three.Mesh, u float64) *three.Mesh {
	if u <= 0 {
		return a
	}
	if u >= 1 {
		return b
	}
	out := &three.Mesh{Vertices: make([]geom.Vec3, len(a.Vertices)), Triangles: a.Triangles}
	for i := range out.Vertices {
		out.Vertices[i] = a.Vertices[i].Mul(1 - u).Add(b.Vertices[i].Mul(u))
	}
	if len(a.Normals) != 0 {
		out.Normals = make([]geom.Vec3, len(a.Normals))
		for i := range out.Normals {
			n := a.Normals[i].Mul(1 - u).Add(b.Normals[i].Mul(u))
			out.Normals[i] = n.Mul(1 / math.Sqrt(n.Dot(n)))
		}
	}
	if len(a.UVs) != 0 {
		out.UVs = make([]three.UV, len(a.UVs))
		for i := range out.UVs {
			out.UVs[i] = three.UV{U: a.UVs[i].U*(1-u) + b.UVs[i].U*u, V: a.UVs[i].V*(1-u) + b.UVs[i].V*u}
		}
	}
	return out
}

func TestMeshKeysAttributesAndMaterialEndpoints(t *testing.T) {
	c, cameras, tracks, a, b := meshKeyFixture(t)
	src := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	src.SetNRGBA(0, 0, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	texture, err := three.NewTexture(src)
	if err != nil {
		t.Fatal(err)
	}
	tracks[0].Material.Texture = texture
	tracks[0].Material.Sampler = three.Sampler{Filter: three.Bilinear, AddressU: three.Repeat}
	ca, cb := color.NRGBA{B: 255}, color.NRGBA{R: 255, A: 255}
	tracks[0].MaterialKeys = []MaterialKey{{Time: .25, Color: ca}, {Time: 1.75, Color: cb}}
	s := mustNew(t, c, cameras, tracks)
	for _, time := range []float64{0, .25, .625, 1, 1.375, 1.75, 2} {
		u := math.Max(0, math.Min(1, (time-.25)/1.5))
		mesh, err := s.tracks[0].meshAt(time)
		if err != nil {
			t.Fatal(err)
		}
		want := referenceMeshAttributes(a, b, u)
		if u == 0 || u == 1 {
			if !reflect.DeepEqual(mesh, want) {
				t.Fatal("exact mesh endpoint changed authored values", time)
			}
		}
		for i := range mesh.Vertices {
			if mesh.Vertices[i].Sub(want.Vertices[i]).Norm() > 2e-14 || mesh.Normals[i].Sub(want.Normals[i]).Norm() > 2e-14 || math.Abs(mesh.UVs[i].U-want.UVs[i].U) > 2e-14 || math.Abs(mesh.UVs[i].V-want.UVs[i].V) > 2e-14 {
				t.Fatal("intermediate mesh attributes differ", time, i)
			}
			if u > 0 && u < 1 && math.Abs(mesh.Normals[i].Norm()-1) > 2e-14 {
				t.Fatal("interpolated normal not normalized")
			}
		}
		mat := s.tracks[0].materialAt(time)
		if mat.Texture != texture || mat.Sampler != tracks[0].Material.Sampler || mat.Color != referencePointColor(ca, cb, u) || (u > 0 && mat.Color.B != 0) {
			t.Fatal("material key changed texture/sampler or leaked hidden RGB", time, mat)
		}
	}
	// A material key replaces base Color, it does not multiply it. The one-pixel
	// opaque white texture still multiplies that animated color in three.
	c.Renderer.Width, c.Renderer.Height = 1, 1
	tracks[0].MeshKeys, tracks[0].Mesh = nil, a
	tracks[0].Material.Color = color.NRGBA{G: 255, A: 17}
	s = mustNew(t, c, cameras, tracks)
	frame, err := s.Sample(1)
	if err != nil || frame.Objects[0].Bitmap.RGBAAt(0, 0) != (color.RGBA{R: 128, A: 128}) {
		t.Fatal("material animation multiplied base Color or lost premultiplied alpha", err)
	}
	// No MaterialKeys remains exactly the original material.
	tracks[0].MaterialKeys = nil
	if got := tracks[0].materialAt(1); got != tracks[0].Material {
		t.Fatal("default material path changed")
	}
}

func TestDeformingMeshIndependentPixels(t *testing.T) {
	for _, projection := range []three.Projection{three.Perspective, three.Orthographic} {
		for _, ssaa := range []int{1, 2, 4} {
			for _, filter := range []three.TextureFilter{three.Nearest, three.Bilinear} {
				t.Run(fmt.Sprintf("projection%d/ssaa%d/filter%d", projection, ssaa, filter), func(t *testing.T) {
					c, cameras, tracks, a, b := meshKeyFixture(t)
					c.Renderer.Supersampling = ssaa
					c.Renderer.Background = color.NRGBA{R: 17, G: 31, B: 73, A: 107}
					cameras[0].Camera.Projection, cameras[0].Camera.OrthoHeight = projection, 4.7
					endCam := cameras[0]
					endCam.Time = 2
					endCam.Camera.Position, endCam.Camera.Target = geom.Vec3{X: .8, Y: .3, Z: 4.7}, geom.Vec3{X: .1, Y: -.1}
					endCam.Camera.FOVY, endCam.Camera.OrthoHeight = 1.1, 5.2
					cameras = append(cameras, endCam)
					start, end := IdentityPose(), IdentityPose()
					start.Scale, end.Scale = geom.Vec3{X: -.8, Y: 1.1, Z: .9}, geom.Vec3{X: -1.1, Y: .8, Z: 1.3}
					start.Position, end.Position = geom.Vec3{X: -.2, Y: .1}, geom.Vec3{X: .3, Z: -.1}
					start.Angle, end.Angle = -.4, 2.7
					tracks[0].Axis = geom.Vec3{X: 1, Y: 2, Z: 3}
					tracks[0].Keys = []PoseKey{{Pose: start}, {Time: 2, Pose: end}}
					colors := []color.NRGBA{{B: 255}, {R: 231, G: 93, B: 57, A: 255}, {R: 71, G: 213, B: 173, A: 97}}
					tracks[0].MaterialKeys = []MaterialKey{{Time: .25, Color: colors[0]}, {Time: 1, Color: colors[1]}, {Time: 1.75, Color: colors[2]}}
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
					p := []geom.Vec3{{X: .2, Y: .1, Z: .5}, {X: -.5, Z: -.1}}
					pc := []color.NRGBA{{R: 231, B: 173, A: 128}, {G: 211, A: 255}}
					pr := []float64{4, 2.6}
					cloud := mustCloud(t, p, pc, pr)
					s := mustNewPoints(t, c, cameras, tracks, []PointTrack{{ID: 11, Cloud: cloud}})
					refPoints := []referencePoint{{p[0], pc[0], pr[0]}, {p[1], pc[1], pr[1]}}
					for _, time := range []float64{0, .25, .625, 1, 1.375, 1.75, 2} {
						v, u := time/2, math.Max(0, math.Min(1, (time-.25)/1.5))
						mesh := referenceMeshAttributes(a, b, u)
						pose := Pose{Position: start.Position.Mul(1 - v).Add(end.Position.Mul(v)), Scale: start.Scale.Mul(1 - v).Add(end.Scale.Mul(v)), Angle: start.Angle*(1-v) + end.Angle*v}
						camera := cameras[0].Camera
						camera.Position, camera.Target = camera.Position.Mul(1-v).Add(endCam.Camera.Position.Mul(v)), camera.Target.Mul(1-v).Add(endCam.Camera.Target.Mul(v))
						camera.FOVY, camera.OrthoHeight = camera.FOVY*(1-v)+endCam.Camera.FOVY*v, camera.OrthoHeight*(1-v)+endCam.Camera.OrthoHeight*v
						mat := tracks[0].Material
						if time <= 1 {
							mat.Color = referencePointColor(colors[0], colors[1], math.Max(0, (time-.25)/.75))
						} else {
							mat.Color = referencePointColor(colors[1], colors[2], math.Min(1, (time-1)/.75))
						}
						want := referenceMixedPixels(c, camera, refPoints, referenceInstance{mesh, tracks[0].Axis, pose, mat, src}, referenceInstance{cross.Mesh, geom.Vec3{Z: 1}, IdentityPose(), cross.Material, nil}, referenceInstance{back.Mesh, geom.Vec3{Z: 1}, IdentityPose(), back.Material, nil})
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

func TestDeformingNormalsLighting(t *testing.T) {
	for _, kind := range []three.LightKind{three.DirectionalLight, three.PointLight} {
		c, cameras, tracks, a, b := meshKeyFixture(t)
		c.Renderer.Supersampling = 2
		c.Renderer.Light = three.Light{Kind: kind, Direction: geom.Vec3{X: 1, Y: 2, Z: 3}, Position: geom.Vec3{X: -.5, Y: .7, Z: 2}, Ambient: .13, Diffuse: .87, Attenuation: .03}
		pose := IdentityPose()
		pose.Position, pose.Scale, pose.Angle = geom.Vec3{X: .13, Z: -.2}, geom.Vec3{X: -1.2, Y: .7, Z: 1.4}, .53
		tracks[0].Axis, tracks[0].Keys = geom.Vec3{X: 1, Y: 2, Z: 3}, []PoseKey{{Pose: pose}}
		s := mustNew(t, c, cameras, tracks)
		r, err := three.NewRenderer(c.Renderer)
		if err != nil {
			t.Fatal(err)
		}
		for _, time := range []float64{0, .625, 1, 1.375, 2} {
			u := math.Max(0, math.Min(1, (time-.25)/1.5))
			mesh := referenceMeshAttributes(a, b, u)
			want, err := r.Render(cameras[0].Camera, three.Instance{Mesh: mesh, Material: tracks[0].Material, Model: referenceMatrix(tracks[0].Axis, pose)})
			if err != nil {
				t.Fatal(err)
			}
			frame, err := s.Sample(time)
			if err != nil {
				t.Fatal(err)
			}
			for i, got := range bitmapBytes(frame.Objects[0].Bitmap) {
				if d := int(got) - int(want.Pix[i]); d < -1 || d > 1 {
					t.Fatal("animated normals/light did not use the deformed mesh", kind, time, i, got, want.Pix[i])
				}
			}
		}
	}
}

func TestMeshKeysNoPoseOrBitmapSubstitution(t *testing.T) {
	c, cameras, tracks := basicFixture(t)
	a, b := tracks[0].Mesh, &three.Mesh{Vertices: []geom.Vec3{{X: -1, Y: -.8, Z: 1}, {X: 1.2, Y: -.7, Z: 1}, {X: .1, Y: 1, Z: 1}}, Triangles: []three.Triangle{{0, 1, 2}}}
	b.UVs = a.UVs
	tracks[0].Mesh, tracks[0].MeshKeys = nil, []MeshKey{{Mesh: a}, {Time: 2, Mesh: b}}
	cloud := mustCloud(t, []geom.Vec3{{Z: .5}}, []color.NRGBA{{G: 255, A: 255}}, []float64{3})
	s := mustNewPoints(t, c, cameras, tracks, []PointTrack{{ID: 11, Cloud: cloud}})
	first, _ := s.Sample(0)
	last, _ := s.Sample(2)
	if first.Objects[0].Bitmap.RGBAAt(20, 15) != (color.RGBA{G: 255, A: 255}) || last.Objects[0].Bitmap.RGBAAt(20, 15) != (color.RGBA{R: 231, G: 83, B: 47, A: 255}) {
		t.Fatal("geometry animation failed to cross actual point depth")
	}
	if pose, _, _ := s.PoseAt(7, 1); pose != IdentityPose() {
		t.Fatal("geometry keys were secretly implemented with a model pose")
	}
	if first.Objects[0].ID != s.BitmapID() || last.Objects[0].ID != s.BitmapID() || !reflect.DeepEqual(first.Objects[0].Path, last.Objects[0].Path) || !reflect.DeepEqual(first.Camera, last.Camera) {
		t.Fatal("geometry animation changed the bitmap plane or its identity")
	}
}

func TestMeshKeyNearFarClippingReference(t *testing.T) {
	for _, projection := range []three.Projection{three.Perspective, three.Orthographic} {
		for _, ssaa := range []int{1, 2, 4} {
			c, cameras, tracks := basicFixture(t)
			c.Renderer.Width, c.Renderer.Height, c.Renderer.Supersampling = 37, 29, ssaa
			camera := three.Camera{Position: geom.Vec3{}, Target: geom.Vec3{Z: 1}, Up: geom.Vec3{Y: 1}, Projection: projection, Near: .25, Far: 4, FOVY: .9, OrthoHeight: 2}
			cameras = []CameraKey{{Camera: camera}}
			// Irregular coordinates keep ray boundary inclusion distinct from
			// raster top-left ties while still crossing both clipping planes.
			a := &three.Mesh{Vertices: []geom.Vec3{{X: -.313, Y: -.227, Z: .123}, {X: .719, Y: -.219, Z: 1.037}, {X: .113, Y: .617, Z: 1.213}}, Triangles: []three.Triangle{{0, 1, 2}}}
			b := &three.Mesh{Vertices: []geom.Vec3{{X: -.419, Y: -.317, Z: .817}, {X: .613, Y: -.313, Z: 5.037}, {X: .217, Y: .719, Z: 1.019}}, Triangles: []three.Triangle{{0, 1, 2}}}
			tracks[0].Mesh, tracks[0].MeshKeys = nil, []MeshKey{{Mesh: a}, {Time: 2, Mesh: b}}
			s := mustNew(t, c, cameras, tracks)
			for _, time := range []float64{0, .5, 1, 1.5, 2} {
				mesh := referenceMeshAttributes(a, b, time/2)
				want := referencePixels(c, camera, referenceInstance{mesh, geom.Vec3{Z: 1}, IdentityPose(), tracks[0].Material, nil})
				frame, err := s.Sample(time)
				if err != nil {
					t.Fatal(err)
				}
				for i, got := range bitmapBytes(frame.Objects[0].Bitmap) {
					if d := int(got) - int(want.Pix[i]); d < -1 || d > 1 {
						t.Fatal("deforming near/far clip", projection, ssaa, time, i, got, want.Pix[i])
					}
				}
			}
		}
	}
}
