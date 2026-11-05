package scenes

import (
	"gomath"
	"gomath/geom"
	"gomath/render"
	"gomath/scene3d"
	"gomath/three"
	"image/color"
	"math"
)

func QuaternionTimeline() (render.FrameSource, error) {
	box, err := three.Box(2.1, 1.1, .7)
	if err != nil {
		return nil, err
	}
	cloud, err := three.NewPointCloud([]geom.Vec3{{X: -1, Y: -.5, Z: -.3}, {X: 1, Y: -.5, Z: -.3}, {X: 0, Y: 1, Z: 0}, {Z: 1.1}},
		[]color.NRGBA{{R: 250, G: 190, B: 60, A: 255}, {R: 70, G: 190, B: 240, A: 255}, {R: 230, G: 80, B: 180, A: 255}, {R: 100, G: 235, B: 160, A: 255}}, []float64{7, 7, 7, 7})
	if err != nil {
		return nil, err
	}
	middle, err := geom.QuatFromAxisAngle(geom.Vec3{X: 1, Y: 1}, math.Pi*.75)
	if err != nil {
		return nil, err
	}
	last, err := geom.QuatFromAxisAngle(geom.Vec3{Z: 1}, math.Pi)
	if err != nil {
		return nil, err
	}
	rotations := []scene3d.RotationKey{{Rotation: geom.IdentityQuat()}, {Time: 2.5, Rotation: middle}, {Time: 5, Rotation: last}}
	left, right := scene3d.IdentityPose(), scene3d.IdentityPose()
	left.Position.X = -1.65
	right.Position.X = 1.65
	material := three.DefaultMaterial()
	material.Color = color.NRGBA{R: 80, G: 185, B: 240, A: 255}
	config := scene3d.DefaultConfig(480, 270, 5)
	config.FrameWidth = render.DefaultConfig().FrameWidth
	config.Renderer.Supersampling = 2
	camera := three.Camera{Position: geom.Vec3{X: 4, Y: -8, Z: 4.5}, Target: geom.Vec3{}, Up: geom.Vec3{Z: 1}, Projection: three.Perspective, FOVY: .7, Near: .1, Far: 30}
	source, err := scene3d.NewWithPoints(config, []scene3d.CameraKey{{Camera: camera}},
		[]scene3d.MeshTrack{{ID: 1, Mesh: box, Material: material, RotationKeys: rotations, Keys: []scene3d.PoseKey{{Pose: left}}}},
		[]scene3d.PointTrack{{ID: 2, Cloud: cloud, RotationKeys: rotations, Keys: []scene3d.PoseKey{{Pose: right}}}})
	if err != nil {
		return nil, err
	}
	heading, err := title("Quaternion tracks / shortest arc SLERP / mesh and point rotation")
	if err != nil {
		return nil, err
	}
	heading.Scale(.9)
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
