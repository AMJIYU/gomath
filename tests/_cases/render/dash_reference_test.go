package render

import (
	"bytes"
	"encoding/json"
	"gomath"
	"gomath/geom"
	"gomath/path"
	"gomath/svg"
	"image/color"
	"image/png"
	"math"
	"os"
	"sort"
	"strings"
	"testing"
)

func TestDashCairoSVGReference(t *testing.T) {
	for _, tc := range []struct {
		name          string
		width, height int
	}{{"dash-cascade", 240, 180}, {"dash-square-dots", 128, 128}, {"dash-edge-cases", 200, 160}} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := os.ReadFile("../tests/data/svg/" + tc.name + ".svg")
			if err != nil {
				t.Fatal(err)
			}
			root, err := svg.Read(strings.NewReader(string(data)), svg.Options{Width: float64(tc.width)})
			if err != nil {
				t.Fatal(err)
			}
			c := DefaultConfig()
			c.Width, c.Height, c.FrameWidth, c.Workers, c.Background = tc.width, tc.height, float64(tc.width), 1, gomath.Transparent
			r, err := NewRasterizer(c)
			if err != nil {
				t.Fatal(err)
			}
			actual, err := r.Render(&gomath.Frame{Objects: []*gomath.Object{root}})
			if err != nil {
				t.Fatal(err)
			}
			data, err = os.ReadFile("../tests/data/svg/" + tc.name + ".png")
			if err != nil {
				t.Fatal(err)
			}
			reference, err := png.Decode(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			total, maxDiff, pixels := 0.0, 0, 0
			for y := range tc.height {
				for x := range tc.width {
					a, b := actual.RGBAAt(x, y), color.RGBAModel.Convert(reference.At(x, y)).(color.RGBA)
					changed := false
					for _, d := range []int{int(a.R) - int(b.R), int(a.G) - int(b.G), int(a.B) - int(b.B), int(a.A) - int(b.A)} {
						delta := int(math.Abs(float64(d)))
						total += float64(delta)
						maxDiff = max(maxDiff, delta)
						changed = changed || delta > 8
					}
					if changed {
						pixels++
					}
				}
			}
			mean, ratio := total/float64(tc.width*tc.height*4), float64(pixels)/float64(tc.width*tc.height)
			t.Logf("CairoSVG %s: MAE=%g max=%d fraction_over_8=%g", tc.name, mean, maxDiff, ratio)
			if mean > .1 || maxDiff > 32 || ratio > .005 {
				t.Fatalf("dash reference differs: MAE=%g max=%d ratio=%g", mean, maxDiff, ratio)
			}
		})
	}
}
func TestDashPinnedManimGeometry(t *testing.T) {
	var ref struct {
		Cases []struct {
			Shape    string         `json:"shape"`
			Closed   bool           `json:"closed"`
			Vertices [][2]float64   `json:"base_vertices"`
			Pattern  []float64      `json:"native_pattern"`
			Offset   float64        `json:"native_offset"`
			Dashes   [][][2]float64 `json:"dashes"`
		}
	}
	data, err := os.ReadFile("../tests/data/svg/dash-manim.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &ref); err != nil {
		t.Fatal(err)
	}
	for _, tc := range ref.Cases {
		line := path.Polyline{Closed: tc.Closed}
		for _, p := range tc.Vertices {
			line.Points = append(line.Points, geom.V(p[0], p[1]))
		}
		got, err := dashLines([]path.Polyline{line}, dashPattern(t, tc.Pattern, tc.Offset))
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != len(tc.Dashes) {
			t.Fatalf("%s offset %g: dash count %d want %d", tc.Shape, tc.Offset, len(got), len(tc.Dashes))
		}
		key := func(a, b [2]float64) bool {
			if math.Abs(a[0]-b[0]) > 1e-9 {
				return a[0] < b[0]
			}
			return a[1] < b[1]
		}
		sort.Slice(got, func(i, j int) bool {
			a, b := got[i].Points[0], got[j].Points[0]
			return key([2]float64{a.X, a.Y}, [2]float64{b.X, b.Y})
		})
		sort.Slice(tc.Dashes, func(i, j int) bool { return key(tc.Dashes[i][0], tc.Dashes[j][0]) })
		for i, piece := range got {
			want := tc.Dashes[i]
			if len(piece.Points) != len(want) {
				t.Fatalf("%s offset%g piece%d lost joins: %v want %v", tc.Shape, tc.Offset, i, piece.Points, want)
			}
			for j, p := range piece.Points {
				if math.Hypot(p.X-want[j][0], p.Y-want[j][1]) > 1e-9 {
					t.Fatalf("%s offset%g piece%d point%d %v want %v", tc.Shape, tc.Offset, i, j, p, want[j])
				}
			}
		}
	}
}
