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

func TestGeneralFractionsDistinctSizesBaselineAndClearance(t *testing.T) {
	em := DefaultOptions().Size
	for _, command := range []string{"dfrac", "tfrac", "binom", "dbinom", "tbinom"} {
		t.Run(command, func(t *testing.T) {
			source := `\` + command + `{g}{n}`
			b := mustLayout(t, source)
			checkLimitsBox(t, b)
			num, den := named(b.object, "FractionNumerator"), named(b.object, "FractionDenominator")
			if num == nil || den == nil || num.Bounds().Min.Y <= den.Bounds().Max.Y {
				t.Fatal("missing or colliding operands")
			}
			ratio, gap := .7, .14*em
			if command == "dfrac" || command == "dbinom" {
				ratio, gap = 1, .24*em
			}
			near(t, named(num, "g").Bounds().Size().Y, mustLayout(t, "g").object.Bounds().Size().Y*ratio)
			near(t, named(den, "n").Bounds().Size().Y, mustLayout(t, "n").object.Bounds().Size().Y*ratio)
			rule := named(b.object, "FractionRule")
			if command == "dfrac" || command == "tfrac" {
				if rule == nil {
					t.Fatal("missing real fraction rule")
				}
				near(t, rule.Bounds().Min.Y, .16*em)
				near(t, rule.Style.StrokeWidth, .045*em)
				near(t, num.Bounds().Min.Y-(rule.Bounds().Max.Y+rule.Style.StrokeWidth/2), gap)
				near(t, (rule.Bounds().Min.Y-rule.Style.StrokeWidth/2)-den.Bounds().Max.Y, gap)
			} else {
				if rule != nil {
					t.Fatal("binomial acquired a fraction bar")
				}
				near(t, num.Bounds().Min.Y-den.Bounds().Max.Y, 2*gap)
				for _, name := range []string{"Delimiter (", "Delimiter )"} {
					d := named(b.object, name)
					if d == nil || d.Style.Fill.A != 0 || d.Style.StrokeWidth != .045*em || len(d.Path.Contours) != 1 || len(d.Path.Contours[0].Curves) != 2 {
						t.Fatal("not the real existing stroked delimiter path", name)
					}
					if d.Bounds().Max.Y <= num.Bounds().Max.Y || d.Bounds().Min.Y >= den.Bounds().Min.Y {
						t.Fatal("binomial delimiter does not enclose its operands")
					}
				}
			}
			// The baseline stays y=0 when the fraction is followed by normal text.
			followed := mustLayout(t, source+`x`)
			near(t, named(followed.object, "x").Bounds().Min.Y, mustLayout(t, "x").object.Bounds().Min.Y)
			near(t, followed.object.Children[0].Bounds().Min.Y, b.object.Bounds().Min.Y)
			if followed.object.Children[1].Bounds().Min.X < b.width {
				t.Fatal("following glyph overlaps fraction advance")
			}
		})
	}
	display := mustLayout(t, `\dfrac{g}{n}`)
	text := mustLayout(t, `\tfrac{g}{n}`)
	legacy := mustLayout(t, `\frac{g}{n}`)
	if display.height+display.depth <= legacy.height+legacy.depth || legacy.height+legacy.depth <= text.height+text.depth || display.width <= text.width {
		t.Fatal("forced fraction commands remain aliases")
	}
	equalLayoutMetrics(t, mustLayout(t, `\binom{g}{n}`), mustLayout(t, `\tbinom{g}{n}`))
	equalLayoutMetrics(t, mustLayout(t, `\displaystyle\binom{g}{n}`), mustLayout(t, `\dbinom{g}{n}`))
	// Existing fraction geometry and glyph sizing stay unchanged by declarations.
	equalLayoutMetrics(t, legacy, mustLayout(t, `\displaystyle\frac{g}{n}`))
	requireSameDrawablePaths(t, legacy.object, mustLayout(t, `\displaystyle\frac{g}{n}`).object)
}

func TestGeneralFractionScriptsForceNominalSizeAndScope(t *testing.T) {
	for _, command := range []string{"dfrac", "tfrac", "dbinom", "tbinom"} {
		plain := mustLayout(t, `\`+command+`{g}{n}`)
		for _, source := range []string{`x^{\` + command + `{g}{n}}`, `x_{y_{\` + command + `{g}{n}}}`, `\overset{\` + command + `{g}{n}}{=}`} {
			b := mustLayout(t, source)
			near(t, named(b.object, "g").Bounds().Size().Y, named(plain.object, "g").Bounds().Size().Y)
			checkLimitsBox(t, b)
		}
	}
	plain := mustLayout(t, `\binom{g}{n}`)
	for _, test := range []struct {
		source string
		ratio  float64
	}{
		{`x^{\binom{g}{n}}`, .65},
		{`x^{y_{\binom{g}{n}}}`, .65 * .65},
		{`x^{\textstyle\binom{g}{n}}`, 1},
		{`x^{\displaystyle\binom{g}{n}}`, 1 / .7},
	} {
		b := mustLayout(t, test.source)
		near(t, named(b.object, "g").Bounds().Size().Y, named(plain.object, "g").Bounds().Size().Y*test.ratio)
		checkLimitsBox(t, b)
	}
	for _, source := range []string{
		`\displaystyle\dfrac{\sum_i^n}{\sum_j^m}\sum_k^p`,
		`\displaystyle\tfrac{\sum_i^n}{\sum_j^m}\sum_k^p`,
		`\displaystyle\binom{\sum_i^n}{\sum_j^m}\sum_k^p`,
	} {
		b := mustLayout(t, source)
		if named(b.object.Children[0], "OperatorLimits") != nil {
			t.Fatal("outer display mode leaked into fraction operands", source)
		}
		stackedOperator(t, b.object.Children[1], true)
	}
	for _, source := range []string{
		`\dfrac{\displaystyle\sum_i^n}{\sum_j^m}\sum_k^p`,
		`\tfrac{\displaystyle\sum_i^n}{\sum_j^m}\sum_k^p`,
		`\binom{\displaystyle\sum_i^n}{\sum_j^m}\sum_k^p`,
	} {
		b := mustLayout(t, source)
		if named(b.object.Children[0], "OperatorLimits") == nil {
			t.Fatal("local display mode in operand lost")
		}
		stackedOperator(t, b.object.Children[1], false)
	}
	// Declarations in groups, delimiters, script arguments and matrix cells
	// restore the new nominal-em state as well as the old operator placement mode.
	for _, source := range []string{
		`x^{{\displaystyle\binom{g}{n}}\binom{a}{b}}`,
		`x^{\left(\displaystyle\binom{g}{n}\right)\binom{a}{b}}`,
		`x^{\begin{matrix}\displaystyle\binom{g}{n}&\binom{a}{b}\end{matrix}}`,
	} {
		b := mustLayout(t, source)
		near(t, named(b.object, "g").Bounds().Size().Y, mustLayout(t, "g").object.Bounds().Size().Y)
		near(t, named(b.object, "a").Bounds().Size().Y, mustLayout(t, "a").object.Bounds().Size().Y*.65*.7)
	}
}

func TestGeneralFractionNestedStylesEmptyAndOwnership(t *testing.T) {
	for _, command := range []string{"dfrac", "tfrac", "binom", "dbinom", "tbinom"} {
		for _, style := range []text.FontStyle{text.Regular, text.Bold, text.Italic, text.BoldItalic} {
			opt := DefaultOptions()
			opt.Style, opt.Color = style, gomath.RGB(.2, .8, .4)
			b, err := layout(`\`+command+`{\mathbb{R}+g}{\mathcal{A}+n}_i^2`, opt)
			if err != nil {
				t.Fatal(err)
			}
			b.object.Walk(func(n *gomath.Object) {
				if len(n.Path.Contours) > 0 && n.Style.Fill.A > 0 && n.Style.Fill != opt.Color {
					t.Error("fraction lost glyph color")
				}
			})
			checkLimitsBox(t, b)
		}
		for _, operands := range []string{`{}{}`, `{}{g}`, `{g}{}`, `{\quad}{\quad}`} {
			checkLimitsBox(t, mustLayout(t, `\`+command+operands))
		}
	}
	for _, source := range []string{
		`\dfrac{\tfrac{a}{b}}{\dbinom{n}{k}}`,
		`\binom{\hat{n}+\frac{a}{b}}{\substack{i=1\\j=2}}^2`,
		`\widehat{\dbinom{n}{\tfrac{a}{g}}}+\underline{\tfrac{x}{y}}`,
		`\left[\begin{matrix}\dfrac{a}{b}&\tfrac{g}{n}\\\dbinom{N}{K}&\tbinom{n}{k}\end{matrix}\right]`,
	} {
		checkLimitsBox(t, mustLayout(t, source))
	}
	custom, err := text.Parse(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	opt := DefaultOptions()
	opt.Font = custom
	if _, err := New(`\dbinom{g}{n}`, opt); err != nil {
		t.Fatal("custom font operands and path delimiters rejected", err)
	}
	if object, err := New(`\dbinom{\mathbb{R}}{n}`, opt); err == nil || object != nil {
		t.Fatal("custom missing alphabet silently replaced")
	}
	const source = `\dfrac{\mathbb{R}}{\tfrac{g}{n}}+\dbinom{n}{k}+\sum\limits_{\substack{i=1\\j=2}}^n`
	expected := mustLayout(t, source)
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
				if b.object.Bounds() != expected.object.Bounds() || b.width != expected.width || b.height != expected.height || b.depth != expected.depth {
					t.Error("concurrent fraction layout differs")
					return
				}
				named(b.object, "ℝ").Path.Contours[0].Curves[0].P0.X += 100
				b.object.Shift(geom.V(100, 100)).SetFill(gomath.Red)
			}
		}()
	}
	wg.Wait()
	requireSameDrawablePaths(t, expected.object, mustLayout(t, source).object)
	pair := mustLayout(t, `\dbinom{g}{g}`)
	num, den := named(pair.object, "FractionNumerator"), named(pair.object, "FractionDenominator")
	before := den.Copy()
	named(num, "g").Path.Contours[0].Curves[0].P0.X += 100
	if !reflect.DeepEqual(den.Path, before.Path) || den.Bounds() != before.Bounds() {
		t.Fatal("fraction operands share mutable contours")
	}
}

func TestGeneralFractionInvalidInputAndCurves(t *testing.T) {
	for _, command := range []string{"dfrac", "tfrac", "binom", "dbinom", "tbinom"} {
		for _, suffix := range []string{"", "x", "{n}", "{n}{", "{n}{k}\\limits_i", "{n}{k}_i_j", "{\\input{file}}{k}"} {
			if object, err := New(`\`+command+suffix, DefaultOptions()); err == nil || object != nil {
				t.Fatal("invalid fraction accepted", command, suffix, err)
			}
		}
		for _, size := range []float64{0, -1, math.Inf(1), math.NaN(), 1e6 + 1, math.SmallestNonzeroFloat64} {
			opt := DefaultOptions()
			opt.Size = size
			if object, err := New(`\`+command+`{}{}`, opt); err == nil || object != nil {
				t.Fatal("invalid/collapsed dimensions", command, size, err)
			}
		}
		source := `\` + command + `{g}{n}`
		total := countAccentCurves(mustLayout(t, source).object)
		for _, over := range []int{0, 1} {
			p := parser{source: []rune(source), options: DefaultOptions(), curves: MaxLayoutCurves - total + over}
			b, _, err := p.row(topRow, p.options.Size)
			if over == 0 && (err != nil || p.curves != MaxLayoutCurves || countAccentCurves(b.object) != total) || over == 1 && (err == nil || !strings.Contains(err.Error(), "curves")) {
				t.Fatal("fraction shared curve boundary", command, over, p.curves, err)
			}
		}
	}
	for _, source := range []string{`{a\over b}`, `\genfrac{(}{)}{0}{0}{n}{k}`, `\cfrac{a}{b}`, `\choose`, `\dfrac{\unknown}{g}`} {
		if object, err := New(source, DefaultOptions()); err == nil || object != nil {
			t.Fatal("unsupported generalized syntax accepted", source, err)
		}
	}
}

func BenchmarkGeneralFractionsAndSubstack(b *testing.B) {
	const source = `\displaystyle\sum_{\substack{i=1\\j\in\mathbb{N}}}^{n}\dfrac{\dbinom{n}{i}}{\tfrac{g}{k}}+\tbinom{\mathcal{A}}{\hat{n}}`
	if _, err := New(source, DefaultOptions()); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := New(source, DefaultOptions()); err != nil {
			b.Fatal(err)
		}
	}
}
