package number

import (
	"encoding/json"
	"gomath"
	"gomath/geom"
	"gomath/path"
	"gomath/text"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func root(t *testing.T) string {
	t.Helper()
	p, e := os.Getwd()
	if e != nil {
		t.Fatal(e)
	}
	for {
		if _, e := os.Stat(filepath.Join(p, "go.mod")); e == nil {
			return p
		}
		n := filepath.Dir(p)
		if n == p {
			t.Fatal("missing test root")
		}
		p = n
	}
}
func data(t *testing.T, name string) []byte {
	t.Helper()
	b, e := os.ReadFile(filepath.Join(root(t), "tests/data/number", name))
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func near(t *testing.T, a, b float64) {
	t.Helper()
	if math.Abs(a-b) > 2e-10 {
		t.Fatalf("got %.17g want %.17g", a, b)
	}
}
func samePoint(t *testing.T, a, b geom.Vec3) {
	t.Helper()
	near(t, a.X, b.X)
	near(t, a.Y, b.Y)
	near(t, a.Z, b.Z)
}
func mustNew(t *testing.T, v float64, o Options) *DecimalNumber {
	t.Helper()
	n, e := New(v, o)
	if e != nil {
		t.Fatal(e)
	}
	return n
}
func encode(t *testing.T, v any) string {
	t.Helper()
	b, e := json.Marshal(v)
	if e != nil {
		t.Fatal(e)
	}
	return string(b)
}
func geometry(t *testing.T, o *gomath.Object) string {
	t.Helper()
	type leaf struct {
		Path  path.Path
		Style gomath.Style
		Z     int
	}
	ls := []leaf{}
	o.Walk(func(o *gomath.Object) {
		if len(o.Path.Contours) > 0 {
			ls = append(ls, leaf{o.Path, o.Style, o.ZIndex})
		}
	})
	return encode(t, ls)
}
func assertAtomic(t *testing.T, n *DecimalNumber, f func() error) {
	t.Helper()
	before, old, txt := encode(t, n.Object), math.Float64bits(n.Value()), n.Text()
	if e := f(); e == nil {
		t.Fatal("expected rejected operation")
	}
	if before != encode(t, n.Object) || old != math.Float64bits(n.Value()) || txt != n.Text() {
		t.Fatal("failed operation mutated authoring state")
	}
}

func TestNumberPublicFormattingAndBounds(t *testing.T) {
	o := DefaultOptions()
	o.IncludeSign = true
	o.Unit = "kg"
	o.Size = .8
	n := mustNew(t, -1234.5, o)
	if n.Text() != "-1,234.50 kg" || n.Value() != -1234.5 {
		t.Fatal(n.Text(), n.Value())
	}
	if p, e := n.AnchorPoint(); e != nil || p.Norm() > 1e-12 {
		t.Fatal("initial baseline anchor", p, e)
	}
	if len(n.Object.Children) != len([]rune(n.Text())) {
		t.Fatal("not one slot per rune")
	}
	f := text.DefaultFont()
	to := text.DefaultOptions()
	to.Size = o.Size
	to.Color = o.Color
	want, m, e := text.Layout(n.Text(), f, to)
	if e != nil {
		t.Fatal(e)
	}
	want.Shift(geom.V(-m.Width, 0))
	gotPoints, wantPoints := []geom.Vec3{}, []geom.Vec3{}
	n.Object.Walk(func(n *gomath.Object) {
		visit([]path.Path{n.Path}, func(_ pointRef, p geom.Vec3) { gotPoints = append(gotPoints, p) })
	})
	want.Walk(func(n *gomath.Object) {
		visit([]path.Path{n.Path}, func(_ pointRef, p geom.Vec3) { wantPoints = append(wantPoints, p) })
	})
	if len(gotPoints) != len(wantPoints) {
		t.Fatal("number changed real text topology")
	}
	for i := range gotPoints {
		samePoint(t, gotPoints[i], wantPoints[i])
	}
	samePoint(t, n.Object.Bounds().Min, want.Bounds().Min)
	samePoint(t, n.Object.Bounds().Max, want.Bounds().Max)
	for _, v := range []float64{999.995, math.SmallestNonzeroFloat64, -math.SmallestNonzeroFloat64, math.MaxFloat64, -math.MaxFloat64} {
		if e := n.SetValue(v); e != nil {
			t.Fatal("finite binary64 endpoint", v, e)
		}
		if math.Float64bits(n.Value()) != math.Float64bits(v) {
			t.Fatal("input value lost")
		}
	}
	if e := n.SetValue(math.Copysign(0, -1)); e != nil {
		t.Fatal(e)
	}
	if n.Text() != "+0.00 kg" || !math.Signbit(n.Value()) {
		t.Fatal("negative-zero display/value contract")
	}
}

func TestIntegerFullRangeAndIdentity(t *testing.T) {
	o := DefaultIntegerOptions()
	o.GroupThousands = false
	n, e := NewInteger(math.MaxInt64, o)
	if e != nil {
		t.Fatal(e)
	}
	if n.Text() != "9223372036854775807" || n.Value() != math.MaxInt64 {
		t.Fatal("int64 narrowed")
	}
	root := n.Object
	last := root.Children[len(root.Children)-1]
	for _, v := range []int64{1, 9007199254740993, math.MinInt64, 0, math.MaxInt64} {
		if e := n.SetValue(v); e != nil {
			t.Fatal(e)
		}
		if n.Value() != v || root != n.Object || last != root.Children[len(root.Children)-1] {
			t.Fatal("integer identity/value")
		}
	}
	if n, e := NewInteger(1, DefaultOptions()); e == nil || n != nil {
		t.Fatal("integer silently ignored decimal precision")
	}
}

func TestNumberRightSlotsGrowthShrinkAndStyles(t *testing.T) {
	o := DefaultOptions()
	o.GroupThousands = false
	n := mustNew(t, 9.5, o)
	before := append([]*gomath.Object(nil), n.Object.Children...)
	root := n.Object
	id := root.ID
	before[0].SetFill(gomath.Red)
	before[len(before)-1].SetFill(gomath.Blue).SetOpacity(.7)
	root.SetOpacity(.6)
	root.ZIndex = 4
	if e := n.SetValue(12345.5); e != nil {
		t.Fatal(e)
	}
	if root != n.Object || id != root.ID || root.Style.Opacity != .6 || root.ZIndex != 4 {
		t.Fatal("root changed")
	}
	shift := len(root.Children) - len(before)
	for i, g := range before {
		if root.Children[i+shift] != g {
			t.Fatal("existing right slot replaced")
		}
	}
	for _, g := range root.Children[:shift] {
		if g.Style.Fill != gomath.Red {
			t.Fatal("new left slots did not inherit left appearance")
		}
	}
	removed := root.Children[0]
	removedID := removed.ID
	removedJSON := encode(t, removed)
	if e := n.SetValue(1.5); e != nil {
		t.Fatal(e)
	}
	if encode(t, removed) != removedJSON {
		t.Fatal("detached slot was cleared")
	}
	last := root.Children[len(root.Children)-1]
	if last != before[len(before)-1] || last.Style.Fill != gomath.Blue || last.Style.Opacity != .7 {
		t.Fatal("slot appearance lost")
	}
	if e := n.SetValue(98765.5); e != nil {
		t.Fatal(e)
	}
	for _, g := range root.Children {
		if g == removed || g.ID == removedID {
			t.Fatal("detached slot recycled")
		}
	}
	if e := root.Validate(); e != nil {
		t.Fatal(e)
	}
}

func TestNumberAffineAnchorsAndImmutableSource(t *testing.T) {
	for _, anchor := range []Alignment{Right, Left, Center} {
		o := DefaultOptions()
		o.Anchor = anchor
		o.Unit = "kg"
		o.Style = text.BoldItalic
		n := mustNew(t, 12.5, o)
		m := geom.Identity()
		m[0], m[1], m[4], m[5], m[8], m[9], m[3], m[7], m[11] = 1.2, .35, -.2, .8, .1, .2, 2, -1, .4
		n.Object.ApplyMatrix(m).Rotate(.4).Scale(1.7).Shift(geom.V(-.2, .7))
		p, e := n.AnchorPoint()
		if e != nil {
			t.Fatal(e)
		}
		source, e := n.Source()
		if e != nil {
			t.Fatal(e)
		}
		expected, e := source.At(-98765.125)
		if e != nil {
			t.Fatal(e)
		}
		for range 4 {
			if e := n.SetValue(-98765.125); e != nil {
				t.Fatal(e)
			}
			p2, e := n.AnchorPoint()
			if e != nil {
				t.Fatal(e)
			}
			samePoint(t, p, p2)
		}
		// Compare actual control points with the immutable affine factory result.
		a, b := []geom.Vec3{}, []geom.Vec3{}
		n.Object.Walk(func(o *gomath.Object) { visit([]path.Path{o.Path}, func(_ pointRef, p geom.Vec3) { a = append(a, p) }) })
		expected.Walk(func(o *gomath.Object) { visit([]path.Path{o.Path}, func(_ pointRef, p geom.Vec3) { b = append(b, p) }) })
		if len(a) != len(b) {
			t.Fatal("source geometry mismatch")
		}
		for i := range a {
			samePoint(t, a[i], b[i])
		}
		stable := geometry(t, expected)
		n.Object.SetFill(gomath.Red).Shift(geom.V(10, 10))
		if e := n.SetValue(1); e != nil {
			t.Fatal(e)
		}
		again, e := source.At(-98765.125)
		if e != nil || geometry(t, again) != stable {
			t.Fatal("source retained live authoring data", e)
		}
		again.Shift(geom.V(20, 20)).SetFill(gomath.Green)
		once, e := source.At(-98765.125)
		if e != nil || geometry(t, once) != stable {
			t.Fatal("source outputs shared paths/styles", e)
		}
	}
}

func TestNumberLateFontFailureAndLimitsAreAtomic(t *testing.T) {
	bytes := data(t, "restricted.ttf")
	f, e := text.Parse(bytes)
	if e != nil {
		t.Fatal(e)
	}
	o := DefaultOptions()
	o.Font = f
	o.GroupThousands = false
	o.DecimalPlaces = 0
	n := mustNew(t, 12, o)
	for i := range bytes {
		bytes[i] = 0
	} // Font owns source bytes.
	assertAtomic(t, n, func() error { return n.SetValue(9) })
	assertAtomic(t, n, func() error { return n.SetValue(-1) })
	for _, v := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		assertAtomic(t, n, func() error { return n.SetValue(v) })
	}
	o = DefaultOptions()
	o.MaxGlyphs = 4
	n = mustNew(t, 1, o)
	assertAtomic(t, n, func() error { return n.SetValue(1000) })
	o = DefaultOptions()
	o.DecimalPlaces = 0
	o.MaxCurves = 10
	// '1' has fewer curves than '8': test the actual local work boundary.
	n = mustNew(t, 1, o)
	assertAtomic(t, n, func() error { return n.SetValue(8) })
	for _, change := range []func(*Options){func(o *Options) { o.DecimalPlaces = -1 }, func(o *Options) { o.DecimalPlaces = 19 }, func(o *Options) { o.Size = math.NaN() }, func(o *Options) { o.Size = 1e-13 }, func(o *Options) { o.Size = 1e7 }, func(o *Options) { o.Font = &text.Font{} }, func(o *Options) { o.Font = text.DefaultFont(); o.Style = text.Bold }, func(o *Options) { o.Style = 99 }, func(o *Options) { o.Anchor = 99 }, func(o *Options) { o.Unit = "x\ny" }, func(o *Options) { o.Unit = "e\u0301" }, func(o *Options) { o.Unit = "\u202e" }, func(o *Options) { o.Unit = string([]byte{0xff}) }, func(o *Options) { o.Unit = strings.Repeat("x", 65) }, func(o *Options) { o.Unit = "\U0010ffff" }, func(o *Options) { o.MaxGlyphs = 513 }, func(o *Options) { o.MaxCurves = MaxCurves + 1 }, func(o *Options) { o.MaxGlyphs = -1 }, func(o *Options) { o.Color.R = math.Inf(1) }} {
		o := DefaultOptions()
		change(&o)
		if n, e := New(1, o); e == nil || n != nil {
			t.Fatal("invalid options succeeded", o)
		}
	}
	for _, size := range []float64{1e-12, 1e6} {
		o := DefaultOptions()
		o.Size = size
		if _, e := New(123.25, o); e != nil {
			t.Fatal("valid size boundary", size, e)
		}
	}
	var nd *DecimalNumber
	var ni *Integer
	if nd.SetValue(1) == nil || ni.SetValue(1) == nil {
		t.Fatal("nil author accepted")
	}
	if _, e := (*DecimalSource)(nil).At(1); e == nil {
		t.Fatal("nil source accepted")
	}
}

func TestNumberRejectsModifiedTopologyAndNonAffinePaths(t *testing.T) {
	for _, edit := range []func(*DecimalNumber){
		func(n *DecimalNumber) { n.Object.ID++ }, func(n *DecimalNumber) { n.Object.Children[0].ID++ }, func(n *DecimalNumber) { n.Object.Children = append(n.Object.Children, gomath.Group()) },
		func(n *DecimalNumber) {
			n.Object.Children[0].Children = append(n.Object.Children[0].Children, gomath.Group())
		},
		func(n *DecimalNumber) { n.Object.Children[0].Path.Contours[0].Curves[0].P1.X += .01 },
		func(n *DecimalNumber) { n.Object.Scale(0) }, func(n *DecimalNumber) { n.Object.ApplyMatrix(geom.Scaling(1, 1e-14, 1)) },
		func(n *DecimalNumber) { n.Object.Shift(geom.V(2e12, 0)) },
	} {
		n := mustNew(t, 123.5, DefaultOptions())
		edit(n)
		assertAtomic(t, n, func() error { return n.SetValue(14) })
		if s, e := n.Source(); e == nil || s != nil {
			t.Fatal("invalid source snapshot succeeded")
		}
	}
}

func TestNumberSourceConcurrentAndIntegerSnapshot(t *testing.T) {
	n := mustNew(t, 1, DefaultOptions())
	source, e := n.Source()
	if e != nil {
		t.Fatal(e)
	}
	base, e := source.At(-1234.5)
	if e != nil {
		t.Fatal(e)
	}
	want := geometry(t, base)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 24 {
				a, e := source.At(-1234.5)
				if e != nil {
					t.Error(e)
					return
				}
				if geometry(t, a) != want {
					t.Error("concurrent factory changed")
				}
				a.Shift(geom.V(10, 20)).SetFill(gomath.Red)
			}
		}()
	}
	wg.Wait()
	i, e := NewInteger(math.MaxInt64, DefaultIntegerOptions())
	if e != nil {
		t.Fatal(e)
	}
	s, e := i.Source()
	if e != nil {
		t.Fatal(e)
	}
	a, e := s.At(math.MinInt64)
	if e != nil {
		t.Fatal(e)
	}
	g := geometry(t, a)
	i.Object.Shift(geom.V(30, 40))
	if e := i.SetValue(2); e != nil {
		t.Fatal(e)
	}
	b, e := s.At(math.MinInt64)
	if e != nil || geometry(t, b) != g {
		t.Fatal("integer snapshot shared", e)
	}
}

func TestNumberSlotLedgerSurvivesUpdatesAndPublicSliceEdits(t *testing.T) {
	for _, replace := range []func(*gomath.Object){
		func(o *gomath.Object) { o.Children[0] = nil },
		func(o *gomath.Object) {
			old := o.Children[0]
			forged := old.Copy()
			forged.ID = old.ID
			o.Children[0] = forged
		},
		func(o *gomath.Object) { o.Children[0], o.Children[1] = o.Children[1], o.Children[0] },
	} {
		n := mustNew(t, 1, DefaultOptions())
		if e := n.SetValue(1234); e != nil {
			t.Fatal(e)
		}
		replace(n.Object)
		assertAtomic(t, n, func() error { return n.SetValue(3) })
		if s, e := n.Source(); e == nil || s != nil {
			t.Fatal("public slot edit replaced private identity ledger")
		}
		if _, e := n.AnchorPoint(); e == nil {
			t.Fatal("anchor accepted invalid slot identity")
		}
	}
}

func TestNumberLateWorldBudgetAndReflection(t *testing.T) {
	o := DefaultOptions()
	o.Anchor = Left
	o.Size = 1e6
	o.DecimalPlaces = 0
	n := mustNew(t, 1, o)
	n.Object.Shift(geom.V(MaxWorldCoordinate-1e6, 0))
	s, e := n.Source()
	if e != nil {
		t.Fatal("valid current world frame", e)
	}
	assertAtomic(t, n, func() error { return n.SetValue(123456789) })
	if object, e := s.At(123456789); e == nil || object != nil {
		t.Fatal("source leaked over-budget future geometry")
	}
	for _, reflection := range []geom.Mat4{geom.Scaling(-1, 1, 1), geom.Scaling(1, -1, 1)} {
		n = mustNew(t, 12.5, DefaultOptions())
		n.Object.ApplyMatrix(reflection).Shift(geom.V(2, -.5))
		if e := n.SetValue(-9876.5); e != nil {
			t.Fatal("well-conditioned reflection rejected", e)
		}
		p, e := n.AnchorPoint()
		if e != nil {
			t.Fatal(e)
		}
		samePoint(t, p, geom.V(2, -.5))
	}
}

var benchmarkObject *gomath.Object

func BenchmarkNumber(b *testing.B) {
	b.Run("New", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			n, e := New(-12345.5, DefaultOptions())
			if e != nil {
				b.Fatal(e)
			}
			benchmarkObject = n.Object
		}
	})
	b.Run("SetValue", func(b *testing.B) {
		n, e := New(999, DefaultOptions())
		if e != nil {
			b.Fatal(e)
		}
		b.ReportAllocs()
		b.ResetTimer()
		for i := range b.N {
			if e := n.SetValue(999 + float64(i%200)/100); e != nil {
				b.Fatal(e)
			}
		}
		benchmarkObject = n.Object
	})
	b.Run("SourceAt", func(b *testing.B) {
		n, e := New(0, DefaultOptions())
		if e != nil {
			b.Fatal(e)
		}
		s, e := n.Source()
		if e != nil {
			b.Fatal(e)
		}
		b.ReportAllocs()
		b.ResetTimer()
		for i := range b.N {
			o, e := s.At(float64(i%2000) / 100)
			if e != nil {
				b.Fatal(e)
			}
			benchmarkObject = o
		}
	})
}
