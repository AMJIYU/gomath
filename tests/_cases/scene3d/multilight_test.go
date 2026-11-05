package scene3d

import (
	"bytes"
	"gomath/geom"
	"gomath/three"
	"image/color"
	"math"
	"strings"
	"sync"
	"testing"
)

func timelineMultiLights() []three.PointLightSource {
	return []three.PointLightSource{
		{Position: geom.Vec3{X: 2, Y: -1, Z: 3}, RGB: [3]float64{1, .13, .03}, Diffuse: .83, Attenuation: .07},
		{Position: geom.Vec3{X: -1, Y: 2, Z: 2}, RGB: [3]float64{.03, .7, 1}, Diffuse: .71, Attenuation: .09},
	}
}

func TestMultiLightTimelineOwnershipConcurrentOrder(t *testing.T) {
	for _, oriented := range []bool{false, true} {
		c, cameras, tracks := basicFixture(t)
		c.Renderer.Light = three.Light{Kind: three.PointLightList, Ambient: .13}
		c.Renderer.PointLights = timelineMultiLights()
		c.Renderer.Supersampling = 2
		first, last := IdentityPose(), IdentityPose()
		first.Scale = geom.Vec3{X: -1, Y: .8, Z: 1.2}
		last.Scale = geom.Vec3{X: -1.2, Y: 1.1, Z: .9}
		last.Position, last.Angle = geom.Vec3{X: .2, Y: .1, Z: -.1}, .6
		tracks[0].Axis = geom.Vec3{X: .3, Y: 1, Z: .5}
		tracks[0].Keys = []PoseKey{{Pose: first}, {Time: 2, Pose: last}}
		tracks[0].Mesh.Normals = []geom.Vec3{{X: .2, Z: 1}, {Y: .3, Z: 1}, {X: -.2, Z: 1}}
		tracks[0].Material.Color.A = 173
		p := IdentityPose()
		p.Position.X = .2
		points := []PointTrack{{ID: 11, Cloud: mustCloud(t, []geom.Vec3{{Z: .3}}, []color.NRGBA{{G: 193, B: 71, A: 137}}, []float64{3}), Keys: []PoseKey{{Pose: IdentityPose()}, {Time: 2, Pose: p}}}}
		end := cameras[0]
		end.Time = 2
		end.Camera.Position.X = .5
		cameras = append(cameras, end)
		var s *Scene
		if oriented {
			q, err := geom.QuatFromAxisAngle(geom.Vec3{Z: 1}, .2)
			if err != nil {
				t.Fatal(err)
			}
			s = mustOriented(t, c, OrientedCameraTrack{Projection: three.Perspective, Near: .1, Far: 20, Keys: []OrientedCameraKey{{Position: geom.Vec3{Z: 5}, Orientation: geom.Quat{W: 1}, FOVY: .9, OrthoHeight: 4}, {Time: 2, Position: geom.Vec3{X: .5, Z: 5}, Orientation: q, FOVY: 1.1, OrthoHeight: 4}}}, tracks, points)
		} else {
			s = mustNewPoints(t, c, cameras, tracks, points)
		}
		r, err := three.NewRenderer(c.Renderer)
		if err != nil {
			t.Fatal(err)
		}
		times := []float64{0, .25, .75, 1.5, 2}
		wants := make([][]byte, len(times))
		for i, time := range times {
			camera, err := s.CameraAt(time)
			if err != nil {
				t.Fatal(err)
			}
			u := time / 2
			pose := Pose{Position: first.Position.Mul(1 - u).Add(last.Position.Mul(u)), Scale: first.Scale.Mul(1 - u).Add(last.Scale.Mul(u)), Angle: .6 * u}
			cloud := mustCloud(t, []geom.Vec3{{X: .2 * u, Z: .3}}, []color.NRGBA{{G: 193, B: 71, A: 137}}, []float64{3})
			want, err := r.RenderWithPoints(camera, []*three.PointCloud{cloud}, three.Instance{Mesh: tracks[0].Mesh, Material: tracks[0].Material, Model: referenceMatrix(tracks[0].Axis, pose)})
			if err != nil {
				t.Fatal(err)
			}
			frame, err := s.Sample(time)
			if err != nil {
				t.Fatal(err)
			}
			wants[i] = bitmapBytes(frame.Objects[0].Bitmap)
			for j, ch := range want.Pix {
				if math.Abs(float64(ch)-float64(wants[i][j])) > 1 {
					t.Fatal("multi-light TRS/point integration", time, j, ch, wants[i][j])
				}
			}
		}
		var wg sync.WaitGroup
		start := make(chan struct{})
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for range 100 {
				for i := range c.Renderer.PointLights {
					c.Renderer.PointLights[i].Position.X = math.NaN()
					c.Renderer.PointLights[i].RGB = [3]float64{math.Inf(1)}
				}
			}
		}()
		for worker := range 8 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				for j := range 12 {
					i := (worker + 3*j) % len(times)
					f, err := s.Sample(times[i])
					if err != nil || !bytes.Equal(bitmapBytes(f.Objects[0].Bitmap), wants[i]) {
						t.Errorf("retained authoring light slice oriented%t: %v", oriented, err)
						return
					}
				}
			}()
		}
		close(start)
		wg.Wait()
	}
}

func TestMultiLightTimelineValidationAndRecoverableBudget(t *testing.T) {
	c, cameras, tracks := basicFixture(t)
	c.Renderer.Light = three.Light{Kind: three.PointLightList, Ambient: .1}
	c.Renderer.PointLights = timelineMultiLights()
	for _, invalid := range []func(*three.Config){
		func(c *three.Config) { c.PointLights[0].RGB[1] = math.NaN() },
		func(c *three.Config) { c.Light.Diffuse = .5 },
		func(c *three.Config) { c.Shadow.Enabled = true },
		func(c *three.Config) { c.PointLights = make([]three.PointLightSource, three.MaxPointLights+1) },
		func(c *three.Config) { c.PointLightEvaluationLimit = three.MaxPointLightEvaluations + 1 },
	} {
		bad := c
		bad.Renderer.PointLights = timelineMultiLights()
		invalid(&bad.Renderer)
		if s, err := New(bad, cameras, tracks); s != nil || err == nil {
			t.Fatal("invalid renderer accepted by timeline")
		}
	}
	c.Renderer.PointLightEvaluationLimit = 1
	tracks[0].Visible = []Visibility{{Add: .5, Remove: 1.5}}
	s := mustNew(t, c, cameras, tracks)
	for _, time := range []float64{0, 1, 2, 1, .25} {
		frame, err := s.Sample(time)
		if time == 1 {
			if frame != nil || err == nil || !strings.Contains(err.Error(), "light evaluation budget") {
				t.Fatal("light overflow should fail entire Frame", err)
			}
		} else if frame == nil || err != nil {
			t.Fatal("budget error poisoned hidden sample", err)
		}
	}
}
