package export_test

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"gomath"
	"gomath/export"
	"gomath/geom"
	"gomath/rasterimage"
	"gomath/render"
	"image"
	"image/color"
	"image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSVGEmbedsBitmapWithAffineTransform(t *testing.T) {
	pixels := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	pixels.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 128})
	pixels.SetNRGBA(1, 0, color.NRGBA{B: 255, A: 255})
	o, err := rasterimage.New(pixels, 4)
	if err != nil {
		t.Fatal(err)
	}
	o.Rotate(.3).Shift(geom.V(1, 2)).SetOpacity(.5)
	c := render.DefaultConfig()
	c.Width, c.Height = 640, 360
	file := filepath.Join(t.TempDir(), "image.svg")
	if err := export.SVG(file, &gomath.Frame{Objects: []*gomath.Object{o}}, c); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	found := false
	for {
		token, err := decoder.Token()
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
		found = true
		attrs := map[string]string{}
		for _, a := range start.Attr {
			attrs[a.Name.Local] = a.Value
		}
		if attrs["opacity"] != "0.5" || attrs["width"] != "2" || attrs["height"] != "1" {
			t.Fatal("wrong image metadata", attrs)
		}
		encoded := strings.TrimPrefix(attrs["href"], "data:image/png;base64,")
		pngData, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := png.Decode(bytes.NewReader(pngData))
		if err != nil || decoded.Bounds() != pixels.Bounds() {
			t.Fatal("embedded image invalid", err)
		}
		pr, pg, pb, pa := decoded.At(0, 0).RGBA()
		er, eg, eb, ea := o.Bitmap.At(0, 0).RGBA()
		if pr != er || pg != eg || pb != eb || pa != ea {
			t.Fatal("embedded pixels changed")
		}
		var a, b, cc, d, e, f float64
		if n, err := fmt.Sscanf(attrs["transform"], "matrix(%f %f %f %f %f %f)", &a, &b, &cc, &d, &e, &f); err != nil || n != 6 {
			t.Fatal("invalid image transform", err)
		}
		tl, tr, bl, err := o.ImageCorners()
		if err != nil {
			t.Fatal(err)
		}
		scale := float64(c.Width) / c.FrameWidth
		for _, point := range []struct{ x, y, worldX, worldY float64 }{
			{0, 0, tl.X, tl.Y}, {2, 0, tr.X, tr.Y}, {0, 1, bl.X, bl.Y},
		} {
			x, y := a*point.x+cc*point.y+e, b*point.x+d*point.y+f
			if math.Abs(x-(320+point.worldX*scale)) > 1e-5 || math.Abs(y-(180-point.worldY*scale)) > 1e-5 {
				t.Fatal("SVG image transform differs from raster geometry")
			}
		}
	}
	if !found {
		t.Fatal("bitmap silently omitted")
	}
	bad := o.Copy().SetStroke(gomath.White, .1)
	badFile := filepath.Join(t.TempDir(), "bad.svg")
	if export.SVG(badFile, &gomath.Frame{Objects: []*gomath.Object{bad}}, c) == nil {
		t.Fatal("unsupported image style accepted")
	}
	if _, err := os.Stat(badFile); !os.IsNotExist(err) {
		t.Fatal("failed SVG was published")
	}
}
