package mathtex

import (
	"gomath/text"
	"strings"
	"testing"
)

func TestOperatorCommandsUseRealEmbeddedGlyphs(t *testing.T) {
	for _, style := range []text.FontStyle{text.Regular, text.Bold, text.Italic, text.BoldItalic} {
		for command, symbol := range map[string]string{"int": "∫", "sum": "∑", "prod": "∏"} {
			t.Run(command+"/"+[]string{"regular", "bold", "italic", "bold-italic"}[style], func(t *testing.T) {
				opt := DefaultOptions()
				opt.Style = style
				object, err := New(`\`+command, opt)
				if err != nil {
					t.Fatal(err)
				}
				sameGlyphShape(t, named(object, symbol), symbol, style)
			})
		}
	}
}

func TestOperatorSideScriptsAndNestedFractions(t *testing.T) {
	for command, symbol := range map[string]string{"int": "∫", "sum": "∑", "prod": "∏"} {
		for _, scripts := range []string{`_{0}^{1}`, `^{1}_{0}`, `_{\frac{a}{b}}^{\frac{n}{2}}`} {
			b := mustLayout(t, `\`+command+scripts)
			operator := b.object.Children[0]
			if len(operator.Children) != 3 {
				t.Fatal("operator lost base or scripts", command, scripts)
			}
			base, sup, sub := operator.Children[0].Bounds(), operator.Children[1].Bounds(), operator.Children[2].Bounds()
			if sup.Min.X <= base.Max.X || sub.Min.X <= base.Max.X {
				t.Fatal("scripts must stay to the right of the operator", command, scripts)
			}
			if sup.Max.Y <= base.Max.Y || sub.Min.Y >= base.Min.Y {
				t.Fatal("operator script extents lost", command, scripts)
			}
			near(t, b.height, sup.Max.Y)
			near(t, b.depth, -sub.Min.Y)
			sameGlyphShape(t, named(operator, symbol), symbol, text.Regular)
		}
	}
	mustLayout(t, `\int_{0}^{1}\frac{x^{2}}{1+x}dx+\sum_{i=1}^{n}\frac{1}{i}+\prod_{j=1}^{m}x_j`)
	mustLayout(t, `\left[\begin{matrix}\mathbf{a}&b\\c&\mathit{d}\end{matrix}\right]\quad\int f(x)dx`)
}

func TestOperatorDisplayLimitsDirectiveRemainsUnsupported(t *testing.T) {
	for _, command := range []string{"int", "sum", "prod"} {
		for _, directive := range []string{"displaylimits"} {
			object, err := New(`\`+command+`\`+directive+`_{0}^{1}`, DefaultOptions())
			if err == nil || object != nil || !strings.Contains(err.Error(), `unsupported command \`+directive) {
				t.Fatal("unsupported limits placement must fail explicitly", command, directive, err)
			}
		}
	}
}
