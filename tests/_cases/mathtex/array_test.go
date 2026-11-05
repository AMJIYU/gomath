package mathtex

import (
	"fmt"
	"golang.org/x/image/font/gofont/goregular"
	"gomath"
	"gomath/geom"
	"gomath/render"
	"gomath/text"
	"math"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func arrayLayout(t *testing.T, s string) box {
	t.Helper()
	b, err := layout(s, DefaultOptions())
	if err != nil {
		t.Fatal(s, err)
	}
	return b
}
func arrayCurveCount(o *gomath.Object) int {
	n := 0
	o.Walk(func(q *gomath.Object) {
		for _, c := range q.Path.Contours {
			n += len(c.Curves)
		}
	})
	return n
}
func arrayCell(t *testing.T, b box, i, j int) *gomath.Object {
	t.Helper()
	n := named(b.object, fmt.Sprintf("array cell[%d,%d]", i, j))
	if n == nil {
		t.Fatal("missing array cell", i, j)
	}
	return n
}
func arrayRule(t *testing.T, b box, axis string, i int) *gomath.Object {
	t.Helper()
	n := named(b.object, fmt.Sprintf("Array%sRule[%d]", axis, i))
	if n == nil || len(n.Path.Contours) != 1 || len(n.Path.Contours[0].Curves) != 1 {
		t.Fatal("missing real vector rule", axis, i)
	}
	return n
}
func TestArrayLiteralSpecAndExactLayoutPolicy(t *testing.T) {
	const source = `\begin{array}{|l|c|r|}\hline n&g&W\\\hline W&i&g\\ i&W&n\\\hline\end{array}`
	b := arrayLayout(t, source)
	s := DefaultOptions().Size
	operands := [][]string{{"n", "g", "W"}, {"W", "i", "g"}, {"i", "W", "n"}}
	widths := make([]float64, 3)
	aa, dd := make([]float64, 3), make([]float64, 3)
	for i, row := range operands {
		aa[i], dd[i] = .7*s, .2*s
		for j, v := range row {
			c := arrayLayout(t, v)
			widths[j] = math.Max(widths[j], c.width)
			aa[i] = math.Max(aa[i], c.height)
			dd[i] = math.Max(dd[i], c.depth)
		}
	}
	stroke, pad, gap := .045*s, .25*s, .3*s
	total := .3*s + 2*gap + 3*stroke
	w := 4*stroke + 6*pad
	for _, v := range widths {
		w += v
	}
	for i := range aa {
		total += aa[i] + dd[i]
	}
	near(t, b.width, w)
	near(t, b.height, .16*s+total/2)
	near(t, b.depth, total-b.height)
	x, y := stroke+pad, b.height-stroke-.15*s
	for i, row := range operands {
		y -= aa[i]
		x = stroke + pad
		for j, v := range row {
			c := arrayLayout(t, v)
			dx := 0.0
			if j == 1 {
				dx = (widths[j] - c.width) / 2
			}
			if j == 2 {
				dx = widths[j] - c.width
			}
			a, z := arrayCell(t, b, i, j).Bounds(), c.object.Bounds()
			near(t, a.Min.X-z.Min.X, x+dx)
			near(t, a.Min.Y-z.Min.Y, y)
			x += widths[j] + 2*pad + stroke
		}
		y -= dd[i]
		if i < 2 {
			y -= gap
			if i == 0 {
				y -= stroke
			}
		}
	}
	vx := stroke / 2
	for j := range 4 {
		rule := arrayRule(t, b, "V", j)
		v := rule.Path.Contours[0].Curves[0]
		near(t, v.P0.X, vx)
		near(t, v.P3.X, vx)
		near(t, v.P0.Y, b.height)
		near(t, v.P3.Y, -b.depth)
		near(t, rule.Style.StrokeWidth, stroke)
		if rule.Style.StrokeCap != gomath.CapButt {
			t.Fatal("rule must stay in reserved band")
		}
		if j < 3 {
			vx += widths[j] + 2*pad + stroke
		}
	}
	for i, want := range map[int]float64{0: b.height - stroke/2, 1: b.height - stroke - .15*s - aa[0] - dd[0] - gap/2 - stroke/2, 3: -b.depth + stroke/2} {
		v := arrayRule(t, b, "H", i).Path.Contours[0].Curves[0]
		near(t, v.P0.X, 0)
		near(t, v.P3.X, b.width)
		near(t, v.P0.Y, want)
		near(t, v.P3.Y, want)
	}
	if named(b.object, "ArrayHRule[2]") != nil {
		t.Fatal("unrequested horizontal rule")
	}
	a := arrayLayout(t, `\begin{array}{lcr}n&g&W\\W&i&g\end{array}`)
	tail := arrayLayout(t, `\begin{array}{ l c r }n&g&W\\W&i&g\\\end{array}`)
	near(t, a.width, tail.width)
	near(t, a.height, tail.height)
	near(t, a.depth, tail.depth)
	requireSameDrawablePaths(t, a.object, tail.object)
}
func TestArrayNestedCellsScopesAndPhantoms(t *testing.T) {
	for _, s := range []string{
		`\left[\begin{array}{l|r}\frac{n}{g}&\sqrt[\hat{n}]{g}\\\hline\begin{matrix}a&b\\c&d\end{matrix}&\begin{aligned}n&g\\a&b\end{aligned}\end{array}\right]`,
		`x^{\begin{array}{l|r}\mathbf{n}&\mathit{g}\\\hline i&j\end{array}}`,
		`\boxed{\begin{array}{|l|c|}\hline\mathbb{R}&\mathcal{A}\\\hline\phantom{\frac{n}{g}}&\begin{array}{r}W\\g\end{array}\\\hline\end{array}}`,
		`\begin{gathered}\begin{array}{c}g\\n\end{array}\\\begin{array}{l|r}a&b\end{array}\end{gathered}`,
	} {
		b := arrayLayout(t, s)
		if err := b.object.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	for _, kind := range []string{"phantom", "hphantom", "vphantom"} {
		body := arrayLayout(t, `\begin{array}{|l|r|}\hline g&W\\\hline\end{array}`)
		hidden := arrayLayout(t, `\`+kind+`{\begin{array}{|l|r|}\hline g&W\\\hline\end{array}}`)
		if hidden.object.Bounds().Valid || arrayCurveCount(hidden.object) != 0 {
			t.Fatal("phantom array leaked ink")
		}
		if kind != "vphantom" {
			near(t, hidden.width, body.width)
		}
		if kind != "hphantom" {
			near(t, hidden.height, body.height)
			near(t, hidden.depth, body.depth)
		}
	}
	b := arrayLayout(t, `\color{red}\begin{array}{|l|r|}\hline\color{blue}a&b\\\hline c&\textcolor{green}{d}\\\hline\end{array}e`)
	blue := arrayLayout(t, `\textcolor{blue}{a}`).object
	red := arrayLayout(t, `\textcolor{red}{b}`).object
	green := arrayLayout(t, `\textcolor{green}{d}`).object
	for _, tc := range []struct {
		i, j int
		want *gomath.Object
	}{{0, 0, blue}, {0, 1, red}, {1, 0, red}, {1, 1, green}} {
		if !reflect.DeepEqual(mathPaints(arrayCell(t, b, tc.i, tc.j)), mathPaints(tc.want)) {
			t.Fatal("cell color scope leak", tc.i, tc.j)
		}
	}
	for _, name := range []string{"ArrayVRule[0]", "ArrayHRule[0]", "ArrayHRule[1]", "ArrayHRule[2]"} {
		if named(b.object, name).Style.Stroke != gomath.RGB(1, 0, 0) {
			t.Fatal("cell color leaked into rules")
		}
	}
	b = arrayLayout(t, `\begin{array}{cc}\displaystyle\sum_i^n&\sum_i^n\end{array}\sum_i^n`)
	// Explicit display is local to one cell; the sibling matches standalone textstyle.
	cell := arrayCell(t, b, 0, 1)
	want := arrayLayout(t, `\sum_i^n`)
	near(t, cell.Bounds().Size().Y, want.object.Bounds().Size().Y)
	inherited := arrayLayout(t, `\displaystyle\begin{array}{c}\sum_i^n\end{array}`)
	display := arrayLayout(t, `\displaystyle\sum_i^n`)
	near(t, arrayCell(t, inherited, 0, 0).Bounds().Size().Y, display.object.Bounds().Size().Y)
	blank := arrayLayout(t, `\begin{array}{|l|r|}\hline&\\\hline&\\\hline\end{array}`)
	if arrayCurveCount(blank.object) != 6 {
		t.Fatal("explicit blank cells must retain only requested rules")
	}
}
func TestArrayAtomicUnsupportedGrammar(t *testing.T) {
	for _, s := range []string{
		`\begin{array}a\end{array}`, `\begin{array}{}a\end{array}`, `\begin{array}{|}a\end{array}`, `\begin{array}{||l}a\end{array}`, `\begin{array}{l||r}a&b\end{array}`,
		`\begin{array}{p{2cm}}a\end{array}`, `\begin{array}{c@{+}c}a&b\end{array}`, `\begin{array}{*{2}{c}}a&b\end{array}`, `\begin{array}{{c}}a\end{array}`, `\begin{array}{\foo}a\end{array}`, `\begin{array}{L}a\end{array}`, `\begin{array}[t]{c}a\end{array}`,
		`\begin{array}{c}\end{array}`, `\begin{array}{c}\hline\end{array}`, `\begin{array}{c}\\a\end{array}`, `\begin{array}{c}a\\\\b\end{array}`, `\begin{array}{cc}a\end{array}`, `\begin{array}{c}a&b\end{array}`, `\begin{array}{cc}a&b\\c\end{array}`,
		`\begin{array}{c}a\hline\end{array}`, `\begin{array}{c}\hline\hline a\end{array}`, `\begin{array}{c}a\\\hline\hline b\end{array}`, `\begin{array}{c}a\\[1em]b\end{array}`, `\begin{array}{c}a\\*b\end{array}`,
		`\begin{array}{c}a\end{matrix}`, `\begin{array}{c}a`, `\begin{array}{c}\multicolumn{1}{c}{a}\end{array}`, `\begin{array}{c}\cline{1-1}a\end{array}`, `\begin{array}{c}\noalign{a}b\end{array}`,
		`\hline`, `\begin{matrix}\hline a\end{matrix}`, `\begin{aligned}\hline a&b\end{aligned}`, `\phantom{\begin{array}{p}a\end{array}}`,
	} {
		if o, err := New(s, DefaultOptions()); err == nil || o != nil {
			t.Fatal("unsupported grammar yielded partial object", s, err)
		}
	}
	for _, size := range []float64{0, -1, math.NaN(), math.Inf(1), 1e6 + 1, math.SmallestNonzeroFloat64} {
		o := DefaultOptions()
		o.Size = size
		if got, err := New(`\begin{array}{|c|}\hline{}\\\hline\end{array}`, o); err == nil || got != nil {
			t.Fatal("invalid dimensions", size, err)
		}
	}
}
func TestArrayGlobalBudgetsHiddenAndLegacyStructuralCurves(t *testing.T) {
	const source = `\begin{array}{|l|r|}\hline\frac{n}{g}&\sqrt{g}\\\hline\hat{g}&\begin{matrix}a&b\end{matrix}\\\hline\end{array}`
	visible := arrayLayout(t, source)
	count := arrayCurveCount(visible.object)
	for _, hidden := range []bool{false, true} {
		formula := source
		if hidden {
			formula = `\phantom{` + formula + `}`
		}
		for _, over := range []int{0, 1} {
			p := parser{source: []rune(formula), options: DefaultOptions(), curves: MaxLayoutCurves - count + over}
			b, _, err := p.row(topRow, p.options.Size)
			if (err != nil) != (over == 1) {
				t.Fatal("shared curve budget or double count", hidden, over, p.curves, count, err)
			}
			if over == 0 && p.curves != MaxLayoutCurves {
				t.Fatal("legacy/rule curves not charged", p.curves)
			}
			if hidden && err == nil && b.object.Bounds().Valid {
				t.Fatal("phantom leaked rules")
			}
		}
	}
	row := strings.TrimSuffix(strings.Repeat("a&", MaxEnvironmentColumns), "&")
	body := strings.TrimSuffix(strings.Repeat(row+`\\`, MaxEnvironmentRows), `\\`)
	full := `\begin{array}{` + strings.Repeat("c", MaxEnvironmentColumns) + `}` + body + `\end{array}`
	p := parser{source: []rune(full), options: DefaultOptions()}
	if _, _, err := p.row(topRow, p.options.Size); err != nil || p.cells != MaxEnvironmentCells {
		t.Fatal("actual maximum cells", p.cells, err)
	}
	for _, replacement := range []string{`\begin{array}{c}a\end{array}`, `\phantom{\begin{gathered}a\end{gathered}}`, `\substack{a}`} {
		s := strings.Replace(full, "a", replacement, 1)
		if o, err := New(s, DefaultOptions()); err == nil || o != nil {
			t.Fatal("nested/hidden cell budget reset", replacement)
		}
	}
	for _, s := range []string{`\begin{array}{` + strings.Repeat("c", 65) + `}a\end{array}`, `\begin{array}{c}` + strings.Repeat(`a\\`, 64) + `a\end{array}`} {
		if o, err := New(s, DefaultOptions()); err == nil || o != nil {
			t.Fatal("row/column limit")
		}
	}
	for _, over := range []int{0, 1} {
		s := `\begin{array}{c}a\end{array}`
		s += strings.Repeat(" ", MaxSourceRunes-len(s)+over)
		if o, err := New(s, DefaultOptions()); (err != nil) != (over == 1) || err != nil && o != nil {
			t.Fatal("source budget", over, err)
		}
	}
	nested := strings.Repeat(`\begin{array}{c}`, MaxNesting) + "a" + strings.Repeat(`\end{array}`, MaxNesting)
	if o, err := New(`\phantom{`+nested+`}`, DefaultOptions()); err == nil || o != nil {
		t.Fatal("nesting budget reset")
	}
}

func TestArrayNestedCellReservationAtExactLimit(t *testing.T) {
	const source = `\begin{array}{c}\begin{array}{cc}{}&{}\end{array}\end{array}`
	for _, hidden := range []bool{false, true} {
		formula := source
		if hidden {
			formula = `\phantom{` + source + `}`
		}
		for _, over := range []int{0, 1} {
			p := parser{source: []rune(formula), options: DefaultOptions(), cells: MaxEnvironmentCells - 3 + over}
			_, _, err := p.row(topRow, p.options.Size)
			if (err != nil) != (over == 1) || p.cells > MaxEnvironmentCells {
				t.Fatal("outer cell must reserve capacity before nested cells", hidden, over, p.cells, err)
			}
		}
	}
	row := strings.TrimSuffix(strings.Repeat("{}&", MaxEnvironmentColumns), "&")
	body := strings.TrimSuffix(strings.Repeat(row+`\\`, MaxEnvironmentRows), `\\`)
	inner := `\begin{array}{` + strings.Repeat("c", MaxEnvironmentColumns) + `}` + body + `\end{array}`
	if o, err := New(`\begin{array}{c}`+inner+`\end{array}`, DefaultOptions()); err == nil || o != nil {
		t.Fatal("maximal nested array bypassed outer cell charge", err)
	}
}
func TestArrayOwnershipFontsConcurrentSnapshots(t *testing.T) {
	const source = `\color{red}\begin{array}{|l|c|r|}\hline\mathbf{n}&\frac{n}{g}&\phantom{W}\\\hline\begin{array}{c}\mathit{g}\end{array}&\hat{n}&\textcolor{blue}{W}\\\hline\end{array}`
	a := arrayLayout(t, source)
	expected := a.object.Copy()
	paints := mathPaints(expected)
	ids := map[uint64]bool{}
	a.object.Walk(func(o *gomath.Object) {
		if ids[o.ID] {
			t.Error("duplicate node ID")
		}
		ids[o.ID] = true
	})
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 3 {
				b, err := layout(source, DefaultOptions())
				if err != nil {
					t.Error(err)
					return
				}
				if b.object.Bounds() != expected.Bounds() || !reflect.DeepEqual(mathPaints(b.object), paints) {
					t.Error("concurrent array geometry changed")
				}
				b.object.Shift(geom.V(10, 20)).SetColor(gomath.Black)
				if expected.Bounds() != a.object.Bounds() {
					t.Error("layout/snapshot alias")
				}
			}
		}()
	}
	wg.Wait()
	if !reflect.DeepEqual(mathPaints(a.object), paints) {
		t.Fatal("concurrent layout paints alias")
	}
	requireSameDrawablePaths(t, a.object, expected)
	q := arrayLayout(t, source)
	rule := named(q.object, "ArrayVRule[0]")
	rule.Path.Contours[0].Curves[0].P0.X += 50
	rule.SetStroke(gomath.Blue, 2)
	requireSameDrawablePaths(t, a.object, expected)
	for _, style := range []text.FontStyle{text.Regular, text.Bold, text.Italic, text.BoldItalic} {
		o := DefaultOptions()
		o.Style = style
		if got, err := New(`\begin{array}{|c|}\hline ng\\\hline\end{array}`, o); err != nil || got == nil {
			t.Fatal("real font face", style, err)
		}
	}
}
func TestArrayWriteTransformIntermediateSnapshots(t *testing.T) {
	from := arrayLayout(t, `\begin{array}{|l|r|}\hline n&g\\\hline W&i\\\hline\end{array}`).object
	scene := gomath.NewScene()
	scene.Add(from)
	if err := scene.Play(1, gomath.Write(from).WithRate(gomath.Linear)); err != nil {
		t.Fatal(err)
	}
	c := render.DefaultConfig()
	c.Width, c.Height, c.Workers, c.FrameWidth = 320, 240, 1, 4
	r, err := render.NewRasterizer(c)
	if err != nil {
		t.Fatal(err)
	}
	var frames []*gomath.Frame
	var pixels [][]uint8
	for _, time := range []float64{0, .35, .7, 1} {
		f, err := scene.Sample(time)
		if err != nil {
			t.Fatal(err)
		}
		o := f.Find(from.ID)
		if o == nil || named(o, "ArrayHRule[0]") == nil {
			t.Fatal("write lost vector rules or identity")
		}
		o.Walk(func(n *gomath.Object) {
			if len(n.Path.Contours) > 0 {
				near(t, n.Style.Reveal, time)
			}
		})
		im, err := r.Render(f)
		if err != nil {
			t.Fatal(err)
		}
		frames = append(frames, f)
		pixels = append(pixels, append([]uint8(nil), im.Pix...))
	}
	if reflect.DeepEqual(pixels[0], pixels[1]) || reflect.DeepEqual(pixels[1], pixels[2]) || reflect.DeepEqual(pixels[2], pixels[3]) {
		t.Fatal("Write did not reveal actual ink/rules")
	}
	earlier := frames[1].Find(from.ID)
	original := earlier.Copy()
	from.Shift(geom.V(2, 3))
	requireSameDrawablePaths(t, earlier, original)
	a := arrayLayout(t, `\begin{array}{|l|r|}\hline n&g\\\hline W&i\\\hline\end{array}`).object
	orig := a.Copy()
	target := a.Copy().ApplyMatrix(geom.Translation(geom.V(.2, .3)).Mul(geom.Scaling(.8, 1.1, 1)))
	morph := gomath.NewScene()
	morph.Add(a)
	if err := morph.Play(1, gomath.Transform(a, target).WithRate(gomath.Linear)); err != nil {
		t.Fatal(err)
	}
	for _, time := range []float64{.35, .7} {
		f, err := morph.Sample(time)
		if err != nil {
			t.Fatal(err)
		}
		want := orig.Copy().ApplyMatrix(geom.Translation(geom.V(.2*time, .3*time)).Mul(geom.Scaling(1-.2*time, 1+.1*time, 1)))
		got := f.Find(a.ID)
		if got == nil {
			t.Fatal("morph identity")
		}
		arrayNearDrawablePaths(t, got, want)
		if _, err := r.Render(f); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			f, err := morph.Sample(.35)
			if err != nil {
				t.Error(err)
				return
			}
			copy := f.Find(a.ID)
			if copy == nil {
				t.Error("concurrent sample lost array ID")
				return
			}
			copy.Shift(geom.V(30, 40)).SetColor(gomath.Black)
		}()
	}
	wg.Wait()
	frame, err := morph.Sample(.35)
	if err != nil {
		t.Fatal(err)
	}
	want := orig.Copy().ApplyMatrix(geom.Translation(geom.V(.2*.35, .3*.35)).Mul(geom.Scaling(1-.2*.35, 1+.1*.35, 1)))
	arrayNearDrawablePaths(t, frame.Find(a.ID), want)
}
func BenchmarkArrayLayout(b *testing.B) {
	const source = `\color{blue}\begin{array}{|l|c|r|}\hline\mathbf{n}&\frac{n}{g}&\phantom{W}\\\hline\sqrt[n]{g}&\sum_{i=1}^n&\begin{matrix}a&b\end{matrix}\\\hline\end{array}`
	b.ReportAllocs()
	for b.Loop() {
		if _, err := New(source, DefaultOptions()); err != nil {
			b.Fatal(err)
		}
	}
}

func arrayNearDrawablePaths(t *testing.T, a, b *gomath.Object) {
	t.Helper()
	var aa, bb []*gomath.Object
	collect := func(root *gomath.Object, out *[]*gomath.Object) {
		root.Walk(func(o *gomath.Object) {
			if len(o.Path.Contours) > 0 {
				*out = append(*out, o)
			}
		})
	}
	collect(a, &aa)
	collect(b, &bb)
	if len(aa) != len(bb) {
		t.Fatal("drawable count differs")
	}
	for i, x := range aa {
		y := bb[i]
		if len(x.Path.Contours) != len(y.Path.Contours) {
			t.Fatal("contours differ")
		}
		for j, c := range x.Path.Contours {
			d := y.Path.Contours[j]
			if c.Closed != d.Closed || len(c.Curves) != len(d.Curves) {
				t.Fatal("curve topology differs")
			}
			if c.Start.Sub(d.Start).Norm() > 1e-12 {
				t.Fatal("contour start differs")
			}
			for k, v := range c.Curves {
				w := d.Curves[k]
				for n, p := range []geom.Vec3{v.P0, v.P1, v.P2, v.P3} {
					q := []geom.Vec3{w.P0, w.P1, w.P2, w.P3}[n]
					if p.Sub(q).Norm() > 1e-12 {
						t.Fatal("interpolated path differs", p, q)
					}
				}
			}
		}
	}
}

func TestArrayBoundaryDepthAndCustomFontErrors(t *testing.T) {
	for _, over := range []int{0, 1} {
		p := parser{source: []rune(`\begin{array}{c}a\end{array}`), options: DefaultOptions(), depth: MaxNesting - 2 + over}
		if _, _, err := p.row(topRow, p.options.Size); (err != nil) != (over == 1) {
			t.Fatal("shared depth boundary", over, err)
		}
	}
	f, err := text.Parse(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	opt := DefaultOptions()
	opt.Font = f
	for _, s := range []string{`\begin{array}{|c|}\hline\mathbf{a}\end{array}`, `\phantom{\begin{array}{c}\mathit{g}\end{array}}`} {
		if o, err := New(s, opt); err == nil || o != nil {
			t.Fatal("array hid unavailable font variant", s, err)
		}
	}
	row := strings.TrimSuffix(strings.Repeat("{}&", 64), "&")
	body := strings.Repeat(`\hline `+row+`\\`, 64) + `\hline `
	p := parser{source: []rune(`\begin{array}{|` + strings.Repeat("c|", 64) + `}` + body + `\end{array}`), options: DefaultOptions()}
	b, _, err := p.row(topRow, p.options.Size)
	if err != nil || p.cells != 4096 || p.curves != 130 || arrayCurveCount(b.object) != 130 {
		t.Fatal("maximal requested vector-rule/cell budget", p.cells, p.curves, err)
	}
}
