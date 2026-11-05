package mathtex

import (
	"bytes"
	"encoding/json"
	"golang.org/x/image/font/gofont/goregular"
	"gomath"
	"gomath/geom"
	"gomath/render"
	"gomath/text"
	"math"
	"strings"
	"sync"
	"testing"
)

func braceLayout(t *testing.T, source string) box {
	t.Helper()
	opt := DefaultOptions()
	opt.Size = 1
	b, err := layout(source, opt)
	if err != nil {
		t.Fatal(source, err)
	}
	if b.object.Bounds().Valid {
		checkAccentBox(t, b)
	} else if err := b.object.Validate(); err != nil {
		t.Fatal(err)
	}
	return b
}

func TestBraceActualOperandGeometry(t *testing.T) {
	for _, cmd := range []string{"overbrace", "underbrace"} {
		for _, body := range []string{`i`, `abcdef`, `\mathit{f}`, `x_i^{n^2}`, `\frac{n}{g}`, `\sqrt[3]{g}`, `\begin{matrix}a&b\\g&n\end{matrix}`, `\overbrace{n}^{k}`, `\underbrace{g}_{i}`} {
			t.Run(cmd+body, func(t *testing.T) {
				plain := braceLayout(t, body)
				b := braceLayout(t, `\`+cmd+`{`+body+`}`)
				operand := named(b.object, "BraceBody").Bounds()
				ink := plain.object.Bounds()
				mark := named(b.object, "BraceMark "+cmd)
				near(t, operand.Min.Y, ink.Min.Y)
				near(t, operand.Max.Y, ink.Max.Y)
				near(t, operand.Size().X, ink.Size().X)
				mb := mark.Bounds()
				near(t, mb.Size().X, math.Max(plain.width, .6))
				near(t, mb.Size().Y, .28)
				near(t, mb.Center().X, b.width/2)
				if cmd == "overbrace" {
					near(t, mb.Min.Y, plain.height+.1)
					near(t, b.depth, plain.depth)
				} else {
					near(t, mb.Max.Y, -plain.depth-.1)
					near(t, b.height, plain.height)
				}
				if len(mark.Path.Contours) != 1 || !mark.Path.Contours[0].Closed || len(mark.Path.Contours[0].Curves) != 8 || mark.Style.Fill != gomath.White || mark.Style.StrokeWidth != 0 {
					t.Fatal("brace is not actual closed filled cubic geometry")
				}
				c := mark.Path.Contours[0].Curves[1]
				if c.P1.Y == c.P3.Y {
					t.Fatal("brace was flattened to a placeholder rule")
				}
			})
		}
	}
}

func TestBraceLabelsScopeAndNesting(t *testing.T) {
	for _, cmd := range []string{"overbrace", "underbrace"} {
		a := braceLayout(t, `\`+cmd+`{x_i^2}^{nnnn}_{g}`)
		b := braceLayout(t, `\`+cmd+`{x_i^2}_{g}^{nnnn}`)
		near(t, a.width, b.width)
		near(t, a.height, b.height)
		near(t, a.depth, b.depth)
		center := named(a.object, "BraceMark "+cmd).Bounds().Center().X
		up, down := named(a.object, "BraceUpperLabel").Bounds(), named(a.object, "BraceLowerLabel").Bounds()
		near(t, up.Center().X, center)
		near(t, down.Center().X, center)
		if up.Min.Y <= named(a.object, "HorizontalBrace "+cmd).Bounds().Max.Y || down.Max.Y >= named(a.object, "HorizontalBrace "+cmd).Bounds().Min.Y {
			t.Fatal("labels overlap brace body")
		}
		full := braceLayout(t, `\`+cmd+`{x}`)
		script := braceLayout(t, `z^{\`+cmd+`{x}^{n_i}}`)
		near(t, named(script.object, "BraceMark "+cmd).Bounds().Size().X, named(full.object, "BraceMark "+cmd).Bounds().Size().X*.65)
		for _, s := range []string{`\` + cmd + ` x^n`, `\` + cmd + `{x}^n`, `\displaystyle\` + cmd + `{x}^n`, `\textstyle\` + cmd + `{x}^n`} {
			got := braceLayout(t, s)
			near(t, got.width, braceLayout(t, `\`+cmd+`{x}^n`).width)
			if named(got.object, "BraceUpperLabel") == nil {
				t.Fatal("missing stacked label", s)
			}
		}
		grouped := braceLayout(t, `{\`+cmd+`{x}}^n`)
		if named(grouped.object, "BraceUpperLabel") != nil {
			t.Fatal("group incorrectly retains brace eligibility")
		}
		if named(grouped.object, "n").Bounds().Min.X <= named(grouped.object, "BraceMark "+cmd).Bounds().Max.X {
			t.Fatal("ordinary grouped script is not at side")
		}
	}
	for _, s := range []string{`\overbrace{\underbrace{x_i^2}_{g}}^{n_{\overbrace{x}^{k}}}`, `\frac{\underbrace{n}_{k^2}}{\overbrace{g}^{i_j}}`, `\left(\overbrace{x+y}^{n}\right)^k`, `\begin{array}{cc}\overbrace{x}^{n}&\underbrace{y}_{g}\end{array}`} {
		braceLayout(t, s)
	}
}

func TestBraceZeroInkAndPhantomExtents(t *testing.T) {
	for _, cmd := range []string{"overbrace", "underbrace"} {
		for _, operand := range []string{`{}`, `\quad`, `\text{ }`, `\phantom{g}`, `\hphantom{W}`, `\vphantom{\frac{n}{g}}`} {
			plain := braceLayout(t, operand)
			b := braceLayout(t, `\`+cmd+`{`+operand+`}^{\phantom{W}}_{\quad}`)
			body := named(b.object, "BraceBody")
			if body.Bounds().Valid {
				t.Fatal("phantom leaked ink", operand)
			}
			mark := named(b.object, "BraceMark "+cmd).Bounds()
			near(t, mark.Size().X, math.Max(.6, plain.width))
			if cmd == "overbrace" {
				near(t, mark.Min.Y, plain.height+.1)
			} else {
				near(t, mark.Max.Y, -plain.depth-.1)
			}
		}
	}
}

func TestBraceFontsColorsAndFiniteSize(t *testing.T) {
	for _, style := range []text.FontStyle{text.Regular, text.Bold, text.Italic, text.BoldItalic} {
		opt := DefaultOptions()
		opt.Size, opt.Style, opt.Color = 1, style, gomath.Blue
		b, err := layout(`\overbrace{x}^{n}_{g}`, opt)
		if err != nil {
			t.Fatal(err)
		}
		checkAccentBox(t, b)
		sameGlyphShape(t, named(b.object, "x").Copy().ApplyMatrix(geom.Scaling(.55, .55, .55)), "x", style)
		b.object.Walk(func(n *gomath.Object) {
			if len(n.Path.Contours) > 0 && n.Style.Fill != gomath.Blue {
				t.Error("context color lost")
			}
		})
		thickness := .045
		if style&text.Bold != 0 {
			thickness = .060
		}
		c := named(b.object, "BraceMark overbrace").Path.Contours[0].Curves
		near(t, c[1].P3.Y-c[5].P3.Y, thickness)
	}
	b := braceLayout(t, `\color{green}\overbrace{\color{red}\mathbf{x}}^{\textcolor{blue}{\mathit{n}}}y`)
	sameGlyphShape(t, named(b.object, "x").Copy().ApplyMatrix(geom.Scaling(.55, .55, .55)), "x", text.Bold)
	sameGlyphShape(t, named(b.object, "n").Copy().ApplyMatrix(geom.Scaling(.55/.65, .55/.65, .55/.65)), "n", text.Italic)
	if named(b.object, "BraceMark overbrace").Style.Fill != mathColors["green"] || named(b.object, "y").Style.Fill != mathColors["green"] {
		t.Fatal("operand scope escaped")
	}
	f, err := text.Parse(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	opt := DefaultOptions()
	opt.Font = f
	if _, err := New(`\overbrace{x}^{n}`, opt); err != nil {
		t.Fatal(err)
	}
	if o, err := New(`\mathbf{\overbrace{x}^{n}}`, opt); err == nil || o != nil {
		t.Fatal("missing custom bold variant accepted")
	}
	for _, size := range []float64{.001, 1, 1e6} {
		opt := DefaultOptions()
		opt.Size = size
		b, err := layout(`\overbrace{x}^{n_i}`, opt)
		if err != nil {
			t.Fatal(err)
		}
		checkAccentBox(t, b)
	}
	opt = DefaultOptions()
	opt.Size = math.SmallestNonzeroFloat64
	if o, err := New(`\overbrace{}`, opt); err == nil || o != nil {
		t.Fatal("underflow accepted")
	}
}

func TestBraceInvalidAtomicAndSharedBudgets(t *testing.T) {
	for _, s := range []string{`\overbrace`, `\overbrace{`, `\underbrace}`, `\overbrace{x}^`, `\underbrace{x}_`, `\overbrace{x}^{n}^{k}`, `\underbrace{x}_g_n`, `\overbrace{\unknown{x}}`, `\overbrace{x}^{\input{f}}`, `\underbrace{x}\limits_n`, `\overbrace{x}\nolimits^n`, `\overbrace{x}\displaylimits^n`, `\overbracket{x}`, `\overbrace{x}^\color{red}n`} {
		if o, err := New(s, DefaultOptions()); err == nil || o != nil {
			t.Fatal("invalid accepted", s, err)
		}
	}
	for _, s := range []string{strings.Repeat(`\overbrace `, MaxNesting) + `x`, strings.Repeat(`\underbrace{`, MaxNesting) + `x` + strings.Repeat(`}`, MaxNesting), `\overbrace{x}^{` + strings.Repeat(`{`, MaxNesting) + `n` + strings.Repeat(`}`, MaxNesting) + `}`} {
		if o, err := New(s, DefaultOptions()); err == nil || o != nil {
			t.Fatal("nesting bypass")
		}
	}
	for _, over := range []int{0, 1} {
		s := `\overbrace{x}^{n}`
		s += strings.Repeat(" ", MaxSourceRunes-len(s)+over)
		if o, err := New(s, DefaultOptions()); (err != nil) != (over == 1) || err != nil && o != nil {
			t.Fatal("source limit", over, err)
		}
	}
	// The same counters account for legacy fraction/radical rules, nested
	// braces, labels and content removed by phantom. No hidden refunds.
	for _, source := range []string{`\overbrace{\frac{n}{g}}^{\sqrt{x}}_{i}`, `\phantom{\underbrace{\overbrace{x}^{n}}_{g}}`, `\overbrace{\phantom{\frac{n}{g}}}^{\phantom{\underbrace{x}_{i}}}`} {
		base := parser{source: []rune(source), options: DefaultOptions()}
		_, _, err := base.row(topRow, base.options.Size)
		if err != nil {
			t.Fatal(err)
		}
		if base.curves < 8 {
			t.Fatal("mark not charged")
		}
		for _, over := range []int{0, 1} {
			p := parser{source: []rune(source), options: DefaultOptions(), curves: MaxLayoutCurves - base.curves + over}
			_, _, err := p.row(topRow, p.options.Size)
			if (err != nil) != (over == 1) || over == 0 && p.curves != MaxLayoutCurves {
				t.Fatal("shared curve limit", source, over, p.curves, err)
			}
		}
	}
	source := `\overbrace{\begin{array}{c}\phantom{x}\end{array}}^{\substack{n\\g}}`
	for _, over := range []int{0, 1} {
		p := parser{source: []rune(source), options: DefaultOptions(), cells: MaxEnvironmentCells - 3 + over}
		_, _, err := p.row(topRow, p.options.Size)
		if (err != nil) != (over == 1) || over == 0 && p.cells != MaxEnvironmentCells {
			t.Fatal("shared cell limit", p.cells, err)
		}
	}
	input := []string{`\overbrace{x}^{n}`, `\underbrace{g}_i`, `\unknown{x}`}
	saved, _ := json.Marshal(input)
	if p, m, err := LayoutParts(input, DefaultOptions()); err == nil || p != nil || m != (PartsMetrics{}) {
		t.Fatal("partial result escaped")
	}
	after, _ := json.Marshal(input)
	if string(saved) != string(after) {
		t.Fatal("caller sources mutated")
	}
}

func TestBraceConcurrentLayoutsAndSnapshots(t *testing.T) {
	const source = `\color{green}\overbrace{\frac{\mathbf{x_i}}{\textcolor{red}{g}}}^{n^2}_{\underbrace{g}_{i}}`
	b := braceLayout(t, source)
	before := b.object.Copy()
	scene := gomath.NewScene()
	scene.Add(b.object)
	target := b.object.Copy().Shift(geom.V(.2, .3))
	if err := scene.Play(1, gomath.Transform(b.object, target).WithRate(gomath.Linear)); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 6 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 4 {
				opt := DefaultOptions()
				opt.Size = 1
				o, err := New(source, opt)
				if err != nil {
					t.Error(err)
					return
				}
				if o.Bounds() != before.Bounds() {
					t.Error("concurrent geometry changed")
				}
				f, err := scene.Sample(.35)
				if err != nil {
					t.Error(err)
					return
				}
				got := f.Find(b.object.ID)
				if got == nil {
					t.Error("snapshot identity lost")
					return
				}
				mark := named(got, "BraceMark overbrace")
				mark.Path.Contours[0].Curves[0].P1.X += 100
				got.Shift(geom.V(10, 20))
				o.SetColor(gomath.Black)
			}
		}()
	}
	wg.Wait()
	again, err := scene.Sample(.35)
	if err != nil {
		t.Fatal(err)
	}
	if named(again.Find(b.object.ID), "BraceMark overbrace").Bounds().Size().X > 10 {
		t.Fatal("snapshot path alias")
	}
	requireSameDrawablePaths(t, before.Copy().Shift(geom.V(.2, .3)), b.object)
}

func TestBraceHiddenCurvesChargedExactlyOnce(t *testing.T) {
	for _, s := range []string{`\overbrace{\frac{n}{g}}^{\underbrace{x}_{i}}`, `\overbrace{\phantom{\frac{n}{g}}}^{\phantom{\underbrace{x}_{i}}}`} {
		p := parser{source: []rune(s), options: DefaultOptions()}
		if _, _, err := p.row(topRow, p.options.Size); err != nil {
			t.Fatal(err)
		}
		visible := braceLayout(t, `\overbrace{\frac{n}{g}}^{\underbrace{x}_{i}}`)
		if p.curves != countAccentCurves(visible.object) {
			t.Fatal("hidden content was refunded or audited twice", p.curves, countAccentCurves(visible.object))
		}
	}
	if _, err := New(strings.Repeat(`\overbrace `, MaxNesting-1)+"x", DefaultOptions()); err != nil {
		t.Fatal("valid unbraced nesting boundary", err)
	}
}

func TestBraceWriteIntermediateFrames(t *testing.T) {
	from := braceLayout(t, `\overbrace{x+y}^{n}_{g}`).object
	from.MoveTo(geom.V(0, 0))
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
	var previous []byte
	for _, time := range []float64{0, .35, .7, 1} {
		frame, err := scene.Sample(time)
		if err != nil {
			t.Fatal(err)
		}
		got := frame.Find(from.ID)
		if got == nil || named(got, "BraceMark overbrace") == nil {
			t.Fatal("Write lost brace paths/identity")
		}
		got.Walk(func(n *gomath.Object) {
			if len(n.Path.Contours) > 0 {
				near(t, n.Style.Reveal, time)
			}
		})
		im, err := r.Render(frame)
		if err != nil {
			t.Fatal(err)
		}
		if previous != nil && bytes.Equal(previous, im.Pix) {
			t.Fatal("Write failed to change intermediate pixels", time)
		}
		previous = append(previous[:0], im.Pix...)
	}
}

func BenchmarkBraceLayout(b *testing.B) {
	for _, source := range []struct{ name, tex string }{{"short", `\overbrace{x+y}^{n}`}, {"nested", `\overbrace{\frac{x_i^2}{\underbrace{\sqrt[3]{g}}_{k}}}^{\substack{n=1\\m}}_{\phantom{W}}`}} {
		b.Run(source.name, func(b *testing.B) {
			opt := DefaultOptions()
			b.ReportAllocs()
			for b.Loop() {
				if _, err := New(source.tex, opt); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
