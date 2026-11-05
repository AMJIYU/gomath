package scenes

import (
	"gomath"
	"gomath/geom"
	"gomath/render"
	"gomath/scene3d"
	"gomath/three"
	"image"
	"image/color"
	"math"
)

// MeshMorph combines actual vertex/normal/UV keys with material and TRS keys.
func MeshMorph() (render.FrameSource, error) {
	const steps = 28
	makeMesh := func(amplitude, uvShift float64) (*three.Mesh, error) {
		mesh, err := three.ParametricSurface(-1.8, 1.8, -1.8, 1.8, steps, steps, func(x, y float64) geom.Vec3 {
			return geom.Vec3{X: x, Y: y, Z: amplitude * math.Sin(1.5*x) * math.Cos(1.5*y)}
		})
		if err != nil {
			return nil, err
		}
		for _, vertex := range mesh.Vertices {
			mesh.UVs = append(mesh.UVs, three.UV{U: (vertex.X+1.8)/3.6 + uvShift, V: (vertex.Y + 1.8) / 3.6})
		}
		return mesh, nil
	}
	first, err := makeMesh(0, 0)
	if err != nil {
		return nil, err
	}
	middle, err := makeMesh(.85, .15)
	if err != nil {
		return nil, err
	}
	last, err := makeMesh(-.85, .3)
	if err != nil {
		return nil, err
	}
	pixels := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for y := range 64 {
		for x := range 64 {
			c := color.NRGBA{R: 235, G: 190, B: 100, A: 255}
			if (x/8+y/8)%2 == 0 {
				c = color.NRGBA{R: 60, G: 145, B: 220, A: 255}
			}
			pixels.SetNRGBA(x, y, c)
		}
	}
	texture, err := three.NewTexture(pixels)
	if err != nil {
		return nil, err
	}
	material := three.DefaultMaterial()
	material.Texture = texture
	material.Sampler = three.Sampler{Filter: three.Bilinear, AddressU: three.Repeat, AddressV: three.Repeat}
	end := scene3d.IdentityPose()
	end.Angle = .65
	config := scene3d.DefaultConfig(480, 270, 5)
	config.FrameWidth = render.DefaultConfig().FrameWidth
	config.Renderer.Supersampling = 2
	camera := three.Camera{Position: geom.Vec3{X: 4.5, Y: -6, Z: 4.5}, Target: geom.Vec3{}, Up: geom.Vec3{Z: 1}, Projection: three.Perspective, FOVY: .65, Near: .1, Far: 30}
	source, err := scene3d.New(config, []scene3d.CameraKey{{Camera: camera}}, []scene3d.MeshTrack{{
		ID: 1, Material: material,
		MeshKeys:     []scene3d.MeshKey{{Mesh: first}, {Time: 2.5, Mesh: middle}, {Time: 5, Mesh: last}},
		MaterialKeys: []scene3d.MaterialKey{{Color: color.NRGBA{R: 255, G: 255, B: 255, A: 255}}, {Time: 2.5, Color: color.NRGBA{R: 170, G: 235, B: 255, A: 220}}, {Time: 5, Color: color.NRGBA{R: 255, G: 210, B: 230, A: 255}}},
		Keys:         []scene3d.PoseKey{{Pose: scene3d.IdentityPose()}, {Time: 5, Pose: end}},
	}})
	if err != nil {
		return nil, err
	}
	heading, err := title("3D mesh keys / vertices and normals / UV and material / TRS")
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
