package mathtex

import (
	"golang.org/x/image/font/gofont/goregular"
	"gomath"
	"gomath/geom"
	"gomath/text"
	"math"
	"strings"
	"sync"
	"testing"
)

func mustLayout(t *testing.T, source string) box {
	t.Helper()
	b, err := layout(source, DefaultOptions())
	if err != nil {
		t.Fatal(source, err)
	}
	return b
}
func near(t *testing.T, a, b float64) {
	t.Helper()
	if !finite(a) || math.Abs(a-b) > 1e-8 {
		t.Fatalf("got %.12g want %.12g", a, b)
	}
}
func named(o *gomath.Object, name string) *gomath.Object {
	var found *gomath.Object
	o.Walk(func(n *gomath.Object) {
		if n.Name == name && (found == nil || len(n.Path.Contours) > 0) {
			found = n
		}
	})
	return found
}
func TestScalableDelimitersEncloseNestedInk(t *testing.T) {
	for _, pair := range [][2]string{{"(", ")"}, {"[", "]"}, {`\{`, `\}`}, {`\langle`, `\rangle`}, {"|", "|"}, {`\|`, `\|`}, {`\lfloor`, `\rfloor`}, {`\lceil`, `\rceil`}, {"/", `\backslash`}, {"(", "]"}} {
		source := `\left` + pair[0] + `\frac{1+x^{\frac{2}{y}}}{\sqrt{1+\frac{a}{b}}}\right` + pair[1]
		b := mustLayout(t, source)
		d := b.object.Children[0]
		if len(d.Children) != 3 {
			t.Fatal("missing delimiter layout")
		}
		inner := d.Children[1].Bounds()
		for _, edge := range []*gomath.Object{d.Children[0], d.Children[2]} {
			bounds := edge.Bounds()
			if !bounds.Valid || bounds.Max.Y <= inner.Max.Y || bounds.Min.Y >= inner.Min.Y {
				t.Fatal("delimiter clips tall inner ink", source, bounds, inner)
			}
		}
	}
	plain := mustLayout(t, `x`)
	invisible := mustLayout(t, `\left.x\right.`)
	near(t, plain.width, invisible.width)
	if invisible.object.Children[0].Children[0].Bounds().Valid || invisible.object.Children[0].Children[2].Bounds().Valid {
		t.Fatal("invisible delimiter has ink")
	}
	oneSide := mustLayout(t, `\left.\frac{a}{b}\right|_{x=0}^{x=1}`)
	if oneSide.object.Children[0].Children[0].Bounds().Valid || named(oneSide.object, "Delimiter |") == nil {
		t.Fatal("invisible side changed")
	}
	small := mustLayout(t, `\left(x\right)`)
	large := mustLayout(t, `\left(\frac{a}{\frac{b}{c}}\right)`)
	if large.height+large.depth <= small.height+small.depth {
		t.Fatal("delimiters did not scale")
	}
	mustLayout(t, `\left[\frac{1}{\left(1+x\right)^2}\right]_{i=1}`)
}

func TestEnvironmentBaselinesColumnsAndNestedBoxes(t *testing.T) {
	source := `\begin{matrix}\frac{1}{x}&a\\y&\frac{b}{c}\end{matrix}`
	b := mustLayout(t, source)
	entries := []*gomath.Object{}
	for _, name := range []string{"matrix cell[0,0]", "matrix cell[0,1]", "matrix cell[1,0]", "matrix cell[1,1]"} {
		cell := named(b.object, name)
		if cell == nil {
			t.Fatal("missing cell", name)
		}
		entries = append(entries, cell)
	}
	inputs := []string{`\frac{1}{x}`, "a", "y", `\frac{b}{c}`}
	baselines := make([]float64, 4)
	for i, cell := range entries {
		standalone := mustLayout(t, inputs[i])
		baselines[i] = cell.Bounds().Min.Y - standalone.object.Bounds().Min.Y
	}
	near(t, baselines[0], baselines[1])
	near(t, baselines[2], baselines[3])
	if entries[0].Bounds().Max.X >= entries[1].Bounds().Min.X || entries[2].Bounds().Max.X >= entries[3].Bounds().Min.X {
		t.Fatal("environment columns overlap")
	}
	row0Bottom := math.Min(entries[0].Bounds().Min.Y, entries[1].Bounds().Min.Y)
	row1Top := math.Max(entries[2].Bounds().Max.Y, entries[3].Bounds().Max.Y)
	if row0Bottom <= row1Top {
		t.Fatal("environment rows overlap")
	}
	for _, env := range []string{"matrix", "pmatrix", "bmatrix", "Bmatrix", "vmatrix", "Vmatrix", "cases"} {
		source := `\begin{` + env + `}\frac{1}{x}&\left(\begin{pmatrix}a&b\\c&d\end{pmatrix}\right)^{2}\\\sqrt{y}&\begin{cases}0&\text{ if }x<0\\1&\text{ otherwise}\end{cases}\end{` + env + `}`
		nested := mustLayout(t, source)
		if nested.height+nested.depth <= b.height+b.depth {
			t.Fatal("nested environment extents lost", env)
		}
	}
	cases := mustLayout(t, `\begin{cases}a&x<0\\aaaa&x>0\end{cases}`)
	a, aaaa := named(cases.object, "cases cell[0,0]"), named(cases.object, "cases cell[1,0]")
	near(t, a.Bounds().Min.X, aaaa.Bounds().Min.X)
	if named(cases.object, "Delimiter {") == nil || named(cases.object, "Delimiter .") == nil {
		t.Fatal("cases delimiters incorrect")
	}
	blank := mustLayout(t, `\begin{bmatrix}&a\\b&\end{bmatrix}`)
	if blank.width <= 0 || named(blank.object, "bmatrix cell[0,0]") == nil || named(blank.object, "bmatrix cell[0,0]").Bounds().Valid {
		t.Fatal("blank cell unsupported")
	}
	noTrailing := mustLayout(t, `\begin{matrix}a&b\\c&d\end{matrix}`)
	trailing := mustLayout(t, `\begin{matrix}a&b\\c&d\\ \end{matrix}`)
	near(t, noTrailing.height, trailing.height)
	near(t, noTrailing.depth, trailing.depth)
	near(t, noTrailing.width, trailing.width)
}

func TestLiteralTextPreservesAllSpaces(t *testing.T) {
	opt := text.DefaultOptions()
	opt.Size = DefaultOptions().Size
	for _, source := range []string{"a  b  ", "  a ", "   ", "a\nb "} {
		b := mustLayout(t, `\text{`+source+`}`)
		_, metrics, err := text.Layout(source, text.DefaultFont(), opt)
		if err != nil {
			t.Fatal(err)
		}
		near(t, b.width, metrics.Width)
	}
	literal := mustLayout(t, `\text{a{ b} \{c\} \&  }`)
	expected := mustLayout(t, `\text{a b \{c\} \&  }`)
	near(t, literal.width, expected.width)
	padded := mustLayout(t, `\text{a   }x`)
	compact := mustLayout(t, `\text{a}x`)
	_, space, err := text.Layout(" ", text.DefaultFont(), opt)
	if err != nil {
		t.Fatal(err)
	}
	near(t, named(padded.object, "x").Bounds().Min.X-named(compact.object, "x").Bounds().Min.X, 3*space.Width)
	// Operators, alignment and script characters inside text are literal glyphs.
	mustLayout(t, `\text{a & b_2^3 # $ %}`)
}

func sameGlyphShape(t *testing.T, actual *gomath.Object, letter string, style text.FontStyle) {
	t.Helper()
	font, err := text.DefaultFontForStyle(style)
	if err != nil {
		t.Fatal(err)
	}
	opt := text.DefaultOptions()
	opt.Size = DefaultOptions().Size
	expected, err := text.New(letter, font, opt)
	if err != nil {
		t.Fatal(err)
	}
	want := expected.Children[0]
	if actual == nil {
		t.Fatal("glyph missing", letter)
	}
	ap, wp := actual.Path.Transform(geom.Translation(actual.Center().Mul(-1))), want.Path.Transform(geom.Translation(want.Center().Mul(-1)))
	if len(ap.Contours) != len(wp.Contours) {
		t.Fatal("wrong font contours", letter, style)
	}
	for i, c := range ap.Contours {
		wc := wp.Contours[i]
		if len(c.Curves) != len(wc.Curves) {
			t.Fatal("wrong font curves", letter, style)
		}
		for j, v := range c.Curves {
			w := wc.Curves[j]
			for _, pair := range [][2]geom.Vec3{{v.P0, w.P0}, {v.P1, w.P1}, {v.P2, w.P2}, {v.P3, w.P3}} {
				near(t, pair[0].X, pair[1].X)
				near(t, pair[0].Y, pair[1].Y)
			}
		}
	}
}
func TestFontCommandsSelectActualGlyphsAndScope(t *testing.T) {
	for _, test := range []struct {
		source string
		style  text.FontStyle
	}{
		{`A`, text.Regular}, {`\mathbf{A}`, text.Bold}, {`\mathit{A}`, text.Italic}, {`\mathbf{\mathit{A}}`, text.BoldItalic},
		{`\boldsymbol{A}`, text.Bold}, {`\mathit{\mathrm{A}}`, text.Regular}, {`\textbf{A}`, text.Bold}, {`\textit{A}`, text.Italic},
	} {
		b := mustLayout(t, test.source)
		sameGlyphShape(t, named(b.object, "A"), "A", test.style)
	}
	b := mustLayout(t, `\mathbf{\mathit{A}B}C`)
	sameGlyphShape(t, named(b.object, "A"), "A", text.BoldItalic)
	sameGlyphShape(t, named(b.object, "B"), "B", text.Bold)
	sameGlyphShape(t, named(b.object, "C"), "C", text.Regular)
	opt := DefaultOptions()
	opt.Style = text.BoldItalic
	object, err := New("A", opt)
	if err != nil {
		t.Fatal(err)
	}
	sameGlyphShape(t, named(object, "A"), "A", text.BoldItalic)
	custom, err := text.Parse(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	opt = DefaultOptions()
	opt.Font = custom
	if _, err := New(`\mathbf{A}`, opt); err == nil {
		t.Fatal("custom font silently substituted for bold")
	}
	opt.BoldFont, _ = text.DefaultFontForStyle(text.Bold)
	object, err = New(`\mathbf{A}`, opt)
	if err != nil {
		t.Fatal(err)
	}
	sameGlyphShape(t, named(object, "A"), "A", text.Bold)
}

func TestMalformedStructuresAndUnsupportedSyntax(t *testing.T) {
	for _, source := range []string{
		`\left(x`, `x\right)`, `\left(x\right`, `\left x\right)`, `\left(x}`, `\left(\frac{x}{y}\end{matrix}`,
		`\right.`, `\left\unknown x\right)`, `\left( x \right\unknown`,
		`a&b`, `a\\b`, `\begin{matrix}a&b\\c\end{matrix}`, `\begin{matrix}a\end{pmatrix}`,
		`\begin{matrix}a`, `\end{matrix}`, `\begin{matrix}\end{matrix}`, `\begin{matrix}\\a\end{matrix}`,
		`\begin{matrix}a\\\\b\end{matrix}`, `\begin{cases}a\end{cases}`, `\begin{cases}a&b&c\end{cases}`,
		`\begin{bmatrix}a&\begin{pmatrix}x&y\end{bmatrix}\end{pmatrix}`, `\begin{matrix}a&{b&c}\end{matrix}`,
		`\begin{matrix}a\\[1em]b\end{matrix}`, `\begin{array}x\end{array}`, `\begin{matrix*}x\end{matrix*}`,
		`\text{x`, `\text{\unknown{x}}`, `\text{x\}`, `\mathbf x`, `\mathit`, `x ^`, `x ^ 2 ^ 3`, `\sqrt[3{x}`,
	} {
		if _, err := New(source, DefaultOptions()); err == nil {
			t.Errorf("accepted malformed or unsupported %q", source)
		}
	}
	// Whitespace between a base and scripts is accepted, including both scripts.
	mustLayout(t, `x ^ {2} _ {i}`)
}
func TestTypesettingResourceAndOptionLimits(t *testing.T) {
	for _, bad := range []float64{0, -1, math.NaN(), math.Inf(1), math.Inf(-1), 1e7} {
		opt := DefaultOptions()
		opt.Size = bad
		if _, err := New("", opt); err == nil {
			t.Fatal("bad size accepted", bad)
		}
	}
	opt := DefaultOptions()
	opt.Color = gomath.RGB(math.NaN(), 0, 0)
	if _, err := New("", opt); err == nil {
		t.Fatal("bad color accepted")
	}
	opt = DefaultOptions()
	opt.Style = 99
	if _, err := New("", opt); err == nil {
		t.Fatal("bad style accepted")
	}
	opt = DefaultOptions()
	opt.ItalicFont = &text.Font{}
	if _, err := New("", opt); err == nil {
		t.Fatal("uninitialized font variant accepted")
	}
	for _, source := range []string{
		string([]byte{0xff}), strings.Repeat(" ", MaxSourceRunes+1),
		strings.Repeat(`\left(`, MaxNesting) + "x" + strings.Repeat(`\right)`, MaxNesting),
		strings.Repeat(`\begin{matrix}`, MaxNesting) + "x" + strings.Repeat(`\end{matrix}`, MaxNesting),
		`\text{` + strings.Repeat("{", MaxNesting) + "x" + strings.Repeat("}", MaxNesting) + "}",
		`\begin{matrix}` + strings.Repeat("x&", MaxEnvironmentColumns) + `x\end{matrix}`,
		`\begin{matrix}` + strings.Repeat(`x\\`, MaxEnvironmentRows) + `x\end{matrix}`,
	} {
		if _, err := New(source, DefaultOptions()); err == nil {
			t.Fatal("resource limit ignored")
		}
	}
	// A maximal blank grid stays small while exercising the global nested-cell
	// bound independently of the source-length and per-axis bounds.
	row := strings.Repeat("&", MaxEnvironmentColumns-1)
	grid := `\begin{matrix}` + strings.Repeat(row+`\\`, MaxEnvironmentRows-1) + row + `\end{matrix}`
	if _, err := New(grid, DefaultOptions()); err != nil {
		t.Fatal("maximum environment shape rejected", err)
	}
	if _, err := New(`\begin{matrix}`+grid+`\end{matrix}`, DefaultOptions()); err == nil || !strings.Contains(err.Error(), "4096 environment cells") {
		t.Fatal("nested cell budget not enforced", err)
	}
	opt = DefaultOptions()
	opt.Size = math.SmallestNonzeroFloat64
	if _, err := New("x", opt); err == nil {
		t.Fatal("underflowed glyph coordinates accepted")
	}
}
func TestConcurrentMathLayoutUsesIndependentGeometry(t *testing.T) {
	const source = `\left[\begin{pmatrix}\mathbf{x}&\frac{1}{y}\\\sqrt{z}&\mathit{a}\end{pmatrix}\right]^{2}`
	want := mustLayout(t, source).object.Bounds()
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 8 {
				object, err := New(source, DefaultOptions())
				if err != nil {
					t.Error(err)
					return
				}
				if object.Bounds() != want {
					t.Error("concurrent layout differs")
				}
				object.Shift(geom.V(100, 100))
			}
		}()
	}
	wg.Wait()
}

func BenchmarkNestedTypesetting(b *testing.B) {
	const source = `\left[\begin{pmatrix}\mathbf{x}_{i}&\frac{1}{1+\frac{a}{b}}\\\sqrt{z}&\mathit{a}^{2}\end{pmatrix}\right]^{n}`
	_, _ = New(source, DefaultOptions())
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := New(source, DefaultOptions()); err != nil {
			b.Fatal(err)
		}
	}
}
