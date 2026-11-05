package path

import (
	"gomath/geom"
	"math"
)

// The open/closed tridiagonal equations and closure tolerance are derived from
// Manim Community 0.22.1, commit 16ea443e2a5f1d79db0a721161411ce916d6e733,
// manim/utils/bezier.py and mobject/types/vectorized_mobject.py.
// Copyright (c) 2018 3Blue1Brown LLC
// Copyright (c) 2024, the Manim Community Developers
// MIT license: LICENSES/manim-LICENSE.txt and manim-LICENSE.community.txt.
// Unlike the reference's mutable global memo arrays, all scratch is per call.
func anchorContour(a []geom.Vec3, mode AnchorMode) Contour {
	n := len(a) - 1
	out := Contour{Start: a[0], Closed: n > 0 && a[0] == a[n], Curves: make([]Cubic, n)}
	for i := range out.Curves {
		out.Curves[i] = Line(a[i], a[i+1])
	}
	if mode == AnchorJagged || n < 2 {
		return out
	}
	closed := editAnchorsClosed(a[0], a[n])
	cp := make([]float64, n-1)
	if closed {
		cp[0] = 1.0 / 3
	} else {
		cp[0] = .5
	}
	for i := 1; i < n-1; i++ {
		cp[i] = 1 / (4 - cp[i-1])
	}
	if !closed {
		out.Curves[0].P1 = a[0].Mul(.5).Add(a[1])
		for i := 1; i < n-1; i++ {
			out.Curves[i].P1 = a[i].Mul(4).Add(a[i+1].Mul(2)).Sub(out.Curves[i-1].P1).Mul(cp[i])
		}
		out.Curves[n-1].P1 = a[n-1].Mul(8).Add(a[n]).Sub(out.Curves[n-2].P1.Mul(2)).Mul(1 / (7 - 2*cp[n-2]))
		for i := n - 2; i >= 0; i-- {
			out.Curves[i].P1 = out.Curves[i].P1.Sub(out.Curves[i+1].P1.Mul(cp[i]))
		}
	} else {
		q := make([]float64, n)
		q[0] = 1.0 / 3
		for i := 1; i < n-1; i++ {
			q[i] = -cp[i] * q[i-1]
		}
		last := 1 / (3 - cp[n-2])
		q[n-1] = last * (1 - q[n-2])
		for i := n - 2; i >= 0; i-- {
			q[i] -= cp[i] * q[i+1]
		}
		out.Curves[0].P1 = a[0].Mul(4).Add(a[1].Mul(2)).Mul(1.0 / 3)
		for i := 1; i < n; i++ {
			weight := last
			if i < n-1 {
				weight = cp[i]
			}
			out.Curves[i].P1 = a[i].Mul(4).Add(a[i+1].Mul(2)).Sub(out.Curves[i-1].P1).Mul(weight)
		}
		for i := n - 2; i >= 0; i-- {
			out.Curves[i].P1 = out.Curves[i].P1.Sub(out.Curves[i+1].P1.Mul(cp[i]))
		}
		factor := out.Curves[0].P1.Add(out.Curves[n-1].P1).Mul(1 / (1 + q[0] + q[n-1]))
		for i := range out.Curves {
			out.Curves[i].P1 = out.Curves[i].P1.Sub(factor.Mul(q[i]))
		}
	}
	for i := 0; i < n-1; i++ {
		out.Curves[i].P2 = a[i+1].Mul(2).Sub(out.Curves[i+1].P1)
	}
	if closed {
		// The actual final anchor is retained even for approximately closed inputs.
		out.Curves[n-1].P2 = a[n].Mul(2).Sub(out.Curves[0].P1)
	} else {
		out.Curves[n-1].P2 = a[n].Add(out.Curves[n-1].P1).Mul(.5)
	}
	return out
}

func editAnchorsClosed(a, b geom.Vec3) bool {
	return math.Abs(b.X-a.X) <= 1e-8+1e-5*math.Abs(a.X) &&
		math.Abs(b.Y-a.Y) <= 1e-8+1e-5*math.Abs(a.Y) &&
		math.Abs(b.Z-a.Z) <= 1e-8+1e-5*math.Abs(a.Z)
}
