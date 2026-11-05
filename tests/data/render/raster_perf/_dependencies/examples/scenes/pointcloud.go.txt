package scenes

import (
	"gomath"
	"gomath/geom"
	"gomath/rasterimage"
	"gomath/three"
	"image/color"
	"math"
)

// PointCloud renders actual point disks together with an opaque mesh using one
// depth buffer. The immutable cloud is reused across all camera sample times.
func PointCloud() (*gomath.Scene, error) {
	s := gomath.NewScene()
	heading, err := title("Point cloud / shared mesh depth / camera orbit")
	if err != nil {
		return nil, err
	}
	const count = 2400
	positions := make([]geom.Vec3, count)
	colors := make([]color.NRGBA, count)
	radii := make([]float64, count)
	for i := range count {
		y := 1 - 2*(float64(i)+.5)/count
		r := math.Sqrt(1 - y*y)
		angle := float64(i) * math.Pi * (3 - math.Sqrt(5))
		positions[i] = geom.Vec3{X: 2.2 * r * math.Cos(angle), Y: 2.2 * y, Z: 2.2 * r * math.Sin(angle)}
		colors[i] = color.NRGBA{R: uint8(70 + 170*(y+1)/2), G: uint8(120 + 80*(1-y*y)), B: 225, A: 190}
		radii[i] = 1.3
	}
	cloud, err := three.NewPointCloud(positions, colors, radii)
	if err != nil {
		return nil, err
	}
	box, err := three.Box(1.9, 1.9, 1.9)
	if err != nil {
		return nil, err
	}
	config := three.DefaultConfig(480, 270)
	config.Supersampling = 2
	renderer, err := three.NewRenderer(config)
	if err != nil {
		return nil, err
	}
	draw := func(_ *gomath.Frame, t float64) (*gomath.Object, error) {
		angle := t * .6
		camera := three.Camera{Position: geom.Vec3{X: 7 * math.Sin(angle), Y: 2.5, Z: 7 * math.Cos(angle)}, Up: geom.Vec3{Y: 1}, Projection: three.Perspective, FOVY: math.Pi / 3, Near: .1, Far: 100}
		instance := three.NewInstance(box)
		instance.Material.Color = color.NRGBA{R: 235, G: 150, B: 65, A: 255}
		pixels, err := renderer.RenderWithPoints(camera, []*three.PointCloud{cloud}, instance)
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
