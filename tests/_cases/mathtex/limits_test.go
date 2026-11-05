package mathtex

import (
	"gomath"
	"gomath/geom"
	"gomath/text"
	"math"
	"reflect"
	"strings"
	"sync"
	"testing"
)

var limitsCommands = []string{"sum", "prod", "lim", "int"}

func checkLimitsBox(t *testing.T, b box) {
	t.Helper()
	ink := b.object.Bounds()
	// At the 1e6 size ceiling an absolute 1e-10 check is smaller than a
	// coordinate ULP. Allow bounded floating-point roundoff at every scale.
	eps := 64 * (math.Nextafter(1, 2) - 1) * math.Max(1, math.Max(b.width, math.Max(b.height, b.depth)))
	if !ink.Valid || ink.Min.X < -eps || ink.Max.X > b.width+eps || ink.Max.Y > b.height+eps || ink.Min.Y < -b.depth-eps {
		t.Fatal("limits ink escapes layout box", ink, b.width, b.height, b.depth)
	}
	if err := b.object.Validate(); err != nil {
		t.Fatal(err)
	}
}

func stackedOperator(t *testing.T, o *gomath.Object, want bool) {
	t.Helper()
	if (o.Name == "OperatorLimits") != want {
		t.Fatalf("stacked operator = %t, want %t (%s)", o.Name == "OperatorLimits", want, o.Name)
	}
}

func equalLayoutMetrics(t *testing.T, a, b box) {
	t.Helper()
	near(t, a.width, b.width)
	near(t, a.height, b.height)
	near(t, a.depth, b.depth)
}

func TestLimitsActualCenteredGeometryBaselineAndAdvance(t *testing.T) {
	size := DefaultOptions().Size
	for _, command := range limitsCommands {
		for _, suffix := range []string{`^{g}`, `_{g}`, `_{abcdefgh}^{\frac{g}{q}}`, `_{\quad g\quad}^{\hat{y}}`} {
			t.Run(command+suffix, func(t *testing.T) {
				plain := mustLayout(t, `\`+command)
				b := mustLayout(t, `\`+command+`\limits`+suffix)
				checkLimitsBox(t, b)
				o := b.object.Children[0]
				stackedOperator(t, o, true)
				body := o.Children[0].Bounds()
				before := plain.object.Bounds()
				near(t, body.Min.Y, before.Min.Y)
				near(t, body.Max.Y, before.Max.Y)
				near(t, body.Size().X, before.Size().X)
				for _, child := range o.Children[1:] {
					ink := child.Bounds()
					near(t, ink.Center().X, body.Center().X)
					if child.Name == "UpperLimit" {
						if ink.Min.Y < plain.height+size*.12-1e-10 {
							t.Fatal("upper limit lost vertical clearance", ink, body)
						}
					} else if child.Name == "LowerLimit" {
						if ink.Max.Y > -plain.depth-size*.12+1e-10 {
							t.Fatal("lower limit lost vertical clearance", ink, body)
						}
					} else {
						t.Fatal("unidentified limits child", child.Name)
					}
				}
				// A following ordinary atom must retain the same baseline and clear
				// the complete wide-limit advance, rather than the body width.
				followed := mustLayout(t, `\`+command+`\limits`+suffix+`q`)
				q := followed.object.Children[1].Bounds()
				q0 := mustLayout(t, `q`).object.Bounds()
				near(t, q.Min.Y, q0.Min.Y)
				near(t, q.Max.Y, q0.Max.Y)
				if q.Min.X <= followed.object.Children[0].Bounds().Max.X {
					t.Fatal("following atom overlaps the limits advance")
				}
			})
		}
	}
}

func TestLimitsScriptAndDirectiveOrder(t *testing.T) {
	for _, command := range limitsCommands {
		for _, directive := range []string{`\limits`, `\nolimits`} {
			want := mustLayout(t, `\`+command+directive+`_{g}^{n}`)
			for _, suffix := range []string{
				directive + `^{n}_{g}`, `_{g}` + directive + `^{n}`,
				`^{n}` + directive + `_{g}`, `_{g}^{n}` + directive, `^{n}_{g}` + directive,
			} {
				got := mustLayout(t, `\`+command+suffix)
				equalLayoutMetrics(t, got, want)
				// Exact geometry comparison ignores fresh identities, checking all
				// independent paths and their order, not just the outer bbox.
				var a, b []*gomath.Object
				got.object.Walk(func(n *gomath.Object) {
					if len(n.Path.Contours) > 0 {
						a = append(a, n)
					}
				})
				want.object.Walk(func(n *gomath.Object) {
					if len(n.Path.Contours) > 0 {
						b = append(b, n)
					}
				})
				if len(a) != len(b) {
					t.Fatal("script order changed the drawable count")
				}
				for i := range a {
					if !reflect.DeepEqual(a[i].Path, b[i].Path) {
						t.Fatal("script order changed actual geometry", command, suffix)
					}
				}
			}
		}
	}
}

func TestLimitsDefaultInlineAndDisplayOverrides(t *testing.T) {
	for _, command := range limitsCommands {
		inline := mustLayout(t, `\`+command+`_g^n`)
		no := mustLayout(t, `\`+command+`\nolimits_g^n`)
		textMode := mustLayout(t, `\textstyle\`+command+`_g^n`)
		equalLayoutMetrics(t, inline, no)
		equalLayoutMetrics(t, inline, textMode)
		for _, b := range []box{inline, no, textMode} {
			o := b.object.Children[0]
			stackedOperator(t, o, false)
			for _, script := range o.Children[1:] {
				if script.Bounds().Min.X <= o.Children[0].Bounds().Max.X {
					t.Fatal("inline script moved off the side")
				}
			}
		}
		display := mustLayout(t, `\displaystyle\`+command+`_g^n`)
		stackedOperator(t, display.object.Children[0], command != "int")
		explicit := mustLayout(t, `\displaystyle\`+command+`\nolimits_g^n`)
		equalLayoutMetrics(t, explicit, inline)
		explicit = mustLayout(t, `\textstyle\`+command+`\limits_g^n`)
		stackedOperator(t, explicit.object.Children[0], true)
		// Declarations never substitute or scale the actual operator body.
		plain := mustLayout(t, `\`+command)
		displayPlain := mustLayout(t, `\displaystyle\`+command)
		equalLayoutMetrics(t, plain, displayPlain)
	}
	// Declarations do not create atoms, spacing, or visible placeholder paths.
	equalLayoutMetrics(t, mustLayout(t, `a\displaystyle b\textstyle c`), mustLayout(t, `abc`))
	equalLayoutMetrics(t, mustLayout(t, `\displaystyle\textstyle`), mustLayout(t, ``))
}

func TestLimitsDeclarationScope(t *testing.T) {
	for _, tc := range []struct {
		source        string
		first, second bool
	}{
		{`{\displaystyle\sum_a^b}\sum_c^d`, true, false},
		{`\displaystyle{\textstyle\sum_a^b}\sum_c^d`, false, true},
		{`\left(\displaystyle\sum_a^b\right)\sum_c^d`, true, false},
		{`\displaystyle\left(\textstyle\sum_a^b\right)\sum_c^d`, false, true},
		{`\mathbf{\displaystyle\sum_a^b}\sum_c^d`, true, false},
		{`\displaystyle\sum_a^b\textstyle\sum_c^d`, true, false},
	} {
		b := mustLayout(t, tc.source)
		first := b.object.Children[0]
		found := false
		first.Walk(func(n *gomath.Object) {
			if n.Name == "OperatorLimits" {
				found = true
			}
		})
		if found != tc.first {
			t.Fatal("declaration scope wrong inside group", tc.source)
		}
		stackedOperator(t, b.object.Children[1], tc.second)
		checkLimitsBox(t, b)
	}
	// Row scopes include each individual matrix cell and row, and declarations
	// inside them never alter a following cell or the containing math row.
	b := mustLayout(t, `\begin{matrix}\displaystyle\sum_a^b&\sum_c^d\\\sum_e^f&\displaystyle\sum_g^h\end{matrix}\sum_i^j`)
	cells := b.object.Children[0].Children
	for i, cell := range cells {
		stackedOperator(t, cell.Children[0], i == 0 || i == 3)
	}
	stackedOperator(t, b.object.Children[1], false)
	b = mustLayout(t, `\displaystyle\begin{matrix}\textstyle\sum_a^b&\sum_c^d\end{matrix}\sum_e^f`)
	stackedOperator(t, b.object.Children[0].Children[0].Children[0], false)
	stackedOperator(t, b.object.Children[0].Children[1].Children[0], true)
	stackedOperator(t, b.object.Children[1], true)
}

func TestLimitsNestedScriptAndFractionStyle(t *testing.T) {
	for _, source := range []string{
		`\displaystyle x^{\sum_a^b}\sum_c^d`,
		`\displaystyle x_{\sum_a^b}\sum_c^d`,
		`\displaystyle\frac{\sum_a^b}{\sum_c^d}\sum_e^f`,
		`\displaystyle\dfrac{\sum_a^b}{\sum_c^d}\sum_e^f`,
		`\displaystyle\tfrac{\sum_a^b}{\sum_c^d}\sum_e^f`,
	} {
		b := mustLayout(t, source)
		b.object.Children[0].Walk(func(n *gomath.Object) {
			if n.Name == "OperatorLimits" {
				t.Fatal("implicit display style leaked into script/fraction", source)
			}
		})
		stackedOperator(t, b.object.Children[1], true)
	}
	for _, source := range []string{
		`x^{\displaystyle\sum_a^b}\sum_c^d`,
		`x_{\displaystyle\sum_a^b}\sum_c^d`,
		`\frac{\displaystyle\sum_a^b}{q}\sum_c^d`,
		`\frac{q}{\displaystyle\sum_a^b}\sum_c^d`,
	} {
		b := mustLayout(t, source)
		if named(b.object.Children[0], "OperatorLimits") == nil {
			t.Fatal("explicit local display style lost", source)
		}
		stackedOperator(t, b.object.Children[1], false)
		checkLimitsBox(t, b)
	}
	for _, source := range []string{
		`\sum\limits_{\displaystyle\prod_{a}^{b}}^{\lim\limits_{x\rightarrow0}\frac{a}{b}}`,
		`\widehat{\sum\limits_{\underline{x}}^{\overline{g}}}`,
		`\underline{\int\limits_{\frac{g}{y}}^{\ddot{n}}}`, `\sqrt{\displaystyle\sum_{g}^{n}}`,
		`\left[\sum\limits_{g}^{\hat{n}}\right]_{i}^{2}`,
	} {
		checkLimitsBox(t, mustLayout(t, source))
	}
}

func TestLimitsStylesSizesAndEmptyBounds(t *testing.T) {
	for _, command := range limitsCommands {
		for _, style := range []text.FontStyle{text.Regular, text.Bold, text.Italic, text.BoldItalic} {
			for _, size := range []float64{.01, .55, 1e6} {
				opt := DefaultOptions()
				opt.Style, opt.Size, opt.Color = style, size, gomath.Blue
				b, err := layout(`\`+command+`\limits_{abcdefgh}^{\frac{g}{y}}`, opt)
				if err != nil {
					t.Fatal(command, style, size, err)
				}
				checkLimitsBox(t, b)
				b.object.Walk(func(n *gomath.Object) {
					for _, contour := range n.Path.Contours {
						if contour.Closed && n.Style.Fill != gomath.Blue {
							t.Fatal("limits fill color lost")
						}
						if n.Style.StrokeWidth > 0 && n.Style.Stroke != gomath.Blue {
							t.Fatal("limits stroke color lost")
						}
					}
				})
				if size == .55 {
					value, wantStyle := symbols[command], style
					if command == "lim" {
						value, wantStyle = "lim", style&text.Bold
						for _, letter := range value {
							sameGlyphShape(t, named(b.object, string(letter)), string(letter), wantStyle)
						}
					} else {
						sameGlyphShape(t, named(b.object, value), value, wantStyle)
					}
				}
			}
		}
		plain := mustLayout(t, `\`+command)
		for _, suffix := range []string{`\limits`, `\nolimits`, `\limits_{}^{}`} {
			equalLayoutMetrics(t, mustLayout(t, `\`+command+suffix), plain)
		}
	}
	// Long limits expand the advance and keep all their visible ink centered.
	b := mustLayout(t, `\sum\limits_{`+strings.Repeat("g", 300)+`}^{`+strings.Repeat("n", 250)+`}`)
	checkLimitsBox(t, b)
	if b.width < DefaultOptions().Size*50 {
		t.Fatal("long limits were squeezed into the body")
	}
}

func TestLimitsInvalidPositionsAndBudgets(t *testing.T) {
	for _, source := range []string{
		`\limits`, `\nolimits`, `x\limits_a^b`, `1\nolimits`, `∑\limits_a`,
		`\sin\limits_a`, `\max\limits_a`, `{\sum}\limits_a`, `\mathbf{\sum}\limits_a`,
		`\hat{\sum}\limits_a`, `\sum\quad\limits_a`, `x^{a}\limits_b`,
		`\sum\limits\nolimits_a`, `\sum\limits_a\limits^b`, `\sum\nolimits_a^b\nolimits`,
		`\sum\limits_a_a`, `\sum^a\limits^b`, `\sum\limits_`, `\sum\limits^`,
		`\sum_\displaystyle`, `\hat\displaystyle x`, `\displaystyle_a`,
		`\sum\displaylimits_a`, `\operatorname{\sum}\limits_a`, `\scriptstyle\sum_a^b`,
		`\displaystyle\input{file}`, `\sum\limits_{\unknown}`,
	} {
		o, err := New(source, DefaultOptions())
		if err == nil || o != nil || !strings.Contains(err.Error(), "math at rune") {
			t.Fatal("invalid operator syntax accepted", source, err)
		}
	}
	for _, source := range []string{
		strings.Repeat(`\sum\limits_{`, MaxNesting) + "x" + strings.Repeat("}", MaxNesting),
		strings.Repeat(`{\displaystyle`, MaxNesting) + `\sum_a^b` + strings.Repeat("}", MaxNesting),
		strings.Repeat(`\displaystyle `, MaxSourceRunes/14+1),
		`\sum\limits_{` + strings.Repeat("x", MaxSourceRunes) + `}`,
	} {
		if o, err := New(source, DefaultOptions()); err == nil || o != nil {
			t.Fatal("limits bypassed source/depth budget")
		}
	}
	for _, size := range []float64{0, -1, 1e6 + 1, math.Inf(1), math.NaN(), math.SmallestNonzeroFloat64} {
		opt := DefaultOptions()
		opt.Size = size
		if o, err := New(`\sum\limits_a^b`, opt); err == nil || o != nil {
			t.Fatal("invalid/underflowed limits size accepted", size)
		}
	}
	// Both forms accept the exact active-frame boundary, then reject the
	// otherwise identical source with one more enclosing group.
	for _, tc := range []struct {
		script string
		groups int
	}{
		{`^x`, MaxNesting - 2},
		{`^{x}`, MaxNesting - 3},
	} {
		for _, extra := range []int{0, 1} {
			n := tc.groups + extra
			source := strings.Repeat("{", n) + `\sum\limits` + tc.script + strings.Repeat("}", n)
			_, err := New(source, DefaultOptions())
			if extra == 0 && err != nil || extra == 1 && (err == nil || !strings.Contains(err.Error(), "nesting")) {
				t.Fatal("script nesting boundary wrong", tc.script, extra, err)
			}
		}
	}
	const source = `\displaystyle\sum\limits_{\widehat{a}}^{\prod\limits_b^c}`
	want := countAccentCurves(mustLayout(t, source).object)
	for _, over := range []int{0, 1} {
		p := parser{source: []rune(source), options: DefaultOptions(), curves: MaxLayoutCurves - want + over}
		b, _, err := p.row(topRow, p.options.Size)
		if over == 0 {
			if err != nil || p.curves != MaxLayoutCurves || countAccentCurves(b.object) != want {
				t.Fatal("limits curve budget miscounted", p.curves, err)
			}
		} else if err == nil || !strings.Contains(err.Error(), "curves") {
			t.Fatal("limits exceeded shared curve budget", err)
		}
	}
}

func TestLimitsConcurrentDeclarationAndGeometryOwnership(t *testing.T) {
	const source = `\displaystyle\sum_{\frac{g}{y}}^{\hat{n}}+{\textstyle\prod_a^b}+\int\limits_0^1`
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
				if got.width != want.width || got.height != want.height || got.depth != want.depth || got.object.Bounds() != want.object.Bounds() {
					t.Error("concurrent limits layout differs")
				}
				got.object.Shift(geom.V(10, 10))
			}
		}()
	}
	wg.Wait()
}

func BenchmarkOperatorLimitsNestedTypesetting(b *testing.B) {
	const source = `\displaystyle\sum_{i=1}^{\hat{n}}\frac{x_i}{\prod\limits_{j=1}^{m}j}+\lim\limits_{x\rightarrow0}\int\limits_0^1f(x)dx`
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
