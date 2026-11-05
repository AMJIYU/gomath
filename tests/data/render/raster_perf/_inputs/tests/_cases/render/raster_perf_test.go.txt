package render

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"gomath"
	"gomath/geom"
	"gomath/path"
	"gomath/text"
	"image"
	"os"
	"strings"
	"sync"
	"testing"
)

// Authored once, then reused without layout/scene construction in the timed
// region. Both workloads contain vector text, nested clipping, bitmap alpha,
// linear gradients, and dashed strokes. The complex case repeats distinct
// transformed panels rather than timing a synthetic scratch allocation loop.
func rasterPerfScene(t testing.TB, complex bool) (*gomath.Frame, Config) {
	t.Helper()
	c := ssConfig()
	c.Width, c.Height, c.FrameWidth = 640, 360, 110
	panels := 1
	if complex {
		panels = 6
	}
	f := &gomath.Frame{}
	for i := range panels {
		panel := ssFrame(t)
		opt := text.DefaultOptions()
		opt.Size = 7
		label, err := text.New("Go: clip & AA", text.DefaultFont(), opt)
		if err != nil {
			t.Fatal(err)
		}
		label.Shift(geom.V(-28, -3))
		panel.Objects = append(panel.Objects, label)
		group := gomath.Group(panel.Objects...)
		clip, err := gomath.NewClip(gomath.Rectangle(78.5, 48.3).Rotate(-.07).Path)
		if err != nil {
			t.Fatal(err)
		}
		group.SetClip(clip)
		if complex {
			group.Scale(.47).Rotate(.025 * float64(i-2)).Shift(geom.V(float64(i%3-1)*34, 14-float64(i/3)*28))
		}
		f.Objects = append(f.Objects, group)
	}
	return f, c
}

// A previously painted layer must not survive a change in shape, origin,
// alpha, fill rule, camera, or a failed clip. Compare reused workers to clean
// workers, and retain every returned image to detect output/scratch aliasing.
func TestClipScratchSequenceIsolation(t *testing.T) {
	large := ssFrame(t)
	frames := []*gomath.Frame{large}
	for i, size := range [][2]float64{{50, 8}, {8, 50}, {31, 17}, {17, 31}, {2, 3}} {
		o := gomath.Rectangle(size[0], size[1]).SetFill(gomath.Color{R: .8, B: .4, A: .3}).SetStroke(gomath.White, .7)
		if i%2 == 0 {
			o.SetFillRule(path.EvenOdd)
		} else {
			o.SetFill(gomath.Transparent)
		}
		o.Shift(geom.V(float64(i*7-18), float64(i*4-13)))
		clip, err := gomath.NewClip(gomath.Circle(22).Path)
		if err != nil {
			t.Fatal(err)
		}
		o.SetClip(clip)
		frames = append(frames, &gomath.Frame{Objects: []*gomath.Object{o}})
	}
	viewed := *large
	viewed.Camera = &gomath.Camera2D{Center: geom.V(4, -3), Rotation: .17, FrameWidth: 88}
	frames = append(frames, &gomath.Frame{}, &viewed, large)
	for _, scale := range []int{1, 2, 4} {
		c := ssConfig()
		c.Supersampling = scale
		r, _ := NewRasterizer(c)
		var retained []*image.RGBA
		var snapshots [][]byte
		for _, f := range frames {
			got, err := r.Render(f)
			if err != nil {
				t.Fatal(err)
			}
			clean, _ := NewRasterizer(c)
			want, err := clean.Render(f)
			if err != nil || !bytes.Equal(got.Pix, want.Pix) {
				t.Fatalf("ss%d scratch contaminated next shape/view: %v", scale, err)
			}
			retained = append(retained, got)
			snapshots = append(snapshots, bytes.Clone(got.Pix))
		}
		for i, im := range retained {
			if !bytes.Equal(im.Pix, snapshots[i]) {
				t.Fatal("later render mutated retained output")
			}
		}
	}
}

func TestClipScratchFailureBudgetAndRecovery(t *testing.T) {
	c := ssConfig()
	c.Width, c.Height, c.FrameWidth = 8, 8, 8
	clip, err := gomath.NewClip(gomath.Square(6).Path)
	if err != nil {
		t.Fatal(err)
	}
	o := gomath.Square(4).SetFill(gomath.White).SetClip(clip)
	r, _ := NewRasterizer(c)
	frame := &gomath.Frame{Objects: []*gomath.Object{o}}
	want, err := r.Render(frame)
	if err != nil {
		t.Fatal(err)
	}
	// Exhaust work before acquiring a new layer. The destination must stay
	// untouched; resetting per-frame counters must restore normal rendering.
	r.clipWork = MaxClipRasterWork
	dst := image.NewRGBA(want.Bounds())
	if err := r.paintClipped(dst, drawable{object: o, opacity: 1, clips: []*gomath.Clip{clip}}); err == nil || !strings.Contains(err.Error(), "budget") {
		t.Fatal("clip work budget", err)
	}
	if !bytes.Equal(dst.Pix, make([]byte, len(dst.Pix))) {
		t.Fatal("budget rejection published pixels")
	}
	// Failure after the layer was painted, while producing its clip mask.
	noisy := gomath.Polygon(geom.V(-3, -3), geom.V(3, 3), geom.V(-3, 3), geom.V(3, -3)).Path
	for len(noisy.Contours) < gomath.MaxClipContours {
		noisy.Contours = append(noisy.Contours, noisy.Contours[0])
	}
	bad, err := gomath.NewClipWithRule(noisy, path.EvenOdd)
	if err != nil {
		t.Fatal(err)
	}
	o.SetClip(bad)
	if got, err := r.Render(frame); err == nil || got != nil {
		t.Fatal("failed mask exposed partial frame", err)
	}
	o.SetClip(clip).SetFill(gomath.Color{R: .2, G: .5, A: .2})
	got, err := r.Render(frame)
	clean, _ := NewRasterizer(c)
	expected, cleanErr := clean.Render(frame)
	if err != nil || cleanErr != nil || !bytes.Equal(got.Pix, expected.Pix) {
		t.Fatal("failed clip retained opaque pixels/mask", err, cleanErr)
	}
	if want.RGBAAt(4, 4).A != 255 || got.RGBAAt(4, 4).A != 51 {
		t.Fatal("retained output or changed alpha", want.RGBAAt(4, 4), got.RGBAAt(4, 4))
	}
}

func TestClipScratchIndependentWorkers(t *testing.T) {
	f, c := rasterPerfScene(t, true)
	c.Width, c.Height, c.Supersampling = 159, 91, 2
	r, _ := NewRasterizer(c)
	want, err := r.Render(f)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			worker, _ := NewRasterizer(c)
			for range 3 {
				got, err := worker.Render(f)
				if err != nil || !bytes.Equal(got.Pix, want.Pix) {
					t.Error("independent worker changed shared snapshot", err)
					return
				}
			}
		})
	}
	wg.Wait()
}

func BenchmarkRasterPerfMixed(b *testing.B) {
	for _, complex := range []bool{false, true} {
		name := "ordinary"
		if complex {
			name = "complex"
		}
		b.Run(name, func(b *testing.B) {
			f, c := rasterPerfScene(b, complex)
			r, err := NewRasterizer(c)
			if err != nil {
				b.Fatal(err)
			}
			if _, err = r.Render(f); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if _, err := r.Render(f); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// The corpus is captured from the fresh before implementation by a separate
// development-only generator. Normal regression runs never rewrite fixtures.
func TestRasterPerfFrozenPixels(t *testing.T) {
	data, err := os.ReadFile("../tests/data/render/raster_perf/pixels.json")
	if err != nil {
		t.Fatal(err)
	}
	var want map[string]string
	if err := json.Unmarshal(data, &want); err != nil {
		t.Fatal(err)
	}
	for _, complex := range []bool{false, true} {
		f, c := rasterPerfScene(t, complex)
		for _, scale := range []int{1, 2, 4} {
			c.Supersampling = scale
			r, err := NewRasterizer(c)
			if err != nil {
				t.Fatal(err)
			}
			got, err := r.Render(f)
			if err != nil {
				t.Fatal(err)
			}
			key := fmt.Sprintf("complex=%t/ss%d", complex, scale)
			h := sha256.Sum256(got.Pix)
			if hex.EncodeToString(h[:]) != want[key] {
				t.Fatalf("%s changed premultiplied RGBA bytes: %x", key, h)
			}
		}
	}
}
