package rasterimage_test

import (
	"bytes"
	"encoding/binary"
	"gomath"
	"gomath/geom"
	"gomath/rasterimage"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func source() *image.NRGBA {
	img := image.NewNRGBA(image.Rect(10, 20, 12, 21))
	img.SetNRGBA(10, 20, color.NRGBA{R: 255, A: 128})
	img.SetNRGBA(11, 20, color.NRGBA{B: 255, A: 255})
	return img
}

func TestImmutableNormalizedAssetAndCopy(t *testing.T) {
	img := source()
	o, err := rasterimage.New(img, 4)
	if err != nil {
		t.Fatal(err)
	}
	if o.Bitmap.Bounds() != image.Rect(0, 0, 2, 1) || o.Bitmap.RGBAAt(0, 0) != (color.RGBA{R: 128, A: 128}) {
		t.Fatal("source was not normalized and premultiplied")
	}
	img.SetNRGBA(10, 20, color.NRGBA{G: 255, A: 255})
	copy := o.Copy().Shift(geom.V(3, 0))
	if copy.Bitmap != o.Bitmap || o.Bitmap.RGBAAt(0, 0).G != 0 || o.Center() != (geom.Vec3{}) || o.Bounds().Size() != (geom.Vec3{X: 4, Y: 2}) {
		t.Fatal("immutable sharing or geometry copy failed")
	}
}

func TestDecodeFormatsAndFile(t *testing.T) {
	for _, format := range []string{"png", "jpeg", "gif"} {
		t.Run(format, func(t *testing.T) {
			var encoded bytes.Buffer
			var err error
			switch format {
			case "png":
				err = png.Encode(&encoded, source())
			case "jpeg":
				err = jpeg.Encode(&encoded, source(), nil)
			case "gif":
				err = gif.Encode(&encoded, source(), nil)
			}
			if err != nil {
				t.Fatal(err)
			}
			o, err := rasterimage.Decode(bytes.NewReader(encoded.Bytes()), 2)
			if err != nil || o.Bitmap.Bounds().Size() != image.Pt(2, 1) {
				t.Fatal("decode", err)
			}
			filename := filepath.Join(t.TempDir(), "image."+format)
			if err := os.WriteFile(filename, encoded.Bytes(), 0644); err != nil {
				t.Fatal(err)
			}
			loaded, err := rasterimage.Load(filename, 3)
			if err != nil || loaded.Name != filename {
				t.Fatal("load", err)
			}
		})
	}
}

func TestInvalidImagesAndHeaderLimits(t *testing.T) {
	for _, width := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		if _, err := rasterimage.New(source(), width); err == nil {
			t.Fatal("invalid width accepted")
		}
	}
	if _, err := rasterimage.New(nil, 1); err == nil {
		t.Fatal("nil image accepted")
	}
	if _, err := gomath.NewImage(&gomath.Bitmap{}, 1); err == nil {
		t.Fatal("empty asset accepted")
	}
	if _, err := rasterimage.Decode(nil, 1); err == nil {
		t.Fatal("nil reader accepted")
	}
	if _, err := rasterimage.Decode(bytes.NewBufferString("not an image"), 1); err == nil {
		t.Fatal("bad format accepted")
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, source()); err != nil {
		t.Fatal(err)
	}
	data := buffer.Bytes()
	binary.BigEndian.PutUint32(data[16:20], uint32(gomath.MaxBitmapPixels))
	binary.BigEndian.PutUint32(data[20:24], 2)
	binary.BigEndian.PutUint32(data[29:33], crc32.ChecksumIEEE(data[12:29]))
	if _, err := rasterimage.Decode(bytes.NewReader(data), 1); err == nil {
		t.Fatal("oversized decoded dimensions accepted")
	}
}

func TestImageAnimationAndRejectedMorphs(t *testing.T) {
	o, err := rasterimage.New(source(), 2)
	if err != nil {
		t.Fatal(err)
	}
	s := gomath.NewScene()
	if err := s.Play(1, gomath.FadeIn(o).WithRate(gomath.Linear)); err != nil {
		t.Fatal(err)
	}
	if err := s.Play(1, gomath.Shift(o, geom.V(2, 0)).WithRate(gomath.Linear)); err != nil {
		t.Fatal(err)
	}
	for _, time := range []float64{1.5, .5, 1.5} {
		f, err := s.Sample(time)
		if err != nil {
			t.Fatal(err)
		}
		image := f.Find(o.ID)
		if image.Bitmap != o.Bitmap || image == o {
			t.Fatal("sampling broke image asset sharing")
		}
		if time == .5 && image.Style.Opacity != .5 {
			t.Fatal("fade wrong")
		}
		if time == 1.5 && math.Abs(image.Center().X-1) > 1e-10 {
			t.Fatal("shift wrong")
		}
		image.Rotate(.5)
	}
	other, _ := rasterimage.New(source(), 2)
	for _, animation := range []gomath.Animation{
		gomath.Transform(o, gomath.Circle(1)), gomath.Transform(o, other),
		gomath.Create(o), gomath.Write(gomath.Group(o.Copy())),
	} {
		before := s.Duration()
		if err := s.Play(1, animation); err == nil || s.Duration() != before {
			t.Fatal("unsupported image operation committed")
		}
	}
	// A newly added image child has a valid plane even at its transparent start.
	group := gomath.Group(gomath.Square(1))
	s2 := gomath.NewScene()
	if err := s2.Add(group); err != nil {
		t.Fatal(err)
	}
	target := group.Copy()
	target.Children = append(target.Children, o.Copy())
	if err := s2.Play(1, gomath.Transform(group, target)); err != nil {
		t.Fatal(err)
	}
	frame, err := s2.Sample(.5)
	if err != nil || frame.Objects[0].Validate() != nil {
		t.Fatal("new image child invalid", err)
	}
}
