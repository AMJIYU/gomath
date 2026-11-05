// Coordinate formulas, vertex order and face sets adapted from Manim Community
// manim/mobject/three_d/polyhedra.py at 16ea443e2a5f1d79db0a721161411ce916d6e733.
// Copyright (c) 2018 3Blue1Brown LLC
// Copyright (c) 2024, the Manim Community Developers
// MIT; see LICENSE and LICENSES/manim-LICENSE{,.community}.txt.
package three

import (
	"fmt"
	"gomath/geom"
	"math"
	"slices"
)

// Tetrahedron creates the origin-centered regular tetrahedron in Manim's
// orientation: (u,u,u), (u,-u,-u), (-u,u,-u), (-u,-u,u), u=edgeLength*sqrt(2)/4.
// edgeLength must be finite in [1e-6,MaxCoordinate]. Faces are split for flat
// outward normals and independent planar UVs; no graph/vertex spheres are added.
func Tetrahedron(edgeLength float64) (*Mesh, error) {
	if !primitiveDimension(edgeLength) {
		return nil, platonicEdgeError()
	}
	u := edgeLength * math.Sqrt2 / 4
	return platonicMesh([]geom.Vec3{{X: u, Y: u, Z: u}, {X: u, Y: -u, Z: -u}, {X: -u, Y: u, Z: -u}, {X: -u, Y: -u, Z: u}}, [][]int{{0, 1, 2}, {3, 0, 2}, {0, 1, 3}, {3, 1, 2}})
}

// Octahedron creates Manim's origin-centered regular octahedron with vertices
// +/-edgeLength/sqrt(2) on X, Y and Z. Dimensions/normals/UVs follow Tetrahedron.
func Octahedron(edgeLength float64) (*Mesh, error) {
	if !primitiveDimension(edgeLength) {
		return nil, platonicEdgeError()
	}
	u := edgeLength * math.Sqrt2 / 2
	return platonicMesh([]geom.Vec3{{X: u}, {X: -u}, {Y: u}, {Y: -u}, {Z: u}, {Z: -u}}, [][]int{{2, 4, 1}, {0, 4, 2}, {4, 3, 0}, {1, 3, 4}, {3, 5, 0}, {1, 5, 3}, {2, 5, 1}, {0, 5, 2}})
}

// Icosahedron creates Manim's origin-centered regular icosahedron: cyclic
// coordinate families (0,+/-b,+/-a), (+/-b,+/-a,0), (+/-a,0,+/-b), where
// a=edgeLength*(1+sqrt(5))/4 and b=edgeLength/2. Hard face normals and UVs.
func Icosahedron(edgeLength float64) (*Mesh, error) {
	if !primitiveDimension(edgeLength) {
		return nil, platonicEdgeError()
	}
	a, b := edgeLength*((1+math.Sqrt(5))/4), edgeLength*.5
	return platonicMesh([]geom.Vec3{{Y: b, Z: a}, {Y: -b, Z: a}, {Y: b, Z: -a}, {Y: -b, Z: -a}, {X: b, Y: a}, {X: b, Y: -a}, {X: -b, Y: a}, {X: -b, Y: -a}, {X: a, Z: b}, {X: a, Z: -b}, {X: -a, Z: b}, {X: -a, Z: -b}}, [][]int{{1, 8, 0}, {1, 5, 7}, {8, 5, 1}, {7, 3, 5}, {5, 9, 3}, {8, 9, 5}, {3, 2, 9}, {9, 4, 2}, {8, 4, 9}, {0, 4, 8}, {6, 4, 0}, {6, 2, 4}, {11, 2, 6}, {3, 11, 2}, {0, 6, 10}, {10, 1, 0}, {10, 7, 1}, {11, 7, 3}, {10, 11, 7}, {10, 11, 6}})
}

// Dodecahedron creates Manim's origin-centered regular dodecahedron. Its eight
// (+/-a,+/-a,+/-a) vertices and twelve cyclic (0,+/-c,+/-b) families use
// a=edgeLength*(1+sqrt(5))/4, b=edgeLength*(3+sqrt(5))/4, c=edgeLength/2.
// b must be <=MaxCoordinate; valid edgeLength alone does not guarantee that.
// Pentagon fans retain flat face normals and independent planar UVs.
func Dodecahedron(edgeLength float64) (*Mesh, error) {
	if !primitiveDimension(edgeLength) {
		return nil, platonicEdgeError()
	}
	a, b, c := edgeLength*((1+math.Sqrt(5))/4), edgeLength*((3+math.Sqrt(5))/4), edgeLength*.5
	return platonicMesh([]geom.Vec3{{X: a, Y: a, Z: a}, {X: a, Y: a, Z: -a}, {X: a, Y: -a, Z: a}, {X: a, Y: -a, Z: -a}, {X: -a, Y: a, Z: a}, {X: -a, Y: a, Z: -a}, {X: -a, Y: -a, Z: a}, {X: -a, Y: -a, Z: -a}, {Y: c, Z: b}, {Y: c, Z: -b}, {Y: -c, Z: -b}, {Y: -c, Z: b}, {X: c, Y: b}, {X: -c, Y: b}, {X: c, Y: -b}, {X: -c, Y: -b}, {X: b, Z: c}, {X: -b, Z: c}, {X: b, Z: -c}, {X: -b, Z: -c}}, [][]int{{18, 16, 0, 12, 1}, {3, 18, 16, 2, 14}, {3, 10, 9, 1, 18}, {1, 9, 5, 13, 12}, {0, 8, 4, 13, 12}, {2, 16, 0, 8, 11}, {4, 17, 6, 11, 8}, {17, 19, 5, 13, 4}, {19, 7, 15, 6, 17}, {6, 15, 14, 2, 11}, {19, 5, 9, 10, 7}, {7, 10, 3, 14, 15}})
}

func platonicEdgeError() error {
	return fmt.Errorf("three: platonic edge length must be finite in [1e-6,%g] and resulting positions within +/- %g", MaxCoordinate, MaxCoordinate)
}

// Only these fixed, origin-centered source tables get automatic orientation.
// Public Polyhedron never repairs caller winding. Keep each face's first vertex
// and face order when reversing the cycle, making UV anchors deterministic.
func platonicMesh(vertices []geom.Vec3, faces [][]int) (*Mesh, error) {
	for _, face := range faces {
		a, b, c := vertices[face[0]], vertices[face[1]], vertices[face[2]]
		if b.Sub(a).Cross(c.Sub(a)).Dot(a) < 0 {
			slices.Reverse(face[1:])
		}
	}
	return Polyhedron(vertices, faces)
}
