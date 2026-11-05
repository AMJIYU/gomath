package graph_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"gomath"
	"gomath/geom"
	"gomath/graph"
	"os"
	"testing"
)

func TestPinnedManimSignedBarUpdates(t *testing.T) {
	var fixture struct {
		Source struct {
			Version, Commit string
			Generator       string `json:"generator_sha256"`
		}
		Cases []struct {
			Alpha, Angle       float64
			Shift              [3]float64
			Start, End, Values []float64
			Vertices           [][][3]float64
		}
	}
	data, err := os.ReadFile("../tests/data/graph/manim-bars-0.22.1.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Source.Version != "0.22.1" || fixture.Source.Commit != "16ea443e2a5f1d79db0a721161411ce916d6e733" || len(fixture.Cases) != 14 {
		t.Fatal("bar reference provenance/count")
	}
	generator, err := os.ReadFile("../tests/reference/barchart.py")
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(generator)
	if fixture.Source.Generator != hex.EncodeToString(hash[:]) {
		t.Fatal("bar generator changed")
	}
	for _, tc := range fixture.Cases {
		options := graph.DefaultBarChartOptions()
		options.StrokeWidth = 0
		chart, err := graph.NewBarChart(tc.Start, graph.Range{Min: -3, Max: 5, Step: 1}, 8, 4, options)
		if err != nil {
			t.Fatal(err)
		}
		origin, err := chart.Point(0, 0)
		if err != nil {
			t.Fatal(err)
		}
		chart.Object.Shift(origin.Mul(-1)).ApplyMatrix(geom.RotationZ(tc.Angle)).Shift(geom.Vec3{X: tc.Shift[0], Y: tc.Shift[1], Z: tc.Shift[2]})
		target, err := chart.TargetBars(tc.End)
		if err != nil {
			t.Fatal(err)
		}
		scene := gomath.NewScene()
		if err = scene.Add(chart.Object); err != nil {
			t.Fatal(err)
		}
		if err = scene.Play(1, gomath.Transform(chart.BarsObject(), target).WithRate(gomath.Linear)); err != nil {
			t.Fatal(err)
		}
		frame, err := scene.Sample(tc.Alpha)
		if err != nil {
			t.Fatal(err)
		}
		for i, bar := range chart.Bars() {
			actual := frame.Find(bar.ID).Path.Contours[0]
			if len(actual.Curves) != 4 || len(tc.Vertices[i]) != 4 {
				t.Fatal("reference contour count")
			}
			// Manim's Rectangle starts at a different corner. Match the complete
			// vertex multiset, preserving duplicates for zero-height bars.
			used := [4]bool{}
			for _, curve := range actual.Curves {
				found := false
				for j, p := range tc.Vertices[i] {
					if !used[j] && curve.P0.Sub(geom.Vec3{X: p[0], Y: p[1], Z: p[2]}).Norm() <= 2e-12 {
						used[j], found = true, true
						break
					}
				}
				if !found {
					t.Fatalf("alpha %g angle %g bar %d vertex %v does not match Manim %v", tc.Alpha, tc.Angle, i, curve.P0, tc.Vertices[i])
				}
			}
		}
	}
}
