package render_test

import (
	"bytes"
	"gomath"
	"gomath/geom"
	"gomath/path"
	"gomath/render"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"testing"
)

func renderClip(t *testing.T, object *gomath.Object) *gomath.Clip {
	t.Helper()
	c, err := gomath.NewClip(object.Path)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func clipConfig(size int) render.Config {
	c := render.DefaultConfig()
	c.Width, c.Height, c.FrameWidth = size, size, float64(size)
	c.Background, c.Workers = gomath.Black, 1
	return c
}

func TestNestedClipIntersectionsOpacityAndSiblingIsolation(t *testing.T) {
	c := clipConfig(64)
	child := gomath.Square(48).SetFill(gomath.White).SetStroke(gomath.Transparent, 0).SetOpacity(.5)
	child.SetClip(renderClip(t, gomath.Square(32).Shift(geom.V(0, 8))))
	other := gomath.Square(8).SetFill(gomath.Red).SetStroke(gomath.Transparent, 0).Shift(geom.V(-24, -24))
	root := gomath.Group(child, other).SetOpacity(.5).SetClip(renderClip(t, gomath.Square(32).Shift(geom.V(8, 0))))
	r, err := render.NewRasterizer(c)
	if err != nil {
		t.Fatal(err)
	}
	got, err := r.Render(&gomath.Frame{Objects: []*gomath.Object{root}})
	if err != nil {
		t.Fatal(err)
	}
	for y := range 64 {
		for x := range 64 {
			want := uint8(0)
			if x >= 24 && x < 48 && y >= 16 && y < 40 {
				want = 64
			}
			pixel := got.RGBAAt(x, y)
			if pixel != (color.RGBA{R: want, G: want, B: want, A: 255}) {
				t.Fatalf("pixel %d,%d = %v, want gray %d", x, y, pixel, want)
			}
		}
	}
	// Removing an inherited clip must restore the sibling, while its peer's
	// private child clip remains confined to that peer.
	root.SetClip(nil)
	got, err = r.Render(&gomath.Frame{Objects: []*gomath.Object{root}})
	if err != nil {
		t.Fatal(err)
	}
	if got.RGBAAt(8, 56).R == 0 {
		t.Fatal("sibling inherited another child's clip")
	}
}

func TestClippedBitmapAndStrokeHaveNoOffscreenLayerOffset(t *testing.T) {
	c := clipConfig(64)
	pixels := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	pixels.SetNRGBA(0, 0, color.NRGBA{G: 255, A: 255})
	asset, err := gomath.NewBitmap(pixels)
	if err != nil {
		t.Fatal(err)
	}
	o, err := gomath.NewImage(asset, 40)
	if err != nil {
		t.Fatal(err)
	}
	o.Shift(geom.V(8, -8)).SetClip(renderClip(t, gomath.Square(16).Shift(geom.V(8, -8))))
	r, _ := render.NewRasterizer(c)
	got, err := r.Render(&gomath.Frame{Objects: []*gomath.Object{o}})
	if err != nil {
		t.Fatal(err)
	}
	for y := range 64 {
		for x := range 64 {
			want := uint8(0)
			if x >= 32 && x < 48 && y >= 32 && y < 48 {
				want = 255
			}
			if pixel := got.RGBAAt(x, y); pixel.G != want || pixel.R != 0 || pixel.B != 0 {
				t.Fatalf("bitmap clipping at %d,%d: %v", x, y, pixel)
			}
		}
	}
	line := gomath.Line(geom.V(-20, 0), geom.V(20, 0)).SetStroke(gomath.White, 16)
	line.SetClip(renderClip(t, gomath.Rectangle(16, 4).Shift(geom.V(8, 0))))
	got, err = r.Render(&gomath.Frame{Objects: []*gomath.Object{line}})
	if err != nil {
		t.Fatal(err)
	}
	if got.RGBAAt(40, 31).R != 255 || got.RGBAAt(40, 28).R != 0 || got.RGBAAt(30, 31).R != 0 {
		t.Fatal("stroke escaped clipping or lost ROI padding")
	}
}

func TestClipEmptyImplicitClosureNonzeroHolesAndLimits(t *testing.T) {
	r, _ := render.NewRasterizer(clipConfig(64))
	empty, err := gomath.NewClip(path.Path{})
	if err != nil {
		t.Fatal(err)
	}
	o := gomath.Square(60).SetFill(gomath.White).SetStroke(gomath.Transparent, 0).SetClip(empty)
	got, err := r.Render(&gomath.Frame{Objects: []*gomath.Object{o}})
	if err != nil {
		t.Fatal(err)
	}
	if got.RGBAAt(32, 32).R != 0 {
		t.Fatal("empty clip did not hide content")
	}
	open := gomath.Polyline(geom.V(-16, -16), geom.V(16, -16), geom.V(0, 16))
	o.SetClip(renderClip(t, open))
	got, err = r.Render(&gomath.Frame{Objects: []*gomath.Object{o}})
	if err != nil {
		t.Fatal(err)
	}
	if got.RGBAAt(32, 32).R != 255 || got.RGBAAt(16, 16).R != 0 {
		t.Fatal("open clip contour was not implicitly closed")
	}
	ring, err := gomath.Annulus(8, 24)
	if err != nil {
		t.Fatal(err)
	}
	o.SetClip(renderClip(t, ring))
	got, err = r.Render(&gomath.Frame{Objects: []*gomath.Object{o}})
	if err != nil {
		t.Fatal(err)
	}
	if got.RGBAAt(32, 32).R != 0 || got.RGBAAt(48, 32).R != 255 {
		t.Fatal("clip hole lost nonzero winding")
	}
	root := o
	for range gomath.MaxClipDepth {
		root = gomath.Group(root).SetClip(empty)
	}
	if _, err = r.Render(&gomath.Frame{Objects: []*gomath.Object{root}}); err == nil {
		t.Fatal("clip ancestor depth budget ignored")
	}
	o.SetClip(&gomath.Clip{})
	if _, err = r.Render(&gomath.Frame{Objects: []*gomath.Object{o}}); err == nil {
		t.Fatal("uninitialized clip rendered")
	}
	o.SetClip(renderClip(t, gomath.Square(1e10)))
	if _, err = r.Render(&gomath.Frame{Objects: []*gomath.Object{o}}); err == nil {
		t.Fatal("unrepresentable clip raster coordinate rendered")
	}
	// Check work before drawing 64 copies of a million-pixel mask.
	large := gomath.Square(1025).SetFill(gomath.White)
	mask := renderClip(t, large)
	for range gomath.MaxClipDepth {
		large = gomath.Group(large).SetClip(mask)
	}
	bounded, _ := render.NewRasterizer(clipConfig(1025))
	if _, err = bounded.Render(&gomath.Frame{Objects: []*gomath.Object{large}}); err == nil {
		t.Fatal("clip raster work budget ignored")
	}
	o.SetClip(empty)
	if _, err = bounded.Render(&gomath.Frame{Objects: []*gomath.Object{o}}); err != nil {
		t.Fatal("failed clip render poisoned later frame", err)
	}
}

func TestClipIndependentCairoSVGReferenceAndCamera(t *testing.T) {
	c := clipConfig(192)
	c.Height = 144
	for _, name := range []string{"clip-nested", "clip-bitmap"} {
		t.Run(name, func(t *testing.T) {
			var object *gomath.Object
			if name == "clip-nested" {
				ring, err := gomath.Annulus(20, 46)
				if err != nil {
					t.Fatal(err)
				}
				leaf := gomath.Rectangle(160, 110).SetFill(gomath.RGB(64.0/255, 160.0/255, 240.0/255)).SetStroke(gomath.Transparent, 0).SetOpacity(.8).SetClip(renderClip(t, ring))
				outer := gomath.Polygon(geom.V(-70, -45), geom.V(65, -35), geom.V(55, 50), geom.V(-60, 40))
				object = gomath.Group(leaf).SetClip(renderClip(t, outer))
			} else {
				pixels := image.NewNRGBA(image.Rect(0, 0, 16, 16))
				for y := range 16 {
					for x := range 16 {
						pixels.SetNRGBA(x, y, color.NRGBA{R: 230, G: 120, B: 40, A: 128})
					}
				}
				asset, err := gomath.NewBitmap(pixels)
				if err != nil {
					t.Fatal(err)
				}
				object, err = gomath.NewImage(asset, 120)
				if err != nil {
					t.Fatal(err)
				}
				object.SetOpacity(.6).SetClip(renderClip(t, gomath.Circle(44)))
			}
			r, _ := render.NewRasterizer(c)
			got, err := r.Render(&gomath.Frame{Objects: []*gomath.Object{object}})
			if err != nil {
				t.Fatal(err)
			}
			file, err := os.Open("../tests/data/render/" + name + ".png")
			if err != nil {
				t.Fatal(err)
			}
			ref, err := png.Decode(file)
			file.Close()
			if err != nil {
				t.Fatal(err)
			}
			var sum int
			maximum, changed := 0, 0
			for y := range c.Height {
				for x := range c.Width {
					a, b := got.RGBAAt(x, y), color.RGBAModel.Convert(ref.At(x, y)).(color.RGBA)
					pixelChanged := false
					for _, pair := range [][2]uint8{{a.R, b.R}, {a.G, b.G}, {a.B, b.B}, {a.A, b.A}} {
						d := int(pair[0]) - int(pair[1])
						if d < 0 {
							d = -d
						}
						sum += d
						maximum = max(maximum, d)
						pixelChanged = pixelChanged || d > 8
					}
					if pixelChanged {
						changed++
					}
				}
			}
			mae := float64(sum) / float64(c.Width*c.Height*4)
			ratio := float64(changed) / float64(c.Width*c.Height)
			t.Logf("CairoSVG %s MAE %.6f/255 max %d fraction>8 %.6f", name, mae, maximum, ratio)
			if mae > .35 || maximum > 40 || ratio > .015 {
				t.Fatal("clip reference tolerance exceeded")
			}
			// Independently author camera/world transforms whose inverse returns
			// exactly the same intended viewport geometry and clip paths.
			object.ApplyMatrix(geom.Translation(geom.V(10, 20)).Mul(geom.RotationZ(math.Pi / 6)))
			camera := gomath.Camera2D{Center: geom.V(10, 20), Rotation: math.Pi / 6, FrameWidth: 192}
			moved, err := r.Render(&gomath.Frame{Objects: []*gomath.Object{object}, Camera: &camera})
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got.Pix, moved.Pix) {
				t.Fatal("camera projection altered clipping")
			}
		})
	}
}
