package path

import (
	"math/big"
	"strings"
	"testing"
)

func TestBooleanRoundingRejectsThinLoopOrientationReversal(t *testing.T) {
	// Three distinct rational vertices round to three distinct float64 vertices
	// with reversed orientation, without introducing a segment crossing. Vertex
	// collisions and cyclic ray order alone cannot detect this thin-loop failure.
	const d int64 = 1 << 55
	e := newBooleanEngine(DefaultBooleanOptions().Limits)
	for _, xy := range [][2]int64{{0, 0}, {d + 3, d - 1}, {2*d - 2, 2*d - 6}} {
		e.point(booleanPoint{big.NewRat(xy[0], d), big.NewRat(xy[1], d)})
	}
	if e.orient(e.points[0], e.points[1], e.points[2]).Sign() != 1 {
		t.Fatal("fixture is not counterclockwise")
	}
	var edges []booleanEdge
	for i := range 3 {
		j := (i + 1) % 3
		edges = append(edges, booleanEdge{a: i, b: j, delta: e.vec(e.points[j], e.points[i])})
	}
	err := e.verifyRounding(edges)
	if err == nil || !strings.Contains(err.Error(), "winding or orientation") {
		t.Fatal("rounded reversed loop was accepted", err)
	}
}
