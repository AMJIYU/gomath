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

// Scene3DTimeline uses immutable mesh/camera authoring keys, with actual
// visibility boundaries; no frame callback mutates a shared mesh or camera.
func Scene3DTimeline() (render.FrameSource, error) {
	const duration = 5.0
	cfg := scene3d.DefaultConfig(480, 270, duration)
	cfg.FrameWidth = render.DefaultConfig().FrameWidth
	cfg.Renderer.Supersampling = 2
	cfg.Renderer.Light = three.Light{Direction: geom.Vec3{X: -1, Y: -2, Z: -3}, Ambient: .25, Diffuse: .75}
	box, err := three.Box(1.3, 1.1, 1.4)
	if err != nil {
		return nil, err
	}
	ball, err := three.Sphere(.75, 24, 12)
	if err != nil {
		return nil, err
	}
	ground := &three.Mesh{Vertices: []geom.Vec3{{X: -3, Y: -2}, {X: 3, Y: -2}, {X: 3, Y: 2}, {X: -3, Y: 2}}, Triangles: []three.Triangle{{0, 1, 2}, {0, 2, 3}}}
	material := func(c color.NRGBA) three.Material { m := three.DefaultMaterial(); m.Color = c; return m }
	pose := func(position, scale geom.Vec3, angle float64) scene3d.Pose {
		return scene3d.Pose{Position: position, Scale: scale, Angle: angle}
	}
	unit := geom.Vec3{X: 1, Y: 1, Z: 1}
	tracks := []scene3d.MeshTrack{
		{ID: 1, Mesh: ground, Material: material(color.NRGBA{R: 85, G: 105, B: 135, A: 255})},
		{ID: 2, Mesh: box, Material: material(color.NRGBA{R: 230, G: 140, B: 60, A: 255}), Axis: geom.Vec3{X: .25, Y: .5, Z: 1},
			Keys: []scene3d.PoseKey{{Pose: pose(geom.Vec3{X: -1.4, Z: 1}, unit, 0)},
				{Time: 2.5, Pose: pose(geom.Vec3{X: 0, Z: 1.4}, geom.Vec3{X: 1.3, Y: .8, Z: 1.1}, math.Pi)},
				{Time: 5, Pose: pose(geom.Vec3{X: 1.4, Z: 1}, unit, 2*math.Pi)}}},
		{ID: 3, Mesh: ball, Material: material(color.NRGBA{R: 65, G: 190, B: 235, A: 255}),
			Keys: []scene3d.PoseKey{{Pose: pose(geom.Vec3{X: 1.6, Y: .4, Z: .75}, unit, 0)},
				{Time: 5, Pose: pose(geom.Vec3{X: -1.5, Y: .4, Z: .75}, geom.Vec3{X: 1, Y: 1, Z: 1.4}, 0)}},
			Visible: []scene3d.Visibility{{Add: .8, Remove: 3.1}, {Add: 3.8}}},
	}
	first := three.Camera{Position: geom.Vec3{X: 5, Y: -7, Z: 5}, Target: geom.Vec3{Z: .8}, Up: geom.Vec3{Z: 1}, Projection: three.Perspective, FOVY: .75, Near: .1, Far: 30}
	middle, last := first, first
	middle.Position = geom.Vec3{X: 1, Y: -8, Z: 4.2}
	middle.Target = geom.Vec3{Z: 1}
	middle.FOVY = .65
	last.Position = geom.Vec3{X: -5, Y: -7, Z: 5}
	last.FOVY = .8
	source, err := scene3d.New(cfg, []scene3d.CameraKey{{Camera: first}, {Time: 2.5, Camera: middle}, {Time: 5, Camera: last}}, tracks)
	if err != nil {
		return nil, err
	}
	label, err := title("3D timeline / mesh poses / camera keys / add and remove")
	if err != nil {
		return nil, err
	}
	label.ZIndex = 1
	overlay := gomath.NewScene()
	if err = overlay.Add(label); err != nil {
		return nil, err
	}
	if err = overlay.Wait(duration); err != nil {
		return nil, err
	}
	return &sourceOverlay{source: source, overlay: overlay}, nil
}
