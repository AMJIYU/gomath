package scenes

import (
	"gomath"
	"gomath/geom"
	"gomath/rasterimage"
	"gomath/three"
	"image/color"
	"math"
)

func PointLight() (*gomath.Scene, error) {
	s := gomath.NewScene()
	heading, err := title("World point light / distance falloff / moving source")
	if err != nil {
		return nil, err
	}
	plane := &three.Mesh{Vertices: []geom.Vec3{{X: -3, Y: -2.5}, {X: 3, Y: -2.5}, {X: 3, Y: 2.5}, {X: -3, Y: 2.5}}, Triangles: []three.Triangle{{0, 1, 2}, {0, 2, 3}}}
	sphere, err := three.Sphere(.8, 32, 16)
	if err != nil {
		return nil, err
	}
	box, err := three.Box(1.2, 1.2, 1.4)
	if err != nil {
		return nil, err
	}
	draw := func(_ *gomath.Frame, time float64) (*gomath.Object, error) {
		position := geom.Vec3{X: 2 * math.Cos(time*1.4), Y: 1.5 * math.Sin(time*1.4), Z: 2.5}
		config := three.DefaultConfig(480, 270)
		config.Supersampling = 2
		config.Light = three.Light{Kind: three.PointLight, Position: position, Ambient: .12, Diffuse: .88, Attenuation: .06}
		renderer, err := three.NewRenderer(config)
		if err != nil {
			return nil, err
		}
		ground, ball, cube := three.NewInstance(plane), three.NewInstance(sphere), three.NewInstance(box)
		ground.Material.Color = color.NRGBA{R: 180, G: 200, B: 225, A: 255}
		ball.Material.Color = color.NRGBA{R: 55, G: 175, B: 235, A: 255}
		cube.Material.Color = color.NRGBA{R: 240, G: 145, B: 70, A: 255}
		ball.Model = geom.Translation(geom.Vec3{X: -1.2, Z: .8})
		cube.Model = geom.Translation(geom.Vec3{X: 1.1, Z: .7}).Mul(geom.RotationZ(.4))
		marker, err := three.NewPointCloud([]geom.Vec3{position}, []color.NRGBA{{R: 255, G: 255, B: 180, A: 255}}, []float64{4})
		if err != nil {
			return nil, err
		}
		camera := three.Camera{Position: geom.Vec3{X: 4, Y: -6, Z: 4.5}, Target: geom.Vec3{Z: .65}, Up: geom.Vec3{Z: 1}, Projection: three.Perspective, FOVY: .75, Near: .1, Far: 20}
		pixels, err := renderer.RenderWithPoints(camera, []*three.PointCloud{marker}, ground, ball, cube)
		if err != nil {
			return nil, err
		}
		object, err := rasterimage.New(pixels, 11.6)
		if err != nil {
			return nil, err
		}
		return object.MoveTo(geom.V(0, -.4)), nil
	}
	object, err := draw(nil, 0)
	if err != nil {
		return nil, err
	}
	if err = s.Add(object, heading); err != nil {
		return nil, err
	}
	if err = s.AddRedraw(object, draw); err != nil {
		return nil, err
	}
	if err = s.Wait(4); err != nil {
		return nil, err
	}
	return s, nil
}
