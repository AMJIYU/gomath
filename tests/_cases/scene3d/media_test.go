package scene3d

import (
	"bytes"
	"context"
	"fmt"
	"gomath"
	"gomath/export"
	"gomath/render"
	"image/color"
	"image/gif"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

type overlaySource struct {
	*Scene
	overlay *gomath.Object
}

func (s overlaySource) Sample(time float64) (*gomath.Frame, error) {
	f, err := s.Scene.Sample(time)
	if err != nil {
		return nil, err
	}
	f.Objects = append(f.Objects, s.overlay) // completed immutable overlay
	return f, nil
}

func mediaFixture(t testing.TB) (*Scene, render.Config) {
	t.Helper()
	c, k, m := basicFixture(t)
	c.Duration = 1.25
	c.Renderer.Width, c.Renderer.Height = 32, 24
	p := IdentityPose()
	p.Position.X = .7
	p.Angle = 1.3
	m[0].Keys = []PoseKey{{Pose: IdentityPose()}, {Time: 1.25, Pose: p}}
	s := mustNew(t, c, k, m)
	rc := render.DefaultConfig()
	rc.Width, rc.Height, rc.FPS, rc.Workers = 32, 24, 4, 3
	rc.Background = gomath.Transparent
	rc.FrameWidth = 123 // source Camera2D must supply the actual bitmap width
	return s, rc
}

func TestMediaFrameSourcePNGAndGIF(t *testing.T) {
	s, rc := mediaFixture(t)
	dir := t.TempDir()
	target := filepath.Join(dir, "frames")
	sink, err := export.NewPNGSequence(target)
	if err != nil {
		t.Fatal(err)
	}
	stats, err := render.RenderScene(context.Background(), s, rc, sink)
	if err != nil || stats.Frames != 5 || stats.Duration != 1.25 {
		t.Fatalf("source media stats%+v %v", stats, err)
	}
	entries, err := os.ReadDir(target)
	if err != nil || len(entries) != 5 {
		t.Fatal(entries, err)
	}
	for i := range 5 {
		f, err := os.Open(filepath.Join(target, fmt.Sprintf("%06d.png", i)))
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := png.Decode(f)
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		sample, err := s.Sample(float64(i) / 4)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(bitmapBytes(decoded), bitmapBytes(sample.Objects[0].Bitmap)) {
			t.Fatalf("PNG frame%d did not preserve real CPU bitmap pixels", i)
		}
	}
	gifPath := filepath.Join(dir, "scene.gif")
	gifSink, err := export.NewGIF(gifPath, 32, 24, 4)
	if err != nil {
		t.Fatal(err)
	}
	rc.Background = gomath.Black
	stats, err = render.RenderScene(context.Background(), s, rc, gifSink)
	if err != nil || stats.Frames != 5 {
		t.Fatal(stats, err)
	}
	f, err := os.Open(gifPath)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := gif.DecodeAll(f)
	f.Close()
	if err != nil || len(decoded.Image) != 5 {
		t.Fatal("independent GIF decode", err)
	}
	for _, delay := range decoded.Delay {
		if delay != 25 {
			t.Fatal("media timing changed", decoded.Delay)
		}
	}
	if bytes.Equal(decoded.Image[0].Pix, decoded.Image[2].Pix) {
		t.Fatal("GIF timeline lost intermediate motion")
	}
}

func TestOverlayAndMediaFailureCleanup(t *testing.T) {
	s, rc := mediaFixture(t)
	o := gomath.Rectangle(.5, .5).SetFill(gomath.RGB(0, 1, 0)).SetStroke(gomath.Transparent, 0)
	o.ZIndex = 1
	source := overlaySource{s, o}
	r, err := render.NewRasterizer(rc)
	if err != nil {
		t.Fatal(err)
	}
	for _, time := range []float64{0, .5, 1.25} {
		frame, err := source.Sample(time)
		if err != nil {
			t.Fatal(err)
		}
		img, err := r.Render(frame)
		if err != nil || img.RGBAAt(16, 12) != (color.RGBA{G: 255, A: 255}) {
			t.Fatal("screen overlay missing", err)
		}
		if frame.Find(s.BitmapID()) == nil {
			t.Fatal("overlay lost scene bitmap identity")
		}
	}
	c, k, m := basicFixture(t)
	c.Renderer.TransparentFragmentLimit = 1
	m[0].Material.Color.A = 128
	bad := mustNew(t, c, k, m)
	dir := t.TempDir()
	target := filepath.Join(dir, "failed")
	sink, err := export.NewPNGSequence(target)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := render.RenderScene(context.Background(), bad, rc, sink); err == nil {
		t.Fatal("sampling failure was hidden by media pipeline")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("failed media target published", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatal("failed media scratch was retained", entries, err)
	}
}
