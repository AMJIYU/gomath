package three

import (
	"gomath/geom"
	"math"
	"testing"
)

func testCamera(projection Projection) Camera {
	return Camera{Target: geom.Vec3{Z: -1}, Up: geom.Vec3{Y: 1}, Projection: projection, Near: 1, Far: 10, FOVY: math.Pi / 2, OrthoHeight: 4}
}

func closeVec(a, b geom.Vec3) bool { return a.Sub(b).Norm() < 1e-10 }

func TestCameraLookAtAndProjection(t *testing.T) {
	c := testCamera(Perspective)
	p, err := c.Project(geom.Vec3{X: 1, Y: 1, Z: -2}, 200, 100)
	if err != nil || math.Abs(p.X-125) > 1e-10 || math.Abs(p.Y-25) > 1e-10 || p.Depth != 2 || !p.Inside {
		t.Fatalf("perspective %+v: %v", p, err)
	}
	c.Projection = Orthographic
	p, err = c.Project(geom.Vec3{X: 1, Y: 1, Z: -2}, 200, 100)
	if err != nil || math.Abs(p.X-125) > 1e-10 || math.Abs(p.Y-25) > 1e-10 || !p.Inside {
		t.Fatalf("ortho %+v: %v", p, err)
	}
	// Looking along +x rotates world +z to camera right, while preserving up.
	c.Position = geom.Vec3{X: -3, Y: 2, Z: 4}
	c.Target = geom.Vec3{X: 0, Y: 2, Z: 4}
	v, err := c.WorldToView(geom.Vec3{X: 2, Y: 3, Z: 6})
	if err != nil || !closeVec(v, geom.Vec3{X: 2, Y: 1, Z: 5}) {
		t.Fatalf("rotated view %+v: %v", v, err)
	}
	for _, proj := range []Projection{Perspective, Orthographic} {
		c = testCamera(proj)
		for _, z := range []float64{-.5, -11} {
			p, err = c.Project(geom.Vec3{Z: z}, 100, 100)
			if err != nil || p.Inside {
				t.Fatalf("unclipped projection %+v: %v", p, err)
			}
		}
		p, err = c.Project(geom.Vec3{X: 100, Z: -2}, 100, 100)
		if err != nil || p.Inside {
			t.Fatalf("viewport projection %+v: %v", p, err)
		}
	}
}

func TestCameraValidation(t *testing.T) {
	base := testCamera(Perspective)
	cases := []Camera{base, base, base, base, base, base, base, base, base, base}
	cases[0].Target = cases[0].Position
	cases[1].Up = geom.Vec3{Z: -1}
	cases[2].Near = 0
	cases[3].Far = cases[3].Near
	cases[4].FOVY = math.Pi
	cases[5].Projection = Projection(9)
	cases[6].Position.X = math.NaN()
	cases[7].Up = geom.Vec3{}
	cases[8].Projection = Orthographic
	cases[8].OrthoHeight = -1
	cases[9].Far = math.Inf(1)
	for i, c := range cases {
		if c.Validate() == nil {
			t.Errorf("invalid camera %d accepted", i)
		}
	}
	if _, err := base.Project(geom.Vec3{Z: 1}, 100, 100); err == nil {
		t.Error("behind-eye point accepted")
	}
	if _, err := base.Project(geom.Vec3{Z: -2}, 0, 100); err == nil {
		t.Error("zero width accepted")
	}
	if _, err := base.WorldToView(geom.Vec3{X: math.NaN()}); err == nil {
		t.Error("nonfinite point accepted")
	}
}
