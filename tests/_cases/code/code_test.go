package code

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"golang.org/x/image/font/gofont/gomono"
	"gomath"
	"gomath/geom"
	"gomath/path"
	"gomath/text"
)

func fixture(t testing.TB, name string) []byte {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			b, e := os.ReadFile(filepath.Join(dir, "tests", "data", "code", name))
			if e != nil {
				t.Fatal(e)
			}
			return b
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no repository root")
		}
		dir = parent
	}
}

func mustCode(t testing.TB, source string, opt Options) *Code {
	t.Helper()
	c, err := New(source, opt)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func closeValue(t testing.TB, a, b float64) {
	t.Helper()
	if math.Abs(a-b) > 2e-8 {
		t.Fatalf("got %.17g, want %.17g", a, b)
	}
}

func TestCodePhysicalGridAndStableSelections(t *testing.T) {
	o := DefaultOptions()
	o.LineNumbers = true
	o.Background = true
	o.FirstLineNumber = 98
	s := "\tvar α = `a\tb\r\nc` /* x\r\n\ty */\r\n\r\n"
	c := mustCode(t, s, o)
	if c.Text() != s || len(c.Lines()) != 5 || c.Lines()[0].Columns != 17 || c.Lines()[4].Columns != 0 {
		t.Fatal(c.Text(), c.Lines())
	}
	if c.Background() == nil || c.Background() != c.Object.Children[0] {
		t.Fatal("background must be first")
	}
	if c.metrics.Glyphs <= len(c.Glyphs()) || c.metrics.Curves < 4 {
		t.Fatal("line numbers missing from budgets")
	}
	for i, line := range c.Lines() {
		g, err := c.Line(i)
		if err != nil || g.Name != s[line.Start:line.End] {
			t.Fatal("line selection", i, err)
		}
		n, err := c.LineNumber(i)
		if err != nil || n.Name != []string{"98", "99", "100", "101", "102"}[i] {
			t.Fatal("decimal label", i, err)
		}
		if len(n.Children) == 0 {
			t.Fatal("empty number outline")
		}
	}
	var raw, comment int
	for _, tok := range c.Tokens() {
		if tok.Text != s[tok.Start:tok.End] {
			t.Fatal("nonverbatim token")
		}
		if tok.Category == String {
			raw = tok.Index
			if tok.Text != "`a\tb\r\nc`" {
				t.Fatal(tok)
			}
		}
		if tok.Category == Comment {
			comment = tok.Index
			if tok.Text != "/* x\r\n\ty */" {
				t.Fatal(tok)
			}
		}
	}
	for _, index := range []int{raw, comment} {
		selected, err := c.Token(index)
		if err != nil || len(selected) < 4 {
			t.Fatal("multiline selection", err)
		}
		p := selected[0]
		selected[0] = nil
		again, _ := c.Token(index)
		if again[0] != p {
			t.Fatal("selection slice aliases owner")
		}
		copy, err := c.TokenCopy(index)
		if err != nil || copy.Children[0] == p || copy.Children[0].ID == p.ID {
			t.Fatal("token copy aliases", err)
		}
		if err := gomath.NewScene().Add(c.Object, copy); err != nil {
			t.Fatal(err)
		}
	}
	lines, tokens, glyphs := c.Lines(), c.Tokens(), c.Glyphs()
	lines[0].Columns = -1
	tokens[0].Text = "changed"
	glyphs[0].Offset = -1
	if c.Lines()[0].Columns < 0 || c.Tokens()[0].Text == "changed" || c.Glyphs()[0].Offset < 0 {
		t.Fatal("metadata slice aliases")
	}
	first, _ := c.Line(0)
	glyph := c.Glyphs()[0].Object
	c.Object.Shift(geom.V(1, -2)).Scale(1.2)
	if same, _ := c.Line(0); same != first || c.Glyphs()[0].Object != glyph {
		t.Fatal("transforms lost child identity")
	}
	if _, err := c.Copy(); err != nil {
		t.Fatal(err)
	}
}

func TestCodeEmptyWhitespaceAndTabs(t *testing.T) {
	for _, s := range []string{"", " ", "\t  \r\n \t\n", "\n\n"} {
		o := DefaultOptions()
		c := mustCode(t, s, o)
		if len(c.Glyphs()) != 0 || len(c.Tokens()) != 0 {
			t.Fatal("whitespace fabricated ink")
		}
		if c.metrics.Lines != 1+strings.Count(s, "\n") {
			t.Fatal("lost empty lines")
		}
		if err := c.Object.Validate(); err != nil {
			t.Fatal(err)
		}
		if _, err := c.LineNumber(0); !errors.Is(err, ErrSelection) {
			t.Fatal(err)
		}
		o.LineNumbers = true
		o.Background = true
		d := mustCode(t, s, o)
		if !d.Object.Bounds().Valid {
			t.Fatal("empty listing lacks line boxes")
		}
	}
	for _, tab := range []int{1, 3, 4, 8, 64} {
		o := DefaultOptions()
		o.TabWidth = tab
		c := mustCode(t, "  \tvar x = 1\t ", o)
		first := 2 + tab - 2%tab
		if c.Tokens()[0].Column != first || c.Glyphs()[0].Column != first {
			t.Fatal("tab is not next stop", tab, c.Tokens())
		}
	}
}

func TestCodeErrorsAndLimits(t *testing.T) {
	type bad struct {
		name, source string
		edit         func(*Options)
		want         error
	}
	cases := []bad{
		{"python", "print(1)", func(o *Options) { o.Language = "python" }, ErrLanguage},
		{"alias", "x", func(o *Options) { o.Language = "golang" }, ErrLanguage},
		{"utf8", string([]byte{0xff}), nil, ErrSource},
		{"bare-cr", "// a\rb", nil, ErrSource}, {"null", "// \x00", nil, ErrSource},
		{"delete", "// \x7f", nil, ErrSource}, {"bidi", "// \u202e", nil, ErrSource},
		{"combining", "// a\u0301", nil, ErrSource}, {"nbsp", "// \u00a0", nil, ErrSource},
		{"bom", "\ufeffvar x=1", nil, ErrSource}, {"unicode-newline", "// \u2028", nil, ErrSource},
		{"escape", `var x = "\q"`, nil, ErrSource}, {"comment", "/* never closed", nil, ErrSource},
		{"raw", "`never closed", nil, ErrSource}, {"number", "var x=0x_", nil, ErrSource},
		{"missing", "// 数学", nil, ErrFont},
		{"font", "var x=1", func(o *Options) { o.Font = &text.Font{} }, ErrFont},
		{"zero-options", "x", func(o *Options) { *o = Options{} }, ErrOptions},
		{"size-low", "x", func(o *Options) { o.Size = math.SmallestNonzeroFloat64 }, ErrOptions},
		{"size-nan", "x", func(o *Options) { o.Size = math.NaN() }, ErrOptions},
		{"size-inf", "x", func(o *Options) { o.Size = math.Inf(1) }, ErrOptions},
		{"height", "x", func(o *Options) { o.LineHeight = .99 }, ErrOptions},
		{"height-inf", "x", func(o *Options) { o.LineHeight = math.Inf(1) }, ErrOptions},
		{"padding", "x", func(o *Options) { o.Padding = -1 }, ErrOptions},
		{"padding-nan", "x", func(o *Options) { o.Padding = math.NaN() }, ErrOptions},
		{"tab", "x", func(o *Options) { o.TabWidth = 0 }, ErrOptions},
		{"tab-high", "x", func(o *Options) { o.TabWidth = 65 }, ErrOptions},
		{"number-negative", "x", func(o *Options) { o.FirstLineNumber = -1 }, ErrOptions},
		{"number-high", "x", func(o *Options) { o.FirstLineNumber = 1e9 + 1 }, ErrOptions},
		{"palette", "x", func(o *Options) { p := DefaultPalette(); p.Comment.R = math.NaN(); o.Palette = &p }, ErrOptions},
		{"bytes", "//abcd", func(o *Options) { o.Limits.SourceBytes = 5 }, ErrLimit},
		{"runes", "//αβγ", func(o *Options) { o.Limits.Runes = 4 }, ErrLimit},
		{"lines", "\n\n", func(o *Options) { o.Limits.Lines = 2 }, ErrLimit},
		{"columns", "\t\t", func(o *Options) { o.Limits.Columns = 7 }, ErrLimit},
		{"tokens", "var x=1", func(o *Options) { o.Limits.Tokens = 3 }, ErrLimit},
		{"depth", "(((x)))", func(o *Options) { o.Limits.Depth = 2 }, ErrLimit},
		{"glyphs", "var x=1", func(o *Options) { o.Limits.Glyphs = 5 }, ErrLimit},
		{"number-glyphs", "\n", func(o *Options) { o.LineNumbers = true; o.Limits.Glyphs = 1 }, ErrLimit},
		{"curves", "8", func(o *Options) { o.Limits.Curves = 1 }, ErrLimit},
		{"bg-curves", "", func(o *Options) { o.Background = true; o.Limits.Curves = 3 }, ErrLimit},
		{"dimensions", strings.Repeat(" ", MaxColumns), func(o *Options) { o.Size = 1e3 }, ErrLimit},
		{"hard-bytes", strings.Repeat(" ", MaxSourceBytes+1), nil, ErrLimit},
		{"hard-runes", strings.Repeat(" ", MaxRunes+1), nil, ErrLimit},
		{"hard-lines", strings.Repeat("\n", MaxLines), nil, ErrLimit},
		{"hard-columns", strings.Repeat(" ", MaxColumns+1), nil, ErrLimit},
		{"hard-depth", strings.Repeat("(", MaxDepth+1), nil, ErrLimit},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o := DefaultOptions()
			if tc.edit != nil {
				tc.edit(&o)
			}
			c, err := New(tc.source, o)
			if c != nil || !errors.Is(err, tc.want) {
				t.Fatalf("got %v / %v, want %v", c, err, tc.want)
			}
		})
	}
	for _, field := range []string{"SourceBytes", "Runes", "Lines", "Columns", "Tokens", "Depth", "Glyphs", "Curves"} {
		for _, n := range []int{-1, MaxCurves + 1} {
			o := DefaultOptions()
			reflect.ValueOf(&o.Limits).Elem().FieldByName(field).SetInt(int64(n))
			if c, e := New("x", o); c != nil || !errors.Is(e, ErrOptions) {
				t.Fatal("bad limit accepted", field, n, e)
			}
		}
	}
}

func TestCodeExactBudgetBoundariesAndFonts(t *testing.T) {
	o := DefaultOptions()
	o.Background = true
	o.LineNumbers = true
	c := mustCode(t, "var α = 8\r\n", o)
	o.Limits = Limits{SourceBytes: len(c.Text()), Runes: len([]rune(c.Text())), Lines: len(c.Lines()), Columns: c.Metrics().Columns, Tokens: len(c.Tokens()), Glyphs: c.Metrics().Glyphs, Curves: c.Metrics().Curves}
	mustCode(t, c.Text(), o)
	o.Limits.Curves--
	if _, e := New(c.Text(), o); !errors.Is(e, ErrLimit) {
		t.Fatal("curve count not exact", e)
	}
	o = DefaultOptions()
	mustCode(t, strings.Repeat("(", MaxDepth), o)
	for _, size := range []float64{1e-6, 1e3} {
		o := DefaultOptions()
		o.Size = size
		mustCode(t, "var x=1", o)
	}
	f, e := text.Parse(fixture(t, "broken.ttf"))
	if e != nil {
		t.Fatal(e)
	}
	o = DefaultOptions()
	o.Font = f
	if _, e := New("A", o); !errors.Is(e, ErrFont) {
		t.Fatal("malformed lazy outline hidden", e)
	}
	o.Font = text.DefaultFont()
	if _, e := New("iii", o); !errors.Is(e, ErrFont) {
		t.Fatal("proportional face accepted", e)
	}
	bytes := append([]byte(nil), gomono.TTF...)
	f, e = text.Parse(bytes)
	if e != nil {
		t.Fatal(e)
	}
	for i := range bytes {
		bytes[i] = 0
	}
	o.Font = f
	mustCode(t, "var x=1", o)
	shared, e := DefaultFont()
	if e != nil {
		t.Fatal(e)
	}
	again, _ := DefaultFont()
	if shared != again {
		t.Fatal("default font is not cached")
	}
}

func TestCodePhysicalScannerErrorsAndPaletteSnapshot(t *testing.T) {
	o := DefaultOptions()
	o.TabWidth = 8
	s := "//line pretend.go:900\r\n\tvar α = \"\\q\""
	_, e := New(s, o)
	var se *SourceError
	if !errors.As(e, &se) || se.Offset != strings.Index(s, "q") || se.Line != 1 || se.Column != 18 {
		t.Fatal("physical source location", e, se)
	}
	p := DefaultPalette()
	p.Keyword = gomath.Red
	o.Palette = &p
	c := mustCode(t, "var x=1", o)
	p.Keyword = gomath.Green
	if c.Glyphs()[0].Object.Style.Fill != gomath.Red {
		t.Fatal("palette edits changed existing glyphs")
	}
	if _, e := c.Token(-1); !errors.Is(e, ErrSelection) {
		t.Fatal(e)
	}
	if _, e := c.Line(100); !errors.Is(e, ErrSelection) {
		t.Fatal(e)
	}
	if _, e := c.TokenCopy(100); !errors.Is(e, ErrSelection) {
		t.Fatal(e)
	}
	var empty *Code
	if _, e := empty.Copy(); !errors.Is(e, ErrObject) {
		t.Fatal(e)
	}
	if _, e := empty.Snapshot(); !errors.Is(e, ErrObject) {
		t.Fatal(e)
	}
	if _, e := (*Snapshot)(nil).Copy(); !errors.Is(e, ErrObject) {
		t.Fatal(e)
	}
	if empty.Text() != "" || empty.Background() != nil || empty.Lines() != nil || empty.Glyphs() != nil || empty.Tokens() != nil || empty.Metrics() != (Metrics{}) {
		t.Fatal("nil metadata")
	}
}

func TestCodeCopyRejectsInvalidAuthoringWithoutRecursion(t *testing.T) {
	for _, edit := range []func(*Code){
		func(c *Code) { c.Object.Children = append(c.Object.Children, c.Object) },
		func(c *Code) { c.Object.Children[0] = c.Object },
		func(c *Code) { c.Object = c.Object.Copy() },
		func(c *Code) { c.Glyphs()[0].Object.ID = 0 },
		func(c *Code) { c.Glyphs()[0].Object.ID = c.Object.ID },
		func(c *Code) { c.Object.Style.Opacity = math.NaN() },
		func(c *Code) { c.Object.Bitmap = &gomath.Bitmap{} },
		func(c *Code) { c.Glyphs()[0].Object.Path.Contours[0].Start.X = math.Inf(1) },
		func(c *Code) { c.Object.Shift(geom.V(MaxCoordinate*2, 0)) },
		func(c *Code) { c.Glyphs()[0].Object.Path.Contours = make([]path.Contour, DefaultMaxCurves+1) },
	} {
		c := mustCode(t, "x", DefaultOptions())
		edit(c)
		if q, e := c.Copy(); q != nil || e == nil {
			t.Fatal("bad authored tree copied")
		}
	}
}

func TestCodeConcurrentSnapshotsAndFontOwnership(t *testing.T) {
	o := DefaultOptions()
	o.LineNumbers = true
	o.Background = true
	c := mustCode(t, "package p\n\nfunc f() int {\n\treturn 42 // α\n}\n", o)
	frozen, e := c.Snapshot()
	if e != nil {
		t.Fatal(e)
	}
	reference, e := frozen.Copy()
	if e != nil {
		t.Fatal(e)
	}
	before := reference.Glyphs()[0].Object.Path.Clone()
	var wg sync.WaitGroup
	for worker := range 8 {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for range 12 {
				q, e := frozen.Copy()
				if e != nil {
					t.Error(e)
					return
				}
				if !reflect.DeepEqual(q.Glyphs()[0].Object.Path, before) {
					t.Error("snapshot modified")
					return
				}
				q.Object.Shift(geom.V(float64(worker), 1))
				q.Glyphs()[0].Object.Path.Contours[0].Curves[0].P1.X += 20
				if _, e := New("var α = 12\n", o); e != nil {
					t.Error(e)
					return
				}
			}
		}(worker)
	}
	for range 40 {
		c.Object.Shift(geom.V(.01, 0))
	}
	wg.Wait()
	q, e := frozen.Copy()
	if e != nil || !reflect.DeepEqual(q.Glyphs()[0].Object.Path, before) {
		t.Fatal("original edits reached snapshot", e)
	}
	copy, e := q.Copy()
	if e != nil {
		t.Fatal(e)
	}
	if e := gomath.NewScene().Add(q.Object, copy.Object); e != nil {
		t.Fatal("fresh copy IDs", e)
	}
	for i, g := range q.Glyphs() {
		if copy.Glyphs()[i].Object == g.Object || copy.Glyphs()[i].Object.ID == g.Object.ID {
			t.Fatal("copied glyph identity alias")
		}
	}
}

func BenchmarkCodeNewGo100Lines(b *testing.B) {
	source := "package benchmark\n" + strings.Repeat("var value = 123 // bounded real outlines\n", 100)
	o := DefaultOptions()
	o.LineNumbers = true
	o.Background = true
	b.ReportAllocs()
	b.SetBytes(int64(len(source)))
	b.ResetTimer()
	for b.Loop() {
		if _, e := New(source, o); e != nil {
			b.Fatal(e)
		}
	}
}

func TestCodeHardBoundariesAndAllLexicalCategories(t *testing.T) {
	o := DefaultOptions()
	mustCode(t, strings.Repeat("\n", MaxLines-1), o)
	mustCode(t, strings.Repeat(" ", MaxColumns), o)
	// One byte of whitespace per one-byte token, with physical breaks rather
	// than extra bytes, reaches rune/token/glyph ceilings simultaneously.
	row := strings.Repeat("+ ", 127) + "+\n"
	c := mustCode(t, strings.Repeat(row, 128), o)
	if len(c.Tokens()) != MaxTokens || len(c.Glyphs()) != MaxGlyphs || len([]rune(c.Text())) != MaxRunes {
		t.Fatal("hard ceilings were not exercised")
	}
	keywords := "break default func interface select case defer go map struct chan else goto package switch const fallthrough if range type continue for import return var"
	c = mustCode(t, keywords, o)
	for _, tok := range c.Tokens() {
		if tok.Category != Keyword {
			t.Fatal(tok)
		}
	}
	c = mustCode(t, "+ - * / % & | ^ << >> &^ += -= *= /= %= &= |= ^= <<= >>= &^= && || <- ++ -- == < > = ! != <= >= := ... ~", o)
	for _, tok := range c.Tokens() {
		if tok.Category != Operator {
			t.Fatal(tok)
		}
	}
	c = mustCode(t, "( ) [ ] { } , . ; :", o)
	for _, tok := range c.Tokens() {
		if tok.Category != Punctuation {
			t.Fatal(tok)
		}
	}
	c = mustCode(t, "// "+strings.Repeat("(", MaxDepth+1), o)
	if len(c.Tokens()) != 1 || c.Tokens()[0].Category != Comment {
		t.Fatal("comment delimiters counted as nesting")
	}
}

func FuzzCodeCheckedSource(f *testing.F) {
	for _, seed := range []string{"", "var x = 1\n", "//line x:99\r\n\tα := `a\r\nb`", "/*\r\n*/", "\xff", "\x00", "\u202e", "\"\\q\"", "((("} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, source string) {
		if len(source) > 512 {
			t.Skip()
		}
		o := DefaultOptions()
		o.LineNumbers, o.Background = true, true
		o.Limits = Limits{SourceBytes: 512, Runes: 256, Lines: 16, Columns: 64, Tokens: 128, Depth: 16, Glyphs: 128, Curves: 4096}
		c, err := New(source, o)
		if err != nil {
			if c != nil {
				t.Fatal("partially published failed construction")
			}
			return
		}
		if c == nil || c.Text() != source || c.Metrics().Glyphs > 128 || c.Metrics().Curves > 4096 {
			t.Fatal("successful result violated source/budgets")
		}
		if err := c.Object.Validate(); err != nil {
			t.Fatal(err)
		}
		if _, err := c.Copy(); err != nil {
			t.Fatal(err)
		}
		for _, tok := range c.Tokens() {
			if tok.Start < 0 || tok.End > len(source) || tok.Start >= tok.End || tok.Text != source[tok.Start:tok.End] {
				t.Fatal("invalid original byte span")
			}
		}
	})
}

func BenchmarkCodeSnapshotCopy100Lines(b *testing.B) {
	c := mustCode(b, "package benchmark\n"+strings.Repeat("var value = 123 // bounded real outlines\n", 100), DefaultOptions())
	s, e := c.Snapshot()
	if e != nil {
		b.Fatal(e)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, e := s.Copy(); e != nil {
			b.Fatal(e)
		}
	}
}
