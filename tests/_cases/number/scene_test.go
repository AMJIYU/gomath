package number

import (
	"encoding/xml"
	"gomath"
	"gomath/export"
	"gomath/geom"
	"gomath/render"
	"gomath/text"
	"image"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func displayed(o *gomath.Object) string {
	var b strings.Builder
	for _, g := range o.Children {
		b.WriteString(g.Name)
	}
	return b.String()
}
func pixels(img *image.RGBA) int {
	n := 0
	for y := range img.Bounds().Dy() {
		for x := range img.Bounds().Dx() {
			p := img.RGBAAt(x, y)
			if p.R != 0 || p.G != 0 || p.B != 0 {
				n++
			}
		}
	}
	return n
}
func numberRenderer(t *testing.T) (*render.Rasterizer, render.Config) {
	t.Helper()
	c := render.DefaultConfig()
	c.Width, c.Height, c.Workers, c.FrameWidth, c.Background = 800, 320, 1, 8, gomath.Black
	r, e := render.NewRasterizer(c)
	if e != nil {
		t.Fatal(e)
	}
	return r, c
}
func sample(t *testing.T, s *gomath.Scene, time float64) *gomath.Frame {
	t.Helper()
	f, e := s.Sample(time)
	if e != nil {
		t.Fatal(e)
	}
	return f
}

func TestNumberCustomKerningAndCurveBoundary(t *testing.T) {
	var ref struct {
		Metrics map[string]struct {
			Advance, Origin float64
			BBox            []float64
		} `json:"fixture_metrics"`
		Kern float64 `json:"fixture_AV_kern"`
	}
	decode(t, data(t, "layout.json"), &ref)
	f, e := text.Parse(data(t, "restricted.ttf"))
	if e != nil {
		t.Fatal(e)
	}
	o := DefaultOptions()
	o.Font = f
	o.Size = 1
	o.Anchor = Left
	o.DecimalPlaces = 0
	o.Unit = "AV"
	n := mustNew(t, 0, o)
	if n.Text() != "0 AV" {
		t.Fatal(n.Text())
	}
	position := ref.Metrics["0"].Advance + ref.Metrics[" "].Advance + ref.Metrics["A"].Advance + ref.Kern + ref.Metrics["V"].Origin + ref.Metrics["V"].BBox[0]
	near(t, n.Object.Children[3].Bounds().Min.X, position)
	o = DefaultOptions()
	o.DecimalPlaces = 0
	n = mustNew(t, 8, o)
	count := 0
	n.Object.Walk(func(g *gomath.Object) {
		for _, c := range g.Path.Contours {
			count += len(c.Curves)
		}
	})
	o.MaxCurves = count
	if _, e := New(8, o); e != nil {
		t.Fatal("exact curve boundary", e)
	}
	o.MaxCurves = count - 1
	if n, e := New(8, o); e == nil || n != nil {
		t.Fatal("curve boundary bypass")
	}
	o = DefaultOptions()
	o.DecimalPlaces = 18
	o.Unit = strings.Repeat("m", MaxUnitRunes)
	n = mustNew(t, -math.MaxFloat64, o)
	if len(n.Object.Children) > MaxGlyphs {
		t.Fatal("maximum binary64/precision/unit exceeded hard slots")
	}
	for _, size := range []float64{1e-12, 1e6} {
		o := DefaultOptions()
		o.Size = size
		n := mustNew(t, 12, o)
		if e := n.SetValue(14); e != nil {
			t.Fatal("valid scale boundary update", size, e)
		}
		if _, e := n.Source(); e != nil {
			t.Fatal("valid scale boundary source", size, e)
		}
	}
}

func TestNumberTrackerDynamicSnapshotsAndIndependentMidFrames(t *testing.T) {
	var refs struct {
		Frames []struct {
			Alpha, Time float64
			Bits, Text  string
		} `json:"linear_frames"`
	}
	decode(t, data(t, "formats.json"), &refs)
	if len(refs.Frames) != 5 {
		t.Fatal("missing independent fixed linear samples")
	}
	o := DefaultOptions()
	o.Size = .8
	o.Unit = "kg"
	o.Color, _ = gomath.Hex("32a0ff")
	n := mustNew(t, 0, o)
	n.Object.Shift(geom.V(2, 0))
	factory, e := n.Source()
	if e != nil {
		t.Fatal(e)
	}
	tracker, e := gomath.NewValueTracker(999.994)
	if e != nil {
		t.Fatal(e)
	}
	s := gomath.NewScene()
	if e := s.Add(n.Object, tracker.Object); e != nil {
		t.Fatal(e)
	}
	if e := s.AddDynamicRedraw(n.Object, func(f *gomath.Frame, _ float64) (*gomath.Object, error) {
		v, e := f.Value(tracker.ID)
		if e != nil {
			return nil, e
		}
		return factory.At(v)
	}, gomath.DynamicRedrawOptions{MaxObjects: MaxGlyphs + 1, MaxCurves: o.MaxCurves}); e != nil {
		t.Fatal(e)
	}
	if e := s.Play(2, gomath.ValueTo(tracker, 1000.014).WithRate(gomath.Linear)); e != nil {
		t.Fatal(e)
	}
	// Capture copies at registration/call time. No callback reads this mutable
	// authoring state, the final tracker value, or an incrementing accumulator.
	original := encode(t, n.Object)
	saved := map[float64]string{}
	r, _ := numberRenderer(t)
	for _, ref := range refs.Frames {
		frame := sample(t, s, ref.Time)
		v, e := frame.Value(tracker.ID)
		if e != nil {
			t.Fatal(e)
		}
		if math.Float64bits(v) != math.Float64bits(fromBits(t, ref.Bits)) {
			t.Fatal("independent binary64 linear sample", ref.Time, v)
		}
		num := frame.Find(n.Object.ID)
		if num == nil || displayed(num) != ref.Text {
			t.Fatal("independent intermediate display", ref.Time, displayed(num), ref.Text)
		}
		img, e := r.Render(frame)
		if e != nil || pixels(img) < 250 {
			t.Fatal("intermediate number not actually rendered", ref.Time, e)
		}
		saved[ref.Time] = encode(t, frame)
		num.SetFill(gomath.Red).Shift(geom.V(10, 10))
		if encode(t, sample(t, s, ref.Time)) != saved[ref.Time] {
			t.Fatal("sample retained caller mutation")
		}
	}
	if encode(t, n.Object) != original || n.Text() != "0.00 kg" {
		t.Fatal("dynamic redraw mutated author")
	}
	// Each Source result has independent IDs; DynamicRedraw owns its distinct
	// deterministic preorder-ID policy, not authoring's right-slot policy.
	a, b := sample(t, s, 0).Find(n.Object.ID), sample(t, s, 1.5).Find(n.Object.ID)
	if len(a.Children) == len(b.Children) {
		t.Fatal("digit/group growth was not exercised")
	}
	if a.Children[0].ID != b.Children[0].ID {
		t.Fatal("dynamic ordinal ID changed")
	}
	n.Object.Shift(geom.V(20, 20)).SetFill(gomath.Red)
	if e := n.SetValue(-7); e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 3 {
				for _, ref := range refs.Frames {
					f, e := s.Sample(ref.Time)
					if e != nil {
						t.Error(e)
						return
					}
					if encode(t, f) != saved[ref.Time] {
						t.Error("concurrent sample changed")
					}
				}
			}
		}()
	}
	wg.Wait()
}

func TestNumberSceneBeforeSetValueIsIndependentAndSamplingErrorsAtomic(t *testing.T) {
	n := mustNew(t, 123.25, DefaultOptions())
	s := gomath.NewScene()
	if e := s.Add(n.Object); e != nil {
		t.Fatal(e)
	}
	before := encode(t, sample(t, s, 0))
	if e := n.SetValue(10000); e != nil {
		t.Fatal(e)
	}
	if encode(t, sample(t, s, 0)) != before {
		t.Fatal("previous scene snapshot changed after author edit")
	}
	f, e := text.Parse(data(t, "restricted.ttf"))
	if e != nil {
		t.Fatal(e)
	}
	o := DefaultOptions()
	o.Font = f
	o.DecimalPlaces = 0
	n = mustNew(t, 1, o)
	src, e := n.Source()
	if e != nil {
		t.Fatal(e)
	}
	if geometry, e := src.At(9); e == nil || geometry != nil {
		t.Fatal("late missing source glyph succeeded")
	}
	s = gomath.NewScene()
	if e := s.Add(n.Object); e != nil {
		t.Fatal(e)
	}
	if e := s.AddDynamicRedraw(n.Object, func(*gomath.Frame, float64) (*gomath.Object, error) { return src.At(9) }, gomath.DynamicRedrawOptions{}); e != nil {
		t.Fatal(e)
	}
	if frame, e := s.Sample(0); e == nil || frame != nil {
		t.Fatal("sampling leaked partial failure")
	}
}

func TestNumberWriteTransformAndPreview(t *testing.T) {
	r, c := numberRenderer(t)
	dir := t.TempDir()
	if preview := os.Getenv("GOMATH_NUMBER_PREVIEW_DIR"); preview != "" {
		dir = preview
		if e := os.MkdirAll(dir, 0755); e != nil {
			t.Fatal(e)
		}
	}
	o := DefaultOptions()
	o.Size = 1
	o.GroupThousands = false
	o.Unit = "kg"
	o.Color, _ = gomath.Hex("32a0ff")
	n := mustNew(t, 12.5, o)
	n.Object.Shift(geom.V(2, 0))
	source, e := n.Source()
	if e != nil {
		t.Fatal(e)
	}
	target, e := source.At(98.75)
	if e != nil {
		t.Fatal(e)
	}
	s := gomath.NewScene()
	if e := s.Add(n.Object); e != nil {
		t.Fatal(e)
	}
	if e := s.Play(1, gomath.Write(n.Object)); e != nil {
		t.Fatal(e)
	}
	if e := s.Play(1, gomath.Transform(n.Object, target).WithRate(gomath.Linear)); e != nil {
		t.Fatal(e)
	}
	target.Shift(geom.V(100, 100))
	last := ""
	for _, time := range []float64{0, .25, .5, .75, 1, 1.25, 1.5, 1.75, 2} {
		f := sample(t, s, time)
		img, e := r.Render(f)
		if e != nil {
			t.Fatal(e)
		}
		if time == 0 && pixels(img) != 0 {
			t.Fatal("Write start visible")
		}
		if time > 0 && pixels(img) < 100 {
			t.Fatal("Write/Transform intermediate invisible")
		}
		current := hash(img.Pix)
		if time > 0 && last == current {
			t.Fatal("intermediate pixels unchanged")
		}
		last = current
		if time == .5 || time == 1 || time == 1.5 || time == 2 {
			name := map[float64]string{.5: "write-half", 1: "write-complete", 1.5: "transform-half", 2: "transform-complete"}[time]
			if e := export.PNG(filepath.Join(dir, name+".png"), img); e != nil {
				t.Fatal(e)
			}
			file := filepath.Join(dir, name+".svg")
			if e := export.SVG(file, f, c); e != nil {
				t.Fatal(e)
			}
			data, e := os.Open(file)
			if e != nil {
				t.Fatal(e)
			}
			decoder := xml.NewDecoder(data)
			count := 0
			for {
				tok, e := decoder.Token()
				if e == io.EOF {
					break
				}
				if e != nil {
					data.Close()
					t.Fatal(e)
				}
				if node, ok := tok.(xml.StartElement); ok {
					if node.Name.Local == "text" || node.Name.Local == "image" {
						t.Fatal("number preview is not real vector glyphs")
					}
					if node.Name.Local == "path" {
						count++
					}
				}
			}
			data.Close()
			if count < 5 {
				t.Fatal("missing real glyph SVG paths")
			}
		}
	}
}
