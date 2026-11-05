package scenes

import (
	"gomath"
	"gomath/geom"
	"gomath/render"
	"gomath/scene3d"
	"gomath/text"
	"gomath/three"
	"image/color"
	"math"
)

func CheckerboardSurface() (render.FrameSource, error) {
	groups, err := three.CheckerboardSurface(-2, 2, -2, 2, 16, 16, func(u, v float64) (geom.Vec3, error) {
		return geom.Vec3{X: u, Y: v, Z: .55 * math.Sin(u) * math.Cos(v)}, nil
	})
	if err != nil {
		return nil, err
	}
	var tracks []scene3d.MeshTrack
	for i, mesh := range groups {
		if mesh == nil {
			continue
		}
		start, end := scene3d.IdentityPose(), scene3d.IdentityPose()
		start.Scale, end.Scale = geom.Vec3{X: 1.3, Y: 1.3, Z: 1.3}, geom.Vec3{X: 1.3, Y: 1.3, Z: 1.3}
		start.Angle, end.Angle = -.25, .8
		material := three.DefaultMaterial()
		material.Color = []color.NRGBA{{R: 88, G: 166, B: 255, A: 255}, {R: 239, G: 181, B: 70, A: 255}}[i]
		tracks = append(tracks, scene3d.MeshTrack{ID: uint64(i + 1), Axis: geom.Vec3{Z: 1}, Mesh: mesh, Material: material, Keys: []scene3d.PoseKey{{Pose: start}, {Time: 5, Pose: end}}})
	}
	camera := three.Camera{Position: geom.Vec3{X: 3, Y: -9, Z: 7}, Up: geom.Vec3{Z: 1}, Projection: three.Orthographic, OrthoHeight: 8, Near: .1, Far: 30}
	config := scene3d.DefaultConfig(480, 270, 5)
	config.FrameWidth = render.DefaultConfig().FrameWidth
	config.Renderer.Supersampling = 2
	config.Renderer.Light = three.Light{Direction: geom.Vec3{X: -1, Y: -2, Z: 3}, Ambient: .3, Diffuse: .7}
	source, err := scene3d.New(config, []scene3d.CameraKey{{Camera: camera}}, tracks)
	if err != nil {
		return nil, err
	}
	overlay := gomath.NewScene()
	heading, err := title("Parametric surface / two actual mesh partitions / shared grid normals")
	if err != nil {
		return nil, err
	}
	heading.Scale(.82)
	options := text.DefaultOptions()
	options.Size = .3
	caption, err := text.New("512 real triangles / material parity by grid cell / no per-frame sampling", text.DefaultFont(), options)
	if err != nil {
		return nil, err
	}
	caption.MoveTo(geom.V(0, -2.6))
	if err = overlay.Add(heading, caption); err != nil {
		return nil, err
	}
	if err = overlay.Wait(5); err != nil {
		return nil, err
	}
	return &sourceOverlay{source: source, overlay: overlay}, nil
}
