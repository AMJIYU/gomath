package mathtex

import (
	"gomath"
	"gomath/geom"
	"gomath/path"
	"gomath/text"
	"math"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func requireSameDrawablePaths(t *testing.T, a, b *gomath.Object) {
	t.Helper()
	paths := func(o *gomath.Object) []path.Path {
		var result []path.Path
		o.Walk(func(n *gomath.Object) {
			if len(n.Path.Contours) > 0 {
				result = append(result, n.Path)
			}
		})
		return result
	}
	if !reflect.DeepEqual(paths(a), paths(b)) {
		t.Fatal("actual path control points differ")
	}
}

func TestNamedOperatorLiteralUprightGlyphsAndUnicode(t *testing.T) {
	for _, name := range []string{"rank", "arg max", "arg  max", "L2", "log-10", "a.b", "Éλ2", "αβ", "Ранг2"} {
		for _, star := range []string{"", "*"} {
			b := mustLayout(t, `\operatorname`+star+`{`+name+`}`)
			literal := mustLayout(t, `\text{`+name+`}`)
			equalLayoutMetrics(t, b, literal)
			requireSameDrawablePaths(t, b.object, literal.object)
			checkLimitsBox(t, b)
		}
	}
	for _, style := range []text.FontStyle{text.Regular, text.Bold, text.Italic, text.BoldItalic} {
		opt := DefaultOptions()
		opt.Style, opt.Color = style, gomath.Blue
		b, err := layout(`\operatorname{réλ}`, opt)
		if err != nil {
			t.Fatal(err)
		}
		for _, letter := range "réλ" {
			sameGlyphShape(t, named(b.object, string(letter)), string(letter), style&text.Bold)
		}
		b.object.Walk(func(o *gomath.Object) {
			if len(o.Path.Contours) > 0 && o.Style.Fill != gomath.Blue {
				t.Fatal("named operator color lost")
			}
		})
	}
	for _, name := range []string{"lim", "sin", "max"} {
		// These literal names do not acquire special builtin-command defaults.
		b := mustLayout(t, `\displaystyle\operatorname{`+name+`}_a^b`)
		stackedOperator(t, b.object.Children[0], false)
	}
	if mustLayout(t, `\operatorname{arg  max}`).width <= mustLayout(t, `\operatorname{arg max}`).width {
		t.Fatal("literal internal spaces were collapsed")
	}
}

func TestNamedOperatorModesDirectivesAndOrder(t *testing.T) {
	for _, star := range []string{"", "*"} {
		for _, display := range []string{"", `\textstyle`, `\displaystyle`} {
			base := display + `\operatorname` + star + `{arg max}`
			b := mustLayout(t, base+`_g^n`)
			stackedOperator(t, b.object.Children[0], star == "*" && display == `\displaystyle`)
			for _, directive := range []string{`\limits`, `\nolimits`} {
				want := mustLayout(t, base+directive+`_{g}^{n}`)
				stackedOperator(t, want.object.Children[0], directive == `\limits`)
				for _, suffix := range []string{directive + `^{n}_{g}`, `^{n}` + directive + `_{g}`, `_{g}^{n}` + directive} {
					got := mustLayout(t, base+suffix)
					equalLayoutMetrics(t, got, want)
					requireSameDrawablePaths(t, got.object, want.object)
				}
			}
		}
	}
	a := mustLayout(t, `\operatorname * {rank}\limits_i^n`)
	b := mustLayout(t, `\operatorname*{rank}\limits_i^n`)
	equalLayoutMetrics(t, a, b)
	requireSameDrawablePaths(t, a.object, b.object)
	// The builtin lim and the explicitly starred literal lim share geometry.
	a = mustLayout(t, `\displaystyle\operatorname*{lim}_{x\rightarrow0}`)
	b = mustLayout(t, `\displaystyle\lim_{x\rightarrow0}`)
	equalLayoutMetrics(t, a, b)
	requireSameDrawablePaths(t, a.object, b.object)
	for _, directive := range []string{`\limits`, `\nolimits`} {
		a = mustLayout(t, `\operatorname{rank}`+directive)
		b = mustLayout(t, `\operatorname{rank}`)
		equalLayoutMetrics(t, a, b)
		requireSameDrawablePaths(t, a.object, b.object)
	}
}

func TestNamedOperatorStyleAndDeclarationScope(t *testing.T) {
	b := mustLayout(t, `\mathit{\operatorname{rank}_{q}x}`)
	for _, r := range "rank" {
		sameGlyphShape(t, named(b.object, string(r)), string(r), text.Regular)
	}
	// Script-size italic q and the following full-size x use the enclosing
	// font style, rather than leaking the operator's upright style.
	sameGlyphShape(t, named(b.object, "x"), "x", text.Italic)
	q := named(b.object, "q")
	if q == nil {
		t.Fatal("named operator side script missing")
	}
	opt := DefaultOptions()
	opt.Size *= .65
	opt.Style = text.Italic
	q0, err := New("q", opt)
	if err != nil {
		t.Fatal(err)
	}
	near(t, q.Bounds().Size().X, q0.Bounds().Size().X)
	near(t, q.Bounds().Size().Y, q0.Bounds().Size().Y)
	b = mustLayout(t, `\mathbf{\mathit{\operatorname*{rank}}}`)
	for _, r := range "rank" {
		sameGlyphShape(t, named(b.object, string(r)), string(r), text.Bold)
	}
	for _, tc := range []struct {
		source string
		first  bool
	}{
		{`{\displaystyle\operatorname*{arg max}_a^b}\operatorname*{rank}_c^d`, true},
		{`\displaystyle{\textstyle\operatorname*{arg max}_a^b}\operatorname{rank}_c^d`, false},
	} {
		b := mustLayout(t, tc.source)
		stackedOperator(t, b.object.Children[0].Children[0], tc.first)
		stackedOperator(t, b.object.Children[1], false)
	}
	for _, source := range []string{
		`\displaystyle x^{\operatorname*{rank}_a^b}\operatorname*{rank}_c^d`,
		`\displaystyle\frac{\operatorname*{rank}_a^b}{q}\operatorname*{rank}_c^d`,
	} {
		b := mustLayout(t, source)
		if named(b.object.Children[0], "OperatorLimits") != nil {
			t.Fatal("display style leaked into argument")
		}
		stackedOperator(t, b.object.Children[1], true)
	}
}

func TestExplicitStackGeometryBaselineAndOuterScripts(t *testing.T) {
	for _, command := range []string{"overset", "underset", "stackrel"} {
		for _, body := range []string{`x`, `g`, `=`, `\frac{}{}`, `x_i^2`, `\sum\limits_i^n`, `\operatorname{rank}`} {
			t.Run(command+body, func(t *testing.T) {
				plain := mustLayout(t, body)
				b := mustLayout(t, `\`+command+`{\hat{g}+\frac{a}{y}}{`+body+`}`)
				checkLimitsBox(t, b)
				o := b.object.Children[0]
				if o.Name != "Stack "+command || len(o.Children) != 2 {
					t.Fatal("missing actual stack geometry")
				}
				bodyInk, labelInk := o.Children[0].Bounds(), o.Children[1].Bounds()
				near(t, bodyInk.Min.Y, plain.object.Bounds().Min.Y)
				near(t, bodyInk.Max.Y, plain.object.Bounds().Max.Y)
				near(t, bodyInk.Size().X, plain.object.Bounds().Size().X)
				near(t, labelInk.Center().X, bodyInk.Center().X)
				if command == "underset" {
					if labelInk.Max.Y > -plain.depth-DefaultOptions().Size*.12+1e-10 {
						t.Fatal("under-label overlaps body")
					}
				} else if labelInk.Min.Y < plain.height+DefaultOptions().Size*.12-1e-10 {
					t.Fatal("over-label overlaps body")
				}
				opt := DefaultOptions()
				opt.Size *= .65
				label, err := New(`\hat{g}+\frac{a}{y}`, opt)
				if err != nil {
					t.Fatal(err)
				}
				near(t, labelInk.Size().X, label.Bounds().Size().X)
				near(t, labelInk.Size().Y, label.Bounds().Size().Y)
				outer := mustLayout(t, `\`+command+`{n}{`+body+`}_q^r`)
				s := outer.object.Children[0]
				if len(s.Children) != 4 {
					t.Fatal("outer scripts did not attach to compound stack")
				}
				for _, script := range s.Children[2:] {
					if script.Bounds().Min.X <= math.Max(s.Children[0].Bounds().Max.X, s.Children[1].Bounds().Max.X) {
						t.Fatal("outer script overlaps stack")
					}
				}
				followed := mustLayout(t, `\`+command+`{abcdefgh}{`+body+`}q`)
				q := followed.object.Children[1].Bounds()
				near(t, q.Min.Y, mustLayout(t, "q").object.Bounds().Min.Y)
				if q.Min.X <= followed.object.Children[0].Bounds().Max.X {
					t.Fatal("following atom overlaps stack")
				}
			})
		}
		for _, body := range []string{`g`, `\operatorname{rank}`} {
			empty := mustLayout(t, `\`+command+`{}{`+body+`}`)
			plain := mustLayout(t, body)
			equalLayoutMetrics(t, empty, plain)
			requireSameDrawablePaths(t, empty.object, plain.object)
		}
	}
	a := mustLayout(t, `\overset{\hat{n}}{=}`)
	b := mustLayout(t, `\stackrel{\hat{n}}{=}`)
	equalLayoutMetrics(t, a, b)
	requireSameDrawablePaths(t, a.object, b.object)
}

func TestExplicitStackNestedStyleAndScope(t *testing.T) {
	for _, source := range []string{
		`\overset{\displaystyle\sum_i^n}{\operatorname*{rank}_a^b}\sum_c^d`,
		`\underset{\displaystyle\sum_i^n}{\operatorname*{rank}_a^b}\sum_c^d`,
	} {
		b := mustLayout(t, source)
		s := b.object.Children[0]
		if named(s.Children[1], "OperatorLimits") == nil {
			t.Fatal("local annotation declaration lost")
		}
		if named(s.Children[0], "OperatorLimits") != nil {
			t.Fatal("annotation declaration leaked to body")
		}
		stackedOperator(t, b.object.Children[1], false)
	}
	b := mustLayout(t, `\displaystyle\overset{\textstyle\operatorname*{rank}_a^b}{\sum_c^d}\sum_e^f`)
	if named(b.object.Children[0].Children[1], "OperatorLimits") != nil {
		t.Fatal("annotation did not enter text style")
	}
	stackedOperator(t, b.object.Children[0].Children[0].Children[0], true)
	stackedOperator(t, b.object.Children[1], true)
	b = mustLayout(t, `\displaystyle\overset{n}{\textstyle\sum_c^d}\sum_e^f`)
	if named(b.object.Children[0], "OperatorLimits") != nil {
		t.Fatal("body declaration lost")
	}
	stackedOperator(t, b.object.Children[1], true)
	b = mustLayout(t, `\mathit{\overset{\mathbf{n}}{x}y}`)
	sameGlyphShape(t, named(b.object, "x"), "x", text.Italic)
	sameGlyphShape(t, named(b.object, "y"), "y", text.Italic)
	for _, source := range []string{
		`\overset{a}{\underset{b}{=}}`, `\underset{\overset{n}{m}}{\widehat{x}}`,
		`\widehat{\stackrel{\operatorname{def}}{=}}`, `\sqrt{\overset{\hat{n}}{\frac{g}{y}}}`,
		`\left[\underset{g}{\operatorname*{arg max}\limits_i^n}\right]_a`,
		`\begin{matrix}\overset{a}{x}&\operatorname{rank}\\\stackrel{n}{=}&\underset{g}{y}\end{matrix}`,
		`x^{\overset{n}{\operatorname{rank}}}`, `\frac{\underset{g}{x}}{\overset{n}{y}}`,
	} {
		checkLimitsBox(t, mustLayout(t, source))
	}
}

func TestNamedAndStackErrorsSizesAndBudgets(t *testing.T) {
	for _, name := range []string{"", " ", "-.", " rank", "rank ", "a\tb", "a\nb", "a\r", "a\x00b", "a\u00a0b", "a\u200db", "a\u202eb", "e\u0301", "ع", "א", "१", `x_i`, `x^2`, `a+b`, `a=b`, `a&b`, `a%b`, `a#b`, `a*b`, `{rank}`, `\alpha`, `arg\,max`, `\mathbf{rank}`, `\input{file}`, `\displaystyle rank`} {
		if o, err := New(`\operatorname{`+name+`}`, DefaultOptions()); err == nil || o != nil {
			t.Fatal("unsupported name semantics accepted", name)
		}
	}
	for _, source := range []string{
		`\operatorname`, `\operatorname*`, `\operatorname**{rank}`, `\operatorname rank`, `\operatorname{rank`,
		`\operatorname{中}`, `\operatorname{Ʞ}`, `\operatorname{rank}\limits\nolimits_a`, `\operatorname{rank}_a_a`,
		`{\operatorname{rank}}\limits`, `\overset{a}{\operatorname{rank}}\limits`,
		`\overset`, `\overset{a}`, `\overset a{x}`, `\overset{a}x`, `\overset{a}{}`, `\overset{a}{\quad}`,
		`\underset{a}{\text{ }}`, `\stackrel{a}{\left.\right.}`, `\overset{\unknown}{x}`, `\underset{a}{\input{file}}`,
		`\displaylimits`, `\mathop{rank}`, `\DeclareMathOperator{\foo}{rank}`,
	} {
		if o, err := New(source, DefaultOptions()); err == nil || o != nil {
			t.Fatal("invalid named/stack input accepted", source, err)
		}
	}
	for _, letter := range []string{"a", "λ"} {
		if _, err := New(`\operatorname{`+strings.Repeat(letter, MaxOperatorNameRunes)+`}`, DefaultOptions()); err != nil {
			t.Fatal("exact rune-count name bound rejected", err)
		}
		if _, err := New(`\operatorname{`+strings.Repeat(letter, MaxOperatorNameRunes+1)+`}`, DefaultOptions()); err == nil || !strings.Contains(err.Error(), "256") {
			t.Fatal("name rune-count budget bypassed", err)
		}
	}
	for _, atom := range []string{`\operatorname*{rank}_x`, `\overset{a}{x}`} {
		for _, extra := range []int{0, 1} {
			n := MaxNesting - 2 + extra
			_, err := New(strings.Repeat("{", n)+atom+strings.Repeat("}", n), DefaultOptions())
			if extra == 0 && err != nil || extra == 1 && (err == nil || !strings.Contains(err.Error(), "nesting")) {
				t.Fatal("named/stack nesting boundary wrong", atom, extra, err)
			}
		}
	}
	for _, source := range []string{
		strings.Repeat(`\overset{a}{`, MaxNesting) + `x` + strings.Repeat("}", MaxNesting),
		strings.Repeat(`\underset{`, MaxNesting) + `a` + strings.Repeat(`}{x}`, MaxNesting),
		`\operatorname{` + strings.Repeat("a", MaxSourceRunes) + `}`, `\overset{a}{` + strings.Repeat("x", MaxSourceRunes) + `}`,
		"\\operatorname{a\xffb}",
	} {
		if o, err := New(source, DefaultOptions()); err == nil || o != nil {
			t.Fatal("resource/UTF-8 budget bypass", err)
		}
	}
	for _, source := range []string{`\operatorname{rank}\limits_x^n`, `\overset{\hat{n}}{=}`, `\underset{g}{x}`} {
		for _, size := range []float64{.01, .55, 1e6} {
			opt := DefaultOptions()
			opt.Size = size
			b, err := layout(source, opt)
			if err != nil {
				t.Fatal(source, size, err)
			}
			checkLimitsBox(t, b)
		}
		for _, size := range []float64{math.SmallestNonzeroFloat64, math.Inf(1), math.NaN(), 0, 1e6 + 1} {
			opt := DefaultOptions()
			opt.Size = size
			if o, err := New(source, opt); err == nil || o != nil {
				t.Fatal("invalid/underflowed dimensions accepted", source, size)
			}
		}
	}
	const source = `\overset{\hat{n}}{\operatorname*{arg max}\limits_x^y}+\underset{g}{z}`
	want := countAccentCurves(mustLayout(t, source).object)
	for _, over := range []int{0, 1} {
		p := parser{source: []rune(source), options: DefaultOptions(), curves: MaxLayoutCurves - want + over}
		_, _, err := p.row(topRow, p.options.Size)
		if over == 0 && (err != nil || p.curves != MaxLayoutCurves) || over == 1 && (err == nil || !strings.Contains(err.Error(), "curves")) {
			t.Fatal("named/stack shared curve accounting wrong", p.curves, err)
		}
	}
}

func TestNamedStacksConcurrentIndependentOwnership(t *testing.T) {
	const source = `\displaystyle\operatorname*{arg max}_{\underset{g}{x}}^{\hat{n}}+\overset{\operatorname{def}}{\stackrel{m}{=}}`
	want := mustLayout(t, source)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 4 {
				got, err := layout(source, DefaultOptions())
				if err != nil {
					t.Error(err)
					return
				}
				if got.object.Bounds() != want.object.Bounds() || got.width != want.width || got.height != want.height || got.depth != want.depth {
					t.Error("concurrent layout differs")
				}
				got.object.Shift(geom.V(20, 20))
			}
		}()
	}
	wg.Wait()
}

func BenchmarkNamedOperatorStacksTypesetting(b *testing.B) {
	const source = `\displaystyle\operatorname*{arg max}_{x\infty}^{\hat{n}}\frac{\overset{m}{x}}{\underset{g}{y}}+a\stackrel{\operatorname{def}}{=}b`
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
