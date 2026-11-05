package three

import (
	"fmt"
	"gomath/geom"
)

// MaxCheckerboardSamples bounds synchronous callback invocations in one
// CheckerboardSurface construction. Successful calls sample each grid node once.
const MaxCheckerboardSamples = (MaxSubdivisions + 1) * (MaxSubdivisions + 1)

// CheckerboardSurface creates two material-independent triangle partitions.
// A cell (i,j) belongs to group (i+j)%2; each cell has triangles (a,b,c),(a,c,d),
// with a=(i,j), b=(i+1,j), c=(i+1,j+1), d=(i,j+1). Winding follows sampled
// dP/du cross dP/dv. Both ranges must be finite, strictly increasing and have
// finite spans and representably distinct samples; steps are in [1,256].
//
// Normals are normalized area-weighted incident triangle sums over the FULL
// grid, before splitting materials. Shared nodes have exactly equal positions,
// normals and UVs in both groups, but independent storage. UV is (i/uSteps,
// 1-j/vSteps), spanning the entire domain, not restarting in each cell. Domain
// edges/seams are not welded or automatically closed; collapsed triangles and
// canceled vertex normals are errors. No analytic derivatives are requested.
//
// sample runs synchronously in row order (v outer, u inner), once per node,
// including exact range endpoints. Error/nonfinite output stops at that node;
// a callback panic becomes an indexed error without formatting its payload.
// Geometric degeneracy is checked after all nodes are sampled. All failures
// return [2]*Mesh{}; callback side effects cannot be rolled back. The callback
// must return: there is no timeout or recovery from Goexit/process termination.
// No callback, input slice or mutable cache is retained after construction.
//
// A group with no cells is nil, not an invalid empty Mesh (only group 1 for a
// 1x1 grid). Skip nil groups when creating Instances/scene3d tracks. Nonempty
// meshes own independent mutable slices; finish editing before concurrent use.
func CheckerboardSurface(uMin, uMax, vMin, vMax float64, uSteps, vSteps int, sample func(u, v float64) (geom.Vec3, error)) ([2]*Mesh, error) {
	var empty [2]*Mesh
	if sample == nil || uSteps < 1 || uSteps > MaxSubdivisions || vSteps < 1 || vSteps > MaxSubdivisions {
		return empty, fmt.Errorf("three: checkerboard surface requires a callback and both step counts in [1,%d]", MaxSubdivisions)
	}
	nodes, cells := (uSteps+1)*(vSteps+1), uSteps*vSteps
	if nodes > MaxCheckerboardSamples || 2*nodes > MaxVertices || 2*cells > MaxTriangles {
		return empty, fmt.Errorf("three: checkerboard surface sample/aggregate mesh budget exceeded")
	}
	u, err := checkerboardAxis(uMin, uMax, uSteps)
	if err != nil {
		return empty, fmt.Errorf("three: checkerboard U domain: %w", err)
	}
	v, err := checkerboardAxis(vMin, vMax, vSteps)
	if err != nil {
		return empty, fmt.Errorf("three: checkerboard V domain: %w", err)
	}
	positions := make([]geom.Vec3, nodes)
	for j, vv := range v {
		for i, uu := range u {
			p, err := checkerboardSample(sample, uu, vv, i, j)
			if err != nil {
				return empty, err
			}
			if !bounded(p) {
				return empty, fmt.Errorf("three: checkerboard sample (%d,%d) must be finite within +/- %g", i, j, MaxCoordinate)
			}
			positions[j*(uSteps+1)+i] = p
		}
	}
	normals := make([]geom.Vec3, nodes)
	triangles := func(i, j int) [2]Triangle {
		a := j*(uSteps+1) + i
		b := a + 1
		d := a + uSteps + 1
		c := d + 1
		return [2]Triangle{{a, b, c}, {a, c, d}}
	}
	for j := 0; j < vSteps; j++ {
		for i := 0; i < uSteps; i++ {
			for k, tri := range triangles(i, j) {
				n := positions[tri[1]].Sub(positions[tri[0]]).Cross(positions[tri[2]].Sub(positions[tri[0]]))
				if _, ok := unit(n); !ok {
					return empty, fmt.Errorf("three: checkerboard cell (%d,%d) triangle %d is degenerate", i, j, k)
				}
				for _, index := range tri {
					normals[index] = normals[index].Add(n)
				}
			}
		}
	}
	for i, n := range normals {
		var ok bool
		normals[i], ok = unit(n)
		if !ok {
			return empty, fmt.Errorf("three: checkerboard incident normals cancel at node (%d,%d)", i%(uSteps+1), i/(uSteps+1))
		}
	}
	var result [2]*Mesh
	var remap [2][]int
	for group := range 2 {
		count := cells / 2
		if group == 0 {
			count = (cells + 1) / 2
		}
		if count == 0 {
			continue
		}
		mesh, err := primitiveStorage(nodes, 2*count)
		if err != nil {
			return empty, err
		}
		result[group] = mesh
		remap[group] = make([]int, nodes) // zero is unused; stored local index + 1
	}
	for j := 0; j < vSteps; j++ {
		for i := 0; i < uSteps; i++ {
			group := (i + j) % 2
			mesh := result[group]
			for _, tri := range triangles(i, j) {
				var local Triangle
				for k, global := range tri {
					if remap[group][global] == 0 {
						mesh.primitiveVertex(positions[global], normals[global], UV{U: float64(global%(uSteps+1)) / float64(uSteps), V: 1 - float64(global/(uSteps+1))/float64(vSteps)})
						remap[group][global] = len(mesh.Vertices)
					}
					local[k] = remap[group][global] - 1
				}
				mesh.Triangles = append(mesh.Triangles, local)
			}
		}
	}
	for group, mesh := range result {
		if mesh != nil {
			if err := mesh.Validate(); err != nil {
				return empty, fmt.Errorf("three: checkerboard group %d: %w", group, err)
			}
		}
	}
	return result, nil
}

func checkerboardAxis(start, end float64, steps int) ([]float64, error) {
	span := end - start
	if !finite(start) || !finite(end) || !finite(span) || span <= 0 {
		return nil, fmt.Errorf("range must be finite and strictly increasing with a finite span")
	}
	values := make([]float64, steps+1)
	values[0], values[steps] = start, end
	for i := 1; i < steps; i++ {
		if 2*i <= steps {
			values[i] = start + span*(float64(i)/float64(steps))
		} else {
			values[i] = end - span*(float64(steps-i)/float64(steps))
		}
	}
	for i := 1; i <= steps; i++ {
		if values[i] <= values[i-1] {
			return nil, fmt.Errorf("step %d is not representably increasing", i)
		}
	}
	return values, nil
}

func checkerboardSample(sample func(float64, float64) (geom.Vec3, error), u, v float64, i, j int) (point geom.Vec3, err error) {
	completed := false
	defer func() {
		if !completed {
			_ = recover()
			point = geom.Vec3{}
			err = fmt.Errorf("three: checkerboard callback panicked at node (%d,%d)", i, j)
		}
	}()
	point, err = sample(u, v)
	completed = true
	if err != nil {
		return geom.Vec3{}, fmt.Errorf("three: checkerboard sample (%d,%d): %w", i, j, err)
	}
	return point, nil
}
