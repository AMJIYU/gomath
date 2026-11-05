package rasterimage_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"gomath"
	"gomath/export"
	"gomath/geom"
	"gomath/rasterimage"
	"gomath/render"
	"image"
	"image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func pixelScene(t testing.TB) (*gomath.Scene, *gomath.Object, render.Config) {
	t.Helper()
	oracle := readPixelOracle(t)
	a, err := rasterimage.NewAsset(pixelInput(t, oracle.Scene.Input, oracle.Scene.Width, oracle.Scene.Height))
	if err != nil {
		t.Fatal(err)
	}
	a = runPixelSteps(t, a, oracle.Scene.Steps)
	o, err := a.NewImage(16)
	if err != nil {
		t.Fatal(err)
	}
	o.Rotate(math.Pi / 2)
	s := gomath.NewScene()
	if err := s.Play(1, gomath.FadeIn(o).WithRate(gomath.Linear)); err != nil {
		t.Fatal(err)
	}
	if err := s.Play(1, gomath.Shift(o, geom.V(4, -2)).WithRate(gomath.Linear)); err != nil {
		t.Fatal(err)
	}
	if err := s.Play(1, gomath.FadeOut(o).WithRate(gomath.Linear)); err != nil {
		t.Fatal(err)
	}
	c := render.DefaultConfig()
	c.Width, c.Height, c.FrameWidth, c.Background, c.FPS, c.Workers = 64, 64, 64, gomath.Black, 4, 1
	return s, o, c
}

func TestPixelPinnedAnimationFramesAndPNG(t *testing.T) {
	s, object, c := pixelScene(t)
	oracle := readPixelOracle(t)
	r, err := render.NewRasterizer(c)
	if err != nil {
		t.Fatal(err)
	}
	// Arbitrary order catches sample-to-sample ownership and state leakage.
	order := []int{5, 2, 8, 1, 6, 0, 4, 7, 3, 5, 2}
	for _, i := range order {
		state := oracle.Scene.States[i]
		t.Run(fmt.Sprintf("time_%g_visit_%d", state.Time, i), func(t *testing.T) {
			frame, err := s.Sample(state.Time)
			if err != nil {
				t.Fatal(err)
			}
			img, err := r.Render(frame)
			if err != nil {
				t.Fatal(err)
			}
			want, err := png.Decode(bytes.NewReader(readPixelFile(t, state.PNG)))
			if err != nil {
				t.Fatal(err)
			}
			requirePixelBytes(t, img.Pix, bitmapPixelBytes(t, want))
			name := filepath.Join(t.TempDir(), "frame.png")
			if err := export.PNG(name, img); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(name)
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := png.Decode(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			requirePixelBytes(t, bitmapPixelBytes(t, decoded), bitmapPixelBytes(t, want))
			if sampled := frame.Find(object.ID); sampled != nil {
				if sampled == object || sampled.Bitmap != object.Bitmap {
					t.Fatal("sample did not independently own geometry/share immutable pixels")
				}
				sampled.Rotate(.7).SetOpacity(.2)
			}
		})
	}
}

func TestPixelWithPixelsGeometryAndAtomicRejection(t *testing.T) {
	old, err := rasterimage.New(source(), 4)
	if err != nil {
		t.Fatal(err)
	}
	old.Rotate(.37).ApplyMatrix(geom.Scaling(-1, 2, 1)).Shift(geom.V(3, -2)).SetOpacity(.7)
	old.Name, old.ZIndex, old.Scalar = "positioned image", 3, .25
	old.Clip, err = gomath.NewClip(gomath.Square(20).Path)
	if err != nil {
		t.Fatal(err)
	}
	a, err := rasterimage.NewAsset(source())
	if err != nil {
		t.Fatal(err)
	}
	a, err = a.Invert()
	if err != nil {
		t.Fatal(err)
	}
	before := old.Copy()
	updated, err := a.WithPixels(old)
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID == old.ID || updated.Bitmap != a.Bitmap() || !reflect.DeepEqual(updated.Path, old.Path) || updated.Clip != old.Clip || updated.Style != old.Style || updated.Name != old.Name || updated.ZIndex != old.ZIndex || updated.Scalar != old.Scalar {
		t.Fatal("replacement changed affine geometry/style/metadata or identity")
	}
	updated.Rotate(.2)
	if !reflect.DeepEqual(old.Path, before.Path) || old.Bitmap != before.Bitmap || old.Style != before.Style {
		t.Fatal("replacement aliases input geometry")
	}
	s := gomath.NewScene()
	if err := s.Add(old, updated); err != nil {
		t.Fatal("original and edited images cannot coexist", err)
	}
	// Same-asset transforms preserve the immutable bitmap throughout interiors.
	target := updated.Copy().Rotate(.2).Shift(geom.V(.3, -.5))
	if err := s.Play(1, gomath.Transform(updated, target).WithRate(gomath.Linear)); err != nil {
		t.Fatal(err)
	}
	frame, err := s.Sample(.5)
	if err != nil || frame.Find(updated.ID).Bitmap != a.Bitmap() {
		t.Fatal("transformed pixel asset was lost", err)
	}
	beforeDuration := s.Duration()
	if err := s.Play(1, gomath.Transform(updated, old)); err == nil || s.Duration() != beforeDuration {
		t.Fatal("unsupported cross-asset pixel morph was committed")
	}
	other, err := rasterimage.New(image.NewNRGBA(image.Rect(0, 0, 3, 1)), 4)
	if err != nil {
		t.Fatal(err)
	}
	invalid := old.Copy()
	invalid.Path.Contours[0].Curves[0].P1.X += 1
	for _, bad := range []*gomath.Object{nil, gomath.Square(1), other, invalid, old.Copy().SetFill(gomath.White), gomath.Group(old.Copy())} {
		if got, err := a.WithPixels(bad); err == nil || got != nil {
			t.Fatal("invalid replacement input accepted")
		}
	}
	withChild := old.Copy()
	withChild.Children = []*gomath.Object{gomath.Square(1)}
	if got, err := a.WithPixels(withChild); err == nil || got != nil {
		t.Fatal("leaf-only replacement silently edited a group")
	}
	for _, width := range []float64{0, -1, math.NaN(), math.Inf(1), math.SmallestNonzeroFloat64} {
		if got, err := a.NewImage(width); err == nil || got != nil {
			t.Fatal("invalid/underflowing world dimensions accepted", width)
		}
	}
	if !reflect.DeepEqual(old.Path, before.Path) || old.Style != before.Style || old.Bitmap != before.Bitmap {
		t.Fatal("failed edit/animation changed original object")
	}
}

func TestPixelDifferentAssetsFadeTransform(t *testing.T) {
	a, err := rasterimage.NewAsset(source())
	if err != nil {
		t.Fatal(err)
	}
	b, err := a.Invert()
	if err != nil {
		t.Fatal(err)
	}
	from, err := a.NewImage(4)
	if err != nil {
		t.Fatal(err)
	}
	to, err := b.WithPixels(from)
	if err != nil {
		t.Fatal(err)
	}
	s := gomath.NewScene()
	if err := s.Add(from); err != nil {
		t.Fatal(err)
	}
	if err := s.Play(1, gomath.FadeTransform(from, to).WithRate(gomath.Linear)); err != nil {
		t.Fatal(err)
	}
	for _, time := range []float64{.75, .25, .5} {
		frame, err := s.Sample(time)
		if err != nil {
			t.Fatal(err)
		}
		f, target := frame.Find(from.ID), frame.Find(to.ID)
		if f == nil || target == nil || f.Bitmap != a.Bitmap() || target.Bitmap != b.Bitmap() || math.Abs(f.Style.Opacity-(1-time)) > 1e-12 || math.Abs(target.Style.Opacity-time) > 1e-12 {
			t.Fatal("crossfade did not retain two independent pixel assets", time)
		}
		r, err := render.NewRasterizer(render.DefaultConfig())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := r.Render(frame); err != nil {
			t.Fatal(err)
		}
	}
	frame, err := s.Sample(1)
	if err != nil || frame.Find(from.ID) != nil || frame.Find(to.ID) == nil || frame.Find(to.ID).Bitmap != b.Bitmap() {
		t.Fatal("crossfade final ownership is incorrect", err)
	}
}

func TestPixelSVGEmbeddedBytesAndExistingStyleBoundary(t *testing.T) {
	s, object, c := pixelScene(t)
	frame, err := s.Sample(.5)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "pixels.svg")
	if err := export.SVG(file, frame, c); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	d := xml.NewDecoder(bytes.NewReader(data))
	found := 0
	for {
		token, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "image" {
			continue
		}
		found++
		attrs := map[string]string{}
		for _, a := range start.Attr {
			attrs[a.Name.Local] = a.Value
		}
		if attrs["opacity"] != "0.5" || attrs["width"] != "16" || attrs["height"] != "16" {
			t.Fatal("wrong embedded image metadata", attrs)
		}
		payload, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(attrs["href"], "data:image/png;base64,"))
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := png.Decode(bytes.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		// Compare against the same Bitmap encoded directly, including the
		// documented premultiplied-to-straight PNG quantization boundary.
		var direct bytes.Buffer
		if err := png.Encode(&direct, object.Bitmap); err != nil {
			t.Fatal(err)
		}
		want, err := png.Decode(bytes.NewReader(direct.Bytes()))
		if err != nil {
			t.Fatal(err)
		}
		requirePixelBytes(t, bitmapPixelBytes(t, decoded), bitmapPixelBytes(t, want))
		var a, b, cc, dd, e, f float64
		if n, err := fmt.Sscanf(attrs["transform"], "matrix(%f %f %f %f %f %f)", &a, &b, &cc, &dd, &e, &f); err != nil || n != 6 {
			t.Fatal("malformed affine transform", err)
		}
		tl, tr, bl, err := frame.Find(object.ID).ImageCorners()
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range []struct {
			x, y float64
			v    geom.Vec3
		}{{0, 0, tl}, {16, 0, tr}, {0, 16, bl}} {
			if math.Abs(a*p.x+cc*p.y+e-(32+p.v.X)) > 1e-6 || math.Abs(b*p.x+dd*p.y+f-(32-p.v.Y)) > 1e-6 {
				t.Fatal("SVG affine geometry changed")
			}
		}
	}
	if found != 1 {
		t.Fatal("pixel asset missing/duplicated in SVG")
	}
	// Pixel edits must not weaken the existing unsupported image paint guard.
	bad := frame.Find(object.ID).Copy().SetFill(gomath.Blue).SetColor(gomath.Red)
	badFrame := &gomath.Frame{Objects: []*gomath.Object{bad}}
	r, _ := render.NewRasterizer(c)
	if _, err := r.Render(badFrame); err == nil {
		t.Fatal("unsupported Object.SetColor tint silently accepted")
	}
	target := filepath.Join(t.TempDir(), "preserve.svg")
	if err := os.WriteFile(target, []byte("keep existing output"), 0600); err != nil {
		t.Fatal(err)
	}
	if export.SVG(target, badFrame, c) == nil {
		t.Fatal("unsupported Object.SetColor paint exported")
	}
	got, err := os.ReadFile(target)
	if err != nil || string(got) != "keep existing output" {
		t.Fatal("failed SVG replaced existing output", err)
	}
}

type pixelFrameSink struct{ frames [][]byte }

func (s *pixelFrameSink) WriteFrame(_ int, frame *image.RGBA) error {
	s.frames = append(s.frames, append([]byte(nil), frame.Pix...))
	return nil
}
func (*pixelFrameSink) Close() error { return nil }
func (*pixelFrameSink) Abort() error { return nil }

func TestPixelConcurrentSceneAndPNGSequence(t *testing.T) {
	s, _, c := pixelScene(t)
	serial := &pixelFrameSink{}
	if _, err := render.RenderScene(context.Background(), s, c, serial); err != nil {
		t.Fatal(err)
	}
	c.Workers = 4
	parallel := &pixelFrameSink{}
	if _, err := render.RenderScene(context.Background(), s, c, parallel); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(serial.frames, parallel.frames) {
		t.Fatal("parallel rendering changed pixel asset frames")
	}
	// Exercise the actual existing sequence exporter with worker rendering.
	dir := filepath.Join(t.TempDir(), "sequence")
	sink, err := export.NewPNGSequence(dir)
	if err != nil {
		t.Fatal(err)
	}
	stats, err := render.RenderScene(context.Background(), s, c, sink)
	if err != nil || stats.Frames != len(serial.frames) {
		t.Fatal("PNG sequence failed", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != stats.Frames {
		t.Fatal("PNG sequence frame count differs", err)
	}
	for i, entry := range entries {
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		requirePixelBytes(t, bitmapPixelBytes(t, img), serial.frames[i])
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for _, time := range []float64{1.5, .5, 2.5, 0} {
				frame, err := s.Sample(time)
				if err != nil {
					t.Error(err)
					return
				}
				frame.Objects[0].Rotate(.3)
			}
		})
	}
	wg.Wait()
}
