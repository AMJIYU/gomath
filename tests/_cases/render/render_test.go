package render_test

import (
	"bytes"
	"context"
	"errors"
	"gomath"
	"gomath/geom"
	"gomath/render"
	"image"
	"math"
	"sync/atomic"
	"testing"
	"time"
)

func config() render.Config {
	c := render.DefaultConfig()
	c.Width, c.Height, c.FPS, c.Workers, c.FrameWidth = 128, 128, 12, 4, 8
	c.Background = gomath.Black
	return c
}
func TestFillStrokeAndAlpha(t *testing.T) {
	c := config()
	r, e := render.NewRasterizer(c)
	if e != nil {
		t.Fatal(e)
	}
	square := gomath.Square(2).SetFill(gomath.Red).SetStroke(gomath.Transparent, 0)
	frame := &gomath.Frame{Objects: []*gomath.Object{square}}
	img, e := r.Render(frame)
	if e != nil {
		t.Fatal(e)
	}
	if p := img.RGBAAt(64, 64); p.R < 240 || p.G > 120 {
		t.Fatal("fill missing", p)
	}
	if p := img.RGBAAt(5, 5); p.R != 0 || p.G != 0 || p.B != 0 {
		t.Fatal("background wrong", p)
	}
	line := gomath.Line(geom.V(-2, 0), geom.V(2, 0)).SetStroke(gomath.White, .5)
	img, e = r.Render(&gomath.Frame{Objects: []*gomath.Object{line}})
	if e != nil {
		t.Fatal(e)
	}
	if img.RGBAAt(64, 64).R < 250 || img.RGBAAt(64, 50).R != 0 {
		t.Fatal("stroke width wrong")
	}
	square.SetFill(gomath.White)
	group := gomath.Group(square).SetOpacity(.5)
	img, e = r.Render(&gomath.Frame{Objects: []*gomath.Object{group}})
	if e != nil {
		t.Fatal(e)
	}
	p := img.RGBAAt(64, 64)
	if p.R < 126 || p.R > 129 {
		t.Fatal("group opacity wrong", p)
	}
}
func TestClosedStrokeHasHole(t *testing.T) {
	r, _ := render.NewRasterizer(config())
	o := gomath.Square(3).SetStroke(gomath.White, .3)
	img, e := r.Render(&gomath.Frame{Objects: []*gomath.Object{o}})
	if e != nil {
		t.Fatal(e)
	}
	if img.RGBAAt(64, 64).R != 0 {
		t.Fatal("stroke filled center")
	}
	if img.RGBAAt(88, 64).R < 240 {
		t.Fatal("stroke edge missing")
	}
}

func TestArrowShaftAndTipVisible(t *testing.T) {
	r, _ := render.NewRasterizer(config())
	arrow := gomath.Arrow(geom.V(-2, 0), geom.V(2, 0), .6)
	img, err := r.Render(&gomath.Frame{Objects: []*gomath.Object{arrow}})
	if err != nil {
		t.Fatal(err)
	}
	// A subpixel shaft should still contribute coverage; a filled tip has an
	// opaque interior. This catches recursive styling erasing the shaft.
	if img.RGBAAt(48, 64).R < 30 || img.RGBAAt(91, 64).R < 200 {
		t.Fatal("arrow shaft or tip invisible")
	}
}
func TestThinPolylineHasNoJoinSpikes(t *testing.T) {
	r, _ := render.NewRasterizer(config())
	points := make([]geom.Vec3, 81)
	for i := range points {
		points[i] = geom.V(-2+float64(i)*.05, .03)
	}
	o := gomath.Polyline(points...).SetStroke(gomath.White, .04)
	img, e := r.Render(&gomath.Frame{Objects: []*gomath.Object{o}})
	if e != nil {
		t.Fatal(e)
	}
	minSum, maxSum := 10000, 0
	for x := 40; x < 88; x++ {
		sum := 0
		for y := 58; y < 70; y++ {
			sum += int(img.RGBAAt(x, y).R)
		}
		minSum = min(minSum, sum)
		maxSum = max(maxSum, sum)
	}
	if maxSum-minSum > 8 {
		t.Fatalf("join spikes: min=%d max=%d", minSum, maxSum)
	}
}

type capture struct {
	pixels          [][]byte
	closed, aborted bool
	fail            bool
}

func (s *capture) WriteFrame(index int, img *image.RGBA) error {
	if s.fail {
		return errors.New("sink failure")
	}
	if index != len(s.pixels) {
		return errors.New("unordered frame")
	}
	s.pixels = append(s.pixels, append([]byte(nil), img.Pix...))
	return nil
}
func (s *capture) Close() error { s.closed = true; return nil }
func (s *capture) Abort() error { s.aborted = true; return nil }
func build(t *testing.T) *gomath.Scene {
	t.Helper()
	s := gomath.NewScene()
	o := gomath.Circle(1).SetFill(gomath.Blue)
	if e := s.Add(o); e != nil {
		t.Fatal(e)
	}
	if e := s.Play(1, gomath.Shift(o, geom.V(2, 0))); e != nil {
		t.Fatal(e)
	}
	return s
}
func TestParallelFramesEqualSerial(t *testing.T) {
	s := build(t)
	c := config()
	c.Workers = 1
	a := &capture{}
	if _, e := render.RenderScene(context.Background(), s, c, a); e != nil {
		t.Fatal(e)
	}
	c.Workers = 4
	b := &capture{}
	if _, e := render.RenderScene(context.Background(), s, c, b); e != nil {
		t.Fatal(e)
	}
	if len(a.pixels) != 12 || len(b.pixels) != 12 {
		t.Fatal("wrong frame count")
	}
	for i := range a.pixels {
		if !bytes.Equal(a.pixels[i], b.pixels[i]) {
			t.Fatalf("parallel differs at frame %d", i)
		}
	}
	if !a.closed || !b.closed || a.aborted {
		t.Fatal("bad finalization")
	}
}
func TestCancelAndSinkFailureAbort(t *testing.T) {
	for _, cancelFirst := range []bool{true, false} {
		ctx, cancel := context.WithCancel(context.Background())
		if cancelFirst {
			cancel()
		}
		sink := &capture{fail: !cancelFirst}
		_, e := render.RenderScene(ctx, build(t), config(), sink)
		cancel()
		if e == nil || !sink.aborted || sink.closed {
			t.Fatal("failure did not abort", e)
		}
	}
}
func TestUpdaterFailureStopsWorkers(t *testing.T) {
	s := build(t)
	sentinel := errors.New("stop")
	s.AddUpdater(func(*gomath.Frame, float64) error { return sentinel })
	sink := &capture{}
	_, e := render.RenderScene(context.Background(), s, config(), sink)
	if !errors.Is(e, sentinel) || !sink.aborted {
		t.Fatal("updater error lost", e)
	}
}
func TestOutstandingFramesAreBounded(t *testing.T) {
	s := build(t)
	var calls atomic.Int64
	s.AddUpdater(func(f *gomath.Frame, t float64) error {
		calls.Add(1)
		if t == 0 {
			time.Sleep(15 * time.Millisecond)
		}
		return nil
	})
	sink := &countSink{calls: &calls, limit: 8, t: t}
	if _, e := render.RenderScene(context.Background(), s, config(), sink); e != nil {
		t.Fatal(e)
	}
}

type countSink struct {
	calls *atomic.Int64
	limit int64
	t     *testing.T
}

func (s *countSink) WriteFrame(i int, _ *image.RGBA) error {
	if n := s.calls.Load() - int64(i); n > s.limit {
		s.t.Errorf("%d frames outstanding, limit %d", n, s.limit)
	}
	return nil
}
func (*countSink) Close() error { return nil }
func (*countSink) Abort() error { return nil }
func TestInvalidConfigAndFrame(t *testing.T) {
	c := config()
	c.FrameWidth = math.Inf(1)
	if _, e := render.NewRasterizer(c); e == nil {
		t.Fatal("invalid config accepted")
	}
	r, _ := render.NewRasterizer(config())
	if _, e := r.Render(nil); e == nil {
		t.Fatal("nil frame accepted")
	}
	o := gomath.Group()
	o.Children = []*gomath.Object{o}
	if _, e := r.Render(&gomath.Frame{Objects: []*gomath.Object{o}}); e == nil {
		t.Fatal("cycle accepted")
	}
}
func BenchmarkRasterScene(b *testing.B) {
	s := gomath.NewScene()
	for i := range 20 {
		o := gomath.Circle(.15).MoveTo(geom.V(float64(i%5)-2, float64(i/5)-1.5))
		if e := s.Add(o); e != nil {
			b.Fatal(e)
		}
	}
	f, e := s.Sample(0)
	if e != nil {
		b.Fatal(e)
	}
	c := config()
	c.Width, c.Height = 640, 360
	r, _ := render.NewRasterizer(c)
	b.ReportAllocs()
	for b.Loop() {
		if _, e := r.Render(f); e != nil {
			b.Fatal(e)
		}
	}
}
