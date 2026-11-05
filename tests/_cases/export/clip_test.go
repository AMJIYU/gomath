package export_test

import (
	"encoding/xml"
	"gomath"
	"gomath/export"
	"gomath/geom"
	"gomath/path"
	"gomath/render"
	"image"
	"image/color"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func exportClip(t *testing.T, object *gomath.Object) *gomath.Clip {
	t.Helper()
	clip, err := gomath.NewClip(object.Path)
	if err != nil {
		t.Fatal(err)
	}
	return clip
}

func TestSVGClipsSurviveCameraZOrderBitmapAndSharedMasks(t *testing.T) {
	outer := exportClip(t, gomath.Square(4))
	inner := exportClip(t, gomath.Circle(1))
	left := gomath.Square(3).SetFill(gomath.Blue).SetClip(inner)
	left.Name, left.ZIndex = "left", 9
	asset, err := gomath.NewBitmap(image.NewUniform(color.NRGBA{R: 255, A: 255}))
	// Uniform images have effectively unbounded bounds, so use a finite asset.
	if err == nil || asset != nil {
		t.Fatal("unbounded bitmap accepted")
	}
	pixels := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	pixels.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	asset, err = gomath.NewBitmap(pixels)
	if err != nil {
		t.Fatal(err)
	}
	picture, err := gomath.NewImage(asset, 3)
	if err != nil {
		t.Fatal(err)
	}
	picture.Name, picture.ZIndex = "picture", 2
	picture.SetClip(inner)
	group := gomath.Group(left, picture).SetClip(outer)
	group.Name = "group"
	group.Shift(geom.V(2, 3))
	camera := gomath.Camera2D{Center: geom.V(2, 3), FrameWidth: 8}
	c := render.DefaultConfig()
	c.Width, c.Height, c.FrameWidth = 160, 80, 16
	frame := &gomath.Frame{Objects: []*gomath.Object{group}, Camera: &camera}
	file := filepath.Join(t.TempDir(), "clipped.svg")
	if err = export.SVG(file, frame, c); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, `clipPathUnits="userSpaceOnUse"`) || !strings.Contains(content, `clip-rule="nonzero"`) || !strings.Contains(content, `scale(20 -20)`) {
		t.Fatal("clip coordinates or rule absent", content)
	}
	if strings.Index(content, `aria-label="picture"`) > strings.Index(content, `aria-label="left"`) {
		t.Fatal("clip wrapper changed Z order")
	}
	decoder := xml.NewDecoder(strings.NewReader(content))
	var clips, stack []string
	painted := 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal("invalid nested SVG", err)
		}
		switch token := token.(type) {
		case xml.StartElement:
			stack = append(stack, token.Name.Local)
			if token.Name.Local == "clipPath" {
				for _, a := range token.Attr {
					if a.Name.Local == "id" {
						clips = append(clips, a.Value)
					}
				}
			}
			for _, a := range token.Attr {
				if a.Name.Local == "aria-label" && (a.Value == "left" || a.Value == "picture") {
					painted++
					if len(stack) < 3 || stack[len(stack)-2] != "g" || stack[len(stack)-3] != "g" {
						t.Fatal("leaf lost ancestor clip intersection")
					}
				}
			}
		case xml.EndElement:
			stack = stack[:len(stack)-1]
		}
	}
	if len(clips) != 3 || painted != 2 {
		t.Fatal("missing independent transformed clip definitions/leaves", clips, painted)
	}
	if left.Clip.Bounds().Center().X != 2 || frame.Camera == nil {
		t.Fatal("SVG projection mutated source")
	}
}

func TestSVGEmptyClipDepthAndInvalidClipAreExplicit(t *testing.T) {
	c := render.DefaultConfig()
	empty, err := gomath.NewClip(path.Path{})
	if err != nil {
		t.Fatal(err)
	}
	o := gomath.Square(1).SetClip(empty)
	file := filepath.Join(t.TempDir(), "empty.svg")
	if err = export.SVG(file, &gomath.Frame{Objects: []*gomath.Object{o}}, c); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(file)
	if !strings.Contains(string(data), `d=""/></clipPath>`) {
		t.Fatal("empty clip silently removed")
	}
	for range gomath.MaxClipDepth {
		o = gomath.Group(o).SetClip(empty)
	}
	file = filepath.Join(t.TempDir(), "invalid.svg")
	if err = export.SVG(file, &gomath.Frame{Objects: []*gomath.Object{o}}, c); err == nil {
		t.Fatal("excess clip nesting exported")
	}
	if _, err = os.Stat(file); !os.IsNotExist(err) {
		t.Fatal("failed SVG published output", err)
	}
	o = gomath.Square(1).SetClip(&gomath.Clip{})
	if err = export.SVG(file, &gomath.Frame{Objects: []*gomath.Object{o}}, c); err == nil {
		t.Fatal("invalid clip exported")
	}
}
