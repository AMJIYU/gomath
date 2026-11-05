package mathtex

import (
	"gomath"
	"math"
	"strings"
	"testing"
)

func TestSubstackRowsBaselineCenterAndScriptSize(t *testing.T) {
	b := mustLayout(t, `\substack{n\\g\\abcd}`)
	stack := b.object.Children[0]
	if stack.Name != "Substack" || len(stack.Children) != 3 {
		t.Fatal("missing real multiline stack")
	}
	size := DefaultOptions().Size * .65
	for i, literal := range []string{"n", "g", "abcd"} {
		opt := DefaultOptions()
		opt.Size = size
		plain, err := layout(literal, opt)
		if err != nil {
			t.Fatal(err)
		}
		row := stack.Children[i]
		near(t, row.Bounds().Min.X-plain.object.Bounds().Min.X, (b.width-plain.width)/2)
		near(t, row.Bounds().Size().X, plain.object.Bounds().Size().X)
		near(t, row.Bounds().Size().Y, plain.object.Bounds().Size().Y)
		if i != 0 && row.Bounds().Max.Y >= stack.Children[i-1].Bounds().Min.Y-size*.19 {
			t.Fatal("adjacent rows overlap or lose baseline clearance")
		}
	}
	near(t, (b.height-b.depth)/2, size*.16)
	checkLimitsBox(t, b)
	for _, source := range []string{`x_{\substack{n\\g}}`, `\sum\limits_{\substack{n\\g}}`, `\substack{n\\g}`} {
		b := mustLayout(t, source)
		near(t, named(b.object, "n").Bounds().Size().Y, named(mustLayout(t, "n").object, "n").Bounds().Size().Y*.65)
		checkLimitsBox(t, b)
	}
	b = mustLayout(t, `x^{y_{\substack{n\\g}}}`)
	near(t, named(b.object, "n").Bounds().Size().Y, mustLayout(t, "n").object.Bounds().Size().Y*.65*.65)
	// Wide/different-height rows center by advance and clear full baseline boxes.
	for _, source := range []string{
		`\substack{\quad n\quad\\g}`, `\substack{\dfrac{g}{n}\\\hat{x}}`,
		`\substack{\binom{n}{k}\\\begin{matrix}a&b\end{matrix}}`,
		`\substack{\substack{i=1\\j=2}\\k=3}`, `\substack{\frac{}{}}`,
		`\displaystyle\sum_{\substack{i=1\\j\in\mathbb{N}}}^n x`,
		`\binom{\substack{a\\b}}{\substack{c\\d}}`,
	} {
		checkLimitsBox(t, mustLayout(t, source))
	}
}

func TestSubstackDeclarationAndFontScope(t *testing.T) {
	b := mustLayout(t, `\displaystyle\substack{\displaystyle\sum_i^n\\\sum_j^m}\sum_k^p`)
	stack := named(b.object.Children[0], "Substack")
	stackedOperator(t, stack.Children[0].Children[0], true)
	stackedOperator(t, stack.Children[1].Children[0], false)
	stackedOperator(t, b.object.Children[1], true)
	b = mustLayout(t, `\substack{\displaystyle\sum_i^n\\\sum_j^m}\sum_k^p`)
	stackedOperator(t, b.object.Children[1], false)
	// The first row's nominal-em declaration cannot enlarge a later row's binom.
	b = mustLayout(t, `\substack{\displaystyle\binom{g}{n}\\\binom{a}{b}}`)
	near(t, named(b.object, "g").Bounds().Size().Y, mustLayout(t, "g").object.Bounds().Size().Y)
	near(t, named(b.object, "a").Bounds().Size().Y, mustLayout(t, "a").object.Bounds().Size().Y*.65*.7)
	for _, source := range []string{
		`\mathbf{\substack{g\\n}}x`, `\mathit{\substack{g\\n}}x`,
		`\mathbb{\substack{R\\N}}x`, `\substack{\mathbf{g}\\n}x`,
	} {
		b := mustLayout(t, source)
		sameGlyphShape(t, named(b.object, "x"), "x", 0)
		checkLimitsBox(t, b)
	}
	opt := DefaultOptions()
	opt.Color = gomath.Red
	b, err := layout(`\substack{\mathbb{R}\\g}`, opt)
	if err != nil {
		t.Fatal(err)
	}
	b.object.Walk(func(n *gomath.Object) {
		if len(n.Path.Contours) > 0 && n.Style.Fill.A > 0 && n.Style.Fill != gomath.Red {
			t.Error("substack lost inherited glyph color")
		}
	})
}

func TestSubstackRejectsUnsupportedRowsAndBudgets(t *testing.T) {
	for _, source := range []string{
		`\substack`, `\substack x`, `\substack{}`, `\substack{ }`, `\substack{\quad}`,
		`\substack{a\\}`, `\substack{\\a}`, `\substack{a\\\\b}`, `\substack{a&b}`,
		`\substack{a\\[2pt]b}`, `\substack{a`, `\substack{a\end{matrix}}`,
		`\substack{a}\limits`, `\substack{a}\nolimits`, `\substack{a}_{i}_{j}`,
		`\substack{\unknown}`, `\substack{\input{file}}`, `a\\b`,
	} {
		if object, err := New(source, DefaultOptions()); err == nil || object != nil {
			t.Fatal("invalid substack accepted", source, err)
		}
	}
	for _, size := range []float64{0, -1, math.NaN(), math.Inf(1), 1e6 + 1, math.SmallestNonzeroFloat64} {
		opt := DefaultOptions()
		opt.Size = size
		if object, err := New(`\substack{g}`, opt); err == nil || object != nil {
			t.Fatal("invalid substack dimensions", size, err)
		}
	}
	for _, over := range []int{0, 1} {
		rows := strings.Repeat(`g\\`, MaxSubstackRows-1+over) + "g"
		_, err := New(`\substack{`+rows+`}`, DefaultOptions())
		if (err != nil) != (over == 1) {
			t.Fatal("substack row boundary", over, err)
		}
		n := MaxNesting - 1 + over
		_, err = New(strings.Repeat(`\substack{`, n)+"g"+strings.Repeat("}", n), DefaultOptions())
		if (err != nil) != (over == 1) {
			t.Fatal("substack nesting boundary", over, err)
		}
	}
	for _, source := range []string{`\begin{matrix}\substack{a\\b}&c\end{matrix}`, `\substack{\substack{a\\b}\\c}`} {
		for _, over := range []int{0, 1} {
			p := parser{source: []rune(source), options: DefaultOptions(), cells: MaxEnvironmentCells - 4 + over}
			_, _, err := p.row(topRow, p.options.Size)
			if over == 0 && (err != nil || p.cells != MaxEnvironmentCells) || over == 1 && (err == nil || !strings.Contains(err.Error(), "4096")) {
				t.Fatal("mixed environment/substack total-cell boundary", over, p.cells, err)
			}
		}
	}
	source := `\substack{\hat{g}\\\mathbb{R}}`
	total := countAccentCurves(mustLayout(t, source).object)
	for _, over := range []int{0, 1} {
		p := parser{source: []rune(source), options: DefaultOptions(), curves: MaxLayoutCurves - total + over}
		_, _, err := p.row(topRow, p.options.Size)
		if over == 0 && (err != nil || p.curves != MaxLayoutCurves) || over == 1 && (err == nil || !strings.Contains(err.Error(), "curves")) {
			t.Fatal("substack curve boundary", over, err)
		}
	}
	boundary := strings.Repeat(" ", MaxSourceRunes-len(source)) + source
	if _, err := New(boundary, DefaultOptions()); err != nil {
		t.Fatal(err)
	}
	if _, err := New(boundary+" ", DefaultOptions()); err == nil {
		t.Fatal("substack source boundary bypass")
	}
}
