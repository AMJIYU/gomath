package mathtex

import (
	"gomath"
	"gomath/geom"
	"gomath/text"
	"math"
	"strings"
	"sync"
	"testing"
)

var accentCommands = []string{"overline", "underline", "hat", "widehat", "bar", "vec", "dot", "ddot"}

func checkAccentBox(t *testing.T, b box) {
	t.Helper()
	ink := b.object.Bounds()
	if !ink.Valid || ink.Min.X < -1e-10 || ink.Max.X > b.width+1e-10 || ink.Max.Y > b.height+1e-10 || ink.Min.Y < -b.depth-1e-10 {
		t.Fatal("ink escapes layout box", ink, b.width, b.height, b.depth)
	}
	if err := b.object.Validate(); err != nil {
		t.Fatal(err)
	}
}

func countAccentCurves(o *gomath.Object) int {
	total := 0
	o.Walk(func(node *gomath.Object) {
		for _, contour := range node.Path.Contours {
			total += len(contour.Curves)
		}
	})
	return total
}

func TestAccentGeometryBaselineAndFilledBounds(t *testing.T) {
	for _, command := range accentCommands {
		for _, source := range []string{`x`, `g`, `\mathit{f}`, `\frac{x_i}{y^2}`, `\begin{matrix}a&b\\c&d\end{matrix}`} {
			t.Run(command+"/"+source, func(t *testing.T) {
				plain := mustLayout(t, source)
				b := mustLayout(t, `\`+command+`{`+source+`}`)
				checkAccentBox(t, b)
				a := b.object.Children[0]
				if a.Name != "Accent "+command || len(a.Children) != 2 {
					t.Fatal("missing base/mark structure")
				}
				base, mark := a.Children[0].Bounds(), a.Children[1].Bounds()
				before := plain.object.Bounds()
				near(t, base.Min.Y, before.Min.Y)
				near(t, base.Max.Y, before.Max.Y)
				near(t, base.Size().X, before.Size().X)
				if command == "underline" {
					if mark.Max.Y >= -plain.depth || b.depth <= plain.depth {
						t.Fatal("underline overlaps base or loses descent", base, mark)
					}
					near(t, b.height, plain.height)
				} else {
					if mark.Min.Y <= plain.height || b.height <= plain.height {
						t.Fatal("accent overlaps base or loses ascent", base, mark)
					}
					near(t, b.depth, plain.depth)
				}
				a.Children[1].Walk(func(node *gomath.Object) {
					if len(node.Path.Contours) == 0 {
						return
					}
					if node.Style.Fill != DefaultOptions().Color || node.Style.StrokeWidth != 0 || node.Style.Stroke.A != 0 {
						t.Fatal("mark must use actual filled paths with full geometric bounds")
					}
					for _, c := range node.Path.Contours {
						if !c.Closed || len(c.Curves) == 0 {
							t.Fatal("mark is an empty/open path")
						}
					}
				})
			})
		}
	}
}

func TestAccentWidthsCentersAndRuleThickness(t *testing.T) {
	size := DefaultOptions().Size
	for _, command := range accentCommands {
		small := mustLayout(t, `\`+command+`{i}`)
		wide := mustLayout(t, `\`+command+`{abcdef}`)
		checkAccentBox(t, small)
		checkAccentBox(t, wide)
		for _, b := range []box{small, wide} {
			a := b.object.Children[0]
			base, mark := a.Children[0].Bounds(), a.Children[1].Bounds()
			if command != "overline" && command != "underline" && command != "widehat" {
				near(t, mark.Center().X, base.Center().X)
			}
			if command == "overline" || command == "underline" {
				near(t, mark.Min.X, 0)
				near(t, mark.Max.X, b.width)
				near(t, mark.Size().Y, size*.045)
			}
		}
		sw := small.object.Children[0].Children[1].Bounds().Size().X
		ww := wide.object.Children[0].Children[1].Bounds().Size().X
		if command == "overline" || command == "underline" || command == "widehat" {
			if ww <= sw {
				t.Fatal("wide mark failed to follow advance width", command, sw, ww)
			}
		} else {
			near(t, sw, ww)
		}
	}
	dot := mustLayout(t, `\dot{x}`).object.Children[0].Children[1]
	ddot := mustLayout(t, `\ddot{x}`).object.Children[0].Children[1]
	if len(ddot.Children) != 2 || countAccentCurves(ddot) != 2*countAccentCurves(dot) {
		t.Fatal("ddot must contain two actual circular paths")
	}
	a, b := ddot.Children[0].Bounds(), ddot.Children[1].Bounds()
	near(t, a.Center().Y, b.Center().Y)
	near(t, a.Size().X, b.Size().X)
	if a.Max.X >= b.Min.X {
		t.Fatal("double dots overlap")
	}
}

func TestAccentNestingScriptsAndComposition(t *testing.T) {
	for _, command := range accentCommands {
		bare := mustLayout(t, `\`+command+` x_i^2`)
		braced := mustLayout(t, `\`+command+`{x}_i^2`)
		near(t, bare.width, braced.width)
		near(t, bare.height, braced.height)
		near(t, bare.depth, braced.depth)
		a := bare.object.Children[0]
		if len(a.Children) != 4 {
			t.Fatal("scripts must attach to the accent atom", command)
		}
		mark := a.Children[1].Bounds()
		for _, script := range a.Children[2:] {
			if script.Bounds().Min.X <= mark.Max.X {
				t.Fatal("script overlaps accent width", command)
			}
		}
		inside := mustLayout(t, `\`+command+`{x_i^2}`)
		if len(inside.object.Children[0].Children) != 2 {
			t.Fatal("inside scripts escaped the accent argument")
		}
		checkAccentBox(t, inside)
		checkAccentBox(t, bare)
	}
	single := mustLayout(t, `\hat{x}`)
	double := mustLayout(t, `\hat{\hat{x}}`)
	if double.height <= single.height {
		t.Fatal("nested accent did not add clearance")
	}
	inner := double.object.Children[0].Children[0].Children[0]
	outerMark := double.object.Children[0].Children[1].Bounds()
	if outerMark.Min.Y <= inner.Bounds().Max.Y {
		t.Fatal("nested accent overlaps inner mark")
	}
	for _, source := range []string{
		`\hat\bar\vec x`, `\underline{\overline{\ddot{x}}}`, `\frac{\widehat{a+b}}{\underline{x_i}}`,
		`\sqrt{\overline{a}+\vec{x}}`, `\left(\widehat{x_i^2}\right)^{\dot{n}}`,
		`\begin{pmatrix}\vec{x}&\bar{y}\\\underline{a}&\ddot{b}\end{pmatrix}`,
	} {
		checkAccentBox(t, mustLayout(t, source))
	}
	// Unbraced chaining and explicit nesting have the same atom semantics.
	bare := mustLayout(t, `\hat\bar x`)
	braced := mustLayout(t, `\hat{\bar{x}}`)
	near(t, bare.width, braced.width)
	near(t, bare.height, braced.height)
	near(t, bare.depth, braced.depth)
}

func TestAccentStyleColorAndSize(t *testing.T) {
	for _, style := range []text.FontStyle{text.Regular, text.Bold, text.Italic, text.BoldItalic} {
		for _, command := range accentCommands {
			opt := DefaultOptions()
			opt.Style, opt.Color = style, gomath.Blue
			b, err := layout(`\`+command+`{x}`, opt)
			if err != nil {
				t.Fatal(err)
			}
			checkAccentBox(t, b)
			sameGlyphShape(t, named(b.object, "x"), "x", style)
			b.object.Walk(func(node *gomath.Object) {
				if len(node.Path.Contours) > 0 && node.Style.Fill != gomath.Blue {
					t.Error("color not propagated to glyph/mark")
				}
			})
		}
	}
	b := mustLayout(t, `\mathbf{\hat{x}}+\mathit{\bar{y}}+\hat{\mathrm{z}}`)
	sameGlyphShape(t, named(b.object, "x"), "x", text.Bold)
	sameGlyphShape(t, named(b.object, "y"), "y", text.Italic)
	sameGlyphShape(t, named(b.object, "z"), "z", text.Regular)
	for _, command := range accentCommands {
		for _, size := range []float64{.01, 1, 1e6} {
			opt := DefaultOptions()
			opt.Size = size
			b, err := layout(`\`+command+`{x}`, opt)
			if err != nil {
				t.Fatal(command, size, err)
			}
			checkAccentBox(t, b)
		}
	}
	regular := mustLayout(t, `\bar{x}`).object.Children[0].Children[1].Bounds()
	bold := mustLayout(t, `\mathbf{\bar{x}}`)
	if named(bold.object, "AccentMark bar").Bounds().Size().Y <= regular.Size().Y {
		t.Fatal("bold context did not thicken the vector rule")
	}
}

func TestAccentMalformedArgumentsAndInputBudgets(t *testing.T) {
	for _, command := range accentCommands {
		for _, suffix := range []string{"", "{", "{}", `{\quad}`, `{\text{ }}`, "}", "_i", `{\unknown{x}}`, `{x}^{1}^{2}`} {
			if object, err := New(`\`+command+suffix, DefaultOptions()); err == nil || object != nil {
				t.Fatal("invalid accent accepted", command, suffix, err)
			}
		}
	}
	for _, source := range []string{`\tilde{x}`, `\acute{x}`, `\hat{\input{file}}`, `\overline{\def\x{x}}`} {
		if object, err := New(source, DefaultOptions()); err == nil || object != nil {
			t.Fatal("unsupported command accepted", source, err)
		}
	}
	for _, source := range []string{
		strings.Repeat(`\hat `, MaxNesting) + "x",
		strings.Repeat(`\overline{`, MaxNesting) + "x" + strings.Repeat("}", MaxNesting),
		strings.Repeat(`\hat x`, MaxSourceRunes/6+1),
	} {
		if object, err := New(source, DefaultOptions()); err == nil || object != nil {
			t.Fatal("accent budget bypass", err)
		}
	}
	// The boundary applies to unbraced recursion too, not only group rows.
	if _, err := New(strings.Repeat(`\hat `, MaxNesting-1)+"x", DefaultOptions()); err != nil {
		t.Fatal("valid unbraced nesting boundary rejected", err)
	}
	opt := DefaultOptions()
	opt.Size = math.SmallestNonzeroFloat64
	if _, err := New(`\dot{x}`, opt); err == nil {
		t.Fatal("underflowed accent accepted")
	}
}

func TestAccentScriptScaleAndScopedMarkStyle(t *testing.T) {
	for _, command := range accentCommands {
		full := mustLayout(t, `\`+command+`{x}`)
		fullMark := named(full.object, "AccentMark "+command).Bounds().Size()
		for _, source := range []string{`x^{\` + command + `{x}}`, `x_{\` + command + `{x}}`} {
			script := mustLayout(t, source)
			mark := named(script.object, "AccentMark "+command).Bounds().Size()
			near(t, mark.X, fullMark.X*.65)
			near(t, mark.Y, fullMark.Y*.65)
			checkAccentBox(t, script)
		}
	}
	for _, tc := range []struct {
		source    string
		thickness float64
		style     text.FontStyle
	}{
		{`\bar{\mathbf{x}}`, .045, text.Bold},
		{`\mathbf{\bar{\mathrm{x}}}`, .060, text.Regular},
	} {
		b := mustLayout(t, tc.source)
		near(t, named(b.object, "AccentMark bar").Bounds().Size().Y, DefaultOptions().Size*tc.thickness)
		sameGlyphShape(t, named(b.object, "x"), "x", tc.style)
	}
}

func TestAccentSharesGlyphCurveBudget(t *testing.T) {
	for _, command := range accentCommands {
		b := mustLayout(t, `\`+command+`{x}`)
		total := countAccentCurves(b.object)
		for _, over := range []int{0, 1} {
			p := parser{source: []rune("x"), options: DefaultOptions(), depth: 1, curves: MaxLayoutCurves - total + over}
			_, err := p.accent(command, p.options.Size)
			if over == 0 && (err != nil || p.curves != MaxLayoutCurves) || over == 1 && (err == nil || !strings.Contains(err.Error(), "glyph/accent curves")) {
				t.Fatal("glyph and mark must charge a shared budget exactly once", command, p.curves, err)
			}
		}
	}
	p := parser{source: []rune(`\ddot{\bar{\hat{x}}}`), options: DefaultOptions()}
	b, _, err := p.row(topRow, p.options.Size)
	if err != nil || p.curves != countAccentCurves(b.object) {
		t.Fatal("nested accents double-counted or escaped curve budget", p.curves, err)
	}
}

func TestAccentConcurrentGeometryOwnership(t *testing.T) {
	const source = `\widehat{\mathbf{x}_i+\dot{y}}+\underline{\vec{z}}`
	want := mustLayout(t, source).object.Bounds()
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 4 {
				o, err := New(source, DefaultOptions())
				if err != nil {
					t.Error(err)
					return
				}
				if o.Bounds() != want {
					t.Error("concurrent accent geometry differs")
				}
				o.Shift(geom.V(100, 100))
			}
		}()
	}
	wg.Wait()
}

func BenchmarkAccentNestedTypesetting(b *testing.B) {
	const source = `\widehat{\mathbf{x}_{i}+\vec{y}}^{\dot{n}}+\underline{\frac{\ddot{a}}{\overline{b}}}`
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
