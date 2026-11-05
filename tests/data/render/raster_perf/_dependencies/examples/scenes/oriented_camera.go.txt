package scenes

import (
	"gomath"
	"gomath/geom"
	"gomath/render"
	"gomath/scene3d"
	"gomath/three"
	"image/color"
)

func OrientedCameraTimeline() (render.FrameSource, error) {
	box, err := three.Box(1.7, 1.4, 1.1)
	if err != nil {
		return nil, err
	}
	torus, err := three.Torus(.75, .24, 32, 12)
	if err != nil {
		return nil, err
	}
	cloud, err := three.NewPointCloud([]geom.Vec3{{X: -1.5, Y: .65, Z: .7}, {X: .2, Y: -1.1, Z: 1}, {X: 1.8, Y: 1.2, Z: .3}},
		[]color.NRGBA{{R: 250, G: 190, B: 60, A: 255}, {R: 100, G: 235, B: 160, A: 255}, {R: 220, G: 150, B: 240, A: 255}}, []float64{6, 6, 6})
	if err != nil {
		return nil, err
	}
	orientation := func(yaw, pitch, roll float64) (geom.Quat, error) {
		y, err := geom.QuatFromAxisAngle(geom.Vec3{Y: 1}, yaw)
		if err != nil {
			return geom.Quat{}, err
		}
		x, err := geom.QuatFromAxisAngle(geom.Vec3{X: 1}, pitch)
		if err != nil {
			return geom.Quat{}, err
		}
		z, err := geom.QuatFromAxisAngle(geom.Vec3{Z: 1}, roll)
		return y.Mul(x).Mul(z), err
	}
	middle, err := orientation(.15, -.1, .45)
	if err != nil {
		return nil, err
	}
	last, err := orientation(-.14, .07, -.3)
	if err != nil {
		return nil, err
	}
	camera := scene3d.OrientedCameraTrack{Projection: three.Perspective, Near: .1, Far: 30,
		Keys: []scene3d.OrientedCameraKey{
			{Position: geom.Vec3{Z: 7.5}, Orientation: geom.IdentityQuat(), FOVY: .72},
			{Time: 2.5, Position: geom.Vec3{X: 1.1, Y: .7, Z: 7.2}, Orientation: middle, FOVY: .64},
			{Time: 5, Position: geom.Vec3{X: -1, Y: -.45, Z: 7}, Orientation: last, FOVY: .75},
		}}
	left, right := scene3d.IdentityPose(), scene3d.IdentityPose()
	left.Position.X, left.Angle = -1.4, .3
	right.Position.X, right.Angle = 1.4, .3
	blue, pink := three.DefaultMaterial(), three.DefaultMaterial()
	blue.Color, pink.Color = color.NRGBA{R: 80, G: 185, B: 240, A: 255}, color.NRGBA{R: 230, G: 100, B: 190, A: 255}
	config := scene3d.DefaultConfig(480, 270, 5)
	config.FrameWidth = render.DefaultConfig().FrameWidth
	config.Renderer.Supersampling = 2
	source, err := scene3d.NewWithOrientedCamera(config, camera,
		[]scene3d.MeshTrack{{ID: 1, Mesh: box, Material: blue, Axis: geom.Vec3{Y: 1}, Keys: []scene3d.PoseKey{{Pose: left}}},
			{ID: 2, Mesh: torus, Material: pink, Axis: geom.Vec3{X: 1}, Keys: []scene3d.PoseKey{{Pose: right}}}},
		[]scene3d.PointTrack{{ID: 3, Cloud: cloud, Keys: []scene3d.PoseKey{{Pose: scene3d.IdentityPose()}}}})
	if err != nil {
		return nil, err
	}
	heading, err := title("Quaternion camera / roll and view direction / eye and lens keys")
	if err != nil {
		return nil, err
	}
	heading.Scale(.88)
	heading.ZIndex = 1
	overlay := gomath.NewScene()
	if err = overlay.Add(heading); err != nil {
		return nil, err
	}
	if err = overlay.Wait(5); err != nil {
		return nil, err
	}
	return &sourceOverlay{source: source, overlay: overlay}, nil
}
