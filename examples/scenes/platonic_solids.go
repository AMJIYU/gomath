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

// PlatonicSolids renders actual flat-normal meshes in one shared depth buffer.
// Edge lengths vary to give the four different circumradii comparable framing.
func PlatonicSolids() (render.FrameSource, error) {
	constructors := []func(float64) (*three.Mesh, error){three.Tetrahedron, three.Octahedron, three.Icosahedron, three.Dodecahedron}
	edges := []float64{1.8, 1.55, 1.25, .85}
	colors := []color.NRGBA{{R: 88, G: 166, B: 255, A: 255}, {R: 247, G: 120, B: 186, A: 255}, {R: 78, G: 201, B: 153, A: 255}, {R: 239, G: 181, B: 70, A: 255}}
	var tracks []scene3d.MeshTrack
	for i, constructor := range constructors {
		mesh, err := constructor(edges[i])
		if err != nil {
			return nil, err
		}
		start, end := scene3d.IdentityPose(), scene3d.IdentityPose()
		start.Position, end.Position = geom.Vec3{X: -4.65 + 3.1*float64(i)}, geom.Vec3{X: -4.65 + 3.1*float64(i)}
		start.Angle, end.Angle = .2, 1.9
		material := three.DefaultMaterial()
		material.Color = colors[i]
		tracks = append(tracks, scene3d.MeshTrack{ID: uint64(i + 1), Axis: geom.Vec3{X: .3, Y: 1, Z: .7}, Mesh: mesh, Material: material, Keys: []scene3d.PoseKey{{Pose: start}, {Time: 5, Pose: end}}})
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
	heading, err := title("Platonic solids / checked convex meshes / flat outward normals")
	if err != nil {
		return nil, err
	}
	heading.Scale(.9)
	if err = overlay.Add(heading); err != nil {
		return nil, err
	}
	for i, name := range []string{"Tetrahedron", "Octahedron", "Icosahedron", "Dodecahedron"} {
		options := text.DefaultOptions()
		options.Size = .3
		label, err := text.New(name, text.DefaultFont(), options)
		if err != nil {
			return nil, err
		}
		label.MoveTo(geom.V(-4.65+3.1*float64(i), -2))
		if err = overlay.Add(label); err != nil {
			return nil, err
		}
	}
	if err = overlay.Wait(5); err != nil {
		return nil, err
	}
	return &sourceOverlay{source: source, overlay: overlay}, nil
}
