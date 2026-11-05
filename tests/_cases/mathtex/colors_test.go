package mathtex

import (
	"golang.org/x/image/font/gofont/goregular"
	"gomath"
	"gomath/geom"
	"gomath/text"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func mathPaints(o *gomath.Object) []gomath.Color {
	var colors []gomath.Color
	o.Walk(func(n *gomath.Object) {
		if len(n.Path.Contours) == 0 {
			return
		}
		if n.Style.Fill.A > 0 {
			colors = append(colors, n.Style.Fill)
		}
		if n.Style.Stroke.A > 0 && n.Style.StrokeWidth > 0 {
			colors = append(colors, n.Style.Stroke)
		}
	})
	return colors
}

func TestMathColorPaletteAndHTML(t *testing.T) {
	// Explicit independent RGB triples, not GoMath's animation constants.
	palette := map[string][3]float64{
		"black": {0, 0, 0}, "blue": {0, 0, 1}, "brown": {.75, .5, .25},
		"cyan": {0, 1, 1}, "darkgray": {.25, .25, .25}, "gray": {.5, .5, .5},
		"green": {0, 1, 0}, "lightgray": {.75, .75, .75}, "lime": {.75, 1, 0},
		"magenta": {1, 0, 1}, "olive": {.5, .5, 0}, "orange": {1, .5, 0},
		"pink": {1, .75, .75}, "purple": {.75, 0, .25}, "red": {1, 0, 0},
		"teal": {0, .5, .5}, "violet": {.5, 0, .5}, "white": {1, 1, 1}, "yellow": {1, 1, 0},
	}
	if len(palette) != len(mathColors) {
		t.Fatal("palette coverage changed")
	}
	for name, rgb := range palette {
		for _, source := range []string{`\color{` + name + `}x`, `\textcolor{` + name + `}{x}`} {
			got := mathPaints(mustLayout(t, source).object)
			want := []gomath.Color{gomath.RGB(rgb[0], rgb[1], rgb[2])}
			if !reflect.DeepEqual(got, want) {
				t.Fatal(source, got, want)
			}
		}
	}
	for _, source := range []string{`\color[HTML]{aB80fF}x`, `\textcolor [ HTML ] { aB80fF } {x}`} {
		got := mathPaints(mustLayout(t, source).object)
		if !reflect.DeepEqual(got, []gomath.Color{gomath.RGB(171.0/255, 128.0/255, 1)}) {
			t.Fatal("six digit RGB", source, got)
		}
	}
	for _, source := range []string{`\color{` + strings.Repeat(" ", 13) + `red}x`, `\color[` + strings.Repeat(" ", 12) + `HTML]{000000}x`} {
		mustLayout(t, source) // exactly sixteen token runes, including surrounding spaces
	}
	opt := DefaultOptions()
	opt.Color = gomath.Color{R: .2, G: .4, B: .6, A: .3}
	b, err := layout(`a{\color{red}b}c`, opt)
	if err != nil || !reflect.DeepEqual(mathPaints(b.object), []gomath.Color{opt.Color, gomath.RGB(1, 0, 0), opt.Color}) {
		t.Fatal("opaque command / original alpha scope", err)
	}
}

func TestMathColorScopesAndDescendants(t *testing.T) {
	w, r, g, b := gomath.White, gomath.RGB(1, 0, 0), gomath.RGB(0, 1, 0), gomath.RGB(0, 0, 1)
	for _, tc := range []struct {
		source string
		want   []gomath.Color
	}{
		{`a\color{red}b{\color{blue}c}d`, []gomath.Color{w, r, b, r}},
		{`\color{red}\textcolor{blue}{a\color{green}b}c`, []gomath.Color{b, g, r}},
		{`\frac{\color{red}a}{b}c`, []gomath.Color{r, w, w, w}},
		{`\color{red}\dfrac{\color{blue}a}{b}c`, []gomath.Color{b, r, r, r}},
		{`\color{green}\dbinom{\color{red}a}{b}c`, []gomath.Color{g, r, g, g, g}},
		{`\color{red}\left(\color{blue}a\right)b`, []gomath.Color{r, b, r, r}},
		{`\hat{\color{blue}a}b`, []gomath.Color{b, w, w}},
		{`\color{red}\overline{\textcolor{blue}{a}}b`, []gomath.Color{b, r, r}},
		{`\color{red}\sqrt{\color{blue}a}b`, []gomath.Color{r, b, r}},
		{`a_{\color{red}b}^{c}d`, []gomath.Color{w, w, r, w}},
		{`\color{red}\sum\limits_{\color{blue}a}^{b}c`, []gomath.Color{r, r, b, r}},
		{`\textcolor{red}{a}_b c`, []gomath.Color{r, w, w}},
		{`\begin{pmatrix}\color{red}a&b\\\textcolor{blue}{c}&d\end{pmatrix}e`, []gomath.Color{w, r, w, b, w, w, w}},
		{`\color{red}\substack{\color{blue}a\\b}c`, []gomath.Color{b, r, r}},
		{`\overset{\color{red}a}{b}c`, []gomath.Color{w, r, w}},
		{`\textcolor{red}{\operatorname*{arg max}\limits_a^b}c`, []gomath.Color{r, r, r, r, r, r, r, r, w}},
		{`\textcolor{blue}{\mathbb{R}\mathcal{A}\mathfrak{g}\mathbf{n}\in}`, []gomath.Color{b, b, b, b, b}},
		{`\textcolor{red}{\text{hi}}\textit{a}`, []gomath.Color{r, r, w}},
	} {
		t.Run(tc.source, func(t *testing.T) {
			object := mustLayout(t, tc.source).object
			if got := mathPaints(object); !reflect.DeepEqual(got, tc.want) {
				t.Fatal("drawable paints in tree order", got, tc.want)
			}
		})
	}
	for _, source := range []string{`\color{red}`, `\color{red}\color{blue}`, `\textcolor{red}{}`} {
		if b := mustLayout(t, source); b.width != 0 || b.height != 0 || b.depth != 0 || b.object.Bounds().Valid {
			t.Fatal("declaration/empty group creates geometry", source)
		}
	}
}

func TestMathColorGeometryAndFontsUnchanged(t *testing.T) {
	for _, pair := range [][2]string{
		{`abc`, `a\color{red}b\color[HTML]{008040}c`},
		{`{\dfrac{\hat{n}}{\mathbb{R}}}`, `\textcolor{blue}{\dfrac{\hat{n}}{\mathbb{R}}}`},
		{`\begin{matrix}a&\binom{n}{k}\\\substack{n\\g}&\sum_i^n\end{matrix}`, `\color{green}\begin{matrix}\color{red}a&\binom{n}{k}\\\substack{\color{blue}n\\g}&\sum_i^n\end{matrix}`},
	} {
		plain, colored := mustLayout(t, pair[0]), mustLayout(t, pair[1])
		equalLayoutMetrics(t, plain, colored)
		requireSameDrawablePaths(t, plain.object, colored.object)
		if plain.object.Bounds() != colored.object.Bounds() {
			t.Fatal("color changed actual bbox")
		}
	}
	for _, style := range []text.FontStyle{text.Regular, text.Bold, text.Italic, text.BoldItalic} {
		opt := DefaultOptions()
		opt.Style = style
		plain, err := layout(`{\hat{g}+\alpha+\mathbf{n}}x`, opt)
		if err != nil {
			t.Fatal(err)
		}
		colored, err := layout(`\textcolor{red}{\hat{g}+\alpha+\mathbf{n}}x`, opt)
		if err != nil {
			t.Fatal(err)
		}
		equalLayoutMetrics(t, plain, colored)
		requireSameDrawablePaths(t, plain.object, colored.object)
		paints := mathPaints(colored.object)
		for i, paint := range paints {
			want := gomath.RGB(1, 0, 0)
			if i == len(paints)-1 {
				want = opt.Color
			}
			if paint != want {
				t.Fatal("font style color scope", style, i, paint)
			}
		}
	}
	custom, err := text.Parse(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	opt := DefaultOptions()
	opt.Font = custom
	if _, err := New(`\textcolor{red}{g\frac{a}{n}}`, opt); err != nil {
		t.Fatal("custom font rejected", err)
	}
	for _, source := range []string{`\textcolor{red}{\mathbf{g}}`, `\color{red}\mathbb{R}`} {
		if o, err := New(source, opt); err == nil || o != nil {
			t.Fatal("color introduced font fallback", source)
		}
	}
}

func TestMathColorErrorsAndBudgets(t *testing.T) {
	for _, source := range []string{
		`\color`, `\color red`, `\color{}`, `\color{ }`, `\color{Red}x`, `\color{transparent}x`,
		`\color{#ff0000}x`, `\color{red!50!blue}x`, `\color{红}x`, `\color{re d}x`, `\color{red`,
		`\color{\input{file}}x`, `\color{{red}}x`, `\color[rgb]{1,0,0}x`, `\color[RGB]{255 0 0}x`,
		`\color[gray]{0}x`, `\color[cmyk]{0 0 0 1}x`, `\color[named]{red}x`, `\color[html]{ff0000}x`,
		`\color[]{ff0000}x`, `\color[HTML{x`, `\color[HTML]{ff000000}x`, `\color[HTML]{fff}x`,
		`\color[HTML]{00gg00}x`, `\color[HTML]{00 000}x`, `\color[HTML]{#000000}x`,
		`\textcolor{red}`, `\textcolor{red}x`, `\textcolor{red}{x`, `\textcolor{red}{\unknown}`,
		`x^\color{red}a`, `\hat\color{red}a`, `\textcolor{red}{\sum}\limits_i`,
		`\color{` + strings.Repeat("a", 17) + `}x`, `\color[` + strings.Repeat("A", 17) + `]{red}x`,
	} {
		if o, err := New(source, DefaultOptions()); err == nil || o != nil {
			t.Fatal("invalid/unsupported color syntax accepted", source, err)
		}
	}
	for _, source := range []string{`\color{red}\textcolor{blue}{\unknown}`, `\color{red}\frac{\color{blue}a}{\unknown}`} {
		p := parser{source: []rune(source), options: DefaultOptions()}
		if _, _, err := p.row(topRow, p.options.Size); err == nil || p.options.Color != DefaultOptions().Color {
			t.Fatal("error failed to restore color scope", err, p.options.Color)
		}
	}
	for _, over := range []int{0, 1} {
		depth := MaxNesting - 1 + over
		source := strings.Repeat(`\textcolor{red}{`, depth) + "x" + strings.Repeat("}", depth)
		if _, err := New(source, DefaultOptions()); (err != nil) != (over == 1) {
			t.Fatal("color nesting boundary", over, err)
		}
	}
	for _, over := range []int{0, 1} {
		const prefix = `\color{red}x`
		source := prefix + strings.Repeat(" ", MaxSourceRunes-len(prefix)+over)
		if _, err := New(source, DefaultOptions()); (err != nil) != (over == 1) {
			t.Fatal("source boundary", over, err)
		}
	}
	const source = `\textcolor{red}{\hat{\mathbb{R}}+\dfrac{n}{g}}`
	p := parser{source: []rune(source), options: DefaultOptions()}
	if _, _, err := p.row(topRow, p.options.Size); err != nil {
		t.Fatal(err)
	}
	total := p.curves
	for _, over := range []int{0, 1} {
		q := parser{source: []rune(source), options: DefaultOptions(), curves: MaxLayoutCurves - total + over}
		if _, _, err := q.row(topRow, q.options.Size); (err != nil) != (over == 1) {
			t.Fatal("outline budget boundary", over, err)
		}
		q = parser{source: []rune(`\color{red}\begin{matrix}a&b\end{matrix}`), options: DefaultOptions(), cells: MaxEnvironmentCells - 2 + over}
		if _, _, err := q.row(topRow, q.options.Size); (err != nil) != (over == 1) {
			t.Fatal("shared cell budget", over, err)
		}
	}
	if total == 0 {
		t.Fatal("colored outlines bypassed accounting")
	}
}

func TestMathColorOwnershipAndConcurrentLayouts(t *testing.T) {
	const source = `\color{red}\dfrac{\textcolor{blue}{\hat{\mathbb{R}}}}{\substack{\color{green}n\\g}}+\dbinom{n}{k}`
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
				if !reflect.DeepEqual(mathPaints(b.object), paints) || b.object.Bounds() != expected.object.Bounds() {
					t.Error("concurrent color layout differs")
				}
				copy := expected.object.Copy()
				copy.Shift(geom.V(10, 20)).SetColor(gomath.Black)
				b.object.SetColor(gomath.White)
			}
		}()
	}
	wg.Wait()
	if !reflect.DeepEqual(mathPaints(expected.object), paints) {
		t.Fatal("copy or unrelated layout mutated original paints")
	}
}

func BenchmarkMathColorLayout(b *testing.B) {
	for _, tc := range []struct{ name, source string }{
		{"uncolored", `\displaystyle\sum_{\substack{i=1\\j\in\mathbb{N}}}^n{\dfrac{\hat{n}}{\tfrac{g}{k}}}+{\dbinom{n}{k}}`},
		{"colored", `\color{red}\displaystyle\sum_{\substack{\color{blue}i=1\\j\in\mathbb{N}}}^n\textcolor[HTML]{008040}{\dfrac{\hat{n}}{\tfrac{g}{k}}}+\textcolor{violet}{\dbinom{n}{k}}`},
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
