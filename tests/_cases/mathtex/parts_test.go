package mathtex

import (
	"gomath"
	"gomath/geom"
	"gomath/path"
	"gomath/text"
	"math"
	"reflect"
	"strings"
	"sync"
	"testing"

	"golang.org/x/image/font/gofont/goregular"
)

func partsMust(t *testing.T, keys ...string) *Parts {
	t.Helper()
	p, err := NewParts(keys, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func partsSameGeometry(t *testing.T, a, b *gomath.Object) {
	t.Helper()
	var aa, bb []*gomath.Object
	a.Walk(func(o *gomath.Object) { aa = append(aa, o) })
	b.Walk(func(o *gomath.Object) { bb = append(bb, o) })
	if len(aa) != len(bb) {
		t.Fatal("tree size mismatch", len(aa), len(bb))
	}
	for i := range aa {
		if !reflect.DeepEqual(aa[i].Path, bb[i].Path) || !reflect.DeepEqual(aa[i].Style, bb[i].Style) {
			t.Fatal("geometry/style mismatch", i)
		}
	}
}
func partsNear(t *testing.T, a, b float64) {
	t.Helper()
	if math.Abs(a-b) > 1e-10*math.Max(1, math.Max(math.Abs(a), math.Abs(b))) {
		t.Fatal("metric mismatch", a, b)
	}
}

func TestPartsLayoutExactKeysBaselinesAdvancesAndStyles(t *testing.T) {
	keys := []string{" n ", "n", "", "{}", `\quad`, `\hphantom{W}`, `\vphantom{g}`, `\phantom{\frac{n}{g}}`, `\color{red}\mathbf{n}`, "g", `\displaystyle\sum_i^n`, `\sum_i^n`, `\begin{array}{c}g\\n\end{array}`}
	p, metrics, err := LayoutParts(keys, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(keys, p.Keys()) || p.Len() != len(keys) || metrics != p.Metrics() {
		t.Fatal("keys/metrics lost")
	}
	x, height, depth := 0.0, 0.0, 0.0
	for i, source := range keys {
		b, err := layout(source, DefaultOptions())
		if err != nil {
			t.Fatal(err)
		}
		info, ok := p.Info(i)
		o, exists := p.Part(i)
		if !ok || !exists || o != p.Object().Children[i] || info.Source != source || info.Index != i || info.ObjectID != o.ID || o.Name != source {
			t.Fatal("fragment identity/key mismatch", i)
		}
		partsNear(t, info.X, x)
		partsNear(t, info.Advance, b.width)
		partsNear(t, info.Height, b.height)
		partsNear(t, info.Depth, b.depth)
		b.object.Shift(geom.V(x, 0))
		partsSameGeometry(t, o, b.object)
		x += b.width
		height = math.Max(height, b.height)
		depth = math.Max(depth, b.depth)
	}
	partsNear(t, metrics.Advance, x)
	partsNear(t, metrics.Height, height)
	partsNear(t, metrics.Depth, depth)
	keys[0] = "changed"
	p.Keys()[1] = "changed"
	if p.Keys()[0] != " n " || p.Keys()[1] != "n" || p.Select("n").Len() != 1 || p.Select(" n ").Len() != 1 || p.Select("missing").Len() != 0 {
		t.Fatal("keys alias, normalize or use substring matching")
	}
	p.Object().Children[1].Name = "renamed during authoring"
	if p.Select("n").Len() != 1 || p.Select("renamed during authoring").Len() != 0 {
		t.Fatal("mutable object name changed exact source key")
	}
	for _, i := range []int{-1, p.Len()} {
		if o, ok := p.Part(i); ok || o != nil {
			t.Fatal("bad part index")
		}
		if _, ok := p.Info(i); ok {
			t.Fatal("bad info index")
		}
	}
	blank := partsMust(t, "", " ", `{}`, `\quad`, `\phantom{g}`, `\hphantom{n}`, `\vphantom{W}`)
	if blank.Object().Bounds().Valid || blank.Len() != 7 || blank.Metrics().Advance <= 0 || blank.Metrics().Height <= 0 || blank.Metrics().Depth <= 0 {
		t.Fatal("zero-ink fragments lost semantic dimensions")
	}
	// This deliberately differs from the ordinary row's .04em atom gap.
	split := partsMust(t, "n", "g")
	joined, err := layout("ng", DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	partsNear(t, joined.width-split.Metrics().Advance, .04*DefaultOptions().Size)
}

func TestPartsStableSelectionsCopiesSnapshotsAndTransforms(t *testing.T) {
	p := partsMust(t, "n", "g", "n", `\phantom{W}`)
	sel := p.Select("n")
	handles := sel.Objects()
	if sel.Len() != 2 || handles[0] != p.Object().Children[0] || handles[1] != p.Object().Children[2] {
		t.Fatal("selection order")
	}
	sel.Objects()[0] = nil
	if sel.Objects()[0] != handles[0] {
		t.Fatal("selection slice alias")
	}
	frozen, err := p.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	before := frozen.Object()
	copy, err := p.Copy()
	if err != nil {
		t.Fatal(err)
	}
	selectedCopy, err := sel.Copy()
	if err != nil {
		t.Fatal(err)
	}
	all := gomath.Group(p.Object(), copy.Object(), selectedCopy)
	if err := all.Validate(); err != nil {
		t.Fatal("copy did not receive independent IDs/ownership", err)
	}
	copy.Object().Shift(geom.V(8, 9)).SetColor(gomath.Red)
	selectedCopy.Shift(geom.V(-8, 9))
	scene := gomath.NewScene()
	scene.Add(p.Object())
	target := handles[0].Copy().Shift(geom.V(.7, .3)).SetColor(gomath.Blue)
	if err := scene.Play(1, gomath.Transform(handles[0], target).WithRate(gomath.Linear)); err != nil {
		t.Fatal(err)
	}
	if sel.Objects()[0] != handles[0] || p.Select("n").Objects()[0] != handles[0] {
		t.Fatal("Transform invalidated live handles")
	}
	for _, time := range []float64{.35, .7} {
		frame, err := scene.Sample(time)
		if err != nil {
			t.Fatal(err)
		}
		got := frame.Find(handles[0].ID)
		if got == nil {
			t.Fatal("sample lost fragment ID")
		}
		want, _ := frozen.Part(0)
		want.Shift(geom.V(.7*time, .3*time))
		partsNear(t, got.Bounds().Min.X, want.Bounds().Min.X)
		partsNear(t, got.Bounds().Min.Y, want.Bounds().Min.Y)
		untouched, _ := frozen.Part(2)
		partsSameGeometry(t, frame.Find(handles[1].ID), untouched)
	}
	for _, item := range frozen.Select("n").Objects() {
		item.Shift(geom.V(100, 100)).SetColor(gomath.Black)
	}
	frozen.Object().Children = nil
	frozen.Keys()[0] = "bad"
	part, ok := frozen.Part(0)
	if !ok || part.ID != handles[0].ID {
		t.Fatal("snapshot lost captured IDs")
	}
	info, _ := frozen.Info(0)
	if info.ObjectID != part.ID || info.Source != "n" || frozen.Metrics() != p.Metrics() {
		t.Fatal("snapshot metadata")
	}
	if _, ok := frozen.Part(-1); ok {
		t.Fatal("snapshot index")
	}
	if _, ok := frozen.Info(4); ok {
		t.Fatal("snapshot info index")
	}
	partsSameGeometry(t, frozen.Object(), before)
	// Identity lookup in an independently sampled frame can seed future matching.
	rootTarget := p.Object().Copy().Shift(geom.V(-.2, .6))
	if err := scene.Play(1, gomath.Transform(p.Object(), rootTarget).WithRate(gomath.Linear)); err != nil {
		t.Fatal(err)
	}
	if p.Select("n").Objects()[1] != handles[1] {
		t.Fatal("parent Transform invalidated handles")
	}
	partsSameGeometry(t, frozen.Object(), before)
	if _, err := p.Snapshot(); err != nil {
		t.Fatal("valid transformed authoring rejected", err)
	}
	if empty, err := p.Select("absent").Copy(); err != nil || len(empty.Children) != 0 {
		t.Fatal("empty selection copy", err)
	}
}

func TestPartsConcurrentImmutableSnapshotsAndIndependentSampling(t *testing.T) {
	p := partsMust(t, `\frac{n}{g}`, "n", `\phantom{W}`, "n")
	s, err := p.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	original := s.Object()
	scene := gomath.NewScene()
	scene.Add(p.Object())
	if err := scene.Play(1, gomath.Transform(p.Object(), p.Object().Copy().Shift(geom.V(.4, .5))).WithRate(gomath.Linear)); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 12 {
				got := s.Object()
				if got.Bounds() != original.Bounds() {
					t.Error("snapshot drift")
				}
				got.Shift(geom.V(20, 30)).SetColor(gomath.Red)
				for _, o := range s.Select("n").Objects() {
					o.Path = path.Path{}
					o.Children = nil
				}
				if _, err := p.Snapshot(); err != nil {
					t.Error(err)
				}
				f, err := scene.Sample(.35)
				if err != nil {
					t.Error(err)
					return
				}
				f.Find(p.Object().ID).Shift(geom.V(50, 60))
			}
		}()
	}
	wg.Wait()
	partsSameGeometry(t, s.Object(), original)
}

func TestPartsAtomicInvalidFragmentsOptionsAndAggregateRunes(t *testing.T) {
	for _, keys := range [][]string{nil, {}, make([]string, MaxParts+1), {"n", `\frac{n}`, "g"}, {`\frac{`, "n}{g}"}, {`\left(`, "n", `\right)`}, {"n", "_i"}, {"n", string([]byte{0xff})}, {"n", `\unknown`}, {strings.Repeat(" ", MaxSourceRunes), " "}, {strings.Repeat("界", MaxSourceRunes), "n"}} {
		p, m, err := LayoutParts(keys, DefaultOptions())
		if err == nil || p != nil || m != (PartsMetrics{}) {
			t.Fatal("partial result for invalid input", len(keys), err)
		}
	}
	for _, keys := range [][]string{make([]string, MaxParts), {strings.Repeat(" ", MaxSourceRunes-1), " "}} {
		if _, err := NewParts(keys, DefaultOptions()); err != nil {
			t.Fatal("exact source/part boundary", err)
		}
	}
	for _, size := range []float64{0, -1, math.NaN(), math.Inf(1), 1e6 + 1} {
		o := DefaultOptions()
		o.Size = size
		if p, err := NewParts([]string{"", "n"}, o); err == nil || p != nil {
			t.Fatal("invalid size", size)
		}
	}
	custom, err := text.Parse(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"font", "style", "color", "variant", "hidden-style"} {
		o := DefaultOptions()
		keys := []string{"n", ""}
		switch kind {
		case "font":
			o.Font = nil
		case "style":
			o.Style = 255
		case "color":
			o.Color.R = math.NaN()
		case "variant":
			o.BoldFont = &text.Font{}
		case "hidden-style":
			o.Font = custom
			keys[1] = `\phantom{\mathbf{n}}`
		}
		if p, err := NewParts(keys, o); err == nil || p != nil {
			t.Fatal("invalid option", kind)
		}
	}
	for _, style := range []text.FontStyle{text.Regular, text.Bold, text.Italic, text.BoldItalic} {
		o := DefaultOptions()
		o.Style = style
		if _, err := NewParts([]string{"", "n", `\mathrm{g}`}, o); err != nil {
			t.Fatal("font face", style, err)
		}
	}
	valid := partsMust(t, "n")
	before := valid.Object().Copy()
	if _, err := NewParts([]string{"n", `\frac`}, DefaultOptions()); err == nil {
		t.Fatal("expected error")
	}
	partsSameGeometry(t, valid.Object(), before)
}

func partsBuilderForTest() partsBuilder {
	b := partsBuilder{parser: parser{options: DefaultOptions(), style: text.Regular}}
	b.parser.beginOperandAudit()
	return b
}
func TestPartsSharedCurvesHiddenInkStructuralPathsAndCells(t *testing.T) {
	for _, source := range []string{"g", `\frac{}{}`, `\sqrt{}`, `\boxed{\frac{n}{g}}`, `\phantom{\sqrt{g}}`, `\phantom{\frac{n}{g}}`} {
		measure := partsBuilderForTest()
		if err := measure.append(0, source); err != nil {
			t.Fatal(err)
		}
		cost := measure.parser.curves
		if cost <= 0 {
			t.Fatal("operand not charged", source)
		}
		for _, over := range []int{0, 1} {
			b := partsBuilderForTest()
			b.parser.curves = MaxLayoutCurves - 2*cost + over
			if err := b.append(0, source); err != nil {
				t.Fatal("first bounded fragment", err)
			}
			err := b.append(1, source)
			if (err != nil) != (over == 1) {
				t.Fatal("cross-fragment curve reset/double charge", source, over, b.parser.curves, err)
			}
			if over == 0 && b.parser.curves != MaxLayoutCurves {
				t.Fatal("wrong cumulative curve cost")
			}
		}
	}
	// Actual external input: each fragment is below every individual parser cap.
	row := strings.TrimSuffix(strings.Repeat("{}&", 64), "&")
	body := strings.TrimSuffix(strings.Repeat(row+`\\`, 32), `\\`)
	matrix := `\begin{matrix}` + body + `\end{matrix}`
	if _, err := NewParts([]string{matrix, matrix}, DefaultOptions()); err != nil {
		t.Fatal("4096 total cells", err)
	}
	if p, err := NewParts([]string{matrix, matrix, `\phantom{\substack{a}}`}, DefaultOptions()); err == nil || p != nil || !strings.Contains(err.Error(), "cell") {
		t.Fatal("split bypassed shared cell budget", err)
	}
	nested := strings.Repeat("{", MaxNesting-1) + "n" + strings.Repeat("}", MaxNesting-1)
	if _, err := NewParts([]string{"n", nested}, DefaultOptions()); err != nil {
		t.Fatal("exact depth", err)
	}
	if p, err := NewParts([]string{"n", "{" + nested + "}"}, DefaultOptions()); err == nil || p != nil {
		t.Fatal("fragment reset nested depth")
	}
}

func TestPartsAggregateTreeGlyphCoordinateBoundsAndInvalidOwnership(t *testing.T) {
	measure := partsBuilderForTest()
	if err := measure.append(0, "n"); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"nodes", "glyphs", "curves"} {
		for _, over := range []int{0, 1} {
			b := partsBuilderForTest()
			switch kind {
			case "nodes":
				b.budget.nodes = MaxPartsNodes - measure.budget.nodes + over
			case "glyphs":
				b.budget.glyphs = MaxPartsGlyphs - measure.budget.glyphs + over
			case "curves":
				b.budget.curves = MaxLayoutCurves - measure.budget.curves + over
			}
			if err := b.append(0, "n"); (err != nil) != (over == 1) {
				t.Fatal("aggregate budget", kind, over, err)
			}
		}
	}
	// Overall advance fails although each separately valid fragment fits.
	o := DefaultOptions()
	o.Size = 1e6
	space := strings.Repeat(`\qquad`, 300)
	if _, err := NewParts([]string{space}, o); err != nil {
		t.Fatal("single under coordinate limit", err)
	}
	if p, err := NewParts([]string{space, space}, o); err == nil || p != nil {
		t.Fatal("aggregate advance bypassed coordinate cap")
	}
	// Reach the public source/glyph limit across every available handle.
	keys := make([]string, MaxParts)
	for i := range keys {
		keys[i] = strings.Repeat("n", MaxSourceRunes/MaxParts)
	}
	if p, err := NewParts(keys, DefaultOptions()); err != nil || p == nil {
		t.Fatal("exact split glyph/source limit", err)
	}
	for _, over := range []int{0, 1} {
		p := partsMust(t, "")
		root := p.parts[0]
		for range MaxPartsTreeDepth - 2 + over {
			child := gomath.Group()
			root.Children = []*gomath.Object{child}
			root = child
		}
		if _, err := p.Snapshot(); (err != nil) != (over == 1) {
			t.Fatal("authored exact tree depth limit", over, err)
		}
	}
	for _, kind := range []string{"cycle", "detached", "duplicate", "id", "coordinate", "depth", "nil", "order"} {
		p := partsMust(t, "n", "g")
		switch kind {
		case "cycle":
			p.parts[0].Children = append(p.parts[0].Children, p.object)
		case "detached":
			p.object.Children[0] = p.parts[0].Copy()
		case "duplicate":
			p.parts[0].Children = append(p.parts[0].Children, p.parts[0].Children[0])
		case "id":
			p.parts[0].ID = 0
		case "coordinate":
			p.parts[0].Shift(geom.V(MaxPartsCoordinate+1, 0))
		case "depth":
			root := p.parts[0]
			for range MaxPartsTreeDepth {
				q := gomath.Group()
				root.Children = []*gomath.Object{q}
				root = q
			}
		case "nil":
			p.parts[0].Children = append(p.parts[0].Children, nil)
		case "order":
			p.object.Children[0], p.object.Children[1] = p.object.Children[1], p.object.Children[0]
		}
		if s, err := p.Snapshot(); err == nil || s != nil {
			t.Fatal("invalid ownership snapshot escaped", kind)
		}
		if q, err := p.Copy(); err == nil || q != nil {
			t.Fatal("invalid ownership copy escaped", kind)
		}
	}
	var nilParts *Parts
	if _, err := nilParts.Snapshot(); err == nil {
		t.Fatal("nil authoring snapshot")
	}
}

func BenchmarkPartsLayout(b *testing.B) {
	keys := []string{`\mathbf{n}`, "=", `\frac{n}{g}`, `\quad`, `\phantom{W}`, `\begin{array}{|c|}\hline n\\g\\\hline\end{array}`}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := NewParts(keys, DefaultOptions()); err != nil {
			b.Fatal(err)
		}
	}
}

func TestPartsAuthoredContourAndChildBudgetsBeforeCopy(t *testing.T) {
	p, e := NewParts([]string{"x"}, DefaultOptions())
	if e != nil {
		t.Fatal(e)
	}
	part, _ := p.Part(0)
	part.Path.Contours = make([]path.Contour, MaxLayoutCurves+1)
	if q, e := p.Copy(); e == nil || q != nil {
		t.Fatal("zero-curve contour payload bypassed copy budget")
	}
	p, e = NewParts([]string{"x"}, DefaultOptions())
	if e != nil {
		t.Fatal(e)
	}
	part, _ = p.Part(0)
	part.Children = make([]*gomath.Object, MaxPartsNodes+1)
	if q, e := p.Snapshot(); e == nil || q != nil {
		t.Fatal("oversized child payload bypassed snapshot preflight")
	}
}
