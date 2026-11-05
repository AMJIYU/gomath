package render

import (
	"bytes"
	"context"
	"fmt"
	"gomath"
	"gomath/geom"
	"gomath/path"
	"image"
	"image/color"
	"math"
	"reflect"
	"strings"
	"testing"
)

func ssConfig() Config {
	c := DefaultConfig()
	c.Width, c.Height, c.FrameWidth, c.Workers, c.Background = 95, 63, 95, 1, gomath.Transparent
	return c
}
func ssPolygon(points ...geom.Vec3) path.Path {
	var b path.Builder
	b.MoveTo(points[0])
	for _, v := range points[1:] {
		b.LineTo(v)
	}
	b.Close()
	return b.Path()
}
func ssFrame(t testing.TB) *gomath.Frame {
	t.Helper()
	// Pixel-space positions deliberately have fractional boundaries and odd ROI.
	outer := ssPolygon(geom.V(-40, -24), geom.V(39, -24), geom.V(39, 25), geom.V(-40, 25))
	hole := ssPolygon(geom.V(-12, -10), geom.V(-12, 11), geom.V(13, 11), geom.V(13, -10))
	outer.Contours = append(outer.Contours, hole.Contours...)
	g, err := gomath.NewLinearGradient(geom.V(-40, 0), geom.V(40, 0), []gomath.GradientStop{{Offset: 0, Color: gomath.Color{R: 1, A: .4}}, {Offset: 1, Color: gomath.Color{B: 1, A: .8}}})
	if err != nil {
		t.Fatal(err)
	}
	fill := gomath.NewObject(outer).SetFillGradient(g).SetStroke(gomath.White, .7).SetOpacity(.8)
	clip, err := gomath.NewClip(gomath.Rectangle(73.3, 47.7).Rotate(.12).Path)
	if err != nil {
		t.Fatal(err)
	}
	fill.SetClip(clip)
	pixels := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	pixels.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 128})
	pixels.SetNRGBA(1, 0, color.NRGBA{G: 255, A: 255})
	pixels.SetNRGBA(0, 1, color.NRGBA{B: 255, A: 200})
	pixels.SetNRGBA(1, 1, color.NRGBA{R: 255, G: 255, A: 64})
	asset, err := gomath.NewBitmap(pixels)
	if err != nil {
		t.Fatal(err)
	}
	bitmap, err := gomath.NewImage(asset, 18)
	if err != nil {
		t.Fatal(err)
	}
	bitmap.Rotate(.27).Shift(geom.V(19, 4)).SetClip(clip)
	stroke := gomath.Polyline(geom.V(-38, -18), geom.V(-20, 21), geom.V(-2, -15)).SetStroke(gomath.Green, 1.1)
	dash, err := gomath.NewDashPattern([]float64{4, 2}, .5)
	if err != nil {
		t.Fatal(err)
	}
	stroke.Style.Dash = dash
	stroke.Style.StrokeCap = gomath.CapSquare
	stroke.Style.StrokeJoin = gomath.JoinRound
	return &gomath.Frame{Objects: []*gomath.Object{fill, bitmap, stroke}}
}

func TestSupersamplingDefaultPreservesExistingPixels(t *testing.T) {
	f := ssFrame(t)
	for _, background := range []gomath.Color{gomath.Transparent, gomath.RGB(.1, .2, .3), {R: 1, G: .3, A: .35}} {
		c := ssConfig()
		c.Background = background
		baseline, _ := NewRasterizer(c)
		want, err := baseline.renderFrame(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, scale := range []int{0, 1} {
			c.Supersampling = scale
			r, err := NewRasterizer(c)
			if err != nil {
				t.Fatal(err)
			}
			got, err := r.Render(f)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got.Pix, want.Pix) || got.Bounds() != want.Bounds() {
				t.Fatalf("default %d changed pixels", scale)
			}
		}
	}
}
func TestSupersamplingBoxResolvePremultipliedOddDimensions(t *testing.T) {
	for _, scale := range []int{2, 4, 8} {
		high := image.NewRGBA(image.Rect(0, 0, 3*scale, 5*scale))
		for y := range high.Rect.Dy() {
			for x := range high.Rect.Dx() {
				high.SetRGBA(x, y, color.RGBA{R: 12, G: 34, B: 56, A: 78})
			}
		}
		// One opaque red sample among transparent samples must average to red/A
		// together, with no unpremultiply/double alpha or hidden transparent color.
		for y := range scale {
			for x := range scale {
				high.SetRGBA(x, y, color.RGBA{})
			}
		}
		high.SetRGBA(0, 0, color.RGBA{R: 255, A: 255})
		got := resolveBox(high, 3, 5, scale)
		unit := uint8((255 + scale*scale/2) / (scale * scale))
		if got.RGBAAt(0, 0) != (color.RGBA{R: unit, A: unit}) || got.RGBAAt(2, 4) != (color.RGBA{R: 12, G: 34, B: 56, A: 78}) {
			t.Fatal("premultiplied/last pixel resolve", scale, got.RGBAAt(0, 0), got.RGBAAt(2, 4))
		}
		if got.Bounds() != image.Rect(0, 0, 3, 5) {
			t.Fatal("odd output dimensions")
		}
	}
}
func ssIndependentResolve(high *image.RGBA, w, h, scale int) *image.RGBA {
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			var v [4]float64
			for sy := y * scale; sy < (y+1)*scale; sy++ {
				for sx := x * scale; sx < (x+1)*scale; sx++ {
					c := high.RGBAAt(sx, sy)
					for i, b := range []uint8{c.R, c.G, c.B, c.A} {
						v[i] += float64(b)
					}
				}
			}
			for i := range v {
				out.Pix[y*out.Stride+x*4+i] = uint8(math.Floor(v[i]/float64(scale*scale) + .5))
			}
		}
	}
	return out
}
func TestSupersamplingEntirePipelineCameraAndStateIsolation(t *testing.T) {
	frame := ssFrame(t)
	cam := gomath.Camera2D{FrameWidth: 88, Center: geom.V(3, -2), Rotation: .19}
	viewed := *frame
	viewed.Camera = &cam
	before := frame.Objects[0].Copy()
	// Copy deliberately assigns fresh IDs; retain the original identity for
	// this snapshot comparison and snapshot immutable paint/clip values too.
	before.ID = frame.Objects[0].ID
	before.Children = nil // snapshot() creates an empty slice for leaf objects
	before.Clip, _ = gomath.NewClip(before.Clip.Path())
	before.Style.FillGradient, _ = before.Style.FillGradient.Transform(geom.Identity())
	for _, scale := range []int{2, 4, 8} {
		c := ssConfig()
		c.Supersampling = scale
		r, err := NewRasterizer(c)
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range []*gomath.Frame{frame, &viewed, frame} {
			base := r.config
			got, err := r.Render(f)
			if err != nil {
				t.Fatal(err)
			}
			if r.config != base {
				t.Fatal("working dimensions or camera leaked into base config")
			}
			highConfig := c
			highConfig.Width *= scale
			highConfig.Height *= scale
			highConfig.Supersampling = 1
			highRaster, err := NewRasterizer(highConfig)
			if err != nil {
				t.Fatal(err)
			}
			high, err := highRaster.Render(f)
			if err != nil {
				t.Fatal(err)
			}
			want := ssIndependentResolve(high, c.Width, c.Height, scale)
			if got.Bounds() != image.Rect(0, 0, c.Width, c.Height) || !bytes.Equal(got.Pix, want.Pix) {
				t.Fatalf("scale %d did not sample complete pipeline", scale)
			}
			for i := 0; i < len(got.Pix); i += 4 {
				if got.Pix[i] > got.Pix[i+3] || got.Pix[i+1] > got.Pix[i+3] || got.Pix[i+2] > got.Pix[i+3] {
					t.Fatal("resolve broke premultiplied RGBA")
				}
			}
		}
		if bad, err := r.Render(&gomath.Frame{Camera: &gomath.Camera2D{}}); err == nil || bad != nil {
			t.Fatal("bad camera returned partial output")
		}
		if bad, err := r.Render(&gomath.Frame{Objects: []*gomath.Object{nil}}); err == nil || bad != nil {
			t.Fatal("bad object returned partial output")
		}
		if bad, err := r.Render(nil); err == nil || bad != nil {
			t.Fatal("nil frame accepted")
		}
		first, err := r.Render(frame)
		if err != nil {
			t.Fatal(err)
		}
		copyFirst := append([]byte(nil), first.Pix...)
		second, err := r.Render(frame)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(first.Pix, copyFirst) || !bytes.Equal(first.Pix, second.Pix) {
			t.Fatal("later render changed output storage or config")
		}
		second.Pix[0] ^= 255
		if !bytes.Equal(first.Pix, copyFirst) {
			t.Fatal("outputs alias")
		}
	}
	if !reflect.DeepEqual(before, frame.Objects[0]) {
		t.Fatal("supersampling mutated geometry/clip/gradient")
	}
}
func TestSupersamplingConfigurationBudgets(t *testing.T) {
	for _, scale := range []int{0, 1, 2, 4, 8} {
		c := ssConfig()
		c.Supersampling = scale
		if err := c.Validate(); err != nil {
			t.Fatal(scale, err)
		}
	}
	for _, scale := range []int{-1, 3, 5, 16, math.MaxInt} {
		c := ssConfig()
		c.Supersampling = scale
		if _, err := NewRasterizer(c); err == nil {
			t.Fatal("invalid scale", scale)
		}
	}
	for _, tc := range []struct {
		w, h, s, workers int
		want             string
	}{
		{8193, 1, 2, 1, "dimensions"}, {2048, 2049, 2, 1, "working pixels"}, {2048, 2048, 2, 1, "worker pixel"}, {1024, 1024, 2, 9, "configured-worker"}, {math.MaxInt, 1, 8, 1, "dimensions"},
	} {
		c := ssConfig()
		c.Width, c.Height, c.Supersampling, c.Workers = tc.w, tc.h, tc.s, tc.workers
		if r, err := NewRasterizer(c); r != nil || err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("budget %v: %v", tc, err)
		}
	}
	// The aggregate check reserves all configured workers, even for one frame.
	c := ssConfig()
	c.Width, c.Height, c.Supersampling, c.Workers = 1024, 1024, 2, 8
	if err := c.Validate(); err != nil {
		t.Fatal("valid aggregate boundary", err)
	}
}

type ssSource struct{ frame *gomath.Frame }

func (s ssSource) Duration() float64                     { return .25 }
func (s ssSource) Sample(float64) (*gomath.Frame, error) { return s.frame, nil }

type ssSink struct {
	frames          []*image.RGBA
	closed, aborted bool
}

func (s *ssSink) WriteFrame(i int, f *image.RGBA) error {
	if i != len(s.frames) {
		return fmt.Errorf("unordered frame")
	}
	s.frames = append(s.frames, f)
	return nil
}
func (s *ssSink) Close() error { s.closed = true; return nil }
func (s *ssSink) Abort() error { s.aborted = true; return nil }
func TestSupersamplingWorkersOrderOwnershipAndBudgetAbort(t *testing.T) {
	frame := ssFrame(t)
	source := ssSource{frame}
	var prior *ssSink
	for _, workers := range []int{1, 3} {
		c := ssConfig()
		c.Supersampling, c.Workers, c.FPS = 4, workers, 12
		sink := &ssSink{}
		stats, err := RenderScene(context.Background(), source, c, sink)
		if err != nil {
			t.Fatal(err)
		}
		if stats.Frames != 3 || !sink.closed || sink.aborted {
			t.Fatal("sequence finalization", stats, sink)
		}
		for i, f := range sink.frames {
			if f.Bounds() != image.Rect(0, 0, 95, 63) {
				t.Fatal("worker exposed high dimensions")
			}
			if prior != nil && !bytes.Equal(f.Pix, prior.frames[i].Pix) {
				t.Fatal("worker nondeterminism")
			}
		}
		sink.frames[0].Pix[0] ^= 255
		if bytes.Equal(sink.frames[0].Pix, sink.frames[1].Pix) {
			t.Fatal("queued output aliases")
		}
		sink.frames[0].Pix[0] ^= 255
		prior = sink
	}
	c := ssConfig()
	c.Width, c.Height, c.Supersampling, c.Workers = 1024, 1024, 2, 9
	sink := &ssSink{}
	if _, err := RenderScene(context.Background(), source, c, sink); err == nil || !sink.aborted || sink.closed || len(sink.frames) != 0 {
		t.Fatal("invalid budget did not abort before output", err)
	}
}

func TestSupersamplingCameraCompensation(t *testing.T) {
	frame := ssFrame(t)
	for _, scale := range []int{1, 2, 4, 8} {
		c := ssConfig()
		c.Supersampling = scale
		r, err := NewRasterizer(c)
		if err != nil {
			t.Fatal(err)
		}
		want, err := r.Render(frame)
		if err != nil {
			t.Fatal(err)
		}
		cam := gomath.Camera2D{FrameWidth: c.FrameWidth, Center: geom.V(7, -3), Rotation: .31}
		viewed := &gomath.Frame{Camera: &cam}
		for _, o := range frame.Objects {
			viewed.Objects = append(viewed.Objects, o.Copy().ApplyMatrix(geom.RotationZ(cam.Rotation)).Shift(cam.Center))
		}
		got, err := r.Render(viewed)
		if err != nil {
			t.Fatal(err)
		}
		for i, b := range got.Pix {
			if math.Abs(float64(b)-float64(want.Pix[i])) > 1 {
				t.Fatalf("scale=%d camera compensation changed channel %d", scale, i)
			}
		}
	}
}

func TestSupersamplingWorkingClipBudgetReturnsNoPartialOutput(t *testing.T) {
	c := ssConfig()
	c.Width, c.Height, c.FrameWidth, c.Supersampling = 100, 100, 100, 8
	clip, err := gomath.NewClip(gomath.Rectangle(100, 100).Path)
	if err != nil {
		t.Fatal(err)
	}
	leaf := gomath.Rectangle(100, 100).SetFill(gomath.White).SetStroke(gomath.Transparent, 0).SetClip(clip)
	root := leaf
	for range gomath.MaxClipDepth - 1 {
		root = gomath.Group(root).SetClip(clip)
	}
	frame := &gomath.Frame{Objects: []*gomath.Object{root, root.Copy()}}
	r, err := NewRasterizer(c)
	if err != nil {
		t.Fatal(err)
	}
	// Each drawable needs 64 masks. The first paints successfully, while the
	// second exceeds work only at 8x; that failure must not expose the first.
	if got, err := r.Render(frame); got != nil || err == nil || !strings.Contains(err.Error(), "clip raster work") {
		t.Fatal("working-resolution budget returned partial output", err)
	}
	empty, err := r.Render(&gomath.Frame{})
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range empty.Pix {
		if v != 0 {
			t.Fatal("failed render leaked pixels into next frame")
		}
	}
	c.Supersampling = 1
	r, err = NewRasterizer(c)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.Render(frame); err != nil {
		t.Fatal("baseline budget changed", err)
	}
}

// One complete mixed frame per operation: alpha gradient with a compound hole,
// clipped rotated bitmap, and dashed cap/join stroke, at fixed 640x360 output.
// Scene construction and scratch warm-up are excluded; resolve/all output and
// per-frame scratch allocations are included. Each subbenchmark uses one worker.
func BenchmarkSupersamplingMixedFrame(b *testing.B) {
	frame := ssFrame(b)
	for _, scale := range []int{1, 2, 4} {
		b.Run(fmt.Sprintf("scale%d", scale), func(b *testing.B) {
			c := ssConfig()
			c.Width, c.Height, c.FrameWidth, c.Supersampling = 640, 360, 110, scale
			r, err := NewRasterizer(c)
			if err != nil {
				b.Fatal(err)
			}
			if _, err = r.Render(frame); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				image, err := r.Render(frame)
				if err != nil {
					b.Fatal(err)
				}
				if image.Rect.Dx() != 640 || image.Rect.Dy() != 360 {
					b.Fatal("output dimensions")
				}
			}
			b.ReportMetric(float64(c.Width*c.Height*scale*scale), "working-pixels/frame")
		})
	}
}
