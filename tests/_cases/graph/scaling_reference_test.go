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

func TestPinnedManimScalesAndNumberLine(t *testing.T) {
	var fixture struct {
		Source struct {
			Version, Commit string
			Generator       string `json:"generator_sha256"`
		}
		Cases []struct {
			Kind                     string
			Parameter, Angle, Length float64
			Origin                   [3]float64
			Range                    [3]float64
			Samples                  []struct {
				Coordinate, Value, Inverse float64
				Point                      [3]float64
				ProjectedPoint             [3]float64 `json:"projected_point"`
			}
			Ticks []struct {
				Value  float64
				Center [3]float64
			}
		}
	}
	data, err := os.ReadFile("../tests/data/graph/manim-scales-0.22.1.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Source.Version != "0.22.1" || fixture.Source.Commit != "16ea443e2a5f1d79db0a721161411ce916d6e733" || len(fixture.Cases) != 10 {
		t.Fatal("reference provenance")
	}
	generator, err := os.ReadFile("../tests/reference/scales.py")
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(generator)
	if fixture.Source.Generator != hex.EncodeToString(hash[:]) {
		t.Fatal("generator changed; regenerate/review reference")
	}
	vec := func(p [3]float64) geom.Vec3 { return geom.Vec3{X: p[0], Y: p[1], Z: p[2]} }
	close := func(a, b float64) bool { return math.Abs(a-b) <= 2e-13*math.Max(1, math.Max(math.Abs(a), math.Abs(b))) }
	for _, tc := range fixture.Cases {
		var scale graph.Scale
		if tc.Kind == "linear" {
			scale, err = graph.NewLinearScale(tc.Parameter)
		} else {
			scale, err = graph.NewLogScale(tc.Parameter)
		}
		if err != nil {
			t.Fatal(err)
		}
		opt := graph.DefaultNumberLineOptions()
		opt.Direction = geom.V(math.Cos(tc.Angle), math.Sin(tc.Angle))
		opt.Origin = vec(tc.Origin)
		line, err := graph.NewScaledNumberLine(graph.Range{Min: tc.Range[0], Max: tc.Range[1], Step: tc.Range[2]}, tc.Length, scale, opt)
		if err != nil {
			t.Fatal(err)
		}
		if len(tc.Samples) != 9 || len(tc.Ticks) != 6 {
			t.Fatal("reference counts")
		}
		for _, sample := range tc.Samples {
			v, err := scale.Forward(sample.Coordinate)
			if err != nil || !close(v, sample.Value) {
				t.Fatal(tc.Kind, tc.Parameter, v, sample.Value, err)
			}
			u, err := scale.Inverse(sample.Value)
			if err != nil || !close(u, sample.Coordinate) {
				t.Fatal("inverse", u, sample.Coordinate, err)
			}
			p, err := line.Point(sample.Value)
			if err != nil || p.Sub(vec(sample.Point)).Norm() > 2e-12 {
				t.Fatal("point", p, sample.Point, err)
			}
			value, err := line.Coordinate(vec(sample.ProjectedPoint))
			if err != nil || !close(value, sample.Inverse) {
				t.Fatal("projected inverse", value, sample.Inverse, err)
			}
		}
		for i, tick := range line.Ticks() {
			want := tc.Ticks[i]
			if !close(tick.Value, want.Value) || tick.Tick.Center().Sub(vec(want.Center)).Norm() > 2e-12 {
				t.Fatal("tick", tick.Value, want.Value)
			}
		}
	}
}
