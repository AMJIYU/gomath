package three

import (
	"fmt"
	"gomath/geom"
	"math"
	"reflect"
	"slices"
	"testing"
)

func fmtSegments(a, b int) string { return fmt.Sprintf("%dx%d", a, b) }

// Weld positions only for this topological check: UV seams, cap normals and
// apex sectors intentionally have distinct indices in the authored mesh.
func checkPrimitiveTopology(t *testing.T, m *Mesh, euler int) {
	t.Helper()
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(m.Normals) != len(m.Vertices) || len(m.UVs) != len(m.Vertices) {
		t.Fatal("missing complete normals/UVs")
	}
	positions := map[geom.Vec3]int{}
	indices := make([]int, len(m.Vertices))
	used := make([]bool, len(m.Vertices))
	for i, p := range m.Vertices {
		id, ok := positions[p]
		if !ok {
			id = len(positions)
			positions[p] = id
		}
		indices[i] = id
		if math.Abs(m.Normals[i].Norm()-1) > 1e-12 {
			t.Fatalf("nonunit normal %d: %v", i, m.Normals[i])
		}
		if uv := m.UVs[i]; uv.U < 0 || uv.U > 1 || uv.V < 0 || uv.V > 1 {
			t.Fatalf("UV outside unit square: %v", uv)
		}
	}
	type edge struct{ a, b int }
	type incident struct{ count, direction int }
	edges := map[edge]incident{}
	volume := 0.0
	for _, tri := range m.Triangles {
		a, b, c := m.Vertices[tri[0]], m.Vertices[tri[1]], m.Vertices[tri[2]]
		n := b.Sub(a).Cross(c.Sub(a))
		avg := m.Normals[tri[0]].Add(m.Normals[tri[1]]).Add(m.Normals[tri[2]])
		if n.Dot(avg) <= 0 {
			t.Fatalf("triangle contradicts outward normals: %v", tri)
		}
		volume += a.Dot(b.Cross(c)) / 6
		for j, index := range tri {
			used[index] = true
			a, b := indices[index], indices[tri[(j+1)%3]]
			sign := 1
			if a > b {
				a, b, sign = b, a, -1
			}
			key := edge{a, b}
			v := edges[key]
			v.count++
			v.direction += sign
			edges[key] = v
		}
	}
	for key, v := range edges {
		if v.count != 2 || v.direction != 0 {
			t.Fatalf("open/nonmanifold/inconsistently oriented edge %v: %+v", key, v)
		}
	}
	for i, yes := range used {
		if !yes {
			t.Fatalf("unused vertex %d", i)
		}
	}
	if got := len(positions) - len(edges) + len(m.Triangles); got != euler || volume <= 0 {
		t.Fatalf("Euler=%d want %d, signed volume=%g", got, euler, volume)
	}
}

func checkPrimitiveSeam(t *testing.T, m *Mesh, a, b int, uvA, uvB UV) {
	t.Helper()
	if a == b || m.Vertices[a] != m.Vertices[b] || m.Normals[a] != m.Normals[b] || m.UVs[a] != uvA || m.UVs[b] != uvB {
		t.Fatalf("seam %d/%d not exactly split: positions %v/%v normals %v/%v UV %v/%v", a, b, m.Vertices[a], m.Vertices[b], m.Normals[a], m.Normals[b], m.UVs[a], m.UVs[b])
	}
}

func TestAxialPrimitivesTopologyNormalsUV(t *testing.T) {
	for _, cone := range []bool{false, true} {
		for _, sub := range [][2]int{{3, 1}, {8, 4}, {17, 9}, {MaxSubdivisions, MaxSubdivisions}} {
			n, h := sub[0], sub[1]
			construct, name := Cylinder, "Cylinder"
			if cone {
				construct, name = Cone, "Cone"
			}
			t.Run(name+"/"+fmtSegments(n, h), func(t *testing.T) {
				m, err := construct(2, 3, n, h)
				if err != nil {
					t.Fatal(err)
				}
				rings, caps, wantTriangles := h+1, 2, 2*n*(h+1)
				wantVertices := (h + 3) * (n + 1)
				if cone {
					rings, caps, wantTriangles = h, 1, 2*n*h
					wantVertices = (h+1)*(n+1) + n
				}
				if len(m.Vertices) != wantVertices || len(m.Triangles) != wantTriangles {
					t.Fatalf("counts %d/%d want %d/%d", len(m.Vertices), len(m.Triangles), wantVertices, wantTriangles)
				}
				checkPrimitiveTopology(t, m, 2)
				for j := 0; j < rings; j++ {
					v := float64(j) / float64(h)
					r := 2.0
					if cone {
						r *= 1 - v
					}
					checkPrimitiveSeam(t, m, j*(n+1), j*(n+1)+n, UV{V: 1 - v}, UV{U: 1, V: 1 - v})
					for i := 0; i <= n; i++ {
						idx := j*(n+1) + i
						p, normal, uv := m.Vertices[idx], m.Normals[idx], m.UVs[idx]
						if math.Abs(math.Hypot(p.X, p.Y)-r) > 1e-12 || math.Abs(p.Z-3*(v-.5)) > 1e-12 || uv != (UV{U: float64(i) / float64(n), V: 1 - v}) {
							t.Fatalf("bad side position/UV %d", idx)
						}
						want := geom.Vec3{X: p.X / r, Y: p.Y / r}
						if cone {
							want = geom.Vec3{X: want.X * 3, Y: want.Y * 3, Z: 2}.Mul(1 / math.Sqrt(13))
						}
						if normal.Sub(want).Norm() > 1e-12 {
							t.Fatalf("bad analytic side normal %d: %v want %v", idx, normal, want)
						}
					}
				}
				capStart := rings * (n + 1)
				if cone {
					for i := 0; i < n; i++ {
						idx := capStart + i
						u := (float64(i) + .5) / float64(n)
						wantNormal := geom.Vec3{X: 3 * math.Cos(2*math.Pi*u), Y: 3 * math.Sin(2*math.Pi*u), Z: 2}.Mul(1 / math.Sqrt(13))
						if m.Vertices[idx] != (geom.Vec3{Z: 1.5}) || m.UVs[idx] != (UV{U: u}) || m.Normals[idx].Sub(wantNormal).Norm() > 1e-12 {
							t.Fatalf("bad split tip %d", i)
						}
					}
					capStart += n
				}
				for cap := 0; cap < caps; cap++ {
					sign := float64(2*cap - 1)
					center := capStart + cap*(n+1)
					if m.Vertices[center] != (geom.Vec3{Z: 1.5 * sign}) || m.UVs[center] != (UV{U: .5, V: .5}) {
						t.Fatal("bad cap center")
					}
					for i := 0; i <= n; i++ {
						idx := center + i
						p := m.Vertices[idx]
						if m.Normals[idx] != (geom.Vec3{Z: sign}) || m.UVs[idx] != (UV{U: .5 + p.X/4, V: .5 - sign*p.Y/4}) {
							t.Fatalf("cap hard normal/disk UV %d", idx)
						}
						if i > 0 {
							side := (rings-1)*cap*(n+1) + i - 1
							if m.Vertices[idx] != m.Vertices[side] || m.Normals[idx] == m.Normals[side] {
								t.Fatal("cap does not split coincident side vertex")
							}
						}
					}
				}
			})
		}
	}
}

func TestTorusTopologyNormalsUV(t *testing.T) {
	for _, sub := range [][2]int{{3, 3}, {8, 6}, {17, 11}, {MaxSubdivisions, MaxSubdivisions}} {
		t.Run(fmtSegments(sub[0], sub[1]), func(t *testing.T) {
			n, k := sub[0], sub[1]
			m, err := Torus(2, .7, n, k)
			if err != nil {
				t.Fatal(err)
			}
			if len(m.Vertices) != (n+1)*(k+1) || len(m.Triangles) != 2*n*k {
				t.Fatal("torus counts")
			}
			checkPrimitiveTopology(t, m, 0)
			for j := 0; j <= k; j++ {
				checkPrimitiveSeam(t, m, j*(n+1), j*(n+1)+n, UV{V: float64(j) / float64(k)}, UV{U: 1, V: float64(j) / float64(k)})
			}
			for i := 0; i <= n; i++ {
				checkPrimitiveSeam(t, m, i, k*(n+1)+i, UV{U: float64(i) / float64(n)}, UV{U: float64(i) / float64(n), V: 1})
			}
			for i, p := range m.Vertices {
				radial := math.Hypot(p.X, p.Y)
				center := geom.Vec3{X: 2 * p.X / radial, Y: 2 * p.Y / radial}
				offset := p.Sub(center)
				if math.Abs(offset.Norm()-.7) > 1e-12 || m.Normals[i].Sub(offset.Mul(1/.7)).Norm() > 1e-12 {
					t.Fatalf("torus tube/normal %d", i)
				}
				uv := m.UVs[i]
				u, v := uv.U*2*math.Pi, uv.V*2*math.Pi
				want := geom.Vec3{X: (2 + .7*math.Cos(v)) * math.Cos(u), Y: (2 + .7*math.Cos(v)) * math.Sin(u), Z: .7 * math.Sin(v)}
				if p.Sub(want).Norm() > 1e-12 {
					t.Fatalf("torus UV parameterization %d", i)
				}
			}
		})
	}
}

func TestNewPrimitiveBoundsBudgetsAndStorage(t *testing.T) {
	for ci, construct := range []func(float64, float64, int, int) (*Mesh, error){Cylinder, Cone, Torus} {
		for _, d := range []float64{-1, 0, 1e-7, math.NaN(), math.Inf(1), math.Inf(-1), MaxCoordinate * 2} {
			for _, dims := range [][2]float64{{d, .5}, {1, d}} {
				if m, err := construct(dims[0], dims[1], 8, 4); err == nil || m != nil {
					t.Fatalf("invalid dimensions %v accepted", dims)
				}
			}
		}
		for _, counts := range [][2]int{{-1, 3}, {0, 3}, {2, 3}, {3, 0}, {257, 3}, {3, 257}, {int(^uint(0) >> 1), 4}, {8, int(^uint(0) >> 1)}} {
			if m, err := construct(1, .5, counts[0], counts[1]); err == nil || m != nil {
				t.Fatalf("invalid segment counts %v accepted", counts)
			}
		}
		for _, dims := range [][2]float64{{1, .5}, {1e-6, 1e-6}, {MaxCoordinate, MaxCoordinate}} {
			if ci == 2 && dims[0] <= dims[1] {
				continue
			}
			if m, err := construct(dims[0], dims[1], 8, 4); err != nil || m.Validate() != nil {
				t.Fatalf("valid dimensions %v rejected: %v", dims, err)
			}
		}
		a, _ := construct(1, .5, 8, 4)
		b, _ := construct(1, .5, 8, 4)
		before := Mesh{Vertices: slices.Clone(b.Vertices), Normals: slices.Clone(b.Normals), UVs: slices.Clone(b.UVs), Triangles: slices.Clone(b.Triangles)}
		// All four slices must be owned independently by each constructor call.
		a.Vertices[0] = geom.Vec3{}
		a.Normals[0] = geom.Vec3{}
		a.UVs[0] = UV{U: .313}
		a.Triangles[0] = Triangle{}
		if !reflect.DeepEqual(before, *b) {
			t.Fatal("constructor slices alias")
		}
	}
	for _, dims := range [][2]float64{{1, 1}, {1, 2}, {MaxCoordinate, 1}, {MaxCoordinate / 2, 1e-6}} {
		if m, err := Torus(dims[0], dims[1], 8, 4); err == nil || m != nil {
			t.Fatalf("horn/spindle/overflow/unrepresentable torus accepted: %v", dims)
		}
	}
	if _, err := Torus(4e11, 1e11, 8, 4); err != nil {
		t.Fatal(err)
	}
	if _, err := Torus(2e-6, 1e-6, 8, 4); err != nil {
		t.Fatal(err)
	}
	// The full +/-tube offsets are representable here, but adjacent radial
	// steps round together near the equators and would fold into axial strips.
	if m, err := Torus(5e11, 1e-4, 8, 256); err == nil || m != nil {
		t.Fatal("numerically collapsed tube steps accepted")
	}
	if _, err := Torus(1, .5, 3, 2); err == nil {
		t.Fatal("two tube segments accepted")
	}
	for _, counts := range [][2]int{{MaxVertices + 1, 1}, {3, MaxTriangles + 1}, {2, 1}, {3, 0}} {
		if m, err := primitiveStorage(counts[0], counts[1]); err == nil || m != nil {
			t.Fatal("primitive allocation budget ignored")
		}
	}
}
