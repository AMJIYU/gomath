package three

import (
	"encoding/json"
	"gomath/geom"
	"math"
	"os"
	"reflect"
	"slices"
	"sync"
	"testing"
)

type polyhedronControl struct {
	Name               string
	Vertices           [][3]float64
	SourceFaces, Faces [][]int `json:"-"`
	Planes             []struct {
		Vertices []int
		Normal   [3]float64
		Offset   float64
	}
}

// Explicit decoding keeps upstream raw and outward cycles distinct.
func loadPolyhedronControls(tb testing.TB) []polyhedronControl {
	tb.Helper()
	data, err := os.ReadFile("../tests/data/three/polyhedra/manim-controls.json")
	if err != nil {
		tb.Fatal(err)
	}
	var fixture struct {
		Commit    string
		SourceSHA string `json:"source_sha256"`
		Shapes    []struct {
			Name        string
			Vertices    [][3]float64
			SourceFaces [][]int `json:"source_faces"`
			Faces       [][]int
			Planes      []struct {
				Vertices []int
				Normal   [3]float64
				Offset   float64
			}
		}
	}
	if err = json.Unmarshal(data, &fixture); err != nil {
		tb.Fatal(err)
	}
	if fixture.Commit != "16ea443e2a5f1d79db0a721161411ce916d6e733" || len(fixture.SourceSHA) != 64 || len(fixture.Shapes) != 4 {
		tb.Fatal("incomplete pinned polyhedra reference")
	}
	result := make([]polyhedronControl, len(fixture.Shapes))
	for i, s := range fixture.Shapes {
		result[i] = polyhedronControl{s.Name, s.Vertices, s.SourceFaces, s.Faces, s.Planes}
	}
	return result
}

func polyhedronConstructor(tb testing.TB, name string) func(float64) (*Mesh, error) {
	tb.Helper()
	switch name {
	case "Tetrahedron":
		return Tetrahedron
	case "Octahedron":
		return Octahedron
	case "Icosahedron":
		return Icosahedron
	case "Dodecahedron":
		return Dodecahedron
	}
	tb.Fatalf("unknown solid %s", name)
	return nil
}

func polyhedronInput(c polyhedronControl, edge float64) ([]geom.Vec3, [][]int) {
	v := make([]geom.Vec3, len(c.Vertices))
	for i, p := range c.Vertices {
		v[i] = geom.Vec3{X: p[0] * edge, Y: p[1] * edge, Z: p[2] * edge}
	}
	f := make([][]int, len(c.Faces))
	for i := range f {
		f[i] = slices.Clone(c.Faces[i])
	}
	return v, f
}

func TestPlatonicPinnedGeometry(t *testing.T) {
	counts := map[string][3]int{"Tetrahedron": {4, 6, 4}, "Octahedron": {6, 12, 8}, "Icosahedron": {12, 30, 20}, "Dodecahedron": {20, 30, 12}}
	for _, control := range loadPolyhedronControls(t) {
		for _, edge := range []float64{1e-6, .25, 1, 3.7, MaxCoordinate * .7} {
			t.Run(control.Name, func(t *testing.T) {
				m, err := polyhedronConstructor(t, control.Name)(edge)
				if err != nil {
					t.Fatal(err)
				}
				if err = m.Validate(); err != nil {
					t.Fatal(err)
				}
				logical := map[geom.Vec3]int{}
				edges := map[[2]int]int{}
				boundaryCount, triangleCount := 0, 0
				for fi, face := range control.Faces {
					count := len(face)
					base := boundaryCount
					if len(m.Vertices) < base+count {
						t.Fatal("missing face corners")
					}
					n := m.Normals[base]
					if math.Abs(n.Norm()-1) > 2e-14 || n.Dot(m.Vertices[base]) <= 0 {
						t.Fatal("flat normal not unit/outward", fi, n)
					}
					minU, maxU, minV, maxV := 1., 0., 1., 0.
					ids := make([]int, count)
					for j, index := range face {
						p := m.Vertices[base+j]
						want := control.Vertices[index]
						if p.Sub(geom.Vec3{X: want[0] * edge, Y: want[1] * edge, Z: want[2] * edge}).Norm() > edge*4e-15 {
							t.Fatal("pinned coordinate/face order mismatch", fi, j, p, want, edge)
						}
						if m.Normals[base+j] != n {
							t.Fatal("face normal was smoothed")
						}
						id, ok := logical[p]
						if !ok {
							id = len(logical)
							logical[p] = id
						}
						ids[j] = id
						uv := m.UVs[base+j]
						if uv.U < 0 || uv.U > 1 || uv.V < 0 || uv.V > 1 {
							t.Fatal("invalid face UV", uv)
						}
						minU, maxU = math.Min(minU, uv.U), math.Max(maxU, uv.U)
						minV, maxV = math.Min(minV, uv.V), math.Max(maxV, uv.V)
					}
					if minU != 0 || maxU != 1 || minV != 0 || maxV != 1 || m.UVs[base+1].U <= m.UVs[base].U || math.Abs(m.UVs[base+1].V-m.UVs[base].V) > 2e-14 {
						t.Fatal("face UV bounds/first-edge direction")
					}
					for j, a := range ids {
						b := ids[(j+1)%count]
						edges[[2]int{min(a, b), max(a, b)}]++
						if math.Abs(m.Vertices[base+j].Sub(m.Vertices[base+(j+1)%count]).Norm()/edge-1) > 4e-15 {
							t.Fatal("edgeLength does not measure logical edges")
						}
					}
					for j := 1; j+1 < count; j++ {
						tri := m.Triangles[triangleCount]
						triangleCount++
						if tri != (Triangle{base, base + j, base + j + 1}) {
							t.Fatal("invalid face fan", tri)
						}
						a, b, c := m.Vertices[tri[0]], m.Vertices[tri[1]], m.Vertices[tri[2]]
						cross := b.Sub(a).Cross(c.Sub(a))
						if cross.Dot(n) <= 0 || cross.Mul(1/cross.Norm()).Sub(n).Norm() > 3e-14 {
							t.Fatal("winding differs from hard normal")
						}
					}
					boundaryCount += count
				}
				if len(m.Vertices) != boundaryCount || len(m.Normals) != boundaryCount || len(m.UVs) != boundaryCount || len(m.Triangles) != triangleCount {
					t.Fatal("extra/missing output buffers")
				}
				want := counts[control.Name]
				if len(logical) != want[0] || len(edges) != want[1] || len(control.Faces) != want[2] || len(logical)-len(edges)+len(control.Faces) != 2 {
					t.Fatal("logical topology/Euler mismatch")
				}
				// Independent regular-solid radius/area/volume identities, in
				// edge units to keep the same precision at both scale extremes.
				radius := map[string]float64{"Tetrahedron": math.Sqrt(6) / 4, "Octahedron": math.Sqrt2 / 2, "Icosahedron": math.Sqrt(10+2*math.Sqrt(5)) / 4, "Dodecahedron": math.Sqrt(3) * (1 + math.Sqrt(5)) / 4}[control.Name]
				areaWant := map[string]float64{"Tetrahedron": math.Sqrt(3), "Octahedron": 2 * math.Sqrt(3), "Icosahedron": 5 * math.Sqrt(3), "Dodecahedron": 3 * math.Sqrt(25+10*math.Sqrt(5))}[control.Name]
				volumeWant := map[string]float64{"Tetrahedron": 1 / (6 * math.Sqrt2), "Octahedron": math.Sqrt2 / 3, "Icosahedron": 5 * (3 + math.Sqrt(5)) / 12, "Dodecahedron": (15 + 7*math.Sqrt(5)) / 4}[control.Name]
				centroid := geom.Vec3{}
				for p := range logical {
					u := p.Mul(1 / edge)
					if math.Abs(u.Norm()-radius) > 4e-15 {
						t.Fatal("circumradius", u.Norm(), radius)
					}
					centroid = centroid.Add(u)
				}
				if centroid.Norm() > 2e-14 {
					t.Fatal("solid not origin-centered", centroid)
				}
				area, volume := 0., 0.
				for _, tri := range m.Triangles {
					a, b, c := m.Vertices[tri[0]].Mul(1/edge), m.Vertices[tri[1]].Mul(1/edge), m.Vertices[tri[2]].Mul(1/edge)
					area += b.Sub(a).Cross(c.Sub(a)).Norm() / 2
					volume += a.Dot(b.Cross(c)) / 6
				}
				if math.Abs(area/areaWant-1) > 5e-15 || math.Abs(volume/volumeWant-1) > 5e-15 {
					t.Fatal("regular solid area/volume", area, volume, areaWant, volumeWant)
				}
				for _, uses := range edges {
					if uses != 2 {
						t.Fatal("open logical edge")
					}
				}
				// Duplicate geometric positions at hard edges retain different
				// normals, not rounded averages across adjacent polygon planes.
				for p := range logical {
					set := map[geom.Vec3]bool{}
					for i, q := range m.Vertices {
						if p == q {
							set[m.Normals[i]] = true
						}
					}
					if len(set) < 3 {
						t.Fatal("hard face seams missing")
					}
				}
			})
		}
	}
}

func TestPlatonicDimensionBounds(t *testing.T) {
	for _, name := range []string{"Tetrahedron", "Octahedron", "Icosahedron", "Dodecahedron"} {
		build := polyhedronConstructor(t, name)
		for _, edge := range []float64{0, -1, math.SmallestNonzeroFloat64, math.Nextafter(1e-6, 0), MaxCoordinate + 1, math.MaxFloat64, math.NaN(), math.Inf(1)} {
			if m, err := build(edge); m != nil || err == nil {
				t.Fatal("invalid edge accepted", name, edge)
			}
		}
		maxEdge := MaxCoordinate
		if name == "Dodecahedron" {
			maxEdge = MaxCoordinate / ((3 + math.Sqrt(5)) / 4)
			if m, err := build(MaxCoordinate); m != nil || err == nil {
				t.Fatal("unbounded dodecahedron accepted")
			}
		}
		if _, err := build(maxEdge); err != nil {
			t.Fatal("largest bounded solid rejected", name, err)
		}
	}
}

func TestPolyhedronOrientationAndRepresentability(t *testing.T) {
	for _, control := range loadPolyhedronControls(t) {
		v, _ := polyhedronInput(control, 1)
		if mesh, err := Polyhedron(v, control.SourceFaces); mesh != nil || err == nil {
			t.Fatal("public constructor repaired mixed upstream winding", control.Name)
		}
	}
	v, f := polyhedronCube()
	for i := range v {
		v[i].X = -v[i].X
	}
	if mesh, err := Polyhedron(v, f); mesh != nil || err == nil {
		t.Fatal("public constructor repaired reflected source winding")
	}
	for _, face := range f {
		slices.Reverse(face)
	}
	if _, err := Polyhedron(v, f); err != nil {
		t.Fatal("explicitly corrected reflection", err)
	}
	for _, span := range []float64{MinPolyhedronSpan, math.Nextafter(MinPolyhedronSpan, 0)} {
		v, f := polyhedronCube()
		for i := range v {
			v[i] = v[i].Mul(span / 2)
		}
		m, err := Polyhedron(v, f)
		if span == MinPolyhedronSpan && err != nil {
			t.Fatal("inclusive minimum span rejected", err)
		}
		if span < MinPolyhedronSpan && (m != nil || err == nil) {
			t.Fatal("subminimum span accepted")
		}
	}
	// Relative geometry checks operate after rebasing, not around world zero.
	v, f = polyhedronCube()
	for i := range v {
		v[i] = v[i].Mul(.01).Add(geom.Vec3{X: 1e11, Y: -1e11, Z: 5e10})
	}
	if _, err := Polyhedron(v, f); err != nil {
		t.Fatal("representable translated solid", err)
	}
	// A closed but numerically unresolved slab is explicitly rejected.
	v, f = polyhedronCube()
	for i := range v {
		v[i].Z *= 1e-12
	}
	if m, err := Polyhedron(v, f); m != nil || err == nil {
		t.Fatal("unresolved thin solid accepted")
	}
}

func TestPolyhedronMaximumPolygonFace(t *testing.T) {
	n := MaxPolyhedronFaceVertices
	v := make([]geom.Vec3, 2*n)
	f := make([][]int, 2, n+2)
	for i := range n {
		a := 2 * math.Pi * float64(i) / float64(n)
		v[i] = geom.Vec3{X: math.Cos(a), Y: math.Sin(a), Z: -1}
		v[n+i] = geom.Vec3{X: math.Cos(a), Y: math.Sin(a), Z: 1}
		f[0] = append(f[0], n-1-i)
		f[1] = append(f[1], n+i)
		j := (i + 1) % n
		f = append(f, []int{i, j, n + j, n + i})
	}
	m, err := Polyhedron(v, f)
	if err != nil {
		t.Fatal("inclusive polygon-face cap rejected", err)
	}
	if len(m.Vertices) != 6*n || len(m.Triangles) != 4*n-4 {
		t.Fatal("maximum-face prism topology")
	}
	for _, tri := range m.Triangles {
		a, b, c := m.Vertices[tri[0]], m.Vertices[tri[1]], m.Vertices[tri[2]]
		if b.Sub(a).Cross(c.Sub(a)).Dot(m.Normals[tri[0]]) <= 0 {
			t.Fatal("maximum-face fan folded")
		}
	}
}

func polyhedronCube() ([]geom.Vec3, [][]int) {
	v := []geom.Vec3{{X: -1, Y: -1, Z: -1}, {X: 1, Y: -1, Z: -1}, {X: 1, Y: 1, Z: -1}, {X: -1, Y: 1, Z: -1}, {X: -1, Y: -1, Z: 1}, {X: 1, Y: -1, Z: 1}, {X: 1, Y: 1, Z: 1}, {X: -1, Y: 1, Z: 1}}
	f := [][]int{{0, 3, 2, 1}, {4, 5, 6, 7}, {0, 1, 5, 4}, {1, 2, 6, 5}, {2, 3, 7, 6}, {3, 0, 4, 7}}
	return v, f
}

func polyhedronBipyramid(sides int) ([]geom.Vec3, [][]int) {
	v := make([]geom.Vec3, sides+2)
	v[sides], v[sides+1] = geom.Vec3{Z: 1}, geom.Vec3{Z: -1}
	f := make([][]int, 0, 2*sides)
	for i := range sides {
		angle := 2 * math.Pi * float64(i) / float64(sides)
		v[i] = geom.Vec3{X: math.Cos(angle), Y: math.Sin(angle)}
		next := (i + 1) % sides
		f = append(f, []int{i, next, sides}, []int{i, sides + 1, next})
	}
	return v, f
}

func TestPolyhedronRejectsInvalidGeometryAndBudgets(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*[]geom.Vec3, *[][]int)
	}{
		{"nan", func(v *[]geom.Vec3, f *[][]int) { (*v)[0].X = math.NaN() }},
		{"infinity", func(v *[]geom.Vec3, f *[][]int) { (*v)[0].Y = math.Inf(-1) }},
		{"coordinate", func(v *[]geom.Vec3, f *[][]int) { (*v)[0].Z = MaxCoordinate + 1 }},
		{"flat", func(v *[]geom.Vec3, f *[][]int) {
			for i := range *v {
				(*v)[i].Z = 0
			}
		}},
		{"tiny", func(v *[]geom.Vec3, f *[][]int) {
			for i := range *v {
				(*v)[i] = (*v)[i].Mul(1e-12)
			}
		}},
		{"duplicate-position", func(v *[]geom.Vec3, f *[][]int) { (*v)[1] = (*v)[0] }},
		{"almost-duplicate", func(v *[]geom.Vec3, f *[][]int) { (*v)[1] = (*v)[0].Add(geom.Vec3{X: 1e-12}) }},
		{"open", func(v *[]geom.Vec3, f *[][]int) { *f = (*f)[:5] }},
		{"reversed-face", func(v *[]geom.Vec3, f *[][]int) { slices.Reverse((*f)[1]) }},
		{"inward", func(v *[]geom.Vec3, f *[][]int) {
			for _, face := range *f {
				slices.Reverse(face)
			}
		}},
		{"duplicate-face", func(v *[]geom.Vec3, f *[][]int) { *f = append(*f, slices.Clone((*f)[0])) }},
		{"duplicate-reversed-face", func(v *[]geom.Vec3, f *[][]int) {
			last := slices.Clone((*f)[0])
			slices.Reverse(last)
			*f = append(*f, last)
		}},
		{"repeated-index", func(v *[]geom.Vec3, f *[][]int) { (*f)[0] = []int{0, 3, 2, 0, 1} }},
		{"negative-index", func(v *[]geom.Vec3, f *[][]int) { (*f)[0][0] = -1 }},
		{"huge-index", func(v *[]geom.Vec3, f *[][]int) { (*f)[0][0] = int(^uint(0) >> 1) }},
		{"nonplanar", func(v *[]geom.Vec3, f *[][]int) { (*v)[6].Z += .001 }},
		{"unused", func(v *[]geom.Vec3, f *[][]int) { *v = append(*v, geom.Vec3{}) }},
		{"concave-triangles", func(v *[]geom.Vec3, f *[][]int) {
			var triangles [][]int
			for _, face := range *f {
				triangles = append(triangles, []int{face[0], face[1], face[2]}, []int{face[0], face[2], face[3]})
			}
			*f = triangles
			(*v)[6] = geom.Vec3{X: .1, Y: .1, Z: .1}
		}},
		{"crossed-boundary", func(v *[]geom.Vec3, f *[][]int) { (*f)[1] = []int{4, 6, 5, 7} }},
		{"collinear-corner", func(v *[]geom.Vec3, f *[][]int) {
			*v = append(*v, geom.Vec3{Y: -1, Z: 1})
			(*f)[1] = []int{4, 8, 5, 6, 7}
		}},
		{"vertex-budget", func(v *[]geom.Vec3, f *[][]int) { *v = make([]geom.Vec3, MaxPolyhedronVertices+1) }},
		{"face-budget", func(v *[]geom.Vec3, f *[][]int) { *f = make([][]int, MaxPolyhedronFaces+1) }},
		{"face-corner-budget", func(v *[]geom.Vec3, f *[][]int) { (*f)[0] = make([]int, MaxPolyhedronFaceVertices+1) }},
		{"total-corner-budget", func(v *[]geom.Vec3, f *[][]int) {
			*f = make([][]int, MaxPolyhedronCorners/MaxPolyhedronFaceVertices+1)
			for i := range *f {
				(*f)[i] = make([]int, MaxPolyhedronFaceVertices)
			}
		}},
		{"disconnected", func(v *[]geom.Vec3, f *[][]int) {
			other, ff := polyhedronCube()
			for i := range other {
				other[i].X += 5
			}
			for _, face := range ff {
				for j := range face {
					face[j] += 8
				}
			}
			*v = append(*v, other...)
			*f = append(*f, ff...)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v, f := polyhedronCube()
			tc.mutate(&v, &f)
			if m, err := Polyhedron(v, f); m != nil || err == nil {
				t.Fatal("invalid geometry/budget accepted")
			}
		})
	}
	if m, err := Polyhedron(nil, nil); m != nil || err == nil {
		t.Fatal("empty polyhedron accepted")
	}
	// Prove the inclusive vertex cap is usable with a real closed convex solid,
	// not only length-checked on dummy input. 1,024 vertices / 2,044 faces.
	v, f := polyhedronBipyramid(MaxPolyhedronVertices - 2)
	if _, err := Polyhedron(v, f); err != nil {
		t.Fatal("valid maximum vertex budget", err)
	}
	for _, scale := range []float64{MinPolyhedronSpan, MaxCoordinate} {
		v, f := polyhedronCube()
		for i := range v {
			v[i] = v[i].Mul(scale)
		}
		if _, err := Polyhedron(v, f); err != nil {
			t.Fatal("finite scale", scale, err)
		}
	}
}

func TestPolyhedronSourceIsolationAndConcurrentConstruction(t *testing.T) {
	for _, c := range loadPolyhedronControls(t) {
		v, f := polyhedronInput(c, 1)
		beforeV := slices.Clone(v)
		beforeF := make([][]int, len(f))
		for i := range f {
			beforeF[i] = slices.Clone(f[i])
		}
		m, err := Polyhedron(v, f)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(v, beforeV) || !reflect.DeepEqual(f, beforeF) {
			t.Fatal("constructor mutated input")
		}
		other, err := polyhedronConstructor(t, c.Name)(1)
		if err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		for range 8 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for range 3 {
					got, err := polyhedronConstructor(t, c.Name)(1)
					if err != nil || !reflect.DeepEqual(got, other) {
						t.Error("constructor shared mutable state", err)
						return
					}
				}
			}()
		}
		for i := range v {
			v[i] = geom.Vec3{X: math.NaN()}
		}
		for i := range f {
			for j := range f[i] {
				f[i][j] = -999
			}
		}
		if err := m.Validate(); err != nil {
			t.Fatal("input mutation reached constructed mesh", err)
		}
		m.Vertices[0].X = math.NaN()
		m.Normals[0].Z = math.NaN()
		m.UVs[0].U = math.NaN()
		m.Triangles[0][0] = -1
		wg.Wait()
		if err := other.Validate(); err != nil {
			t.Fatal("independent outputs alias", err)
		}
	}
}
