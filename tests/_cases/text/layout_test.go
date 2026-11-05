package text_test

import (
	"golang.org/x/image/font/gofont/goregular"
	"gomath"
	"gomath/geom"
	"gomath/text"
	"math"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestDefaultFontCacheAndRealStyleOutlines(t *testing.T) {
	fonts := make([]*text.Font, 4)
	shapes := make([]*gomath.Object, 4)
	for i := range fonts {
		var err error
		fonts[i], err = text.DefaultFontForStyle(text.FontStyle(i))
		if err != nil {
			t.Fatal(err)
		}
		again, err := text.DefaultFontForStyle(text.FontStyle(i))
		if err != nil || again != fonts[i] {
			t.Fatal("font not cached")
		}
		shapes[i], err = text.New("Agα", fonts[i], text.DefaultOptions())
		if err != nil {
			t.Fatal(err)
		}
	}
	if fonts[0] != text.DefaultFont() {
		t.Fatal("regular font has separate cache")
	}
	for i := range fonts {
		for j := 0; j < i; j++ {
			if fonts[i] == fonts[j] || reflect.DeepEqual(shapes[i].Children[0].Path, shapes[j].Children[0].Path) {
				t.Fatal("font styles share regular outlines", i, j)
			}
		}
	}
	if _, err := text.DefaultFontForStyle(99); err == nil {
		t.Fatal("unknown font style accepted")
	}
	var wg sync.WaitGroup
	for i := range 12 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			f, err := text.DefaultFontForStyle(text.FontStyle(i % 4))
			if err != nil || f != fonts[i%4] {
				t.Error("concurrent cache differs")
			}
			if _, err := text.New("Styled αβ", f, text.DefaultOptions()); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	if n := testing.AllocsPerRun(100, func() { _ = text.DefaultFont() }); n != 0 {
		t.Fatal("default font reparsed", n)
	}
}

func TestEmbeddedIntegralSumProductGlyphs(t *testing.T) {
	for _, style := range []text.FontStyle{text.Regular, text.Bold, text.Italic, text.BoldItalic} {
		f, err := text.DefaultFontForStyle(style)
		if err != nil {
			t.Fatal(err)
		}
		for _, symbol := range []string{"∫", "∑", "∏"} {
			object, err := text.New(symbol, f, text.DefaultOptions())
			if err != nil {
				t.Fatalf("style %d symbol %s: %v", style, symbol, err)
			}
			if len(object.Children) != 1 || len(object.Children[0].Path.Contours) == 0 || !object.Bounds().Valid {
				t.Fatalf("style %d symbol %s has no real outline", style, symbol)
			}
		}
	}
}

func TestLayoutWhitespaceAdvancesAndNewlines(t *testing.T) {
	opt := text.DefaultOptions()
	font := text.DefaultFont()
	_, space, err := text.Layout(" ", font, opt)
	if err != nil {
		t.Fatal(err)
	}
	a, am, err := text.Layout("A", font, opt)
	if err != nil {
		t.Fatal(err)
	}
	padded, pm, err := text.Layout("  A   ", font, opt)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(pm.Width-am.Width-5*space.Width) > 1e-9 {
		t.Fatal("leading/trailing spaces lost", am, pm, space)
	}
	if math.Abs(padded.Bounds().Min.X-a.Bounds().Min.X-2*space.Width) > 1e-9 {
		t.Fatal("leading spaces not positioned")
	}
	_, blank, err := text.Layout("   ", font, opt)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(blank.Width-3*space.Width) > 1e-9 {
		t.Fatal("blank run has no advance")
	}
	_, multiline, err := text.Layout("A \nA\n", font, opt)
	if err != nil {
		t.Fatal(err)
	}
	if multiline.Lines != 3 || math.Abs(multiline.Width-am.Width-space.Width) > 1e-9 || math.Abs(multiline.Height-multiline.Ascent-multiline.Descent-2*opt.Size*opt.LineHeight) > 1e-9 {
		t.Fatal("bad multiline metrics", multiline)
	}
	_, tab, err := text.Layout("\t", font, opt)
	if err != nil || math.Abs(tab.Width-space.Width) > 1e-9 {
		t.Fatal("tab compatibility changed")
	}
}

func TestParsedFontOwnsBytesAndObjectsAreIndependent(t *testing.T) {
	data := append([]byte(nil), goregular.TTF...)
	font, err := text.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	before, err := text.New("Ag", font, text.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	clear(data)
	after, err := text.New("Ag", font, text.DefaultOptions())
	if err != nil {
		t.Fatal("font references mutable caller bytes", err)
	}
	for i := range before.Children {
		if !reflect.DeepEqual(before.Children[i].Path, after.Children[i].Path) {
			t.Fatal("font mutated")
		}
	}
	after.Shift(geom.V(100, 100))
	third, err := text.New("Ag", font, text.DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if before.Bounds() != third.Bounds() {
		t.Fatal("glyph objects shared mutable paths")
	}
}

func TestTextValidationAndBounds(t *testing.T) {
	for _, n := range []float64{0, -1, math.NaN(), math.Inf(1), math.Inf(-1), 1e7} {
		for _, field := range []string{"size", "line height"} {
			opt := text.DefaultOptions()
			if field == "size" {
				opt.Size = n
			} else {
				opt.LineHeight = n
			}
			if _, err := text.New("", text.DefaultFont(), opt); err == nil {
				t.Fatal("invalid option accepted", field, n)
			}
		}
	}
	for _, color := range []gomath.Color{gomath.RGB(math.NaN(), 0, 0), gomath.RGB(2, 0, 0), {A: math.Inf(1)}} {
		opt := text.DefaultOptions()
		opt.Color = color
		if _, err := text.New("", text.DefaultFont(), opt); err == nil {
			t.Fatal("invalid color accepted")
		}
	}
	if _, err := text.New("", &text.Font{}, text.DefaultOptions()); err == nil {
		t.Fatal("uninitialized font accepted")
	}
	if _, err := text.New(strings.Repeat(" ", text.MaxRunes+1), text.DefaultFont(), text.DefaultOptions()); err == nil {
		t.Fatal("text rune bound ignored")
	}
}

func BenchmarkDefaultFontCached(b *testing.B) {
	_ = text.DefaultFont()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = text.DefaultFont()
	}
}
func BenchmarkStyledTextLayout(b *testing.B) {
	f, _ := text.DefaultFontForStyle(text.BoldItalic)
	opt := text.DefaultOptions()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, _, err := text.Layout("GoMath αβ 123  ", f, opt); err != nil {
			b.Fatal(err)
		}
	}
}
