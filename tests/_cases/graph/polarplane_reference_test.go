package graph_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"gomath/geom"
	"gomath/graph"
	"math"
	"os"
	"testing"
)

func TestPinnedManimPolarPlaneMappings(t *testing.T) {
	var fixture struct {
		Source struct {
			Version, Commit string
			Generator       string `json:"generator_sha256"`
		}
		Cases []struct {
			Maximum, Step, World float64
			Matrix               geom.Mat4
			Samples              []struct {
				Radius, Azimuth float64
				Point           [3]float64
				Inverse         [2]float64
			}
		}
	}
	data, err := os.ReadFile("../tests/data/graph/manim-polarplane-0.22.1.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Source.Version != "0.22.1" || fixture.Source.Commit != "16ea443e2a5f1d79db0a721161411ce916d6e733" || len(fixture.Cases) != 14 {
		t.Fatal("polar-plane reference provenance/count")
	}
	generator, err := os.ReadFile("../tests/reference/polarplane.py")
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(generator)
	if fixture.Source.Generator != hex.EncodeToString(hash[:]) {
		t.Fatal("polar-plane generator changed")
	}
	for _, tc := range fixture.Cases {
		options := graph.DefaultPolarPlaneOptions()
		options.Rays = 12
		p, err := graph.NewPolarPlane(graph.Range{Min: 0, Max: tc.Maximum, Step: tc.Step}, tc.World, options)
		if err != nil {
			t.Fatal(err)
		}
		p.Object.ApplyMatrix(tc.Matrix)
		if len(tc.Samples) != 24 {
			t.Fatal("polar reference sample count")
		}
		for _, sample := range tc.Samples {
			point, err := p.Point(sample.Radius, sample.Azimuth)
			want := geom.Vec3{X: sample.Point[0], Y: sample.Point[1], Z: sample.Point[2]}
			if err != nil || point.Sub(want).Norm() > 2e-12 {
				t.Fatal("pinned polar forward", sample.Radius, sample.Azimuth, point, want, err)
			}
			r, a, err := p.Coordinates(want)
			if err != nil || math.Abs(r-sample.Inverse[0]) > 2e-12 || sample.Radius != 0 && math.Abs(math.Remainder(a-sample.Inverse[1], 2*math.Pi)) > 2e-12 {
				t.Fatal("pinned polar inverse", sample, r, a, err)
			}
		}
	}
}
