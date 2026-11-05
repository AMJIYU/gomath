package three_test

import (
	"fmt"
	"gomath/geom"
	"gomath/rasterimage"
	"gomath/three"
	"image/color"
	"math"
)

func ExampleRenderer_Render() {
	mesh, err := three.Box(2, 2, 2)
	if err != nil {
		panic(err)
	}
	instance := three.NewInstance(mesh)
	instance.Material.Color = color.NRGBA{R: 80, G: 160, B: 240, A: 255}
	// At animation time t, derive the model directly from t. No accumulated
	// rotation or renderer state is required for out-of-order frame sampling.
	t := 0.75
	instance.Model = geom.Rotation(geom.Vec3{Y: 1}, t)
	camera := three.Camera{
		Position: geom.Vec3{X: 3, Y: 2, Z: 5}, Target: geom.Vec3{}, Up: geom.Vec3{Y: 1},
		Projection: three.Perspective, FOVY: math.Pi / 3, Near: .1, Far: 100,
	}
	config := three.DefaultConfig(320, 180)
	config.CullBackfaces = true
	renderer, err := three.NewRenderer(config)
	if err != nil {
		panic(err)
	}
	frame, err := renderer.Render(camera, instance)
	if err != nil {
		panic(err)
	}
	object, err := rasterimage.New(frame, 8)
	if err != nil {
		panic(err)
	}
	if err := object.ValidateVisual(); err != nil {
		panic(err)
	}
	fmt.Printf("%dx%d; bitmap present: %t\n", frame.Bounds().Dx(), frame.Bounds().Dy(), object.Bitmap != nil)
	// Output: 320x180; bitmap present: true
}

func ExampleParametricSurface() {
	mesh, err := three.ParametricSurface(-2, 2, -2, 2, 16, 16, func(u, v float64) geom.Vec3 {
		return geom.Vec3{X: u, Y: v, Z: .3 * math.Sin(u) * math.Cos(v)}
	})
	if err != nil {
		panic(err)
	}
	fmt.Printf("%d vertices, %d triangles, %d normals\n", len(mesh.Vertices), len(mesh.Triangles), len(mesh.Normals))
	// Output: 289 vertices, 512 triangles, 289 normals
}

func ExampleCamera_Project() {
	camera := three.Camera{Position: geom.Vec3{Z: 3}, Up: geom.Vec3{Y: 1}, Projection: three.Orthographic, OrthoHeight: 4, Near: .1, Far: 20}
	point, err := camera.Project(geom.Vec3{}, 160, 90)
	if err != nil {
		panic(err)
	}
	fmt.Printf("x=%.0f y=%.0f depth=%.0f visible=%t\n", point.X, point.Y, point.Depth, point.Inside)
	// Output: x=80 y=45 depth=3 visible=true
}

func ExampleConfig_supersampling() {
	config := three.DefaultConfig(1, 1)
	config.Supersampling = 4 // 4x per axis: 16 independent shaded/depth samples.
	config.Light = three.Light{Ambient: 1}
	renderer, err := three.NewRenderer(config)
	if err != nil {
		panic(err)
	}
	// An opaque white quad covers the left half of a transparent output pixel.
	mesh := &three.Mesh{
		Vertices:  []geom.Vec3{{X: -2, Y: -2, Z: -2}, {Y: -2, Z: -2}, {Y: 2, Z: -2}, {X: -2, Y: 2, Z: -2}},
		Triangles: []three.Triangle{{0, 1, 2}, {0, 2, 3}},
	}
	camera := three.Camera{
		Target: geom.Vec3{Z: -1}, Up: geom.Vec3{Y: 1},
		Projection: three.Orthographic, OrthoHeight: 2, Near: 1, Far: 10,
	}
	frame, err := renderer.Render(camera, three.NewInstance(mesh))
	if err != nil {
		panic(err)
	}
	fmt.Println(frame.RGBAAt(0, 0)) // premultiplied RGB equals coverage alpha.
	// Output: {128 128 128 128}
}
