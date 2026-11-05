package mathtex

import (
	"encoding/json"
	"golang.org/x/image/font/gofont/goregular"
	"gomath"
	"gomath/geom"
	"gomath/text"
	"math"
	"strings"
	"sync"
	"testing"
)

func rbLayout(t *testing.T, source string) box {
	t.Helper()
	opt := DefaultOptions()
	opt.Size = 1
	b, err := layout(source, opt)
	if err != nil {
		t.Fatal(source, err)
	}
	return b
}
func rbNear(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-12 {
		t.Fatalf("got %.17g want %.17g", got, want)
	}
}
func rbNamed(o *gomath.Object, name string) *gomath.Object {
	var found *gomath.Object
	o.Walk(func(n *gomath.Object) {
		if found == nil && n.Name == name {
			found = n
		}
	})
	return found
}
func rbCurves(o *gomath.Object) int {
	n := 0
	o.Walk(func(o *gomath.Object) {
		for _, c := range o.Path.Contours {
			n += len(c.Curves)
		}
	})
	return n
}

func TestRadicalBoxPhantomSemanticExtents(t *testing.T) {
	for _, operand := range []string{`g`, `\frac{n^i}{g_j}`, `\sqrt{g}`, `\left(\dfrac{n}{g}\right)`, `\begin{matrix}g&n\\i&W\end{matrix}`, `\quad`, `{}`} {
		body := rbLayout(t, operand)
		for _, command := range []string{"phantom", "hphantom", "vphantom"} {
			b := rbLayout(t, `\`+command+`{`+operand+`}`)
			if b.object.Bounds().Valid || rbCurves(b.object) != 0 {
				t.Fatal("phantom emitted ink", command, operand)
			}
			w, h, d := body.width, body.height, body.depth
			if command == "hphantom" {
				h, d = 0, 0
			}
			if command == "vphantom" {
				w = 0
			}
			rbNear(t, b.width, w)
			rbNear(t, b.height, h)
			rbNear(t, b.depth, d)
			row := rbLayout(t, `A\`+command+`{`+operand+`}B`)
			a, bb := rbLayout(t, "A"), rbLayout(t, "B")
			rbNear(t, row.width, a.width+.04+w+.04+bb.width)
			rbNear(t, rbNamed(row.object, "B").Bounds().Min.X, a.width+.08+w+rbNamed(bb.object, "B").Bounds().Min.X)
		}
	}
	// Invisible ascent/depth must still affect script placement and delimiters.
	plain := rbLayout(t, `g^n_i`)
	hidden := rbLayout(t, `\phantom{g}^n_i`)
	rbNear(t, hidden.width, plain.width)
	rbNear(t, hidden.height, plain.height)
	rbNear(t, hidden.depth, plain.depth)
	for _, glyph := range []string{"n", "i"} {
		if rbNamed(hidden.object, glyph).Bounds() != rbNamed(plain.object, glyph).Bounds() {
			t.Fatal("script baseline changed", glyph)
		}
	}
	a := rbLayout(t, `\left(\vphantom{\dfrac{n}{g}}\right)`)
	b := rbLayout(t, `\left(\dfrac{n}{g}\right)`)
	rbNear(t, a.height, b.height)
	rbNear(t, a.depth, b.depth)
}

func TestRadicalBoxOutlineAndScripts(t *testing.T) {
	for _, operand := range []string{`g`, `\quad`, `\phantom{\frac{n}{g}}`, `\textcolor{red}{\mathbb{R}}`, `{}`} {
		body := rbLayout(t, operand)
		b := rbLayout(t, `\boxed{`+operand+`}`)
		rbNear(t, b.width, body.width+.49)
		rbNear(t, b.height, body.height+.245)
		rbNear(t, b.depth, body.depth+.245)
		border := rbNamed(b.object, "BoxedRule")
		if border == nil || rbCurves(border) != 4 || !border.Path.Contours[0].Closed || border.Style.Fill != gomath.Transparent || border.Style.StrokeJoin != gomath.JoinRound {
			t.Fatal("not a closed outline")
		}
		rbNear(t, border.Style.StrokeWidth, .045)
		bounds := border.Bounds()
		rbNear(t, bounds.Min.X, .0225)
		rbNear(t, bounds.Max.X, b.width-.0225)
		rbNear(t, bounds.Max.Y, b.height-.0225)
		rbNear(t, bounds.Min.Y, -b.depth+.0225)
		if body.object.Bounds().Valid {
			want := body.object.Bounds()
			want.Min.X += .245
			want.Max.X += .245
			if rbNamed(b.object, "BoxedBody").Bounds() != want {
				t.Fatal("boxed body baseline/padding", operand)
			}
		}
		withScripts := rbLayout(t, `\boxed{`+operand+`}_i^n`)
		sup := rbLayout(t, `n`)
		// Script glyph metrics scale .65; side scripts start beyond outer stroke.
		rbNear(t, rbNamed(withScripts.object, "n").Bounds().Min.X, b.width+.04+rbNamed(sup.object, "n").Bounds().Min.X*.65)
		rbNear(t, rbNamed(withScripts.object, "i").Bounds().Max.Y, -math.Max(.25, b.depth+.08))
	}
}

func TestRadicalBoxIndexPlacementAndScopes(t *testing.T) {
	for _, index := range []string{`3`, `n_i`, `\frac{g}{n}`, `\sqrt[3]{g}`, `{[n]}`, `\left[n\right]`, `\text{a]b}`, `\phantom{n}`, `\quad`} {
		b := rbLayout(t, `\sqrt[`+index+`]{g}`)
		rad := rbLayout(t, "g")
		opt := DefaultOptions()
		opt.Size = .5
		ib, err := layout(index, opt)
		if err != nil {
			t.Fatal(err)
		}
		start := .0225 + math.Max(0, ib.width-.18)
		top := math.Max(.4, rad.height+.1)
		iy := math.Max(.28, .55*top) + ib.depth
		rbNear(t, b.width, start+.48+rad.width+.05+.0225)
		rbNear(t, b.height, math.Max(top+.0225, iy+ib.height))
		rbNear(t, b.depth, rad.depth+.0825)
		rule := rbNamed(b.object, "RadicalRule")
		if rule == nil || rbCurves(rule) != 4 || rule.Style.StrokeJoin != gomath.JoinRound {
			t.Fatal("missing real radical path")
		}
		if ib.object.Bounds().Valid {
			got := rbNamed(b.object, "RootIndex").Bounds()
			want := ib.object.Bounds()
			dx := start + .18 - ib.width
			want.Min = want.Min.Add(geom.V(dx, iy))
			want.Max = want.Max.Add(geom.V(dx, iy))
			if got != want {
				t.Fatal("root index positioning", index, got, want)
			}
		}
		rbNear(t, rbNamed(b.object, "g").Bounds().Min.Y, rbNamed(rad.object, "g").Bounds().Min.Y)
	}
	// Explicit style and color declarations are scoped to their operand/row.
	b := rbLayout(t, `\color{green}\sqrt[\color{red}\displaystyle\sum_i^n]{\color{blue}g}\boxed{\color{red}n}W`)
	if rbNamed(b.object, "RadicalRule").Style.Stroke != mathColors["green"] || rbNamed(b.object, "BoxedRule").Style.Stroke != mathColors["green"] {
		t.Fatal("operand color leaked to border")
	}
	for _, spec := range []struct{ glyph, color string }{{"W", "green"}, {"g", "blue"}} {
		rbNamed(b.object, spec.glyph).Walk(func(n *gomath.Object) {
			if len(n.Path.Contours) > 0 && n.Style.Fill != mathColors[spec.color] {
				t.Error("operand color leaked across its group", spec.glyph)
			}
		})
	}
	if rbNamed(b.object, "RootIndex") == nil || rbNamed(rbNamed(b.object, "RootIndex"), "OperatorLimits") == nil {
		t.Fatal("local display declaration lost")
	}
	b = rbLayout(t, `\displaystyle\sqrt[\sum_i^n]{\sum_i^n}\sum_i^n`)
	if rbNamed(rbNamed(b.object, "RootIndex"), "OperatorLimits") != nil || rbNamed(rbNamed(b.object, "Radicand"), "OperatorLimits") == nil {
		t.Fatal("index text style/radicand display scope")
	}
	// An indexed radical inside an ordinary script uses the local .65em,
	// and its own index starts at half that, without restoring nominal em.
	b = rbLayout(t, `x^{\sqrt[n]{g}}`)
	for _, spec := range []struct {
		glyph string
		scale float64
	}{{"n", .65 * .5}, {"g", .65}} {
		got, want := rbNamed(b.object, spec.glyph).Bounds(), rbNamed(rbLayout(t, spec.glyph).object, spec.glyph).Bounds()
		rbNear(t, got.Size().X, want.Size().X*spec.scale)
		rbNear(t, got.Size().Y, want.Size().Y*spec.scale)
	}
	for _, source := range []string{`\mathbf{\mathit{\sqrt[n]{g}}}`, `x^{\sqrt[n_i]{\boxed{g}}}`, `\boxed{\begin{aligned}a&=\sqrt[\hat{n}]{g}\\b&=\hphantom{g}W\end{aligned}}`, `\sqrt[\substack{n\\g}]{\mathbb{R}}`} {
		if _, err := New(source, DefaultOptions()); err != nil {
			t.Fatal(source, err)
		}
	}
}

func TestRadicalBoxErrorsAndBudgets(t *testing.T) {
	for _, source := range []string{`\sqrt[]{x}`, `\sqrt[{}]{x}`, `\sqrt[3{x}`, `\sqrt[3]x`, `\sqrt[3]{`, `\sqrt[[n]]{g}`, `\sqrt[n^[x]]{g}`, `\sqrt[n^]x]{g}`, `\sqrt[n&x]{g}`, `\sqrt[n\\x]{g}`, `\sqrt[\unknown]{g}`, `\boxed x`, `\phantom x`, `\hphantom{\unknown}`, `\vphantom{\mathbb{β}}`, `\boxed{\color{unknown}x}`, `\phantom{\sum\limits_i_i}`, `\sqrt[\phantom{}]{x}`} {
		o, err := New(source, DefaultOptions())
		if err == nil || o != nil {
			t.Fatal("expected error/nil", source, err)
		}
	}
	for _, size := range []float64{math.SmallestNonzeroFloat64, 1e-320, math.Inf(1), math.NaN(), 1e7} {
		for _, source := range []string{`\sqrt[n]{g}`, `\boxed{}`, `\phantom{}`} {
			opt := DefaultOptions()
			opt.Size = size
			if o, err := New(source, opt); err == nil || o != nil {
				t.Fatal("numeric error/nil", size, source)
			}
		}
	}
	for _, source := range []string{strings.Repeat("x", MaxSourceRunes+1), strings.Repeat(`\phantom{`, MaxNesting) + "g" + strings.Repeat("}", MaxNesting), strings.Repeat(`\sqrt[n]{`, MaxNesting) + "g" + strings.Repeat("}", MaxNesting)} {
		if o, err := New(source, DefaultOptions()); err == nil || o != nil {
			t.Fatal("source/nesting limit")
		}
	}
	row := strings.Repeat("&", 63)
	grid := `\begin{matrix}` + strings.Repeat(row+`\\`, 63) + row + `\end{matrix}`
	if _, err := New(`\phantom{`+grid+`}`, DefaultOptions()); err != nil {
		t.Fatal("boundary cell count", err)
	}
	if o, err := New(`\boxed{\begin{matrix}`+grid+`\end{matrix}}`, DefaultOptions()); err == nil || o != nil {
		t.Fatal("hidden cell budget not shared")
	}
	if o, err := New(`\sqrt[`+grid+`]{\begin{matrix}x\end{matrix}}`, DefaultOptions()); err == nil || o != nil {
		t.Fatal("index and radicand cell budgets not shared")
	}
	opt := DefaultOptions()
	opt.Font = text.DefaultFont()
	opt.BoldFont = nil
	custom, err := text.Parse(testFontBytes())
	if err != nil {
		t.Fatal(err)
	}
	opt.Font = custom
	if o, err := New(`\phantom{\mathbf{x}}`, opt); err == nil || o != nil {
		t.Fatal("hidden missing font variant")
	}
}

// A tiny real font fixture is unnecessary: an independently parsed default face
// exercises custom-font variant policy while keeping every outline identical.
func testFontBytes() []byte { return goregular.TTF }

func TestRadicalBoxSharedCurveLedger(t *testing.T) {
	opt := DefaultOptions()
	opt.Size = 1
	for _, operand := range []string{`\sqrt{g}`, `\frac{n}{g}`, `\left(\frac{n}{g}\right)`, `\binom{n}{g}`, `\hat{n}`} {
		count := rbCurves(rbLayout(t, operand).object)
		for _, source := range []string{`\phantom{` + operand + `}`, `\phantom{\phantom{` + operand + `}}`, `\phantom{\phantom{g}` + operand + `}`} {
			want := count
			if strings.Contains(source, `\phantom{g}`) {
				want += rbCurves(rbLayout(t, "g").object)
			}
			p := parser{source: []rune(source), options: opt}
			if _, _, err := p.row(topRow, 1); err != nil {
				t.Fatal(err)
			}
			if p.curves != want {
				t.Fatal("wrong hidden curve accounting", source, p.curves, want)
			}
			p = parser{source: []rune(source), options: opt, curves: MaxLayoutCurves - want}
			if _, _, err := p.row(topRow, 1); err != nil {
				t.Fatal("exact curve budget", source, err)
			}
			p = parser{source: []rune(source), options: opt, curves: MaxLayoutCurves - want + 1}
			if _, _, err := p.row(topRow, 1); err == nil {
				t.Fatal("hidden curve budget bypass", source)
			}
		}
	}
	for _, source := range []string{`\sqrt[n]{\frac{n}{g}}`, `\boxed{\left(\sqrt{g}\right)}`, `\boxed{\boxed{\sqrt[n]{g}}}`} {
		count := rbCurves(rbLayout(t, source).object)
		p := parser{source: []rune(source), options: opt}
		if _, _, err := p.row(topRow, 1); err != nil {
			t.Fatal(err)
		}
		if p.curves != count {
			t.Fatal("new visible curve charging", source, p.curves, count)
		}
		p = parser{source: []rune(source), options: opt, curves: MaxLayoutCurves - count}
		if _, _, err := p.row(topRow, 1); err != nil {
			t.Fatal("new structure exact curve limit", source, err)
		}
		p = parser{source: []rune(source), options: opt, curves: MaxLayoutCurves - count + 1}
		if _, _, err := p.row(topRow, 1); err == nil {
			t.Fatal("new structure curve limit bypass", source)
		}
	}
}

func TestRadicalBoxConcurrentOwnership(t *testing.T) {
	const source = `\color{blue}\boxed{\sqrt[\textcolor{red}{\hat{n}_i}]{\dfrac{\mathbb{R}}{g}}\phantom{\frac{n}{g}}W}_i^n`
	want := rbLayout(t, source)
	type draw struct {
		Path  any
		Style gomath.Style
	}
	digest := func(o *gomath.Object) string {
		d := []draw{}
		o.Walk(func(n *gomath.Object) {
			if len(n.Path.Contours) > 0 {
				d = append(d, draw{n.Path, n.Style})
			}
		})
		v, _ := json.Marshal(d)
		return string(v)
	}
	expected := digest(want.object)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 12 {
				b, err := layout(source, func() Options { o := DefaultOptions(); o.Size = 1; return o }())
				if err != nil {
					t.Error(err)
					return
				}
				if digest(b.object) != expected {
					t.Error("concurrent geometry changed")
					return
				}
				b.object.Shift(geom.V(10, 20)).SetFill(gomath.Red)
				if digest(want.object) != expected {
					t.Error("layout ownership shared")
					return
				}
			}
		}()
	}
	wg.Wait()
}

var rbBenchmarkSink *gomath.Object

func BenchmarkRadicalBoxes(b *testing.B) {
	for _, tc := range []struct{ name, source string }{
		{"indexed", `\sqrt[\hat{n}_i]{\dfrac{\mathbb{R}}{g}}`},
		{"boxed-phantoms", `\boxed{\sqrt[3]{g}\phantom{\frac{n}{g}}+\hphantom{W}\vphantom{\dfrac{n}{g}}n}_i^n`},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				o, err := New(tc.source, DefaultOptions())
				if err != nil {
					b.Fatal(err)
				}
				rbBenchmarkSink = o
			}
		})
	}
}
