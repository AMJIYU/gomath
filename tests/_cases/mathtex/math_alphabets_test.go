package mathtex

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"gomath"
	"gomath/geom"
	"gomath/text"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestMathAlphabetCoverageUnicodeAndLetterlike(t *testing.T) {
	for _, ref := range alphabetReferences(t) {
		command := mustLayout(t, `\`+ref.Command+`{`+ref.Input+`}`)
		encoded := mustLayout(t, ref.Rune)
		equalLayoutMetrics(t, command, encoded)
		requireSameDrawablePaths(t, command.object, encoded.object)
	}
	// The Unicode blocks have holes: these must use their Letterlike codepoints.
	for _, test := range []struct{ command, inputs, want string }{
		{"mathbb", "CHNPQRZ", "ℂℍℕℙℚℝℤ"},
		{"mathcal", "BEFHILRMego", "ℬℰℱℋℐℒℛℳℯℊℴ"},
		{"mathfrak", "CHIRZ", "ℭℌℑℜℨ"},
	} {
		b := mustLayout(t, `\`+test.command+`{`+test.inputs+`}`)
		for _, r := range test.want {
			if n := named(b.object, string(r)); n == nil || len(n.Path.Contours) == 0 {
				t.Fatal("missing Unicode Letterlike exception", test.command, string(r))
			}
		}
	}
	for _, command := range []string{"mathbb", "mathcal", "mathfrak"} {
		letters := "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
		if command == "mathbb" {
			letters += "0123456789"
		}
		checkLimitsBox(t, mustLayout(t, `\`+command+`{`+letters+`}`))
	}
	// Distinct real font families, not regular Latin aliases or synthetic slants.
	var paths []any
	for _, source := range []string{"A", `\mathbb{A}`, `\mathcal{A}`, `\mathfrak{A}`} {
		b := mustLayout(t, source)
		var path any
		b.object.Walk(func(n *gomath.Object) {
			if len(n.Path.Contours) != 0 {
				path = n.Path
			}
		})
		for _, previous := range paths {
			if reflect.DeepEqual(previous, path) {
				t.Fatal("alphabet substituted a previous glyph", source)
			}
		}
		paths = append(paths, path)
	}
}

func TestMathAlphabetStylesScopeAndLiteralContexts(t *testing.T) {
	for _, command := range []string{"mathbb", "mathcal", "mathfrak"} {
		base := mustLayout(t, `\`+command+`{R}`)
		for _, enclosing := range []string{"mathbf", "mathit", "mathrm", "boldsymbol"} {
			b := mustLayout(t, `\`+enclosing+`{\`+command+`{R}}`)
			equalLayoutMetrics(t, b, base)
			requireSameDrawablePaths(t, b.object, base.object)
		}
		for _, inner := range []struct {
			command string
			style   text.FontStyle
		}{{"mathbf", text.Bold}, {"boldsymbol", text.Bold}, {"mathit", text.Italic}, {"mathrm", text.Regular}} {
			b := mustLayout(t, `\`+command+`{R\`+inner.command+`{A}R}x`)
			sameGlyphShape(t, named(b.object, "A"), "A", inner.style)
			sameGlyphShape(t, named(b.object, "x"), "x", text.Regular)
		}
		for _, nested := range []string{"mathbb", "mathcal", "mathfrak"} {
			source := `\` + command + `{R\` + nested + `{A}R}x`
			checkLimitsBox(t, mustLayout(t, source))
		}
		for _, outer := range []struct {
			command string
			style   text.FontStyle
		}{{"mathbf", text.Bold}, {"mathit", text.Italic}} {
			b := mustLayout(t, `\`+outer.command+`{\`+command+`{R}x}y`)
			sameGlyphShape(t, named(b.object, "x"), "x", outer.style)
			sameGlyphShape(t, named(b.object, "y"), "y", text.Regular)
		}
		// Literal text and named operators select their own existing upright font;
		// alphabet state resumes after the command's argument.
		b := mustLayout(t, `\`+command+`{R\text{A}\operatorname{rank}\sin\lim R}`)
		sameGlyphShape(t, named(b.object, "A"), "A", text.Regular)
		sameGlyphShape(t, named(b.object, "k"), "k", text.Regular)
		sameGlyphShape(t, named(b.object, "s"), "s", text.Regular)
		checkLimitsBox(t, b)
	}
	opt := DefaultOptions()
	opt.Color = gomath.RGB(.2, .7, .4)
	opt.Style = text.BoldItalic
	b, err := layout(`\mathbb{R}+\mathcal{A}+\mathfrak{g}`, opt)
	if err != nil {
		t.Fatal(err)
	}
	b.object.Walk(func(n *gomath.Object) {
		if len(n.Path.Contours) > 0 && (n.Style.Fill != opt.Color || n.Style.StrokeWidth != 0) {
			t.Error("alphabet glyph lost actual fill/color")
		}
	})
}

func TestMathAlphabetBaselineScriptsAndComposition(t *testing.T) {
	for _, source := range []string{`\mathbb{R}`, `\mathcal{g}`, `\mathfrak{g}`} {
		plain := mustLayout(t, source)
		followed := mustLayout(t, source+`x`)
		near(t, followed.object.Children[0].Bounds().Min.Y, plain.object.Bounds().Min.Y)
		near(t, followed.object.Children[1].Bounds().Min.X, plain.width+DefaultOptions().Size*.04+mustLayout(t, "x").object.Bounds().Min.X)
		for _, script := range []string{`x^{` + source + `}`, `x_{` + source + `}`} {
			b := mustLayout(t, script)
			n := b.object.Children[0].Children[len(b.object.Children[0].Children)-1]
			near(t, n.Bounds().Size().X, plain.object.Bounds().Size().X*.65)
			near(t, n.Bounds().Size().Y, plain.object.Bounds().Size().Y*.65)
			checkLimitsBox(t, b)
		}
	}
	for _, source := range []string{
		`\mathbb{R_{i}^{2}}+\mathcal{A_{g}}+\mathfrak{g_{a}}`,
		`\hat{\mathbb{R}}+\overline{\mathcal{AB}}+\underline{\mathfrak{g}}`,
		`\frac{\mathbb{R}}{\mathcal{A}}+\sqrt{\mathfrak{g}}`,
		`\left[\begin{matrix}\mathbb{R}&\mathcal{A}\\\mathfrak{g}&\mathbb{0}\end{matrix}\right]`,
		`\displaystyle\operatorname*{arg max}_{x\in\mathbb{R}}\overset{\mathcal{A}}{\mathfrak{f}}(x)`,
		`\mathbb{R+\frac{A}{B}+\sum\limits_{i}^{N}x+\mathcal{A}}`,
	} {
		checkLimitsBox(t, mustLayout(t, source))
	}
	// Display declarations nested inside an alphabet group retain their existing
	// row scope: the first sum stacks; the following sum is inline again.
	b := mustLayout(t, `\mathbb{\displaystyle\sum_i^N R}\sum_i^n`)
	stackedOperator(t, b.object.Children[0].Children[0], true)
	stackedOperator(t, b.object.Children[1], false)
}

func TestMathAlphabetCustomFontStrictCoverage(t *testing.T) {
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
	for _, source := range []string{`\mathbb{R}`, `ℝ`, `\mathcal{A}`, `\mathfrak{g}`, `\mathbf{𝔸}`} {
		if object, err := New(source, opt); err == nil || object != nil {
			t.Fatal("custom font silently supplemented", source, err)
		}
	}
	opt = DefaultOptions()
	opt.BoldFont = customBold
	if object, err := New(`\mathbf{𝔸}`, opt); err == nil || object != nil {
		t.Fatal("custom style variant silently supplemented")
	}
	dataDir := filepath.Join("..", "tests", "data", "mathtex", "alphabets")
	manifest, err := os.ReadFile(filepath.Join(dataDir, "reference.json"))
	if err != nil {
		t.Fatal(err)
	}
	var ref struct {
		Custom struct{ File, SHA256 string } `json:"custom_font"`
	}
	if err := json.Unmarshal(manifest, &ref); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dataDir, ref.Custom.File))
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	if hex.EncodeToString(digest[:]) != ref.Custom.SHA256 {
		t.Fatal("custom font fixture hash changed")
	}
	f, err := text.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	opt = DefaultOptions()
	opt.Font = f
	for _, source := range []string{`\mathbb{R}`, `\mathcal{A}`, `\mathfrak{A}`, `ℝ𝒜𝔄`} {
		if _, err := New(source, opt); err != nil {
			t.Fatal("custom font's actual glyph rejected", source, err)
		}
	}
	for _, source := range []string{`\mathbb{N}`, `\mathcal{B}`, `\mathfrak{g}`, `\mathbf{ℝ}`} {
		if object, err := New(source, opt); err == nil || object != nil {
			t.Fatal("custom missing glyph/style accepted", source, err)
		}
	}
}

func TestMathAlphabetErrorsAndSharedBudgets(t *testing.T) {
	for _, source := range []string{
		`\mathbb`, `\mathbb R`, `\mathbb{R`, `\mathcal{1}`, `\mathfrak{0}`,
		`\mathbb{\alpha}`, `\mathcal{é}`, `\mathfrak{Ж}`, "\\mathbb{R\u0301}",
		`\mathbb{٠}`, `\mathbb{R}\limits_i`, `\mathscr{A}`, `\mathbbit{R}`,
		`\mathbb{\unknown{R}}`, `\mathbb{R}_i_j`, "\U0001d53a", "\\mathbb{R}\xff",
	} {
		if object, err := New(source, DefaultOptions()); err == nil || object != nil {
			t.Fatal("unsupported/malformed alphabet accepted", source, err)
		}
	}
	for _, size := range []float64{0, -1, 1e6 + 1, math.Inf(1), math.NaN(), math.SmallestNonzeroFloat64} {
		opt := DefaultOptions()
		opt.Size = size
		if object, err := New(`\mathbb{R}`, opt); err == nil || object != nil {
			t.Fatal("invalid/collapsed size accepted", size, err)
		}
	}
	opt := DefaultOptions()
	opt.Size = 1e6
	if _, err := New(`\mathbb{R}`, opt); err != nil {
		t.Fatal("valid size boundary rejected", err)
	}
	boundary := strings.Repeat(" ", MaxSourceRunes-len(`\mathbb{R}`)) + `\mathbb{R}`
	if _, err := New(boundary, DefaultOptions()); err != nil {
		t.Fatal(err)
	}
	if _, err := New(boundary+" ", DefaultOptions()); err == nil {
		t.Fatal("source boundary bypass")
	}
	for _, over := range []int{0, 1} {
		n := MaxNesting - 1 + over
		source := strings.Repeat(`\mathbb{`, n) + "R" + strings.Repeat("}", n)
		if _, err := New(source, DefaultOptions()); (err != nil) != (over == 1) {
			t.Fatal("alphabet nesting boundary", n, err)
		}
	}
	for _, ref := range alphabetReferences(t) {
		total := countAccentCurves(mustLayout(t, ref.Rune).object)
		for _, over := range []int{0, 1} {
			p := parser{options: DefaultOptions(), curves: MaxLayoutCurves - total + over}
			_, err := p.alphabetGlyph([]rune(ref.Rune)[0], p.options.Size)
			if over == 0 && (err != nil || p.curves != MaxLayoutCurves) || over == 1 && (err == nil || !strings.Contains(err.Error(), "curves")) {
				t.Fatal("alphabet shared curve boundary", ref.Rune, p.curves, err)
			}
		}
	}
	p := parser{source: []rune(`\hat{\mathbb{R}}_{\mathfrak{g}}+\mathcal{A\cup B}`), options: DefaultOptions()}
	b, _, err := p.row(topRow, p.options.Size)
	if err != nil || p.curves != countAccentCurves(b.object) {
		t.Fatal("alphabet composite curve count", p.curves, err)
	}
}

func TestMathAlphabetConcurrentIndependentOwnership(t *testing.T) {
	const source = `\mathbb{R}+\mathcal{A_g}+\mathfrak{g}+\displaystyle\sum_{x\in\mathbb{N}}^{n}x`
	expected := mustLayout(t, source)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 6 {
				b, err := layout(source, DefaultOptions())
				if err != nil {
					t.Error(err)
					return
				}
				if b.object.Bounds() != expected.object.Bounds() || b.width != expected.width || b.height != expected.height || b.depth != expected.depth {
					t.Error("concurrent alphabet layout differs")
					return
				}
				named(b.object, "ℝ").Path.Contours[0].Curves[0].P0.X += 100
				b.object.Shift(geom.V(100, 100)).SetFill(gomath.Red)
			}
		}()
	}
	wg.Wait()
	requireSameDrawablePaths(t, expected.object, mustLayout(t, source).object)
	pair := mustLayout(t, `\mathbb{RR}`)
	row := pair.object.Children[0]
	before := row.Children[1].Bounds()
	named(row.Children[0], "ℝ").Path.Contours[0].Curves[0].P0.X += 100
	if row.Children[1].Bounds() != before {
		t.Fatal("alphabet glyphs share mutable contours")
	}
}

func BenchmarkMathAlphabetsTypesetting(b *testing.B) {
	const source = `\displaystyle\operatorname*{arg max}_{x\in\mathbb{R}}\overset{\mathcal{A}}{\mathfrak{f}}(x)+\mathbb{C^{2}}+\frac{\mathcal{L}_{g}}{\mathfrak{h}}`
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
