package graph_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"gomath/geom"
	"gomath/graph"
	"math/cmplx"
	"os"
	"testing"
)

func TestPinnedManimComplexPlaneMappings(t *testing.T) {
	var fixture struct {
		Source struct {
			Version, Commit string
			Generator       string `json:"generator_sha256"`
		}
		Cases []struct {
			X, Y          [3]float64
			Width, Height float64
			Matrix        geom.Mat4
			Samples       []struct {
				Number, Inverse [2]float64
				Point           [3]float64
				OffPlane        [3]float64 `json:"off_plane"`
			}
		}
	}
	data, err := os.ReadFile("../tests/data/graph/manim-complexplane-0.22.1.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Source.Version != "0.22.1" || fixture.Source.Commit != "16ea443e2a5f1d79db0a721161411ce916d6e733" || len(fixture.Cases) != 14 {
		t.Fatal("complex-plane reference provenance/count")
	}
	generator, err := os.ReadFile("../tests/reference/complexplane.py")
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(generator)
	if fixture.Source.Generator != hex.EncodeToString(hash[:]) {
		t.Fatal("complex-plane generator changed")
	}
	for _, tc := range fixture.Cases {
		c, err := graph.NewComplexPlane(graph.Range{Min: tc.X[0], Max: tc.X[1], Step: tc.X[2]}, graph.Range{Min: tc.Y[0], Max: tc.Y[1], Step: tc.Y[2]}, tc.Width, tc.Height, graph.DefaultNumberPlaneOptions())
		if err != nil {
			t.Fatal(err)
		}
		c.Object.ApplyMatrix(tc.Matrix)
		if len(tc.Samples) != 5 {
			t.Fatal("complex reference sample count")
		}
		for _, sample := range tc.Samples {
			z := complex(sample.Number[0], sample.Number[1])
			p, err := c.Point(z)
			want := geom.Vec3{X: sample.Point[0], Y: sample.Point[1], Z: sample.Point[2]}
			if err != nil || p.Sub(want).Norm() > 2e-12 {
				t.Fatal("pinned n2p", z, p, want, err)
			}
			back, err := c.Number(geom.Vec3{X: sample.OffPlane[0], Y: sample.OffPlane[1], Z: sample.OffPlane[2]})
			if err != nil || cmplx.Abs(back-complex(sample.Inverse[0], sample.Inverse[1])) > 2e-12 {
				t.Fatal("pinned p2n", z, back, sample.Inverse, err)
			}
		}
	}
}
