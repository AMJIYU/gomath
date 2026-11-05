package scenes

import (
	"gomath"
	"gomath/geom"
	"gomath/render"
	"gomath/scene3d"
	"gomath/three"
	"image/color"
)

// PointLightsTimeline uses fixed world-space colored sources, moving meshes
// and a camera track. Point markers are deliberately unlit source indicators.
func PointLightsTimeline() (render.FrameSource, error) {
	plane := &three.Mesh{Vertices: []geom.Vec3{{X: -3, Y: -2.5}, {X: 3, Y: -2.5}, {X: 3, Y: 2.5}, {X: -3, Y: 2.5}}, Triangles: []three.Triangle{{0, 1, 2}, {0, 2, 3}}}
	sphere, err := three.Sphere(.8, 32, 16)
	if err != nil {
		return nil, err
	}
	box, err := three.Box(1.2, 1.2, 1.5)
	if err != nil {
		return nil, err
	}
	sources := []three.PointLightSource{
		{Position: geom.Vec3{X: -2.2, Y: -.8, Z: 2.2}, RGB: [3]float64{1, .28, .14}, Diffuse: .85, Attenuation: .08},
		{Position: geom.Vec3{X: 2.2, Y: .5, Z: 2.6}, RGB: [3]float64{.16, .5, 1}, Diffuse: .95, Attenuation: .06},
		{Position: geom.Vec3{Y: 1.8, Z: 1.5}, RGB: [3]float64{.3, 1, .4}, Diffuse: .45, Attenuation: .1},
	}
	cloud, err := three.NewPointCloud([]geom.Vec3{sources[0].Position, sources[1].Position, sources[2].Position},
		[]color.NRGBA{{R: 255, G: 95, B: 60, A: 255}, {R: 60, G: 145, B: 255, A: 255}, {R: 100, G: 255, B: 130, A: 255}}, []float64{4, 4, 4})
	if err != nil {
		return nil, err
	}
	left, middle, right := scene3d.IdentityPose(), scene3d.IdentityPose(), scene3d.IdentityPose()
	left.Position, middle.Position, right.Position = geom.Vec3{X: -1.2, Y: .6, Z: .8}, geom.Vec3{Y: .9, Z: .8}, geom.Vec3{X: 1.2, Y: .6, Z: .8}
	cubeStart, cubeEnd := scene3d.IdentityPose(), scene3d.IdentityPose()
	cubeStart.Position, cubeEnd.Position = geom.Vec3{X: 1.2, Y: -.7, Z: .75}, geom.Vec3{X: -1.2, Y: -.7, Z: .75}
	cubeStart.Angle, cubeEnd.Angle = .2, 1.3
	material := three.DefaultMaterial()
	material.Color = color.NRGBA{R: 215, G: 215, B: 215, A: 255}
	camera := three.Camera{Position: geom.Vec3{X: 4, Y: -6, Z: 4.5}, Target: geom.Vec3{Z: .7}, Up: geom.Vec3{Z: 1}, Projection: three.Perspective, FOVY: .75, Near: .1, Far: 25}
	lastCamera := camera
	lastCamera.Position = geom.Vec3{X: 5, Y: -5, Z: 4.8}
	config := scene3d.DefaultConfig(480, 270, 5)
	config.FrameWidth = render.DefaultConfig().FrameWidth
	config.Renderer.Supersampling = 2
	config.Renderer.Light = three.Light{Kind: three.PointLightList, Ambient: .12}
	config.Renderer.PointLights = sources
	source, err := scene3d.NewWithPoints(config, []scene3d.CameraKey{{Camera: camera}, {Time: 5, Camera: lastCamera}},
		[]scene3d.MeshTrack{
			{ID: 1, Mesh: plane, Material: material},
			{ID: 2, Mesh: sphere, Material: material, Keys: []scene3d.PoseKey{{Pose: left}, {Time: 2.5, Pose: middle}, {Time: 5, Pose: right}}},
			{ID: 3, Mesh: box, Material: material, Keys: []scene3d.PoseKey{{Pose: cubeStart}, {Time: 5, Pose: cubeEnd}}},
		}, []scene3d.PointTrack{{ID: 4, Cloud: cloud}})
	if err != nil {
		return nil, err
	}
	heading, err := title("Three world point lights / RGB Lambert / distance falloff")
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
