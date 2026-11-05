package scenes

import (
	"gomath"
	"gomath/geom"
	"gomath/rasterimage"
	"gomath/three"
	"image/color"
	"math"
)

func Shadows() (*gomath.Scene, error) {
	s := gomath.NewScene()
	heading, err := title("Directional shadows / world depth / opaque meshes")
	if err != nil {
		return nil, err
	}
	plane := &three.Mesh{Vertices: []geom.Vec3{{X: -3.4, Y: -2.7}, {X: 3.4, Y: -2.7}, {X: 3.4, Y: 2.7}, {X: -3.4, Y: 2.7}}, Triangles: []three.Triangle{{0, 1, 2}, {0, 2, 3}}}
	box, err := three.Box(1.1, .9, 1.4)
	if err != nil {
		return nil, err
	}
	sphere, err := three.Sphere(.65, 24, 12)
	if err != nil {
		return nil, err
	}
	config := three.DefaultConfig(480, 270)
	config.Supersampling = 2
	config.Light = three.Light{Direction: geom.Vec3{X: .65, Y: -.3, Z: 1}, Ambient: .23, Diffuse: .77}
	config.Shadow = three.ShadowConfig{Enabled: true, Center: geom.Vec3{Z: .5}, Size: 10, Depth: 12, Resolution: 512, Bias: .05}
	renderer, err := three.NewRenderer(config)
	if err != nil {
		return nil, err
	}
	draw := func(_ *gomath.Frame, t float64) (*gomath.Object, error) {
		ground, cube, ball := three.NewInstance(plane), three.NewInstance(box), three.NewInstance(sphere)
		ground.Material.Color = color.NRGBA{R: 195, G: 215, B: 235, A: 255}
		cube.Material.Color = color.NRGBA{R: 235, G: 145, B: 65, A: 255}
		ball.Material.Color = color.NRGBA{R: 70, G: 175, B: 225, A: 255}
		cube.Model = geom.Translation(geom.Vec3{X: .6, Y: .3, Z: .7}).Mul(geom.RotationZ(t * .7))
		ball.Model = geom.Translation(geom.Vec3{X: -1.35 + .25*math.Sin(t), Y: .2, Z: .75 + .3*math.Sin(t*.8)})
		camera := three.Camera{Position: geom.Vec3{X: 4, Y: -6, Z: 5}, Target: geom.Vec3{Z: .4}, Up: geom.Vec3{Z: 1}, Projection: three.Perspective, FOVY: .7, Near: .1, Far: 20}
		pixels, err := renderer.Render(camera, ground, cube, ball)
		if err != nil {
			return nil, err
		}
		object, err := rasterimage.New(pixels, 11.8)
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
