package gomath_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"gomath"
	"math"
	"os"
	"strconv"
	"testing"
)

func TestPinnedManimElbowOrderedTransformStates(t *testing.T) {
	var fixture struct {
		Source struct {
			Version, Commit string
			Hash            string `json:"generator_sha256"`
		}
		Cases []struct {
			Width, Angle float64
			States       []connectionReferenceState
		}
	}
	data, err := os.ReadFile("tests/data/root/manim-elbow-0.22.1.json")
	must(t, err)
	must(t, json.Unmarshal(data, &fixture))
	generator, err := os.ReadFile("tests/reference/elbow.py")
	must(t, err)
	hash := sha256.Sum256(generator)
	if fixture.Source.Version != "0.22.1" || fixture.Source.Commit != "16ea443e2a5f1d79db0a721161411ce916d6e733" || fixture.Source.Hash != hex.EncodeToString(hash[:]) || len(fixture.Cases) != 15 {
		t.Fatal("elbow provenance or count differs")
	}
	for i, ref := range fixture.Cases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			object, err := gomath.Elbow(ref.Width, ref.Angle)
			must(t, err)
			must(t, object.Validate())
			checkConnectionTransformStates(t, object, ref.States)
		})
	}
}

func TestElbowPreservesSideLengthsAndRejectsOutOfRange(t *testing.T) {
	for _, width := range []float64{1e-9, .2, 1, 1e6, 1e12} {
		for _, angle := range []float64{0, .37, -.92, 1e20} {
			object, err := gomath.Elbow(width, angle)
			if err != nil {
				if width != 1e12 {
					t.Fatal(err)
				}
				continue // A rotated corner may exceed the world-coordinate cap.
			}
			curves := object.Path.Contours[0].Curves
			a, b := curves[0].P3.Sub(curves[0].P0), curves[1].P3.Sub(curves[1].P0)
			if math.Abs(a.Norm()/width-1) > 1e-14 || math.Abs(b.Norm()/width-1) > 1e-14 || math.Abs(a.Dot(b)/(width*width)) > 1e-14 {
				t.Fatal("elbow side geometry differs", width, angle)
			}
		}
	}
	for _, pair := range [][2]float64{{0, 0}, {-1, 0}, {math.NaN(), 0}, {1, math.Inf(1)}, {1e12 + 1, 0}, {1e12, math.Pi / 4}} {
		if got, err := gomath.Elbow(pair[0], pair[1]); got != nil || err == nil {
			t.Fatal("invalid elbow accepted", pair)
		}
	}
}
