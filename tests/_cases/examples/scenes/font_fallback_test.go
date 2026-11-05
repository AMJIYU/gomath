package scenes

import (
	"gomath/text"
	"testing"
)

func TestFontFallbackDemoUsesActualSecondaryCoverage(t *testing.T) {
	font, err := fallbackDemoFont()
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"ℝ", "𝒜", "𝔄"} {
		options := text.DefaultOptions()
		if object, err := text.New(value, text.DefaultFont(), options); err == nil || object != nil {
			t.Fatal("example no longer exercises a missing primary glyph", value)
		}
		object, err := text.NewWithFallback(value, []*text.Font{text.DefaultFont(), font}, options)
		if err != nil || len(object.Children) != 1 || !object.Bounds().Valid {
			t.Fatal("example secondary glyph missing", value, err)
		}
	}
	if object, err := text.NewWithFallback("∈", []*text.Font{text.DefaultFont(), font}, text.DefaultOptions()); err == nil || object != nil {
		t.Fatal("missing-in-all example glyph silently substituted")
	}
}
