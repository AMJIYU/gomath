package mathtex

import (
	"golang.org/x/image/font/gofont/goregular"
	"gomath"
	"gomath/geom"
	"gomath/text"
	"math"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func multilineSource(name string, rows [][]string) string {
	var lines []string
	for _, row := range rows {
		lines = append(lines, strings.Join(row, "&"))
	}
	return `\begin{` + name + `}` + strings.Join(lines, `\\`) + `\end{` + name + `}`
}

func TestMultilineAlternatingAlignmentAndBaselineBoxes(t *testing.T) {
	rows := [][]string{{`\quad i`, `=\dfrac{n}{g}`, "W", "=g"}, {"W", "=n", "i", `=\hat{g}_{j}`}, {"", "=g", "g", "=W"}}
	b := mustLayout(t, multilineSource("aligned", rows))
	widths := make([]float64, 4)
	heights, depths := make([]float64, len(rows)), make([]float64, len(rows))
	inputs := make([][]box, len(rows))
	size := DefaultOptions().Size
	for i, row := range rows {
		heights[i], depths[i] = size*.7, size*.2
		for j, source := range row {
			cell := mustLayout(t, source)
			inputs[i] = append(inputs[i], cell)
			widths[j] = math.Max(widths[j], cell.width)
			heights[i] = math.Max(heights[i], cell.height)
			depths[i] = math.Max(depths[i], cell.depth)
		}
	}
	total := size * .3 * float64(len(rows)-1)
	for i := range rows {
		total += heights[i] + depths[i]
	}
	near(t, b.height, size*.16+total/2)
	near(t, b.depth, total-b.height)
	near(t, b.width, widths[0]+widths[1]+widths[2]+widths[3]+size*1.24)
	y := b.height
	for i, row := range rows {
		y -= heights[i]
		x := 0.0
		for j := range row {
			cell := named(b.object, "aligned cell["+string(rune('0'+i))+","+string(rune('0'+j))+"]")
			if cell == nil {
				t.Fatal("missing explicit alignment cell", i, j)
			}
			input := inputs[i][j]
			wantX := x
			if j%2 == 0 {
				wantX += widths[j] - input.width
			}
			if input.object.Bounds().Valid {
				near(t, cell.Bounds().Min.X-input.object.Bounds().Min.X, wantX)
				near(t, cell.Bounds().Min.Y-input.object.Bounds().Min.Y, y)
			}
			x += widths[j]
			if j%2 == 0 {
				x += size * .12
			} else {
				x += size
			}
		}
		y -= depths[i] + size*.3
	}
	checkLimitsBox(t, b)
}

func TestMultilineGatheredCentersAdvancesAndNestedBodies(t *testing.T) {
	for _, rows := range [][][]string{
		{{"W"}, {"i"}, {"g"}},
		{{`\quad n\quad`}, {`\dfrac{n}{g}`}, {`\hat{g}_i`}},
	} {
		b := mustLayout(t, multilineSource("gathered", rows))
		for i, row := range rows {
			input := mustLayout(t, row[0])
			cell := named(b.object, "gathered cell["+string(rune('0'+i))+",0]")
			if cell == nil {
				t.Fatal("missing gathered row")
			}
			near(t, cell.Bounds().Min.X-input.object.Bounds().Min.X+input.width/2, b.width/2)
			if i > 0 {
				previous := named(b.object, "gathered cell["+string(rune('0'+i-1))+",0]")
				if previous.Bounds().Min.Y <= cell.Bounds().Max.Y {
					t.Fatal("row ink overlaps")
				}
			}
		}
		checkLimitsBox(t, b)
	}
	for _, source := range []string{
		`\left[\begin{aligned}a&=\dfrac{n}{g}\\b&=\binom{n}{k}\end{aligned}\right]_i^n`,
		`\hat{\begin{gathered}n\\g\end{gathered}}`,
		`\dfrac{\begin{aligned}a&=b\\&=c\end{aligned}}{\begin{gathered}n\\g\end{gathered}}`,
		`\begin{matrix}\begin{aligned}a&=b\end{aligned}&\begin{gathered}g\\n\end{gathered}\end{matrix}`,
		`\begin{aligned}\begin{gathered}a\\g\end{gathered}&=\begin{aligned}b&c\end{aligned}\end{aligned}`,
		`\substack{\begin{gathered}n\\g\end{gathered}\\k}`,
		`x^{\begin{aligned}a&b\\n&g\end{aligned}}_{\begin{gathered}i\\j\end{gathered}}`,
		`\begin{gathered}{[n]}\\\left[g\right]\end{gathered}`,
		`\begin{gathered}a\\{*}b\end{gathered}`,
	} {
		checkLimitsBox(t, mustLayout(t, source))
	}
}

func TestMultilineDisplayColorAndFontScopes(t *testing.T) {
	b := mustLayout(t, `\begin{aligned}\sum_i^n&\textstyle\sum_j^m\\\sum_k^p&\sum_l^q\end{aligned}\sum_u^v`)
	for i, name := range []string{"aligned cell[0,0]", "aligned cell[0,1]", "aligned cell[1,0]", "aligned cell[1,1]"} {
		if (named(named(b.object, name), "OperatorLimits") != nil) != (i != 1) {
			t.Fatal("cell display style did not reset", name)
		}
	}
	if named(b.object.Children[len(b.object.Children)-1], "OperatorLimits") != nil {
		t.Fatal("environment display mode leaked outside")
	}
	b = mustLayout(t, `\begin{gathered}\textstyle\sum_i^n\\\sum_j^m\end{gathered}`)
	if named(named(b.object, "gathered cell[0,0]"), "OperatorLimits") != nil || named(named(b.object, "gathered cell[1,0]"), "OperatorLimits") == nil {
		t.Fatal("gathered declaration leaked across rows")
	}
	b = mustLayout(t, `x^{\begin{aligned}n&\binom{g}{n}\end{aligned}}`)
	n1, n2 := named(named(b.object, "aligned cell[0,0]"), "n"), named(named(b.object, "aligned cell[0,1]"), "n")
	near(t, n1.Bounds().Size().X, n2.Bounds().Size().X)
	w, r, blue := gomath.White, gomath.RGB(1, 0, 0), gomath.RGB(0, 0, 1)
	for _, tc := range []struct {
		source string
		want   []gomath.Color
	}{
		{`\begin{aligned}\color{red}a&b\\c&\textcolor{blue}{d}\end{aligned}e`, []gomath.Color{r, w, w, blue, w}},
		{`\color{red}\begin{gathered}\color{blue}a\\b\end{gathered}c`, []gomath.Color{blue, r, r}},
		{`\textcolor{blue}{\begin{aligned}\mathbb{R}&\mathcal{A}\\\mathfrak{g}&\hat{n}\end{aligned}}x`, []gomath.Color{blue, blue, blue, blue, blue, w}},
	} {
		if got := mathPaints(mustLayout(t, tc.source).object); !reflect.DeepEqual(got, tc.want) {
			t.Fatal("multiline color scope", tc.source, got, tc.want)
		}
	}
	for _, style := range []text.FontStyle{text.Regular, text.Bold, text.Italic, text.BoldItalic} {
		opt := DefaultOptions()
		opt.Style = style
		plain, err := layout(`\begin{aligned}n&g\end{aligned}`, opt)
		if err != nil {
			t.Fatal(err)
		}
		styled, err := layout(`\textcolor{red}{\begin{aligned}n&g\end{aligned}}`, opt)
		if err != nil {
			t.Fatal(err)
		}
		equalLayoutMetrics(t, plain, styled)
		requireSameDrawablePaths(t, plain.object, styled.object)
	}
	custom, err := text.Parse(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	opt := DefaultOptions()
	opt.Font = custom
	if _, err := New(`\begin{gathered}n\\g\end{gathered}`, opt); err != nil {
		t.Fatal(err)
	}
	if o, err := New(`\begin{aligned}\mathbb{R}&n\end{aligned}`, opt); err == nil || o != nil {
		t.Fatal("multiline introduced custom-font fallback")
	}
}

func TestMultilineGrammarAndSharedBudgets(t *testing.T) {
	for _, source := range []string{
		`\begin{aligned}\end{aligned}`, `\begin{gathered}\end{gathered}`,
		`\begin{aligned}a\end{aligned}`, `\begin{aligned}a&b&c\end{aligned}`,
		`\begin{aligned}a&b\\c&d&e&f\end{aligned}`, `\begin{aligned}&&&\end{aligned}`,
		`\begin{gathered}a&b\end{gathered}`, `\begin{gathered}a\\\\b\end{gathered}`,
		`\begin{gathered}\quad\end{gathered}`, `\begin{gathered}\color{red}\end{gathered}`,
		`\begin{aligned}a&b\\\end{aligned}`, `\begin{gathered}a\\\end{gathered}`,
		`\begin{aligned}[t]a&b\end{aligned}`, `\begin{gathered}[b]a\end{gathered}`,
		`\begin{aligned}a&b\\[1em]c&d\end{aligned}`, `\begin{gathered}a\\[2pt]b\end{gathered}`,
		`\begin{aligned}a&b\\*c&d\end{aligned}`, `\begin{gathered}a\\*b\end{gathered}`,
		`\begin{aligned}a&b\end{gathered}`, `\begin{gathered}a\end{aligned}`,
		`\begin{aligned}a&b`, `\begin{gathered}a`, `\begin{aligned}a&\unknown\end{aligned}`,
		`\begin{aligned}a&b\tag{1}\end{aligned}`, `\begin{gathered}a\notag\end{gathered}`,
		`\begin{align}a&b\end{align}`, `\begin{alignedat}{2}a&b\end{alignedat}`,
		`\begin{gathered}a\end{gathered}\limits_i`,
	} {
		if o, err := New(source, DefaultOptions()); err == nil || o != nil {
			t.Fatal("invalid multiline structure accepted", source, err)
		}
	}
	for _, source := range []string{`\begin{aligned}&a\end{aligned}`, `\begin{aligned}a&\end{aligned}`, `\begin{aligned}a&&&b\end{aligned}`} {
		mustLayout(t, source) // Explicit empty cells, with visible row content.
	}
	for _, name := range []string{"aligned", "gathered"} {
		row := "a"
		if name == "aligned" {
			row = "a&b"
		}
		for _, over := range []int{0, 1} {
			source := `\begin{` + name + `}` + strings.TrimSuffix(strings.Repeat(row+`\\`, MaxEnvironmentRows+over), `\\`) + `\end{` + name + `}`
			if _, err := New(source, DefaultOptions()); (err != nil) != (over == 1) {
				t.Fatal("row boundary", name, over, err)
			}
			start := `\begin{` + name + `}`
			if name == "aligned" {
				start += "&"
			}
			n := MaxNesting - 1 + over
			source = strings.Repeat(start, n) + "a" + strings.Repeat(`\end{`+name+`}`, n)
			if _, err := New(source, DefaultOptions()); (err != nil) != (over == 1) {
				t.Fatal("nesting boundary", name, over, err)
			}
		}
	}
	for _, over := range []int{0, 1} {
		source := `\begin{aligned}` + strings.TrimSuffix(strings.Repeat("a&", MaxEnvironmentColumns+2*over), "&") + `\end{aligned}`
		if _, err := New(source, DefaultOptions()); (err != nil) != (over == 1) {
			t.Fatal("column boundary", over, err)
		}
		const mixed = `\begin{aligned}\begin{gathered}a\\b\end{gathered}&c\end{aligned}`
		p := parser{source: []rune(mixed), options: DefaultOptions(), cells: MaxEnvironmentCells - 4 + over}
		if _, _, err := p.row(topRow, p.options.Size); (err != nil) != (over == 1) {
			t.Fatal("shared cell boundary", over, err)
		}
		const sourcePrefix = `\begin{gathered}n\end{gathered}`
		source = sourcePrefix + strings.Repeat(" ", MaxSourceRunes-len(sourcePrefix)+over)
		if _, err := New(source, DefaultOptions()); (err != nil) != (over == 1) {
			t.Fatal("source boundary", over, err)
		}
	}
	const source = `\begin{aligned}\hat{\mathbb{R}}&\dfrac{n}{g}\\i&j\end{aligned}`
	p := parser{source: []rune(source), options: DefaultOptions()}
	b, _, err := p.row(topRow, p.options.Size)
	if err != nil {
		t.Fatal(err)
	}
	curves := 0
	b.object.Walk(func(n *gomath.Object) {
		for _, c := range n.Path.Contours {
			curves += len(c.Curves)
		}
	})
	if curves != p.curves || curves == 0 {
		t.Fatal("multiline lost/double-counted contours", curves, p.curves)
	}
	for _, over := range []int{0, 1} {
		q := parser{source: []rune(source), options: DefaultOptions(), curves: MaxLayoutCurves - curves + over}
		if _, _, err := q.row(topRow, q.options.Size); (err != nil) != (over == 1) {
			t.Fatal("curve boundary", over, err)
		}
	}
	for _, size := range []float64{0, -1, math.NaN(), math.Inf(1), 1e6 + 1, math.SmallestNonzeroFloat64} {
		opt := DefaultOptions()
		opt.Size = size
		if o, err := New(`\begin{aligned}a&b\end{aligned}`, opt); err == nil || o != nil {
			t.Fatal("invalid/collapsed dimensions", size, err)
		}
	}
}

func TestMultilineOwnershipAndConcurrentLayouts(t *testing.T) {
	const source = `\color{red}\begin{aligned}\textcolor{blue}{\mathbb{R}}&\dfrac{n}{g}\\\sum_{\substack{i=1\\j=2}}^n&\begin{gathered}\hat{n}\\g\end{gathered}\end{aligned}`
	expected := mustLayout(t, source)
	paints := mathPaints(expected.object)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 5 {
				b, err := layout(source, DefaultOptions())
				if err != nil {
					t.Error(err)
					return
				}
				if b.object.Bounds() != expected.object.Bounds() || !reflect.DeepEqual(mathPaints(b.object), paints) {
					t.Error("concurrent multiline layout differs")
				}
				named(b.object, "ℝ").Path.Contours[0].Curves[0].P0.X += 100
				copy := expected.object.Copy()
				copy.Shift(geom.V(10, 10)).SetColor(gomath.Black)
			}
		}()
	}
	wg.Wait()
	requireSameDrawablePaths(t, expected.object, mustLayout(t, source).object)
	if !reflect.DeepEqual(mathPaints(expected.object), paints) {
		t.Fatal("copy/layout mutated shared colors")
	}
}

func TestMultilineActualMaximumCellWork(t *testing.T) {
	// A real 64x64 source fits the rune and curve bounds and consumes exactly
	// the shared cell budget. One nested cell exceeds it without increasing
	// the outer row/column counts, so this exercises the global work bound.
	row := strings.TrimSuffix(strings.Repeat("a&", MaxEnvironmentColumns), "&")
	body := strings.TrimSuffix(strings.Repeat(row+`\\`, MaxEnvironmentRows), `\\`)
	for _, over := range []int{0, 1} {
		source := body
		if over == 1 {
			source = strings.Replace(source, "a", `\begin{gathered}a\end{gathered}`, 1)
		}
		p := parser{source: []rune(`\begin{aligned}` + source + `\end{aligned}`), options: DefaultOptions()}
		_, _, err := p.row(topRow, p.options.Size)
		if (err != nil) != (over == 1) || p.cells != MaxEnvironmentCells+over {
			t.Fatal("actual maximum shared multiline cell work", over, p.cells, err)
		}
	}
}

func BenchmarkMultilineLayout(b *testing.B) {
	for _, tc := range []struct{ name, source string }{
		{"aligned", `\begin{aligned}\color{red}f(x)&=\dfrac{\hat{n}}{g}&A&=\mathbb{R}\\\sum_{\substack{i=1\\j=2}}^n i&=\binom{n}{k}&B&=\mathcal{A}\end{aligned}`},
		{"gathered", `\begin{gathered}\color{red}f(x)=\dfrac{\hat{n}}{g}\\\sum_{\substack{i=1\\j=2}}^n i=\binom{n}{k}\\\mathbb{R}=\mathcal{A}\end{gathered}`},
	} {
		b.Run(tc.name, func(b *testing.B) {
			opt := DefaultOptions()
			b.ReportAllocs()
			for b.Loop() {
				if _, err := New(tc.source, opt); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
