package field_test

import (
	"bytes"
	"context"
	"errors"
	"gomath"
	"gomath/field"
	"gomath/geom"
	"gomath/render"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
)

func TestAdaptiveActualObjectsBorrowedChildrenAndRender(t *testing.T) {
	var calls, colors atomic.Int64
	fn := func(p geom.Vec3) geom.Vec3 { calls.Add(1); return geom.V(-p.Y, p.X) }
	o := field.DefaultAdaptiveStreamOptions()
	o.Integration = adaptiveOptions()
	o.Integration.EndTime = 2.2
	o.ColorFunc = func(_, _ geom.Vec3, _ float64) gomath.Color { colors.Add(1); return gomath.Red }
	seeds := []geom.Vec3{geom.V(1, 0), geom.V(2, 0)}
	s, e := field.NewAdaptiveStreamLines(context.Background(), fn, seeds, o)
	if e != nil {
		t.Fatal(e)
	}
	if len(s.Lines()) != 2 || len(s.Object.Children) != 2 || colors.Load() != 2 {
		t.Fatal("missing lines/colors")
	}
	stats := field.AdaptiveStats{}
	for i, line := range s.Lines() {
		if line.Object != s.Object.Children[i] || line.Object.Style.Stroke != gomath.Red || len(line.Object.Path.Contours) != 1 || len(line.Object.Path.Contours[0].Curves) != line.Result.Stats().AcceptedSteps {
			t.Fatal("actual polyline child missing")
		}
		rs := line.Result.Stats()
		stats.AcceptedSteps += rs.AcceptedSteps
		stats.RejectedSteps += rs.RejectedSteps
		stats.Evaluations += rs.Evaluations
		points := line.Result.Points()
		for j, c := range line.Object.Path.Contours[0].Curves {
			if c.P0 != points[j] || c.P3 != points[j+1] {
				t.Fatal("accepted points were resampled")
			}
		}
	}
	if stats != s.Stats() || int64(stats.Evaluations) != calls.Load() {
		t.Fatal("shared stats mismatch")
	}
	lines := s.Lines()
	lines[0].Object = nil
	lines[0].Result.Points()[0] = geom.V(999, 999)
	if s.Lines()[0].Object == nil || s.Lines()[0].Result.Points()[0] == geom.V(999, 999) {
		t.Fatal("records/slices alias")
	}
	seeds[0] = geom.V(999, 999)
	scene := gomath.NewScene()
	if e = scene.Add(s.Object); e != nil {
		t.Fatal(e)
	}
	if e = scene.Play(1, gomath.Shift(s.Object, geom.V(.25, 0))); e != nil {
		t.Fatal(e)
	}
	cfg := render.DefaultConfig()
	cfg.Width, cfg.Height, cfg.FrameWidth = 96, 96, 6
	draw := func() []byte {
		frame, e := scene.Sample(.5)
		if e != nil {
			t.Error(e)
			return nil
		}
		r, e := render.NewRasterizer(cfg)
		if e != nil {
			t.Error(e)
			return nil
		}
		img, e := r.Render(frame)
		if e != nil {
			t.Error(e)
			return nil
		}
		return img.Pix
	}
	want := draw()
	lit := 0
	for i := 0; i < len(want); i += 4 {
		if want[i] > 0 {
			lit++
		}
	}
	if lit < 50 {
		t.Fatal("no adaptive streamline geometry rendered")
	}
	before := calls.Load()
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			if !bytes.Equal(want, draw()) {
				t.Error("concurrent sampled pixels differ")
			}
		})
	}
	wg.Wait()
	if calls.Load() != before || colors.Load() != 2 {
		t.Fatal("rendering called field/color callbacks")
	}
	copied := s.Object.Copy()
	if copied.Children[0] == s.Object.Children[0] || copied.Children[0].ID == s.Object.Children[0].ID {
		t.Fatal("copy retained borrowed identities")
	}
}

func TestAdaptiveStreamSharedBudgetsStyleAndEmpty(t *testing.T) {
	ctx := context.Background()
	calls := 0
	fn := func(geom.Vec3) geom.Vec3 { calls++; return geom.V(1, 0) }
	o := field.DefaultAdaptiveStreamOptions()
	o.Integration = adaptiveOptions()
	o.Integration.EndTime = .125
	o.Integration.InitialStep = .125
	o.Integration.Limits = field.AdaptiveLimits{MaxAcceptedSteps: 2, MaxRejectedSteps: 0, MaxEvaluations: 14, MaxStorageBytes: 64<<10 + 1024*4}
	seeds := []geom.Vec3{geom.V(0, 0), geom.V(0, .5)}
	s, e := field.NewAdaptiveStreamLines(ctx, fn, seeds, o)
	if e != nil || s.Stats().AcceptedSteps != 2 || calls != 14 {
		t.Fatalf("exact aggregate budget %v %v %d", s, e, calls)
	}
	for _, kind := range []string{"accepted", "evaluations", "storage"} {
		q := o
		switch kind {
		case "accepted":
			q.Integration.Limits.MaxAcceptedSteps = 1
		case "evaluations":
			q.Integration.Limits.MaxEvaluations = 13
		case "storage":
			q.Integration.Limits.MaxStorageBytes--
		}
		s, e = field.NewAdaptiveStreamLines(ctx, fn, seeds, q)
		if s != nil || !errors.Is(e, field.ErrAdaptiveLimit) {
			t.Fatalf("aggregate %s %v %v", kind, s, e)
		}
	}
	calls = 0
	s, e = field.NewAdaptiveStreamLines(ctx, fn, []geom.Vec3{geom.V(0, 0), geom.V(999, 0)}, o)
	if s != nil || !errors.Is(e, field.ErrAdaptiveOptions) || calls != 0 {
		t.Fatal("later malformed seed not preflighted")
	}
	s, e = field.NewAdaptiveStreamLines(ctx, fn, make([]geom.Vec3, 100001), o)
	if s != nil || !errors.Is(e, field.ErrAdaptiveLimit) || calls != 0 {
		t.Fatal("seed cap not preflighted")
	}
	for _, mutate := range []func(*field.AdaptiveStreamOptions){func(o *field.AdaptiveStreamOptions) { o.StrokeWidth = 0 }, func(o *field.AdaptiveStreamOptions) { o.Color.R = 2 }, func(o *field.AdaptiveStreamOptions) {
		o.ColorFunc = func(_, _ geom.Vec3, _ float64) gomath.Color { return gomath.Color{R: 2} }
	}} {
		q := o
		mutate(&q)
		s, e = field.NewAdaptiveStreamLines(ctx, fn, seeds, q)
		if s != nil || !errors.Is(e, field.ErrAdaptiveOptions) {
			t.Fatalf("style %v %v", s, e)
		}
	}
	o = field.DefaultAdaptiveStreamOptions()
	o.ColorFunc = func(_, _ geom.Vec3, _ float64) gomath.Color {
		t.Fatal("invalid seed vector reached color function")
		return gomath.Red
	}
	s, e = field.NewAdaptiveStreamLines(ctx, func(geom.Vec3) geom.Vec3 { return geom.Vec3{Z: 1} }, seeds, o)
	if e != nil || s.Lines()[0].Result.Reason() != field.InvalidVector || s.Lines()[0].Result.Stats().AcceptedSteps != 0 {
		t.Fatalf("invalid seed disappeared %v %v", s, e)
	}
	s, e = field.NewAdaptiveStreamLines(ctx, fn, nil, o)
	if e != nil || len(s.Lines()) != 0 || s.Stats() != (field.AdaptiveStats{}) {
		t.Fatal("empty seeds")
	}
	o.Integration.EndTime = o.Integration.StartTime
	s, e = field.NewAdaptiveStreamLines(ctx, fn, seeds, o)
	if e != nil || s.Lines()[0].Result.Stats() != (field.AdaptiveStats{}) {
		t.Fatal("zero duration called color/field", e)
	}
	var n *field.AdaptiveStreamLines
	if n.Lines() != nil || n.Stats() != (field.AdaptiveStats{}) {
		t.Fatal("nil stream methods")
	}
}

func TestAdaptiveConcurrentConstructionAndSnapshots(t *testing.T) {
	fn := func(p geom.Vec3) geom.Vec3 { return geom.V(-p.Y, p.X) }
	o := field.DefaultAdaptiveStreamOptions()
	o.Integration = adaptiveOptions()
	seed := []geom.Vec3{geom.V(1, 0)}
	base, e := field.NewAdaptiveStreamLines(context.Background(), fn, seed, o)
	if e != nil {
		t.Fatal(e)
	}
	want := base.Lines()[0].Result.Points()
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			s, e := field.NewAdaptiveStreamLines(context.Background(), fn, seed, o)
			if e != nil {
				t.Error(e)
				return
			}
			if !reflect.DeepEqual(s.Lines()[0].Result.Points(), want) {
				t.Error("callback sampling is nondeterministic")
			}
			for range 3 {
				p := base.Lines()[0].Result.Points()
				p[0].X = 999
				if base.Lines()[0].Result.Points()[0].X == 999 {
					t.Error("concurrent copies alias")
				}
			}
		})
	}
	wg.Wait()
}

func TestAdaptiveCallbacksCannotChangeValidatedSeedSnapshot(t *testing.T) {
	o := field.DefaultAdaptiveStreamOptions()
	o.Integration = adaptiveOptions()
	o.Integration.EndTime = .1
	seeds := []geom.Vec3{geom.V(1, 0), geom.V(2, 0)}
	original := append([]geom.Vec3(nil), seeds...)
	fn := func(p geom.Vec3) geom.Vec3 {
		seeds[1] = geom.V(999, 999)
		if !o.Integration.Domain.Contains(p) {
			t.Fatal("mutated caller seed escaped preflight", p)
		}
		return geom.V(-p.Y, p.X)
	}
	o.ColorFunc = func(_, _ geom.Vec3, _ float64) gomath.Color { seeds[0] = geom.V(-999, -999); return gomath.Red }
	s, e := field.NewAdaptiveStreamLines(context.Background(), fn, seeds, o)
	if e != nil {
		t.Fatal(e)
	}
	for i, line := range s.Lines() {
		if line.Result.Points()[0] != original[i] || line.Object.Path.Contours[0].Start != original[i] {
			t.Fatal("callback mutated owned input snapshot")
		}
	}
}
