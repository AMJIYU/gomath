package table_test

import (
	"gomath"
	"gomath/geom"
	"gomath/table"
	"math"
	"testing"
)

func TestLabelsOwnGeometryAndKeepDataIndices(t *testing.T) {
	for _, rowHeader := range []bool{false, true} {
		for _, columnHeader := range []bool{false, true} {
			for _, corner := range []bool{false, true} {
				if corner && (!rowHeader || !columnHeader) {
					continue
				}
				opt := table.DefaultOptions()
				source := gomath.Rectangle(.8, .5).MoveTo(geom.V(7, 8))
				if rowHeader {
					opt.RowLabels = []*gomath.Object{source, source}
				}
				if columnHeader {
					opt.ColumnLabels = []*gomath.Object{source, source}
				}
				if corner {
					opt.Corner = source
				}
				tab, err := table.NewObjects([][]*gomath.Object{{source, source}, {source, source}}, opt)
				if err != nil {
					t.Fatal(err)
				}
				if source.Center().Sub(geom.V(7, 8)).Norm() > 1e-12 {
					t.Fatal("source changed")
				}
				r, c := tab.Dimensions()
				if r != 2 || c != 2 {
					t.Fatal("headers changed data dimensions")
				}
				rOffset, cOffset := 0, 0
				if columnHeader {
					rOffset = 1
				}
				if rowHeader {
					cOffset = 1
				}
				gr, gc := tab.GridDimensions()
				if gr != r+rOffset || gc != c+cOffset {
					t.Fatal("wrong grid dimensions")
				}
				if len(tab.Lines().Children) != gr+gc+2 {
					t.Fatal("missing header rules")
				}
				seen := map[*gomath.Object]bool{source: true}
				for i := range gr {
					for j := range gc {
						entry, err := tab.GridEntry(i, j)
						if err != nil || seen[entry] {
							t.Fatal("layout entry alias", err)
						}
						seen[entry] = true
					}
				}
				entry, _ := tab.Entry(0, 0)
				gridEntry, _ := tab.GridEntry(rOffset, cOffset)
				background, _ := tab.CellBackground(0, 0)
				gridBackground, _ := tab.GridCellBackground(rOffset, cOffset)
				if entry != gridEntry || background != gridBackground {
					t.Fatal("data offset incorrect")
				}
				if err := tab.HighlightCell(0, 0, gomath.Blue); err != nil {
					t.Fatal(err)
				}
				if background.Style.Fill != gomath.Blue {
					t.Fatal("highlight missed data cell")
				}
				row, _ := tab.Row(0)
				column, _ := tab.Column(0)
				if len(row) != 2 || len(column) != 2 || row[0] != entry || column[0] != entry {
					t.Fatal("data selections contain labels")
				}
				if rowHeader {
					labels := tab.RowLabels()
					original := labels[0]
					labels[0] = nil
					if tab.RowLabels()[0] != original {
						t.Fatal("selection slice leaked")
					}
					gridLabel, _ := tab.GridEntry(rOffset, 0)
					if original != gridLabel {
						t.Fatal("not actual row label")
					}
				}
				if columnHeader {
					labels := tab.ColumnLabels()
					original := labels[0]
					labels[0] = nil
					if tab.ColumnLabels()[0] != original {
						t.Fatal("selection slice leaked")
					}
					gridLabel, _ := tab.GridEntry(0, cOffset)
					if original != gridLabel {
						t.Fatal("not actual column label")
					}
				}
				wantLabels := 2*rOffset + 2*cOffset
				if corner {
					wantLabels++
				}
				if len(tab.Labels()) != wantLabels || (tab.Corner() != nil) != corner {
					t.Fatal("wrong label selection")
				}
				if rowHeader && columnHeader && !corner {
					blank, _ := tab.GridEntry(0, 0)
					if len(blank.Path.Contours) != 0 || len(blank.Children) != 0 {
						t.Fatal("missing corner painted content")
					}
				}
				labels := tab.Labels()
				scene := gomath.NewScene()
				if err := scene.Add(tab.Object); err != nil {
					t.Fatal(err)
				}
				before := entry.Center()
				if err := scene.Play(1, gomath.Shift(tab.Object, geom.V(1, -2))); err != nil {
					t.Fatal(err)
				}
				if after, _ := tab.Entry(0, 0); after != entry || after.Center().Sub(before.Add(geom.V(1, -2))).Norm() > 1e-10 {
					t.Fatal("data reference lost")
				}
				for i, label := range labels {
					if label != tab.Labels()[i] {
						t.Fatal("label reference lost")
					}
				}
				for _, time := range []float64{0, .3, 1, .1} {
					if _, err := scene.Sample(time); err != nil {
						t.Fatal(err)
					}
				}
			}
		}
	}
}

func TestLabelValidationBeforeLayout(t *testing.T) {
	source := gomath.Circle(.3).MoveTo(geom.V(7, 8))
	for _, modify := range []func(*table.Options){
		func(o *table.Options) { o.RowLabels = []*gomath.Object{source} },
		func(o *table.Options) { o.ColumnLabels = []*gomath.Object{source} },
		func(o *table.Options) { o.Corner = source },
		func(o *table.Options) { o.RowLabels = []*gomath.Object{source, nil} },
		func(o *table.Options) { o.ColumnLabels = []*gomath.Object{source, nil} },
		func(o *table.Options) {
			o.RowLabels = []*gomath.Object{source, source}
			o.ColumnLabels = []*gomath.Object{source, source}
			o.Corner = gomath.Square(math.Inf(1))
		},
	} {
		opt := table.DefaultOptions()
		modify(&opt)
		if _, err := table.NewObjects([][]*gomath.Object{{source, source}, {source, source}}, opt); err == nil {
			t.Fatal("invalid label configuration accepted")
		}
		if source.Center().Sub(geom.V(7, 8)).Norm() > 1e-12 {
			t.Fatal("failed construction mutated source")
		}
	}
	opt := table.DefaultOptions()
	opt.RowLabels = []*gomath.Object{source}
	opt.ColumnLabels = []*gomath.Object{source}
	tab, err := table.New([][]string{{"x"}}, opt)
	if err != nil {
		t.Fatal(err)
	}
	for _, position := range [][2]int{{-1, 0}, {0, -1}, {1, 0}, {0, 1}, {math.MaxInt, 0}} {
		if _, err := tab.Entry(position[0], position[1]); err == nil {
			t.Fatal("data index accepted", position)
		}
		if _, err := tab.CellBackground(position[0], position[1]); err == nil {
			t.Fatal("background index accepted", position)
		}
	}
	if _, err := tab.GridCellBackground(2, 0); err == nil {
		t.Fatal("grid background index accepted")
	}
	oversized := make([][]string, 257)
	for i := range oversized {
		oversized[i] = make([]string, 256)
	}
	if _, err := table.New(oversized, table.DefaultOptions()); err == nil {
		t.Fatal("layout budget ignored")
	}
}
