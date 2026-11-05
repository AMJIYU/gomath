package table_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"gomath"
	"gomath/geom"
	"gomath/table"
	"math"
	"os"
	"testing"
)

func TestManimLabelGeometryReference(t *testing.T) {
	var fixture struct {
		Source struct {
			Version, Commit string
			Generator       string `json:"generator_sha256"`
		}
		Cases []struct {
			Name    string
			Data    [][][2]float64
			Rows    [][2]float64 `json:"row_labels"`
			Columns [][2]float64 `json:"column_labels"`
			Corner  *[2]float64
			Angle   float64
			Shift   [3]float64
			Entries []struct {
				Row, Column int
				Center      [3]float64
				Size        [2]float64
			}
			Rules []struct{ Start, End [3]float64 }
		}
	}
	data, err := os.ReadFile("../tests/data/table/manim-labels-0.22.1.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Source.Version != "0.22.1" || fixture.Source.Commit != "16ea443e2a5f1d79db0a721161411ce916d6e733" || len(fixture.Cases) != 15 {
		t.Fatal("reference provenance mismatch")
	}
	generator, err := os.ReadFile("../tests/reference/table_labels.py")
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(generator)
	if fixture.Source.Generator != hex.EncodeToString(hash[:]) {
		t.Fatal("reference generator changed; regenerate and review fixture")
	}
	vector := func(p [3]float64) geom.Vec3 { return geom.Vec3{X: p[0], Y: p[1], Z: p[2]} }
	for _, tc := range fixture.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			objects := make([][]*gomath.Object, len(tc.Data))
			for i, row := range tc.Data {
				objects[i] = make([]*gomath.Object, len(row))
				for j, size := range row {
					objects[i][j] = gomath.Rectangle(size[0], size[1])
				}
			}
			opt := table.DefaultOptions()
			opt.MinCellWidth, opt.MinCellHeight = 0, 0
			for _, size := range tc.Rows {
				opt.RowLabels = append(opt.RowLabels, gomath.Rectangle(size[0], size[1]))
			}
			for _, size := range tc.Columns {
				opt.ColumnLabels = append(opt.ColumnLabels, gomath.Rectangle(size[0], size[1]))
			}
			if tc.Corner != nil {
				opt.Corner = gomath.Rectangle(tc.Corner[0], tc.Corner[1])
			}
			tab, err := table.NewObjects(objects, opt)
			if err != nil {
				t.Fatal(err)
			}
			tab.ApplyMatrix(geom.RotationZ(tc.Angle)).Shift(vector(tc.Shift))
			for _, want := range tc.Entries {
				entry, err := tab.GridEntry(want.Row, want.Column)
				if err != nil {
					t.Fatal(err)
				}
				bounds := entry.Bounds()
				if bounds.Center().Sub(vector(want.Center)).Norm() > 1e-11 || math.Abs(bounds.Size().X-want.Size[0]) > 1e-11 || math.Abs(bounds.Size().Y-want.Size[1]) > 1e-11 {
					t.Fatalf("entry [%d,%d] center/size mismatch: %v %v", want.Row, want.Column, bounds.Center(), bounds.Size())
				}
			}
			if len(tc.Rules) != len(tab.Lines().Children) {
				t.Fatal("rule count mismatch")
			}
			matched := make([]bool, len(tc.Rules))
			for _, line := range tab.Lines().Children {
				curves := line.Path.Contours[0].Curves
				a, b := curves[0].P0, curves[len(curves)-1].P3
				found := false
				for i, want := range tc.Rules {
					if matched[i] {
						continue
					}
					wa, wb := vector(want.Start), vector(want.End)
					if a.Sub(wa).Norm() < 1e-11 && b.Sub(wb).Norm() < 1e-11 || a.Sub(wb).Norm() < 1e-11 && b.Sub(wa).Norm() < 1e-11 {
						matched[i] = true
						found = true
						break
					}
				}
				if !found {
					t.Fatalf("unmatched rule %v -> %v", a, b)
				}
			}
		})
	}
}
