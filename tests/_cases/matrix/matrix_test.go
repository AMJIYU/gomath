package matrix_test

import (
	"gomath"
	"gomath/geom"
	"gomath/matrix"
	"gomath/render"
	"math"
	"testing"
)

func TestFormulaLayoutAndSelections(t *testing.T) {
	m, err := matrix.New([][]string{{`\frac{1}{2}`, `x^{2}`}, {`-123`, `\sqrt{y}`}}, matrix.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if r, c := m.Dimensions(); r != 2 || c != 2 {
		t.Fatal(r, c)
	}
	for i := range 2 {
		row, err := m.Row(i)
		if err != nil || row[0].Bounds().Max.X >= row[1].Bounds().Min.X {
			t.Fatal("columns overlap", err)
		}
		row[0] = nil
		if entry, _ := m.Entry(i, 0); entry == nil {
			t.Fatal("selection slice aliases storage")
		}
	}
	column, _ := m.Column(0)
	if column[0].Bounds().Min.Y <= column[1].Bounds().Max.Y {
		t.Fatal("rows overlap")
	}
	if err := m.SetRowColor(0, gomath.Pink); err != nil {
		t.Fatal(err)
	}
	if err := m.SetColumnColor(1, gomath.Blue); err != nil {
		t.Fatal(err)
	}
	column, _ = m.Column(1)
	for _, o := range column {
		o.Walk(func(p *gomath.Object) {
			if p.Style.Fill.A > 0 && p.Style.Fill != gomath.Blue {
				t.Fatal("column color not applied to glyphs")
			}
		})
	}
	if err := m.SetRowColor(0, gomath.Color{R: math.NaN()}); err == nil {
		t.Fatal("invalid color accepted")
	}
	for _, call := range []func() error{
		func() error { _, e := m.Entry(-1, 0); return e },
		func() error { _, e := m.Row(2); return e },
		func() error { _, e := m.Column(2); return e },
	} {
		if call() == nil {
			t.Fatal("invalid selection accepted")
		}
	}
}

func TestAllBracketsRenderAndEnclose(t *testing.T) {
	for _, kind := range []matrix.Bracket{matrix.Square, matrix.Round, matrix.Curly, matrix.Angle, matrix.Bar, matrix.None} {
		t.Run(string(kind), func(t *testing.T) {
			opt := matrix.DefaultOptions()
			opt.Bracket = kind
			m, err := matrix.New([][]string{{"1", ""}, {`\frac{2}{3}`, "4"}, {"5", "6"}}, opt)
			if err != nil {
				t.Fatal(err)
			}
			brackets := m.Brackets()
			if kind != matrix.None {
				left, right := brackets.Children[0].Bounds(), brackets.Children[1].Bounds()
				if math.Abs(left.Min.X+right.Max.X) > 1e-10 || math.Abs(left.Max.X+right.Min.X) > 1e-10 {
					t.Fatal("brackets are not mirrors")
				}
				for i := range 3 {
					for j := range 2 {
						o, _ := m.Entry(i, j)
						b := o.Bounds()
						if b.Valid && (b.Min.X <= left.Max.X || b.Max.X >= right.Min.X || b.Min.Y <= left.Min.Y || b.Max.Y >= left.Max.Y) {
							t.Fatal("entry not enclosed", i, j)
						}
					}
				}
			} else if len(brackets.Children) != 0 {
				t.Fatal("none has visible brackets")
			}
			c := render.DefaultConfig()
			c.Width, c.Height = 192, 108
			r, err := render.NewRasterizer(c)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := r.Render(&gomath.Frame{Objects: []*gomath.Object{m.Object}}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCopiesAndAnimationIdentity(t *testing.T) {
	source := gomath.Square(.5).MoveTo(geom.V(10, 20))
	m, err := matrix.NewObjects([][]*gomath.Object{{source, source}}, matrix.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	a, _ := m.Entry(0, 0)
	b, _ := m.Entry(0, 1)
	if a == b || a.ID == b.ID || a.ID == source.ID || source.Center() != geom.V(10, 20) {
		t.Fatal("inputs not independently copied")
	}
	s := gomath.NewScene()
	if err = s.Add(m.Object); err != nil {
		t.Fatal(err)
	}
	if err = s.Play(1, gomath.Transform(a, a.Copy().SetFill(gomath.Blue))); err != nil {
		t.Fatal(err)
	}
	start := a.Center()
	if err = s.Play(1, gomath.Shift(m.Object, geom.V(0, 2)).WithRate(gomath.Linear)); err != nil {
		t.Fatal(err)
	}
	if current, _ := m.Entry(0, 0); current != a || a.Center().Sub(start.Add(geom.V(0, 2))).Norm() > 1e-9 {
		t.Fatal("whole-matrix transform lost entry reference")
	}
	for _, time := range []float64{1.5, .5, 1.5} {
		f, err := s.Sample(time)
		if err != nil {
			t.Fatal(err)
		}
		entry := f.Find(a.ID)
		if entry == nil || entry == a {
			t.Fatal("sample does not isolate matrix entry")
		}
		if time == 1.5 && entry.Center().Sub(start.Add(geom.V(0, 1))).Norm() > 1e-9 {
			t.Fatal("mid-animation position incorrect")
		}
		entry.Shift(geom.V(99, 99))
	}
}

func TestFormattingAndErrors(t *testing.T) {
	m, err := matrix.NewIntegers([][]int64{{math.MinInt64, math.MaxInt64}}, matrix.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	entry, _ := m.Entry(0, 0)
	if entry.Name != "-9223372036854775808" {
		t.Fatal(entry.Name)
	}
	m, err = matrix.NewDecimals([][]float64{{1.234, -.5}}, 2, matrix.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	entry, _ = m.Entry(0, 0)
	if entry.Name != "1.23" {
		t.Fatal(entry.Name)
	}
	for _, input := range [][][]string{nil, {{}}, {{"1"}, {"2", "3"}}, {{`\unsupported`}}} {
		if _, err := matrix.New(input, matrix.DefaultOptions()); err == nil {
			t.Fatal("invalid matrix accepted", input)
		}
	}
	for _, digits := range []int{-1, 16} {
		if _, err := matrix.NewDecimals([][]float64{{1}}, digits, matrix.DefaultOptions()); err == nil {
			t.Fatal("invalid precision accepted")
		}
	}
	if _, err := matrix.NewDecimals([][]float64{{math.Inf(1)}}, 2, matrix.DefaultOptions()); err == nil {
		t.Fatal("nonfinite decimal accepted")
	}
	opt := matrix.DefaultOptions()
	opt.Bracket = "unsupported"
	if _, err := matrix.New([][]string{{"1"}}, opt); err == nil {
		t.Fatal("unknown delimiter accepted")
	}
}
