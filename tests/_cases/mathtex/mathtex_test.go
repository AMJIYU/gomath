package mathtex_test

import (
	"gomath/mathtex"
	"strings"
	"testing"
)

func TestMathSubset(t *testing.T) {
	for _, s := range []string{`x^{2}+y_{1}`, `\frac{a+1}{b^{2}}`, `\sqrt{x^{2}+1}`, `\alpha+\beta=\pi`, `\frac{\frac{1}{2}}{\sqrt{x}}`, `\sin(x)+\cos(x)`} {
		t.Run(s, func(t *testing.T) {
			o, e := mathtex.New(s, mathtex.DefaultOptions())
			if e != nil {
				t.Fatal(e)
			}
			b := o.Bounds()
			if !b.Valid || b.Size().X <= 0 || b.Size().Y <= 0 {
				t.Fatal("empty formula")
			}
		})
	}
}
func TestRejectUnsupportedOrMalformed(t *testing.T) {
	for _, s := range []string{`\unknown{x}`, `\frac{x}`, `x^{`, "}", `x^^2`, `x^{2}^{3}`, strings.Repeat("{", 130) + "x" + strings.Repeat("}", 130)} {
		if _, e := mathtex.New(s, mathtex.DefaultOptions()); e == nil {
			t.Errorf("accepted %q", s)
		}
	}
}
