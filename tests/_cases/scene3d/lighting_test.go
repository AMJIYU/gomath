package scene3d

import (
	"fmt"
	"gomath/geom"
	"gomath/three"
	"image/color"
	"testing"
)

// Lighting itself has independent analytic references in three's tests. Here
// an independent Rodrigues/basis model checks that the immutable timeline
// passes actual camera/pose/normal data to the requested renderer configuration.
func TestRendererLightingIntegration(t *testing.T) {
	for _, mode := range []string{"directional", "point", "shadow"} {
		t.Run(mode, func(t *testing.T) {
			c, cameras, tracks := basicFixture(t)
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
			tracks[0].Mesh.Normals = []geom.Vec3{{X: .2, Z: 1}, {Y: .3, Z: 1}, {X: -.2, Z: 1}}
			a, z := IdentityPose(), IdentityPose()
			a.Scale, z.Scale = geom.Vec3{X: -1, Y: .9, Z: 1.2}, geom.Vec3{X: -1.2, Y: 1.1, Z: .9}
			z.Position, z.Angle = geom.Vec3{X: .2, Y: .1, Z: -.1}, .6
			tracks[0].Keys = []PoseKey{{Pose: a}, {Time: 2, Pose: z}}
			tracks[0].Axis = geom.Vec3{X: .3, Y: 1, Z: .5}
			last := cameras[0]
			last.Time, last.Camera.Position.X, last.Camera.Target.Y = 2, 1, .1
			cameras = append(cameras, last)
			r, err := three.NewRenderer(c.Renderer)
			if err != nil {
				t.Fatal(err)
			}
			s := mustNew(t, c, cameras, tracks)
			for _, time := range []float64{0, .7, 2} {
				u := time / 2
				pose := Pose{Position: a.Position.Mul(1 - u).Add(z.Position.Mul(u)), Scale: a.Scale.Mul(1 - u).Add(z.Scale.Mul(u)), Angle: z.Angle * u}
				view := cameras[0].Camera
				view.Position = view.Position.Mul(1 - u).Add(last.Camera.Position.Mul(u))
				view.Target = view.Target.Mul(1 - u).Add(last.Camera.Target.Mul(u))
				want, err := r.Render(view, three.Instance{Mesh: tracks[0].Mesh, Material: tracks[0].Material, Model: referenceMatrix(tracks[0].Axis, pose)})
				if err != nil {
					t.Fatal(err)
				}
				frame, err := s.Sample(time)
				if err != nil {
					t.Fatal(err)
				}
				got := bitmapBytes(frame.Objects[0].Bitmap)
				nonzero := false
				for i, x := range want.Pix {
					if d := int(x) - int(got[i]); d < -1 || d > 1 {
						t.Fatal(fmt.Sprintf("t%g channel%d got%d want%d", time, i, got[i], x))
					}
					nonzero = nonzero || x != 0
				}
				if !nonzero || frame.Objects[0].Bitmap.RGBAAt(20, 15) == (color.RGBA{}) {
					t.Fatal("lit triangle missing")
				}
			}
		})
	}
}
