package three

import (
	"encoding/json"
	"gomath/geom"
	"math"
	"os"
	"testing"
)

func TestOrientedCameraSciPy(t *testing.T) {
	var fixture struct {
		Cases []struct {
			Q      [4]float64
			Eye    [3]float64
			Matrix [9]float64
			Points []struct{ World, View [3]float64 }
		}
	}
	data, err := os.ReadFile("../tests/data/three/camera_orientation_scipy.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if len(fixture.Cases) != 18 {
		t.Fatal("incomplete camera fixture")
	}
	for _, v := range fixture.Cases {
		for _, projection := range []Projection{Perspective, Orthographic} {
			c := Camera{Mode: OrientedCamera, Position: geom.Vec3{X: v.Eye[0], Y: v.Eye[1], Z: v.Eye[2]}, Orientation: geom.Quat{W: v.Q[0], X: v.Q[1], Y: v.Q[2], Z: v.Q[3]}, Projection: projection, Near: .1, Far: 20, FOVY: .9, OrthoHeight: 4.7}
			f, err := c.frame(197.0 / 113)
			if err != nil {
				t.Fatal(err)
			}
			for i, a := range []geom.Vec3{f.right, f.up, f.forward} {
				sign := 1.0
				if i == 2 {
					sign = -1
				}
				want := geom.Vec3{X: sign * v.Matrix[i], Y: sign * v.Matrix[3+i], Z: sign * v.Matrix[6+i]}
				if a.Sub(want).Norm() > 3e-14 {
					t.Fatal("camera basis", a, want)
				}
			}
			for _, p := range v.Points {
				world := geom.Vec3{X: p.World[0], Y: p.World[1], Z: p.World[2]}
				got, err := c.WorldToView(world)
				if err != nil || got.Sub(geom.Vec3{X: p.View[0], Y: p.View[1], Z: p.View[2]}).Norm() > 3e-13 {
					t.Fatal("view", got, p.View, err)
				}
				projected, err := c.Project(world, 197, 113)
				if projection == Perspective && p.View[2] <= 0 {
					if err == nil {
						t.Fatal("behind-eye perspective accepted")
					}
					continue
				}
				if err != nil {
					t.Fatal(err)
				}
				span := c.OrthoHeight / 2
				if projection == Perspective {
					span = math.Tan(c.FOVY/2) * p.View[2]
				}
				if math.Abs(projected.X-(197.0/2+p.View[0]/span*113/2)) > 1e-10 || math.Abs(projected.Y-(113.0/2-p.View[1]/span*113/2)) > 1e-10 || math.Abs(projected.Depth-p.View[2]) > 3e-13 {
					t.Fatal("projection", projected, p.View)
				}
				// Inclusive planes are tested with exact identity below. Floating
				// rotations of boundary points can differ by an ulp independently.
				if math.Abs(p.View[2]-.1) > 1e-12 && math.Abs(p.View[2]-20) > 1e-12 {
					inside := p.View[2] >= .1 && p.View[2] <= 20 && projected.X >= 0 && projected.X <= 197 && projected.Y >= 0 && projected.Y <= 113
					if projected.Inside != inside {
						t.Fatal("clip flag", projected)
					}
				}
			}
		}
	}
}

func TestOrientedCameraValidationAndLargeEye(t *testing.T) {
	base := Camera{Mode: OrientedCamera, Orientation: geom.IdentityQuat(), Near: 1, Far: 10, FOVY: math.Pi / 2, OrthoHeight: 4}
	for _, projection := range []Projection{Perspective, Orthographic} {
		c := base
		c.Projection = projection
		for _, z := range []float64{-1, -10} {
			p, err := c.Project(geom.Vec3{Z: z}, 100, 100)
			if err != nil || !p.Inside {
				t.Fatal(p, err)
			}
		}
	}
	for _, edit := range []func(*Camera){
		func(c *Camera) { c.Orientation = geom.Quat{} }, func(c *Camera) { c.Orientation.X = math.NaN() }, func(c *Camera) { c.Orientation.W = math.Inf(1) },
		func(c *Camera) { c.Target = geom.Vec3{Z: -1} }, func(c *Camera) { c.Up = geom.Vec3{Y: 1} }, func(c *Camera) { c.Mode = CameraMode(8) },
		func(c *Camera) { c.Mode = LookAtCamera; c.Target = geom.Vec3{Z: -1}; c.Up = geom.Vec3{Y: 1} },
		func(c *Camera) { c.Near = 0 }, func(c *Camera) { c.Far = c.Near }, func(c *Camera) { c.FOVY = math.NaN() }, func(c *Camera) { c.Position.X = MaxCoordinate + 1 },
	} {
		c := base
		edit(&c)
		if c.Validate() == nil {
			t.Fatal("invalid mode/pose accepted", c)
		}
	}
	// No synthetic eye+forward target: a tiny forward component at a large
	// bounded eye must survive rather than quantizing away through subtraction.
	base.Position = geom.Vec3{X: MaxCoordinate, Y: MaxCoordinate, Z: MaxCoordinate}
	base.Orientation = geom.Quat{W: 1, Y: 1e-8}
	f, err := base.frame(1)
	if err != nil || math.Abs(f.forward.X+2e-8) > 1e-20 {
		t.Fatal(f, err)
	}
	base.Orientation = geom.Quat{X: math.SmallestNonzeroFloat64}
	if base.Validate() != nil {
		t.Fatal("subnormal orientation rejected")
	}
	base.Orientation = geom.Quat{W: 1e308, X: 1e308, Y: -1e308, Z: 1e308}
	if base.Validate() != nil {
		t.Fatal("large finite orientation rejected")
	}
}
