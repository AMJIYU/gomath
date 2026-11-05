package scenes

import (
	"gomath"
	"gomath/geom"
	"gomath/render"
	"gomath/scene3d"
	"gomath/text"
	"gomath/three"
	"image/color"
)

// Segments3D renders real closed endpoint meshes through one shared depth buffer.
// Moving the instance models preserves the constructor's deterministic roll.
func Segments3D() (render.FrameSource, error) {
	line, err := three.CylinderBetween(geom.Vec3{X: -.8, Y: -.45, Z: -1.2}, geom.Vec3{X: .8, Y: .45, Z: 1.2}, .14, 32, 3)
	if err != nil {
		return nil, err
	}
	options := three.ArrowOptions{ShaftRadius: .12, HeadRadius: .4, HeadLength: .7, RadialSegments: 32, ShaftSegments: 3, HeadSegments: 3}
	arbitrary, err := three.ArrowBetween(geom.Vec3{X: -.8, Y: -.45, Z: -1.2}, geom.Vec3{X: .8, Y: .45, Z: 1.2}, options)
	if err != nil {
		return nil, err
	}
	reverse, err := three.ArrowBetween(geom.Vec3{Z: 1.4}, geom.Vec3{Z: -1.4}, options)
	if err != nil {
		return nil, err
	}
	colors := []color.NRGBA{{R: 88, G: 166, B: 255, A: 255}, {R: 78, G: 201, B: 153, A: 255}, {R: 247, G: 120, B: 186, A: 255}}
	var tracks []scene3d.MeshTrack
	for i, mesh := range []*three.Mesh{line, arbitrary, reverse} {
		start, end := scene3d.IdentityPose(), scene3d.IdentityPose()
		start.Position, end.Position = geom.Vec3{X: -4.3 + 4.3*float64(i)}, geom.Vec3{X: -4.3 + 4.3*float64(i)}
		start.Angle, end.Angle = -.2, 1.1
		material := three.DefaultMaterial()
		material.Color = colors[i]
		tracks = append(tracks, scene3d.MeshTrack{ID: uint64(i + 1), Axis: geom.Vec3{Y: .4, Z: 1}, Mesh: mesh, Material: material, Keys: []scene3d.PoseKey{{Pose: start}, {Time: 5, Pose: end}}})
	}
	camera := three.Camera{Position: geom.Vec3{Y: -12, Z: 6}, Up: geom.Vec3{Z: 1}, Projection: three.Orthographic, OrthoHeight: 8, Near: .1, Far: 30}
	config := scene3d.DefaultConfig(480, 270, 5)
	config.FrameWidth = render.DefaultConfig().FrameWidth
	config.Renderer.Supersampling = 2
	config.Renderer.Light = three.Light{Direction: geom.Vec3{X: -1, Y: -2, Z: -3}, Ambient: .25, Diffuse: .75}
	source, err := scene3d.New(config, []scene3d.CameraKey{{Camera: camera}}, tracks)
	if err != nil {
		return nil, err
	}
	overlay := gomath.NewScene()
	heading, err := title("3D endpoints / closed cylinders and arrows / actual mesh geometry")
	if err != nil {
		return nil, err
	}
	heading.Scale(.88)
	if err = overlay.Add(heading); err != nil {
		return nil, err
	}
	for i, name := range []string{"cylinder / oblique axis", "arrow / shoulder annulus", "arrow / reversed Z"} {
		labelOptions := text.DefaultOptions()
		labelOptions.Size = .3
		label, err := text.New(name, text.DefaultFont(), labelOptions)
		if err != nil {
			return nil, err
		}
		label.MoveTo(geom.V(-4.3+4.3*float64(i), -2))
		if err = overlay.Add(label); err != nil {
			return nil, err
		}
	}
	if err = overlay.Wait(5); err != nil {
		return nil, err
	}
	return &sourceOverlay{source: source, overlay: overlay}, nil
}
