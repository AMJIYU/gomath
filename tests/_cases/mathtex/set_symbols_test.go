package mathtex

import (
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gobolditalic"
	"golang.org/x/image/font/gofont/goitalic"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/sfnt"
	"gomath"
	"gomath/geom"
	"gomath/text"
	"math"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestSetMembershipOperatorLayoutExample(t *testing.T) {
	const source = `\displaystyle\operatorname*{arg max}_{x\in R}\overset{\hat{n}}{f}(x)`
	b := mustLayout(t, source)
	checkLimitsBox(t, b)
	if n := named(b.object, "∈"); n == nil || len(n.Path.Contours) == 0 {
		t.Fatal("membership has no actual glyph outline")
	}
	stackedOperator(t, b.object.Children[0], true)
	b.object.Walk(func(n *gomath.Object) {
		if n.Name == "∈" && n.Style.StrokeWidth != 0 {
			t.Fatal("membership outline should be filled")
		}
	})
}

// Pin the actual native cmap decision: supplementary contours are required,
// while union/intersection reuse exactly the existing Go font glyphs.
func TestSetSymbolEmbeddedCoverage(t *testing.T) {
	for name, data := range map[string][]byte{"regular": goregular.TTF, "bold": gobold.TTF, "italic": goitalic.TTF, "bold-italic": gobolditalic.TTF} {
		f, err := sfnt.Parse(data)
		if err != nil {
			t.Fatal(err)
		}
		var buf sfnt.Buffer
		for command, r := range setSymbolCommands {
			index, err := f.GlyphIndex(&buf, r)
			if err != nil {
				t.Fatal(err)
			}
			native := command == "cup" || command == "cap"
			if (index != 0) != native {
				t.Fatalf("%s %s native cmap changed: glyph %d", name, command, index)
			}
			if _, supplemental := supplementalSetOutlines[r]; supplemental == native {
				t.Fatal("source contour coverage mismatch", command)
			}
		}
	}
}

func TestSetSymbolUnicodeStyleAndScope(t *testing.T) {
	for command, r := range setSymbolCommands {
		for _, style := range []text.FontStyle{text.Regular, text.Bold, text.Italic, text.BoldItalic} {
			opt := DefaultOptions()
			opt.Style = style
			byCommand, err := layout(`\`+command, opt)
			if err != nil {
				t.Fatal(err)
			}
			byRune, err := layout(string(r), opt)
			if err != nil {
				t.Fatal(err)
			}
			equalLayoutMetrics(t, byCommand, byRune)
			requireSameDrawablePaths(t, byCommand.object, byRune.object)
			if command == "cup" || command == "cap" {
				sameGlyphShape(t, named(byCommand.object, string(r)), string(r), style)
			}
		}
	}
	for r := range supplementalSetOutlines {
		regular := mustLayout(t, string(r))
		italic := mustLayout(t, `\mathit{`+string(r)+`}`)
		bold := mustLayout(t, `\mathbf{`+string(r)+`}`)
		boldItalic := mustLayout(t, `\mathbf{\mathit{`+string(r)+`}}`)
		requireSameDrawablePaths(t, regular.object, italic.object)
		requireSameDrawablePaths(t, bold.object, boldItalic.object)
		if reflect.DeepEqual(named(regular.object, string(r)).Path, named(bold.object, string(r)).Path) {
			t.Fatal("bold must select a real distinct face", string(r))
		}
	}
	for _, source := range []string{`\mathbf{\in}x`, `\mathit{\notin}x`, `\mathbf{\mathit{\subseteq}}x`, `\displaystyle\operatorname*{arg max}_{x\in R}x`} {
		b := mustLayout(t, source)
		// Last x must retain regular shape even after scoped styles/limits.
		sameGlyphShape(t, named(b.object.Children[len(b.object.Children)-1], "x"), "x", text.Regular)
		checkLimitsBox(t, b)
	}
	equalLayoutMetrics(t, mustLayout(t, `\owns`), mustLayout(t, `\ni`))
	requireSameDrawablePaths(t, mustLayout(t, `\owns`).object, mustLayout(t, `\ni`).object)
}

func TestSetSymbolScriptsBaselineAndComposition(t *testing.T) {
	for command, r := range setSymbolCommands {
		plain := mustLayout(t, `\`+command)
		followed := mustLayout(t, `\`+command+` q`)
		near(t, named(followed.object, string(r)).Bounds().Min.Y, plain.object.Bounds().Min.Y)
		near(t, followed.object.Children[1].Bounds().Min.X, plain.width+DefaultOptions().Size*.04+mustLayout(t, "q").object.Bounds().Min.X)
		for _, source := range []string{`x^{\` + command + `}`, `x_{\` + command + `}`} {
			b := mustLayout(t, source)
			script := named(b.object, string(r)).Bounds()
			near(t, script.Size().X, plain.object.Bounds().Size().X*.65)
			near(t, script.Size().Y, plain.object.Bounds().Size().Y*.65)
			checkLimitsBox(t, b)
		}
		nested := mustLayout(t, `x^{y_{\`+command+`}}`)
		near(t, named(nested.object, string(r)).Bounds().Size().Y, plain.object.Bounds().Size().Y*.65*.65)
		checkLimitsBox(t, nested)
	}
	for _, source := range []string{
		`\displaystyle\sum_{x\notin A}^{B\subseteq C}x`,
		`\hat{\in}+\overline{A\cup B}+\underline{A\cap B}`,
		`\overset{\notin}{\subseteq}+\underset{\emptyset}{\supsetneq}_i`,
		`\frac{A\setminus B}{C\ni x}+\sqrt{A\nsubseteq B}`,
		`\left[\begin{matrix}A\subset B&A\supset B\\A\subseteq B&A\supseteq B\end{matrix}\right]`,
		`\displaystyle\operatorname*{arg max}_{x\in R}\overset{\hat{n}}{f}(x)`,
	} {
		checkLimitsBox(t, mustLayout(t, source))
	}
}

func TestSetSymbolCustomFontsStayStrict(t *testing.T) {
	custom, err := text.Parse(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	customBold, err := text.Parse(gobold.TTF)
	if err != nil {
		t.Fatal(err)
	}
	opt := DefaultOptions()
	opt.Font, opt.BoldFont = custom, customBold
	for _, source := range []string{`\in`, `∈`, `\mathbf{\notin}`, `\text{∈}`, `\operatorname{∈}`} {
		if o, err := New(source, opt); err == nil || o != nil {
			t.Fatal("custom font silently substituted or name grammar bypassed", source)
		}
	}
	for _, source := range []string{`\cup`, `\cap`, `\mathbf{\cup}`} {
		if _, err := New(source, opt); err != nil {
			t.Fatal("custom font's real glyph rejected", source, err)
		}
	}
	opt = DefaultOptions()
	opt.BoldFont = customBold
	if o, err := New(`\mathbf{\in}`, opt); err == nil || o != nil {
		t.Fatal("explicit variant silently supplemented")
	}
	// Mathematical supplements are deliberately not a general text fallback.
	for _, source := range []string{`\text{∈}`, `\textbf{⊆}`, `\operatorname{A∈B}`} {
		if o, err := New(source, DefaultOptions()); err == nil || o != nil {
			t.Fatal("supplement escaped math atoms", source)
		}
	}
}

func TestSetSymbolErrorsSizesAndBudgets(t *testing.T) {
	for _, source := range []string{`\in\limits_i`, `\cup\nolimits`, `\not\in`, `\varnothing`, `\bigcup`, `\bigcap`, `\in_i_j`, `\notin^{`, `\input{file}`, "∈\xff"} {
		if o, err := New(source, DefaultOptions()); err == nil || o != nil {
			t.Fatal("invalid/unsupported source accepted", source, err)
		}
	}
	for _, size := range []float64{0, -1, 1e6 + 1, math.Inf(1), math.NaN(), math.SmallestNonzeroFloat64} {
		opt := DefaultOptions()
		opt.Size = size
		for _, source := range []string{`\in`, `\cup`, `x_{\notin}`} {
			if o, err := New(source, opt); err == nil || o != nil {
				t.Fatal("invalid size accepted", size, source)
			}
		}
	}
	boundary := strings.Repeat(" ", MaxSourceRunes-3) + `\in`
	if _, err := New(boundary, DefaultOptions()); err != nil {
		t.Fatal(err)
	}
	if _, err := New(boundary+" ", DefaultOptions()); err == nil {
		t.Fatal("source budget bypass")
	}
	for _, over := range []int{0, 1} {
		n := MaxNesting - 1 + over
		source := strings.Repeat("{", n) + `\notin` + strings.Repeat("}", n)
		_, err := New(source, DefaultOptions())
		if (err != nil) != (over == 1) {
			t.Fatal("nesting boundary", n, err)
		}
	}
	for command, r := range setSymbolCommands {
		total := countAccentCurves(mustLayout(t, `\`+command).object)
		for _, over := range []int{0, 1} {
			p := parser{options: DefaultOptions(), curves: MaxLayoutCurves - total + over}
			_, err := p.setSymbol(r, p.options.Size)
			if over == 0 && (err != nil || p.curves != MaxLayoutCurves) || over == 1 && (err == nil || !strings.Contains(err.Error(), "curves")) {
				t.Fatal("shared curve boundary", command, p.curves, err)
			}
		}
	}
	p := parser{source: []rune(`\hat{\in}_{x\notin A}+\mathbf{\cup\subseteq}`), options: DefaultOptions()}
	b, _, err := p.row(topRow, p.options.Size)
	if err != nil || p.curves != countAccentCurves(b.object) {
		t.Fatal("composite curve count differs", p.curves, err)
	}
}

func TestSetSymbolConcurrentGeometryOwnership(t *testing.T) {
	const source = `\mathbf{\notin\subseteq}+\mathit{\cup\cap}+\displaystyle\operatorname*{arg max}_{x\in R}\overset{\emptyset}{f}(x)`
	expected := mustLayout(t, source)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 6 {
				got, err := layout(source, DefaultOptions())
				if err != nil {
					t.Error(err)
					return
				}
				if got.object.Bounds() != expected.object.Bounds() || got.width != expected.width || got.height != expected.height || got.depth != expected.depth {
					t.Error("concurrent geometry differs")
					return
				}
				leaf := named(got.object, "∉")
				leaf.Path.Contours[0].Curves[0].P0.X += 100 // mutate an owned backing array
				got.object.Shift(geom.V(100, 100)).SetFill(gomath.Red)
			}
		}()
	}
	wg.Wait()
	requireSameDrawablePaths(t, expected.object, mustLayout(t, source).object)
	// Repeated identical glyphs in one formula also own independent curve slices.
	pair := mustLayout(t, `\in\in`)
	first, second := pair.object.Children[0], pair.object.Children[1]
	before := second.Bounds()
	named(first, "∈").Path.Contours[0].Curves[0].P0.Y += 100
	if second.Bounds() != before {
		t.Fatal("symbols share mutable contours")
	}
}

func BenchmarkSetSymbolsTypesetting(b *testing.B) {
	const source = `\displaystyle\operatorname*{arg max}_{x\in A\setminus B}\overset{\hat{n}}{f}(x)+\mathbf{A\subseteq B\cup C}+x\notin\emptyset`
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
