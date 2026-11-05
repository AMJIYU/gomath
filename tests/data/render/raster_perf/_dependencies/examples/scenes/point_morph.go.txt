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

func PointMorph() (render.FrameSource, error) {
	const count = 360
	a, b := make([]geom.Vec3, count), make([]geom.Vec3, count)
	ca, cb := make([]color.NRGBA, count), make([]color.NRGBA, count)
	ra, rb := make([]float64, count), make([]float64, count)
	for i := range count {
		t := float64(i) / float64(count-1)
		z := 1 - 2*t
		r := math.Sqrt(math.Max(0, 1-z*z))
		angle := float64(i) * math.Pi * (3 - math.Sqrt(5))
		a[i] = geom.Vec3{X: 1.6 * r * math.Cos(angle), Y: 1.6 * r * math.Sin(angle), Z: 1.6 * z}
		turn := 6 * math.Pi * t
		b[i] = geom.Vec3{X: 1.6 * math.Cos(turn), Y: 1.6 * math.Sin(turn), Z: 3 * (t - .5)}
		ca[i] = color.NRGBA{R: 60, G: uint8(130 + 110*t), B: 240, A: 210}
		cb[i] = color.NRGBA{R: 245, G: uint8(90 + 130*t), B: 80, A: 210}
		ra[i], rb[i] = 2, 2.8
	}
	first, err := three.NewPointCloud(a, ca, ra)
	if err != nil {
		return nil, err
	}
	last, err := three.NewPointCloud(b, cb, rb)
	if err != nil {
		return nil, err
	}
	box, err := three.Box(.9, .9, .9)
	if err != nil {
		return nil, err
	}
	material := three.DefaultMaterial()
	material.Color = color.NRGBA{R: 70, G: 100, B: 145, A: 255}
	cfg := scene3d.DefaultConfig(480, 270, 5)
	cfg.FrameWidth = render.DefaultConfig().FrameWidth
	cfg.Renderer.Supersampling = 2
	camera := three.Camera{Position: geom.Vec3{X: 4, Y: -6, Z: 3}, Target: geom.Vec3{}, Up: geom.Vec3{Z: 1}, Projection: three.Perspective, FOVY: .7, Near: .1, Far: 30}
	end := scene3d.IdentityPose()
	end.Angle = 2 * math.Pi
	source, err := scene3d.NewWithPoints(cfg, []scene3d.CameraKey{{Camera: camera}},
		[]scene3d.MeshTrack{{ID: 1, Mesh: box, Material: material}},
		[]scene3d.PointTrack{{ID: 2, Axis: geom.Vec3{Z: 1}, PointKeys: []scene3d.PointKey{{Cloud: first}, {Time: 3.5, Cloud: last}},
			Keys: []scene3d.PoseKey{{Pose: scene3d.IdentityPose()}, {Time: 5, Pose: end}}}})
	if err != nil {
		return nil, err
	}
	heading, err := title("3D point timelines / buffer morph / color and radius / real depth")
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
