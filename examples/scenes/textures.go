package scenes

import (
	"gomath"
	"gomath/geom"
	"gomath/rasterimage"
	"gomath/three"
	"image"
	"image/color"
	"math"
)

// Textures combines perspective-correct UVs, partially transparent texels,
// intersecting transparent meshes and opaque occlusion in the same depth pass.
func Textures() (*gomath.Scene, error) {
	s := gomath.NewScene()
	label, err := title("UV textures / perspective / transparent depth layers")
	if err != nil {
		return nil, err
	}
	texels := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for y := range 64 {
		for x := range 64 {
			c := color.NRGBA{R: 245, G: 190, B: 70, A: 255}
			if (x/8+y/8)%2 == 0 {
				c = color.NRGBA{R: 50, G: 150, B: 245, A: 160}
			}
			texels.SetNRGBA(x, y, c)
		}
	}
	texture, err := three.NewTexture(texels)
	if err != nil {
		return nil, err
	}
	box, err := three.Box(2, 2, 2)
	if err != nil {
		return nil, err
	}
	for range 6 {
		box.UVs = append(box.UVs, three.UV{U: 0, V: 1}, three.UV{U: 1, V: 1}, three.UV{U: 1, V: 0}, three.UV{U: 0, V: 0})
	}
	sphere, err := three.Sphere(1.35, 24, 12)
	if err != nil {
		return nil, err
	}
	config := three.DefaultConfig(480, 270)
	config.Supersampling = 2
	r, err := three.NewRenderer(config)
	if err != nil {
		return nil, err
	}
	draw := func(_ *gomath.Frame, t float64) (*gomath.Object, error) {
		camera := three.Camera{Position: geom.Vec3{X: 4, Y: 2.6, Z: 7}, Up: geom.Vec3{Y: 1}, Projection: three.Perspective, FOVY: math.Pi / 3, Near: .1, Far: 100}
		a, b, c := three.NewInstance(box), three.NewInstance(sphere), three.NewInstance(sphere)
		a.Model = geom.Translation(geom.V(-1.5, 0)).Mul(geom.Rotation(geom.Vec3{X: .3, Y: 1}, t*.7))
		a.Material.Texture = texture
		a.Material.Sampler = three.Sampler{Filter: three.Bilinear, AddressU: three.Repeat, AddressV: three.Clamp}
		b.Model = geom.Translation(geom.V(1.15, 0))
		c.Model = geom.Translation(geom.Vec3{X: 2.3, Y: .25, Z: -.7})
		b.Material.Color = color.NRGBA{R: 245, G: 90, B: 155, A: 130}
		c.Material.Color = color.NRGBA{R: 80, G: 220, B: 170, A: 180}
		pixels, err := r.Render(camera, c, a, b)
		if err != nil {
			return nil, err
		}
		o, err := rasterimage.New(pixels, 11.8)
		if err != nil {
			return nil, err
		}
		return o.MoveTo(geom.V(0, -.4)), nil
	}
	object, err := draw(nil, 0)
	if err != nil {
		return nil, err
	}
	if err = s.Add(object, label); err != nil {
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
