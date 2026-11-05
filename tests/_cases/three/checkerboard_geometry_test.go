package three_test

import (
	"errors"
	"fmt"
	"gomath/geom"
	"gomath/three"
	"math"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func checkerPoint(u, v float64) geom.Vec3 {
	return geom.Vec3{X: u, Y: v, Z: .27*u*u + .19*v*v + .13*u*v}
}
func checkerBuild(tb testing.TB, n, m int, point func(float64, float64) geom.Vec3) [2]*three.Mesh {
	tb.Helper()
	groups, e := three.CheckerboardSurface(-1, 1, -.8, .9, n, m, func(u, v float64) (geom.Vec3, error) { return point(u, v), nil })
	if e != nil {
		tb.Fatal(e)
	}
	return groups
}

func TestCheckerboardTopologyNormalsAndUV(t *testing.T) {
	for _, counts := range [][2]int{{1, 1}, {1, 4}, {5, 1}, {2, 3}, {3, 3}, {7, 5}} {
		for _, scale := range []float64{1e-9, 1, 1e10} {
			t.Run(fmt.Sprintf("%dx%d/%g", counts[0], counts[1], scale), func(t *testing.T) {
				n, m := counts[0], counts[1]
				point := func(u, v float64) geom.Vec3 { return checkerPoint(u, v).Mul(scale) }
				groups := checkerBuild(t, n, m, point)
				ref := checkerReferenceGrid(-1, 1, -.8, .9, n, m, point)
				seen := map[[2]int]int{}
				for group, mesh := range groups {
					if mesh == nil {
						if group != 1 || n*m != 1 {
							t.Fatal("unexpected nil group")
						}
						continue
					}
					if e := mesh.Validate(); e != nil {
						t.Fatal(e)
					}
					used := make([]bool, len(mesh.Vertices))
					for _, tri := range mesh.Triangles {
						var cu, cv float64
						for _, id := range tri {
							used[id] = true
							uv := mesh.UVs[id]
							cu += uv.U / 3
							cv += (1 - uv.V) / 3
						}
						i, j := int(cu*float64(n)), int(cv*float64(m))
						if (i+j)%2 != group {
							t.Fatal("wrong parity", group, i, j)
						}
						seen[[2]int{i, j}]++
						for _, id := range tri {
							uv := mesh.UVs[id]
							col, row := int(math.Round(uv.U*float64(n))), int(math.Round((1-uv.V)*float64(m)))
							global := row*(n+1) + col
							if uv.U < 0 || uv.U > 1 || uv.V < 0 || uv.V > 1 {
								t.Fatal("UV out of domain", uv)
							}
							if mesh.Vertices[id].Sub(ref.positions[global]).Norm() > 2e-14*scale || mesh.Normals[id].Sub(ref.normals[global]).Norm() > 2e-14 {
								t.Fatal("independent coordinate/global normal", group, id, mesh.Vertices[id], ref.positions[global], mesh.Normals[id], ref.normals[global])
							}
						}
						a, b, c := mesh.Vertices[tri[0]], mesh.Vertices[tri[1]], mesh.Vertices[tri[2]]
						if b.Sub(a).Cross(c.Sub(a)).Z <= 0 {
							t.Fatal("winding not du cross dv")
						}
					}
					for _, value := range used {
						if !value {
							t.Fatal("unused partition vertex")
						}
					}
				}
				if len(seen) != n*m {
					t.Fatal("missing/extra cells", len(seen))
				}
				for cell, count := range seen {
					if count != 2 {
						t.Fatal("cell not exactly two triangles", cell, count)
					}
				}
				if groups[1] != nil {
					shared := 0
					for i, uv := range groups[0].UVs {
						for j, other := range groups[1].UVs {
							if uv == other {
								shared++
								if groups[0].Vertices[i] != groups[1].Vertices[j] || groups[0].Normals[i] != groups[1].Normals[j] {
									t.Fatal("material boundary changed shared geometry/normal")
								}
							}
						}
					}
					if shared == 0 {
						t.Fatal("no shared material boundary")
					}
				}
			})
		}
	}
}

func TestCheckerboardDomainsCallbackOrderAndSeams(t *testing.T) {
	var calls [][2]float64
	_, e := three.CheckerboardSurface(-3, 5, 2, 8, 4, 3, func(u, v float64) (geom.Vec3, error) {
		calls = append(calls, [2]float64{u, v})
		return geom.Vec3{X: u, Y: v}, nil
	})
	if e != nil {
		t.Fatal(e)
	}
	if len(calls) != 20 {
		t.Fatal("callback must run once per node", len(calls))
	}
	for j := 0; j <= 3; j++ {
		for i := 0; i <= 4; i++ {
			if calls[j*5+i] != [2]float64{-3 + 2*float64(i), 2 + 2*float64(j)} {
				t.Fatal("callback order/endpoints", i, j, calls)
			}
		}
	}
	// Open seam: endpoint positions intentionally coincide, but normals and UV
	// are not welded/averaged across the parameter boundary.
	groups, e := three.CheckerboardSurface(0, 1, -1, 1, 8, 2, func(u, v float64) (geom.Vec3, error) {
		s, c := math.Sincos(2 * math.Pi * u)
		if u == 1 {
			s, c = 0, 1
		}
		return geom.Vec3{X: c, Y: s, Z: v}, nil
	})
	if e != nil {
		t.Fatal(e)
	}
	var ends [2][]geom.Vec3
	for _, mesh := range groups {
		for i, uv := range mesh.UVs {
			if uv.U == 0 {
				ends[0] = append(ends[0], mesh.Normals[i])
			}
			if uv.U == 1 {
				ends[1] = append(ends[1], mesh.Normals[i])
			}
		}
	}
	if len(ends[0]) == 0 || len(ends[1]) == 0 || ends[0][0] == ends[1][0] {
		t.Fatal("periodic endpoints were silently welded")
	}
	flipped := checkerBuild(t, 2, 2, func(u, v float64) geom.Vec3 { return geom.Vec3{X: -u, Y: v} })
	for _, mesh := range flipped {
		for _, normal := range mesh.Normals {
			if normal != (geom.Vec3{Z: -1}) {
				t.Fatal("callback flip must flip normals", normal)
			}
		}
	}
	// Large finite parameter ranges are allowed when their span is finite.
	_, e = three.CheckerboardSurface(-1e300, 1e300, 0, 1, 2, 1, func(u, v float64) (geom.Vec3, error) { return geom.Vec3{X: u / 1e300, Y: v}, nil })
	if e != nil {
		t.Fatal(e)
	}
}

type checkerPanicPayload struct{}

func (checkerPanicPayload) String() string { panic("panic payload must never be formatted") }

func TestCheckerboardErrorAtomicityAndBudgets(t *testing.T) {
	callback := func(u, v float64) (geom.Vec3, error) { return geom.Vec3{X: u, Y: v}, nil }
	var count int
	counted := func(u, v float64) (geom.Vec3, error) { count++; return callback(u, v) }
	for _, domain := range [][4]float64{{0, 0, 0, 1}, {1, 0, 0, 1}, {0, 1, 2, 1}, {math.NaN(), 1, 0, 1}, {0, math.Inf(1), 0, 1}, {0, 1, math.Inf(-1), 1}, {-math.MaxFloat64, math.MaxFloat64, 0, 1}, {1, math.Nextafter(1, 2), 0, 1}} {
		count = 0
		g, e := three.CheckerboardSurface(domain[0], domain[1], domain[2], domain[3], 2, 2, counted)
		if g != ([2]*three.Mesh{}) || e == nil || count != 0 {
			t.Fatal("invalid domain must reject before callback", domain, e, count)
		}
	}
	for _, steps := range [][2]int{{0, 1}, {1, 0}, {-1, 1}, {257, 1}, {1, 257}, {int(^uint(0) >> 1), int(^uint(0) >> 1)}} {
		count = 0
		g, e := three.CheckerboardSurface(0, 1, 0, 1, steps[0], steps[1], counted)
		if g != ([2]*three.Mesh{}) || e == nil || count != 0 {
			t.Fatal("step budget rejected too late", steps, e, count)
		}
	}
	if g, e := three.CheckerboardSurface(0, 1, 0, 1, 1, 1, nil); g != ([2]*three.Mesh{}) || e == nil {
		t.Fatal("nil callback accepted")
	}
	sentinel := errors.New("sample failed")
	for _, mode := range []string{"error", "panic", "panicnil", "nonfinite", "outside"} {
		count = 0
		g, e := three.CheckerboardSurface(0, 1, 0, 1, 4, 3, func(u, v float64) (geom.Vec3, error) {
			count++
			if count == 7 {
				switch mode {
				case "error":
					return geom.Vec3{}, sentinel
				case "panic":
					panic(checkerPanicPayload{})
				case "panicnil":
					panic(nil)
				case "nonfinite":
					return geom.Vec3{X: math.NaN()}, nil
				case "outside":
					return geom.Vec3{Z: three.MaxCoordinate + 1}, nil
				}
			}
			return callback(u, v)
		})
		if g != ([2]*three.Mesh{}) || e == nil || count != 7 || !strings.Contains(e.Error(), "(1,1)") {
			t.Fatal("callback failure not atomic/indexed/fail-fast", mode, e, count)
		}
		if mode == "error" && !errors.Is(e, sentinel) {
			t.Fatal("callback error identity lost", e)
		}
	}
	for _, point := range []func(float64, float64) geom.Vec3{func(u, v float64) geom.Vec3 { return geom.Vec3{X: u} }, func(u, v float64) geom.Vec3 { return geom.Vec3{X: u, Y: u * v} }, func(u, v float64) geom.Vec3 { return geom.Vec3{X: -1.5*u*u + 3.5*u, Y: v} }} {
		g, e := three.CheckerboardSurface(0, 2, 0, 1, 2, 1, func(u, v float64) (geom.Vec3, error) { return point(u, v), nil })
		if g != ([2]*three.Mesh{}) || e == nil {
			t.Fatal("degenerate/canceled geometry accepted", e)
		}
	}
	count = 0
	g, e := three.CheckerboardSurface(0, 1, 0, 1, 256, 256, counted)
	if e != nil {
		t.Fatal(e)
	}
	if count != three.MaxCheckerboardSamples {
		t.Fatal("maximum sampling bound", count)
	}
	for _, mesh := range g {
		if len(mesh.Vertices) != 66047 || len(mesh.Triangles) != 65536 {
			t.Fatal("max partition counts", len(mesh.Vertices), len(mesh.Triangles))
		}
	}
	if _, e := three.CheckerboardSurface(0, 1, 0, 1, 1, 1, callback); e != nil {
		t.Fatal("failed construction poisoned next call", e)
	}
}

func TestCheckerboardStorageAndConcurrentConstruction(t *testing.T) {
	want := checkerBuild(t, 5, 4, checkerPoint)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got := checkerBuild(t, 5, 4, checkerPoint)
			if !reflect.DeepEqual(got, want) {
				t.Error("concurrent constructor changed mesh")
			}
			got[0].Vertices[0] = geom.Vec3{}
			got[0].Normals[0] = geom.Vec3{}
			got[0].UVs[0] = three.UV{}
			got[0].Triangles[0] = three.Triangle{}
		}()
	}
	wg.Wait()
	copyGroups := checkerBuild(t, 5, 4, checkerPoint)
	for i := range copyGroups[0].Vertices {
		copyGroups[0].Vertices[i] = geom.Vec3{X: 123}
		copyGroups[0].Normals[i] = geom.Vec3{X: 123}
		copyGroups[0].UVs[i] = three.UV{U: 123}
	}
	for i := range copyGroups[0].Triangles {
		copyGroups[0].Triangles[i] = three.Triangle{}
	}
	if !reflect.DeepEqual(copyGroups[1], want[1]) {
		t.Fatal("partition slices alias each other")
	}
}
