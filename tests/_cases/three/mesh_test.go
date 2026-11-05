package three

import (
	"gomath/geom"
	"math"
	"testing"
)

func TestPrimitivesIndicesNormalsAndOrientation(t *testing.T) {
	box, err := Box(2, 3, 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(box.Vertices) != 24 || len(box.Triangles) != 12 {
		t.Fatal("box topology")
	}
	for _, dimensions := range [][2]int{{3, 2}, {12, 8}, {MaxSubdivisions, MaxSubdivisions}} {
		sphere, err := Sphere(2, dimensions[0], dimensions[1])
		if err != nil {
			t.Fatal(err)
		}
		want := 2 * dimensions[0] * (dimensions[1] - 1)
		if len(sphere.Triangles) != want {
			t.Fatalf("sphere triangles %d != %d", len(sphere.Triangles), want)
		}
		for i, p := range sphere.Vertices {
			if math.Abs(p.Norm()-2) > 1e-12 || math.Abs(sphere.Normals[i].Norm()-1) > 1e-12 {
				t.Fatal("sphere radius or normal")
			}
		}
		checkOutward(t, sphere)
	}
	checkOutward(t, box)
	surface, err := ParametricSurface(-1, 1, -1, 1, 4, 3, func(u, v float64) geom.Vec3 { return geom.Vec3{X: u, Y: v, Z: u * u * .1} })
	if err != nil {
		t.Fatal(err)
	}
	if len(surface.Vertices) != 20 || len(surface.Triangles) != 24 {
		t.Fatal("surface topology")
	}
	for i, n := range surface.Normals {
		if math.Abs(n.Norm()-1) > 1e-12 || n.Z <= 0 {
			t.Fatalf("surface normal %d: %+v", i, n)
		}
	}
	if _, err := ParametricSurface(0, 1e308, 0, 1, 4, 1, func(u, v float64) geom.Vec3 { return geom.Vec3{X: u / 1e308, Y: v} }); err != nil {
		t.Fatalf("finite large parameters: %v", err)
	}
}

func checkOutward(t *testing.T, m *Mesh) {
	t.Helper()
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, face := range m.Triangles {
		a, b, c := m.Vertices[face[0]], m.Vertices[face[1]], m.Vertices[face[2]]
		n := b.Sub(a).Cross(c.Sub(a))
		center := a.Add(b).Add(c).Mul(1.0 / 3)
		if n.Dot(center) <= 0 {
			t.Fatalf("inward triangle: %v", face)
		}
		for _, i := range face {
			if n.Dot(m.Normals[i]) <= 0 {
				t.Fatalf("inward normal: %v", face)
			}
		}
	}
}

func triangleMesh() *Mesh {
	return &Mesh{Vertices: []geom.Vec3{{X: -1, Y: -1, Z: -2}, {X: 1, Y: -1, Z: -2}, {Y: 1, Z: -2}}, Triangles: []Triangle{{0, 1, 2}}}
}

func TestMeshValidationAndPrimitiveBounds(t *testing.T) {
	if (*Mesh)(nil).Validate() == nil {
		t.Fatal("nil mesh accepted")
	}
	mutations := []func(*Mesh){
		func(m *Mesh) { m.Vertices[0].X = math.Inf(1) },
		func(m *Mesh) { m.Vertices[0].X = MaxCoordinate + 1 },
		func(m *Mesh) { m.Triangles[0][0] = -1 },
		func(m *Mesh) { m.Triangles[0][0] = 3 },
		func(m *Mesh) { m.Triangles[0][1] = 0 },
		func(m *Mesh) { m.Vertices[2] = m.Vertices[0].Lerp(m.Vertices[1], .5) },
		func(m *Mesh) { m.Normals = []geom.Vec3{{Z: 1}} },
		func(m *Mesh) { m.Normals = make([]geom.Vec3, 3) },
		func(m *Mesh) { m.Triangles = nil },
	}
	for i, mutate := range mutations {
		m := triangleMesh()
		mutate(m)
		if m.Validate() == nil {
			t.Errorf("invalid mesh %d accepted", i)
		}
	}
	for _, d := range []float64{0, -1, math.NaN(), math.Inf(1), MaxCoordinate * 2} {
		if _, err := Box(d, 1, 1); err == nil {
			t.Errorf("box dimension %g accepted", d)
		}
		if _, err := Sphere(d, 8, 4); err == nil {
			t.Errorf("sphere radius %g accepted", d)
		}
	}
	for _, sub := range [][2]int{{2, 2}, {3, 1}, {MaxSubdivisions + 1, 3}, {3, MaxSubdivisions + 1}} {
		if _, err := Sphere(1, sub[0], sub[1]); err == nil {
			t.Errorf("sphere subdivisions %v accepted", sub)
		}
	}
	if _, err := ParametricSurface(0, 1, 0, 1, 0, 1, func(u, v float64) geom.Vec3 { return geom.Vec3{X: u, Y: v} }); err == nil {
		t.Fatal("surface zero subdivisions accepted")
	}
	if _, err := ParametricSurface(0, 1, 0, 1, 1, 1, nil); err == nil {
		t.Fatal("nil callback accepted")
	}
	if _, err := ParametricSurface(0, 1, 0, 1, 1, 1, func(u, v float64) geom.Vec3 { return geom.Vec3{} }); err == nil {
		t.Fatal("collapsed surface accepted")
	}
}

func TestFiniteExtremeNormalMagnitudes(t *testing.T) {
	for _, n := range []geom.Vec3{{X: math.MaxFloat64, Y: math.MaxFloat64, Z: math.MaxFloat64}, {Z: math.SmallestNonzeroFloat64}} {
		m := triangleMesh()
		m.Normals = []geom.Vec3{n, n, n}
		if err := m.Validate(); err != nil {
			t.Fatal(err)
		}
		u, ok := unit(n)
		if !ok || math.Abs(u.Norm()-1) > 1e-12 {
			t.Fatalf("normalization of finite vector %+v: %+v, %t", n, u, ok)
		}
	}
}
