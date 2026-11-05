package text_test

import (
	"bytes"
	"encoding/json"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
	"gomath"
	"gomath/geom"
	"gomath/path"
	"gomath/render"
	"gomath/text"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func fallbackDir() string { return filepath.Join("..", "tests", "data", "text", "fallback") }

func fallbackFonts(t testing.TB) []*text.Font {
	t.Helper()
	fonts := make([]*text.Font, 2)
	for i, filename := range []string{"primary.ttf", "secondary.ttf"} {
		data, err := os.ReadFile(filepath.Join(fallbackDir(), filename))
		if err != nil {
			t.Fatal(err)
		}
		fonts[i], err = text.Parse(data)
		if err != nil {
			t.Fatal(err)
		}
	}
	return fonts
}

func closeMetric(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-12 {
		t.Fatalf("metric %.17g, want %.17g", got, want)
	}
}

func textGeometry(o *gomath.Object) []byte {
	type drawable struct {
		Name  string
		Path  path.Path
		Style gomath.Style
	}
	var nodes []drawable
	o.Walk(func(n *gomath.Object) { nodes = append(nodes, drawable{n.Name, n.Path, n.Style}) })
	data, err := json.Marshal(nodes)
	if err != nil {
		panic(err)
	}
	return data
}

func TestFallbackCoverageOrderAndKerning(t *testing.T) {
	fonts := fallbackFonts(t)
	opt := text.DefaultOptions()
	opt.Size = 1
	width := func(value string, f *text.Font) float64 {
		t.Helper()
		_, m, err := text.Layout(value, f, opt)
		if err != nil {
			t.Fatal(err)
		}
		return m.Width
	}
	for _, tc := range []struct {
		value string
		want  float64
	}{
		{"AV", width("A", fonts[0]) + width("V", fonts[0]) - 160./2048},
		{"βg", width("β", fonts[1]) + width("g", fonts[1]) - 96./2048},
		{"Aβ", width("A", fonts[0]) + width("β", fonts[1])},
		{"βA", width("β", fonts[1]) + width("A", fonts[0])},
		{"A\nV", math.Max(width("A", fonts[0]), width("V", fonts[0]))},
	} {
		_, m, err := text.LayoutWithFallback(tc.value, fonts, opt)
		if err != nil {
			t.Fatal(err)
		}
		closeMetric(t, m.Width, tc.want)
	}
	for _, order := range [][]*text.Font{fonts, {fonts[1], fonts[0]}} {
		got, err := text.NewWithFallback("AV", order, opt)
		if err != nil {
			t.Fatal(err)
		}
		want, err := text.New("AV", order[0], opt)
		if err != nil || !bytes.Equal(textGeometry(got), textGeometry(want)) {
			t.Fatal("actual first-font outline/kerning priority lost", err)
		}
	}
	// Empty outlines are real coverage: a space must not select .notdef.
	spaces, m, err := text.LayoutWithFallback(" \t ", fonts, opt)
	if err != nil || len(spaces.Children) != 0 {
		t.Fatal("whitespace replaced by missing glyph", err)
	}
	closeMetric(t, m.Width, 3*width(" ", fonts[0]))
	duplicated := []*text.Font{fonts[0], fonts[0], fonts[1]}
	_, m, err = text.LayoutWithFallback("AV", duplicated, opt)
	if err != nil {
		t.Fatal(err)
	}
	closeMetric(t, m.Width, width("AV", fonts[0]))
}

func TestFallbackLineMetricsAndBaseline(t *testing.T) {
	fonts := fallbackFonts(t)
	opt := text.DefaultOptions()
	opt.Size, opt.LineHeight = .8, 1.7
	for _, value := range []string{"", "A", "β", "\n\n", "A\nβ\n"} {
		_, m, err := text.LayoutWithFallback(value, fonts, opt)
		if err != nil {
			t.Fatal(err)
		}
		closeMetric(t, m.Ascent, 2300./2048*opt.Size)
		closeMetric(t, m.Descent, 700./2048*opt.Size)
		if m.Lines != strings.Count(value, "\n")+1 {
			t.Fatal("empty lines lost", m)
		}
		closeMetric(t, m.Height, m.Ascent+m.Descent+float64(m.Lines-1)*opt.Size*opt.LineHeight)
	}
	multi, _, err := text.LayoutWithFallback("A\nβ", fonts, opt)
	if err != nil {
		t.Fatal(err)
	}
	for i, tc := range []struct {
		value string
		font  *text.Font
	}{{"A", fonts[0]}, {"β", fonts[1]}} {
		glyph, err := text.New(tc.value, tc.font, opt)
		if err != nil {
			t.Fatal(err)
		}
		glyph.Shift(geom.V(0, -float64(i)*opt.Size*opt.LineHeight))
		got, want := multi.Children[i].Path, glyph.Children[0].Path
		if len(got.Contours) != len(want.Contours) {
			t.Fatal("fallback outline topology differs", i)
		}
		for j, contour := range got.Contours {
			if len(contour.Curves) != len(want.Contours[j].Curves) {
				t.Fatal("fallback curve count differs")
			}
			for k, curve := range contour.Curves {
				other := want.Contours[j].Curves[k]
				for point, p := range []geom.Vec3{curve.P0, curve.P1, curve.P2, curve.P3} {
					q := []geom.Vec3{other.P0, other.P1, other.P2, other.P3}[point]
					if p.Sub(q).Norm() > 1e-12 {
						t.Fatal("fallback baseline/em differs from single font", i, j, k, point)
					}
				}
			}
		}
	}
}

func TestFallbackMissingPolicy(t *testing.T) {
	fonts := fallbackFonts(t)
	opt := text.DefaultOptions()
	const missing = "\U0010ffff"
	if o, m, err := text.LayoutWithFallback("β"+missing, fonts, opt); err == nil || o != nil || m != (text.Metrics{}) || !strings.Contains(err.Error(), "U+10FFFF") {
		t.Fatal("missing character not explicit/atomic", o, m, err)
	}
	opt.AllowMissing = true
	// AllowMissing must still search later fonts for actual glyphs first.
	actual, _, err := text.LayoutWithFallback("β", fonts, opt)
	if err != nil {
		t.Fatal(err)
	}
	want, err := text.New("β", fonts[1], opt)
	if err != nil || !bytes.Equal(textGeometry(actual), textGeometry(want)) {
		t.Fatal("AllowMissing skipped actual fallback coverage", err)
	}
	actual, am, err := text.LayoutWithFallback(missing, fonts, opt)
	if err != nil {
		t.Fatal(err)
	}
	want, wm, err := text.Layout(missing, fonts[0], opt)
	if err != nil || !bytes.Equal(textGeometry(actual), textGeometry(want)) || am.Width != wm.Width {
		t.Fatal("missing glyph did not use primary .notdef", err)
	}
}

func TestFallbackMalformedSelectedOutlineIsAnError(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(fallbackDir(), "broken.ttf"))
	if err != nil {
		t.Fatal(err)
	}
	broken, err := text.Parse(data)
	if err != nil {
		t.Fatal("fixture must parse before lazy outline validation", err)
	}
	fonts := fallbackFonts(t)
	for _, allow := range []bool{false, true} {
		opt := text.DefaultOptions()
		opt.AllowMissing = allow
		if o, m, err := text.LayoutWithFallback("A", []*text.Font{broken, fonts[1]}, opt); err == nil || o != nil || m != (text.Metrics{}) || !strings.Contains(err.Error(), "load fallback glyph U+0041") {
			t.Fatal("malformed actual glyph silently replaced by later font", err)
		}
	}
}

func TestFallbackValidationAndBudgets(t *testing.T) {
	fonts := fallbackFonts(t)
	opt := text.DefaultOptions()
	tooMany := make([]*text.Font, text.MaxFallbackFonts+1)
	for i := range tooMany {
		tooMany[i] = fonts[0]
	}
	for _, list := range [][]*text.Font{nil, {}, {nil}, {&text.Font{}}, {fonts[0], nil}, tooMany} {
		if o, m, err := text.LayoutWithFallback("", list, opt); err == nil || o != nil || m != (text.Metrics{}) {
			t.Fatal("invalid list accepted", len(list), err)
		}
	}
	maxList := append([]*text.Font(nil), tooMany[:text.MaxFallbackFonts]...)
	maxList[len(maxList)-1] = fonts[1]
	if _, err := text.NewWithFallback("β", maxList, opt); err != nil {
		t.Fatal("last permitted font not searched", err)
	}
	for _, value := range []string{string([]byte{0xff}), strings.Repeat(" ", text.MaxRunes+1), strings.Repeat("\U0010ffff", text.MaxRunes+1)} {
		if _, _, err := text.LayoutWithFallback(value, fonts, opt); err == nil {
			t.Fatal("invalid source accepted")
		}
	}
	for _, field := range []string{"size", "lineheight"} {
		for _, n := range []float64{0, -1, math.NaN(), math.Inf(1), 1e7, math.SmallestNonzeroFloat64} {
			bad := opt
			if field == "size" {
				bad.Size = n
			} else {
				bad.LineHeight = n
				// Ensure multiplication underflows as well.
				if n == math.SmallestNonzeroFloat64 {
					bad.Size = .01
				}
			}
			if _, _, err := text.LayoutWithFallback("", fonts, bad); err == nil {
				t.Fatal("invalid option accepted", field, n)
			}
		}
	}
	for _, paint := range []gomath.Color{{R: math.NaN()}, {A: 2}, {B: -1}} {
		bad := opt
		bad.Color = paint
		if _, _, err := text.LayoutWithFallback("A", fonts, bad); err == nil {
			t.Fatal("invalid color accepted")
		}
	}
	if _, m, err := text.LayoutWithFallback(strings.Repeat(" ", text.MaxRunes), fonts, opt); err != nil || m.Lines != 1 {
		t.Fatal("exact rune budget rejected", m, err)
	}
	if _, _, err := text.LayoutWithFallback(strings.Repeat("g", text.MaxRunes), fonts, opt); err == nil || !strings.Contains(err.Error(), "glyph segments") {
		t.Fatal("combined fallback segment budget ignored", err)
	}
}

func TestSingleFontFallbackCompatibilityAndIndependentObjects(t *testing.T) {
	opt := text.DefaultOptions()
	for _, f := range []*text.Font{text.DefaultFont(), fallbackFonts(t)[0]} {
		for _, value := range []string{"", "AV", "x A\tV \n\nAV ", "\U0010ffff"} {
			opt.AllowMissing = true
			a, am, err := text.Layout(value, f, opt)
			if err != nil {
				t.Fatal(err)
			}
			b, bm, err := text.LayoutWithFallback(value, []*text.Font{f}, opt)
			if err != nil || am != bm || !bytes.Equal(textGeometry(a), textGeometry(b)) {
				t.Fatal("single-font geometry/metrics changed", err)
			}
			if len(a.Children) > 0 && a.Children[0] == b.Children[0] {
				t.Fatal("glyph objects shared")
			}
		}
	}
	fonts := fallbackFonts(t)
	a, err := text.NewWithFallback("AVβg", fonts, opt)
	if err != nil {
		t.Fatal(err)
	}
	before := textGeometry(a)
	// Neither the caller list nor the returned object is retained by the fonts.
	fonts[0], fonts[1] = nil, nil
	fonts = fallbackFonts(t)
	b, err := text.NewWithFallback("AVβg", fonts, opt)
	if err != nil || !bytes.Equal(before, textGeometry(b)) {
		t.Fatal("list ownership changed output", err)
	}
	b.Shift(geom.V(10, 10))
	b.Children[0].Style.Fill = gomath.Red
	if !bytes.Equal(before, textGeometry(a)) {
		t.Fatal("returned glyphs share mutable storage")
	}
}

func TestFallbackSegmentBudgetCombinesFonts(t *testing.T) {
	counts := make([]int, 2)
	for i, filename := range []string{"primary.ttf", "secondary.ttf"} {
		data, err := os.ReadFile(filepath.Join(fallbackDir(), filename))
		if err != nil {
			t.Fatal(err)
		}
		f, err := sfnt.Parse(data)
		if err != nil {
			t.Fatal(err)
		}
		var buf sfnt.Buffer
		index, err := f.GlyphIndex(&buf, []rune{'A', 'g'}[i])
		if err != nil || index == 0 {
			t.Fatal("missing budget fixture glyph", err)
		}
		segments, err := f.LoadGlyph(&buf, index, fixed.Int26_6(64*1024), nil)
		if err != nil || len(segments) == 0 {
			t.Fatal("invalid budget fixture glyph", err)
		}
		counts[i] = len(segments)
	}
	// Each individual font contributes less than the cap; their combined run
	// must fail before allocating a large output tree. Both are actual outlines.
	first := (3 * text.MaxGlyphSegments / 10) / counts[0]
	second := (8 * text.MaxGlyphSegments / 10) / counts[1]
	if first+second > text.MaxRunes || first*counts[0] >= text.MaxGlyphSegments || second*counts[1] >= text.MaxGlyphSegments || first*counts[0]+second*counts[1] <= text.MaxGlyphSegments {
		t.Fatal("combined budget fixture no longer exercises the boundary", counts, first, second)
	}
	value := strings.Repeat("A", first) + strings.Repeat("g", second)
	if o, m, err := text.LayoutWithFallback(value, fallbackFonts(t), text.DefaultOptions()); err == nil || o != nil || m != (text.Metrics{}) || !strings.Contains(err.Error(), "glyph segments") {
		t.Fatal("segment cap was applied separately per font", err)
	}
}

func TestFallbackConcurrentLayoutAndRendering(t *testing.T) {
	fonts := fallbackFonts(t)
	opt := text.DefaultOptions()
	base, err := text.NewWithFallback("AV βg\nVx", fonts, opt)
	if err != nil {
		t.Fatal(err)
	}
	base.Shift(geom.V(-1, 0))
	before := textGeometry(base)
	c := render.DefaultConfig()
	c.Width, c.Height, c.Workers, c.FrameWidth, c.Background = 240, 160, 1, 4, gomath.Black
	r, err := render.NewRasterizer(c)
	if err != nil {
		t.Fatal(err)
	}
	want, err := r.Render(&gomath.Frame{Objects: []*gomath.Object{base}})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := render.NewRasterizer(c)
			if err != nil {
				t.Error(err)
				return
			}
			for range 4 {
				o, err := text.NewWithFallback("AV βg\nVx", fonts, opt)
				if err != nil {
					t.Error(err)
					return
				}
				o.Shift(geom.V(-1, 0))
				if !bytes.Equal(before, textGeometry(o)) {
					t.Error("concurrent glyph layout changed")
				}
				for _, object := range []*gomath.Object{base, o.Copy()} {
					got, err := r.Render(&gomath.Frame{Objects: []*gomath.Object{object}})
					if err != nil || got == nil || !bytes.Equal(want.Pix, got.Pix) {
						t.Error("concurrent render changed", err)
					}
				}
			}
		}()
	}
	wg.Wait()
	if !bytes.Equal(before, textGeometry(base)) {
		t.Fatal("shared render input mutated")
	}
}

func BenchmarkFallbackLayout(b *testing.B) {
	fonts := fallbackFonts(b)
	opt := text.DefaultOptions()
	for _, count := range []int{2, text.MaxFallbackFonts} {
		b.Run(map[int]string{2: "2fonts", text.MaxFallbackFonts: "16fonts"}[count], func(b *testing.B) {
			list := make([]*text.Font, count)
			for i := range list {
				list[i] = fonts[0]
			}
			list[count-1] = fonts[1]
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if _, _, err := text.LayoutWithFallback("AV βgA\n\tVx ", list, opt); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
